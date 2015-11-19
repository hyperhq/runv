package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/hypervisor"
)

var signalMap = map[string]syscall.Signal{
	"ABRT":   syscall.SIGABRT,
	"ALRM":   syscall.SIGALRM,
	"BUS":    syscall.SIGBUS,
	"CHLD":   syscall.SIGCHLD,
	"CLD":    syscall.SIGCLD,
	"CONT":   syscall.SIGCONT,
	"FPE":    syscall.SIGFPE,
	"HUP":    syscall.SIGHUP,
	"ILL":    syscall.SIGILL,
	"INT":    syscall.SIGINT,
	"IO":     syscall.SIGIO,
	"IOT":    syscall.SIGIOT,
	"KILL":   syscall.SIGKILL,
	"PIPE":   syscall.SIGPIPE,
	"POLL":   syscall.SIGPOLL,
	"PROF":   syscall.SIGPROF,
	"PWR":    syscall.SIGPWR,
	"QUIT":   syscall.SIGQUIT,
	"SEGV":   syscall.SIGSEGV,
	"STKFLT": syscall.SIGSTKFLT,
	"STOP":   syscall.SIGSTOP,
	"SYS":    syscall.SIGSYS,
	"TERM":   syscall.SIGTERM,
	"TRAP":   syscall.SIGTRAP,
	"TSTP":   syscall.SIGTSTP,
	"TTIN":   syscall.SIGTTIN,
	"TTOU":   syscall.SIGTTOU,
	"UNUSED": syscall.SIGUNUSED,
	"URG":    syscall.SIGURG,
	"USR1":   syscall.SIGUSR1,
	"USR2":   syscall.SIGUSR2,
	"VTALRM": syscall.SIGVTALRM,
	"WINCH":  syscall.SIGWINCH,
	"XCPU":   syscall.SIGXCPU,
	"XFSZ":   syscall.SIGXFSZ,
}

type killContainerCmd struct {
	Name   string
	Root   string
	Signal syscall.Signal
}

func requestDaemonKillContainer(root, container string, signal syscall.Signal) error {
	conn, err := net.Dial("unix", path.Join(root, container, "runv.sock"))
	if err != nil {
		return err
	}

	killCmd := &killContainerCmd{Name: container, Root: root, Signal: signal}
	cmd, err := json.Marshal(killCmd)
	if err != nil {
		return err
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_KILLCONTAINER,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])

	return nil
}

var killCommand = cli.Command{
	Name:  "kill",
	Usage: "kill sends the specified signal (default: SIGTERM) to the container's init process",
	Action: func(context *cli.Context) {
		sigstr := context.Args().First()
		if sigstr == "" {
			sigstr = "SIGTERM"
		}

		signal, err := parseSignal(sigstr)
		if err != nil {
			fmt.Printf("kill container failed %v\n", err)
			os.Exit(-1)
		}

		err = requestDaemonKillContainer(context.GlobalString("root"), context.GlobalString("id"), signal)
		if err != nil {
			fmt.Printf("kill container failed %v\n", err)
			os.Exit(-1)
		}
	},
}

func parseSignal(rawSignal string) (syscall.Signal, error) {
	s, err := strconv.Atoi(rawSignal)
	if err == nil {
		return syscall.Signal(s), nil
	}
	signal, ok := signalMap[strings.TrimPrefix(strings.ToUpper(rawSignal), "SIG")]
	if !ok {
		return -1, fmt.Errorf("unknown signal %q", rawSignal)
	}
	return signal, nil
}
