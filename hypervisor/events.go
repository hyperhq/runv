package hypervisor

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

type ShutdownCommand struct {
	Wait bool
}
type ReleaseVMCommand struct{}

type VolumeUnmounted struct {
	Name    string
	Success bool
}

type BlockdevInsertedEvent struct {
	DeviceName string
	ScsiAddr   string // pass scsi addr to agent
}

type InterfaceCreated struct {
	Id         string //user specified in (ref api.InterfaceDescription: a user identifier of interface, user may use this to specify a nic, normally you can use IPAddr as an Id.)
	Index      int
	PCIAddr    int
	TapFd      int
	Bridge     string
	HostDevice string
	DeviceName string
	MacAddr    string
	IpAddr     string
	NewName    string
	Mtu        uint64
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
	TapFd      int
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
func (qe *VolumeUnmounted) Event() int       { return EVENT_BLOCK_EJECTED }
func (qe *BlockdevInsertedEvent) Event() int { return EVENT_BLOCK_INSERTED }
func (qe *InterfaceCreated) Event() int      { return EVENT_INTERFACE_ADD }
func (qe *NetDevInsertedEvent) Event() int   { return EVENT_INTERFACE_INSERTED }
func (qe *NetDevRemovedEvent) Event() int    { return EVENT_INTERFACE_EJECTED }
func (qe *ShutdownCommand) Event() int       { return COMMAND_SHUTDOWN }
func (qe *ReleaseVMCommand) Event() int      { return COMMAND_RELEASE }
func (qe *InitFailedEvent) Event() int       { return ERROR_INIT_FAIL }
func (qe *DeviceFailed) Event() int          { return ERROR_QMP_FAIL }
func (qe *Interrupted) Event() int           { return ERROR_INTERRUPTED }
