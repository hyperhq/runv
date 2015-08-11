package qemu

import (
	"os"

	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (qd *QemuDriver) InitNetwork(bIface, bIP string) error {
	return os.ErrNotExist
}

func (qc *QemuContext) AllocateNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, os.ErrNotExist
}

func (qc *QemuContext) ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort,
	file *os.File) error {
	return os.ErrNotExist
}
