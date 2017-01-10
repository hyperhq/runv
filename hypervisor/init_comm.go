package hypervisor

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/lib/utils"
)

type hyperstartCmd struct {
	Code     uint32
	Message  interface{}
	Event    VmEvent
	callback func(error, []byte)
}

func NewVmMessage(m *hyperstartapi.DecodedMessage) []byte {
	length := len(m.Message) + 8
	msg := make([]byte, length)
	binary.BigEndian.PutUint32(msg[:], uint32(m.Code))
	binary.BigEndian.PutUint32(msg[4:], uint32(length))
	copy(msg[8:], m.Message)
	return msg
}

func ReadVmMessage(conn *net.UnixConn) (*hyperstartapi.DecodedMessage, error) {
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
		nr, err := conn.Read(buf[:want])
		if err != nil {
			glog.Error("read init data failed")
			return nil, err
		}

		res = append(res, buf[:nr]...)
		read = read + nr

		if length == 0 && read >= 8 {
			length = int(binary.BigEndian.Uint32(res[4:8]))
			if length > 8 {
				needRead = length
			}
		}
	}

	return &hyperstartapi.DecodedMessage{
		Code:    binary.BigEndian.Uint32(res[:4]),
		Message: res[8:],
	}, nil
}

func waitInitReady(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot connect to hyper socket " + err.Error(),
		}
		return
	}

	if ctx.Boot.BootFromTemplate {
		glog.Info("boot from template")
		ctx.PauseState = PauseStatePaused
		ctx.Hub <- &InitConnectedEvent{conn: conn.(*net.UnixConn)}
		go waitCmdToInit(ctx, conn.(*net.UnixConn))
		// TODO call getVMHyperstartAPIVersion(ctx) after unpaused
		return
	}

	glog.Info("Wating for init messages...")

	msg, err := ReadVmMessage(conn.(*net.UnixConn))
	if err != nil {
		glog.Error("read init message failed... ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "read init message failed... " + err.Error(),
		}
		conn.Close()
	} else if msg.Code == hyperstartapi.INIT_READY {
		glog.Info("Get init ready message")
		ctx.Hub <- &InitConnectedEvent{conn: conn.(*net.UnixConn)}
		if !ctx.Boot.BootToBeTemplate {
			go waitCmdToInit(ctx, conn.(*net.UnixConn))
		}
	} else {
		glog.Warningf("Get init message %d", msg.Code)
		ctx.Hub <- &InitFailedEvent{
			Reason: fmt.Sprintf("Get init message %d", msg.Code),
		}
		conn.Close()
	}
}

func connectToInit(ctx *VmContext) {
	conn, err := utils.UnixSocketConnect(ctx.HyperSockName)
	if err != nil {
		glog.Error("Cannot re-connect to hyper socket ", err.Error())
		ctx.Hub <- &InitFailedEvent{
			Reason: "Cannot re-connect to hyper socket " + err.Error(),
		}
		return
	}

	go waitCmdToInit(ctx, conn.(*net.UnixConn))
}

func convertMsg2Data(cmd *hyperstartCmd) ([]byte, error) {
	var message []byte
	if message1, ok := cmd.Message.([]byte); ok {
		message = message1
	} else if message2, err := json.Marshal(cmd.Message); err == nil {
		message = message2
	} else {
		glog.Infof("marshal command %d failed. object: %v", cmd.Code, cmd.Message)
		return nil, fmt.Errorf("marshal command %d failed", cmd.Code)
	}

	msg := &hyperstartapi.DecodedMessage{
		Code:    cmd.Code,
		Message: message,
	}
	glog.V(1).Infof("send command %d to init, payload: '%s'.", cmd.Code, string(msg.Message))

	return NewVmMessage(msg), nil
}

func getNextHyperstartCmd(ctx *VmContext) *hyperstartCmd {
	select {
	case cmd, ok := <-ctx.vm:
		if !ok {
			glog.Info("context is closed, last round of command to init")
			return nil
		}
		return cmd
	case <-time.After(30 * time.Second):
		return &hyperstartCmd{
			Code:     hyperstartapi.INIT_PING,
			callback: func(error, []byte) {},
		}
	}
}

