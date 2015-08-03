package network

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/hyperhq/runv/lib/govbox"
	"github.com/hyperhq/runv/hypervisor/pod"
)

const (
	DefaultBridgeIface = "hyper0"
	DefaultBridgeIP    = "192.168.123.0/24"
)

var (
	IpAllocator   = ipallocator.New()
	PortMapper    = portmapper.New()
	BridgeIPv4Net *net.IPNet
	BridgeIface   string
	BridgeIP      string
)


func InitNetwork(bIface, bIP string) error {
	return fmt.Error("Generial Network driver is unsupported on this os")
}

func Allocate(vmId, requestedIP string, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	return nil, fmt.Error("Generial Network driver is unsupported on this os")
}

// Release an interface for a select ip
func Release(vmId, releasedIP string, maps []pod.UserContainerPort, file *os.File) error {
	return fmt.Error("Generial Network driver is unsupported on this os")
}
