package hypervisor

import (
	"sync"

	"github.com/hyperhq/runv/api"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
)

type ContainerContext struct {
	*api.ContainerDescription

	sandbox *VmContext

	root *DiskContext

	process   *hyperstartapi.Process
	fsmap     []*hyperstartapi.FsmapDescriptor
	vmVolumes []*hyperstartapi.VolumeDescriptor
}

func (cc *ContainerContext) VmSpec() *hyperstartapi.Container {
	restart := "never"
	if cc.RestartPolicy != "" {
		restart = cc.RestartPolicy
	}

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
		Process:       cc.process,
		Sysctl:        cc.Sysctl,
		RestartPolicy: restart,
		Initialize:    cc.Initialize,
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

		for _, vc := range vcs {
			if vol.IsDir() {
				cc.Log(DEBUG, "volume (fs mapping) %s is ready", vn)
				cc.fsmap = append(cc.fsmap, &hyperstartapi.FsmapDescriptor{
					Source:       vol.Filename,
					Path:         vc.Path,
					ReadOnly:     vc.ReadOnly,
					DockerVolume: vol.DockerVolume,
				})
			} else {
				cc.Log(DEBUG, "volume (disk) %s is ready", vn)
				cc.vmVolumes = append(cc.vmVolumes, &hyperstartapi.VolumeDescriptor{
					Device:       vol.DeviceName,
					Addr:         vol.ScsiAddr,
					Mount:        vc.Path,
					Fstype:       vol.Fstype,
					ReadOnly:     vc.ReadOnly,
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

	cc.Log(INFO, "all images and volume resources have been added to sandbox")
	result <- api.NewResultBase(cc.Id, true, "")
}

func (cc *ContainerContext) configProcess() {
	c := cc.ContainerDescription

	envs := []hyperstartapi.EnvironmentVar{}
	for e, v := range c.Envs {
		envs = append(envs, hyperstartapi.EnvironmentVar{Env: e, Value: v})
	}
	cc.process = &hyperstartapi.Process{
		User:             c.UGI.User,
		Group:            c.UGI.Group,
		AdditionalGroups: c.UGI.AdditionalGroups,
		Terminal:         c.Tty,
		Stdio:            cc.sandbox.ptys.nextAttachId(),
		Stderr:           0,
		Args:             append([]string{c.Path}, c.Args...),
		Envs:             envs,
		Workdir:          c.Workdir,
		Rlimits:          make([]hyperstartapi.Rlimit, len(c.Rlimits)),
	}
	for i, l := range c.Rlimits {
		cc.process.Rlimits[i].Type = l.Type
		cc.process.Rlimits[i].Hard = l.Hard
		cc.process.Rlimits[i].Soft = l.Soft
	}

	if !c.Tty {
		cc.process.Stderr = cc.sandbox.ptys.nextAttachId()
	}

}
