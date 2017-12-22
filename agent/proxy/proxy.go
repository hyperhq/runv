package proxy

import (
	"fmt"
	"syscall"
	"time"

	google_protobuf "github.com/gogo/protobuf/types"
	"github.com/hyperhq/runv/agent"
	runvapi "github.com/hyperhq/runv/api"
	kagenta "github.com/kata-containers/agent/protocols/grpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type jsonProxy struct {
	// json hyperstart api
	json agent.SandboxAgent

	// grpc server
	address string
	self    *grpc.Server
}

func pbEmpty(err error) *google_protobuf.Empty {
	if err != nil {
		return nil
	}
	return &google_protobuf.Empty{}
}

func adaptorStorageFromKata(storages []*kagenta.Storage) []*agent.Storage {
	kataStorages := make([]*agent.Storage, len(storages))
	for i, s := range storages {
		kataStorages[i] = &agent.Storage{
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

// execution
func (proxy *jsonProxy) CreateContainer(ctx context.Context, req *kagenta.CreateContainerRequest) (*google_protobuf.Empty, error) {
	var ugi *runvapi.UserGroupInfo
	if req.StringUser != nil {
		ugi = &runvapi.UserGroupInfo{}
		ugi.User = req.StringUser.Uid
		ugi.Group = req.StringUser.Gid
		ugi.AdditionalGroups = req.StringUser.AdditionalGids
	}
	if req.OCI == nil {
		return nil, fmt.Errorf("missing req.OCI")
	}
	ociSpec, err := kagenta.GRPCtoOCI(req.OCI)
	if err != nil {
		return nil, fmt.Errorf("failed kagenta.GRPCtoOCI(req.OCI)")
	}
	err = proxy.json.CreateContainer(req.ContainerId, ugi, adaptorStorageFromKata(req.Storages), ociSpec)
	return pbEmpty(err), err
}

func (proxy *jsonProxy) StartContainer(ctx context.Context, req *kagenta.StartContainerRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.StartContainer(req.ContainerId)
	return pbEmpty(err), err
}

func (proxy *jsonProxy) RemoveContainer(ctx context.Context, req *kagenta.RemoveContainerRequest) (*google_protobuf.Empty, error) {
	return nil, fmt.Errorf("todo RemoveContainer() is to be implemented")
}

func (proxy *jsonProxy) ExecProcess(ctx context.Context, req *kagenta.ExecProcessRequest) (*google_protobuf.Empty, error) {
	var ugi *runvapi.UserGroupInfo
	if req.StringUser != nil {
		ugi = &runvapi.UserGroupInfo{}
		ugi.User = req.StringUser.Uid
		ugi.Group = req.StringUser.Gid
		ugi.AdditionalGroups = req.StringUser.AdditionalGids
	}
	if req.Process == nil {
		return nil, fmt.Errorf("missing req.Processs")
	}
	ociProcess, err := kagenta.ProcessGRPCtoOCI(req.Process)
	if err != nil {
		return nil, fmt.Errorf("failed kagenta.GRPCtoOCI(req.OCI)")
	}
	err = proxy.json.ExecProcess(req.ContainerId, req.ProcessId, ugi, ociProcess)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) SignalProcess(ctx context.Context, req *kagenta.SignalProcessRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.SignalProcess(req.ContainerId, req.ProcessId, syscall.Signal(req.Signal))
	return pbEmpty(err), err
}
func (proxy *jsonProxy) WaitProcess(ctx context.Context, req *kagenta.WaitProcessRequest) (*kagenta.WaitProcessResponse, error) {
	ret := proxy.json.WaitProcess(req.ContainerId, req.ProcessId)
	return &kagenta.WaitProcessResponse{Status: int32(ret)}, nil
}

// stdio
func (proxy *jsonProxy) WriteStdin(ctx context.Context, req *kagenta.WriteStreamRequest) (*kagenta.WriteStreamResponse, error) {
	length, err := proxy.json.WriteStdin(req.ContainerId, req.ProcessId, req.Data)
	return &kagenta.WriteStreamResponse{Len: uint32(length)}, err
}
func (proxy *jsonProxy) ReadStdout(ctx context.Context, req *kagenta.ReadStreamRequest) (*kagenta.ReadStreamResponse, error) {
	data := make([]byte, req.Len)
	length, err := proxy.json.ReadStdout(req.ContainerId, req.ProcessId, data)
	return &kagenta.ReadStreamResponse{Data: data[0:length]}, err
}
func (proxy *jsonProxy) ReadStderr(ctx context.Context, req *kagenta.ReadStreamRequest) (*kagenta.ReadStreamResponse, error) {
	data := make([]byte, req.Len)
	length, err := proxy.json.ReadStderr(req.ContainerId, req.ProcessId, data)
	return &kagenta.ReadStreamResponse{Data: data[0:length]}, err
}
func (proxy *jsonProxy) CloseStdin(ctx context.Context, req *kagenta.CloseStdinRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.CloseStdin(req.ContainerId, req.ProcessId)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) TtyWinResize(ctx context.Context, req *kagenta.TtyWinResizeRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.TtyWinResize(req.ContainerId, req.ProcessId, uint16(req.Row), uint16(req.Column))
	return pbEmpty(err), err
}

func (proxy *jsonProxy) CreateSandbox(ctx context.Context, req *kagenta.CreateSandboxRequest) (*google_protobuf.Empty, error) {
	sb := &runvapi.SandboxConfig{
		Hostname: req.Hostname,
		Dns:      req.Dns,
	}
	err := proxy.json.StartSandbox(sb, adaptorStorageFromKata(req.Storages))
	return pbEmpty(err), err
}
func (proxy *jsonProxy) DestroySandbox(ctx context.Context, req *kagenta.DestroySandboxRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.DestroySandbox()
	// we can not call proxy.self.GracefulStop() directly, otherwise the err would be "transport is closing"
	time.AfterFunc(10*time.Millisecond, proxy.self.GracefulStop)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) AddInterface(ctx context.Context, req *kagenta.AddInterfaceRequest) (*google_protobuf.Empty, error) {
	if req.Interface == nil {
		return nil, fmt.Errorf("AddInterface() wrong request: req.Interface == nil")
	}

	addresses := []agent.IpAddress{}
	for _, addr := range req.Interface.IpAddresses {
		addresses = append(addresses, agent.IpAddress{addr.Address, addr.Mask})
	}

	err := proxy.json.UpdateInterface(agent.AddInf, req.Interface.Device, req.Interface.Name, addresses, req.Interface.Mtu)
	return pbEmpty(err), err
}

func (proxy *jsonProxy) UpdateInterface(ctx context.Context, req *kagenta.UpdateInterfaceRequest) (*google_protobuf.Empty, error) {
	if req.Interface == nil {
		return nil, fmt.Errorf("UpdateInterface() wrong request: req.Interface == nil")
	}

	addresses := []agent.IpAddress{}
	for _, addr := range req.Interface.IpAddresses {
		addresses = append(addresses, agent.IpAddress{addr.Address, addr.Mask})
	}

	var err error
	if req.Type&(kagenta.UpdateType_AddIP|kagenta.UpdateType_RemoveIP) == kagenta.UpdateType_AddIP|kagenta.UpdateType_RemoveIP {
		err = fmt.Errorf("kagenta.UpdateType_AddIP|UpdateType_RemoveIP")
	}
	if err == nil && req.Type&req.Type&kagenta.UpdateType_Name != 0 {
		err = fmt.Errorf("kagenta.UpdateType_Name: unsupported yet")

	}
	if err == nil && req.Type&req.Type&kagenta.UpdateType_AddIP != 0 {
		err = proxy.json.UpdateInterface(agent.AddIP, req.Interface.Device, req.Interface.Name, addresses, req.Interface.Mtu)

	}
	if err == nil && req.Type&req.Type&kagenta.UpdateType_RemoveIP != 0 {
		err = proxy.json.UpdateInterface(agent.DelIP, req.Interface.Device, req.Interface.Name, addresses, req.Interface.Mtu)

	}
	if err == nil && req.Type&req.Type&kagenta.UpdateType_MTU != 0 {
		err = proxy.json.UpdateInterface(agent.SetMtu, req.Interface.Device, req.Interface.Name, addresses, req.Interface.Mtu)

	}
	return pbEmpty(err), err
}
func (proxy *jsonProxy) RemoveInterface(ctx context.Context, req *kagenta.RemoveInterfaceRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.UpdateInterface(agent.DelInf, "", req.Name, []agent.IpAddress{}, 0)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) AddRoute(ctx context.Context, req *kagenta.RouteRequest) (*google_protobuf.Empty, error) {
	routes := []agent.Route{}
	for _, r := range []*kagenta.Route{req.Route} {
		routes = append(routes, agent.Route{
			Dest:    r.Dest,
			Gateway: r.Gateway,
			Device:  r.Device,
		})
	}
	err := proxy.json.AddRoute(routes)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) RemoveRoute(ctx context.Context, req *kagenta.RouteRequest) (*google_protobuf.Empty, error) {
	routes := []agent.Route{}
	for _, r := range []*kagenta.Route{req.Route} {
		routes = append(routes, agent.Route{
			Dest:    r.Dest,
			Gateway: r.Gateway,
			Device:  r.Device,
		})
	}
	return nil, fmt.Errorf("todo RemoveRoute() is to be implemented %v", routes)
}
func (proxy *jsonProxy) OnlineCPUMem(ctx context.Context, req *kagenta.OnlineCPUMemRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.OnlineCpuMem()
	return pbEmpty(err), err
}

// NewServer initializes a brand new grpc server with registered grpc services
func NewServer(address string, json agent.SandboxAgent) (*grpc.Server, error) {
	s := grpc.NewServer()
	jp := &jsonProxy{
		json: json,
		self: s,
	}
	kagenta.RegisterAgentServiceServer(s, jp)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	return s, nil
}
