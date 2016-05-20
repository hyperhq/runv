package json

import (
	"encoding/binary"
)

// Message
type DecodedMessage struct {
	Code    uint32
	Message []byte
}

type TtyMessage struct {
	Session uint64
	Message []byte
}

func (tm *TtyMessage) ToBuffer() []byte {
	length := len(tm.Message) + 12
	buf := make([]byte, length)
	binary.BigEndian.PutUint64(buf[:8], tm.Session)
	binary.BigEndian.PutUint32(buf[8:12], uint32(length))
	copy(buf[12:], tm.Message)
	return buf
}
