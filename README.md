# civitai CLI

> **Browse and download Civitai models, images, and articles — and author,
> validate, and submit App Blocks.** Two paths in one static binary: a
> **read/download client** for the public API (reads are anonymous; downloads
> need a token), and the toolchain for shipping **Apps** (every `civitai app`
> command needs one).

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
**public** API (browse and fetch models, images, and articles — no account needed
to read those), and it's the toolchain to **author, validate, and ship Apps**.
Everything under `civitai app` needs a credential, including the App-store
browse commands `app list` / `app view` — see
[Browse the App store](#browse-the-app-store).

An **App** is a small, sandboxed web app that runs inside Civitai
surfaces (it's served in an iframe; the platform owns the build and the
runtime). The CLI replaces the error-prone "hand-format a ZIP" flow: it
**scaffolds** a correct project, **validates** the manifest against the platform
contract, and **packages/submits** it for review.

> New here? The
> [Build your first App](https://github.com/civitai/civitai-app-starters/blob/main/docs/build-your-first-app-block.md)
> guide is the full end-to-end walkthrough.

## Contents

**Get started**

- [Install](#install)
  - [npm (Node)](#npm-node)
  - [Homebrew (macOS / Linux)](#homebrew-macos--linux)
  - [Nix flake](#nix-flake)
  - [Prebuilt binary](#prebuilt-binary)
  - [Go install (from source, Go 1.25+)](#go-install-from-source-go-125)
- [Quickstart: browse & download](#quickstart-browse--download)
- [Quickstart: build an App Block](#quickstart-build-an-app-block)
- [Command reference](#command-reference) — every command, one table

**Author an App**

- [SDK packages](#sdk-packages)
- [The blockId](#the-blockid)
- [Templates](#templates)
- [The host handshake (`BLOCK_READY`)](#the-host-handshake-block_ready)
- [Local dev loop (harness: mock vs live)](#local-dev-loop-harness-mock-vs-live)
- [Preview in the real host (`app dev-tunnel`)](#preview-in-the-real-host-app-dev-tunnel)
- [Examples](#examples)
- [Validate fidelity](#validate-fidelity)
  - [The `--json` result shape](#the---json-result-shape)
- [Submit & auth](#submit--auth)
  - [After you submit: review → approve → deploy](#after-you-submit-review--approve--deploy)
  - [Listing media requirements](#listing-media-requirements)
- [Submission status](#submission-status)
  - [Is your repo behind what you shipped?](#is-your-repo-behind-what-you-shipped)
  - [Deployed is not the same as listed in the store](#deployed-is-not-the-same-as-listed-in-the-store)
- [Pull your app's repository (`app pull`)](#pull-your-apps-repository-app-pull)
- [Browse the App store](#browse-the-app-store)
- [App metrics](#app-metrics)

**Use the API**

- [Browse the public API](#browse-the-public-api)
- [Download model files](#download-model-files)
- [Generate](#generate) — **spends real Buzz**
  - [`--max-cost` is an estimate check, not a spending cap](#---max-cost-is-an-estimate-check-not-a-spending-cap)
  - [A checkpoint does not carry its ecosystem](#-a-checkpoint-does-not-carry-its-ecosystem)
  - [Silent model substitution](#-silent-model-substitution)
  - [Confirmation](#confirmation)
  - [Image-to-image: `--image` and `--ecosystem`](#image-to-image---image-and---ecosystem)
  - [The content flags, and why there aren't twelve](#the-content-flags-and-why-there-arent-twelve)
  - [Raw graphs: `--print-input` and `--input`](#raw-graphs---print-input-and---input)
  - [Waiting, downloading, and re-attaching](#waiting-downloading-and-re-attaching)
  - [Listing and cancelling workflows](#listing-and-cancelling-workflows)
  - [What the server says went wrong](#what-the-server-says-went-wrong)
  - [Reading a workflow's Buzz transactions](#reading-a-workflows-buzz-transactions)
  - [Exit codes specific to `generate`](#exit-codes-specific-to-generate)
- [Scripting with `--json`](#scripting-with---json)
  - [Cursor pagination loop](#cursor-pagination-loop)
  - [Clean output for pipelines](#clean-output-for-pipelines)
  - [Generation `--json`](#generation---json)
  - [Gotchas](#gotchas)
  - [Worked example — top LoRAs for a base model, then plan a download](#worked-example--top-loras-for-a-base-model-then-plan-a-download)

**Reference**

- [Upgrading](#upgrading)
- [Global flags](#global-flags) — colour, `--version`, the update nag
- [Configuration](#configuration)
- [Exit codes](#exit-codes)
- [Troubleshooting](#troubleshooting) — **look the error message up here**
- [Development](#development)
- [Releasing](#releasing)
- [License](#license)

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

Reads of the **public catalog** — models, model versions, images, tags,
creators, users, articles, collections — are **anonymous**: no `login` needed
for the commands in this section. Every one of them takes `--json` to emit the
raw API response for scripting.

> **What is *not* anonymous.** `civitai download` needs a token, and so does
> every `civitai app …` command — **including the App-store browse commands**
> `civitai app list` and `civitai app view`, which exit `3` with
> `no token configured` when you have not logged in. The store endpoint keys the
> visible catalog off your identity, so there is no anonymous view of it. See
> [Browse the App store](#browse-the-app-store).

```bash
# Search models — filter by base model, type, and sort:
civitai models search --base-model Illustrious --type Checkpoint --sort "Most Downloaded"

# --base-model works on any type, including embeddings (TextualInversion):
civitai models search --type TextualInversion --base-model "SDXL 1.0"

# Inspect a specific model or a specific model version:
civitai models get 618692
civitai model-versions get 691639

# Download a version's file(s) — SHA256-verified, streamed atomically.
# `--layout` routes each file into the right app subfolder (also `a1111`);
# `--dry-run` prints the plan without transferring. Downloads require `civitai login`.
civitai download 691639 --layout comfyui --root ~/ComfyUI
civitai download 691639 --dry-run

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
#    Interactively this asks you to confirm. In CI — or any non-TTY shell —
#    a token-carrying submit REFUSES without --yes rather than firing a real
#    moderator-review request nobody approved. Scripts must pass it:
#      civitai app submit --yes
civitai app submit

# 6. Check where your submission is in review / deploy.
civitai app status

# 7. Attach the store-listing media. An icon AND a cover are REQUIRED before the
#    listing can publish — do it now, while the app is in review; it carries
#    forward on approval. `listing status` shows what's still missing.
#    The scaffold creates `assets/` and a README of the requirements, but NO
#    images — save your own icon.png and cover.png in there first:
#      icon   png/jpeg/webp, <= 2 MiB, square-ish  — start from 512 x 512
#      cover  png/jpeg/webp, <= 4 MiB, landscape   — start from 1600 x 900
#    Full bounds (and who checks what) in "Listing media requirements" below.
civitai app listing set-icon ./assets/icon.png
civitai app listing set-cover ./assets/cover.png
civitai app listing status
```

> **Step 7 needs artwork you supply.** Every template scaffolds an `assets/`
> directory with a README of the requirements, and deliberately **no placeholder
> images** — a placeholder passes every check and uploads cleanly, which is how a
> stub icon reaches a public listing. Sizes, formats and aspect ratios are in
> [Listing media requirements](#listing-media-requirements).

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
imports lives in two published npm packages (the `page-money` template wires
them for you; `static` and `page-vite` are deliberately dependency-free):

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
| `civitai login [--scopes <set>] [--token [<t>]] [--no-browser]` | Browser OAuth device login by default (stores auto-refreshing tokens). The default scope set grants identity + Apps submit + dev-tunnel and **not** Buzz-spend; `--scopes generate` additively grants generation + Buzz **spend** (needed by `civitai generate` and money-path `dev:live`). `--token <t>` stores a personal API key instead (not combinable with `--scopes`). `--token` with **no value** prints where to create a personal key (`civitai.com/user/account`) and how to re-run — handy when you know you want a personal key but haven't minted one yet. Config at `~/.config/civitai/config.yaml`, 0600. Also reads `CIVITAI_TOKEN`. |
| `civitai whoami [--scopes] [--json]` | Verify the stored token; print the authenticated user **and a Capabilities section** — credential type (**OAuth login** vs **personal API key**), **Read Buzz balance**, and **Spend Buzz** — decoded from the token's scope, so a money-path dead end (a default OAuth login can't spend) is visible before `dev:live` — and when it can't, the output names the fix for that credential (`login --scopes generate` for an OAuth login, a full-scope key otherwise). `--scopes` also lists every granted scope; `--json` emits the user + `credentialType`/`canReadBalance`/`canSpend`/`scopes` (scriptable). |
| `civitai buzz [--json]` | Show your spendable Buzz balance (**blue / green / yellow**, plus a **total**). Needs the BuzzRead scope — a full-scope personal API key or `civitai login --scopes generate`; a **default** OAuth login token can't read it, and gets a clear message naming both fixes. `--json` emits `{blue,green,yellow,total}` (scriptable — handy for before/after diffing a `dev:live` spend). |
| `civitai app list [--kind <k>] [--category <c>] [--sort <s>] [--limit <n>] [--cursor <c>] [--json]` | **Discover published Apps in the store** (`GET /api/v1/apps`) — filter-based discovery, not free-text search. **Needs a credential** (`civitai login` or `CIVITAI_TOKEN`): the endpoint keys the visible catalog off your identity, so this is *not* one of the anonymous reads. Cursor-paged. See [Browse the App store](#browse-the-app-store). |
| `civitai app view <slug> [--json]` | **Show one published App's store detail** (`GET /api/v1/apps/{slug}`) — description, category, rating, gallery, live/external target. **Needs a credential**, same as `app list`. Reads the *public store catalog*, which is a different resource from your own deploy — a not-found here says nothing about `<slug>.civit.ai`. See [Browse the App store](#browse-the-app-store). |
| `civitai app create [name] [dir] [--template static\|page-vite\|page-money] [--dir <path>] [--name <display>] [--slug <slug>] [--yes]` | **The friendly happy path.** Scaffold a ready-to-build App, defaulting to the batteries-included `page-money` SDK template (default dir `./<slug>`). `--slug` sets the **blockId** explicitly instead of deriving it from the name — needed when derivation refuses the name (see [The blockId](#the-blockid)). `-y`/`--yes` is the non-interactive form: it never prompts, taking flags and defaults instead, and fails if a name is missing. |
| `civitai app init [name] [dir] [--yes] [...]` | Same scaffolder as `create` with a no-build `static` default (back-compat alias); same `--yes`. |
| `civitai app dev-token <slug> [--env] [--spend] [--budget <n>]` | **Mint a short-lived (~4h) dev block token for `npm run dev:live`.** `--spend` must be asked for explicitly to request real-Buzz spend — without it the CLI filters `ai:write:budgeted` out of the mint request. `--env` prints a paste-ready `VITE_LIVE_BLOCK_TOKEN=<token>`. See [Local dev loop](#local-dev-loop-harness-mock-vs-live). |
| `civitai app dev-tunnel [blockId] [--block <id>] [--port <n>] [--local-host <host>] [--tunnel-endpoint <h:p>] [--idle-timeout <d>] [--ready-timeout <d>] [--no-wait]` | **(Pre-GA / invite-gated)** Preview your **local** dev server inside the **real** Civitai host at `civitai.com/apps/dev/<blockId>` — a prod-fidelity inner-dev-loop. Pre-flights whether the host can actually **embed** your dev server and warns (never fatally) when it cannot. See [Preview in the real host](#preview-in-the-real-host-app-dev-tunnel). |
| `civitai app validate [dir] [--strict] [--json]` | Best-effort local pre-check of `block.manifest.json`; emits non-fatal warnings (`--strict` fails on them). `--json` emits the structured result (`ok`, plus `errors`/`warnings` each with `field`/`message` — **`field` is always present and never `null`**) for scriptable parsing — still exits non-zero on failure. 🔴 **BREAKING:** a `[dir]` that does not exist, or is not a directory, is now a **usage error** — exit `2` with **no JSON object** on stdout, where it used to print `{"ok": false, …}` and exit `1`. See [Validate fidelity](#validate-fidelity) and [The `--json` result shape](#the---json-result-shape). |
| `civitai app submit [dir] [--yes] [--package-only] [--out f.zip] [--skip-validate] [--allow-downgrade] [--allow-dirty]` | Validate + package the source tree + upload it with your stored token (or, with no token, write the bundle + print next steps). **A submit that would really upload asks for confirmation, and in a non-interactive shell it refuses without `--yes`** — `civitai app submit --yes` is the CI form. (The refusal is reached only when there is a token to upload with: `--package-only`, and the no-token fallback that just writes the .zip, never submit and so never ask.) **It also refuses a version that is not strictly above the highest APPROVED version of that app** — approving an older (or identical) version replaces the newer live deployment — with `--allow-downgrade` as the deliberate-rollback escape hatch. **And it refuses a dirty git work tree** — the bundle is packaged from what is on disk, so approving one deploys code that exists in no commit — with `--allow-dirty` as the escape hatch; that guard degrades rather than enforcing, so a directory in no git repo (every scaffolded app starts that way) submits unchanged, and a clean tree whose `HEAD` is on no remote warns instead of refusing. All three refusals are skipped on the routes that never reach the server. |
| `civitai app pull [dir] --app <slug\|appBlockId>` | **Clone (or sync) the canonical git repository behind one of your approved Apps** — the read side of git authoring. ⚠ The clone URL embeds your access token, and a fresh clone persists it into `.git/config`. See [Pull your app's repository](#pull-your-apps-repository-app-pull). |
| `civitai app listing status [--json]\|set-icon <file>\|set-cover <file>\|add-screenshot <file>\|rm-screenshot <id>\|reorder <id...>\|submit-revision` | **Attach the store-listing media your App needs before it can be published** — an **icon and a cover are mandatory** (screenshots are optional, up to 8). `listing status` prints what is attached vs. what the publish floor still requires, and `listing status --json` emits the same read as one object — including **`parentId` and `shadowId`, the two listing ids a change is addressed to**, which the human output shows neither of. 🔴 **`--json` is not a pure read: on a live listing it opens the revision draft described below, so do not poll it.** The CLI checks format + byte size locally; **dimensions and aspect ratio are checked by the platform at attach**. **On a listing that is already LIVE, every change — including `reorder` — opens a REVISION for moderator re-review rather than editing the live listing**, and `listing status` reports that in-progress revision's media (so the `alsc_…` ids it prints are the revision's, which is what `reorder` addresses). `rm-screenshot` is the one change deliberately left **staged**: it does **not** submit the revision (curating a gallery is usually several removals), so `submit-revision` is what sends it to moderator review and makes the change public. See [After you submit](#after-you-submit-review--approve--deploy) and [Listing media requirements](#listing-media-requirements). |
| `civitai app status [blockId] [--id <pubreq>] [--limit N] [--json]` | Check the review/deploy status of **your own** submissions. No arg lists them all; `--limit N` shows only the newest N (display-side — this route cannot page); a `blockId` (app slug) or `--id` shows one in detail (rejection reason if rejected, live URL once deployed). Run from **inside** an app checkout it also warns on **stderr** when your local `block.manifest.json` is **BEHIND** your highest approved version — advisory only, the exit code never changes. See [Submission status](#submission-status) and [Is your repo behind what you shipped?](#is-your-repo-behind-what-you-shipped). |
| `civitai app metrics <slug> [--from <d>] [--to <d>] [--json]` | **Owner-only analytics for one of your Apps** — installs, runs + Buzz spent, Buzz purchased, and API engagement. Always prints the window the **server** served (it defaults to 30 days and clamps to 366), so a zero is never ambiguous. Needs a **personal API key** (an OAuth login is refused). See [App metrics](#app-metrics). |
| `civitai app withdraw [pubreq-id] [--id <pubreq>] [--yes]` | **Withdraw your own pending submission** (the `pubreq_…` id from `civitai app status`). Frees the slug so a fresh `civitai app submit` can replace it. **Also deletes a first-version app's store listing — icon, cover and every captioned screenshot**, so it asks first and needs `--yes` in a script. Idempotent for the submission only; only a `pending` request can be withdrawn. See [Submission status](#submission-status). |
| `civitai generate "<prompt>" [--negative-prompt <p>] [--quantity <n>] [--aspect-ratio <r>] [--checkpoint <version-id>] [--lora <version-id>[:strength]] [--image <path-or-url>] [--ecosystem <key>] [--input <file>] [--print-input] [--dry-run] [--json] [--max-cost <buzz>] [--fail-on-substitution] [--yes] [--no-wait] [--timeout <dur>] [--out-dir <dir>] [--out-name <template>] [--no-download] [--force] [--external-id <key>]` | **Generate images from a text prompt — this SPENDS REAL BUZZ.** Prices the job with the server's estimator, shows the cost + your balance, asks before spending, submits, then **waits and downloads** the results. `--dry-run` prices it and exits without submitting; `--max-cost` is an **estimate check, not a spending cap**. Needs the AI Services scopes — `civitai login --scopes generate` or a full-scope **personal API key**; a **default** OAuth login is refused. See [Generate](#generate) for the wait/download flags, image-to-image, raw graphs, and [silent model substitution](#-silent-model-substitution). |
| `civitai workflows list [--limit <n>] [--cursor <c>] [--tag <t>] [--json]` | **List the generation workflows you have submitted**, newest first — status, when, cost, and `deliverable/total` outputs. Where the server recorded **an account of what happened** to a workflow, it is printed in full on indented lines under that workflow's row. Cursor-paged: the next cursor is printed on stdout when more results exist. Reading spends nothing. See [Generate](#listing-and-cancelling-workflows). |
| `civitai workflows get <workflow-id> [--json]` | **Look up one generation workflow** — status, steps, outputs, the Buzz transactions the server recorded for it, and **the account the orchestrator recorded** for the run where there is one (printed under *The server reported:*). This is how you re-attach after `--no-wait`, a `--timeout` expiry or a Ctrl-C. Outputs that are blocked, unavailable or hidden are listed **with why they were excluded** rather than omitted — and where the excluded outputs died of *different* server-reported causes, each line names its own. Output URLs are presigned and expire; re-run for fresh links. Reading spends nothing. See [Generate](#waiting-downloading-and-re-attaching) and [Reading a workflow's Buzz transactions](#reading-a-workflows-buzz-transactions). |
| `civitai workflows cancel <workflow-id> [--yes] [--json]` | **Stop a running generation.** 🔴 **You are billed for what it already delivered**; the orchestrator re-prices the rest server-side and this CLI cannot report the figure. Cancel because you no longer want the output. Asks for confirmation (default **no**); `--yes` skips the prompt and a non-TTY without it refuses. See [Generate](#listing-and-cancelling-workflows). |
| `civitai upgrade [--force]` | **Self-update this binary in place** — resolve the latest GitHub release, verify its SHA-256 against `checksums.txt`, and replace the running executable. A Homebrew install delegates to `brew upgrade` instead; `--force` reinstalls anyway (and self-replaces a Homebrew install). See [Upgrading](#upgrading). |
| `civitai version` | Print version / commit / build date. |
| `civitai completion [shell]` | Generate a shell-completion script. |

Run `civitai help`, `civitai app --help`, or `civitai <command> --help` for the
full details and examples.

### The blockId

The **blockId** is your app's permanent public identity: the hostname it will be
served at once approved (`https://<blockId>.civit.ai/`) and the argument every
later command takes (`app status`, `app metrics`, `app listing`, `app dev-token`,
`app dev-tunnel`). **It cannot be renamed afterwards.** `app create` / `app init`
echo the one they chose, so it is on screen before you commit anything.

By default it is derived from the name: `"My Cool Block"` → `my-cool-block`.
Pass **`--slug <slug>`** to choose it yourself — it bypasses derivation entirely,
so name, blockId and directory are three fully independent axes.

> **Breaking change.** Derivation used to lowercase the name and replace every
> run of non-`[a-z0-9]` with a hyphen, which silently **dropped characters**:
> `civitai app create "Café App"` minted the blockId **`caf-app`**, and
> `"ÜberApp"` minted **`berapp`** — a different permanent public id than the
> author typed, with no warning and exit 0. Derivation now **refuses** and names
> the offending characters, exiting **2** and asking for `--slug`. **If you have
> a script passing a non-ASCII name, it must now pass `--slug <slug>`.** The old
> output was wrong, so the break is the point — but it is a break.

> **Breaking change.** Derivation also used to **truncate**. A name whose blockId
> ran past the **40-character** cap was silently cut to fit, at exit 0 — so
> `civitai app create "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee"`
> and the same 54-character name ending `ZZZZZZZZZZ` **both** minted
> `aaaaaaaaaa-bbbbbbbbbb-cccccccccc-ddddddd`: two apps competing for one id that
> cannot be renamed, with nothing local to tell the author it happened. An
> **explicit** `--slug` of that length was already refused (`must be 3-40
> chars`), so the derived path — the one a first-time author walks — was the
> inconsistent one. It now refuses too, exiting **2** and naming the length.
> **If you have a script passing a long name, it must now pass `--slug
> <slug>`.** A name deriving **exactly 40** characters is *at* the cap, not over
> it, and still derives byte-identically.

Derivation refuses two things: **letters, digits and marks** above ASCII that the
slug alphabet cannot carry, and a name whose blockId would **exceed 40
characters**. Both refusals exit `2` and both are settled the same way — pass
`--slug`. Three things still derive rather than refuse, and they are deliberate:

| input | blockId | why |
| --- | --- | --- |
| `"Rocket 🚀 App"` | `rocket-app` | Symbols, emoji and non-ASCII punctuation are **separators** — that is what makes `"Widget — Pro"` → `widget-pro` right. An emoji has no lossless ASCII form either, so refusing would only trade a silent drop for a dead end. Tracked as [#272](https://github.com/civitai/cli/issues/272). |
| `"İstanbul App"` | `istanbul-app` | Exactly two runes above ASCII lowercase **into** ASCII — `İ` (U+0130) and `K` (U+212A). Lowercasing is what decides whether a character survives, so these transliterate for free. |
| `"My  Cool___Block"` | `my-cool-block` | ASCII is exempt by construction — every derivation that worked before still produces the byte-identical blockId. |

A name that is **not valid UTF-8** is refused outright (it used to lose the bad
bytes from the blockId *and* write them into `block.manifest.json`).

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

Every template also scaffolds an **`assets/`** directory holding a README of the
store-listing media requirements — and no images, so the `set-icon` / `set-cover`
step fails loudly until you supply real artwork. See
[Listing media requirements](#listing-media-requirements).

### The host handshake (`BLOCK_READY`)

Every template declares a `page` surface, and the host **will not reveal a page
app until the app posts `BLOCK_READY`** — that handler is the only transition
into the host's ready state. An app that never sends it is replaced by a visible
failure card once the host's bounded retries run out, even though the app itself
renders perfectly. Nothing you can run locally reproduces that.

`page-money` gets the handshake for free: `@civitai/blocks-react`'s iframe
transport acks internally, which is why the SDK templates never touch raw
`postMessage`. The two **SDK-free** templates (`static`, `page-vite`) therefore
ship a small vendored emitter, **`civitai-host.js`**, loaded from the entry
point. Leave it in place — and if you are retrofitting it into an older app,
note that the file has to be *referenced* as well as copied: `static` loads it
with `<script src="./civitai-host.js"></script>` in `index.html`, `page-vite`
with `import './civitai-host.js';` as the first line of `src/main.jsx`.
`civitai app validate` checks that reference where it can resolve your entry
point, and tells you when it can't.

> ⚠️ **If you adopt `@civitai/blocks-react`, delete `civitai-host.js` in the
> same change.** This is the one situation where removing it is correct, and
> running both is worse than running neither: whichever handshake answers the
> host's *first* `BLOCK_INIT` cancels the host's retry loop **and** its
> readiness timeout. If the vendored emitter wins that race, the SDK transport
> can be left never having seen an init — its `waitForInit` rejects after 10s
> and the host sits "ready", showing an app that never started, with no retry
> and no error card.

Two rules it encodes, which apply to every message you add afterwards:

- **The envelope is `{ type, payload }`.** The host dispatches
  `event.data.payload` to its subscribers, so fields put at the top level
  (`{ type: 'X', height: 0 }`) arrive as `payload: undefined`.
- **Answer, don't announce.** The ack goes out in *response* to the host's
  `BLOCK_INIT`, addressed at the origin that init arrived from rather than
  broadcast to `'*'`. It is also why nothing is posted when you preview locally:
  there is no host to send `BLOCK_INIT`, so the emitter stays silent by design.

> 🔒 **The emitter checks the sender *window*, not the sender's *identity*.** It
> answers `window.parent` — whoever framed you — which is sound for this one
> message because the ack carries no data. It is **not** sufficient for anything
> you add next. The moment you handle an inbound message carrying a token, a
> viewer, storage or a result, check `event.origin` against an allowlist of
> origins you trust, or any page that frames your app can feed it whatever it
> likes. The emitter deliberately does not vendor that allowlist — the real list
> (production, preview subdomains, dev tunnels) is platform state that moves
> without notice, and `@civitai/blocks-react` already maintains it from
> `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`. Adopt the SDK before you handle data.

`RESIZE_IFRAME` is **not** part of a page app's protocol: the host renders a
page block full-viewport, so it does not size to content and ignores the
message. (`useBlockResize` is surface-agnostic and page-money still calls it —
on a page surface it is simply a no-op, which is why the SDK templates can share
component code across surfaces.) The pre-#206 templates demoed a **raw**
`postMessage` of `RESIZE_IFRAME`, so a project scaffolded before that fix still
carries dead code you can delete. (This CLI's own CI fails if a shipped template
ever reintroduces it; there is no author-facing command that scans your project
for it — `civitai app validate` checks the manifest and the handshake, not this.)

### Local dev loop (harness: mock vs live)

A scaffolded App is a sandboxed iframe, and locally there is no host to send
`BLOCK_INIT` — so `npm run dev` shows you your own UI and nothing of the
protocol. The **`page-money`** template ships a dev **harness** (the SDK's
[`@civitai/blocks-react/testing`](https://www.npmjs.com/package/@civitai/blocks-react)
hosts) to close that gap, with two modes:

| Command | Mode | What it does |
|---|---|---|
| `npm run dev:harness` | **mock** (default) | Mounts the SDK **mock host** — synthetic replies, **no real Buzz, no compute, no network.** Safe to spam; drive money/error/insufficient-Buzz UX via on-screen scenarios or `?` URL params. Start here. |
| `npm run dev:live` | **live** | Mounts the SDK **live host** (`createLiveHost`) — forwards the App protocol to the **real Civitai backend** with a pasted dev token (Bearer). **Spends REAL Buzz / real compute.** |

> ⚠️ **`dev:live` works on a pending (un-approved) app.** The dev-token mint
> (`POST /api/v1/blocks/dev-token`) accepts a **pending** slug — right after a
> successful `civitai app submit` (status `pending`) it returns `200` with
> `appId: pending-pubreq_…` and `dev:live` mounts the live host against the
> pending app. For **real generation** you must mint with a credential carrying
> AI Services — a **full-scope personal API key**, or an OAuth login that opted
> in via `civitai login --scopes generate`. A **default** OAuth login
> (`civitai login`, no `--scopes`) mints read-only (`user:read:self`) and
> **cannot spend**. Use `civitai buzz` / `civitai whoami` to confirm your
> credential can spend before a live run, and pass `--spend` to `dev-token` to
> request the spend scope explicitly — **without it the CLI filters
> `ai:write:budgeted` out of the request**, so a spend-capable credential still
> mints a token that will not generate.

**Live mode** needs a short-lived dev block token. Mint it with **`civitai app
dev-token`** (the CLI handles the invite-gated `POST /api/v1/blocks/dev-token`
call with your stored credential — no hand-rolled curl) and paste it into
`.env.development.local` as `VITE_LIVE_BLOCK_TOKEN=`:

```bash
# From your scaffolded project dir (reads scopes from block.manifest.json):
civitai app dev-token my-block --env >> .env.development.local
npm run dev:live
```

`dev-token` reads the scopes to request from your **local**
`block.manifest.json`, so it works on a slug you have never submitted.
`.env.development.local` is the git-ignored one (the scaffold's `.gitignore`
covers `.env.*.local`; `.env.development` itself is tracked, and holds no secret
as scaffolded). `submit` excludes both, and the token is short-lived (~4h) —
re-run `dev-token` when it expires. For real generation mint
with a spend-capable credential (**full-scope personal API key** or
`civitai login --scopes generate`) **and** add `--spend`: the CLI never asks for
budgeted spend implicitly — **even when your manifest declares it** (the
scaffolded money app does) — so without the flag the mint filters
`ai:write:budgeted` out and dev:live refuses to generate with `block lacks
ai:write:budgeted scope`. `--budget <n>` sets the token's **per-generation** Buzz
budget (1–250; omit it and the server picks one — 50 for an unsubmitted app). A
default OAuth login mints a read-only token either way (the command warns you at
mint time). With no token, `dev:live` **fails safe**
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

**Which credential can spend?** Spending Buzz — a real `dev:live` generation in
your app, or a [`civitai generate`](#generate) run from the terminal — needs the
**AI Services** scope. Two credentials carry it; the **default** OAuth login
deliberately does not:

| Credential | Can spend Buzz? (`dev:live`, `civitai generate`) | How to get it |
|---|---|---|
| **Personal API key** (full scope) | ✅ **Yes** — estimate → submit → generation → real Buzz | create it **in the web UI** at `civitai.com/user/account`, then `civitai login --token <key>` (a personal key carries AI Services) |
| **`civitai login --scopes generate`** (OAuth, opt-in) | ✅ **Yes** — additive on top of the default, so it also keeps submit + dev-tunnel | `civitai login --scopes generate` (the `civitai-cli` client's `allowedScopes` on `civitai.com` includes the generate bits — see [Submit & auth](#submit--auth)) |
| **`civitai login`** (OAuth, default) | ❌ No — viewer + catalog + app storage only | the default scope set omits AI Services, so the server strips the spend scope — fine for read/identity `dev:live`, not for generation |

This is the single most common blocker for `civitai generate`: a default OAuth
login looks perfectly valid, and the refusal is a scope problem, not a login
problem — re-running plain `civitai login` will not fix it, but
`civitai login --scopes generate` (or a full-scope personal key) will.
`civitai whoami` shows the capability as **Spend Buzz (AI Services)** and names
the fix for whichever credential you have.

You can't mint a personal key over OAuth or the CLI (`apiKey.add` returns 403
without a full-scope session) — create it in the web UI. The dev token always grants
`user:read:self`, so your viewer resolves on either path. For the scope mechanics
behind this, see [Submit & auth](#submit--auth).

Env vars (`VITE_BLOCK_ALLOWED_PARENT_ORIGINS`, `VITE_HARNESS_MODE`,
`VITE_LIVE_BLOCK_TOKEN`, …) and the scenario knobs are documented in depth in the
scaffolded project's own `README.md` and `.env.example` — see
[`internal/scaffold/templates/page-money/README.md.tmpl`](internal/scaffold/templates/page-money/README.md.tmpl).

### Preview in the real host (`app dev-tunnel`)

> **(Pre-GA / invite-gated.)** Access is gated behind an Apps-author invite
> **and** a server kill-switch flag, so if you are not enrolled the mint reports
> **"not available"** — ask to be added to the cohort.

The harness is a *mock* of the host. `civitai app dev-tunnel` is the other end of
that trade: it previews your **local** dev server inside the **real** Civitai
host at `civitai.com/apps/dev/<blockId>`, so you see prod chrome, prod sandbox
and the prod handshake against the code in your editor.

```bash
npm run dev:tunnel                 # start your dev server first
civitai app dev-tunnel my-block    # then open the tunnel
```

It mints an **ephemeral in-memory ssh keypair**, opens a reverse tunnel from your
dev port to the Civitai tunnel endpoint (`sish.civitai.com:2224`, live), prints
the URL to open, and tears everything down on Ctrl-C or an idle timeout.
Publishing the tunnel host through external-dns + Cloudflare usually takes 1–3
min (occasionally longer); the command waits for it and prints the elapsed time.

**Embeddability preflight.** Before minting, the command checks whether the host
can actually *embed* your dev server. The host iframes it sandboxed, at an opaque
`null` origin, so a dev server that is missing `Access-Control-Allow-Origin: *`,
missing the `.civit.ai` entry in `allowedHosts`, or sending a framing header that
excludes `civitai.com` loads as a **blank iframe with no error anywhere** — the
worst kind of failure to debug. Those are printed as **warnings, never fatal**,
and deliberately **twice**: once the moment the checks run, so you still get them
if you Ctrl-C the DNS wait, and again just above the URL, with the
`vite.config.ts` fix. Apps scaffolded by `civitai app create` (the `page-money`
template) already satisfy all of it.

**Flags.** The defaults match what the scaffold's `npm run dev:tunnel` binds, so
most authors pass none of these:

| Flag | Default | What it is for |
| --- | --- | --- |
| `--block <id>` | the `blockId` in `block.manifest.json` in the CWD | The app to tunnel, if you are not standing in its project (it is also the positional argument). |
| `--port <n>` | `5186` | The local dev-server port. Matches the scaffold's `dev:tunnel` script. |
| `--local-host <host>` | `localhost` | **The host your dev server is bound to.** Change this when the dev server is *not* on the CLI's own loopback — inside a **container or pod** (`--local-host 10.42.0.100`), a **VM**, or bound to one specific interface. A dev server that is reachable in your browser but not from the CLI's `localhost` is exactly what this flag is for. |
| `--no-wait` | off | Print the public URL immediately instead of waiting for it to start serving. It may `404`/`NXDOMAIN` for a few minutes while DNS and routing propagate. |
| `--ready-timeout <d>` | `0` (wait indefinitely) | Cap the readiness wait. On expiry the command warns and prints the URL anyway rather than failing. |
| `--idle-timeout <d>` | `30m` | Tear the tunnel down after this much inactivity. |
| `--tunnel-endpoint <host:port>` | `sish.civitai.com:2224` | The sish SSH endpoint to dial. Also settable with **`CIVITAI_DEV_TUNNEL_ENDPOINT`**; the flag wins. |

> **Publishing DNS takes 1–3 minutes, sometimes longer.** That wait is normal
> and the command reports elapsed time while it happens — it is not a hang. If
> you Ctrl-C out of it you also lose the embeddability warnings, which is why
> they are printed once *before* the wait as well as again after it.

**`CIVITAI_DEVTUNNEL_DEBUG` — a debug-only escape hatch.** ⚠️ Not supported
surface: it is a diagnostic for *this* CLI's tunnel plumbing, and its output
format, its trigger and its existence may change or be removed in any release.
Don't build anything on it.

Set it to **any non-empty value** before running `civitai app dev-tunnel` and
the tunnel writes one extra `[debug]` line to **stderr for each inbound
connection** the sish endpoint forwards — naming the address/port and *origin*
address/port sish stamped on that SSH channel, the subdomain label the CLI
bound, and whether Go's `x/crypto/ssh` built-in listener would have **rejected**
the channel. That last field is the point: the built-in listener parses the
origin as an IP address and sish sends a hostname, which is why this CLI accepts
every forwarded channel itself rather than using `client.Listen`. Reach for it
when the tunnel reports ready and the public URL still `502`s — it tells you
whether requests are arriving at the CLI at all, and what they look like when
they do.

```bash
CIVITAI_DEVTUNNEL_DEBUG=1 civitai app dev-tunnel
```

It is **output only** — nothing about how the tunnel behaves changes. The value
is never parsed, so `CIVITAI_DEVTUNNEL_DEBUG=0` and `=false` switch it *on* just
like `=1`; leaving it **unset** is the only way to switch it off. It is read once
as the tunnel is established, and prints nothing until traffic actually arrives,
so a tunnel nobody has loaded stays as quiet as it was before.

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
| `civitai images get <id>` | Get one image by id (`GET /api/v1/images?imageId=<id>`) | `--json`, `--anon` |
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
civitai download 691639                       # the version's primary file → ./<server-name>
civitai download --model 4384                 # resolve model 4384's default version, then download its primary file
civitai download --model 4384 --dry-run       # print the plan (files, sizes, hashes, targets) — download nothing
civitai download 691639 --out ./flux_dev.safetensors
civitai download 290640 --file vae --out-dir ./models   # pick a file by name; write into a dir
civitai download 691639 --file 1234567                  # pick one of two same-named files by its file id
civitai download 290640 --all --out-dir ./models        # every file in the version
civitai download 290640 --all --layout comfyui --root ~/ComfyUI   # route each file to its type folder
civitai download 691639 --layout a1111 --for-base "SDXL 1.0"      # A1111 layout + base-model compat warning
```

> **Downloads require authentication.** Every model-file download needs a token —
> even a small public embedding 401s anonymously. Run `civitai login` first. The
> read/search commands work anonymously; downloads do not. `--anon` is meaningful
> for the read commands, not for `download`.

Behavior:

- **Identifier** — exactly one of the positional id or `--model <model-id>` is required. The positional is normally a model-**version** id, but because `models search` / `models get` print **model** ids, handing one over just works: the CLI notices it is a model id and downloads that model's default version, printing a `note: <id> is a model id — downloading its default version <v>` line. When a number is **both** a valid model id and a valid version id (common for low/mid numbers), the CLI **stops** rather than guess, naming both interpretations — re-run with `--model <id>` (that model's default version), `--version <id>` (that version as-is), or `--yes` to take the version interpretation and have it echoed back. `--version` names a version id explicitly and skips the stop entirely.
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

Both properties hold for `civitai generate` and `civitai workflows …` too, but
their payloads are **not** Site API REST shapes — generation has no REST route,
so those commands pass through the raw *orchestrator* reply. Read
[Generation `--json`](#generation---json) before scripting against them.

Two `app` commands emit a shape this **CLI composes**, not a wire payload:
`civitai app validate --json` (see
[The `--json` result shape](#the---json-result-shape)) and
`civitai app listing status --json`, which joins the two listing reads into one
object naming `parentId` and `shadowId` (see
[After you submit](#after-you-submit-review--approve--deploy)). Both keep the two
properties above; only the *provenance* of the fields differs. 🔴 **And
`app listing status --json` is a read with a server-side SIDE EFFECT** — on a
live listing it opens a revision draft — so, unlike the reads above, it must not
be polled.

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

### Generation `--json`

`civitai generate --dry-run --json`, `civitai workflows list --json` and
`civitai workflows get <id> --json` emit the raw orchestrator payload. Two
caveats have bitten people, and neither shows up as an error:

- **Output URLs are presigned and EXPIRE.** The links in a workflow payload are
  short-lived signatures, not durable addresses. A pipeline that stores them and
  fetches later gets a 401/403 from the storage host that no credential can fix
  — re-run `civitai workflows get <id>` for fresh links instead of caching the
  old ones. (Fetch them with **no** `Authorization` header; they are already
  authorized, and the CLI deliberately attaches nothing to them.)
- **`--json` still exits `0` when the server reports the resources are
  unavailable.** `--dry-run --json` prints the estimate and exits `0` even when
  the payload says `"ready": false`. A human `--dry-run` prints a warning and
  this CLI refuses to submit in that state, but a script reading only the exit
  code sees success. **Branch on the field**, exactly as `app metrics` requires
  branching on `notOwned` — and note the shape below, which **fails closed**:

  ```bash
  q=$(civitai generate "a cat" --dry-run --json) || exit $?
  case "$(printf '%s' "$q" | jq -r 'if has("ready") then .ready else "absent" end')" in
    false)   echo "resources unavailable" >&2; exit 1 ;;   # decisive: do not submit
    true)    ;;                                            # NOT a green light — see below
    *)       echo "no readable .ready field" >&2; exit 1 ;; # absent, null, or jq failed
  esac
  printf '%s' "$q" | jq -r .cost.total
  ```

  The `*` arm is the point. An earlier version of this snippet tested
  `[ … = "false" ] && exit 1`, which exits **0** when the key is absent or `jq`
  fails — it read "we could not ask" as "we asked and it was fine", the
  fabricated-zero mistake `app metrics` documents for `views.unavailable`.

  🔴 **`ready` is one-directional, and the human label says so.** It reports
  only that the resources this job needs are currently available — the server
  computes it as "every job's queue position reports `support: available`", and
  a job carrying no queue position at all is skipped, leaving the flag `true`.
  It is not a moderation verdict and not a prediction that the job produces an
  image; `--dry-run` therefore prints it as **`Resources ready`**, not
  "Generatable". A run reporting `ready: true` can still be charged and return
  nothing — measured: 8 submits across 3 checkpoints that all quoted
  `ready: true` produced 0 outputs. So gate on the FALSE direction, as above,
  and never treat `true` as a success predicate. The only thing that settles
  whether a job produced output is the finished workflow
  (`civitai workflows get <id>`), and `civitai generate` exits non-zero when it
  waited and got no deliverable output.

  **What `ready: false` gets you is a LOCAL refusal, and that is all this repo
  can evidence.** `civitai generate` reads the flag and refuses to submit; no
  server-side enforcement of it has been found — `support !== 'available'`
  appears once in the whatIf reply builder and nowhere on the submit path, and
  the checkpoints that failed in the measurement above surfaced as HTTP 400s
  rather than as `ready: false`. Treat it as this CLI's own pre-flight, not as
  a promise about what the server would have done.

  Cost keys (`cost.factors`, `cost.fixed`) are server-owned and passed through
  **verbatim**, so treat them as an open map rather than a fixed set.

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

The lockfile also has to **be** one, not merely exist. A `package-lock.json`
must parse as JSON and declare a numeric `"lockfileVersion"` of 1 or more; a
`pnpm-lock.yaml` or `yarn.lock` must be non-empty. That version rule is
deliberately **stricter than `npm ci` measures** — npm states it as a
precondition but will happily install from an otherwise-intact lockfile whose
version key is `0`, a string, `null` or absent — and `validate` keeps it because
npm never *writes* those shapes, so a file carrying one was made by hand.
An **empty** lockfile fails the platform build exactly like a missing one, so
`touch package-lock.json` is not a fix — run the package manager and commit what
it writes. If the lockfile cannot be read, or is implausibly large, `validate`
says nothing rather than guessing: it never blocks a submit on a file it could
not inspect.

Finally it emits one **advisory** about the
[host handshake](#the-host-handshake-block_ready): if your manifest declares a
`page` surface and **nothing your app loads posts `BLOCK_READY`**, `validate`
says so. That is the shape of an app scaffolded before the templates were fixed
(#206) — it renders perfectly everywhere you can look locally and is replaced by
a failure card in the real host. It is a **warning, never an error**: unlike the
lockfile rule (where the platform build provably dies), this one infers
*runtime* behaviour from *static text* and can be wrong, so it must not fail a
correct project.

> ⚠️ **Copying `civitai-host.js` in is only half the fix.** A browser never
> fetches a file nothing references, so the emitter has to be *loaded* too — a
> `<script src="./civitai-host.js"></script>` in `index.html`, or an
> `import './civitai-host.js';` at the top of the entry module `index.html`
> loads. Earlier releases of this check looked only for the *text*
> `BLOCK_READY` anywhere in your tree, so an unreferenced copy silenced it and a
> still-broken app validated clean. It now resolves what your `index.html`
> actually loads.

Four things follow:

- **It checks REACHABILITY where it can, and says when it can't.** Starting at
  `index.html` it follows every `<script src>`, inline module and `import` it can
  resolve, and asks whether any of *those* files posts the message. When that
  resolution is complete you get a precise finding — including "you have an
  emitter, but nothing loads it". When it *isn't* — no `index.html` at your
  project root, a bundler alias (`import '@/…'`), a reference to a file that
  isn't there, an import chain deeper than it follows — it falls back to
  scanning your whole tree for the text, and the
  warning **says so in as many words**: *"it did NOT check that the file is
  loaded"*. Read that sentence as it is written; in that mode, adding the emitter
  without referencing it will silence the warning and leave the app broken.
- **A dependency that acks ends the check** — today that is
  `@civitai/blocks-react`, and nothing else. Its iframe transport acks internally
  and the literal never appears in your `src/`, so a `page-money` app is never
  flagged. This is an *exact* list, not the `@civitai/` scope:
  `@civitai/app-sdk` is the server-side SDK and no runtime code in it posts
  `BLOCK_READY`, and `@civitai/theme` / `@civitai/components` are CSS. Depending
  on those does not give you the handshake, so it does not silence the check
  either.
- **It reads source only** — never `node_modules`, never the conventional build
  directories (`dist`, `build`, `out`, …), and never a `.md` file: a README
  *describing* the handshake is not an implementation of it. Comments are
  stripped too, so a comment naming `BLOCK_READY` does not satisfy it. A `src`
  that is a **symlink** into a shared package *is* followed.
- **It stays quiet when it cannot see the whole project.** An unreadable file, a
  file over 2 MiB, a very large tree, or a directory holding only a manifest all
  mean "we could not look" — reported as nothing, never as a finding. Likewise a
  project whose entry graph can't be resolved *and* which contains the literal
  somewhere: quiet, deliberately, because warning at a correct project is the
  more expensive mistake.

If it fires on a project you know is correct — your ack arrives from a bundled
dependency, or from a file type this scan doesn't open — it is a false alarm, and
it never blocks (exit 0) unless you pass `--strict`. What it proves stays narrow
even at its strongest: that a file your `index.html` really loads *mentions* the
message. It cannot prove the ack ever **fires**; only the real host can.

`civitai app submit` prints the same warnings before it uploads, and likewise
does not block on them.

> ⚠️ **If you already run `civitai app validate --strict` in CI**, this advisory
> is new and can turn a previously-green project red — which is what `--strict`
> asks for. If it is a false alarm for your project, drop `--strict` or add the
> ack, and please open an issue: a warning at a correct project is a bug in the
> check, not something you should have to work around.

The **durable fix** is a server-side `civitai app validate` endpoint that calls
the real `BlockManifestValidator` (the faithful contract), with this schema
published as the syntactic half. See [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) for the full
caveat and how the vendored schema + Go checks are kept in sync.

### The `--json` result shape

```jsonc
{
  "ok": false,
  "dir": "./my-block",
  "errors":   [ { "field": "iframe.sandbox", "message": "…" } ],
  "warnings": [ { "field": "page.buzzBudgetPerGen", "message": "…" } ]
}
```

Two guarantees, both of which the previous release broke:

- **`field` is present on every finding, and is never `null` or empty.** It used
  to be recovered by parsing the message text, which worked only for the JSON
  Schema errors — so every *semantic* finding (the sandbox rules, the
  justification rules, the money-path warnings: the ones a local pre-check exists
  for) arrived as `"field": null`, and grouping by field in CI silently dropped
  them. Findings now carry their field from where they are produced.
- **One notation: dotted paths.** `blockId`, `iframe.sandbox`, `scopes[1]`,
  `targets[0].slotId`, `scopeJustifications.<scope>` — the same way the
  human-readable messages, this README and the schema all name fields. Earlier
  releases mixed JSON Pointer (`/blockId`, `/scopes/1`) into `--json` while the
  text output used dotted; **if you were matching on `/`-prefixed fields, update
  your scripts.**

Two findings have no single manifest field, and say so explicitly rather than
omitting the key:

| `field` | meaning |
| --- | --- |
| `(root)` | the manifest **document** — it is missing, unparseable, or the schema reports a violation at the top level (e.g. `missing property 'contentRating'`). |
| `(project)` | **repository state outside the manifest** — the committed lockfile, or the source tree the `BLOCK_READY` advisory reads. No manifest edit alone resolves these. |

`ok` already accounts for `--strict`: it is `false` when there are hard errors,
and also when `--strict` is passed and there are warnings. The process exit code
matches, and the JSON goes to **stdout** while the failure is reported on
**stderr** — so `civitai app validate --json | jq` works on a project that fails
*validation*.

🔴 **BREAKING — a refused path now emits no object at all.** This object is
written only when validation actually produced a result. A path that does **not
exist**, or that is not a directory, is a mistake about the invocation: it writes
**nothing** to stdout and exits `2`. It used to print
`{"ok": false, "dir": "/nope", "errors": [ … ]}` and exit `1` — a fabricated
validation result, complete with a finding about a manifest nobody could have
written. A failure that produces **no validation result at all** likewise emits
no object and exits `1` — a project directory the CLI cannot **stat** (it is
unreadable, or a component of the path below it is not a directory), and in
principle an internal schema failure, which is a directory the CLI *can* read
that still yields nothing to print.

> **An unreadable `block.manifest.json` is not one of those cases** — it is a
> validation *verdict*, and the object is printed in full with a single
> `(root)` finding carrying the `permission denied` message. The distinction is
> how far the CLI got before it stopped: it could not read your *manifest*,
> which is something to report about the project; it could not read the
> *directory*, which is nothing at all.

**So branch on the exit code before parsing:**

| exit | stdout |
| --- | --- |
| `0` | the object, `"ok": true` |
| `1` | the object with `"ok": false` for a validation **verdict** — including an unreadable manifest — but **nothing** when validation produced no result at all (an unreadable project *directory*, or an `ENOTDIR` partway down the path; also an internal schema failure, which a released binary should never hit) |
| `2` | **nothing** — the path does not exist, or is not a directory |

🔴 **`jq -e` is the wrong tool for reading `ok`.** It exits `1` on a JSON
`false`, which is indistinguishable from its exit code for a missing key — so
the obvious one-liner reports a *failing but perfectly well-formed* result as
"no result", which is the one distinction this whole section exists to draw.
Test for an empty string instead, and read `ok` as a value:

```bash
out=$(civitai app validate ./my-block --json); rc=$?
case $rc in
  2) echo "bad path — check the argument"; exit 2 ;;
  0|1)
    if [ -z "$out" ]; then
      echo "no result to parse (rc=$rc)"; exit "$rc"
    fi
    echo "ok=$(jq -r .ok <<<"$out")"          # true | false — the verdict
    jq -r '.errors[]   | "ERROR   \(.field): \(.message)"' <<<"$out"
    jq -r '.warnings[] | "WARNING \(.field): \(.message)"' <<<"$out"
    ;;
esac
```

## Submit & auth

`civitai login` (no flags) runs the **OAuth device-authorization grant**: it
prints a URL + a short code, you approve in your browser, and the CLI stores a
short-lived access token (1h) plus a refresh token (30d) that it rotates
automatically before requests and once on a `401`. By **default** it requests
`UserRead | AppBlocksSubmit | AppBlocksDevTunnel` (== `100663297`) — identity,
Apps submit (which gates both `app submit` and the dev-token mint), and the
on-site dev tunnel. That default deliberately omits `AIServicesWrite`, so a plain
login **cannot spend Buzz**: it drives the **read/identity** `dev:live` paths
(viewer, catalog, app storage) but, for a generation app, has its
`ai:write:budgeted` scope stripped at mint time and **cannot estimate, submit, or
spend real Buzz**.

Opt into generation explicitly with a **named scope set**:

```bash
civitai login --scopes generate   # additive: keeps submit + dev-tunnel, ADDS generation
```

which requests `100777985` (the default **plus** `AIServicesRead |
AIServicesWrite | BuzzRead`) — one credential that can both submit apps and run
[`civitai generate`](#generate). `--scopes` takes a **named set**, never a raw
bitmask, and an unknown name is rejected with the valid list. It applies only to
the browser device login and is refused alongside `--token`.

> 🔴 **The device-flow scope check is all-or-nothing**: requesting any bit the
> `civitai-cli` OAuth client's `allowedScopes` does not permit rejects the
> **whole** login with `invalid_scope`. On `civitai.com` that client's
> `allowedScopes` **is** `100777985`, so `--scopes generate` works. Against a
> **self-hosted or older** auth server (a non-default `CIVITAI_BASE_URL`) that
> predates the widening it is rejected, and the CLI maps the rejection to a
> message telling you plain `civitai login` still works there.

A full-scope **personal API key** remains the other way to get spend authority
(see the credential table under
[Local dev loop](#local-dev-loop-harness-mock-vs-live) above), and is still the
only credential carrying the rest of the Full scope mask.
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

#### How big can a bundle be?

**Nobody here knows, and this CLI will not pretend otherwise.**

`app submit` reports three sizes:

```
Packaged 68 file(s) (8201270 bytes compressed, 8866319 decompressed; 10935065 bytes as the base64 JSON submit body)
```

🔴 **The third one is the one a request-body limit applies to.** The `.zip` is
not what goes on the wire: it is base64-encoded into a JSON document
(`{"bundleBase64":"…"}`) before it is sent, so the server receives about **4/3**
of the compressed size. That number is exact, not an estimate, and it is printed
on every path — including `--package-only`, which is how you inspect a bundle
you cannot submit.

**The size caps this CLI enforces are its own, not the server's.** They are
generous (2000 files, 10 MiB per file, 50 MiB compressed, 200 MiB decompressed)
and clearing them is **not** a prediction that the submit will be accepted.
[#423](https://github.com/civitai/cli/issues/423) is the measured
counterexample: an **8.20 MB** compressed bundle passed every one of them and
was refused, while a **2.32 MB** one was accepted. That brackets the server's
real ceiling to somewhere in **(2.32 MB, 8.20 MB]** — and no further, because
each probe costs a real submission.

The CLI deliberately does **not** vendor a number from inside that bracket. A
local refusal at a guessed limit would reject bundles the server accepts, with
no override and nothing to tell you it was a guess rather than the real rule —
worse than the failure it would be fixing. So it **reports** instead: the size
above before you submit, and, when a submit fails, what it sent.

**What the refusal looks like.** The server answers `400: Invalid JSON` — an
error about the *parse*, downstream of the cause, naming nothing size-shaped.
(That string is the server's own, printed verbatim; there is no better message
hiding inside the response.) The CLI adds what it knows on top:

```
Error: server returned 400: Invalid JSON

What this CLI sent (it cannot tell whether that is why the submit failed):
  10935065 bytes on the wire — a 8201270-byte zip, base64-encoded into a JSON body.
  largest entries in the bundle (compressed / original):
       2411008 / 2418844     docs/screenshots/flow-01.png
       1904772 / 1911233     docs/screenshots/flow-02.png
       …
```

Entries are ranked by **compressed** size, because that is what the upload is
made of — a large text file that deflates to nothing is not what to delete. In
#423 the cause was a `docs/screenshots/` directory of review PNGs, which
`app submit` packages along with everything else; finding it took
`--package-only` plus `unzip -l`, which is what that table replaces. The account
is printed for any refusal that might be about the bundle, and deliberately
**not** for a `401`/`403` (a credential problem, unrelated) or a `429`.

#### What the packager left out

Under the `Packaged …` line, `app submit` prints one more line naming every path
it skipped — on every path including `--package-only`, and not at all when it
skipped nothing:

```
Skipped 4 path(s): public/environment.env (*.env), .git/, dist/, node_modules/
```

- The count is of **skip decisions, not files**. An excluded directory is one
  entry, printed with a trailing `/`: the walk stops there and never learns how
  many files are underneath, so no number for that is printed rather than a
  number that was guessed.
- A **pattern** rule is tagged with the pattern that matched — `(*.env)`,
  `(.env*)`, `(*.zip)`, `(.env or .env.*)` for a dotenv-shaped directory, and
  `(not a regular file)` for a symlink or other non-regular entry. That tag is
  the actionable part: it says which rule reached the file, so it says that
  renaming recovers it. Tagged entries are listed first for that reason.
- A **fixed** name from the excluded-directory list (`node_modules`, `dist`,
  `.git`, …) carries no tag — `civitai app submit --help` prints that list
  verbatim.
- Past 12 entries the list is elided with `… and K more`; the count before the
  colon still counts every one.

This does not change what is packaged. It changes an exclusion from something
you meet at runtime in the deployed app into something you read at submit time.

#### Which dotenv files end up in the bundle

"The CLI excludes dotenv files" is the natural reading of the `.env.development*`
sentence above, and it is **not** what the packager does. The rule is a
**three-name allow-list with a catch-all**, not a blanket exclusion and not the
enumeration the table below might otherwise read as:

> **Every file whose base name starts with `.env` is excluded — except
> `.env.example`, `.env.sample` and `.env.production`, which are always
> included.**

| file | in the bundle? | why |
| --- | --- | --- |
| `.env`, `.env.local`, `.env.*.local`, `.env.development`, `.env.test` — **and every other `.env*` name**, dotted or not: `.env.staging`, `.env-local`, and `.envrc` | **excluded** | the catch-all: any `.env*` the allow-list does not name is assumed dev-local and secret-bearing. The money template points a real `VITE_LIVE_BLOCK_TOKEN` at the git-ignored `.env.development.local`; `.envrc` is the direnv convention and routinely holds exported credentials. |
| `.env.example`, `.env.sample` | **included** | meant to be placeholder templates the reviewer reads — **but see below: the allow-list is by NAME and nothing reads the contents**, and the money template's own `.env.example` carries an empty `VITE_LIVE_BLOCK_TOKEN=` line whose comment sends the real token to `.env.development.local` **because this file is uploaded**. A test pins the *scaffolded* line empty; nothing checks the copy in **your** project, so the packager will upload whatever you put there |
| `.env.production` | **included** | the platform build runs `vite build` in production mode, which reads it |

**Directories count too, and by a narrower rule.** A *directory* named `.env` or
beginning with `.env.` — `.env.d/`, `.env.local/`, `.env.secrets/` — is excluded
whole, at any depth, and so is one whose name ends in `.zip`. The three-name
allow-list does **not** apply to directories: `vite build` reads a dotenv *file*,
so a directory called `.env.production/` is dropped like any other.

The directory rule deliberately stops at the dot, which the file rule does not,
because matching too much here removes a whole subtree from your submission with
nothing to tell you: `.environment/` and `.envoy/` still ship.

**A file is also dropped when its name *ends* in `.env`** — `db.env`,
`prod.env`, `local.env`, `config.env`, at any depth and in any directory. That
is the shape tooling writes, and until this rule existed no rule saw it: every
dotenv rule was a *prefix* rule, so a name not starting with `.env` was
invisible to all of them. This one is **files only** — a directory called
`config.env/` still ships, because dropping a whole subtree on a suffix match is
the silent loss the directory rule is aimed away from.

Matching is **case-insensitive** for every dotenv and `*.zip` rule, on
directories and on files alike: `.ENV.LOCAL/`, `.ENV.LOCAL`, `X.ZIP/` and
`Bundle.ZIP` all go. (The *fixed-name* directory list is the exception — see the
table below.) The three kept names are still matched
**exactly** — `.ENV.PRODUCTION` is not the file `vite build` reads, so it is
dropped rather than uploaded.

🔴 **This still closes shapes, not the class.** The packager matches *names*,
never contents. Measured, these are **packaged** today:

| still uploaded | why it slips through |
| --- | --- |
| `.env-backup/.env.production` | the three kept names are kept **by name**, wherever every directory above them is itself packaged |
| `NODE_MODULES/`, `Dist/` | the *fixed-name* directory list is matched case-sensitively, on purpose — `Build/` and `Dist/` are plausible content directory names, and dropping one is a silent subtree loss. The two *pattern* rules (`.env…`, `*.zip`) **are** case-insensitive |
| a secret in a name that is not dotenv-shaped at all | `secrets.json`, `credentials.yaml`, a key pasted into `src/config.ts` — the packager matches **names, never contents** |

⚠️ **The `*.env` rule costs something, and the packager now says so.** `.env` is
also **Babylon.js's environment-texture format** — a 3D block shipping
`public/environment.env` will have it dropped. `sample.env` and `template.env` go
the same way, and the three-name allow-list has no suffix-shaped counterpart:
`.env.sample` is kept, `sample.env` is not. Rename the file
(`environment.envmap`) and it travels.

This used to be invisible until runtime in the deployed app, because the submit
output printed a file *count* and named nothing. It now prints a second line
listing what it left out, with the rule that matched:

```
Packaged 38 file(s) (49213 bytes compressed, 118442 decompressed; 65637 bytes as the base64 JSON submit body)
Skipped 4 path(s): public/environment.env (*.env), .git/, dist/, node_modules/
```

Read the tag: it names the rule, so it also names the fix. The line is printed on
every path including `--package-only`, and not at all when nothing was skipped.

The mirror of the `.env-backup/.env.production` row: a kept name does **not**
rescue its directory. `.env.d/.env.production` is dropped along with `.env.d/`,
and so is `node_modules/pkg/.env.production` — the walk skips an excluded
directory before it ever looks at a file name, at any depth beneath it.

Keep secrets in a `.env`-dotted name — `.env.local`, `.env.d/` — and both rules
drop them. Anything else is on you to check before you submit; `--package-only`
writes the exact `.zip` that would be uploaded, so unzip it and look.

🔴 **The allow-list is by FILE NAME. Nothing inspects what is inside those three
files, so nothing stops one of them carrying a secret to the platform.** Whatever
you put in `.env.example`, `.env.sample` or `.env.production` is packaged and
uploaded verbatim. **Do not put a token in any of them** — not a `VITE_`-prefixed
one (Vite inlines those into the client bundle, so they are public the moment
your app loads) and not a plain unprefixed one either (Vite leaves that out of
the bundle, but the CLI still ships the file). Put nothing in the three kept
files you would not paste into a public page.

⚠️ **If your project was scaffolded before this was documented, check it by
hand.** The money template's `.env.example` used to say *"Paste it here"* next to
`VITE_LIVE_BLOCK_TOKEN`. Templates apply at `civitai app create` only — an
existing project keeps whatever text (and whatever value) it already has, and
nothing in `validate` or `submit` inspects a dotenv file's contents. Open
`.env.example`, make sure that line is bare, and if a real token was ever there,
treat it as disclosed and mint a new one (`civitai app dev-token <slug> --env >>
.env.development.local`).

`.env.production` being **shipped** is the one worth knowing about, because it is
the least expected: the server-side build needs it, and the scaffolded file holds
one public value, `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`. That is what the
*scaffolded* file holds — it is not a guarantee about yours.
`.env.production.local` is *not* kept: `.local` is the dev-local override
convention and falls to the catch-all with everything else.

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

**Budget time for artwork.** Review and deploy are not the only gate: a store
listing **cannot go live without an icon AND a cover**. `civitai app submit`
mints your listing as a **draft** and prints this reminder inline — which is the
moment to act on it, because the media is settable **while the app is in review**
and will carry forward on APPROVAL only — withdrawing the submission, or a
moderator rejecting it, deletes the listing and everything on it. Attach it
without the browser:

```text
$ civitai app listing status                        # what's attached vs. required
App:             my-app
Listing status:  draft
Icon:            MISSING (required)
Cover:           MISSING (required)
Screenshots:     0

⚠ Not publishable yet — missing icon and cover.
  Add one:  civitai app listing set-icon <file>
  Add one:  civitai app listing set-cover <file>

$ civitai app listing set-icon ./assets/icon.png    # png/jpeg/webp, ≤2 MiB
$ civitai app listing set-cover ./assets/cover.png  # png/jpeg/webp, ≤4 MiB
$ civitai app listing add-screenshot ./assets/screenshot-1.png --caption "Grid view"   # optional, up to 8
```

`assets/` is scaffolded by every template, with a README of the requirements and
**no placeholder images** — the files above are ones you supply.

**Every `app listing` subcommand has to work out *which* app you mean, and it
does that from the working directory** — it reads `blockId` out of the
`block.manifest.json` next to you. Run from somewhere else and there is no
manifest to read, so it stops before it builds a request:

```text
$ cd /tmp && civitai app listing status
Error: could not resolve the app — run this from your app directory (with block.manifest.json) or pass --slug: no block.manifest.json found in . — is this an App project? run `civitai app init` to create one
```

Two flags name the app instead: **`--slug <blockId>`** skips the manifest
entirely, and **`--dir <path>`** points at the app directory from wherever you
are. Both work on every `listing` subcommand.

Details worth knowing before you start:

- **Source images** are png/jpeg/webp and are size-checked **locally before any
  upload** — icon ≤2 MiB, cover ≤4 MiB, screenshot ≤2 MiB. That is the whole of
  what the CLI **enforces**.
- **Dimension and aspect rules are the platform's, and it states them.** The CLI
  does not enforce them locally — a copied number goes stale and starts refusing
  valid images — but it does *document* the current bounds, as guidance rather
  than as a gate: see
  [Listing media requirements](#listing-media-requirements). It uploads,
  **attaches, and then waits for the content scan** — in that order, because the
  platform validates dimensions, aspect and format at the *attach* step. So a
  wrongly-shaped image comes back in a couple of seconds with the platform's own
  message naming the bound and your value
  (e.g. `icon must be square-ish (aspect 2.00 outside 0.9–1.1)`), instead of
  after the scan has finished.
- **A blocked image never goes live.** The scan verdict is still waited on, so
  these commands never report success on a pending or blocked scan; a failure
  tells you what state the listing was left in.
- **The app is resolved from `block.manifest.json`** in the current directory;
  pass `--slug <blockId>` (with `--dir` if you prefer) to run it from anywhere.
- **On a listing that is already LIVE**, attaching media does not edit the live
  listing — it opens a **revision** that goes back to moderator review. Describe
  it with `--changelog "<what changed>"`; `-y`/`--yes` skips the confirmation.
  `civitai app listing status` on a live listing reports that in-progress
  revision's media, and tells you when a revision is already under review.
  **`reorder` works the same way** — it opens the revision, reorders *its*
  gallery and submits it — with two differences: it takes `--changelog` but has
  no confirmation to skip, so there is no `-y`, and it never uploads anything.
- **A live listing that is still below the publish floor stages, and does not
  submit.** A revision cannot go to a moderator until the listing has both an
  icon and a cover, and clearing that floor takes two commands — so the first
  one attaches its image to the revision (or, for `reorder`, writes the new
  order into it), leaves the revision **open and
  unsubmitted**, prints what is still missing, and **exits `0`**. Nothing
  failed: the second command reuses the same revision and submits both together.
  The output says `staged on an open revision — not submitted for review yet`,
  never `pending moderator review`, and your live listing is untouched
  throughout. This applies only when the server actually **refused** the submit
  for the floor: an outage (`500`, `503`) or a listing the CLI cannot read back
  still fails, with its usual exit code — see the
  [Troubleshooting row](#troubleshooting) for the exact conditions.
- **Screenshots** are managed by id (the `alsc_…` ids `listing status` prints):
  `rm-screenshot <id>` removes one, and `reorder <id...>` takes **all** the
  current ids in the new order — a partial set is rejected.
- **`civitai app listing status --json` is the scriptable form, and it names the
  two listing ids the human output never shows.** A live listing has a *parent*
  and, once a revision is open, a *shadow* — and which one a change is addressed
  to is what decides whether the server accepts it (that gap is what
  [#430](https://github.com/civitai/cli/issues/430) was). `--json` reports both:

  ```console
  $ civitai app listing status --slug my-app --json
  {
    "slug": "my-app",
    "parentId": "apl_01KXPENN7GJV51HBG649P3Y99M",
    "shadowId": "apl_01M064YJGDR973P73WHC1FGDP9",
    "status": "approved",
    "hasPendingRevision": false,
    "assets": {
      "icon":  { "present": true,  "imageId": 4711 },
      "cover": { "present": false, "imageId": null },
      "screenshots": [
        { "id": "alsc_01M0…", "order": 0, "caption": "Grid view", "imageId": 8123 }
      ]
    },
    "floor": { "met": false, "missing": ["cover"] }
  }
  ```

  `shadowId` is **`null`** when no revision draft exists (every draft or pending
  listing, and a live one nobody has edited), never absent — so `.shadowId`
  answers on every listing. `status` is the **parent's** lifecycle status;
  `hasPendingRevision` means *submitted for review*, not "a shadow exists".
  `floor.missing` is `[]` when the floor is met. The server's own
  **`editTargetId` is deliberately not there**: this CLI does not decode that
  field, and printing an id it never read would be a guess — use `shadowId` when
  it is non-null and `parentId` otherwise.
- 🔴 **`listing status` is NOT a pure read — `--json` or not — so do not poll it
  in a loop.** On a LIVE (approved) listing the read behind it
  (`getMyListingForEdit`) opens the revision draft server-side, idempotently —
  the same one each time — so a script calling it repeatedly keeps a revision
  open on your listing. Whether that should still be classified a read is
  [#389](https://github.com/civitai/cli/issues/389), which is open. Reading it
  once per change is fine; a watch loop is a writer.
  ⚠ **This now applies to OFFSITE apps too.** Before
  [#422](https://github.com/civitai/cli/issues/422) every `app listing`
  subcommand refused for an offsite app and therefore could not write anything;
  now that they resolve by slug, `app listing status` against an **approved**
  offsite listing mints that shadow revision like any other. Every offsite app
  measured on civitai.com is approved, so this is the *normal* state there, not
  an edge case. #422 retired the "cannot be addressed" refusal; it did not
  retire #389 — the shadow is a property of `getMyListingForEdit`, not of how
  the listing id was obtained.
- 🔴 **`reorder` addresses the same listing `status` printed, and on a live
  listing that is the *revision*, not the live one.** The server validates a
  reorder against the screenshot ids of the listing it is *addressed to*, and
  those two id sets differ while a revision is open — so until
  [#430](https://github.com/civitai/cli/issues/430) every reorder of a live
  listing was refused with `orderedIds must be exactly the listing's current
  screenshot ids`, naming the ids the CLI had *just printed*. It now opens the
  revision, reorders that, and submits it: you get `New screenshot order staged
  on a revision — pending moderator review (alpr_…)`, and your live gallery is
  unchanged until that revision is approved. On a **draft** listing there is no
  revision, so it reorders the listing itself and prints
  `✓ Reordered N screenshots`.
- 🔴 **On a LIVE listing, `rm-screenshot` STAGES the removal and does not
  publish it — and until
  [#436](https://github.com/civitai/cli/issues/436) it said `✓ Screenshot
  removed` and exited `0` anyway.** The ids `listing status` prints on a live
  listing belong to the open **revision**, and `removeScreenshot` resolves the
  listing it edits from the row itself, so the removal lands in that revision.
  That is correct; what was wrong is that nothing said so and **no command could
  submit the revision**, so the change reached the public gallery never.
  Measured on `model-benchmarking`: after the removal `listing status` reported
  3 screenshots while `GET /api/v1/apps/model-benchmarking` still returned 4.
  It now prints `Screenshot removal staged on an open revision — not submitted
  for review yet`, says your live listing still shows the screenshot, and names
  `civitai app listing submit-revision`. On a **draft** listing the removal is
  the listing, and `✓ Screenshot removed` is what you get, unchanged.
- **`civitai app listing submit-revision` sends the open revision to moderator
  review** — the explicit publish step for anything staged in it, and the only
  way to publish a removal. `--changelog "<what changed>"` describes it for the
  moderator. It is **not** implicit in `rm-screenshot`: curating a gallery is
  usually several removals, and auto-submitting on the first would open a review
  cycle in the middle of an edit. (A command that makes ONE complete change does
  submit the revision it opened — the attach commands `set-icon`, `set-cover`
  and `add-screenshot`, and `reorder` since
  [#430](https://github.com/civitai/cli/issues/430), do so today; see the notes
  above.) It refuses,
  without submitting, when the listing is **not live** (a draft or
  pending listing is edited directly, so there is no revision) and when there is
  **no open revision** to submit — a moderator never receives an empty one.
  Submitting is idempotent: a revision already awaiting review returns that same
  request rather than opening a second. ⚠ **A submit refused because the listing
  is below the publish floor FAILS here** (exit `2`), unlike the attach commands
  above — the submit *is* what you asked for, so reporting success would be the
  same false claim [#436](https://github.com/civitai/cli/issues/436) is about.
  The floor gap is printed as context alongside the server's refusal.
- 🔴 **`app listing` WORKS for an OFFSITE app** — every subcommand
  (`status`, `set-icon`, `set-cover`, `add-screenshot`, `rm-screenshot`,
  `reorder`, `submit-revision`), because they all resolve the same way. An
  offsite app is a registered URL rather than a block bundle, so it has no block
  **submission**; the CLI resolves that first (the onsite path) and, when there
  is none, falls back to selecting the listing **by slug**. That fallback needs
  `appListings.getMyListingForApp`'s slug selector, which
  [`civitai/civitai#3989`](https://github.com/civitai/civitai/pull/3989)
  rescoped and which is deployed on civitai.com — measured 2026-08-17: four
  offsite apps and one onsite control all resolved by slug, an unknown slug
  still 404ed. Tracked as [#422](https://github.com/civitai/cli/issues/422).
- **If BOTH lookups answer *not found*, the CLI says so and points at the
  website.** That happens against a Civitai that predates `#3989` (an older or
  self-hosted deployment), or for an app with no listing row. A listing your
  account does **not own** is not one of those on a current server: the by-slug
  lookup resolves the row and then refuses it `403`, so you get that error and
  exit `3`, not this message.
  The message names those cases, names `civitai app view <slug>`
  (which still shows the icon and cover the listing is serving), and names the
  **App-store listing UI on civitai.com** as the surface that always works. It
  explicitly says `civitai app submit` is *not* the missing step, because it
  cannot create a submission for such an app. Exit `4`, the same as any other
  slug this command cannot resolve.
  **That wording is best-effort; the exit code is not.** Naming the app as
  offsite takes one extra lookup against the public store catalog
  (`GET /api/v1/apps/{slug}`), which answers only for a **published** store
  listing and is still behind a launch flag — until the catalog opens publicly
  you see an app there only as a moderator or app-dev-tester. If that lookup
  cannot answer (unpublished listing, no such access, or a network/5xx failure)
  you get the generic `no such submission … run civitai app submit first`
  message instead, still on exit `4`.
- **A failure that is not a 404 keeps its own message and its own exit code —
  from *either* lookup.** A `403` from the invite-gated submissions route, a
  `5xx` or a dropped connection is not evidence about whether a listing exists,
  so the submissions lookup is never retried by slug and the by-slug lookup's
  own failure is never reported as the "listing not reachable" message above.
  The codes are **not** just `3` and `5`: measured end-to-end, `401`/`403` →
  `3`, `429` → `6`, `502`/`503`/`504` and a dropped connection → `5`, and a
  plain **`500` → `1`**. See [Exit code 4](#exit-code-4).

**Need to change the bundle while a request is still `pending`?** Withdraw it
first to free the slug, then resubmit:

```text
$ civitai app status                          # find the pubreq_ id
$ civitai app withdraw pubreq_01HZX           # frees the slug — and DELETES the store listing
$ civitai app submit                          # resubmit the new bundle (listing starts EMPTY)
$ civitai app listing set-icon ./assets/icon.png    # re-attach the media
```

> **⚠ Withdrawing a first-version submission deletes that app's store listing**
> — its icon, its cover and **every screenshot with its caption**. That is
> server-side and the CLI cannot opt out of it: the withdraw route takes the
> publish-request id and nothing else. Resubmitting mints an **empty** listing;
> the media does not come back, and neither do the captions. The same discard
> happens when a moderator **rejects** a first-version submission. A withdraw on
> an app whose listing is already **approved** (a subsequent version) leaves the
> live listing alone.
>
> Because that is irreversible, `civitai app withdraw` **asks first** on a
> terminal, and **refuses** in a non-interactive shell unless you pass `--yes`.

`civitai app withdraw <pubreq-id>` (or `--id <pubreq>`) withdraws **your own**
pending publish request. It is **idempotent with respect to the submission**
(an already-withdrawn request still returns success) and only a **`pending`**
request can be withdrawn — an already approved/rejected one cannot. The
idempotency does **not** extend to the listing: the first withdraw deletes it,
and a second call does not bring it back.

### Listing media requirements

Two different things check your listing images, and it is worth knowing which is
which before you open an image editor.

**What the CLI checks, locally, before anything is uploaded:** the file's
**format** and its **byte size**. That is all — and for an **icon** that is
deliberately not the whole story, because the platform measures an image the CLI
never sees (the re-encode note below). What the CLI *does* do is show you the
quantity every server-side bound is a function of, on the line it uploads:

```text
Uploading icon (37.3 KiB, 1024×1024)…
```

| kind | how many | format | byte cap (the file you pass) |
| --- | --- | --- | --- |
| **icon** | 1, required | png / jpeg / webp | ≤ 2 MiB |
| **cover** | 1, required | png / jpeg / webp | ≤ 4 MiB |
| **screenshot** | up to 8, optional | png / jpeg / webp | ≤ 2 MiB |

**What the platform checks, server-side, when the image is attached:** the
dimensions and the aspect ratio. The CLI does not reproduce these, so this table
is *guidance* — the server is the authority and its rejection names the bound and
your value (`icon must be square-ish (aspect 2.00 outside 0.9–1.1)`).

| kind | aspect (width ÷ height) | minimum size |
| --- | --- | --- |
| **icon** | 0.9 – 1.1 — square or near-square (1:1 is fine) | 128 px on the shorter side |
| **cover** | 1.3 – 2.4 — landscape, ~4:3 to ~21:9 | 640 px wide |
| **screenshot** | 0.4 – 2.6 — either orientation | 320 px on the shorter side |

Easy starting points: a **512 × 512** icon and a **1600 × 900** cover.

Four behaviours that are not obvious from the numbers:

- **Icons are re-encoded server-side, and that re-encode is what gets capped.**
  Whatever you upload is downscaled to at most **1024 px** on its longer side and
  re-encoded to PNG — aspect preserved, and **never enlarged**, so an undersized
  icon is not rescued: the 128 px floor still bites. The platform then caps that
  **re-encoded** image at 1 MiB, a different measurement from the 2 MiB the CLI
  applies to the file you pass — **and it is not a corner case.** Measured on
  2026-08-10: a **1024 × 1024** photographic JPEG of **37.3 KiB** — under 2% of
  the local cap — was refused at attach, because the PNG the platform made from it
  was about **1.15 MiB**. A second source at the same 1024 px ceiling re-encoded
  to about **2.1 MiB**; a **512 × 512** icon in the same run went through. So the
  bytes in that rejection are the platform's, not your file's, and the lever is
  **pixel dimensions, not heavier compression**. The CLI cannot predict the
  number — the re-encoded size depends on how compressible your artwork is, and
  flat, simple artwork re-encodes far smaller — so it prints the dimensions it
  decoded and repeats this mechanism in the error rather than guessing a bound.
- **An icon's upper bound is a PIXEL count, not a file size.** The decoder that
  re-encodes it refuses a source above roughly **16 megapixels** — about
  4096 × 4096 — and it refuses it *regardless of how small the file is*. A flat
  5000 × 5000 PNG compresses to a few hundred KB, so it clears every byte cap in
  the first table and is still rejected. Downscale before you upload:
  **1024 × 1024** is plenty, because that is what the server re-encodes to
  anyway.
- **Covers and screenshots are not rescaled.** What you upload is what the store
  renders, so ship them at the size you want shown.
- **A wrong image is rejected, not quietly accepted, and it comes back fast.**
  The CLI attaches *before* it waits on the content scan, so the platform's
  verdict on shape arrives in a couple of seconds rather than after a scan that
  can take two minutes — and always before a moderator sees it. The message names
  the bound it applied and the value it measured — read it rather than guessing
  which limit you crossed.

> **Why the CLI does not enforce the second table.** These are platform
> constants that can move. Stale *guidance* costs you one rejection that carries
> the current bound; a stale local *gate* refuses valid images and cannot be
> argued with. The asymmetry is the reason the split exists — please don't
> "helpfully" promote these numbers into a local check (see
> [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) item 25).

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

**An OFFSITE app has no block submissions at all**, and where the CLI can tell,
asking about one says that instead of telling you to submit. `civitai app
status` reads the block-submission pipeline; an offsite app is a registered URL,
not a block bundle, so there is nothing there to be pending, approved or
deployed — nothing is missing and `civitai app submit` would not create one. The
message names `civitai app view <slug>` for what the CLI *can* show about it.
Exit `4`, like any other slug this lookup cannot resolve.

Recognising it takes one extra lookup against the public store catalog
(`GET /api/v1/apps/{slug}`), and that route serves only a **published** store
listing and is still behind a launch flag — until the catalog opens publicly you
see an app there only as a moderator or app-dev-tester. So an offsite app whose
listing is unpublished, a caller without that access, or a network/5xx failure
of the lookup all still get the generic `no such submission … run civitai app
submit first` message, on the same exit `4`. The wording is best-effort; the
exit code is not.

The unfiltered listing is **capped server-side at 100 rows**, and the API returns
no cursor and no total — so there is no way to page and no way to know how many
were dropped. When a full-length page comes back the CLI says so on **stderr**
rather than presenting it as your complete history:

```text
note: the server returned the newest 100 submissions — the API caps this listing and offers no way to page, so older submissions may exist but are not listed. Look up a specific app with `civitai app status <blockId>`.
```

That is an inference (a page that is exactly full is indistinguishable from one
that was cut off), so it says *may*. A per-app lookup — `civitai app status
<blockId>` — is **not** affected: the server narrows to the slug before applying
the cap.

`--limit N` shows only the newest **N** of the submissions that came back:

```text
$ civitai app status --limit 5
BLOCK_ID    VERSION  STATUS    DEPLOY    SUBMITTED   URL
gen-matrix  0.6.0    approved  live      2026-06-22  https://gen-matrix.civit.ai/
…
```

It is a **display** limit, not a page size — unlike `civitai app list --limit`,
which is sent to the server. This route accepts no limit and no cursor (that is
what the cap note above is about), so the CLI fetches the same page either way
and simply prints fewer rows of it. Two consequences worth stating:

- `--limit` **cannot reach submissions the API did not return**, and it does not
  suppress the cap note — if the server capped the page, you are still told.
- It applies to `--json` too, so `--limit 5 --json` emits five records.

`--limit 0` (or negative) is refused as a usage mistake, and so is `--limit`
alongside a `blockId`/`--id` — a single submission cannot be limited, and
silently ignoring the flag would be worse than saying so.

`--json` emits the raw response for scripting. An empty list prints a friendly
"run `civitai app submit`" hint; with no token it points you at `civitai login`.
Notes like the cap caveat go to stderr, so `--json` stdout stays pure and the
exit code stays 0.

### Is your repo behind what you shipped?

A checkout can fall **behind its own live deployment** — you (or a teammate)
released 0.5.2, then came back to a working copy still on 0.4.0. Submitting from
there is accepted, and on approval it **replaces newer code with older code**
while the version number reads like an ordinary forward bump.

So when you ask about a single submission **from inside that app's directory**,
`civitai app status` compares the local `block.manifest.json` against your
**highest approved** version of the same app and says so on **stderr**:

```text
$ civitai app status custom-generators
Block ID:         custom-generators
Version:          0.7.0
...

⚠ local block.manifest.json is 0.4.0 — BEHIND the highest APPROVED version of custom-generators, which is 0.5.2.
  An approved version is what gets deployed, so submitting from this repo would replace newer code on approval.
  Sync the released code (civitai app pull . --app custom-generators) or raise the local version above 0.5.2 before civitai app submit.
```

The remedy is `civitai app pull . --app <slug>` — the `.` matters. Without a
`[dir]`, `app pull` clones into `./<slug>` (see [Pull your app's
repository](#pull-your-apps-repository-app-pull)), which from inside the checkout
would create a **second copy nested in your repo** and leave the checkout itself
just as far behind. The other way out is legitimate too: bump the local version
above the published one, which is what you want when the local work really is
newer.

What it does **not** do:

- **It never changes the exit code.** It is a warning, not a refusal — nothing
  that scripts `app status` starts failing because a repo is out of date.
- **It goes to stderr**, like the cap caveat above, so `--json` stdout stays a
  pure, parseable payload on both renderings.
- **It says nothing unless it is sure.** No local manifest, an unreadable one, a
  manifest for a *different* app, a version on either side this CLI cannot order,
  a listing that 403s/500s/429s, or nothing approved yet — every one of those is
  silence. A false "your repo is behind" would send you to re-pull released code
  for no reason, so the check only ever speaks when it has both numbers.
- **It only speaks when you are BEHIND.** Being *ahead* is the normal state of a
  repo about to release, and being *equal* is the healthy state right after one.
- **It is scoped to the detail view** (`app status <blockId>` or `--id`). The
  bare listing is many apps at once and has no single version line to attach to.

The reference is your **highest approved** version, which is deliberately not the
same thing as the newest row: the newest row can be a `pending` resubmission or a
`withdrawn` duplicate, and neither is code anyone is running. It is also the
version `civitai app submit` measures against — the two commands run the **same**
predicate over the same rows, so they cannot quote different numbers for the same
repo.

**Pre-release and build metadata are not ordered at all.** A version carrying a
`-rc1`, `-beta.2`, `-3-gabc123` or `+build.7` suffix is treated as *not
comparable* rather than reduced to its numeric triple, because real semver ranks
`0.5.0` **above** `0.5.0-beta.1` while a truncating compare calls them equal and
ranks `0.6.0-rc.1` above `0.5.2`. So:

- a **local** version with a suffix produces **no drift warning** — `app status`
  stays quiet rather than guessing;
- an **approved** version with a suffix is **skipped**, never quoted as "the
  highest APPROVED version". `app submit` names the ones it skipped and carries
  on; `app status` says nothing, which is its rule for every fact it could not
  establish.

### Deployed is not the same as listed in the store

`civitai app status` and `civitai app view` read **different resources**, and an
app can legitimately be in one and not the other:

- `civitai app status <slug>` reads your **submission pipeline**
  (`GET /api/v1/blocks/submissions`) — review status, deploy state, live URL.
- `civitai app view <slug>` reads the **public store catalog**
  (`GET /api/v1/apps/{slug}`) — the published store listing.

So `app status` can show `approved / live` with a working `<slug>.civit.ai` URL
while `app view <slug>` returns **not found** (exit 4). That 404 is truthful and
says nothing about your deploy: the store lists an app only once its **store
listing** is published (a listing needs an icon and a cover — see
`civitai app listing status`), and the catalog itself is still gated by a launch
flag while the store is pre-GA. When the 404 lands on a slug **you own**, the CLI
detects that and says so, naming both next commands, instead of leaving you with
a bare "App not found".

## Pull your app's repository (`app pull`)

`civitai app pull` is the **read side of git authoring**: it clones (or, if
`[dir]` is already a checkout, syncs) the canonical repository backing one of
**your** approved Apps, so you can edit locally and then `civitai app submit` or
push.

```bash
civitai app pull --app my-block                # clone into ./my-block
civitai app pull ./my-block --app my-block     # clone/sync into ./my-block
civitai app pull . --app my-block              # sync the current directory
```

`--app` is required (the slug or `appBlockId`; find it with `civitai app
status`). Authentication uses your stored credential (`civitai login` or a
personal API key) — the command calls an owner-only endpoint that **lazily
provisions a scoped, read-only Forgejo identity** for you and returns a clone URL
with a pull token embedded.

The repo only exists once your **first version has been submitted as a ZIP and
approved**; before then the command tells you so rather than failing obscurely —
`app <slug> has no approved version yet …`, naming the latest submission's state
and the next step for it (where it is in review, or — for a **rejected** or
**withdrawn** submission — that nothing is in review and a new `civitai app
submit` is what moves it).

That better message needs a submission the CLI can *see*, so it is not the
answer in every case. **`no such app for your account` is what you get whenever
the CLI cannot prove otherwise** — the slug matches none of your submissions,
**or** the submissions lookup itself failed, **or** a version *is* approved (so
the repo is missing for some other reason), **or** you passed `--app
<appBlockId>` instead of the slug (the check reads your submissions by slug; an
`appBlockId` only exists once a version is approved, so it is never the
not-approved case). Either way `civitai app status` is the command that settles
it.

`--app` is also where the app goes, and the positional is the **directory**:
`civitai app pull my-block` (slug typed positionally) is refused with a message
saying so, not a bare framework error — as is a bare `civitai app pull`.

> ⚠️ **SECURITY — TOKEN-IN-URL LEAKAGE.** The clone URL embeds your access token
> as HTTP-Basic credentials (`https://<user>:<token>@…`).
>
> - On a fresh **CLONE**, git writes the remote URL into `.git/config`, so **the
>   token lands on disk** in the checkout. Treat the directory as sensitive: do
>   **not** commit `.git/config` or share the directory. To drop the token, point
>   the remote at the credential-less HTTPS URL —
>   `git -C <dir> remote set-url origin <httpUrl>` — which is exactly the command
>   the CLI prints for you after a clone.
> - On a **SYNC** (pull into an existing checkout) the URL is passed explicitly
>   and is **not** persisted to `.git/config`. The token still appears
>   transiently in the git child process's arguments, so it is briefly visible to
>   other local processes via `ps` / `/proc/<pid>/cmdline`.
>
> The CLI prints the applicable half of this warning to stderr after every run.

A sync is `git fetch` + `git merge --ff-only`, so it never creates a merge commit
and refuses rather than clobbering diverged history or a conflicting dirty tree.

## Browse the App store

`civitai app list` and `civitai app view <slug>` read the **public App store
catalog** (`GET /api/v1/apps` and `GET /api/v1/apps/{slug}`).

> 🔴 **These are not anonymous reads.** Unlike the model/image/article commands
> in [Browse the public API](#browse-the-public-api), both **require a
> credential** and exit `3` with
> `no token configured — run 'civitai login' (or set CIVITAI_TOKEN) to browse the App store`
> without one. The store endpoint keys the **visible catalog off your identity**,
> so an anonymous call would see nothing; the CLI refuses up front rather than
> presenting an empty catalog as the whole store.

```bash
civitai app list
civitai app list --kind onsite --sort popular --limit 10
civitai app list --category generation --json
civitai app list --cursor '<next-cursor-from-a-previous-page>'
civitai app view my-cool-app
civitai app view my-cool-app --json
```

- **Filter-based discovery, not search.** There is no free-text query — the store
  service does not expose one, which is why there is no `civitai app search`.
  Filter with `--kind` (`all`, `onsite`, `offsite`), `--category` (`generation`,
  `games`, `utility`, `discovery`, `moderation`, `analytics`, `other`) and
  `--sort` (`top-rated`, `popular`, `newest`, `name`).
- **Keyset cursor pagination**, not page numbers: the next cursor is printed
  after the results, and you pass it back with `--cursor`. `--limit` is 1–50.
- **The store is gated by a launch flag.** Until it opens publicly you only see
  apps if your account is a moderator or an app-dev-tester, so a perfectly valid
  login may get an **empty list**. That is the pre-GA state, not a broken login.
- **Rate-limited per caller** — a tight scripted loop may see `429`s, which the
  CLI backs off and retries automatically.
- `app view` reads the **store catalog**, which is *not* your deploy: see
  [Deployed is not the same as listed in the store](#deployed-is-not-the-same-as-listed-in-the-store).

## App metrics

`civitai app metrics <slug>` shows the owner-only analytics for one of **your**
App Blocks. The slug is resolved to its `appBlockId` through your own
submissions, so analytics exist only once a version has been **approved** — an
app still in review reports that instead of an empty dashboard. Like `app pull`,
the message names the next step **for** the latest submission's own state: where
it is in review, or — for a **rejected** or **withdrawn** submission — that
nothing is in review and a new `civitai app submit` is what moves it. (Unlike
`app pull`, it does not print the `(latest submission: <version> <status>)` pair
— a **pending** app is told where to look, without its version being named.)

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
  `civitai login` token gets a 403. Both refusals — the 403 *and* the
  no-token-at-all case — name the route that actually works
  (`civitai login --token <key>`, created at `civitai.com/user/account`) rather
  than the generic "run `civitai login`", which here is the one route that
  cannot succeed.

Two data caveats.

**Engagement counts only authenticated, scope-gated API
calls.** An app that ships no scoped API surface shows real installs and revenue
with a flat engagement section — that is expected, not a bug. **Installs** is a
different case again: it shows **`n/a`** for an app that cannot be installed at
all (a page app has no install slot, so an install record cannot exist), which
is deliberately distinct from a real `0` on an installable app nobody has
installed yet. **App loads is the exception**: it is measured on every load, so it counts signed-out visitors and
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

**Installs carries `installs.notApplicable`** for the case above. It is NOT an
outage flag — it means the question does not apply to this app type, so a script
should render it as "not applicable" rather than retrying or warning about
infrastructure. `--json` passes it through and still exits `0`, so branch on it
rather than trusting the counts.

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

## Generate

`civitai generate "<prompt>"` runs a text-to-image generation on Civitai's
generator.

> 🔴 **This spends real Buzz and cannot be undone.** A submitted generation is
> charged the moment the orchestrator accepts it, and nothing local calls that
> back — not `--timeout`, not Ctrl-C, not `civitai workflows cancel`. Price it
> with `--dry-run` first — that calls the cost estimator and spends nothing.

> 🔴 **The CLI states no RULE about what becomes of a charge, in either
> direction — but it does report the transactions the server hands it.** Two
> different things, and the difference is the whole point:
>
> - **Your account Buzz ledger** is not readable from here. `civitai buzz`
>   reports a balance, not a history, so a balance alone cannot settle "did
>   *this* run come back" unless you noted it beforehand. Where no per-workflow
>   record is available the CLI says exactly that and points you at your
>   [transaction history](https://civitai.com/user/transactions).
> - **One workflow's own transactions** often *are* in the payload. When they
>   are, `civitai workflows get <id>` — and `civitai generate` on a failed run —
>   print them: each `debit` and `credit` the server recorded for that workflow,
>   and the net. See [Reading a workflow's Buzz
>   transactions](#reading-a-workflows-buzz-transactions).
>
> The CLI does **not** tell you the charge stands, and does **not** tell you it
> was refunded — earlier versions asserted the first, which the platform's own
> client contradicts for `failed`/`expired`/`canceled`. Reading civitai/cli#307
> established the server RE-PRICES a failed or cancelled run by the share of its
> outputs that never landed; the CLI still promises no *amount*, because the
> amount depends on how far the run got. Tracked at civitai/cli#307 and
> civitai/cli#346.

```bash
# Price it. Spends nothing.
civitai generate "a cat wearing sunglasses" --dry-run

# The same estimate as raw JSON, for scripts
civitai generate "a cat wearing sunglasses" --dry-run --json

# Generate, refusing if the estimate exceeds 50 Buzz
civitai generate "a cat wearing sunglasses" --quantity 4 --max-cost 50

# Your own checkpoint (a VERSION id) plus a LoRA — name the ecosystem that
# checkpoint belongs to; it does NOT bring one with it. See below.
civitai generate "a cat" --ecosystem <key> --checkpoint <version-id> \
  --lora <version-id>:0.8 --dry-run

# Wait for the result and write the images into ./out
civitai generate "a cat" --yes --out-dir ./out

# …naming the files yourself — {n} keeps a batch from colliding
civitai generate "a cat" --yes --quantity 4 --out-dir ./out --out-name 'cat-{n}{ext}'

# Fire and forget; collect the results later
civitai generate "a cat" --yes --no-wait
civitai workflows list
civitai workflows get <workflow-id>

# Non-interactive (CI) — --yes is required, or the run is refused
civitai generate "a cat" --yes --max-cost 20

# Image-to-image from a local file — --ecosystem is REQUIRED with --image
civitai generate "make it winter" --ecosystem Flux1Kontext --image ./cat.png --dry-run

# …or from a public https URL, with two reference images
civitai generate "combine these" --ecosystem Seedream \
  --image https://example.com/a.jpg --image ./b.png --yes

# Graduate from flags to a raw graph: print, edit, send back
civitai generate "a cat" --quantity 2 --print-input > graph.json
civitai generate --input graph.json --dry-run
```

**Credential.** Generation needs the AI Services scopes. Either a full-scope
**personal API key** ([create one](https://civitai.com/user/account), then
`civitai login --token <key>`), or a browser login that opted in:
`civitai login --scopes generate`. A **default** OAuth browser login
(`civitai login`, no `--scopes`) does **not** carry them and is refused.
`civitai whoami` shows the capability as **Spend Buzz (AI Services)**.

The one exception is `--print-input`: it assembles the graph and exits before the
estimator, the submit and the balance read, so it needs no credential — useful
for building a graph to edit before you have logged in. Two caveats, and they are
**not** the same caveat:

- `--print-input` **with `--image`** does need a credential, because it uploads
  each local file first and that upload is authenticated.
- `--print-input` **with `--checkpoint` or `--lora`** needs none — the
  model-version lookup is a public read — but it is **not offline**: that lookup
  is a real request, and with no network it fails (exit `5`) rather than printing
  a graph. Measured against a dead endpoint: bare `--print-input` exits `0`;
  `--print-input --checkpoint <id>` exits `5` after the read's retries.

So only a **bare** `--print-input` needs neither a credential nor a network.

### 🔴 `--max-cost` is an estimate check, not a spending cap

The cost this command shows is an **estimate, not a quote**: the server's
estimator returns no quote id, no signed price and no expiry, so there is
nothing to hand back at submit time — and **no server-side spending ceiling is
reachable from an API key at all**. The realized charge can exceed the estimate,
and `--max-cost` cannot claw the difference back — it never reaches the server.
What the ledger then does with that charge is not something this CLI reports —
see **What the LEDGER does with a charge** under [Generate](#generate).

`--max-cost` compares that estimate against your number and refuses **locally**
before submitting. It catches a `--quantity` typo. That is all it can do. Do not
run an unattended loop believing it caps spend. (The per-API-key `buzzLimit` on
your account does not bind this path either — the generator meters a separate
server-minted subject, not your key.)

### 🔴 A checkpoint does not carry its ecosystem

`--checkpoint` selects a model **version** and nothing else. The settings the
server generates with — engine, steps, cfg scale, sampler — follow the
**ecosystem** (`--ecosystem`, or the server's default when you pass none), *not*
the checkpoint you named. There is no `--steps` / `--cfg-scale` here to correct
them either, deliberately — see
[Raw graphs](#raw-graphs---print-input-and---input) for the route that does reach
them, and [the content flags](#the-content-flags-and-why-there-arent-twelve) for
why they are not flags.

Pairing a checkpoint with an ecosystem it does not belong to is refused by
**nothing** — not by this CLI, not by the estimator, not by the generator.
Measured once, on a live token (2026-08-10): `--checkpoint 128713` with no
`--ecosystem` submitted `diffuserModel: urn:air:sd1:checkpoint:civitai:4384@128713`
(SD 1.5) beside `ecosystem: zImage`, `engine: sdcpp`, `steps: 9`, `cfgScale: 1`.
It was charged 8 Buzz, queued 15 minutes, ran 4.5 minutes, and finished
`Status: failed` with **0 deliverable outputs**. `--dry-run` had reported
`Resources ready: true` beforehand. One paid observation, not independently
reproduced — reproducing it costs Buzz.

The two things that look like they should catch it do not:

- the model-version lookup proves the id **exists**, not that it **fits**;
- `Resources ready` reports resource availability, not coherence — it is
  [not a promise of output](#generation---json), and this is now a second
  measured case of `ready: true` preceding zero outputs.

**Which ecosystem a given checkpoint belongs to is server knowledge this CLI does
not hold and will not guess.** It is not vendored here for the reason
[`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) item 13 gives for the whole generation path: a local copy
of server state goes stale and starts refusing valid *new* inputs, which is worse
than the gap it closes. So if you name a checkpoint, name the `--ecosystem` it
belongs to as well. Whether `--checkpoint` *should* carry an ecosystem with it —
and whether anything pre-submit could see this — is an open design question,
tracked at civitai/cli#352.

### 🔴 Silent model substitution

If you pass a `--checkpoint` version id that is not valid for the model family
being generated, the server does **not** reject it. It substitutes that family's
default checkpoint, runs the job, and bills you for **what actually ran**. Until
recently the reply was indistinguishable from success — a nonexistent version id
came back `200 OK` at the default price.

The server now reports each swap, and `civitai generate` surfaces it. A version
id that is perfectly real but belongs to a *different* model family is the
easiest way to see it — here an SD 1.5 checkpoint sent to a Flux ecosystem:

```console
$ civitai generate "a cat" --ecosystem Flux1Kontext --checkpoint 128713 --dry-run
⚠ The server will NOT use the checkpoint you asked for. It has substituted a different model, and this estimate prices the SUBSTITUTE. Nothing has been submitted or charged yet.
    requested version 128713 -> will run version 1892509  (reason: unrecognized)
      the server does not offer that version in this model family at all — it may be a community checkpoint that was never offered for generation, or a version retired since this command was written. Check it with `civitai model-versions get <id>` and pin a version that is still offered
To refuse a run like this instead of being told about it, pass --fail-on-substitution.
```

The checkpoint line in the summary you approve is annotated too, so the model
that will *not* run is never the last one you read before saying yes:

```console
Checkpoint:       DreamShaper — 8 (Checkpoint, id 128713)  [SUPERSEDED — the server will run version 1892509 instead; see the warning above]
```

Your own id stays on the line: the CLI marks it, it never quietly substitutes
the applied one.

Two things about that transcript, so you can tell a *reproduction* from a
*mismatch*:

- **The applied id is the server's current answer, not a constant.** `1892509`
  is whatever that family's default was when this was run; yours will differ,
  and so will the price. What reproduces is the *shape* — a warning naming both
  ids and a `reason`, and a `[SUPERSEDED …]` note on the checkpoint line.
- **A version id that does not exist at all will NOT get you here.** `generate`
  resolves every `--checkpoint` against the public model-version API before it
  prices anything, so `--checkpoint 999999999` fails locally with
  `--checkpoint 999999999: not found (404): Model not found` and exit `4` — no
  estimate, no submit, nothing charged. That is the [deliberate live
  lookup](#-a-checkpoint-does-not-carry-its-ecosystem) doing its job: it is the
  *nonexistent* id it closes off, and it cannot tell you whether an id that does
  exist belongs to the family you are generating with.

It is reported **on the estimate** (`--dry-run`, and before the confirmation
prompt on a real run — while you can still back out), **again after the submit**
(where it is the receipt for what was billed), and **on a later read** with
`civitai workflows get <id>`, which is the only place a `--no-wait` run can still
discover it. The report always goes to **stderr**, so `--json` stdout stays
machine-clean — and `--json` carries the raw `modelSubstitutions` array itself.

There are three `reason` values, and they want different fixes:

| `reason` | What it means | What to do |
| --- | --- | --- |
| `wrong-workflow` | The version is real for this family but scoped to a **different** workflow (e.g. an edit-only version sent to text-to-image). | Pick a version offered for the workflow you are running, or change `--ecosystem` to match. |
| `unrecognized` | The version is in no list for this family — a community checkpoint, or one **retired** since your script was written. | Check it with `civitai model-versions get <id>` and pin one that is still offered. |
| `gated` | The version **is** offered here, but a gate rule hides it from your account. | An entitlement issue, not a command mistake: may need a membership, an early-access window, or an accepted licence. |

**By default this is a warning and the run continues** — the substitution is a
deliberate graceful degradation, so a script pinned to a version that was later
retired keeps working rather than breaking on a CLI upgrade. Pass
**`--fail-on-substitution`** to refuse instead; it is checked against the
**estimate**, so nothing is submitted and nothing is charged when it refuses.

🔴 **Silence is not an assurance.** The field is omitted when nothing was
substituted, so "no warning" means *either* "no substitution" *or* "a server
older than this feature" — the CLI cannot tell those apart and deliberately
never claims the negative.

🔴 **`--fail-on-substitution` is therefore NOT a spend guard.** It can only
refuse what the server *reports*. Against a deployment that does not report
substitutions the flag is **silently inert** — exit `0`, submitted, charged —
and there is no signal distinguishing that from "nothing was substituted". Three
further limits worth knowing before you build a pipeline on it:

- It is evaluated on the **estimate**. A substitution appearing only on the
  submit reply is **reported** (and is in `--json`) but exits `0`, because the
  money is already gone and failing there would strand a result you paid for and
  still need to collect.
- It refuses on the *first* reported substitution; the message names that one and
  counts the rest.
- **With `--input` it stays live, but its coverage is unknown to this CLI.** It
  still fires on the estimate's record, so it still refuses before any spend — a
  raw graph does not disarm it. What the CLI cannot do is relate that record to
  your file: the graph is not interpreted, so nothing local knows which model
  references it contains. Measured once: a checkpoint named under `resources` was
  charged 28 Buzz and ran a different model version with **no record at all**, so
  the flag did not fire. Read a silent run as *"nothing was reported"*, never as
  *"nothing was substituted"*.

### Confirmation

An interactive run prints the estimate, your balance and the resolved model
names, then asks. A **non-interactive shell (pipe/CI) without `--yes` is
refused** rather than charged silently. Everything the confirmation prints goes
to **stderr**, so `--json` keeps stdout machine-clean.

### Image-to-image: `--image` and `--ecosystem`

`--image <path-or-url>` (repeatable) attaches a reference image and turns the
job into an edit. A **local `.png`/`.jpg`** is uploaded to Civitai first and the
stored blob is referenced; an **`https` URL** is passed through as-is, but must
be publicly reachable — the generator downloads it server-side too, and an
unfetchable URL is a `400` after you have already been priced. Either way the
CLI reads the image's width and height from its **header only** (never decoding
the pixels) and sends them, because the server requires both and rejects an entry
without them. `http://`, `file://` and `data:` are refused, local files are
capped at **64 MiB** (checked by `stat`, before a byte is read), and only png and
jpeg are supported — webp would need a new third-party decoder dependency.

> 🔴 **`--image` requires `--ecosystem`, and the reason is money.** The server
> promotes a text-to-image job to image-to-image only when the request *names an
> ecosystem*. Without one it **ignores the images, generates from the prompt
> alone, and charges you the full amount** — HTTP 200, no error, no warning.
> Measured: the same graph with and without `images[]` priced byte-identically.

**The workflow on screen still says `txt2img`, and that is correct.** Image
editing is *requested* as `txt2img` plus your images — the server does the
promotion itself, from the request body. So the `--dry-run` quote and the spend
confirmation both name `txt2img` and, when `--image` is set, say why on the same
line. The CLI does not rename the field, because the value it sends really is
`txt2img`; and it does not claim the promotion *happened*, because it cannot see
that (the note below is the honest half of it).

Two more things the CLI genuinely cannot check for you, so it says them instead
of pretending:

- **Only some ecosystems accept reference images at all.** `Qwen`,
  `Flux1Kontext`, `NanoBanana`, `Seedream`, `OpenAI`, `Grok`, `Reve`, `MAI`,
  `Boogu` and a few more do; the Stable Diffusion family and the *default*
  ecosystem do **not** — and for those the images are dropped silently and
  billed. **The cost estimate cannot tell you which case you are in**: several
  edit-capable ecosystems price identically with and without images (measured on
  `Flux1Kontext`, `NanoBanana` and `Seedream`), so a price comparison is not a
  detector. Name an ecosystem you know supports editing.
- **Too many reference images are silently truncated.** Per-ecosystem limits run
  from 1 to 7, live only inside the server's per-engine graphs, and the extras
  are dropped *before* any limit check can fire — so the server never reports it
  and the truncated job is billed. Measured on `Qwen` (limit 3): 4, 5, 6 and 12
  images all priced identically to 3. The CLI refuses **more than 7** (no
  ecosystem accepts more, so that refusal can never block a valid request) and
  **warns** for anything above 1. It deliberately does not vendor the
  per-ecosystem table — see [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) items 13 and 19(c).

`--ecosystem` is sent to the server **verbatim and is not checked locally**;
an unknown value comes back as the server's own `unknown ecosystem` error.

`--dry-run` **does** upload local `--image` files, because an estimate built on a
graph with no `images[]` prices a plain text-to-image job. Uploading spends no
Buzz, and `--dry-run` still never submits.

### The content flags, and why there aren't twelve

`--negative-prompt`, `--quantity`, `--aspect-ratio`, `--checkpoint
<version-id>`, `--lora <version-id>[:strength]` (repeatable), plus `--image` /
`--ecosystem` above.

The generator is **permissive, not a validator** — it returns HTTP 200 for
things it silently changes:

- An out-of-range `--quantity` is **clamped** with no error (asking for 40
  charges you for the server's limit). The CLI warns when you cross it.
- `--steps 0` / `--cfg-scale 0` are *accepted* and price a degenerate, cheaper,
  wrong job — which is exactly why those flags are **not** exposed yet.
- A checkpoint id that does not exist is accepted, the ecosystem default is
  **silently substituted**, and you are billed for it.

So `--checkpoint` and every `--lora` is resolved against the public
model-version API **before** anything is submitted: a bad id becomes a hard
local *not found* (exit 4) instead of a wrong charge, and the confirmation
echoes the resolved **model name** so you approve a name rather than an integer.
`--model` is deliberately absent: `civitai download --model` takes a *model* id,
while this takes a *version* id.

**`seed`, `steps`, `cfgScale` and `sampler` have no flags at all**, and the
`steps 0` measurement above is why: a flag whose unset value could reach the
request would buy a broken run at a discount. They are still reachable — through
the graph, not a flag — and that is the only route to a **reproducible** run,
because the seed lives there. See
[Raw graphs](#raw-graphs---print-input-and---input) below.

### Raw graphs: `--print-input` and `--input`

The five flags cover the common job. Everything else the generator understands
lives in the **generation graph** — the JSON document the flags assemble. That
includes **`seed`, `steps`, `cfgScale` and `sampler`**, none of which has a flag:
a seed set in the graph is the only way to reproduce a run, and the graph is the
only way to raise cfg or move steps. You can write that document yourself:

```bash
# 1. Assemble it from flags, print it, and exit. No submit, no cost estimate,
#    no balance read — with no --checkpoint/--lora, no request at all.
civitai generate "a cat" --quantity 2 --aspect-ratio 1:1 --print-input > graph.json

# 2. Edit graph.json however you like.

# 3. Send it as-is. Price it first; --dry-run still spends nothing.
civitai generate --input graph.json --dry-run
civitai generate --input graph.json --yes

# …or pipe it, with `-`
jq '.prompt = "a dog"' graph.json | civitai generate --input - --dry-run

# The same route is how you set a seed — there is no --seed flag.
jq '.seed = 12345' graph.json | civitai generate --input - --yes
```

`--print-input` reaches **no money seam**: not the submit, not the cost
estimator, not the balance read. With `--checkpoint`/`--lora` it does still make
the public model-version *read* those flags always make — that lookup supplies
`model.type`, which graph `resources[]` require, so skipping it would print a
document `--input` could not submit.

`--print-input`'s output is a valid `--input` document by construction — that
round-trip is the point of the pair, and it is what replaces a `--set
some.path=value` expression language the CLI deliberately does not have (a wrong
type in such an expression is accepted by the server silently, and billed; an
edited file is inspectable before it is sent).

Five things to know, all of them consequences of it being a **passthrough**:

- **txt2img only.** A graph declaring any other workflow is refused. The
  server's content audit reads the top-level `prompt` node, and it rebuilds what
  it inspects from *declared* graph nodes — so a graph carrying its prompt
  somewhere else (a comfy node, a nested step input) is exactly the shape that
  could reach the generator unaudited. That question is open upstream, and this
  CLI will not be the path that answers it the wrong way.
- **Envelope keys are refused, not ignored.** `civitaiTip`, `creatorTip`,
  `buzzType`, `tags`, `externalId`, `sourceMetadata`, `sourceMetadataMap`,
  `remixOfId` and a top-level `input` belong to the request *envelope* around the
  graph, not to the graph. A file setting `civitaiTip` would charge a tip that
  **`--dry-run` structurally cannot show you** — the estimator prices a strictly
  smaller request and is never sent tips at all — so the file is rejected with an
  error rather than quietly cleaned up.
- **Keys the CLI does not model are passed through exactly as written, with a
  warning — and the warning claims nothing about the server.** It says the key is
  not modelled *here* and was not checked *here*; it is **not** a claim that the
  key is invalid, and **not** a claim that the server ignores it. The CLI does not
  carry a copy of the server's node registry, so it cannot tell those apart. What
  the key does — **including what it costs** — is the server's answer. `--dry-run`
  prices the graph with your key included, so the estimate is where a price effect
  would show; nothing local can predict one. Check the spelling.

  🔴 This warning used to assert the opposite, and it was **wrong**: it said an
  undeclared key "returns HTTP 200, prices the same, and simply has no effect".
  A graph carrying `"priority": "high"` drew that sentence and then priced at
  **28** with a `fixed → priority 20` component three lines below it, against
  **8** for `normal`. The key was honoured, it tripled the price, and the warning
  would have talked you out of the one change that clears a slow queue.
- **No model-id safety net.** `--checkpoint` / `--lora` are resolved against the
  public API before submitting; a raw graph is not interpreted, so a nonexistent
  id in it is accepted, the ecosystem default is substituted, and you are billed.
- **`--fail-on-substitution` works here, but its reach is not knowable from
  here.** Every *execution* flag still applies, this one included — it fires on
  the estimate's substitution record, so it refuses before any spend just as it
  does on the flag path. The limit is that a raw graph is not interpreted, so the
  CLI cannot tell you which model references your file contains or which of them
  a record would name. One measured case (a checkpoint under `resources`) was
  charged and ran a different version with **no record at all**. The command warns
  about exactly this whenever the two flags appear together — unconditionally,
  without reading your file, because deciding *which* graphs to warn about would
  mean vendoring which keys the server reports on. Read a silent run as *"nothing
  was reported"*, never as *"nothing was substituted"*.

`--input` cannot be combined with a prompt argument or with
`--negative-prompt` / `--quantity` / `--aspect-ratio` / `--checkpoint` /
`--lora` — there is no predictable answer to "does `--lora` append to or replace
the file's `resources`?", so the combination is a usage error. Every *execution*
flag (`--dry-run`, `--yes`, `--max-cost`, `--json`, `--no-wait`, `--timeout`,
`--out-dir`, `--out-name`, `--no-download`, `--force`, `--external-id`) still
applies, `--fail-on-substitution` included — see the coverage caveat above.

### Waiting, downloading, and re-attaching

By default `generate` **waits** for the job to finish and writes every
deliverable output into `--out-dir` (default `.`) as
`<workflow-id>-<n>.<ext>`. `--force` overwrites existing files; without it a
collision is refused *before any bytes move*.

- `--out-name <template>` names the files instead of the default scheme. Three
  placeholders expand and everything else is literal:

  | Placeholder  | Expands to                                              |
  |--------------|---------------------------------------------------------|
  | `{workflow}` | the workflow id                                          |
  | `{n}`        | the output number, **1-based**                           |
  | `{ext}`      | the file extension **including its leading dot** (`.jpeg`) |

  The default is `{workflow}-{n}{ext}`, so `--out-name 'cat-{n}{ext}'` writes
  `cat-1.jpeg`, `cat-2.jpeg`, … into `--out-dir`.

  > **The rendered name must be a plain file name inside `--out-dir`.** A path
  > separator, a leading `/`, or a `..` is **refused, not stripped** — silently
  > sanitising a traversal writes a file you did not ask for under a name you
  > did not choose. An unknown placeholder (`{workflowid}`) and a template that
  > renders to nothing are refused too. All of these are **usage errors
  > (exit 2) raised before the job is priced or submitted**, so a bad template
  > costs nothing.

  A template that would give **two outputs the same name** — one with no `{n}`
  on a multi-image run — is refused before any byte is downloaded, and `--force`
  does *not* override it: there is no earlier file to replace, only the run's own
  other output, and the presigned URLs spent getting it are not re-issued.
  Include `{n}` for a batch. A single-output run with a fixed name is fine.

- `--no-wait` submits, prints the workflow id and exits `0`.
- `--no-download` waits and prints the output URLs instead of writing files.
- `--timeout` bounds how long the CLI waits, and defaults to **30m**. That is
  deliberately generous: the wait has to outlast the **queue**, not just the
  execution. A healthy job has been measured sitting in `scheduled` for
  **11m41s** before execution even began, so a shorter default walks away from a
  run that has already been charged.
- `civitai workflows get <workflow-id>` shows a workflow at any time. It is the
  re-attach path for every case where the CLI stopped early, and it spends
  nothing.

> 🔴 **`--timeout` stops *waiting*. It does not stop the *job*.** When the
> deadline passes (or you press Ctrl-C) the generation keeps running
> server-side, and finishes and bills exactly as if you had stayed — neither
> case cancels anything. Both exit **non-zero**, print the workflow id, the
> idempotency key and the exact `civitai workflows get …` command, and never
> report success. `civitai workflows cancel` is the only thing that stops the
> remaining work; see [cancel](#listing-and-cancelling-workflows) for what that
> does to the charge.

**Output URLs are presigned and expire.** Download promptly; re-read the
workflow for fresh links. The blob fetch deliberately carries **no credential**
— the URL is already authorized, and attaching your full-scope API key to it
would hand 25 unrelated permissions to a request that needs none.

**A finished workflow can contain fewer usable results than you paid for.** An
output can be blocked by moderation, never land, or be one you hid on the
website. Those are filtered out of the download — and **reported, with the
reason**, plus an explicit note when the count differs from `--quantity`.
Silently writing three files for a four-image job is the failure this exists to
prevent. If *every* output is filtered out the command exits non-zero.

**Crash safety.** The orchestrator's idempotency key is written to
`~/.config/civitai/pending/<key>.json` **before** the request is sent, because
the money moves server-side even if the process dies mid-POST. If a submit's
reply never arrives, re-run with `--external-id <key>`: the orchestrator dedupes
on it and returns the **pre-existing** workflow instead of charging again (it
answers a duplicate with HTTP 200, not a 409, so re-attachment is inferred
locally).

**Polling cadence.** The status poll starts at 5s, backs off exponentially to a
cap, and backs off harder on a `429`. That floor is not tunable downward: the
workflow read proxies straight through to the orchestrator with no cache and no
server-side rate limit, so the CLI's own restraint is the only thing between it
and a 429 storm.

### Listing and cancelling workflows

```bash
civitai workflows list                      # newest first
civitai workflows list --limit 5
civitai workflows list --limit 50 --cursor <next-cursor>
civitai workflows list --json               # raw server payload, incl. nextCursor
civitai workflows cancel <workflow-id>      # asks for confirmation
civitai workflows cancel <workflow-id> -y   # skip the prompt (scripts/CI)
```

`list` is **cursor-paged**, not page-numbered: when more results exist it prints
`Next cursor: <c>` on **stdout**, which you pass back as `--cursor`. `--tag`
filters on orchestrator workflow tags (repeatable).

The `OUTPUTS` column reads `deliverable/total`. The two differ when an output was
blocked by moderation, never landed, or you hid it on the website — so
`0/4` means four images were produced and paid for and none of them are usable,
which is a very different fact from `0/0`. `civitai workflows get <id>` shows
the per-output reason.

**Where the server recorded an account of what happened to a workflow, the list
prints it under that workflow's row**, indented, in full and in the server's own
words:

```console
WORKFLOW ID                STATUS     CREATED                   COST  OUTPUTS
8753561-20260810224136984  succeeded  2026-08-10T22:41:36.984Z  104   1/1
8753561-20260810223715659  failed     2026-08-10T22:37:15.659Z  0     0/1
    Google Gemini: Could not generate images with the given prompts and images.
    Please try again with different inputs.
```

That gives up strictly-one-line-per-workflow, deliberately: `--json` is the
scripting contract and the table is not. Not every workflow has an account
recorded — nothing is printed for those, and the absence is the server's, not
the CLI's. See [What the server says went
wrong](#what-the-server-says-went-wrong).

<sub>Until civitai/cli#382 this was discarded at parse time. The reason arrives on the SAME response that renders `failed … 0/1`, at `steps[].errors` — a *different* path from the `steps[].output.errors` that civitai/cli#367 fixed on `workflows get`, which is why that change ruled this surface out of scope.</sub>

> 🔴 **`cancel` is not a clean refund — and it is not a total loss either.**
> Buzz is charged **up front**, when the orchestrator schedules the run.
> Cancelling stops the steps that have not finished; the orchestrator then
> **re-prices the workflow against the work it actually did** and settles the
> difference against what it took. What the run already delivered is billed: a
> step that had already finished keeps its full cost, and a job a worker has
> already started and cannot interrupt runs to completion and is billed for.
>
> So cancel a job because you no longer want its *output*. How much of the
> charge that leaves you paying depends on how far the run had got — the
> settlement is server-side and happens once the workflow reaches its final
> state, not when the command returns, and **this CLI never sees your account
> Buzz ledger**, so `cancel` itself reports no figure. Once the workflow reaches
> its final state, `civitai workflows get <id>` shows the transactions the
> server recorded for it — see [Reading a workflow's Buzz
> transactions](#reading-a-workflows-buzz-transactions). Otherwise settle it
> against your [transaction history](https://civitai.com/user/transactions).
>
> (This is also why `--timeout` and Ctrl-C deliberately do **not** cancel: they
> stop the wait, not the job.)
>
> <sub>This paragraph used to read *"`cancel` does not undo the charge … stopping it does not call that back"*. That was **wrong**, and wrong in the direction that costs you: it was written when the orchestrator service could not be read, and reading it (civitai/cli#307) showed a cancelled workflow is re-priced by the share of each job's outputs that never landed, with the difference refunded.</sub>

`cancel` **asks for confirmation**, matching `civitai generate` and
`civitai app submit`. It is the one irreversible action here — it throws away
whatever the run has not produced yet — so it is gated the same way every other
destructive path in this feature is:

- `--yes` / `-y` proceeds without prompting;
- an interactive terminal prints what is lost and prompts — the default is
  **no**, so a bare Enter aborts;
- a **non-interactive** shell without `--yes` **refuses** rather than cancelling
  silently. Scripts must pass `--yes` explicitly.

Nothing is cancelled when the confirmation is refused — the gate runs before the
request goes out.

**An id the server does not know is refused, not reported as cancelled.** The
cancel procedure answers the same empty success for a typo as for a real
workflow, so `cancel` reads the workflow back first: an unknown id exits **`4`**
— the same code `civitai workflows get` gives it — and **no cancel request is
sent at all**. If that read fails for any *other* reason (a timeout, a 5xx, a
rate limit, an auth failure) the cancel is still sent, because a flaky read must
never be what stops you halting a job that is spending your Buzz. Between the
read and the cancel a workflow can reach a final status on its own; that is
harmless, since cancelling a finished workflow is a server-side no-op.

<sub>Until civitai/cli#341, `civitai workflows cancel not-a-real-workflow-zzz --yes` printed *"Cancelled workflow not-a-real-workflow-zzz"* and exited `0` — while `civitai workflows get` on the same id correctly reported a 404.</sub>

### What the server says went wrong

When a generation ends badly the orchestrator often records an account of what
happened on the step. `civitai generate` puts it at the end of the error,
`civitai workflows get <id>` prints it under **The server reported:**, and
`civitai workflows list` prints it on indented lines under the workflow's row —

```console
Workflow ID:  wf_abc123
Status:       failed

The server reported:
  - Could not generate images with the given prompts and images. Please try again with different inputs.
```

Five things it is not:

- **It is not this CLI's opinion.** The text is the server's. Nothing here
  classifies it, matches on its wording, or maps it to a list of known messages
  — so a message the platform adds tomorrow arrives intact rather than being
  swallowed by a stale table.
- **It is not quite byte-for-byte, on the terminal.** Before printing any
  server-supplied text, the CLI removes escape and control bytes plus every
  character Unicode marks **default-ignorable** — the zero-width spaces and
  joiners, the bidi overrides that reverse the order a line is *displayed* in,
  the variation selectors (except the one that keeps an emoji looking like an
  emoji) — plus two runes that paint nothing while Unicode files them as a
  symbol and a mark, and four Hangul *fillers* that paint nothing while Unicode
  files them as letters. Punctuation, accents, CJK, emoji, ordinary letters and
  right-to-left *script* survive, as do the format characters Unicode says must
  be drawn (Arabic end-of-ayah, the Syriac abbreviation mark, ruby annotation
  marks).
  🔴 **Every removed character is invisible on its own, but some of them change
  how their NEIGHBOURS are drawn**, so this is not free: an emoji built from a
  zero-width joiner renders as its components, a subdivision flag falls back to
  🏴, and text that uses the join controls to make a distinction loses it —
  Persian/Arabic (`می‌روم` renders joined), Malayalam (the chillu `ണ്‍` becomes
  `ണ്`, a different letter), Devanagari, Bengali, Tamil, Kannada, Sinhala and
  Mongolian. **`--json` is not filtered at all** — it is a raw passthrough, so a
  script sees exactly what the server sent.
- **It is not applied to the prompt you typed.** The filter is for text the
  server sent. Your prompt, your negative prompt, `--aspect-ratio`,
  `--ecosystem`, the paths you pass to `--image` / `--input`, and the ids you
  give `workflows cancel` / `app withdraw` are all echoed back exactly as typed
  — most importantly on the confirmation screen before a spend, which has to
  show what will really be sent. **Two deliberate exceptions**, both outside
  that screen: a value read out of an `--input` *file* is filtered like server
  text (a graph file can come from anywhere), and `civitai download` filters the
  path it reports even when you set it with `--out`, because the same variable
  usually holds a filename the **server** chose — so an `--out` path containing
  an invisible character is reported without it while the file is written to the
  path you gave.
- **It is not always there.** Some failures record nothing. In that case the
  error says so instead: *"the orchestrator often supplies no failure reason, so
  it may not say why"*. That is a measured case, not a gap in the CLI, and there
  is no further detail to go and look for.
- **It is not a statement about your Buzz.** Whether a charge stands, is
  re-priced or comes back is decided server-side and reported separately — see
  [Reading a workflow's Buzz
  transactions](#reading-a-workflows-buzz-transactions).

The heading is *"The server reported"* rather than *"Why it failed"* on purpose:
the CLI prints the record whatever the workflow's status, and it has not
established that the orchestrator populates it only on failure. `workflows list`
prints its lines on the same footing, for the same reason.

**The two commands can word the same failure differently, and neither is the
CLI's doing.** `workflows list` reads the platform's *normalized* feed, which
runs each message through a server-side sanitiser that names the provider —
*"Google Gemini: Could not generate images…"*. `workflows get` reads the
orchestrator's raw workflow, where the same message has no such prefix. So the
list can be the more specific of the two.

**It can also be the less specific one, and that is the half worth knowing when
you are debugging.** The same server-side sanitiser *replaces* any message it
cannot vouch for — one carrying a URL, a path, a stack frame, an infra name, or
running past 300 characters or one line — with a generic *"… reported a system
error"*. `workflows get` applies none of that, so where the two disagree the raw
one can be the more informative. **When a failure is worth chasing, read both.**
The CLI reproduces neither transform; it prints what each endpoint sent, less the
invisible characters described above.

<sub>Until civitai/cli#367 this was discarded at parse time — the field existed on the wire and the CLI had no place to put it — so every failure printed the same generic sentence no matter what caused it, and `civitai workflows get` answered a failed run with a status and nothing else. `civitai workflows list` kept discarding it until civitai/cli#382, at a different wire path.</sub>

### Reading a workflow's Buzz transactions

`orchestrator.getWorkflow` often returns the orchestrator's own money record for
that one workflow. When it does, `civitai workflows get <id>` prints it — and so
does `civitai generate` when a run it waited on ends `failed`, `expired` or
`canceled`:

```
Buzz transactions for this workflow (2 recorded)
  debit   8
  credit  8
  net     0
```

**These are entries the server returned, not a rule the CLI is asserting.** The
block reports what is recorded against *this* workflow id and nothing more —
`net` is simply `debit` minus `credit` over the rows above it. It is **not**
your account balance (`civitai buzz` reports that), it is **not** your account
transaction history, and it does not tell you what a failure or a cancel does in
general. Where the payload carries no record, the CLI says so and points you at
your [transaction history](https://civitai.com/user/transactions) instead — an
absent record is *"nothing to report"*, never *"no money moved"*.

A transaction `type` this build does not recognise makes the net **unreportable**
rather than dropping the entry out of the arithmetic:

```
Buzz transactions for this workflow (3 recorded)
  debit  12  (2 entries)
  hold   3
  net    (not computed)
```

`--json` is unaffected: it has always passed the raw payload through, including
the `transactions` object with every field this CLI does not model.

<sub>Until civitai/cli#346, `civitai workflows get` on a failed run printed *"this CLI cannot see your Buzz ledger … settle it against your Buzz transaction history"* while holding `{"type":"debit","amount":8}` and `{"type":"credit","amount":8}` from that very run — and `civitai workflows list` was already rendering the same settlement as `COST 0`.</sub>

### Exit codes specific to `generate`

`generate` follows the [global exit-code table](#exit-codes), with one
deliberate refinement. The API answers several very different failures with the
same HTTP status, and the generic mapping would send a script down the wrong
path — in particular a caller who is out of Buzz, muted, or hitting a
server-side outage must **never** be told to re-run `civitai login`. Those cases
therefore exit `1` (generic), not `3` (auth) or `2` (usage):

🔴 **An exit code does not tell you whether you were charged.** Every failure
above the divider happens *before* anything is submitted, so nothing was spent.
Every failure below it happens *after* the submit, and **the Buzz is gone** —
including a `--timeout`, a Ctrl-C, and a workflow that ends `failed`. Do not
write a retry loop that branches on the exit code alone; re-attach with
`civitai workflows get <workflow-id>` instead of re-submitting.

| Failure | Exit |
| --- | --- |
| *— nothing submitted, nothing spent —* | |
| Missing AI Services scope / no token / not authenticated | `3` |
| Not enough Buzz (caught locally against your balance, or reported by the server) | `1` |
| Account muted, or onboarding incomplete | `1` |
| Generation disabled server-side | `1` |
| Prompt refused by content moderation — 🔴 **never retry**, repeated blocked prompts get the account muted | `1` |
| The server priced the job but reports `ready: false` (a selected resource is not currently available) | `2` |
| Estimate above `--max-cost`, an unknown ecosystem, or a resource that resolved fine but is "not enabled for generation" (the ids exist; the *combination* is not runnable — distinct from exit `4`, which means "no such id") | `2` |
| `--fail-on-substitution` and the **estimate** reported a substituted checkpoint — nothing submitted (see [Silent model substitution](#-silent-model-substitution)). 🔴 A substitution that appears **only on the submit reply** is reported but exits **`0`**: by then the charge has happened, and failing would strand a result you paid for. And against a server that does not report substitutions at all the flag is **inert** — exit `0`, submitted, charged | `1` |
| `--input` that is malformed, declares a non-txt2img workflow, carries an envelope key (`civitaiTip`, …), or is combined with a content flag | `2` |
| No such `--checkpoint` / `--lora` version id | `4` |
| `civitai workflows get` / `workflows cancel` on an unknown workflow id (a read; spends nothing) | `4` |
| *— 🔴 submitted: the Buzz is already spent —* | |
| `--timeout` expired, or Ctrl-C while waiting — the job **keeps running** server-side and was **not** cancelled | `1` |
| The workflow finished `failed` / `expired` / `canceled` | `1` |
| The workflow succeeded but every output was filtered out (blocked / unavailable / hidden) | `1` |

## Upgrading

`civitai upgrade` replaces the running binary with the latest GitHub release:

```bash
civitai upgrade           # no-op (and says so) when already current
civitai upgrade --force   # reinstall anyway
```

The release is resolved from the **public** GitHub releases API — no token is
ever sent — and the downloaded archive is verified against its SHA-256 entry in
the release's `checksums.txt` before anything is replaced. A mismatch, or a
release carrying no `checksums.txt` at all, **aborts and leaves the current
binary untouched**: it will not upgrade without integrity verification.

If this binary came from Homebrew, `upgrade` does not self-replace it — it tells
you to run `brew upgrade civitai/tap/civitai`, so the package manager keeps
owning the file. `--force` overrides that and self-replaces anyway.

The other install paths update the way they normally do — `npm install -g
@civitai/cli@latest`, `nix profile upgrade`, or re-running `go install …@latest`.

Separately from this command, the CLI runs a **background check** for a newer
release and prints a one-line notice; `--no-update-check` (or
`CIVITAI_NO_UPDATE_CHECK=1`) turns that off, which is what you want in CI.

## Global flags

Four of these are accepted by **every** command. `-v` / `--version` is the
exception — it is **root-only**:

| Flag | What it does |
| --- | --- |
| `-v`, `--version` | **On the root command only.** `civitai --version` prints the version and exits; on a subcommand it is not a flag at all — `civitai app validate --version` fails with `unknown flag: --version` and exits `2`. From a script, use `civitai version` (version + commit + build date), which works from anywhere. |
| `-h`, `--help` | Help for any command. `civitai --help` also prints the [exit-code](#exit-codes) contract. |
| `--no-color` | Disable all colour and styling. Also via `NO_COLOR` or `CIVITAI_NO_COLOR`. |
| `--color` | Force colour **even when stdout is not a TTY**. Also via `CLICOLOR_FORCE` or `CIVITAI_COLOR`. |
| `--no-update-check` | Skip the background check for a newer release. Also via `CIVITAI_NO_UPDATE_CHECK`. |

**The colour contract, for pipelines.** Colour is **off by default whenever
stdout is not a TTY**, so a redirected or piped run already emits plain text
with no escape sequences — you do not have to ask for anything. When you do want
to override that, the precedence is fixed, highest first:

1. `--no-color` / `NO_COLOR` / `CIVITAI_NO_COLOR` → **off**
2. `--color` / `CLICOLOR_FORCE` / `CIVITAI_COLOR` → **on**
3. otherwise: on if stdout is a TTY, off if it is not

Off always beats on, so a `NO_COLOR` in the environment cannot be re-enabled by
a `--color` further down a pipeline. `NO_COLOR` follows the
[no-color.org](https://no-color.org) convention — *present and non-empty* is
what counts, not the value.

🔴 **`--json` output is never styled**, at any of those settings. It is written
without passing through the presentation layer at all, so `--json` is always
safe to pipe into `jq` regardless of how colour is configured or whether a TTY
is attached.

## Configuration

| Setting | Config key | Env var | Default |
| --- | --- | --- | --- |
| Personal API key | `token` | `CIVITAI_TOKEN` | — |
| OAuth tokens (device login) | `auth_kind`, `access_token`, `refresh_token`, `token_expiry`, `scope` | — | — |
| API base URL | `base_url` | `CIVITAI_BASE_URL` | `https://civitai.com` |
| Submit endpoint | — | `CIVITAI_SUBMIT_PATH` | `/api/v1/blocks/submit-version` |
| Skip the update check | — | `CIVITAI_NO_UPDATE_CHECK` | unset (the check runs) |
| dev-tunnel SSH endpoint | — | `CIVITAI_DEV_TUNNEL_ENDPOINT` | `sish.civitai.com:2224` |
| Disable colour | — | `NO_COLOR`, `CIVITAI_NO_COLOR` | unset |
| Force colour | — | `CLICOLOR_FORCE`, `CIVITAI_COLOR` | unset |
| ⚠️ dev-tunnel channel debug log — **debug only, not supported surface** | — | `CIVITAI_DEVTUNNEL_DEBUG` | unset (no debug output) |

Where a setting also has a flag — `--token`, `--tunnel-endpoint`, and the colour
and update-check flags — the flag wins over the environment. See
[Global flags](#global-flags) for the full colour precedence.

Everything in this table except the last row is supported surface.
`CIVITAI_DEVTUNNEL_DEBUG` is listed only so it is findable: it is a diagnostic
for the `app dev-tunnel` plumbing, any non-empty value turns it on, and its
output and its existence may change or be removed in any release. See
[Preview in the real host](#preview-in-the-real-host-app-dev-tunnel) for what it
actually prints.

Config lives at `~/.config/civitai/config.yaml` (honours `XDG_CONFIG_HOME`),
written owner-readable only.

## Exit codes

`civitai` returns a differentiated exit code so scripts can branch on the *kind*
of failure without parsing stderr. The human-readable error message is unchanged
by this — only `echo $?` differs.

**The table is the index.** Each code's full ledger — the paths it covers, the
residuals it deliberately does not, and the rules a script has to branch on — is
in that code's `### Exit code N` subsection below. `civitai --help` prints the
table's summaries and points here for the rest.

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Generic / unclassified error. A **filesystem failure** lands here, and so does a **validation verdict** — an invalid manifest, or a real directory holding no manifest. [Detail](#exit-code-1) |
| `2` | Usage error — a bad flag, a **missing required flag or argument**, a bad flag **value**, or a path that does not exist / is not a directory. [Detail](#exit-code-2) |
| `3` | Not authorized — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403). [Detail](#exit-code-3) |
| `4` | Not found — the requested resource does not exist. [Detail](#exit-code-4) |
| `5` | Network/transport failure or service unavailable — the code to **retry** on. [Detail](#exit-code-5) |
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

### Exit code 1

- A **filesystem failure** lands here — a file that exists but cannot be read, an unwritable config directory, an I/O error. It is neither a mistake about the invocation (`2`) nor a transport failure (`5`), and there is no filesystem-specific code.
- A **validation verdict** lands here, and deliberately not on `2`: `civitai app validate` exits `1` when the manifest is invalid, and likewise when the directory you named is a real directory with no `block.manifest.json` at its root — you pointed at a real place, so the invocation was right and the project is wrong. (A path that does **not exist**, or that is not a directory, is the invocation being wrong, and exits `2`.)
- **When validation produces a result**, `civitai app validate --json` prints it in full and its `ok` field is the structured form of the same answer; a failure that produces no result at all — a project directory the CLI cannot **stat**, say, because it is unreadable or because a path component below it is not a directory — still exits `1` with **nothing on stdout**, so branch on the exit code before parsing. The full exit→stdout table is in [The `--json` result shape](#the---json-result-shape).
- A resource that **exists but is not ready** lands here too, and deliberately not on `4`: `civitai app metrics <slug>` for an app whose submitted version is still in review exits `1`, because the slug is right and the app does exist — only its analytics do not exist yet, and the error names `civitai app status <slug>` as the next command. `4` stays reserved for a slug with no submissions at all, so the two remain separately actionable: fix the slug, versus wait for approval.
- A **version regression** lands here for the same reason: `civitai app submit` refuses when the manifest version is not strictly **above the highest approved version** of that app, because approving an older (or identical) version replaces the newer live deployment. Nothing about the invocation is wrong, so it is a verdict about the project, not a `2`. `--allow-downgrade` is the deliberate-rollback escape hatch, and the guard is skipped entirely by `--package-only` or a run with no token — neither reaches the server.
- A **dirty git work tree** lands here too: `civitai app submit` refuses while files that go into the bundle are uncommitted, because the bundle is packaged from what is on disk and approving one deploys code that exists in no commit. `--allow-dirty` submits the tree as it is. It **degrades rather than enforcing** — a directory that is not in a git repo, or a machine with no `git` on `PATH`, submits exactly as before (scaffolded apps have no repo, and that path must keep working), and a clean tree whose `HEAD` is on no remote **warns** instead of refusing. Like the version guard it is skipped by `--package-only` and by a run with no token.
- **"Wait for approval" is the *pending* case only.** The same `1` covers an app whose latest submission was **rejected** or **withdrawn** — nothing is in review there, so `civitai app metrics <slug>` says so and names a new `civitai app submit` as the next step instead of a review to wait for. What separates `1` from `4` is unchanged: the slug is right and the app exists.

### Exit code 2

- Usage error — a bad flag, a **missing required flag or argument** (e.g. `civitai app withdraw` with no publish-request id), a bad flag **value** (`--limit` out of range, a non-integer id, `--template nope`), or a request the API rejected as malformed (HTTP 400, e.g. a bad `--period`/`--sort` enum).
- This does not depend on where the refusal happens: a mistake the CLI catches locally and one the server rejects both exit `2`.
- A local image the CLI refuses before uploading anything (`civitai app listing set-icon <file>`, `civitai generate --image`) exits `2` when the file is missing, empty, a directory, over the size cap, or not a PNG/JPEG/WebP — but a file that exists and cannot be **read** (permissions, an I/O error) is a filesystem failure rather than a mistake about the invocation, and exits `1`, not `2`.
- That split is not images-only and it is not flags-only — it holds for **a flag's value and a positional argument alike**, over the paths listed here: `civitai generate --input <file>` likewise exits `2` for a path that is not there or is a directory, and `1` when the file is there and the read fails.
- The project commands take a positional path and refuse it the same way: `civitai app validate <dir>` and `civitai app submit <dir>` exit `2` when the path does not exist **or is not a directory**, because both are mistakes about the invocation. A directory that **does** exist but holds no `block.manifest.json` is a validation verdict instead, and exits `1`.
- `app listing set-cover` and `app listing add-screenshot` take the same positional `<file>` and refuse it the same way. (The CLI has no `--file` image flag at all: the only `--file` is `civitai download --file`, which picks a file *inside* a model version.)
- **Paths outside that list are not covered, and mostly exit `1`.** `civitai app listing … --dir <missing>` exits `1` (it reports "no `block.manifest.json` found in …", the same way it does for a directory that is really there but holds no manifest), and so does `civitai app submit … --out <path under a directory that does not exist>`. Both are stated rather than promised: this is a ledger of the paths the split is published for, not a claim about every path in the CLI.
- A usage error emits **no JSON object**, in every mode. `civitai app validate /nope --json` therefore writes nothing to stdout and exits `2`; it used to print `{"ok": false, …}` and exit `1`, which reported a nonexistent path as a validation result. Scripts that parsed that object must branch on the exit code first.

### Exit code 3

- Authentication/authorization — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403, or no token configured).
- **`civitai generate` refines this**: several of its failures are *not* credential problems but would otherwise land here or on `2`, so they exit `1` instead and a script never loops on `civitai login`. A **muted account or incomplete onboarding** arrives as a bare `403` that is byte-identical to a missing scope; **out of Buzz** and **generation disabled** arrive as `400` (the upstream 403 is re-thrown server-side as a tRPC `BAD_REQUEST`), which would otherwise read as "bad flags". See [Generate](#exit-codes-specific-to-generate).

### Exit code 4

- Usually an HTTP 404, but not always: some lookups answer `200` with an empty result set instead (`civitai app status <slug>` for an unregistered slug, `civitai users get` for an unknown username), and those exit `4` too.
- The same question therefore exits the same way however the API happens to phrase the miss.
- **An app that EXISTS but is `offsite` exits `4` from `civitai app status <slug>`, and that is deliberate.** That command resolves through the app's block submission, which an offsite app never has, so the *resource it looks up* is genuinely absent even though the app is not. Only the message changes — where the CLI can tell, it says the app is offsite and names a next step that can work, instead of `civitai app submit`, which cannot. This is **not** the `has no approved App Block yet` case on `1` above, which exits `1` because the thing looked up (analytics) is expected to appear later; an offsite app's block submission never will.
- **`civitai app listing …` no longer exits `4` for an offsite app in the normal case** — it falls back to selecting the listing by slug and succeeds ([#422](https://github.com/civitai/cli/issues/422), needing `civitai/civitai#3989` server-side). It still exits `4` when that fallback *also* answers **not-found**: a Civitai without `#3989` (older or self-hosted), or an app with no listing row. A listing your account does not own is **not** in that set on a current server — the by-slug lookup resolves it and then refuses it `403`, so it exits `3`.
- **A lookup failure that is not a 404 keeps its own code, and that is not always `3` or `5`.** Neither of the two lookups is retried past a non-404 — a `403`, a `5xx` or a transport failure says nothing about whether the resource exists — so you get the failing lookup's own error and its own code. **Measured** end-to-end on both lookups (`cmd/civitai/app_listing_lookup_exitcode_test.go`): `401`/`403` → `3`, `429` → `6`, `502`/`503`/`504` and a transport failure → `5`, and a plain **`500` → `1`**, because that status carries no classification sentinel at all. Do not read "the API failed" as "exit `5`": the code to retry on is `5`, and a `500` is not it.
- **The offsite wording is best-effort, and the exit code is not.** Naming an app as offsite costs one extra lookup against the public store catalog (`GET /api/v1/apps/{slug}`), and that route answers only for a **published** store listing and is itself still behind a launch flag — until the catalog opens publicly you see an app there only as a moderator or app-dev-tester. So an offsite app whose listing is not published, a caller without that access, or any network/5xx failure of the lookup all keep the generic `no such submission … run civitai app submit first` message. **Exit `4` either way**, so a script branching on the code is unaffected; only the human-readable half degrades, and always in that direction.

### Exit code 5

- Network/transport failure or service unavailable — dial/timeout, or HTTP 502/503/504 after retries.
- This is the code to **retry** on, so a **filesystem** failure never lands here however retryable its errno looks: a permissions or I/O problem does not fix itself, and a loop that sleeps and re-runs would never terminate. Those exit `1`.

## Troubleshooting

**Look up the message you got.** Every row's left column is a fragment of a
string this CLI really prints, so searching this page for a few words of your
error should land you on the right row. (A test asserts that each of these
strings still exists in the source, so the index cannot quietly go stale the way
a hand-written list does — and for the rows most easily confused with one
another, a second test runs the command the row names and checks it really
prints that string, because "the string exists somewhere" was green while a row
credited it to the wrong command.)

### Credentials and access

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `no token configured` | Nothing is logged in. Run `civitai login`, or set `CIVITAI_TOKEN`. This includes `civitai app list` / `app view`: the App **store** is not an anonymous read. | [Submit & auth](#submit--auth), [Browse the App store](#browse-the-app-store) |
| `not logged in (401)` | The credential is present but invalid or expired. OAuth tokens refresh themselves; when the *refresh* token has also expired, log in again. For a personal key, mint a new one at `civitai.com/user/account`. | [Submit & auth](#submit--auth) |
| `forbidden (403)` | Usually the invite-only Apps beta rather than a broken token — the same account reads the public API fine. | [Submit & auth](#submit--auth) |
| `not permitted for your account (403)` | Managing a **store listing** needs Apps-author access, which is a narrower grant than being able to submit. | [Listing media requirements](#listing-media-requirements) |
| `not permitted to read this app's analytics (403)` | `app metrics` needs a **full-scope personal API key**. An OAuth login is refused here even when it can submit. | [App metrics](#app-metrics) |
| `block lacks ai:write:budgeted scope` | Printed by your app at runtime under `dev:live`. The dev token was minted **without** `--spend`, so the CLI filtered the budgeted-spend scope out — it never requests that scope implicitly, even when your manifest declares it. | [Local dev loop](#local-dev-loop-harness-mock-vs-live) |
| `insufficient Buzz` / `generation disabled` | Not credential problems, which is why they exit `1` rather than `3` — a script must not loop on `civitai login` for either. | [Exit codes specific to `generate`](#exit-codes-specific-to-generate) |
| `rate limited (429)` | Throttled; exit `6`. For deep paging use `--cursor` rather than `--page`. | [Exit codes](#exit-codes) |

### Scaffolding a project

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `cannot derive a slug from` / `cannot appear in a blockId` | The name holds characters the blockId alphabet cannot carry, and dropping them would mint a **different permanent public id** than you typed. Choose one yourself with `--slug <slug>`. | [The blockId](#the-blockid) |
| `is not valid UTF-8` | The same refusal one step earlier: the name's bytes cannot be read at all. Pass `--slug`, and a `--name` that is valid UTF-8. | [The blockId](#the-blockid) |
| `… and the limit is …` | The name derives a blockId longer than the 40-character cap. It is **not** truncated — two names sharing the first 40 characters would be given the same un-renameable id. Pick a shorter one with `--slug <slug>`. | [The blockId](#the-blockid) |
| `refusing to overwrite. Scaffold somewhere else` | `app create` / `app init` will not clobber a non-empty directory, and there is deliberately **no `--force`** — overwriting a directory you already have is not recoverable. Use `--dir <new path>`, or remove the directory first. | [Templates](#templates) |

### Validating and submitting

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `… not found at project root …` | **`civitai app validate`** found no `block.manifest.json` in the directory you named — the finding reads `block.manifest.json not found at project root <dir>`, which the terminal wraps onto a second line for a long path (`--json` carries it as one `message` string). `app submit` prints it too, because it validates first. `app submit --skip-validate` never prints it, because it waives the validation that produces it — that run fails on the row below instead. The path itself was fine, which is why this exits `1` and not `2`. | [Exit code 1](#exit-code-1) |
| `is this an App project?` | The same cause, reported by a command that did not validate first: `civitai app listing …`, which has to work out *which* app you mean from the working directory, and `app submit --skip-validate`, which waived the check that produces the row above. `app validate` and a plain `app submit` never print it, because validation reports the row above first. Run `app listing` from the app directory, or name the app with `--slug` / `--dir`. | [After you submit](#after-you-submit-review--approve--deploy) |
| `the server rejected this store-listing lookup (400)` | A **read** was refused — resolving your app's listing, reading it for edit, or polling an asset's scan state. Nothing was changed. The value at fault is one the CLI derived (an app slug, a listing id, an image id), so start from `civitai app status` and check the app you named. Exit `2`. | [Listing media requirements](#listing-media-requirements) |
| `the server rejected the image-upload request (400)` | The **image** was refused while being ingested — the presigned upload mint, the inline icon upload, or the row that records the uploaded file. **No listing was changed** by any of those three: an image is attached to nothing until `set-icon` / `set-cover` / `add-screenshot` runs. The server's own reason follows the code and names the bound it applied — read it rather than guessing which limit you crossed. Exit `2`. **The raw upload of the bytes is a fourth step and reports differently** — see the row below. | [Listing media requirements](#listing-media-requirements) |
| `image upload PUT failed` | The full-resolution upload of the **bytes themselves**, to the presigned URL, was refused by storage (e.g. `EntityTooLarge`) — the step between minting the URL and recording the row. No listing was changed. ⚠️ Unlike the three ingest steps above it is **not** classified as a bad request, so it exits **`1`, not `2`**, and the message leads with the HTTP mechanism rather than with what you were doing. That inconsistency is known and tracked ([#388](https://github.com/civitai/cli/issues/388)), not intended. | [Listing media requirements](#listing-media-requirements) |
| `the server rejected this store-listing change (400)` | The **listing** was refused — an attach, a removal, a reorder, opening a revision, or submitting one. Unlike the rows above, this one may have **partially applied**, which is why it points you at `civitai app listing status` rather than claiming nothing happened. It deliberately does **not** tell you to go fix a value, because the seven routes it covers do not all carry one. Where the CLI can name something concrete it does so on the lines that follow, and only there: `set-icon`, `set-cover` and `add-screenshot` print the bytes, pixel dimensions and MIME type of the **file** you sent — never the `--caption`. `rm-screenshot`, `reorder` and the two revision steps print this line **alone**, so for those the server's own reason is the whole story; *opening* a revision in particular sends nothing but a listing id the CLI derived from a lookup that had already succeeded. Exit `2`. **One refusal is deliberately not reported here at all**: a revision *submit* refused because the listing is still below the publish floor is not a failure of the command that ran — the change was written to the revision, and the floor takes two commands to clear — so that case prints `staged on an open revision` and exits `0`. The CLI decides that from the listing's own state, never from the server's wording. It reports progress only when all three hold: the refusal was a **400** (a `500`/`503` is an outage, not the floor, and still fails with its usual exit code), what it just wrote is really there — matched by **image id** for an icon or cover, by the `alsc_…` row id for a screenshot (a listing being re-branded already has an *old* icon, which proves nothing), and for `reorder` by the revision's gallery holding **exactly the ids you passed, in the order you passed them** (a revision that exists but was never reordered proves nothing either) — and the floor is really unmet. If it cannot read the listing back it claims nothing — you get this row and exit `2`. **This applies to `reorder` since [#430](https://github.com/civitai/cli/issues/430)**, which is also when a live `reorder` started reaching the revision routes at all: before it, a reorder of a live listing could only ever produce this row, because it was addressed to the parent listing while the ids it carried belonged to the open revision. 🔴 **That exception is scoped to the commands whose job was the STAGED CHANGE — the three attaches and `reorder` — and `civitai app listing submit-revision` is deliberately not one of them**: there the submit *is* what you asked for, so a below-floor refusal is a real failure — you get this row and exit `2`, with the floor gap printed alongside it as context. Reporting `0` there would be [#436](https://github.com/civitai/cli/issues/436)'s own false success, rebuilt in the command that fixes it. | [Listing media requirements](#listing-media-requirements) |
| `there is no open revision to submit` | `civitai app listing submit-revision` found no revision staged on your live listing, so there is nothing to send a moderator and it refuses rather than opening an empty one. Stage the change first (`rm-screenshot <id>`, or one of the attach commands), then re-run it. Exit `1`. | [Listing media requirements](#listing-media-requirements) |
| `this listing is not live` | `civitai app listing submit-revision` on a draft or pending listing. Only an approved listing has revisions — everything else is edited directly, so your change is already applied. Check it with `civitai app listing status`. Exit `1`. | [Listing media requirements](#listing-media-requirements) |
| `no such directory — pass the path to an App project root` | The path does not exist. This is a **usage** error: exit `2`, and `--json` prints nothing at all. | [Exit codes](#exit-codes) |
| `is not a directory — pass the App project ROOT` | You pointed at a file — often the manifest itself. Pass the directory holding it. Exit `2`. | [Exit codes](#exit-codes) |
| `it did NOT check that the file is loaded` | The `BLOCK_READY` advisory on its **weak** tier: it could not resolve what your `index.html` loads, so it only checked whether *some* file mentions the message. The lines that follow name what it could not follow. | [The host handshake](#the-host-handshake-block_ready) |
| `nothing index.html loads reaches it` | The **strong** tier: the emitter is in your project but nothing the browser loads reaches it — an orphan file. Copying `civitai-host.js` in is only half the fix; it has to be referenced too. | [The host handshake](#the-host-handshake-block_ready) |
| `no lockfile is committed` / `is not a lockfile` | The platform build installs **strictly** from the committed lockfile, so a missing one — or a zero-byte one created with `touch` — fails the build server-side. A lockfile is generated by the package manager, never hand-written. | [Validate fidelity](#validate-fidelity) |
| `refusing to submit without --yes` | A submit that would really upload asked for confirmation and found no TTY. Pass `--yes` in CI, or `--package-only` to just write the .zip. | [Command reference](#command-reference) |
| `What this CLI sent` / `largest entries in the bundle` | Not an error of its own — it is printed **under** a failed submit, and it is the CLI's account of what left your machine: the exact bytes that went on the wire, and the biggest entries they were made of. It appears because the server's own message may name nothing you can act on; `400: Invalid JSON` is the measured case ([#423](https://github.com/civitai/cli/issues/423)), an error about the *parse* raised downstream of an oversized request body. **The CLI does not claim to know why the submit failed** — it cannot see the server's limits, and its own size caps are much higher than the server's real one, which is bracketed only to (2.32 MB, 8.20 MB] and is deliberately not vendored here. If the bundle is large, drop what the platform build does not need and retry. Not printed for a `401`/`403`/`429`. | [Submit & auth](#submit--auth) — *How big can a bundle be?* |
| `Your repo may be behind what was last released` / `Resubmitting the version that is already live is almost always an accident` / `That version is approved but not live` | The **monotonic-version guard**: `civitai app submit` refused because the manifest version is not strictly above the highest **approved** version of that app, and approving an older (or identical) version supersedes the newer one — replacing the live deployment when that version is serving. Exit `1`; `--allow-downgrade` submits anyway. **The second line tells you which of four cases you are in, and each says only what is actually known.** A **lower** version against a version that is **live** replaces that deployment on approval, so your repo may be behind what was last released — bump the manifest, or `civitai app pull` first. A **lower** version against an approved row that is **not** live deploys code older than the highest approved version; no deployment of that version is being replaced, because none is running. The **same** version against a deployment that is really live is almost always an accident. The **same** version against an approved row that is **not** live is a resubmit of a deploy that has not landed — still building, still deploying, or failed — which is a plausible deliberate act, and `--allow-downgrade` submits that version again. **"Live" is the server's own answer** (the `liveUrl` it returns, the same field `civitai app status` prints `Live at:` from), not a guess from the deploy state — so a legacy approval that predates deploy-state tracking is correctly treated as serving. | [Exit code 1](#exit-code-1) |
| `from a dirty git work tree` / `that go into the bundle are not committed` | The **dirty-work-tree guard**: `civitai app submit` refused because files that go into the bundle are uncommitted. The bundle is packaged from what is on disk, so approving one deploys code that exists in no commit and nothing afterwards can say which revision is live. The refusal names the paths (`git status` spelling, relative to the packaged directory) — commit them, or pass `--allow-dirty` to submit the tree exactly as it is. Exit `1`. **It only fires inside a git repository**: a scaffolded app that was never `git init`ed submits unchanged, and so does a machine with no `git` on `PATH`. Paths the packager never ships — `dist/`, `node_modules/`, a stray `.zip`, a `.env.local`, anything `.gitignore`d, and any symlink (the packager bundles regular files only) — are not counted, because they are not in the bundle. 🔴 **A repository with no commits yet refuses *everything*, `block.manifest.json` included** — a `git init` you have not committed into means nothing in the bundle is in a commit, which is exactly what the guard checks. That is the row between "no repo" and "repo, dirty": make the first commit, or pass `--allow-dirty`. Scaffolding never puts you here — `civitai app create` / `app init` run no `git init`. **A `git mv` counts as two changes**, because the bundle gains the destination and loses the original; both are named, even when the destination is a path the packager drops (`git mv src/App.tsx dist/App.tsx` is refused, naming `src/App.tsx`). | [Exit code 1](#exit-code-1) |
| `HEAD is on no remote` | A **warning**, not a refusal — the submit continued. The packaged tree is clean, but the commit it is clean against exists only on this machine, so the deployed version cannot be traced back to anything anyone else can fetch. Push the branch. | [Command reference](#command-reference) |
| `refusing to withdraw without --yes` | A withdraw asked for confirmation and found no TTY. It gates because withdrawing a **first-version** submission deletes that app's store listing — icon, cover and every captioned screenshot. Pass `--yes` once you have accepted that; nothing was withdrawn and no request was sent. 🔴 **BREAKING:** this refusal is new — a scripted `civitai app withdraw <id>` that used to exit `0` now exits `1` until you add `--yes`. | [After you submit](#after-you-submit-review--approve--deploy) |

### Generating

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `refusing to spend Buzz without --yes` | The same gate on the money path. `--dry-run` prices the job without spending anything. | [Confirmation](#confirmation) |
| `--image requires --ecosystem` | Without an ecosystem the server never promotes the job to image-to-image: your images are silently dropped and you are billed for a plain text-to-image run. Hence a refusal rather than a warning. | [Image-to-image](#image-to-image---image-and---ecosystem) |
| `interrupted while waiting` | **The generation is still running and has already been charged.** Ctrl-C stopped the wait, not the job. Re-attach with `civitai workflows get <id>`. | [Waiting, downloading, and re-attaching](#waiting-downloading-and-re-attaching) |
| `model substituted` | The server ran a **different checkpoint** than you asked for and billed for what ran. Warned by default; `--fail-on-substitution` turns it into a refusal on the estimate, before any spend. | [Silent model substitution](#-silent-model-substitution) |
| `The server reported: …` | The orchestrator recorded an account of what happened, and what follows is the server's own words. The CLI does not interpret them and does not know whether the failure is retryable; the only thing it changes is that invisible and direction-reversing characters are removed before the text reaches your terminal (`--json` is unfiltered). Printed on the `generate` error and by `civitai workflows get`. | [What the server says went wrong](#what-the-server-says-went-wrong) |
| `An indented line under a row is what the server recorded` | The same record, on `civitai workflows list`: the indented lines beneath a workflow's row are the server's own words for that workflow, wrapped but never abbreviated (the invisible characters above are removed, and wrapping collapses runs of whitespace and breaks a token longer than the line — no words are dropped). The indent is not decoration — it keeps server text out of the column a real row starts in, so a message cannot pose as a workflow of yours. It holds for the line breaks the CLI makes: the CLI wraps to a fixed 79 columns and never asks how wide your terminal is, so in a **narrower** terminal — or with wide (CJK) characters — your terminal re-wraps and the overflow can still reach column zero. | [What the server says went wrong](#what-the-server-says-went-wrong) |
| `the orchestrator often supplies no failure reason, so it may not say why` | The same failure with **no** account recorded — a real, measured case, not a CLI limitation. Neither `civitai workflows get <id>` nor `civitai workflows list` will say why either. | [What the server says went wrong](#what-the-server-says-went-wrong) |

### Everything else

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `has no approved App Block yet` | The slug is right and the app exists — its analytics do not, because no submitted version has been **approved**. Like `app pull` below, the message names the next step for the latest submission's own state: where it is in review, or — for a **rejected** or **withdrawn** one — that nothing is in review and a new `civitai app submit` is what moves it. Exit `1`, not `4`. | [App metrics](#app-metrics) |
| `no such app for your account` | The server did not recognise the app for your account — exit `4`. From `civitai app pull` it means, more precisely, that the CLI could not prove the app is yours-but-unapproved: no submission matches the slug, **or** the submissions lookup itself failed, **or** a version *is* approved, **or** you passed an `appBlockId` rather than the slug. Settle it with `civitai app status`; if that lists the app as unapproved, `app pull` says so with the message below instead. | [Submission status](#submission-status) |
| `has no approved version yet` | `civitai app pull` clones a repository that only exists once a submitted version has been **approved**. The app is real; the message names the latest submission's state and the next step for it. Exit `4`. | [Pull your app's repository](#pull-your-apps-repository-app-pull) |
| `no such submission` | For a **slug**: nothing has been submitted for that app yet — `civitai app submit` creates the submission **and** the draft store listing the `app listing` commands read. For an `--id`: no publish request with that id, so check the id itself. **An OFFSITE app should no longer reach this row from `civitai app listing`**: that command falls back to selecting the listing by slug and normally succeeds. It still reaches it from `civitai app status`, where the app is offsite but the store-catalog lookup could not confirm it (unpublished listing, or a caller who cannot see the catalog yet) — the row below is the confirmed version, and this one carries advice that cannot work. | [Submit & auth](#submit--auth) |
| `is an OFFSITE app` | The slug is right, the app exists, and it is an **offsite** app — a registered URL rather than a block bundle — so it has no block submission for either command to resolve through. From `civitai app status` that is simply the truth, and it is the normal outcome: there is nothing to be pending, approved or deployed. From `civitai app listing` it is **not** the normal outcome any more — that command reaches an offsite listing by slug (`civitai/civitai#3989`, deployed), so seeing this means the by-slug lookup *also* answered **not found**: a Civitai without `#3989` (older or self-hosted), or an app with no listing row. A listing your account does not own is not one of those — it resolves and is refused `403`, exit `3`. The message names those, names `civitai app view <slug>` for what the CLI can still show, and names the **App-store listing UI on civitai.com** as the surface that always works. Exit `4`. Neither message names `civitai app submit`, because that step cannot exist here. **You only get this row when the CLI can see the app in the public store catalog** (`GET /api/v1/apps/{slug}`) — that route serves only a **published** listing and is still behind a launch flag, so an unpublished offsite app, or a caller without moderator/app-dev-tester access, gets the `no such submission` row above instead. Same exit `4`, worse advice. | [After you submit](#after-you-submit-review--approve--deploy) |
| `is ambiguous — it matches` | A model version has several files sharing that name. Select one by its numeric file id with `--file <id>`. | [Download model files](#download-model-files) |
| `SHA256 mismatch for` | A download's hash did not match, and the partial file was deleted. Retry — this is integrity checking working, not a bug. | [Download model files](#download-model-files) |
| `checksum mismatch for` | The same, during `civitai upgrade`. The binary was **not** replaced. | [Upgrading](#upgrading) |
| ``git is required for `civitai app pull` `` | `app pull` shells out to `git`, which is not on your `PATH`. | [Pull your app's repository](#pull-your-apps-repository-app-pull) |

Still stuck? Every command takes `--help`, `civitai --help` prints the exit-code
contract, and failures are differentiated by [exit code](#exit-codes) — so a
script can branch on the *kind* of failure without matching any of these
strings.

## Development

```bash
make ci      # go mod tidy + vet + test + build
make lint    # golangci-lint — a SEPARATE job; `make ci` does not run it
make test
make build   # -> bin/civitai
make fmt
go test ./... -cover
```

🔴 **`make ci` is not a mirror of CI.** It runs tidy + vet + test + build and
**does not run lint**, which is its own CI job — so run `make lint` too before
calling a change done. It errors out when `golangci-lint` is not on `PATH`
rather than degrading to something weaker, which is what makes a clean run mean
anything.

- **Language:** Go 1.25, [Cobra](https://github.com/spf13/cobra) (commands) +
  [Viper](https://github.com/spf13/viper) (config).
- **Layout / conventions / how to add a command / release process:** see
  [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md).
- **Contributing:** see
  [`CONTRIBUTING.md`](https://github.com/civitai/cli/blob/main/CONTRIBUTING.md).

**CI is eight jobs, not four steps.** `.github/workflows/ci.yml` runs
`build-test` (vet + `gofmt -s -l .` + test + build), `lint`, `schema-drift`,
`pins-vs-published`, `ready-ack-runtime`, `template-page-vite`,
`template-page-money` and `scaffold-currency` on every push to `main` and every
PR. Several exist to catch drift between this repo and the platform — the
vendored schema, the scaffold's npm pins, the block→host handshake — which a
plain `go test` cannot see.

**Running and gating are different questions**, and fewer of those jobs gate a
merge than run. The measured set of required status checks, and the instruction
to re-measure rather than trust a written copy, live in [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md)
item 11 — deliberately in one place, because a second copy is how the original
claim went stale. Notably `lint` reports without blocking, which is another
reason to run it locally.

## Releasing

Releases are built by [goreleaser](https://goreleaser.com) from a GitHub
Actions workflow on a `v*` tag push:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This cross-compiles for linux/darwin/windows × amd64/arm64, stamps
version/commit/date, and creates a **draft** GitHub Release with archives +
`checksums.txt`. It also *renders* the Homebrew cask and attaches it to the
release, but does not push it to the tap — see below. See
[`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) for the full process. The tap-write secret
(`HOMEBREW_TAP_GITHUB_TOKEN`) belongs to
`.github/workflows/release-homebrew.yml`, which runs at *publish* time; the
tag-time run holds no credential that can write to the tap.

🔴 **There are three publication channels, and clicking "Publish release" fires
the other two.** `.github/workflows/release-npm.yml` publishes the `npm/`
wrapper as **[`@civitai/cli`](https://www.npmjs.com/package/@civitai/cli)** —
the very first install option at the top of this README — and
`.github/workflows/release-homebrew.yml` pushes the cask to
`civitai/homebrew-tap`. Both trigger on `release: [published]`. So publishing
the draft is not the last step of the GitHub release; it is also, in the same
click, an npm publish and a Homebrew release. That matters because **npm
unpublish is restricted**: a bad version is corrected by publishing another one,
not by taking it back.

🔴 **Nothing downstream may act on a tag alone, and the Homebrew channel used
to.** Until [#308](https://github.com/civitai/cli/pull/308), the same goreleaser
run that created the *draft* release also pushed the cask bump, so the cask
named a version whose archives 404 for everyone until a human clicked "Publish
release". Measured on 2026-08-09, with v0.1.91 tagged at 01:09Z and still a
draft hours later:

```
cask Casks/civitai.rb          version "0.1.91"
GET .../download/v0.1.91/civitai_0.1.91_linux_amd64.tar.gz   404
GET .../download/v0.1.90/civitai_0.1.90_linux_amd64.tar.gz   200
npm @civitai/cli                                             0.1.90   (correct)
```

`brew install civitai/tap/civitai` failed for every user for ~2 hours. npm was
correct throughout because it already waited for `release: [published]`; the
Homebrew channel now waits for the same event, and `tools/caskcheck` asserts the
invariant — the cask must never name a version that is not publicly
downloadable — on every publish and once a day, over real unauthenticated HTTP.

🔴 **A failing scheduled run notifies almost nobody, so the daily check files
its own report.** GitHub sends scheduled-workflow notifications only to the one
account that last edited the `cron` line, through that person's own per-user
Actions setting — there is no org-, team- or repo-level failure notification.
Measured in this repo: every scheduled run of `bump-flake-vendorhash.yml` failed
on 2026-07-20, 2026-07-27 and 2026-08-03, and no issue was ever filed about it.
So `release-homebrew.yml` opens a **GitHub issue** titled `[cask-check] …` when
the check is not green, rewrites that one issue's body on each subsequent
failure (an edit notifies nobody, so a week of failures is one issue and zero
comments), comments only when the *kind* of failure changes, and **closes** the
issue when the check goes green — which is what makes the next failure notify
again. It distinguishes "the check failed" from "the check could not run": an
unreachable tap, an unreadable cask, an unclassifiable error and a `verify` job
that died before producing a verdict each get their own wording, and none of
them is allowed to read as a clean bill of health.

`gh workflow run release-homebrew.yml -f drill=broken|lagging|unmeasurable` is a
**fire drill**: it points the check at a fixture, opens a real issue exactly as a
real failure would, and cannot touch the tap. Run it after changing any of this —
a notification path nobody has watched work is not a notification path.

A cask that merely **lags** a published release is green for the first 24h,
because that is the normal state right after a publish and a permanently-red
check is worse than none. After that it is a finding of its own, distinct from
the 404 outage above and explicitly *not* claiming users are broken: the only
thing allowed to move the cask is that `release: published` job, so a cask still
lagging a day later means the event never reached it — a dropped webhook, an
expired `HOMEBREW_TAP_GITHUB_TOKEN`, a failed push nobody read. The threshold is
measured, not guessed: across the 30 most recent releases the gap from
`published_at` to the tap commit is ~1 minute, and the tag→publish gap (a
strictly larger window) has a median of ~2m30s and a worst case of 1h55m — the
2026-08-09 incident itself.

Authentication for that job is **OIDC trusted publishing** — there is no
`NPM_TOKEN` secret. The trust is bound to the repository *and to that workflow
file's path*, so moving or renaming `release-npm.yml` breaks publishing, and no
secret rotation will fix it.

## License

[Apache License 2.0](LICENSE).
