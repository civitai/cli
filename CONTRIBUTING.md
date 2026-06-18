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
make build   # build ./bin/civitai
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -s -w .
make lint    # golangci-lint if installed, else go vet
```

## Before you open a PR

Please make sure all of these pass — CI runs the same checks:

```bash
go build ./...
go test ./...
go vet ./...
gofmt -s -l .        # must print nothing
```

New behaviour should come with tests. Cover error paths, not just the happy
path — see the existing `*_test.go` files for the table-driven / httptest
patterns we use.

## Architecture

See [`CLAUDE.md`](CLAUDE.md) for the full layout, conventions, how to add a new
command, and the release process. The short version:

- `cmd/civitai` — the binary entrypoint.
- `internal/cmd` — the Cobra command tree (one file per command).
- `internal/{scaffold,validate,pkgzip,manifest,api,config}` — the building blocks.
- `schema/` — the vendored App Block manifest JSON Schema.

## The validate fidelity caveat

`civitai app validate` is a **best-effort local mirror** of the server-side
`BlockManifestValidator`. The server is the source of truth. If you change a
validation rule, keep the vendored schema (`schema/`) and the ported Go checks
(`internal/validate`) in sync with the server validator, and update the docs.
See `CLAUDE.md` for details.

## Commit / PR style

- Keep PRs focused; describe what changed and why.
- Conventional-commit-style subjects are appreciated (`feat:`, `fix:`,
  `docs:`, `test:`, `chore:`) — the changelog filters on them.

## License

By contributing you agree that your contributions are licensed under the
project's [Apache 2.0 license](LICENSE).
