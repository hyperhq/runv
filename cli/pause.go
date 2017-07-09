package main

import (
	"fmt"
	"path/filepath"

	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/lib/linuxsignal"
	"github.com/urfave/cli"
)

var pauseCommand = cli.Command{
	Name:      "pause",
	Usage:     "suspend all processes in the container",
	ArgsUsage: `<container-id>`,
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError(fmt.Sprintf("container id cannot be empty"), -1)
		}

		h, err := libhyperstart.NewGrpcBasedHyperstart(filepath.Join(context.GlobalString("root"), container, "sandbox", "hyperstartgrpc.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to get hyperstart: %v", err), -1)
		}

		plist, err := getProcessList(context, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't get process list, %v", err), -1)
		}

		for _, p := range plist {
			if err := h.SignalProcess(container, p, linuxsignal.SIGSTOP); err != nil {
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
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError(fmt.Sprintf("container id cannot be empty"), -1)
		}

		h, err := libhyperstart.NewGrpcBasedHyperstart(filepath.Join(context.GlobalString("root"), container, "sandbox", "hyperstartgrpc.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to get client: %v", err), -1)
		}

		plist, err := getProcessList(context, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't get process list, %v", err), -1)
		}

		for _, p := range plist {
			if err := h.SignalProcess(container, p, linuxsignal.SIGCONT); err != nil {
				return cli.NewExitError(fmt.Sprintf("resume signal failed, %v", err), -1)
			}
		}

		return nil
	},
}
