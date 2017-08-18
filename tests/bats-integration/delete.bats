#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runv delete" {
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

@test "runv delete --force" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # check state
  testcontainer test_busybox running

  # force delete test_busybox
  runv delete --force test_busybox

  runv state test_busybox
  [ "$status" -ne 0 ]
}

@test "runv delete --force ignore not exist" {
  runv delete --force notexists
  [ "$status" -eq 0 ]
}
