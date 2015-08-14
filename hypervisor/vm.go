package hypervisor

import (
	"errors"
	"fmt"
	"io"

	"encoding/json"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

type Vm struct {
	Id              string
	Pod             *Pod
	Status          uint
	Cpu             int
	Mem             int
	Lazy            bool
	Keep            int
	VmChan          interface{}
	VmClientChan    interface{}
	SubVmClientChan interface{}
}

func (vm *Vm) GetVmChan() (interface{}, interface{}, interface{}, error) {
	if vm.VmChan != nil && vm.VmClientChan != nil {
		return vm.VmChan, vm.VmClientChan, vm.SubVmClientChan, nil
	}
	return nil, nil, nil, fmt.Errorf("Can not find the VM chan for pod: %s!", vm.Id)
}

func (vm *Vm) SetVmChan(vmchan, vmclient, subVmClient interface{}) error {
	if vm.VmChan == nil {
		if vmchan != nil {
			vm.VmChan = vmchan
		}
		if vmclient != nil {
			vm.VmClientChan = vmclient
		}
		if subVmClient != nil {
			vm.SubVmClientChan = subVmClient
		}
		return nil
	}
	return fmt.Errorf("Already setup chan for vm: %s!", vm.Id)
}

func (vm *Vm) Launch(b *BootConfig) (err error) {
	var (
		PodEvent  = make(chan VmEvent, 128)
		Status    = make(chan *types.VmResponse, 128)
		subStatus = make(chan *types.VmResponse, 128)
	)

	if vm.Lazy {
		go LazyVmLoop(vm.Id, PodEvent, Status, b, vm.Keep)
	} else {
		go VmLoop(vm.Id, PodEvent, Status, b, vm.Keep)
	}

	if err := vm.SetVmChan(PodEvent, Status, subStatus); err != nil {
		glog.V(1).Infof("SetVmChan error: %s", err.Error())
		return err
	}

	return nil
}

func (vm *Vm) Kill() (int, string, error) {
	PodEvent, Status, subStatus, err := vm.GetVmChan()
	if err != nil {
		return -1, "", err
	}
	var Response *types.VmResponse
	shutdownPodEvent := &ShutdownCommand{Wait: false}
	PodEvent.(chan VmEvent) <- shutdownPodEvent
	// wait for the VM response
	for {
		stop := 0
		select {
		case Response = <-Status.(chan *types.VmResponse):
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
			if Response.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		case Response = <-subStatus.(chan *types.VmResponse):
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
			if Response.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		}
		if stop == 1 {
			break
		}
	}
	close(Status.(chan *types.VmResponse))
	close(subStatus.(chan *types.VmResponse))

	return Response.Code, Response.Cause, nil
}

// This function will only be invoked during daemon start
func (vm *Vm) AssociateVm(mypod *Pod, data []byte) error {
	glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)
	var (
		PodEvent  = make(chan VmEvent, 128)
		Status    = make(chan *types.VmResponse, 128)
		subStatus = make(chan *types.VmResponse, 128)
	)

	go VmAssociate(mypod.Vm, PodEvent, Status, mypod.Wg, data)

	go vm.handlePodEvent(mypod)

	ass := <-Status
	if ass.Code != types.E_OK {
		glog.Errorf("cannot associate with vm: %s, error status %d (%s)", mypod.Vm, ass.Code, ass.Cause)
		return errors.New("load vm status failed")
	}

	if err := vm.SetVmChan(PodEvent, Status, subStatus); err != nil {
		glog.V(1).Infof("SetVmChan error: %s", err.Error())
		return err
	}

	mypod.Status = types.S_POD_RUNNING
	mypod.SetContainerStatus(types.S_POD_RUNNING)

	vm.Status = types.S_VM_ASSOCIATED
	vm.Pod = mypod

	return nil
}

func (vm *Vm) ReleaseVm() (int, error) {
	var Response *types.VmResponse
	PodEvent, _, Status, err := vm.GetVmChan()
	if err != nil {
		return -1, err
	}
	if vm.Status == types.S_VM_IDLE {
		shutdownPodEvent := &ShutdownCommand{Wait: false}
		PodEvent.(chan VmEvent) <- shutdownPodEvent
		for {
			Response = <-Status.(chan *types.VmResponse)
			if Response.Code == types.E_VM_SHUTDOWN {
				break
			}
		}
		close(Status.(chan *types.VmResponse))
	} else {
		releasePodEvent := &ReleaseVMCommand{}
		PodEvent.(chan VmEvent) <- releasePodEvent
		for {
			Response = <-Status.(chan *types.VmResponse)
			if Response.Code == types.E_VM_SHUTDOWN ||
				Response.Code == types.E_OK {
				break
			}
			if Response.Code == types.E_BUSY {
				return types.E_BUSY, fmt.Errorf("VM busy")
			}
		}
	}

	return types.E_OK, nil
}

func defaultHandlePodEvent(Response *types.VmResponse, data interface{},
	mypod *Pod, vm *Vm) bool {
	if Response.Code == types.E_POD_FINISHED {
		mypod.SetPodContainerStatus(Response.Data.([]uint32))
		mypod.Vm = ""
		vm.Status = types.S_VM_IDLE
	} else if Response.Code == types.E_VM_SHUTDOWN {
		if mypod.Status == types.S_POD_RUNNING {
			mypod.Status = types.S_POD_SUCCEEDED
			mypod.SetContainerStatus(types.S_POD_SUCCEEDED)
		}
		mypod.Vm = ""
		return true
	}

	return false
}

