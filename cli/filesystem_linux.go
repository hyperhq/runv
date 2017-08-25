// +build linux

package main

import (
	"os"
	"syscall"

	"github.com/hyperhq/runv/lib/utils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func platformSetupFs(root string, spec *specs.Spec) (err error) {
	if spec.Linux == nil {
		return nil
	}

	for _, path := range spec.Linux.MaskedPaths {
		if err = maskPath(root + "/" + path); err != nil {
			return err
		}
	}

	for _, path := range spec.Linux.ReadonlyPaths {
		if err = readonlyPath(root + "/" + path); err != nil {
			return err
		}
	}

	return nil
}

// For files, bind mount them to /dev/null
// For dirs, mount readonly tmpfs over them
// TODO: move this to hyperstart
func maskPath(path string) error {
	if err := syscall.Mount("/dev/null", path, "", syscall.MS_BIND, ""); err != nil && !os.IsNotExist(err) {
		if err == syscall.ENOTDIR {
			return syscall.Mount("tmpfs", path, "tmpfs", syscall.MS_RDONLY, "")
		}
		return err
	}
	return nil
}

func readonlyPath(path string) error {
	return utils.SetReadonly(path)
}
