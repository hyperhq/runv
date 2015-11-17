package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"

	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/lib/utils"

	"github.com/opencontainers/specs"
)

const (
	_ = iota
	RUNV_INITCONTAINER
	RUNV_STARTCONTAINER
	RUNV_ACK
	RUNV_EXECCMD
	RUNV_WINSIZE
)

const shortLen = 12

func TruncateID(id string) string {
	trimTo := shortLen
	if len(id) < shortLen {
		trimTo = len(id)
	}
	return id[:trimTo]
}

// GenerateRandomID returns an unique id
func GenerateRandomID() string {
	for {
		id := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, id); err != nil {
			panic(err) // This shouldn't happen
		}
		value := hex.EncodeToString(id)
		// if we try to parse the truncated for as an int and we don't have
		// an error then the value is all numberic and causes issues when
		// used as a hostname. ref #3869
		if _, err := strconv.ParseInt(TruncateID(value), 10, 64); err == nil {
			continue
		}
		return value
	}
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

func execPrestartHooks(rt *specs.RuntimeSpec, state *specs.State) error {
	for _, hook := range rt.Hooks.Prestart {
		err := execHook(hook, state)
		if err != nil {
			return err
		}
	}

	return nil
}

func execPoststartHooks(rt *specs.RuntimeSpec, state *specs.State) error {
	for _, hook := range rt.Hooks.Poststart {
		err := execHook(hook, state)
		if err != nil {
			fmt.Printf("exec Poststart hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func execPoststopHooks(rt *specs.RuntimeSpec, state *specs.State) error {
	for _, hook := range rt.Hooks.Poststop {
		err := execHook(hook, state)
		if err != nil {
			fmt.Printf("exec Poststop hook %s failed %s", hook.Path, err.Error())
		}
	}

	return nil
}

func prepareInfo(config *startConfig, c *pod.UserContainer, vmId string) (*hypervisor.ContainerInfo, error) {
	var root string
	var err error

	containerId := GenerateRandomID()
	sharedDir := path.Join(hypervisor.BaseDir, vmId, hypervisor.ShareDirTag)
	rootDir := path.Join(sharedDir, containerId)
	os.MkdirAll(rootDir, 0755)

	rootDir = path.Join(rootDir, "rootfs")

	if !filepath.IsAbs(c.Image) {
		root = path.Join(config.BundlePath, c.Image)
	} else {
		root = c.Image
	}

	err = utils.Mount(root, rootDir)
	if err != nil {
		fmt.Printf("mount %s to %s failed: %s\n", root, rootDir, err.Error())
		return nil, err
	}

	containerInfo := &hypervisor.ContainerInfo{
		Id:     containerId,
		Rootfs: "rootfs",
		Image:  containerId,
		Fstype: "dir",
	}

	return containerInfo, nil
}

func startVm(config *startConfig, userPod *pod.UserPod, vmId string) (*hypervisor.Vm, error) {
	var (
		err error
		cpu = 1
		mem = 128
	)

	vbox := config.Vbox
	if _, err = os.Stat(vbox); err == nil {
		vbox, err = filepath.Abs(vbox)
		if err != nil {
			fmt.Printf("Cannot get abs path for vbox: %s\n", err.Error())
			return nil, err
		}
	}

	if config.Driver == "vbox" {
		if _, err = os.Stat(config.Vbox); err != nil {
			return nil, err
		}

	} else {
		if _, err = os.Stat(config.Kernel); err != nil {
			return nil, err
		}

		if _, err = os.Stat(config.Initrd); err != nil {
			return nil, err
		}
	}

	if userPod.Resource.Vcpu > 0 {
		cpu = userPod.Resource.Vcpu
	}

	if userPod.Resource.Memory > 0 {
		mem = userPod.Resource.Memory
	}

	b := &hypervisor.BootConfig{
		Kernel: config.Kernel,
		Initrd: config.Initrd,
		Bios:   "",
		Cbfs:   "",
		Vbox:   config.Vbox,
		CPU:    cpu,
		Memory: mem,
	}

	vm := hypervisor.NewVm(vmId, cpu, mem, false, types.VM_KEEP_NONE)
	err = vm.Launch(b)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return nil, err
	}

	return vm, nil
}

func startRunvPod(context *nsContext, config *startConfig) (err error) {
	context.lock.Lock()
	defer context.lock.Unlock()
	if context.firstConfig == nil {
		context.firstConfig = config
	} else {
		// check stopped
		if len(context.actives) == 0 {
			return fmt.Errorf("The namespace service was stopped")
		}
		// check match
		if config.Root != "" && config.Root != context.firstConfig.Root {
			return fmt.Errorf("The root is not match")
		}
		if config.Driver != "" && config.Driver != context.firstConfig.Driver {
			return fmt.Errorf("The driver is not match")
		}
		if config.Kernel != "" && config.Kernel != context.firstConfig.Kernel {
			return fmt.Errorf("The kernel is not match")
		}
		if config.Initrd != "" && config.Initrd != context.firstConfig.Initrd {
			return fmt.Errorf("The initrd is not match")
		}
		if config.Vbox != "" && config.Vbox != context.firstConfig.Vbox {
			return fmt.Errorf("The vbox is not match")
		}
		// check shared namespace
		for _, ns := range config.LinuxRuntimeSpec.Linux.Namespaces {
			if ns.Path == "" {
				continue
			}
			_, ok := context.actives[ns.Path]
			if !ok {
				return fmt.Errorf("Cann't share namespace with: %s", ns.Path)
			}
		}
		// OK, the pod has been started, add this config and return
		context.actives[config.Name] = config
		return nil
	}

	hypervisor.InterfaceCount = 0

	driver := config.Driver
	if hypervisor.HDriver, err = driverloader.Probe(driver); err != nil {
		fmt.Printf("%s\n", err.Error())
		return err
	}

	context.podId = fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	context.vmId = fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))

	context.userPod = pod.ConvertOCF2PureUserPod(&config.LinuxSpec, &config.LinuxRuntimeSpec)
	context.podStatus = hypervisor.NewPod(context.podId, context.userPod)
	context.vm, err = startVm(config, context.userPod, context.vmId)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return err
	}

	Response := context.vm.StartPod(context.podStatus, context.userPod, nil, nil)
	if Response.Data == nil {
		fmt.Printf("StartPod fail: QEMU response data is nil\n")
		return fmt.Errorf("StartPod fail")
	}
	fmt.Printf("result: code %d %s\n", Response.Code, Response.Cause)

	context.actives[config.Name] = config
	return nil
}

