package api

type SandboxConfig struct {
	Hostname   string
	Dns        []string
	Neighbors  *NeighborNetworks
	DnsOptions []string
	DnsSearch  []string
}

type ContainerDescription struct {
	Id string
	// Static Info, got from client input
	Name  string
	Image string
	// User content or user specified behavior
	Labels     map[string]string
	Tty        bool
	StopSignal string
	// Creation Info, got during creation
	RootVolume *VolumeDescription
	MountId    string
	RootPath   string
	// runtime info, combined during creation
	UGI        *UserGroupInfo
	Envs       map[string]string
	Workdir    string
	Path       string
	Args       []string
	Rlimits    []*Rlimit
	Sysctl     map[string]string
	Volumes    map[string]*VolumeReference
	Initialize bool
}

type VolumeDescription struct {
	Name         string
	Source       string
	Format       string
	Fstype       string
	Options      *VolumeOption
	DockerVolume bool
	ReadOnly     bool
}

type InterfaceDescription struct {
	Id      string
	Name    string
	Lo      bool
	Bridge  string
	Ip      string
	Mac     string
	Mtu     uint64
	Gw      string
	TapName string
	Options string
}

type PortDescription struct {
	HostPort      int32
	ContainerPort int32
	Protocol      string
}

type NeighborNetworks struct {
	InternalNetworks []string
	ExternalNetworks []string
}

type VolumeReference struct {
	Name        string
	MountPoints []*VolumeMount
}

type VolumeMount struct {
	Path     string
	ReadOnly bool
}

type VolumeOption struct {
	User        string
	Monitors    []string
	Keyring     string
	BytesPerSec int32
	Iops        int32
}

type UserGroupInfo struct {
	User             string
	Group            string
	AdditionalGroups []string
}

type Rlimit struct {
	Type string
	Hard uint64
	Soft uint64
}

type Process struct {
	Container       string
	Id              string
	User            string
	Group           string
	AdditionalGroup []string
	Terminal        bool
	Args            []string
	Envs            []string
	Workdir         string
}
