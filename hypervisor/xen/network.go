package xen

import (
	"os"

	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (xd *XenDriver) InitNetwork(bIface, bIP string) error {
	return os.ErrNotExist
}

func (xc *XenContext) AllocateNetwork(vmId, requestedIP string,
	maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, os.ErrNotExist
}

func (xc *XenContext) ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort,
	file *os.File) error {
	return os.ErrNotExist
}
