package supervisor

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/factory"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/kardianos/osext"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/vishvananda/netlink"
)

type NetlinkUpdateType string

const (
	UpdateTypeLink  NetlinkUpdateType = "link"
	UpdateTypeAddr  NetlinkUpdateType = "addr"
	UpdateTypeRoute NetlinkUpdateType = "route"
)

// NetlinkUpdate tracks the change of network namespace.
type NetlinkUpdate struct {
	// AddrUpdate is used to pass information back from AddrSubscribe()
	Addr netlink.AddrUpdate
	// RouteUpdate is used to pass information back from RouteSubscribe()
	Route netlink.RouteUpdate
	// We only support veth link at present.
	Link netlink.LinkUpdate

	// UpdateType indicates which part of the netlink information has been changed.
	UpdateType NetlinkUpdateType
}

type HyperPod struct {
	Containers map[string]*Container
	Processes  map[string]*Process

	//userPod   *pod.UserPod
	//podStatus *hypervisor.PodStatus
	vm *hypervisor.Vm
	sv *Supervisor

	nslistener *nsListener

	// networkInited indicates whether the network namespace has already been set or not.
	networkInited bool
}

type InterfaceInfo struct {
	Index     int
	PeerIndex int
	Ip        string
	Mac       string
}

type nsListener struct {
	enc *gob.Encoder
	dec *gob.Decoder
	cmd *exec.Cmd
}

func GetBridgeFromIndex(idx int) (string, string, error) {
	var attr, bridge *netlink.LinkAttrs
	var options string

	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
		return "", "", err
	}

	for _, link := range links {
		if link.Type() != "veth" {
			continue
		}

		if link.Attrs().Index == idx {
			attr = link.Attrs()
			break
		}
	}

	if attr == nil {
		return "", "", fmt.Errorf("cann't find nic whose ifindex is %d", idx)
	}

	for _, link := range links {
		if link.Type() != "bridge" && link.Type() != "openvswitch" {
			continue
		}

		if link.Attrs().Index == attr.MasterIndex {
			bridge = link.Attrs()
			break
		}
	}

	if bridge == nil {
		return "", "", fmt.Errorf("cann't find bridge contains nic whose ifindex is %d", idx)
	}

	if bridge.Name == "ovs-system" {
		veth, err := netlink.LinkByIndex(idx)
		if err != nil {
			return "", "", err
		}

		out, err := exec.Command("ovs-vsctl", "port-to-br", veth.Attrs().Name).CombinedOutput()
		if err != nil {
			return "", "", err
		}
		bridge.Name = strings.TrimSpace(string(out))

		out, err = exec.Command("ovs-vsctl", "get", "port", veth.Attrs().Name, "tag").CombinedOutput()
		if err != nil {
			return "", "", err
		}
		options = "tag=" + strings.TrimSpace(string(out))
	}

	glog.V(3).Infof("find bridge %s", bridge.Name)

	return bridge.Name, options, nil
}

func (hp *HyperPod) initPodNetwork(c *Container) error {
	// Only start first container will setup netns
	if hp.networkInited == true {
		return nil
	}

	// container has no prestart hooks, means no net for this container
	if c.Spec.Hooks == nil || len(c.Spec.Hooks.Prestart) == 0 {
		// FIXME: need receive interface settting?
		return nil
	}

	listener := hp.nslistener

	/* send collect netns request to nsListener */
	if err := listener.enc.Encode("init"); err != nil {
		glog.Errorf("listener.dec.Decode init error: %v", err)
		return err
	}

	infos := []InterfaceInfo{}
	/* read nic information of ns from pipe */
	err := listener.dec.Decode(&infos)
	if err != nil {
		glog.Error("listener.dec.Decode infos error: %v", err)
		return err
	}

	routes := []netlink.Route{}
	err = listener.dec.Decode(&routes)
	if err != nil {
		glog.Error("listener.dec.Decode route error: %v", err)
		return err
	}

	var gw_route *netlink.Route
	for idx, route := range routes {
		if route.Dst == nil {
			gw_route = &routes[idx]
		}
	}

	glog.V(3).Infof("interface configuration of pod ns is %#v", infos)
	for _, info := range infos {
		bridge, options, err := GetBridgeFromIndex(info.PeerIndex)
		if err != nil {
			glog.Error(err)
			continue
		}

		nicId := strconv.Itoa(info.Index)

		conf := &api.InterfaceDescription{
			Id:      nicId, // link index as an id
			Lo:      false,
			Bridge:  bridge,
			Ip:      []string{info.Ip},
			Mac:     info.Mac,
			Options: options,
		}

		if gw_route != nil && gw_route.LinkIndex == info.Index {
			conf.Gw = gw_route.Gw.String()
		}

		// TODO(hukeping): the name here is always eth1, 2, 3, 4, 5, etc.,
		// which would not be the proper way to name device name, instead it
		// should be the same as what we specified in the network namespace.
		//err = hp.vm.AddNic(info.Index, fmt.Sprintf("eth%d", idx), conf)
		err = hp.vm.AddNic(conf)
		if err != nil {
			glog.Error(err)
			return err
		}
	}

	err = hp.vm.AddDefaultRoute()
	if err != nil {
		glog.Error(err)
		return err
	}

	hp.networkInited = true

	go hp.nsListenerStrap()

	return nil
}

