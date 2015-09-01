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

build-runv:
	go build -tags "static_build $(HYPER_BULD_TAGS)" -o runv .
