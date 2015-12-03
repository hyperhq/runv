package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/lib/utils"
	"github.com/kardianos/osext"
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

	// If config.BundlePath is "", this code sets it to the current work directory
	if !filepath.IsAbs(config.BundlePath) {
		config.BundlePath, err = filepath.Abs(config.BundlePath)
		if err != nil {
			fmt.Printf("Cannot get abs path for bundle path: %s\n", err.Error())
			return nil, err
		}
	}

	ocffile := filepath.Join(config.BundlePath, specConfig)
	runtimefile := filepath.Join(config.BundlePath, runtimeConfig)

	if _, err = os.Stat(ocffile); os.IsNotExist(err) {
		fmt.Printf("Please make sure bundle directory contains config.json\n")
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

func firstExistingFile(candidates []string) string {
	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			return file
		}
	}
	return ""
}

var startCommand = cli.Command{
	Name:  "start",
	Usage: "create and run a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Usage: "path to the root of the bundle directory",
		},
	},
	Action: func(context *cli.Context) {
		config, err := loadStartConfig(context)
		if err != nil {
			fmt.Printf("load config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Printf("runv should be run as root\n")
			os.Exit(-1)
		}
		_, err = os.Stat(filepath.Join(config.Root, config.Name))
		if err == nil {
			fmt.Printf("Container %s exists\n", config.Name)
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
				_, err = os.Stat(filepath.Join(config.Root, sharedContainer, "state.json"))
				if err != nil {
					fmt.Printf("The container %s is not existing or not ready\n", sharedContainer)
					os.Exit(-1)
				}
				_, err = os.Stat(filepath.Join(config.Root, sharedContainer, "runv.sock"))
				if err != nil {
					fmt.Printf("The container %s is not ready\n", sharedContainer)
					os.Exit(-1)
				}
			}
		}

		// only set the default Kernel/Initrd/Vbox when it is the first container(sharedContainer == "")
		if config.Kernel == "" && sharedContainer == "" && config.Driver != "vbox" {
			config.Kernel = firstExistingFile([]string{
				filepath.Join(config.BundlePath, config.LinuxSpec.Spec.Root.Path, "boot/vmlinuz"),
				filepath.Join(config.BundlePath, "boot/vmlinuz"),
				filepath.Join(config.BundlePath, "vmlinuz"),
				"/var/lib/hyper/kernel",
			})
		}
		if config.Initrd == "" && sharedContainer == "" && config.Driver != "vbox" {
			config.Initrd = firstExistingFile([]string{
				filepath.Join(config.BundlePath, config.LinuxSpec.Spec.Root.Path, "boot/initrd.img"),
				filepath.Join(config.BundlePath, "boot/initrd.img"),
				filepath.Join(config.BundlePath, "initrd.img"),
				"/var/lib/hyper/hyper-initrd.img",
			})
		}
		if config.Vbox == "" && sharedContainer == "" && config.Driver == "vbox" {
			config.Vbox = firstExistingFile([]string{
				filepath.Join(config.BundlePath, "vbox.iso"),
				"/opt/hyper/static/iso/hyper-vbox-boot.iso",
			})
		}

		// convert the paths to abs
		if config.Kernel != "" && !filepath.IsAbs(config.Kernel) {
			config.Kernel, err = filepath.Abs(config.Kernel)
			if err != nil {
				fmt.Printf("Cannot get abs path for kernel: %s\n", err.Error())
				os.Exit(-1)
			}
		}
		if config.Initrd != "" && !filepath.IsAbs(config.Initrd) {
			config.Initrd, err = filepath.Abs(config.Initrd)
			if err != nil {
				fmt.Printf("Cannot get abs path for initrd: %s\n", err.Error())
				os.Exit(-1)
			}
		}
		if config.Vbox != "" && !filepath.IsAbs(config.Vbox) {
			config.Vbox, err = filepath.Abs(config.Vbox)
			if err != nil {
				fmt.Printf("Cannot get abs path for vbox: %s\n", err.Error())
				os.Exit(-1)
			}
		}

		if sharedContainer != "" {
			initCmd := &initContainerCmd{Name: config.Name, Root: config.Root}
			conn, err := runvRequest(config.Root, sharedContainer, RUNV_INITCONTAINER, initCmd)
			if err != nil {
				os.Exit(-1)
			}
			conn.Close()
		} else {
			path, err := osext.Executable()
			if err != nil {
				fmt.Printf("cannot find self executable path for %s: %v\n", os.Args[0], err)
				os.Exit(-1)
			}

			_, err = utils.ExecInDaemon(path, []string{"runv-ns-daemon", "--root", config.Root, "--id", config.Name})
			if err != nil {
				fmt.Printf("failed to launch runv daemon, error:%v\n", err)
				os.Exit(-1)
			}
		}

		status, err := startContainer(config)
		if err != nil {
			fmt.Printf("start container failed: %v", err)
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

func startContainer(config *startConfig) (int, error) {
	conn, err := runvRequest(config.Root, config.Name, RUNV_STARTCONTAINER, config)
	if err != nil {
		return -1, err
	}

	return containerTtySplice(config.Root, config.Name, conn)
}
