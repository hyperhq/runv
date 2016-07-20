package supervisor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
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

func (c *Container) run(p *Process) {
	go func() {
		err := c.start(p)
		if err != nil {
			c.ownerPod.sv.reap(c.Id, p.Id)
			return
		}
		e := Event{
			ID:        c.Id,
			Type:      EventContainerStart,
			Timestamp: time.Now(),
		}
		c.ownerPod.sv.Events.notifySubscribers(e)

		err = c.wait(p)
		e = Event{
			ID:        c.Id,
			Type:      EventExit,
			Timestamp: time.Now(),
			PID:       p.Id,
			Status:    -1,
		}
		if err == nil {
			if cs := c.ownerPod.podStatus.GetContainer(c.Id); cs != nil {
				e.Status = int(cs.ExitCode)
			}
		}
		c.ownerPod.sv.Events.notifySubscribers(e)
	}()
}

func (c *Container) start(p *Process) error {
	// save the state
	glog.V(3).Infof("save state id %s, boundle %s", c.Id, c.BundlePath)
	stateDir := filepath.Join(c.ownerPod.sv.StateDir, c.Id)
	_, err := os.Stat(stateDir)
	if err == nil {
		glog.V(1).Infof("Container %s exists\n", c.Id)
		return fmt.Errorf("Container %s exists\n", c.Id)
	}
	err = os.MkdirAll(stateDir, 0644)
	if err != nil {
		glog.V(1).Infof("%s\n", err.Error())
		return err
	}

	state := &specs.State{
		Version:    c.Spec.Version,
		ID:         c.Id,
		Pid:        c.ownerPod.getNsPid(),
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

	// Mount rootfs
	err = utils.Mount(u.Image, vmRootfs)
	if err != nil {
		glog.Errorf("mount %s to %s failed: %s\n", u.Image, vmRootfs, err.Error())
		return err
	}

	// Pre-create dirs necessary for hyperstart before setting rootfs readonly
	// TODO: a better way to support readonly rootfs
	if err = preCreateDirs(u.Image); err != nil {
		return err
	}

	// Mount necessary files and directories from spec
	for _, m := range c.Spec.Mounts {
		if err := mountToRootfs(&m, vmRootfs, ""); err != nil {
			return fmt.Errorf("mounting %q to rootfs %q at %q failed: %v", m.Source, vmRootfs, m.Destination, err)
		}
	}

	// set rootfs readonly
	if c.Spec.Root.Readonly {
		err = utils.SetReadonly(vmRootfs)
		if err != nil {
			glog.Errorf("set rootfs %s readonly failed: %s\n", vmRootfs, err.Error())
			return err
		}
	}

	envs := make(map[string]string)
	for _, env := range u.Envs {
		envs[env.Env] = env.Value
	}

	_ = &hypervisor.ContainerInfo{
		Id:     c.Id,
		Rootfs: "rootfs",
		Image:  pod.UserVolume{Source: c.Id},
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

	err = c.ownerPod.initPodNetwork(c)
	if err != nil {
		glog.Errorf("fail to initialize pod network %v", err)
		return err
	}

	c.ownerPod.podStatus.AddContainer(c.Id, c.ownerPod.podStatus.Id, "", []string{}, types.S_POD_CREATED)
	//Todo: vm.AddContainer here
	return c.ownerPod.vm.StartContainer(c.Id)
}

func (c *Container) wait(p *Process) error {
	state := &specs.State{
		Version:    c.Spec.Version,
		ID:         c.Id,
		Pid:        -1,
		BundlePath: c.BundlePath,
	}

	err := execPoststartHooks(c.Spec, state)
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
		c.ownerPod.podStatus.AddExec(c.Id, processId, "", spec.Terminal)
		err := c.ownerPod.vm.AddProcess(c.Id, processId, spec.Terminal, spec.Args, spec.Env, spec.Cwd, p.stdio)
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
			Status:    -1,
		}
		if err != nil {
			glog.V(1).Infof("get exit code failed %s\n", err.Error())
		} else {
			if es := c.ownerPod.podStatus.GetExec(processId); es != nil {
				e.Status = int(es.ExitCode)
			}
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

func mountToRootfs(m *specs.Mount, rootfs, mountLabel string) error {
	// TODO: we don't use mountLabel here because it looks like mountLabel is
	// only significant when SELinux is enabled.
	var (
		dest = m.Destination
	)
	if !strings.HasPrefix(dest, rootfs) {
		dest = filepath.Join(rootfs, dest)
	}

	switch m.Type {
	case "proc", "sysfs", "mqueue", "tmpfs", "cgroup", "devpts":
		glog.V(3).Infof("Skip mount point %q of type %s", m.Destination, m.Type)
		return nil
	case "bind":
		stat, err := os.Stat(m.Source)
		if err != nil {
			// error out if the source of a bind mount does not exist as we will be
			// unable to bind anything to it.
			return err
		}
		// ensure that the destination of the bind mount is resolved of symlinks at mount time because
		// any previous mounts can invalidate the next mount's destination.
		// this can happen when a user specifies mounts within other mounts to cause breakouts or other
		// evil stuff to try to escape the container's rootfs.
		if dest, err = symlink.FollowSymlinkInScope(filepath.Join(rootfs, m.Destination), rootfs); err != nil {
			return err
		}
		if err := checkMountDestination(rootfs, dest); err != nil {
			return err
		}
		// update the mount with the correct dest after symlinks are resolved.
		m.Destination = dest
		if err := createIfNotExists(dest, stat.IsDir()); err != nil {
			return err
		}
		if err := mount.Mount(m.Source, dest, m.Type, strings.Join(m.Options, ",")); err != nil {
			return err
		}
	default:
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
		return mount.Mount(m.Source, dest, m.Type, strings.Join(m.Options, ","))
	}
	return nil
}

// checkMountDestination checks to ensure that the mount destination is not over the top of /proc.
// dest is required to be an abs path and have any symlinks resolved before calling this function.
func checkMountDestination(rootfs, dest string) error {
	invalidDestinations := []string{
		"/proc",
	}
	// White list, it should be sub directories of invalid destinations
	validDestinations := []string{
		// These entries can be bind mounted by files emulated by fuse,
		// so commands like top, free displays stats in container.
		"/proc/cpuinfo",
		"/proc/diskstats",
		"/proc/meminfo",
		"/proc/stat",
		"/proc/net/dev",
	}
	for _, valid := range validDestinations {
		path, err := filepath.Rel(filepath.Join(rootfs, valid), dest)
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
	}
	for _, invalid := range invalidDestinations {
		path, err := filepath.Rel(filepath.Join(rootfs, invalid), dest)
		if err != nil {
			return err
		}
		if path == "." || !strings.HasPrefix(path, "..") {
			return fmt.Errorf("%q cannot be mounted because it is located inside %q", dest, invalid)
		}

	}
	return nil
}

// preCreateDirs creates necessary dirs for hyperstart
func preCreateDirs(rootfs string) error {
	dirs := []string{
		"proc",
		"sys",
		"dev",
		"lib/modules",
	}
	for _, dir := range dirs {
		err := createIfNotExists(filepath.Join(rootfs, dir), true)
		if err != nil {
			return err
		}
	}
	return nil
}

// createIfNotExists creates a file or a directory only if it does not already exist.
func createIfNotExists(path string, isDir bool) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if isDir {
				return os.MkdirAll(path, 0755)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			f.Close()
		}
	}
	return nil
}
