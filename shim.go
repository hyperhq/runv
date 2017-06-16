package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/golang/glog"
	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/urfave/cli"
	"golang.org/x/net/context"
)

var shimCommand = cli.Command{
	Name:  "shim",
	Usage: "internal command for proxy changes to the container/process",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "container",
		},
		cli.StringFlag{
			Name: "process",
		},
		cli.BoolFlag{
			Name: "proxy-exit-code",
		},
		cli.BoolFlag{
			Name: "proxy-signal",
		},
		cli.BoolFlag{
			Name: "proxy-winsize",
		},
	},
	Action: func(context *cli.Context) error {
		root := context.GlobalString("root")
		container := context.String("container")
		process := context.String("process")
		c, err := getClient(filepath.Join(root, container, "namespace", "namespaced.sock"))
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("failed to get client: %v", err), -1)
		}
		exitcode := -1
		if context.Bool("proxy-exit-code") {
			glog.V(3).Infof("using shim to proxy exit code")
			defer func() { os.Exit(exitcode) }()
		}

		if context.Bool("proxy-winsize") {
			glog.V(3).Infof("using shim to proxy winsize")
			s, err := term.SetRawTerminal(os.Stdin.Fd())
			if err != nil {
				return cli.NewExitError(fmt.Sprintf("failed to set raw terminal: %v", err), -1)
			}
			defer term.RestoreTerminal(os.Stdin.Fd(), s)
			monitorTtySize(c, container, process)
		}

		if context.Bool("proxy-signal") {
			// TODO
			glog.V(3).Infof("using shim to proxy signal")
			sigc := forwardAllSignals(c, container, process)
			defer signal.Stop(sigc)
		}

		// wait until exit
		evChan := containerEvents(c, container)
		for e := range evChan {
			if e.Type == "exit" && e.Pid == process {
				exitcode = int(e.Status)
				break
			}
		}
		return nil
	},
}

func forwardAllSignals(c types.APIClient, cid, process string) chan os.Signal {
	sigc := make(chan os.Signal, 2048)
	// handle all signals for the process.
	signal.Notify(sigc)
	signal.Ignore(syscall.SIGCHLD, syscall.SIGPIPE, syscall.SIGWINCH)

	go func() {
		for s := range sigc {
			if s == syscall.SIGCHLD || s == syscall.SIGPIPE || s == syscall.SIGWINCH {
				//ignore these
				continue
			}
			// forward this signal to containerd
			sysSig, ok := s.(syscall.Signal)
			if !ok {
				err := fmt.Errorf("can't forward unknown signal %q", s.String())
				fmt.Fprintf(os.Stderr, "%v", err)
				glog.Errorf("%v", err)
				continue
			}
			if _, err := c.Signal(context.Background(), &types.SignalRequest{
				Id:     cid,
				Pid:    process,
				Signal: uint32(sysSig),
			}); err != nil {
				err = fmt.Errorf("forward signal %q failed: %v", s.String(), err)
				fmt.Fprintf(os.Stderr, "%v", err)
				glog.Errorf("%v", err)
			}
		}
	}()
	return sigc
}
