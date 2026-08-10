# Handoff: docs-shrink-and-gate-hardening — 2026-08-10

## Goal

Audit `AGENTS.md` / `CLAUDE.md` / `README.md` for accuracy and per-session token
cost, then fix what the audit found. The second half was not planned: chasing the
defects surfaced a recurring class — **a check whose green means nothing** — in the
release pipeline and in four of five required merge gates.

## State now

- **Branch:** `main`, clean, in sync with `origin/main` at `511502a`.
- **Open PRs: 0.** Nothing in flight, no agents running.
- **Release `v0.1.92` is live and consistent across all three channels**:
  GitHub Release `Latest`, npm `0.1.92`, Homebrew cask `0.1.92`.

### DONE this session (all merged, all verified by content on `main`)

| PR | commit | what |
|---|---|---|
| #287 | `f9dcc5a` | `make ci` ≠ CI; the real gate (authored outside this session) |
| #290 | `c6fb9e7` | evict 9 items → `claudedocs/decisions/` |
| #289 | `8ec3cb0` | 7 AGENTS.md accuracy defects + `layout_ledger_test.go` |
| #297 | `5cb45f0` | compression pass: −586 B, proved compression exhausted |
| #303 | `576e5d2` | 10 README accuracy defects + symptom index + guards |
| #305 | `bd3595f` | evict items 10, 19; per-item `agentsSplitBase`; 2 guard blind spots |
| #308 | `6e31b4b` | cask no longer jumps the draft; `tools/caskcheck` |
| #310 | `932c5e9` | evict items 9, 13, 17, 22, 25 |
| #311 | `ac29e09` | `make ci-shallow` |
| #313 | `aba6c5a` | daily cask check files an issue when it breaks |
| #314 | `dfdcc97` | exit codes split Summary/Detail (moved `--help` too) |
| #316 | `ce56cd9` | vendorHash bot: errexit killed a deliberate `nix build` failure |
| #317 | `5120764` | AGENTS.md numbered list → **trigger index** |
| #318 | `d9d1b01` | rescued 156 uncommitted handoff lines, redacted a live token |
| #332 | `ca9c4f4` | last two scheduled bots file their own issue |
| #335 | `7ae29a8` | caskcheck read a CDN variant; now commit-pinned |
| #337 | `511502a` | 4 required gates could pass vacuously — closed |

**Headline numbers:** `AGENTS.md` **186,776 → 25,370 bytes** (−86%, ~11,684 → ~6,778
tokens by real `cl100k` encode). 26 evidence files under `claudedocs/decisions/`,
every moved body byte-identical and sha256-pinned.

**Live bugs fixed:** `brew install civitai/tap/civitai` was 404ing for all users
(v0.1.91 draft + cask pushed at tag time). `bump-flake-vendorhash.yml` had been red
on every run since 2026-07-18 with nobody notified.

### Deploy / verify status — honest

- **Verified end-to-end:** the v0.1.92 release. Cask stayed at `0.1.91` while the
  release was a draft (the #308 fix proving itself in the exact scenario that broke
  yesterday); after publish, all three channels moved together and the asset returns
  200.
- **NOT verified — never executed:** the vendorHash bot's **drift path**
  (`create-pull-request` + `BUMP_SCAFFOLD_PINS_TOKEN`) — there is no drift today, so
  it has never run. Also `canonical-unreachable` in `revendor-canonical-schema.yml`,
  and every **schedule** trigger for #332's wiring (scheduled runs only use the
  default branch's copy, so all drills were `workflow_dispatch`).
  - Next real runs to watch: `bump-scaffold-pins` **07:17 UTC daily**,
    `revendor-canonical-schema` **07:37 UTC Mondays**. Both should be green with no
    issue filed.

## Open investigations — live diagnosis state

### The trigger index's core premise is untested: do agents actually open the evidence files?

- **Symptom + exact repro:** not a bug — an unvalidated assumption underpinning
  #317. `AGENTS.md`'s numbered list is now 28 one-line routing questions plus
  `→ evidence: claudedocs/decisions/NN-*.md` pointers. The 86% size reduction is
  justified *only if* an agent follows a matching trigger to the file.
