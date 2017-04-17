package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/docker/docker/integration-cli/checker"
	"github.com/go-check/check"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	testDataDir        = "test_data"
	busyboxTarName     = "busybox.tar"
	configFileName     = "config.json"
	kernelName         = "kernel"
	initrdName         = "hyper-initrd.img"
	binaryName         = "runv"
	rootfsName         = "rootfs"
	logFileNamePattern = "runv*INFO*"
)

var (
	defaultTestSpec = specs.Spec{
		Version: specs.Version,
		Platform: specs.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Root: specs.Root{
			Path:     rootfsName,
			Readonly: true,
		},
		Process: specs.Process{
			Terminal: false,
			User:     specs.User{},
			Args: []string{
				"top",
			},
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"TERM=xterm",
			},
			Cwd: "/",
		},
		Hostname: "shell",
		Linux: &specs.Linux{
			Resources: &specs.LinuxResources{},
		},
	}
)

func Test(t *testing.T) { check.TestingT(t) }

type RunVSuite struct {
	binaryPath string
	kernelPath string
	initrdPath string
	bundlePath string
	configPath string
	logPath    string
}

var _ = check.Suite(&RunVSuite{})

func (s *RunVSuite) SetUpSuite(c *check.C) {
	var err error
	s.binaryPath, err = exec.LookPath(binaryName)
	c.Assert(err, checker.IsNil)

	// Prepare bundle and rootfs
	s.bundlePath = c.MkDir()
	rootfs := filepath.Join(s.bundlePath, rootfsName)
	err = os.Mkdir(rootfs, 777)
	c.Assert(err, checker.IsNil)

	// untar busybox image tar file into bundle/rootfs dir
	busyboxTarPath := filepath.Join(testDataDir, busyboxTarName)
	_, err = os.Stat(busyboxTarPath)
	c.Assert(err, checker.IsNil)
	cmd := exec.Command("tar", "-xf", busyboxTarPath, "-C", rootfs)
	var errStr bytes.Buffer
	cmd.Stderr = &errStr
	err = cmd.Run()
	c.Assert(err, checker.IsNil, check.Commentf("errors: %s", errStr.String()))

	// set kernel path
	s.kernelPath, err = filepath.Abs(filepath.Join(testDataDir, kernelName))
	c.Assert(err, checker.IsNil)
	_, err = os.Stat(s.kernelPath)
	c.Assert(err, checker.IsNil)

	// set initrd path
	s.initrdPath, err = filepath.Abs(filepath.Join(testDataDir, initrdName))
	c.Assert(err, checker.IsNil)
	_, err = os.Stat(s.initrdPath)
	c.Assert(err, checker.IsNil)

	// write spec into config file
	s.configPath = filepath.Join(s.bundlePath, configFileName)
	specData, err := json.MarshalIndent(defaultTestSpec, "", "\t")
	c.Assert(err, checker.IsNil)
	err = ioutil.WriteFile(s.configPath, specData, 0666)
	c.Assert(err, checker.IsNil)
}

func (s *RunVSuite) TearDownSuite(c *check.C) {}

func (s *RunVSuite) SetUpTest(c *check.C) {
	s.logPath = c.MkDir()
}

func (s *RunVSuite) TearDownTest(c *check.C) {
	// FIXME: Use runv kill/delete to do reliable garbage collection
	// after kill/delete functions are stable
	killAllRunvComponent(9)
}

func (s *RunVSuite) PrintLog(c *check.C) {
	if c.Failed() {
		// kill runv gently to enable garbage collection and flush log
		killAllRunvComponent(15)
		time.Sleep(3 * time.Second)
		out, err := exec.Command("sh", "-c", fmt.Sprintf("find %s -type f -name '%s' -exec echo -e '\n\nLog of ' {} ':' \\; -exec cat {} \\;", s.logPath, logFileNamePattern)).CombinedOutput()
		c.Assert(err, checker.IsNil)
		c.Logf("Test case %s failed, retrieve runv log from directory %s:\n%s", c.TestName(), s.logPath, out)
	}
}
