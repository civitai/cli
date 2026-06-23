# CLAUDE.md — repo guide for the civitai CLI

This file orients AI agents and human contributors working in this repo. It is
the source of truth for layout, conventions, and the release process.

## What this is

`civitai` is a single-binary Go CLI (in the `gh` / `kubectl` / `stripe` mold)
for [Civitai](https://civitai.com). Its first feature group is **App Blocks
authoring** under `civitai app`: it scaffolds a correct project, validates the
manifest against the platform contract, and packages/submits it for review.

## Architecture

```
cmd/civitai/main.go        binary entrypoint; injects build version/commit/date
internal/cmd/              the Cobra command tree (one file per command)
  root.go                  root command + SetBuildInfo (ldflags authoritative; falls back to runtime/debug.ReadBuildInfo for `go install` builds) + subcommand wiring
  app.go                   `civitai app` group
  app_init.go              `civitai app init`   -> internal/scaffold
  app_validate.go          `civitai app validate` -> internal/validate
  app_submit.go            `civitai app submit` -> internal/{pkgzip,api}
  app_status.go            `civitai app status` -> internal/api (GET /api/v1/blocks/submissions, self-scoped)
  login.go / whoami.go     auth -> internal/{config,api}
  version.go               `civitai version`
  completion.go            `civitai completion` (Cobra built-in generators)
internal/scaffold/         embedded project templates (go:embed) + slug logic
internal/validate/         JSON-Schema + ported semantic checks (see fidelity caveat)
internal/pkgzip/           canonical source-tree ZIP packaging + server caps
internal/manifest/         the manifest filename + a thin reader
internal/api/              HTTP client (submit + whoami + submission status) behind interfaces
internal/config/           Viper-backed config (~/.config/civitai/config.yaml)
schema/                    vendored App Block manifest JSON Schema (embedded)
examples/                  real example manifests (validated by examples_test.go)
schema.go / main.go        module-root package: go:embed of schema + examples
```

The module root (`package cli`, `main.go` + `schema.go`) exists only to embed
the vendored schema and the example manifests with `go:embed`. The executable
is `./cmd/civitai`.

## Conventions

- **Cobra + Viper.** Each command is a `newXxxCmd() *cobra.Command` constructor
  in `internal/cmd`, added to the tree in `root.go`. Always set a clear
  `Short`, a useful `Long`, and an `Example`.
- **Errors:** return `error` from `RunE`; lowercase, no trailing punctuation;
  wrap with `%w` when the cause matters. The root sets `SilenceUsage` +
  `SilenceErrors`, and `main` prints `Error: ...` to stderr with exit 1. Make
  errors actionable (tell the user the next command to run).
- **Output:** write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never bare
  `fmt.Println`, so commands stay testable.
- **Testability:** network/disk seams sit behind small interfaces
  (`api.Submitter`, `api.Verifier`) or take a dir argument, so tests use
  `httptest` and `t.TempDir()` with no live server.
- **Config:** persisted at `~/.config/civitai/config.yaml` (0600, atomic
  write). Overridable by `CIVITAI_*` env vars (`CIVITAI_TOKEN`,
  `CIVITAI_BASE_URL`). The default base URL is `https://civitai.com`.

## How to add a new command

1. Create `internal/cmd/<name>.go` with `func new<Name>Cmd() *cobra.Command`.
   Set `Use`, `Short`, `Long`, `Example`, `Args`, and `RunE`.
2. Register it in `root.go` (`root.AddCommand(new<Name>Cmd())`), or under a
   group constructor (e.g. `newAppCmd`) for a subcommand.
3. Write `internal/cmd/<name>_test.go` covering the happy path and the error
   paths. Drive it via `NewRootCmd()` + `SetArgs`, or call the constructor and
   capture `SetOut`/`SetErr` buffers.
4. `make ci` must stay green; update `README.md` (command table + a section).

## The manifest-schema-fidelity caveat (IMPORTANT)

`civitai app validate` is a **best-effort LOCAL mirror** of the server-side
`BlockManifestValidator`
(`civitai/civitai → src/server/services/block-manifest-validator.service.ts`).
**The server is the source of truth** at review time.

- `schema/app-block.manifest.schema.json` (embedded via `schema.go`) covers the
  **syntactic** rules only (types, enums, ranges when a field is present).
- The **semantic** rules the JSON Schema cannot express — sandbox trust-tier
  allowlist, `page` ⇒ `iframe`, required iframe sub-fields, the `renderMode`
  tier gate, `targets[].slotId` registry membership — are **ported into Go** in
  `internal/validate/{semantic.go,targets.go}`.
- A few checks are necessarily approximate locally (the slot registry is
  **vendored** in `targets.go`; origin-binding and scope⊆client checks depend
  on per-app server state the CLI can't see, so they are not reproduced).

**The durable fix is a server-side `civitai app validate` endpoint** that calls
the real `BlockManifestValidator` — the faithful contract — with this schema
published as the syntactic half. Until that exists: on any change to a
validation rule, keep the **vendored schema** and the **ported Go checks** in
sync with the server validator, and update `examples_test.go` (which asserts the
shipped example manifests validate clean) + the README.

## Build / test / lint

```bash
make build   # -> bin/civitai (with version ldflags from git describe)
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -s -w .
make lint    # golangci-lint if installed, else go vet
make ci      # tidy + vet + test + build (mirrors GitHub Actions CI)
```

Coverage: `go test ./... -cover` (per-package) or
`go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`.

CI (`.github/workflows/ci.yml`) runs `go vet`, `gofmt -s -l .`, `go test ./...`,
and `go build ./...` on every push to `main` and every PR.

## Release process

Releases are built and published by **goreleaser** from a GitHub Actions
workflow on a `v*` tag push.

1. Make sure `main` is green and `CHANGELOG`-worthy commits are merged.
2. Tag and push:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
3. `.github/workflows/release.yml` runs `goreleaser release`, which:
   - cross-compiles for linux/darwin/windows × amd64/arm64 (no windows/arm64),
   - stamps `version`/`commit`/`date` via `-ldflags` into `cmd/civitai`,
   - produces archives (`.tar.gz`, `.zip` on Windows) + `checksums.txt`,
   - creates the GitHub Release (currently `draft: true` — publish it manually
     after sanity-checking the artifacts),
   - bumps the Homebrew tap formula in `civitai/homebrew-tap`.

Validate the config locally without releasing:

```bash
goreleaser check                 # config is valid
goreleaser release --snapshot --clean   # full dry-run build into ./dist
```

### Secrets the release workflow needs

- `GITHUB_TOKEN` — provided automatically by Actions; used to create the
  release on this repo.
- `HOMEBREW_TAP_GITHUB_TOKEN` — a PAT (or fine-grained token) with write
  access to the **`civitai/homebrew-tap`** repo, so goreleaser can push the
  updated formula. **The tap repo must exist** and the secret must be set, or
  the `brews:` step fails. If you don't want a Homebrew tap yet, comment out
  the `brews:` block in `.goreleaser.yaml`.

## License

Apache License 2.0 (`LICENSE`), matching the main `civitai/civitai` repo.
