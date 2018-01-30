package agent

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"syscall"
	"time"

	runvapi "github.com/hyperhq/runv/api"
	kagenta "github.com/kata-containers/agent/protocols/grpc"
	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
)

type kataAgent struct {
	conn  *grpc.ClientConn
	agent kagenta.AgentServiceClient
}

// NewKataAgent create a SandboxAgent with kata protocol
func NewKataAgent(kataAgentSock string) (SandboxAgent, error) {
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(5 * time.Second)}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		},
		))
	conn, err := grpc.Dial(kataAgentSock, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &kataAgent{
		conn:  conn,
		agent: kagenta.NewAgentServiceClient(conn),
	}, nil
}

func (kata *kataAgent) Close() {
	kata.conn.Close()
}

func (kata *kataAgent) LastStreamSeq() uint64 {
	return 0
}

func (kata *kataAgent) APIVersion() (uint32, error) {
	return 4244, nil
}

func (kata *kataAgent) PauseSync() error {
	// TODO:
	return nil
}

func (kata *kataAgent) Unpause() error {
	// TODO:
	return nil
}

func (kata *kataAgent) AddRoute(routes []Route) error {
	for _, r := range routes {
		_, err := kata.agent.AddRoute(context.Background(), &kagenta.RouteRequest{
			Route: &kagenta.Route{
				Dest:    r.Dest,
				Gateway: r.Gateway,
				Device:  r.Device,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (kata *kataAgent) UpdateInterface(t InfUpdateType, dev, newName string, ipAddresses []IpAddress, mtu uint64) error {
	op := kagenta.UpdateType_None
	inf := &kagenta.Interface{
		Device: dev,
		Name:   newName,
		Mtu:    mtu,
	}
	for _, addr := range ipAddresses {
		inf.IpAddresses = append(inf.IpAddresses, &kagenta.IPAddress{Address: addr.IpAddress, Mask: addr.NetMask})
	}
	switch t {
	case AddIP:
		op = kagenta.UpdateType_AddIP
	case DelIP:
		op = kagenta.UpdateType_RemoveIP
	case SetMtu:
		op = kagenta.UpdateType_MTU
	}
	if op != kagenta.UpdateType_None {
		_, err := kata.agent.UpdateInterface(context.Background(), &kagenta.UpdateInterfaceRequest{Type: op, Interface: inf})
		return err
	}
	var err error
	switch t {
	case AddInf:
		_, err = kata.agent.AddInterface(context.Background(), &kagenta.AddInterfaceRequest{Interface: inf})
	case DelInf:
		_, err = kata.agent.RemoveInterface(context.Background(), &kagenta.RemoveInterfaceRequest{Name: newName})
	default:
		err = fmt.Errorf("unknown")
	}
	return err
}

func (kata *kataAgent) WriteStdin(container, process string, data []byte) (int, error) {
	ret, err := kata.agent.WriteStdin(context.Background(), &kagenta.WriteStreamRequest{
		ContainerId: container,
		ExecId:      process,
		Data:        data,
	})
	if err == nil {
		return int(ret.Len), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (kata *kataAgent) ReadStdout(container, process string, data []byte) (int, error) {
	ret, err := kata.agent.ReadStdout(context.Background(), &kagenta.ReadStreamRequest{
		ContainerId: container,
		ExecId:      process,
		Len:         uint32(len(data)),
	})
	if err == nil {
		copy(data, ret.Data)
		return len(ret.Data), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (kata *kataAgent) ReadStderr(container, process string, data []byte) (int, error) {
	ret, err := kata.agent.ReadStderr(context.Background(), &kagenta.ReadStreamRequest{
		ContainerId: container,
		ExecId:      process,
		Len:         uint32(len(data)),
	})
	if err == nil {
		copy(data, ret.Data)
		return len(ret.Data), nil
	}
	// check if it is &grpc.rpcError{code:0xb, desc:"EOF"} and return io.EOF instead
	if grpc.Code(err) == codes.OutOfRange && grpc.ErrorDesc(err) == "EOF" {
		return 0, io.EOF
	}
	return 0, err
}

func (kata *kataAgent) CloseStdin(container, process string) error {
	_, err := kata.agent.CloseStdin(context.Background(), &kagenta.CloseStdinRequest{
		ContainerId: container,
		ExecId:      process,
	})
	return err
}

func (kata *kataAgent) TtyWinResize(container, process string, row, col uint16) error {
	_, err := kata.agent.TtyWinResize(context.Background(), &kagenta.TtyWinResizeRequest{
		ContainerId: container,
		ExecId:      process,
		Row:         uint32(row),
		Column:      uint32(col),
	})
	return err
}

func (kata *kataAgent) OnlineCpuMem() error {
	_, err := kata.agent.OnlineCPUMem(context.Background(), &kagenta.OnlineCPUMemRequest{})
	return err
}

func adaptorStorage2Kata(storages []*Storage) []*kagenta.Storage {
	kataStorages := make([]*kagenta.Storage, len(storages))
	for i, s := range storages {
		kataStorages[i] = &kagenta.Storage{
			Driver: s.Driver,
			//todo DriverOptions: s.DriverOptions,
			Source:     s.Source,
			Fstype:     s.Fstype,
			Options:    s.Options,
			MountPoint: s.MountPoint,
		}
	}
	return kataStorages
}

func (kata *kataAgent) CreateContainer(container string, user *runvapi.UserGroupInfo, storages []*Storage, c *ocispecs.Spec) error {
	kataSpec, err := kagenta.OCItoGRPC(c)
	if err != nil {
		return err
	}
	if user == nil {
		user = &runvapi.UserGroupInfo{}
	}
	_, err = kata.agent.CreateContainer(context.Background(), &kagenta.CreateContainerRequest{
		ContainerId: container,
		ExecId:      "init",
		StringUser: &kagenta.StringUser{
			Uid:            user.User,
			Gid:            user.Group,
			AdditionalGids: user.AdditionalGroups,
		},
		Storages: adaptorStorage2Kata(storages),
		OCI:      kataSpec,
	})
	return err
}

func (kata *kataAgent) StartContainer(id string) error {
	_, err := kata.agent.StartContainer(context.Background(), &kagenta.StartContainerRequest{
		ContainerId: id,
	})
	return err
}

func (kata *kataAgent) ExecProcess(container, process string, user *runvapi.UserGroupInfo, p *ocispecs.Process) error {
	kataProcess, err := kagenta.ProcessOCItoGRPC(p)
	if err != nil {
		return err
	}
	if user == nil {
		user = &runvapi.UserGroupInfo{}
	}
	_, err = kata.agent.ExecProcess(context.Background(), &kagenta.ExecProcessRequest{
		ContainerId: container,
		ExecId:      process,
		StringUser: &kagenta.StringUser{
			Uid:            user.User,
			Gid:            user.Group,
			AdditionalGids: user.AdditionalGroups,
		},
		Process: kataProcess,
	})
	return err
}

func (kata *kataAgent) SignalProcess(container, process string, signal syscall.Signal) error {
	_, err := kata.agent.SignalProcess(context.Background(), &kagenta.SignalProcessRequest{
		ContainerId: container,
		ExecId:      process,
		Signal:      uint32(signal),
	})
	return err
}

// wait the process until exit. like waitpid()
// the state is saved until someone calls WaitProcess() if the process exited earlier
// the non-first call of WaitProcess() after process started MAY fail to find the process if the process exited earlier
func (kata *kataAgent) WaitProcess(container, process string) int {
	ret, err := kata.agent.WaitProcess(context.Background(), &kagenta.WaitProcessRequest{
		ContainerId: container,
		ExecId:      process,
	})
	if err != nil {
		return -1
	}
	return int(ret.Status)
}

func (kata *kataAgent) StartSandbox(sb *runvapi.SandboxConfig, storages []*Storage) error {
	_, err := kata.agent.CreateSandbox(context.Background(), &kagenta.CreateSandboxRequest{
		Hostname:     sb.Hostname,
		Dns:          sb.Dns,
		Storages:     adaptorStorage2Kata(storages),
		SandboxPidns: true,
	})
	return err
}

func (kata *kataAgent) DestroySandbox() error {
	_, err := kata.agent.DestroySandbox(context.Background(), &kagenta.DestroySandboxRequest{})
	return err
}
