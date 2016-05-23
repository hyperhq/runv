package hypervisor

import (
	"fmt"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
)

// reportVmRun() send report to daemon, notify about that:
//    1. Vm has been running.
//    2. Init is ready for accepting commands
func (ctx *VmContext) reportVmRun() {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_VM_RUNNING,
		Cause: "Vm runs",
	}
}

// reportVmShutdown() send report to daemon, notify about that:
//    1. Vm has been shutdown
func (ctx *VmContext) reportVmShutdown() {
	defer func() {
		err := recover()
		if err != nil {
			glog.Warning("panic during send shutdown message to channel")
		}
	}()
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_VM_SHUTDOWN,
		Cause: "VM shut down",
	}
}

func (ctx *VmContext) reportPauseResult(result *PauseResult) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Reply: result.Reply,
		Cause: result.Cause,
	}
}

func (ctx *VmContext) reportPodRunning(msg string, data interface{}) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_RUNNING,
		Cause: msg,
		Data:  data,
	}
}

func (ctx *VmContext) reportPodStopped() {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_STOPPED,
		Cause: "All device detached successful",
	}
}

func (ctx *VmContext) reportPodFinished(result *PodFinished) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_FINISHED,
		Cause: "POD run finished",
		Data:  result.result,
	}
}

func (ctx *VmContext) reportContainerFinished(result *ContainerFinished) {
	glog.V(1).Infof("container finished result %v", result)
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_CONTAINER_FINISHED,
		Cause: "Container run finished",
		Data:  result,
	}
}

func (ctx *VmContext) reportSuccess(msg string, data interface{}) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_OK,
		Cause: msg,
		Data:  data,
	}
}

func (ctx *VmContext) reportBusy(msg string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_BUSY,
		Cause: msg,
	}
}

// reportBadRequest send report to daemon, notify about that:
//   1. anything wrong in the request, such as json format, slice length, etc.
func (ctx *VmContext) reportBadRequest(cause string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_BAD_REQUEST,
		Cause: cause,
	}
}

// reportUnexpectedRequest send report to daemon, notify about that:
//   1. unexpected event in current state
func (ctx *VmContext) reportUnexpectedRequest(ev VmEvent, state string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_UNEXPECTED,
		Reply: ev,
		Cause: "unexpected event during " + state,
	}
}

// reportVmFault send report to daemon, notify about that:
//   1. vm op failed due to some reason described in `cause`
func (ctx *VmContext) reportVmFault(cause string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_FAILED,
		Cause: cause,
	}
}

// reportExec send report to daemon, notify about that:
//   1. exec status
func (ctx *VmContext) reportExec(ev VmEvent, fail bool) {
	response := &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_EXEC_FINISH,
		Reply: ev,
		Cause: "",
	}

	if fail {
		response.Cause = "exec failed"
	}

	ctx.client <- response
}

func (ctx *VmContext) reportKill(ev VmEvent, ok bool) {
	code := types.E_OK
	if !ok {
		code = types.E_FAILED
	}
	response := &types.VmResponse{
		VmId:  ctx.Id,
		Code:  code,
		Reply: ev,
		Cause: "",
	}
	ctx.client <- response
}

func (ctx *VmContext) reportGenericOperation(ev VmEvent, success bool) {
	gop := ev.(*GenericOperation)
	if success {
		gop.Result <- nil
		return
	}

	gop.Result <- fmt.Errorf("fail to handle event %s", gop.OpName)
	close(gop.Result)
}

func (ctx *VmContext) reportPodIP(ev VmEvent) {
	ips := []string{}
	for _, i := range ctx.vmSpec.Interfaces {
		if i.Device == "lo" {
			continue
		}
		ips = append(ips, i.IpAddress)
	}
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_IP,
		Cause: "",
		Reply: ev,
		Data:  ips,
	}
}

func (ctx *VmContext) reportPodStats(ev VmEvent) {
	response := types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_POD_STATS,
		Cause: "",
		Reply: ev,
		Data:  nil,
	}

	stats, err := ctx.DCtx.Stats(ctx)
	if err != nil {
		response.Cause = "Get pod stats failed"
	} else {
		response.Data = stats
	}

	ctx.client <- &response
}

func (ctx *VmContext) reportFile(reply VmEvent, code uint32, data []byte, err bool) {
	response := &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_WRITEFILE,
		Cause: "",
		Reply: reply,
		Data:  data,
	}

	if code == hyperstartapi.INIT_READFILE {
		response.Code = types.E_READFILE
		if err {
			response.Cause = "readfile failed"
		}
	} else if err == true {
		response.Cause = "writefile failed"
	}

	ctx.client <- response
}
