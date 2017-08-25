[![Build Status](https://travis-ci.org/hyperhq/runv.svg?branch=master)](https://travis-ci.org/hyperhq/runv)

## ![runV](logo.png)

`runV` is a hypervisor-based runtime for [OCI](https://github.com/opencontainers/runtime-spec).

### OCI

`runV` is compatible with OCI. However, due to the difference between hypervisors and containers, the following sections of OCI don't apply to runV:
- Namespace
- Capability
- Device
- `linux` and `mount` fields in OCI specs are ignored

### Hypervisor

The current release of `runV` supports the following hypervisors:
- KVM (QEMU 2.1 or later)
- KVM (Kvmtool)
- Xen (4.5 or later)
- QEMU without KVM (NOT RECOMMENDED. QEMU 2.1 or later)

### Distro

The current release of `runV` supports the following distros:

- Ubuntu 64bit
	- 15.04 Vivid
	- 14.10 Utopic
	- 14.04 Trusty
- CentOS 64bit
	- 7.0
	- 6.x (upgrade to QEMU 2.1)
- Fedora 20-22 64bit
- Debian 64bit
	- 8.0 jessie
	- 7.x wheezy (upgrade to QEMU 2.1)

### Build

```bash
# install autoconf automake pkg-config make gcc golang qemu
# optional install device-mapper and device-mapper-devel for device-mapper storage
# optional install xen and xen-devel for xen driver
# optional install libvirt and libvirt-devel for libvirt driver
# note: the above package names might be different in various distros
# create a 'github.com/hyperhq' in your GOPATH/src
$ cd $GOPATH/src/github.com/hyperhq
$ git clone https://github.com/hyperhq/runv/
$ cd runv
$ ./autogen.sh
$ ./configure --without-xen
$ make
$ sudo make install
```

### Run

#### Install qemu
runv by default uses [qemu](https://www.qemu.org/) to start virtual machines and makes use of [KVM](https://wiki.qemu.org/Features/KVM) if it is supported. Please make sure qemu is installed on the machine.

#### Install hyperstart
runv needs [hyperstart](https://github.com/hyperhq/hyperstart) to provide guest kernel and initrd. By default, it looks for kernel and hyper-initrd.img from `/var/lib/hyper/` directory. Build hyperstart and copy them there:
```
$ git clone https://github.com/hyperhq/hyperstart.git
$ cd hyperstart
$ ./autogen.sh ;./configure ;make
$ mkdir /var/lib/hyper/
$ cp build/hyper-initrd.img build/kernel /var/lib/hyper
```

#### Creating an OCI Bundle

In order to use runv you must have your container in the format of an OCI bundle.
If you have Docker installed you can use its `export` method to acquire a root filesystem from an existing Docker container.

```bash
# create the top most bundle directory
mkdir /containerbundle
cd /containerbundle

# create the rootfs directory
mkdir rootfs

# export busybox via Docker into the rootfs directory
docker export $(docker create busybox) | tar -C rootfs -xvf -
```

#### Creating an OCI Container Spec

After a root filesystem is populated you just generate a spec in the format of a `config.json` file inside your bundle.
`runv` provides a `spec` command to generate a base template spec that you are then able to edit.
To find features and documentation for fields in the spec please refer to the [specs](https://github.com/opencontainers/runtime-spec) repository.

```bash
runv spec
```

`runc spec` can be also used here for gernerating the `config.json` for a runv containter, since runv and runc are both OCI compatitible runtime.

#### Running Containers

The convenience command `run` will handle creating, starting, and deleting the container after it exits.

```bash
# run as root
cd /containerbundle
runv --kernel /path/to/kernel --initrd /path/to/initrd.img run mycontainer
# If you used the unmodified `runv spec` template this should give you a `sh` session inside the container.
$ ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.1   4352   232 ttyS0    S+   05:54   0:00 /init
root         2  0.0  0.5   4448   632 pts/0    Ss   05:54   0:00 sh
root         4  0.0  1.6  15572  2032 pts/0    R+   05:57   0:00 ps aux
```

The arguemnts `--kernel /path/to/kernel` and `--initrd /path/to/initrd.img` can be omitted.
In this case, `/var/lib/hyper/kernel` and `/var/lib/hyper/hyper-initrd.img` will be used by runv.

A container can be also be run via using the specs lifecycle operations.
Such as `runv create mycontainer`, `runv start mycontainer` and `runv delete mycontainer`.
This gives you more power over how the container is created and managed while it is running.

### Run it with docker

`runv` is a runtime implementation of [OCI runtime](https://github.com/opencontainers/runtime-spec) and its command line is highly compatible with the 1.0.0-rc3(keeping updated with the newest released runc). But it is still under development and uncompleted.

`runV` provides [a detailed walk-though](docs/configure-runv-with-containerd-docker.md) to integrate with latest versions of docker and containerd.

Quick example (requires 17.06.1-ce that talks runc-1.0.0-rc3 command line):

Configure docker to use `runV` as the default runtime.
```bash
$cat /etc/docker/daemon.json
{
  "default-runtime": "runv",
  "runtimes": {
    "runv": {
      "path": "runv"
    }
  }
}
```

Start docker, pull and create busybox container.
```bash
$sudo systemctl start docker
$docker pull busybox
Using default tag: latest
latest: Pulling from library/busybox
Digest: sha256:2605a2c4875ce5eb27a9f7403263190cd1af31e48a2044d400320548356251c4
Status: Image is up to date for busybox:latest
$docker run --rm -it busybox
/ # ls
bin   dev   etc   home  lib   proc  root  sys   tmp   usr   var
/ # exit
```
