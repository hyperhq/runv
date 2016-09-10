export GO15VENDOREXPERIMENT=1
if WITH_XEN
XEN_BUILD_TAG=with_xen
else
XEN_BUILD_TAG=
endif

if WITH_LIBVIRT
LIBVIRT_BUILD_TAG=with_libvirt
else
LIBVIRT_BUILD_TAG=
endif

HYPER_BULD_TAGS=$(XEN_BUILD_TAG) $(LIBVIRT_BUILD_TAG)
all-local: build-runv
clean-local:
	-rm -f runv
install-exec-local: 
	$(INSTALL_PROGRAM) runv $(bindir)

build-runv:
	go build -tags "static_build $(HYPER_BULD_TAGS)" -o runv .
test-integration:
	cd integration-test && go test -check.v -test.timeout=120m ${TESTFLAGS} .
