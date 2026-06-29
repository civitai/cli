# civitai CLI

The command-line interface for [Civitai](https://civitai.com) — a single static
binary for authoring and shipping **App Blocks**.

An **App Block** is a small, sandboxed web app that runs inside Civitai
surfaces (it's served in an iframe; the platform owns the build and the
runtime). The CLI replaces the error-prone "hand-format a ZIP" flow: it
**scaffolds** a correct project, **validates** the manifest against the platform
contract, and **packages/submits** it for review.

> New here? The
> [Build your first App Block](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md)
> guide is the full end-to-end walkthrough.

> ⚠️ **App Blocks is in a limited, moderator-gated preview (pre-GA).** You can
> install this CLI, `login`, scaffold, validate, and run a block locally right
> now — but **`civitai app submit` requires App Blocks access**. While the
> feature is dark, submission is restricted to **Civitai moderators / the team**:
> a non-moderator account can't submit (or its block won't be reviewed/approved,
> so it won't go live) until App Blocks opens to the public. There is no public
> self-serve "request access" form yet — watch [civitai.com](https://civitai.com) and the
> [issues on this repo](https://github.com/civitai/cli/issues) for the
> general-availability announcement.

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

```bash
brew install civitai/tap/civitai
```

## Quickstart

```bash
# 1. Authenticate once (browser device login; or `civitai login --token <t>`).
civitai login

# 2. Scaffold a ready-to-build App Block (batteries-included page-money default).
civitai app create my-app
cd my-app

# 3. Install deps and run it locally against the mock host (no real Buzz/compute).
#    `npm run dev` alone renders blank — the harness supplies the host.
npm install
npm run dev:harness

# 4. Edit your app; build it, then check the manifest before submitting.
#    (the `static` template has no build step — skip `npm run build`.)
npm run build
civitai app validate

# 5. Package + submit for review (uploads with your stored token by default).
civitai app submit

# 6. Check where your submission is in review / deploy.
civitai app status
```

> Want to drive the **real** backend (real Buzz/compute) before submitting? Mint
> a dev token with `civitai app dev-token` and run `npm run dev:live` — see
> [Local dev loop](#local-dev-loop-harness-mock-vs-live).

> **Submit ≠ live.** `civitai app submit` enters your block into **moderator
> review** — it is **not** published immediately. The lifecycle is
> **submit → review → approve → build + deploy → `https://<blockId>.civit.ai/`**:
> that URL **404s until a moderator approves your submission and the platform
> builds + deploys it** (a few minutes after approval). Until then, track status
> on **`/apps/my-submissions`** (a fresh submission sits at `pending`). See
> [Submit & auth](#submit--auth) for the full flow. (And note App Blocks is a
> gated preview — see the warning above.)

Enable shell completion (optional):

```bash
source <(civitai completion bash)   # bash; see `civitai completion --help` for zsh/fish/powershell
```

## SDK packages

This CLI scaffolds, validates, and submits — but the code your block actually
imports lives in two published npm packages (the `page-money` / `page-vite`
templates wire them for you):

| Package | What it is |
| --- | --- |
| [`@civitai/blocks-react`](https://www.npmjs.com/package/@civitai/blocks-react) | The React hooks + iframe transport block authors call — `useBlockContext`, `useBuzzWorkflow`, `useBlockResize`, the `/ui` component pack, and the `/testing` dev hosts. **Start here for the hook reference.** |
| [`@civitai/app-sdk`](https://www.npmjs.com/package/@civitai/app-sdk) | The framework-agnostic contract under the hooks — manifest types, scope strings, the `postMessage` protocol, and the `defineBlock` validator (`@civitai/app-sdk/blocks`). |

```bash
# Already installed by the scaffold; this is the explicit install line:
pnpm add @civitai/blocks-react @civitai/app-sdk react
```

The full hook-by-hook reference (with snippets) lives in each package's npm
README. For the end-to-end walkthrough, see
[Build your first App Block](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md).

## Command reference

| Command | What it does |
| --- | --- |
| `civitai login [--token <t>] [--no-browser]` | Browser OAuth device login by default (stores auto-refreshing tokens); `--token` stores a personal API key instead. Config at `~/.config/civitai/config.yaml`, 0600. Also reads `CIVITAI_TOKEN`. |
| `civitai whoami` | Verify the stored token; print the authenticated user **and a Capabilities section** (`Spend Buzz`, `Read Buzz balance`) decoded from the token's scope, so a money-path dead end (OAuth login can't spend) is visible before `dev:live`. |
| `civitai buzz` | Show your spendable Buzz balance (blue / green / **yellow** — the generation-spend currency). Needs a full-scope personal API key to read; an OAuth login token can't, and gets a clear "switch to a personal key" message. |
| `civitai app create [name] [dir] [--template static\|page-vite\|page-money] [--dir <path>] [--name <display>]` | **The friendly happy path.** Scaffold a ready-to-build App Block, defaulting to the batteries-included `page-money` SDK template (default dir `./<slug>`). |
| `civitai app init [name] [dir] [...]` | Same scaffolder as `create` with a no-build `static` default (back-compat alias). |
| `civitai app dev-token <slug> [--env]` | **Mint a short-lived (~4h) dev block token for `npm run dev:live`** — calls the moderator-gated mint route with your stored credential, reading scopes from your local `block.manifest.json` (so it works on an unsubmitted slug). Prints the token (`--env` prints `VITE_LIVE_BLOCK_TOKEN=<token>`, paste-ready); warns at mint time if the token is read-only (can't spend). See [Local dev loop](#local-dev-loop-harness-mock-vs-live). |
| `civitai app validate [dir] [--strict]` | Best-effort local pre-check of `block.manifest.json`; emits non-fatal warnings (`--strict` fails on them). See [Validate fidelity](#validate-fidelity). |
| `civitai app submit [dir] [--package-only] [--out f.zip] [--skip-validate]` | Validate + package the source tree + upload it with your stored token (or, with no token, write the bundle + print next steps). |
| `civitai app status [blockId] [--id <pubreq>] [--json]` | Check the review/deploy status of **your own** submissions. No arg lists them all; a `blockId` (app slug) or `--id` shows one in detail (rejection reason if rejected, live URL once deployed). See [Submission status](#submission-status). |
| `civitai app withdraw [pubreq-id] [--id <pubreq>]` | **Withdraw your own pending submission** (the `pubreq_…` id from `civitai app status`). Frees the slug so a fresh `civitai app submit` can replace it. Idempotent; only a `pending` request can be withdrawn. See [Submission status](#submission-status). |
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

### Local dev loop (harness: mock vs live)

A scaffolded App Block is a sandboxed iframe — `npm run dev` alone shows a blank
screen because there's no host to send `BLOCK_INIT`. The `page-money` /
`page-vite` templates ship a dev **harness** (the SDK's
[`@civitai/blocks-react/testing`](https://www.npmjs.com/package/@civitai/blocks-react)
hosts) with two modes:

| Command | Mode | What it does |
|---|---|---|
| `npm run dev:harness` | **mock** (default) | Mounts the SDK **mock host** — synthetic replies, **no real Buzz, no compute, no network.** Safe to spam; drive money/error/insufficient-Buzz UX via on-screen scenarios or `?` URL params. Start here. |
| `npm run dev:live` | **live** | Mounts the SDK **live host** (`createLiveHost`) — forwards the App-Block protocol to the **real Civitai backend** with a pasted dev token (Bearer). **Spends REAL Buzz / real compute.** |

> ⚠️ **`dev:live` works on a pending (un-approved) app.** The dev-token mint
> (`POST /api/v1/blocks/dev-token`) accepts a **pending** slug — right after a
> successful `civitai app submit` (status `pending`) it returns `200` with
> `appId: pending-pubreq_…` and `dev:live` mounts the live host against the
> pending app. For **real generation** you must mint with a **full-scope personal
> API key**; an OAuth (`civitai login`) token mints read-only (`user:read:self`)
> and **cannot spend**. Use `civitai buzz` / `civitai whoami` to confirm your
> credential can spend before a live run.

**Live mode** needs a short-lived dev block token. Mint it with **`civitai app
dev-token`** (the CLI handles the moderator-gated `POST /api/v1/blocks/dev-token`
call with your stored credential — no hand-rolled curl) and paste it into
`.env.development.local` as `VITE_LIVE_BLOCK_TOKEN=`:

```bash
# From your scaffolded project dir (reads scopes from block.manifest.json):
civitai app dev-token my-block --env >> .env.development.local
npm run dev:live
```

`.env.development*` is never committed (`submit` excludes it) and the token is
short-lived (~4h) — re-run `dev-token` when it expires. Mint with a **full-scope
personal API key** for real generation; an OAuth login mints a read-only token
(the command warns you at mint time). With no token, `dev:live` **fails safe**
(renders a notice, never spends). Live v1 covers the money path
(`estimate`/`submit`/`poll`/`cancel`); pickers, checkpoint-set, App-Storage KV,
and in-band Buzz purchase are mock-only.

**Under the hood (the scaffold wires this — you don't configure it):** `dev:live`
routes the live host's backend calls through the **vite dev proxy**
(`server.proxy['/api']`), not straight to `civitai.com`: `createLiveHost` fetches
`/api/...` SAME-ORIGIN against the dev server (`localhost:5186`), and vite proxies
that server-side to civitai with the `Origin` header rewritten to an allowlisted
host. This is load-bearing — a direct cross-origin fetch from `localhost` is both
blocked by CORS preflight and rejected by civitai's tRPC origin gate. The
same-origin proxy + Origin rewrite fixes both. `VITE_LIVE_HOST_ORIGIN` overrides
the proxy target (default `https://civitai.com`).

**Which credential can spend?** Only a full-scope **personal API key** can run a
real `dev:live` generation — the default OAuth login can't:

| Credential | Real `dev:live` generation? | How to get it |
|---|---|---|
| **Personal API key** (full scope) | ✅ **Yes** — estimate → submit → generation → real Buzz | create it **in the web UI** at `civitai.com/user/account`, then `civitai login --token <key>` (a personal key carries AI Services) |
| **`civitai login`** (OAuth, default) | ❌ No — viewer + catalog + app storage only | the `civitai-cli` client has no AI Services scope, so the server strips the spend scope — fine for read/identity `dev:live`, not generation |

You can't mint a personal key over OAuth or the CLI (`apiKey.add` returns 403
without a full-scope session) — create it in the web UI. The dev token always grants
`user:read:self`, so your viewer resolves on either path. For the scope mechanics
behind this, see [Submit & auth](#submit--auth).

Env vars (`VITE_BLOCK_ALLOWED_PARENT_ORIGINS`, `VITE_HARNESS_MODE`,
`VITE_LIVE_BLOCK_TOKEN`, …) and the scenario knobs are documented in depth in the
scaffolded project's own `README.md` and `.env.example` — see
[`internal/scaffold/templates/page-money/README.md.tmpl`](internal/scaffold/templates/page-money/README.md.tmpl).

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
automatically before requests and once on a `401`. It **requests** the scopes
`UserRead | AppBlocksSubmit` (== `33554433`, exactly the `civitai-cli` OAuth
client's `allowedScopes`) — identity plus App-Blocks submit, which gates both
`app submit` and the dev-token mint. It deliberately does **not** request
`AIServicesWrite`: the server's device-flow scope check is all-or-nothing, so
asking for a scope the client doesn't allow would reject the whole login. A
login token therefore drives the **read/identity** `dev:live` paths (viewer,
catalog, app storage) but — for a generation app whose only `ai:write:budgeted`
scope is stripped — **cannot estimate, submit, or spend real Buzz**. For real
generation use a full-scope **personal API key** (see the credential table under
[Local dev loop](#local-dev-loop-harness-mock-vs-live) above), which carries AI
Services.
`civitai login --token <key>` stores a personal API key instead (no refresh).
`CIVITAI_TOKEN` overrides the stored credential (treated as a personal key).

`civitai app submit`:

- always **validates + packages** the canonical source ZIP, then
- **uploads it with your stored token** to the token-authenticated route
  `POST /api/v1/blocks/submit-version` (`Authorization: Bearer`). OAuth tokens
  refresh transparently. Set `CIVITAI_SUBMIT_PATH` to override the route.
- With **no token configured** (and not `--package-only`), it instead writes the
  `.zip` and prints the next steps (`civitai login`, or web upload at
  `/apps/submit`).

`--package-only` always just writes the `.zip` and stops.

### After you submit: review → approve → deploy

A successful `submit` does **not** publish your block — it queues it for
moderator review. The lifecycle is:

1. **submit** → your submission lands at `/apps/my-submissions` with status
   `pending`.
2. **review** → a moderator reviews the manifest + files. They either **approve**
   or **reject** (with a reason you can read inline, then fix and resubmit).
3. **deploy** → on **approval**, the platform builds and deploys your block
   (injects its build recipe → builds the image → deploys → programs the
   `<blockId>.civit.ai` DNS record). A few minutes after approval it serves live
   at **`https://<blockId>.civit.ai/`**.

Before approval, **`https://<blockId>.civit.ai/` 404s** — submitting does not
make the subdomain serve (but `dev:live` works against a pending app — see
[Local dev loop](#local-dev-loop-harness-mock-vs-live)). For the full end-to-end
walkthrough (build → submit → review → deploy), see the
[Build your first App Block](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md)
guide.

**Need to change the bundle while a request is still `pending`?** Withdraw it
first to free the slug, then resubmit:

```text
$ civitai app status                          # find the pubreq_ id
$ civitai app withdraw pubreq_01HZX           # frees the slug
$ civitai app submit                          # resubmit the new bundle
```

`civitai app withdraw <pubreq-id>` (or `--id <pubreq>`) withdraws **your own**
pending publish request. It is **idempotent** (an already-withdrawn request still
returns success) and only a **`pending`** request can be withdrawn — an already
approved/rejected one cannot.

## Submission status

`civitai app status` checks where **your own** submissions are in that lifecycle
without leaving the terminal. It calls the token-authenticated, self-scoped route
`GET /api/v1/blocks/submissions` with your stored credential — you only ever see
your own submissions (the same token that submitted can read its status; OAuth
tokens need the App Blocks submit scope).

With no argument it lists every submission, newest first:

```text
$ civitai app status
BLOCK_ID    VERSION  STATUS    DEPLOY    SUBMITTED   URL
gen-matrix  0.6.0    approved  live      2026-06-22  https://gen-matrix.civit.ai/
my-block    0.2.0    pending   -         2026-06-21  -
old-app     0.1.0    approved  building  2026-06-19  -
```

Pass a `blockId` (app slug) or `--id <pubreq_id>` to see one in detail — including
the **rejection reason** if it was rejected (so you can fix + resubmit) and the
**live URL** once it is approved and deployed:

```text
$ civitai app status gen-matrix
Block ID:         gen-matrix
Version:          0.6.0
Publish request:  pubreq_01HZX
Status:           rejected
Deploy state:     -
Submitted:        2026-06-22 09:05 CDT
Reviewed:         2026-06-22 11:40 CDT

Rejection reason:
  the budgeted scope needs the per-app Sybil cap signed off first

Not live yet — gen-matrix.civit.ai only serves after the app is approved and deployed (deployState 'live').
```

`--json` emits the raw response for scripting. An empty list prints a friendly
"run `civitai app submit`" hint; with no token it points you at `civitai login`.

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
  App Blocks access while the feature is in its gated preview (see the warning at
  the top of this README). Submission is limited to Civitai moderators / the team
  until App Blocks reaches general availability.
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
