## runv
`runv` is another runtime for OCI specification(https://github.com/opencontainers/spec).
Different with runc(https://github.com/opencontainers/runc), runv runs applications in
hypervisor environment.

### compatible
Since the difference between hypervisor and container, some configuration in OCI spec
in useless for runv.

Since application is running in hypervisor environment, there is no need to setup
namespace ,capabilities and devices for isolating resources and dropping capability,
The `linux` and `mount` field in OCI spec are ignored.

### Building
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

### Using
To run a OCI defined container, execute runv with the JSON format as the argument
or have a `config.json` file in the current working directory. hyper kernel and hyper initrd
is needed too, You can find them in hyperstart repo(https://github.com/hyperhq/hyperstart/).
If dont specify kernel and initrd argument, runv will read the `kernel` and `initrd.img` files
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
Please check the runc example to get the container rootfs.
https://github.com/opencontainers/runc#examples

And you can get a sample oci config.json on
https://github.com/opencontainers/runc#ocf-container-json-format or
simpliy run `runc spec`.
