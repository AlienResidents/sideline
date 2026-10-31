BINARY := sideline
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

.PHONY: build build-all test lint clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sideline

# No cgo, so every target builds from any machine with the Go toolchain.
build-all:
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 ./cmd/sideline
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 ./cmd/sideline
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64  ./cmd/sideline
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64  ./cmd/sideline

test:
	go test ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

clean:
	rm -rf bin
