#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <sched.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>

/* Get all of the CLONE_NEW* flags. */
#include "namespace.h"

#define bail(fmt, ...)								\
	do {									\
		int ret = __COUNTER__ + 1;					\
		fprintf(stderr, "nsenter: " fmt ": %m\n", ##__VA_ARGS__);	\
		exit(ret);							\
	} while(0)

static int get_RUNVNETNSPID(void)
{
	int pid;
	char *nspidStr, *endptr;

	nspidStr = getenv("_RUNVNETNSPID");
	if (nspidStr== NULL || *nspidStr == '\0')
		return -1;

	pid = strtol(nspidStr, &endptr, 10);
	if (*endptr != '\0')
		bail("unable to parse _RUNVNETNSPID");

	return pid;
}

void nsexec(void)
{
	int nsPid, nsFd;
	char *path;

	nsPid = get_RUNVNETNSPID();
	if (nsPid <= 0) {
		return;
	}

	path = malloc(sizeof(char)*64);
	sprintf(path, "/proc/%d/ns/net", nsPid);
	nsFd = open(path, O_RDONLY);
	setns(nsFd, CLONE_NEWNET);

	free(path);
	close(nsFd);
	return;
}
