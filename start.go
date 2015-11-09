package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/codegangsta/cli"
	"github.com/opencontainers/specs"
)

type startConfig struct {
	Name       string
	BundlePath string
	Root       string
	Driver     string
	Kernel     string
	Initrd     string
	Vbox       string
	specs.LinuxSpec
	specs.LinuxRuntimeSpec
}

func loadStartConfig(context *cli.Context) (*startConfig, error) {
	config := &startConfig{
		Name:   context.GlobalString("id"),
		Root:   context.GlobalString("root"),
		Driver: context.GlobalString("driver"),
		Kernel: context.GlobalString("kernel"),
		Initrd: context.GlobalString("initrd"),
		Vbox:   context.GlobalString("vbox"),
	}

	if config.Name == "" {
		return nil, fmt.Errorf("Please specify container ID")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	config.BundlePath = cwd

	ocffile := context.String("config-file")
	runtimefile := context.String("runtime-file")

	if _, err := os.Stat(ocffile); os.IsNotExist(err) {
		fmt.Printf("Please specify ocffile or put config.json under current working directory\n")
		return nil, err
	}

	ocfData, err := ioutil.ReadFile(ocffile)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return nil, err
	}

	var runtimeData []byte = nil
	_, err = os.Stat(runtimefile)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("Fail to stat %s, %s\n", runtimefile, err.Error())
			return nil, err
		}
	} else {
		runtimeData, err = ioutil.ReadFile(runtimefile)
		if err != nil {
			fmt.Printf("Fail to readfile %s, %s\n", runtimefile, err.Error())
			return nil, err
		}
	}

	if err := json.Unmarshal(ocfData, &config.LinuxSpec); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(runtimeData, &config.LinuxRuntimeSpec); err != nil {
		return nil, err
	}

	return config, nil
}

var startCommand = cli.Command{
	Name:  "start",
	Usage: "create and run a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config-file, c",
			Value: "config.json",
			Usage: "path to spec config file",
		},
		cli.StringFlag{
			Name:  "runtime-file, r",
			Value: "runtime.json",
			Usage: "path to runtime config file",
		},
	},
	Action: func(context *cli.Context) {
		config, err := loadStartConfig(context)
		if err != nil {
			fmt.Errorf("load config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Errorf("runv should be run as root\n")
			os.Exit(-1)
		}

		startVContainer(config)
	},
}
