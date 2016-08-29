package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/codegangsta/cli"
	"github.com/docker/containerd/api/grpc/types"
	"github.com/hyperhq/runv/lib/term"
	"github.com/kardianos/osext"
	"github.com/opencontainers/runtime-spec/specs-go"
	netcontext "golang.org/x/net/context"
)

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
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host.`,
	Description: `The start command creates an instance of a container for a bundle. The bundle
is a directory with a specification file named "` + specConfig + `" and a root
filesystem.

The specification file includes an args parameter. The args parameter is used
to specify command(s) that get run when the container is started. To change the
command(s) that get executed on start, edit the args parameter of the spec. See
"runv spec --help" for more explanation.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Usage: "path to the root of the bundle directory",
		},
		cli.StringFlag{
			Name:  "console",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "specify the file to write the process id to",
		},
		cli.BoolFlag{
			Name:  "no-pivot",
			Usage: "[ignore on runv] do not use pivot root to jail process inside rootfs.  This should be used whenever the rootfs is on top of a ramdisk",
		},
		cli.BoolFlag{
			Name:  "detach, d",
			Usage: "detach from the container's process",
		},
	},
	Action: func(context *cli.Context) {
		root := context.GlobalString("root")
		bundle := context.String("bundle")
		container := context.Args().First()
		ocffile := filepath.Join(bundle, specConfig)
		spec, err := loadSpec(ocffile)
		if err != nil {
			fmt.Printf("load config failed %v\n", err)
			os.Exit(-1)
		}
		if os.Geteuid() != 0 {
			fmt.Printf("runv should be run as root\n")
			os.Exit(-1)
		}
		_, err = os.Stat(filepath.Join(root, container))
		if err == nil {
			fmt.Printf("Container %s exists\n", container)
			os.Exit(-1)
		}

		var sharedContainer string
		for _, ns := range spec.Linux.Namespaces {
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
				_, err = os.Stat(filepath.Join(root, sharedContainer, stateJson))
				if err != nil {
					fmt.Printf("The container %s is not existing or not ready\n", sharedContainer)
					os.Exit(-1)
				}
				_, err = os.Stat(filepath.Join(root, sharedContainer, "namespace"))
				if err != nil {
					fmt.Printf("The container %s is not ready\n", sharedContainer)
					os.Exit(-1)
				}
			}
		}

		kernel := context.GlobalString("kernel")
		initrd := context.GlobalString("initrd")
		// only set the default kernel/initrd when it is the first container(sharedContainer == "")
		if kernel == "" && sharedContainer == "" {
			kernel = firstExistingFile([]string{
				filepath.Join(bundle, spec.Root.Path, "boot/vmlinuz"),
				filepath.Join(bundle, "boot/vmlinuz"),
				filepath.Join(bundle, "vmlinuz"),
				"/var/lib/hyper/kernel",
			})
		}
		if initrd == "" && sharedContainer == "" {
			initrd = firstExistingFile([]string{
				filepath.Join(bundle, spec.Root.Path, "boot/initrd.img"),
				filepath.Join(bundle, "boot/initrd.img"),
				filepath.Join(bundle, "initrd.img"),
				"/var/lib/hyper/hyper-initrd.img",
			})
		}

		// convert the paths to abs
		kernel, err = filepath.Abs(kernel)
		if err != nil {
			fmt.Printf("Cannot get abs path for kernel: %s\n", err.Error())
			os.Exit(-1)
		}
		initrd, err = filepath.Abs(initrd)
		if err != nil {
			fmt.Printf("Cannot get abs path for initrd: %s\n", err.Error())
			os.Exit(-1)
		}

		var address string
		var cmd *exec.Cmd
		if sharedContainer != "" {
			address = filepath.Join(root, container, "namespace/namespaced.sock")
		} else {
			path, err := osext.Executable()
			if err != nil {
				fmt.Printf("cannot find self executable path for %s: %v\n", os.Args[0], err)
				os.Exit(-1)
			}

			os.MkdirAll(context.String("log_dir"), 0755)
			namespace, err := ioutil.TempDir("/run", "runv-namespace-")
			if err != nil {
				fmt.Printf("Failed to create runv namespace path: %v", err)
				os.Exit(-1)
			}

			args := []string{"--kernel", kernel, "--initrd", initrd}
			if context.GlobalBool("debug") {
				args = append(args, "--debug")
			}
			if context.GlobalIsSet("driver") {
				args = append(args, "--driver", context.GlobalString("driver"))
			}
			for _, goption := range []string{"log_dir", "template"} {
				if context.GlobalIsSet(goption) {
					abs_path, err := filepath.Abs(context.GlobalString(goption))
					if err != nil {
						fmt.Printf("Cannot get abs path for %s: %v\n", goption, err)
						os.Exit(-1)
					}
					args = append(args, "--"+goption, abs_path)
				}
			}
			args = append(args,
				"containerd", "--solo-namespaced",
				"--containerd-dir", namespace,
				"--state-dir", root,
				"--listen", filepath.Join(namespace, "namespaced.sock"),
			)
			cmd = exec.Command("runv", args...)
			cmd.Path = path
			cmd.Dir = "/"
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true,
			}
			err = cmd.Start()
			if err != nil {
				fmt.Printf("failed to launch runv containerd, error:%v\n", err)
				os.Exit(-1)
			}
			address = filepath.Join(namespace, "namespaced.sock")
		}

		status := startContainer(context, container, address, spec)
		if status < 0 && sharedContainer == "" {
			cmd.Process.Signal(syscall.SIGINT)
		}
		os.Exit(status)
	},
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
// * Shared namespaces is configured in Spec.Linux.Namespaces, the namespace Path should be existing container name.
// * In runv, shared namespaces multiple containers are located in the same VM which is managed by a runv-daemon.
// * Any running container can exit in any arbitrary order, the runv-daemon and the VM are existed until the last container of the VM is existed

