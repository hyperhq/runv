package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli"
)

var psCommand = cli.Command{
	Name:      "ps",
	Usage:     "ps displays the processes running inside a container",
	ArgsUsage: `<container-id> [ps options]`, Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "format, f",
			Value: "table",
			Usage: `select one of: ` + formatOptions,
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError("container id cannot be empty", -1)
		}
		pids, err := getProcessList(context, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't access container, %v", err), -1)
		}

		switch context.String("format") {
		case "table":
			w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
			fmt.Fprint(w, "PROCESS\tCMD\n")
			for _, p := range pids {
				fmt.Fprintf(w, "%s\t%s\n",
					p,
					"todo process.Args")
			}
			if err := w.Flush(); err != nil {
				fatal(err)
			}
		case "json":
			pids := make([]string, 0)

			data, err := json.Marshal(pids)
			if err != nil {
				fatal(err)
			}
			os.Stdout.Write(data)
			return nil
		default:
			return cli.NewExitError(fmt.Sprintf("invalid format option"), -1)
		}

		return nil
	},
}
