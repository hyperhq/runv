package hypervisor

import (
	"net"
	"os"
	"sync"
	"syscall"

	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/pod"
)

type VmEvent interface {
	Event() int
}

type VmExit struct{}

type VmStartFailEvent struct {
	Message string
}

type VmKilledEvent struct {
	Success bool
}

type PodFinished struct {
	result []uint32
}

type VmTimeout struct{}

type InitFailedEvent struct {
	Reason string
}

type InitConnectedEvent struct {
	conn *net.UnixConn
}

type GetPodIPCommand struct {
	Id string
}

type GetPodStatsCommand struct {
	Id string
}

type RunPodCommand struct {
	Spec       *pod.UserPod
	Containers []*ContainerInfo
	Volumes    map[string]*VolumeInfo
	Wg         *sync.WaitGroup
}

type ReplacePodCommand RunPodCommand

type PauseCommand struct {
	Pause bool
}

type PauseResult struct {
	Cause string
	Reply *PauseCommand
}

type NewContainerCommand struct {
	container *pod.UserContainer
	info      *ContainerInfo
}

type ExecCommand struct {
	*TtyIO    `json:"-"`
	Container string                `json:"container,omitempty"`
	Process   hyperstartapi.Process `json:"process"`
}

type KillCommand struct {
	Container string         `json:"container"`
	Signal    syscall.Signal `json:"signal"`
}

type OnlineCpuMemCommand struct{}

type WriteFileCommand struct {
	Container string `json:"container"`
	File      string `json:"file"`
	Data      []byte `json:"-"`
}

type ReadFileCommand struct {
	Container string `json:"container"`
	File      string `json:"file"`
}

type StopPodCommand struct{}
type ShutdownCommand struct {
	Wait bool
}
type ReleaseVMCommand struct{}

type AttachCommand struct {
	Streams   *TtyIO
	Stderr    *TtyIO
	Size      *WindowSize
	Container string
}

type CommandAck struct {
	reply *DecodedMessage
	msg   []byte
}

type CommandError CommandAck

type WindowSizeCommand struct {
	ClientTag string
	Size      *WindowSize
}

type ContainerCreatedEvent struct {
	Index  int
	Id     string
	Rootfs string
	Image  string // if fstype is `dir`, this should be a path relative to share_dir
	// which described the mounted aufs or overlayfs dir.
	Fstype     string
	Workdir    string
	Entrypoint []string
	Cmd        []string
	Envs       map[string]string
}

type ContainerInfo struct {
	Id      string
	User    string
	MountId string
	Rootfs  string
	Image   string // if fstype is `dir`, this should be a path relative to share_dir
	// which described the mounted aufs or overlayfs dir.
	Fstype     string
	Workdir    string
	Entrypoint []string
	Cmd        []string
	Envs       map[string]string
	Initialize bool // need to initialize container environment in start
}

type ContainerUnmounted struct {
	Index   int
	Success bool
}

type VolumeReadyEvent struct {
	Name     string //volumen name in spec
	Filepath string //block dev absolute path, or dir path relative to share dir
	Fstype   string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format   string //"raw" (or "qcow2") for volume, no meaning for dir path
}

type VolumeInfo struct {
	Name         string //volumen name in spec
	Filepath     string //block dev absolute path, or dir path relative to share dir
	Fstype       string //"xfs", "ext4" etc. for block dev, or "dir" for dir path
	Format       string //"raw" (or "qcow2") for volume, no meaning for dir path
	DockerVolume bool
}

type VolumeUnmounted struct {
	Name    string
	Success bool
}

type BlockdevInsertedEvent struct {
	Name       string
	SourceType string //image or volume
	DeviceName string
	ScsiId     int
	ScsiAddr   string // pass scsi addr to hyperstart
}

type DevSkipEvent struct{}

type InterfaceCreated struct {
	Index      int
	PCIAddr    int
	Fd         *os.File
	Bridge     string
	HostDevice string
	DeviceName string
	MacAddr    string
	IpAddr     string
	NetMask    string
	RouteTable []*RouteRule
}

type InterfaceReleased struct {
	Index   int
	Success bool
}

