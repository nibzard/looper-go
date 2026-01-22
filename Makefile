PREFIX ?= $(HOME)/.local
CODEX_HOME ?= $(HOME)/.codex
GO ?= go
GOFLAGS ?=

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION)"

BINARY := looper
BUILDDIR := bin

.PHONY: all build install uninstall smoke test clean

all: build

build:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILDDIR)/$(BINARY) ./cmd/looper

install: build
	PREFIX="$(PREFIX)" CODEX_HOME="$(CODEX_HOME)" ./install.sh

uninstall:
	PREFIX="$(PREFIX)" CODEX_HOME="$(CODEX_HOME)" ./uninstall.sh

smoke:
	./scripts/smoke.sh

test: smoke
	$(GO) test ./...

clean:
	rm -f $(BUILDDIR)/$(BINARY)