func cleanupRunvPod(context *nsContext, name string) {
	context.lock.Lock()
	defer context.lock.Unlock()
	delete(context.actives, name)
	if len(context.actives) > 0 {
		return
	}

	Response := context.vm.StopPod(context.podStatus, "yes")

	if Response.Data == nil {
		fmt.Printf("StopPod fail: QEMU response data is nil\n")
		return
	}
	fmt.Printf("result: code %d %s\n", Response.Code, Response.Cause)
	os.RemoveAll(path.Join(hypervisor.BaseDir, context.vmId))
}

func startVContainer(context *nsContext, root, container string) {
	// create stateDir
	stateDir := path.Join(root, container)
	_, err := os.Stat(stateDir)
	if err == nil {
		fmt.Printf("Container %s exists\n", container)
		return
	}
	err = os.MkdirAll(stateDir, 0644)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}
	defer os.RemoveAll(stateDir)

	// create connection sock
	sock, err := net.Listen("unix", path.Join(stateDir, "runv.sock"))
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}
	defer sock.Close()
	conn, err := sock.Accept()
	if err != nil {
		fmt.Printf("accept on runv Socket err: %v\n", err)
		return
	}

	// get config from sock
	msg, err := hypervisor.ReadVmMessage(conn.(*net.UnixConn))
	if err != nil || msg.Code != RUNV_STARTCONTAINER {
		fmt.Printf("read runv client data failed: %v\n", err)
		return
	}
	config := &startConfig{}
	err = json.Unmarshal(msg.Message, config)
	if err != nil || config.Root != root || config.Name != container {
		fmt.Printf("parse runv start config failed: %v\n", err)
		return
	}

	// start pure pod
	err = startRunvPod(context, config)
	if err != nil {
		fmt.Printf("Start Pod failed: %s\n", err.Error())
		return
	}
	defer cleanupRunvPod(context, container)

	// save the state
	state := &specs.State{
		Version:    config.LinuxSpec.Spec.Version,
		ID:         container,
		Pid:        -1,
		BundlePath: config.BundlePath,
	}
	stateData, err := json.MarshalIndent(state, "", "\t")
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}
	stateFile := path.Join(stateDir, "state.json")
	err = ioutil.WriteFile(stateFile, stateData, 0644)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	userContainer := pod.ConvertOCF2UserContainer(&config.LinuxSpec, &config.LinuxRuntimeSpec)
	info, err := prepareInfo(config, userContainer, context.vmId)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	defer func() {
		rootDir := path.Join(hypervisor.BaseDir, context.vmId, hypervisor.ShareDirTag, info.Id, "rootfs")
		utils.Umount(rootDir)
		os.RemoveAll(path.Join(hypervisor.BaseDir, context.vmId, hypervisor.ShareDirTag, info.Id))
	}()

	tag, _ := runvAllocAndRespondTag(conn)
	ttyCallback := make(chan *types.VmResponse, 1)
	err = context.vm.Attach(conn, conn, tag, info.Id, ttyCallback, nil)
	if err != nil {
		fmt.Printf("StartPod fail: fail to set up tty connection.\n")
		return
	}

	err = execPrestartHooks(&config.LinuxRuntimeSpec.RuntimeSpec, state)
	if err != nil {
		fmt.Printf("execute Prestart hooks failed, %s\n", err.Error())
		return
	}

	context.podStatus.AddContainer(info.Id, context.podId, "", []string{}, types.S_POD_CREATED)
	context.vm.NewContainer(userContainer, info)
	ListenAndHandleRunvRequests(context, info, sock)

	err = execPoststartHooks(&config.LinuxRuntimeSpec.RuntimeSpec, state)
	if err != nil {
		fmt.Printf("execute Poststart hooks failed %s\n", err.Error())
	}

	<-ttyCallback

	err = execPoststopHooks(&config.LinuxRuntimeSpec.RuntimeSpec, state)
	if err != nil {
		fmt.Printf("execute Poststop hooks failed %s\n", err.Error())
		return
	}
}

