package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/opencontainers/specs"
)

var execCommand = cli.Command{
	Name:  "exec",
	Usage: "exec a new program in runv container",
	Action: func(context *cli.Context) {
		config, err := loadProcessConfig(context.Args().First())
		if err != nil {
			fmt.Errorf("load process config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Errorf("runv should be run as root\n")
			os.Exit(-1)
		}
		status, err := execProcess(context, config)
		if err != nil {
			fmt.Errorf("exec failed: %v", err)
		}
		os.Exit(status)
	},
}

func execProcess(context *cli.Context, config *specs.Process) (int, error) {
	container := context.GlobalString("id")
	root := context.GlobalString("root")
	stateDir := path.Join(root, container)

	if container == "" {
		return -1, fmt.Errorf("Please specify container ID")
	}

	conn, err := net.Dial("unix", path.Join(stateDir, "runv.sock"))
	if err != nil {
		return -1, err
	}

	cmd, err := json.Marshal(config.Args)
	if err != nil {
		return -1, err
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_EXECCMD,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])

	return containerTtySplice(stateDir, conn)
}

// loadProcessConfig loads the process configuration from the provided path.
func loadProcessConfig(path string) (*specs.Process, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON configuration file for %s not found", path)
		}
		return nil, err
	}
	defer f.Close()
	var s *specs.Process
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}