func (hp *HyperPod) handleLinkUpdate(update *NetlinkUpdate) {
	link := update.Link

	if link.Link == nil {
		glog.Errorf("Link update must have an non-nil Link param!")
		return
	}

	stringid := fmt.Sprintf("%d", link.Attrs().Index)
	infCreated, err := hp.vm.GetNic(stringid)
	if err != nil {
		glog.Error(err)
		return
	}

	// ParentIndex==0 means link is to be deleted
	if link.Attrs().ParentIndex == 0 {
		if err := hp.vm.DeleteNic(stringid); err != nil {
			glog.Errorf("[netns] delete nic failed: %v", err)
			return
		}
		glog.V(3).Infof("interface %s deleted", stringid)
		return
	}

	if link.IfInfomsg.Flags&syscall.IFF_UP == 1 {
		if infCreated.Mtu != uint64(link.Attrs().MTU) {
			glog.V(3).Infof("[netns] MTU changed from %d to %d\n", infCreated.Mtu, link.Attrs().MTU)
			if err := hp.vm.UpdateMtu(stringid, uint64(link.Attrs().MTU)); err != nil {
				glog.Error("failed to set mtu to %d: %v", uint64(link.Attrs().MTU), err)
			}
		}
	}
}

func (hp *HyperPod) handleAddrUpdate(update *NetlinkUpdate) {
	link := update.Link

	// If there is a delete operation upon an link, it will also trigger
	// the address change event which the link will be NIL since it has
	// already been deleted before the address change event be triggered.
	if link.Link == nil {
		glog.V(3).Infof("Link for this address has already been deleted.")
		return
	}

	stringid := fmt.Sprintf("%d", link.Attrs().Index)
	_, getNicErr := hp.vm.GetNic(stringid)

	// true=added false=deleted
	if update.Addr.NewAddr == false {
		// if we want to delete ip (or remove the whole nic), make sure it exist
		if getNicErr != nil {
			glog.Errorf("failed to get nic %q: %v", stringid, getNicErr)
			return
		}

		// remove single ip address
		if err := hp.vm.DeleteIPAddr(stringid, update.Addr.LinkAddress.String()); err != nil {
			glog.Errorf("[netns] delete ip address failed: %v", err)
			return
		}
		glog.V(3).Infof("IP(%q) of nic %s deleted", update.Addr.LinkAddress.String(), stringid)
		return
	}

	// This is just a sanity check.
	//
	// The link should be the one which the address on it has been changed.
	if link.Attrs().Index != update.Addr.LinkIndex {
		glog.Errorf("Get the wrong link with ID %d, expect %d", link.Attrs().Index, update.Addr.LinkIndex)
		return
	}

	if getNicErr != nil {
		if getNicErr != hypervisor.ErrNoSuchInf {
			glog.Error(getNicErr)
			return
		}

		// add a new interface
		bridge, options, err := GetBridgeFromIndex(link.Attrs().ParentIndex)
		if err != nil {
			glog.Error(err)
			return
		}

		inf := &api.InterfaceDescription{
			Id:      strconv.Itoa(link.Attrs().Index),
			Lo:      false,
			Bridge:  bridge,
			Ip:      []string{update.Addr.LinkAddress.String()},
			Mac:     link.Attrs().HardwareAddr.String(),
			Mtu:     uint64(link.Attrs().MTU),
			Options: options,
		}

		err = hp.vm.AddNic(inf)
		if err != nil {
			glog.Error(err)
			return
		}
	} else {
		// interface exists, we only want to add a new IP address
		if err := hp.vm.AddIPAddr(stringid, update.Addr.LinkAddress.String()); err != nil {
			glog.Error("failed to add ip address %q to network: %v", err)
			return
		}
	}
}

