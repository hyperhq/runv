// +build !with_xen

package xen

import (
	"errors"
	"github.com/hyperhq/runv/hypervisor"
)

type XenDriver struct{}

type XenContext struct {
	driver *XenDriver
}

var globalDriver *XenDriver = nil

func InitDriver() *XenDriver {
	return nil
}

//judge if the xl is available and if the version and cap is acceptable

func (xd *XenDriver) Initialize() error {
	return errors.New("Did not built with xen support")
}

func (xd *XenDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return nil
}

func (xd *XenDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	return nil, errors.New("Did not built with xen support")
}

func (xd *XenDriver) SupportLazyMode() bool {
	return false
}
