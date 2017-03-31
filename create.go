package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/codegangsta/cli"
	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/kardianos/osext"
	"github.com/kr/pty"
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

func getDefaultBundlePath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

var createCommand = cli.Command{
	Name:  "create",
	Usage: "create a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container that you
are creating. The name you provide for the container instance must be unique on
your host.`,
	Description: `The create command creates an instance of a container for a bundle. The bundle
is a directory with a specification file named "` + specConfig + `" and a root
filesystem.

The specification file includes an args parameter. The args parameter is used
to specify command(s) that get run when the container is started. To change the
command(s) that get executed on start, edit the args parameter of the spec. See
"runv spec --help" for more explanation.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle, b",
			Value: getDefaultBundlePath(),
			Usage: "path to the root of the bundle directory, defaults to the current directory",
		},
		cli.StringFlag{
			Name:  "console",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.StringFlag{
			Name:  "console-socket",
			Usage: "specify the unix socket for sending the pty master back",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "specify the file to write the process id to",
		},
		cli.BoolFlag{
			Name:  "no-pivot",
			Usage: "[ignore on runv] do not use pivot root to jail process inside rootfs.  This should be used whenever the rootfs is on top of a ramdisk",
		},
	},
	Action: func(context *cli.Context) {
		runContainer(context, true)
	},
}

func runContainer(context *cli.Context, createOnly bool) {
	root := context.GlobalString("root")
	bundle := context.String("bundle")
	container := context.Args().First()
	ocffile := filepath.Join(bundle, specConfig)
	spec, err := loadSpec(ocffile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(-1)
	}
	if spec.Linux == nil {
		fmt.Fprintf(os.Stderr, "it is not linux container config\n")
		os.Exit(-1)
	}
	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "runv should be run as root\n")
		os.Exit(-1)
	}
	if container == "" {
		fmt.Fprintf(os.Stderr, "no container id provided\n")
		os.Exit(-1)
	}
	_, err = os.Stat(filepath.Join(root, container))
	if err == nil {
		fmt.Fprintf(os.Stderr, "Container %q exists\n", container)
		os.Exit(-1)
	}
	checkConsole(context, &spec.Process, createOnly)

	var sharedContainer string
	if containerType, ok := spec.Annotations["ocid/container_type"]; ok {
		if containerType == "container" {
			sharedContainer = spec.Annotations["ocid/sandbox_name"]
		}
	} else {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				if strings.Contains(ns.Path, "/") {
					fmt.Fprintf(os.Stderr, "Runv doesn't support path to namespace file, it supports containers name as shared namespaces only\n")
					os.Exit(-1)
				}
				if ns.Type == "mount" {
					// TODO support it!
					fmt.Fprintf(os.Stderr, "Runv doesn't support shared mount namespace currently\n")
					os.Exit(-1)
				}
				sharedContainer = ns.Path
				_, err = os.Stat(filepath.Join(root, sharedContainer, stateJson))
				if err != nil {
					fmt.Fprintf(os.Stderr, "The container %q is not existing or not ready\n", sharedContainer)
					os.Exit(-1)
				}
				_, err = os.Stat(filepath.Join(root, sharedContainer, "namespace"))
				if err != nil {
					fmt.Fprintf(os.Stderr, "The container %q is not ready\n", sharedContainer)
					os.Exit(-1)
				}
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
		fmt.Fprintf(os.Stderr, "Cannot get abs path for kernel: %v\n", err)
		os.Exit(-1)
	}
	initrd, err = filepath.Abs(initrd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot get abs path for initrd: %v\n", err)
		os.Exit(-1)
	}

	var namespace string
	var cmd *exec.Cmd
	if sharedContainer != "" {
		namespace = filepath.Join(root, sharedContainer, "namespace")
		namespace, err = os.Readlink(namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot get namespace link of the shared container: %v\n", err)
			os.Exit(-1)
		}
	} else {
		path, err := osext.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot find self executable path for %s: %v\n", os.Args[0], err)
			os.Exit(-1)
		}

		namespace, err = ioutil.TempDir("/run", "runv-namespace-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create runv namespace path: %v", err)
			os.Exit(-1)
		}

		args := []string{
			"--kernel", kernel,
			"--initrd", initrd,
			"--default_cpus", fmt.Sprintf("%d", context.GlobalInt("default_cpus")),
			"--default_memory", fmt.Sprintf("%d", context.GlobalInt("default_memory")),
		}
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
					fmt.Fprintf(os.Stderr, "Cannot get abs path for %s: %v\n", goption, err)
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
		cmd = &exec.Cmd{
			Path: path,
			Args: append([]string{"runv"}, args...),
		}
		cmd.Dir = "/"
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
		err = cmd.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to launch runv containerd: %v\n", err)
			os.Exit(-1)
		}
		if _, err = os.Stat(filepath.Join(namespace, "namespaced.sock")); os.IsNotExist(err) {
			time.Sleep(3 * time.Second)
		}
	}

	address := filepath.Join(namespace, "namespaced.sock")
	err = createContainer(context, container, address, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error %v\n", err)
		cmd.Process.Signal(syscall.SIGINT)
		os.Exit(-1)
	}
	err = os.Symlink(namespace, filepath.Join(root, container, "namespace"))
	if err != nil {
		cmd.Process.Signal(syscall.SIGINT)
		os.Exit(-1)
	}
	if !createOnly {
		startContainer(context, bundle, container, address, spec, context.Bool("detach"))
	}
	os.Exit(0)
}