func (hp *HyperPod) handleRouteUpdate(update *NetlinkUpdate) {
	route := update.Route
	if route.Type == syscall.RTM_DELROUTE || route.Type == syscall.RTM_GETROUTE {
		glog.V(3).Infof("currently we only support adding new route, delete or query isn't supported")
		return
	}

	stringid := fmt.Sprintf("%d", route.LinkIndex)
	infCreated, err := hp.vm.GetNic(stringid)
	if err != nil {
		glog.Errorf("failed to get information of nic with id %d", route.LinkIndex)
		return
	}

	r := hyperstartapi.Route{
		Dest:    route.Dst.String(),
		Gateway: route.Gw.String(),
		Device:  infCreated.DeviceName,
	}
	if err = hp.vm.AddRoute([]hyperstartapi.Route{r}); err != nil {
		glog.Errorf("failed to add route: %v", err)
		return
	}
}

func (hp *HyperPod) nsListenerStrap() {
	listener := hp.nslistener

	// Keep watching container network setting
	// and then update vm/hyperstart
	for {
		update := NetlinkUpdate{}
		err := listener.dec.Decode(&update)
		if err != nil {
			if err == io.EOF {
				glog.V(3).Infof("listener.dec.Decode NetlinkUpdate: %v", err)
				break
			}
			glog.Error("listener.dec.Decode NetlinkUpdate error: %v", err)
			continue
		}

		glog.V(3).Infof("network namespace information of %s has been changed", update.UpdateType)
		switch update.UpdateType {
		case UpdateTypeRoute:
			glog.V(3).Infof("[netns] route has been changed: %#v", update.Route)
			hp.handleRouteUpdate(&update)
		case UpdateTypeLink:
			glog.V(3).Infof("[netns] link has been changed: %#v", update.Link)
			hp.handleLinkUpdate(&update)
		case UpdateTypeAddr:
			glog.V(3).Infof("[netns] address has been changed: %#v", update.Addr)
			hp.handleAddrUpdate(&update)
		}
	}
}

func newPipe() (parent, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func (hp *HyperPod) startNsListener() (err error) {
	var parentPipe, childPipe *os.File
	var path string
	if hp.nslistener != nil {
		return nil
	}

	path, err = osext.Executable()
	if err != nil {
		glog.Errorf("cannot find self executable path for %s: %v", os.Args[0], err)
		return err
	}

	glog.V(3).Infof("get exec path %s", path)
	parentPipe, childPipe, err = newPipe()
	if err != nil {
		glog.Errorf("create pipe for containerd-nslistener failed: %v", err)
		return err
	}

	defer func() {
		if err != nil {
			parentPipe.Close()
			childPipe.Close()
		}
	}()

	cmd := exec.Command(path)
	cmd.Args[0] = "containerd-nslistener"
	cmd.ExtraFiles = append(cmd.ExtraFiles, childPipe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err = cmd.Start(); err != nil {
		glog.Errorf("start containerd-nslistener failed: %v", err)
		return err
	}

	childPipe.Close()

	enc := gob.NewEncoder(parentPipe)
	dec := gob.NewDecoder(parentPipe)
	gob.Register(&netlink.Veth{})

	hp.nslistener = &nsListener{
		enc: enc,
		dec: dec,
		cmd: cmd,
	}

	defer func() {
		if err != nil {
			hp.stopNsListener()
		}
	}()

	/* Make sure nsListener create new netns */
	var ready string
	if err = dec.Decode(&ready); err != nil {
		glog.Errorf("Get ready message from containerd-nslistener failed: %v", err)
		return err
	}

	if ready != "init" {
		err = fmt.Errorf("containerd get incorrect init message: %s", ready)
		return err
	}

	glog.V(1).Infof("nsListener pid is %d", hp.getNsPid())
	return nil
}

func (hp *HyperPod) stopNsListener() {
	if hp.nslistener != nil {
		hp.nslistener.cmd.Process.Kill()
	}
}

func (hp *HyperPod) getNsPid() int {
	if hp.nslistener == nil {
		return -1
	}

	return hp.nslistener.cmd.Process.Pid
}

func (hp *HyperPod) createContainer(container, bundlePath, stdin, stdout, stderr string, spec *specs.Spec) (*Container, error) {
	inerProcessId := container + "-init"
	if _, ok := hp.Processes[inerProcessId]; ok {
		return nil, fmt.Errorf("The process id: %s is in used", inerProcessId)
	}

	c := &Container{
		Id:         container,
		BundlePath: bundlePath,
		Spec:       spec,
		Processes:  make(map[string]*Process),
		ownerPod:   hp,
	}
	hp.Containers[container] = c
	p := &Process{
		Id:     "init",
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Spec:   &spec.Process,
		ProcId: c.ownerPod.getNsPid(),

		inerId:    inerProcessId,
		ownerCont: c,
		init:      true,
	}
	c.Processes["init"] = p
	hp.Processes[inerProcessId] = p
	return c, nil
}

func chooseKernel(spec *specs.Spec) (kernel string) {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.kernel" {
			kernel = slices[1]
			// remove kernel env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			break
		}
	}
	return
}

