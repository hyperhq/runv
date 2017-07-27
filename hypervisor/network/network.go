package network

import (
	"net"
	"os"

	"github.com/hyperhq/runv/hypervisor/network/ipallocator"
	"github.com/hyperhq/runv/hypervisor/network/portmapper"
)

type Settings struct {
	Mac       string
	IPAddress []string
	Mtu       uint64
	Gateway   string
	Bridge    string
	Device    string
	File      *os.File
	Automatic bool
}

const (
	DefaultBridgeIface = "hyper0"
	DefaultBridgeIP    = "192.168.123.0/24"
	DefaultMtu         = 1450
)

var (
	IpAllocator   = ipallocator.New()
	PortMapper    = portmapper.New()
	BridgeIPv4Net *net.IPNet
	BridgeIface   string
	BridgeIP      string
)
