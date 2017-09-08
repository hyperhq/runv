package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/hyperhq/runv/api"
	"github.com/urfave/cli"
)

var interfaceCommand = cli.Command{
	Name:  "interface",
	Usage: "manage interfaces for container",
	Subcommands: []cli.Command{
		infAddCommand,
		infRmCommand,
		infUpdateCommand,
		infListCommand,
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, context.Bool("detach"))
	},
	Action: func(context *cli.Context) error {
		return nil
	},
}

var infAddCommand = cli.Command{
	Name:      "add",
	Usage:     "add an interface into a container",
	ArgsUsage: `add <container-id>`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "tapname",
			Usage: "set tap name, if interface with same name exists, use existing one instead of creating new one",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "set interface name in container",
		},
		cli.StringFlag{
			Name:  "ip",
			Usage: "set ip address with a mask, format: 192.168.0.2/24",
		},
		cli.StringFlag{
			Name:  "mac",
			Usage: "set mac address",
		},
		cli.IntFlag{
			Name:  "mtu",
			Usage: "set mtu",
		},
	},
	Action: func(context *cli.Context) error {
		container := context.Args().First()

		if container == "" {
			return cli.NewExitError("Please specify container ID", -1)
		}

		vm, lockfile, err := getSandbox(filepath.Join(context.GlobalString("root"), container, "sandbox"))
		if err != nil {
			return fmt.Errorf("failed to get sandbox for container %q: %v", container, err)
		}
		defer putSandbox(vm, lockfile)

		ip := context.String("ip")
		conf := &api.InterfaceDescription{
			Name:    context.String("name"),
			Ip:      []string{ip},
			Mac:     context.String("mac"),
			TapName: context.String("tapname"),
			Mtu:     context.Uint64("mtu"),
		}

		return vm.AddNic(conf)
	},
}

var infListCommand = cli.Command{
	Name:      "ls",
	Usage:     "list network interfaces in a container",
	ArgsUsage: `ls <container-id>`,
	Flags:     []cli.Flag{},
	Action: func(context *cli.Context) error {
		container := context.Args().First()

		if container == "" {
			return cli.NewExitError("Please specify container ID", -1)
		}

		vm, lockfile, err := getSandbox(filepath.Join(context.GlobalString("root"), container, "sandbox"))
		if err != nil {
			return fmt.Errorf("failed to get sandbox for container %q: %v", container, err)
		}
		defer putSandbox(vm, lockfile)

		tw := tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', 0)
		fmt.Fprintln(tw, "Name\tMac\tIP\tMtu")
		nics := vm.AllNics()
		for _, i := range nics {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", i.NewName, i.Mac, strings.Join(i.IpAddr, ","), i.Mtu)
		}
		tw.Flush()
		return nil
	},
}

var infRmCommand = cli.Command{
	Name:      "rm",
	Usage:     "remove an interface from container",
	ArgsUsage: `rm <container-id> <interface-name>`,
	Flags:     []cli.Flag{},
	Action: func(context *cli.Context) error {
		return nil
	},
}

var infUpdateCommand = cli.Command{
	Name:      "update",
	Usage:     "update configuration of interface",
	ArgsUsage: `update <container-id> <interface-name>`,
	Flags:     []cli.Flag{},
	Action: func(context *cli.Context) error {
		return nil
	},
}
