export GOPATH:=$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
if WITH_XEN
HYPER_BULD_TAGS=with_xen
else
HYPER_BULD_TAGS=
endif

all-local: build-runv
clean-local:
	-rm -f runv
	-rm -f Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
install-exec-local: 
	$(INSTALL_PROGRAM) runv $(bindir)

# supporting linux container on non-linux platform (copy for catering to go build)
if ON_LINUX
linux_container: Godeps/_workspace/src/github.com/opencontainers/specs/config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime_config_linux.go
else
linux_container: Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go:
	cp Godeps/_workspace/src/github.com/opencontainers/specs/config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/config-linux.go
Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go:
	cp Godeps/_workspace/src/github.com/opencontainers/specs/runtime_config_linux.go Godeps/_workspace/src/github.com/opencontainers/specs/runtime-config-linux.go
endif

build-runv: linux_container
	go build -tags "static_build $(HYPER_BULD_TAGS)" -o runv .
