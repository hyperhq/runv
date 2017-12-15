package agent

import (
	"syscall"

	runvapi "github.com/hyperhq/runv/api"
	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
)

type InfUpdateType uint64

const (
	AddInf InfUpdateType = 1 << iota
	DelInf
	AddIP
	DelIP
	SetMtu
)

// adaptor types for different protocols
type IpAddress struct {
	IpAddress string
	NetMask   string
}

type Route struct {
	Dest    string
	Gateway string
	Device  string
}

type Storage struct {
	Driver        string
	DriverOptions map[string]string

	Source     string
	Fstype     string
	Options    []string
	MountPoint string
}

// SandboxAgent interface to agent API
type SandboxAgent interface {
	Close()
	LastStreamSeq() uint64

	PauseSync() error
	Unpause() error

	APIVersion() (uint32, error)
	CreateContainer(container string, user *runvapi.UserGroupInfo, storages []*Storage, c *ocispecs.Spec) error
	StartContainer(container string) error
	ExecProcess(container, process string, user *runvapi.UserGroupInfo, p *ocispecs.Process) error
	SignalProcess(container, process string, signal syscall.Signal) error
	WaitProcess(container, process string) int

	WriteStdin(container, process string, data []byte) (int, error)
	ReadStdout(container, process string, data []byte) (int, error)
	ReadStderr(container, process string, data []byte) (int, error)
	CloseStdin(container, process string) error
	TtyWinResize(container, process string, row, col uint16) error

	StartSandbox(sb *runvapi.SandboxConfig, storages []*Storage) error
	DestroySandbox() error
	AddRoute(r []Route) error
	UpdateInterface(t InfUpdateType, dev, newName string, addresses []IpAddress, mtu uint64) error
	OnlineCpuMem() error
}

var NewHyperstart = NewJsonBasedHyperstart
