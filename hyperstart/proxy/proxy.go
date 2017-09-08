package proxy

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	hyperstartgrpc "github.com/hyperhq/runv/hyperstart/api/grpc"
	hyperstartjson "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type jsonProxy struct {
	// json hyperstart api
	json libhyperstart.Hyperstart

	// grpc server
	address string
	self    *grpc.Server
}

func process4grpc2json(p *hyperstartgrpc.Process) *hyperstartjson.Process {
	envs := []hyperstartjson.EnvironmentVar{}
	for e, v := range p.Envs {
		envs = append(envs, hyperstartjson.EnvironmentVar{Env: e, Value: v})
	}
	return &hyperstartjson.Process{
		Id:               p.Id,
		User:             p.User.Uid,
		Group:            p.User.Gid,
		AdditionalGroups: p.User.AdditionalGids,
		Terminal:         p.Terminal,
		Envs:             envs,
		Args:             p.Args,
		Workdir:          p.Workdir,
	}
}

func container4grpc2json(c *hyperstartgrpc.Container, init *hyperstartgrpc.Process) *hyperstartjson.Container {
	hostfs := "vm:/dev/hostfs/"
	if len(c.Mounts) != 1 || !strings.HasPrefix(c.Mounts[0].Source, hostfs) || c.Mounts[0].Dest != "/" {
		panic("unsupported rootfs(temporary)")
	}
	return &hyperstartjson.Container{
		Id:      c.Id,
		Rootfs:  filepath.Base(strings.TrimPrefix(c.Mounts[0].Source, hostfs)),
		Image:   filepath.Dir(strings.TrimPrefix(c.Mounts[0].Source, hostfs)),
		Sysctl:  c.Sysctl,
		Process: process4grpc2json(init),
	}
}

func pbEmpty(err error) *google_protobuf.Empty {
	if err != nil {
		return nil
	}
	return &google_protobuf.Empty{}
}

// execution
func (proxy *jsonProxy) AddContainer(ctx context.Context, req *hyperstartgrpc.AddContainerRequest) (*google_protobuf.Empty, error) {
	c := container4grpc2json(req.Container, req.Init)
	err := proxy.json.NewContainer(c)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) AddProcess(ctx context.Context, req *hyperstartgrpc.AddProcessRequest) (*google_protobuf.Empty, error) {
	p := process4grpc2json(req.Process)
	err := proxy.json.AddProcess(req.Container, p)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) SignalProcess(ctx context.Context, req *hyperstartgrpc.SignalProcessRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.SignalProcess(req.Container, req.Process, syscall.Signal(req.Signal))
	return pbEmpty(err), err
}
func (proxy *jsonProxy) WaitProcess(ctx context.Context, req *hyperstartgrpc.WaitProcessRequest) (*hyperstartgrpc.WaitProcessResponse, error) {
	ret := proxy.json.WaitProcess(req.Container, req.Process)
	return &hyperstartgrpc.WaitProcessResponse{Status: int32(ret)}, nil
}

// stdio
func (proxy *jsonProxy) WriteStdin(ctx context.Context, req *hyperstartgrpc.WriteStreamRequest) (*hyperstartgrpc.WriteStreamResponse, error) {
	length, err := proxy.json.WriteStdin(req.Container, req.Process, req.Data)
	return &hyperstartgrpc.WriteStreamResponse{Len: uint32(length)}, err
}
func (proxy *jsonProxy) ReadStdout(ctx context.Context, req *hyperstartgrpc.ReadStreamRequest) (*hyperstartgrpc.ReadStreamResponse, error) {
	data := make([]byte, req.Len)
	length, err := proxy.json.ReadStdout(req.Container, req.Process, data)
	return &hyperstartgrpc.ReadStreamResponse{Data: data[0:length]}, err
}
func (proxy *jsonProxy) ReadStderr(ctx context.Context, req *hyperstartgrpc.ReadStreamRequest) (*hyperstartgrpc.ReadStreamResponse, error) {
	data := make([]byte, req.Len)
	length, err := proxy.json.ReadStderr(req.Container, req.Process, data)
	return &hyperstartgrpc.ReadStreamResponse{Data: data[0:length]}, err
}
func (proxy *jsonProxy) CloseStdin(ctx context.Context, req *hyperstartgrpc.CloseStdinRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.CloseStdin(req.Container, req.Process)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) TtyWinResize(ctx context.Context, req *hyperstartgrpc.TtyWinResizeRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.TtyWinResize(req.Container, req.Process, uint16(req.Row), uint16(req.Column))
	return pbEmpty(err), err
}

func (proxy *jsonProxy) StartSandbox(ctx context.Context, req *hyperstartgrpc.StartSandboxRequest) (*google_protobuf.Empty, error) {
	pod := &hyperstartjson.Pod{
		Hostname: req.Hostname,
		Dns:      req.Dns,
		ShareDir: "share_dir",
	}
	err := proxy.json.StartSandbox(pod)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) DestroySandbox(ctx context.Context, req *hyperstartgrpc.DestroySandboxRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.DestroySandbox()
	// we can not call proxy.self.GracefulStop() directly, otherwise the err would be "transport is closing"
	time.AfterFunc(10*time.Millisecond, proxy.self.GracefulStop)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) UpdateInterface(ctx context.Context, req *hyperstartgrpc.UpdateInterfaceRequest) (*google_protobuf.Empty, error) {
	addresses := []hyperstartjson.IpAddress{}
	for _, addr := range req.IpAddresses {
		addresses = append(addresses, hyperstartjson.IpAddress{addr.Address, addr.Mask})
	}

	err := proxy.json.UpdateInterface(req.Device, addresses)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) AddRoute(ctx context.Context, req *hyperstartgrpc.AddRouteRequest) (*google_protobuf.Empty, error) {
	routes := []hyperstartjson.Route{}
	for _, r := range req.Routes {
		routes = append(routes, hyperstartjson.Route{
			Dest:    r.Dest,
			Gateway: r.Gateway,
			Device:  r.Device,
		})
	}
	err := proxy.json.AddRoute(routes)
	return pbEmpty(err), err
}
func (proxy *jsonProxy) OnlineCPUMem(ctx context.Context, req *hyperstartgrpc.OnlineCPUMemRequest) (*google_protobuf.Empty, error) {
	err := proxy.json.OnlineCpuMem()
	return pbEmpty(err), err
}

func StartServer(address string, json libhyperstart.Hyperstart) (*grpc.Server, error) {

	s := grpc.NewServer()
	jp := &jsonProxy{
		json: json,
		self: s,
	}
	hyperstartgrpc.RegisterHyperstartServiceServer(s, jp)
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	return s, nil
}
