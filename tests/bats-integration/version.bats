#!/usr/bin/env bats

load helpers

@test "runv version" {
  skip "need to update the check"
  runv -v
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ runv\ version\ [0-9]+\.[0-9]+\.[0-9]+ ]]
  [[ ${lines[1]} =~ commit:+ ]]
  [[ ${lines[2]} =~ spec:\ [0-9]+\.[0-9]+\.[0-9]+ ]]
}
