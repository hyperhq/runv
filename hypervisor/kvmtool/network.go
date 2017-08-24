// +build linux

package kvmtool

import (
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor/network"
)

/*
	kvmtool doesn't support hot-add nic, so we have to start lkvm with
	one nic attached and use kvmtool adjust network mode
*/

func (kd *KvmtoolDriver) InitNetwork(bIface, bIP string, disableIptables bool) error {
	return network.InitNetwork(bIface, bIP, disableIptables)
}

func (kd *KvmtoolDriver) ConfigureNetwork(config *api.InterfaceDescription) (*network.Settings, error) {
	return network.Configure(true, &network.VhostUserInfo{}, config)
}

func (kd *KvmtoolDriver) ReleaseNetwork(releasedIP string) error {
	return network.Release(releasedIP)
}
