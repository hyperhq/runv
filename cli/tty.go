package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperhq/runv/agent"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runc/libcontainer/utils"
)

func resizeTty(h agent.SandboxAgent, container, process string) {
	ws, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		fmt.Printf("Error getting size: %s", err.Error())
		return
	}

	if err = h.TtyWinResize(container, process, uint16(ws.Height), uint16(ws.Width)); err != nil {
		fmt.Printf("set winsize failed, %v\n", err)
	}
}

func monitorTtySize(h agent.SandboxAgent, container, process string) {
	resizeTty(h, container, process)
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for range sigchan {
			resizeTty(h, container, process)
		}
	}()
}

func sendtty(consoleSocket string, pty *os.File) error {
	// the caller of runc will handle receiving the console master
	conn, err := net.Dial("unix", consoleSocket)
	if err != nil {
		return err
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("casting to UnixConn failed")
	}
	socket, err := uc.File()
	if err != nil {
		return err
	}

	return utils.SendFd(socket, pty)
}
