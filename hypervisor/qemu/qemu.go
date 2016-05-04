package qemu

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/types"
)

//implement the hypervisor.HypervisorDriver interface
type QemuDriver struct {
	executable string
}

//implement the hypervisor.DriverContext interface
type QemuContext struct {
	driver      *QemuDriver
	qmp         chan QmpInteraction
	waitQmp     chan int
	wdt         chan string
	qmpSockName string
	cpus        int
	process     *os.Process
}

func qemuContext(ctx *hypervisor.VmContext) *QemuContext {
	return ctx.DCtx.(*QemuContext)
}

func InitDriver() *QemuDriver {
	cmd, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return nil
	}

	return &QemuDriver{
		executable: cmd,
	}
}

func (qd *QemuDriver) InitContext(homeDir string) hypervisor.DriverContext {
	return &QemuContext{
		driver:      qd,
		qmp:         make(chan QmpInteraction, 128),
		wdt:         make(chan string, 16),
		qmpSockName: homeDir + QmpSockName,
		process:     nil,
	}
}

func (qd *QemuDriver) LoadContext(persisted map[string]interface{}) (hypervisor.DriverContext, error) {
	if t, ok := persisted["hypervisor"]; !ok || t != "qemu" {
		return nil, errors.New("wrong driver type in persist info")
	}

	var sock string
	var proc *os.Process = nil
	var err error

	s, ok := persisted["qmpSock"]
	if !ok {
		return nil, errors.New("cannot read the qmp socket info from persist info")
	} else {
		switch s.(type) {
		case string:
			sock = s.(string)
		default:
			return nil, errors.New("wrong sock name type in persist info")
		}
	}

	p, ok := persisted["pid"]
	if !ok {
		return nil, errors.New("cannot read the pid info from persist info")
	} else {
		switch p.(type) {
		case float64:
			proc, err = os.FindProcess(int(p.(float64)))
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("wrong pid field type in persist info")
		}
	}

	return &QemuContext{
		driver:      qd,
		qmp:         make(chan QmpInteraction, 128),
		wdt:         make(chan string, 16),
		waitQmp:     make(chan int, 1),
		qmpSockName: sock,
		process:     proc,
	}, nil
}

func (qc *QemuContext) Launch(ctx *hypervisor.VmContext) {
	go launchQemu(qc, ctx)
	go qmpHandler(ctx)
}

func (qc *QemuContext) Associate(ctx *hypervisor.VmContext) {
	go associateQemu(ctx)
	go qmpHandler(ctx)
}

func (qc *QemuContext) Dump() (map[string]interface{}, error) {
	if qc.process == nil {
		return nil, errors.New("can not serialize qemu context: no process running")
	}

	return map[string]interface{}{
		"hypervisor": "qemu",
		"qmpSock":    qc.qmpSockName,
		"pid":        qc.process.Pid,
	}, nil
}

func (qc *QemuContext) Shutdown(ctx *hypervisor.VmContext) {
	qmpQemuQuit(ctx, qc)
}

func (qc *QemuContext) Kill(ctx *hypervisor.VmContext) {
	defer func() {
		err := recover()
		if glog.V(1) && err != nil {
			glog.Info("kill qemu, but channel has already been closed")
		}
	}()
	qc.wdt <- "kill"
}

func (qc *QemuContext) Stats(ctx *hypervisor.VmContext) (*types.PodStats, error) {
	return nil, nil
}

func (qc *QemuContext) Close() {
	qc.wdt <- "quit"
	_ = <-qc.waitQmp
	close(qc.waitQmp)
	close(qc.qmp)
	close(qc.wdt)
}

func (qc *QemuContext) Pause(ctx *hypervisor.VmContext, cmd *hypervisor.PauseCommand) {
	commands := make([]*QmpCommand, 1)

	if cmd.Pause {
		commands[0] = &QmpCommand{
			Execute: "stop",
		}
	} else {
		commands[0] = &QmpCommand{
			Execute: "cont",
		}
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: func(err error) {
			cause := ""
			if err != nil {
				cause = err.Error()
			}

			ctx.Hub <- &hypervisor.PauseResult{Cause: cause, Reply: cmd}
		},
	}

}

func (qc *QemuContext) AddDisk(ctx *hypervisor.VmContext, sourceType string, blockInfo *hypervisor.BlockDescriptor) {
	name := blockInfo.Name
	filename := blockInfo.Filename
	format := blockInfo.Format
	id := blockInfo.ScsiId

	if format == "rbd" {
		if blockInfo.Options != nil {
			keyring := blockInfo.Options["keyring"]
			user := blockInfo.Options["user"]
			if keyring != "" && user != "" {
				filename += ":id=" + user + ":key=" + keyring
			}

			monitors := blockInfo.Options["monitors"]
			for i, m := range strings.Split(monitors, ";") {
				monitor := strings.Replace(m, ":", "\\:", -1)
				if i == 0 {
					filename += ":mon_host=" + monitor
					continue
				}
				filename += ";" + monitor
			}
		}
	}

	newDiskAddSession(ctx, qc, name, sourceType, filename, format, id)
}

