package hypervisor

import (
	"path/filepath"
	"sync"

	hyperstartapi "github.com/hyperhq/runv/agent/api/hyperstart"
	"github.com/hyperhq/runv/api"
)

type ContainerContext struct {
	*api.ContainerDescription

	sandbox *VmContext

	root *DiskContext

	logPrefix string
}

func (cc *ContainerContext) VmSpec() *hyperstartapi.Container {
	rootfsType := ""
	if !cc.RootVolume.IsDir() {
		rootfsType = cc.RootVolume.Fstype
	}

	rtContainer := &hyperstartapi.Container{ // runtime Container
		Id:            cc.Id,
		Rootfs:        cc.RootPath,
		Fstype:        rootfsType,
		Process:       hyperstartapi.ProcessFromOci("init", cc.OciSpec.Process),
		Sysctl:        cc.OciSpec.Linux.Sysctl,
		RestartPolicy: "never",
		Initialize:    cc.Initialize,
		ReadOnly:      cc.RootVolume.ReadOnly,
	}

	if cc.RootVolume.IsDir() {
		rtContainer.Image = cc.RootVolume.Source
	} else {
		rtContainer.Image = cc.root.DeviceName
		rtContainer.Addr = cc.root.ScsiAddr
	}
	cc.fillHyperstartVolSpec(rtContainer)

	cc.Log(TRACE, "generate vm container %#v", rtContainer)

	return rtContainer
}

func (cc *ContainerContext) fillHyperstartVolSpec(spec *hyperstartapi.Container) {
	// should be called after all the volumes are ready
	if !cc.root.isReady() {
		cc.Log(ERROR, "root volume insert failed")
		return
	}

	for vn, vcs := range cc.Volumes {
		vol, ok := cc.sandbox.volumes[vn]
		if !ok || !vol.isReady() {
			cc.Log(ERROR, "vol %s is failed to insert", vn)
			return
		}

		for _, mp := range vcs.MountPoints {
			if vol.IsDir() {
				cc.Log(DEBUG, "volume (fs mapping) %s is ready", vn)
				spec.Fsmap = append(spec.Fsmap, &hyperstartapi.FsmapDescriptor{
					Source:       vol.Filename,
					Path:         filepath.Clean(mp.Path),
					ReadOnly:     mp.ReadOnly,
					DockerVolume: vol.DockerVolume,
				})
			} else {
				cc.Log(DEBUG, "volume (disk) %s is ready", vn)
				spec.Volumes = append(spec.Volumes, &hyperstartapi.VolumeDescriptor{
					Device:       vol.DeviceName,
					Addr:         vol.ScsiAddr,
					Mount:        filepath.Clean(mp.Path),
					Fstype:       vol.Fstype,
					ReadOnly:     mp.ReadOnly,
					DockerVolume: vol.DockerVolume,
				})
			}
		}
	}
}

func (cc *ContainerContext) add(wgDisk *sync.WaitGroup, result chan api.Result) {
	wgDisk.Wait()
	for vn := range cc.Volumes {
		vol, ok := cc.sandbox.volumes[vn]
		if !ok || !vol.isReady() {
			cc.Log(ERROR, "vol %s is failed to insert", vn)
			result <- api.NewResultBase(cc.Id, false, "volume failed")
			return
		}
	}

	if !cc.root.isReady() {
		result <- api.NewResultBase(cc.Id, false, "root volume insert failed")
		return
	}

	if cc.sandbox.LogLevel(TRACE) {
		vmspec := cc.VmSpec()
		cc.Log(TRACE, "resource ready for container: %#v", vmspec)
	}

	cc.Log(TRACE, "all images and volume resources have been added to sandbox")
	result <- api.NewResultBase(cc.Id, true, "")
}
