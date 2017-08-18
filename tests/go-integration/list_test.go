package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type containerState struct {
	specs.State
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
}

func (s *RunVSuite) TestListSleep(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "testListSleep"
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

	out, exitCode := s.runvCommand(c, "list")
	c.Assert(exitCode, checker.Equals, 0)
	c.Assert(out, checker.Contains, ctrName)
	<-exitChan
}

func (s *RunVSuite) TestListSleepJson(c *check.C) {
	defer s.PrintLog(c)
	ctrName := "testListSleepJson"
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

	out, exitCode := s.runvCommand(c, "list", "-f", "json")
	c.Assert(exitCode, checker.Equals, 0)
	c.Assert(out, checker.Contains, ctrName)

	decoder := json.NewDecoder(strings.NewReader(out))
	c.Assert(decoder, checker.NotNil)

	var list []containerState
	c.Assert(decoder.Decode(&list), checker.IsNil)

	flag := 1
	for _, cs := range list {
		if cs.ID == ctrName {
			c.Assert(cs.ID, check.Equals, ctrName)
			c.Assert(cs.Pid, checker.Not(checker.Equals), 0)
			c.Assert(cs.Bundle, checker.Equals, s.bundlePath)
			c.Assert(cs.Status, checker.Equals, "running")
			flag = 0
			break
		}
	}
	c.Assert(flag, check.Equals, 0)

	<-exitChan
}