func startContainer(context *cli.Context, container, address string, config *specs.Spec) int {
	pid := os.Getpid()
	r := &types.CreateContainerRequest{
		Id:         container,
		BundlePath: context.String("bundle"),
		Stdin:      fmt.Sprintf("/proc/%d/fd/0", pid),
		Stdout:     fmt.Sprintf("/proc/%d/fd/1", pid),
		Stderr:     fmt.Sprintf("/proc/%d/fd/2", pid),
	}

	c := getClient(address)
	timestamp := uint64(time.Now().Unix())
	if _, err := c.CreateContainer(netcontext.Background(), r); err != nil {
		fmt.Printf("error %v\n", err)
		return -1
	}
	if config.Process.Terminal {
		s, err := term.SetRawTerminal(os.Stdin.Fd())
		if err != nil {
			fmt.Printf("error %v\n", err)
			return -1
		}
		defer term.RestoreTerminal(os.Stdin.Fd(), s)
		monitorTtySize(c, container, "init")
	}
	if context.String("pid-file") != "" {
		stateData, err := ioutil.ReadFile(filepath.Join(context.GlobalString("root"), container, stateJson))
		if err != nil {
			fmt.Printf("read state.json error %v\n", err)
			return -1
		}

		var s specs.State
		if err := json.Unmarshal(stateData, &s); err != nil {
			fmt.Printf("unmarshal state.json error %v\n", err)
			return -1
		}
		err = createPidFile(context.String("pid-file"), s.Pid)
		if err != nil {
			fmt.Printf("create pid-file error %v\n", err)
		}
	}
	return waitForExit(c, timestamp, container, "init")

}

// createPidFile creates a file with the processes pid inside it atomically
// it creates a temp file with the paths filename + '.' infront of it
// then renames the file
func createPidFile(path string, pid int) error {
	var (
		tmpDir  = filepath.Dir(path)
		tmpName = filepath.Join(tmpDir, fmt.Sprintf(".%s", filepath.Base(path)))
	)
	f, err := os.OpenFile(tmpName, os.O_RDWR|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0666)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%d", pid)
	f.Close()
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
