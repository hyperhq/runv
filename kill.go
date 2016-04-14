package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/lib/linuxsignal"
)

var linuxSignalMap = map[string]syscall.Signal{
	"ABRT":   linuxsignal.SIGABRT,
	"ALRM":   linuxsignal.SIGALRM,
	"BUS":    linuxsignal.SIGBUS,
	"CHLD":   linuxsignal.SIGCHLD,
	"CLD":    linuxsignal.SIGCLD,
	"CONT":   linuxsignal.SIGCONT,
	"FPE":    linuxsignal.SIGFPE,
	"HUP":    linuxsignal.SIGHUP,
	"ILL":    linuxsignal.SIGILL,
	"INT":    linuxsignal.SIGINT,
	"IO":     linuxsignal.SIGIO,
	"IOT":    linuxsignal.SIGIOT,
	"KILL":   linuxsignal.SIGKILL,
	"PIPE":   linuxsignal.SIGPIPE,
	"POLL":   linuxsignal.SIGPOLL,
	"PROF":   linuxsignal.SIGPROF,
	"PWR":    linuxsignal.SIGPWR,
	"QUIT":   linuxsignal.SIGQUIT,
	"SEGV":   linuxsignal.SIGSEGV,
	"STKFLT": linuxsignal.SIGSTKFLT,
	"STOP":   linuxsignal.SIGSTOP,
	"SYS":    linuxsignal.SIGSYS,
	"TERM":   linuxsignal.SIGTERM,
	"TRAP":   linuxsignal.SIGTRAP,
	"TSTP":   linuxsignal.SIGTSTP,
	"TTIN":   linuxsignal.SIGTTIN,
	"TTOU":   linuxsignal.SIGTTOU,
	"UNUSED": linuxsignal.SIGUNUSED,
	"URG":    linuxsignal.SIGURG,
	"USR1":   linuxsignal.SIGUSR1,
	"USR2":   linuxsignal.SIGUSR2,
	"VTALRM": linuxsignal.SIGVTALRM,
	"WINCH":  linuxsignal.SIGWINCH,
	"XCPU":   linuxsignal.SIGXCPU,
	"XFSZ":   linuxsignal.SIGXFSZ,
}

type killContainerCmd struct {
	Name   string
	Root   string
	Signal syscall.Signal
}

var killCommand = cli.Command{
	Name:  "kill",
	Usage: "kill sends the specified signal (default: SIGTERM) to the container's init process",
	ArgsUsage: `<container-id> <signal>

Where "<container-id>" is the name for the instance of the container and
"<signal>" is the signal to be sent to the init process.

For example, if the container id is "ubuntu01" the following will send a "KILL"
signal to the init process of the "ubuntu01" container:

       # runv kill ubuntu01 KILL`,
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		container := context.Args().First()
		sigstr := context.Args().Get(1)
		if sigstr == "" {
			sigstr = "SIGTERM"
		}

		signal, err := parseSignal(sigstr)
		if err != nil {
			fmt.Printf("kill container failed %v\n", err)
			os.Exit(-1)
		}

		killCmd := &killContainerCmd{Name: container, Root: root, Signal: signal}
		conn, err := runvRequest(root, container, RUNV_KILLCONTAINER, killCmd)
		if err != nil {
			fmt.Printf("kill container failed %v\n", err)
			os.Exit(-1)
		}
		conn.Close()
	},
}

func parseSignal(rawSignal string) (syscall.Signal, error) {
	s, err := strconv.Atoi(rawSignal)
	if err == nil {
		return syscall.Signal(s), nil
	}
	signal, ok := linuxSignalMap[strings.TrimPrefix(strings.ToUpper(rawSignal), "SIG")]
	if !ok {
		return -1, fmt.Errorf("unknown signal %q", rawSignal)
	}
	return signal, nil
}
