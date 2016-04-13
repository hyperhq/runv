package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/codegangsta/cli"
)

const (
	version       = "0.4.0"
	specConfig    = "config.json"
	runtimeConfig = "runtime.json"
	usage         = `Open Container Initiative hypervisor-based runtime

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
	if os.Args[0] == "runv-ns-daemon" {
		runvNamespaceDaemon()
		os.Exit(0)
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
			Value: getDefaultDriver(),
			Usage: "hypervisor driver (supports: kvm xen vbox)",
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
			Name:  "vbox",
			Usage: "runv-compatible boot ISO for the container for vbox driver",
		},
	}
	app.Commands = []cli.Command{
		startCommand,
		specCommand,
		execCommand,
		killCommand,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}

func getDefaultDriver() string {
	if runtime.GOOS == "linux" {
		return "qemu"
	}
	if runtime.GOOS == "darwin" {
		return "vbox"
	}
	return ""
}
