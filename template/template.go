package template

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
)

// The TemplateVm will be booted, paused, saved, and killed. The TemplateVm
// is not existed any more but just the states left. The states includes two
// parts, the memory is StatePath/memory and devices states
// is StatePath/state
//
// New Vm can be booted from the saved TemplateVm states with all the initial
// memory is shared(copy-on-write) with the TemplateVm(templatePath/vmName/memory)
//
// Phoenix rising from the ashes

type TemplateVmState struct {
	StatePath string `json:"statepath"`
	Cpu       int    `json:"cpu"`
	Memory    int    `json:"memory"`
	Kernel    string `json:"kernel"`
	Initrd    string `json:"initrd"`
}

func CreateTemplateVM(templatePath, vmName string, cpu, mem int, kernel, initrd string) (t *TemplateVmState, err error) {
	statePath := templatePath + "/" + vmName
	defer func() {
		if err != nil {
			(&TemplateVmState{StatePath: statePath}).Destroy()
		}
	}()

	// prepare statePath
	if err := os.MkdirAll(statePath, 0700); err != nil {
		glog.Infof("create template state path failed: %v", err)
		return nil, err
	}
	flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
	opts := fmt.Sprintf("size=%dM", mem+8)
	if err = syscall.Mount("tmpfs", statePath, "tmpfs", flags, opts); err != nil {
		glog.Infof("mount template state path failed: %v", err)
		return nil, err
	}
	if f, err := os.Create(statePath + "/memory"); err != nil {
		glog.Infof("create memory path failed: %v", err)
		return nil, err
	} else {
		f.Close()
	}

	// launch vm
	b := &hypervisor.BootConfig{
		CPU:              cpu,
		Memory:           mem,
		HotAddCpuMem:     true,
		BootToBeTemplate: true,
		BootFromTemplate: false,
		MemoryPath:       statePath + "/memory",
		Kernel:           kernel,
		Initrd:           initrd,
	}
	vm, err := hypervisor.GetVm(vmName, b, true, false, 0)
	if err != nil {
		return nil, err
	}
	defer vm.Kill()

	// pasue and save devices state
	if err = vm.Pause(true); err != nil {
		glog.Infof("failed to pause template vm:%v", err)
		return nil, err
	}
	if err = vm.Save(statePath + "/state"); err != nil {
		glog.Infof("failed to save template vm states: %v", err)
		return nil, err
	}

	// TODO: qemu driver's qmp doesn't wait migration finish.
	// so we wait here. We should fix it in the qemu driver side.
	time.Sleep(1 * time.Second)

	return &TemplateVmState{StatePath: statePath, Cpu: cpu, Memory: mem, Kernel: kernel, Initrd: initrd}, nil
}

func (t *TemplateVmState) BootConfigFromTemplate() *hypervisor.BootConfig {
	return &hypervisor.BootConfig{
		CPU:              t.Cpu,
		Memory:           t.Memory,
		HotAddCpuMem:     true,
		BootToBeTemplate: false,
		BootFromTemplate: true,
		MemoryPath:       t.StatePath + "/memory",
		DevicesStatePath: t.StatePath + "/state",
		Kernel:           t.Kernel,
		Initrd:           t.Initrd,
	}
}

// boot vm from template, the returned vm is paused
func (t *TemplateVmState) NewVmFromTemplate(vmName string) (*hypervisor.Vm, error) {
	return hypervisor.GetVm(vmName, t.BootConfigFromTemplate(), true, false, 0)
}

func (t *TemplateVmState) Destroy() {
	for i := 0; i < 5; i++ {
		err := syscall.Unmount(t.StatePath, 0)
		if err != nil {
			glog.Infof("Failed to unmount the template state path: %v", err)
		} else {
			break
		}
		time.Sleep(time.Second) // TODO: only sleep&retry when unmount() returns EBUSY
	}
	err := os.Remove(t.StatePath)
	if err != nil {
		glog.Infof("Failed to remove the template state path: %v", err)
	}
}
