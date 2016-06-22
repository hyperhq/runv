package hypervisor

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/hyperhq/runv/lib/utils"
)

type WindowSize struct {
	Row    uint16 `json:"row"`
	Column uint16 `json:"column"`
}

type TtyIO struct {
	Stdin     io.ReadCloser
	Stdout    io.WriteCloser
	ClientTag string
	Callback  chan *types.VmResponse
	ExitCode  uint8
}

func (tty *TtyIO) WaitForFinish() error {
	tty.ExitCode = 255

	if tty.Callback == nil {
		return fmt.Errorf("cannot wait on this tty")
	}

	Response, ok := <-tty.Callback
	if !ok {
		return fmt.Errorf("get response failed")
	}

	glog.V(1).Infof("Got response: %d: %s", Response.Code, Response.Cause)
	if Response.Code == types.E_EXEC_FINISH {
		tty.ExitCode = Response.Data.(uint8)
		glog.V(1).Infof("Exit code %d", tty.ExitCode)
	}

	if tty.Stdin != nil {
		tty.Stdin.Close()
	}
	if tty.Stdout != nil {
		tty.Stdout.Close()
	}

	return nil
}

type ttyAttachments struct {
	persistent  bool
	started     bool
	closed      bool
	tty         bool
	attachments []*TtyIO
}

type pseudoTtys struct {
	attachId    uint64 //next available attachId for attached tty
	channel     chan *hyperstartapi.TtyMessage
	ttys        map[uint64]*ttyAttachments
	ttySessions map[string]uint64
	pendingTtys []*AttachCommand
	lock        *sync.Mutex
}

func newPts() *pseudoTtys {
	return &pseudoTtys{
		attachId:    1,
		channel:     make(chan *hyperstartapi.TtyMessage, 256),
		ttys:        make(map[uint64]*ttyAttachments),
		ttySessions: make(map[string]uint64),
		pendingTtys: []*AttachCommand{},
		lock:        &sync.Mutex{},
	}
}

func readTtyMessage(conn *net.UnixConn) (*hyperstartapi.TtyMessage, error) {
	needRead := 12
	length := 0
	read := 0
	buf := make([]byte, 512)
	res := []byte{}
	for read < needRead {
		want := needRead - read
		if want > 512 {
			want = 512
		}
		glog.V(1).Infof("tty: trying to read %d bytes", want)
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read tty data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		glog.V(1).Infof("tty: read %d/%d [length = %d]", read, needRead, length)

		if length == 0 && read >= 12 {
			length = int(binary.BigEndian.Uint32(res[8:12]))
			glog.V(1).Infof("data length is %d", length)
			if length > 12 {
				needRead = length
			}
		}
	}

	return &hyperstartapi.TtyMessage{
		Session: binary.BigEndian.Uint64(res[:8]),
		Message: res[12:],
	}, nil
}

func waitTtyMessage(ctx *VmContext, conn *net.UnixConn) {
	for {
		msg, ok := <-ctx.ptys.channel
		if !ok {
			glog.V(1).Info("tty chan closed, quit sent goroutine")
			break
		}

		glog.V(3).Infof("trying to write to session %d", msg.Session)

		if _, ok := ctx.ptys.ttys[msg.Session]; ok {
			_, err := conn.Write(msg.ToBuffer())
			if err != nil {
				glog.V(1).Info("Cannot write to tty socket: ", err.Error())
				return
			}
		}
	}
}

func waitPts(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.TtySockName)
	if err != nil {
		glog.Error("Cannot connect to tty socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot connect to tty socket " + err.Error(),
		}
		return
	}

	glog.V(1).Info("tty socket connected")

	go waitTtyMessage(ctx, conn.(*net.UnixConn))

	for {
		res, err := readTtyMessage(conn.(*net.UnixConn))
		if err != nil {
			glog.V(1).Info("tty socket closed, quit the reading goroutine ", err.Error())
			ctx.Hub <- &Interrupted{Reason: "tty socket failed " + err.Error()}
			close(ctx.ptys.channel)
			return
		}
		if ta, ok := ctx.ptys.ttys[res.Session]; ok {
			if len(res.Message) == 0 {
				glog.V(1).Infof("session %d closed by peer, close pty", res.Session)
				ta.closed = true
			} else if ta.closed {
				var code uint8 = 255
				if len(res.Message) == 1 {
					code = uint8(res.Message[0])
				}
				glog.V(1).Infof("session %d, exit code %d", res.Session, code)
				ctx.ptys.Close(res.Session, code)
			} else {
				for _, tty := range ta.attachments {
					if tty.Stdout != nil {
						_, err := tty.Stdout.Write(res.Message)
						if err != nil {
							glog.V(1).Infof("fail to write session %d, close pty attachment", res.Session)
							ctx.ptys.Detach(res.Session, tty)
						}
					}
				}
			}
		}
	}
}

