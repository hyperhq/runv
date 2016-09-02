// +build linux,ppc64le

package qemu

import (
	"fmt"
	"strconv"

	"github.com/hyperhq/runv/hypervisor"
)

const (
	QEMU_SYSTEM_EXE = "qemu-system-ppc64le"
)

func (qc *QemuContext) arguments(ctx *hypervisor.VmContext) []string {
	if ctx.Boot == nil {
		ctx.Boot = &hypervisor.BootConfig{
			CPU:    1,
			Memory: 256, // The minimum requirement for running a VM on PowerPC LE arch is 256M
			Kernel: hypervisor.DefaultKernel,
			Initrd: hypervisor.DefaultInitrd,
		}
	}
	boot := ctx.Boot
	qc.cpus = boot.CPU

	var memParams, cpuParams string
	if ctx.Boot.HotAddCpuMem {
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", ctx.Boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", ctx.Boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		memParams = strconv.Itoa(ctx.Boot.Memory)
		cpuParams = strconv.Itoa(ctx.Boot.CPU)
	}
	memParams = "256"
	cpuParams = "1"

	return []string{
		"-machine", "pseries,accel=kvm,usb=off", "-global", "kvm-pit.lost_tick_policy=discard", "-cpu", "host",
		"-kernel", boot.Kernel, "-initrd", boot.Initrd,
		"-machine", "pseries,accel=kvm,usb=off", "-cpu", "host",
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults",
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
	}
}
