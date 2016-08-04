package qemu

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

// implement io.Reader interface
type qemuLogReader struct {
	logFile string
	offset  int64
}

func (f *qemuLogReader) Read(p []byte) (n int, err error) {
	reader, err := os.Open(f.logFile)
	defer reader.Close()
	if err != nil {
		return 0, err
	}
	reader.Seek(f.offset, 0)

	n, err = reader.Read(p)

	if err == io.EOF {
		time.Sleep(1 * time.Second)
	}
	f.offset += int64(n)

	return n, err
}

func watchLogFile(logFile string) {
	file := &qemuLogReader{logFile, 0}
	br := bufio.NewReader(file)

	for {
		log, _, err := br.ReadLine()
		if err == io.EOF {
			continue
		}
		if err != nil {
			glog.Errorf("read log file %s failed: %v", logFile, err)
			return
		}
		glog.Infof("qemu log: %s", string(log))
	}
}

func watchDog(qc *QemuContext, hub chan hypervisor.VmEvent) {
	wdt := qc.wdt
	for {
		msg, ok := <-wdt
		if ok {
			switch msg {
			case "quit":
				glog.V(1).Info("quit watch dog.")
				return
			case "kill":
				success := false
				if qc.process != nil {
					glog.V(0).Infof("kill Qemu... %d", qc.process.Pid)
					if err := qc.process.Kill(); err == nil {
						success = true
					}
				} else {
					glog.Warning("no process to be killed")
				}
				hub <- &hypervisor.VmKilledEvent{Success: success}
				return
			}
		} else {
			glog.V(1).Info("chan closed, quit watch dog.")
			break
		}
	}
}

func (qc *QemuContext) watchPid(pid int, hub chan hypervisor.VmEvent) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	qc.process = proc
	go watchDog(qc, hub)

	return nil
}

// launchQemu run qemu and wait it's quit, includes
func launchQemu(qc *QemuContext, ctx *hypervisor.VmContext) {
	qemu := qc.driver.executable
	if qemu == "" {
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "can not find qemu executable"}
		return
	}

	args := qc.arguments(ctx)

	if glog.V(1) {
		glog.Info("cmdline arguments: ", strings.Join(args, " "))
	}

	cmd := exec.Command(qemu, append(args, "-daemonize", "-pidfile", qc.qemuPidFile, "-D", qc.qemuLogFile)...)

	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	if stdout.Len() != 0 {
		glog.Info(stdout.String())
	}
	if stderr.Len() != 0 {
		glog.Error(stderr.String())
	}
	if err != nil {
		//fail to daemonize
		glog.Errorf("%v", err)
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "try to start qemu failed"}
		return
	}

	var file *os.File
	t := time.NewTimer(time.Second * 5)
	// keep opening file until it exists or timeout
	for {
		select {
		case <-t.C:
			glog.Error("open pid file timeout")
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "pid file not exist, timeout"}
			return
		default:
		}

		if file, err = os.OpenFile(qc.qemuPidFile, os.O_RDONLY, 0640); err != nil {
			file.Close()
			if os.IsNotExist(err) {
				continue
			}
			glog.Errorf("open pid file failed: %v", err)
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "open pid file failed"}
			return
		}
		break
	}

	var pid uint32
	t = time.NewTimer(time.Second * 5)
	for {
		select {
		case <-t.C:
			glog.Error("read pid file timeout")
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "read pid file timeout"}
			return
		default:
		}

		file.Seek(0, os.SEEK_SET)
		if _, err := fmt.Fscan(file, &pid); err != nil {
			if err == io.EOF {
				continue
			}
			glog.Errorf("read pid file failed: %v", err)
			ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "read pid file failed"}
			return
		}
		break
	}

	file.Close()

	glog.V(1).Infof("starting daemon with pid: %d", pid)

	go watchLogFile(qc.qemuLogFile)

	err = ctx.DCtx.(*QemuContext).watchPid(int(pid), ctx.Hub)
	if err != nil {
		glog.Error("watch qemu process failed")
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "watch qemu process failed"}
		return
	}
}

func associateQemu(ctx *hypervisor.VmContext) {
	go watchDog(ctx.DCtx.(*QemuContext), ctx.Hub)
}
