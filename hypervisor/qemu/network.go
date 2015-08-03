package qemu

import (
	"os"
	"fmt"

	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (qd *QemuDriver) InitNetwork(bIface, bIP string) error {
	return fmt.Errorf("Please use generic network driver")
}

func (qc *QemuContext) AllocateNetwork(vmId, requestedIP string,
		maps []pod.UserContainerPort) (*network.Settings, error) {
	return nil, fmt.Errorf("Please use generic network driver")
}

func (qc *QemuContext) ReleaseNetwork(vmId, releasedIP string, maps []pod.UserContainerPort,
				file *os.File) error {
	return fmt.Errorf("Please use generic network driver")
}
