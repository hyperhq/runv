#!/usr/bin/env bats

load helpers

function setup() {
	teardown_busybox
	setup_busybox
}

function teardown() {
	teardown_busybox
}

@test "runv run [tty ptsname]" {
	# Replace sh script with readlink.
    sed -i 's|"sh"|"sh", "-c", "for file in /proc/self/fd/[012]; do readlink $file; done"|' config.json

	# run busybox
	runv run test_busybox
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ /dev/pts/+ ]]
	[[ ${lines[1]} =~ /dev/pts/+ ]]
	[[ ${lines[2]} =~ /dev/pts/+ ]]
}

@test "runv run [tty owner]" {
	# tty chmod is not doable in rootless containers.
	# TODO: this can be made as a change to the gid test.
	requires root

	# Replace sh script with stat.
	sed -i 's/"sh"/"sh", "-c", "stat -c %u:%g $(tty) | tr : \\\\\\\\n"/' config.json

	# run busybox
	runv run test_busybox
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ 0 ]]
	# This is set by the default config.json (it corresponds to the standard tty group).
	[[ ${lines[1]} =~ 5 ]]
}

@test "runv run [tty owner] ({u,g}id != 0)" {
	# tty chmod is not doable in rootless containers.
	requires root

	# replace "uid": 0 with "uid": 1000
	# and do a similar thing for gid.
	sed -i 's;"uid": 0;"uid": 1000;g' config.json
	sed -i 's;"gid": 0;"gid": 100;g' config.json

	# Replace sh script with stat.
	sed -i 's/"sh"/"sh", "-c", "stat -c %u:%g $(tty) | tr : \\\\\\\\n"/' config.json

	# run busybox
	runv run test_busybox
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ 1000 ]]
	# This is set by the default config.json (it corresponds to the standard tty group).
	[[ ${lines[1]} =~ 5 ]]
}

@test "runv exec [tty ptsname]" {
	# run busybox detached
	runv run -d --console-socket $CONSOLE_SOCKET test_busybox
	[ "$status" -eq 0 ]

	# make sure we're running
	testcontainer test_busybox running

	# run the exec
	runv exec --tty test_busybox sh -c 'for file in /proc/self/fd/[012]; do readlink $file; done'
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ /dev/pts/+ ]]
	[[ ${lines[1]} =~ /dev/pts/+ ]]
	[[ ${lines[2]} =~ /dev/pts/+ ]]
}

@test "runv exec [tty owner]" {
	# tty chmod is not doable in rootless containers.
	# TODO: this can be made as a change to the gid test.
	requires root

	# run busybox detached
	runv run -d --console-socket $CONSOLE_SOCKET test_busybox
	[ "$status" -eq 0 ]

	# make sure we're running
	testcontainer test_busybox running

	# run the exec
	runv exec --tty test_busybox sh -c 'stat -c %u:%g $(tty) | tr : \\n'
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ 0 ]]
	[[ ${lines[1]} =~ 5 ]]
}

@test "runv exec [tty owner] ({u,g}id != 0)" {
	# tty chmod is not doable in rootless containers.
	requires root

	# replace "uid": 0 with "uid": 1000
	# and do a similar thing for gid.
	sed -i 's;"uid": 0;"uid": 1000;g' config.json
	sed -i 's;"gid": 0;"gid": 100;g' config.json

	# run busybox detached
	runv run -d --console-socket $CONSOLE_SOCKET test_busybox
	[ "$status" -eq 0 ]

	# make sure we're running
	testcontainer test_busybox running

	# run the exec
	runv exec --tty test_busybox sh -c 'stat -c %u:%g $(tty) | tr : \\n'
	[ "$status" -eq 0 ]
	[[ ${lines[0]} =~ 1000 ]]
	[[ ${lines[1]} =~ 5 ]]
}
