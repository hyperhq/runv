package hypervisor

import (
	"errors"
	"fmt"
	"io"
	"time"

	"encoding/json"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

type Vm struct {
	Id     string
	Pod    *Pod
	Status uint
	Cpu    int
	Mem    int
	Lazy   bool
	Keep   int

	VmChan  chan VmEvent
	clients *Fanout
}

func (vm *Vm) GetRequestChan() (chan VmEvent, error) {
	return vm.VmChan, nil
}

func (vm *Vm) ReleaseRequestChan(chan VmEvent) {
	//do nothing, the existence of this method is just let the
	//API be symmetric
}

func (vm *Vm) GetResponseChan() (chan *types.VmResponse, error) {
	if vm.clients != nil {
		return vm.clients.Acquire()
	}
	return nil, errors.New("No channels available")
}

func (vm *Vm) ReleaseResponseChan(ch chan *types.VmResponse) {
	if vm.clients != nil {
		vm.clients.Release(ch)
	}
}

func (vm *Vm) Launch(b *BootConfig) (err error) {
	var (
		PodEvent = make(chan VmEvent, 128)
		Status   = make(chan *types.VmResponse, 128)
	)

	if vm.Lazy {
		go LazyVmLoop(vm.Id, PodEvent, Status, b, vm.Keep)
	} else {
		go VmLoop(vm.Id, PodEvent, Status, b, vm.Keep)
	}

	vm.VmChan = PodEvent
	vm.clients = CreateFanout(Status, 128, false)

	return nil
}

func (vm *Vm) Kill() (int, string, error) {
	PodEvent, err := vm.GetRequestChan()
	if err != nil {
		return -1, "", err
	}

	Status, err := vm.GetResponseChan()
	if err != nil {
		return -1, "", err
	}

	var Response *types.VmResponse
	shutdownPodEvent := &ShutdownCommand{Wait: false}
	PodEvent <- shutdownPodEvent
	// wait for the VM response
	stop := false
	for !stop {
		Response = <-Status
		glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
		if Response.Code == types.E_VM_SHUTDOWN {
			vm.ReleaseResponseChan(Status)
			vm.ReleaseRequestChan(PodEvent)
			if vm.clients != nil {
				vm.clients.Close()
				vm.clients = nil
			}
			stop = true
		}
	}

	return Response.Code, Response.Cause, nil
}

// This function will only be invoked during daemon start
func (vm *Vm) AssociateVm(mypod *Pod, data []byte) error {
	glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)
	var (
		PodEvent = make(chan VmEvent, 128)
		Status   = make(chan *types.VmResponse, 128)
	)

	go VmAssociate(mypod.Vm, PodEvent, Status, mypod.Wg, data)

	go vm.handlePodEvent(mypod)

	ass := <-Status
	if ass.Code != types.E_OK {
		glog.Errorf("cannot associate with vm: %s, error status %d (%s)", mypod.Vm, ass.Code, ass.Cause)
		return errors.New("load vm status failed")
	}

	vm.VmChan = PodEvent
	vm.clients = CreateFanout(Status, 128, false)

	mypod.Status = types.S_POD_RUNNING
	mypod.StartedAt = time.Now().Format("2006-01-02T15:04:05Z")
	mypod.SetContainerStatus(types.S_POD_RUNNING)

	vm.Status = types.S_VM_ASSOCIATED
	vm.Pod = mypod

	return nil
}

