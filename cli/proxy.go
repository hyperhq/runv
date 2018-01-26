package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/golang/glog"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
)

func createProxy(context *cli.Context, VMID, ctlSock, streamSock, grpcSock string) error {
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
	args = append(args, "proxy", "--vmid", VMID, "--hyperstart-ctl-sock", ctlSock,
		"--hyperstart-stream-sock", streamSock, "--proxy-hyperstart", grpcSock)
	cmd = &exec.Cmd{
		Path: path,
		Args: args,
		Dir:  "/",
		SysProcAttr: &syscall.SysProcAttr{
			Setsid: true,
		},
	}

	glog.V(2).Infof("start proxy with argument: %v", args)
	err = cmd.Start()
	if err != nil {
		glog.Errorf("createProxy failed with err %#v", err)
		return err
	}
	glog.V(2).Infof("createProxy succeeded with proxy pid: %d", cmd.Process.Pid)

	return nil
}
