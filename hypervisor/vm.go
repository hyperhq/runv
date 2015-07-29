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
	Id                string
	Pod               *Pod
	Status            uint
	Cpu               int
	Mem               int
	QemuChan          interface{}
	QemuClientChan    interface{}
	SubQemuClientChan interface{}
}

func (vm *Vm) GetQemuChan() (interface{}, interface{}, interface{}, error) {
	if vm.QemuChan != nil && vm.QemuClientChan != nil {
		return vm.QemuChan, vm.QemuClientChan, vm.SubQemuClientChan, nil
	}
	return nil, nil, nil, fmt.Errorf("Can not find the Qemu chan for pod: %s!", vm.Id)
}

func (vm *Vm) SetQemuChan(qemuchan, qemuclient, subQemuClient interface{}) error {
	if vm.QemuChan == nil {
		if qemuchan != nil {
			vm.QemuChan = qemuchan
		}
		if qemuclient != nil {
			vm.QemuClientChan = qemuclient
		}
		if subQemuClient != nil {
			vm.SubQemuClientChan = subQemuClient
		}
		return nil
	}
	return fmt.Errorf("Already setup chan for vm: %s!", vm.Id)
}

func (vm *Vm) Launch(b *BootConfig) (err error) {
	var (
		qemuPodEvent  = make(chan VmEvent, 128)
		qemuStatus    = make(chan *types.QemuResponse, 128)
		subQemuStatus = make(chan *types.QemuResponse, 128)
	)

	go VmLoop(vm.Id, qemuPodEvent, qemuStatus, b)
	if err := vm.SetQemuChan(qemuPodEvent, qemuStatus, subQemuStatus); err != nil {
		glog.V(1).Infof("SetQemuChan error: %s", err.Error())
		return err
	}

	return nil
}

func (vm *Vm) Kill() (int, string, error) {
	qemuPodEvent, qemuStatus, subQemuStatus, err := vm.GetQemuChan()
	if err != nil {
		return -1, "", err
	}
	var qemuResponse *types.QemuResponse
	shutdownPodEvent := &ShutdownCommand{Wait: false}
	qemuPodEvent.(chan VmEvent) <- shutdownPodEvent
	// wait for the qemu response
	for {
		stop := 0
		select {
		case qemuResponse = <-qemuStatus.(chan *types.QemuResponse):
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		case qemuResponse = <-subQemuStatus.(chan *types.QemuResponse):
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				stop = 1
			}
		}
		if stop == 1 {
			break
		}
	}
	close(qemuStatus.(chan *types.QemuResponse))
	close(subQemuStatus.(chan *types.QemuResponse))

	return qemuResponse.Code, qemuResponse.Cause, nil
}

// This function will only be invoked during daemon start
func (vm *Vm) AssociateVm(mypod *Pod, data []byte) error {
	glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)
	var (
		qemuPodEvent  = make(chan VmEvent, 128)
		qemuStatus    = make(chan *types.QemuResponse, 128)
		subQemuStatus = make(chan *types.QemuResponse, 128)
	)

	go VmAssociate(mypod.Vm, qemuPodEvent,
		qemuStatus, mypod.Wg, data)

	go vm.handlePodEvent(mypod)

	ass := <-qemuStatus
	if ass.Code != types.E_OK {
		glog.Errorf("cannot associate with vm: %s, error status %d (%s)", mypod.Vm, ass.Code, ass.Cause)
		return errors.New("load vm status failed")
	}

	if err := vm.SetQemuChan(qemuPodEvent, qemuStatus, subQemuStatus); err != nil {
		glog.V(1).Infof("SetQemuChan error: %s", err.Error())
		return err
	}

	mypod.Status = types.S_POD_RUNNING
	mypod.SetContainerStatus(types.S_POD_RUNNING)

	vm.Status = types.S_VM_ASSOCIATED
	vm.Pod = mypod

	return nil
}

func (vm *Vm) ReleaseVm() (int, error) {
	var qemuResponse *types.QemuResponse
	qemuPodEvent, _, qemuStatus, err := vm.GetQemuChan()
	if err != nil {
		return -1, err
	}
	if vm.Status == types.S_VM_IDLE {
		shutdownPodEvent := &ShutdownCommand{Wait: false}
		qemuPodEvent.(chan VmEvent) <- shutdownPodEvent
		for {
			qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				break
			}
		}
		close(qemuStatus.(chan *types.QemuResponse))
	} else {
		releasePodEvent := &ReleaseVMCommand{}
		qemuPodEvent.(chan VmEvent) <- releasePodEvent
		for {
			qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
			if qemuResponse.Code == types.E_VM_SHUTDOWN ||
				qemuResponse.Code == types.E_OK {
				break
			}
			if qemuResponse.Code == types.E_BUSY {
				return types.E_BUSY, fmt.Errorf("VM busy")
			}
		}
	}

	return types.E_OK, nil
}