func waitCmdToInit(ctx *VmContext, init *net.UnixConn) {
	var index int = 0
	var got int = 0
	var timer *time.Timer = nil
	var hyperstartReplyChan = make(chan *hyperstartapi.DecodedMessage, 1)

	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	go waitInitAck(ctx, init, hyperstartReplyChan)

	//get hyperstart version firstly
	cmd := &hyperstartCmd{
		Code: hyperstartapi.INIT_VERSION,
		callback: func(err error, data []byte) {
			if len(data) < 4 {
				glog.Infof("get hyperstart API version error: %v\n", data)
				return
			}
			ctx.vmHyperstartAPIVersion = binary.BigEndian.Uint32(data[:4])
			glog.Infof("hyperstart API version: %d, VM hyperstart API version: %d\n",
				hyperstartapi.VERSION, ctx.vmHyperstartAPIVersion)
		},
	}

	for cmd != nil {
		if cmd.callback == nil {
			cmd.callback = func(err error, data []byte) {
				if err == nil {
					ctx.Hub <- &CommandAck{reply: cmd, msg: data}
				} else {
					ctx.Hub <- &CommandError{reply: cmd, msg: data}
				}
			}
		}
		glog.Infof("got cmd:%d", cmd.Code)

		data, err := convertMsg2Data(cmd)
		if err != nil {
			cmd.callback(err, nil)
			cmd = getNextHyperstartCmd(ctx)
			continue
		}
		for {
			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote, _ := init.Write(data[:end])
				glog.V(1).Infof("write %d to hyperstart.", wrote)
				index += wrote
				// timeout
				if timer == nil {
					glog.V(1).Info("message sent, set pong timer")
					timer = time.AfterFunc(30*time.Second, func() {
						if ctx.PauseState == PauseStateUnpaused {
							ctx.Hub <- &Interrupted{Reason: "init not reply ping mesg"}
						}
					})
				}
			}
			msg := <-hyperstartReplyChan
			if msg.Code == hyperstartapi.INIT_ACK || msg.Code == hyperstartapi.INIT_ERROR {
				if timer != nil {
					glog.V(1).Info("ack got, clear pong timer")
					timer.Stop()
					timer = nil
				}
				err = nil
				if msg.Code == hyperstartapi.INIT_ERROR {
					err = fmt.Errorf("Error: %s", string(msg.Message))
				}
				cmd.callback(err, msg.Message)
				if cmd.Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got response of shutdown command, last round of command to init")
					return
				}
				break
			} else if msg.Code == hyperstartapi.INIT_NEXT {
				got += int(binary.BigEndian.Uint32(msg.Message[0:4]))
				glog.V(1).Infof("get command NEXT: send %d, receive %d", index, got)
				if index == got {
					/* received the sent out message */
					tmp := data[index:]
					data = tmp
					index = 0
					got = 0
				}
			}
		}

		cmd = getNextHyperstartCmd(ctx)
	}
}

func waitInitAck(ctx *VmContext, init *net.UnixConn, reply chan *hyperstartapi.DecodedMessage) {
	for {
		res, err := ReadVmMessage(init)
		if err != nil {
			ctx.Hub <- &Interrupted{Reason: "init socket failed " + err.Error()}
			return
		}

		glog.V(3).Infof("ReadVmMessage code: %d, len: %d", res.Code, len(res.Message))
		if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR {
			reply <- res
		} else if res.Code == hyperstartapi.INIT_PROCESSASYNCEVENT {
			var pae hyperstartapi.ProcessAsyncEvent
			glog.V(3).Infof("ProcessAsyncEvent: %s", string(res.Message))
			if err := json.Unmarshal(res.Message, &pae); err != nil {
				glog.V(1).Info("read invalid ProcessAsyncEvent")
			} else {
				ctx.handleProcessAsyncEvent(&pae)
			}
		}
	}
}
