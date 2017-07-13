package network

import (
	"fmt"
	"os"

	"github.com/hyperhq/runv/api"
)

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}

func Configure(addrOnly bool, inf *api.InterfaceDescription) (*Settings, error) {
	return nil, fmt.Errorf("Generial Network driver is unsupported on this os")
}

// Release an interface for a select ip
func Release(releasedIP string) error {
	return fmt.Errorf("Generial Network driver is unsupported on this os")
}
