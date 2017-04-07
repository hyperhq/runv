package supervisor

import (
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
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
	glog.V(3).Infof("process setupIO: stdin %s, stdout %s, stderr %s", p.Stdin, p.Stdout, p.Stderr)

	// use a new go routine to avoid deadlock when stdin is fifo
	go func() {
		if stdinCloser, err := os.OpenFile(p.Stdin, syscall.O_WRONLY, 0); err == nil {
			p.stdinCloser = stdinCloser
		}
	}()

	var stdin, stdout, stderr *os.File
	var err error

	stdin, err = os.OpenFile(p.Stdin, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}

	stdout, err = os.OpenFile(p.Stdout, syscall.O_RDWR, 0)
	if err != nil {
		return err
	}

	// Docker does not create stderr if it's a terminal process since at least 1.13+
	// github.com/docker/containerd/containerd-shim/process.go:239
	// This stanza keeps the API somewhat consistent
	if st, err := os.Stat(p.Stderr); st != nil || !p.Spec.Terminal {
		stderr, err = os.OpenFile(p.Stderr, syscall.O_RDWR, 0)
		if err != nil {
			return err
		}
	}

	p.stdio = &hypervisor.TtyIO{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	glog.V(3).Infof("process setupIO() successfully")

	return nil
}

func (p *Process) ttyResize(container string, width, height int) error {
	// If working on the primary process, do not pass execId (it won't be recognized)
	if p.inerId == fmt.Sprintf("%s-init", container) {
		return p.ownerCont.ownerPod.vm.Tty(container, "", height, width)
	}
	return p.ownerCont.ownerPod.vm.Tty(container, p.inerId, height, width)
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
		return p.ownerCont.ownerPod.vm.SignalProcess(p.ownerCont.Id, p.Id, syscall.Signal(sig))
	}
}

func (p *Process) reap() {
	p.closeStdin()
}
