package main

import (
	"fmt"
	"runtime"

	"github.com/containernetworking/plugins/pkg/ns"
)

func nsSetRun(nsPid int, cb func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	curr, err := ns.GetCurrentNS()
	if err != nil {
		return err
	}
	defer curr.Close()

	target, err := ns.GetNS(fmt.Sprintf("/proc/%d/ns/net", nsPid))
	if err != nil {
		return err
	}
	if err = target.Set(); err != nil {
		return err
	}
	defer curr.Set()

	return cb()
}
