#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

if [[ -z "$(which protoc)" || "$(protoc --version)" != "libprotoc 3.0."* ]]; then
  echo "Generating protobuf requires protoc 3.0.0-beta1 or newer. Please download and"
  echo "install the platform appropriate Protobuf package for your OS: "
  echo
  echo "  https://github.com/google/protobuf/releases"
  echo
  echo "WARNING: Protobuf changes are not being validated"
  exit 1
fi

RUNV_ROOT=$(dirname "${BASH_SOURCE}")/..
PROTO_ROOT=${RUNV_ROOT}/api

protoc --proto_path=${PROTO_ROOT} --go_out=${PROTO_ROOT} ${PROTO_ROOT}/descriptions.proto
echo "Generated types from proto updated."
