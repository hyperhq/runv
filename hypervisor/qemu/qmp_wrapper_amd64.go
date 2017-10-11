// +build linux,amd64

package qemu

import (
	"fmt"

	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	busAddr := fmt.Sprintf("0x%x", guest.Busaddr)
	//make sure devId is unique, mix unique id with devicename
	devId := guest.Device + host.Id
	commands := []*QmpCommand{}
	if ctx.Boot.EnableVhostUser {
		chardevId := guest.Device + "-chardev"
		commands = append(commands, &QmpCommand{
			Execute: "chardev-add",
			Arguments: map[string]interface{}{
				"id": chardevId,
				"backend": map[string]interface{}{
					"type": "socket",
					"data": map[string]interface{}{
						"addr": map[string]interface{}{
							"type": "unix",
							"data": map[string]interface{}{
								"path": ctx.HomeDir + "/" + host.Id,
							},
						},
						"wait":   false,
						"server": true,
					},
				},
			},
		}, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type":       "vhost-user",
				"id":         devId,
				"chardev":    chardevId,
				"vhostforce": true,
			},
		})
	} else {
		commands = append(commands, &QmpCommand{
			Execute: "netdev_add",
			Arguments: map[string]interface{}{
				"type":   "tap",
				"script": "no",
				"id":     devId,
				"ifname": host.Device,
				"br":     host.Bridge,
			},
		})
	}
	commands = append(commands, &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-pci",
			"netdev": devId,
			"mac":    host.Mac,
			"bus":    "pci.0",
			"addr":   busAddr,
			"id":     devId,
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
