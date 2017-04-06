#!/bin/bash
#  This file is part of cc-oci-runtime.
#
#  Copyright (C) 2017 Intel Corporation
#
#  This program is free software; you can redistribute it and/or
#  modify it under the terms of the GNU General Public License
#  as published by the Free Software Foundation; either version 2
#  of the License, or (at your option) any later version.
#
#  This program is distributed in the hope that it will be useful,
#  but WITHOUT ANY WARRANTY; without even the implied warranty of
#  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#  GNU General Public License for more details.
#
#  You should have received a copy of the GNU General Public License
#  along with this program; if not, write to the Free Software
#  Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.

set -e -x

source $(dirname "$0")/lib/ci-common.sh

cd $CC_ROOT

configure_opts=""

# disable uesless ones for runv
configure_opts+=" --disable-code-coverage"
configure_opts+=" --disable-tests"
configure_opts+=" --disable-cppcheck"
configure_opts+=" --disable-valgrind"
configure_opts+=" --disable-valgrind-helgrind"
configure_opts+=" --disable-valgrind-drd"
configure_opts+=" --disable-silent-rules"

# additional controls
configure_opts+=" --srcdir=\"${CC_ROOT}\""
configure_opts+=" --enable-auto-bundle-creation"
configure_opts+=" --with-auto-bundle-creation-path=\"${BUNDLE_PATH}\""
# fake hyperstart initrd as clear containers image
configure_opts+=" --with-cc-image=\"${HYPERSTART_INITRD}\""
# fake hyperstart kernel as clear containers kernel
configure_opts+=" --with-cc-kernel=\"${HYPERSTART_KERNEL}\""

# Test enable
configure_opts+=" --enable-functional-tests"

eval ./autogen.sh "$configure_opts"
