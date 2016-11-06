// +build linux

package vsock

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func Dial(cid uint32, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}

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
