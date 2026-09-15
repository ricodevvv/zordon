BINARY := zordon
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint install clean

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/zordon

test:
	go test -race ./...

lint:
	gofmt -l .
	go vet ./...

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/zordon

clean:
	rm -f $(BINARY)
	rm -rf dist
