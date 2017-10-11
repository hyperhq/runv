// +build linux,s390x

package qemu

import (
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

func newNetworkAddSession(ctx *hypervisor.VmContext, qc *QemuContext, id string, fd int, device, mac string, index, addr int, result chan<- hypervisor.VmEvent) {
	commands := []*QmpCommand{}
	scm := syscall.UnixRights(fd)
	commands = appends(commands, &QmpCommand{
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
			"driver": "virtio-net-ccw",
			"netdev": guest.Device,
			"mac":    host.Mac,
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
