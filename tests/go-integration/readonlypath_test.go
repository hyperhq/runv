package main

import (
	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
)

func (s *RunVSuite) TestReadonlyNonExist(c *check.C) {
	defer s.PrintLog(c)
	spec := defaultTestSpec
	spec.Linux.ReadonlyPaths = []string{"foobar"}
	spec.Root.Readonly = false
	spec.Process.Args = []string{"ls", "foobar"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, _, _ := s.runvCommandWithError("run", "--bundle", s.bundlePath, "testReadonlyNonExist")
	c.Assert(out, checker.Contains, "ls: foobar: No such file or directory\n")
}

func (s *RunVSuite) TestReadonlyFile(c *check.C) {
	defer s.PrintLog(c)
	spec := defaultTestSpec
	spec.Linux.ReadonlyPaths = []string{"/testdata/foobar"}
	spec.Root.Readonly = false
	spec.Process.Args = []string{"touch", "/testdata/foobar"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, _, _ := s.runvCommandWithError("run", "--bundle", s.bundlePath, "testReadonlyFile")
	c.Assert(out, checker.Contains, "touch: /testdata/foobar: Read-only file system")
}

func (s *RunVSuite) TestReadonlyDir(c *check.C) {
	defer s.PrintLog(c)
	spec := defaultTestSpec
	spec.Linux.ReadonlyPaths = []string{"/testdata"}
	spec.Root.Readonly = false
	spec.Process.Args = []string{"touch", "/testdata/foobar"}
	c.Assert(s.addSpec(&spec), checker.IsNil)

	out, _, _ := s.runvCommandWithError("run", "--bundle", s.bundlePath, "testReadonlyDir")
	c.Assert(out, checker.Contains, "touch: /testdata/foobar: Read-only file system")
}
