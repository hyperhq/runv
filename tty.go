package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
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

func sendtty(pty *os.File, container, consoleSocket string) error {
	return fmt.Errorf("sendtty(): TODO: to be implemented")
}
