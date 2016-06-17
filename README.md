[![Build Status](https://travis-ci.org/hyperhq/runv.svg?branch=master)](https://travis-ci.org/hyperhq/runv)
## runV

`runV` is a hypervisor-based runtime for [OCI](https://github.com/opencontainers/runtime-spec).

### OCI
`runV` is compatible with OCI. However, due to the difference between hypervisors and containers, the following sections of OCI don't apply to runV:
- Namespace
- Capability
- Device
- `linux` and `mount` fields in OCI specs are ignored

### Hypervisor
The current release of `runV` supports the following hypervisors:
- KVM (QEMU 2.0 or later)
- Xen (4.5 or later)
- VirtualBox (Mac OS X)

### Distro
The current release of `runV` supports the following distros:

- Ubuntu 64bit
	- 15.04 Vivid
	- 14.10 Utopic
	- 14.04 Trusty
- CentOS 64bit
	- 7.0
	- 6.x (upgrade to QEMU 2.0)
- Fedora 20-22 64bit
- Debian 64bit
	- 8.0 jessie
	- 7.x wheezy (upgrade to QEMU 2.0)

### Build
```bash
# install autoconf automake pkg-config make gcc golang qemu
# optional install device-mapper and device-mapper-devel for device-mapper storage
# optional install xen and xen-devel for xen driver
# optional install libvirt and libvirt-devel for libvirt driver
# note: the above package names might be different in various distros
# create a 'github.com/hyperhq' in your GOPATH/src
cd $GOPATH/src/github.com/hyperhq
git clone https://github.com/hyperhq/runv/
cd runv
./autogen.sh
./configure --without-xen
make
sudo make install
```

### Run
To run a OCI image, execute `runv` with the [OCI JSON format file](https://github.com/opencontainers/runc#oci-container-json-format) as argument, or have a `config.json` file in `CWD`.

Also, a kernel and initrd images are needed too. We recommend you to build them from [HyperStart](https://github.com/hyperhq/hyperstart/) repo. If not specified, runV will try to load the `kernel` and `initrd.img` files from `CWD`.

```bash
runv --kernel kernel --initrd initrd.img
# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.1   4352   232 ttyS0    S+   05:54   0:00 /init
root         2  0.0  0.5   4448   632 pts/0    Ss   05:54   0:00 sh
root         4  0.0  1.6  15572  2032 pts/0    R+   05:57   0:00 ps aux
```

### Run it with docker
`runv` is a runtime implementation of [OCI](https://github.com/opencontainers/runtime-spec) and its commandline is partially compatible with the runc commandline. (it is compatible with the commandline of the docker-runc of the docker-1.1 currently, so official docker-1.1 binary works in the example)

Note, runv project also provides [other way](https://github.com/hyperhq/runv/tree/master/containerd) to integrate with docker.
Note, container tty is not working currently.

```bash
# in terminal #1
docker-containerd --debug -l /var/run/docker/libcontainerd/docker-containerd.sock \
  --runtime /path/to/runv --runtime-args --debug --runtime-args --driver=libvirt \
  --runtime-args --kernel=/opt/hyperstart/build/kernel \
  --runtime-args --initrd=/opt/hyperstart/build/hyper-initrd.img \
  --start-timeout 2m
# in terminal #2
docker daemon -D -l debug --containerd=/var/run/docker/libcontainerd/docker-containerd.sock
# in terminal #3 for trying it
docker run  busybox ls
bin
dev
etc
home
lib
proc
root
sys
tmp
usr
var
```

### Example
Please check the runC example to get the container rootfs.
https://github.com/opencontainers/runc#examples

And you can get a sample OCI config.json at
https://github.com/opencontainers/runc#oci-container-json-format or
simply execute `runv spec`.
