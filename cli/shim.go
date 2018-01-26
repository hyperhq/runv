package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	_ "github.com/hyperhq/runv/cli/nsenter"
	"github.com/kardianos/osext"
	"github.com/kr/pty"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func createShim(options RunvOptions, container, process string, spec *specs.Process) (*os.Process, error) {
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
			Setsid:  tty != nil || !options.Attach,
		},
	}
	if options.WithContainer == nil {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWNET
	} else {
		cmd.Env = append(os.Environ(), fmt.Sprintf("_RUNVNETNSPID=%d", options.WithContainer.Pid))
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
