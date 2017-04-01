package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/lib/term"
)

var shimCommand = cli.Command{
	Name:  "shim",
	Usage: "internal command for proxy changes to the container/process",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "container",
		},
		cli.StringFlag{
			Name: "process",
		},
		cli.BoolFlag{
			Name: "proxy-exit-code",
		},
		cli.BoolFlag{
			Name: "proxy-signal",
		},
		cli.BoolFlag{
			Name: "proxy-winsize",
		},
	},
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		container := context.String("container")
		process := context.String("process")
		c := getClient(filepath.Join(root, container, "namespace/namespaced.sock"))
		exitcode := -1
		if context.Bool("proxy-exit-code") {
			defer func() { os.Exit(exitcode) }()
		}

		if context.Bool("proxy-winsize") {
			s, err := term.SetRawTerminal(os.Stdin.Fd())
			if err != nil {
				fmt.Printf("error %v\n", err)
				return
			}
			defer term.RestoreTerminal(os.Stdin.Fd(), s)
			monitorTtySize(c, container, process)
		}

		if context.Bool("proxy-signal") {
			// TODO
		}

		// wait until exit
		evChan := containerEvents(c, container)
		for e := range evChan {
			if e.Type == "exit" && e.Pid == process {
				exitcode = int(e.Status)
				break
			}
		}
	},
}
