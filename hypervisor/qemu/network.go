package qemu

import (
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (qd *QemuDriver) BuildinNetwork() bool {
	return false
}

func (qd *QemuDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (qc *QemuContext) ConfigureNetwork(config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (qc *QemuContext) ReleaseNetwork(releasedIP string) error {
	return nil
}
