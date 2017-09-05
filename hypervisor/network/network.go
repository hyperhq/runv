package network

import (
	"net"
	"os"

	"github.com/hyperhq/runv/hypervisor/network/ipallocator"
)

type Settings struct {
	Mac       string
	IP        []string
	Gateway   string
	Bridge    string
	Device    string
	Mtu       uint64
	File      *os.File
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
