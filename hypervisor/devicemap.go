package hypervisor

import (
//	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (ctx *VmContext) deviceReady() bool {
	ready := ctx.progress.adding.isEmpty() && ctx.progress.deleting.isEmpty()
	if ready && ctx.wait {
		ctx.Log(DEBUG, "All resource being released, someone is waiting for us...")
		ctx.wg.Done()
		ctx.wait = false
	}

	return ready
}

func (ctx *VmContext) onContainerRemoved(c *ContainerUnmounted) bool {
	ctx.lock.Lock()
	defer ctx.lock.Unlock()

	if _, ok := ctx.progress.deleting.containers[c.Index]; ok {
		glog.V(1).Infof("container %d umounted", c.Index)
		delete(ctx.progress.deleting.containers, c.Index)
	}

	return c.Success
}

func (ctx *VmContext) onVolumeRemoved(v *VolumeUnmounted) bool {
	if _, ok := ctx.progress.deleting.volumes[v.Name]; ok {
		glog.V(1).Infof("volume %s umounted", v.Name)
		delete(ctx.progress.deleting.volumes, v.Name)
	}
	return v.Success
}

func (ctx *VmContext) removeVolumeDrive() {
	for name, vol := range ctx.devices.volumeMap {
		if vol.info.Format == "raw" || vol.info.Format == "qcow2" || vol.info.Format == "vdi" || vol.info.Format == "rbd" {
			glog.V(1).Infof("need detach volume %s (%s) ", name, vol.info.DeviceName)
			ctx.DCtx.RemoveDisk(ctx, vol.info, &VolumeUnmounted{Name: name, Success: true})
			ctx.progress.deleting.volumes[name] = true
		}
	}
}

func (ctx *VmContext) removeImageDrive() {
	for _, image := range ctx.devices.imageMap {
		if image.info.Fstype != "dir" {
			glog.V(1).Infof("need eject no.%d image block device: %s", image.pos, image.info.DeviceName)
			ctx.progress.deleting.containers[image.pos] = true
			ctx.DCtx.RemoveDisk(ctx, image.info, &ContainerUnmounted{Index: image.pos, Success: true})
		}
	}
}

func (ctx *VmContext) releaseAllNetwork() {
	var maps []pod.UserContainerPort

	for _, c := range ctx.userSpec.Containers {
		for _, m := range c.Ports {
			maps = append(maps, m)
		}
	}

	for idx, nic := range ctx.devices.networkMap {
		glog.V(1).Infof("remove network card %d: %s", idx, nic.IpAddr)
		ctx.progress.deleting.networks[idx] = true
		go ctx.networks.cleanupInf(idx, nic.IpAddr, nic.Fd, maps)
		maps = nil
	}
}
