# civitai CLI

The command-line interface for [Civitai](https://civitai.com) — a single static
binary for authoring and shipping **App Blocks**.

An **App Block** is a small, sandboxed web app that runs inside Civitai
surfaces (it's served in an iframe; the platform owns the build and the
runtime). The CLI replaces the error-prone "hand-format a ZIP" flow: it
**scaffolds** a correct project, **validates** the manifest against the platform
contract, and **packages/submits** it for review.

> Learn more about App Blocks in the [Civitai App Blocks docs](https://civitai.com/articles)
> (search "App Blocks").

## Install

### Go install (Go 1.25+)

```bash
go install github.com/civitai/cli/cmd/civitai@latest
# installs the `civitai` binary into $(go env GOPATH)/bin
```

### Prebuilt binary

Download a prebuilt binary for your OS/arch from the
[GitHub Releases](https://github.com/civitai/cli/releases) page (linux, macOS,
windows × amd64/arm64), verify it against `checksums.txt`, then put it on your
`PATH`:

```bash
tar xzf civitai_*_linux_amd64.tar.gz
sudo mv civitai /usr/local/bin/
civitai version
```

### Homebrew

> ⚠️ Not available yet — the Homebrew tap is still being set up. Use **Go install**
> or a **prebuilt binary** (above) for now. Once the tap is live:
>
> ```bash
> brew install civitai/tap/civitai
> ```

## Quickstart

```bash
# 1. Authenticate once (browser device login; or `civitai login --token <t>`).
civitai login

# 2. Scaffold a ready-to-build App Block (batteries-included page-money default).
civitai app create my-app
cd my-app

# 3. Edit your app, then check the manifest before submitting.
civitai app validate

# 4. Package + submit for review (uploads with your stored token by default).
civitai app submit
```

Enable shell completion (optional):

```bash
source <(civitai completion bash)   # bash; see `civitai completion --help` for zsh/fish/powershell
```

## Command reference

| Command | What it does |
| --- | --- |
| `civitai login [--token <t>] [--no-browser]` | Browser OAuth device login by default (stores auto-refreshing tokens); `--token` stores a personal API key instead. Config at `~/.config/civitai/config.yaml`, 0600. Also reads `CIVITAI_TOKEN`. |
| `civitai whoami` | Verify the stored token; print the authenticated user. |
| `civitai app create [name] [dir] [--template static\|page-vite\|page-money] [--dir <path>] [--name <display>]` | **The friendly happy path.** Scaffold a ready-to-build App Block, defaulting to the batteries-included `page-money` SDK template (default dir `./<slug>`). |
| `civitai app init [name] [dir] [...]` | Same scaffolder as `create` with a no-build `static` default (back-compat alias). |
| `civitai app validate [dir] [--strict]` | Best-effort local pre-check of `block.manifest.json`; emits non-fatal warnings (`--strict` fails on them). See [Validate fidelity](#validate-fidelity). |
| `civitai app submit [dir] [--package-only] [--out f.zip] [--skip-validate]` | Validate + package the source tree + upload it with your stored token (or, with no token, write the bundle + print next steps). |
| `civitai version` | Print version / commit / build date. |
| `civitai completion [shell]` | Generate a shell-completion script. |

Run `civitai help`, `civitai app --help`, or `civitai <command> --help` for the
full details and examples.

### Templates

- **`static`** — a no-build page block (`index.html` + a tiny `app.js`,
  `block.manifest.json` with `page:{}`, no build step).
- **`page-vite`** — a Vite + React page block with config-as-code build fields
  (`buildCommand: "npm run build"` + `outputDir: "dist"`).
- **`page-money`** — a Vite + React + TypeScript full-page (W10) **money-path**
  block wired to the published App SDK (`@civitai/blocks-react` +
  `@civitai/app-sdk`): prompt → estimate → lazy consent → submit → poll → real
  Buzz spend, via `useBuzzWorkflow` / `useRequestConsent` / `useBlockResize`
  (never raw `postMessage`). Ships a `dev:harness` mock host, `.env.*` allowed
  parent-origin config, and a unit-test stub. Run `npm run dev:harness` (plain
  `npm run dev` renders blank without a host).

### Examples

Two real example manifests live under [`examples/`](examples/) (copied from the
shipping `civitai-block-*` apps) — a good reference for a correct manifest:

- [`examples/buzz-generator.block.manifest.json`](examples/buzz-generator.block.manifest.json)
- [`examples/notepad.block.manifest.json`](examples/notepad.block.manifest.json)

Both validate clean (`examples_test.go` asserts this so the claim stays true).

## Validate fidelity

`civitai app validate` is a **best-effort LOCAL mirror** of the platform's
approve-time validator (`BlockManifestValidator`). **The server is the source of
truth** at review time — passing `validate` locally is a strong pre-check, not a
guarantee of approval.

It checks `block.manifest.json` against a **vendored JSON Schema**
([`schema/app-block.manifest.schema.json`](schema/app-block.manifest.schema.json),
syntactic shape) **plus the ported semantic rules** the server runs (sandbox
trust-tier allowlist, `page` ⇒ `iframe`, required iframe sub-fields, the
`renderMode` tier gate, `targets[].slotId` registry membership) and structural
project checks. A few checks are necessarily approximate locally (the slot
registry is vendored; per-app origin-binding/scope checks the CLI can't see are
not reproduced).

The **durable fix** is a server-side `civitai app validate` endpoint that calls
the real `BlockManifestValidator` (the faithful contract), with this schema
published as the syntactic half. See [`CLAUDE.md`](CLAUDE.md) for the full
caveat and how the vendored schema + Go checks are kept in sync.

## Submit & auth

`civitai login` (no flags) runs the **OAuth device-authorization grant**: it
prints a URL + a short code, you approve in your browser, and the CLI stores a
short-lived access token (1h) plus a refresh token (30d) that it rotates
automatically before requests and once on a `401`. `civitai login --token <key>`
stores a personal API key instead (no refresh). `CIVITAI_TOKEN` overrides the
stored credential (treated as a personal key).

`civitai app submit`:

- always **validates + packages** the canonical source ZIP, then
- **uploads it with your stored token** to the token-authenticated route
  `POST /api/v1/blocks/submit-version` (`Authorization: Bearer`). OAuth tokens
  refresh transparently. Set `CIVITAI_SUBMIT_PATH` to override the route.
- With **no token configured** (and not `--package-only`), it instead writes the
  `.zip` and prints the next steps (`civitai login`, or web upload at
  `/apps/submit`).

`--package-only` always just writes the `.zip` and stops.

## Configuration

| Setting | Config key | Env var | Default |
| --- | --- | --- | --- |
| Personal API key | `token` | `CIVITAI_TOKEN` | — |
| OAuth tokens (device login) | `auth_kind`, `access_token`, `refresh_token`, `token_expiry`, `scope` | — | — |
| API base URL | `base_url` | `CIVITAI_BASE_URL` | `https://civitai.com` |
| Submit endpoint | — | `CIVITAI_SUBMIT_PATH` | `/api/v1/blocks/submit-version` |

Config lives at `~/.config/civitai/config.yaml` (honours `XDG_CONFIG_HOME`),
written owner-readable only.

## Troubleshooting

- **`no token configured`** — run `civitai login` (or set `CIVITAI_TOKEN`).
- **`unauthorized (401)`** — your token is invalid/expired. OAuth tokens refresh
  automatically; if the refresh token has also expired, run `civitai login`
  again. For a personal key, create a new one at
  `https://civitai.com/user/account` and `civitai login --token <key>`.
- **`forbidden (403)` / `service unavailable (503)`** — your account may lack
  App Blocks access while the feature is gated.
- **`validation failed`** — read each `- ...` line; fix the manifest, or pass
  `--skip-validate` to package anyway (the server will still re-validate).
- **`<dir> is not empty — refusing to overwrite`** — `app init` won't clobber an
  existing directory; pick a new name or remove the directory.

## Development

```bash
make ci      # go mod tidy + vet + test + build (mirrors CI)
make test
make build   # -> bin/civitai
make fmt
go test ./... -cover
```

- **Language:** Go 1.25, [Cobra](https://github.com/spf13/cobra) (commands) +
  [Viper](https://github.com/spf13/viper) (config).
- **Layout / conventions / how to add a command / release process:** see
  [`CLAUDE.md`](CLAUDE.md).
- **Contributing:** see [`CONTRIBUTING.md`](CONTRIBUTING.md).

CI (`.github/workflows/ci.yml`) runs `go vet`, `gofmt -s -l .`, `go test ./...`,
and `go build ./...` on every push/PR.

## Releasing

Releases are built by [goreleaser](https://goreleaser.com) from a GitHub
Actions workflow on a `v*` tag push:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This cross-compiles for linux/darwin/windows × amd64/arm64, stamps
version/commit/date, and publishes a GitHub Release with archives +
`checksums.txt` plus a Homebrew tap bump. See [`CLAUDE.md`](CLAUDE.md) for the
full process and the secrets it needs (`HOMEBREW_TAP_GITHUB_TOKEN`).

## License

[Apache License 2.0](LICENSE).
