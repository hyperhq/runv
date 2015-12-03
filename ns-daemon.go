package main

import (
	"flag"
	"os"
	"sync"

	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

// TODO: logging for daemon

// namespace context for runv daemon
type nsContext struct {
	lock        sync.Mutex
	podId       string
	vmId        string
	userPod     *pod.UserPod
	podStatus   *hypervisor.PodStatus
	vm          *hypervisor.Vm
	firstConfig *startConfig
	actives     map[string]*startConfig
	wg          sync.WaitGroup
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
	startVContainer(context, root, id)
	context.wg.Wait()
}
