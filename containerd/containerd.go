package containerd

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/containerd/api/grpc/server"
	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/factory"
	singlefactory "github.com/hyperhq/runv/factory/single"
	templatefactory "github.com/hyperhq/runv/factory/template"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/supervisor"
	templatecore "github.com/hyperhq/runv/template"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

const (
	usage               = `High performance hypervisor based container daemon`
	defaultStateDir     = "/run/runv-containerd"
	defaultListenType   = "unix"
	defaultGRPCEndpoint = "/run/runv-containerd/containerd.sock"
	// runv-containerd is a relativly short term program
	// since we can't change the flush interval in glog, flush here manaully.
	glogFlushInterval = 5 * time.Second
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
			Name:  "containerd-dir",
			Value: defaultStateDir,
			Usage: "containerd daemon state directory",
		},
		cli.StringFlag{
			Name:  "listen,l",
			Value: defaultGRPCEndpoint,
			Usage: "Address on which GRPC API will listen",
		},
		cli.BoolFlag{
			Name:  "solo-namespaced",
			Usage: "launch as a solo namespaced for shared containers",
		},
	},
	Before: func(context *cli.Context) error {
		logdir := context.GlobalString("log_dir")
		if logdir != "" {
			if err := os.MkdirAll(logdir, 0750); err != nil {
				return fmt.Errorf("can't create dir %s for log files", logdir)
			}
		}
		if context.GlobalBool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", logdir, "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", logdir})
		}
		return nil
	},

	Action: func(context *cli.Context) {
		driver := context.GlobalString("driver")
		kernel := context.GlobalString("kernel")
		initrd := context.GlobalString("initrd")
		bios := context.GlobalString("bios")
		cbfs := context.GlobalString("cbfs")
		vsock := context.GlobalBool("vsock")
		template := context.GlobalString("template")
		stateDir := context.String("state-dir")
		containerdDir := context.String("containerd-dir")
		if containerdDir == "" {
			containerdDir = stateDir
		}

		var tconfig *templatecore.TemplateVmConfig
		if template != "" {
			path := filepath.Join(template, "config.json")
			f, err := os.Open(path)
			if err != nil {
				glog.Errorf("open template JSON configuration file failed: %v", err)
				os.Exit(-1)
			}
			if err := json.NewDecoder(f).Decode(&tconfig); err != nil {
				glog.Errorf("parse template JSON configuration file failed: %v", err)
				f.Close()
				os.Exit(-1)
			}
			f.Close()

			if (driver != "" && driver != tconfig.Driver) ||
				(kernel != "" && kernel != tconfig.Config.Kernel) ||
				(initrd != "" && initrd != tconfig.Config.Initrd) ||
				(bios != "" && bios != tconfig.Config.Bios) ||
				(cbfs != "" && cbfs != tconfig.Config.Cbfs) {
				glog.Warningf("template config is not match the driver, kernel, initrd, bios or cbfs argument, disable template")
				template = ""
			} else if driver == "" {
				driver = tconfig.Driver
			}
		} else if (bios == "" || cbfs == "") && (kernel == "" || initrd == "") {
			glog.Error("argument kernel+initrd or bios+cbfs must be set")
			os.Exit(1)
		}

		hypervisor.InterfaceCount = 0
		var err error
		if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
			glog.Errorf("%v", err)
			os.Exit(1)
		}

		var f factory.Factory
		if template != "" {
			f = singlefactory.New(templatefactory.NewFromExisted(tconfig))
		} else {
			bootConfig := hypervisor.BootConfig{
				Kernel:      kernel,
				Initrd:      initrd,
				Bios:        bios,
				Cbfs:        cbfs,
				EnableVsock: vsock,
			}
			f = singlefactory.Dummy(bootConfig)
		}
		sv, err := supervisor.New(stateDir, containerdDir, f,
			context.GlobalInt("default_cpus"), context.GlobalInt("default_memory"))
		if err != nil {
			glog.Errorf("%v", err)
			os.Exit(1)
		}

		if context.Bool("solo-namespaced") {
			go namespaceShare(sv, containerdDir, stateDir)
		}

		if err = daemon(sv, context.String("listen")); err != nil {
			glog.Errorf("%v", err)
			os.Exit(1)
		}

		if context.Bool("solo-namespaced") {
			os.RemoveAll(containerdDir)
		}
	},
}

func daemon(sv *supervisor.Supervisor, address string) error {
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	go glogFlushDaemon()

	server, err := startServer(address, sv)
	if err != nil {
		return err
	}
	glog.V(1).Infof("containerd daemon started successfully.")
	sig := <-s
	glog.V(1).Infof("stopping containerd after receiving %s", sig)
	time.Sleep(3 * time.Second) // TODO: fix it by proper way
	server.Stop()
	glog.Flush()
	return nil
}

func namespaceShare(sv *supervisor.Supervisor, namespace, state string) {
	events := sv.Events.Events(time.Time{})
	containerCount := 0
	for e := range events {
		if e.Type == supervisor.EventContainerStart {
			containerCount++
		} else if e.Type == supervisor.EventExit && e.PID == "init" {
			containerCount--
			if containerCount == 0 {
				syscall.Kill(0, syscall.SIGQUIT)
			}
		}
	}
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
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	go func() {
		glog.V(3).Infof("containerd: grpc api on %s", address)
		if err := s.Serve(l); err != nil {
			glog.Errorf("containerd serve grpc error: %v", err)
		}
		glog.V(1).Infof("containerd grpc server started")
	}()
	return s, nil
}

func glogFlushDaemon() {
	for range time.NewTicker(glogFlushInterval).C {
		glog.Flush()
	}
}
