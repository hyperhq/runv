package hypervisor

import (
	"errors"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"os"
)

type BootConfig struct {
	CPU              int
	Memory           int
	HotAddCpuMem     bool
	BootToBeTemplate bool
	BootFromTemplate bool
	MemoryPath       string
	DevicesStatePath string
	Kernel           string
	Initrd           string
	Bios             string
	Cbfs             string
	Vbox             string
}

type HostNicInfo struct {
	Fd      uint64
	Device  string
	Mac     string
	Bridge  string
	Gateway string
}

type GuestNicInfo struct {
	Device  string
	Ipaddr  string
	Index   int
	Busaddr int
}

type HypervisorDriver interface {
	InitContext(homeDir string) DriverContext

	LoadContext(persisted map[string]interface{}) (DriverContext, error)

	BuildinNetwork() bool

	InitNetwork(bIface, bIP string, disableIptables bool) error

	SupportLazyMode() bool
}

var HDriver HypervisorDriver

type DriverContext interface {
	Launch(ctx *VmContext)
	Associate(ctx *VmContext)
	Dump() (map[string]interface{}, error)

	AddDisk(ctx *VmContext, sourceType string, blockInfo *BlockDescriptor)
	RemoveDisk(ctx *VmContext, blockInfo *BlockDescriptor, callback VmEvent)

	AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo)
	RemoveNic(ctx *VmContext, n *InterfaceCreated, callback VmEvent)

	AddCpu(ctx *VmContext, id int, callback VmEvent)
	AddMem(ctx *VmContext, slot, size int, callback VmEvent)

	Save(ctx *VmContext, path string, callback VmEvent)

	Shutdown(ctx *VmContext)
	Kill(ctx *VmContext)

	Pause(ctx *VmContext, cmd *PauseCommand)

	ConfigureNetwork(vmId, requestedIP string, maps []pod.UserContainerPort, config pod.UserInterface) (*network.Settings, error)
	AllocateNetwork(vmId, requestedIP string, maps []pod.UserContainerPort) (*network.Settings, error)
	ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort, file *os.File) error

	Stats(ctx *VmContext) (*types.PodStats, error)

	Close()
}

type LazyDriverContext interface {
	DriverContext

	LazyLaunch(ctx *VmContext)
	InitVM(ctx *VmContext) error
	LazyAddDisk(ctx *VmContext, name, sourceType, filename, format string, id int)
	LazyAddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo)
}

type EmptyDriver struct{}

type EmptyContext struct{}

func (ed *EmptyDriver) Initialize() error {
	return nil
}

func (ed *EmptyDriver) InitContext(homeDir string) DriverContext {
	return &EmptyContext{}
}

func (ed *EmptyDriver) LoadContext(persisted map[string]interface{}) (DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "empty" {
		return nil, errors.New("wrong driver type in persist info")
	}
	return &EmptyContext{}, nil
}

func (ed *EmptyDriver) SupportLazyMode() bool {
	return false
}

func (ec *EmptyContext) Launch(ctx *VmContext) {}

func (ec *EmptyContext) Associate(ctx *VmContext) {}

func (ec *EmptyContext) Dump() (map[string]interface{}, error) {
	return map[string]interface{}{"hypervisor": "empty"}, nil
}

func (ec *EmptyContext) AddDisk(ctx *VmContext, sourceType string, blockInfo *BlockDescriptor) {}

func (ec *EmptyContext) RemoveDisk(ctx *VmContext, blockInfo *BlockDescriptor, callback VmEvent) {
}

func (ec *EmptyContext) AddNic(ctx *VmContext, host *HostNicInfo, guest *GuestNicInfo) {}

func (ec *EmptyContext) RemoveNic(ctx *VmContext, n *InterfaceCreated, callback VmEvent) {}

func (ec *EmptyContext) AddCpu(ctx *VmContext, id int, callback VmEvent) {}
func (ec *EmptyContext) AddMem(ctx *VmContext, slot, size int, callback VmEvent) {
}

func (ec *EmptyContext) Save(ctx *VmContext, path string, callback VmEvent) {}

func (ec *EmptyContext) Shutdown(ctx *VmContext) {}

func (ec *EmptyContext) Kill(ctx *VmContext) {}

func (ec *EmptyContext) Pause(ctx *VmContext, cmd *PauseCommand) {}

func (ec *EmptyContext) BuildinNetwork() bool { return false }

func (ec *EmptyContext) ConfigureNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort, config pod.UserInterface) (*network.Settings, error) {
	return nil, nil
}

func (ec *EmptyContext) AllocateNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, nil
}

func (ec *EmptyContext) ReleaseNetwork(vmId, releasedIP string,
	maps []pod.UserContainerPort, file *os.File) error {
	return nil
}

func (ec *EmptyContext) Stats(ctx *VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (ec *EmptyContext) Close() {}
