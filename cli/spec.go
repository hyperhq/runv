package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/golang/glog"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var specCommand = cli.Command{
	Name:  "spec",
	Usage: "create a new specification file",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Usage: "path to the root of the bundle directory",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
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
				Cwd: "/",
			},
			Hostname: "shell",
			Linux: &specs.Linux{
				Resources: &specs.LinuxResources{},
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

		bundle := context.String("bundle")
		if bundle != "" {
			if err := os.Chdir(bundle); err != nil {
				fmt.Printf("Failed to chdir to bundle dir:%s\nerror:%v\n", bundle, err)
				return
			}
		}
		if err := checkNoFile(specConfig); err != nil {
			fmt.Printf("%s\n", err.Error())
			return
		}
		data, err := json.MarshalIndent(&spec, "", "\t")
		if err != nil {
			fmt.Printf("%s\n", err.Error())
			return
		}
		if err := ioutil.WriteFile(specConfig, data, 0666); err != nil {
			fmt.Printf("%s\n", err.Error())
			return
		}
	},
}

func loadSpec(ocffile string) (*specs.Spec, error) {
	var spec specs.Spec

	if _, err := os.Stat(ocffile); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%q doesn't exists", ocffile)
		}
		return nil, fmt.Errorf("Stat %q error: %v", ocffile, err)
	}

	ocfData, err := ioutil.ReadFile(ocffile)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(ocfData, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
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

func saveStateFile(root, container string, state *specs.State) error {
	stateFile := filepath.Join(root, container, stateJSON)
	stateData, err := json.MarshalIndent(state, "", "\t")
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	err = ioutil.WriteFile(stateFile, stateData, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	return nil
}

func loadStateFile(root, container string) (*specs.State, error) {
	stateFile := filepath.Join(root, container, stateJSON)
	file, err := os.Open(stateFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var state specs.State
	if err = json.NewDecoder(file).Decode(&state); err != nil {
		return nil, fmt.Errorf("Decode state file %s error: %s", stateFile, err.Error())
	}
	return &state, nil
}

func loadStateAndSpec(root, container string) (*specs.State, *specs.Spec, error) {
	state, err := loadStateFile(root, container)
	if err != nil {
		return nil, nil, err
	}
	spec, err := loadSpec(filepath.Join(state.Bundle, specConfig))
	if err != nil {
		return nil, nil, err
	}
	return state, spec, nil
}
