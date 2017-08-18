#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}


@test "kill detached busybox" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  runv kill test_busybox KILL
  [ "$status" -eq 0 ]

  retry 10 1 eval "__runv state test_busybox | grep -q 'stopped'"

  runv delete test_busybox
  [ "$status" -eq 0 ]
}
