package main

import (
	"strings"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
)

func (s *RunVSuite) TestKillKILL(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "testKillKILL"
	spec := defaultTestSpec
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

	_, exitCode := s.runvCommand(c, "kill", ctrName, "KILL")
	c.Assert(exitCode, checker.Equals, 0)

	timeout := true
	for count := 0; count < 10; count++ {
		out, exitCode := s.runvCommand(c, "list")
		c.Assert(exitCode, checker.Equals, 0)
		if !strings.Contains(out, ctrName) {
			timeout = false
			break
		}
		time.Sleep(1 * time.Second)
	}
	c.Assert(timeout, checker.Equals, false)
	<-exitChan
}
