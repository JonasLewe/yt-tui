BINDIR ?= $(HOME)/.local/bin
BINARY := yt-tui$(shell go env GOEXE)
COMMIT := $(shell git rev-parse --short=8 HEAD 2>/dev/null)
VERSION := $(if $(COMMIT),dev-$(COMMIT),dev)

.PHONY: build install

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o "$(BINARY)" .

install: build
	mkdir -p "$(BINDIR)"
	install -m 755 "$(BINARY)" "$(BINDIR)/$(BINARY)"
