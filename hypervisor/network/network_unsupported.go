// +build !linux,!darwin

package network

func InitNetwork(bIface, bIP string) error {
	return nil
}

func Allocate(vmId, requestedIP string, addrOnly bool, maps []pod.UserContainerPort) (*Settings, error) {
	return nil, nil
}

func Release(vmId, releasedIP string, maps []pod.UserContainerPort, file *os.File) error {
	return nil
}
