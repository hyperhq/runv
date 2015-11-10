package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/term"
)

type tty struct {
	stateDir string
	tag      string
	termFd   uintptr
	terminal bool
}

type ttyWinSize struct {
	Tag    string
	Height int
	Width  int
}

// stdin/stdout <-> conn
func containerTtySplice(stateDir string, conn net.Conn) (int, error) {
	tag, err := runvGetTag(conn)
	if err != nil {
		return -1, err
	}
	fmt.Printf("tag=%s\n", tag)

	inFd, _ := term.GetFdInfo(os.Stdin)
	outFd, isTerminalOut := term.GetFdInfo(os.Stdout)
	oldState, err := term.SetRawTerminal(inFd)
	if err != nil {
		return -1, err
	}
	defer term.RestoreTerminal(inFd, oldState)

	br := bufio.NewReader(conn)

	receiveStdout := make(chan error, 1)
	go func() {
		_, err = io.Copy(os.Stdout, br)
		receiveStdout <- err
	}()

	sendStdin := make(chan error, 1)
	go func() {
		io.Copy(conn, os.Stdin)

		if sock, ok := conn.(interface {
			CloseWrite() error
		}); ok {
			if err := sock.CloseWrite(); err != nil {
				fmt.Printf("Couldn't send EOF: %s\n", err.Error())
			}
		}
		// Discard errors due to pipe interruption
		sendStdin <- nil
	}()

	newTty(stateDir, tag, outFd, isTerminalOut).monitorTtySize()

	if err := <-receiveStdout; err != nil {
		return -1, err
	}
	sendStdin <- nil

	if err := <-sendStdin; err != nil {
		return -1, err
	}

	return 0, nil
}

func newTty(stateDir string, tag string, termFd uintptr, terminal bool) *tty {
	return &tty{
		stateDir: stateDir,
		tag:      tag,
		termFd:   termFd,
		terminal: terminal,
	}
}

func (tty *tty) resizeTty() {
	if !tty.terminal {
		return
	}

	height, width := getTtySize(tty.termFd)

	conn, err := net.Dial("unix", path.Join(tty.stateDir, "runv.sock"))
	if err != nil {
		fmt.Printf("resize dial fail\n")
		return //TODO
	}

	winSize := &ttyWinSize{
		Tag:    tty.tag,
		Height: height,
		Width:  width,
	}
	cmd, err := json.Marshal(winSize)
	if err != nil {
		fmt.Printf("resize encode fail\n")
		return //TODO
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_WINSIZE,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])
	conn.Close()
}

func (tty *tty) monitorTtySize() {
	tty.resizeTty()
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for range sigchan {
			tty.resizeTty()
		}
	}()
}

func getTtySize(termFd uintptr) (int, int) {
	ws, err := term.GetWinsize(termFd)
	if err != nil {
		fmt.Printf("Error getting size: %s", err.Error())
		if ws == nil {
			return 0, 0
		}
	}
	return int(ws.Height), int(ws.Width)
}