func (vm *Vm) handlePodEvent(mypod *Pod) {
	glog.V(1).Infof("hyperHandlePodEvent pod %s, vm %s", mypod.Id, vm.Id)

	_, ret2, ret3, err := vm.GetVmChan()
	if err != nil {
		return
	}

	glog.V(1).Infof("hyperHandlePodEvent pod %s, vm %s", mypod.Id, vm.Id)
	Status := ret2.(chan *types.VmResponse)
	subStatus := ret3.(chan *types.VmResponse)

	for {
		defer func() {
			err := recover()
			if err != nil {
				glog.Warning("panic during send shutdown message to channel")
			}
		}()
		Response := <-Status
		subStatus <- Response

		exit := mypod.Handler.Handle(Response, mypod.Handler.Data, mypod, vm)
		if exit {
			break
		}
	}
}

func (vm *Vm) StartPod(mypod *Pod, userPod *pod.UserPod,
	cList []*ContainerInfo, vList []*VolumeInfo) *types.VmResponse {
	mypod.Vm = vm.Id

	vm.Pod = mypod
	vm.Status = types.S_VM_ASSOCIATED

	var response *types.VmResponse

	if mypod.Status == types.S_POD_RUNNING {
		err := fmt.Errorf("The pod(%s) is running, can not start it", mypod.Id)
		response = &types.VmResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	if mypod.Type == "kubernetes" && mypod.Status != types.S_POD_CREATED {
		err := fmt.Errorf("The pod(%s) is finished with kubernetes type, can not start it again",
			mypod.Id)
		response = &types.VmResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	ret1, _, ret3, err := vm.GetVmChan()
	if err != nil {
		response = &types.VmResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	PodEvent := ret1.(chan VmEvent)
	subStatus := ret3.(chan *types.VmResponse)

	go vm.handlePodEvent(mypod)

	mypod.Status = types.S_POD_RUNNING
	// Set the container status to online
	mypod.SetContainerStatus(types.S_POD_RUNNING)

	runPodEvent := &RunPodCommand{
		Spec:       userPod,
		Containers: cList,
		Volumes:    vList,
		Wg:         mypod.Wg,
	}

	PodEvent <- runPodEvent

	// wait for the VM response
	for {
		response = <-subStatus
		glog.V(1).Infof("Get the response from VM, VM id is %s!", response.VmId)
		if response.Code == types.E_VM_RUNNING {
			continue
		}
		if response.VmId == vm.Id {
			break
		}
	}

	return response
}

func (vm *Vm) StopPod(mypod *Pod, stopVm string) *types.VmResponse {
	var Response *types.VmResponse

	PodEvent, _, Status, err := vm.GetVmChan()
	if err != nil {
		Response = &types.VmResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return Response
	}

	if stopVm == "yes" {
		mypod.Wg.Add(1)
		shutdownPodEvent := &ShutdownCommand{Wait: true}
		PodEvent.(chan VmEvent) <- shutdownPodEvent
		// wait for the VM response
		for {
			Response = <-Status.(chan *types.VmResponse)
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
			if Response.Code == types.E_VM_SHUTDOWN {
				mypod.Vm = ""
				break
			}
		}
		close(Status.(chan *types.VmResponse))
		// wait for goroutines exit
		mypod.Wg.Wait()
	} else {
		stopPodEvent := &StopPodCommand{}
		PodEvent.(chan VmEvent) <- stopPodEvent
		// wait for the VM response
		for {
			Response = <-Status.(chan *types.VmResponse)
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
			if Response.Code == types.E_POD_STOPPED || Response.Code == types.E_BAD_REQUEST || Response.Code == types.E_FAILED {
				mypod.Vm = ""
				vm.Status = types.S_VM_IDLE
				break
			}
		}
	}

	mypod.Status = types.S_POD_FAILED
	mypod.SetContainerStatus(types.S_POD_FAILED)

	return Response
}

func (vm *Vm) Exec(Stdin io.ReadCloser, Stdout io.WriteCloser, cmd, tag, container string) error {
	var command []string
	Callback := make(chan *types.VmResponse, 1)

	if cmd == "" {
		return fmt.Errorf("'exec' without command")
	}

	if err := json.Unmarshal([]byte(cmd), &command); err != nil {
		return err
	}

	execCmd := &ExecCommand{
		Command:   command,
		Container: container,
		Streams: &TtyIO{
			Stdin:     Stdin,
			Stdout:    Stdout,
			ClientTag: tag,
			Callback:  Callback,
		},
	}

	Event, _, _, err := vm.GetVmChan()
	if err != nil {
		return err
	}

	Event.(chan VmEvent) <- execCmd

	<-Callback

	return nil
}

func (vm *Vm) Tty(tag string, row, column int) error {
	var ttySizeCommand = &WindowSizeCommand{
		ClientTag: tag,
		Size:      &WindowSize{Row: uint16(row), Column: uint16(column)},
	}

	Event, _, _, err := vm.GetVmChan()
	if err != nil {
		return err
	}

	Event.(chan VmEvent) <- ttySizeCommand
	return nil
}

func NewVm(vmId string, cpu, memory int, lazy bool, keep int) *Vm {
	return &Vm{
		Id:     vmId,
		Pod:    nil,
		Lazy:   lazy,
		Status: types.S_VM_IDLE,
		Cpu:    cpu,
		Mem:    memory,
		Keep:   keep,
	}
}
