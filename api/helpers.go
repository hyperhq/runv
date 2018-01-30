package api

import (
	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
)

func (v *VolumeDescription) IsDir() bool {
	return v.Format == "vfs"
}

func (v *VolumeDescription) IsNas() bool {
	return v.Format == "nas"
}

func SandboxInfoFromOCF(s *ocispecs.Spec) *SandboxConfig {
	return &SandboxConfig{
		Hostname: s.Hostname,
	}
}

func ContainerDescriptionFromOCF(id string, s *ocispecs.Spec) *ContainerDescription {
	container := &ContainerDescription{
		Id:         id,
		Name:       s.Hostname,
		Image:      "",
		Labels:     make(map[string]string),
		RootVolume: nil,
		RootPath:   "rootfs",
		OciSpec:    *s,
	}

	if container.OciSpec.Linux.Sysctl == nil {
		container.OciSpec.Linux.Sysctl = map[string]string{}
	}
	if _, ok := container.OciSpec.Linux.Sysctl["vm.overcommit_memory"]; !ok {
		container.OciSpec.Linux.Sysctl["vm.overcommit_memory"] = "1"
	}

	// the mounts need to be filtered and add it back after ContainerDescriptionFromOCF()
	// virtual mounts are added back here.
	container.OciSpec.Mounts = []ocispecs.Mount{}
	for _, m := range s.Mounts {
		switch m.Type {
		case "proc", "sysfs", "mqueue", "tmpfs", "cgroup", "devpts":
			container.OciSpec.Mounts = append(container.OciSpec.Mounts, m)
		}
	}
	container.OciSpec.Root.Path = "" // already mounted on filepath.Join(rootfs.Source, container.RootPath)

	rootfs := &VolumeDescription{
		Name:     id,
		Source:   id,
		Fstype:   "dir",
		Format:   "vfs",
		ReadOnly: s.Root.Readonly,
	}
	container.RootVolume = rootfs

	return container
}
