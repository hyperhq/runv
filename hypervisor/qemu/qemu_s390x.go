// +build linux,s390x

package qemu

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/hyperhq/runv/hypervisor"
)

func InitDriver() *QemuDriver {
	cmd, err := exec.LookPath("qemu-system-s390x")
	if err != nil {
		return nil
	}

	return &QemuDriver{
		executable: cmd,
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
	qc.cpus = boot.CPU

	var memParams, cpuParams string
	if ctx.Boot.HotAddCpuMem {
		memParams = fmt.Sprintf("size=%d,slots=1,maxmem=%dM", ctx.Boot.Memory, hypervisor.DefaultMaxMem) // TODO set maxmem to the total memory of the system
		cpuParams = fmt.Sprintf("cpus=%d,maxcpus=%d", ctx.Boot.CPU, hypervisor.DefaultMaxCpus)           // TODO set it to the cpus of the system
	} else {
		memParams = strconv.Itoa(ctx.Boot.Memory)
		cpuParams = strconv.Itoa(ctx.Boot.CPU)
	}

	return []string{
		"-machine", "s390-ccw-virtio,accel=kvm,usb=off", "-cpu", "host",
		"-kernel", boot.Kernel, "-initrd", boot.Initrd,
		"-append", "\"console=ttyS1 panic=1 no_timer_check\"",
		"-realtime", "mlock=off", "-no-user-config", "-nodefaults", "-enable-kvm",
		"-rtc", "base=utc,driftfix=slew", "-no-reboot", "-display", "none", "-boot", "strict=on",
		"-m", memParams, "-smp", cpuParams,
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qc.qmpSockName),
		"-chardev", fmt.Sprintf("socket,id=charconsole0,path=%s,server,nowait", ctx.ConsoleSockName),
		"-device", "sclpconsole,chardev=charconsole0",
		"-device", "virtio-serial-ccw,id=virtio-serial0",
		"-device", "virtio-scsi-ccw,id=scsi0",
		"-chardev", fmt.Sprintf("socket,id=charch0,path=%s,server,nowait", ctx.HyperSockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=1,chardev=charch0,id=channel0,name=sh.hyper.channel.0",
		"-chardev", fmt.Sprintf("socket,id=charch1,path=%s,server,nowait", ctx.TtySockName),
		"-device", "virtserialport,bus=virtio-serial0.0,nr=2,chardev=charch1,id=channel1,name=sh.hyper.channel.1",
		"-fsdev", fmt.Sprintf("local,id=virtio9p,path=%s,security_model=none", ctx.ShareDir),
		"-device", fmt.Sprintf("virtio-9p-ccw,fsdev=virtio9p,mount_tag=%s", hypervisor.ShareDirTag),
	}

}
