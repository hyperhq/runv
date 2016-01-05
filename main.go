package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/codegangsta/cli"
	"github.com/opencontainers/specs"
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
    # runv start start [ -b bundle ]

If not specified, the default value for the 'bundle' is the current directory.
'Bundle' is the directory where '` + specConfig + `' and '` + runtimeConfig + `' must be located.`
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
		cli.StringFlag{
			Name:  "id",
			Value: getDefaultID(),
			Usage: "specify the ID to be used for the container",
		},
		cli.StringFlag{
			Name:  "root",
			Value: specs.LinuxStateDirectory,
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

func getDefaultID() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Base(cwd)
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