func runvAllocAndRespondTag(conn net.Conn) (tag string, err error) {
	tag = pod.RandStr(8, "alphanum")
	m := &hypervisor.DecodedMessage{
		Code:    RUNV_ACK,
		Message: []byte(tag),
	}
	data := hypervisor.NewVmMessage(m)
	conn.Write(data)

	return tag, nil
}

func runvGetTag(conn net.Conn) (tag string, err error) {
	msg, err := hypervisor.ReadVmMessage(conn.(*net.UnixConn))
	if err != nil {
		fmt.Printf("read runv server data failed: %v\n", err)
		return "", err
	}

	if msg.Code != RUNV_ACK {
		return "", fmt.Errorf("unexpected respond code")
	}

	return string(msg.Message), nil
}

func HandleRunvRequest(context *nsContext, info *hypervisor.ContainerInfo, conn net.Conn) {
	defer context.wg.Done()
	defer conn.Close()

	msg, err := hypervisor.ReadVmMessage(conn.(*net.UnixConn))
	if err != nil {
		fmt.Printf("read runv client data failed: %v\n", err)
		return
	}

	switch msg.Code {
	case RUNV_INITCONTAINER:
		{
			initCmd := &initContainerCmd{}
			err = json.Unmarshal(msg.Message, initCmd)
			if err != nil {
				fmt.Printf("parse runv init container command failed: %v\n", err)
				return
			}
			startVContainer(context, initCmd.Root, initCmd.Name)
		}
	case RUNV_EXECCMD:
		{
			tag, _ := runvAllocAndRespondTag(conn)

			fmt.Printf("client exec cmd request %s\n", msg.Message[:])
			err = context.vm.Exec(conn, conn, string(msg.Message[:]), tag, info.Id)

			if err != nil {
				fmt.Printf("read runv client data failed: %v\n", err)
			}
		}
	case RUNV_WINSIZE:
		{
			var winSize ttyWinSize
			json.Unmarshal(msg.Message, &winSize)
			//fmt.Printf("client exec winsize request %v\n", winSize)
			context.vm.Tty(winSize.Tag, winSize.Height, winSize.Width)
		}
	default:
		fmt.Printf("unknown cient request\n")
	}
}

func ListenAndHandleRunvRequests(context *nsContext, info *hypervisor.ContainerInfo, sock net.Listener) {
	context.wg.Add(1)
	go func() {
		defer context.wg.Done()

		for {
			conn, err := sock.Accept()
			if err != nil {
				fmt.Printf("accept on runv Socket err: %v\n", err)
				break
			}

			context.wg.Add(1)
			go HandleRunvRequest(context, info, conn)
		}
	}()
}
