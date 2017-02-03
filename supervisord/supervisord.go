package supervisord

import (
	"encoding/json"
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/codegangsta/cli"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/factory"
	singlefactory "github.com/hyperhq/runv/factory/single"
	templatefactory "github.com/hyperhq/runv/factory/template"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/supervisor"
	"github.com/hyperhq/runv/supervisord/api/grpc/server"
	"github.com/hyperhq/runv/supervisord/api/grpc/types"
	"github.com/hyperhq/runv/supervisord/osutils"
	templatecore "github.com/hyperhq/runv/template"
	"google.golang.org/grpc"
)

const (
	usage               = `High performance hypervisor based container daemon`
	defaultStateDir     = "/run/runv-supervisord"
	defaultListenType   = "unix"
	defaultGRPCEndpoint = "/run/runv-supervisord/supervisord.sock"
)

var SupervisordCommand = cli.Command{
	Name:  "supervisord",
	Usage: usage,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "state-dir",
			Value: defaultStateDir,
			Usage: "runtime state directory",
		},
		cli.StringFlag{
			Name:  "supervisord-dir",
			Value: defaultStateDir,
			Usage: "supervisord daemon state directory",
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
	Action: func(context *cli.Context) {
		driver := context.GlobalString("driver")
		kernel := context.GlobalString("kernel")
		initrd := context.GlobalString("initrd")
		template := context.GlobalString("template")
		stateDir := context.String("state-dir")
		supervisordDir := context.String("supervisord-dir")
		if supervisordDir == "" {
			supervisordDir = stateDir
		}

		if context.GlobalBool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", context.GlobalString("log_dir"), "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", context.GlobalString("log_dir")})
		}

		var tconfig *templatecore.TemplateVmConfig
		if template != "" {
			path := filepath.Join(template, "config.json")
			f, err := os.Open(path)
			if err != nil {
				glog.Errorf("open template JSON configuration file failed: %v\n", err)
				os.Exit(-1)
			}
			if err := json.NewDecoder(f).Decode(&tconfig); err != nil {
				glog.Errorf("parse template JSON configuration file failed: %v\n", err)
				f.Close()
				os.Exit(-1)
			}
			f.Close()

			if (driver != "" && driver != tconfig.Driver) ||
				(kernel != "" && kernel != tconfig.Kernel) ||
				(initrd != "" && initrd != tconfig.Initrd) {
				glog.Infof("template config is not match the driver, kernel or initrd argument, disable template")
				template = ""
			} else if driver == "" {
				driver = tconfig.Driver
			}
		} else if kernel == "" || initrd == "" {
			glog.Infof("argument kernel and initrd must be set")
			os.Exit(1)
		}

		hypervisor.InterfaceCount = 0
		var err error
		if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
			glog.V(1).Infof("%s\n", err.Error())
			os.Exit(1)
		}

		var f factory.Factory
		if template != "" {
			f = singlefactory.New(templatefactory.NewFromExisted(tconfig))
		} else {
			f = factory.NewFromConfigs(kernel, initrd, nil)
		}
		sv, err := supervisor.New(stateDir, supervisordDir, f,
			context.GlobalInt("default_cpus"), context.GlobalInt("default_memory"))
		if err != nil {
			glog.Infof("%v", err)
			os.Exit(1)
		}

		if context.Bool("solo-namespaced") {
			go namespaceShare(sv, supervisordDir, stateDir)
		}

		if err = daemon(sv, context.String("listen")); err != nil {
			glog.Infof("%v", err)
			os.Exit(1)
		}

		if context.Bool("solo-namespaced") {
			os.RemoveAll(supervisordDir)
		}
	},
}

func daemon(sv *supervisor.Supervisor, address string) error {
	// setup a standard reaper so that we don't leave any zombies if we are still alive
	// this is just good practice because we are spawning new processes
	s := make(chan os.Signal, 2048)
	signal.Notify(s, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

	server, err := startServer(address, sv)
	if err != nil {
		return err
	}
	for ss := range s {
		switch ss {
		case syscall.SIGCHLD:
			if _, err := osutils.Reap(); err != nil {
				glog.Infof("supervisord: reap child processes")
			}
		default:
			glog.Infof("stopping supervisord after receiving %s", ss)
			time.Sleep(3 * time.Second) // TODO: fix it by proper way
			server.Stop()
			return nil
		}
	}
	return nil
}

func namespaceShare(sv *supervisor.Supervisor, namespace, state string) {
	events := sv.Events.Events(time.Time{})
	containerCount := 0
	for e := range events {
		if e.Type == supervisor.EventContainerStart {
			os.Symlink(namespace, filepath.Join(state, e.ID, "namespace"))
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
	go func() {
		glog.Infof("supervisord: grpc api on %s", address)
		if err := s.Serve(l); err != nil {
			glog.Infof("supervisord: serve grpc error")
		}
	}()
	return s, nil
}
