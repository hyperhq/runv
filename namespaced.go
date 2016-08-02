package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docker/containerd/api/grpc/types"
	"github.com/docker/containerd/osutils"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/containerd/api/grpc/server"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/factory"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/supervisor"
	"google.golang.org/grpc"
)

func runvNamespaceDaemon() {
	var (
		namespace string
		state     string
		driver    string
		kernel    string
		initrd    string
	)
	flag.StringVar(&namespace, "namespace", "", "")
	flag.StringVar(&state, "state", "", "")
	flag.StringVar(&driver, "driver", "", "")
	flag.StringVar(&kernel, "kernel", "", "")
	flag.StringVar(&initrd, "initrd", "", "")
	flag.Parse()

	hypervisor.InterfaceCount = 0
	var err error
	if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		os.Exit(1)
	}

	daemon(namespace, state, kernel, initrd)
}

func daemon(namespace, state, kernel, initrd string) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)

	// TODO: make the factory create only one vm atmost
	f := factory.NewFromConfigs(kernel, initrd, nil)
	sv, err := supervisor.New(state, namespace, f)
	if err != nil {
		return err
	}

	address := filepath.Join(namespace, "namespaced.sock")
	server, err := startServer(address, sv)
	if err != nil {
		return err
	}
	go namespaceShare(sv, namespace, state, server)

	for ss := range s {
		switch ss {
		case syscall.SIGCHLD:
			if _, err := osutils.Reap(); err != nil {
				glog.Infof("containerd: reap child processes")
			}
		default:
			glog.Infof("stopping containerd after receiving %s", ss)
			server.Stop()
			os.RemoveAll(namespace)
			os.Exit(0)
		}
	}
	return nil
}

func namespaceShare(sv *supervisor.Supervisor, namespace, state string, server *grpc.Server) {
	events := sv.Events.Events(time.Time{})
	containerCount := 0
	for e := range events {
		if e.Type == supervisor.EventContainerStart {
			os.Symlink(namespace, filepath.Join(state, e.ID, "namespace"))
			containerCount++
		} else if e.Type == supervisor.EventExit && e.PID == "init" {
			containerCount--
			if containerCount == 0 {
				os.RemoveAll(namespace)
				time.Sleep(3 * time.Second)
				server.Stop()
				os.Exit(0)
			}
		}
	}
}

func startServer(address string, sv *supervisor.Supervisor) (*grpc.Server, error) {
	if err := os.RemoveAll(address); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", address)
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer()
	types.RegisterAPIServer(s, server.NewServer(sv))
	go func() {
		glog.Infof("containerd: grpc api on %s", address)
		if err := s.Serve(l); err != nil {
			glog.Infof("containerd: serve grpc error")
		}
	}()
	return s, nil
}
