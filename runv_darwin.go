package main

import (
	"syscall"
)

func mount(src, dst string) error {
	//return syscall.Symlink(src, dst)
	return syscall.Link(src, dst)
}

func umount(path string) {
	syscall.Unlink(path)
}
