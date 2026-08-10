BINARY := civitai
PKG := ./cmd/civitai
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build install test vet lint fmt clean tidy ci ci-shallow mutate dogfood-check

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
	@echo
	@echo "make ci passed — note this is a FULL clone. CI checks out at depth 1,"
	@echo "where tests that read git history cannot resolve their base blobs."
	@echo "Before you push a change that touches those, run: make ci-shallow"

# Run the test suite in a DEPTH-1 clone of the current branch, which is what
# `actions/checkout@v4` gives CI. Deliberately NOT a prerequisite of `ci`: it
# costs a clone plus an uncached test run, and most changes do not need it.
# Committed state only — uncommitted work is invisible to it, and the target
# says so on stdout. See scripts/ci-shallow.sh for the controls it asserts.
ci-shallow:
	./scripts/ci-shallow.sh

# Mutation-test ONE package and print a report corrected for gremlins'
# non-compiling-mutant misclassification. Local investigation tool only:
# deliberately NOT part of `ci`, deliberately not a CI job, and it exits 0
# whatever it finds. `make mutate PKG=internal/blockproto` to point it
# elsewhere. Read the "what this does not cover" header in scripts/mutate.sh
# before believing a clean run — in particular, it cannot see a defect that
# lives in TEST code, which is most of what it would need to see here.
# Measurements: claudedocs/mutation-testing-experiment.md
mutate:
	./scripts/mutate.sh $(PKG)

# The dogfood sandbox (scripts/dogfood-sandbox.sh) is a shell file that gates a
# money-spending CLI, so it gets its own shellcheck + a selftest. `--offline`
# needs no credential and no network: it drives the real gate against a stub
# binary, so the meter, the ledger lock and every refusal are regression-tested
# here rather than only during a run. See claudedocs/dogfood-3-sandbox.md.
dogfood-check:
	@command -v shellcheck >/dev/null 2>&1 || { \
		echo "shellcheck not found. Install it: https://www.shellcheck.net/ (Nix: nix-shell -p shellcheck)"; \
		exit 1; \
	}
	shellcheck -x --exclude=SC2153 scripts/dogfood-sandbox.sh
	bash scripts/dogfood-sandbox.sh selftest --offline

clean:
	rm -rf bin dist
