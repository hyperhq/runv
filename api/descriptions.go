package api

import (
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

type Rlimit struct {
	// Type of the rlimit to set
	Type string `json:"type"`
	// Hard is the hard limit for the specified type
	Hard uint64 `json:"hard"`
	// Soft is the soft limit for the specified type
	Soft uint64 `json:"soft"`
}

type ContainerDescription struct {
	Id string

	// Static Info, got from client input
	Name  string
	Image string

	// User content or user specified behavior
	Labels        map[string]string `json:"labels"`
	Tty           bool              `json:"tty,omitempty"`

	// Creation Info, got during creation
	RootVolume *VolumeDescription // The root device of container, previous `Image` field of the ContainerInfo structure. if fstype is `dir`, this should be a path relative to share_dir, which described the mounted aufs or overlayfs dir.
	MountId    string
	RootPath   string // root path relative to the root volume, always be 'rootfs', previous `Rootfs` field of the ContainerInfo structure

	// runtime info, combined during creation
	UGI     *UserGroupInfo
	Envs    map[string]string //TODO: Should I use []string or map[string]string?
	Workdir string
	Path    string
	Args    []string
	Rlimits []*Rlimit
	Sysctl  map[string]string `json:"sysctl,omitempty"`

	StopSignal string

	Volumes map[string]*VolumeReference `json:"volumes"`

	Initialize bool // need to initialize container environment in start
}

type VolumeDescription struct {
	Name   string
	Source string

	Options *VolumeOption

	Fstype       string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format       string //"raw" (or "qcow2" later) for volume, "vfs" for dir path
	DockerVolume bool
}

type VolumeOption struct {
	User        string   `json:"user"`
	Monitors    []string `json:"monitors"`
	Keyring     string   `json:"keyring"`
	BytesPerSec int      `json:"bytespersec"`
	Iops        int      `json:"iops"`
}

type VolumeMount struct {
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly"`
}

type VolumeReference struct {
	Name        string `json:"name"`
	MountPoints []*VolumeMount
}

type SandboxConfig struct {
	Hostname  string
	Neighbors *NeighborNetworks
	Dns       []string
}

type InterfaceDescription struct {
	Id      string // a user identifier of interface, user may use this to specify a nic, normally you can use IPAddr as an Id, however, in some driver (probably vbox?), user may not specify the IPAddr.
	Lo      bool
	Bridge  string `json:"bridge"`
	Ip      string `json:"ip"`
	Mac     string `json:"mac,omitempty"`
	Gw      string `json:"gateway,omitempty"`
	TapName string
}

type PortDescription struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type NeighborNetworks struct {
	InternalNetworks []string `json:"internalNetworks,omitempty"`
	ExternalNetworks []string `json:"externalNetworks,omitempty"`
}

type UserGroupInfo struct {
	User             string   `json:"name"`
	Group            string   `json:"group"`
	AdditionalGroups []string `json:"additionalGroups,omitempty"`
}

func (v *VolumeDescription) IsDir() bool {
	return v.Format == "vfs"
}

func SandboxInfoFromOCF(s *specs.Spec) *SandboxConfig {
	return &SandboxConfig{
		Hostname: s.Hostname,
	}
}

func ContainerDescriptionFromOCF(id string, s *specs.Spec) *ContainerDescription {
	container := &ContainerDescription{
		Id:            id,
		Name:          s.Hostname,
		Image:         "",
		Labels:        make(map[string]string),
		Tty:           s.Process.Terminal,
		RootVolume:    nil,
		MountId:       "",
		RootPath:      "rootfs",
		UGI:           UGIFromOCF(&s.Process.User),
		Envs:          make(map[string]string),
		Workdir:       s.Process.Cwd,
		Path:          s.Process.Args[0],
		Args:          s.Process.Args[1:],
		Rlimits:       []*Rlimit{},
		Sysctl:        s.Linux.Sysctl,
	}

	for _, value := range s.Process.Env {
		values := strings.SplitN(value, "=", 2)
		container.Envs[values[0]] = values[1]
	}

	for idx := range s.Process.Rlimits {
		container.Rlimits = append(container.Rlimits, &Rlimit{
			Type: s.Process.Rlimits[idx].Type,
			Hard: s.Process.Rlimits[idx].Hard,
			Soft: s.Process.Rlimits[idx].Soft,
		})
	}

	rootfs := &VolumeDescription{
		Name:   id,
		Source: id,
		Fstype: "dir",
		Format: "vfs",
	}
	container.RootVolume = rootfs

	return container
}

func UGIFromOCF(u *specs.User) *UserGroupInfo {

	if u == nil || (u.UID == 0 && u.GID == 0 && len(u.AdditionalGids) == 0) {
		return nil
	}

	ugi := &UserGroupInfo{}
	if u.UID != 0 {
		ugi.User = strconv.FormatUint(uint64(u.UID), 10)
	}
	if u.GID != 0 {
		ugi.Group = strconv.FormatUint(uint64(u.GID), 10)
	}
	if len(u.AdditionalGids) > 0 {
		ugi.AdditionalGroups = []string{}
		for _, gid := range u.AdditionalGids {
			ugi.AdditionalGroups = append(ugi.AdditionalGroups, strconv.FormatUint(uint64(gid), 10))
		}
	}

	return ugi
}
