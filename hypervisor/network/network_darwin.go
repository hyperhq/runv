package network

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/hyperhq/runv/hypervisor/network/iptables"
	"github.com/hyperhq/runv/hypervisor/network/portmapper"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/lib/glog"
	"github.com/hyperhq/runv/lib/govbox"
)

var (
	IPAddressList = map[string]bool{}
	NICNum        = 1
	portMapper    = portmapper.New()
)

func InitNetwork(bIface, bIP string) error {
	if bIface == "" {
		BridgeIface = DefaultBridgeIface
	} else {
		BridgeIface = bIface
	}

	if bIP == "" {
		BridgeIP = DefaultBridgeIP
	} else {
		BridgeIP = bIP
	}
	return nil
}

func SetupPortMaps(vmId string, index int, containerip string, maps []pod.UserContainerPort) error {
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

		if iptables.PortMapExists(proto, strconv.Itoa(m.HostPort)) {
			return nil
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

func ReleasePortMaps(vmId string, index int, containerip string, maps []pod.UserContainerPort) error {
	if len(maps) == 0 {
		return nil
	}

	for _, m := range maps {
		glog.V(1).Infof("release port map %d", m.HostPort)
		err := portMapper.ReleaseMap(m.Protocol, m.HostPort)
		if err != nil {
			continue
		}

		var proto string

		if strings.EqualFold(m.Protocol, "udp") {
			proto = "udp"
		} else {
			proto = "tcp"
		}

		iptables.OperatePortMap(iptables.Delete, vmId, index, proto, m.HostPort, containerip, m.ContainerPort)
	}
	/* forbid to map ports twice */
	return nil
}

func Allocate(vmId, requestedIP string, index int, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	ip, ipnet, err := net.ParseCIDR(BridgeIP)
	if err != nil {
		return nil, err
	}
	var (
		i       = 0
		gateway = ip.Mask(ipnet.Mask)
		find    = false
	)
	for ; ipnet.Contains(gateway) && i < 2; inc(gateway) {
		i++
	}
	i = 0
	for ip = ip.Mask(ipnet.Mask); ipnet.Contains(ip) && i < 15; inc(ip) {
		i++
	}
	for ; ipnet.Contains(ip); inc(ip) {
		if _, ok := IPAddressList[ip.String()]; !ok {
			IPAddressList[ip.String()] = true
			find = true
			break
		}
	}
	if find == false {
		ip = gateway
		for inc(ip); ipnet.Contains(ip); inc(ip) {
			if _, ok := IPAddressList[ip.String()]; !ok {
				IPAddressList[ip.String()] = true
				find = true
				break
			}
		}
		if find == false {
			return nil, fmt.Errorf("can not find a available IP address")
		}
	}
	// find a available IP to assign
	return &Settings{
		Mac:         "",
		IPAddress:   ip.String(),
		Gateway:     gateway.String(),
		IPPrefixLen: 24,
		Device:      "",
		File:        nil,
	}, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Release an interface for a select ip
func Release(vmId, releasedIP string, index int, maps []pod.UserContainerPort, file *os.File) error {
	if file != nil {
		file.Close()
	}

	delete(IPAddressList, releasedIP)

	if err := ReleasePortMaps(vmId, index, releasedIP, maps); err != nil {
		glog.Errorf("fail to release port map %s", err)
		return err
	}
	return nil
}
