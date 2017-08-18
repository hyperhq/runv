package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
)

type cState struct {
	// Version is the OCI version for the container
	Version string `json:"ociVersion"`
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Annotations contains the list of annotations associated with the container
	Annotations map[string]string `json:"annotations"`
}

func (s *RunVSuite) TestStateSleep(c *check.C) {
	//TODO: enable this after fixing!!!
	//c.Skip("enable this after fixing")

	defer s.PrintLog(c)
	ctrName := "testStateSleep"
	spec := defaultTestSpec
	spec.Process.Args = []string{"sleep", "10"}
	c.Assert(s.addSpec(&spec), checker.IsNil)
	exitChan := make(chan struct{}, 0)

	go func() {
		defer close(exitChan)
		_, exitCode := s.runvCommand(c, "run", "--bundle", s.bundlePath, ctrName)
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
	c.Assert(cs.ID, check.Equals, ctrName)
	c.Assert(cs.InitProcessPid, checker.Not(checker.Equals), 0)
	c.Assert(cs.Bundle, checker.Equals, s.bundlePath)
	c.Assert(cs.Status, checker.Equals, "running")
	c.Assert(cs.Annotations, checker.IsNil)
	<-exitChan
}
