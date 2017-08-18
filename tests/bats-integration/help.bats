#!/usr/bin/env bats

load helpers

@test "runv -h" {
  runv -h
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ NAME:+ ]]
  [[ ${lines[1]} =~ runv\ '-'\ Open\ Container\ Initiative\ hypervisor-based\ runtime+ ]]

  runv --help
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ NAME:+ ]]
  [[ ${lines[1]} =~ runv\ '-'\ Open\ Container\ Initiative\ hypervisor-based\ runtime+ ]]
}

@test "runv command -h" {
  runv delete -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ delete+ ]]

  runv exec -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ exec+ ]]

  runv kill -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ kill+ ]]

  runv list -h
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ NAME:+ ]]
  [[ ${lines[1]} =~ runv\ list+ ]]

  runv list --help
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ NAME:+ ]]
  [[ ${lines[1]} =~ runv\ list+ ]]

  runv pause -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ pause+ ]]

  runv resume -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ resume+ ]]

  # We don't use runv_spec here, because we're just testing the help page.
  runv spec -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ spec+ ]]

  runv start -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ start+ ]]

  runv run -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ run+ ]]

  runv state -h
  [ "$status" -eq 0 ]
  [[ ${lines[1]} =~ runv\ state+ ]]

}

@test "runv foo -h" {
  runv foo -h
  [ "$status" -ne 0 ]
  [[ "${output}" == *"No help topic for 'foo'"* ]]
}
