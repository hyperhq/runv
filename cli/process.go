// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Process struct {
	// process id
	Id string `json:"id"`
	// process shim pid
	Pid int `json:"pid"`
	// Process command line
	CMD string `json:"cmd"`
	// process shim created time
	CreateTime uint64 `json:"create_time"`
}

type ProcessList struct {
	file *os.File
}

// Return locked processlist, which needs to be released by caller after using
func NewProcessList(root, name string) (*ProcessList, error) {
	f, err := os.OpenFile(filepath.Join(root, name, processJSON), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		f.Close()
		return nil, fmt.Errorf("Placing LOCK_SH lock on process json file failed: %s", err.Error())
	}

	return &ProcessList{file: f}, nil
}

func (pl *ProcessList) Release() {
	syscall.Flock(int(pl.file.Fd()), syscall.LOCK_UN)
	pl.file.Close()
	pl.file = nil
}

func (pl *ProcessList) Load() ([]Process, error) {
	_, err := pl.file.Seek(0, os.SEEK_SET)
	if err != nil {
		return nil, fmt.Errorf("Seek process json file failed: %s", err.Error())
	}

	var p []Process
	if err = json.NewDecoder(pl.file).Decode(&p); err != nil {
		return nil, fmt.Errorf("Decode process json file failed: %s", err.Error())
	}

	return p, nil
}

func (pl *ProcessList) Save(p []Process) error {
	// upgrade SH lock to EX lock
	err := syscall.Flock(int(pl.file.Fd()), syscall.LOCK_EX)
	if err != nil {
		return fmt.Errorf("Placing LOCK_EX on process json file failed: %s", err.Error())
	}

	if err = pl.file.Truncate(0); err != nil {
		return fmt.Errorf("Failed to truncate process json file: %s", err.Error())
	}

	data, err := json.MarshalIndent(p, "", "\t")
	if err != nil {
		return err
	}
	if _, err = pl.file.WriteAt(data, 0); err != nil {
		return fmt.Errorf("Saving process json file failed: %s", err.Error())
	}

	return nil
}

func (pl *ProcessList) Add(p Process) error {
	list, err := pl.Load()
	if err != nil {
		return err
	}

	list = append(list, p)
	return pl.Save(list)
}
