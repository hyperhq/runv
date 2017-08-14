// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/urfave/cli"
)

// cState represents the platform agnostic pieces relating to a running
// container's status and state.  Note: The fields in this structure adhere to
// the opencontainers/runtime-spec/specs-go requirement for json fields that must be returned
// in a state command.
type cState struct {
	// Version is the OCI version for the container
	Version string `json:"ociVersion"`
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundlePath"`
	// Rootfs is a path to a directory containing the container's root filesystem.
	Rootfs string `json:"rootfsPath"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
}

var stateCommand = cli.Command{
	Name:  "state",
	Usage: "output the state of a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container.`,
	Description: `The state command outputs current state information for the
instance of a container.`,
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) {
		id := context.Args().First()
		if id == "" {
			fatal(fmt.Errorf("You must specify one container id"))
		}
		cs, err := getContainer(context, id)
		if err != nil {
			fatal(err)
		}
		data, err := json.MarshalIndent(cs, "", "  ")
		if err != nil {
			fatal(err)
		}
		os.Stdout.Write(data)
	},
}

func getContainer(context *cli.Context, name string) (*cState, error) {
	root := context.GlobalString("root")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	stateFile := filepath.Join(absRoot, name, stateJSON)
	_, err = os.Stat(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("State file of %s not found", name)
		}
		return nil, fmt.Errorf("Stat file %s error: %s", stateFile, err.Error())
	}

	state, err := loadStateFile(absRoot, name)
	if err != nil {
		return nil, fmt.Errorf("Load state file %s failed: %s", stateFile, err.Error())
	}

	status := state.Status
	var stat system.Stat_t
	stat, err = system.Stat(state.Pid)
	if err != nil || stat.StartTime != state.ShimCreateTime || stat.State == system.Zombie || stat.State == system.Dead {
		status = "stopped"
	}

	if state.Status != status {
		saveStateFile(absRoot, name, state)
	}

	s := &cState{
		Version:        state.Version,
		ID:             state.ID,
		InitProcessPid: state.Pid,
		Status:         status,
		Bundle:         state.Bundle,
		Rootfs:         filepath.Join(state.Bundle, "rootfs"),
		Created:        time.Unix(state.ContainerCreateTime, 0),
	}
	return s, nil
}
