// +build linux

package vsock

import (
	"fmt"
	"net"
	"sync"

	"github.com/RoaringBitmap/roaring"
	"golang.org/x/sys/unix"
)

const hyperDefaultVsockCid = 1024
const hyperDefaultVsockBitmapSize = 16384

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
	GetNextCid() (uint32, error)
	SetCid(uint32) bool
	ClearCid(uint32)
}

type DefaultVsockCid struct {
	sync.Mutex
	bitmap *roaring.Bitmap
	start  uint32
	size   uint32
	pivot  uint32
}

func NewDefaultVsockCid() VsockCid {
	return &DefaultVsockCid{
		bitmap: roaring.NewBitmap(),
		start:  hyperDefaultVsockCid,
		size:   hyperDefaultVsockBitmapSize,
		pivot:  hyperDefaultVsockCid,
	}
}

func (vc *DefaultVsockCid) GetNextCid() (uint32, error) {
	var cid uint32
	vc.Lock()
	defer vc.Unlock()
	for i := uint32(0); i < vc.size; i++ {
		cid = vc.pivot + i
		if cid >= vc.start+vc.size {
			cid -= vc.size
		}
		if vc.bitmap.CheckedAdd(cid) {
			vc.pivot = cid + 1
			return cid, nil
		}
	}

	return cid, fmt.Errorf("No more available cid")
}

func (vc *DefaultVsockCid) SetCid(cid uint32) bool {
	vc.Lock()
	defer vc.Unlock()
	success := vc.bitmap.CheckedAdd(cid)
	return success
}

func (vc *DefaultVsockCid) ClearCid(cid uint32) {
	vc.Lock()
	defer vc.Unlock()
	vc.bitmap.Remove(cid)
}
