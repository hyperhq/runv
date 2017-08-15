package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var execCommand = cli.Command{
	Name:  "exec",
	Usage: "exec a new program in runv container",
	ArgsUsage: `<container-id> <container command>

Where "<container-id>" is the name for the instance of the container and
"<container command>" is the command to be executed in the container.

For example, if the container is configured to run the linux ps command the
following will output a list of processes running in the container:

       # runv exec <container-id> ps`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "console",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.StringFlag{
			Name:  "console-socket",
			Usage: "specify the unix socket for sending the pty master back",
		},
		cli.StringFlag{
			Name:  "cwd",
			Usage: "current working directory in the container",
		},
		cli.StringSliceFlag{
			Name:  "env, e",
			Usage: "set environment variables",
		},
		cli.BoolFlag{
			Name:  "tty, t",
			Usage: "allocate a pseudo-TTY",
		},
		cli.StringFlag{
			Name:  "user, u",
			Usage: "UID (format: <uid>[:<gid>])",
		},
		cli.StringFlag{
			Name:  "process, p",
			Usage: "path to the process.json",
		},
		cli.BoolFlag{
			Name:  "detach,d",
			Usage: "detach from the container's process",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "specify the file to write the process id to",
		},
		cli.StringFlag{
			Name:  "process-label",
			Usage: "[ignore on runv] set the asm process label for the process commonly used with selinux",
		},
		cli.StringFlag{
			Name:  "apparmor",
			Usage: "[ignore on runv] set the apparmor profile for the process",
		},
		cli.BoolFlag{
			Name:  "no-new-privs",
			Usage: "[ignore on runv] set the no new privileges value for the process",
		},
		cli.StringSliceFlag{
			Name:  "cap, c",
			Usage: "[ignore on runv] add a capability to the bounding set for the process",
		},
		cli.BoolFlag{
			Name:  "no-subreaper",
			Usage: "[ignore on runv] disable the use of the subreaper used to reap reparented processes",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, true, context.Bool("detach"))
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

		cState, cSpec, err := loadStateAndSpec(root, container)
		if err != nil {
			return cli.NewExitError(err, -1)
		}

		// get process
		config, err := getProcess(context, cSpec)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("get process config failed %v", err), -1)
		}
		if err := checkConsole(context, config, false); err != nil {
			return cli.NewExitError(err.Error(), -1)
		}

		code, err := runProcess(context, container, cState, config)
		if code != 0 {
			return cli.NewExitError(err, code)
		} else if err != nil {
			return cli.NewExitError(err, -1)
		}
		return nil
	},
	SkipArgReorder: true,
}

func getProcess(context *cli.Context, spec *specs.Spec) (*specs.Process, error) {
	if path := context.String("process"); path != "" {
		return loadProcessConfig(path)
	}
	// process via cli flags
	p := spec.Process
	p.Args = context.Args()[1:]
	if p.Cwd == "" {
		return nil, fmt.Errorf("Cwd property must not be empty")
	}
	// override the cwd, if passed
	if context.String("cwd") != "" {
		p.Cwd = context.String("cwd")
	}
	// append the passed env variables
	for _, e := range context.StringSlice("env") {
		p.Env = append(p.Env, e)
	}
	// set the tty, always override it
	p.Terminal = context.Bool("tty")
	// override the user, if passed
	if context.String("user") != "" {
		u := strings.SplitN(context.String("user"), ":", 2)
		if len(u) > 1 {
			gid, err := strconv.Atoi(u[1])
			if err != nil {
				return nil, fmt.Errorf("parsing %s as int for gid failed: %v", u[1], err)
			}
			p.User.GID = uint32(gid)
		}
		uid, err := strconv.Atoi(u[0])
		if err != nil {
			return nil, fmt.Errorf("parsing %s as int for uid failed: %v", u[0], err)
		}
		p.User.UID = uint32(uid)
	}
	return &p, nil
}

func runProcess(context *cli.Context, container string, cState *State, config *specs.Process) (int, error) {
	pid := os.Getpid()
	process := fmt.Sprintf("p-%x", pid+0xabcdef) // uniq name

	options := runvOptions{Context: context, withContainer: cState, attach: !context.Bool("detach")}

	vm, lockFile, err := getSandbox(filepath.Join(context.GlobalString("root"), container, "sandbox"))
	if err != nil {
		return -1, err
	}

	shim, err := addProcess(options, vm, container, process, config)
	putSandbox(vm, lockFile)
	if err != nil {
		return -1, err
	}

	if !context.Bool("detach") {
		return osProcessWait(shim)
	}
	return 0, nil
}
