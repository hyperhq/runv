package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/hyperstart/libhyperstart"
	"github.com/hyperhq/runv/hyperstart/proxy"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/telnet"
	"github.com/hyperhq/runv/lib/term"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

var proxyCommand = cli.Command{
	Name:     "proxy",
	Usage:    "[internal command] proxy hyperstart API into vm and watch vm",
	HideHelp: true,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "vmid",
			Usage: "the vm name",
		},
		cli.StringFlag{
			Name:  "hyperstart-ctl-sock",
			Usage: "the vm's ctl sock address to be connected",
		},
		cli.StringFlag{
			Name:  "hyperstart-stream-sock",
			Usage: "the vm's stream sock address to be connected",
		},
		cli.StringFlag{
			Name:  "proxy-hyperstart",
			Usage: "gprc sock address to be created for proxying hyperstart API",
		},
		cli.StringFlag{
			Name:  "watch-vm-console",
			Usage: "vm's console sock address to connected(readonly)",
		},
		cli.BoolFlag{
			Name:  "watch-hyperstart",
			Usage: "ping hyperstart for every 60 seconds via Version() API",
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
		if context.String("vmid") == "" || context.String("hyperstart-ctl-sock") == "" ||
			context.String("hyperstart-stream-sock") == "" || context.String("proxy-hyperstart") == "" {
			return err
		}
		glog.Infof("libhyperstart.NewJsonBasedHyperstart")
		h, _ := libhyperstart.NewJsonBasedHyperstart(context.String("vmid"), context.String("hyperstart-ctl-sock"), context.String("hyperstart-stream-sock"), 1, false, false)

		var s *grpc.Server
		if context.Bool("watch-hyperstart") {
			glog.Infof("watchHyperstart")
			go func() {
				watchHyperstart(h)
				s.Stop()
			}()
		}
		if context.String("watch-vm-console") != "" {
			glog.Infof("watchConsole() sock: %s", context.String("watch-vm-console"))
			err = watchConsole(context.String("watch-vm-console"))
			if err != nil {
				glog.Errorf("watchConsole() failed, err: %#v", err)
				return err
			}
		}

		grpcSock := context.String("proxy-hyperstart")
		glog.Infof("proxy.NewServer")
		s, err = proxy.NewServer(grpcSock, h)
		if err != nil {
			glog.Errorf("proxy.NewServer() failed with err: %#v", err)
			return err
		}
		if _, err := os.Stat(grpcSock); !os.IsNotExist(err) {
			return fmt.Errorf("%s existed, someone may be in service", grpcSock)
		}
		glog.Infof("net.Listen() to grpcsock: %s", grpcSock)
		l, err := net.Listen("unix", grpcSock)
		if err != nil {
			glog.Errorf("net.Listen() failed with err: %#v", err)
			return err
		}

		glog.Infof("proxy: grpc api on %s", grpcSock)
		if err = s.Serve(l); err != nil {
			glog.Errorf("proxy serve grpc with error: %v", err)
		}

		return err
	},
}

func watchConsole(console string) error {
	var (
		br *bufio.Reader
	)

	if utils.IsUnixSocket(console) {
		conn, err := utils.UnixSocketConnect(console)
		if err != nil {
			return err
		}
		tc, err := telnet.NewConn(conn)
		if err != nil {
			return err
		}
		br = bufio.NewReader(tc)
	} else {
		//lkvm
		file, err := os.OpenFile(console, os.O_RDWR|syscall.O_NOCTTY, 0600)
		if err != nil {
			return err
		}

		_, err = term.SetRawTerminal(file.Fd())
		if err != nil {
			glog.Errorf("fail to set raw mode for %v: %v", console, err)
			return err
		}
		br = bufio.NewReader(file)
	}

	go func() {
		for {
			log, _, err := br.ReadLine()
			if err == io.EOF {
				break
			}
			if err != nil {
				glog.Errorf("read console %s failed: %v", console, err)
				return
			}
			if len(log) != 0 {
				glog.Info("vmconsole: ", string(log))
			}
		}
	}()

	return nil
}

func watchHyperstart(h libhyperstart.Hyperstart) error {
	next := time.NewTimer(10 * time.Second)
	timeout := time.AfterFunc(60*time.Second, func() {
		glog.Errorf("watch hyperstart timeout")
		h.Close()
	})
	defer next.Stop()
	defer timeout.Stop()

	for {
		glog.V(2).Infof("issue VERSION request for keep-alive test")
		_, err := h.APIVersion()
		if err != nil {
			h.Close()
			glog.Errorf("h.APIVersion() failed with %#v", err)
			return err
		}
		if !timeout.Stop() {
			<-timeout.C
		}
		<-next.C
		next.Reset(10 * time.Second)
		timeout.Reset(60 * time.Second)
	}
}

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
		"--hyperstart-stream-sock", streamSock, "--proxy-hyperstart", grpcSock,
		"--watch-vm-console", filepath.Join(hypervisor.BaseDir, VMID, hypervisor.ConsoleSockName),
		"--watch-hyperstart")
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
