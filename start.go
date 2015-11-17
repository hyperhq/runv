package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/hypervisor"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/opencontainers/specs"
)

type startConfig struct {
	Name       string
	BundlePath string
	Root       string
	Driver     string
	Kernel     string
	Initrd     string
	Vbox       string

	specs.LinuxSpec        `json:"config"`
	specs.LinuxRuntimeSpec `json:"runtime"`
}

func getDefaultBundlePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func loadStartConfig(context *cli.Context) (*startConfig, error) {
	config := &startConfig{
		Name:       context.GlobalString("id"),
		Root:       context.GlobalString("root"),
		Driver:     context.GlobalString("driver"),
		Kernel:     context.GlobalString("kernel"),
		Initrd:     context.GlobalString("initrd"),
		Vbox:       context.GlobalString("vbox"),
		BundlePath: context.String("bundle"),
	}
	var err error

	if config.Name == "" {
		return nil, fmt.Errorf("Please specify container ID")
	}

	if !filepath.IsAbs(config.BundlePath) {
		config.BundlePath, err = filepath.Abs(config.BundlePath)
		if err != nil {
			fmt.Printf("Cannot get abs path for bundle path: %s\n", err.Error())
			return nil, err
		}
	}

	if config.Kernel != "" && !filepath.IsAbs(config.Kernel) {
		config.Kernel, err = filepath.Abs(config.Kernel)
		if err != nil {
			fmt.Printf("Cannot get abs path for kernel: %s\n", err.Error())
			return nil, err
		}
	}

	if config.Initrd != "" && !filepath.IsAbs(config.Initrd) {
		config.Initrd, err = filepath.Abs(config.Initrd)
		if err != nil {
			fmt.Printf("Cannot get abs path for initrd: %s\n", err.Error())
			return nil, err
		}
	}

	if config.Vbox != "" && !filepath.IsAbs(config.Vbox) {
		config.Vbox, err = filepath.Abs(config.Vbox)
		if err != nil {
			fmt.Printf("Cannot get abs path for vbox: %s\n", err.Error())
			return nil, err
		}
	}

	ocffile := context.String("config-file")
	runtimefile := context.String("runtime-file")

	if _, err = os.Stat(ocffile); os.IsNotExist(err) {
		fmt.Printf("Please specify ocffile or put config.json under current working directory\n")
		return nil, err
	}

	ocfData, err := ioutil.ReadFile(ocffile)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		return nil, err
	}

	var runtimeData []byte = nil
	_, err = os.Stat(runtimefile)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("Fail to stat %s, %s\n", runtimefile, err.Error())
			return nil, err
		}
	} else {
		runtimeData, err = ioutil.ReadFile(runtimefile)
		if err != nil {
			fmt.Printf("Fail to readfile %s, %s\n", runtimefile, err.Error())
			return nil, err
		}
	}

	if err := json.Unmarshal(ocfData, &config.LinuxSpec); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(runtimeData, &config.LinuxRuntimeSpec); err != nil {
		return nil, err
	}

	return config, nil
}

var startCommand = cli.Command{
	Name:  "start",
	Usage: "create and run a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config-file, c",
			Value: "config.json",
			Usage: "path to spec config file",
		},
		cli.StringFlag{
			Name:  "runtime-file, r",
			Value: "runtime.json",
			Usage: "path to runtime config file",
		},
		cli.StringFlag{
			Name:  "bundle, b",
			Value: getDefaultBundlePath(),
			Usage: "path to the root of the bundle directory",
		},
	},
	Action: func(context *cli.Context) {
		config, err := loadStartConfig(context)
		if err != nil {
			fmt.Errorf("load config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Errorf("runv should be run as root\n")
			os.Exit(-1)
		}

		var sharedContainer string
		for _, ns := range config.LinuxRuntimeSpec.Linux.Namespaces {
			if ns.Path != "" {
				if strings.Contains(ns.Path, "/") {
					fmt.Printf("Runv doesn't support path to namespace file, it supports containers name as shared namespaces only\n")
					os.Exit(-1)
				}
				if ns.Type == "mount" {
					// TODO support it!
					fmt.Printf("Runv doesn't support shared mount namespace currently\n")
					os.Exit(-1)
				}
				sharedContainer = ns.Path
				_, err = os.Stat(path.Join(config.Root, sharedContainer, "state.json"))
				if err != nil {
					fmt.Printf("The container %s is not existing or not ready\n", sharedContainer)
					os.Exit(-1)
				}
				_, err = os.Stat(path.Join(config.Root, sharedContainer, "runv.sock"))
				if err != nil {
					fmt.Printf("The container %s is not ready\n", sharedContainer)
					os.Exit(-1)
				}
			}
		}

		if sharedContainer != "" {
			err = requestDaemonInitContainer(sharedContainer, config.Root, config.Name)
			if err != nil {
				os.Exit(-1)
			}
		} else {
			utils.ExecInDaemon("/proc/self/exe", []string{"runv", "--root", config.Root, "--id", config.Name, "daemon"})
		}

		status, err := startContainer(config)
		if err != nil {
			fmt.Errorf("start container failed: %v", err)
		}
		os.Exit(status)
	},
}

type initContainerCmd struct {
	Name string
	Root string
}

// Shared namespaces multiple containers suppurt
// The runv supports pod-style shared namespaces currently.
// (More fine grain shared namespaces style (docker/runc style) is under implementation)
//
// Pod-style shared namespaces:
// * if two containers share at least one type of namespace, they share all kinds of namespaces except the mount namespace
// * mount namespace can't be shared, each container has its own mount namespace
//
// Implementation detail:
// * Shared namespaces is configured in LinuxRuntimeSpec.Linux.Namespaces, the namespace Path should be existing container name.
// * In runv, shared namespaces multiple containers are located in the same VM which is managed by a runv-daemon.
// * Any running container can exit in any arbitrary order, the runv-daemon and the VM are existed until the last container of the VM is existed
func requestDaemonInitContainer(sharedContainer, root, container string) error {
	conn, err := net.Dial("unix", path.Join(root, sharedContainer, "runv.sock"))
	if err != nil {
		return err
	}

	initCmd := &initContainerCmd{Name: container, Root: root}
	cmd, err := json.Marshal(initCmd)
	if err != nil {
		return err
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_INITCONTAINER,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])

	return nil
}

func startContainer(config *startConfig) (int, error) {
	stateDir := path.Join(config.Root, config.Name)

	conn, err := utils.UnixSocketConnect(path.Join(stateDir, "runv.sock"))
	if err != nil {
		return -1, err
	}

	cmd, err := json.Marshal(config)
	if err != nil {
		return -1, err
	}

	m := &hypervisor.DecodedMessage{
		Code:    RUNV_STARTCONTAINER,
		Message: []byte(cmd),
	}

	data := hypervisor.NewVmMessage(m)
	conn.Write(data[:])

	return containerTtySplice(stateDir, conn)
}
