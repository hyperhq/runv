package hypervisor

import (
	"fmt"
	"github.com/hyperhq/runv/hypervisor/network"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
	"net"
	"os"
)

func (ctx *VmContext) CreateInterface(index int, pciAddr int, name string,
			maps []pod.UserContainerPort) {
	inf, err := network.Allocate(ctx.Id, "", ctx.DCtx.BuildinNetwork(), maps)
	if err != nil {
		glog.Error("interface creating failed: ", err.Error())
		callback <- &DeviceFailed{
			Session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
		}
		return
	}

	interfaceGot(index, pciAddr, name, ctx.Hub, inf)
}

func (ctx *VmContext) ReleaseInterface(index int, ipAddr string, file *os.File,
			maps []pod.UserContainerPort) {
	success := true
	err := network.Release(ctx.Id, ipAddr, maps, file)
	if err != nil {
		glog.Warning("Unable to release network interface, address: ", ipAddr, err)
		success = false
	}
	ctx.Hub <- &InterfaceReleased{Index: index, Success: success}
}

func interfaceGot(index int, pciAddr int, name string, callback chan VmEvent, inf *network.Settings) {
	ip, nw, err := net.ParseCIDR(fmt.Sprintf("%s/%d", inf.IPAddress, inf.IPPrefixLen))
	if err != nil {
		glog.Error("can not parse cidr")
		callback <- &DeviceFailed{
			Session: &InterfaceCreated{Index: index, PCIAddr: pciAddr, DeviceName: name},
		}
		return
	}
	var tmp []byte = nw.Mask
	var mask net.IP = tmp

	rt := []*RouteRule{
	//        &RouteRule{
	//            Destination: fmt.Sprintf("%s/%d", nw.IP.String(), inf.IPPrefixLen),
	//            Gateway:"", ViaThis:true,
	//        },
	}
	if index == 0 {
		rt = append(rt, &RouteRule{
			Destination: "0.0.0.0/0",
			Gateway:     inf.Gateway, ViaThis: true,
		})
	}

	event := &InterfaceCreated{
		Index:      index,
		PCIAddr:    pciAddr,
		Bridge:     inf.Bridge,
		HostDevice: inf.Device,
		DeviceName: name,
		Fd:         inf.File,
		MacAddr:    inf.Mac,
		IpAddr:     ip.String(),
		NetMask:    mask.String(),
		RouteTable: rt,
	}

	callback <- event
}
