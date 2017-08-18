#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runv run detached" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running
}

@test "runv run detached ({u,g}id != 0)" {
  # cannot start containers as another user in rootless setup
  requires root

  # replace "uid": 0 with "uid": 1000
  # and do a similar thing for gid.
  sed -i 's;"uid": 0;"uid": 1000;g' config.json
  sed -i 's;"gid": 0;"gid": 100;g' config.json

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running
}

@test "runv run detached --pid-file" {
  # run busybox detached
  runv run --pid-file pid.txt -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} == $(__runv state test_busybox | jq '.pid') ]]
}

@test "runv run detached --pid-file with new CWD" {
  # create pid_file directory as the CWD
  run mkdir pid_file
  [ "$status" -eq 0 ]
  run cd pid_file
  [ "$status" -eq 0 ]

  # run busybox detached
  runv run --pid-file pid.txt -d  -b $BUSYBOX_BUNDLE --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} == $(__runv state test_busybox | jq '.pid') ]]
}
