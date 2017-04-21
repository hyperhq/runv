package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func (s *RunVSuite) TestSpecDefault(c *check.C) {
	defer s.PrintLog(c)
	expectedSpec := defaultTestSpec
	expectedSpec.Process.Terminal = true
	expectedSpec.Process.Args = []string{"sh"}

	// test runv spec without --bundle flag
	_, exitCode := s.runvCommand(c, "spec")
	c.Assert(exitCode, checker.Equals, 0)

	_, err := os.Stat(configFileName)
	c.Assert(err, checker.IsNil)

	b, err := ioutil.ReadFile(configFileName)
	c.Assert(err, checker.IsNil)

	var generatedSpec specs.Spec
	err = json.NewDecoder(bytes.NewReader(b)).Decode(&generatedSpec)
	c.Assert(err, checker.IsNil)

	// runv spec should generated contents as expected
	c.Assert(&generatedSpec, checker.DeepEquals, &expectedSpec)

	// test runv spec with --bundle flag
	newConfigPath := filepath.Join(s.bundlePath, configFileName)

	// FIXME: if file exists, runv spec should get exitcode 1
	// TODO: detect exitcode 1 when spec file already exists
	_, err = os.Stat(newConfigPath)
	if err == nil { // file exists, remove it first
		c.Assert(os.Remove(newConfigPath), checker.IsNil)
	}

	_, exitCode = s.runvCommand(c, "spec", "--bundle", s.bundlePath)
	c.Assert(exitCode, checker.Equals, 0)

	_, err = os.Stat(newConfigPath)
	c.Assert(err, checker.IsNil)

	b, err = ioutil.ReadFile(newConfigPath)
	c.Assert(err, checker.IsNil)

	err = json.NewDecoder(bytes.NewReader(b)).Decode(&generatedSpec)
	c.Assert(err, checker.IsNil)

	// runv spec should generated contents as expected
	c.Assert(&generatedSpec, checker.DeepEquals, &expectedSpec)
}
