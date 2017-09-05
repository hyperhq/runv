package network

import (
	"fmt"
	"net"

	"github.com/hyperhq/runv/hypervisor/network/ipallocator"
)

type Settings struct {
	Mac         string
	IPAddress   string
	IPPrefixLen int
	Gateway     string
	Bridge      string
	Device      string
	Automatic   bool
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
