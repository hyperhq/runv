package main

import (
	"fmt"
	"path/filepath"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/linuxsignal"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
)

var pauseCommand = cli.Command{
	Name:      "pause",
	Usage:     "suspend all processes in the container",
	ArgsUsage: `<container-id>`,
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError(fmt.Sprintf("container id cannot be empty"), -1)
		}

		c, err := getClient(filepath.Join(context.GlobalString("root"), container, "namespace/namespaced.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to get client: %v", err), -1)
		}

		plist, err := getProcessList(c, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't get process list, %v", err), -1)
		}

		for _, p := range plist {
			if _, err := c.Signal(netcontext.Background(), &types.SignalRequest{
				Id:     container,
				Pid:    p,
				Signal: uint32(linuxsignal.SIGSTOP),
			}); err != nil {
				return cli.NewExitError(fmt.Sprintf("suspend signal failed, %v", err), -1)
			}
		}

		return nil
	},
}

var resumeCommand = cli.Command{
	Name:      "resume",
	Usage:     "resume all processes in the container",
	ArgsUsage: `<container-id>`,
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError(fmt.Sprintf("container id cannot be empty"), -1)
		}

		c, err := getClient(filepath.Join(context.GlobalString("root"), container, "namespace/namespaced.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to get client: %v", err), -1)
		}

		plist, err := getProcessList(c, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't get process list, %v", err), -1)
		}

		for _, p := range plist {
			if _, err := c.Signal(netcontext.Background(), &types.SignalRequest{
				Id:     container,
				Pid:    p,
				Signal: uint32(linuxsignal.SIGCONT),
			}); err != nil {
				return cli.NewExitError(fmt.Sprintf("resume signal failed, %v", err), -1)
			}
		}

		return nil
	},
}
