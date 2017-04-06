#!/bin/bash

set -e -x

source $(dirname "$0")/.cc-configure.sh

(cd "$CC_ROOT" \
    && make runv-test)
