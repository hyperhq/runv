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

	"github.com/hyperhq/runv/containerd/api/grpc/types"
	"github.com/kardianos/osext"
	"github.com/kr/pty"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli"
	netcontext "golang.org/x/net/context"
)

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
	Action: func(context *cli.Context) error {
		if err := runContainer(context, true); err != nil {
			return cli.NewExitError(fmt.Sprintf("Run Container error: %v", err), -1)
		}
		return nil
	},
}

func runContainer(context *cli.Context, createOnly bool) error {
	root := context.GlobalString("root")
	bundle := context.String("bundle")
	container := context.Args().First()
	ocffile := filepath.Join(bundle, specConfig)
	spec, err := loadSpec(ocffile)
	if err != nil {
		return fmt.Errorf("load config failed: %v", err)
	}
	if spec.Linux == nil {
		return fmt.Errorf("it is not linux container config")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("runv should be run as root")
	}
	if container == "" {
		return fmt.Errorf("no container id provided")
	}
	_, err = os.Stat(filepath.Join(root, container))
	if err == nil {
		return fmt.Errorf("container %q exists", container)
	}
	if err = checkConsole(context, &spec.Process, createOnly); err != nil {
		return err
	}

	var sharedContainer string
	if containerType, ok := spec.Annotations["ocid/container_type"]; ok {
		if containerType == "container" {
			sharedContainer = spec.Annotations["ocid/sandbox_name"]
		}
	} else {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Path != "" {
				if strings.Contains(ns.Path, "/") {
					return fmt.Errorf("Runv doesn't support path to namespace file, it supports containers name as shared namespaces only")
				}
				if ns.Type == "mount" {
					// TODO support it!
					return fmt.Errorf("Runv doesn't support shared mount namespace currently")
				}
				sharedContainer = ns.Path
				_, err = os.Stat(filepath.Join(root, sharedContainer, stateJson))
				if err != nil {
					return fmt.Errorf("The container %q is not existing or not ready", sharedContainer)
				}
				_, err = os.Stat(filepath.Join(root, sharedContainer, "namespace"))
				if err != nil {
					return fmt.Errorf("The container %q is not ready", sharedContainer)
				}
			}
		}
	}

	var namespace string
	var cmd *exec.Cmd
	if sharedContainer != "" {
		namespace = filepath.Join(root, sharedContainer, "namespace")
		namespace, err = os.Readlink(namespace)
		if err != nil {
			return fmt.Errorf("cannot get namespace link of the shared container: %v", err)
		}
	} else {
		path, err := osext.Executable()
		if err != nil {
			return fmt.Errorf("cannot find self executable path for %s: %v", os.Args[0], err)
		}

		kernel, initrd, bios, cbfs, err := getKernelFiles(context, spec.Root.Path)
		if err != nil {
			return fmt.Errorf("can't find kernel/initrd/bios/cbfs files")
		}

		namespace, err = ioutil.TempDir("/run", "runv-namespace-")
		if err != nil {
			return fmt.Errorf("failed to create runv namespace path: %v", err)
		}

		args := []string{
			"--default_cpus", fmt.Sprintf("%d", context.GlobalInt("default_cpus")),
			"--default_memory", fmt.Sprintf("%d", context.GlobalInt("default_memory")),
		}

		// if bios+cbfs exist, use them first.
		if bios != "" && cbfs != "" {
			args = append(args, "--bios", bios, "--cbfs", cbfs)
		} else if kernel != "" && initrd != "" {
			args = append(args, "--kernel", kernel, "--initrd", initrd)
		} else {
			fmt.Fprintf(os.Stderr, "either bios+cbfs or kernel+initrd must be specified")
			os.Exit(-1)
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
					return fmt.Errorf("Cannot get abs path for %s: %v\n", goption, err)
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
			return fmt.Errorf("failed to launch runv containerd: %v", err)
		}
		if _, err = os.Stat(filepath.Join(namespace, "namespaced.sock")); os.IsNotExist(err) {
			time.Sleep(3 * time.Second)
		}
	}

	err = createContainer(context, container, namespace, spec)
	if err != nil {
		cmd.Process.Signal(syscall.SIGINT)
		return fmt.Errorf("failed to create container: %v", err)
	}
	if !createOnly {
		address := filepath.Join(namespace, "namespaced.sock")
		startContainer(context, bundle, container, address, spec, context.Bool("detach"))
	}
	return nil
}

func checkConsole(context *cli.Context, p *specs.Process, createOnly bool) error {
	if context.String("console") != "" && context.String("console-socket") != "" {
		return fmt.Errorf("only one of --console & --console-socket can be specified")
	}
	detach := createOnly
	if !createOnly {
		detach = context.Bool("detach")
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && !detach {
		return fmt.Errorf("--console[-socket] should be used on detached mode\n")
	}
	if (context.String("console") != "" || context.String("console-socket") != "") && !p.Terminal {
		return fmt.Errorf("--console[-socket] should be used on tty mode\n")
	}
	return nil
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

func createContainer(context *cli.Context, container, namespace string, config *specs.Spec) error {
	address := filepath.Join(namespace, "namespaced.sock")
	c, err := getClient(address)
	if err != nil {
		return err
	}

	return ociCreate(context, container, "init", func(stdin, stdout, stderr string) error {
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

		// create symbol link to namespace file
		namespaceDir := filepath.Join(context.GlobalString("root"), container, "namespace")
		if err := os.Symlink(namespace, namespaceDir); err != nil {
			return fmt.Errorf("failed to create symbol link %q: %v", filepath.Join(namespaceDir, "namespaced.sock"), err)
		}
		return nil
	})

}

func ociCreate(context *cli.Context, container, process string, createFunc func(stdin, stdout, stderr string) error) error {
	path, err := osext.Executable()
	if err != nil {
		return fmt.Errorf("cannot find self executable path for %s: %v\n", os.Args[0], err)
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
		}
		if context.GlobalString("log_dir") != "" {
			args = append(args, "--log_dir", filepath.Join(context.GlobalString("log_dir"), "shim-"+container))
		}
		if context.GlobalBool("debug") {
			args = append(args, "--debug")
		}
		args = append(args, "shim", "--container", container, "--process", process)
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
