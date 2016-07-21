package api

import (
	"github.com/opencontainers/runtime-spec/specs-go"
)

type ContainerDescription struct {

	Id string

	// Static Info, got from client input
	Name  string
	Image string

	// User content or user specified behavior
	Labels        map[string]string     `json:"labels"`
	Tty           bool                  `json:"tty,omitempty"`
	RestartPolicy string                `json:"restartPolicy"`

	// Creation Info, got during creation
	RootVolume *VolumeDescription // The root device of container, previous `Image` field of the ContainerInfo structure. if fstype is `dir`, this should be a path relative to share_dir, which described the mounted aufs or overlayfs dir.
	MountId    string
	RootPath   string // root path relative to the root volume, always be 'rootfs', previous `Rootfs` field of the ContainerInfo structure

	// runtime info, combined during creation
	UGI        *UserGroupInfo
	Envs       map[string]string //TODO: Should I use []string or map[string]string?
	Workdir    string
	Path       string
	Args       []string
	Sysctl        map[string]string     `json:"sysctl,omitempty"`

	Volumes       map[string]*VolumeReference `json:"volumes"`

	Initialize bool // need to initialize container environment in start
}

type VolumeDescription struct {

	Name         string
	Source       string

	Options      *VolumeOption

	Fstype       string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format       string //"raw" (or "qcow2") for volume, no meaning for dir path
	DockerVolume bool
}

type VolumeOption struct {
	User        string   `json:"user"`
	Monitors    []string `json:"monitors"`
	Keyring     string   `json:"keyring"`
	BytesPerSec int      `json:"bytespersec"`
	Iops        int      `json:"iops"`
}

type VolumeReference struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	ReadOnly bool   `json:"readOnly"`
}

type SandboxConfig struct {
	Hostname   string
	Neighbors  *NeighborNetworks
	Dns        []string
}

// TODO: I think the ExecDescription is not essential
type ExecDescription struct {
	Id        string
	Container string
	Cmds      string
	Tty       bool
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

