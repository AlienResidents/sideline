BINARY := sideline
# The version lives in internal/version/VERSION and is embedded at compile
# time, so every build path reports the same value.
LDFLAGS := -s -w

# Recommended install location. ~/.local/bin needs no root and is already on
# PATH on most setups; override with `make install PREFIX=/usr/local/bin`.
PREFIX ?= $(HOME)/.local/bin

.PHONY: build build-all install dist test lint clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sideline

# No cgo, so every target builds from any machine with the Go toolchain.
build-all:
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 ./cmd/sideline
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 ./cmd/sideline
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64  ./cmd/sideline
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64  ./cmd/sideline

# Local dry run of the release: the same archives and checksums CI publishes,
# written to dist/, with nothing uploaded. Needs goreleaser installed.
dist:
	command -v goreleaser || { echo "goreleaser is not installed"; exit 2; }
	goreleaser release --snapshot --clean

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
