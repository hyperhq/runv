# runv Integration Tests

Integration tests provide end-to-end testing of runv to check
if runv matches the runtime-spec and runtime-cli-spec in some cases.

Integration tests are written in *bash* using the
[bats](https://github.com/sstephenson/bats) framework.

## Running integration tests

The easiest way to run integration tests is with make command,
and the test runs directly on your host.
```
$ sudo make integration #todo
```
Or you can just run them directly using bats
```
$ sudo bats tests/integration
```
To run a single test bucket:
```
$ make integration TESTFLAGS="/exec.bats"
# or
sudo bats tests/integration/exec.bats
```


To run them on your host, you will need to setup a development environment plus
[bats](https://github.com/sstephenson/bats#installing-bats-from-source)
For example:
```
$ cd ~/go/src/github.com
$ git clone https://github.com/sstephenson/bats.git
$ cd bats
$ ./install.sh /usr/local
```

## Writing integration tests

[helper functions]
(https://github.com/hyperhq/runv/blob/master/test/integration/helpers.bash)
are provided in order to facilitate writing tests.

```sh
#!/usr/bin/env bats

# This will load the helpers.
load helpers

# setup is called at the beginning of every test.
function setup() {
  # see functions teardown_hello and setup_hello in helpers.bash, used to
  # create a pristine environment for running your tests
  teardown_hello
  setup_hello
}

# teardown is called at the end of every test.
function teardown() {
  teardown_hello
}

@test "this is a simple test" {
  runv run containerid
  # "The runv macro" automatically populates $status, $output and $lines.
  # Please refer to bats documentation to find out more.
  [ "$status" -eq 0 ]

  # check expected output
  [[ "${output}" == *"Hello"* ]]
}

```
