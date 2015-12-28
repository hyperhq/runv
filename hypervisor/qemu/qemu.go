package qemu

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

var PersistentMemDisk bool

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
	process     *os.Process
	devicesArgs []string
	callbacks   []hypervisor.VmEvent
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
		case int:
			proc, err = os.FindProcess(p.(int))
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
	qmpQemuQuit(qc)
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

func (qc *QemuContext) Close() {
	qc.wdt <- "quit"
	_ = <-qc.waitQmp
	close(qc.waitQmp)
	close(qc.qmp)
	close(qc.wdt)
}

func (qc *QemuContext) AddDisk(ctx *hypervisor.VmContext, name, sourceType, filename, format string, id int) {
	newDiskAddSession(qc, name, sourceType, filename, format, id)
}

func (qc *QemuContext) RemoveDisk(ctx *hypervisor.VmContext, filename, format string, id int, callback hypervisor.VmEvent) {
	newDiskDelSession(qc, id, callback)
}

func (qc *QemuContext) AddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
	newNetworkAddSession(qc, host.Fd, guest.Device, host.Mac, guest.Index, guest.Busaddr)
}

func (qc *QemuContext) RemoveNic(ctx *hypervisor.VmContext, n *hypervisor.InterfaceCreated, callback hypervisor.VmEvent) {
	newNetworkDelSession(qc, n.DeviceName, callback)
}

func (qd *QemuDriver) SupportLazyMode() bool {
	return false
}

func (qd *QemuDriver) AsyncDiskBoot() bool {
	return !PersistentMemDisk
}

func (qd *QemuDriver) AsyncNicBoot() bool {
	return true
}

func (qc *QemuContext) InitVM(ctx *hypervisor.VmContext) error {
	return nil
}

func (qc *QemuContext) LazyAddDisk(ctx *hypervisor.VmContext, name, sourceType, filename, format string, id int) {
	qc.devicesArgs = append(qc.devicesArgs,
		"-drive", "file="+filename+",if=none,id="+"drive"+strconv.Itoa(id)+",format="+format+",cache=writeback",
		"-device", "scsi-hd,bus=scsi0.0,scsi-id="+strconv.Itoa(id)+",drive=drive"+strconv.Itoa(id)+",id=scsi-disk"+strconv.Itoa(id),
	)

	callback := &hypervisor.BlockdevInsertedEvent{
		Name:       name,
		SourceType: sourceType,
		DeviceName: scsiId2Name(id),
		ScsiId:     id,
	}
	qc.callbacks = append(qc.callbacks, callback)
}

func (qc *QemuContext) LazyAddNic(ctx *hypervisor.VmContext, host *hypervisor.HostNicInfo, guest *hypervisor.GuestNicInfo) {
}

func (qc *QemuContext) LazyLaunch(ctx *hypervisor.VmContext) {
	qc.Launch(ctx)

	for _, cb := range qc.callbacks {
		ctx.Hub <- cb
	}
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

	params := []string{
		"-machine", "pc-i440fx-2.0,accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host"}
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		glog.V(1).Info("kvm not exist change to no kvm mode")
		params = []string{"-machine", "pc-i440fx-2.0,usb=off", "-cpu", "core2duo"}
	}

	if boot.Bios != "" && boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Bios),
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else if boot.Bios != "" {
		params = append(params,
			"-bios", boot.Bios,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "\"console=ttyS0 panic=1 no_timer_check\"")
	} else if boot.Cbfs != "" {
		params = append(params,
			"-drive", fmt.Sprintf("if=pflash,file=%s,readonly=on", boot.Cbfs))
	} else {
		params = append(params,
			"-kernel", boot.Kernel, "-initrd", boot.Initrd, "-append", "\"console=ttyS0 panic=1 no_timer_check\"")
	}

	params = append(params,
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-no-hpet",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", strconv.Itoa(ctx.Boot.Memory), "-smp", strconv.Itoa(ctx.Boot.CPU),
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName), "-serial", fmt.Sprintf("unix:%s,server,nowait", ctx.ConsoleSockName),
		"-device", "virtio-serial-pci,id=virtio-serial0,bus=pci.0,addr=0x2", "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x3",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-pci,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	)

	return append(params, qc.devicesArgs...)
}
