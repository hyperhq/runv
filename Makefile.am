export GO15VENDOREXPERIMENT=1
if WITH_XEN
XEN_BUILD_TAG=with_xen
else
XEN_BUILD_TAG=
endif

if WITH_XEN490
XEN490_BUILD_TAG=with_xen490
else
XEN490_BUILD_TAG=
endif

if WITH_LIBVIRT
LIBVIRT_BUILD_TAG=with_libvirt
else
LIBVIRT_BUILD_TAG=
endif

COMMIT=`git describe --dirty --always --tags 2> /dev/null || true`
GOLDFLAGS="-X main.gitCommit=${COMMIT} -X main.version=${VERSION}"
HYPER_BULD_TAGS=$(XEN_BUILD_TAG) $(XEN490_BUILD_TAG) $(LIBVIRT_BUILD_TAG)

all-local: build-runv
clean-local:
	-rm -f runv
install-exec-local:
	$(INSTALL_PROGRAM) runv $(bindir)

build-runv:
	go build -tags "static_build $(HYPER_BULD_TAGS)" -ldflags ${GOLDFLAGS} -o runv ./cli/
go-integration:
	cd tests/go-integration/test_data && make
	cd tests/go-integration && go test -check.v -test.timeout=120m ${TESTFLAGS} .
bats:
	go build -tags "static_build $(HYPER_BULD_TAGS)" -ldflags ${GOLDFLAGS} -o ./tests/recvtty/recvtty ./tests/recvtty/
	bats tests/bats-integration/
test-integration: go-integration bats
