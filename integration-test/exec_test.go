package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/kr/pty"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func (s *RunVSuite) TestExecHelloWorld(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "testExecHelloWorld"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, ctrName)
		c.Assert(exitCode, checker.Equals, 0)
	}()

	for count := 0; count < 10; count++ {
		_, exitCode, err := s.runvCommandWithError("state", ctrName)
		if exitCode == 0 {
			c.Assert(err, checker.IsNil)
			break
		}
		time.Sleep(1 * time.Second)
	}

	out, exitCode := s.runvCommand(c, "exec", ctrName, "echo", "hello")
	c.Assert(out, checker.Equals, "hello\n")
	c.Assert(exitCode, checker.Equals, 0)
	<-exitChan
}

func (s *RunVSuite) TestExecWithTty(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "TestExecWithTty"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, ctrName)
		c.Assert(exitCode, checker.Equals, 0)
	}()

	for count := 0; count < 10; count++ {
		_, exitCode, _ := s.runvCommandWithError("state", ctrName)
		if exitCode == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	cmdArgs := []string{"exec", "-t", ctrName, "sh"}
	cmd := exec.Command(s.binaryPath, cmdArgs...)
	tty, err := pty.Start(cmd)
	c.Assert(err, checker.IsNil)
	defer tty.Close()

	_, err = tty.Write([]byte("uname && exit\n"))
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
	<-exitChan
}

func (s *RunVSuite) TestExecWithProcessJson(c *check.C) {
	defer s.PrintLog(c)
	process := specs.Process{
		Terminal: false,
		User:     specs.User{},
		Args: []string{
			"echo", "hello",
		},
		Env: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"TERM=xterm",
		},
		Cwd: "/",
	}

	data, err := json.MarshalIndent(&process, "", "\t")
	c.Assert(err, checker.IsNil)

	processPath := filepath.Join(s.bundlePath, "process.json")
	err = ioutil.WriteFile(processPath, data, 0666)
	c.Assert(err, checker.IsNil)

	ctrName := "TestExecWithProcessJson"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, ctrName)
		c.Assert(exitCode, checker.Equals, 0)
	}()

	for count := 0; count < 10; count++ {
		_, exitCode, err := s.runvCommandWithError("state", ctrName)
		if exitCode == 0 {
			c.Assert(err, checker.IsNil)
			break
		}
		time.Sleep(1 * time.Second)
	}

	out, exitCode := s.runvCommand(c, "exec", "-p", processPath, ctrName)
	c.Assert(out, checker.Equals, "hello\n")
	c.Assert(exitCode, checker.Equals, 0)
	<-exitChan
}
