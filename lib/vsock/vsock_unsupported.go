// +build darwin dragonfly freebsd netbsd openbsd solaris

package vsock

import (
	"fmt"
	"net"
	"sync"
)

type VsockCidAllocator interface {
	sync.Locker
	GetCid() (uint32, error)
	MarkCidInuse(uint32) bool
	ReleaseCid(uint32)
}

func Dial(cid uint32, port uint32) (net.Conn, error) {
	return nil, fmt.Errorf("vsock is not supported")
}
