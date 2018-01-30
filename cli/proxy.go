package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/golang/glog"
	"github.com/hashicorp/yamux"
	"github.com/hyperhq/runv/agent"
	"github.com/hyperhq/runv/agent/proxy"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/kardianos/osext"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

var proxyCommand = cli.Command{
	Name:     "proxy",
	Usage:    "[internal command] proxy hyperstart API into vm",
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
			Name:  "kata-yamux-sock",
			Usage: "the vm's kata yamux sock address to be connected",
		},
		cli.StringFlag{
			Name:  "proxy-hyperstart",
			Usage: "gprc sock address to be created for proxying hyperstart API",
		},
	},
	Before: func(context *cli.Context) error {
		return cmdPrepare(context, false, false)
	},
	Action: func(context *cli.Context) (err error) {
		if context.String("vmid") == "" || context.String("proxy-hyperstart") == "" {
			return fmt.Errorf("missing arguements")
		}

		if context.String("kata-yamux-sock") != "" {
			glog.Infof("kata-yamux proxy server")
			return kataYamuxServer(context.String("kata-yamux-sock"), context.String("proxy-hyperstart"))
		}
		if context.String("hyperstart-ctl-sock") == "" || context.String("hyperstart-stream-sock") == "" {
			return fmt.Errorf("missing arguements")
		}

		glog.Infof("agent.NewJsonBasedHyperstart")
		h, _ := agent.NewJsonBasedHyperstart(context.String("vmid"), context.String("hyperstart-ctl-sock"), context.String("hyperstart-stream-sock"), 1, true, false)

		var s *grpc.Server
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

func createProxy(context *cli.Context, VMID, ctlSock, streamSock, yamuxSock, grpcSock string) error {
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
	args = append(args, "proxy", "--vmid", VMID, "--proxy-hyperstart", grpcSock)
	if yamuxSock == "" {
		args = append(args, "--hyperstart-ctl-sock", ctlSock, "--hyperstart-stream-sock", streamSock)
	} else {
		args = append(args, "--kata-yamux-sock", yamuxSock)
	}
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

func serve(servConn io.ReadWriteCloser, proto, addr string, results chan error) error {
	session, err := yamux.Client(servConn, nil)
	if err != nil {
		return err
	}

	// serving connection
	l, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}

	go func() {
		var err error
		defer func() {
			l.Close()
			results <- err
		}()

		for {
			var conn, stream net.Conn
			conn, err = l.Accept()
			if err != nil {
				return
			}

			stream, err = session.Open()
			if err != nil {
				return
			}

			go proxyConn(conn, stream)
		}
	}()

	return nil
}

func proxyConn(conn1 net.Conn, conn2 net.Conn) {
	wg := &sync.WaitGroup{}
	once := &sync.Once{}
	cleanup := func() {
		conn1.Close()
		conn2.Close()
	}
	copyStream := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		if err != nil {
			once.Do(cleanup)
		}
		wg.Done()
	}

	wg.Add(2)
	go copyStream(conn1, conn2)
	go copyStream(conn2, conn1)
	go func() {
		wg.Wait()
		once.Do(cleanup)
	}()
}

func unixAddr(uri string) (string, error) {
	if len(uri) == 0 {
		return "", errors.New("empty uri")

	}
	addr, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if addr.Scheme != "" && addr.Scheme != "unix" {
		return "", errors.New("invalid address scheme")
	}
	return addr.Host + addr.Path, nil
}

func kataYamuxServer(channel, proxyAddr string) error {
	muxAddr, err := unixAddr(channel)
	if err != nil {
		glog.Error("invalid mux socket address")
		return err
	}
	listenAddr, err := unixAddr(proxyAddr)
	if err != nil {
		glog.Error("invalid listen socket address")
		return err
	}

	// yamux connection
	servConn, err := utils.UnixSocketConnect(muxAddr)
	if err != nil {
		glog.Errorf("failed to dial channel(%q): %s", muxAddr, err)
		return err
	}
	defer servConn.Close()

	results := make(chan error)
	err = serve(servConn, "unix", listenAddr, results)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	for err = range results {
		if err != nil {
			glog.Errorf(err.Error())
			return err
		}
	}
	return nil
}