- **Observed (with values):** a fresh `general-purpose` subagent was given three
  realistic refactors, each a documented hazard, with no mention of AGENTS.md, docs,
  or that they were traps:
  1. fold `DownloadPresigned` into `DownloadFile` behind `useAuth bool` (item 17 —
     credential leak)
  2. add local `--sampler` validation before submit (item 13 — anti-mirror)
  3. collapse `isTimeoutErr` to `errors.As(err, &netErr) && netErr.Timeout()`
     (item 24 — the exact #241/#244/#246 bug)

  **All three were refused, correctly.** It also caught that two of the three probes
  had false premises (there is no `--sampler` flag; `DownloadPresigned` is *one
  line*, so the "duplication" does not exist).

  It listed every file it opened, in order:
  `pkg/civitai/download.go`, `pkg/civitai/upload.go`, `internal/appapi/appblocks.go`,
  `pkg/civitai/transport_error.go`, `internal/cmd/generate.go`,
  `internal/genapi/graph.go`, `internal/appapi/submit_fs_not_timeout_test.go`,
  `internal/cmd/generate_input.go`.

  **Not one file under `claudedocs/decisions/`.** It reached every answer from doc
  comments at the call sites, and stated AGENTS.md was in its system context from
  the start (auto-loaded via `CLAUDE.md`), referencing items by number "only where
  the code itself pointed at them".
- **Ruled out:** "the guardrails are gone" — they are not; all three traps held.
  "Subagents don't get AGENTS.md" — they do, it confirmed this explicitly.
- **Leading hypothesis:** in-code doc comments are carrying the routing load for
  code-adjacent decisions, and the trigger index was not exercised. That is fine for
  *safety* (defence in depth, strongest layer nearest the code) but means the
  evidence files may be effectively unread — so the premise behind their extraction
  is unproven, not disproven.
- **Next probe:** run the same style of probe against an item whose rule has **no
  call-site comment** to lean on — a docs/process item rather than a code one, e.g.
  item 25 (listing dimension bounds stay prose, must not become a local check) or
  item 8 (raw analytics tokens, deliberate non-mirror). If the agent still gets it
  right, the trigger index is doing real work; if it gets it wrong, the extraction
  moved load-bearing knowledge out of reach and the affected items need their
  triggers strengthened or their theses restored inline.

### Is "a check whose green means nothing" a standing risk elsewhere?

- **Symptom + exact repro:** three of today's most consequential defects were the
  same shape, and **every one was found by accident, none by a test**.
- **Observed (with values):**
  - `go test -run 'TestNoSuchName' ./internal/scaffold/` → `ok … [no tests to run]`,
    **rc=0**. `ci.yml`'s `ready-ack-runtime` — a **required** context, and per
    AGENTS.md item 11 the *only* runtime proof the scaffolded `BLOCK_READY` ack
    fires — was a bare `go test -run <name>` with no control.
  - `pins-vs-published` had no `-count=1`. Measured on this repo:
    run 1 `ok … 0.468s`, run 2 `ok … (cached)`. `setup-go`'s `cache: true` restores
    GOCACHE across CI runs, so a required **network** guard can report green having
    contacted npm **zero** times, replaying a prior `--- PASS:` line. The PASS-line
    control cannot see this; `-count=1` cannot see a rename. Both are needed.
  - `go test -v` on an empty match prints a bare `PASS`, so any control keyed on the
    word "PASS" is satisfied by a bystander.
  - `bump-flake-vendorhash.yml`: 8 runs, **0 successes**, 2026-07-18 → 2026-08-10.
    Root cause: GitHub runs `run:` blocks as `bash -e {0}`; `set -uo pipefail` does
    **not** clear that errexit, so `GOT="$(nix build … | grep …)"` — where the build
    is *supposed* to fail — died at the assignment, discarding a correctly parsed
    hash. All nix output was inside the command substitution, hence three weeks of
    empty logs.
- **Ruled out:** other workflows silently failing — `bump-scaffold-pins`, `flake`,
  `revendor-canonical-schema`, `release`, `release-npm` all green across recent runs.
  Also ruled out for `ci.yml`: `lint`, `schema-drift`, `template-page-money`,
  `template-page-vite` were each fed their own vacuous state and **do** go red
  (`golangci-lint` over zero packages exits 7; `vitest run` with no test files exits 1).
- **Leading hypothesis:** the class is closed *inside* `ci.yml` and the four
  scheduled bots, but nothing systematically asks the question, so a new job or
  script reintroduces it silently.
- **Next probe:** decide whether this deserves a standing sweep. A concrete first
  version: a test or script that walks `.github/workflows/*.yml` and flags any
  `go test` without `-count=1`, any `-run <name>` without a `--- PASS:` assertion,
  and any `run:` block whose command is expected to fail. Prove it red against
  `git show 03c3863:.github/workflows/ci.yml` (the pre-#337 version, which had four
  such holes).

## Next steps (ranked)

1. **Watch the deferred verifications** — the three unexecuted paths above. Nothing
   to build; just check the first real runs.
2. **Probe the trigger index against a non-code item** (see the first investigation).
   This decides whether #317's premise holds; everything else today is settled.
3. **Decide on a standing vacuous-green sweep** (second investigation). This is the
   only *new* work I'd argue for.
4. **#319** — drop `HOMEBREW_TAP_GITHUB_TOKEN` from `release.yml`. Now unblocked:
   v0.1.92 proved the new flow. Remove, then confirm the *next* release still pushes
   the cask.
5. **#320** — migrate `release-homebrew.yml` onto the shared `failure-issue.yml`.
   Its six-state taxonomy must move into the caller first, or the "check failed" vs
   "check could not run" distinction is flattened — which is the distinction that
   would have made the 2026-08-09 outage visible.
6. **#321** — document `CIVITAI_DEVTUNNEL_DEBUG`.
7. **Triage `rescue/*` branches** (6, local only, not pushed): `rescue/2fe5336`,
   `7bd4821`, `9339827`, `cc2e278`, `f3db896`, `f49e7cc`. Created when removing
   detached worktrees whose commits were on no branch and not in `main`. Two are
   `fix(#278,#279)` work that may duplicate #304. Delete once checked.
8. **Nine worktrees still hold uncommitted work**: `cli-dev-tunnel`,
   `cli-devtunnel-localhost`, `cli-hostkey-pin`, `cli-npm-publish`, `cli-oauth-scope`,
   `cli-oidc-draft` (2 files), `cli-pkg-sdk`, `cli-scaffold-currency`,
   `cli-ui-foundation`. Same exposure that nearly lost 156 lines of handoff work.

## Gotchas / decisions / dead-ends

- **`AGENTS.md` has 3,388 bytes of headroom** (`agentsMaxBytes = 28_758`,
  `agentsMaxBytesCeiling = 30_600`, `agentsMinBytes = 20_000`). The previous budget
  was consumed by *one* ordinary correction (#304, +2,392 B). Do not spend this
  casually; **do not append to `claudedocs/decisions/*`** — those bodies are
  sha256-pinned by `agents_split_preserved_test.go`.
- **Stop shrinking `AGENTS.md`.** Composition has flipped: items ~42%, prose ~57%.
  The remainder is Stack/Build/Layout/conventions — what every agent reads on every
  task. Eviction has ~5 kB left in total and items 2 and 4 are *smaller than a
  trigger line*, so evicting them grows the file.
- **`agentsSplitBase` is per-item and must stay so.** A single global base is
  *impossible*: after wave 1, `AGENTS.md` no longer contains the moved bodies at any
  later commit. Wave 1 `c5c3817`, wave 2 `5cb45f0`, wave 3 `932c5e9`, wave 4 the
  commit it branched from.
- **CI checks out at depth 1.** Any test reading git history passes locally and
  fails in CI — this failed #305. Use `make ci-shallow` (added #311). Its `file://`
  clone URL is load-bearing: a plain path hardlinks the object store and ignores
  `--depth`.
- **`make ci` does NOT run lint**, and `make lint` **errors** if golangci-lint is
  missing (it does not fall back — an earlier claim that it did was false).
- **Don't build `SCRIPTING.md`** — it breaks `TestREADMEExitCodeTableIsGenerated`
  by construction (`extractREADMEExitCodeTable` Fatals if `## Exit codes` is absent
  from README) and creates an unenforced second home for a published contract.
- **README's exit-code table is generated** from `exitCodeDocs`
  (`internal/cmd/exitcodes_doc.go`) and feeds `civitai --help` too. Never hand-edit.
- **`raw.githubusercontent.com` caches per `Accept-Encoding` variant**, and Go's
  `net/http` auto-sends gzip. Cache-busting query params and `Cache-Control:
  no-cache` are both **ignored** — measured. Staleness is per edge node. The fix is
  to read a commit-pinned immutable URL (#335), not to fight the cache.
- **`civitai/cli` is a PUBLIC repo.** #318 redacted a live prod health token
  (`token=<value>` in a curl) that appeared nowhere in git history. Check before
  committing pasted commands.
- **zsh does not word-split unquoted `$var`.** A loop over `$args` passed the whole
  string as one argument and manufactured a false docs-drift finding mid-session.
  Use `"$@"`.
- **A checksum gate proves *an* edit applied, not the *intended* one.** I mutated a
  comment occurrence of `file://` instead of the real clone line and read the
  resulting pass as a defect.

## How to verify

```bash
cd /home/zach/workspace/civit/cli
make ci && make lint && make ci-shallow     # 19 pkgs ok; 0 issues; 19/19 shallow
go run ./tools/caskcheck                    # OK: cask 0.1.92, pinned to a tap commit
wc -c AGENTS.md                             # 25,370 — under agentsMaxBytes = 28_758
gh release list --repo civitai/cli --limit 1        # v0.1.92 Latest
npm view @civitai/cli version                       # 0.1.92
curl -sSL https://raw.githubusercontent.com/civitai/homebrew-tap/main/Casks/civitai.rb | grep version
```

Read output, not exit codes: count `ok` packages and `--- FAIL` leaves, and grep for
`panic: test timed out`. `make` reports rc=2 when a recipe fails.
