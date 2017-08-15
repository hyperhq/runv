package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/golang/glog"
	_ "github.com/hyperhq/runv/cli/nsenter"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/lib/term"
	"github.com/kardianos/osext"
	"github.com/kr/pty"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

var shimCommand = cli.Command{
	Name:     "shim",
	Usage:    "[internal command] proxy operations(io, signal ...) to the container/process",
	HideHelp: true,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "container",
		},
		cli.StringFlag{
			Name: "process",
		},
		cli.BoolFlag{
			Name: "proxy-exit-code",
		},
		cli.BoolFlag{
			Name: "proxy-stdio",
		},
		cli.BoolFlag{
			Name: "proxy-signal",
		},
		cli.BoolFlag{
			Name: "proxy-winsize",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) error {
		container := context.String("container")
		process := context.String("process")

		h, err := libhyperstart.NewGrpcBasedHyperstart(filepath.Join(context.GlobalString("root"), container, "sandbox", "hyperstartgrpc.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to connect to hyperstart proxy: %v", err), -1)
		}

		if process == "init" {
			waitSigUsr1 := make(chan os.Signal, 1)
			signal.Notify(waitSigUsr1, syscall.SIGUSR1)
			<-waitSigUsr1
			signal.Stop(waitSigUsr1)
		}

		if context.Bool("proxy-stdio") {
			wg := &sync.WaitGroup{}
			proxyStdio(h, container, process, wg)
			defer wg.Wait()
		}

		if context.Bool("proxy-winsize") {
			glog.V(3).Infof("using shim to proxy winsize")
			s, err := term.SetRawTerminal(os.Stdin.Fd())
			if err != nil {
				return cli.NewExitError(fmt.Sprintf("failed to set raw terminal: %v", err), -1)
			}
			defer term.RestoreTerminal(os.Stdin.Fd(), s)
			monitorTtySize(h, container, process)
		}

		if context.Bool("proxy-signal") {
			glog.V(3).Infof("using shim to proxy signal")
			sigc := forwardAllSignals(h, container, process)
			defer signal.Stop(sigc)
		}

		// wait until exit
		exitcode := h.WaitProcess(container, process)
		if context.Bool("proxy-exit-code") {
			glog.V(3).Infof("using shim to proxy exit code: %d", exitcode)
			if exitcode != 0 {
				cli.NewExitError("process returns non zero exit code", exitcode)
			}
			return nil
		}

		return nil
	},
}

func proxyStdio(h libhyperstart.Hyperstart, container, process string, wg *sync.WaitGroup) {
	// don't wait the copying of the stdin, because `io.Copy(inPipe, os.Stdin)`
	// can't terminate when no input. todo: find a better way.
	wg.Add(2)
	inPipe, outPipe, errPipe := libhyperstart.StdioPipe(h, container, process)
	go func() {
		_, err1 := io.Copy(inPipe, os.Stdin)
		err2 := h.CloseStdin(container, process)
		glog.V(3).Infof("copy stdin %#v %#v", err1, err2)
	}()

	go func() {
		_, err := io.Copy(os.Stdout, outPipe)
		glog.V(3).Infof("copy stdout %#v", err)
		wg.Done()
	}()

	go func() {
		_, err := io.Copy(os.Stderr, errPipe)
		glog.V(3).Infof("copy stderr %#v", err)
		wg.Done()
	}()
}

func forwardAllSignals(h libhyperstart.Hyperstart, container, process string) chan os.Signal {
	sigc := make(chan os.Signal, 2048)
	// handle all signals for the process.
	signal.Notify(sigc)
	signal.Ignore(syscall.SIGCHLD, syscall.SIGPIPE)

	go func() {
		for s := range sigc {
			if s == syscall.SIGCHLD || s == syscall.SIGPIPE || s == syscall.SIGWINCH {
				//ignore these
				continue
			}
			// forward this signal to container
			sysSig, ok := s.(syscall.Signal)
			if !ok {
				err := fmt.Errorf("can't forward unknown signal %q", s.String())
				fmt.Fprintf(os.Stderr, "%v", err)
				glog.Errorf("%v", err)
				continue
			}
			if err := h.SignalProcess(container, process, sysSig); err != nil {
				err = fmt.Errorf("forward signal %q failed: %v", s.String(), err)
				fmt.Fprintf(os.Stderr, "%v", err)
				glog.Errorf("%v", err)
			}
		}
	}()
	return sigc
}

func createShim(options runvOptions, container, process string, spec *specs.Process) (*os.Process, error) {
	path, err := osext.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find self executable path for %s: %v", os.Args[0], err)
	}

	var ptymaster, tty *os.File
	if options.String("console") != "" {
		tty, err = os.OpenFile(options.String("console"), os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
	} else if options.String("console-socket") != "" {
		ptymaster, tty, err = pty.Open()
		if err != nil {
			return nil, err
		}
		if err = sendtty(options.String("console-socket"), ptymaster); err != nil {
			return nil, err
		}
		ptymaster.Close()
	}

	args := []string{"runv", "--root", options.GlobalString("root")}
	if options.GlobalString("log_dir") != "" {
		args = append(args, "--log_dir", filepath.Join(options.GlobalString("log_dir"), "shim-"+container))
	}
	if options.GlobalBool("debug") {
		args = append(args, "--debug")
	}
	args = append(args, "shim", "--container", container, "--process", process)
	args = append(args, "--proxy-stdio", "--proxy-exit-code", "--proxy-signal")
	if spec.Terminal {
		args = append(args, "--proxy-winsize")
	}

	cmd := exec.Cmd{
		Path: path,
		Args: args,
		Dir:  "/",
		SysProcAttr: &syscall.SysProcAttr{
			Setctty: tty != nil,
			Setsid:  tty != nil || !options.attach,
		},
	}
	if options.withContainer == nil {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWNET
	} else {
		cmd.Env = append(os.Environ(), fmt.Sprintf("_RUNVNETNSPID=%d", options.withContainer.Pid))
	}
	if tty == nil {
		// inherit stdio/tty
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		defer tty.Close()
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	if options.String("pid-file") != "" {
		err = createPidFile(options.String("pid-file"), cmd.Process.Pid)
		if err != nil {
			cmd.Process.Kill()
			return nil, err
		}
	}

	return cmd.Process, nil
}

// createPidFile creates a file with the processes pid inside it atomically
// it creates a temp file with the paths filename + '.' infront of it
// then renames the file
func createPidFile(path string, pid int) error {
	var (
		tmpDir  = filepath.Dir(path)
		tmpName = filepath.Join(tmpDir, fmt.Sprintf(".%s", filepath.Base(path)))
	)
	f, err := os.OpenFile(tmpName, os.O_RDWR|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0666)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%d", pid)
	f.Close()
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
