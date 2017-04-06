#!/bin/bash

# A set of helpers for building hyperstart for tests

hyper::hyperstart::build() {
  if [ -e ${HYPERSTART_INITRD} ] && [ -e ${HYPERSTART_KERNEL} ]; then
      echo "hypetstart initrd and kernel already exist, skip build"
      return 0
  fi
  # build hyperstart
  echo "clone hyperstart repo"
  local tmp=$(mktemp -d)
  git clone https://github.com/hyperhq/hyperstart ${tmp}/hyperstart
  cd ${tmp}/hyperstart
  echo "build hyperstart"
  ./autogen.sh
  ./configure
  make

  KERNEL_PATH="${tmp}/hyperstart/build/kernel"
  if [ ! -f ${KERNEL_PATH} ]; then
    return 1
  fi
  INITRD_PATH="${tmp}/hyperstart/build/hyper-initrd.img"
  if [ ! -f ${INITRD_PATH} ]; then
    return 1
  fi

  cp -v $KERNEL_PATH $HYPERSTART_KERNEL
  cp -v $INITRD_PATH $HYPERSTART_INITRD

  rm -rf "${HYPER_TEMP}/hyperstart"
}
