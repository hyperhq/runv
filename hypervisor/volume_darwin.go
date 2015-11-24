package hypervisor

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
)

func CreateContainer(userPod *pod.UserPod, sharedDir string, hub chan VmEvent) (string, error) {
	return "", nil
}

func UmountOverlayContainer(shareDir, image string, index int, hub chan VmEvent) {
	glog.Warningf("Non support")
}

func UmountAufsContainer(shareDir, image string, index int, hub chan VmEvent) {
	glog.Warningf("Non support")
}

func UmountVfsContainer(shareDir, image string, index int, hub chan VmEvent) {
	mount := filepath.Join(shareDir, image)
	success := true
	for i := 0; i < 10; i++ {
		time.Sleep(3 * time.Second / 1000)
		err := syscall.Unlink(mount)
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "device or resource busy") {
				success = true
				break
			}
			glog.Warningf("Cannot umount vfs %s: %s", mount, err.Error())
			success = false
		} else {
			success = true
			break
		}
	}
	hub <- &ContainerUnmounted{Index: index, Success: success}
}

func UmountVolume(shareDir, volPath string, name string, hub chan VmEvent) {
	mount := filepath.Join(shareDir, volPath)
	success := true

	if err := syscall.Unlink(mount); err != nil {
		success = false
	}
	if success == true {
		os.Remove(mount)
	}

	// After umount that device, we need to delete it
	hub <- &VolumeUnmounted{Name: name, Success: success}
}

func UmountDMDevice(deviceFullPath, name string, hub chan VmEvent) {
	// After umount that device, we need to delete it
	hub <- &BlockdevRemovedEvent{Name: name, Success: true}
}

func supportAufs() bool {
	return false
}

func supportOverlay() bool {
	return false
}
