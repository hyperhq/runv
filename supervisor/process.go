package supervisor

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type Process struct {
	Id     string
	Stdin  string
	Stdout string
	Stderr string
	Spec   *specs.Process
	ProcId int

	// inerId is Id or container id + "-init"
	// pass to hypervisor package and HyperPod.Processes
	inerId      string
	ownerCont   *Container
	init        bool
	stdio       *hypervisor.TtyIO
	stdinCloser io.Closer
}

func (p *Process) setupIO() error {
	glog.Infof("process setupIO: stdin %s, stdout %s, stderr %s", p.Stdin, p.Stdout, p.Stderr)

	// use a new go routine to avoid deadlock when stdin is fifo
	go func() {
		if stdinCloser, err := os.OpenFile(p.Stdin, syscall.O_WRONLY, 0); err == nil {
			p.stdinCloser = stdinCloser
		}
	}()

	stdin, err := os.OpenFile(p.Stdin, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	stdout, err := os.OpenFile(p.Stdout, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}
	// TODO: setup stderr
	if p.Spec.Terminal {
	}
	p.stdio = &hypervisor.TtyIO{
		ClientTag: p.inerId,
		Stdin:     stdin,
		Stdout:    stdout,
		Callback:  make(chan *types.VmResponse, 1),
	}
	glog.Infof("process setupIO() success")

	return nil
}

func (p *Process) ttyResize(width, height int) error {
	return p.ownerCont.ownerPod.vm.Tty(p.inerId, height, width)
}

func (p *Process) closeStdin() error {
	var err error
	if p.stdinCloser != nil {
		err = p.stdinCloser.Close()
		p.stdinCloser = nil
	}
	return err
}

func (p *Process) signal(sig int) error {
	if p.init {
		// TODO: change vm.KillContainer()
		return p.ownerCont.ownerPod.vm.KillContainer(p.ownerCont.Id, syscall.Signal(sig))
	} else {
		// TODO support it
		return fmt.Errorf("Kill to non-init process of container is unsupported")
	}
}

func (p *Process) reap() {
	p.closeStdin()
}
