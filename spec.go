package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/codegangsta/cli"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var specCommand = cli.Command{
	Name:  "spec",
	Usage: "create a new specification file",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config-file, c",
			Value: "config.json",
			Usage: "path to spec config file for writing",
		},
		cli.StringFlag{
			Name:  "runtime-file, r",
			Value: "runtime.json",
			Usage: "path to runtime config file for writing",
		},
	},
	Action: func(context *cli.Context) {
		spec := specs.Spec{
			Version: specs.Version,
			Platform: specs.Platform{
				OS:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
			Root: specs.Root{
				Path:     "rootfs",
				Readonly: true,
			},
			Process: specs.Process{
				Terminal: true,
				User:     specs.User{},
				Args: []string{
					"sh",
				},
				Env: []string{
					"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
					"TERM=xterm",
				},
			},
			Hostname: "shell",
			Linux: specs.Linux{
				Resources: &specs.Resources{},
			},
		}

		checkNoFile := func(name string) error {
			_, err := os.Stat(name)
			if err == nil {
				return fmt.Errorf("File %s exists. Remove it first", name)
			}
			if !os.IsNotExist(err) {
				return err
			}
			return nil
		}

		cName := context.String("config-file")
		if err := checkNoFile(cName); err != nil {
			fmt.Printf("%s", err.Error())
			return
		}
		data, err := json.MarshalIndent(&spec, "", "\t")
		if err != nil {
			fmt.Printf("%s", err.Error())
			return
		}
		if err := ioutil.WriteFile(cName, data, 0666); err != nil {
			fmt.Printf("%s", err.Error())
			return
		}
	},
}
