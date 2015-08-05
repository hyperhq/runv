package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
	"github.com/hyperhq/runv/hypervisor/types"
	"github.com/hyperhq/runv/driverloader"
	"github.com/hyperhq/runv/lib/term"
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

func resizeTty(vm *hypervisor.Vm, tag string, outFd uintptr, isTerminalOut bool) {
	height, width := getTtySize(outFd, isTerminalOut)
	if height == 0 && width == 0 {
		return
	}

	vm.Tty(tag, height, width)
}

func monitorTtySize(vm *hypervisor.Vm, tag string, outFd uintptr, isTerminalOut bool) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			resizeTty(vm, tag, outFd, isTerminalOut)
		}
	}()
}

func getTtySize(outFd uintptr, isTerminalOut bool) (int, int) {
	if !isTerminalOut {
		return 0, 0
	}
	ws, err := term.GetWinsize(outFd)
	if err != nil {
		fmt.Printf("Error getting size: %s", err.Error())
		if ws == nil {
			return 0, 0
		}
	}
	return int(ws.Height), int(ws.Width)
}

func main() {
	hypervisor.InterfaceCount = 0

	var containerInfoList []*hypervisor.ContainerInfo
	var roots []string
	var containerId string
	var err error

	ocffile := flag.String("config", "", "ocf configure file")
	kernel := flag.String("kernel", "", "hyper kernel")
	initrd := flag.String("initrd", "", "hyper initrd")
	vbox := flag.String("vbox", "", "vbox boot iso")
	driver := flag.String("driver", "", "hypervisor driver")

	flag.Parse()

	if *ocffile == "" {
		*ocffile = "config.json"
	}

	if _, err = os.Stat(*ocffile); os.IsNotExist(err) {
		fmt.Printf("Please specify ocffile or put config.json under current working directory\n")
		return
	}

	if *vbox == "" {
		*vbox = "./vbox.iso"
	}

	if *kernel == "" {
		*kernel = "./kernel"
	}

	if *initrd == "" {
		*initrd = "./initrd.img"
	}

	if *driver == "" {
		*driver = "kvm"
		fmt.Printf("Use default hypervisor KVM")
	}

	if hypervisor.HDriver, err = driverloader.Probe(*driver); err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	vmId := fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))

	ocfData, err := ioutil.ReadFile(*ocffile)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	fmt.Printf("spec: %s", string(ocfData))

	userPod, err := pod.OCFConvert2Pod(ocfData)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	mypod := hypervisor.NewPod(podId, userPod)

	var (
		cpu = 1
		mem = 128
	)

	if userPod.Resource.Vcpu > 0 {
		cpu = userPod.Resource.Vcpu
	}

	if userPod.Resource.Memory > 0 {
		mem = userPod.Resource.Memory
	}

	b := &hypervisor.BootConfig {
		Kernel:	*kernel,
		Initrd: *initrd,
		Bios:	"",
		Cbfs:	"",
		Vbox:	*vbox,
		CPU:	cpu,
		Memory:	mem,
	}

	vm := hypervisor.NewVm(vmId, cpu, mem, false)
	err = vm.Launch(b)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	sharedDir := path.Join(hypervisor.BaseDir, vm.Id, hypervisor.ShareDirTag)

	for _, c := range userPod.Containers {
		var root string
		var err error

		containerId = GenerateRandomID()
		fmt.Printf("containerID %s\n", containerId)
		rootDir := path.Join(sharedDir, containerId)
		os.MkdirAll(rootDir, 0755)

		rootDir = path.Join(rootDir, "rootfs")

		if !filepath.IsAbs(c.Image) {
			root, err = filepath.Abs(c.Image)
			if err != nil {
				fmt.Printf("%s\n", err.Error())
				return
			}
		} else {
			root = c.Image
		}

		fmt.Printf("mount %s to %s\n", root, rootDir)
		err = mount(root, rootDir)
		if err != nil {
			fmt.Printf("mount %s to %s failed: %s\n", root, rootDir, err.Error())
			return
		}
		roots = append(roots, rootDir)

		containerInfo := &hypervisor.ContainerInfo {
			Id:		containerId,
			Rootfs:		"rootfs",
			Image:		containerId,
			Fstype:		"dir",
		}

		containerInfoList = append(containerInfoList, containerInfo)
		mypod.AddContainer(containerId, podId, "", []string{}, types.S_POD_CREATED)
	}

	qemuResponse := vm.StartPod(mypod, userPod, containerInfoList, nil)
	if qemuResponse.Data == nil {
		fmt.Printf("StartPod fail: QEMU response data is nil\n")
		return
	}
	fmt.Printf("result: code %d %s\n", qemuResponse.Code, qemuResponse.Cause)

	inFd, _ := term.GetFdInfo(os.Stdin)
	outFd, isTerminalOut := term.GetFdInfo(os.Stdout)

	oldState, err := term.SetRawTerminal(inFd)
	if err != nil {
		return
	}

	height , width := getTtySize(outFd, isTerminalOut)
	winSize := &hypervisor.WindowSize {
		Row:	uint16(height),
		Column:	uint16(width),
	}

	tag := pod.RandStr(8, "alphanum")

	monitorTtySize(vm, tag, outFd, isTerminalOut)

	vm.Attach(os.Stdin, os.Stdout, tag, containerId, winSize)

	qemuResponse = vm.StopPod(mypod, "yes")

	term.RestoreTerminal(inFd, oldState)

	for _, root := range roots {
		umount(root)
	}

	if qemuResponse.Data == nil {
		fmt.Printf("StopPod fail: QEMU response data is nil\n")
		return
	}
	fmt.Printf("result: code %d %s\n", qemuResponse.Code, qemuResponse.Cause)
}
