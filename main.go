package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperhq/runv/containerd"
	"github.com/hyperhq/runv/containerd/api/grpc/types"
	_ "github.com/hyperhq/runv/supervisor/proxy" // for proxy.init()
	netcontext "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

const (
	version    = "0.8.0"
	specConfig = "config.json"
	stateJson  = "state.json"
	usage      = `Open Container Initiative hypervisor-based runtime

runv is a command line client for running applications packaged according to
the Open Container Format (OCF) and is a compliant implementation of the
Open Container Initiative specification.  However, due to the difference
between hypervisors and containers, the following sections of OCF don't
apply to runV:
    Namespace
    Capability
    Device
    "linux" and "mount" fields in OCI specs are ignored

The current release of "runV" supports the following hypervisors:
    KVM (QEMU 2.0 or later)
    Xen (4.5 or later)
    VirtualBox (Mac OS X)

After creating a spec for your root filesystem, you can execute a container
in your shell by running:

    # cd /mycontainer
    # runv start start [ -b bundle ] <container-id>

If not specified, the default value for the 'bundle' is the current directory.
'Bundle' is the directory where '` + specConfig + `' must be located.`
)

func main() {
	if reexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "runv"
	app.Usage = usage
	app.Version = version
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug output for logging, saved on the dir specified by log_dir via glog style",
		},
		cli.StringFlag{
			Name:  "log_dir",
			Value: "/var/log/hyper",
			Usage: "the directory for the logging (glog style)",
		},
		cli.StringFlag{
			Name:  "log",
			Usage: "[ignored on runv] set the log file path where internal debug information is written",
		},
		cli.StringFlag{
			Name:  "log-format",
			Usage: "[ignored on runv] set the format used by logs ('text' (default), or 'json')",
		},
		cli.StringFlag{
			Name:  "root",
			Value: "/run/runv",
			Usage: "root directory for storage of container state (this should be located in tmpfs)",
		},
		cli.StringFlag{
			Name:  "driver",
			Usage: "hypervisor driver (supports: kvm xen vbox)",
		},
		cli.IntFlag{
			Name:  "default_cpus",
			Usage: "default number of vcpus to assign pod",
			Value: 1,
		},
		cli.IntFlag{
			Name:  "default_memory",
			Usage: "default memory to assign pod (mb)",
			Value: 128,
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
			Name:  "template",
			Usage: "path to the template vm state directory",
		},
		cli.StringFlag{
			Name:  "vbox",
			Usage: "runv-compatible boot ISO for the container for vbox driver",
		},
	}
	app.Before = func(context *cli.Context) error {
		logdir := context.GlobalString("log_dir")
		if logdir != "" {
			if err := os.MkdirAll(logdir, 0750); err != nil {
				return fmt.Errorf("can't create dir %q for log files", logdir)
			}
		}
		if context.GlobalBool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", logdir, "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", logdir})
		}
		return nil
	}

	app.Commands = []cli.Command{
		createCommand,
		execCommand,
		killCommand,
		listCommand,
		runCommand,
		specCommand,
		startCommand,
		stateCommand,
		manageCommand,
		containerd.ContainerdCommand,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}

func getClient(address string) types.APIClient {
	// reset the logger for grpc to log to dev/null so that it does not mess with our stdio
	grpclog.SetLogger(log.New(ioutil.Discard, "", log.LstdFlags))
	dialOpts := []grpc.DialOption{grpc.WithInsecure(), grpc.WithTimeout(5 * time.Second)}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		},
		))
	conn, err := grpc.Dial(address, dialOpts...)
	if err != nil {
		fmt.Printf("grpc.Dial error: %v", err)
		os.Exit(-1)
	}
	return types.NewAPIClient(conn)
}

func containerEvents(c types.APIClient, container string) <-chan *types.Event {
	evChan := make(chan *types.Event)
	ts := time.Now()
	go func() {
		for {
			tsp, err := ptypes.TimestampProto(ts)
			if err != nil {
				close(evChan)
				return
			}
			events, err := c.Events(netcontext.Background(), &types.EventsRequest{Timestamp: tsp})
			if err != nil {
				fmt.Printf("c.Events error: %v", err)
				// TODO try to find a way to kill the process ?
				close(evChan)
				return
			}
			for {
				e, err := events.Recv()
				if err != nil {
					time.Sleep(1 * time.Second)
					break
				}
				ts, err = ptypes.Timestamp(e.Timestamp)
				if e.Id == container {
					evChan <- e
				}
			}
		}
	}()
	return evChan
}
