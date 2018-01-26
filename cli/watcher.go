package cli

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
