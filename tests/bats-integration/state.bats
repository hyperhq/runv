#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "state (kill + delete)" {
  runv state test_busybox
  [ "$status" -ne 0 ]

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  runv kill test_busybox KILL
  [ "$status" -eq 0 ]

  # wait for busybox to be in the destroyed state
  retry 10 1 eval "__runv state test_busybox | grep -q 'stopped'"

  # delete test_busybox
  runv delete test_busybox
  [ "$status" -eq 0 ]

  runv state test_busybox
  [ "$status" -ne 0 ]
}

@test "state (pause + resume)" {
  # XXX: pause and resume require cgroups.
  requires root

  runv state test_busybox
  [ "$status" -ne 0 ]

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  # pause busybox
  runv pause test_busybox
  [ "$status" -eq 0 ]

  # test state of busybox is paused
  testcontainer test_busybox paused

  # resume busybox
  runv resume test_busybox
  [ "$status" -eq 0 ]

  # test state of busybox is back to running
  testcontainer test_busybox running
}
