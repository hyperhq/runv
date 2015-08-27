package hypervisor

import (
	"encoding/json"
	"fmt"

	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/glog"
)

func LazyVmLoop(vmId string, hub chan VmEvent, client chan *types.VmResponse, boot *BootConfig, keep int) {

	glog.V(1).Infof("Start VM %s in lazy mode, not started yet actually", vmId)

	context, err := InitContext(vmId, hub, client, nil, boot, keep)
	if err != nil {
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: err.Error(),
		}
		return
	}

	if _, ok := context.DCtx.(LazyDriverContext); !ok {
		glog.Error("not a lazy driver, cannot call lazy loop")
		context.reportBadRequest("not a lazy driver, cannot call lazy loop")
		return
	}

	err = context.DCtx.(LazyDriverContext).InitVM(context)
	if err != nil {
		estr := fmt.Sprintf("failed to create VM(%s): %s", vmId, err.Error())
		glog.Error(estr)
		client <- &types.VmResponse{
			VmId:  vmId,
			Code:  types.E_BAD_REQUEST,
			Cause: estr,
		}
		return
	}
	context.Become(statePreparing, "PREPARING")

	context.loop()
}

func statePreparing(ctx *VmContext, ev VmEvent) {
	switch ev.Event() {
	case EVENT_VM_EXIT, ERROR_INTERRUPTED:
		glog.Info("VM exited before start...")
	case COMMAND_SHUTDOWN, COMMAND_RELEASE:
		glog.Info("got shutdown or release command, not started yet")
		ctx.reportVmShutdown()
		ctx.Become(nil, "NONE")
	case COMMAND_EXEC:
		ctx.execCmd(ev.(*ExecCommand))
	case COMMAND_WINDOWSIZE:
		cmd := ev.(*WindowSizeCommand)
		ctx.setWindowSize(cmd.ClientTag, cmd.Size)
	case COMMAND_RUN_POD, COMMAND_REPLACE_POD:
		glog.Info("got spec, prepare devices")
		if ok := ctx.lazyPrepareDevice(ev.(*RunPodCommand)); ok {
			ctx.startSocks()
			ctx.DCtx.(LazyDriverContext).LazyLaunch(ctx)
			ctx.setTimeout(60)
			ctx.Become(stateStarting, "STARTING")
		} else {
			glog.Warning("Fail to prepare devices, quit")
			ctx.Become(nil, "None")
		}
	default:
		glog.Warning("got event during pod initiating")
	}
}

func (ctx *VmContext) lazyPrepareDevice(cmd *RunPodCommand) bool {

	if len(cmd.Spec.Containers) != len(cmd.Containers) {
		ctx.reportBadRequest("Spec and Container Info mismatch")
		return false
	}

	ctx.InitDeviceContext(cmd.Spec, cmd.Wg, cmd.Ip, cmd.Containers, cmd.Volumes)

	if glog.V(2) {
		res, _ := json.MarshalIndent(*ctx.vmSpec, "    ", "    ")
		glog.Info("initial vm spec: ", string(res))
	}

	err := ctx.lazyAllocateNetworks()
	if err != nil {
		ctx.reportVmFault(err.Error())
		return false
	}
	ctx.lazyAddBlockDevices()

	return true
}

func networkConfigure(info *InterfaceCreated) (*HostNicInfo, *GuestNicInfo) {
	return &HostNicInfo{
			Fd:      uint64(info.Fd.Fd()),
			Device:  info.HostDevice,
			Mac:     info.MacAddr,
			Bridge:  info.Bridge,
			Gateway: info.Bridge,
		}, &GuestNicInfo{
			Device:  info.DeviceName,
			Ipaddr:  info.IpAddr,
			Index:   info.Index,
			Busaddr: info.PCIAddr,
		}
}

func (ctx *VmContext) lazyAllocateNetworks() error {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for i := range ctx.progress.adding.networks {
		name := fmt.Sprintf("eth%d", i)
		addr := ctx.nextPciAddr()
		nic, err := ctx.allocateInterface(i, addr, name, maps)
		if err != nil {
			return err
		}
		ctx.interfaceCreated(nic)
		h, g := networkConfigure(nic)
		ctx.DCtx.(LazyDriverContext).LazyAddNic(ctx, h, g)
	}

	return nil
}

func (ctx *VmContext) lazyAddBlockDevices() {
	for blk := range ctx.progress.adding.blockdevs {
		if info, ok := ctx.devices.volumeMap[blk]; ok {
			sid := ctx.nextScsiId()
			ctx.DCtx.(LazyDriverContext).LazyAddDisk(ctx, info.info.name, "volume", info.info.filename, info.info.format, sid)
		} else if info, ok := ctx.devices.imageMap[blk]; ok {
			sid := ctx.nextScsiId()
			ctx.DCtx.(LazyDriverContext).LazyAddDisk(ctx, info.info.name, "image", info.info.filename, info.info.format, sid)
		} else {
			continue
		}
	}
}
