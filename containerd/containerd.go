package containerd

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

var ContainerdCommand = cli.Command{
	Name:  "containerd",
	Usage: usage,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "state-dir",
			Value: defaultStateDir,
			Usage: "runtime state directory",
		},
		cli.StringFlag{
			Name:  "listen,l",
			Value: defaultGRPCEndpoint,
			Usage: "Address on which GRPC API will listen",
		},
	},
	Action: func(context *cli.Context) {
		kernel := context.GlobalString("kernel")
		initrd := context.GlobalString("initrd")
		stateDir := context.String("state-dir")

		if context.GlobalBool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", context.GlobalString("log_dir"), "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", context.GlobalString("log_dir")})
		}

		if kernel == "" || initrd == "" {
			glog.Infof("argument kernel and initrd must be set")
			os.Exit(1)
		}
		hypervisor.InterfaceCount = 0
		var err error
		if hypervisor.HDriver, err = driverloader.Probe(context.GlobalString("driver")); err != nil {
			glog.V(1).Infof("%s\n", err.Error())
			os.Exit(1)
		}

		f := factory.NewFromConfigs(kernel, initrd, nil)
		sv, err := supervisor.New(stateDir, stateDir, f)
		if err != nil {
			glog.Infof("%v", err)
			os.Exit(1)
		}

		if err = daemon(sv, context.String("listen")); err != nil {
			glog.Infof("%v", err)
			os.Exit(1)
		}
	},
}

func daemon(sv *supervisor.Supervisor, address string) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT)

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
