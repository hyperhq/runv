package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/factory"
	singlefactory "github.com/hyperhq/runv/factory/single"
	templatefactory "github.com/hyperhq/runv/factory/template"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/hypervisor"
	templatecore "github.com/hyperhq/runv/template"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
)

func setupFactory(context *cli.Context, spec *specs.Spec) (factory.Factory, error) {
	kernel, initrd, bios, cbfs, err := getKernelFiles(context, spec)
	if err != nil {
		return nil, fmt.Errorf("can't find kernel/initrd/bios/cbfs files")
	}
	glog.V(3).Infof("Using kernel: %s; Initrd: %s; bios: %s; cbfs: %s;", kernel, initrd, bios, cbfs)

	driver := context.GlobalString("driver")
	vsock := context.GlobalBool("vsock")
	template := context.GlobalString("template")

	var tconfig *templatecore.TemplateVmConfig
	if template != "" {
		path := filepath.Join(template, "config.json")
		f, err := os.Open(path)
		if err != nil {
			err = fmt.Errorf("open template JSON configuration file failed: %v", err)
			glog.Error(err)
			return nil, err
		}
		if err := json.NewDecoder(f).Decode(&tconfig); err != nil {
			err = fmt.Errorf("parse template JSON configuration file failed: %v", err)
			glog.Error(err)
			f.Close()
			return nil, err
		}
		f.Close()

		if (driver != "" && driver != tconfig.Driver) ||
			(kernel != "" && kernel != tconfig.Config.Kernel) ||
			(initrd != "" && initrd != tconfig.Config.Initrd) ||
			(bios != "" && bios != tconfig.Config.Bios) ||
			(cbfs != "" && cbfs != tconfig.Config.Cbfs) {
			glog.Warningf("template config is not match the driver, kernel, initrd, bios or cbfs argument, disable template")
			template = ""
		} else if driver == "" {
			driver = tconfig.Driver
		}
	} else if (bios == "" || cbfs == "") && (kernel == "" || initrd == "") {
		err := fmt.Errorf("argument kernel+initrd or bios+cbfs must be set")
		glog.Error(err)
		return nil, err
	}

	if template != "" {
		return singlefactory.New(templatefactory.NewFromExisted(tconfig)), nil
	}
	bootConfig := hypervisor.BootConfig{
		Kernel:      kernel,
		Initrd:      initrd,
		Bios:        bios,
		Cbfs:        cbfs,
		EnableVsock: vsock,
	}
	return singlefactory.Dummy(bootConfig), nil
}

func createAndLockSandBox(f factory.Factory, spec *specs.Spec, cpu int, mem int) (*hypervisor.Vm, *os.File, error) {
	if spec.Linux != nil && spec.Linux.Resources != nil && spec.Linux.Resources.Memory != nil && spec.Linux.Resources.Memory.Limit != nil {
		mem = int(*spec.Linux.Resources.Memory.Limit >> 20)
	}

	vm, err := f.GetVm(cpu, mem)
	if err != nil {
		glog.Errorf("Create VM failed with err: %v", err)
		return nil, nil, err
	}

	r := make(chan api.Result, 1)
	go func() {
		r <- vm.WaitInit()
	}()

	sandbox := api.SandboxInfoFromOCF(spec)
	vm.InitSandbox(sandbox)

	rsp := <-r

	if !rsp.IsSuccess() {
		vm.Kill()
		glog.Errorf("StartPod fail, response: %#v", rsp)
		return nil, nil, fmt.Errorf("StartPod fail")
	}
	glog.V(3).Infof("%s init sandbox successfully", rsp.ResultId())

	lockFile, err := lockSandbox(sandboxPath(vm))
	if err != nil {
		vm.Kill()
		return nil, nil, err
	}

	return vm, lockFile, nil
}

func lockAndAssociateSandbox(sandboxPath string) (*hypervisor.Vm, *os.File, error) {
	sandboxIDPath, err := os.Readlink(sandboxPath)
	if err != nil {
		return nil, nil, err
	}

	lockFile, err := lockSandbox(sandboxPath)
	if err != nil {
		return nil, nil, err
	}

	pinfoPath := filepath.Join(sandboxIDPath, "persist.json")
	data, err := ioutil.ReadFile(pinfoPath)
	if err != nil {
		unlockSandbox(lockFile)
		return nil, nil, err
	}
	sandboxID := filepath.Base(sandboxIDPath)
	vm, err := hypervisor.AssociateVm(sandboxID, data)
	if err != nil {
		unlockSandbox(lockFile)
		return nil, nil, err
	}
	return vm, lockFile, nil
}

