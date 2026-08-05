# civitai CLI

> **Browse and download Civitai models, images, and articles — and author,
> validate, and submit App Blocks.** Two paths in one static binary: an
> anonymous **read/download client** for the public API, and the toolchain for
> shipping **Apps**.

> ⚠️ **Apps is in a limited, invite-only beta (pre-GA).** You can install this
> CLI, `login`, scaffold, validate, and run an app locally right now — but
> **`civitai app submit` and `dev:live` require an invite**: submission and
> `dev:live` are limited to **invited beta testers** while the feature is in a
> limited (pre-GA) beta, until Apps opens to the public.
>
> **Anyone can request an invite** — open a request below and we'll review it:

[![Request access](https://img.shields.io/badge/Request%20access-invite--only%20beta-3b82f6?style=for-the-badge&logo=github)](https://github.com/civitai/cli/issues/new?template=request-access.yml)

The command-line interface for [Civitai](https://civitai.com) — a single static
binary that does two things: it's a thin **read/download client** for Civitai's
public API (browse and fetch models, images, and articles — no account needed to
read), and it's the toolchain to **author, validate, and ship Apps**.

An **App** is a small, sandboxed web app that runs inside Civitai
surfaces (it's served in an iframe; the platform owns the build and the
runtime). The CLI replaces the error-prone "hand-format a ZIP" flow: it
**scaffolds** a correct project, **validates** the manifest against the platform
contract, and **packages/submits** it for review.

> New here? The
> [Build your first App](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md)
> guide is the full end-to-end walkthrough.

## Install

Pick whichever fits — **npm** is the most convenient if you already have Node
(App authors usually do); Homebrew is quickest on macOS/Linux; the prebuilt
binary needs no toolchain; `go install` builds from source.

### npm (Node)

A thin wrapper that downloads the matching prebuilt binary for your OS/arch on
install and verifies its sha256 against the release `checksums.txt`:

```bash
npm install -g @civitai/cli
# or run it without installing:
npx @civitai/cli --help
```

### Homebrew (macOS / Linux)

```bash
brew install civitai/tap/civitai
```

### Nix flake

This repo is a [Nix flake](https://nixos.org/manual/nix/stable/command-ref/new-cli/nix3-flake.html),
so you can run or install `civitai` without a Go toolchain (works on
`x86_64`/`aarch64` Linux and macOS):

```bash
# Run without installing:
nix run github:civitai/cli -- models search "sdxl"

# Install into your Nix profile:
nix profile install github:civitai/cli
```

Pin it as an input in your own flake:

```nix
{
  inputs.civitai-cli.url = "github:civitai/cli";

  outputs = { self, nixpkgs, civitai-cli }: {
    # e.g. add to a devShell / home-manager / systemPackages:
    #   civitai-cli.packages.${system}.default
  };
}
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

### Go install (from source, Go 1.25+)

```bash
go install github.com/civitai/cli/cmd/civitai@latest
# installs the `civitai` binary into $(go env GOPATH)/bin
```

## Quickstart: browse & download

Reads are **anonymous** — no `login` needed. Every command takes `--json` to
emit the raw API response for scripting.

```bash
# Search models — filter by base model, type, and sort:
civitai models search --base-model Illustrious --type Checkpoint --sort "Most Downloaded"

# --base-model works on any type, including embeddings (TextualInversion):
civitai models search --type TextualInversion --base-model "SDXL 1.0"

# Inspect a specific model or a specific model version:
civitai models get 4384
civitai model-versions get 128713

# Download a version's file(s) — SHA256-verified, streamed atomically.
# `--layout` routes each file into the right app subfolder (also `a1111`);
# `--dry-run` prints the plan without transferring. Downloads require `civitai login`.
civitai download 128713 --layout comfyui --root ~/ComfyUI
civitai download 128713 --dry-run

# Find and read articles (guides) right in the terminal:
civitai articles search --query "comfyui workflow"
civitai articles get 32680 --content
```

See [Browse the public API](#browse-the-public-api) and
[Download model files](#download-model-files) below for the full command and
flag reference (images, tags, creators, collections, pagination, folder routing,
base-model compatibility checks, and more).

## Quickstart: build an App Block

```bash
# 1. Authenticate once (browser device login; or `civitai login --token <t>`).
civitai login

# 2. Scaffold a ready-to-build App (batteries-included page-money default).
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

> **Submit ≠ live.** `civitai app submit` enters your app into **moderator
> review** — it is **not** published immediately. The lifecycle is
> **submit → review → approve → build + deploy → `https://<blockId>.civit.ai/`**:
> that URL **404s until a moderator approves your submission and the platform
> builds + deploys it** (a few minutes after approval). Until then, track status
> on **`/apps/my-submissions`** (a fresh submission sits at `pending`). See
> [Submit & auth](#submit--auth) for the full flow. (And note Apps is in an
> invite-only beta — see the warning above.)

Enable shell completion (optional):

```bash
source <(civitai completion bash)   # bash; see `civitai completion --help` for zsh/fish/powershell
```

## SDK packages

This CLI scaffolds, validates, and submits — but the code your app actually
imports lives in two published npm packages (the `page-money` / `page-vite`
templates wire them for you):

| Package | What it is |
| --- | --- |
| [`@civitai/blocks-react`](https://www.npmjs.com/package/@civitai/blocks-react) | The React hooks + iframe transport app authors call — `useBlockContext`, `useBuzzWorkflow`, `useBlockResize`, the `/ui` component pack, and the `/testing` dev hosts. **Start here for the hook reference.** |
| [`@civitai/app-sdk`](https://www.npmjs.com/package/@civitai/app-sdk) | The framework-agnostic contract under the hooks — manifest types, scope strings, the `postMessage` protocol, and the `defineBlock` validator (`@civitai/app-sdk/blocks`). |

```bash
# Already installed by the scaffold; this is the explicit install line:
pnpm add @civitai/blocks-react @civitai/app-sdk react
```

The full hook-by-hook reference (with snippets) lives in each package's npm
README. For the end-to-end walkthrough, see
[Build your first App](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md).

## Command reference

| Command | What it does |
| --- | --- |
| `civitai login [--token [<t>]] [--no-browser]` | Browser OAuth device login by default (stores auto-refreshing tokens); `--token <t>` stores a personal API key instead. `--token` with **no value** prints where to create a personal key (`civitai.com/user/account`) and how to re-run — handy when you know you want a personal key but haven't minted one yet. Config at `~/.config/civitai/config.yaml`, 0600. Also reads `CIVITAI_TOKEN`. |
| `civitai whoami [--scopes] [--json]` | Verify the stored token; print the authenticated user **and a Capabilities section** — credential type (**OAuth login** vs **personal API key**), **Read Buzz balance**, and **Spend Buzz** — decoded from the token's scope, so a money-path dead end (OAuth login can't spend) is visible before `dev:live`. `--scopes` also lists every granted scope; `--json` emits the user + `credentialType`/`canReadBalance`/`canSpend`/`scopes` (scriptable). |
| `civitai buzz [--json]` | Show your spendable Buzz balance (**blue / green / yellow**, plus a **total**). Needs a full-scope personal API key to read; an OAuth login token can't, and gets a clear "switch to a personal key" message. `--json` emits `{blue,green,yellow,total}` (scriptable — handy for before/after diffing a `dev:live` spend). |
| `civitai app create [name] [dir] [--template static\|page-vite\|page-money] [--dir <path>] [--name <display>]` | **The friendly happy path.** Scaffold a ready-to-build App, defaulting to the batteries-included `page-money` SDK template (default dir `./<slug>`). |
| `civitai app init [name] [dir] [...]` | Same scaffolder as `create` with a no-build `static` default (back-compat alias). |
| `civitai app dev-token <slug> [--env]` | **Mint a short-lived (~4h) dev block token for `npm run dev:live`** — calls the invite-gated mint route with your stored credential, reading scopes from your local `block.manifest.json` (so it works on an unsubmitted slug). Prints the token (`--env` prints `VITE_LIVE_BLOCK_TOKEN=<token>`, paste-ready); warns at mint time if the token is read-only (can't spend). See [Local dev loop](#local-dev-loop-harness-mock-vs-live). |
| `civitai app dev-tunnel [blockId] [--port] [--tunnel-endpoint] [--idle-timeout]` | **(Pre-GA / dark)** Preview your **local** dev server inside the **real** Civitai host at `civitai.com/apps/dev/<blockId>` — a prod-fidelity inner-dev-loop. Mints an **ephemeral in-memory ssh keypair**, opens a reverse tunnel from your dev port (start `npm run dev:tunnel` first) to the Civitai tunnel endpoint, prints the URL to open, and tears everything down on Ctrl-C or an idle timeout. Before minting it also **pre-flights whether the host can actually embed your dev server** — the host iframes it sandboxed (opaque `null` origin), so a dev server missing `Access-Control-Allow-Origin: *`, missing the `.civit.ai` entry in `allowedHosts`, or sending a framing header that excludes `civitai.com` loads as a blank iframe with no error anywhere. Those are printed as warnings (never fatal) just above the URL, with the `vite.config.ts` fix. Apps scaffolded by `civitai app init --template page-money` already satisfy all of it. Gated behind an Apps-author invite **and** a kill-switch flag that is off today, and the tunnel endpoint is not exposed yet — so it reports "not available" until it ships. |
| `civitai app validate [dir] [--strict] [--json]` | Best-effort local pre-check of `block.manifest.json`; emits non-fatal warnings (`--strict` fails on them). `--json` emits the structured result (`ok`, plus `errors`/`warnings` each with `field`/`message`) for scriptable parsing — still exits non-zero on failure. See [Validate fidelity](#validate-fidelity). |
| `civitai app submit [dir] [--package-only] [--out f.zip] [--skip-validate]` | Validate + package the source tree + upload it with your stored token (or, with no token, write the bundle + print next steps). |
| `civitai app status [blockId] [--id <pubreq>] [--json]` | Check the review/deploy status of **your own** submissions. No arg lists them all; a `blockId` (app slug) or `--id` shows one in detail (rejection reason if rejected, live URL once deployed). See [Submission status](#submission-status). |
| `civitai app metrics <slug> [--from <d>] [--to <d>] [--json]` | **Owner-only analytics for one of your Apps** — installs, runs + Buzz spent, Buzz purchased, and API engagement. Always prints the window the **server** served (it defaults to 30 days and clamps to 366), so a zero is never ambiguous. Needs a **personal API key** (an OAuth login is refused). See [App metrics](#app-metrics). |
| `civitai app withdraw [pubreq-id] [--id <pubreq>]` | **Withdraw your own pending submission** (the `pubreq_…` id from `civitai app status`). Frees the slug so a fresh `civitai app submit` can replace it. Idempotent; only a `pending` request can be withdrawn. See [Submission status](#submission-status). |
| `civitai version` | Print version / commit / build date. |
| `civitai completion [shell]` | Generate a shell-completion script. |

Run `civitai help`, `civitai app --help`, or `civitai <command> --help` for the
full details and examples.

### Templates

- **`static`** — a no-build page app (`index.html` + a tiny `app.js`,
  `block.manifest.json` with `page:{}`, no build step).
- **`page-vite`** — a Vite + React page app with config-as-code build fields
  (`buildCommand: "npm run build"` + `outputDir: "dist"`).
- **`page-money`** — a Vite + React + TypeScript full-page (W10) **money-path**
  app wired to the published App SDK (`@civitai/blocks-react` +
  `@civitai/app-sdk`): prompt → estimate → lazy consent → submit → poll → real
  Buzz spend, via `useBuzzWorkflow` / `useRequestConsent` / `useBlockResize`
  (never raw `postMessage`). Ships a `dev:harness` mock host, `.env.*` allowed
  parent-origin config, and a unit-test stub. Run `npm run dev:harness` (plain
  `npm run dev` renders blank without a host).

### Local dev loop (harness: mock vs live)

A scaffolded App is a sandboxed iframe — `npm run dev` alone shows a blank
screen because there's no host to send `BLOCK_INIT`. The `page-money` /
`page-vite` templates ship a dev **harness** (the SDK's
[`@civitai/blocks-react/testing`](https://www.npmjs.com/package/@civitai/blocks-react)
hosts) with two modes:

| Command | Mode | What it does |
|---|---|---|
| `npm run dev:harness` | **mock** (default) | Mounts the SDK **mock host** — synthetic replies, **no real Buzz, no compute, no network.** Safe to spam; drive money/error/insufficient-Buzz UX via on-screen scenarios or `?` URL params. Start here. |
| `npm run dev:live` | **live** | Mounts the SDK **live host** (`createLiveHost`) — forwards the App protocol to the **real Civitai backend** with a pasted dev token (Bearer). **Spends REAL Buzz / real compute.** |

> ⚠️ **`dev:live` works on a pending (un-approved) app.** The dev-token mint
> (`POST /api/v1/blocks/dev-token`) accepts a **pending** slug — right after a
> successful `civitai app submit` (status `pending`) it returns `200` with
> `appId: pending-pubreq_…` and `dev:live` mounts the live host against the
> pending app. For **real generation** you must mint with a **full-scope personal
> API key**; an OAuth (`civitai login`) token mints read-only (`user:read:self`)
> and **cannot spend**. Use `civitai buzz` / `civitai whoami` to confirm your
> credential can spend before a live run.

**Live mode** needs a short-lived dev block token. Mint it with **`civitai app
dev-token`** (the CLI handles the invite-gated `POST /api/v1/blocks/dev-token`
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
`civitai-block-*` dogfood apps). Read them for manifest **shape** — between them
they cover the required fields, `$schema` wiring, the `page`/`iframe` blocks, and
scope declarations with justifications:

- [`examples/buzz-generator.block.manifest.json`](examples/buzz-generator.block.manifest.json)
- [`examples/notepad.block.manifest.json`](examples/notepad.block.manifest.json)

The *values* are those apps' own choices, not recommendations. In particular don't
copy `buzz-generator`'s `page.buzzBudgetPerGen` — it is a safety ceiling against a
malicious or compromised app, not an estimate of one run, so size your own from the
field's description in the [canonical schema](https://civitai.com/schemas/app-block/v1.json)
(`notepad` doesn't take the budgeted scope, so it has no budget at all).

Both validate clean (`examples_test.go` asserts this so the claim stays true) —
schema conformance only, which says nothing about whether a value is well-sized.

## Browse the public API

Beyond authoring Apps, the CLI is a thin client for Civitai's **public read REST
API** (`GET /api/v1/**`). These subcommands **work anonymously** — no `login`
needed, because the data is public — but when you're logged in your stored token
is sent automatically (pass `--anon` to force a no-auth request). Every command
also takes `--json` to print the **raw API JSON response** for scripting.

| Command | What it does | Notable flags |
| --- | --- | --- |
| `civitai models search` | Search models (`GET /api/v1/models`) | `--query`, `--tag`, `--username`, `--type`, `--base-model` (repeatable), `--sort`, `--period`, `--nsfw`; paging `--limit` (≤100), `--page`, `--cursor` |
| `civitai models get <id>` | Get one model by id | `--json`, `--anon` |
| `civitai model-versions get <id>` | Get a model version by id (alias `mv`) | `--json`, `--anon` |
| `civitai model-versions by-hash <hash>` | Look up a model version by file hash (AutoV2, SHA256, …) | `--json`, `--anon` |
| `civitai download <version-id>` | Download a model version's file(s) | `--model`, `--file`, `--all`, `--out`, `--out-dir`, `--layout`, `--root`, `--for-base`, `--no-verify`, `--force`, `--anon` |
| `civitai images search` | Search images (`GET /api/v1/images`) | `--model-id`, `--model-version-id`, `--post-id`, `--username`, `--base-model` (repeatable), `--type` (image/video/audio), `--sort`, `--period`, `--nsfw`, `--meta` (include generation metadata); paging `--limit` (≤200), `--page`, `--cursor` |
| `civitai tags search` | Search model tags | `--query`; paging `--limit` (≤200), `--page` |
| `civitai creators search` | Search creators | `--query`; paging `--limit` (≤200), `--page` |
| `civitai users get <username-or-id>` | Look up a user via public search (a number = exact id; a name = exact-username match, else it lists close matches) | `--json`, `--anon` |
| `civitai articles search` | Search articles (`GET /api/v1/articles`) | `--query`, `--tags`, `--username`, `--sort`, `--nsfw`; paging `--limit` (≤100), `--cursor` |
| `civitai articles get <id>` | Get one article by id (`--content` renders the article body as readable text/markdown) | `--content`, `--json`, `--anon` |
| `civitai collections search` | Search public collections (`GET /api/v1/collections`) | `--query`, `--sort`, `--nsfw`; paging `--limit` (≤100), `--cursor` |
| `civitai collections get <id>` | Get one collection by id | `--json`, `--anon` |

**Pagination.** List commands print a compact footer with the next-page hint.
`models`/`images` support both shallow `--page` and deep `--cursor` paging (the
API caps `page*limit` at 1000 and 429s beyond it — prefer `--cursor` for deep
paging); `articles`/`collections` are **cursor-only** (keyset feed — no
`--page`); `tags`/`creators` are `--page`-only. Each endpoint caps `--limit`
(models/articles/collections 100; images/tags/creators 200).

```bash
civitai models search --query "pony" --limit 5
civitai models get 4384
civitai model-versions by-hash 5D8D26E2A6
civitai articles get 32680
civitai articles get 32680 --content   # render the article body (the guide) as readable text/markdown
civitai images search --model-id 4384 --sort "Most Reactions" --json   # raw JSON for scripting
```

**Filtering by base model.** `--base-model` is repeatable and maps to the REST
`baseModels` filter (an OR across the values). It's the key discovery filter for
things `--type` can't separate — e.g. video checkpoints all share
`--type Checkpoint` and are distinguished only by base model. It works on both
`models search` and `images search`:

```bash
civitai models search --type Checkpoint --base-model "Wan Video 2.2 T2V-A14B"
civitai models search --base-model Pony --base-model Illustrious --limit 20
# images too — find recent-popular images generated with a given base model:
civitai images search --base-model "Krea 2" --sort "Most Reactions" --period Week
civitai images search --type video --sort "Most Reactions"   # videos only
```

**Generation metadata (`--meta`).** By default the image list is a compact table
without generation data (matching the API, which omits `meta` unless asked). Add
`--meta` to include each image's prompt, sampler, cfg, steps, seed, and model —
rendered as an indented detail block per image (the table can't hold a prompt).
Images whose uploader chose to hide their generation data show
`meta: (hidden by uploader)`. With `--json`, `--meta` adds the raw `meta` object
to each item.

```bash
civitai images search --nsfw --sort "Most Reactions" --period Month --meta
civitai images search --model-version-id 128713 --meta --json | jq '.items[].meta'
```

The human table includes a `BASE MODEL` column (the base model each image was
generated with, when the API reports one; `-` when it doesn't), so you can see
the ecosystem at a glance without dropping to `--json`.

**`--sort` is ignored with `--model-id`.** The REST API returns images for a
given `modelId` in its own default order regardless of `sort`, so
`images search --model-id <id> --sort …` prints a one-line note on stderr and the
results are NOT re-sorted. (`--model-version-id` is unaffected — it honours
`--sort`.)

**Non-weights file marker.** In the human (non-`--json`) output of
`models get` and `model-versions get`, a version whose **primary file is not
model weights** (`type != "Model"`) is tagged with its actual file type — e.g.
`[Archive]` (a "Workflows" model's downloadable deliverable), `[Training Data]`,
or `[Other]` — so you can see at a glance that the version's file isn't weights.
It's purely informational: any file type still downloads. `--json` output is an
unchanged raw passthrough.

## Download model files

`civitai download` fetches the file(s) of a model **version**. Identify the
version deterministically by its numeric **version id**, or resolve a model's
default (first) published version with `--model`:

```bash
civitai download 128713                       # the version's primary file → ./<server-name>
civitai download --model 4384                 # resolve model 4384's default version, then download its primary file
civitai download --model 4384 --dry-run       # print the plan (files, sizes, hashes, targets) — download nothing
civitai download 128713 --out ./dreamshaper.safetensors
civitai download 128713 --file vae --out-dir ./models   # pick a file by name; write into a dir
civitai download 691639 --file 1234567                  # pick one of two same-named files by its file id
civitai download 128713 --all --out-dir ./models        # every file in the version
civitai download 128713 --all --layout comfyui --root ~/ComfyUI   # route each file to its type folder
civitai download 128713 --layout a1111 --for-base "SDXL 1.0"      # A1111 layout + base-model compat warning
```

> **Downloads require authentication.** Every model-file download needs a token —
> even a small public embedding 401s anonymously. Run `civitai login` first. The
> read/search commands work anonymously; downloads do not. `--anon` is meaningful
> for the read commands, not for `download`.

Behavior:

- **Identifier** — exactly one of the positional `<version-id>` or `--model <model-id>` is required (no numeric-ambiguity guessing).
- **`--model` resolves the default version** — the model's default (first published) version; its primary file is downloaded regardless of file type. Any model type works, including a `type: Workflows` model whose deliverable is a downloadable `Archive`.
- **`--dry-run`** — resolve the version + selected file(s) and print the plan (each file's name, size, SHA256, resolved target path, and whether authentication will be required) then exit `0`, transferring nothing and creating no file (not even a `.part`). Works with `--file`, `--all`, `--model`, `--out`, `--out-dir`, and `--layout`/`--root` (the plan shows the routed target paths).
- **File selection** — defaults to the version's **primary** file. `--file` selects one file by **numeric file id** (the version's `files[].id`) or by **name** (exact, else a unique case-insensitive substring; ambiguous/none errors and lists the candidate files with their ids). `--all` downloads every file.
- **Same-named files (no silent overwrite)** — a version can ship two files that share a name (e.g. Flux Dev's fp16 **and** fp8, both `flux_dev.safetensors`). Selecting that shared name with `--file` is **ambiguous** and errors, listing both files with their ids — pass the **numeric id** to pick exactly one (`--file 1234567`; the id is shown by `--dry-run` and in the error). `--all` **refuses** to run when two selected files would resolve to the **same on-disk path** (which would silently clobber one) — it fails *before* transferring anything, lists the colliding files with their ids/sizes, and tells you to pick one with `--file <id>` (or write them to separate paths). No download ever silently overwrites another.
- **Output** — `--out <path>` sets an exact target path (single file only). `--out-dir <dir>` writes server-named files into a directory (works with `--all`). Parent directories are created as needed. Default is the server-provided filename in the current directory.
- **Type-aware folder routing (`--layout`)** — `--layout <a1111|comfyui>` writes each file into the correct subfolder for that app, keyed by the file/model **type**, under `--root <dir>` (default `.`). This fixes the footgun where `--all --out-dir X` dumps a **bundled VAE into the checkpoint folder** and pollutes the model dropdown: with `--layout`, the checkpoint lands in the checkpoints folder and the VAE in the VAE folder. `--layout` is mutually exclusive with `--out`/`--out-dir`; `--root` only applies with `--layout`. An unmapped type (Poses, Wildcards, Archive, …) is written to `--root` with a stderr note rather than silently misplaced. The routed folder maps:

  | Civitai type | A1111 / Forge | ComfyUI |
  |---|---|---|
  | Checkpoint | `models/Stable-diffusion` | `models/checkpoints` |
  | VAE (standalone **or** bundled) | `models/VAE` | `models/vae` |
  | LORA / LoCon / DoRA | `models/Lora` | `models/loras` |
  | TextualInversion (embedding) | `embeddings` | `models/embeddings` |
  | Hypernetwork | `models/hypernetworks` | `models/hypernetworks` |
  | Controlnet | `models/ControlNet` | `models/controlnet` |
  | Upscaler | `models/ESRGAN` | `models/upscale_models` |

  (Sources: the [AUTOMATIC1111 wiki](https://github.com/AUTOMATIC1111/stable-diffusion-webui/wiki/Command-Line-Arguments-and-Settings) + the sd-webui-controlnet `models/ControlNet` default; the [ComfyUI models docs](https://docs.comfy.org/development/core-concepts/models).)
- **Mis-file warning (without `--layout`)** — when `--all` would place files of **differing types** into one directory (the mis-file footgun), the CLI prints a one-line stderr warning naming the off-type file(s) and suggesting `--layout`. It's a warning, not an error; a single-type download stays quiet.
- **Base model + compatibility (`--for-base`)** — the version's **base model** is always shown in the plan/output. `--for-base "<baseModel>"` warns on stderr when the version's base model is in a **confidently different family** than your target (e.g. an `SD 1.5` embedding like EasyNegative downloaded for an `SDXL 1.0` model → won't work; the wrong VAE → black images). The check is conservative — it groups the common bases into architecture families (SD1.x, SD2.x, the SDXL family [SDXL/Pony/Illustrious/NoobAI, treated loosely], SD3, Flux, video, …) and only warns on an architecture-level mismatch, never on near-neighbours (Pony vs Illustrious) or unclassifiable bases.
- **Streaming + atomicity** — the body streams to `<target>.part` and is renamed into place only on success, so an interrupted run never leaves a truncated final file. Large files (10+ GB) are never buffered in memory. TTY-aware progress is printed to **stderr**. The Civitai download URL 302-redirects to signed storage; the CLI follows it.
- **Auth** — your stored login token (`civitai login`) or `CIVITAI_TOKEN` is used automatically; **Civitai requires a token to download any model file, even public ones**, so an anonymous download gets an actionable 401 (`401` → run `civitai login`; `403` → the file is gated for your account). `--anon` forces no token.
- **Transient-failure retry (reads)** — the read endpoints (search / model / version / images / tags / creators / users / articles / collections) retry a transient `502`/`503`/`504` or network error a few times with exponential backoff (with jitter), noting each retry on stderr. A `429` is retried **only** when it carries a `Retry-After` header (a genuine throttle, honored up to a cap); a `429` **without** `Retry-After` is Civitai's deterministic deep-paging limit and is surfaced immediately with the hint to use `--cursor` instead of `--page`. The download **stream** is not retried mid-transfer.
- **Integrity (default on)** — the streamed bytes are verified against the file's `SHA256`; a mismatch deletes the `.part` and fails. `--no-verify` skips it; a file with no published SHA256 downloads with a warning (not a hard failure). Note that **SHA256 verifies integrity (the bytes match what the API advertised), not authenticity** — it proves the download wasn't corrupted or truncated in transit, but a compromised source that advertises a matching hash for malicious bytes cannot be detected by the hash alone. Only download models from creators you trust.
- **Pickle/archive safety note** — when a downloaded file has a pickle/executable extension (`.ckpt`, `.pt`, `.pth`, `.bin`, `.pickle`, `.pkl`) or an archive extension (`.zip`, `.tar`, `.tar.gz`, `.tgz`, `.rar`, `.7z`), the CLI prints a one-line stderr note: these formats **can execute arbitrary code when loaded** by ComfyUI/A1111/`torch.load`, and they land in folders those apps auto-scan. `safetensors` and image files are inert and get no note. The note is informational — it never blocks the download.
- **ControlNet preprocessor note** — when the parent model is a **ControlNet**, the CLI prints a one-line stderr note: a ControlNet model needs a matching **preprocessor/annotator** (e.g. the ComfyUI `comfyui_controlnet_aux` custom node — OpenPose/Canny/Depth) to derive the control image from your input, and that preprocessor is a **separate install, not hosted on Civitai**. The note is informational — it never blocks the download.
- **Idempotency** — an already-present target (that verifies, or with `--no-verify`) is skipped with a note; `--force` re-downloads.
- **Any file type downloads** — the selected/primary file is downloaded whatever its `type` (`Model` weights, a `type: Workflows` model's `Archive`, training data, or other artifacts). The human `models get` / `model-versions get` output tags a non-weights primary file with its type (e.g. `[Archive]`) purely for information; it never blocks a download.

## Scripting with `--json`

Every read subcommand takes `--json`, which prints the **raw `/api/v1/...` REST
response** — a stable passthrough, not a CLI-invented shape. So the field schema
is exactly the public Site API's; keep the
[REST field reference](https://developer.civitai.com/site/reference/) open
(e.g. [models](https://developer.civitai.com/site/reference/models),
[model-versions](https://developer.civitai.com/site/reference/model-versions))
rather than reverse-engineering fields with `jq keys`.

Two properties make the output safe to pipe:

- **`--json` stdout is pure JSON** — nothing else is written to stdout, so
  `... --json | jq -e .` always parses.
- **Errors go to stderr with a non-zero exit** — a failed call writes the error
  to **stderr**, exits non-zero, and prints **nothing to stdout**, so `jq` never
  sees error prose. For example `civitai model-versions get 999999999 --json`
  exits `4` with `Error: not found (404): Model not found` on stderr and an empty
  stdout.

### Cursor pagination loop

For deep paging use `--cursor` (**not** `--page` — the API caps `page*limit` at
1000 and 429s beyond it). Read `.metadata.nextCursor` from each response and feed
it back via `--cursor`; stop when it's absent/null:

```bash
export CIVITAI_NO_UPDATE_CHECK=1
cursor=""
while :; do
  page=$(civitai models search --type LORA --base-model Illustrious \
           --sort "Most Downloaded" --limit 5 ${cursor:+--cursor "$cursor"} --json) || break
  echo "$page" | jq -r '.items[].id'                 # do your work here
  cursor=$(echo "$page" | jq -r '.metadata.nextCursor // empty')
  [ -z "$cursor" ] && break                          # no more pages
done
```

### Clean output for pipelines

The CLI runs a background check for a newer release and prints a nag to
**stderr**. In scripts, silence it with `CIVITAI_NO_UPDATE_CHECK=1` (env) or
`--no-update-check` (flag). Either way stdout stays pure JSON — the nag never
touches stdout — but suppressing it keeps stderr clean for logs.

### Gotchas

- **SHA256 is UPPER-case** in the API/`--json` (e.g.
  `42BA94DF20CC0F4E6DF46E3C294587A2F8CF133BF0134185884EE1C9C5E108C4`), while
  `sha256sum` emits lowercase. Case-fold before comparing if you roll your own
  verify (`civitai download`'s built-in check is already case-insensitive):
  `[ "$(echo "$api_sha" | tr A-Z a-z)" = "$(sha256sum file | cut -d' ' -f1)" ]`.
- **`models search` already embeds `.modelVersions[]`** — each item carries its
  full versions, including `files[].hashes.SHA256` and `trainedWords`. If you're
  iterating search results you usually **don't** need a follow-up
  `model-versions get` per version.
- **Creator + model-level download counts live only in the search response.**
  `model-versions get <id>` returns a **version**, whose `.model` is just
  `{name, type, nsfw, poi}` — no `creator`, no model `stats.downloadCount`. If
  you started from a version and need those, fetch them from `models search`
  / `models get` and join on the model id (`.modelId` on the version).

### Worked example — top LoRAs for a base model, then plan a download

Search → pick versions with `jq` → hand each version id to `download` with app
folder routing. `--dry-run` prints the plan (files, sizes, hashes, target paths)
without transferring, so this snippet is safe to copy-paste:

```bash
export CIVITAI_NO_UPDATE_CHECK=1
civitai models search --type LORA --base-model Illustrious \
    --sort "Most Downloaded" --limit 3 --json |
  jq -r '.items[].modelVersions[0].id' |
  while read -r vid; do
    civitai download "$vid" --layout comfyui --root ~/ComfyUI --dry-run
  done
```

Drop `--dry-run` (and `civitai login` first) to actually fetch the files —
`--layout comfyui` routes each into its ComfyUI type folder.

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

It also mirrors one **build-time** rule, because the failure it prevents is
otherwise an opaque server-side "build failed": your committed **lockfile must
match the package manager the platform derives from `buildCommand`**. The
platform build installs *strictly* from the lockfile — no registry re-resolve
fallback — so `"buildCommand": "pnpm run build"` needs `pnpm-lock.yaml`,
`"yarn run build"` needs `yarn.lock`, and `npm run …` / `vite build` /
`npx vite build` / an omitted `buildCommand` all need `package-lock.json`. A
mismatch or a missing lockfile is a hard `validate` error; an *extra* unused
lockfile is a warning. Apps with no `package.json` are static — the platform
never installs for them and they are never flagged.

The **durable fix** is a server-side `civitai app validate` endpoint that calls
the real `BlockManifestValidator` (the faithful contract), with this schema
published as the syntactic half. See [`AGENTS.md`](AGENTS.md) for the full
caveat and how the vendored schema + Go checks are kept in sync.

## Submit & auth

`civitai login` (no flags) runs the **OAuth device-authorization grant**: it
prints a URL + a short code, you approve in your browser, and the CLI stores a
short-lived access token (1h) plus a refresh token (30d) that it rotates
automatically before requests and once on a `401`. It **requests** the scopes
`UserRead | AppBlocksSubmit` (== `33554433`, exactly the `civitai-cli` OAuth
client's `allowedScopes`) — identity plus Apps submit, which gates both
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

A successful `submit` does **not** publish your app — it queues it for
moderator review. The lifecycle is:

1. **submit** → your submission lands at `/apps/my-submissions` with status
   `pending`.
2. **review** → a moderator reviews the manifest + files. They either **approve**
   or **reject** (with a reason you can read inline, then fix and resubmit).
3. **deploy** → on **approval**, the platform builds and deploys your app
   (injects its build recipe → builds the image → deploys → programs the
   `<blockId>.civit.ai` DNS record). A few minutes after approval it serves live
   at **`https://<blockId>.civit.ai/`**.

Before approval, **`https://<blockId>.civit.ai/` 404s** — submitting does not
make the subdomain serve (but `dev:live` works against a pending app — see
[Local dev loop](#local-dev-loop-harness-mock-vs-live)). For the full end-to-end
walkthrough (build → submit → review → deploy), see the
[Build your first App](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md)
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
tokens need the Apps submit scope).

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

The unfiltered listing is **capped server-side at 100 rows**, and the API returns
no cursor and no total — so there is no way to page and no way to know how many
were dropped. When a full-length page comes back the CLI says so on **stderr**
rather than presenting it as your complete history:

```text
note: showing the newest 100 submissions — the API caps this listing and offers no way to page, so older submissions may exist but are not listed. Look up a specific app with `civitai app status <blockId>`.
```

That is an inference (a page that is exactly full is indistinguishable from one
that was cut off), so it says *may*. A per-app lookup — `civitai app status
<blockId>` — is **not** affected: the server narrows to the slug before applying
the cap.

`--json` emits the raw response for scripting. An empty list prints a friendly
"run `civitai app submit`" hint; with no token it points you at `civitai login`.
Notes like the cap caveat go to stderr, so `--json` stdout stays pure and the
exit code stays 0.

## App metrics

`civitai app metrics <slug>` shows the owner-only analytics for one of **your**
App Blocks. The slug is resolved to its `appBlockId` through your own
submissions, so analytics exist only once a version has been **approved** — an
app still in review reports that instead of an empty dashboard.

```text
$ civitai app metrics gen-matrix --from 2026-05-01 --to 2026-08-03
App:          gen-matrix
Window:       2026-05-01 00:00 UTC → 2026-08-03 00:00 UTC
Granularity:  week

Installs
  Total   12
  Active  9

Runs
  Count       20
  Buzz spent  65

Buzz purchased
  Purchases  3
  Buzz       15000
  Gross      $14.97

App loads
  Impressions       124
  Unique viewers    12
  Signed-out loads  40

Engagement
  API calls     26
  Active users  2
  Error rate    3.8%

  Top scopes:
    ai:write:budgeted  20

  Top endpoints:
    /api/v1/blocks/me  4
```

Three things are worth knowing, because each one otherwise produces a
believable-but-wrong reading:

- **The window is always printed, and it comes from the server.** The API
  defaults to the last **30 days** and clamps any request to **366 days**, so a
  real app with 20 runs in mid-June reads `0` under the default window. The
  window shown is the one the server actually served — if it clamped your
  `--from`, the printed range says so. Widen it with `--from` / `--to`, which
  accept a plain `YYYY-MM-DD` (midnight UTC) or a full RFC3339 timestamp; a
  malformed value or an inverted window is a usage error (exit `2`) caught
  before any request.
- **"Not entitled" is not "zero".** When the caller doesn't own the app (or
  lacks Apps-author access) the API answers **HTTP 200 with every counter
  zeroed**, flagged only by a `notOwned` field. The CLI refuses to render a
  dashboard in that case and tells you to check `civitai whoami` /
  `civitai app status <slug>` instead — a silently-empty dashboard that looks
  like real data is the failure mode this command is built to avoid.
- **It needs a personal API key.** The query is full-scope, so an OAuth
  `civitai login` token gets a 403; the error names the fix
  (`civitai login --token <key>`).

Two data caveats.

**Engagement counts only authenticated, scope-gated API
calls.** An app that ships no scoped API surface shows real installs and revenue
with a flat engagement section — that is expected, not a bug. **App loads is the
exception**: it is measured on every load, so it counts signed-out visitors and
static blocks that engagement structurally cannot see. `Unique viewers` counts
signed-in people once each and approximates signed-out ones by network address,
so read it as reach rather than an identity count, and `Signed-out loads` is a
count of LOADS (one anonymous visitor reloading ten times is 10 there and 1
unique viewer), so it can legitimately exceed `Unique viewers`. Note also that
these are mount ATTEMPTS: a load that FAILED still counts, because a failed
mount's only beacon is the same event and it carries no status. `Error rate` is the
share of those calls that failed, and the human view renders it as a percentage
(the server sends it as a `0`–`1` ratio, which `--json` passes through unchanged).
Only a genuine zero prints `0.0%`: a real but tiny rate — a high-traffic app with
a handful of failures — reads `<0.1%` rather than rounding away to look
error-free.

`--json` emits the raw analytics payload (the server's own object, including
`notOwned` and the per-bucket `series` arrays the human view omits) for
scripting. Note that **`--json` does not refuse a not-entitled read the way the
human view does**: a `notOwned: true` payload is passed through with every
counter zeroed and the command still exits `0`, so
`civitai app metrics <slug> --json | jq .runs.count` returns `0` for an app you
can't see. A script must branch on the `notOwned` field rather than trusting the
counts.

**App loads has a SECOND, section-local unavailability flag** — `views.unavailable`,
independent of `notOwned`. It is the one section the server reads from a
different store, which can be unreadable — or merely too slow, the read is
time-bounded server-side — while every other counter in the same response is
genuinely measured. When that happens the human view prints `unavailable` and
says so explicitly rather than printing a `0` you would read as "nobody opened
my app". `--json` passes the flag through and still exits `0`, so a script must
branch on `views.unavailable` too — `jq .views.count` alone cannot tell an
outage from a real zero. A server old enough to predate this section omits the
`views` key entirely; the human view reports that as unavailable as well
(naming the different cause), and a script should treat a missing `.views` the
same way.

## Configuration

| Setting | Config key | Env var | Default |
| --- | --- | --- | --- |
| Personal API key | `token` | `CIVITAI_TOKEN` | — |
| OAuth tokens (device login) | `auth_kind`, `access_token`, `refresh_token`, `token_expiry`, `scope` | — | — |
| API base URL | `base_url` | `CIVITAI_BASE_URL` | `https://civitai.com` |
| Submit endpoint | — | `CIVITAI_SUBMIT_PATH` | `/api/v1/blocks/submit-version` |

Config lives at `~/.config/civitai/config.yaml` (honours `XDG_CONFIG_HOME`),
written owner-readable only.

## Exit codes

`civitai` returns a differentiated exit code so scripts can branch on the *kind*
of failure without parsing stderr. The human-readable error message is unchanged
by this — only `echo $?` differs.

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Generic / unclassified error. |
| `2` | Usage error — a bad flag, a bad flag value (e.g. `--limit` out of range, a non-integer id), or a request the API rejected as malformed (HTTP 400, e.g. a bad `--period`/`--sort` enum). |
| `3` | Authentication/authorization — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403, or no token configured). |
| `4` | Not found — the requested resource does not exist (HTTP 404). |
| `5` | Network/transport failure or service unavailable — dial/timeout, or HTTP 502/503/504 after retries. |
| `6` | Rate limited — throttled by the API (HTTP 429). |

```bash
# Branch on failure kind
if ! civitai models get "$id" >/dev/null 2>&1; then
  case $? in
    3) echo "log in first: civitai login" ;;
    4) echo "no such model: $id" ;;
    5|6) echo "transient — retry later" ;;
    *) echo "failed" ;;
  esac
fi
```

## Troubleshooting

- **`no token configured`** — run `civitai login` (or set `CIVITAI_TOKEN`).
- **`unauthorized (401)`** — your token is invalid/expired. OAuth tokens refresh
  automatically; if the refresh token has also expired, run `civitai login`
  again. For a personal key, create a new one at
  `https://civitai.com/user/account` and `civitai login --token <key>`.
- **`forbidden (403)` / `service unavailable (503)`** — your account may lack
  Apps access while the feature is in its invite-only beta (see the warning at
  the top of this README). Submission is limited to invited beta testers
  until Apps reaches general availability.
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
  [`AGENTS.md`](AGENTS.md).
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
`checksums.txt` plus a Homebrew tap bump. See [`AGENTS.md`](AGENTS.md) for the
full process and the secrets it needs (`HOMEBREW_TAP_GITHUB_TOKEN`).

## License

[Apache License 2.0](LICENSE).
