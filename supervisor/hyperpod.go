package supervisor

import (
	"encoding/gob"
	"fmt"
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
	"github.com/hyperhq/runv/hypervisor"
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
	Addr netlink.AddrUpdate `json:"addrUpdate"`
	// RouteUpdate is used to pass information back from RouteSubscribe()
	Route netlink.RouteUpdate `json:"routeUpdate"`
	// Veth is used to pass information back from LinkSubscribe().
	// We only support veth link at present.
	Veth *netlink.Veth `json:"veth"`

	// UpdateType indicates which part of the netlink information has been changed.
	UpdateType NetlinkUpdateType `json:"updateType"`
}

type HyperPod struct {
	Containers map[string]*Container
	Processes  map[string]*Process

	//userPod   *pod.UserPod
	//podStatus *hypervisor.PodStatus
	vm            *hypervisor.Vm
	sv            *Supervisor
	NsListenerPid int
}

type InterfaceInfo struct {
	Index     int
	PeerIndex int
	Ip        string
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

func (hp *HyperPod) HandleNetlinkUpdate(nlUpdate *NetlinkUpdate) error {
	// Keep watching container network setting
	// and then update vm/hyperstart
	var err error
	glog.V(3).Infof("network namespace information of %s has been changed: %#v", nlUpdate.UpdateType, nlUpdate)
	switch nlUpdate.UpdateType {
	case UpdateTypeLink:
		link := nlUpdate.Veth
		if link.Attrs().ParentIndex == 0 {
			glog.V(3).Infof("The deleted link: %s", link)
			err = hp.vm.DeleteNic(strconv.Itoa(link.Attrs().Index))
			if err != nil {
				return err
			}

		} else {
			glog.V(3).Infof("The changed link: %v", link)
		}

	case UpdateTypeAddr:
		glog.V(3).Infof("The changed address: %v", nlUpdate.Addr)

		link := nlUpdate.Veth

		// If there is a delete operation upon an link, it will also trigger
		// the address change event which the link will be NIL since it has
		// already been deleted before the address change event be triggered.
		if link == nil {
			glog.V(3).Infof("Link for this address has already been deleted.")
			break
		}

		// This is just a sanity check.
		//
		// The link should be the one which the address on it has been changed.
		if link.Attrs().Index != nlUpdate.Addr.LinkIndex {
			glog.Errorf("Get the wrong link with ID %d, expect %d", link.Attrs().Index, nlUpdate.Addr.LinkIndex)
			break
		}

		bridge, options, err := GetBridgeFromIndex(link.Attrs().ParentIndex)
		if err != nil {
			return err
		}

		inf := &api.InterfaceDescription{
			Id:      strconv.Itoa(link.Attrs().Index),
			Lo:      false,
			Bridge:  bridge,
			Ip:      nlUpdate.Addr.LinkAddress.String(),
			Options: options,
		}

		err = hp.vm.AddNic(inf)
		if err != nil {
			return err
		}

	case UpdateTypeRoute:
	}
	return nil
}

func newPipe() (parent, child *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[1]), "parent"), os.NewFile(uintptr(fds[0]), "child"), nil
}

func (hp *HyperPod) getNsPid() int {
	return hp.NsListenerPid
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

	if err := os.RemoveAll(filepath.Join(hypervisor.BaseDir, hp.vm.Id)); err != nil {
		glog.Errorf("can't remove vm dir %q: %v", filepath.Join(hypervisor.BaseDir, hp.vm.Id), err)
	}
	glog.Flush()
}
