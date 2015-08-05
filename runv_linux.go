package main

import (
	"os"
	"syscall"
)

func mount(src, dst string) error {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		os.MkdirAll(dst, 0755)
        }

	return syscall.Mount(src, dst, "", syscall.MS_BIND, "")
}

func umount(root string) {
	syscall.Unmount(root, syscall.MNT_DETACH)
}
