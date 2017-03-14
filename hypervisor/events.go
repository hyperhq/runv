package hypervisor

import (
	"os"
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

type VmTimeout struct{}

type InitFailedEvent struct {
	Reason string
}

type InitConnectedEvent struct{}

type ShutdownCommand struct {
	Wait bool
}

type AttachCommand struct {
	Streams   *TtyIO
	Container string
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

type InterfaceCreated struct {
	Id         string //user specified in (ref api.InterfaceDescription: a user identifier of interface, user may use this to specify a nic, normally you can use IPAddr as an Id, however, in some driver (probably vbox?), user may not specify the IPAddr.)
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

type RouteRule struct {
	Destination string
	Gateway     string
	ViaThis     bool
}

type NetDevInsertedEvent struct {
	Id         string
	Index      int
	DeviceName string
	Address    int
}

func (ne *NetDevInsertedEvent) ResultId() string {
	return ne.Id
}

func (ne *NetDevInsertedEvent) IsSuccess() bool {
	return true
}

func (ne *NetDevInsertedEvent) Message() string {
	return "NIC inserted"
}

type NetDevRemovedEvent struct {
	Index int
}

type DeviceFailed struct {
	Session VmEvent
}

//Device Failed as api.Result
func (df *DeviceFailed) ResultId() string {
	switch s := df.Session.(type) {
	case *InterfaceCreated:
		return s.Id
	case *NetDevInsertedEvent:
		return s.Id
	default:
		return ""
	}
}

func (df *DeviceFailed) IsSuccess() bool {
	return false
}

func (df *DeviceFailed) Message() string {
	return "Device operation failed"
}

type Interrupted struct {
	Reason string
}

func (qe *VmStartFailEvent) Event() int      { return ERROR_VM_START_FAILED }
func (qe *VmExit) Event() int                { return EVENT_VM_EXIT }
func (qe *VmKilledEvent) Event() int         { return EVENT_VM_KILL }
func (qe *VmTimeout) Event() int             { return EVENT_VM_TIMEOUT }
func (qe *InitConnectedEvent) Event() int    { return EVENT_INIT_CONNECTED }
func (qe *VolumeUnmounted) Event() int       { return EVENT_BLOCK_EJECTED }
func (qe *BlockdevInsertedEvent) Event() int { return EVENT_BLOCK_INSERTED }
func (qe *InterfaceCreated) Event() int      { return EVENT_INTERFACE_ADD }
func (qe *NetDevInsertedEvent) Event() int   { return EVENT_INTERFACE_INSERTED }
func (qe *NetDevRemovedEvent) Event() int    { return EVENT_INTERFACE_EJECTED }
func (qe *AttachCommand) Event() int         { return COMMAND_ATTACH }
func (qe *ShutdownCommand) Event() int       { return COMMAND_SHUTDOWN }
func (qe *InitFailedEvent) Event() int       { return ERROR_INIT_FAIL }
func (qe *DeviceFailed) Event() int          { return ERROR_QMP_FAIL }
func (qe *Interrupted) Event() int           { return ERROR_INTERRUPTED }
