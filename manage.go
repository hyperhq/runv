package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/hypervisor"
	templatecore "github.com/hyperhq/runv/template"
	"github.com/urfave/cli"
)

var manageSubCmds = []cli.Command{
	createTemplateCommand,
	removeTemplateCommand,
}

var manageCommand = cli.Command{
	Name:        "manage",
	Usage:       "manage VMs, network, defaults ....",
	ArgsUsage:   "COMMAND [arguments...]",
	Subcommands: manageSubCmds,
	Action: func(context *cli.Context) {
		cli.ShowSubcommandHelp(context)
	},
}

var createTemplateCommand = cli.Command{
	Name: "create-template",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "cpu",
			Value: 1,
			Usage: "the initial number of CPUs of the template VM",
		},
		cli.IntFlag{
			Name:  "mem",
			Value: 128,
			Usage: "the initial size of memory of the template VM",
		},
	},
	Usage: "create a template VM on the directory specified by the global option --template",
	Action: func(context *cli.Context) {
		absOption := func(option string) string {
			path := context.GlobalString(option)
			if path == "" {
				fmt.Printf("The global option --%s should be specified\n", option)
				os.Exit(-1)
			}
			path, eabs := filepath.Abs(path)
			if eabs != nil {
				fmt.Printf("Failed to get the abs path of %s: %v\n", option, eabs)
				os.Exit(-1)
			}
			return path
		}
		kernel := absOption("kernel")
		initrd := absOption("initrd")
		template := absOption("template")

		if err := os.MkdirAll(template, 0700); err != nil {
			fmt.Printf("Failed to create the template directory: %v\n", err)
			os.Exit(-1)
		}

		if context.GlobalBool("debug") {
			flag.CommandLine.Parse([]string{"-v", "3", "--log_dir", context.GlobalString("log_dir"), "--alsologtostderr"})
		} else {
			flag.CommandLine.Parse([]string{"-v", "1", "--log_dir", context.GlobalString("log_dir")})
		}

		var err error
		if hypervisor.HDriver, err = driverloader.Probe(context.GlobalString("driver")); err != nil {
			glog.V(1).Infof("%v\n", err)
			fmt.Printf("Failed to setup the driver: %v\n", err)
			os.Exit(-1)
		}

		if _, err := templatecore.CreateTemplateVM(template, "", context.Int("cpu"), context.Int("mem"), kernel, initrd, context.GlobalBool("vsock")); err != nil {
			fmt.Printf("Failed to create the template: %v\n", err)
			os.Exit(-1)
		}
	},
}

var removeTemplateCommand = cli.Command{
	Name:  "remove-template",
	Usage: "remove the template VM on the directory specified by the global option --template",
	Action: func(context *cli.Context) {
		if err := syscall.Unmount(context.GlobalString("template"), 0); err != nil {
			fmt.Printf("Failed to remove the template: %v\n", err)
			os.Exit(-1)
		}
	},
}
