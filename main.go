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
			Value: getDefaultKernel(),
			Usage: "kernel for the container",
		},
		cli.StringFlag{
			Name:  "initrd",
			Value: getDefaultInitrd(),
			Usage: "runv-compatible initrd for the container",
		},
		cli.StringFlag{
			Name:  "vbox",
			Value: getDefaultVbox(),
			Usage: "runv-compatible boot ISO for the container for vbox driver",
		},
	}
	app.Commands = []cli.Command{
		startCommand,
		specCommand,
		execCommand,
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
		return "kvm"
	}
	if runtime.GOOS == "darwin" {
		return "vbox"
	}
	return ""
}

func firstExistingFile(candidates []string) string {
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			return file
		}
	}
	return ""
}

func getDefaultKernel() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return firstExistingFile([]string{"./kernel", "/var/lib/hyper/kernel"})
}

func getDefaultInitrd() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return firstExistingFile([]string{"./initrd.img", "/var/lib/hyper/hyper-initrd.img"})
}

func getDefaultVbox() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	return firstExistingFile([]string{"./vbox.iso", "/opt/hyper/static/iso/hyper-vbox-boot.iso"})
}
