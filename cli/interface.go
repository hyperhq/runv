package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor"
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
		return cmdPrepare(context, true, true)
	},
	Action: func(context *cli.Context) error {
		return cli.ShowSubcommandHelp(context)
	},
}

var infAddCommand = cli.Command{
	Name:      "add",
	Usage:     "add an interface into a container",
	ArgsUsage: `add <container-id>`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "host-device",
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
		vm, releaseFunc, err := vmByContainerID(context, container)
		if err != nil {
			return err
		}
		defer releaseFunc()
		conf := &api.InterfaceDescription{
			Name:    context.String("name"),
			Ip:      context.String("ip"),
			Mac:     context.String("mac"),
			TapName: context.String("host-device"),
			Mtu:     context.Uint64("mtu"),
		}

		if err = vm.AddNic(conf); err != nil {
			return err
		}
		fmt.Println("Add interface successfully.")
		return nil
	},
}

var infListCommand = cli.Command{
	Name:      "ls",
	Aliases:   []string{"list"},
	Usage:     "list network interfaces in a container",
	ArgsUsage: `ls <container-id>`,
	Flags:     []cli.Flag{},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		vm, releaseFunc, err := vmByContainerID(context, container)
		if err != nil {
			return err
		}
		defer releaseFunc()

		tw := tabwriter.NewWriter(os.Stdout, 10, 1, 3, ' ', 0)
		fmt.Fprintln(tw, "Name\tMac\tIP\tMtu")
		nics := vm.AllNics()
		for _, i := range nics {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", i.NewName, i.MacAddr, i.IpAddr, i.Mtu)
		}
		tw.Flush()
		return nil
	},
}

var infRmCommand = cli.Command{
	Name:      "rm",
	Aliases:   []string{"delete"},
	Usage:     "remove an interface from container",
	ArgsUsage: `rm <container-id> <interface-name>`,
	Flags:     []cli.Flag{},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		inf := context.Args().Get(1)
		if inf == "" {
			return cli.NewExitError("please specify an interface to delete", -1)
		}
		vm, releaseFunc, err := vmByContainerID(context, container)
		if err != nil {
			return err
		}
		defer releaseFunc()

		nics := vm.AllNics()
		for _, i := range nics {
			if i.NewName == inf {
				if err = vm.DeleteNic(i.Id); err != nil {
					return cli.NewExitError(fmt.Sprintf("failed to delete interface %q: %v", inf, err), -1)
				}
				fmt.Printf("Interface %q is deleted\n", inf)
				break
			}
		}
		return nil
	},
}

var infUpdateCommand = cli.Command{
	Name:      "update",
	Usage:     "update configuration of interface",
	ArgsUsage: `update <container-id> <interface-name>`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "add-ip",
			Usage: "add a new ip address with mask (format: 192.168.0.2/24)",
		},
		cli.StringFlag{
			Name:  "delete-ip",
			Usage: "add a new ip address with mask (format: 192.168.0.2/24)",
		},
		cli.IntFlag{
			Name:  "mtu",
			Usage: "update mtu",
		},
	},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		vm, releaseFunc, err := vmByContainerID(context, container)
		if err != nil {
			return err
		}
		defer releaseFunc()

		targetInf := context.Args().Get(1)
		if targetInf == "" {
			return cli.NewExitError("please specify an interface name to update", -1)
		}

		conf := &api.InterfaceDescription{
			Id:  "-1",
			Mtu: context.Uint64("mtu"),
		}
		if ip := context.String("add-ip"); ip != "" {
			conf.Ip = ip
		}
		if ip := context.String("delete-ip"); ip != "" {
			// an IP address prefixed with "-" indicates deleting an ip
			conf.Ip += ",-" + ip
		}

		nics := vm.AllNics()
		for _, i := range nics {
			if i.NewName == targetInf {
				conf.Id = i.Id
				break
			}
		}

		if conf.Id == "-1" {
			return cli.NewExitError(fmt.Sprintf("Can't find target interface name %q", targetInf), -1)
		}

		if err = vm.UpdateNic(conf); err != nil {
			return err
		}
		fmt.Printf("Interface %q is updated\n", targetInf)
		return nil
	},
}

type releaseFunc func()

func vmByContainerID(context *cli.Context, cid string) (*hypervisor.Vm, releaseFunc, error) {
	if cid == "" {
		return nil, nil, cli.NewExitError("Please specify container ID", -1)
	}

	vm, lockfile, err := getSandbox(filepath.Join(context.GlobalString("root"), cid, "sandbox"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get sandbox for container %q: %v", cid, err)
	}
	return vm, func() { putSandbox(vm, lockfile) }, nil
}
