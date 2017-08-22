package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/api"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/linuxsignal"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func startContainer(vm *hypervisor.Vm, root, container string, spec *specs.Spec, state *State) error {
	err := vm.StartContainer(container)
	if err != nil {
		glog.V(1).Infof("Start Container fail: fail to start container with err: %#v", err)
		return err
	}

	err = syscall.Kill(state.Pid, syscall.SIGUSR1)
	if err != nil {
		glog.V(1).Infof("failed to notify the shim to work", err.Error())
		return err
	}

	glog.V(3).Infof("change the status of container %s to `running`", container)
	state.Status = "running"
	state.ContainerCreateTime = time.Now().UTC().Unix()
	if err = saveStateFile(root, container, state); err != nil {
		return err
	}

	var pl *ProcessList
	if pl, err = NewProcessList(root, container); err != nil {
		return err
	}
	defer pl.Release()

	// No need to load, container init process must be the first
	var p []Process
	cmd := strings.Join(spec.Process.Args, " ")
	p = append(p, Process{Id: "init", Pid: state.Pid, CMD: cmd, CreateTime: state.ShimCreateTime})
	if err = pl.Save(p); err != nil {
		return err
	}

	err = execPoststartHooks(spec, state)
	if err != nil {
		glog.V(1).Infof("execute Poststart hooks failed %s", err.Error())
	}

	return err
}

func createContainer(options runvOptions, vm *hypervisor.Vm, container, bundle, stateRoot string, spec *specs.Spec) (shim *os.Process, err error) {
	if err = setupContainerFs(vm, bundle, container, spec); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			removeContainerFs(sandboxPath(vm), container)
		}
	}()

	glog.V(3).Infof("vm.AddContainer()")
	config := api.ContainerDescriptionFromOCF(container, spec)
	r := vm.AddContainer(config)
	if !r.IsSuccess() {
		return nil, fmt.Errorf("add container %s failed: %s", container, r.Message())
	}
	defer func() {
		if err != nil {
			vm.RemoveContainer(container)
		}
	}()

	// Prepare container state directory
	stateDir := filepath.Join(stateRoot, container)
	_, err = os.Stat(stateDir)
	if err == nil {
		glog.Errorf("Container %s exists", container)
		return nil, fmt.Errorf("Container %s exists", container)
	}
	err = os.MkdirAll(stateDir, 0644)
	if err != nil {
		glog.V(1).Infof("%s", err.Error())
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stateDir)
		}
	}()

	// Create sandbox dir symbol link in container root dir
	vmRootLinkPath := filepath.Join(stateDir, "sandbox")
	vmRootPath := sandboxPath(vm)
	if err = os.Symlink(vmRootPath, vmRootLinkPath); err != nil {
		return nil, fmt.Errorf("failed to create symbol link %q: %v", vmRootLinkPath, err)
	}

	// create shim and save the state
	shim, err = createShim(options, container, "init", &spec.Process)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			shim.Kill()
		}
	}()

	var stat system.Stat_t
	stat, err = system.Stat(shim.Pid)
	if err != nil {
		return nil, err
	}

	state := &State{
		State: specs.State{
			Version: spec.Version,
			ID:      container,
			Status:  "created",
			Pid:     shim.Pid,
			Bundle:  bundle,
		},
		ShimCreateTime: stat.StartTime,
	}
	glog.V(3).Infof("save state id %s, boundle %s", container, bundle)
	if err = saveStateFile(stateRoot, container, state); err != nil {
		return nil, err
	}

	err = execPrestartHooks(spec, state)
	if err != nil {
		glog.V(1).Infof("execute Prestart hooks failed, %s", err.Error())
		return nil, err
	}

	// If runv is launched via docker/containerd, we start netlistener to watch/collect network changes.
	// TODO: if runv is launched by cni compatible tools, the cni script can use `runv cni` cmdline to update the network.
	// Create the listener process which will enters into the netns of the shim
	options.withContainer = state
	if err = startNsListener(options, vm); err != nil {
		glog.Errorf("start ns listener fail: %v", err)
		return nil, err
	}

	return shim, nil
}

