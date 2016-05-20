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

type hyperstartCmd struct {
	Code    uint32
	Message interface{}
	Event   VmEvent

	// result
	retMsg []byte
	result chan<- error
}

func defaultHyperstartResultChan(ctx *VmContext, cmd *hyperstartCmd) chan<- error {
	result := make(chan error, 1)
	go func() {
		err := <-result
		if err == nil {
			ctx.Hub <- &CommandAck{
				reply: cmd,
				msg:   cmd.retMsg,
			}
		} else {
			ctx.Hub <- &CommandError{
				reply: cmd,
				msg:   cmd.retMsg,
			}
		}
	}()
	return result
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

	msg, err := hyperstartapi.ReadVmMessage(conn.(*net.UnixConn))
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
	cmds := []*hyperstartCmd{}

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
		if cmd.result == nil {
			cmd.result = defaultHyperstartResultChan(ctx, cmd)
		}
		glog.Infof("got cmd:%d", cmd.Code)
		if cmd.Code == hyperstartapi.INIT_ACK || cmd.Code == hyperstartapi.INIT_ERROR {
			if len(cmds) > 0 {
				if cmds[0].Code == hyperstartapi.INIT_DESTROYPOD {
					glog.Info("got response of shutdown command, last round of command to init")
					looping = false
				}
				if cmd.Code == hyperstartapi.INIT_ACK {
					if cmds[0].Code != hyperstartapi.INIT_PING {
						cmds[0].retMsg = cmd.retMsg
						cmds[0].result <- nil
					}
				} else {
					cmds[0].retMsg = cmd.retMsg
					cmds[0].result <- fmt.Errorf("Error: %s", string(cmd.retMsg))
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
						ctx.vm <- &hyperstartCmd{
							Code: hyperstartapi.INIT_PING,
						}
						pingTimer = nil
					})
				} else {
					pingTimer.Reset(30 * time.Second)
				}
			} else {
				glog.Error("got ack but no command in queue")
			}
		} else if cmd.Code == hyperstartapi.INIT_FINISHPOD {
			num := len(cmd.retMsg) / 4
			results := make([]uint32, num)
			for i := 0; i < num; i++ {
				results[i] = binary.BigEndian.Uint32(cmd.retMsg[i*4 : i*4+4])
			}

			for _, c := range cmds {
				if c.Code == hyperstartapi.INIT_DESTROYPOD {
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
			if cmd.Code == hyperstartapi.INIT_NEXT {
				glog.V(1).Infof("get command NEXT")

				got += int(binary.BigEndian.Uint32(cmd.retMsg[0:4]))
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
				msg, err := hyperstartapi.NewVmMessage(cmd.Code, cmd.Message)
				if err != nil {
					glog.Infof("marshal command %d failed. object: %v", cmd.Code, cmd.Message)
					cmd.result <- err
					continue
				}
				glog.V(1).Infof("send command %d to init, payload: '%s'.", cmd.Code, string(msg.Message))
				cmds = append(cmds, cmd)
				data = append(data, hyperstartapi.VmMessage2Bytes(msg)...)
				timeout = true
			}

			if index == 0 && len(data) != 0 {
				var end int = len(data)
				if end > 512 {
					end = 512
				}

				wrote := hyperstartapi.WriteVmMessage(init, data, end)
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
		res, err := hyperstartapi.ReadVmMessage(init)
		if err != nil {
			ctx.Hub <- &Interrupted{Reason: "init socket failed " + err.Error()}
			return
		} else if res.Code == hyperstartapi.INIT_ACK || res.Code == hyperstartapi.INIT_NEXT ||
			res.Code == hyperstartapi.INIT_ERROR || res.Code == hyperstartapi.INIT_FINISHPOD {
			ctx.vm <- &hyperstartCmd{Code: res.Code, retMsg: res.Message}
		}
	}
}