func newAttachmentsWithTty(persist, isTty bool, tty *TtyIO) *ttyAttachments {
	ta := &ttyAttachments{
		persistent: persist,
		tty:        isTty,
	}

	if tty != nil {
		ta.attach(tty)
	}

	return ta
}

func (ta *ttyAttachments) attach(tty *TtyIO) {
	ta.attachments = append(ta.attachments, tty)
}

func (ta *ttyAttachments) detach(tty *TtyIO) {
	at := []*TtyIO{}
	detached := false
	for _, t := range ta.attachments {
		if tty.ClientTag != t.ClientTag {
			at = append(at, t)
		} else {
			detached = true
		}
	}
	if detached {
		ta.attachments = at
	}
}

func (ta *ttyAttachments) close(code uint8) []string {
	tags := []string{}
	for _, t := range ta.attachments {
		tags = append(tags, t.Close(code))
	}
	ta.attachments = []*TtyIO{}
	return tags
}

func (ta *ttyAttachments) empty() bool {
	return len(ta.attachments) == 0
}

func (ta *ttyAttachments) isTty() bool {
	return ta.tty
}

func (tty *TtyIO) Close(code uint8) string {

	glog.V(1).Info("Close tty ", tty.ClientTag)

	if tty.Callback != nil {
		tty.Callback <- &types.VmResponse{
			Code:  types.E_EXEC_FINISH,
			Cause: "Command finished",
			Data:  code,
		}

		close(tty.Callback)
	} else {
		if tty.Stdin != nil {
			tty.Stdin.Close()
		}
		if tty.Stdout != nil {
			tty.Stdout.Close()
		}
	}

	return tty.ClientTag
}

func (pts *pseudoTtys) nextAttachId() uint64 {
	pts.lock.Lock()
	id := pts.attachId
	pts.attachId++
	pts.lock.Unlock()
	return id
}

func (pts *pseudoTtys) isTty(session uint64) bool {
	if ta, ok := pts.ttys[session]; ok {
		return ta.isTty()
	}
	return false
}

func (pts *pseudoTtys) clientReg(tag string, session uint64) {
	pts.lock.Lock()
	pts.ttySessions[tag] = session
	pts.lock.Unlock()
}

func (pts *pseudoTtys) clientDereg(tag string) {
	if tag == "" {
		return
	}
	pts.lock.Lock()
	if _, ok := pts.ttySessions[tag]; ok {
		delete(pts.ttySessions, tag)
	}
	pts.lock.Unlock()
}

func (pts *pseudoTtys) Detach(session uint64, tty *TtyIO) {
	if ta, ok := pts.ttys[session]; ok {
		pts.lock.Lock()
		ta.detach(tty)
		pts.lock.Unlock()
		if !ta.persistent && ta.empty() {
			pts.Close(session, 0)
		}
		pts.clientDereg(tty.Close(0))
	}
}

func (pts *pseudoTtys) Close(session uint64, code uint8) {
	if ta, ok := pts.ttys[session]; ok {
		pts.lock.Lock()
		tags := ta.close(code)
		delete(pts.ttys, session)
		pts.lock.Unlock()
		for _, t := range tags {
			pts.clientDereg(t)
		}
	}
}

func (pts *pseudoTtys) ptyConnect(persist, isTty bool, session uint64, tty *TtyIO) {
	pts.lock.Lock()
	if ta, ok := pts.ttys[session]; ok {
		ta.attach(tty)
	} else {
		pts.ttys[session] = newAttachmentsWithTty(persist, isTty, tty)
	}
	pts.connectStdin(session, tty)
	pts.lock.Unlock()
}

func (pts *pseudoTtys) startStdin(session uint64, isTty bool) {
	pts.lock.Lock()
	ta, ok := pts.ttys[session]
	if ok {
		if !ta.started {
			ta.started = true
			for _, tty := range ta.attachments {
				pts.connectStdin(session, tty)
			}
		}
	} else {
		ta = newAttachmentsWithTty(true, isTty, nil)
		ta.started = true
		pts.ttys[session] = ta
	}
	pts.lock.Unlock()
}

