package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/supervisor"
	"github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
)

type apiServer struct {
	sv *supervisor.Supervisor
}

// NewServer returns grpc server instance
func NewServer(sv *supervisor.Supervisor) types.APIServer {
	return &apiServer{
		sv: sv,
	}
}

func (s *apiServer) CreateContainer(ctx context.Context, r *types.CreateContainerRequest) (*types.CreateContainerResponse, error) {
	glog.Infof("gRPC handle CreateContainer")
	if r.BundlePath == "" {
		return nil, errors.New("empty bundle path")
	}

	var spec specs.Spec
	ocfData, err := ioutil.ReadFile(filepath.Join(r.BundlePath, "config.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(ocfData, &spec); err != nil {
		return nil, err
	}

	c, p, err := s.sv.CreateContainer(r.Id, r.BundlePath, r.Stdin, r.Stdout, r.Stderr, &spec)
	if err != nil {
		return nil, err
	}

	glog.Infof("end Supervisor.CreateContainer(), build api Container")
	apiP := supervisorProcess2ApiProcess(p)
	apiC := supervisorContainer2ApiContainer(c)
	addApiProcess2ApiContainer(apiC, apiP)

	glog.Infof("gRPC respond CreateContainer")
	return &types.CreateContainerResponse{
		Container: apiC,
	}, nil
}

func (s *apiServer) Signal(ctx context.Context, r *types.SignalRequest) (*types.SignalResponse, error) {
	err := s.sv.Signal(r.Id, r.Pid, int(r.Signal))
	if err != nil {
		return nil, err
	}
	return &types.SignalResponse{}, nil
}

func (s *apiServer) AddProcess(ctx context.Context, r *types.AddProcessRequest) (*types.AddProcessResponse, error) {
	if r.Id == "" {
		return nil, fmt.Errorf("container id cannot be empty")
	}
	if r.Pid == "" {
		return nil, fmt.Errorf("process id cannot be empty")
	}
	spec := &specs.Process{
		Terminal: r.Terminal,
		Args:     r.Args,
		Env:      r.Env,
		Cwd:      r.Cwd,
	}
	_, err := s.sv.AddProcess(r.Id, r.Pid, r.Stdin, r.Stdout, r.Stderr, spec)
	if err != nil {
		return nil, err
	}
	return &types.AddProcessResponse{}, nil
}

func (s *apiServer) State(ctx context.Context, r *types.StateRequest) (*types.StateResponse, error) {
	return nil, errors.New("State() not implemented yet")
}

func (s *apiServer) UpdateContainer(ctx context.Context, r *types.UpdateContainerRequest) (*types.UpdateContainerResponse, error) {
	return nil, errors.New("UpdateContainer() not implemented yet")
}

func (s *apiServer) UpdateProcess(ctx context.Context, r *types.UpdateProcessRequest) (*types.UpdateProcessResponse, error) {
	var err error
	if r.CloseStdin {
		err = s.sv.CloseStdin(r.Id, r.Pid)
	} else {
		err = s.sv.TtyResize(r.Id, r.Pid, int(r.Width), int(r.Height))
	}
	if err != nil {
		return nil, err
	}
	return &types.UpdateProcessResponse{}, nil
}

func (s *apiServer) Events(r *types.EventsRequest, stream types.API_EventsServer) error {
	t := time.Time{}
	if r.Timestamp != 0 {
		t = time.Unix(int64(r.Timestamp), 0)
	}
	events := s.sv.Events.Events(t)
	defer s.sv.Events.Unsubscribe(events)
	for e := range events {
		if err := stream.Send(&types.Event{
			Id:        e.ID,
			Type:      e.Type,
			Timestamp: uint64(e.Timestamp.Unix()),
			Pid:       e.PID,
			Status:    uint32(e.Status),
		}); err != nil {
			return err
		}
	}
	return nil
}

// TODO implement
func (s *apiServer) CreateCheckpoint(ctx context.Context, r *types.CreateCheckpointRequest) (*types.CreateCheckpointResponse, error) {
	return nil, errors.New("CreateCheckpoint() not implemented yet")
}

// TODO implement
func (s *apiServer) DeleteCheckpoint(ctx context.Context, r *types.DeleteCheckpointRequest) (*types.DeleteCheckpointResponse, error) {
	return nil, errors.New("DeleteCheckpoint() not implemented yet")
}

// TODO implement
func (s *apiServer) ListCheckpoint(ctx context.Context, r *types.ListCheckpointRequest) (*types.ListCheckpointResponse, error) {
	return nil, errors.New("ListCheckpoint() not implemented yet")
}

// TODO implement
func (s *apiServer) Stats(ctx context.Context, r *types.StatsRequest) (*types.StatsResponse, error) {
	return nil, errors.New("Stats() not implemented yet")
}

func supervisorProcess2ApiProcess(p *supervisor.Process) *types.Process {
	return &types.Process{
		Pid:       p.Id,
		SystemPid: uint32(p.ProcId),
		Terminal:  p.Spec.Terminal,
		Args:      p.Spec.Args,
		Env:       p.Spec.Env,
		Cwd:       p.Spec.Cwd,
		Stdin:     p.Stdin,
		Stdout:    p.Stdout,
		Stderr:    p.Stderr,
	}
}

func supervisorContainer2ApiContainer(c *supervisor.Container) *types.Container {
	return &types.Container{
		Id:         c.Id,
		BundlePath: c.BundlePath,
		Status:     "running",
		Runtime:    "runv",
	}
}

func addApiProcess2ApiContainer(apiC *types.Container, apiP *types.Process) {
	apiC.Processes = append(apiC.Processes, apiP)
	apiC.Pids = append(apiC.Pids, apiP.SystemPid)
}