func (qc *QemuContext) RemoveDisk(ctx *hypervisor.VmContext, blockInfo *hypervisor.BlockDescriptor, callback hypervisor.VmEvent) {
	id := blockInfo.ScsiId

	newDiskDelSession(ctx, qc, id, callback)
}

func (qc *QemuContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
	newNetworkAddSession(ctx, qc, host.Fd, guest.Device, host.Mac, guest.Index, guest.Busaddr)
}

func (qc *QemuContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent) {
	newNetworkDelSession(ctx, qc, n.DeviceName, callback)
}

func (qc *QemuContext) SetCpus(ctx *hypervisor.VmContext, cpus int, result chan<- error) {
	currcpus := qc.cpus

	if cpus < currcpus {
		result <- fmt.Errorf("can't reduce cpus number from %d to %d", currcpus, cpus)
		return
	} else if cpus == currcpus {
		result <- nil
		return
	}

	commands := make([]*QmpCommand, cpus-currcpus)
	for id := currcpus; id < cpus; id++ {
		commands[id-currcpus] = &QmpCommand{
			Execute: "cpu-add",
			Arguments: map[string]interface{}{
				"id": id,
			},
		}
	}

	qc.qmp <- &QmpSession{
		commands: commands,
		respond: func(err error) {
			if err == nil {
				qc.cpus = cpus
			}
			result <- err
		},
	}
}

func (qc *QemuContext) AddMem(ctx *hypervisor.VmContext, slot, size int, result chan<- error) {
	commands := make([]*QmpCommand, 2)
	commands[0] = &QmpCommand{
		Execute: "object-add",
		Arguments: map[string]interface{}{
			"qom-type": "memory-backend-ram",
			"id":       "mem" + strconv.Itoa(slot),
			"props":    map[string]interface{}{"size": int64(size) << 20},
		},
	}
	commands[1] = &QmpCommand{
		Execute: "device_add",
		Arguments: map[string]interface{}{
			"driver": "pc-dimm",
			"id":     "dimm" + strconv.Itoa(slot),
			"memdev": "mem" + strconv.Itoa(slot),
		},
	}
	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  func(err error) { result <- err },
	}
}

func (qc *QemuContext) Save(ctx *hypervisor.VmContext, path string, result chan<- error) {
	commands := make([]*QmpCommand, 1)

	commands[0] = &QmpCommand{
		Execute: "migrate",
		Arguments: map[string]interface{}{
			"uri": fmt.Sprintf("exec:cat>%s", path),
		},
	}

	// TODO: use query-migrate to query until completed
	qc.qmp <- &QmpSession{
		commands: commands,
		respond:  func(err error) { result <- err },
	}
}

func (qc *QemuDriver) SupportLazyMode() bool {
	return false
}

func (qc *QemuContext) arguments(ctx *hypervisor.VmContext) []string {
	if ctx.Boot == nil {
		ctx.Boot = &hypervisor.BootConfig{
			CPU:    1,
			Memory: 128,
			Kernel: hypervisor.DefaultKernel,
			Initrd: hypervisor.DefaultInitrd,
		}
	}
	boot := ctx.Boot
	qc.cpus = boot.CPU

	var machineClass, memParams, cpuParams string
	if ctx.Boot.HotAddCpuMem {
		machineClass = "pc-i440fx-2.1"
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", ctx.Boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", ctx.Boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		machineClass = "pc-i440fx-2.0"
		memParams = strconv.Itoa(ctx.Boot.Memory)
		cpuParams = strconv.Itoa(ctx.Boot.CPU)
	}

	params := []string{
		"-machine", machineClass + ",accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host"}
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		glog.V(1).Info("kvm not exist change to no kvm mode")
		params = []string{"-machine", machineClass + ",usb=off", "-cpu", "core2duo"}
	}

	if boot.Bios != "" && boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Bios),
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else if boot.Bios != "" {
		params = append(params,
			"-bios", boot.Bios,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "console=ttyS0 panic=1 no_timer_check")
	} else if boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else {
		params = append(params,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "console=ttyS0 panic=1 no_timer_check")
	}

	return append(params,
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-no-hpet",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams,
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.ConsoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	)
}
