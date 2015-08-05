package main

import (
	"syscall"
)

func mount(src, dst string) error {
	return syscall.Mount(src, dst, "", syscall.MS_BIND, "")
}

func umount(root string) {
	syscall.Unmount(root, syscall.MNT_DETACH)
}
