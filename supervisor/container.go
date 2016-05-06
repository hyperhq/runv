package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type Container struct {
	Id         string
	BundlePath string
	Spec       *specs.Spec
	Processes  map[string]*Process

	ownerPod *HyperPod
}

func (c *Container) start(p *Process) {
	e := Event{
		ID:        c.Id,
		Type:      EventContainerStart,
		Timestamp: time.Now(),
	}
	c.ownerPod.sv.Events.notifySubscribers(e)

	go func() {
		err := c.run(p)
		e := Event{
			ID:        c.Id,
			Type:      EventExit,
			Timestamp: time.Now(),
			PID:       p.Id,
			Status:    -1,
		}
		if err == nil {
			e.Status = int(p.stdio.ExitCode)
		}
		c.ownerPod.sv.Events.notifySubscribers(e)
	}()
}

func (c *Container) run(p *Process) error {
	// save the state
	glog.V(3).Infof("save state id %s, boundle %s", c.Id, c.BundlePath)
	stateDir := filepath.Join(c.ownerPod.sv.StateDir, c.Id)
	_, err := os.Stat(stateDir)
	if err == nil {
		glog.V(1).Infof("Container %s exists\n", c.Id)
		return err
	}
	err = os.MkdirAll(stateDir, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	state := &specs.State{
		Version:    c.Spec.Version,
		ID:         c.Id,
		Pid:        -1,
		BundlePath: c.BundlePath,
	}
	stateData, err := json.MarshalIndent(state, "", "\t")
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}
	stateFile := filepath.Join(stateDir, "state.json")
	err = ioutil.WriteFile(stateFile, stateData, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}

	glog.V(3).Infof("prepare hypervisor info")
	u := pod.ConvertOCF2UserContainer(c.Spec)
	if !filepath.IsAbs(u.Image) {
		u.Image = filepath.Join(c.BundlePath, u.Image)
	}
	vmRootfs := filepath.Join(hypervisor.BaseDir, c.ownerPod.vm.Id, hypervisor.ShareDirTag, c.Id, "rootfs")
	os.MkdirAll(vmRootfs, 0755)

	err = utils.Mount(u.Image, vmRootfs, c.Spec.Root.Readonly)
	if err != nil {
		glog.V(1).Infof("mount %s to %s failed: %s\n", u.Image, vmRootfs, err.Error())
		return err
	}

	envs := make(map[string]string)
	for _, env := range u.Envs {
		envs[env.Env] = env.Value
	}

	info := &hypervisor.ContainerInfo{
		Id:     c.Id,
		Rootfs: "rootfs",
		Image:  c.Id,
		Fstype: "dir",
		Cmd:    u.Command,
		Envs:   envs,
	}

	err = c.ownerPod.vm.Attach(p.stdio, c.Id, nil)
	if err != nil {
		glog.V(1).Infof("StartPod fail: fail to set up tty connection.\n")
		return err
	}

	err = execPrestartHooks(c.Spec, state)
	if err != nil {
		glog.V(1).Infof("execute Prestart hooks failed, %s\n", err.Error())
		return err
	}

	c.ownerPod.podStatus.AddContainer(c.Id, c.ownerPod.podStatus.Id, "", []string{}, types.S_POD_CREATED)
	c.ownerPod.vm.NewContainer(u, info)

	err = execPoststartHooks(c.Spec, state)
	if err != nil {
		glog.V(1).Infof("execute Poststart hooks failed %s\n", err.Error())
	}

	err = p.stdio.WaitForFinish()
	if err != nil {
		glog.V(1).Infof("get exit code failed %s\n", err.Error())
	}

	err = execPoststopHooks(c.Spec, state)
	if err != nil {
		glog.V(1).Infof("execute Poststop hooks failed %s\n", err.Error())
		return err
	}
	return nil
}

func (c *Container) addProcess(processId, stdin, stdout, stderr string, spec *specs.Process) (*Process, error) {
	if _, ok := c.ownerPod.Processes[processId]; ok {
		return nil, fmt.Errorf("conflict process ID")
	}
	if _, ok := c.Processes[processId]; ok {
		return nil, fmt.Errorf("conflict process ID")
	}
	if _, ok := c.Processes["init"]; !ok {
		return nil, fmt.Errorf("init process of the container %s had already exited", c.Id)
	}
	if processId == "init" { // test in case the init process is being reaped
		return nil, fmt.Errorf("conflict process ID")
	}

	p := &Process{
		Id:     processId,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Spec:   spec,
		ProcId: -1,

		inerId:    processId,
		ownerCont: c,
	}
	err := p.setupIO()
	if err != nil {
		return nil, err
	}

	c.ownerPod.Processes[processId] = p
	c.Processes[processId] = p

	e := Event{
		ID:        c.Id,
		Type:      EventProcessStart,
		Timestamp: time.Now(),
		PID:       processId,
	}
	c.ownerPod.sv.Events.notifySubscribers(e)

	go func() {
		err := c.ownerPod.vm.AddProcess(c.Id, spec.Terminal, spec.Args, spec.Env, spec.Cwd, p.stdio)
		if err != nil {
			glog.V(1).Infof("add process to container failed: %v\n", err)
		} else {
			err = p.stdio.WaitForFinish()
		}

		e := Event{
			ID:        c.Id,
			Type:      EventExit,
			Timestamp: time.Now(),
			PID:       processId,
		}
		if err != nil {
			e.Status = -1
			glog.V(1).Infof("get exit code failed %s\n", err.Error())
		} else {
			e.Status = int(p.stdio.ExitCode)
		}
		c.ownerPod.sv.Events.notifySubscribers(e)
	}()
	return p, nil
}

func execHook(hook specs.Hook, state *specs.State) error {
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

func execPrestartHooks(rt *specs.Spec, state *specs.State) error {
	for _, hook := range rt.Hooks.Prestart {
		err := execHook(hook, state)
		if err != nil {
			return err
		}
	}

	return nil
}

func execPoststartHooks(rt *specs.Spec, state *specs.State) error {
	for _, hook := range rt.Hooks.Poststart {
		err := execHook(hook, state)
		if err != nil {
			glog.V(1).Infof("exec Poststart hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func execPoststopHooks(rt *specs.Spec, state *specs.State) error {
	for _, hook := range rt.Hooks.Poststop {
		err := execHook(hook, state)
		if err != nil {
			glog.V(1).Infof("exec Poststop hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func (c *Container) reap() {
	containerSharedDir := filepath.Join(hypervisor.BaseDir, c.ownerPod.vm.Id, hypervisor.ShareDirTag, c.Id)
	utils.Umount(filepath.Join(containerSharedDir, "rootfs"))
	os.RemoveAll(containerSharedDir)
	os.RemoveAll(filepath.Join(c.ownerPod.sv.StateDir, c.Id))
}
