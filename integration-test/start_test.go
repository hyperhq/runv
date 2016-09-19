package main

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (s *RunVSuite) TestStartHelloworld(c *check.C) {
	//TODO: enable this after fixing
	//c.Skip("enable this after fixing!")

	spec := defaultTestSpec
	spec.Process.Args = []string{"echo", "hello"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, exitCode := s.runvCommand(c, "start", "--bundle", s.bundlePath, "testStartHelloWorld")
	c.Assert(out, checker.Equals, "hello\n")
	c.Assert(exitCode, checker.Equals, 0)
}

func (s *RunVSuite) TestStartPid(c *check.C) {
	//TODO: enable this after fixing!!!
	//c.Skip("enable this after fixing")

	ctrName := "testStartPid"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	pidFilePath := filepath.Join(s.bundlePath, "pid.file")
	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "start", "--bundle", s.bundlePath, "--pid-file", pidFilePath, ctrName)
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
