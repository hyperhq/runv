#!/bin/bash

set -e -x

root=$(cd `dirname "$0"`/..; pwd -P)
source $(dirname "$0")/lib/ci-common.sh
source $(dirname "$0")/lib/hyperstart.sh

if [ "$SEMAPHORE" = true ]
then
    # SemaphoreCI has different environments that builds can run in. The
    # default environment does not have docker enabled so it is
    # necessary to specify a docker-enabled build environment on the
    # semaphoreci.com web site.
    #
    # However, currently, the docker-enabled environment does not
    # provide nested KVM (whereas the default environment does), so
    # manually enable nested kvm for the time being.
    sudo rmmod kvm-intel || :
    sudo sh -c "echo 'options kvm-intel nested=y' >> /etc/modprobe.d/dist.conf" || :
    sudo modprobe kvm-intel || :
fi

if [ "$nested" = "Y" ]
then
    # Ensure the user can access the kvm device
    sudo chmod g+rw /dev/kvm
    sudo chgrp "$USER" /dev/kvm
fi

pkgs=""

# general
pkgs+=" autoconf"
pkgs+=" automake"
pkgs+=" libtool"
pkgs+=" pkg-config"
pkgs+=" gettext"
pkgs+=" rpm2cpio"
pkgs+=" valgrind"

# runv dependencies
pkgs+=" libdevmapper-dev"
pkgs+=" libvirt-dev"
pkgs+=" libvirt-bin"
pkgs+=" qemu"

# runtime dependencies
pkgs+=" uuid-dev"
pkgs+=" libmnl-dev"
pkgs+=" libffi-dev"
pkgs+=" libpcre3-dev"
pkgs+=" qemu-system-x86"

# runtime + qemu-lite
pkgs+=" zlib1g-dev"

# qemu-lite
pkgs+=" libpixman-1-dev"

# gcc
pkgs+=" libcap-ng-dev"
pkgs+=" libgmp-dev"
pkgs+=" libmpfr-dev"
pkgs+=" libmpc-dev"

# code coverage
pkgs+=" lcov"

# chronic(1)
pkgs+=" moreutils"

# CRI-O
pkgs+=" libseccomp2"
pkgs+=" libseccomp-dev"
pkgs+=" seccomp"
pkgs+=" libdevmapper-dev"
pkgs+=" libdevmapper1.02.1"
pkgs+=" libgpgme11"
pkgs+=" libgpgme11-dev"

sudo apt-get -qq update
eval sudo apt-get -qq install "$pkgs"

function cheat_cc_setup(){
    # clone specified commit cc-oci-runtime repo
    [ ! -d $CC_ROOT ] && git clone https://github.com/01org/cc-oci-runtime.git $CC_ROOT
    cd $CC_ROOT
    git checkout ae8e37e48cdeb6d88d1b2a362b8797bd72037f83

    # replace some files to suite for runv
    cp -r -v $root/hack/.ci/fake_cc_root/* $CC_ROOT/

    # patch Makefile.am
    sed '/AC_INIT(/a\AC_CONFIG_AUX_DIR([m4])' -i $CC_ROOT/configure.ac
    sed '/PKG_CHECK_MODULES(/d' -i $CC_ROOT/configure.ac
    sed '/AS_IF(\[test "$have_required_glib"/d' -i $CC_ROOT/configure.ac
    # inject runv test target into cc Makefile
    cat >> $CC_ROOT/Makefile.am <<EOF 

runv-generate: \$(GENERATED_FILES)

runv-functional: runv-generate \$(BUNDLE_TEST_PATH)
		\$(AM_V_GEN)test -n "\$(BUNDLE_TEST_PATH)" && \\
			echo "Using bundle '\$(BUNDLE_TEST_PATH)'" || true
		@if [ "\$(builddir)" != "\$(srcdir)" ]; then \\
			rm -f \$(builddir)/tests/functional/*.bats ; \\
			for f in \$(abs_top_srcdir)/tests/functional/*.bats; do \\
				ln -s \$\$f \$(builddir)/tests/functional/ ; \\
			done; \\
		fi
		@bash -f \$(abs_builddir)/tests/functional/run-functional-tests.sh

runv-test: runv-functional

EOF
}

function setup_tools(){
    cd $CC_ROOT/installation
    source ./installation-setup.sh

    # Install bats
    bats_setup

    # Build glib
    glib_setup

    # Build json-glib
    json-glib_setup

    # Build check
    # We need to build check as the check version in the OS used by travis isn't
    # -pedantic safe.
    if ! lib_installed "check" "${check_version}"
    then
        file="check-${check_version}.tar.gz"

        if [ ! -e "$file" ]
        then
            curl -L -O "https://github.com/libcheck/check/releases/download/${check_version}/$file"
        fi

        compile check check-${check_version}.tar.gz check-${check_version}
    fi
}

cheat_cc_setup

cd $CC_ROOT/installation
source ./installation-setup.sh
bats_setup
#setup_tools

hyper::hyperstart::build
