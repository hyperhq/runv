package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
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
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, true)
	},
	Action: func(context *cli.Context) error {
		absOption := func(option string) (string, error) {
			path := context.GlobalString(option)
			if path == "" {
				return "", fmt.Errorf("The global option --%s should be specified", option)
			}
			path, eabs := filepath.Abs(path)
			if eabs != nil {
				return "", fmt.Errorf("Failed to get the abs path of %s: %v", option, eabs)
			}
			return path, nil
		}
		kernel, err := absOption("kernel")
		if err != nil {
			return cli.NewExitError(err, -1)
		}
		initrd, err := absOption("initrd")
		if err != nil {
			return cli.NewExitError(err, -1)
		}
		template, err := absOption("template")
		if err != nil {
			return cli.NewExitError(err, -1)
		}

		if err := os.MkdirAll(template, 0700); err != nil {
			return cli.NewExitError(fmt.Errorf("Failed to create the template directory: %v", err), -1)
		}

		boot := hypervisor.BootConfig{
			CPU:         context.Int("cpu"),
			Memory:      context.Int("mem"),
			Kernel:      kernel,
			Initrd:      initrd,
			EnableVsock: context.GlobalBool("vsock"),
		}
		if _, err := templatecore.CreateTemplateVM(template, "", boot); err != nil {
			return cli.NewExitError(fmt.Errorf("Failed to create the template: %v", err), -1)
		}
		return nil
	},
}

var removeTemplateCommand = cli.Command{
	Name:  "remove-template",
	Usage: "remove the template VM on the directory specified by the global option --template",
	Action: func(context *cli.Context) error {
		if err := syscall.Unmount(context.GlobalString("template"), 0); err != nil {
			err := fmt.Errorf("Failed to remove the template: %v", err)
			glog.Error(err)
			return cli.NewExitError(err.Error(), -1)
		}
		return nil
	},
}
