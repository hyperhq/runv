package utils

import (
	"net"
	"time"

	"github.com/hyperhq/runv/lib/vsock"

	"github.com/golang/glog"
)

func DiskId2Name(id int) string {
	var ch byte = 'a' + byte(id%26)
	if id < 26 {
		return string(ch)
	}
	return DiskId2Name(id/26-1) + string(ch)
}

func UnixSocketConnect(name string) (conn net.Conn, err error) {
	glog.Infof("Dialing unix socket %s", name)
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", name)
		if err == nil {
			return
		}
	}

	return
}

func VmSocketConnect(cid uint32, port uint32) (conn net.Conn, err error) {
	glog.Infof("Dialing vsock cid %d port %d", cid, port)
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = vsock.Dial(cid, port)
		if err == nil {
			return
		}
	}

	return
}
