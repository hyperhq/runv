package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/utils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func mountToRootfs(m *specs.Mount, rootfs, mountLabel string) error {
	// TODO: we don't use mountLabel here because it looks like mountLabel is
	// only significant when SELinux is enabled.
	var (
		dest = m.Destination
	)
	if !strings.HasPrefix(dest, rootfs) {
		dest = filepath.Join(rootfs, dest)
	}

	switch m.Type {
	case "proc", "sysfs", "mqueue", "tmpfs", "cgroup", "devpts":
		glog.V(3).Infof("Skip mount point %q of type %s", m.Destination, m.Type)
		return nil
	case "bind":
		stat, err := os.Stat(m.Source)
		if err != nil {
			// error out if the source of a bind mount does not exist as we will be
			// unable to bind anything to it.
			return err
		}
		// ensure that the destination of the bind mount is resolved of symlinks at mount time because
		// any previous mounts can invalidate the next mount's destination.
		// this can happen when a user specifies mounts within other mounts to cause breakouts or other
		// evil stuff to try to escape the container's rootfs.
		if dest, err = symlink.FollowSymlinkInScope(filepath.Join(rootfs, m.Destination), rootfs); err != nil {
			return err
		}
		if err := checkMountDestination(rootfs, dest); err != nil {
			return err
		}
		// update the mount with the correct dest after symlinks are resolved.
		m.Destination = dest
		if err := createIfNotExists(dest, stat.IsDir()); err != nil {
			return err
		}
		if err := mount.Mount(m.Source, dest, m.Type, strings.Join(m.Options, ",")); err != nil {
			return err
		}
	default:
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
		return mount.Mount(m.Source, dest, m.Type, strings.Join(m.Options, ","))
	}
	return nil
}

// checkMountDestination checks to ensure that the mount destination is not over the top of /proc.
// dest is required to be an abs path and have any symlinks resolved before calling this function.
func checkMountDestination(rootfs, dest string) error {
	invalidDestinations := []string{
		"/proc",
	}
	// White list, it should be sub directories of invalid destinations
	validDestinations := []string{
		// These entries can be bind mounted by files emulated by fuse,
		// so commands like top, free displays stats in container.
		"/proc/cpuinfo",
		"/proc/diskstats",
		"/proc/meminfo",
		"/proc/stat",
		"/proc/net/dev",
	}
	for _, valid := range validDestinations {
		path, err := filepath.Rel(filepath.Join(rootfs, valid), dest)
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
	}
	for _, invalid := range invalidDestinations {
		path, err := filepath.Rel(filepath.Join(rootfs, invalid), dest)
		if err != nil {
			return err
		}
		if path == "." || !strings.HasPrefix(path, "..") {
			return fmt.Errorf("%q cannot be mounted because it is located inside %q", dest, invalid)
		}

	}
	return nil
}

// preCreateDirs creates necessary dirs for hyperstart
func preCreateDirs(rootfs string) error {
	dirs := []string{
		"proc",
		"sys",
		"dev",
		"lib/modules",
	}
	for _, dir := range dirs {
		err := createIfNotExists(filepath.Join(rootfs, dir), true)
		if err != nil {
			return err
		}
	}
	return nil
}

// createIfNotExists creates a file or a directory only if it does not already exist.
func createIfNotExists(path string, isDir bool) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if isDir {
				return os.MkdirAll(path, 0755)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			f.Close()
		}
	}
	return nil
}

func setupContainerFs(vm *hypervisor.Vm, bundle, container string, spec *specs.Spec) (err error) {
	containerSharedFs := filepath.Join(hypervisor.BaseDir, vm.Id, hypervisor.ShareDirTag, container)
	rootPath := spec.Root.Path
	if !filepath.IsAbs(rootPath) {
		rootPath = filepath.Join(bundle, rootPath)
	}

	vmRootfs := filepath.Join(containerSharedFs, "rootfs")
	os.MkdirAll(vmRootfs, 0755)

	if err = mount.MakePrivate(containerSharedFs); err != nil {
		glog.Errorf("Make %q private failed: %v", containerSharedFs, err)
		return err
	}

	// Mount rootfs
	err = utils.Mount(rootPath, vmRootfs)
	if err != nil {
		glog.Errorf("mount %s to %s failed: %s\n", rootPath, vmRootfs, err.Error())
		return err
	}

	// Pre-create dirs necessary for hyperstart before setting rootfs readonly
	// TODO: a better way to support readonly rootfs
	if err = preCreateDirs(rootPath); err != nil {
		return err
	}

	// Mount necessary files and directories from spec
	for _, m := range spec.Mounts {
		if err := mountToRootfs(&m, vmRootfs, ""); err != nil {
			return fmt.Errorf("mounting %q to rootfs %q at %q failed: %v", m.Source, m.Destination, vmRootfs, err)
		}
	}

	// set rootfs readonly
	if spec.Root.Readonly {
		err = utils.SetReadonly(vmRootfs)
		if err != nil {
			glog.Errorf("set rootfs %s readonly failed: %s\n", vmRootfs, err.Error())
			return err
		}
	}

	return nil
}

func removeContainerFs(sandboxpath, container string) {
	containerSharedFs := filepath.Join(sandboxpath, hypervisor.ShareDirTag, container)
	utils.Umount(containerSharedFs)
}
