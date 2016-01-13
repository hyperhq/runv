package utils

import (
	"bytes"
	"encoding/json"
	"net"
	"time"
)

func DiskId2Name(id int) string {
	var ch byte = 'a' + byte(id%26)
	if id < 26 {
		return string(ch)
	}
	return DiskId2Name(id/26-1) + string(ch)
}

func UnixSocketConnect(name string) (conn net.Conn, err error) {
	for i := 0; i < 500; i++ {
		time.Sleep(20 * time.Millisecond)
		conn, err = net.Dial("unix", name)
		if err == nil {
			return
		}
	}

	return
}

func JsonMarshal(v interface{}, shellFriendly bool) ([]byte, error) {
	b, err := json.Marshal(v)

	if err == nil && shellFriendly {
		b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
		b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
		b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)
	}

	return b, err
}
