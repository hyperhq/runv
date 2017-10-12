// +build linux,s390x

package qemu

import (
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo, result chan<- hypervisor.VmEvent) {
	commands := []*QmpCommand{}
	//make sure devId is unique, mix unique id with devicename
	devId := guest.Device + host.Id
	commands = appends(commands, &QmpCommand{
		Execute: "netdev_add",
		Arguments: map[string]interface{}{
			"type":   "tap",
			"script": "no",
			"id":     devId,
			"ifname": host.Device,
			"br":     host.Bridge,
		},
	})
	commands = append(commands, &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "virtio-net-ccw",
			"netdev": devId,
			"mac":    host.Mac,
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
