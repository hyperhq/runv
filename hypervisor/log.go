package hypervisor

import (
	"github.com/golang/glog"
)

const (
	ERROR uint = iota
	WARNING
	INFO
	DEBUG
	TRACE
)

func (ctx *VmContext) LogLevel(level uint) bool {
	if level <= INFO {
		return true
	} else if level == DEBUG {
		return bool(glog.V(1))
	} else if level == TRACE {
		return bool(glog.V(4))
	}
	return false
}

func (ctx *VmContext) Log(level uint, format string, args ...interface{}) {
	var (
		logf func(string, ...interface{})
	)
	switch level {
	case ERROR:
		logf = glog.Errorf
	case WARNING:
		logf = glog.Warningf
	case INFO:
		logf = glog.Infof
	case DEBUG:
		logf = glog.V(1).Infof
	case TRACE:
		logf = glog.V(3).Infof
	default:
		return
	}

	logf(ctx.Id+": "+format, args)
}

func (cc *ContainerContext) Log(level uint, format string, args ...interface{}) {
	cc.sandbox.Log(level, cc.Id+": "+format, args)
}
