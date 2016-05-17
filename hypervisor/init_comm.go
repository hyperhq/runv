package hypervisor

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/golang/glog"
	hyperstartapi "github.com/hyperhq/runv/hyperstart/api/json"
	"github.com/hyperhq/runv/lib/telnet"
	"github.com/hyperhq/runv/lib/utils"
)

// Message
type VmMessage struct {
	Msg   *hyperstartapi.DecodedMessage
	Event VmEvent
}

func waitConsoleOutput(ctx *VmContext) {

	conn, err := utils.UnixSocketConnect(ctx.ConsoleSockName)
	if err != nil {
		glog.Error("failed to connected to ", ctx.ConsoleSockName, " ", err.Error())
		return
	}

	glog.V(1).Info("connected to ", ctx.ConsoleSockName)

	tc, err := telnet.NewConn(conn)
	if err != nil {
		glog.Error("fail to init telnet connection to ", ctx.ConsoleSockName, ": ", err.Error())
		return
	}
	glog.V(1).Infof("connected %s as telnet mode.", ctx.ConsoleSockName)

	cout := make(chan string, 128)
	go TtyLiner(tc, cout)

	for {
		line, ok := <-cout
		if ok {
			glog.V(1).Info("[console] ", line)
		} else {
			glog.Info("console output end")
			break
		}
	}
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
		glog.V(1).Infof("trying to read %d bytes", want)
		nr, err := conn.Read(buf[:want])
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
		go waitCmdToInit(ctx, conn.(*net.UnixConn))
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

func waitCmdToInit(ctx *VmContext, init *net.UnixConn) {
	looping := true
	cmds := []*VmMessage{}

	var data []byte
	var timeout bool = false
	var index int = 0
	var got int = 0
	var pingTimer *time.Timer = nil
	var pongTimer *time.Timer = nil

	go waitInitAck(ctx, init)

	for looping {
		cmd, ok := <-ctx.vm
		if !ok {
			glog.Info("vm channel closed, quit")
			break
		}
		glog.Infof("got cmd:%d", cmd.Msg.Code)
		if cmd.Msg.Code == hyperstartapi.INIT_ACK || cmd.Msg.Code == hyperstartapi.INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].Msg.Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got response of shutdown command, last round of command to init")
					looping = false
				}
				if cmd.Msg.Code == hyperstartapi.INIT_ACK {
					if cmds[0].Msg.Code != hyperstartapi.INIT_PING {
						ctx.Hub <- &CommandAck{
							reply: cmds[0],
							msg:   cmd.Msg.Message,
						}
					}
				} else {
					ctx.Hub <- &CommandError{
						reply: cmds[0],
						msg:   cmd.Msg.Message,
					}
				}
				cmds = cmds[1:]

				if pongTimer != nil {
					glog.V(1).Info("ack got, clear pong timer")
					pongTimer.Stop()
					pongTimer = nil
				}
				if pingTimer == nil {
					pingTimer = time.AfterFunc(30*time.Second, func() {
						defer func() { recover() }()
						glog.V(1).Info("Send ping message to init")
						ctx.vm <- &VmMessage{
							Msg: &hyperstartapi.DecodedMessage{Code: hyperstartapi.INIT_PING, Message: []byte{}},
						}
						pingTimer = nil
					})
				} else {
					pingTimer.Reset(30 * time.Second)
				}
			} else {
				glog.Error("got ack but no command in queue")
			}
		} else if cmd.Msg.Code == hyperstartapi.INIT_FINISHPOD {
			num := len(cmd.Msg.Message) / 4
			results := make([]uint32, num)
			for i := 0; i < num; i++ {
				results[i] = binary.BigEndian.Uint32(cmd.Msg.Message[i*4 : i*4+4])
			}

			for _, c := range cmds {
				if c.Msg.Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got pod finish message after having send destroy message")
					looping = false
					ctx.Hub <- &CommandAck{
						reply: c,
					}
					break
				}
			}

			glog.V(1).Infof("Pod finished, returned %d values", num)

			ctx.Hub <- &PodFinished{
				result: results,
			}
		} else {
			if cmd.Msg.Code == hyperstartapi.INIT_NEXT {
				glog.V(1).Infof("get command NEXT")

				got += int(binary.BigEndian.Uint32(cmd.Msg.Message[0:4]))
				glog.V(1).Infof("send %d, receive %d", index, got)
				timeout = false
				if index == got {
					/* received the sent out message */
					tmp := data[index:]
					data = tmp
					index = 0
					got = 0
				}
			} else {
				glog.V(1).Infof("send command %d to init, payload: '%s'.", cmd.Msg.Code, string(cmd.Msg.Message))
				cmds = append(cmds, cmd)
				data = append(data, NewVmMessage(cmd.Msg)...)
				timeout = true
			}

			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote, _ := init.Write(data[:end])
				glog.V(1).Infof("write %d to init, payload: '%s'.", wrote, data[:end])
				index += wrote
			}

			if timeout && pongTimer == nil {
				glog.V(1).Info("message sent, set pong timer")
				pongTimer = time.AfterFunc(30*time.Second, func() {
					if !ctx.Paused {
						ctx.Hub <- &Interrupted{Reason: "init not reply ping mesg"}
					}
				})
			}
		}
	}

	if pingTimer != nil {
		pingTimer.Stop()
	}
	if pongTimer != nil {
		pongTimer.Stop()
	}
}

func waitInitAck(ctx *VmContext, init *net.UnixConn) {
	for {
		res, err := ReadVmMessage(init)
		if err != nil {
			ctx.Hub <- &Interrupted{Reason: "init socket failed " + err.Error()}
			return
		} else if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR || res.Code == hyperstartapi.INIT_FINISHPOD {
			ctx.vm <- &VmMessage{Msg: res}
		}
	}
}
