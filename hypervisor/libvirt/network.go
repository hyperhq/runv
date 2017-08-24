// +build linux,with_libvirt

package libvirt

import (
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

func (ld *LibvirtDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return network.InitNetwork(bIface, bIP, disableIptables)
}

func (ld *LibvirtDriver) ConfigureNetwork(config *api.InterfaceDescription) (*network.Settings, error) {
	return network.Configure(true, &network.VhostUserInfo{}, config)
}

func (ld *LibvirtDriver) ReleaseNetwork(releasedIP string) error {
	return network.Release(releasedIP)
}