// we close the stdin of the container when the last attached
// stdin closed. we should move this decision to hyper and use
// the same policy as docker(stdinOnce)
func (pts *pseudoTtys) isLastStdin(session uint64) bool {
	var count int

	pts.lock.Lock()
	if ta, ok := pts.ttys[session]; ok {
		for _, tty := range ta.attachments {
			if tty.Stdin != nil {
				count++
			}
		}
	}
	pts.lock.Unlock()
	return count == 1
}

func (pts *pseudoTtys) connectStdin(session uint64, tty *TtyIO) {
	if ta, ok := pts.ttys[session]; !ok || !ta.started {
		return
	}

	if tty.Stdin != nil {
		go func() {
			buf := make([]byte, 32)
			keys, _ := term.ToBytes(DetachKeys)
			isTty := pts.isTty(session)

			defer func() { recover() }()
			for {
				nr, err := tty.Stdin.Read(buf)
				if nr == 1 && isTty {
					for i, key := range keys {
						if nr != 1 || buf[0] != key {
							break
						}
						if i == len(keys)-1 {
							glog.Info("got stdin detach keys, exit term")
							pts.Detach(session, tty)
							return
						}
						nr, err = tty.Stdin.Read(buf)
					}
				}
				if err != nil {
					glog.Info("a stdin closed, ", err.Error())
					if err == io.EOF && !isTty && pts.isLastStdin(session) {
						// send eof to hyperstart
						glog.V(1).Infof("session %d send eof to hyperstart", session)
						pts.channel <- &hyperstartapi.TtyMessage{
							Session: session,
							Message: make([]byte, 0),
						}
						// don't detach, we need the last output of the container
					} else {
						pts.Detach(session, tty)
					}
					return
				}

				glog.V(3).Infof("trying to input char: %d and %d chars", buf[0], nr)

				mbuf := make([]byte, nr)
				copy(mbuf, buf[:nr])
				pts.channel <- &hyperstartapi.TtyMessage{
					Session: session,
					Message: mbuf[:nr],
				}
			}
		}()
	}

	return
}

func (pts *pseudoTtys) closePendingTtys() {
	for _, tty := range pts.pendingTtys {
		tty.Streams.Close(255)
	}
	pts.pendingTtys = []*AttachCommand{}
}

func TtyLiner(conn io.Reader, output chan string) {
	buf := make([]byte, 1)
	line := []byte{}
	cr := false
	emit := false
	for {

		nr, err := conn.Read(buf)
		if err != nil || nr < 1 {
			glog.V(1).Info("Input byte chan closed, close the output string chan")
			close(output)
			return
		}
		switch buf[0] {
		case '\n':
			emit = !cr
			cr = false
		case '\r':
			emit = true
			cr = true
		default:
			cr = false
			line = append(line, buf[0])
		}
		if emit {
			output <- string(line)
			line = []byte{}
			emit = false
		}
	}
}

func (vm *Vm) Attach(tty *TtyIO, container string, size *WindowSize) error {
	cmd := &AttachCommand{
		Streams:   tty,
		Size:      size,
		Container: container,
	}

	return vm.GenericOperation("Attach", func(ctx *VmContext, result chan<- error) {
		ctx.attachCmd(cmd, result)
	}, StateInit, StateStarting, StateRunning)
}

func (vm *Vm) GetLogOutput(container, tag string, callback chan *types.VmResponse) (io.ReadCloser, io.ReadCloser, error) {
	stdout, stdoutStub := io.Pipe()
	stderr, stderrStub := io.Pipe()
	outIO := &TtyIO{
		Stdin:     nil,
		Stdout:    stdoutStub,
		ClientTag: tag,
		Callback:  callback,
	}
	errIO := &TtyIO{
		Stdin:     nil,
		Stdout:    stderrStub,
		ClientTag: tag,
		Callback:  nil,
	}

	cmd := &AttachCommand{
		Streams:   outIO,
		Stderr:    errIO,
		Container: container,
	}

	vm.GenericOperation("Attach", func(ctx *VmContext, result chan<- error) {
		ctx.attachCmd(cmd, result)
	}, StateInit, StateStarting, StateRunning)

	return stdout, stderr, nil
}
