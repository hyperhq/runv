package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// GetExitCode returns the ExitStatus of the specified error if its type is
// exec.ExitError, returns 0 and an error otherwise.
func GetExitCode(err error) (int, error) {
	exitCode := 0
	if exiterr, ok := err.(*exec.ExitError); ok {
		if procExit, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			return procExit.ExitStatus(), nil
		}
	}
	return exitCode, fmt.Errorf("failed to get exit code")
}

// ProcessExitCode process the specified error and returns the exit status code
// if the error was of type exec.ExitError, returns nothing otherwise.
func ProcessExitCode(err error) (exitCode int) {
	if err != nil {
		var exiterr error
		if exitCode, exiterr = GetExitCode(err); exiterr != nil {
			// we've failed to retrieve exit code, so we set it to 127
			exitCode = 127
		}
	}
	return
}

func (s *RunVSuite) runvCommandWithError(args ...string) (string, int, error) {
	killer := time.AfterFunc(40*time.Second, func() {
		killAllRunvComponent(9)
	})

	cmdArgs := []string{
		"--kernel", s.kernelPath,
		"--initrd", s.initrdPath,
		"--log_dir", s.logPath,
		"--debug",
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(s.binaryPath, cmdArgs...)
	out, err := cmd.CombinedOutput()
	exitCode := ProcessExitCode(err)
	if !killer.Stop() {
		err = fmt.Errorf("test timeout error, orgin exec error: %v", err)
	}
	return string(out), exitCode, err
}

func (s *RunVSuite) runvCommand(c *check.C, args ...string) (string, int) {
	out, exitCode, err := s.runvCommandWithError(args...)
	if c != nil {
		c.Assert(err, checker.IsNil, check.Commentf("out: %s, exitCode: %d", out, exitCode))
	}
	return out, exitCode
}

func (s *RunVSuite) addSpec(spec *specs.Spec) error {
	specData, err := json.MarshalIndent(spec, "", "\t")
	if err != nil {
		return err
	}

	// write spec contents into file
	s.configPath = filepath.Join(s.bundlePath, configFileName)
	err = ioutil.WriteFile(s.configPath, specData, 0666)
	if err != nil {
		return err
	}
	return nil
}

func killAllRunvComponent(signal int) {
	sigFlag := fmt.Sprintf("-%d", signal)
	exec.Command("pkill", sigFlag, "runv").Run()
	exec.Command("pkill", sigFlag, "qemu").Run()
	exec.Command("pkill", sigFlag, "containerd-nslistener").Run()
}
