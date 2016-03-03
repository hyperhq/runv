#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

# TODO directly test runv here



###########################
# test runv from hyper

function cancel_and_exit()
{
	echo $1
	exit 0 # don't fail in preparing hyper
}

cd ${GOPATH}/src/github.com/hyperhq/hyper || cancel_and_exit "failed to find hyper"
./autogen.sh || cancel_and_exit "failed to autogen hyper"
./configure || cancel_and_exit "failed to configure hyper"
make || cancel_and_exit "failed to compile hyper"
hack/test-cmd.sh
