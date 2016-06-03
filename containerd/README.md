# containerd

runv-containerd is a daemon to control hypercontainer and supports the same gRPC api of the [docker-containerd](https://github.com/docker/containerd/).

## Getting started

 - You need to [build it from source](https://github.com/hyperhq/runv#build) at first.
 - Then you can combind it with [docker](https://github.com/docker/docker) or [containerd-ctr](https://github.com/docker/containerd/tree/master/ctr).

### Try it with docker

```bash
# in terminal #1
runv-containerd --debug --driver libvirt --kernel /opt/hyperstart/build/kernel --initrd /opt/hyperstart/build/hyper-initrd.img
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

