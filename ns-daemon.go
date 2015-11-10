package main

import (
	"github.com/codegangsta/cli"
)

// TODO: logging for daemon

var daemonCommand = cli.Command{
	Name:  "daemon",
	Usage: "internal and hidden daemon command, run in daemon mode",
	Action: func(context *cli.Context) {
		startVContainer(context.GlobalString("root"), context.GlobalString("id"))
	},
	HideHelp: true,
}
