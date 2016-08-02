#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

HYPERSTART_COMMIT="d3cfa23ddeb43c5d99807c8db"

# prepare kernel and initrd
HYPERSTARTPATH="$GOPATH/src/github.com/hyperhq/hyperstart"
RUNVPATH="$GOPATH/src/github.com/hyperhq/runv"
cd $HYPERSTARTPATH && git checkout -q $HYPERSTART_COMMIT
./autogen.sh && ./configure && make
cp -v $HYPERSTARTPATH/build/{kernel,hyper-initrd.img} $RUNVPATH/integration-test/test_data/

# do runv integration-test
cd $RUNVPATH 
make test-integration

###########################
# test runv from hyper

function cancel_and_exit()
{
	echo $1
	exit 0 # don't fail in preparing hyper
}

cd ${GOPATH}/src/github.com/hyperhq/hyperd || cancel_and_exit "failed to find hyper"
./autogen.sh || cancel_and_exit "failed to autogen hyper"
./configure || cancel_and_exit "failed to configure hyper"
make || cancel_and_exit "failed to compile hyper"
hack/test-cmd.sh

