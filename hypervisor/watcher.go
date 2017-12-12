package hypervisor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/agent"
	"github.com/hyperhq/runv/lib/telnet"
	"github.com/hyperhq/runv/lib/term"
	"github.com/hyperhq/runv/lib/utils"
)

const CONSOLE_PROTO_TELNET = "telnet"
const CONSOLE_PROTO_PTY = "pty"

// depend on initialized HDriver
func GetConsoleProto() string {
	switch HDriver.Name() {
	case "kvmtool":
		return CONSOLE_PROTO_PTY
	default:
		return CONSOLE_PROTO_TELNET
	}
}

func WatchConsole(proto, console string) error {
	var (
		br *bufio.Reader
	)

	switch proto {
	// todo support for xen pv based on https://wiki.xenproject.org/wiki/Connecting_a_Console_to_DomU%27s
	case CONSOLE_PROTO_PTY:
		file, err := os.OpenFile(console, os.O_RDWR|syscall.O_NOCTTY, 0600)
		if err != nil {
			return err
		}

		_, err = term.SetRawTerminal(file.Fd())
		if err != nil {
			glog.Errorf("fail to set raw mode for %v: %v", console, err)
			return err
		}
		br = bufio.NewReader(file)
	case CONSOLE_PROTO_TELNET:
		conn, err := utils.UnixSocketConnect(console)
		if err != nil {
			return err
		}
		tc, err := telnet.NewConn(conn)
		if err != nil {
			return err
		}
		br = bufio.NewReader(tc)
	default:
		return fmt.Errorf("unknown console proto %s", proto)
	}

	for {
		log, _, err := br.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			glog.Errorf("read console %s failed: %v", console, err)
			return nil
		}
		if len(log) != 0 {
			glog.Info("vmconsole: ", string(log))
		}
	}

	return nil
}

func WatchHyperstart(h agent.SandboxAgent) error {
	next := time.NewTimer(10 * time.Second)
	timeout := time.AfterFunc(60*time.Second, func() {
		glog.Errorf("watch agent timeout")
		h.Close()
	})
	defer next.Stop()
	defer timeout.Stop()

	for {
		glog.V(2).Infof("issue VERSION request for keep-alive test")
		_, err := h.APIVersion()
		if err != nil {
			h.Close()
			glog.Errorf("h.APIVersion() failed with %#v", err)
			return err
		}
		if !timeout.Stop() {
			<-timeout.C
		}
		<-next.C
		next.Reset(10 * time.Second)
		timeout.Reset(60 * time.Second)
	}
}
