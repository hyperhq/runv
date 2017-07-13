// +build linux,with_xen

package xen

import (
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (xd *XenDriver) BuildinNetwork() bool {
	return false
}

func (xd *XenDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func (xc *XenContext) ConfigureNetwork(config *api.InterfaceDescription) (*network.Settings, error) {
	return nil, nil
}

func (xc *XenContext) ReleaseNetwork(releasedIP string) error {
	return nil
}
