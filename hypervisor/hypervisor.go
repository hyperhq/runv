package hypervisor

import (
	"fmt"

	"github.com/hyperhq/hypercontainer-utils/hlog"
	"github.com/hyperhq/runv/agent"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/types"
)

func (ctx *VmContext) loop() {
	for ctx.handler != nil {
		ev, ok := <-ctx.Hub
		if !ok {
			ctx.Log(ERROR, "hub chan has already been closed")
			break
		} else if ev == nil {
			ctx.Log(DEBUG, "got nil event.")
			continue
		}
		ctx.Log(TRACE, "main event loop got message %d(%s)", ev.Event(), EventString(ev.Event()))
		ctx.handler(ctx, ev)
	}

	// Unless the ctx.Hub channel is drained, processes sending operations can
	// be left hanging waiting for a response. Since the handler is already
	// gone, we return a fail to all these requests.

	ctx.Log(DEBUG, "main event loop exiting")
}

func (ctx *VmContext) WaitSockConnected() {
	<-ctx.sockConnected
}

func (ctx *VmContext) Launch() {
	var err error

	ctx.DCtx.Launch(ctx)

	//launch routines
	if ctx.Boot.BootFromTemplate {
		ctx.Log(TRACE, "boot from template")
		ctx.PauseState = PauseStatePaused
		ctx.agent, err = agent.NewHyperstart(ctx.Id, ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, false, true)
	} else {
		ctx.agent, err = agent.NewHyperstart(ctx.Id, ctx.ctlSockAddr(), ctx.ttySockAddr(), 1, true, false)
	}
	if err != nil {
		ctx.Log(ERROR, "failed to create hypervisor")
	}
	close(ctx.sockConnected)
	go ctx.loop()
}

func VmAssociate(vmId string, hub chan VmEvent, client chan *types.VmResponse, pack []byte) (*VmContext, error) {

	if hlog.IsLogLevel(hlog.DEBUG) {
		hlog.Log(DEBUG, "VM %s trying to reload with serialized data: %s", vmId, string(pack))
	}

	pinfo, err := vmDeserialize(pack)
	if err != nil {
		return nil, err
	}

	if hlog.IsLogLevel(hlog.DEBUG) {
		hlog.Log(DEBUG, "VM %s trying to reload with deserialized pinfo: %#v", vmId, pinfo)
	}

	if pinfo.Id != vmId {
		return nil, fmt.Errorf("VM ID mismatch, %v vs %v", vmId, pinfo.Id)
	}

	context, err := pinfo.vmContext(hub, client)
	if err != nil {
		return nil, err
	}

	paused := context.PauseState == PauseStatePaused
	context.agent, err = agent.NewHyperstart(context.Id, context.ctlSockAddr(), context.ttySockAddr(), pinfo.HwStat.AttachId, false, paused)
	if err != nil {
		context.Log(ERROR, "failed to create hypervisor")
		return nil, err
	}

	context.DCtx.Associate(context)

	go context.loop()
	return context, nil
}

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	if driver, ok := HDriver.(BuildinNetworkDriver); ok {
		return driver.InitNetwork(bIface, bIP, disableIptables)
	}

	return network.InitNetwork(bIface, bIP, disableIptables)
}

func SupportLazyMode() bool {
	return HDriver.SupportLazyMode()
}
