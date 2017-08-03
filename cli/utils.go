package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	specs "github.com/opencontainers/runtime-spec/specs-go"
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

// find it from spec.Process.Env, runv's env(todo) and context.GlobalString
func chooseKernelFromConfigs(context *cli.Context, spec *specs.Spec) string {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.kernel" {
			// remove kernel env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			return slices[1]
		}
	}
	return context.GlobalString("kernel")
}

func chooseInitrdFromConfigs(context *cli.Context, spec *specs.Spec) string {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.initrd" {
			// remove initrd env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			return slices[1]
		}
	}
	return context.GlobalString("initrd")
}

func chooseBiosFromConfigs(context *cli.Context, spec *specs.Spec) string {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.bios" {
			// remove bios env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			return slices[1]
		}
	}
	return context.GlobalString("bios")
}

func chooseCbfsFromConfigs(context *cli.Context, spec *specs.Spec) string {
	for k, env := range spec.Process.Env {
		slices := strings.Split(env, "=")
		if len(slices) == 2 && slices[0] == "hypervisor.cbfs" {
			// remove cbfs env because this is only allow to be used by runv
			spec.Process.Env = append(spec.Process.Env[:k], spec.Process.Env[k+1:]...)
			return slices[1]
		}
	}
	return context.GlobalString("cbfs")
}

// getKernelFiles chooses kernel/initrd/bios/cbfs files based on user specified ones
func getKernelFiles(context *cli.Context, spec *specs.Spec) (string, string, string, string, error) {
	kernel := chooseKernelFromConfigs(context, spec)
	initrd := chooseInitrdFromConfigs(context, spec)
	bios := chooseBiosFromConfigs(context, spec)
	cbfs := chooseCbfsFromConfigs(context, spec)
	bundle := context.String("bundle")

	if kernel != "" && initrd != "" {
		return kernel, initrd, bios, cbfs, nil
	}

	// choose from filesystem
	for k, v := range map[*string][]string{
		&kernel: {
			filepath.Join(bundle, spec.Root.Path, "boot/vmlinuz"),
			filepath.Join(bundle, "boot/vmlinuz"),
			filepath.Join(bundle, "vmlinuz"),
			filepath.Join(defaultKernelInstallDir, "kernel"),
		},
		&initrd: {
			filepath.Join(bundle, spec.Root.Path, "boot/initrd.img"),
			filepath.Join(bundle, "boot/initrd.img"),
			filepath.Join(bundle, "initrd.img"),
			filepath.Join(defaultKernelInstallDir, "hyper-initrd.img"),
		},
		&bios: {
			filepath.Join(bundle, spec.Root.Path, "boot/bios.bin"),
			filepath.Join(bundle, "boot/bios.bin"),
			filepath.Join(bundle, "bios.bin"),
			filepath.Join(defaultKernelInstallDir, "bios.bin"),
		},
		&cbfs: {
			filepath.Join(bundle, spec.Root.Path, "boot/cbfs.rom"),
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

func osProcessWait(process *os.Process) (int, error) {
	state, err := process.Wait()
	if err != nil {
		return -1, err
	}
	if state.Success() {
		return 0, nil
	}

	if status, ok := state.Sys().(syscall.WaitStatus); ok {
		return status.ExitStatus(), err
	}

	return -1, err
}
