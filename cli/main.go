package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/docker/docker/pkg/reexec"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/urfave/cli"
)

var (
	version   = ""
	gitCommit = ""
)

const (
	specConfig = "config.json"
	stateJSON  = "state.json"
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
    # runv run [ -b bundle ] <container-id>

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
	app.Version = fmt.Sprintf("%s, commit: %s", version, gitCommit)
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
			Name:  "bios",
			Usage: "bios for the container",
		},
		cli.StringFlag{
			Name:  "cbfs",
			Usage: "cbfs for the container",
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
	app.After = func(context *cli.Context) error {
		// make sure glog flush all the messages to file
		glog.Flush()
		return nil
	}

	app.Commands = []cli.Command{
		createCommand,
		execCommand,
		killCommand,
		listCommand,
		psCommand,
		runCommand,
		specCommand,
		startCommand,
		stateCommand,
		manageCommand,
		pauseCommand,
		resumeCommand,
		deleteCommand,
		proxyCommand,
		shimCommand,
		nsListenCommand,
	}
	if err := app.Run(os.Args); err != nil {
		glog.Errorf("app.Run(os.Args) failed with err: %#v", err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// runvOptions is used for create aux runv processes (networklistener/shim/proxy)
type runvOptions struct {
	*cli.Context
	withContainer *State
}

func cmdPrepare(context *cli.Context, setupHypervisor, canLogToStderr bool) error {
	if setupHypervisor {
		var err error
		if hypervisor.HDriver, err = driverloader.Probe(context.GlobalString("driver")); err != nil {
			return err
		}
		setupHyperstartFunc(context)
	}

	logdir := context.GlobalString("log_dir")
	if logdir != "" {
		if err := os.MkdirAll(logdir, 0750); err != nil {
			return fmt.Errorf("can't create dir %q for log files", logdir)
		}
	}
	if !context.GlobalBool("debug") {
		flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", logdir})
	} else if canLogToStderr {
		flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", logdir, "--alsologtostderr"})
	} else {
		flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", logdir})
	}
	return nil
}
