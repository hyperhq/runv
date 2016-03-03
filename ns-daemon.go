package main

import (
	"flag"
	"net"
	"os"
	"sync"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

// namespace context for runv daemon
type nsContext struct {
	lock        sync.Mutex
	podId       string
	vmId        string
	userPod     *pod.UserPod
	podStatus   *hypervisor.PodStatus
	vm          *hypervisor.Vm
	firstConfig *startConfig
	ttyList     map[string]*hypervisor.TtyIO
	actives     map[string]*startConfig
	sockets     map[string]net.Listener
	wg          sync.WaitGroup
	sync.RWMutex
}

func runvNamespaceDaemon() {
	var root string
	var id string
	flag.StringVar(&root, "root", "", "")
	flag.StringVar(&id, "id", "", "")
	flag.Parse()
	if root == "" || id == "" {
		// should not happen in daemon
		os.Exit(-1)
	}

	context := &nsContext{}
	context.actives = make(map[string]*startConfig)
	context.sockets = make(map[string]net.Listener)
	context.ttyList = make(map[string]*hypervisor.TtyIO)

	startVContainer(context, root, id)
	context.wg.Wait()
}
