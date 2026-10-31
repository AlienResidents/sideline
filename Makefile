BINARY := sideline
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

# Recommended install location. ~/.local/bin needs no root and is already on
# PATH on most setups; override with `make install PREFIX=/usr/local/bin`.
PREFIX ?= $(HOME)/.local/bin

.PHONY: build build-all install test lint clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sideline

# No cgo, so every target builds from any machine with the Go toolchain.
build-all:
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 ./cmd/sideline
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 ./cmd/sideline
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64  ./cmd/sideline
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64  ./cmd/sideline

install: build
	install -d "$(PREFIX)"
	install -m 0755 bin/$(BINARY) "$(PREFIX)/$(BINARY)"

test:
	go test ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

clean:
	rm -rf bin
