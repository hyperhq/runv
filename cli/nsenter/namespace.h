#ifndef NSENTER_NAMESPACE_H
#define NSENTER_NAMESPACE_H

#ifndef _GNU_SOURCE
#	define _GNU_SOURCE
#endif
#include <sched.h>

/* All of these are taken from include/uapi/linux/sched.h */
#ifndef CLONE_NEWNET
#	define CLONE_NEWNET 0x40000000 /* New network namespace */
#endif

#endif /* NSENTER_NAMESPACE_H */
