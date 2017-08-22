# Configure containerd and docker to run with runv

Because ![runv](../README.md) is an [OCI](https://www.opencontainers.org/) compatible runtime, it can work smoothly with other OCI-compatible management tools, namingly [containerd](https://containerd.io/) and [docker](https://www.docker.com/community-edition), by replacing the default runtime [runc](https://github.com/opencontainers/runc).

This document walks through the steps of setting up environment to run containerd and docker with runv.

## Prerequisites
### Install qemu
runv by default uses [qemu](https://www.qemu.org/) to start virtual machines and makes use of [KVM](https://wiki.qemu.org/Features/KVM) if it is supported. Please make sure qemu is installed on the machine.

### Install hyperstart
runv needs [hyperstart](https://github.com/hyperhq/hyperstart) to provide guest kernel and initrd. By default, it looks for kernel and hyper-initrd.img from `/var/lib/hyper/` directory. Build hyperstart and copy them there:
```
$ git clone https://github.com/hyperhq/hyperstart.git
$ cd hyperstart
$ ./autogen.sh ;./configure ;make
$ mkdir /var/lib/hyper/
$ cp build/hyper-initrd.img build/kernel /var/lib/hyper
```

### Install runv
runv binary should be installed in a system path so that containerd and docker can find it.
```
$ git clone https://github.com/hyperhq/runv.git
$ cd runv
$ ./autogen.sh
$ ./configure
$ make
$ sudo make install
```

## Work with containerd
First, install containerd and save its default configuration to `/etc/containerd/config.toml`, which is the default configuration file containerd looks for.

```
$containerd config default > /etc/containerd/config.toml
```
Then add following lines to `/etc/containerd/config.toml`:
```
[plugins.linux]
  shim = "containerd-shim"
  no_shim = false
  runtime = "runv"
  shim_debug = true
```
Now everything is configured. Start containerd with `containerd` command:
```
$containerd
```
In another window, test with containerd's client commandline tool `ctr`:
```
$ctr pull docker.io/library/busybox:latest
$ctr run --rm -t docker.io/library/busybox:latest foobar sh
/ # ls
bin   dev   etc   home  lib   proc  root  sys   tmp   usr   var
/ # exit
```

## Work with docker
While containerd is a bit low level to users and lacks useful features like network management, docker is more mature and has a lot of fancy features. We choose [docker CE, the community-edition of docker](https://www.docker.com/community-edition) to demonstrate how to configure runv with docker.

First, install docker CE as illustrated in the [docker CE download page](https://www.docker.com/community-edition#/download). As of writing, docker CE version `17.06.1-ce` is used, which is the current latest docker CE edge build.

Stop docker service (with e.g., `systemctl stop docker`) and then create docker's default configuration file `/etc/docker/daemon.json` with following contents, or add following contents to it if it exists already.
```
{
  "default-runtime": "runv",
  "runtimes": {
    "runv": {
      "path": "runv"
    }
  }
}
```
Now just start dockerd (with e.g., `systemctl start docker`) and it's ready.
```
# docker info|grep Runtime
Runtimes: runc runv
Default Runtime: runv
$  docker run --rm -it busybox sh
/ # ls
bin   dev   etc   home  lib   proc  root  sys   tmp   usr   var
/ # exit
```
