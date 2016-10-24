package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/linuxsignal"
	netcontext "golang.org/x/net/context"
)

var deleteCommand = cli.Command{
	Name:  "delete",
	Usage: "delete any resources held by one container or more containers often used with detached containers",
	ArgsUsage: `container-id [container-id...]

Where "<container-id>" is the name for the instance of the container.

EXAMPLE:
For example, if the container id is "ubuntu01" the following will delete resources
held for "ubuntu01" removing "ubuntu01" from the runv list of containers:

       # runv delete ubuntu01`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "[ignore on runv temporarily] forcibly kills the container if it is still running",
		},
	},
	Action: func(context *cli.Context) {
		hasError := false
		if !context.Args().Present() {
			fmt.Printf("runv: \"delete\" requires a minimum of 1 argument")
			os.Exit(-1)
		}

		for _, container := range context.Args() {
			c := getClient(filepath.Join(context.GlobalString("root"), container, "namespace/namespaced.sock"))
			if _, err := c.Signal(netcontext.Background(), &types.SignalRequest{
				Id:     container,
				Pid:    "init",
				Signal: uint32(linuxsignal.SIGKILL),
			}); err != nil {
				fmt.Fprintf(os.Stderr, "delete container %s failed, %v", container, err)
				hasError = true
			}
		}

		if hasError {
			fmt.Errorf("one or more of the container deletions failed")
			os.Exit(-1)
		}
	},
}
