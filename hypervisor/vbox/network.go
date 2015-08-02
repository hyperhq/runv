package vbox

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/hyperhq/runv/lib/glog"
	"github.com/hyperhq/runv/lib/govbox"
	"github.com/hyperhq/runv/hypervisor/pod"
)

func (v *Vbox) InitNetwork(bIface, bIP string) error {
	var err error
	var i = 0

	if bIP == "" {
		BridgeIP = defaultBridgeIP
	} else {
		BridgeIP = bIP
	}

	bip, ipnet, err = net.ParseCIDR(BridgeIP)
	if err != nil {
		return err
	}

	gateway := bip.Mask(ipnet.Mask)
	inc(gateway, 2)

	if !ipnet.Contains(gateway) {
		return fmt.Errorf("get Gateway from BridgeIP %s failed", BridgeIP)
	}

	_, bridgeIPv4Net, err = net.ParseCIDR(gateway.String())
	if err != nil {
		return err
	}

	for bip = bip.Mask(ipnet.Mask); ipnet.Contains(bip) && i < 15; inc(bip, 1) {
		i++
		_, err = ipAllocator.RequestIP(bridgeIPv4Net, bip)

		if err != nil {
			return err
		}
	}

	return nil
}

func SetupPortMaps(vmId string, containerip string, maps []pod.UserContainerPort) error {
	if len(maps) == 0 {
		return nil
	}

	for _, m := range maps {
		var proto string

		if strings.EqualFold(m.Protocol, "udp") {
			proto = "udp"
		} else {
			proto = "tcp"
		}

		rule := virtualbox.PFRule{}
		rule.Proto = virtualbox.PFProto(proto)
		rule.HostIP = nil
		rule.HostPort = uint16(m.HostPort)
		rule.GuestIP = net.ParseIP(containerip)
		rule.GuestPort = uint16(m.ContainerPort)
		err := virtualbox.SetNATPF(vmId, 1, vmId, rule)
		if err != nil {
			return err
		}

		err = portMapper.AllocateMap(m.Protocol, m.HostPort, containerip, m.ContainerPort)
		if err != nil {
			return err
		}
	}
	/* forbid to map ports twice */
	return nil
}

func ReleasePortMaps(vmId string, containerip string, maps []pod.UserContainerPort) error {
	if len(maps) == 0 {
		return nil
	}

	for _, m := range maps {
		glog.V(1).Infof("release port map %d", m.HostPort)
		err := portMapper.ReleaseMap(m.Protocol, m.HostPort)
		if err != nil {
			continue
		}
	}
	/* forbid to map ports twice */
	return nil
}

func (vc *VBoxContext) AllocateNetwork(vmId, requestedIP string, addrOnly bool,
			maps []pod.UserContainerPort) (*Settings, error) {
	ip, err := ipAllocator.RequestIP(bridgeIPv4Net, net.ParseIP(requestedIP))
	if err != nil {
		return nil, err
	}

	maskSize, _ := bridgeIPv4Net.Mask.Size()

	err = SetupPortMaps(vmId, ip.String(), maps)
	if err != nil {
		glog.Errorf("Setup Port Map failed %s", err)
		return nil, err
	}

	return &Settings {
		Mac:         "",
		IPAddress:   ip.String(),
		Gateway:     bridgeIPv4Net.IP.String(),
		Bridge:      "",
		IPPrefixLen: maskSize,
		Device:      "",
		File:        nil,
	}, nil
}

// Release an interface for a select ip
func (vc *VBoxContext) Release(vmId, releasedIP string, maps []pod.UserContainerPort,
				file *os.File) error {
	if err := ipAllocator.ReleaseIP(bridgeIPv4Net, net.ParseIP(releasedIP)); err != nil {
		return err
	}

	if err := ReleasePortMaps(vmId, releasedIP, maps); err != nil {
		glog.Errorf("fail to release port map %s", err)
		return err
	}

	return nil
}

func inc(ip net.IP, count int) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j] += count
		if ip[j] > 0 {
			break
		}
	}
}
