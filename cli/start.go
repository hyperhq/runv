package main

import (
	"os"
	"path/filepath"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name:  "start",
	Usage: "executes the user defined process in a created container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host.`,
	Description: `The start command executes the user defined process in a created container`,
	Flags:       []cli.Flag{},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, true)
	},
	Action: func(context *cli.Context) error {
		root := context.GlobalString("root")
		container := context.Args().First()

		if container == "" {
			return cli.NewExitError("Please specify container ID", -1)
		}
		if os.Geteuid() != 0 {
			return cli.NewExitError("runv should be run as root", -1)
		}

		state, spec, err := loadStateAndSpec(root, container)
		if err != nil {
			return cli.NewExitError(err, -1)
		}

		err = cmdStartContainer(context, container, spec, state)
		if err != nil {
			return cli.NewExitError(err, -1)
		}
		return nil
	},
}

func cmdStartContainer(context *cli.Context, container string, config *specs.Spec, state *specs.State) error {
	vm, fileLock, err := getSandbox(filepath.Join(context.GlobalString("root"), container, "sandbox"))
	if err != nil {
		return err
	}
	defer putSandbox(vm, fileLock) // todo handle error when disassociation
	err = startContainer(vm, container, config, state)
	if err != nil {
		// todo remove the container
	}
	return err
}
