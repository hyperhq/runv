#!/usr/bin/env bats

load helpers

function setup() {
  teardown_hello
  setup_hello
}

function teardown() {
  teardown_hello
}

@test "global --debug" {
  # run hello-world
  runv --debug run test_hello
  echo "${output}"
  [ "$status" -eq 0 ]
}

@test "global --debug to --log" {
  skip "runv doesn't support --log yet"
  # run hello-world
  runv --log log.out --debug run test_hello
  [ "$status" -eq 0 ]

  # check output does not include debug info
  [[ "${output}" != *"level=debug"* ]]

  # check log.out was generated
  [ -e log.out ]

  # check expected debug output was sent to log.out
  run cat log.out
  [ "$status" -eq 0 ]
  [[ "${output}" == *"level=debug"* ]]
}

@test "global --debug to --log --log-format 'text'" {
  skip "runv doesn't support --log yet"
  # run hello-world
  runv --log log.out --log-format "text" --debug run test_hello
  [ "$status" -eq 0 ]

  # check output does not include debug info
  [[ "${output}" != *"level=debug"* ]]

  # check log.out was generated
  [ -e log.out ]

  # check expected debug output was sent to log.out
  run cat log.out
  [ "$status" -eq 0 ]
  [[ "${output}" == *"level=debug"* ]]
}

@test "global --debug to --log --log-format 'json'" {
  skip "runv doesn't support --log yet"
  # run hello-world
  runv --log log.out --log-format "json" --debug run test_hello
  [ "$status" -eq 0 ]

  # check output does not include debug info
  [[ "${output}" != *"level=debug"* ]]

  # check log.out was generated
  [ -e log.out ]

  # check expected debug output was sent to log.out
  run cat log.out
  [ "$status" -eq 0 ]
  [[ "${output}" == *'"level":"debug"'* ]]
}
