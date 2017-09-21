package network

import (
	"fmt"
	"net"

	"github.com/hyperhq/runv/hypervisor/network/ipallocator"
)

type Settings struct {
	Mac       string
	IPAddress string
	Gateway   string
	Bridge    string
	Device    string
	Mtu       uint64
	Automatic bool
}

const (
	DefaultBridgeIface = "hyper0"
	DefaultBridgeIP    = "192.168.123.0/24"
)

var (
	IpAllocator   = ipallocator.New()
	BridgeIPv4Net *net.IPNet
	BridgeIface   string
	BridgeIP      string
)

func NicName(id string, index int) string {
	return fmt.Sprintf("%s%d", id, index)
}

func IpParser(ipstr string) (net.IP, net.IPMask, error) {
	ip, ipnet, err := net.ParseCIDR(ipstr)
	if err == nil {
		return ip, ipnet.Mask, nil
	}

	ip = net.ParseIP(ipstr)
	if ip != nil {
		return ip, ip.DefaultMask(), nil
	}

	return nil, nil, err
}
