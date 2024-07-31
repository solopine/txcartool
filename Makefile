SHELL=/usr/bin/env bash

all: build
.PHONY: all

unexport GOFLAGS

GOCC?=go

GOVERSION:=$(shell $(GOCC) version | tr ' ' '\n' | grep go1 | sed 's/^go//' | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')
GOVERSIONMIN:=$(shell cat GO_VERSION_MIN | awk -F. '{printf "%d%03d%03d", $$1, $$2, $$3}')

ifeq ($(shell expr $(GOVERSION) \< $(GOVERSIONMIN)), 1)
$(warning Your Golang version is go$(shell expr $(GOVERSION) / 1000000).$(shell expr $(GOVERSION) % 1000000 / 1000).$(shell expr $(GOVERSION) % 1000))
$(error Update Golang to version to at least $(shell cat GO_VERSION_MIN))
endif

# git modules that need to be loaded
MODULES:=

CLEAN:=
BINS:=

ldflags=-X=github.com/filecoin-project/lotus/build.CurrentCommit=+git.$(subst -,.,$(shell git describe --always --match=NeVeRmAtCh --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null))
ifneq ($(strip $(LDFLAGS)),)
	ldflags+=-extldflags=$(LDFLAGS)
endif

GOFLAGS+=-ldflags="$(ldflags)"


## FFI

FFI_PATH:=extern/filecoin-ffi/
FFI_DEPS:=.install-filcrypto
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): build/.filecoin-install ;

build/.filecoin-install: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

MODULES+=$(FFI_PATH)
BUILD_DEPS+=build/.filecoin-install
CLEAN+=build/.filecoin-install

ffi-version-check:
	@[[ "$$(awk '/const Version/{print $$5}' extern/filecoin-ffi/version.go)" -eq 3 ]] || (echo "FFI version mismatch, update submodules"; exit 1)
BUILD_DEPS+=ffi-version-check

.PHONY: ffi-version-check

$(MODULES): build/.update-modules ;
# dummy file that marks the last time modules were updated
build/.update-modules:
	git submodule update --init --recursive
	touch $@

# end git modules

## MAIN BINARIES

CLEAN+=build/.update-modules

deps: $(BUILD_DEPS)
.PHONY: deps

build-devnets: build txcar
.PHONY: build-devnets

debug: GOFLAGS+=-tags=debug
debug: build-devnets

2k: GOFLAGS+=-tags=2k
2k: build-devnets

calibnet: GOFLAGS+=-tags=calibnet
calibnet: build-devnets

butterflynet: GOFLAGS+=-tags=butterflynet
butterflynet: build-devnets

interopnet: GOFLAGS+=-tags=interopnet
interopnet: build-devnets

txcar: $(BUILD_DEPS)
	rm -f txcar
	$(GOCC) build $(GOFLAGS) -o txcar ./main
	# $(GOCC) build $(GOFLAGS) -gcflags "all=-N -l" -o lotus ./cmd/lotus
.PHONY: txcar
BINS+=txcar


build: txcar
	@[[ $$(type -P "txcar") ]] && echo "Caution: you have \
an existing lotus binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build

# MISC

buildall: $(BINS)


clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean


print-%:
	@echo $*=$($*)

