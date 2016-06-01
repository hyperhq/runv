package utils

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func daemonize(cmd string, args []string, writer io.WriteCloser) error {
	pid, _, sysErr := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
	if sysErr != 0 {
		return fmt.Errorf("fail to call fork")
	}
	if pid > 0 {
		if _, err := syscall.Wait4(int(pid), nil, 0, nil); err != nil {
			return fmt.Errorf("fail to wait for child process: %v", err)
		}
		return nil
	} else if pid < 0 {
		return fmt.Errorf("child id is incorrect")
	}

	ret, err := syscall.Setsid()
	if err != nil || ret < 0 {
		return fmt.Errorf("fail to call setsid")
	}

	signal.Ignore(syscall.SIGHUP)
	syscall.Umask(0)

	nullFile, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("fail to open os.DevNull: %v", err)
	}
	files := []*os.File{
		nullFile, // (0) stdin
		nullFile, // (1) stdout
		nullFile, // (2) stderr
	}
	attr := &os.ProcAttr{
		Dir:   "/",
		Env:   os.Environ(),
		Files: files,
	}
	child, err := os.StartProcess(cmd, args, attr)
	if err != nil {
		return fmt.Errorf("fail to start process: %v", err)
	}

	buff := make([]byte, 4)
	binary.BigEndian.PutUint32(buff[:], uint32(child.Pid))
	if n, err := writer.Write(buff); err != nil || n != 4 {
		return fmt.Errorf("fail to write back the pid")
	}

	os.Exit(0)
	return nil
}

func ExecInDaemon(cmd string, argv []string) (pid uint32, err error) {
	r, w, err := os.Pipe()
	if err != nil {
		return 0, err
	}

	err = daemonize(cmd, argv, w)
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 4)
	nr, err := r.Read(buf)
	if err != nil || nr != 4 {
		return 0, fmt.Errorf("fail to start %s in daemon mode or fail to get pid: %v", argv[0], err)
	}
	r.Close()
	w.Close()
	pid = binary.BigEndian.Uint32(buf[:nr])

	return pid, nil

}
