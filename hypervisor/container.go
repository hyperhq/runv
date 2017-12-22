package hypervisor

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/hyperhq/runv/agent"
	hyperstartapi "github.com/hyperhq/runv/agent/api/hyperstart"
	"github.com/hyperhq/runv/api"
	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
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

func (cc *ContainerContext) agentStart() error {
	if !cc.root.isReady() {
		cc.Log(ERROR, "root volume insert failed")
		return fmt.Errorf("root volume insert failed")
	}

	// Make depth=1 copy of the spec, we will modify the root/mount without affecting the origin one
	spec := cc.ContainerDescription.OciSpec
	// and make full copy of the mount
	spec.Mounts = append([]ocispecs.Mount{}, cc.ContainerDescription.OciSpec.Mounts...)
	// agent.SandboxAgent will not change the data

	if spec.Root != nil {
		if spec.Root.Path != "" && spec.Root.Path != cc.ContainerDescription.RootPath {
			return fmt.Errorf("RootPath is not match")
		}
		if spec.Root.Readonly && !cc.RootVolume.ReadOnly {
			return fmt.Errorf("Readonly is not match")
		}
	}

	var root agent.Storage
	var storages []*agent.Storage
	if cc.RootVolume.IsDir() {
		// A faked sharefs based root storage
		root = shareStorage
		root.MountPoint = filepath.Join(root.MountPoint, cc.RootVolume.Source)
		// shareStorage was already added in agent.StartSandbox()
		storages = []*agent.Storage{}
	} else {
		if cc.RootVolume.Fstype == "" {
			return fmt.Errorf("fstype is not provided")
		}
		root.MountPoint = fmt.Sprintf("/kata/storage/%s/root_volume", cc.Id)
		root.Fstype = cc.RootVolume.Fstype
		root.Source = cc.root.DeviceName
		if cc.root.ScsiAddr != "" {
			root.Source = cc.root.ScsiAddr
			root.Driver = "scsi"
		}
		storages = []*agent.Storage{&root}
	}
	//if cc.Initialize {
	//	root.Driver = root.Driver + "dockerinit"
	//}
	spec.Root = &ocispecs.Root{
		Path:     filepath.Join(root.MountPoint, cc.ContainerDescription.RootPath),
		Readonly: cc.RootVolume.ReadOnly,
	}

	for vn, vcs := range cc.Volumes {
		vol, ok := cc.sandbox.volumes[vn]
		if !ok || !vol.isReady() {
			cc.Log(ERROR, "vol %s is failed to insert", vn)
			return fmt.Errorf("vol %s is failed to insert", vn)
		}
		if vol.IsDir() {
			cc.Log(DEBUG, "volume (fs mapping) %s is ready", vn)
			for _, mp := range vcs.MountPoints {
				// todo mp.ReadOnly, vol.DockerVolume,
				spec.Mounts = append(spec.Mounts, ocispecs.Mount{
					Destination: filepath.Clean(mp.Path),
					Source:      filepath.Join(shareStorage.MountPoint, vol.Filename),
				})
			}
		} else {
			if vol.Fstype == "" {
				return fmt.Errorf("fstype is not provided")
			}
			cc.Log(DEBUG, "volume (disk) %s is ready", vn)
			storage := &agent.Storage{
				Source:     vol.DeviceName,
				Fstype:     vol.Fstype,
				MountPoint: fmt.Sprintf("/kata/storage/%s/%s", cc.Id, vn),
			}
			if vol.ScsiAddr != "" {
				storage.Source = cc.root.ScsiAddr
				storage.Driver = "scsi"
			}
			storages = append(storages, storage)
			for _, mp := range vcs.MountPoints {
				// todo mp.ReadOnly, vol.DockerVolume,
				spec.Mounts = append(spec.Mounts, ocispecs.Mount{
					Destination: filepath.Clean(mp.Path),
					// docker and hyperstart expect "_data" is the volume dir inside the block
					Source: filepath.Join(storage.MountPoint, "_data"),
				})
			}
		}
	}

	err := cc.sandbox.agent.CreateContainer(cc.Id, cc.ContainerDescription.UGI, storages, &spec)
	if err != nil {
		cc.Log(ERROR, "agent.CreateContainer() failed: %v", err)
		return err
	}
	return cc.sandbox.agent.StartContainer(cc.Id)
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
