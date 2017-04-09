package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runtime-spec/specs-go"
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
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		container := context.Args().First()

		if container == "" {
			fmt.Fprintf(os.Stderr, "Please specify container ID\n")
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Fprintf(os.Stderr, "runv should be run as root\n")
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
		bundle := s.Bundle

		// get spec from bundle
		ocffile := filepath.Join(bundle, specConfig)
		spec, err := loadSpec(ocffile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
			os.Exit(-1)
		}

		address := filepath.Join(root, container, "namespace/namespaced.sock")
		status := startContainer(context, bundle, container, address, spec, true)
		// TODO, kill the containerd if it is the first container
		os.Exit(status)
	},
}

func startContainer(context *cli.Context, bundle, container, address string, config *specs.Spec, detach bool) int {
	var err error

	defer os.RemoveAll(getStdioDir(context, container))

	r := &types.CreateContainerRequest{
		Id:         container,
		Runtime:    "runv-start",
		BundlePath: bundle,
	}

	c := getClient(address)
	evChan := containerEvents(c, container)
	if _, err = c.CreateContainer(netcontext.Background(), r); err != nil {
		fmt.Printf("error %v\n", err)
		return -1
	}
	var (
		stdin io.WriteCloser
		state *term.State
	)
	restoreAndCloseStdin := func() {
		if state != nil {
			term.RestoreTerminal(os.Stdin.Fd(), state)
		}
		if stdin != nil {
			stdin.Close()
		}
	}
	defer restoreAndCloseStdin()

	if !detach {
		if config.Process.Terminal {
			state, err = term.SetRawTerminal(os.Stdin.Fd())
			if err != nil {
				fmt.Printf("error %v\n", err)
				return -1
			}
		}
		var s *stdio
		if s, err = getStdio(context, container, false); err != nil {
			fmt.Printf("error %v\n", err)
			return -1
		}
		if stdin, err = attachStdio(s); err != nil {
			fmt.Printf("error %v\n", err)
			return -1
		}
	}
	var started bool
	for e := range evChan {
		if e.Type == "exit" && e.Pid == "init" {
			fmt.Printf("get exit event before start event\n")
			return int(e.Status)
		}
		if e.Type == "start-container" {
			started = true
			break
		}
	}
	if !started {
		fmt.Printf("failed to get the start event\n")
		return -1
	}
	if detach {
		return 0
	}
	go func() {
		io.Copy(stdin, os.Stdin)
		if _, err := c.UpdateProcess(netcontext.Background(), &types.UpdateProcessRequest{
			Id:         container,
			Pid:        "init",
			CloseStdin: true,
		}); err != nil {
			fmt.Printf("error %v\n", err)
			return
		}
		restoreAndCloseStdin()
	}()
	if config.Process.Terminal {
		monitorTtySize(c, container, "init")
	}
	for e := range evChan {
		if e.Type == "exit" && e.Pid == "init" {
			return int(e.Status)
		}
	}
	return -1
}
