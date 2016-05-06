package supervisor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/factory"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type HyperPod struct {
	Containers map[string]*Container
	Processes  map[string]*Process

	userPod   *pod.UserPod
	podStatus *hypervisor.PodStatus
	vm        *hypervisor.Vm
	sv        *Supervisor
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
		glog.V(1).Infof("StartPod fail: QEMU response data is nil\n")
		return nil, fmt.Errorf("StartPod fail")
	}
	glog.V(1).Infof("result: code %d %s\n", Response.Code, Response.Cause)

	return &HyperPod{
		userPod:    userPod,
		podStatus:  podStatus,
		vm:         vm,
		Containers: make(map[string]*Container),
		Processes:  make(map[string]*Process),
	}, nil
}

func (hp *HyperPod) reap() {
	Response := hp.vm.StopPod(hp.podStatus, "yes")
	if Response.Data == nil {
		glog.V(1).Infof("StopPod fail: QEMU response data is nil\n")
		return
	}
	glog.V(1).Infof("result: code %d %s\n", Response.Code, Response.Cause)
	os.RemoveAll(filepath.Join(hypervisor.BaseDir, hp.vm.Id))
}
