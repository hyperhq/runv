// +build !linux,!darwin

package network

func InitNetwork(bIface, bIP string, disableIptables bool) error {
	return nil
}

func Allocate(vmId, requestedIP string, index int, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	return nil, nil
}

func Release(vmId, releasedIP string, index int, maps []pod.UserContainerPort, file *os.File) error {
	return nil
}