func checkConsole(context *cli.Context, p *specs.Process, createOnly bool) {
	if context.String("console") != "" && context.String("console-socket") != "" {
		fmt.Fprintf(os.Stderr, "only one of --console & --console-socket can be specified")
		os.Exit(-1)
	}
	detach := createOnly
	if !createOnly {
		detach = context.Bool("detach")
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && !detach {
		fmt.Fprintf(os.Stderr, "--console[-socket] should be used on detached mode\n")
		os.Exit(-1)
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && !p.Terminal {
		fmt.Fprintf(os.Stderr, "--console[-socket] should be used on tty mode\n")
		os.Exit(-1)
	}
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

func createContainer(context *cli.Context, container, address string, config *specs.Spec) error {
	c := getClient(address)

	return ociCreate(context, container, func(stdin, stdout, stderr string) error {
		r := &types.CreateContainerRequest{
			Id:         container,
			Runtime:    "runv-create",
			BundlePath: context.String("bundle"),
			Stdin:      stdin,
			Stdout:     stdout,
			Stderr:     stderr,
		}

		if _, err := c.CreateContainer(netcontext.Background(), r); err != nil {
			return err
		}
		return nil
	})

}

func ociCreate(context *cli.Context, container string, createFunc func(stdin, stdout, stderr string) error) error {
	path, err := osext.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot find self executable path for %s: %v\n", os.Args[0], err)
		os.Exit(-1)
	}

	var stdin, stdout, stderr string
	var ptymaster, tty *os.File
	if context.String("console") != "" {
		tty, err = os.OpenFile(context.String("console"), os.O_RDWR, 0)
		if err != nil {
			return err
		}
	} else if context.String("console-socket") != "" {
		ptymaster, tty, err = pty.Open()
		if err != nil {
			return err
		}
		if err = sendtty(context.String("console-socket"), ptymaster); err != nil {
			return err
		}
		ptymaster.Close()
	}
	if tty == nil {
		pid := os.Getpid()
		stdin = fmt.Sprintf("/proc/%d/fd/0", pid)
		stdout = fmt.Sprintf("/proc/%d/fd/1", pid)
		stderr = fmt.Sprintf("/proc/%d/fd/2", pid)
	} else {
		defer tty.Close()
		stdin = tty.Name()
		stdout = tty.Name()
		stderr = tty.Name()
	}
	err = createFunc(stdin, stdout, stderr)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if context.String("pid-file") != "" || tty != nil {
		args := []string{
			"runv", "--root", context.GlobalString("root"),
			"shim", "--container", container, "--process", "init",
		}
		if context.String("pid-file") != "" {
			args = append(args, "--proxy-exit-code", "--proxy-signal")
		}
		if tty != nil {
			args = append(args, "--proxy-winsize")
		}
		cmd = &exec.Cmd{
			Path:   path,
			Args:   args,
			Dir:    "/",
			Stdin:  tty,
			Stdout: tty,
			Stderr: tty,
			SysProcAttr: &syscall.SysProcAttr{
				Setctty: tty != nil,
				Setsid:  true,
			},
		}
		err = cmd.Start()
		if err != nil {
			return err
		}
	}
	if context.String("pid-file") != "" {
		err = createPidFile(context.String("pid-file"), cmd.Process.Pid)
		if err != nil {
			return err
		}
	}

	return nil
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