func chooseInitrd(spec *specs.Spec) (initrd string) {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.initrd" {
			initrd = slices[1]
			// remove kernel env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			break
		}
	}
	return
}

func createHyperPod(f factory.Factory, spec *specs.Spec, defaultCpus int, defaultMemory int) (*HyperPod, error) {
	cpu := defaultCpus
	mem := defaultMemory
	if spec.Linux != nil && spec.Linux.Resources != nil && spec.Linux.Resources.Memory != nil && spec.Linux.Resources.Memory.Limit != nil {
		mem = int(*spec.Linux.Resources.Memory.Limit >> 20)
	}

	kernel := chooseKernel(spec)
	initrd := chooseInitrd(spec)
	glog.V(3).Infof("Using kernel: %s; Initrd: %s; vCPU: %d; Memory %d", kernel, initrd, cpu, mem)

	var (
		vm  *hypervisor.Vm
		err error
	)
	if len(kernel) == 0 && len(initrd) == 0 {
		vm, err = f.GetVm(cpu, mem)
		if err != nil {
			glog.Errorf("Create VM failed with default kernel config: %v", err)
			return nil, err
		}
		glog.V(3).Infof("Creating VM with default kernel config")
	} else if len(kernel) == 0 || len(initrd) == 0 {
		// if user specify a kernel, they must specify an initrd at the same time
		return nil, fmt.Errorf("You must specify an initrd if you specify a kernel, or vice-versa")
	} else {
		boot := &hypervisor.BootConfig{
			CPU:    cpu,
			Memory: mem,
			Kernel: kernel,
			Initrd: initrd,
		}

		vm, err = hypervisor.GetVm("", boot, true)
		if err != nil {
			glog.Errorf("Create VM failed: %v", err)
			return nil, err
		}
		glog.V(3).Infof("Creating VM with specific kernel config")
	}

	r := make(chan api.Result, 1)
	go func() {
		r <- vm.WaitInit()
	}()

	sandbox := api.SandboxInfoFromOCF(spec)
	vm.InitSandbox(sandbox)

	rsp := <-r

	if !rsp.IsSuccess() {
		vm.Kill()
		glog.Errorf("StartPod fail, response: %#v", rsp)
		return nil, fmt.Errorf("StartPod fail")
	}
	glog.V(3).Infof("%s init sandbox successfully", rsp.ResultId())

	hp := &HyperPod{
		vm:         vm,
		Containers: make(map[string]*Container),
		Processes:  make(map[string]*Process),
	}

	// create Listener process running in its own netns
	if err = hp.startNsListener(); err != nil {
		hp.reap()
		glog.Errorf("start ns listener fail: %v", err)
		return nil, err
	}

	return hp, nil
}

func (hp *HyperPod) reap() {
	result := make(chan api.Result, 1)
	go func() {
		result <- hp.vm.Shutdown()
	}()
	select {
	case rsp, ok := <-result:
		if !ok || !rsp.IsSuccess() {
			glog.Errorf("StopPod fail: chan: %v, response: %v", ok, rsp)
			break
		}
		glog.V(1).Infof("StopPod successfully")
	case <-time.After(time.Second * 60):
		glog.Errorf("StopPod timeout")
	}

	hp.stopNsListener()
	if err := os.RemoveAll(filepath.Join(hypervisor.BaseDir, hp.vm.Id)); err != nil {
		glog.Errorf("can't remove vm dir %q: %v", filepath.Join(hypervisor.BaseDir, hp.vm.Id), err)
	}
	glog.Flush()
}
