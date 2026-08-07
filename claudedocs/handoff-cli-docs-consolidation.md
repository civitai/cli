# Handoff: cli-docs-consolidation — 2026-08-07

## Goal
Decide whether `civitai/cli`'s in-repo docs should be dropped in favour of the hosted
docs at developer.civitai.com, and act on the answer. Answer reached and acted on;
what remains is listed below.

**The answer: do NOT drop the in-repo docs — relocate their content into the cobra
command definitions.** 1,397 of `README.md`'s 1,616 lines (86.4%) are prose no
generator can produce. But the README's *command-reference* prose can migrate into
`Long` / `Annotations`, where it becomes generatable AND falls under a drift guard.
Deleting it and generating from `Short` would destroy ~80% of its content: reference
table cells measure 60–2,263 chars (median ~400) against `Short` strings of 24–79.

## State now

- **Branch / PR:** none open from this session. `civitai/cli` @ `f56aa72` (main),
  `civitai-developer-docs` @ `b8a6071` (main). Both clean.
  `civitai-developer-docs` has one unrelated open PR (#5, another session's, from July).
- 🔴 **Both repos are SHARED checkouts.** `cli`'s working tree currently shows
  ` M claudedocs/handoff-app-blocks-hardening.md` — **another session's edit, not mine.**
  Another session also landed #244–#251 in `cli` during this work. Re-check
  `git branch --show-current` before any write.

### DONE — 10 PRs merged

| PR | Repo | What |
|---|---|---|
| #243 | cli | 8 wrong `AGENTS.md` `item N` cross-refs + `agents_xrefs_test.go` (resolution guard) |
| #247 | cli | `agents_index_test.go` — catches an item the file's own index never names |
| #37 | docs | CLI snapshot was 30 commits stale; `metrics` missing; reverse assertion; `__complete` enumeration |
| #38 | docs | `hostHandlerParity` drift was **semantic**, not cosmetic; `replyNote` split |
| #39 | docs | 69 example lines rendered; anchor slugs; heading depth |
| #40 | docs | 7 stale SDK stamps, a wrong `ModelSlotContext` cast, Nix install + 3 guards |
| #41 | docs | least-privilege `contents: read` on all 6 workflows |
| #43 | docs | removed `paths:` from the 4 required workflows (they deadlocked the gate) |
| #44 | docs | took the npm pin-freshness fetch out of the required `test-bridge` gate |
| #42 | docs | **CLI reference 19 → 52 commands + global flags** |

### Verified live
- Published reference now documents `generate`, `workflows`, `download`, `models`,
  `app metrics`, and `--spend`/`--budget` (the site previously told authors
  `dev:live` could not spend Buzz).
- `civitai-developer-docs` branch protection **created this session**: requires
  `test-cli`, `test-messages`, `test-bridge`, `typecheck-snippets`
  (`strict=false`, `enforce_admins=false`, no required reviews). It had **none** before.
- `cli` protection unchanged: `pins-vs-published`, `scaffold-currency`, `build-test`,
  `ready-ack-runtime`, `template-page-vite`.
- Forgejo downgrade (`civitai/civitai#3713`) **rolled out** — prod on `5.0.2249`,
  6/6 pods, new ReplicaSet. Released by another session, not this one.

## Open investigations — live diagnosis state

### `civitai login --help` emits a raw NUL byte — upstream bug, still ships
- **Symptom + exact repro:** `civitai login --help > out.txt` produces a file that
  git, grep and `file(1)` classify as binary.
- **Observed (with values):**
  - `internal/cmd/login.go:42` — `const tokenFlagNoValue = "\x00civitai-token-no-value"`
  - `civitai login --help | tr -dc '\000' | wc -c` → **1**
  - `file -b` on the captured output → **`data`**
  - pflag splits each help row on the **first** `\x00`; with two sentinels present it
    aligns on the wrong one, so its separator survives to stdout and `[="…"]` prints
    with alignment spacing inside it. Parsed as `flags: '--token string[="'`.
- **Ruled out:** not a terminal/TTY artifact — reproduces piped to a file.
  Not a docs-side defect — the byte originates in the CLI.
- **Blast radius:** it made the docs repo's committed snapshot a binary file, so
  `git diff` refused to render it — which would have silently ended hand-review of
  every future re-capture. Repaired **at the docs capture seam only** (PR #42).
  The CLI defect is unfixed and **no issue has been filed**.
- **Next probe / action:** file an issue against `civitai/cli` citing `login.go:42`.
  Likely fix: use a non-NUL sentinel, since the only requirement is that
  `NoOptDefVal` be non-empty and not collide with a real token.

### `scripts/check-appblocks-pins.mjs:34-38` states a trigger that no longer exists
- **Observed:** header still reads *"It ALSO runs on a PR that touches package.json
  so a pin bump is verified against `latest` at review time."*
- **Why false:** #43 removed the `paths:` filter (so it ran on *every* PR, not only
  package.json ones), then #44 removed the PR invocation entirely. `check:pins` is
  now **schedule-only** via `appblocks-drift.yml:72-77`.
- **Ruled out:** not a duplicate of the comment #44 *did* fix — that one was in the
  sibling file `check-design-system-pins.mjs:172-181`.
- **Next probe:** one comment edit pointing at `appblocks-drift.yml`. LOW.

### `appblocks-drift.yml`'s scheduled cron has never been observed firing green
- **Observed:** only `workflow_dispatch` runs have been proven to execute
  `check:pins` for real (run `31149205417`, log line
  `Pins: 2 current · 0 lagging · 0 skipped (unreachable)` — the `0 skipped` is the
  discriminating detail proving the registry was reached).
  Scheduled runs were **red 10 consecutive days** (07-28 → 08-06) on `check:snapshots`.
- **Ruled out:** that it is still broken — PR #38 fixed the underlying
  `hostHandlerParity` drift; two post-#38 dispatched runs are green.
- **Leading hypothesis:** the next scheduled run (cron `37 6 * * *`, observed to
  actually fire 08:32–10:25 UTC — 2–3.8h skew) will be the first green schedule
  since 07-27.
- **Next probe:**
  `gh run list --repo civitai/civitai-developer-docs --workflow appblocks-drift.yml --limit 3 --json conclusion,event,createdAt`

### `parseSemver` rejects SemVer build metadata → a 4th red path nobody enumerated
- **Observed:** `check-appblocks-pins.mjs:59` `parseSemver` throws on `X.Y.Z+meta`
  → `compareSemver:71` → caught at `:190` → **exit 2**.
- **Why it matters:** that exit is triggered purely by *someone else's* publish. The
  in-file comment lists the red paths as "lagging / 404-410 / unexpected error" and
  does not say an upstream publish can cause the third.
- **Next probe:** decide whether to accept build metadata in `parseSemver` or just
  document it. Argues **for** #44 having removed this from the required gate. LOW.

## Next steps (ranked)

1. **File the NUL-byte issue against `civitai/cli`** (evidence above, ready to paste).
   Outward-facing — needs maintainer go-ahead.
2. **Decide `site/guide/cli.md`.** 367 hand-written lines covering the read/download
   half with its own flag table. Measured drift **today**: it carries **14**
   hand-maintained flag rows against **393** flag lines in the generated snapshot,
   and pins **v0.1.67** — 24 releases stale. Options: replace its tables with the
   generated reference, or keep it as narrative and delete the tables. Leaving it is
   not neutral.
3. **Migrate README command-reference prose into `Long` / `Annotations`** (the actual
   answer to this session's question). gh's `Annotations["help:json-fields"]` is the
   working precedent for publishing `--json` shapes from the command definition.
4. **Global flags on the site** — now emitted as `program.globalOptions` by #42, but
   confirm they render as intended before assuming closed.
5. The three LOW items above (pins header, cron confirmation, `parseSemver`).
6. **Prune `claudedocs/`** — 1,562 lines of dated handoffs that belong in neither
   the repo nor the site.

## Gotchas / decisions / dead-ends

- 🔴 **The `cli.json` export command is DISPROVED — do not re-propose it.** Measured:
  the widening it was meant to enable works with the existing scraper (53 nodes, 0
  enumeration disagreements, 0.79s). `Long` recovers **54/54 byte-exact**; defaults
  **0 mismatches over 180 flags** — and an export would be *worse* there, because
  **24 flags carry their real effective default in authored prose that `DefValue`
  does not** (`--dir` → `./<slug>` vs `DefValue=""`). Cobra v1.8.1 → v1.10.2 help
  output: **0 byte-differences across 53 nodes**. `Annotations`/`GroupID`/`Deprecated`
  counts are **0/0/0**. Settled separately: a `go:generate` program *can* import
  `internal/cmd` (negative control: an external module cannot), so the
  published-binary-surface question was moot.
- 🔴 **`paths:` filters + required contexts = deadlock.** A required check whose
  workflow never triggers blocks a PR forever at `MERGEABLE/BLOCKED`. This was
  self-inflicted here (protection added before checking the workflows always run) and
  blocked #42. Fixed in #43 by removing the filters. **Rejected**: the common
  "dummy job that always succeeds with the same name" pattern — it makes a required
  check report green without running anything.
- 🔴 **The generator prefers a live `civitai` on PATH.** `/home/zach/.local/bin/civitai`
  is `v0.1.89-20-g4018e2c` (stale) and silently produced **47 commands instead of 52**,
  dropping `generate` and `workflows`. #42 made the snapshot the default source with
  `CIVITAI_CLI_LIVE=1` opt-in **plus** a live-vs-snapshot diff. Also:
  `cli/bin/civitai` is currently `v0.1.90-19-gf611250-**dirty**` — a binary built from
  a dirty tree is evidence about that working copy and about no commit.
- **zsh ate a variable again:** `"$R:cli"` → the `:c` parsed as a history modifier and
  produced `/home/zach/workspace/civit/clili` with no error. Brace it: `${R}`.
- **Three mutants survived first runs, all the same shape** — `assertEnumerationsAgree`
  (correct but not wired into `buildArtifact`), the slot assertion (satisfied by a code
  *comment*), and "prefer live" (every test drove the pure helper, nothing reached
  `resolveBundle`). Component correct, wiring untested. Mutate the CALL SITES.
- **A single-line grep over wrapped prose gives a confident wrong answer.** "Item 24 is
  unreferenced" was wrong because `and item 24` / `covers the…` wraps across a newline.
  `agents_index_test.go` now joins the region before matching, for exactly this reason.
- **A repeated failure is not a deterministic one.** `api/v1/articles/4797` returned 500
  four times over ~15 min, then recovered; the by-id route 404s correctly for missing
  ids and is healthy across the id space. No platform bug — do not re-file.

## How to verify

```bash
D=/home/zach/workspace/civit/civitai-developer-docs
R=/home/zach/workspace/civit/cli
git -C ${D} fetch origin -q && git -C ${R} fetch origin -q

# #42 — the whole tree is published, snapshot is the default source
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c CIVITAI_CLI_LIVE   # expect 3
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c globalOptions      # expect >=1

# #44 — pin freshness is schedule-only
git -C ${D} show origin/main:.github/workflows/appblocks-bridge.yml | command grep -cE '^\s*run:.*check:pins'  # expect 0
git -C ${D} show origin/main:.github/workflows/appblocks-drift.yml  | command grep -c 'check:pins'             # expect 2

# #243 / #247 — both AGENTS.md guards green
cd ${R} && go test ./ -run 'TestEveryAgentsItemIsNamedByTheIndex|TestAgents'

# protection is real (never infer gating from a job existing)
gh api repos/civitai/civitai-developer-docs/branches/main/protection --jq '.required_status_checks.contexts'

# the NUL bug still ships
${R}/bin/civitai login --help | tr -dc '\000' | wc -c   # expect 1
```
