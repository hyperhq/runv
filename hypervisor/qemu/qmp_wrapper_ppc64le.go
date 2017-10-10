// +build linux,ppc64le

package qemu

import (
	"fmt"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, qc *QemuContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", guest.Busaddr)
	commands := []*QmpCommand{}
	commands = append(commands, &QmpCommand{
		Execute: "netdev_add",
		Arguments: map[string]interface{}{
			"type":   "tap",
			"script": "no",
			"id":     guest.Device,
			"ifname": host.Device,
			"br":     host.Bridge,
		},
	})
	commands = append(commands, &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-pci",
			"netdev": guest.Device,
			"mac":    host.Mac,
			"bus":    "pci.0",
			"addr":   busAddr,
			"id":     guest.Device,
		},
	})

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: defaultRespond(result, &hypervisor.NetDevInsertedEvent{
			Id:         host.Id,
			Index:      guest.Index,
			DeviceName: guest.Device,
			Address:    guest.Busaddr,
		}),
	}
}
