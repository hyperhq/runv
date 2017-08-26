package main

import (
	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
)

func (s *RunVSuite) TestMaskNonExist(c *check.C) {
	defer s.PrintLog(c)
	spec := newSpec()
	spec.Linux.MaskedPaths = []string{"foobar"}
	spec.Process.Args = []string{"ls", "/testdata"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, "testMaskNonExist")
	c.Assert(out, checker.Equals, "foobar\n")
	c.Assert(exitCode, checker.Equals, 0)
}

func (s *RunVSuite) TestMaskFile(c *check.C) {
	defer s.PrintLog(c)
	spec := newSpec()
	spec.Linux.MaskedPaths = []string{"/testdata/foobar"}
	spec.Process.Args = []string{"ls", "-l", "/testdata/foobar"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, "testMaskFile")
	c.Assert(out, checker.Contains, "crw-rw-rw-")
	c.Assert(exitCode, checker.Equals, 0)
}

func (s *RunVSuite) TestMaskDir(c *check.C) {
	defer s.PrintLog(c)
	spec := newSpec()
	spec.Linux.MaskedPaths = []string{"/testdata"}
	spec.Process.Args = []string{"ls", "/testdata"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, "testMaskDir")
	c.Assert(out, checker.Equals, "")
	c.Assert(exitCode, checker.Equals, 0)
}
