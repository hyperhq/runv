// +build linux

package vsock

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

const hyperDefaultVsockCid = 1025

func Dial(cid uint32, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	unix.SetNonblock(fd, false)

	dst := &unix.SockaddrVsock{Cid: cid, Port: port}
	err = unix.Connect(fd, dst)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}

	sa, err := unix.Getsockname(fd)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}

	src, ok := sa.(*unix.SockaddrVsock)
	if !ok {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to make vsock connection")
	}

	return newVsockConn(fd, src, dst), nil
}

type VsockCid interface {
	sync.Locker
	GetNextCid() uint32
}

type DefaultVsockCid struct {
	sync.Mutex
	nextCid uint32
}

func NewDefaultVsockCid() VsockCid {
	return &DefaultVsockCid{nextCid: hyperDefaultVsockCid}
}

func (vc *DefaultVsockCid) GetNextCid() uint32 {
	cid := atomic.AddUint32(&vc.nextCid, 1)
	if cid < hyperDefaultVsockCid {
		// overflow
		vc.Lock()
		if vc.nextCid < hyperDefaultVsockCid {
			vc.nextCid = hyperDefaultVsockCid
		}
		vc.Unlock()
		cid = atomic.AddUint32(&vc.nextCid, 1)
	}

	return cid
}
