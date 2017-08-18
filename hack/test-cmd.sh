#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

source $(dirname "$0")/lib/ci-common.sh

# do runv integration-test
cd $RUNV_ROOT
make test-integration
