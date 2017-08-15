package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var createCommand = cli.Command{
	Name:  "create",
	Usage: "create a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are creating. The name you provide for the container instance must be unique on
your host.`,
	Description: `The create command creates an instance of a container for a bundle. The bundle
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
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, true)
	},
	Action: func(context *cli.Context) error {
		if err := cmdCreateContainer(context, false); err != nil {
			return cli.NewExitError(fmt.Sprintf("Run Container error: %v", err), -1)
		}
		return nil
	},
}

func cmdCreateContainer(context *cli.Context, attach bool) error {
	root := context.GlobalString("root")
	bundle := context.String("bundle")
	container := context.Args().First()
	ocffile := filepath.Join(bundle, specConfig)
	spec, err := loadSpec(ocffile)
	if err != nil {
		return fmt.Errorf("load config failed: %v", err)
	}
	if spec.Linux == nil {
		return fmt.Errorf("it is not linux container config")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("runv should be run as root")
	}
	if container == "" {
		return fmt.Errorf("no container id provided")
	}
	_, err = os.Stat(filepath.Join(root, container))
	if err == nil {
		return fmt.Errorf("container %q exists", container)
	}
	if err = checkConsole(context, &spec.Process, attach); err != nil {
		return err
	}

	var sharedContainer string
	if containerType, ok := spec.Annotations["ocid/container_type"]; ok {
		if containerType == "container" {
			sharedContainer = spec.Annotations["ocid/sandbox_name"]
		}
	} else {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				if strings.Contains(ns.Path, "/") {
					return fmt.Errorf("Runv doesn't support path to namespace file, it supports containers name as shared namespaces only")
				}
				if ns.Type == "mount" {
					return fmt.Errorf("Runv doesn't support containers with shared mount namespace, use `runv exec` instead")
				}
				sharedContainer = ns.Path
				_, err = os.Stat(filepath.Join(root, sharedContainer, stateJSON))
				if err != nil {
					return fmt.Errorf("The container %q is not existing or not ready", sharedContainer)
				}
				_, err = os.Stat(filepath.Join(root, sharedContainer, "namespace"))
				if err != nil {
					return fmt.Errorf("The container %q is not ready", sharedContainer)
				}
			}
		}
	}

	var scState *State
	var vm *hypervisor.Vm
	var lockFile *os.File
	if sharedContainer != "" {
		scState, err = loadStateFile(root, sharedContainer)
		if err != nil {
			return err
		}
		vm, lockFile, err = getSandbox(filepath.Join(context.GlobalString("root"), sharedContainer, "sandbox"))
		if err != nil {
			return err
		}
	} else {
		f, err := setupFactory(context, spec)
		if err != nil {
			return nil
		}
		vm, lockFile, err = createAndLockSandBox(f, spec, context.GlobalInt("default_cpus"), context.GlobalInt("default_memory"))
		if err != nil {
			return nil
		}
	}
	defer putSandbox(vm, lockFile)

	options := runvOptions{Context: context, withContainer: scState, attach: attach}
	_, err = createContainer(options, vm, container, bundle, root, spec)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	return nil
}

func checkConsole(context *cli.Context, p *specs.Process, attach bool) error {
	if context.String("console") != "" && context.String("console-socket") != "" {
		return fmt.Errorf("only one of --console & --console-socket can be specified")
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && attach {
		return fmt.Errorf("--console[-socket] should be used on detached mode")
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && !p.Terminal {
		return fmt.Errorf("--console[-socket] should be used on tty mode")
	}
	return nil
}
