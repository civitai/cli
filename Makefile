BINARY := civitai
PKG := ./cmd/civitai
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install test vet lint fmt clean tidy ci

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test ./...

vet:
	go vet ./...

# `lint` runs golangci-lint if installed, else falls back to vet.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found; running go vet instead"; \
		go vet ./...; \
	fi

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

# Mirror CI locally.
ci: tidy vet test build

clean:
	rm -rf bin dist
