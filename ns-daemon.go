package main

import (
	"sync"

	"github.com/codegangsta/cli"
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

var daemonCommand = cli.Command{
	Name:  "daemon",
	Usage: "internal and hidden daemon command, run in daemon mode",
	Action: func(cliContext *cli.Context) {
		context := &nsContext{}
		context.actives = make(map[string]*startConfig)
		startVContainer(context, cliContext.GlobalString("root"), cliContext.GlobalString("id"))
		context.wg.Wait()
	},
	HideHelp: true,
}
