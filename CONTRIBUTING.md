# Contributing to the civitai CLI

Thanks for your interest in improving the Civitai CLI! This is a small,
focused Go project; contributions of all sizes are welcome.

## Getting set up

You need **Go 1.25+**. Clone the repo and run:

```bash
make ci     # go mod tidy + go vet + go test ./... + go build ./...
```

Common targets:

```bash
make build       # build ./bin/civitai
make test        # go test ./...
make vet         # go vet ./...
make fmt         # gofmt -s -w .
make lint        # golangci-lint if installed, else go vet
make ci-shallow  # go test ./... in a depth-1 clone — what CI actually sees
```

## `make ci-shallow` — the environment your local `make ci` cannot be

CI checks out with `actions/checkout@v4`, which clones at **depth 1**. Several
tests in this repo read git history — the AGENTS.md split guards resolve
`git show <base-commit>:AGENTS.md` — and a base blob that resolves fine in your
full clone is simply **not in the object store** at depth 1.

That divergence is not hypothetical. PR #305 shipped a green local `make ci`
(18 packages ok) and a red `build-test` in CI:

```
--- FAIL: TestEveryBaseBodyLineSurvivedTheMove
    agents_split_preserved_test.go:512: CONTROL failure: the differ compared 0
    lines, so its clean result says nothing
```

The author's local gate and their merged-tree gate were the *same* environment,
so neither could observe a defect that only exists in the other.
`make ci-shallow` is that missing environment, as one command: it clones the
current branch at depth 1 into a temporary directory, runs `go test ./...`
there, and removes the clone afterwards.

**It sees committed state only.** A depth-1 clone carries what you would
*push*, not what you have — uncommitted working-tree changes are invisible to
it. That is the right semantics for a pre-push check, but it is a sharp edge,
so the target prints the ref and sha it cloned and warns when your tree is
dirty. Commit before you rely on it.

It also runs the test suite *only*, not `go vet` / `go build` / `gofmt`. None
of those read git history, so `make ci` already covers them.

**Cost:** measured on a 24-core Linux box, **26.9 s / 27.4 s** with a warm Go
build cache and **33.0 s** with an empty one, against **27.6 s** for `make ci`
on the same tree — so it is roughly a second `make ci`, and the depth-1 clone
itself is a rounding error next to `go test`. Run it before pushing a change
that touches a history-reading test; it is deliberately *not* a prerequisite of
`make ci`, which would slow down every run for a check most changes do not
need.

If it fails, `CI_SHALLOW_KEEP=1 make ci-shallow` keeps the clone so you can
poke at it.

The target refuses to fall back to your working tree if the clone fails, and
asserts its own positive controls: the clone really is shallow, its HEAD is the
sha you asked for, and the expected number of packages reported `ok` — because
a shallow gate that silently tests nothing would read as reassurance while
catching exactly as much as no gate at all.

## Before you open a PR

Please make sure all of these pass — CI runs the same checks:

```bash
go build ./...
go test ./...
go vet ./...
gofmt -s -l .        # must print nothing
```

If your change touches a test that reads git history, also run
`make ci-shallow` (see above) — a full clone cannot observe that class of
failure.

New behaviour should come with tests. Cover error paths, not just the happy
path — see the existing `*_test.go` files for the table-driven / httptest
patterns we use.

## Architecture

See [`AGENTS.md`](AGENTS.md) for the full layout, conventions, how to add a new
command, and the release process. The short version:

- `cmd/civitai` — the binary entrypoint.
- `internal/cmd` — the Cobra command tree (one file per command).
- `internal/{scaffold,validate,pkgzip,manifest,api,config}` — the building blocks.
- `schema/` — the vendored App manifest JSON Schema.

## The validate fidelity caveat

`civitai app validate` is a **best-effort local mirror** of the server-side
`BlockManifestValidator`. The server is the source of truth. If you change a
validation rule, keep the vendored schema (`schema/`) and the ported Go checks
(`internal/validate`) in sync with the server validator, and update the docs.
See `AGENTS.md` for details.

## Commit / PR style

- Keep PRs focused; describe what changed and why.
- Conventional-commit-style subjects are appreciated (`feat:`, `fix:`,
  `docs:`, `test:`, `chore:`) — the changelog filters on them.

## License

By contributing you agree that your contributions are licensed under the
project's [Apache 2.0 license](LICENSE).
