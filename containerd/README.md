# containerd

runv-containerd is a daemon to control hypercontainer and supports the same gRPC api of the [docker-containerd](https://github.com/docker/containerd/).

## Getting started

 - You need to [build it from source](https://github.com/hyperhq/runv#build) at first.
 - Then you can combind it with [docker](https://github.com/docker/docker) or [containerd-ctr](https://github.com/docker/containerd/tree/master/ctr).

### Dependencies
 - If you want to enable network for container, Kernels newer than `Linux 4.1-rc1` or [this commit](https://git.kernel.org/cgit/linux/kernel/git/torvalds/linux.git/commit/drivers/net/veth.c?id=a45253bf32bf49cdb2807bad212b84f5ab51ac26) are required.

### Try it with docker

```bash
# in terminal #1
runv --debug --driver libvirt --kernel /opt/hyperstart/build/kernel --initrd /opt/hyperstart/build/hyper-initrd.img containerd
# in terminal #2
docker daemon -D -l debug --containerd=/run/runv-containerd/containerd.sock
# in terminal #3 for trying it
docker run -ti busybox
# ls   # (already in the terminal of the busybox container)
# exit # (quit the container)
```

## Build
The same as [the build of runv](https://github.com/hyperhq/runv#build)

## Docs

 * [Containerd Client CLI reference (`ctr`)](https://github.com/docker/containerd/tree/master/docs/cli.md)
 * [Creating OCI bundles](https://github.com/docker/containerd/tree/master/docs/bundle.md)
 * [Docker CommandLine reference](https://github.com/docker/docker/tree/master/docs/reference/commandline)

