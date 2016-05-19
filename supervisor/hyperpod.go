package supervisor

import (
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/kardianos/osext"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/vishvananda/netlink"
)

type HyperPod struct {
	Containers map[string]*Container
	Processes  map[string]*Process

	userPod   *pod.UserPod
	podStatus *hypervisor.PodStatus
	vm        *hypervisor.Vm
	sv        *Supervisor

	nslistener *nsListener
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

func GetBridgeFromIndex(idx int) (string, error) {
	var attr, bridge *netlink.LinkAttrs

	links, err := netlink.LinkList()
	if err != nil {
		glog.Error(err)
		return "", err
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
		return "", fmt.Errorf("cann't find nic whose ifindex is %d", idx)
	}

	for _, link := range links {
		if link.Type() != "bridge" {
			continue
		}

		if link.Attrs().Index == attr.MasterIndex {
			bridge = link.Attrs()
			break
		}
	}

	if bridge == nil {
		return "", fmt.Errorf("cann't find bridge contains nic whose ifindex is %d", idx)
	}

	glog.Infof("find bridge %s", bridge.Name)

	return bridge.Name, nil
}

func (hp *HyperPod) initPodNetwork(c *Container) error {
	// Only start first container will setup netns
	if len(hp.Containers) > 1 {
		return nil
	}

	// container has no prestart hooks, means no net for this container
	if len(c.Spec.Hooks.Prestart) == 0 {
		// FIXME: need receive interface settting?
		return nil
	}

	listener := hp.nslistener

	/* send collect netns request to nsListener */
	if err := listener.enc.Encode("init"); err != nil {
		return err
	}

	infos := []InterfaceInfo{}
	/* read nic information of ns from pipe */
	err := listener.dec.Decode(&infos)
	if err != nil {
		return err
	}

	glog.Infof("interface configuration of pod ns is %v", infos)
	for idx, info := range infos {
		bridge, err := GetBridgeFromIndex(info.PeerIndex)
		if err != nil {
			glog.Error(err)
			continue
		}
		conf := pod.UserInterface{
			Bridge: bridge,
			Ip:     info.Ip,
		}

		err = hp.vm.AddNic(info.Index, fmt.Sprintf("eth%d", idx), conf)
		if err != nil {
			glog.Error(err)
			return err
		}
	}
	/*
		go func() {
			// watching container network setting, update vm/hyperstart
		}()
	*/

	return nil
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
		glog.Errorf("cannot find self executable path for %s: %v\n", os.Args[0], err)
		return err
	}

	glog.Infof("get exec path %s", path)
	parentPipe, childPipe, err = newPipe()
	if err != nil {
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
	if err = cmd.Start(); err != nil {
		glog.Error(err)
		return err
	}

	childPipe.Close()

	enc := gob.NewEncoder(parentPipe)
	dec := gob.NewDecoder(parentPipe)

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
		return err
	}

	if ready != "init" {
		err = fmt.Errorf("containerd get incorrect init message: %s", ready)
		return err
	}

	glog.Infof("nsListener pid is %d", hp.getNsPid())
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
	glog.Infof("createContainer()")

	c := &Container{
		Id:         container,
		BundlePath: bundlePath,
		Spec:       spec,
		Processes:  make(map[string]*Process),
		ownerPod:   hp,
	}

	p := &Process{
		Id:     "init",
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Spec:   &spec.Process,
		ProcId: -1,

		inerId:    inerProcessId,
		ownerCont: c,
		init:      true,
	}
	err := p.setupIO()
	if err != nil {
		return nil, err
	}
	glog.Infof("createContainer()")

	c.Processes["init"] = p
	c.ownerPod.Processes[inerProcessId] = p
	c.ownerPod.Containers[container] = c

	glog.Infof("createContainer() calls c.run(p)")
	c.run(p)
	return c, nil
}

func createHyperPod(f factory.Factory, spec *specs.Spec) (*HyperPod, error) {
	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	userPod := pod.ConvertOCF2PureUserPod(spec)
	podStatus := hypervisor.NewPod(podId, userPod)

	cpu := 1
	if userPod.Resource.Vcpu > 0 {
		cpu = userPod.Resource.Vcpu
	}
	mem := 128
	if userPod.Resource.Memory > 0 {
		mem = userPod.Resource.Memory
	}
	vm, err := f.GetVm(cpu, mem)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return nil, err
	}

	Response := vm.StartPod(podStatus, userPod, nil, nil)
	if Response.Data == nil {
		vm.Kill()
		glog.V(1).Infof("StartPod fail: QEMU response data is nil\n")
		return nil, fmt.Errorf("StartPod fail")
	}
	glog.V(1).Infof("result: code %d %s\n", Response.Code, Response.Cause)

	hp := &HyperPod{
		userPod:    userPod,
		podStatus:  podStatus,
		vm:         vm,
		Containers: make(map[string]*Container),
		Processes:  make(map[string]*Process),
	}

	// create Listener process running in its own netns
	if err = hp.startNsListener(); err != nil {
		hp.reap()
		glog.V(1).Infof("start ns listener fail: %s\n", err.Error())
		return nil, err
	}

	return hp, nil
}

func (hp *HyperPod) reap() {
	Response := hp.vm.StopPod(hp.podStatus)
	if Response.Data == nil {
		glog.V(1).Infof("StopPod fail: QEMU response data is nil\n")
		return
	}
	hp.stopNsListener()
	glog.V(1).Infof("result: code %d %s\n", Response.Code, Response.Cause)
	os.RemoveAll(filepath.Join(hypervisor.BaseDir, hp.vm.Id))
}
