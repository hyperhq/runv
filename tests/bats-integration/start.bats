#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runv start" {
  runv create --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox created

  # start container test_busybox
  runv start test_busybox
  [ "$status" -eq 0 ]

  testcontainer test_busybox running

  # delete test_busybox
  runv delete --force test_busybox

  runv state test_busybox
  [ "$status" -ne 0 ]
}