func (vm *Vm) ReleaseVm() (int, error) {
	var Response *types.VmResponse
	PodEvent, err := vm.GetRequestChan()
	if err != nil {
		return -1, err
	}
	defer vm.ReleaseRequestChan(PodEvent)

	Status, err := vm.GetResponseChan()
	if err != nil {
		return -1, err
	}
	defer vm.ReleaseResponseChan(Status)

	if vm.Status == types.S_VM_IDLE {
		shutdownPodEvent := &ShutdownCommand{Wait: false}
		PodEvent <- shutdownPodEvent
		for {
			Response = <-Status
			if Response.Code == types.E_VM_SHUTDOWN {
				break
			}
		}
	} else {
		releasePodEvent := &ReleaseVMCommand{}
		PodEvent <- releasePodEvent
		for {
			Response = <-Status
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

	Status, err := vm.GetResponseChan()
	if err != nil {
		return
	}
	defer vm.ReleaseResponseChan(Status)

	for {
		Response, ok := <-Status
		if !ok {
			break
		}

		exit := mypod.Handler.Handle(Response, mypod.Handler.Data, mypod, vm)
		if exit {
			vm.clients.Close()
			vm.clients = nil
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

	PodEvent, err := vm.GetRequestChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseRequestChan(PodEvent)

	Status, err := vm.GetResponseChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseResponseChan(Status)

	go vm.handlePodEvent(mypod)

	mypod.Status = types.S_POD_RUNNING
	mypod.StartedAt = time.Now().Format("2006-01-02T15:04:05Z")
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
		response = <-Status
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

	PodEvent, err := vm.GetRequestChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseRequestChan(PodEvent)

	Status, err := vm.GetResponseChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseResponseChan(Status)

	if mypod.Status != types.S_POD_RUNNING {
		return errorResponse("The POD has already stoppod")
	}

	if stopVm == "yes" {
		mypod.Wg.Add(1)
		shutdownPodEvent := &ShutdownCommand{Wait: true}
		PodEvent <- shutdownPodEvent
		// wait for the VM response
		for {
			Response = <-Status
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
			if Response.Code == types.E_VM_SHUTDOWN {
				mypod.Vm = ""
				break
			}
		}
		// wait for goroutines exit
		mypod.Wg.Wait()
	} else {
		stopPodEvent := &StopPodCommand{}
		PodEvent <- stopPodEvent
		// wait for the VM response
		for {
			Response = <-Status
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

func (vm *Vm) WriteFile(container, target string, data []byte) error {
	if target == "" {
		return fmt.Errorf("'write' without file")
	}

	writeEvent := &WriteFileCommand{
		Container: container,
		File:      target,
		Data:      []byte{},
	}

	writeEvent.Data = append(writeEvent.Data, data[:]...)
	Event, _, responseEvent, err := vm.GetVmChan()
	if err != nil {
		return err
	}

	Event.(chan VmEvent) <- writeEvent

	for {
		Response := <-responseEvent.(chan *types.VmResponse)
		glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
		if Response.Code == types.E_FILE {
			break
		}
	}

	return nil
}

func (vm *Vm) ReadFile(container, target string) ([]byte, error) {
	if target == "" {
		return nil, fmt.Errorf("'read' without file")
	}

	readEvent := &ReadFileCommand{
		Container: container,
		File:      target,
	}

	Event, _, responseEvent, err := vm.GetVmChan()
	if err != nil {
		return nil, err
	}

	Event.(chan VmEvent) <- readEvent

	for {
		Response := <-responseEvent.(chan *types.VmResponse)
		glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
		if Response.Code == types.E_FILE {
			return Response.Data.([]byte), nil
		}
	}

	return nil, nil
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

	Event, err := vm.GetRequestChan()
	if err != nil {
		return err
	}

	Event <- execCmd
	vm.ReleaseRequestChan(Event)

	<-Callback

	return nil
}

func (vm *Vm) Tty(tag string, row, column int) error {
	var ttySizeCommand = &WindowSizeCommand{
		ClientTag: tag,
		Size:      &WindowSize{Row: uint16(row), Column: uint16(column)},
	}

	Event, err := vm.GetRequestChan()
	if err != nil {
		return err
	}

	Event <- ttySizeCommand
	vm.ReleaseRequestChan(Event)
	return nil
}

func errorResponse(cause string) *types.VmResponse {
	return &types.VmResponse{
		Code:  -1,
		Cause: cause,
		Data:  nil,
	}
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
