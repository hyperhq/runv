package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
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
			Usage: "[TODO] current working directory in the container",
		},
		cli.StringSliceFlag{
			Name:  "env, e",
			Usage: "[TODO] set environment variables",
		},
		cli.BoolFlag{
			Name:  "tty, t",
			Usage: "[TODO] allocate a pseudo-TTY",
		},
		cli.StringFlag{
			Name:  "user, u",
			Usage: "[TODO] UID (format: <uid>[:<gid>])",
		},
		cli.StringFlag{
			Name:  "process, p",
			Usage: "path to the process.json",
		},
		cli.BoolFlag{
			Name:  "detach,d",
			Usage: "[TODO] detach from the container's process",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "[TODO] specify the file to write the process id to",
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
	Action: func(context *cli.Context) error {
		root := context.GlobalString("root")
		container := context.Args().First()

		if container == "" {
			return cli.NewExitError("Please specify container ID", -1)
		}
		if os.Geteuid() != 0 {
			return cli.NewExitError("runv should be run as root", -1)
		}

		// get bundle path from state
		path := filepath.Join(root, container, stateJson)
		f, err := os.Open(path)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("open JSON configuration file failed: %v", err), -1)
		}
		defer f.Close()
		var s *specs.State
		if err := json.NewDecoder(f).Decode(&s); err != nil {
			return cli.NewExitError(fmt.Sprintf("parse JSON configuration file failed: %v", err), -1)
		}
		bundle := s.Bundle

		// get process
		config, err := getProcess(context, bundle)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("get process config failed %v", err), -1)
		}
		if err := checkConsole(context, config, false); err != nil {
			return cli.NewExitError(err.Error(), -1)
		}

		code, err := runProcess(context, container, config)
		if code != 0 {
			return cli.NewExitError(err, code)
		} else if err != nil {
			return cli.NewExitError(err, -1)
		}
		return nil
	},
}

// loadProcessConfig loads the process configuration from the provided path.
func loadProcessConfig(path string) (*specs.Process, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON configuration file for %s not found", path)
		}
		return nil, err
	}
	defer f.Close()
	var s *specs.Process
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}

func getProcess(context *cli.Context, bundle string) (*specs.Process, error) {
	if path := context.String("process"); path != "" {
		return loadProcessConfig(path)
	}
	// process via cli flags
	spec, err := loadSpec(filepath.Join(bundle, specConfig))
	if err != nil {
		return nil, err
	}
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
	// set the tty
	if context.IsSet("tty") {
		p.Terminal = context.Bool("tty")
	}
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

func runProcess(context *cli.Context, container string, config *specs.Process) (int, error) {
	pid := os.Getpid()
	process := fmt.Sprintf("p-%x", pid+0xabcdef) // uniq name
	c, err := getClient(filepath.Join(context.GlobalString("root"), container, "namespace/namespaced.sock"))
	if err != nil {
		return -1, fmt.Errorf("failed to get client: %v", err)
	}
	evChan := containerEvents(c, container)

	if !context.Bool("detach") && config.Terminal {
		s, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return -1, fmt.Errorf("failed to set raw terminal: %v", err)
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), s)
		monitorTtySize(c, container, process)
	}

	err = ociCreate(context, container, process, func(stdin, stdout, stderr string) (int, error) {
		p := &types.AddProcessRequest{
			Id:       container,
			Pid:      process,
			Args:     config.Args,
			Cwd:      config.Cwd,
			Terminal: config.Terminal,
			Env:      config.Env,
			User: &types.User{
				Uid: config.User.UID,
				Gid: config.User.GID,
			},
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		}
		if _, err := c.AddProcess(netcontext.Background(), p); err != nil {
			return -1, err
		}
		return -1, nil
	})
	if err != nil {
		return -1, err
	}

	if !context.Bool("detach") {
		for e := range evChan {
			if e.Type == "exit" && e.Pid == process {
				return int(e.Status), nil
			}
		}
		return -1, fmt.Errorf("unknown error")
	}
	return 0, nil
}
