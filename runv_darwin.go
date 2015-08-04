package main

import (
	"os"
	"fmt"
	"path"
	"path/filepath"
	"syscall"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func setupContainer(userPod *pod.UserPod, sharedDir string) ([]*hypervisor.ContainerInfo, []string) {
	var containerInfoList []*hypervisor.ContainerInfo
	var roots []string

	for _, c := range userPod.Containers {
		var root string
		var err error

		containerId := GenerateRandomID()

		rootDir := path.Join(sharedDir, containerId, "rootfs")

		os.MkdirAll(rootDir, 0755)

		if !filepath.IsAbs(c.Image) {
			root, err = filepath.Abs(c.Image)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				return nil, nil
			}
		} else {
			root = c.Image
		}

		err = syscall.Symlink(root, rootDir)
		if err != nil {
			fmt.Printf("%s\n", err.Error())
			return nil, nil
		}
		roots = append(roots, rootDir)

		containerInfo := &hypervisor.ContainerInfo {
			Id:		containerId,
			Rootfs:		"rootfs",
			Image:		containerId,
			Fstype:		"dir",
		}

		containerInfoList = append(containerInfoList, containerInfo)
	}

	return containerInfoList, roots
}

func cleanupContainer(roots []string) {
	for _, root := range roots {
		syscall.Unlink(root)
	}
}
