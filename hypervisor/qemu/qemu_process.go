package qemu

import (
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/glog"
	"github.com/hyperhq/runv/lib/utils"

	"encoding/binary"
	"os"
	"strings"
	"syscall"
)

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

	pipe := make([]int, 2)
	err := syscall.Pipe(pipe)
	if err != nil {
		glog.Error("fail to create pipe")
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "fail to create pipe"}
		return
	}
	err = utils.ExecInDaemon(qemu, append([]string{"qemu-system-x86_64"}, args...), pipe[1])
	if err != nil {
		//fail to daemonize
		glog.Error("try to start qemu failed")
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "try to start qemu failed"}
		return
	}

	buf := make([]byte, 4)
	nr, err := syscall.Read(pipe[0], buf)
	if err != nil || nr != 4 {
		glog.Error("try to start qemu failed")
		ctx.Hub <- &hypervisor.VmStartFailEvent{Message: "try to start qemu failed"}
		return
	}
	syscall.Close(pipe[1])
	syscall.Close(pipe[0])

	pid := binary.BigEndian.Uint32(buf[:nr])
	glog.V(1).Infof("starting daemon with pid: %d", pid)

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
