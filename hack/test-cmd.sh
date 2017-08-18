#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

# prepare kernel and initrd
export HYPERSTARTPATH="$GOPATH/src/github.com/hyperhq/hyperstart"
export RUNVPATH="$GOPATH/src/github.com/hyperhq/runv"
cd $HYPERSTARTPATH && ./autogen.sh && ./configure && make
cp -v $HYPERSTARTPATH/build/{kernel,hyper-initrd.img} $RUNVPATH/tests/go-integration/test_data/

# do runv integration-test
cd $RUNVPATH
hack/install-bats.sh
make test-integration

