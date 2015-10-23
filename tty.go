package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/term"
)

type tty struct {
	vm       *hypervisor.Vm
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

func newTty(vm *hypervisor.Vm, stateDir string, tag string, termFd uintptr, terminal bool) *tty {
	return &tty{
		vm:       vm,
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
	if tty.vm != nil {
		tty.vm.Tty(tty.tag, height, width)
		return
	}

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
