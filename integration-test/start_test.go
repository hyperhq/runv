package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/kr/pty"
)

func (s *RunVSuite) TestStartHelloworld(c *check.C) {
	defer s.PrintLog(c)
	spec := defaultTestSpec
	spec.Process.Args = []string{"echo", "hello"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, "testStartHelloWorld")
	c.Assert(out, checker.Equals, "hello\n")
	c.Assert(exitCode, checker.Equals, 0)
}

func (s *RunVSuite) TestStartPid(c *check.C) {
	defer s.PrintLog(c)
	c.Skip("enable this after fixing")
	ctrName := "testStartPid"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	pidFilePath := filepath.Join(s.bundlePath, "pid.file")
	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, "--pid-file", pidFilePath, ctrName)
		c.Assert(exitCode, checker.Equals, 0)
	}()

	var stateOut string
	for count := 0; count < 10; count++ {
		out, exitCode, err := s.runvCommandWithError("state", ctrName)
		if exitCode == 0 {
			c.Assert(err, checker.IsNil)
			stateOut = out
			break
		}
		time.Sleep(1 * time.Second)
	}

	decoder := json.NewDecoder(strings.NewReader(stateOut))
	c.Assert(decoder, checker.NotNil)

	cs := &cState{}
	c.Assert(decoder.Decode(cs), checker.IsNil)

	var pidData []byte
	var err error
	t := time.NewTimer(time.Second * 3)
	for {
		// in case that the pid file hasn't been created alreay
		select {
		case <-t.C:
			c.Assert(err, checker.IsNil)
		default:
		}
		pidData, err = ioutil.ReadFile(pidFilePath)
		if err == nil {
			break
		}
	}

	pid := strconv.Itoa(cs.InitProcessPid)
	c.Assert(string(pidData), checker.Equals, pid)

	<-exitChan
}

func (s *RunVSuite) TestStartWithTty(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "TestStartWithTty"
	spec := defaultTestSpec
	spec.Process.Terminal = true
	spec.Process.Args = []string{"sh"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	cmdArgs := []string{"--kernel", s.kernelPath, "--initrd", s.initrdPath}
	cmdArgs = append(cmdArgs, "run", "--bundle", s.bundlePath, ctrName)
	cmd := exec.Command(s.binaryPath, cmdArgs...)
	tty, err := pty.Start(cmd)
	c.Assert(err, checker.IsNil)
	defer tty.Close()

	_, err = tty.Write([]byte("uname && sleep 2 && exit\n"))
	c.Assert(err, checker.IsNil)

	chErr := make(chan error)
	go func() {
		chErr <- cmd.Wait()
	}()
	select {
	case err := <-chErr:
		c.Assert(err, checker.IsNil)
	case <-time.After(15 * time.Second):
		c.Fatal("timeout waiting for start to exit")
	}

	buf := make([]byte, 256)
	n, err := tty.Read(buf)
	c.Assert(err, checker.IsNil)
	c.Assert(bytes.Contains(buf, []byte("Linux")), checker.Equals, true, check.Commentf(string(buf[:n])))
}
