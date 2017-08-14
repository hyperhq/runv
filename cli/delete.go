package main

import (
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name:  "delete",
	Usage: "delete any resources held by the container often used with detached container",
	ArgsUsage: `<container-id>

Where "<container-id>" is the name for the instance of the container.

EXAMPLE:
For example, if the container id is "ubuntu01" and runv list currently shows the
status of "ubuntu01" as "stopped" the following will delete resources held for
"ubuntu01" removing "ubuntu01" from the runv list of containers:

       # runv delete ubuntu01`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "Forcibly deletes the container if it is still running (uses SIGKILL)",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, true)
	},
	Action: func(context *cli.Context) error {
		force := context.Bool("force")
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
			// return success if the container does not exist when force
			// and also try to remove the empty container state dir
			if force {
				if errRmdir := os.Remove(filepath.Join(root, container)); errRmdir == nil || os.IsNotExist(errRmdir) {
					return nil
				}
				_, errState := os.Stat(filepath.Join(root, container, stateJSON))
				if errState != nil && os.IsNotExist(errState) {
					return nil
				}
			}
			return cli.NewExitError(err, -1)
		}

		return cmdDeleteContainer(context, container, force, spec, state)
	},
}

func cmdDeleteContainer(context *cli.Context, container string, force bool, spec *specs.Spec, state *State) error {
	vm, lockFile, err := getSandbox(filepath.Join(context.GlobalString("root"), container, "sandbox"))
	if err != nil {
		return err
	}
	defer putSandbox(vm, lockFile)

	return deleteContainer(vm, context.GlobalString("root"), container, force, spec, state)
}
