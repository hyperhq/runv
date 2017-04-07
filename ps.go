package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
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
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError("container id cannot be empty", -1)
		}
		c, err := getContainerApi(context, container)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("can't access container, %v", err), -1)
		}

		switch context.String("format") {
		case "table":
			w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
			fmt.Fprint(w, "PROCESS\tCMD\n")
			// we are limited by the containerd interface for now
			for _, p := range c.Processes {
				fmt.Fprintf(w, "%s\t%s\n",
					p.Pid,
					p.Args)
			}
			if err := w.Flush(); err != nil {
				fatal(err)
			}
		case "json":
			pids := make([]string, 0)
			for _, p := range c.Processes {
				pids = append(pids, p.Pid)
			}

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

func getContainerApi(context *cli.Context, container string) (*types.Container, error) {
	api, err := getClient(filepath.Join(context.GlobalString("root"), container, "namespace/namespaced.sock"))
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %v", err)
	}

	s, err := api.State(netcontext.Background(), &types.StateRequest{Id: container})
	if err != nil {
		return nil, fmt.Errorf("get container state failed, %v", err)
	}

	for _, c := range s.Containers {
		if c.Id == container {
			return c, nil
		}
	}

	return nil, fmt.Errorf("container %s not found", container)
}
