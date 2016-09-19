# Create busybox.tar
# Use 'make V=1' to enable verbose output
#
JOBS := $(shell grep -c '^processor' /proc/cpuinfo)

BUSYBOX_VER = 1.25.0
BUSYBOX_URL = https://busybox.net/downloads/busybox-$(BUSYBOX_VER).tar.bz2
BUSYBOX_SHA1 = c6c759bf4c4f24b37f52e136e2b15d921a8d44cb
BUSYBOX_SRC = build/busybox-$(BUSYBOX_VER).tar.bz2

CURL_FLAGS = -s
VERBOSE = >/dev/null
CFLAGS = -w

ifdef V
	VERBOSE =
	CFLAGS =
	CURL_FLAGS =
endif

all: busybox.tar

$(BUSYBOX_SRC):
	curl $(CURL_FLAGS) --create-dirs -o $(BUSYBOX_SRC) $(BUSYBOX_URL)
	shasum -a 1 $(BUSYBOX_SRC) | grep ^$(BUSYBOX_SHA1)

build/busybox: $(BUSYBOX_SRC)
	tar xf $(BUSYBOX_SRC) --strip-components=1 -C build
	CFLAGS="$(CFLAGS)" make -C build defconfig $(VERBOSE)
	CFLAGS="$(CFLAGS)" LDFLAGS="--static" make -C build -j $(JOBS) all $(VERBOSE)

busybox.tar: build/busybox
	rm -rf build/rootfs
	mkdir -p build/rootfs
	cd build/rootfs && mkdir bin dev etc proc root sys tmp usr var
	touch build/rootfs/.dockerenv
	install -D build/busybox build/rootfs/bin/busybox
	set -e; for i in `build/busybox --list`; do ln -s busybox build/rootfs/bin/$$i; done
	cd build/rootfs && tar -cf $(CURDIR)/busybox.tar .

clean:
	rm -rf build busybox.tar