type RouteRule struct {
	Destination string
	Gateway     string
	ViaThis     bool
}

type NetDevInsertedEvent struct {
	Index      int
	DeviceName string
	Address    int
}

type NetDevRemovedEvent struct {
	Index int
}

type DeviceFailed struct {
	Session VmEvent
}

type Interrupted struct {
	Reason string
}

type GenericOperation struct {
	OpName string
	State  []string
	OpFunc func(ctx *VmContext, result chan<- error)
	Result chan<- error
}

func (qe *VmStartFailEvent) Event() int      { return EVENT_VM_START_FAILED }
func (qe *VmExit) Event() int                { return EVENT_VM_EXIT }
func (qe *VmKilledEvent) Event() int         { return EVENT_VM_KILL }
func (qe *PauseCommand) Event() int          { return COMMAND_PAUSEVM }
func (qe *PauseResult) Event() int           { return EVENT_PAUSE_RESULT }
func (qe *VmTimeout) Event() int             { return EVENT_VM_TIMEOUT }
func (qe *PodFinished) Event() int           { return EVENT_POD_FINISH }
func (qe *InitConnectedEvent) Event() int    { return EVENT_INIT_CONNECTED }
func (qe *ContainerCreatedEvent) Event() int { return EVENT_CONTAINER_ADD }
func (qe *ContainerUnmounted) Event() int    { return EVENT_CONTAINER_DELETE }
func (qe *VolumeUnmounted) Event() int       { return EVENT_BLOCK_EJECTED }
func (qe *VolumeReadyEvent) Event() int      { return EVENT_VOLUME_ADD }
func (qe *BlockdevInsertedEvent) Event() int { return EVENT_BLOCK_INSERTED }
func (qe *DevSkipEvent) Event() int          { return EVENT_DEV_SKIP }
func (qe *InterfaceCreated) Event() int      { return EVENT_INTERFACE_ADD }
func (qe *InterfaceReleased) Event() int     { return EVENT_INTERFACE_DELETE }
func (qe *NetDevInsertedEvent) Event() int   { return EVENT_INTERFACE_INSERTED }
func (qe *NetDevRemovedEvent) Event() int    { return EVENT_INTERFACE_EJECTED }
func (qe *RunPodCommand) Event() int         { return COMMAND_RUN_POD }
func (qe *GetPodIPCommand) Event() int       { return COMMAND_GET_POD_IP }
func (qe *GetPodStatsCommand) Event() int    { return COMMAND_GET_POD_STATS }
func (qe *StopPodCommand) Event() int        { return COMMAND_STOP_POD }
func (qe *ReplacePodCommand) Event() int     { return COMMAND_REPLACE_POD }
func (qe *NewContainerCommand) Event() int   { return COMMAND_NEWCONTAINER }
func (qe *ExecCommand) Event() int           { return COMMAND_EXEC }
func (qe *KillCommand) Event() int           { return COMMAND_KILL }
func (qe *OnlineCpuMemCommand) Event() int   { return COMMAND_ONLINECPUMEM }
func (qe *WriteFileCommand) Event() int      { return COMMAND_WRITEFILE }
func (qe *ReadFileCommand) Event() int       { return COMMAND_READFILE }
func (qe *AttachCommand) Event() int         { return COMMAND_ATTACH }
func (qe *WindowSizeCommand) Event() int     { return COMMAND_WINDOWSIZE }
func (qe *ShutdownCommand) Event() int       { return COMMAND_SHUTDOWN }
func (qe *ReleaseVMCommand) Event() int      { return COMMAND_RELEASE }
func (qe *CommandAck) Event() int            { return COMMAND_ACK }
func (qe *GenericOperation) Event() int      { return GENERIC_OPERATION }
func (qe *InitFailedEvent) Event() int       { return ERROR_INIT_FAIL }
func (qe *DeviceFailed) Event() int          { return ERROR_QMP_FAIL }
func (qe *Interrupted) Event() int           { return ERROR_INTERRUPTED }
func (qe *CommandError) Event() int          { return ERROR_CMD_FAIL }
