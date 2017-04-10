package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/runc/libcontainer/utils"
	netcontext "golang.org/x/net/context"
)

func resizeTty(c types.APIClient, container, process string) {
	ws, err := term.GetWinsize(os.Stdin.Fd())
	if err != nil {
		fmt.Printf("Error getting size: %s", err.Error())
		return
	}

	if _, err = c.UpdateProcess(netcontext.Background(), &types.UpdateProcessRequest{
		Id:     container,
		Pid:    process,
		Width:  uint32(ws.Width),
		Height: uint32(ws.Height),
	}); err != nil {
		fmt.Printf("set winsize failed, %v\n", err)
	}
}

func monitorTtySize(c types.APIClient, container, process string) {
	resizeTty(c, container, process)
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for range sigchan {
			resizeTty(c, container, process)
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
