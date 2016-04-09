package hypervisor

import (
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"

	"sync"
)

func (ctx *VmContext) startSocks() {
	go waitInitReady(ctx)
	go waitPts(ctx)
	if glog.V(1) {
		go waitConsoleOutput(ctx)
	}
}

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

func VmLoop(vmId string, hub chan VmEvent, client chan *types.VmResponse, boot *BootConfig, keep int) {
	context, err := InitContext(vmId, hub, client, nil, boot, keep)
	if err != nil {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	//launch routines
	context.startSocks()
	context.DCtx.Launch(context)

	context.loop()
}

func VmAssociate(vmId string, hub chan VmEvent, client chan *types.VmResponse,
	wg *sync.WaitGroup, pack []byte) {

	if glog.V(1) {
		glog.Infof("VM %s trying to reload with serialized data: %s", vmId, string(pack))
	}

	pinfo, err := vmDeserialize(pack)
	if err != nil {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	if pinfo.Id != vmId {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: "VM ID mismatch",
		}
		return
	}

	context, err := pinfo.vmContext(hub, client, wg)
	if err != nil {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	client <- &types.VmResponse{
		VmId: vmId,
		Code: types.E_OK,
	}

	context.DCtx.Associate(context)

	go waitPts(context)
	go connectToInit(context)
	if glog.V(1) {
		go waitConsoleOutput(context)
	}

	context.Become(stateRunning, StateRunning)

	for _, c := range context.vmSpec.Containers {
		context.ptys.startStdin(c.Process.Stdio, c.Process.Terminal)
	}

	go context.loop()
}

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	if HDriver.BuildinNetwork() {
		return HDriver.InitNetwork(bIface, bIP, disableIptables)
	}

	return network.InitNetwork(bIface, bIP, disableIptables)
}

func SupportLazyMode() bool {
	return HDriver.SupportLazyMode()
}
