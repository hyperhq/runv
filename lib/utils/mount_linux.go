package utils

import (
	"os"
	"syscall"
)

func Mount(src, dst string) error {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		os.MkdirAll(dst, 0755)
	}

	return syscall.Mount(src, dst, "", syscall.MS_BIND, "")
}

func Umount(root string) {
	syscall.Unmount(root, syscall.MNT_DETACH)
}
