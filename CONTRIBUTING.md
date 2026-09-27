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

## Shell & CI gotchas

Moved here from [`AGENTS.md`](AGENTS.md) verbatim: that file is imported into
every AI-agent session, so it pays for these bytes whether or not the session
runs a single command. AGENTS.md keeps the routing line; the traps live here.


These produce **clean exits and reassuring output while doing nothing** — the
expensive class. Read the tool's *output*, not just its exit code. Host-generic
shell traps were removed from here (they belong in your global rules); what
follows is specific to this repo's toolchain.

- **`gofmt -s -l .` checking zero files** prints nothing and exits 0 — same as
  "all clean". If a path is misquoted or the working tree is wrong, the clean
  verdict says nothing about the code. Verify the directory, and that the tool
  found files.
- **Build/test/tool not on PATH exits `127`; OOM exits `134`** — both non-zero,
  but a script reading `rc != 0` as "N errors found" reports a plausible wrong
  count. Prefer `make ci` (which handles this) over hand-rolled invocations, and
  assert a **minimum expected count** (≥1 package tested, ≥1 file checked) as a
  positive control.
- **`go test ./...` with a broken import in `_test.go`** can compile to 0 tests
  and pass. If a package you expect tests for is silent, check explicitly:
  `go test -v -count=1 ./path/to/pkg | head`.
- **`gh pr checks` / `gh pr view --json statusCheckRollup` pitfalls.** Kept
  because with eight jobs of which only some gate, "have the checks settled?" is
  a live question here, and each trap answers it wrongly in the REASSURING
  direction:
  - `.conclusion` is `null` for commit statuses (only check-runs populate it) —
    poll `.state` instead.
  - Checks go through `QUEUED` → `IN_PROGRESS` → conclusion. A poll matching
    only `PENDING` declares "settled" while checks are still running.
  - A freshly-created check has an **empty** conclusion — matches no busy keyword,
    so a grep-for-busy loop prints "ALL SETTLED" before anything started.
  - Fix: require every check to hold a **terminal** conclusion
    (`SUCCESS|FAILURE|CANCELLED|TIMED_OUT|NEUTRAL|SKIPPED`) and assert a
    minimum expected check count.
- 🔴 **`./scripts/ci-shallow.sh` reads COMMITTED state only** — it clones the
  branch at its tip, so running it on a dirty tree measures the **previous**
  commit and reports green about code you did not write. Its result is a claim
  about `HEAD`, never about the working tree: commit first, and confirm the SHA
  it cloned is the one you meant. This bit three separate agents in one session,
  each time in the reassuring direction.

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
- `internal/{scaffold,validate,pkgzip,manifest,config,auth}` — the building blocks.
- `internal/{appapi,genapi}` — the App-Blocks and generation API clients.
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
