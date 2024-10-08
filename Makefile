SHELL=/usr/bin/env bash

GOCC?=go

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

## ldflags -s -w strips binary

txcartool: $(BUILD_DEPS)
	rm -f txcartool
	#GOAMD64=v3 $(GOCC) build $(GOFLAGS) -o txcartool -ldflags " -s -w \
	GOAMD64=v3 $(GOCC) build $(GOFLAGS) -gcflags "all=-N -l" -o txcartool -ldflags " \
	" \
	./cmd
.PHONY: txcartool
BINS+=txcartool



debug: GOFLAGS+=-tags=debug
debug: build



all: build
.PHONY: all

build: txcartool
	@[[ $$(type -P "txcartool") ]] && echo "Caution: you have \
an existing txcartool binary in your PATH. This may cause problems if you don't run 'sudo make install'" || true

.PHONY: build


# TODO move systemd?

buildall: $(BINS)

clean:
	rm -rf $(CLEAN) $(BINS)
	-$(MAKE) -C $(FFI_PATH) clean
.PHONY: clean

dist-clean:
	git clean -xdff
	git submodule deinit --all -f
.PHONY: dist-clean




fix-imports:
	$(GOCC) run ./scripts/fiximports
