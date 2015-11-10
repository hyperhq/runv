package main

import (
	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/hypervisor/pod"
)

// TODO: logging for daemon

// namespace context for runv daemon
type nsContext struct {
	podId     string
	vmId      string
	userPod   *pod.UserPod
	podStatus *hypervisor.PodStatus
	vm        *hypervisor.Vm
}

var daemonCommand = cli.Command{
	Name:  "daemon",
	Usage: "internal and hidden daemon command, run in daemon mode",
	Action: func(cliContext *cli.Context) {
		context := &nsContext{}
		startVContainer(context, cliContext.GlobalString("root"), cliContext.GlobalString("id"))
	},
	HideHelp: true,
}
