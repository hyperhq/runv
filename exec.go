package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runtime-spec/specs-go"
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
			Usage: "[reject on runv] specify the pty slave path for use with the container",
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
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		container := context.Args().First()

		if context.String("console") != "" {
			fmt.Printf("--console is unsupported on runv\n")
			os.Exit(-1)
		}
		if container == "" {
			fmt.Printf("Please specify container ID\n")
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Printf("runv should be run as root\n")
			os.Exit(-1)
		}

		// get bundle path from state
		path := filepath.Join(root, container, stateJson)
		f, err := os.Open(path)
		if err != nil {
			fmt.Printf("open JSON configuration file failed: %v\n", err)
			os.Exit(-1)
		}
		defer f.Close()
		var s *specs.State
		if err := json.NewDecoder(f).Decode(&s); err != nil {
			fmt.Printf("parse JSON configuration file failed: %v\n", err)
			os.Exit(-1)
		}
		bundle := s.BundlePath

		// get process
		config, err := getProcess(context, bundle)
		if err != nil {
			fmt.Printf("get process config failed %v\n", err)
			os.Exit(-1)
		}

		code := runProcess(root, container, config)
		os.Exit(code)
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

func runProcess(root, container string, config *specs.Process) int {
	pid := os.Getpid()
	process := fmt.Sprintf("p-%x", pid+0xabcdef) // uniq name

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
		Stdin:  fmt.Sprintf("/proc/%d/fd/0", pid),
		Stdout: fmt.Sprintf("/proc/%d/fd/1", pid),
		Stderr: fmt.Sprintf("/proc/%d/fd/2", pid),
	}
	c := getClient(filepath.Join(root, container, "namespace/namespaced.sock"))
	timestamp := uint64(time.Now().Unix())
	if _, err := c.AddProcess(netcontext.Background(), p); err != nil {
		fmt.Printf("error %v\n", err)
		return -1
	}
	if config.Terminal {
		s, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			fmt.Printf("error %v\n", err)
			return -1
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), s)
		monitorTtySize(c, container, process)
	}
	return waitForExit(c, timestamp, container, process)
}
