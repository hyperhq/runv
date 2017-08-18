#!/usr/bin/env bats

load helpers

function setup() {
  teardown_busybox
  setup_busybox
}

function teardown() {
  teardown_busybox
}

@test "events --stats" {
  skip "runv doesn't support subcommand events yet"
  # XXX: currently cgroups require root containers.
  requires root

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # generate stats
  runv events --stats test_busybox
  [ "$status" -eq 0 ]
  [[ "${lines[0]}" == [\{]"\"type\""[:]"\"stats\""[,]"\"id\""[:]"\"test_busybox\""[,]* ]]
  [[ "${lines[0]}" == *"data"* ]]
}

@test "events --interval default " {
  skip "runv doesn't support subcommand events yet"
  # XXX: currently cgroups require root containers.
  requires root

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # spawn two sub processes (shells)
  # the first sub process is an event logger that sends stats events to events.log
  # the second sub process waits for an event that incudes test_busybox then
  # kills the test_busybox container which causes the event logger to exit
  (__runv events test_busybox > events.log) &
  (
    retry 10 1 eval "grep -q 'test_busybox' events.log"
    teardown_running_container test_busybox
  ) &
  wait # wait for the above sub shells to finish

  [ -e events.log ]

  run cat events.log
  [ "$status" -eq 0 ]
  [[ "${lines[0]}" == [\{]"\"type\""[:]"\"stats\""[,]"\"id\""[:]"\"test_busybox\""[,]* ]]
  [[ "${lines[0]}" == *"data"* ]]
}

@test "events --interval 1s " {
  skip "runv doesn't support subcommand events yet"
  # XXX: currently cgroups require root containers.
  requires root

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  # spawn two sub processes (shells)
  # the first sub process is an event logger that sends stats events to events.log once a second
  # the second sub process tries 3 times for an event that incudes test_busybox
  # pausing 1s between each attempt then kills the test_busybox container which
  # causes the event logger to exit
  (__runv events --interval 1s test_busybox > events.log) &
  (
    retry 3 1 eval "grep -q 'test_busybox' events.log"
    teardown_running_container test_busybox
  ) &
  wait # wait for the above sub shells to finish

  [ -e events.log ]

  run eval "grep -q 'test_busybox' events.log"
  [ "$status" -eq 0 ]
}

@test "events --interval 100ms " {
  skip "runv doesn't support subcommand events yet"
  # XXX: currently cgroups require root containers.
  requires root

  # run busybox detached
  runv run -d --console-socket $CONSOLE_SOCKET test_busybox
  [ "$status" -eq 0 ]

  #prove there is no carry over of events.log from a prior test
  [ ! -e events.log ]

  # spawn two sub processes (shells)
  # the first sub process is an event logger that sends stats events to events.log once every 100ms
  # the second sub process tries 3 times for an event that incudes test_busybox
  # pausing 100s between each attempt then kills the test_busybox container which
  # causes the event logger to exit
  (__runv events --interval 100ms test_busybox > events.log) &
  (
    retry 3 0.100 eval "grep -q 'test_busybox' events.log"
    teardown_running_container test_busybox
  ) &
  wait # wait for the above sub shells to finish

  [ -e events.log ]

  run eval "grep -q 'test_busybox' events.log"
  [ "$status" -eq 0 ]
}
