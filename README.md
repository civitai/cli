# civitai CLI

The unified command-line interface for [Civitai](https://civitai.com) — a
single static binary in the `gh` / `kubectl` / `stripe` mold, with room to grow.

Its first feature group is **App Blocks authoring** under `civitai app`. It
replaces the confusing "hand-format a ZIP" flow: the CLI **generates** the
correct project shape, **validates** the manifest against the platform contract,
and **packages/submits** it for review.

## Install

### From source (Go 1.25+)

```bash
go install github.com/civitai/cli/cmd/civitai@latest
# binary is installed as `civitai`
```

### Build locally

```bash
make build          # -> bin/civitai
make install        # -> $GOBIN/civitai
```

### Homebrew

A [goreleaser](.goreleaser.yaml) config publishes binaries + a Homebrew tap
formula on tagged releases:

```bash
brew install civitai/tap/civitai   # once the tap repo is published
```

## What is an App Block?

A block is a **sandboxed static web app** (an iframe serving static JS/HTML/CSS).
The platform owns the build (runs `npm ci` + your build command → static
output, or serves static as-is) and the runtime. The **only mandatory file is
`block.manifest.json`**. Server-owned fields (`iframe.src`, `trustTier`) are
assigned by the platform — your manifest must not set them.

## Command surface

```
civitai
├── app
│   ├── init [name] [--template static|page-vite] [--from <slug>]
│   ├── validate [dir]
│   └── submit [dir] [--package-only] [--out file.zip] [--skip-validate]
├── login [--token <token>]
└── whoami
```

### `civitai app init`

Scaffolds a correct, ready-to-build block project (templates are embedded in the
binary with `go:embed`):

- **`static`** — a no-build page block (`index.html` + a tiny `app.js`,
  `block.manifest.json` with `page:{}`, no build step).
- **`page-vite`** — a Vite + React page block with the **config-as-code** build
  fields `buildCommand: "npm run build"` + `outputDir: "dist"`.

```bash
civitai app init my-block
civitai app init "My Cool Block" --template page-vite
```

`--from <slug>` (fork-to-start) is **not yet wired** — it needs a server
endpoint that returns a published block's source by slug. It prints a clear
"not yet wired" message rather than faking it.

### `civitai app validate`

A **best-effort LOCAL pre-check** that mirrors the platform's approve-time
validator (`BlockManifestValidator`). It catches most rejections before you
submit, but **the server remains the source of truth** — see
[Validate fidelity](#validate-fidelity) below.

It validates `block.manifest.json` against a **vendored JSON Schema**
([`schema/app-block.manifest.schema.json`](schema/app-block.manifest.schema.json),
syntactic shape) **plus the ported semantic rules** the server runs at approve
time and structural project checks:

- manifest present at the project root;
- `buildCommand` + `outputDir` coherence, and `outputDir` is a safe **relative**
  path (no leading `/`, no `..` traversal);
- server-owned `iframe.src` / `trustTier` rejected with a clear message;
- **sandbox** tokens limited to the unverified-tier allowlist
  (`allow-scripts`, `allow-forms`); `allow-same-origin`+`allow-scripts`
  (sandbox escape) rejected explicitly;
- a `page` manifest must declare an `iframe` block, and `renderMode:iframe`
  (the default) requires one too;
- `iframe.minHeight` (40–4000) and `iframe.resizable` are **required** when an
  iframe block is present;
- `renderMode` `inline`/`hybrid` rejected (they need a verified/internal trust
  tier, which the platform only assigns post-submit — `INLINE_REQUIRES_VERIFIED_TIER`);
- `targets[].slotId` must be a **known registered slot** (and not the page slot).

```bash
civitai app validate            # current directory
civitai app validate ./my-block
```

The two real example manifests under [`examples/`](examples/) (buzz-generator,
notepad — copied from the shipping `civitai-block-*` apps) validate clean; a
test asserts this so the claim stays true.

#### Validate fidelity

`validate` is a **local mirror** of the server validator, not the contract
itself. The vendored JSON Schema covers only **syntactic** rules; the server's
**semantic** rules (sandbox trust-tier allowlist, `page`⇒`iframe`, required
iframe sub-fields, `renderMode` tier gate, slot-registry membership) are **ported
into the Go `validate` layer** from
`block-manifest-validator.service.ts`. A few checks are necessarily
approximate locally:

- **`targets[].slotId`** is checked against a **vendored** copy of the slot
  registry (only 4 ids today). If the server adds a slot, the vendored list must
  be updated; until then a manifest using a brand-new slot would false-INVALID
  locally (it still validates correctly server-side).
- Origin-binding (`iframe.src`/`assetBundleUrl` must be on an
  `OauthClient.allowedOrigins`) and the scope⊆client check depend on
  per-app server state the CLI can't see, so they are **not** reproduced.

The durable fix is a **server `civitai app validate` endpoint** that calls the
real `BlockManifestValidator` — the faithful contract. The server companion's
published manifest schema is a first step toward that. Until it exists, keep the
ported checks + vendored schema in sync with the server validator on each change.

### `civitai app submit`

Validates, then packages the **canonical source tree** (manifest + src + build
config — *not* a prebuilt `dist`) into a ZIP that matches the platform's build
recipe and server caps (50 MiB / 2000 files / 10 MiB per file). `.git`,
`node_modules`, and `dist` are excluded — the platform rebuilds from source.

```bash
civitai app submit                 # validate + package + submit (or print next steps)
civitai app submit --package-only  # just write the .zip
```

See **[Submit & auth](#submit--auth-current-state)** for what's automated today.

### `civitai login` / `civitai whoami`

```bash
civitai login --token <token>   # stored in ~/.config/civitai/config.yaml (chmod 600)
civitai whoami                  # verify the token
```

Token can also be supplied via `CIVITAI_TOKEN`. Config keys are overridable by
`CIVITAI_*` env vars (`CIVITAI_BASE_URL`, etc.).

## The vendored manifest schema

[`schema/app-block.manifest.schema.json`](schema/app-block.manifest.schema.json)
is derived from the server-side validator
(`civitai/civitai → src/server/services/block-manifest-validator.service.ts`):
required fields, the scopes enum, page config (incl. positive-integer
`buzzBudgetPerGen`), sandbox tokens, the `contentRating` enum. It is embedded in
the binary (`go:embed`). The schema covers the **syntactic** rules only; the
server's **semantic** rules are ported into the Go `validate` layer (see
[Validate fidelity](#validate-fidelity)).

> **The schema is one step toward a published shared contract — but it is not the
> whole contract.** The JSON Schema cannot express the cross-field / tier-gated
> semantic rules, so the CLI ports those into Go. The durable fix is a
> **server-side validate endpoint** that runs the real `BlockManifestValidator`
> (the faithful contract), with this schema published (e.g. at
> `https://civitai.com/schemas/app-block/v1.json`) as the syntactic half. Until
> then, keep both the vendored schema and the ported Go checks in sync with the
> server validator on each change.

## Submit & auth (current state)

The submit/auth contract is the one **cross-repo dependency** in Phase 1. As of
the investigation (civitai/civitai @ `main`, 2026-06):

- **The live upload route is `POST /api/blocks/submit-version`** and it is
  **session-cookie + moderator** authenticated (`ModEndpoint`). It does **not**
  accept an API key / bearer token. So a fully programmatic `civitai app submit`
  is blocked on a companion server change.
- **The git-push flow** (`blocks.getMyAppRepo`, civitai #2587) provisions a
  scoped Forgejo repo and a push parks a pending review — but it is itself
  session-auth tRPC, and is only available **after the first version has been
  ZIP-approved**.

**What this CLI implements today:**

1. It builds the canonical ZIP and **validates** it locally.
2. If a token *and* a token-accepting submit endpoint are configured
   (`CIVITAI_SUBMIT_PATH`), it uploads directly with `Authorization: Bearer`
   (this is exactly the payload shape the companion endpoint must accept —
   base64 ZIP in `{ "bundleBase64": ... }`).
3. Otherwise it **writes the canonical `.zip` and prints the exact manual next
   steps** (web upload at `/apps/submit`, or the git-push path for updates).

The network/auth layer sits behind small interfaces (`api.Submitter`,
`api.Verifier`) so it is fully testable without a live server.

### Server-side follow-up needed for a clean `submit`

To make `civitai app submit` a one-command programmatic flow, the platform needs
a **token-authenticated** submit endpoint:

- **Endpoint:** a sibling of `POST /api/blocks/submit-version` (or that route
  extended) that accepts `Authorization: Bearer <civitai API key>` instead of a
  session cookie.
- **Body:** the existing `submitVersionSchema` shape — `{ "bundleBase64":
  "<base64 ZIP>" }` (≤ ~72 MiB, the 50 MiB ZIP base64-encoded).
- **Authz:** resolve the API key → user, then apply the same gates the cookie
  route applies (App Blocks flag; the moderator gate stays while App Blocks is
  mod-gated; relax to "is app owner" when the feature widens). Reuse the
  `submitVersion` service unchanged.
- **Response:** the publish-request `{ publishRequestId, slug, version, status }`
  so the CLI can report it.

Once that exists, set `CIVITAI_SUBMIT_PATH` to its path and `civitai app submit`
uploads end-to-end.

## Development

```bash
make ci        # go mod tidy + vet + test + build
make test
make vet
make fmt
```

- **Language:** Go 1.25, [Cobra](https://github.com/spf13/cobra) (commands) +
  [Viper](https://github.com/spf13/viper) (config).
- **Layout:** `cmd/civitai` (entrypoint) · `internal/cmd` (command tree) ·
  `internal/scaffold` (embedded templates) · `internal/validate` (schema +
  structural checks) · `internal/pkgzip` (canonical packaging) · `internal/api`
  (HTTP client) · `internal/config` (Viper) · `internal/manifest` · `schema/`
  (vendored JSON Schema).
- **JSON Schema validation:**
  [`santhosh-tekuri/jsonschema`](https://github.com/santhosh-tekuri/jsonschema).

CI (`.github/workflows/ci.yml`) runs `go vet`, `gofmt -l`, `go test ./...`, and
`go build ./...` on every push/PR.
