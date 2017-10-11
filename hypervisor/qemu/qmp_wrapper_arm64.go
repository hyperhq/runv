// +build linux,arm64

package qemu

import (
	"fmt"

	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
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
			"netdev":         guest.Device,
			"driver":         "virtio-net-pci",
			"disable-modern": "off",
			"disable-legacy": "on",
			"bus":            "pci.0",
			"addr":           busAddr,
			"mac":            host.Mac,
			"id":             guest.Device,
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
