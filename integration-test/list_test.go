package main

import (
	"time"

	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (s *RunVSuite) TestListSleep(c *check.C) {
	//TODO: enable this after fixing!!!
	//c.Skip("enable this after fixing")

	ctrName := "testListSleep"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "start", "--bundle", s.bundlePath, ctrName)
		c.Assert(exitCode, checker.Equals, 0)
	}()

	for count := 0; count < 10; count++ {
		_, exitCode, _ := s.runvCommandWithError("state", ctrName)
		if exitCode == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	out, exitCode := s.runvCommand(c, "list")
	c.Assert(exitCode, checker.Equals, 0)
	c.Assert(out, checker.Contains, ctrName)
	<-exitChan
}
