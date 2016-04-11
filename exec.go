package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var execCommand = cli.Command{
	Name:  "exec",
	Usage: "exec a new program in runv container",
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		container := context.GlobalString("id")
		config, err := loadProcessConfig(context.Args().First())
		if err != nil {
			fmt.Printf("load process config failed %v\n", err)
			os.Exit(-1)
		}
		if container == "" {
			fmt.Printf("Please specify container ID")
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Printf("runv should be run as root\n")
			os.Exit(-1)
		}
		conn, err := runvRequest(root, container, RUNV_EXECCMD, config.Args)
		if err != nil {
			fmt.Printf("exec failed: %v", err)
		}
		code, err := containerTtySplice(root, container, conn, false)
		os.Exit(code)
	},
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
