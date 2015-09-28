package hypervisor

import (
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
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

// reportVmFault send report to daemon, notify about that:
//   1. vm op failed due to some reason described in `cause`
func (ctx *VmContext) reportVmFault(cause string) {
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_FAILED,
		Cause: cause,
	}
}

func (ctx *VmContext) reportPodIP() {
	ips := []string{}
	for _, i := range ctx.vmSpec.Interfaces {
		if i.Device == "lo" {
			continue
		}
		ips = append(ips, i.IpAddress)
	}
	ctx.client <- &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_OK,
		Cause: "",
		Data:  ips,
	}
}

func (ctx *VmContext) reportFile(code uint32, data []byte, err bool) {
	response := &types.VmResponse{
		VmId:  ctx.Id,
		Code:  types.E_WRITEFILE,
		Cause: "",
		Data:  data,
	}

	if code == INIT_READFILE {
		response.Code = types.E_READFILE
		if err {
			response.Cause = "readfile failed"
		}
		ctx.client <- response
	} else {
		if err == true {
			response.Cause = "writefile failed"
		}
		ctx.client <- response
	}
}
