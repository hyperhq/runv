#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

# prepare kernel and initrd
HYPERSTARTPATH="$GOPATH/src/github.com/hyperhq/hyperstart"
RUNVPATH="$GOPATH/src/github.com/hyperhq/runv"
cd $HYPERSTARTPATH && ./autogen.sh && ./configure && make
cp -v $HYPERSTARTPATH/build/{kernel,hyper-initrd.img} $RUNVPATH/integration-test/test_data/

# do runv integration-test
cd $RUNVPATH
make test-integration

