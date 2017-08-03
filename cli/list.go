package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli"
)

const formatOptions = `table or json`

// containerState represents the platform agnostic pieces relating to a
// running container's status and state
type containerState struct {
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
}

var listCommand = cli.Command{
	Name:  "list",
	Usage: "lists containers started by runv with the given root",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "display only container IDs",
		},
		cli.StringFlag{
			Name:  "format, f",
			Value: "",
			Usage: `select one of: ` + formatOptions + `.

The default format is table.  The following will output the list of containers
in json format:

  # runv list -f json`,
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) {
		s, err := getContainers(context)
		if err != nil {
			fatal(err)
		}

		if context.Bool("quiet") {
			for _, item := range s {
				fmt.Println(item.ID)
			}
			return
		}

		switch context.String("format") {
		case "", "table":
			w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
			fmt.Fprint(w, "ID\tPID\tSTATUS\tBUNDLE\tCREATED\n")
			for _, item := range s {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
					item.ID,
					item.InitProcessPid,
					item.Status,
					item.Bundle,
					item.Created.Format(time.RFC3339Nano))
			}
			if err := w.Flush(); err != nil {
				fatal(err)
			}
		case "json":
			data, err := json.Marshal(s)
			if err != nil {
				fatal(err)
			}
			os.Stdout.Write(data)

		default:
			fatal(fmt.Errorf("invalid format option"))
		}

	},
}

func getContainers(context *cli.Context) ([]containerState, error) {
	root := context.GlobalString("root")
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	list, err := ioutil.ReadDir(absRoot)
	if err != nil {
		return nil, err
	}

	var s []containerState
	for _, item := range list {
		if item.IsDir() {
			stateFile := filepath.Join(absRoot, item.Name(), stateJSON)
			fi, err := os.Stat(stateFile)
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("Stat file %s error: %s", stateFile, err.Error())
			}
			state, err := loadStateFile(absRoot, item.Name())
			if err != nil {
				return nil, fmt.Errorf("Load state file %s failed: %s", stateFile, err.Error())
			}

			// TODO: get the Status by inspecting the shim process

			s = append(s, containerState{
				ID:             state.ID,
				InitProcessPid: state.Pid,
				Status:         "running",
				Bundle:         state.Bundle,
				Created:        fi.ModTime(),
			})
		}
	}
	return s, nil
}

// fatal prints the error's details if it is a runv specific error type
// then exits the program with an exit status of 1.
func fatal(err error) {
	// make sure the error is written to the logger
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
