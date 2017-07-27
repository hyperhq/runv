package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/supervisor"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
)

var deleteCommand = cli.Command{
	Name:      "delete",
	Usage:     "delete a stopped container",
	ArgsUsage: `<container-id>`,
	Action: func(context *cli.Context) error {
		container := context.Args().First()
		if container == "" {
			return cli.NewExitError("container id cannot be empty", -1)
		}

		containerPath := filepath.Join(context.GlobalString("root"), container)

		if dir, err := os.Stat(containerPath); err != nil || !dir.IsDir() {
			return fmt.Errorf("container %s does not exist", container)
		}

		api, err := getClient(filepath.Join(containerPath, "namespace/namespaced.sock"))
		if err != nil {
			return fmt.Errorf("failed to get client: %v", err)
		}

		_, err = api.GetServerVersion(netcontext.Background(), nil)
		if err != nil {
			// if we can't connect to the api, runv was killed before it could clean up the stopped containers
			err := os.RemoveAll(containerPath)
			if err != nil {
				return fmt.Errorf("delete stale container %s failed, %v", container, err)
			}
			return nil
		}

		_, err = api.UpdateContainer(netcontext.Background(), &types.UpdateContainerRequest{Id: container, Status: supervisor.ContainerStateDeleted})
		if err != nil {
			return fmt.Errorf("delete container %s failed, %v", container, err)
		}

		return nil
	},
}
