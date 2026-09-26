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

## Using an AI coding agent? Paste this

```text
Fetch and execute the appropriate instructions to set me up for Civitai from https://developer.civitai.com/agent-setup/prompt.md
```

Your agent installs the CLI, configures itself for App development (an
`AGENTS.md` for your project, plus Civitai's two MCP servers), and verifies the
result. It stops before authenticating and hands `civitai login` back to you —
it will not log in on your behalf.

Prefer to read it first? The whole prompt is rendered at
[developer.civitai.com/agent-setup](https://developer.civitai.com/agent-setup/).
If a step is refused or an agent goes off-script, the rest of this README is the
manual path — nothing below depends on having used the prompt.

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

> **This README is the front door, not the manual.** It carries what you need to
> get moving and the contracts a script branches on — exit codes, `--json`
> shapes, the command table. The depth lives in two places that cannot go stale
> the way a second copy would: **`civitai <command> --help`**, which ships inside
> the binary you already have, and the guides on
> **[developer.civitai.com](https://developer.civitai.com)**, which are versioned
> with the platform. Sections below marked 📖 point at one or both.

**Get started**

- [Using an AI coding agent? Paste this](#using-an-ai-coding-agent-paste-this)
- [Install](#install)
  - [npm (Node)](#npm-node)
  - [Homebrew (macOS)](#homebrew-macos)
  - [Nix flake](#nix-flake)
  - [Prebuilt binary](#prebuilt-binary)
  - [Go install (from source, Go 1.25+)](#go-install-from-source-go-125)
- [Quickstart: browse & download](#quickstart-browse--download)
- [Quickstart: build an App Block](#quickstart-build-an-app-block)
- [Command reference](#command-reference) — one table of the authoring &
  account commands (the public-API reads have their own)

**Author an App**

- [Set up your coding agent (`agent-setup`)](#set-up-your-coding-agent-agent-setup) — **run this first**
  - [The two MCP servers](#the-two-mcp-servers)
- [SDK packages](#sdk-packages)
- [The blockId](#the-blockid)
- [Templates](#templates)
- [The host handshake (`BLOCK_READY`)](#the-host-handshake-block_ready)
- [Local dev loop (harness: mock vs live)](#local-dev-loop-harness-mock-vs-live)
- [Preview in the real host (`app dev-tunnel`)](#preview-in-the-real-host-app-dev-tunnel)
- [Examples](#examples)
- [Validate fidelity](#validate-fidelity)
- [Submit & auth](#submit--auth)
  - [What `civitai whoami` reports](#what-civitai-whoami-reports)
  - [Link your source code (`app listing set-source-repo`)](#link-your-source-code-app-listing-set-source-repo) — **material: stages a revision**
- [Submission status](#submission-status)
- [Listing doctor (`app doctor`)](#listing-doctor-app-doctor) — **gates a release on exit code**
- [Pull your app's repository (`app pull`)](#pull-your-apps-repository-app-pull)
- [Browse the App store](#browse-the-app-store)
- [App metrics](#app-metrics)

**Use the API**

- [Browse the public API](#browse-the-public-api)
- [Download model files](#download-model-files)
- [Generate](#generate) — **spends real Buzz**
- [Scripting with `--json`](#scripting-with---json)

**Reference**

- [Upgrading](#upgrading)
- [Global flags](#global-flags) — colour, `--version`, the update nag
  - [What a table cell can contain](#what-a-table-cell-can-contain)
- [Configuration](#configuration)
- [Exit codes](#exit-codes)
- [Troubleshooting](#troubleshooting) — **look the error message up here**
- [Development](#development)
- [License](#license)
## Install

Pick whichever fits — **npm** is the most convenient if you already have Node
(App authors usually do); Homebrew is quickest **on macOS** (it is the only
platform the tap covers); the prebuilt binary needs no toolchain; `go install`
builds from source.

### npm (Node)

A thin wrapper that downloads the matching prebuilt binary for your OS/arch on
install and verifies its sha256 against the release `checksums.txt`:

```bash
npm install -g @civitai/cli
# or run it without installing:
npx @civitai/cli --help
```

### Homebrew (macOS)

```bash
brew install civitai/tap/civitai
```

🔴 **macOS only — there is no Linux Homebrew install.** What lands in
`civitai/homebrew-tap` is rendered by this repo's release config, and that
`.goreleaser.yaml` carries no `brews:` (formula) stanza at all — `brews:` and
`homebrew_casks:` are **goreleaser** stanzas, so that is the file to check, not
the tap repository. Only a **cask** is ever published, a cask is a macOS-only
concept, and the rendered one names `darwin` archives; on Linux, Linuxbrew
included, there is nothing to install. Use [npm](#npm-node), the
[Nix flake](#nix-flake), a [prebuilt binary](#prebuilt-binary) or
[`go install`](#go-install-from-source-go-125).

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
civitai login                                                    # downloads need a token

# Search models — filter by base model, type, and sort:
civitai models search --base-model Illustrious --type Checkpoint --sort "Most Downloaded"

# Download a version's file(s) — SHA256-verified, streamed atomically.
# `--layout` routes each file into the right app subfolder (also `a1111`);
# `--dry-run` prints the plan without transferring.
civitai download 691639 --layout comfyui --root ~/ComfyUI
```

The other read commands — images, tags, creators, users, articles, collections —
and the rest of `download`'s selection, paging and routing flags are walked
through in the
[CLI guide](https://developer.civitai.com/site/guide/cli). This file keeps the
command inventory in [Browse the public API](#browse-the-public-api) and the
full download behaviour in
[Download model files](#download-model-files).

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
#    Full bounds (and who checks what) are in that assets/README.md, and in the
#    Store listing guide linked right after this block.
civitai app listing set-icon ./assets/icon.png
civitai app listing set-cover ./assets/cover.png
civitai app listing status
```

> **Step 7 needs artwork you supply.** Every template scaffolds an `assets/`
> directory with a README of the requirements, and deliberately **no placeholder
> images** — a placeholder passes every check and uploads cleanly, which is how a
> stub icon reaches a public listing. Sizes, formats and aspect ratios are in
> [Store listing](https://developer.civitai.com/apps/guide/store-listing).

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

## Command reference

> **This table is the authoring & account half of the CLI — it is not every
> command.** The **public-API read commands** — `civitai models`,
> `model-versions`, `images`, `articles`, `collections`, `creators`, `tags` and
> `users` — have their own table, with their flags, in
> [Browse the public API](#browse-the-public-api); `civitai download` is
> documented there and in [Download model files](#download-model-files).

| Command | What it does |
| --- | --- |
| `civitai agent-setup [--track app\|api] [--agent <name>] [--dir <path>] [--check] [--json] [--dry-run]` | **Set up the coding agent you are using to build Civitai Apps** — an `AGENTS.md` block, a `CLAUDE.md` shim, and the two Civitai MCP servers in that agent's own config file. 🔴 **It never writes a credential into any of those files**, and it never authenticates. See [Set up your coding agent](#set-up-your-coding-agent-agent-setup). |
| `civitai login [--scopes <set>] [--token [<t>]] [--no-browser]` | Browser OAuth device login by default; `--scopes generate` additively grants generation + Buzz **spend**, which the default set withholds. `--token <t>` stores a personal API key instead. See [Submit & auth](#submit--auth). |
| `civitai whoami [--scopes] [--json]` | Verify the stored token — user, credential type, and a **`Capabilities:`** section decoded from its scope, so a money-path dead end is visible before `dev:live`. **Submit Apps is tri-state**: `unknown` is never `no`. See [What `civitai whoami` reports](#what-civitai-whoami-reports). |
| `civitai buzz [--json]` | Show your spendable Buzz balance (**blue / green / yellow**, plus a **total**); needs the BuzzRead scope, which a **default** OAuth login token lacks (`civitai login --scopes generate`, or a full-scope personal API key). `--json` emits `{blue,green,yellow,total}`. |
| `civitai app list [--kind <k>] [--category <c>] [--sort <s>] [--limit <n>] [--cursor <c>] [--json]` | **Discover published Apps in the store** (`GET /api/v1/apps`) — filter-based, cursor-paged, and **not anonymous**: it needs a credential. See [Browse the App store](#browse-the-app-store). |
| `civitai app view <slug> [--json]` | **Show one published App's store detail** (`GET /api/v1/apps/{slug}`) — the public store catalog, not your own deploy. Needs a credential, same as `app list`. See [Browse the App store](#browse-the-app-store). |
| `civitai app create [name] [dir] [--template static\|page-vite\|page-money] [--dir <path>] [--name <display>] [--slug <slug>] [--yes]` | **The friendly happy path** — scaffold a ready-to-build App, defaulting to the batteries-included `page-money` SDK template (default dir `./<slug>`). `--slug` sets the **blockId** explicitly. See [Templates](#templates) and [The blockId](#the-blockid). |
| `civitai app init [name] [dir] [--yes] [...]` | Same scaffolder as `create` with a no-build `static` default (back-compat alias); same `--yes`. |
| `civitai app dev-token <slug> [--env] [--spend] [--budget <n>]` | **Mint a short-lived (~4h) dev block token for `npm run dev:live`**; `--spend` must be asked for explicitly to request real-Buzz spend, and `--env` prints a paste-ready `VITE_LIVE_BLOCK_TOKEN=<token>`. See [Local dev loop](#local-dev-loop-harness-mock-vs-live). |
| `civitai app dev-tunnel [blockId] [--block <id>] [--port <n>] [--local-host <host>] [--tunnel-endpoint <h:p>] [--idle-timeout <d>] [--ready-timeout <d>] [--no-wait]` | **(Pre-GA / invite-gated)** Preview your **local** dev server inside the **real** Civitai host at `civitai.com/apps/dev/<blockId>`. See [Preview in the real host](#preview-in-the-real-host-app-dev-tunnel). |
| `civitai app validate [dir] [--strict] [--json]` | Best-effort local pre-check of `block.manifest.json` — warnings are non-fatal unless `--strict`, and a `[dir]` that is missing or not a directory is a **usage error** (exit `2`, no JSON). See [Validate fidelity](#validate-fidelity). |
| `civitai app submit [dir] [--yes] [--package-only] [--out f.zip] [--skip-validate] [--allow-downgrade] [--allow-dirty] [--allow-oversize]` | Validate + package the source tree + upload it with your stored token (with no token it writes the bundle and prints next steps). A submit that would really upload asks for confirmation, and in a non-interactive shell it refuses without `--yes`. It also refuses a version that is not strictly above the highest APPROVED version, a dirty git work tree, and a body larger than the server can receive — each waived by the matching flag above, and **all three skipped on the routes that never reach the server**. See [Submit & auth](#submit--auth) and [Exit code 1](#exit-code-1), which maps each refusal to its flag. |
| `civitai app pull [dir] --app <slug\|appBlockId>` | **Clone (or sync) the canonical git repository behind one of your approved Apps.** ⚠ The clone URL embeds your access token and a fresh clone persists it into `.git/config`. See [Pull your app's repository](#pull-your-apps-repository-app-pull). |
| `civitai app listing status [--json]\|set-text [--tagline <t>] [--description <d>] [--category <c>] [--clear <fields>] [--yes]\|set-source-repo <url>\|--clear\|set-icon <file>\|set-cover <file>\|add-screenshot <file>\|rm-screenshot <id>\|reorder <id...>\|submit-revision` | **Attach the store-listing media your App needs before it can be published** — an **icon and a cover are mandatory** (screenshots optional, up to 8), and `listing status` prints what the publish floor still requires. 🔴 **`listing status` is not a pure read — `--json` or not — so do not poll it**: on a live listing it opens a revision draft. On a LIVE listing a **material** change — the media commands, and `set-source-repo` — is **staged on a revision** and is not live until `submit-revision` is approved. 🔴 **`set-text` is the exception: it applies IN PLACE, immediately and publicly**, with no revision to review or abandon. **ON-SITE apps are refused** by `set-text` and `set-source-repo`. See [Store listing](https://developer.civitai.com/apps/guide/store-listing) and [Link your source code](#link-your-source-code-app-listing-set-source-repo). |
| `civitai app status [blockId] [--id <pubreq>] [--limit N] [--json]` | Check the review/deploy status of **your own** submissions — all of them, or one in detail by `blockId` or `--id`, with a **SOURCE** column carrying the commit the submitting client claimed. Run from inside an app checkout it also warns on **stderr** when your `block.manifest.json` is **BEHIND** your highest approved version. See [Submission status](#submission-status). |
| `civitai app doctor [slug] [--json]` | **Diagnose what is incomplete or blocked on your App store listings, and how to fix it**, across every listing you own or hold an **accepted** collaborator seat on. 🔴 **Exits `1` when a blocking problem sits on a listing that can still publish, `0` otherwise**, so it gates a release script. A pure read. See [Listing doctor](#listing-doctor-app-doctor). |
| `civitai app metrics <slug> [--from <d>] [--to <d>] [--json]` | **Owner-only analytics for one of your Apps** — installs, runs + Buzz spent, Buzz purchased, API engagement — always printing the window the **server** served. Needs the **Apps submit scope**. See [App metrics](#app-metrics). |
| `civitai app withdraw [pubreq-id] [--id <pubreq>] [--yes]` | **Withdraw your own pending submission** (the `pubreq_…` id from `civitai app status`), freeing the slug. **It also deletes a first-version app's store listing**, so it asks first and needs `--yes` in a script. See [Submission status](#submission-status). |
| `civitai generate "<prompt>" [--negative-prompt <p>] [--quantity <n>] [--aspect-ratio <r>] [--checkpoint <version-id>] [--lora <version-id>[:strength]] [--image <path-or-url>] [--ecosystem <key>] [--input <file>] [--print-input] [--dry-run] [--json] [--max-cost <buzz>] [--fail-on-substitution] [--yes] [--no-wait] [--timeout <dur>] [--out-dir <dir>] [--out-name <template>] [--no-download] [--force] [--external-id <key>]` | **Generate images from a text prompt — this SPENDS REAL BUZZ.** Prices the job, shows the cost + your balance, asks, submits, then **waits and downloads**. `--dry-run` prices it without submitting; `--max-cost` is an **estimate check, not a spending cap**. Needs the AI Services scopes; a **default** OAuth login is refused. See [Generate](#generate). |
| `civitai workflows list [--limit <n>] [--cursor <c>] [--tag <t>] [--json]` | **List the generation workflows you have submitted**, newest first — status, when, cost and `deliverable/total` outputs. Cursor-paged; reading spends nothing. See [Tracking generations](https://developer.civitai.com/site/guide/cli-workflows#listing). |
| `civitai workflows get <workflow-id> [--json]` | **Look up one generation workflow** — status, steps, outputs and the Buzz transactions recorded for it. This is how you re-attach after `--no-wait`, a `--timeout` expiry or a Ctrl-C; output URLs are presigned and expire. See [Waiting and re-attaching](https://developer.civitai.com/site/guide/cli-generate#waiting-downloading-and-re-attaching). |
| `civitai workflows cancel <workflow-id> [--yes] [--json]` | **Stop a running generation.** 🔴 **You are billed for what it already delivered**, and this CLI cannot report the figure. Asks for confirmation (default **no**); a non-TTY without `--yes` refuses. See [Cancelling a workflow](https://developer.civitai.com/site/guide/cli-workflows#cancelling-a-workflow). |
| `civitai upgrade [--force]` | **Self-update this binary in place**, verifying the release's SHA-256 against `checksums.txt`. The asset follows the platform the release publishes — a `.zip` on Windows, a `.tar.gz` elsewhere. A **macOS** Homebrew install delegates to `brew upgrade`; `--force` reinstalls anyway. See [Upgrading](#upgrading). |
| `civitai version` | Print version / commit / build date. |
| `civitai completion [shell]` | Generate a shell-completion script. |

Run `civitai help`, `civitai app --help`, or `civitai <command> --help` for the
full details and examples.

## Set up your coding agent (`agent-setup`)

If you are building your App with a coding agent — Claude Code, Cursor, Codex,
opencode, VS Code Copilot, Windsurf, Zed — run this once in the project:

```bash
civitai agent-setup
```

It does three things, and **it never authenticates**:

1. Writes an `AGENTS.md` **managed block** into the project — the commands and
   the gotchas an agent cannot infer by reading your code (Buzz is the *viewer's*,
   a newly declared scope is consent-gated, a hung hook is usually a missing HOST
   handler, `useSharedStorage()` has no REST route). Its "Local development"
   section is **read from the directory**, not fixed, so
   **re-run `civitai agent-setup` after changing your scripts.**
2. Writes a one-line `CLAUDE.md` containing `@AGENTS.md`, **only when there is
   no `CLAUDE.md` already**. Claude Code does not read `AGENTS.md` on its own.
3. Registers the **two Civitai MCP servers** in the detected agent's own config
   file.

🔴 **It never writes a credential into any file it touches, and it never
authenticates.** Where a vendor's config supports an env-var reference it writes
that reference; where it does not, the row is reported as `manual` and names no
file for this CLI to write.

**No `@civitai/*` version is pinned anywhere in what this writes.** Pins live in
`civitai app init`, which CI holds against npm; a version literal in an
instruction file rots in silence.

The agent is detected from the environment first and then from marker files in
the project; `--agent <name>` overrides it, and `--dir <path>` points at a
project other than the working directory. A path in `--json` is always
**absolute**, whatever `--dir` you passed — except on a `manual` row, which names
no file for this CLI to write and carries an empty `path`.

### The two MCP servers

| Server | URL | What it reaches | Anonymous? |
| --- | --- | --- | --- |
| `civitai` | `https://mcp.civitai.com/mcp` | the Civitai site — models, images, articles, your account | **yes** — answers without a credential |
| `civitai-orchestration` | `https://orchestration.civitai.com/mcp` | the generation orchestrator — workflows and image generation | **no** — returns `401` until an `Authorization` header is present |

🔴 **That difference decides whether a header-less setup is finished.** It is not
a read/write split: the orchestration server refuses the **handshake** itself, so
nothing on it is reachable until you add a header. A header-less setup still
reaches models, images and articles through the `civitai` server, and
`civitai-orchestration` answers it `401`.

**`--check` contacts nothing** and reports six rows — `cli-version`,
`agents-md`, `claude-md`, `mcp-site`, `mcp-orch`, `authenticated`. **Every row
reports PRESENCE, never acceptance.**

🔴 **`ok` is the AND of every check except `authenticated`, — for any agent
other than `claude` — `claude-md`, and — for an agent this CLI has no config
target for — `mcp-site` and `mcp-orch`.** An unauthenticated setup is a
success, not a failure, and every other agent reads `AGENTS.md` directly so the
`CLAUDE.md` shim is inert for it. So on a `claude` project the exempt row is
`authenticated` alone, because `claude-md` counts there. That last exemption is
conditional and narrow: for a **known** agent a missing entry still fails, and
the rows stay in the report and stay `false`.

`civitai agent-setup --help` carries the per-vendor header spellings, the merge
rules and the JSONC re-encoding caveat in full.

`civitai agent-setup --help` also names the top-level key each agent uses
(`mcpServers`, VS Code's `servers`, opencode's `mcp`, Zed's `context_servers`,
Codex's `mcp_servers`), where that file lives, and what `--check` and
`--dry-run` report — including the exit codes they use.
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

## The blockId

The **blockId** is your app's permanent public identity: the hostname it will be
served at once approved (`https://<blockId>.civit.ai/`) and the argument every
later command takes (`app status`, `app metrics`, `app listing`, `app dev-token`,
`app dev-tunnel`). **It cannot be renamed afterwards.** `app create` / `app init`
echo the one they chose, so it is on screen before you commit anything.

By default it is derived from the name: `"My Cool Block"` → `my-cool-block`.
Pass **`--slug <slug>`** to choose it yourself — it bypasses derivation entirely,
so name, blockId and directory are three fully independent axes.

> **Breaking change.** Derivation used to silently **drop characters** it could
> not carry (`"Café App"` → `caf-app`) and to silently **truncate** past the
> **40-character** cap, both at exit 0 — minting a different permanent public id
> than the author typed. It now **refuses** both, exiting **2** and naming the
> offending characters or the length. **A script passing a non-ASCII or long
> name must now pass `--slug <slug>`.** A name deriving **exactly 40**
> characters is *at* the cap, not over it, and still derives byte-identically.

Three things still derive rather than refuse, and they are deliberate: symbols,
emoji and non-ASCII punctuation are **separators** (`"Rocket 🚀 App"` →
`rocket-app`, which is what makes `"Widget — Pro"` → `widget-pro` right); the two
runes that lowercase **into** ASCII transliterate for free (`"İstanbul App"` →
`istanbul-app`); and ASCII is exempt by construction, so every derivation that
worked before still produces the byte-identical blockId. A name that is **not
valid UTF-8** is refused outright.

📖 Full derivation table, the exemption rationale and the residuals it knowingly
ships with: [`AGENTS.md` item
27](https://github.com/civitai/cli/blob/main/claudedocs/decisions/27-blockid-derivation-refuses.md).
Offline: `civitai app create --help`.
## Templates

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
[Store listing](https://developer.civitai.com/apps/guide/store-listing).

📖 What each template contains and when to pick which:
[Quickstart](https://developer.civitai.com/apps/guide/quickstart).
## The host handshake (`BLOCK_READY`)

Every template declares a `page` surface, and the host **will not reveal a page
app until the app posts `BLOCK_READY`** — that handler is the only transition
into the host's ready state. An app that never sends it is replaced by a visible
failure card once the host's bounded retries run out, even though the app itself
renders perfectly. Nothing you can run locally reproduces that.

`page-money` gets the handshake for free: `@civitai/blocks-react`'s iframe
transport acks internally. The two **SDK-free** templates (`static`,
`page-vite`) ship a small vendored emitter, **`civitai-host.js`**, loaded from
the entry point. Leave it in place — and if you are retrofitting it into an
older app, the file has to be *referenced* as well as copied
(`<script src="./civitai-host.js">`, or `import './civitai-host.js';` as the
first line of the entry module). `civitai app validate` checks that reference
where it can resolve your entry point, and tells you when it can't.

> ⚠️ **If you adopt `@civitai/blocks-react`, delete `civitai-host.js` in the
> same change.** Running both is worse than running neither: whichever handshake
> answers the host's *first* `BLOCK_INIT` cancels the host's retry loop **and**
> its readiness timeout, so if the vendored emitter wins that race the SDK
> transport can be left never having seen an init — the host sits "ready",
> showing an app that never started, with no retry and no error card.

> 🔒 **The emitter checks the sender *window*, not the sender's *identity*.**
> That is sound for this one message, because the ack carries no data. It is
> **not** sufficient for anything you add next: the moment you handle an inbound
> message carrying a token, a viewer, storage or a result, check `event.origin`
> against an allowlist — or any page that frames your app can feed it whatever
> it likes. Adopt the SDK before you handle data; it maintains that list from
> `VITE_BLOCK_ALLOWED_PARENT_ORIGINS`.

📖 The bridge, the `{ type, payload }` envelope, answer-don't-announce, and why
`RESIZE_IFRAME` is not part of a page app's protocol:
[Concepts — the host ↔ block
bridge](https://developer.civitai.com/apps/guide/concepts#the-host-block-bridge)
and the [Message bridge
reference](https://developer.civitai.com/apps/reference/messages).
## Local dev loop (harness: mock vs live)

A scaffolded App is a sandboxed iframe, and locally there is no host to send
`BLOCK_INIT` — so `npm run dev` shows you your own UI and nothing of the
protocol. The **`page-money`** template ships a dev **harness** built on the
SDK's [`@civitai/blocks-react`](https://www.npmjs.com/package/@civitai/blocks-react)
hosts, with two modes on deliberately separate subexports:

| Command | Mode | What it does |
|---|---|---|
| `npm run dev:harness` | **mock** (default; the `/testing` subexport) | Synthetic replies — **no real Buzz, no compute, no network.** Safe to spam. Start here. |
| `npm run dev:live` | **live** (the `/live` subexport) | Forwards the App protocol to the **real Civitai backend** with a pasted dev token. **Spends REAL Buzz / real compute.** |

Live mode needs a short-lived (~4h) dev block token. Mint it with **`civitai app
dev-token`**, which reads the scopes to request from your **local**
`block.manifest.json` — so it works on a slug you have never submitted:

```bash
civitai app dev-token my-block --env >> .env.development.local
npm run dev:live
```

🔴 **Generating from `dev:live` needs the AI Services scope, and nothing asks for
it implicitly.** Mint with a spend-capable credential — a full-scope **personal
API key**, or an OAuth login that opted in via `civitai login --scopes generate`
— **and** pass `--spend`. Without the flag the CLI filters `ai:write:budgeted`
out of the request **even when your manifest declares it** (the scaffolded money
app does), so the mint succeeds and your app then refuses to generate with
`block lacks ai:write:budgeted scope`. A **default** `civitai login` mints
read-only either way. `civitai whoami` reports the capability as **Spend Buzz
(AI Services)** and names the fix for whichever credential you have; with no
token at all, `dev:live` **fails safe** — it renders a notice and never spends.

📖 **[Local dev loop guide](https://developer.civitai.com/apps/guide/local-dev)**
— both modes in depth, the mint flags (`--env`, `--spend`, `--budget`), which
credential can spend, what `dev:live` does and does not yet cover, the vite dev
proxy that makes it work from `localhost`, and the scenario knobs.

## Preview in the real host (`app dev-tunnel`)

> **(Pre-GA / invite-gated.)** Access is gated behind an Apps-author **invite**
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

It mints an **ephemeral in-memory ssh keypair** (never written to disk), opens a
reverse tunnel from your dev port to the Civitai tunnel endpoint
(`sish.civitai.com:2224`, live), prints the URL to open, and tears everything
down on Ctrl-C or an idle timeout.

> **Publishing DNS takes 1–3 minutes, sometimes longer.** That wait is normal
> and the command reports elapsed time while it happens — it is not a hang. If
> you Ctrl-C out of it you also lose the embeddability warnings, which is why
> they are printed once *before* the wait as well as again after it.

**Embeddability preflight.** Before minting, the command checks whether the host
can actually *embed* your dev server. The host iframes it sandboxed, at an
opaque `null` origin, so a dev server that is missing
`Access-Control-Allow-Origin: *`, missing the `.civit.ai` entry in
`allowedHosts`, or sending a framing header that excludes `civitai.com` loads as
a **blank iframe with no error anywhere**. The findings are **warnings, never
fatal** — the check reads your config statically and can be wrong — and apps
scaffolded by `civitai app create` already satisfy all of it.

**What the tunnel declares.** The command reads `scopes` and `auth` from the
`block.manifest.json` it is standing in and sends them with the mint, printing
`Declaring scopes: …` and `Declaring auth: oauth` so you see what the tunnel
token will carry. With `"auth": "oauth"` the host hands your local app a real
OAuth token at any app status, including before you ever submit; without it, or
with an older manifest, it gets the block token. **Neither is ever fatal:** a
missing, unreadable or malformed manifest simply declares nothing (you still need
a `blockId` — from that manifest or from the argument). An `auth` that is neither
`block-token` nor `oauth` — a typo, or the wrong case — is **dropped with a
warning** rather than sent, because the tunnel does not run the validator that
would otherwise report it; `civitai app validate` names the finding.

**`CIVITAI_DEVTUNNEL_DEBUG` — a debug-only escape hatch.** ⚠️ Not supported
surface: it is a diagnostic for *this* CLI's tunnel plumbing, and its output
format, its trigger and its existence may change or be removed in any release.
Don't build anything on it.

Set it to **any non-empty value** and the tunnel writes one extra `[debug]` line
to **stderr for each inbound connection** the sish endpoint forwards — naming
the address/port and *origin* address/port sish stamped on that SSH channel, the
subdomain label the CLI bound, and whether Go's `x/crypto/ssh` built-in listener
would have **rejected** the channel. That last field is the point: the built-in
listener parses the origin as an IP address and sish sends a hostname, which is
why this CLI accepts every forwarded channel itself rather than using
`client.Listen`. Reach for it when the tunnel reports ready and the public URL
still `502`s.

```bash
CIVITAI_DEVTUNNEL_DEBUG=1 civitai app dev-tunnel
```

It is **output only** — nothing about how the tunnel behaves changes. The value
is never parsed, so `CIVITAI_DEVTUNNEL_DEBUG=0` and `=false` switch it *on* just
like `=1`; leaving it **unset** is the only way to switch it off. It is read once
as the tunnel is established, and prints nothing until traffic actually arrives.

📖 Every flag and its default (`--block`, `--port`, `--local-host`, `--no-wait`,
`--ready-timeout`, `--idle-timeout`, `--tunnel-endpoint` and
`CIVITAI_DEV_TUNNEL_ENDPOINT`): `civitai app dev-tunnel --help`. Running
embedded, the allowed-parent-origin config and graceful degradation of a direct
visit: [Running embedded & handling direct
traffic](https://developer.civitai.com/apps/guide/embedding).
## Examples

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

Both validate clean — schema conformance only, which says nothing about whether
a value is well-sized.

## Validate fidelity

`civitai app validate` is a **best-effort LOCAL mirror** of the platform's
approve-time validator (`BlockManifestValidator`). **The server is the source of
truth** at review time — passing `validate` locally is a strong pre-check, not a
guarantee of approval, and a `[dir]` that is missing or not a directory is a
usage error (exit `2`, no JSON).

It checks `block.manifest.json` against a **vendored JSON Schema**
([`schema/app-block.manifest.schema.json`](schema/app-block.manifest.schema.json))
plus the ported semantic rules, one **build-time** lockfile rule, and one
**advisory** about the [host handshake](#the-host-handshake-block_ready) — a
warning, never an error, because it infers runtime behaviour from static text.

📖 **What each of those checks actually proves, the lockfile rule in full, the
two tiers of the `BLOCK_READY` advisory, and the `--json` result shape:
[What `civitai app validate` proves](https://developer.civitai.com/apps/guide/validate).**
Flags and exit codes: `civitai app validate --help`.

The **durable fix** for the mirroring is a server-side validate endpoint calling
the real `BlockManifestValidator`. See
[`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md) for the full
caveat and how the vendored schema + Go checks are kept in sync.
## Submit & auth

`civitai login` (no flags) runs the **OAuth device-authorization grant**: it
prints a URL + a short code, you approve in your browser, and the CLI stores a
short-lived access token (1h) plus a refresh token (30d) it rotates
automatically. By **default** it requests `UserRead | AppBlocksSubmit |
AppBlocksDevTunnel` (== `100663297`). That default deliberately omits
`AIServicesWrite`, so a plain login **cannot spend Buzz**.

```bash
civitai login                     # browser device login, default scopes
civitai login --scopes generate   # additive: keeps submit + dev-tunnel, ADDS
                                  # generation (requests 100777985)
civitai login --token <key>       # store a full-scope personal API key instead
civitai whoami                    # who you are, and what the token can do
```

`--scopes` takes a **named set**, never a raw bitmask, and an unknown name is
rejected with the valid list. It applies only to the browser device login and is
refused alongside `--token`. `CIVITAI_TOKEN` overrides the stored credential
(treated as a personal key); config lives at `~/.config/civitai/config.yaml`,
mode 0600.

> 🔴 **The device-flow scope check is all-or-nothing**: requesting any bit the
> `civitai-cli` OAuth client's `allowedScopes` does not permit rejects the
> **whole** login with `invalid_scope`. On `civitai.com` that client allows
> `100777985`, so `--scopes generate` works; against a **self-hosted or older**
> auth server (a non-default `CIVITAI_BASE_URL`) it is rejected, and the CLI
> says plain `civitai login` still works there.

`civitai app submit` always **validates + packages** the canonical source ZIP,
then **uploads it with your stored token**. With **no token configured** (and
not `--package-only`) it instead writes the `.zip` and prints the next steps.
When an upload fails, `app submit` prints what it sent — the compressed and
decompressed sizes, the base64 JSON body size, and the largest entries the
bundle was made of.

Entries are ranked by **compressed** size, because that is what the upload is
made of — a large text file that deflates to nothing is not what to delete. The
usual culprit is a directory of screenshots or sample assets that `app submit`
packages along with everything else. That block prints under **any error the
upload call reports once the request has gone out**, except a `401`/`403` (a
credential problem, unrelated) or a `429`. A failure that never reached the
connection never prints the past tense — no usable credential, an unwritable
config, a connection that never opened — so `sent` means a request that really
went out, not one the CLI only built. A refusal that stops the
submit **before** the upload step prints nothing at all: no `--yes`, a dirty work
tree, the version guard, a validation failure. The ceiling refusal above is the
one refusal with an entry list of its own, under `What this CLI would have sent`.

`--package-only` always just writes the `.zip` and stops. It refuses a version
that is not strictly above the highest **approved** version, a **dirty** git
work tree, and a body **larger than the server can receive** — each waived by
the matching flag, and all three skipped on the routes that never reach the
server. [Exit code 1](#exit-code-1) maps each refusal to its flag.

⚠️ **A resubmit's store-listing media — icon, cover, screenshots — carry
forward on APPROVAL only — withdrawing the submission, or a moderator rejecting
it, deletes the listing and everything on it.** `civitai app withdraw` on a
**first-version** submission is therefore destructive, which is why it asks
first and needs `--yes` in a script.

📖 **The two kinds of credential, what `civitai whoami` reports (including the
tri-state capability rows) and `whoami --json`:
[CLI credentials and scopes](https://developer.civitai.com/site/guide/cli-auth).**
**What goes in the bundle — how big a bundle can be, what the packager left out,
which dotenv files ship, what looks like a credential, and the dirty-work-tree
guard: [What goes in the bundle](https://developer.civitai.com/apps/guide/packaging).**
**After you submit — the lifecycle, tracking a submission, build provenance, and
why deployed is not the same as listed:
[Review, approval and deploy](https://developer.civitai.com/apps/guide/review-and-deploy).**
Linking your source repository, and every other listing command:
[Your store listing](https://developer.civitai.com/apps/guide/store-listing#link-your-source-code).
Offline: `civitai login --help`, `civitai app submit --help`.

### What `civitai whoami` reports

`civitai whoami` prints the authenticated user, then a **Credential** section
(the credential type) and a **Capabilities** section of three verdicts. That is
the WHOLE of stdout for a full-scope personal key:

```
Logged in as zach (id 1) at https://civitai.com

Credential:
  Type:                     personal API key

Capabilities:
  Read Buzz balance:        yes
  Spend Buzz (AI Services): yes
  Submit Apps:              yes
```

🔴 **Submit Apps is a tri-state — `yes` / `no` / `unknown` — and `unknown` is
not `no`.** When the server reports no scope mask the two **Buzz** rows are
omitted rather than printed as `no`, because the Submit Apps row above them may
still be a known answer:

```
Logged in as zach (id 1) at https://civitai.com

Credential:
  Type:                     OAuth login

Capabilities:
  Submit Apps:              unknown
  (token scope not reported by the server — Buzz capabilities unknown)
```

🔴 **`whoami --json` is a stable, *curated* identity object — not the server's
raw `/api/v1/me` body.** It is a hand-built projection of fourteen keys, and the
two the server sends that never appear are **`email` and `emailVerified`**:
withholding them is the privacy property, not an omission
([#377](https://github.com/civitai/cli/issues/377)).

```json
{
  "base_url": "https://civitai.com",
  "canReadBalance": true,
  "canSpend": false,
  "canSubmitApps": true,
  "capabilities": { "can_read_buzz": true, "can_spend_buzz": false },
  "credentialType": "personal API key",
  "id": 1,
  "isMember": true,
  "scopes": ["UserRead", "BuzzRead"],
  "scopesKnown": true,
  "status": "active",
  "subscriptions": ["yellow"],
  "tier": "silver",
  "username": "zach"
}
```

📖 **Which credential produces which verdict, what each of those keys means,
`scopesKnown`, and the four profile fields that are `null` — never `""` /
`false` / `[]` — when the server did not report them: [CLI credentials and
scopes](https://developer.civitai.com/site/guide/cli-auth).**

#### Which dotenv files end up in the bundle

"The CLI excludes dotenv files" is the natural reading, and it is **not** what
the packager does. The rule is a **three-name allow-list with a catch-all**:

> **Every file whose base name starts with `.env` is excluded — except
> `.env.example`, `.env.sample` and `.env.production` sitting at the project
> root, which are included.** A file whose name *ends* in `.env` — `db.env`,
> `prod.env` — is dropped too, at any depth.

So `.env`, `.env.local`, `.env.*.local`, `.env.development` and `.env.test` are
excluded, and so is **every other `.env*` name the allow-list does not name**,
dotted or not: `.env.staging`, `.env.ci`, `.env.example.bak`, `.env-local`, and
`.envrc` (the direnv convention, which routinely holds exported credentials).
`.env.production.local` goes with them — `.local` is the dev-local override
convention and falls to the catch-all — so learning that `.env.production`
ships tells you nothing about its `.local` override.

**Directories are a separate, narrower rule.** A *directory* named `.env` or
beginning with `.env.` — `.env.d/`, `.env.local/` — is excluded whole at any
depth, and so is one whose name ends in `.zip`. The rule deliberately stops at
the dot, because dropping a subtree is a silent loss: `.envrc/`, `.env-backup/`,
`.envs/`, `.environment/` and `.envoy/` all **ship** as directories. The *file*
rules still reach inside them, so a `.env.production` or a `db.env` living there
is still dropped.

🔴 **The allow-list is by FILE NAME, and nothing reads the contents.** Whatever
you put in `.env.example`, `.env.sample` or `.env.production` is packaged and
uploaded verbatim — a `VITE_`-prefixed value (Vite inlines those into the client
bundle, so they are public the moment your app loads) and a plain unprefixed one
alike. Put nothing in those three files you would not paste into a public page.
`app submit` *warns* when a packaged file looks like it holds a credential, but
that is a heuristic printed after the fact: it never drops a file, and a silent
run is not a statement that the file is clean.

📖 **The rest of the rule — the per-file table, the separate (narrower)
directory rule, the deliberate case-sensitivity split, what still slips through,
what the `*.env` rule costs a Babylon.js `.env` texture, and what the root scope
costs a build that does not run from the project root: [What goes in the
bundle](https://developer.civitai.com/apps/guide/packaging#which-dotenv-files-end-up-in-the-bundle).**

### Link your source code (`app listing set-source-repo`)

An app's store **detail** page can carry a `Source` row linking to the code.
**Where you set it depends on the app's kind, and the two are not
interchangeable.** An **on-site** app takes it from the `repository` key in
`block.manifest.json`, re-synced from the manifest on every approved version —
so `set-source-repo` **refuses an on-site app** (exit `1`) and names that key
instead, because a write here would be re-synced away. An **off-site** app takes
it from the listing:

```bash
civitai app listing set-source-repo https://github.com/me/my-app
civitai app listing set-source-repo --clear          # remove the link
```

🔴 **On an approved listing this is a *material* change, unlike `set-text`.**
The server stages it on a revision and the listing re-enters moderator review,
so **the live listing is unchanged until `civitai app listing submit-revision`
is approved**. `--json` carries the branch the server took as `requiresReview`
and `shadowId`.

**Some states are refused outright rather than staged, and they do NOT share an
exit code.** Read the code, not the word "refused":

| state | exit |
|---|---|
| you unpublished the listing yourself — a material change is blocked while it is down | `2` |
| a moderator removed the listing | `3` |
| the platform has not yet applied the migration that adds the listing's source-repo column | `1` |

📖 **What URLs the server accepts and why the CLI does not pre-validate them,
what counts as a "change", and what happens when a revision was already open:
[Your store
listing](https://developer.civitai.com/apps/guide/store-listing#link-your-source-code).**

## Submission status

`civitai app status` checks where **your own** submissions are in that lifecycle
without leaving the terminal. It calls the token-authenticated, self-scoped route
`GET /api/v1/blocks/submissions` with your stored credential, so you only ever
see your own. With no argument it lists every submission, newest first; pass a
`blockId` (app slug) or `--id <pubreq_id>` to see one in detail — including the
**rejection reason** if it was rejected, and the live URL once it is approved and
deployed. Run from inside an app checkout it also warns on **stderr** when your
local `block.manifest.json` is **BEHIND** your highest **approved** version of
that app, because approving an older version replaces newer code on deploy.

When `civitai app submit` refuses because the manifest version is not strictly
above the highest **approved** version, its second line names which of four
cases you are in — lower-vs-live, lower-vs-approved-not-live, same-vs-live, or
same-vs-approved-not-live — and each says only what is actually known.
`--allow-downgrade` submits anyway. **"Live" is the server's own answer, not the
deploy state:** the check reads the `liveUrl` the submissions route returns and
consults `deployState` only as a fallback, because a **legacy approval** that
predates deploy-state tracking is serving but has no `deployState`.

📖 **[Review & deploy guide](https://developer.civitai.com/apps/guide/review-and-deploy)**
— the listing and detail layouts, the server's 100-row cap and why `--limit` is a
display limit rather than a page size, build provenance (`sourceCommit` /
`sourceDirty`, and why it is a CLAIM the server stores unverified rather than a
proof), the repo-behind warning in full, and why `civitai app status` can say
`approved / live` while `civitai app view` truthfully answers **not found**.
## Listing doctor (`app doctor`)

`civitai app doctor` answers one question — **is this listing ready to
publish, and if not, what do I do about it?** — for every App you own or hold an
accepted collaborator seat on, or for just the one you name:

```bash
civitai app doctor                 # every app you can work on
civitai app doctor my-app          # just one
civitai app doctor --json | jq -e .ok
```

The findings are the **platform's**, not the CLI's, and arrive already classified
into **blocking** and **advisory**. 🔴 **It exits `1` when a blocking problem sits
on a listing that can still publish, and `0` otherwise** — including when every
blocking problem is on a **delisted** listing, and when you have no listings at
all — so `civitai app doctor my-app || exit 1` gates a release script. It is a
**pure read**: it reports, and changes nothing.

📖 **[Store listing guide](https://developer.civitai.com/apps/guide/store-listing)**
— every finding code and the fix `doctor` prints for it, how a delisted listing
is reported without gating, and the `--json` shape (`ok` follows
`summary.gating`, **not** `summary.blocking`; a script asking "is anything wrong
anywhere" reads the latter).

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

⚠ **The clone URL embeds your access token, and a fresh clone persists it into
`.git/config`.** The endpoint lazily provisions a scoped, read-only Forgejo
identity and returns a URL with a pull token in it.

The repo only exists once your **first version has been submitted as a ZIP and
approved**; before then the command says so — `app <slug> has no approved
version yet …` — naming the latest submission's state and the next step for it:
where it is in review, or, for a **rejected** or **withdrawn** submission, that
nothing is in review and a new `civitai app submit` is what moves it. Exit `4`.

That better message needs a submission the CLI can *see*, so **`no such app for
your account` is what you get whenever the CLI cannot prove otherwise** — no
submission matches the slug, **or** the lookup itself failed, **or** a version
*is* approved, **or** you passed an `appBlockId`. `civitai app status` settles
it. `git` must be on `PATH`; that check runs only after the server has answered.

📖 Flags and the full failure matrix: `civitai app pull --help`.
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
  [deployed is not the same as listed in the store](https://developer.civitai.com/apps/guide/review-and-deploy).

## App metrics

`civitai app metrics <slug>` shows the owner-only analytics for one of **your**
App Blocks: installs, runs + Buzz spent, Buzz purchased, App loads and API
engagement. It needs the **Apps submit scope** — the same bit `app submit` and
`app status` require, so one `civitai login` serves all three; an OAuth token
minted **before** that scope existed still gets a `403`, and a full-scope
personal API key also works.

```bash
civitai app metrics gen-matrix --from 2026-05-01 --to 2026-08-03
civitai app metrics gen-matrix --json
```

The slug is resolved to its `appBlockId` through your own submissions, so
analytics exist only once a version has been **approved** — an app still in
review reports that instead of an empty dashboard, and the message names the
next step **for** the latest submission's own state: where it is in review, or
— for a **rejected** or **withdrawn** submission — that nothing is in review and
a new `civitai app submit` is what moves it. Exit `1`, not `4`: the slug is
right and the app exists, only its analytics do not yet.

**The window is always printed, and it comes from the server.** The API defaults
to the last **30 days** and clamps any request to **366 days**, so a real app
with 20 runs in mid-June reads `0` under the default window. `--from` / `--to`
take a plain `YYYY-MM-DD` (midnight UTC) or a full RFC3339 timestamp; a
malformed value or an inverted window is a usage error (exit `2`) caught before
any request.

🔴 **Three flags mean "this number was never measured", and none of them is a
zero. A script must branch on each rather than trusting the counts** — all three
pass through `--json` at exit `0`:

- **`notOwned`** — the caller does not own the app (or lacks Apps-author
  access). The API answers **HTTP 200 with every counter zeroed**, flagged only
  by this field. The **human view refuses to render a dashboard** and names
  `civitai whoami` / `civitai app status <slug>` instead; **`--json` does not
  refuse**, so `… --json | jq .runs.count` returns `0` for an app you cannot see.
- **`installs.notApplicable`** — the app cannot be installed at all (a page app
  has no install slot), so an install record cannot exist. **Not an outage
  flag**: render it "not applicable" rather than retrying. Distinct from a real
  `0` on an installable app nobody has installed yet.
- **`views.unavailable`** — section-local and independent of `notOwned`. App
  loads is the one section read from a different store, which can be unreadable
  or merely too slow (the read is time-bounded server-side) while every other
  counter in the same response is genuinely measured. The human view prints
  `unavailable`. **A server old enough to predate the section omits the `views`
  key entirely** — treat a missing `.views` the same way.

**Engagement counts only authenticated, scope-gated API calls**, so an app that
ships no scoped API surface shows real installs and revenue beside a flat
engagement section. **App loads is the exception** — measured on every load, so
it sees signed-out visitors and static blocks engagement structurally cannot.
`Unique viewers` counts signed-in people once each and approximates signed-out
ones by network address, so read it as **reach, not an identity count**;
`Signed-out loads` counts LOADS, so one anonymous visitor reloading ten times is
`10` there and `1` unique viewer, and it can legitimately exceed `Unique
viewers`. These are mount **ATTEMPTS**: a load that FAILED still counts, because
a failed mount's only beacon is the same event and it carries no status.
`Error rate` is the share of API calls that failed; the server sends a `0`–`1`
ratio (which `--json` passes through unchanged) and the human view renders a
percentage. Only a genuine zero prints `0.0%` — a real but tiny rate reads
`<0.1%` rather than rounding away to look error-free.

📖 The raw endpoint/scope tokens the CLI deliberately does **not** humanise, and
the full `--json` payload including the per-bucket `series` arrays the human
view omits: `civitai app metrics --help`.
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

A few output behaviours worth knowing, all of them documented per command by
`--help`: `--base-model` is repeatable (an OR across the values) on both
`models search` and `images search`; `--meta` adds each image's generation
metadata as an indented detail block (`meta: (hidden by uploader)` where the
uploader hid it); `--sort` is **ignored** with `--model-id`, and the CLI prints
a one-line note on stderr saying so; and `models get` / `model-versions get`
tag a version whose primary file is **not** model weights with its actual type
(`[Archive]`, `[Training Data]`, `[Other]`). `--json` is an unchanged raw
passthrough in every case.

📖 Field-by-field REST reference:
[site/reference](https://developer.civitai.com/site/reference/). Paging in
depth: [Pagination](https://developer.civitai.com/site/guide/pagination).
Worked read recipes: [the CLI
guide](https://developer.civitai.com/site/guide/cli#read-commands).
## Download model files

`civitai download` fetches the file(s) of a model **version**. Identify the
version deterministically by its numeric **version id**, or resolve a model's
default (first) published version with `--model`:

```bash
civitai download 691639                       # the version's primary file → ./<server-name>
civitai download --model 4384                 # resolve model 4384's default version, then download its primary file
civitai download --model 4384 --dry-run       # print the plan (files, sizes, hashes, targets) — download nothing
civitai download 290640 --file vae --out-dir ./models   # pick a file by name; write into a dir
civitai download 290640 --all --layout comfyui --root ~/ComfyUI   # route each file to its type folder
civitai download 691639 --layout a1111 --for-base "SDXL 1.0"      # A1111 layout + base-model compat warning
```

> **Downloads require authentication.** Every model-file download needs a token —
> even a small public embedding 401s anonymously. Run `civitai login` first. The
> read/search commands work anonymously; downloads do not. `--anon` is meaningful
> for the read commands, not for `download`.

The essentials:

- **`--file` matches by substring**, so an ambiguous value is refused (exit `2`)
  naming what it matched; an exact same-name collision is a different message.
- **Integrity is verified by default.** A SHA256 mismatch deletes the partial
  file and fails — that is the check working. `--no-verify` opts out.
- 🔴 **The file name is the *uploader's* string.** Invisible,
  terminal-controlling, newline and tab characters are removed before it is
  printed, and the **progress line** is additionally cut at 120 characters,
  because a name long enough to **wrap** could otherwise strand a forged
  `Saved … (SHA256 verified)` at column zero. The `Saved …` line and the
  download plan print the full (sanitised) name.
- **`--layout comfyui|a1111` + `--root`** route each file into the right folder
  for that runtime; `--for-base` adds a compatibility warning, never a refusal.

📖 File selection, folder routing, the compatibility check and integrity
verification, a paragraph at a time:
[the CLI guide](https://developer.civitai.com/site/guide/cli#download).
Every flag, offline: `civitai download --help`.
## Generate

`civitai generate "<prompt>"` runs a text-to-image generation on Civitai's
generator.

> 🔴 **This spends real Buzz and cannot be undone.** A submitted generation is
> charged the moment the orchestrator accepts it, and nothing local calls that
> back — not `--timeout`, not Ctrl-C, not `civitai workflows cancel`. Price it
> with `--dry-run` first; that calls the cost estimator and spends nothing.

> 🔴 **`--max-cost` is an estimate check, not a spending cap.** It compares the
> server's estimate against your number and refuses **locally**, before
> submitting. It never reaches the server, the realized charge can exceed the
> estimate, and no server-side spending ceiling is reachable from an API key.
> Do not run an unattended loop believing it caps spend.

> 🔴 **The CLI states no rule about what becomes of a charge, in either
> direction.** Your account Buzz ledger is not readable from here (`civitai
> buzz` reports a balance, not a history) — see your
> [transaction history](https://civitai.com/user/transactions). What the CLI
> *does* report is one workflow's **own** transactions when the server hands
> them over: `civitai workflows get <id>` prints each `debit` and `credit`
> recorded for that workflow, and the net.

> 🔴 **A checkpoint does not carry its ecosystem, and a bad pairing is refused
> by nothing** — not this CLI, not the estimator, not the generator. If you name
> `--checkpoint`, name the `--ecosystem` it belongs to as well. A mismatch has
> been measured to charge, run, and finish with zero deliverable outputs.

> 🔴 **The server may silently substitute a different checkpoint** and bill for
> what actually ran. Warned by default; `--fail-on-substitution` refuses on the
> estimate, before any spend.

```bash
# Price it. Spends nothing.
civitai generate "a cat wearing sunglasses" --dry-run

# Generate, refusing if the estimate exceeds 50 Buzz, and wait for the images
civitai generate "a cat" --quantity 4 --max-cost 50 --yes --out-dir ./out

# Fire and forget; collect the results later
civitai generate "a cat" --yes --no-wait
civitai workflows get <workflow-id>
```

**Credential.** Generation needs the AI Services scopes: a full-scope
**personal API key** ([create one](https://civitai.com/user/account), then
`civitai login --token <key>`), or `civitai login --scopes generate`. A
**default** OAuth browser login does **not** carry them and is refused.

The `--dry-run` transcript is the easiest way to see a substitution — here an
SD 1.5 checkpoint sent to a Flux ecosystem:

```console
$ civitai generate "a cat" --ecosystem Flux1Kontext --checkpoint 128713 --dry-run
⚠ The server will NOT use the checkpoint you asked for. It has substituted a different model, and this estimate prices the SUBSTITUTE. Nothing has been submitted or charged yet.
    requested version 128713 -> will run version 1892509  (reason: unrecognized)
      the server does not offer that version in this model family at all — it may be a community checkpoint that was never offered for generation, or a version retired since this command was written. Check it with `civitai model-versions get <id>` and pin a version that is still offered
To refuse a run like this instead of being told about it, pass --fail-on-substitution.
```

Two flags decide what happens after the job is accepted:

- `--no-download` waits and prints the output URLs instead of writing files. One
  row on **stdout** per output that came back **with a URL**, the CLI's own
  number and the URL separated by a tab, so `cut -f2` is the intended read. The
  number is that output's position among the run's kept outputs, so it skips an
  output the server returned without one — a gap in the numbering is the CLI's
  own, not a dropped row. The URL is the **server's**, so it is flattened to one
  line and one tab-separated field first: a presigned URL cannot add a numbered
  row of its own, or an extra field, to that listing.
- `--timeout` bounds how long the CLI waits, and defaults to **30m**. It stops
  the WAIT, never the job: the run continues and stays charged, and
  `civitai workflows get <id>` re-attaches to it.

📖 **The full guide — the shape of a session, confirmation, the content flags,
waiting/downloading/re-attaching and `generate`'s exit codes:
[Generating images from the CLI](https://developer.civitai.com/site/guide/cli-generate).**
Model selection, ecosystems, substitution and image-to-image:
[Choosing a model](https://developer.civitai.com/site/guide/cli-generate-models).
Raw graphs (`--print-input` / `--input`):
[Raw generation graphs](https://developer.civitai.com/site/guide/cli-generation-graphs).
Listing, cancelling, server failure reasons and a workflow's Buzz transactions:
[Tracking and cancelling generations](https://developer.civitai.com/site/guide/cli-workflows).
Every flag, offline: `civitai generate --help`.
## Scripting with `--json`

Every read subcommand takes `--json`, which prints the `/api/v1/...` REST
response **as the API shaped it** — not a CLI-invented shape. So the field
schema is exactly the public Site API's; keep the
[REST field reference](https://developer.civitai.com/site/reference/) open
rather than reverse-engineering fields with `jq keys`.

Two properties make the output safe to pipe:

- **`--json` stdout is pure JSON** — nothing else is written to stdout, so
  `... --json | jq -e .` always parses.
- **Errors go to stderr with a non-zero exit** — a failed call writes the error
  to stderr, exits non-zero, and prints **nothing** to stdout, so `jq` never
  sees error prose.

🔴 **It passes through the DOCUMENT, not the BYTES — do not diff or hash
`--json` output against the wire.** The output is re-indented, and a body that
will not parse is **repaired** first (a raw control byte is rewritten as its
JSON escape). The same strings and characters come out; the bytes do not.

One read is not a pure read, and it is the exception a script must know:

- 🔴 **`listing status` is NOT a pure read — `--json` or not — so do not poll it in a loop.** On a LIVE (approved) listing the read behind it (`getMyListingForEdit`) opens the revision draft server-side, idempotently — the same one each time — so a script calling it repeatedly keeps a revision open on your listing. [#389](https://github.com/civitai/cli/issues/389) asked the narrower question of whether a call that FAILS could leave a revision behind; it cannot, because every refusal the server can return here is raised before the revision is opened. **That settles the error path only — the write on success is unchanged.** Reading it once per change is fine; a watch loop is a writer. ⚠ **This now applies to OFFSITE apps too.** Before [#422](https://github.com/civitai/cli/issues/422) every `app listing` subcommand refused for an offsite app and therefore could not write anything; now that they resolve by slug, `app listing status` against an **approved** offsite listing mints that shadow revision like any other. Every offsite app measured on civitai.com is approved, so this is the *normal* state there, not an edge case. #422 retired the "cannot be addressed" refusal, and #389 retired nothing about the write — the shadow is a property of `getMyListingForEdit`, not of how the listing id was obtained.

📖 **The read-path recipes** — the `--cursor` deep-paging loop, silencing the
update nag for clean pipeline output, the SHA256-case and embedded
`modelVersions` gotchas, and a worked search→download example — are in the
[CLI guide](https://developer.civitai.com/site/guide/cli#scripting-with-json).
**The byte semantics in full, the two `app` commands that compose their own
shape, and generation `--json` (whose payload is the orchestrator's, not a REST
shape, and where the repair never runs):
[Scripting the CLI with `--json`](https://developer.civitai.com/site/guide/cli-json).**
## Upgrading

`civitai upgrade` replaces the running binary with the latest GitHub release:

```bash
civitai upgrade           # no-op (and says so) when already current
civitai upgrade --force   # reinstall anyway
```

The release is resolved from the **public** GitHub releases API — no token is
ever sent — and the downloaded archive (a **`.zip`** on Windows, a **`.tar.gz`**
everywhere else) is verified against its SHA-256 entry in the release's
`checksums.txt` before anything is replaced. A mismatch, an archive that does not
open, an archive whose binary is larger than the 64 MiB this upgrader will read,
or a release carrying no `checksums.txt` at all, **aborts and leaves the current
binary untouched**: it will not upgrade without integrity verification.

If this binary came from Homebrew, `upgrade` does not self-replace it — it tells
you to run `brew upgrade civitai/tap/civitai`, so the package manager keeps
owning the file. `--force` overrides that and self-replaces anyway. That path is
**macOS-only**, because [the tap is macOS-only](#homebrew-macos).

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
| `--no-color` | Disable all colour and styling. Also via `NO_COLOR` (**any** non-empty value) or `CIVITAI_NO_COLOR` (**a boolean value only** — `1`/`true`/…; see below). |
| `--color` | Force colour **even when stdout is not a TTY**. Also via `CLICOLOR_FORCE` (any non-empty value other than `0`) or `CIVITAI_COLOR` (**a boolean value only**). |
| `--no-update-check` | Skip the background check for a newer release. Also via `CIVITAI_NO_UPDATE_CHECK`. |

**The colour contract, for pipelines.** Colour is **off by default whenever
stdout is not a TTY**, so a redirected or piped run already emits plain text
with no escape sequences — you do not have to ask for anything. When you do want
to override that, the precedence is fixed, highest first:

1. `--no-color` / `NO_COLOR` / `CIVITAI_NO_COLOR` → **off**
2. `--color` / `CLICOLOR_FORCE` / `CIVITAI_COLOR` → **on**
3. otherwise: on if stdout is a TTY, off if it is not

Off always beats on, so a `NO_COLOR` in the environment cannot be re-enabled by
a `--color` further down a pipeline.

🔴 **The `CIVITAI_*` pair is NOT interchangeable with the standard pair — it
parses its value, and silently ignores anything it cannot parse.** `NO_COLOR`
follows [no-color.org](https://no-color.org): *present and non-empty* is what
counts, **not** the value, so even `NO_COLOR=0` disables colour.
`CLICOLOR_FORCE` counts when present, non-empty and **not** `0`. But
`CIVITAI_NO_COLOR` and `CIVITAI_COLOR` are read as **booleans**, so only twelve
spellings mean anything — `1`, `t`, `T`, `TRUE`, `true`, `True` (on) and `0`,
`f`, `F`, `FALSE`, `false`, `False` (off). Anything else — `yes`, `on`, `y`,
`enabled`, `2`, an empty string — parses as **false** and does nothing at all,
with no warning: `CIVITAI_NO_COLOR=yes` does **not** disable colour,
`CIVITAI_NO_COLOR=1` does, and an explicit `CIVITAI_NO_COLOR=0` is a real
*false* that leaves colour alone (unlike `NO_COLOR=0`). **When in doubt use
`1`, or the plain `NO_COLOR` spelling.**

🔴 **`--json` output is never styled**, at any of those settings. It is written
without passing through the presentation layer at all, so `--json` is always
safe to pipe into `jq` regardless of how colour is configured or whether a TTY
is attached.

### What a table cell can contain

Almost every value the CLI prints in a human table — a model name, a username, a
tag, a workflow status, a submission's block id — is **text a stranger uploaded**
or that a server chose. The human renderers put all of it through one gate before
it reaches your terminal, and this is what that gate promises:

- **Terminal escapes are removed.** Cursor moves, line clears, OSC sequences and
  the invisible / direction-reversing characters are stripped from the
  server-supplied strings the human renderers print, so a hostile value cannot
  overwrite a line the CLI already printed or reorder what you read. (What the CLI
  prints from what **you** typed — a prompt, a path, a flag value — is echoed
  byte-for-byte and is deliberately *not* rewritten.)
- **A table cell is one line, and one column.** Every **server-supplied** value
  that reaches a cell of a rendered table — `models search`, `images search`, `app status`,
  `workflows list`, the pre-spend cost table, and the rest — has any newline or
  tab in it replaced by a **space**. A newline would otherwise start a line at
  column zero, where it is indistinguishable from a row the CLI wrote; a tab is
  the column separator, so it would add an extra, perfectly aligned column. Both
  read as real output, which is why the value is flattened rather than trusted.
- **`label: value` lines are a narrower promise.** The single-line metadata fields
  — `images … --meta`'s model / sampler / seed / resources, `app status --id`'s
  live URL and block id, `app listing status`'s screenshot ids and captions, the
  `--no-wait` re-attach hint, and `generate`'s wait-path lines (the submit
  receipt, the status line the poll prints or redraws, the server's own message
  when a status check fails and is retried, and the re-attach block printed when
  a wait ends without a result) — are flattened the same way. Other one-off detail
  lines outside a table (for example the header block above `models get`'s version
  table, `collections get`, `app view`) have their escapes stripped but **may still
  carry a newline**, so a hostile value there can start a line at column zero.
  Telling a genuinely single-line field from legitimately multi-line free text is
  a per-field judgement. The list above is **illustrative, not exhaustive** — more
  fields are flattened than it names (`images … --meta`'s cfg, steps and url among
  them). Treat it as "these definitely are", never as "only these are".
- **Genuinely multi-line server text keeps its line breaks**, and is indented under
  the line that introduced it, so a continuation can never sit at column zero.
  There are five such surfaces: the generation prompt and negative prompt
  (`images … --meta`), the orchestrator's failure reason (`workflows get` and
  `workflows list`), the same reason on `generate`'s **error** path, per-output
  exclusion reasons (`generate`), and the reviewer's rejection reason / approval
  notes (`app status --id`). If a field you expect to be multi-line arrives on one
  line, it was in a cell.
- **Two values are shortened, and the rest are not.** The download **progress
  line** and `generate`'s **`could not read your Buzz balance`** warning cut the
  server's text at **120 characters** and mark the cut with a `…`. Both sit
  directly above something the CLI itself asserts — a `Saved … (SHA256 verified)`
  line, and the `Cost: … Buzz` line you approve a spend on — and a value long
  enough to wrap lets the server write extra rows of your terminal that read as
  the CLI's own, with no invisible character involved. Nothing else is shortened:
  the `Saved …` line, the download plan and every table cell still print the
  value in full.
- **Known limits.** Shortening bounds **how many** rows a value can take, not
  whether one of them starts at column zero — the CLI never asks how wide your
  terminal is, so it cannot know where a line breaks — and the 120-character
  budget counts **characters, not screen columns**, so wide (CJK) text takes
  twice the space it accounts for. Every **other** long value is not shortened at
  all, so your terminal can still soft-wrap it to column zero, and one hostile
  value widens a column for every row. Only `workflows list` wraps its reason
  text to a fixed budget; the other multi-line surfaces do not — and that budget
  is a **fixed 79 columns**, never a question about how wide your terminal is.
  Wrapping collapses runs of whitespace and breaks a token longer than the line,
  but **no words are dropped**; in a *narrower* terminal, or with wide (CJK)
  characters, your terminal re-wraps and the overflow can still reach column
  zero. Generation prompts are the deliberate opposite: they are **not**
  soft-wrapped at all, because collapsing whitespace runs and splitting tokens
  would alter prompt weights and syntax, so in a narrow terminal the terminal's
  own soft-wrap is what reaches column zero there. `U+2028` / `U+2029` are passed
  through (no terminal is known to break lines on them).

🔴 **None of this applies to `--json`.** That output is emitted raw, because JSON
already escapes control characters and rewriting the bytes would corrupt what a
script parses. **A script that renders server strings from `--json` onto a
terminal has to do its own sanitising** — the guarantees above are about the
human output only.

## Configuration

| Setting | Config key | Env var | Default |
| --- | --- | --- | --- |
| Personal API key | `token` | `CIVITAI_TOKEN` | — |
| OAuth tokens (device login) | `auth_kind`, `access_token`, `refresh_token`, `token_expiry`, `scope` | — | — |
| API base URL | `base_url` | `CIVITAI_BASE_URL` | `https://civitai.com` |
| Submit endpoint | — | `CIVITAI_SUBMIT_PATH` | `/api/v1/blocks/submit-version` |
| Skip the update check | — | `CIVITAI_NO_UPDATE_CHECK` | unset (the check runs) |
| dev-tunnel SSH endpoint | — | `CIVITAI_DEV_TUNNEL_ENDPOINT` | `sish.civitai.com:2224` |
| Disable colour | — | `NO_COLOR` (any non-empty value), `CIVITAI_NO_COLOR` (**boolean only**: `1`/`true`/…) | unset |
| Force colour | — | `CLICOLOR_FORCE` (non-empty, not `0`), `CIVITAI_COLOR` (**boolean only**: `1`/`true`/…) | unset |
| ⚠️ dev-tunnel channel debug log — **debug only, not supported surface** | — | `CIVITAI_DEVTUNNEL_DEBUG` | unset (no debug output) |

🔴 **Precedence is decided per setting; no one rule covers all four flags.** Only
the first of them is a plain flag-beats-environment override:

- **`--tunnel-endpoint`** (`app dev-tunnel`) wins —
  `CIVITAI_DEV_TUNNEL_ENDPOINT` is read only when the flag is empty.
- **`--token`** is no override at all: it is a `civitai login` flag that
  **writes** the key into the config file, and `CIVITAI_TOKEN` then beats that
  file on every command — including the key `login --token` just stored. For a
  one-off key, set `CIVITAI_TOKEN` for that invocation.
- **The colour flags** invert it — *off beats on*
  ([Global flags](#global-flags)), so `--color` **loses** to a `NO_COLOR` /
  `CIVITAI_NO_COLOR` in the environment.
- **`--no-update-check`** is OR'd with `CIVITAI_NO_UPDATE_CHECK`: either one
  disables the check, and no flag re-enables it once the variable is set.

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
| `6` | Rate limited — throttled by the API (HTTP 429). **Not every 429 lands here**: the deep-paging cap is a usage error and exits `2`. [Detail](#exit-code-6) |

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
- **When validation produces a result**, `civitai app validate --json` prints it in full and its `ok` field is the structured form of the same answer; a failure that produces no result at all — a project directory the CLI cannot **stat**, say, because it is unreadable or because a path component below it is not a directory — still exits `1` with **nothing on stdout**, so branch on the exit code before parsing. The full exit→stdout table is in [The `--json` result shape](https://developer.civitai.com/apps/guide/validate#the-json-result-shape).
- A resource that **exists but is not ready** lands here too, and deliberately not on `4`: `civitai app metrics <slug>` for an app whose submitted version is still in review exits `1`, because the slug is right and the app does exist — only its analytics do not exist yet, and the error names `civitai app status <slug>` as the next command. `4` stays reserved for a slug with no submissions at all, so the two remain separately actionable: fix the slug, versus wait for approval.
- A **version regression** lands here for the same reason: `civitai app submit` refuses when the manifest version is not strictly **above the highest approved version** of that app, because approving an older (or identical) version replaces the newer live deployment. Nothing about the invocation is wrong, so it is a verdict about the project, not a `2`. `--allow-downgrade` is the deliberate-rollback escape hatch, and the guard is skipped entirely by `--package-only` or a run with no token — neither reaches the server.
- A **dirty git work tree** lands here too: `civitai app submit` refuses while files that go into the bundle are uncommitted, because the bundle is packaged from what is on disk and approving one deploys code that exists in no commit. `--allow-dirty` submits the tree as it is. It **degrades rather than enforcing** — a directory that is not in a git repo, or a machine with no `git` on `PATH`, submits exactly as before (scaffolded apps have no repo, and that path must keep working), and a clean tree whose `HEAD` is on no remote **warns** instead of refusing. Like the version guard it is skipped by `--package-only` and by a run with no token.
- **A bundle the server cannot receive** lands here for the same reason, since [#585](https://github.com/civitai/cli/issues/585): `civitai app submit` refuses BEFORE uploading when the base64 JSON body would exceed what the platform accepts, so the transfer costs nothing. Nothing about the invocation is wrong — the project is too big — which is why it is `1` and not `2`, matching the version and dirty-tree refusals above. It previously reached `2` by way of the server answering `400: Invalid JSON`. `--allow-oversize` is the escape hatch, because the ceiling is a vendored number the CLI cannot re-measure; see AGENTS.md item 31.
- The **build-provenance stamp** those two guards now also collect (issue #411 — the commit `civitai app submit` reports and `civitai app status` shows) changes no exit code at all, in either direction. It is sent only when the CLI can establish a value in exactly the shape the server accepts (`^[0-9a-f]{40}$`); every branch that cannot — no repo, no `git` on `PATH`, a repo with no commits, or an answer it does not recognise — sends nothing and submits exactly as it did before. A submit that would have succeeded cannot fail because of it, and a missing stamp is never an error.
- **"Wait for approval" is the *pending* case only.** The same `1` covers an app whose latest submission was **rejected** or **withdrawn** — nothing is in review there, so `civitai app metrics <slug>` says so and names a new `civitai app submit` as the next step instead of a review to wait for. What separates `1` from `4` is unchanged: the slug is right and the app exists.

### Exit code 2

- Usage error — a bad flag, a **missing required flag or argument** (e.g. `civitai app withdraw` with no publish-request id), a bad flag **value** (`--limit` out of range, a non-integer id, `--template nope`), or a request the API rejected as malformed (HTTP 400, e.g. a bad `--period`/`--sort` enum).
- This does not depend on where the refusal happens: a mistake the CLI catches locally and one the server rejects both exit `2`. ⚠ **One exception, added by [#585](https://github.com/civitai/cli/issues/585):** a bundle too large to upload used to reach `2` via the server's `400: Invalid JSON`, and is now refused LOCALLY and exits `1` — a verdict about the project, like the other `app submit` refusals. A script branching on `2` for that case must branch on `1`.
- **A 429 can land here.** The API's deep-paging cap (`page*limit` past its fixed offset ceiling) arrives as HTTP 429, but it is permanent rather than transient — retrying the same page loops forever — so it is classified as a usage error and exits `2` rather than `6`. The fix is `--cursor` instead of `--page`. A genuine throttle still exits `6`; see [exit code 6](#exit-code-6).
- A local image the CLI refuses before uploading anything (`civitai app listing set-icon <file>`, `civitai generate --image`) exits `2` when the file is missing, empty, a directory, over the size cap, or not a PNG/JPEG/WebP — but a file that exists and cannot be **read** (permissions, an I/O error) is a filesystem failure rather than a mistake about the invocation, and exits `1`, not `2`.
- That split is not images-only and it is not flags-only — it holds for **a flag's value and a positional argument alike**, over the paths listed here: `civitai generate --input <file>` likewise exits `2` for a path that is not there or is a directory, and `1` when the file is there and the read fails.
- The project commands take a positional path and refuse it the same way: `civitai app validate <dir>` and `civitai app submit <dir>` exit `2` when the path does not exist **or is not a directory**, because both are mistakes about the invocation. A directory that **does** exist but holds no `block.manifest.json` is a validation verdict instead, and exits `1`.
- `app listing set-cover` and `app listing add-screenshot` take the same positional `<file>` and refuse it the same way. (The CLI has no `--file` image flag at all: the only `--file` is `civitai download --file`, which picks a file *inside* a model version.)
- **Paths outside that list are not covered, and mostly exit `1`.** `civitai app listing … --dir <missing>` exits `1` (it reports "no `block.manifest.json` found in …", the same way it does for a directory that is really there but holds no manifest), and so does `civitai app submit … --out <path under a directory that does not exist>`. Both are stated rather than promised: this is a ledger of the paths the split is published for, not a claim about every path in the CLI.
- A usage error emits **no JSON object**, in every mode. `civitai app validate /nope --json` therefore writes nothing to stdout and exits `2`; it used to print `{"ok": false, …}` and exit `1`, which reported a nonexistent path as a validation result. Scripts that parsed that object must branch on the exit code first.

### Exit code 3

- Authentication/authorization — login required, token invalid/expired, or the credential lacks the needed scope (HTTP 401/403, or no token configured).
- **Not every `403` here is about your credential.** `civitai app listing …` against a listing a **moderator removed** answers `403` and so exits `3`, and nothing about the account is wrong: no login, grant or scope changes it. The message says so rather than sending you to fix access that is already fine, and names asking a moderator to relist the listing as the next step. Look the message itself up under [Troubleshooting](#troubleshooting).
- **`civitai generate` refines this**: several of its failures are *not* credential problems but would otherwise land here or on `2`, so they exit `1` instead and a script never loops on `civitai login`. A **muted account or incomplete onboarding** arrives as a bare `403` that is byte-identical to a missing scope; **out of Buzz** and **generation disabled** arrive as `400` (the upstream 403 is re-thrown server-side as a tRPC `BAD_REQUEST`), which would otherwise read as "bad flags". See [Generate](https://developer.civitai.com/site/guide/cli-generate#exit-codes).

### Exit code 4

- Usually an HTTP 404, but not always: some lookups answer `200` with an empty result set instead (`civitai app status <slug>` for an unregistered slug, `civitai users get` for an unknown username), and those exit `4` too.
- The same question therefore exits the same way however the API happens to phrase the miss.
- **An app that EXISTS but is `offsite` exits `4` from `civitai app status <slug>`, and that is deliberate.** That command resolves through the app's block submission, which an offsite app never has, so the *resource it looks up* is genuinely absent even though the app is not. Only the message changes — where the CLI can tell, it says the app is offsite and names a next step that can work, instead of `civitai app submit`, which cannot. This is **not** the `has no approved App Block yet` case on `1` above, which exits `1` because the thing looked up (analytics) is expected to appear later; an offsite app's block submission never will.
- **`civitai app listing …` no longer exits `4` for an offsite app in the normal case** — it falls back to selecting the listing by slug and succeeds ([#422](https://github.com/civitai/cli/issues/422), needing `civitai/civitai#3989` server-side). It still exits `4` when that fallback *also* answers **not-found**: a Civitai without `#3989` (older or self-hosted), or an app with no listing row. A listing your account does not own is **not** in that set on a current server — the by-slug lookup resolves it and then refuses it `403`, so it exits `3`.
- **A lookup failure that is not a 404 keeps its own code, and that is not always `3` or `5`.** Neither of the two lookups is retried past a non-404 — a `403`, a `5xx` or a transport failure says nothing about whether the resource exists — so you get the failing lookup's own error and its own code. **Measured** end-to-end on both lookups (`cmd/civitai/app_listing_lookup_exitcode_test.go`): `401`/`403` → `3`, `429` → `6`, `502`/`503`/`504` and a transport failure → `5`, and a plain **`500` → `1`**, because that status carries no classification sentinel at all. Do not read "the API failed" as "exit `5`": the code to retry on is `5`, and a `500` is not it.
- **The offsite wording is best-effort, and the exit code is not.** Naming an app as offsite costs one extra lookup against the public store catalog (`GET /api/v1/apps/{slug}`), and that route answers only for a **published** store listing and is itself still behind a launch flag — until the catalog opens publicly you see an app there only as a moderator or app-dev-tester. So an offsite app whose listing is not published, a caller without that access, or any network/5xx failure of the lookup all keep the generic `no such submission … run civitai app submit first` message. **Exit `4` either way**, so a script branching on the code is unaffected; only the human-readable half degrades, and always in that direction.

### Exit code 5

- Network/transport failure or service unavailable — dial/timeout, or HTTP 502/503/504 after retries.
- **A 429 can land here too**, which no surface used to say: a throttle that carries `Retry-After` is retried, and when it persists through every attempt the failure is tagged as service-availability rather than rate-limiting. The 429 STATUS therefore reaches `2` (the deep-paging cap), `5` (a retried throttle that never cleared) or `6` (a throttle terminal on the first response). The MESSAGE does not: this case prints `Civitai returned HTTP 429 after N attempts — the service is temporarily unavailable, try again shortly`, not `rate limited (429)`, so do not look for the latter here — see [exit code 6](#exit-code-6).
- This is the code to **retry** on, so a **filesystem** failure never lands here however retryable its errno looks: a permissions or I/O problem does not fix itself, and a loop that sleeps and re-runs would never terminate. Those exit `1`.

### Exit code 6

- A **genuine throttle** — the API asking you to slow down — exits `6` **when it is terminal on the first response**. Retry with backoff.
- 🔴 **A throttle carrying `Retry-After` is RETRIED for you, and if it survives every attempt it exits `5`, not `6`** — and it prints a DIFFERENT message (`Civitai returned HTTP 429 after N attempts …`), so the `rate limited (429)` text you are reading about never appears. Measured: `rate limited (429)` reaches **`2` or `6`, never `5`**; the 429 STATUS reaches all three.
- 🔴 **THE HEADER IS CONSULTED BEFORE THE MESSAGE, so a cap-worded 429 that carries `Retry-After` exits `5`, NOT `2`.** Measured on a local server: body `You've requested too many pages …` plus `Retry-After: 1` exits `5` after 4 requests. That is a structurally doomed request landing on the code to RETRY on — the exact hazard the `2` reclassification exists to prevent. It is reachable only if the server ever attaches `Retry-After` to a cap 429, which `pkg/civitai/retry.go` assumes it does not; that assumption is vendored and has no local guard, so it is published here rather than relied on silently.
- 🔴 **A 429 that is really the deep-paging cap exits `2`, not `6`, and this row exists because the contract used to say otherwise.** The API caps `page*limit` at a fixed offset ceiling and phrases the refusal as a 429 ("too many pages", "use cursors instead"). That request is **structurally doomed**: retrying the same page loops forever. It is reclassified to `2` so a generic 429 backoff-and-retry loop does not spin on it — the remedy is `--cursor` instead of `--page`, which is a change to the invocation, which is what `2` means.
- The distinction is drawn from the server's own message, deliberately narrowly, so a real throttle is never misclassified as a usage error. The **visible message is unchanged** in both cases: `rate limited (429): … — for deep paging use --cursor instead of --page`. **Branch on the exit code, not the text.**

## Troubleshooting

**Look up the message you got.** Every row's left column is a fragment of a
string this CLI really prints, so searching this page for a few words of your
error should land you on the right row. The third column links the most relevant
section — for most rows that is the full explanation, but where the message
itself already names the remedy the row is deliberately terse and the link is
context rather than instructions.

### Credentials and access

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `no token configured` | Nothing is logged in. Run `civitai login` or set `CIVITAI_TOKEN` — the App **store** (`app list` / `app view`) is not an anonymous read either. | [Submit & auth](#submit--auth), [Browse the App store](#browse-the-app-store) |
| `forbidden (403)` | Usually the invite-only Apps beta rather than a broken token — the same account reads the public API fine. | [Submit & auth](#submit--auth) |
| `not permitted for your account (403)` | The **catch-all** listing `403`: managing a store listing needs Apps-author access, a narrower grant than submitting. The two rows below are the listing `403`s that are *not* about your grant. | [Store listing](https://developer.civitai.com/apps/guide/store-listing) |
| `under a moderator takedown (403)` | A moderator removed this **store listing**; **your account's access is not the problem** and no command reverses it — ask a moderator to relist it. Unpublishing it yourself is a different refusal: a *material* change, `400`, exit `2`. | [Exit code 3](#exit-code-3) |
| `belongs to another account (403)` | The listing is real and readable, but this account is **neither its owner nor an accepted collaborator** — **your access is not the problem**. There is no moderator bypass. Sign in as the owner (`civitai whoami` says who you are), accept a pending invite, or ask the owner. | [Exit code 3](#exit-code-3) |
| `Submit Apps:` | The `civitai whoami` capability row, and it is **tri-state**: **`unknown` is not `no`**, it is the CLI declining to answer. Re-run `civitai login` for a token whose scope the server reports. | [What `civitai whoami` reports](#what-civitai-whoami-reports) |
| `(token scope not reported by the server — Buzz capabilities unknown)` | The server reported no `tokenScope`, so the two **Buzz** rows are omitted rather than printed as `no`. **Submit Apps** above it is unaffected. | [What `civitai whoami` reports](#what-civitai-whoami-reports) |
| `not permitted to read this app's analytics (403)` | `app metrics` needs the **Apps submit scope**. Re-run `civitai login` if your token predates it; a full-scope personal API key also works. | [App metrics](#app-metrics) |
| `block lacks ai:write:budgeted scope` | Printed by your app at runtime under `dev:live`: the dev token was minted **without** `--spend`, and that scope is never requested implicitly, manifest or not. | [Local dev loop](#local-dev-loop-harness-mock-vs-live) |
| `the server can receive` | The submit body exceeds **10485760 bytes** and `app submit` refused **before uploading**, so it cost you nothing. Shrink the bundle, or pass `--allow-oversize` — the ceiling is vendored, not measured. | [What goes in the bundle](https://developer.civitai.com/apps/guide/packaging#how-big-can-a-bundle-be) |
| `insufficient Buzz` / `generation disabled` | Not credential problems, which is why they exit `1` rather than `3` — a script must not loop on `civitai login` for either. | [generate exit codes](https://developer.civitai.com/site/guide/cli-generate#exit-codes) |
| `rate limited (429)` | 🔴 **One message, TWO exit codes — branch on the code, never the text.** `2` for the deep-paging cap, which is structurally doomed (`--cursor`, not `--page`); `6` for a genuine throttle, which you retry. | [Exit codes](#exit-codes) |
| `Civitai returned HTTP` | **A retriable status that survived every read retry** — `502`/`503`/`504`, or a `429` carrying `Retry-After` — exiting **`5`** in every case. Read the number in the message to know which you hit. | [Exit codes](#exit-codes) |

### Scaffolding a project

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `cannot derive a slug from` / `cannot appear in a blockId` | Exit `2`. Pass `--slug`. | [The blockId](#the-blockid) |
| `is not valid UTF-8` | Exit `2`, and `--slug` does not rescue it: the refusal is about the **display name**, which is written into the manifest as you typed it. | [The blockId](#the-blockid) |
| `… and the limit is …` | Exit `2` — the derived blockId would exceed 40 characters. Pass `--slug`. | [The blockId](#the-blockid) |
| `refusing to overwrite. Scaffold somewhere else` | From `app create` and `app init` alike. Exit `1` — a verdict about the directory, not about your invocation. | [Templates](#templates) |

### Validating and submitting

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `… not found at project root …` | **`civitai app validate`** found no `block.manifest.json` in the directory you named — the finding reads `block.manifest.json not found at project root <dir>`, which the terminal wraps onto a second line for a long path (`--json` carries it as one `message` string). `app submit` prints it too, because it validates first. `app submit --skip-validate` never prints it, because it waives the validation that produces it — that run fails on the row below instead. The path itself was fine, which is why this exits `1` and not `2`. | [Exit code 1](#exit-code-1) |
| `is this an App project?` | The same cause, reported by a command that did not validate first: `civitai app listing …`, which has to work out *which* app you mean from the working directory, and `app submit --skip-validate`, which waived the check that produces the row above. `app validate` and a plain `app submit` never print it, because validation reports the row above first. Run `app listing` from the app directory, or name the app with `--slug` / `--dir`. | [Submit & auth](#submit--auth) |
| `the server rejected this store-listing lookup (400)` | A **read** was refused and nothing was changed — a listing resolve, a read-for-edit, an asset-scan poll, or `app doctor`'s enumeration, which carries no input at all and so names no value to fix. Exit `2`. | [Listing doctor](#listing-doctor-app-doctor) |
| `the server rejected the image-upload request (400)` | The **image** was refused while being ingested. **No listing was changed**: nothing is attached until `set-icon` / `set-cover` / `add-screenshot` runs. Read the server's own reason after the code. Exit `2`. | [Store listing](https://developer.civitai.com/apps/guide/store-listing#attaching-media) |
| `image upload PUT failed` | Storage refused the **bytes themselves** (e.g. `EntityTooLarge`), between minting the presigned URL and recording the row. No listing was changed, and it exits **`1`, not `2`** unlike the ingest steps above — a known inconsistency ([#388](https://github.com/civitai/cli/issues/388)). | [Store listing](https://developer.civitai.com/apps/guide/store-listing#attaching-media) |
| `the server rejected this store-listing change (400)` | The **listing** was refused and may have **partially applied** — check `civitai app listing status`. It names no value to fix because the seven routes it covers do not all carry one. Exit `2`, except for a staged change refused only by the publish floor, which reports `staged on an open revision` and exits `0`. | [Store listing](https://developer.civitai.com/apps/guide/store-listing#the-publish-floor) |
| `there is no open revision to submit` | Exit `1`. | [Store listing](https://developer.civitai.com/apps/guide/store-listing#editing-a-listing-that-is-already-live) |
| `this listing is not live` | Exit `1`. | [Store listing](https://developer.civitai.com/apps/guide/store-listing#editing-a-listing-that-is-already-live) |
| `pass a URL or --clear, not both` | Exit `2`, and nothing is sent. | [Link your source code](#link-your-source-code-app-listing-set-source-repo) |
| `nothing to do — pass a repository URL to set the link, or --clear` | `set-source-repo` with neither a URL nor `--clear`. The server would reject the empty patch too, but as a `400` costing a round trip and one of your ~30/hour listing edits. Exit `2`. | [Link your source code](#link-your-source-code-app-listing-set-source-repo) |
| `the source-repository URL is blank` | Exit `2`, and nothing is sent — there is no "set it to empty" state to reach. | [Link your source code](#link-your-source-code-app-listing-set-source-repo) |
| `source-repository link comes from the` | `set-source-repo` on an on-site app, whose link the platform re-syncs from `block.manifest.json` at **every** approved version. Set `repository` there and run `civitai app submit`. Exit `1`. | [Link your source code](#link-your-source-code-app-listing-set-source-repo) |
| `no such directory — pass the path to an App project root` | A **usage** error: exit `2`, and `--json` prints nothing at all. | [Exit codes](#exit-codes) |
| `is not a directory — pass the App project ROOT` | A **usage** error too: exit `2`, and `--json` prints nothing at all. | [Exit codes](#exit-codes) |
| `it did NOT check that the file is loaded` | The `BLOCK_READY` advisory on its **weak** tier: it could not resolve what your `index.html` loads, so it checked only that *some* file mentions the message. The lines after it say what it could not follow. | [The host handshake](#the-host-handshake-block_ready) |
| `nothing index.html loads reaches it` | The **strong** tier: the emitter is in your project but nothing the browser loads reaches it. Copying `civitai-host.js` in is only half the fix — it has to be referenced too. | [The host handshake](#the-host-handshake-block_ready) |
| `no lockfile is committed` / `is not a lockfile` | The platform build installs **strictly** from the committed lockfile, so a missing one, or a zero-byte one from `touch`, fails the build server-side. Generate it with the package manager. | [Validate](https://developer.civitai.com/apps/guide/validate#the-lockfile-rule) |
| `refusing to submit without --yes` | Exit `1`. `--package-only` and the no-token fallback never reach it. | [Command reference](#command-reference) |
| `What this CLI sent` / `What this CLI would have sent` / `largest entries in the bundle` | Not an error of its own: the CLI's account of the bundle, and the largest entries it was made of. `What this CLI **sent**` prints under any error the upload call reports once the request has gone out — **it does not claim to know why** — and not on a `401`/`403`/`429`. A failure that never reached the connection never prints the past tense: no usable credential, an unwritable config and a connection that never opened print neither block. `What this CLI **would have** sent` is the ceiling refusal alone — it sends nothing either, and says so — and that one is exact too: nothing was uploaded. A refusal that stops the submit before the upload step (no `--yes`, a dirty tree, the version guard, a validation failure) prints neither. | [What goes in the bundle](https://developer.civitai.com/apps/guide/packaging) |
| `Your repo may be behind what was last released` / `Resubmitting the version that is already live is almost always an accident` / `That version is approved but not live` | The **monotonic-version guard**: the manifest version is not strictly above the highest **approved** version, and approving an older or identical one supersedes the newer. `--allow-downgrade` submits anyway; the second line names which of four cases you are in. | [Exit code 1](#exit-code-1), [Submission status](#submission-status) |
| `from a dirty git work tree` / `that go into the bundle are not committed` | The **dirty-work-tree guard**: files that go into the bundle are uncommitted, so approving one deploys code that exists in no commit. It names the paths — commit them, or pass `--allow-dirty`. | [Exit code 1](#exit-code-1), [the dirty-work-tree guard](https://developer.civitai.com/apps/guide/packaging#the-dirty-work-tree-guard) |
| `look like they hold credentials` | A **warning**, not a refusal — the exit code is unchanged. A file the packager KEPT holds a line shaped like a credential, and a submitted bundle cannot be recalled. It prints `path:line` and the key name, never the value. | [What looks like a credential](https://developer.civitai.com/apps/guide/packaging#what-looks-like-a-credential) |
| `HEAD is on no remote` | A **warning**, not a refusal. The packaged tree is clean, but its commit exists only on this machine, so the deployed version traces back to nothing anyone can fetch. Push the branch. | [Command reference](#command-reference) |
| `refusing to withdraw without --yes` | A withdraw asked for confirmation and found no TTY; nothing was withdrawn. It gates because withdrawing a **first-version** submission deletes that app's store listing — icon, cover and every screenshot. 🔴 **BREAKING** for a scripted `civitai app withdraw <id>` that used to exit `0`. | [Review & deploy](https://developer.civitai.com/apps/guide/review-and-deploy) |

### Generating

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `could not read your Buzz balance` | A **warning**, not a refusal — the estimate and the confirmation went ahead without the balance check; `civitai buzz` shows the real balance. The reason after it is the **server's**, cut at 120 characters ([What a table cell can contain](#what-a-table-cell-can-contain)). | [Confirmation](https://developer.civitai.com/site/guide/cli-generate#confirmation) |
| `refusing to spend Buzz without --yes` | The same gate on the money path. `--dry-run` prices the job without spending anything. | [Confirmation](https://developer.civitai.com/site/guide/cli-generate#confirmation) |
| `--image requires --ecosystem` | Without an ecosystem the server never promotes the job to image-to-image: your images are silently dropped and you pay for a plain text-to-image run. Hence a refusal, not a warning. | [Image-to-image](https://developer.civitai.com/site/guide/cli-generate-models#image-to-image-image-and-ecosystem) |
| `interrupted while waiting` | **The generation is still running and has already been charged.** Ctrl-C stopped the wait, not the job. Re-attach with `civitai workflows get <id>`. | [Waiting and re-attaching](https://developer.civitai.com/site/guide/cli-generate#waiting-downloading-and-re-attaching) |
| `model substituted` | The server ran a **different checkpoint** than you asked for and billed for what ran. Warned by default; `--fail-on-substitution` refuses on the estimate, before any spend. | [Silent model substitution](https://developer.civitai.com/site/guide/cli-generate-models#silent-model-substitution) |
| `The server reported: …` | The server's own words, which the CLI neither interprets nor calls retryable; only invisible and direction-reversing characters are removed first (`--json` is unfiltered). Printed on the `generate` error and by `civitai workflows get`. | [What the server says went wrong](https://developer.civitai.com/site/guide/cli-workflows#what-the-server-says-went-wrong) |
| `An indented line under a row is what the server recorded` | The same record on `civitai workflows list`, wrapped but never abbreviated. The indent keeps server text out of the column a real row starts in, so a message cannot pose as a workflow of yours. | [What the server says went wrong](https://developer.civitai.com/site/guide/cli-workflows#what-the-server-says-went-wrong) |
| `prompt: …` / `negative: …` | Generation prompts in `civitai images search --meta` and `civitai images get`, indented so a server string cannot impersonate a CLI output header — and deliberately **not** soft-wrapped, because that would alter prompt weights and syntax. | [Command reference](#command-reference) |
| `the orchestrator often supplies no failure reason, so it may not say why` | The same failure with **no** account recorded — a real, measured case, not a CLI limitation. Neither `civitai workflows get <id>` nor `workflows list` will say why either. | [What the server says went wrong](https://developer.civitai.com/site/guide/cli-workflows#what-the-server-says-went-wrong) |

### Everything else

| You saw | What it means | Where to read more |
| --- | --- | --- |
| `has no approved App Block yet` | The slug is right and the app exists — its analytics do not, because no version is **approved** yet. The message names the next step for the latest submission's own state. Exit `1`, not `4`. | [App metrics](#app-metrics) |
| `no such app for your account` | The server did not recognise the app for your account. From `civitai app pull` it means only that the CLI could not prove the app is yours-but-unapproved. Settle it with `civitai app status`. | [Submission status](#submission-status) |
| `has no approved version yet` | `civitai app pull` clones a repository that exists only once a version has been **approved**. The app is real; the message names the latest submission's state. Exit `4`. | [Pull your app's repository](#pull-your-apps-repository-app-pull) |
| `no such submission` | Nothing has been submitted for that app yet — `civitai app submit` creates the submission **and** the draft store listing — or, with `--id`, no publish request carries that id. | [Submit & auth](#submit--auth) |
| `is an OFFSITE app` | The app exists and is **offsite** — a registered URL, not a block bundle — so it has no block submission to resolve through, and never will. Normal from `civitai app status`; the message names `civitai app view <slug>` instead. | [Exit code 4](#exit-code-4) |
| `is ambiguous — it matches` | Your `--file` value matched as a **substring**; an exact same-name collision is a different message. Exit `2`. | [Download model files](#download-model-files) |
| `SHA256 mismatch for` | A download's hash did not match, and the partial file was deleted. Retry — this is integrity checking working, not a bug. The file name is the **uploader's**, so it is sanitised and the progress line cut at 120 characters ([What a table cell can contain](#what-a-table-cell-can-contain)). | [Download model files](#download-model-files) |
| `checksum mismatch for` | The row above, during `civitai upgrade`. | [Upgrading](#upgrading) |
| ``git is required for `civitai app pull` `` | Exit `1`, reached only after the server has already answered. | [Pull your app's repository](#pull-your-apps-repository-app-pull) |
| `unexpected response from` | A public read endpoint answered **`200`** with a body this CLI could not decode — not your request, credential or network, which is why it exits `1`. Two causes are known and fixed ([#513](https://github.com/civitai/cli/issues/513), [#525](https://github.com/civitai/cli/issues/525)); a third means the body is a shape the SDK does not model — **please open an issue with the snippet**. | [Scripting with `--json`](#scripting-with---json) |

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
- **Layout / conventions / how to add a command / the CI job list / the release
  process:** see
  [`AGENTS.md`](https://github.com/civitai/cli/blob/main/AGENTS.md).
- **Contributing:** see
  [`CONTRIBUTING.md`](https://github.com/civitai/cli/blob/main/CONTRIBUTING.md).

## License

[Apache License 2.0](LICENSE).
