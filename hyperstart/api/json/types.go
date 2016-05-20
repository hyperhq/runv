package json

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io"
	"syscall"
)

type FileCommand struct {
	Container string `json:"container"`
	File      string `json:"file"`
}

type KillCommand struct {
	Container string         `json:"container"`
	Signal    syscall.Signal `json:"signal"`
}

type ExecCommand struct {
	Container string  `json:"container,omitempty"`
	Process   Process `json:"process"`
}

type Routes struct {
	Routes []Route `json:"routes,omitempty"`
}

// Message
type DecodedMessage struct {
	Code    uint32
	Message []byte
}

func NewVmMessage(code uint32, msg interface{}) (*DecodedMessage, error) {
	var message []byte
	if message1, ok := msg.([]byte); ok {
		message = message1
	} else if message2, err := json.Marshal(msg); err == nil {
		message = message2
	} else {
		return nil, fmt.Errorf("marshal command %d failed", code)
	}
	return &DecodedMessage{
		Code:    code,
		Message: message,
	}, nil
}

func VmMessage2Bytes(m *DecodedMessage) []byte {
	length := len(m.Message) + 8
	msg := make([]byte, length)
	binary.BigEndian.PutUint32(msg[:], uint32(m.Code))
	binary.BigEndian.PutUint32(msg[4:], uint32(length))
	copy(msg[8:], m.Message)
	return msg
}

func WriteVmMessage(w io.Writer, data []byte, end int) int {
	wrote, _ := w.Write(data[:end])
	glog.V(1).Infof("write %d to init, payload: '%s'.", wrote, data[:end])
	return wrote
}

func ReadVmMessage(r io.Reader) (*DecodedMessage, error) {
	needRead := 8
	length := 0
	read := 0
	buf := make([]byte, 512)
	res := []byte{}
	for read < needRead {
		want := needRead - read
		if want > 512 {
			want = 512
		}
		glog.V(1).Infof("trying to read %d bytes", want)
		nr, err := r.Read(buf[:want])
		if err != nil {
			glog.Error("read init data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		glog.V(1).Infof("read %d/%d [length = %d]", read, needRead, length)

		if length == 0 && read >= 8 {
			length = int(binary.BigEndian.Uint32(res[4:8]))
			glog.V(1).Infof("data length is %d", length)
			if length > 8 {
				needRead = length
			}
		}
	}

	return &DecodedMessage{
		Code:    binary.BigEndian.Uint32(res[:4]),
		Message: res[8:],
	}, nil
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

type WindowSizeMessage struct {
	Seq    uint64 `json:"seq"`
	Row    uint16 `json:"row"`
	Column uint16 `json:"column"`
}
