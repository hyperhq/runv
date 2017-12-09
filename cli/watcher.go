package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
)

var watcherCommand = cli.Command{
	Name:     "watcher",
	Usage:    "[internal command] watch to see if it is work well",
	HideHelp: true,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "watch-vm-console",
			Usage: "vm's console sock address to connected(readonly)",
		},
		cli.StringFlag{
			Name:  "console-proto",
			Usage: "vm's console sock address to connected(readonly)",
			Value: hypervisor.CONSOLE_PROTO_TELNET,
		},
		cli.BoolFlag{
			Name:  "watch-hyperstart",
			Usage: "ping the agent for every 60 seconds via agent API",
		},
		cli.BoolFlag{
			Name:  "watch-vm",
			Usage: "todo: to be implemented",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) (err error) {
		ch := make(chan error, 1)

		if context.Bool("watch-hyperstart") {
			glog.Infof("watchHyperstart")
			// todo
		}
		if context.String("watch-vm-console") != "" {
			glog.Infof("watchConsole() sock: %s", context.String("watch-vm-console"))
			go func() {
				err := hypervisor.WatchConsole(context.String("console-proto"), context.String("watch-vm-console"))
				if err != nil {
					glog.Errorf("watchConsole() failed, err: %#v", err)
				}
				ch <- err
			}()
		}

		err = <-ch
		return err
	},
}

func createWatcher(context *cli.Context, VMID string) error {
	path, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("cannot find self executable path for %s: %v", os.Args[0], err)
	}

	var cmd *exec.Cmd
	args := []string{
		"runv", "--root", context.GlobalString("root"),
	}
	if context.GlobalBool("debug") {
		args = append(args, "--debug")
	}
	if context.GlobalString("log_dir") != "" {
		args = append(args, "--log_dir", context.GlobalString("log_dir"))
	}
	args = append(args, "watcher",
		"--watch-vm-console", filepath.Join(hypervisor.BaseDir, VMID, hypervisor.ConsoleSockName),
		"--console-proto", hypervisor.GetConsoleProto(), "--watch-hyperstart", "--watch-vm")
	cmd = &exec.Cmd{
		Path: path,
		Args: args,
		Dir:  "/",
		SysProcAttr: &syscall.SysProcAttr{
			Setsid: true,
		},
	}

	glog.V(2).Infof("start watcher with argument: %v", args)
	err = cmd.Start()
	if err != nil {
		glog.Errorf("createWatcher failed with err %#v", err)
		return err
	}
	glog.V(2).Infof("createWatcher succeeded with watcher pid: %d", cmd.Process.Pid)

	return nil
}
