package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/codegangsta/cli"
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

const (
	usage               = `High performance hypervisor based container daemon`
	defaultStateDir     = "/run/runv-containerd"
	defaultListenType   = "unix"
	defaultGRPCEndpoint = "/run/runv-containerd/containerd.sock"
)

var daemonFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "debug",
		Usage: "enable debug output in the logs",
	},
	cli.StringFlag{
		Name:  "log_dir",
		Value: "/var/log/hyper",
		Usage: "the directory for the logging (glog style)",
	},
	cli.StringFlag{
		Name:  "state-dir",
		Value: defaultStateDir,
		Usage: "runtime state directory",
	},
	cli.DurationFlag{
		Name:  "metrics-interval",
		Usage: "[ignore] interval for flushing metrics to the store",
	},
	cli.StringFlag{
		Name:  "listen,l",
		Value: defaultGRPCEndpoint,
		Usage: "Address on which GRPC API will listen",
	},
	cli.StringFlag{
		Name:  "runtime,r",
		Value: "runv",
		Usage: "[ignore] name of the OCI compliant runtime to use when executing containers",
	},
	cli.StringSliceFlag{
		Name:  "runtime-args",
		Usage: "[ignore] specify additional runtime args",
	},
	cli.StringFlag{
		Name:  "pprof-address",
		Usage: "[ignore] http address to listen for pprof events",
	},
	cli.DurationFlag{
		Name:  "start-timeout",
		Usage: "[ignore] timeout duration for waiting on a container to start before it is killed",
	},
	cli.StringFlag{
		Name:  "kernel",
		Usage: "kernel for the container",
	},
	cli.StringFlag{
		Name:  "initrd",
		Usage: "runv-compatible initrd for the container",
	},
	cli.StringFlag{
		Name:  "driver",
		Value: "qemu",
		Usage: "hypervisor driver",
	},
}

func main() {
	app := cli.NewApp()
	app.Name = "runv-containerd"
	app.Version = "0.01"
	app.Usage = usage
	app.Flags = daemonFlags

	app.Action = func(context *cli.Context) {
		if context.Bool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", context.String("log_dir"), "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", context.String("log_dir")})
		}

		if context.String("kernel") == "" || context.String("initrd") == "" {
			glog.Infof("argument kernel and initrd must be set")
			os.Exit(1)
		}
		hypervisor.InterfaceCount = 0
		var err error
		if hypervisor.HDriver, err = driverloader.Probe(context.String("driver")); err != nil {
			glog.V(1).Infof("%s\n", err.Error())
			os.Exit(1)
		}

		if err = daemon(
			context.String("listen"),
			context.String("state-dir"),
			context.String("kernel"),
			context.String("initrd"),
		); err != nil {
			glog.Infof("%v", err)
			os.Exit(1)
		}
	}
	if err := app.Run(os.Args); err != nil {
		glog.Infof("%v", err)
		os.Exit(1)
	}
}

func daemon(address, stateDir, kernel, initrd string) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)
	f := factory.NewFromConfigs(kernel, initrd, nil)
	sv, err := supervisor.New(stateDir, stateDir, f)
	if err != nil {
		return err
	}

	server, err := startServer(address, sv)
	if err != nil {
		return err
	}
	for ss := range s {
		switch ss {
		case syscall.SIGCHLD:
			if _, err := osutils.Reap(); err != nil {
				glog.Infof("containerd: reap child processes")
			}
		default:
			glog.Infof("stopping containerd after receiving %s", ss)
			server.Stop()
			os.Exit(0)
		}
	}
	return nil
}

func startServer(address string, sv *supervisor.Supervisor) (*grpc.Server, error) {
	if err := os.RemoveAll(address); err != nil {
		return nil, err
	}
	l, err := net.Listen(defaultListenType, address)
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
