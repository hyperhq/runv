package hypervisor

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"encoding/json"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
)

type Vm struct {
	Id string

	ctx *VmContext

	//Pod    *PodStatus
	Status uint
	Cpu    int
	Mem    int
	Lazy   bool

	Hub     chan VmEvent
	clients *Fanout
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
		vmEvent = make(chan VmEvent, 128)
		Status  = make(chan *types.VmResponse, 128)
		ctx     *VmContext
	)

	//if vm.Lazy {
	//	go LazyVmLoop(vm.Id, PodEvent, Status, b)
	//} else {
	ctx, err = InitContext(vm.Id, vmEvent, Status, nil, b)
	if err != nil {
		Status <- &types.VmResponse{
			VmId:  vm.Id,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return err

	}

	go ctx.Launch()
	vm.ctx = ctx
	//}

	vm.Hub = vmEvent
	vm.clients = CreateFanout(Status, 128, false)

	return nil
}

func (vm *Vm) Kill() (int, string, error) {
	Status, err := vm.GetResponseChan()
	if err != nil {
		return -1, "", err
	}

	var Response *types.VmResponse
	shutdownPodEvent := &ShutdownCommand{Wait: false}
	vm.Hub <- shutdownPodEvent
	// wait for the VM response
	stop := false
	for !stop {
		var ok bool
		Response, ok = <-Status
		if !ok || Response == nil || Response.Code == types.E_VM_SHUTDOWN || Response.Code == types.E_FAILED {
			vm.ReleaseResponseChan(Status)
			vm.clients = nil
			stop = true
		}
		if Response != nil {
			glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
		} else {
			glog.V(1).Infof("Nil response from status chan")
		}
	}

	if Response == nil {
		return types.E_VM_SHUTDOWN, "", nil
	}

	return Response.Code, Response.Cause, nil
}

// This function will only be invoked during daemon start
func (vm *Vm) AssociateVm(mypod *PodStatus, data []byte) error {
	glog.V(1).Infof("Associate the POD(%s) with VM(%s)", mypod.Id, mypod.Vm)
	var (
		PodEvent = make(chan VmEvent, 128)
		Status   = make(chan *types.VmResponse, 128)
	)

	VmAssociate(mypod.Vm, PodEvent, Status, mypod.Wg, data)

	ass := <-Status
	if ass.Code != types.E_OK {
		glog.Errorf("cannot associate with vm: %s, error status %d (%s)", mypod.Vm, ass.Code, ass.Cause)
		return errors.New("load vm status failed")
	}
	go vm.handlePodEvent(mypod)

	vm.Hub = PodEvent
	vm.clients = CreateFanout(Status, 128, false)

	mypod.Status = types.S_POD_RUNNING
	mypod.StartedAt = time.Now().Format("2006-01-02T15:04:05Z")
	mypod.SetContainerStatus(types.S_POD_RUNNING)

	vm.Status = types.S_VM_ASSOCIATED
	//vm.Pod = mypod

	return nil
}

