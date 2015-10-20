package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/term"
	"github.com/opencontainers/specs"
)

var execCommand = cli.Command{
	Name:  "exec",
	Usage: "exec a new program in runv container",
	Action: func(context *cli.Context) {
		config, err := loadProcessConfig(context.Args().First())
		if err != nil {
			fmt.Errorf("load process config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Errorf("runv should be run as root\n")
			os.Exit(-1)
		}
		status, err := execProcess(context, config)
		if err != nil {
			fmt.Errorf("exec failed: %v", err)
		}
		os.Exit(status)
	},
}

func execProcess(context *cli.Context, config *specs.Process) (int, error) {
	podId := context.GlobalString("id")
	root := context.GlobalString("root")
	podPath := path.Join(root, podId)

	if podId == "" {
		return -1, fmt.Errorf("Please specify container")
	}

	conn, err := net.Dial("unix", path.Join(podPath, "runv.sock"))
	if err != nil {
		return -1, err
	}

	cmd, err := json.Marshal(config.Args)
	if err != nil {
		return -1, err
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_EXECCMD,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])

	tag, err := runvGetTag(conn)
	if err != nil {
		return -1, err
	}
	fmt.Printf("tag=%s\n", tag)

	inFd, _ := term.GetFdInfo(os.Stdin)
	outFd, isTerminalOut := term.GetFdInfo(os.Stdout)
	oldState, err := term.SetRawTerminal(inFd)
	if err != nil {
		return -1, err
	}
	defer term.RestoreTerminal(inFd, oldState)

	br := bufio.NewReader(conn)

	receiveStdout := make(chan error, 1)
	go func() {
		_, err = io.Copy(os.Stdout, br)
		receiveStdout <- err
	}()

	sendStdin := make(chan error, 1)
	go func() {
		io.Copy(conn, os.Stdin)

		if sock, ok := conn.(interface {
			CloseWrite() error
		}); ok {
			if err := sock.CloseWrite(); err != nil {
				fmt.Printf("Couldn't send EOF: %s\n", err.Error())
			}
		}
		// Discard errors due to pipe interruption
		sendStdin <- nil
	}()

	newTty(nil, podPath, tag, outFd, isTerminalOut).monitorTtySize()

	if err := <-receiveStdout; err != nil {
		return -1, err
	}
	sendStdin <- nil

	if err := <-sendStdin; err != nil {
		return -1, err
	}

	return 0, nil
}

// loadProcessConfig loads the process configuration from the provided path.
func loadProcessConfig(path string) (*specs.Process, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("JSON configuration file for %s not found", path)
		}
		return nil, err
	}
	defer f.Close()
	var s *specs.Process
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return s, nil
}
