#!/bin/bash

# Return true if currently running in a recognised CI environment
cor_ci_env()
{
    # Set by TravisCI and SemaphoreCI
    [ "$CI" = true ]
}

nested=$(cat /sys/module/kvm_intel/parameters/nested 2>/dev/null \
    || echo N)

# Do not display output if this file is sourced via a BATS test since it
# will cause the test to fail.
[ -z "$BATS_TEST_DIRNAME" ] && echo "INFO: Nested kvm available: $nested"

if [ -n "$SEMAPHORE_CACHE_DIR" ]
then
    # Running under SemaphoreCI
    prefix_dir="$SEMAPHORE_CACHE_DIR/cor"
else
    prefix_dir="$HOME/.cache/cor"
fi

deps_dir="${prefix_dir}/dependencies"
cor_ci_env && mkdir -p "$deps_dir" || :

RUNV_ROOT=$(cd `dirname "${BASH_SOURCE}"`/../..; pwd -P)
CC_ROOT=${GOPATH}/src/github.com/01org/cc-oci-runtime
BUNDLE_PATH=$CC_ROOT/bundle
HYPERSTART_KERNEL=$RUNV_ROOT/integration-test/test_data/kernel
HYPERSTART_INITRD=$RUNV_ROOT/integration-test/test_data/hyper-initrd.img

export LD_LIBRARY_PATH="${prefix_dir}/lib:$LD_LIBRARY_PATH"
export PKG_CONFIG_PATH="${prefix_dir}/lib/pkgconfig:$PKG_CONFIG_PATH"
export PATH="${prefix_dir}/bin:${prefix_dir}/sbin:$PATH"

# Directory to run build and tests in.
#
# An out-of-tree build is used to ensure all necessary files for
# building and testing are distributed and to check srcdir vs builddir
# discrepancies.
if [ -n "$SEMAPHORE_PROJECT_DIR" ]
then
    ci_build_dir="$SEMAPHORE_PROJECT_DIR/ci_build"
else
    ci_build_dir="$TRAVIS_BUILD_DIR/ci_build"
fi

cor_ci_env && mkdir -p "$ci_build_dir" || :
