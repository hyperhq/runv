package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli"
)

const (
	defaultKernelInstallDir string = "/var/lib/hyper"
)

func firstExistingFile(candidates []string) string {
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			return file
		}
	}
	return ""
}

func getDefaultBundlePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// getKernelFiles chooses kernel/initrd/bios/cbfs files based on user specified ones
func getKernelFiles(context *cli.Context, rootPath string) (string, string, string, string, error) {
	kernel := context.GlobalString("kernel")
	initrd := context.GlobalString("initrd")
	bios := context.GlobalString("bios")
	cbfs := context.GlobalString("cbfs")
	bundle := context.String("bundle")

	for k, v := range map[*string][]string{
		&kernel: {
			filepath.Join(bundle, rootPath, "boot/vmlinuz"),
			filepath.Join(bundle, "boot/vmlinuz"),
			filepath.Join(bundle, "vmlinuz"),
			filepath.Join(defaultKernelInstallDir, "kernel"),
		},
		&initrd: {
			filepath.Join(bundle, rootPath, "boot/initrd.img"),
			filepath.Join(bundle, "boot/initrd.img"),
			filepath.Join(bundle, "initrd.img"),
			filepath.Join(defaultKernelInstallDir, "hyper-initrd.img"),
		},
		&bios: {
			filepath.Join(bundle, rootPath, "boot/bios.bin"),
			filepath.Join(bundle, "boot/bios.bin"),
			filepath.Join(bundle, "bios.bin"),
			filepath.Join(defaultKernelInstallDir, "bios.bin"),
		},
		&cbfs: {
			filepath.Join(bundle, rootPath, "boot/cbfs.rom"),
			filepath.Join(bundle, "boot/cbfs.rom"),
			filepath.Join(bundle, "cbfs.rom"),
			filepath.Join(defaultKernelInstallDir, "cbfs.rom"),
		},
	} {
		if *k == "" {
			*k = firstExistingFile(v)
		}
		if *k != "" {
			var err error
			*k, err = filepath.Abs(*k)
			if err != nil {
				return "", "", "", "", fmt.Errorf("cannot get abs path for kernel files: %v", err)
			}
		}
	}

	return kernel, initrd, bios, cbfs, nil
}
