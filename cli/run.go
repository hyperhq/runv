package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

var runCommand = cli.Command{
	Name:  "run",
	Usage: "run a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are running. The name you provide for the container instance must be unique on
your host.`,
	Description: `The run command creates and starts an instance of a container for a bundle. The bundle
is a directory with a specification file named "` + specConfig + `" and a root
filesystem.

The specification file includes an args parameter. The args parameter is used
to specify command(s) that get run when the container is started. To change the
command(s) that get executed on start, edit the args parameter of the spec. See
"runv spec --help" for more explanation.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Value: getDefaultBundlePath(),
			Usage: "path to the root of the bundle directory, defaults to the current directory",
		},
		cli.StringFlag{
			Name:  "console",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.StringFlag{
			Name:  "console-socket",
			Usage: "specify the unix socket for sending the pty master back",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "specify the file to write the process id to",
		},
		cli.BoolFlag{
			Name:  "no-pivot",
			Usage: "[ignore on runv] do not use pivot root to jail process inside rootfs.  This should be used whenever the rootfs is on top of a ramdisk",
		},
		cli.BoolFlag{
			Name:  "detach, d",
			Usage: "detach from the container's process",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, context.Bool("detach"))
	},
	Action: func(context *cli.Context) (retErr error) {
		if err := cmdCreateContainer(context, false); err != nil {
			return cli.NewExitError(fmt.Sprintf("Run Container error: %v", err), -1)
		}

		container := context.Args().First()
		state, spec, err := loadStateAndSpec(context.GlobalString("root"), container)
		if err != nil {
			// TODO: how to delete the container?
			return cli.NewExitError(fmt.Errorf("Failed to load the container after created, err: %#v", err), -1)
		}
		defer func() {
			if !context.Bool("detach") || retErr != nil {
				err := cmdDeleteContainer(context, container, true, spec, state)
				if retErr == nil {
					retErr = err
				}
			}
		}()

		err = cmdStartContainer(context, container, spec, state)
		if err != nil {
			return cli.NewExitError(err, -1)
		}

		if !context.Bool("detach") {
			shim, err := os.FindProcess(state.Pid)
			if err != nil {
				return err
			}
			ret, err := osProcessWait(shim)
			if ret == 0 {
				return nil
			}
			return cli.NewExitError(err, ret)
		}

		return nil
	},
}
