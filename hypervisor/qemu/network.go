// +build linux

package qemu

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/vishvananda/netlink"
)

func GetTapDevice(device, bridge, options string) error {
	la := netlink.NewLinkAttrs()
	la.Name = device
	tapdev := &netlink.Tuntap{LinkAttrs: la, Mode: syscall.IFF_TAP}

	if err := netlink.LinkAdd(tapdev); err != nil {
		glog.Errorf("fail to create tap device: %v, %v", device, err)
		return err
	}

	if err := network.UpAndAddToBridge(device, bridge, options); err != nil {
		glog.Errorf("Add to bridge failed %s %s", bridge, device)
		return err
	}

	return nil
}

func GetVhostUserPort(device, bridge, sockPath, option string) error {
	glog.V(3).Infof("Found ovs bridge %s, attaching tap %s to it\n", bridge, device)
	// append vhost-server-path
	options := fmt.Sprintf("vhost-server-path=%s/%s", sockPath, device)
	if option != "" {
		options = options + "," + option
	}

	// ovs command "ovs-vsctl add-port BRIDGE PORT" add netwok device PORT to BRIDGE,
	// PORT and BRIDGE here indicate the device name respectively.
	out, err := exec.Command("ovs-vsctl", "--may-exist", "add-port", bridge, device, "--", "set", "Interface", device, "type=dpdkvhostuserclient", "options:"+options).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Ovs failed to add port: %s, error :%v", strings.TrimSpace(string(out)), err)
	}

	return nil
}