func deleteContainer(vm *hypervisor.Vm, root, container string, force bool, spec *specs.Spec, state *State) error {
	// non-force killing can only be performed when at least one of the realProcess and shimProcess exited
	exitedVM := true
	for _, c := range vm.ContainerList() {
		if c == container {
			exitedVM = vm.SignalProcess(container, "init", syscall.Signal(0)) != nil // todo: is this check reliable?
			break
		}
	}
	exitedHost := !shimProcessAlive(state.Pid, state.ShimCreateTime)
	if !exitedVM && !exitedHost && !force {
		// don't perform deleting
		return fmt.Errorf("the container %s is still alive, use -f to force kill it?", container)
	}

	if !exitedVM { // force kill the real init process inside the vm
		for i := 0; i < 100; i++ {
			vm.SignalProcess(container, "init", linuxsignal.SIGKILL)
			time.Sleep(100 * time.Millisecond)
			if vm.SignalProcess(container, "init", syscall.Signal(0)) != nil {
				break
			}
		}
	}
	vm.RemoveContainer(container)

	return deleteContainerHost(root, container, spec, state)
}

func deleteContainerHost(root, container string, spec *specs.Spec, state *State) error {
	if shimProcessAlive(state.Pid, state.ShimCreateTime) { // force kill the shim process in the host
		time.Sleep(200 * time.Millisecond) // the shim might be going to exit, wait it
		for i := 0; i < 100; i++ {
			syscall.Kill(state.Pid, syscall.SIGKILL)
			time.Sleep(100 * time.Millisecond)
			if !shimProcessAlive(state.Pid, state.ShimCreateTime) {
				break
			}
		}
	}

	err := execPoststopHooks(spec, state)
	if err != nil {
		glog.V(1).Infof("execute Poststop hooks failed %s", err.Error())
		removeContainerFs(filepath.Join(root, container, "sandbox"), container)
		os.RemoveAll(filepath.Join(root, container))
		return err // return err of the hooks
	}

	removeContainerFs(filepath.Join(root, container, "sandbox"), container)
	return os.RemoveAll(filepath.Join(root, container))
}

func addProcess(options runvOptions, vm *hypervisor.Vm, container, process string, spec *specs.Process) (shim *os.Process, err error) {
	p := &api.Process{
		Container: container,
		Id:        process,
		Terminal:  spec.Terminal,
		Args:      spec.Args,
		Envs:      spec.Env,
		Workdir:   spec.Cwd,
	}
	if spec.User.UID != 0 {
		p.User = strconv.FormatUint(uint64(spec.User.UID), 10)
	}
	if spec.User.GID != 0 {
		p.Group = strconv.FormatUint(uint64(spec.User.GID), 10)
	}
	if len(spec.User.AdditionalGids) > 0 {
		ag := []string{}
		for _, g := range spec.User.AdditionalGids {
			ag = append(ag, strconv.FormatUint(uint64(g), 10))
		}
	}
	err = vm.AddProcess(p, nil)

	if err != nil {
		glog.V(1).Infof("add process to container failed: %v", err)
		return nil, err
	}
	defer func() {
		if err != nil {
			vm.SignalProcess(container, process, linuxsignal.SIGKILL)
		}
	}()

	shim, err = createShim(options, container, process, spec)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			shim.Kill()
		}
	}()

	var stat system.Stat_t
	stat, err = system.Stat(shim.Pid)
	if err != nil {
		return nil, err
	}

	var pl *ProcessList
	if pl, err = NewProcessList(options.GlobalString("root"), container); err != nil {
		return nil, err
	}
	defer pl.Release()
	cmd := strings.Join(spec.Args, " ")
	err = pl.Add(Process{Id: process, Pid: shim.Pid, CMD: cmd, CreateTime: stat.StartTime})
	if err != nil {
		return nil, err
	}

	return shim, nil
}

func execHook(hook specs.Hook, state *State) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	cmd := exec.Cmd{
		Path:  hook.Path,
		Args:  hook.Args,
		Env:   hook.Env,
		Stdin: bytes.NewReader(b),
	}
	return cmd.Run()
}

func execPrestartHooks(rt *specs.Spec, state *State) error {
	if rt.Hooks == nil {
		return nil
	}
	for _, hook := range rt.Hooks.Prestart {
		err := execHook(hook, state)
		if err != nil {
			return err
		}
	}

	return nil
}

func execPoststartHooks(rt *specs.Spec, state *State) error {
	if rt.Hooks == nil {
		return nil
	}
	for _, hook := range rt.Hooks.Poststart {
		err := execHook(hook, state)
		if err != nil {
			glog.V(1).Infof("exec Poststart hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func execPoststopHooks(rt *specs.Spec, state *State) error {
	if rt.Hooks == nil {
		return nil
	}
	for _, hook := range rt.Hooks.Poststop {
		err := execHook(hook, state)
		if err != nil {
			glog.V(1).Infof("exec Poststop hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func shimProcessAlive(pid int, createTime uint64) bool {
	stat, err := system.Stat(pid)
	return err == nil && stat.StartTime == createTime && stat.State != system.Zombie && stat.State != system.Dead
}
