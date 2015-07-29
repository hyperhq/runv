## runV
`runV` is a hypervisor-based runtime for [OCF](https://github.com/opencontainers/spec).

### OCF
Due the difference between hypervisor and container, the following sections in OCF doesn't apply in runV.
- Namespace
- Capability
- Device
- `linux` and `mount` field in OCI spec are ignored.

### Build
```bash
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
To run a OCF image, execute `runv` with the JSON format as the argument
or have a `config.json` file in the current working directory. HyperKernel and hyper initrd
is needed too, You can find them in [HyperStart rpo](https://github.com/hyperhq/hyperstart/).
If not specified, runV will try to load the `kernel` and `initrd.img` files
in the current working directroy.

```bash
runv --kernel kernel --initrd initrd.img
# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.1   4352   232 ttyS0    S+   05:54   0:00 /init
root         2  0.0  0.5   4448   632 pts/0    Ss   05:54   0:00 sh
root         4  0.0  1.6  15572  2032 pts/0    R+   05:57   0:00 ps aux
```

### Example
Please check the runC example to get the container rootfs.
https://github.com/opencontainers/runc#examples

And you can get a sample OCF config.json at
https://github.com/opencontainers/runc#ocf-container-json-format or
simpliy execute `runc spec`.
