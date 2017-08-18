#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runv create" {
  runv create --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  # start the command
  runv start test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox running
}

@test "runv create exec" {
  skip #can't exec before start
  runv create --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  runv exec test_busybox true
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  # start the command
  runv start test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox running
}

@test "runv create --pid-file" {
  runv create --pid-file pid.txt --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} == $(__runv state test_busybox | jq '.pid') ]]

  # start the command
  runv start test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox running
}

@test "runv create --pid-file with new CWD" {
  # create pid_file directory as the CWD
  run mkdir pid_file
  [ "$status" -eq 0 ]
  run cd pid_file
  [ "$status" -eq 0 ]

  runv create --pid-file pid.txt -b $BUSYBOX_BUNDLE --console-socket $CONSOLE_SOCKET  test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} == $(__runv state test_busybox | jq '.pid') ]]

  # start the command
  runv start test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox running
}
