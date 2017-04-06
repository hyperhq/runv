#!/bin/bash

set -e

source $(dirname "$0")/lib/ci-common.sh

[ "$SEMAPHORE_THREAD_RESULT" = "passed" ] && exit 0

printf "=== Build failed ===\n"

cd "$ci_build_dir"

for f in test-suite.log $(ls *_test*.log)
do
    printf "\n=== Log file: '$f' ===\n\n"
    cat "$f"
done
