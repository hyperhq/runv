#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "runv exec" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec test_busybox echo Hello from exec
  [ "$status" -eq 0 ]
  echo text echoed = "'""${output}""'"
  [[ "${output}" == *"Hello from exec"* ]]
}

@test "runv exec --pid-file" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec --pid-file pid.txt test_busybox echo Hello from exec
  [ "$status" -eq 0 ]
  echo text echoed = "'""${output}""'"
  [[ "${output}" == *"Hello from exec"* ]]

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ [0-9]+ ]]
  [[ ${lines[0]} != $(__runv state test_busybox | jq '.pid') ]]
}

@test "runv exec --pid-file with new CWD" {
  # create pid_file directory as the CWD
  run mkdir pid_file
  [ "$status" -eq 0 ]
  run cd pid_file
  [ "$status" -eq 0 ]

  # run busybox detached
  runv run -d -b $BUSYBOX_BUNDLE --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec --pid-file pid.txt test_busybox echo Hello from exec
  [ "$status" -eq 0 ]
  echo text echoed = "'""${output}""'"
  [[ "${output}" == *"Hello from exec"* ]]

  # check pid.txt was generated
  [ -e pid.txt ]

  run cat pid.txt
  [ "$status" -eq 0 ]
  [[ ${lines[0]} =~ [0-9]+ ]]
  [[ ${lines[0]} != $(__runv state test_busybox | jq '.pid') ]]
}

@test "runv exec ls -la" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec test_busybox ls -la
  [ "$status" -eq 0 ]
  [[ ${lines[0]} == *"total"* ]]
  [[ ${lines[1]} == *"."* ]]
  [[ ${lines[2]} == *".."* ]]
}

@test "runv exec ls -la with --cwd" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec --cwd /bin test_busybox pwd
  [ "$status" -eq 0 ]
  [[ ${output} == "/bin" ]]
}

@test "runv exec --env" {
  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec --env RUNV_EXEC_TEST=true test_busybox env
  [ "$status" -eq 0 ]

  [[ ${output} == *"RUNV_EXEC_TEST=true"* ]]
}

@test "runv exec --user" {
  # --user can't work in rootless containers
  requires root

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  runv exec --user 1000:1000 test_busybox id
  [ "$status" -eq 0 ]

  [[ ${output} == "uid=1000 gid=1000" ]]
}
