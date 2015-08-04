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

	var containerId string
	var err error

	if hypervisor.HDriver, err = driverloader.Probe("kvm"); err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	ocifile := flag.String("config", "", "oci configure file")
	kernel := flag.String("kernel", "", "hyper kernel")
	initrd := flag.String("initrd", "", "hyper initrd")
	//bridge := flag.String("br", "", "bridge")
	//subnet := flag.String("ip", "", "subnet")

	flag.Parse()

	if *ocifile == "" {
		fmt.Printf("Please specify oci file\n")
		*ocifile = "config.json"
	}

	if *kernel == "" {
		*kernel = "./kernel"
		if _, err = os.Stat(*kernel); os.IsNotExist(err) {
			fmt.Printf("Please specify kernel or put kernel under current working directory\n")
			return
		}
	}

	*kernel, err = filepath.Abs(*kernel)
	if err != nil {
		fmt.Printf("Cannot get abs path for kernel: %s\n", err.Error())
		return
	}

	if *initrd == "" {
		*initrd = "./initrd.img"
		if _, err := os.Stat(*initrd); os.IsNotExist(err) {
			fmt.Printf("Please specify initrd or put initrd.img under current working directory\n")
			return
		}
	}

	*initrd, err = filepath.Abs(*initrd)
	if err != nil {
		fmt.Printf("Cannot get abs path for initrd.img: %s\n", err.Error())
		return
	}

/*
	err := hypervisor.InitNetwork(hypervisor.HDriver, *bridge, *subnet)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}
*/
	podId := fmt.Sprintf("pod-%s", pod.RandStr(10, "alpha"))
	vmId := fmt.Sprintf("vm-%s", pod.RandStr(10, "alpha"))

	_, err = os.Stat(*ocifile)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	ociData, err := ioutil.ReadFile(*ocifile)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	fmt.Printf("spec: %s", string(ociData))

	userPod, err := pod.OCFConvert2Pod(ociData)
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

	b := &hypervisor.BootConfig{
		CPU:    cpu,
		Memory: mem,
		Kernel: *kernel,
		Initrd: *initrd,
		Bios:   "",
		Cbfs:   "",
	}

	vm := hypervisor.NewVm(vmId, cpu, mem, false)
	err = vm.Launch(b)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return
	}

	sharedDir := path.Join(hypervisor.BaseDir, vm.Id, hypervisor.ShareDirTag)

	containerInfoList, roots := setupContainer(userPod, sharedDir)
	for _, cInfo := range(containerInfoList) {
		mypod.AddContainer(cInfo.Id, podId, "", []string{}, types.S_POD_CREATED)
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
	cleanupContainer(roots)
	if qemuResponse.Data == nil {
		fmt.Printf("StopPod fail: QEMU response data is nil\n")
		return
	}
	fmt.Printf("result: code %d %s\n", qemuResponse.Code, qemuResponse.Cause)
}
