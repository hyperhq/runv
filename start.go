package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
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

		// get spec from bundle
		ocffile := filepath.Join(bundle, specConfig)
		spec, err := loadSpec(ocffile)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("load config failed: %v", err), -1)
		}

		address := filepath.Join(root, container, "namespace/namespaced.sock")
		status, err := startContainer(context, bundle, container, address, spec, true)
		// TODO, kill the containerd if it is the first container
		if status != 0 {
			return cli.NewExitError(err, status)
		} else if err != nil {
			return cli.NewExitError(err, -1)
		}
		return nil
	},
}

func startContainer(context *cli.Context, bundle, container, address string, config *specs.Spec, detach bool) (int, error) {
	r := &types.CreateContainerRequest{
		Id:         container,
		Runtime:    "runv-start",
		BundlePath: bundle,
	}

	c, err := getClient(address)
	if err != nil {
		return -1, fmt.Errorf("failed to get client: %v", err)
	}
	evChan := containerEvents(c, container)
	if _, err := c.CreateContainer(netcontext.Background(), r); err != nil {
		return -1, fmt.Errorf("failed to create container %v", err)
	}
	if !detach && config.Process.Terminal {
		s, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			return -1, fmt.Errorf("failed to set raw terminal %v", err)
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), s)
		monitorTtySize(c, container, "init")
	}
	var started bool
	for e := range evChan {
		if e.Type == "exit" && e.Pid == "init" {
			return int(e.Status), fmt.Errorf("get exit event before start event")
		}
		if e.Type == "start-container" {
			started = true
			break
		}
	}
	if !started {
		return -1, fmt.Errorf("failed to get the start event")
	}
	if !detach {
		for e := range evChan {
			if e.Type == "exit" && e.Pid == "init" {
				return int(e.Status), nil
			}
		}
		return -1, fmt.Errorf("unknown error")
	}
	return 0, nil
}
