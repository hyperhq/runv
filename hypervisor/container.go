package hypervisor

import (
	"path/filepath"
	"sync"

	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
)

type ContainerContext struct {
	*api.ContainerDescription

	sandbox *VmContext

	root *DiskContext

	fsmap     []*hyperstartapi.FsmapDescriptor
	vmVolumes []*hyperstartapi.VolumeDescriptor

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
		Volumes:       cc.vmVolumes,
		Fsmap:         cc.fsmap,
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

	cc.Log(TRACE, "generate vm container %#v", rtContainer)

	return rtContainer
}

func (cc *ContainerContext) add(wgDisk *sync.WaitGroup, result chan api.Result) {
	wgDisk.Wait()
	for vn, vcs := range cc.Volumes {
		vol, ok := cc.sandbox.volumes[vn]
		if !ok || !vol.isReady() {
			cc.Log(ERROR, "vol %s is failed to insert", vn)
			result <- api.NewResultBase(cc.Id, false, "volume failed")
			return
		}

		for _, mp := range vcs.MountPoints {
			if vol.IsDir() {
				cc.Log(DEBUG, "volume (fs mapping) %s is ready", vn)
				cc.fsmap = append(cc.fsmap, &hyperstartapi.FsmapDescriptor{
					Source:       vol.Filename,
					Path:         filepath.Clean(mp.Path),
					ReadOnly:     mp.ReadOnly,
					DockerVolume: vol.DockerVolume,
				})
			} else {
				cc.Log(DEBUG, "volume (disk) %s is ready", vn)
				cc.vmVolumes = append(cc.vmVolumes, &hyperstartapi.VolumeDescriptor{
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
