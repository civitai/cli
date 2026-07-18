BINARY := civitai
PKG := ./cmd/civitai
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

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

# `lint` runs the static-analysis gate. golangci-lint is REQUIRED (config in
# .golangci.yml) — it is the same tool CI runs, so a local `make lint` mirrors
# the PR gate. Install: https://golangci-lint.run/welcome/install/ (or, on Nix,
# `nix-shell -p golangci-lint --run "golangci-lint run"`).
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found. Install it: https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	}
	golangci-lint run

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

# Mirror CI locally.
ci: tidy vet test build

clean:
	rm -rf bin dist
