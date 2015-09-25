package main

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
)

const (
	version = "0.4.0"
	usage   = `Open Container Initiative hypervisor-based runtime

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
    # runv start [ -c spec-config-file ] [ -r runtime-config-file ]

If not specified, the default value for the 'spec-config-file' is 'config.json',
and the default value for the 'runtime-config-file' is 'runtime.json'.`
)

func main() {
	app := cli.NewApp()
	app.Name = "runv"
	app.Usage = usage
	app.Version = version
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "driver",
			Value: "kvm",
			Usage: "hypervisor driver (supports: kvm xen vbox)",
		},
		cli.StringFlag{
			Name:  "kernel",
			Value: "./kernel",
			Usage: "kernel for the container",
		},
		cli.StringFlag{
			Name:  "initrd",
			Value: "./initrd.img",
			Usage: "runv-compatible initrd for the container",
		},
		cli.StringFlag{
			Name:  "vbox",
			Value: "./vbox",
			Usage: "runv-compatible boot ISO for the container for vbox driver",
		},
	}
	app.Commands = []cli.Command{
		startCommand,
		specCommand,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%s\n", err.Error())
	}
}
