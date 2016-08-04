package main

import (
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
