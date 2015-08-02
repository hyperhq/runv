package hypervisor

import (
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
	"sync"
)

func (ctx *VmContext) loop() {
	for ctx.handler != nil {
		ev, ok := <-ctx.Hub
		if !ok {
			glog.Error("hub chan has already been closed")
			break
		} else if ev == nil {
			glog.V(1).Info("got nil event.")
			continue
		}
		glog.V(1).Infof("main event loop got message %d(%s)", ev.Event(), EventString(ev.Event()))
		ctx.handler(ctx, ev)
	}
}

func VmLoop(vmId string, hub chan VmEvent, client chan *types.QemuResponse, boot *BootConfig) {
	context, err := InitContext(vmId, hub, client, nil, boot)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	//launch routines
	go waitInitReady(context)
	go waitPts(context)
	if glog.V(1) {
		go waitConsoleOutput(context)
	}
	context.DCtx.Launch(context)

	context.loop()
}

func VmAssociate(vmId string, hub chan VmEvent, client chan *types.QemuResponse,
	wg *sync.WaitGroup, pack []byte) {

	if glog.V(1) {
		glog.Infof("VM %s trying to reload with serialized data: %s", vmId, string(pack))
	}

	pinfo, err := vmDeserialize(pack)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	if pinfo.Id != vmId {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: "VM ID mismatch",
		}
		return
	}

	context, err := pinfo.vmContext(hub, client, wg)
	if err != nil {
		client <- &types.QemuResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	client <- &types.QemuResponse{
		VmId: vmId,
		Code: types.E_OK,
	}

	context.DCtx.Associate(context)

	go waitPts(context)
	go connectToInit(context)
	if glog.V(1) {
		go waitConsoleOutput(context)
	}

	context.Become(stateRunning, "RUNNING")

	context.loop()
}

func InitNetwork(bIface, bIP string) error {
	if _, ok := HDriver.(InitNetwork); ok {
		return HDriver.InitNetwork(bIface, bIP)
	}

	return network.InitNetwork(bIface, bIP)
}
