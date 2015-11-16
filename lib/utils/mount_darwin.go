package utils

import (
	"syscall"
)

func Mount(src, dst string) error {
	//return syscall.Symlink(src, dst)
	return syscall.Link(src, dst)
}

func Umount(path string) {
	syscall.Unlink(path)
}