func destroySandbox(vm *hypervisor.Vm, lockFile *os.File) {
	result := make(chan api.Result, 1)
	go func() {
		result <- vm.Shutdown()
	}()
	select {
	case rsp, ok := <-result:
		if !ok || !rsp.IsSuccess() {
			glog.Errorf("StopPod fail: chan: %v, response: %v", ok, rsp)
			break
		}
		glog.V(1).Infof("StopPod successfully")
	case <-time.After(time.Second * 60):
		glog.Errorf("StopPod timeout")
	}
	vm.Kill()

	// cli refactor todo: kill the proxy if vm.Shutdown() failed.

	unlockSandbox(lockFile)

	if err := os.RemoveAll(sandboxPath(vm)); err != nil {
		glog.Errorf("can't remove vm dir %q: %v", filepath.Join(hypervisor.BaseDir, vm.Id), err)
	}
	glog.Flush()
}

func releaseAndUnlockSandbox(vm *hypervisor.Vm, lockFile *os.File) error {
	data, err := vm.Dump()
	if err != nil {
		unlockSandbox(lockFile)
		return err
	}
	err = vm.ReleaseVm()
	if err != nil {
		unlockSandbox(lockFile)
		return err
	}
	pinfoPath := filepath.Join(sandboxPath(vm), "persist.json")
	err = ioutil.WriteFile(pinfoPath, data, 0644)
	if err != nil {
		unlockSandbox(lockFile)
		return err
	}

	unlockSandbox(lockFile)
	return nil
}

var getSandbox = lockAndAssociateSandbox

func putSandbox(vm *hypervisor.Vm, lockFile *os.File) {
	if len(vm.ContainerList()) > 0 {
		err := releaseAndUnlockSandbox(vm, lockFile)
		if err == nil {
			return
		}
		// fallthrough: can't recover, destory the whole sandbox
	}
	destroySandbox(vm, lockFile)
}

func sandboxPath(vm *hypervisor.Vm) string {
	return filepath.Join(hypervisor.BaseDir, vm.Id)
}

func setupHyperstartFunc(context *cli.Context) {
	libhyperstart.NewHyperstart = func(vmid, ctlSock, streamSock string, lastStreamSeq uint64, waitReady, paused bool) (libhyperstart.Hyperstart, error) {
		return newHyperstart(context, vmid, ctlSock, streamSock)
	}
}

func newHyperstart(context *cli.Context, vmid, ctlSock, streamSock string) (libhyperstart.Hyperstart, error) {
	grpcSock := filepath.Join(hypervisor.BaseDir, vmid, "hyperstartgrpc.sock")

	glog.Infof("newHyperstart() on socket: %s", grpcSock)
	if st, err := os.Stat(grpcSock); err != nil {
		glog.V(2).Infof("grpcSock stat: %#v, err: %#v", st, err)
		if !os.IsNotExist(err) {
			glog.Errorf("%s existed with wrong stats", grpcSock)
			return nil, fmt.Errorf("%s existed with wrong stats", grpcSock)
		}
		err = createProxy(context, vmid, ctlSock, streamSock, grpcSock)
		if err != nil {
			return nil, err
		}

		for i := 0; i < 500; i++ {
			if _, err := os.Stat(grpcSock); !os.IsNotExist(err) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	h, err := libhyperstart.NewGrpcBasedHyperstart(grpcSock)
	if err != nil {
		glog.Errorf("libhyperstart.NewGrpcBasedHyperstart() failed with err: %#v", err)
	}
	return h, err
}

// lock locks the sandbox to prevent it from being accessed by other processes.
func lockSandbox(sandboxPath string) (*os.File, error) {
	lockFilePath := filepath.Join(sandboxPath, "sandbox.lock")

	lockFile, err := os.OpenFile(lockFilePath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}

	return lockFile, nil
}

// unlock unlocks the sandbox to allow it being accessed by other processes.
func unlockSandbox(lockFile *os.File) error {
	if lockFile == nil {
		return fmt.Errorf("lockFile cannot be empty")
	}

	err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	if err != nil {
		return err
	}

	lockFile.Close()

	return nil
}
