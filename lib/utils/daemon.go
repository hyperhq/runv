package utils

//#include <stdio.h>
//#include <stdlib.h>
//#include <unistd.h>
//#include <sys/types.h>
//#include <sys/stat.h>
//#include <sys/wait.h>
//#include <sys/time.h>
//#include <sys/resource.h>
//#include <fcntl.h>
/*
int daemonize(char *cmd, char *argv[], int pipe, int fds[], int num) {
	int status = 0, fd, pid, i;
	struct sigaction sa;

	pid = fork();
	if (pid < 0) {
		return -1;
	} else if (pid > 0) {
		if (waitpid(pid, &status, 0) < 0)
			return -1;
		return WEXITSTATUS(status);
	}
	//Become a session leader to lose controlling TTY
	setsid();

	//Ensure future opens won't allocate controlling TTYs
	sa.sa_handler = SIG_IGN;
	sigemptyset(&sa.sa_mask);
	sa.sa_flags = 0;
	if (sigaction(SIGHUP, &sa, NULL) < 0) {
		_exit(-1);
	}

	pid = fork();
	if (pid < 0) {
		_exit(-1);
	} else if (pid > 0) {
		_exit(0);
	}

	if (pipe > 0) {
		char buf[4];

		pid = getpid();

		buf[0] = pid >> 24;
		buf[1] = pid >> 16;
		buf[2] = pid >> 8;
		buf[3] = pid;

		write(pipe, buf, 4);
	}

	//Clear file creation mask
	umask(0);
	//Change the current working directory to the root so we won't prevent file system from being unmounted
	if (chdir("/") < 0)
		_exit(-1);

	//Close all open file descriptors
	for (i = 0; i < num; i++) {
		if (fds[i] == 0 || fds[i] == 1 || fds[i] == 2)
			continue;

		close(fds[i]);
	}
	//Attach file descriptors 0, 1, and 2 to /dev/null
	fd = open("/dev/null", O_RDWR);
	dup2(fd, 0);
	dup2(fd, 1);
	dup2(fd, 2);
	close(fd);

	if (execvp(cmd, argv) < 0)
		_exit(-1);
	return -1;
}
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"unsafe"
)

func ExecInDaemon(cmd string, argv []string, pipe int) error {
	fds := (*C.int)(nil)
	num := C.int(0)
	cargs := make([]*C.char, len(argv))

	for idx, a := range argv {
		cargs[idx] = C.CString(a)
	}

	fdlist := listFd()

	if len(fdlist) != 0 {
		fds = (*C.int)(unsafe.Pointer(&fdlist[0]))
		num = C.int(len(fdlist))
	}

	ret, err := C.daemonize(C.CString(cmd), (**C.char)(unsafe.Pointer(&cargs[0])), C.int(pipe), fds, num)
	if err != nil || ret < 0 {
		return fmt.Errorf("fail to start qemu in daemon mode")
	}

	return nil
}

func listFd() []int {
	files, err := ioutil.ReadDir("/proc/self/fd/")
	if err != nil {
		return []int{}
	}

	result := []int{}
	for _, file := range files {
		f, err := strconv.Atoi(file.Name())
		if err != nil {
			continue
		}

		result = append(result, f)
	}

	return result
}