func defaultHandlePodEvent(qemuResponse *types.QemuResponse, data interface{},
	mypod *Pod, vm *Vm) bool {
	if qemuResponse.Code == types.E_POD_FINISHED {
		mypod.SetPodContainerStatus(qemuResponse.Data.([]uint32))
		mypod.Vm = ""
		vm.Status = types.S_VM_IDLE
	} else if qemuResponse.Code == types.E_VM_SHUTDOWN {
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

	_, ret2, ret3, err := vm.GetQemuChan()
	if err != nil {
		return
	}

	glog.V(1).Infof("hyperHandlePodEvent pod %s, vm %s", mypod.Id, vm.Id)
	qemuStatus := ret2.(chan *types.QemuResponse)
	subQemuStatus := ret3.(chan *types.QemuResponse)

	for {
		qemuResponse := <-qemuStatus
		subQemuStatus <- qemuResponse

		exit := mypod.Handler.Handle(qemuResponse, mypod.Handler.Data, mypod, vm)
		if exit {
			break
		}
	}
}

func (vm *Vm) StartPod(mypod *Pod, userPod *pod.UserPod,
	cList []*ContainerInfo, vList []*VolumeInfo) *types.QemuResponse {
	mypod.Vm = vm.Id

	vm.Pod = mypod
	vm.Status = types.S_VM_ASSOCIATED

	var response *types.QemuResponse

	if mypod.Status == types.S_POD_RUNNING {
		err := fmt.Errorf("The pod(%s) is running, can not start it", mypod.Id)
		response = &types.QemuResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	if mypod.Type == "kubernetes" && mypod.Status != types.S_POD_CREATED {
		err := fmt.Errorf("The pod(%s) is finished with kubernetes type, can not start it again",
			mypod.Id)
		response = &types.QemuResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	ret1, _, ret3, err := vm.GetQemuChan()
	if err != nil {
		response = &types.QemuResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	qemuPodEvent := ret1.(chan VmEvent)
	subQemuStatus := ret3.(chan *types.QemuResponse)

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

	qemuPodEvent <- runPodEvent

	// wait for the qemu response
	for {
		response = <-subQemuStatus
		glog.V(1).Infof("Get the response from QEMU, VM id is %s!", response.VmId)
		if response.Code == types.E_VM_RUNNING {
			continue
		}
		if response.VmId == vm.Id {
			break
		}
	}

	return response
}

func (vm *Vm) StopPod(mypod *Pod, stopVm string) *types.QemuResponse {
	var qemuResponse *types.QemuResponse

	qemuPodEvent, _, qemuStatus, err := vm.GetQemuChan()
	if err != nil {
		qemuResponse = &types.QemuResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return qemuResponse
	}

	if stopVm == "yes" {
		mypod.Wg.Add(1)
		shutdownPodEvent := &ShutdownCommand{Wait: true}
		qemuPodEvent.(chan VmEvent) <- shutdownPodEvent
		// wait for the qemu response
		for {
			qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_VM_SHUTDOWN {
				mypod.Vm = ""
				break
			}
		}
		close(qemuStatus.(chan *types.QemuResponse))
		// wait for goroutines exit
		mypod.Wg.Wait()
	} else {
		stopPodEvent := &StopPodCommand{}
		qemuPodEvent.(chan VmEvent) <- stopPodEvent
		// wait for the qemu response
		for {
			qemuResponse = <-qemuStatus.(chan *types.QemuResponse)
			glog.V(1).Infof("Got response: %d: %s", qemuResponse.Code, qemuResponse.Cause)
			if qemuResponse.Code == types.E_POD_STOPPED || qemuResponse.Code == types.E_BAD_REQUEST || qemuResponse.Code == types.E_FAILED {
				mypod.Vm = ""
				vm.Status = types.S_VM_IDLE
				break
			}
		}
	}

	mypod.Status = types.S_POD_FAILED
	mypod.SetContainerStatus(types.S_POD_FAILED)

	return qemuResponse
}

func (vm *Vm) Exec(Stdin io.ReadCloser, Stdout io.WriteCloser, cmd, tag, container string) error {
	var command []string
	qemuCallback := make(chan *types.QemuResponse, 1)

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
			Callback:  qemuCallback,
		},
	}

	qemuEvent, _, _, err := vm.GetQemuChan()
	if err != nil {
		return err
	}

	qemuEvent.(chan VmEvent) <- execCmd

	<-qemuCallback

	return nil
}

func (vm *Vm) Tty(tag string, row, column int) error {
	var ttySizeCommand = &WindowSizeCommand{
		ClientTag: tag,
		Size:      &WindowSize{Row: uint16(row), Column: uint16(column)},
	}

	qemuEvent, _, _, err := vm.GetQemuChan()
	if err != nil {
		return err
	}

	qemuEvent.(chan VmEvent) <- ttySizeCommand
	return nil
}

func NewVm(vmId string, cpu, memory int) *Vm {
	return &Vm{
		Id:     vmId,
		Pod:    nil,
		Status: types.S_VM_IDLE,
		Cpu:    cpu,
		Mem:    memory,
	}
}