func (vm *Vm) ReleaseVm() (int, error) {
	var Response *types.VmResponse

	Status, err := vm.GetResponseChan()
	if err != nil {
		return -1, err
	}
	defer vm.ReleaseResponseChan(Status)

	if vm.Status == types.S_VM_IDLE {
		shutdownPodEvent := &ShutdownCommand{Wait: false}
		vm.Hub <- shutdownPodEvent
		for {
			Response = <-Status
			if Response.Code == types.E_VM_SHUTDOWN {
				break
			}
		}
	} else {
		releasePodEvent := &ReleaseVMCommand{}
		vm.Hub <- releasePodEvent
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

func (vm *Vm) handlePodEvent(mypod *PodStatus) {
	glog.V(1).Infof("hyperHandlePodEvent pod %s, vm %s", mypod.Id, vm.Id)

	Status, err := vm.GetResponseChan()
	if err != nil {
		return
	}
	defer vm.ReleaseResponseChan(Status)

	exit := false
	mypod.Wg.Add(1)
	for {
		Response, ok := <-Status
		if !ok {
			break
		}

		switch Response.Code {
		case types.E_CONTAINER_FINISHED:
			ps, ok := Response.Data.(*types.ProcessFinished)
			if ok {
				mypod.SetOneContainerStatus(ps.Id, ps.Code)
				close(ps.Ack)
			}
		case types.E_EXEC_FINISHED:
			ps, ok := Response.Data.(*types.ProcessFinished)
			if ok {
				mypod.SetExecStatus(ps.Id, ps.Code)
				close(ps.Ack)
			}
		case types.E_POD_FINISHED: // successfully exit
			mypod.SetPodContainerStatus(Response.Data.([]uint32))
			vm.Status = types.S_VM_IDLE
		case types.E_VM_SHUTDOWN: // vm exited, successful or not
			if mypod.Status == types.S_POD_RUNNING { // not received finished pod before
				mypod.Status = types.S_POD_FAILED
				mypod.FinishedAt = time.Now().Format("2006-01-02T15:04:05Z")
				mypod.SetContainerStatus(types.S_POD_FAILED)
			}
			mypod.Vm = ""
			exit = true
		}

		if mypod.Handler != nil {
			mypod.Handler.Handle(Response, mypod.Handler.Data, mypod, vm)
		}

		if exit {
			vm.clients = nil
			break
		}
	}
	mypod.Wg.Done()
}

func (vm *Vm) InitSandbox(config *api.SandboxConfig, result chan<- api.Result) {
	if vm.ctx == nil {
		result <- NewNotReadyError(vm.Id)
		return
	}

	vm.ctx.SetNetworkEnvironment(config)
	vm.ctx.startPod()

	go func() {
		Status, err := vm.GetResponseChan()
		if err != nil {
			vm.ctx.Log(ERROR, "failed to get status chan to monitor startpod: %v", err)
			result <- api.NewResultBase(vm.Id, false, err.Error())
			return
		}
		defer vm.ReleaseResponseChan(Status)

		for {
			s, ok := <-Status
			if !ok {
				result <- api.NewResultBase(vm.Id, false, "status channel broken")
				return
			}

			switch s.Code {
			case types.E_OK:
				result <- api.NewResultBase(vm.Id, true, "set sandbox config successfully")
				return
			case types.E_FAILED, types.E_VM_SHUTDOWN:
				result <- api.NewResultBase(vm.Id, false, "set sandbox config failed")
				return
			default:
				vm.ctx.Log(DEBUG, "got message %#v while waiting start pod command finish")
			}
		}
	}()

}

func (vm *Vm) StartPod(mypod *PodStatus, userPod *pod.UserPod,
	cList []*ContainerInfo, vList map[string]*VolumeInfo) *types.VmResponse {

	var ok bool = false
	var response *types.VmResponse

	mypod.Vm = vm.Id

	//vm.Pod = mypod
	vm.Status = types.S_VM_ASSOCIATED

	if mypod.Status == types.S_POD_RUNNING {
		err := fmt.Errorf("The pod(%s) is running, can not start it", mypod.Id)
		response = &types.VmResponse{
			Code:  -1,
			Cause: err.Error(),
			Data:  nil,
		}
		return response
	}

	Status, err := vm.GetResponseChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseResponseChan(Status)

	go vm.handlePodEvent(mypod)

	runPodEvent := &RunPodCommand{
		Spec:       userPod,
		Containers: cList,
		Volumes:    vList,
		Wg:         mypod.Wg,
	}

	vm.Hub <- runPodEvent

	// wait for the VM response
	for {
		response, ok = <-Status
		if !ok {
			response = &types.VmResponse{
				Code:  -1,
				Cause: "Start pod failed",
				Data:  nil,
			}
			glog.V(1).Infof("return response %v", response)
			break
		}
		glog.V(1).Infof("Get the response from VM, VM id is %s!", response.VmId)
		if response.Code == types.E_VM_RUNNING {
			continue
		}
		if response.VmId == vm.Id {
			break
		}
	}

	if response.Data != nil {
		mypod.Status = types.S_POD_RUNNING
		mypod.StartedAt = time.Now().Format("2006-01-02T15:04:05Z")
		// Set the container status to online
		mypod.SetContainerStatus(types.S_POD_RUNNING)
	}

	return response
}

func (vm *Vm) Shutdown(result chan<- api.Result) {
	if vm.ctx.current != StateRunning {
		result <- api.NewResultBase(vm.Id, false, "not in running state")
		return
	}
	Status, err := vm.GetResponseChan()
	if err != nil {
		result <- api.NewResultBase(vm.Id, false, "fail to get response chan")
		return
	}
	defer vm.ReleaseResponseChan(Status)

	vm.Hub <- &ShutdownCommand{}
	for {
		Response, ok := <-Status
		if !ok {
			result <- api.NewResultBase(vm.Id, false, "status channel broken")
			return
		}
		glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
		if Response.Code == types.E_VM_SHUTDOWN {
			result <- api.NewResultBase(vm.Id, true, "set sandbox config successfully")
			return
		}
	}
}

func (vm *Vm) StopPod(mypod *PodStatus) *types.VmResponse {
	var Response *types.VmResponse

	Status, err := vm.GetResponseChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseResponseChan(Status)

	if mypod.Status != types.S_POD_RUNNING {
		return errorResponse("The POD has already stoppod")
	}

	mypod.Wg.Add(1)
	shutdownPodEvent := &ShutdownCommand{Wait: true}
	vm.Hub <- shutdownPodEvent
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

	mypod.Status = types.S_POD_FAILED
	mypod.SetContainerStatus(types.S_POD_FAILED)

	return Response
}

func (vm *Vm) WriteFile(container, target string, data []byte) error {
	if target == "" {
		return fmt.Errorf("'write' without file")
	}

	return vm.GenericOperation("WriteFile", func(ctx *VmContext, result chan<- error) {
		writeCmd, _ := json.Marshal(hyperstartapi.FileCommand{
			Container: container,
			File:      target,
		})
		writeCmd = append(writeCmd, data[:]...)
		ctx.vm <- &hyperstartCmd{
			Code:    hyperstartapi.INIT_WRITEFILE,
			Message: writeCmd,
			result:  result,
		}
	}, StateRunning)
}

func (vm *Vm) ReadFile(container, target string) ([]byte, error) {
	if target == "" {
		return nil, fmt.Errorf("'read' without file")
	}

	cmd := hyperstartCmd{
		Code: hyperstartapi.INIT_READFILE,
		Message: &hyperstartapi.FileCommand{
			Container: container,
			File:      target,
		},
	}
	err := vm.GenericOperation("ReadFile", func(ctx *VmContext, result chan<- error) {
		cmd.result = result
		ctx.vm <- &cmd
	}, StateRunning)

	return cmd.retMsg, err
}

func (vm *Vm) KillContainer(container string, signal syscall.Signal) error {
	return vm.GenericOperation("KillContainer", func(ctx *VmContext, result chan<- error) {
		if ctx.current != StateRunning {
			glog.V(1).Infof("container %s is already stopped, in %s", container, ctx.current)
			result <- fmt.Errorf("container %s is already stopped", container)
			return
		}
		ctx.killCmd(container, signal, result)
	}, StateRunning, StateTerminating, StateDestroying)
}

func (vm *Vm) AddRoute() error {
	return vm.GenericOperation("AddRoute", func(ctx *VmContext, result chan<- error) {
		routes := ctx.networks.getRoutes()

		ctx.vm <- &hyperstartCmd{
			Code:    hyperstartapi.INIT_SETUPROUTE,
			Message: hyperstartapi.Routes{Routes: routes},
			result:  result,
		}
	}, StateRunning)
}

func (vm *Vm) AddNic(info *api.InterfaceDescription) error {
	client := make(chan api.Result, 1)
	vm.SendGenericOperation("CreateInterface", func(ctx *VmContext, result chan<- error) {
		go ctx.AddInterface(info, client)
	}, StateRunning)

	ev, ok := <-client
	if !ok {
		return fmt.Errorf("internal error")
	}

	if !ev.IsSuccess() {
		return fmt.Errorf("allocate device failed")
	}

	return vm.GenericOperation("InterfaceInserted", func(ctx *VmContext, result chan<- error) {
		if ctx.LogLevel(TRACE) {
			glog.Infof("finial vmSpec.Interface is %#v", ctx.networks.getInterface(info.Id))
		}

		ctx.updateInterface(info.Id, result)
	}, StateRunning)
}

func (vm *Vm) DeleteNic(idx int) error {

	vm.SendGenericOperation("NetDevRemovedEvent", func(ctx *VmContext, result chan<- error) {
		ctx.removeInterfaceByLinkIndex(idx)
	}, StateRunning)

	return nil
}

func (vm *Vm) GetNextNicNameInVM() string {
	name := make(chan string, 1)

	vm.SendGenericOperation("GetNextNicName", func(ctx *VmContext, result chan<- error) {
		ctx.GetNextNicName(name)
	}, StateRunning)

	return <-name
}

// TODO: deprecated api, it will be removed after the hyper.git updated
func (vm *Vm) AddCpu(totalCpu int) error {
	return vm.SetCpus(totalCpu)
}

func (vm *Vm) SetCpus(cpus int) error {
	if vm.Cpu >= cpus {
		return nil
	}

	err := vm.GenericOperation("SetCpus", func(ctx *VmContext, result chan<- error) {
		ctx.DCtx.SetCpus(ctx, cpus, result)
	}, StateInit)

	if err == nil {
		vm.Cpu = cpus
	}
	return err
}

func (vm *Vm) AddMem(totalMem int) error {
	if vm.Mem >= totalMem {
		return nil
	}

	size := totalMem - vm.Mem
	err := vm.GenericOperation("AddMem", func(ctx *VmContext, result chan<- error) {
		ctx.DCtx.AddMem(ctx, 1, size, result)
	}, StateInit)

	if err == nil {
		vm.Mem = totalMem
	}
	return err
}

func (vm *Vm) OnlineCpuMem() error {
	onlineCmd := &OnlineCpuMemCommand{}

	Status, err := vm.GetResponseChan()
	if err != nil {
		return nil
	}
	defer vm.ReleaseResponseChan(Status)

	vm.Hub <- onlineCmd

	return nil
}

func (vm *Vm) Exec(container, execId, cmd string, terminal bool, tty *TtyIO) error {
	var command []string

	if cmd == "" {
		return fmt.Errorf("'exec' without command")
	}

	if err := json.Unmarshal([]byte(cmd), &command); err != nil {
		return err
	}
	return vm.AddProcess(container, execId, terminal, command, []string{}, "/", tty)
}

func (vm *Vm) AddProcess(container, execId string, terminal bool, args []string, env []string, workdir string, tty *TtyIO) error {
	envs := []hyperstartapi.EnvironmentVar{}

	for _, v := range env {
		if eqlIndex := strings.Index(v, "="); eqlIndex > 0 {
			envs = append(envs, hyperstartapi.EnvironmentVar{
				Env:   v[:eqlIndex],
				Value: v[eqlIndex+1:],
			})
		}
	}

	execCmd := &hyperstartapi.ExecCommand{
		Container: container,
		Process: hyperstartapi.Process{
			Terminal: terminal,
			Args:     args,
			Envs:     envs,
			Workdir:  workdir,
		},
	}

	err := vm.GenericOperation("AddProcess", func(ctx *VmContext, result chan<- error) {
		ctx.execCmd(execId, execCmd, tty, result)
	}, StateRunning)

	if err != nil {
		return fmt.Errorf("exec command %v failed: %v", args, err)
	}

	vm.GenericOperation("StartStdin", func(ctx *VmContext, result chan<- error) {
		ctx.ptys.startStdin(execCmd.Process.Stdio, true)
		result <- nil
	}, StateRunning)

	return tty.WaitForFinish()
}

func (vm *Vm) StartContainer(id string) error {

	err := vm.GenericOperation("NewContainer", func(ctx *VmContext, result chan<- error) {
		ctx.newContainer(id, result)
	}, StateInit, StateRunning)

	if err != nil {
		return fmt.Errorf("Create new container failed: %v", err)
	}
	vm.GenericOperation("StartNewContainerStdin", func(ctx *VmContext, result chan<- error) {
		// start stdin. TODO: find the correct idx if parallel multi INIT_NEWCONTAINER
		if cc, ok := ctx.containers[id]; ok {
			ctx.ptys.startStdin(cc.process.Stdio, cc.process.Terminal)
		}
	}, StateInit, StateRunning)

	return nil
}

func (vm *Vm) Tty(containerId, execId string, row, column int) error {
	var ttySizeCommand = &WindowSizeCommand{
		ContainerId: containerId,
		ExecId:      execId,
		Size:        &WindowSize{Row: uint16(row), Column: uint16(column)},
	}

	vm.Hub <- ttySizeCommand
	return nil
}

func (vm *Vm) Stats() *types.VmResponse {
	var response *types.VmResponse

	//if nil == vm.Pod || vm.Pod.Status != types.S_POD_RUNNING {
	//	return errorResponse("The pod is not running, can not get stats for it")
	//}

	Status, err := vm.GetResponseChan()
	if err != nil {
		return errorResponse(err.Error())
	}
	defer vm.ReleaseResponseChan(Status)

	getPodStatsEvent := &GetPodStatsCommand{
		Id: vm.Id,
	}
	vm.Hub <- getPodStatsEvent

	// wait for the VM response
	for {
		response = <-Status
		if response == nil {
			continue
		}
		glog.V(1).Infof("Got response, Code %d, VM id %s!", response.Code, response.VmId)
		if response.Reply != getPodStatsEvent {
			continue
		}
		if response.VmId == vm.Id {
			break
		}
	}

	return response
}

func (vm *Vm) Pause(pause bool) error {
	command := "Pause"
	pauseState := PauseStatePaused
	oldPauseState := PauseStateUnpaused
	if !pause {
		pauseState = PauseStateUnpaused
		command = "Unpause"
	}

	err := vm.GenericOperation(command, func(ctx *VmContext, result chan<- error) {
		oldPauseState = ctx.PauseState
		if ctx.PauseState == PauseStateBusy {
			result <- fmt.Errorf("%s fails: earlier Pause or Unpause operation has not finished", command)
			return
		} else if ctx.PauseState == pauseState {
			result <- nil
			return
		}
		ctx.PauseState = PauseStateBusy
		/* FIXME: only support pause whole vm now */
		ctx.DCtx.Pause(ctx, pause, result)
	}, StateInit, StateRunning)

	if oldPauseState == pauseState {
		return nil
	}
	if err != nil {
		pauseState = oldPauseState // recover the state
	}
	vm.GenericOperation(command+" result", func(ctx *VmContext, result chan<- error) {
		ctx.PauseState = pauseState
		result <- nil
	}, StateInit, StateRunning)
	return err
}

func (vm *Vm) Save(path string) error {
	return vm.GenericOperation("Save", func(ctx *VmContext, result chan<- error) {
		if ctx.PauseState == PauseStatePaused {
			ctx.DCtx.Save(ctx, path, result)
		} else {
			result <- fmt.Errorf("the vm should paused on non-live Save()")
		}
	}, StateInit, StateRunning)
}

func (vm *Vm) SendGenericOperation(name string, op func(ctx *VmContext, result chan<- error), states ...string) <-chan error {
	result := make(chan error, 1)
	goe := &GenericOperation{
		OpName: name,
		State:  states,
		OpFunc: op,
		Result: result,
	}
	vm.Hub <- goe
	return result
}

func (vm *Vm) GenericOperation(name string, op func(ctx *VmContext, result chan<- error), states ...string) error {
	return <-vm.SendGenericOperation(name, op, states...)
}

func errorResponse(cause string) *types.VmResponse {
	return &types.VmResponse{
		Code:  -1,
		Cause: cause,
		Data:  nil,
	}
}

func NewVm(vmId string, cpu, memory int, lazy bool) *Vm {
	return &Vm{
		Id: vmId,
		//Pod:    nil,
		Lazy:   lazy,
		Status: types.S_VM_IDLE,
		Cpu:    cpu,
		Mem:    memory,
	}
}

func GetVm(vmId string, b *BootConfig, waitStarted, lazy bool) (vm *Vm, err error) {
	id := vmId
	if id == "" {
		for {
			id = fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))
			if _, err = os.Stat(BaseDir + "/" + id); os.IsNotExist(err) {
				break
			}
		}
	}

	vm = NewVm(id, b.CPU, b.Memory, lazy)
	if err = vm.Launch(b); err != nil {
		return nil, err
	}

	if waitStarted {
		// wait init connected
		Status, err := vm.GetResponseChan()
		if err != nil {
			vm.Kill()
			return nil, err
		}
		defer vm.ReleaseResponseChan(Status)
		for {
			vmResponse, ok := <-Status
			if !ok || vmResponse.Code == types.E_FAILED {
				vm.Kill()
				return nil, err
			}

			if vmResponse.Code == types.E_VM_RUNNING {
				break
			}
		}
	}
	return vm, nil

}
