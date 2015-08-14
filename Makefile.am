export GOPATH:=$(abs_top_srcdir)/Godeps/_workspace:$(GOPATH)
if WITH_XEN
HYPER_BULD_TAGS=with_xen
else
HYPER_BULD_TAGS=
endif

all-local: build-runv
clean-local:
	-rm -f runv
install-exec-local: 
	$(INSTALL_PROGRAM) runv $(bindir)
if ON_DARWIN
build-runv:
	godep go build -tags "static_build $(HYPER_BULD_TAGS)" runv.go runv_darwin.go
else
build-runv:
	godep go build -tags "static_build $(HYPER_BULD_TAGS)" runv.go runv_linux.go
endif
