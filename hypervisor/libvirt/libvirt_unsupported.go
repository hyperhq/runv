// +build !with_libvirt

package libvirt

import (
	"errors"
	"github.com/hyperhq/runv/hypervisor"
)

type LibvirtDriver struct{}

type LibvirtContext struct {
	driver *LibvirtDriver
}

func InitDriver() *LibvirtDriver {
	return nil
}

var unsupportErr error = errors.New("Did not built with libvirt support")

func (ld *LibvirtDriver) Initialize() error {
	return unsupportErr
}

func (ld *LibvirtDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return nil
}

func (ld *LibvirtDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	return nil, unsupportErr
}

func (ld *LibvirtDriver) SupportLazyMode() bool {
	return false
}
