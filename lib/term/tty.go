package term

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

func TtySplice(conn net.Conn) (int, error) {
	inFd, _ := GetFdInfo(os.Stdin)
	oldState, err := SetRawTerminal(inFd)
	if err != nil {
		return -1, err
	}
	defer RestoreTerminal(inFd, oldState)

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

	if err := <-receiveStdout; err != nil {
		return -1, err
	}
	sendStdin <- nil

	if err := <-sendStdin; err != nil {
		return -1, err
	}

	return 0, nil

}
