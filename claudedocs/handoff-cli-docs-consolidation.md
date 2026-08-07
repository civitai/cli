# Handoff: cli-docs-consolidation — 2026-08-07

## Goal
Decide whether `civitai/cli`'s in-repo docs should be dropped in favour of the hosted
docs at developer.civitai.com, and act on the answer. Decision reached and shipped;
what remains is listed below.

**The answer: do NOT drop the in-repo docs — relocate their content into the cobra
command definitions.** 1,397 of `README.md`'s 1,616 lines (86.4%) are prose no
generator can produce. But the README's *command-reference* prose can migrate into
`Long` / `Annotations`, where it becomes generatable AND falls under a drift guard.
Deleting it and generating from `Short` would destroy ~80% of its content: reference
table cells measure 60–2,263 chars (median ~400) against `Short` strings of 24–79.

## State now

- **Branch / PR:** none open from this session. `civitai/cli` @ `24797c7` (main),
  `civitai-developer-docs` @ `308b633` (main). Both clean.
  Other sessions have two unrelated PRs open in the docs repo: **#45**
  (`.md`/LLM channel App Blocks island payloads — see the llms.txt note below) and
  **#5** (from July).
- 🔴 **Both repos are SHARED checkouts.** `cli`'s tree shows
  ` M claudedocs/handoff-app-blocks-hardening.md` — **another session's edit.**
  Other sessions landed #244–#251 and #254 in `cli` during this work. Always
  `git branch --show-current` before any write; work in a worktree.

### DONE — 12 PRs merged + 1 issue filed

| PR | Repo | What |
|---|---|---|
| #243 | cli | 8 wrong `AGENTS.md` `item N` cross-refs + `agents_xrefs_test.go` |
| #247 | cli | `agents_index_test.go` — catches an item the file's own index never names |
| #252 | cli | this handoff |
| #37 | docs | CLI snapshot 30 commits stale; `metrics` missing; reverse assertion; `__complete` enumeration |
| #38 | docs | `hostHandlerParity` drift was **semantic**, not cosmetic; `replyNote` split |
| #39 | docs | 69 example lines rendered; anchor slugs; heading depth |
| #40 | docs | 7 stale SDK stamps, a wrong `ModelSlotContext` cast, Nix install + 3 guards |
| #41 | docs | least-privilege `contents: read` on all 6 workflows |
| #43 | docs | removed `paths:` from the 4 required workflows (they deadlocked the gate) |
| #44 | docs | took the npm pin-freshness fetch out of the required `test-bridge` gate |
| #42 | docs | **CLI reference 19 → 52 commands + global flags** |
| #46 | docs | retired the hand-maintained flag tables from `site/guide/cli.md` |
| **issue #253** | cli | the `login --help` NUL byte (below) — **filed, unfixed** |

### Verified live
- Published reference documents `generate`, `workflows`, `download`, `models`,
  `app metrics`, and `--spend`/`--budget` (the site previously told authors
  `dev:live` could not spend Buzz).
- `civitai-developer-docs` branch protection **created this session** — it had
  **none** before. Requires `test-cli`, `test-messages`, `test-bridge`,
  `typecheck-snippets` (`strict=false`, `enforce_admins=false`, no reviews).
  `cli` protection unchanged (5 contexts).
- `appblocks-drift.yml` scheduled cron **green at 2026-08-07T07:52:57Z** — first
  green schedule since 07-27; #38 fixed the 10-day red streak.
- Forgejo downgrade (`civitai/civitai#3713`) **rolled out** — prod on `5.0.2249`,
  6/6 pods. Released by another session.

## Open investigations — live diagnosis state

### `civitai login --help` emits a raw NUL byte — filed as #253, still ships
- **Symptom + exact repro:** `civitai login --help > out.txt` yields a file git,
  grep and `file(1)` classify as binary.
- **Observed (with values), on a CLEAN build from `origin/main`** (not a dirty tree):
  - `internal/cmd/login.go:42` — `const tokenFlagNoValue = "\x00civitai-token-no-value"`
  - `civitai login --help | tr -dc '\000' | wc -c` → **1**
  - `file -b` → **`data`**; `grep -c token` → `binary file matches`
  - `od -c` at the break: `e n - n o - v a l u e " ]  \0   s   t`
  - Mechanism: pflag's `FlagUsagesWrapped` builds rows as `<flag>\x00<usage>` and
    **splits on the first `\x00`**. `NoOptDefVal` is interpolated as `[="%s"]`, so
    the row holds two NULs, pflag aligns on ours, and its own separator reaches
    stdout. A parser reading the row sees the flag as `--token string[="`.
- **Ruled out:** terminal/TTY artifact (reproduces piped to a file); a dirty-tree
  artifact (clean build from `origin/main` reproduces it).
- **Status:** repaired **downstream only**, at the docs capture seam (#42). CLI-side
  unfixed. Suggested fix in #253: any non-NUL sentinel — the only requirement is
  that `NoOptDefVal` be non-empty and not collide with a real token — plus a
  regression test asserting `login --help` contains no `\x00`, since the failure is
  invisible in a terminal.

### `scripts/check-appblocks-pins.mjs:35-38` states a trigger that no longer exists
- **Observed, still present on `origin/main`:** *"It ALSO runs on a PR that touches
  package.json so a pin bump is verified against `latest` at review time."*
- **Why false:** #43 removed the `paths:` filter (so it ran on *every* PR, not only
  package.json ones), then #44 removed the PR invocation entirely. `check:pins` is
  now **schedule-only** via `appblocks-drift.yml:72-77`.
- **Ruled out:** not the comment #44 *did* fix — that was the sibling file
  `check-design-system-pins.mjs:172-181`.
- **Next probe:** one comment edit pointing at `appblocks-drift.yml`. LOW.

### `check-appblocks-pins.mjs` exits 2 on a SemVer build-metadata `latest`
- **Observed (measured, mechanism corrected):** `parseSemver` **returns `null`**,
  it does not throw — `parseSemver("0.31.0+build.7")` → `null`,
  `parseSemver("0.31.0")` → `{major:0,minor:31,patch:0,pre:null}`. The throw is an
  explicit guard in `compareSemver`: `if (!pa || !pb) throw new Error(...)`, whose
  doc comment says "Unparseable -> throws" — intentional, not an oversight. That
  propagates to the catch at ~`:190` → **exit 2**.
- **Why it matters:** exit 2 is triggerable purely by *someone else's* publish, and
  the in-file comment lists the red paths as "lagging / 404-410 / unexpected error"
  without saying an upstream publish can cause the third. `X.Y.Z+meta` is valid
  SemVer 2.0.0.
- **Next probe:** accept build metadata in `parseSemver` (strip `+…` before compare)
  or document it. Argues **for** #44 having removed this from the required gate. LOW.

### IA wart: a public-API reader is now sent into the Apps section
- **Symptom:** `site/guide/cli.md` (public-API guide — searching/downloading models)
  now links flag reference to `apps/reference/cli.md`, under `/apps/` (App
  *authoring*). That page's intro greets the reader with "the canonical tool for
  authoring Civitai Apps" before mentioning downloads.
- **Why it exists:** #46 deleted the guide's hand-maintained flag tables; the
  generated reference is the only remaining copy, and it lives under `/apps/`.
- **Options written up, none chosen:** (a) alias or render `<CliReference />` under
  `/site/` too; (b) move the generated reference somewhere audience-neutral;
  (c) minimum — reword the reference page's intro so it does not presume authoring.
- **Note:** docs PR **#45** (another session, open) touches the `.md`/LLM channel and
  may interact — the generated reference does **not** reach `llms-full.txt` because
  `apps/reference/cli.md` is a bare `<CliReference />` tag (INFERRED from how
  `vitepress-plugin-llms` operates; confirm by building and reading the artifact).

## Next steps (ranked)

1. **Migrate README command-reference prose into `Long` / `Annotations`** — the
   actual answer to this session's question, and the only path that converges the
   two surfaces without deleting the richer one. gh's
   `Annotations["help:json-fields"]` is the working precedent for publishing
   `--json` shapes from the command definition.
2. **Decide the IA wart** (above) — cheapest is option (c).
3. **Fix #253** in `civitai/cli` — one-line sentinel change + a regression test.
4. The two LOW items above (pins header comment; `parseSemver` build metadata).
5. **Prune `claudedocs/`** — dated handoffs that belong in neither the repo nor the
   site. Coordinate: other sessions actively write here.

## Gotchas / decisions / dead-ends

- 🔴 **The `cli.json` export command is DISPROVED — do not re-propose it.** The
  widening it was meant to enable works with the existing scraper (53 nodes, 0
  enumeration disagreements, 0.79s). `Long` recovers **54/54 byte-exact**; defaults
  **0 mismatches over 180 flags** — and an export would be *worse* there, because
  **24 flags carry their real effective default in authored prose that `DefValue`
  does not** (`--dir` → `./<slug>` vs `DefValue=""`). Cobra v1.8.1 → v1.10.2 help
  output: **0 byte-differences across 53 nodes**. `Annotations`/`GroupID`/
  `Deprecated` counts are **0/0/0**. A `go:generate` program *can* import
  `internal/cmd` (negative control: an external module cannot), so the
  published-binary-surface question was moot.
- 🔴 **`paths:` filters + required contexts = deadlock.** A required check whose
  workflow never triggers blocks a PR forever at `MERGEABLE/BLOCKED`. Self-inflicted
  here (protection added before checking the workflows always run); it blocked #42.
  Fixed in #43 by removing the filters. **Rejected**: the common "dummy job that
  always succeeds with the same name" pattern — it makes a required check report
  green without running anything.
- 🔴 **The generator prefers a live `civitai` on PATH.** `~/.local/bin/civitai` is
  `v0.1.89-20-g4018e2c` (stale) and silently produced **47 commands instead of 52**,
  dropping `generate` and `workflows`. #42 made the snapshot the default with
  `CIVITAI_CLI_LIVE=1` opt-in **plus** a live-vs-snapshot diff. Also:
  `cli/bin/civitai` is rebuilt by other sessions and is often `-dirty` — a binary
  built from a dirty tree is evidence about that working copy and about no commit.
- **#46's deletion was better-justified than it looked.** The `### Download flags`
  table listed **12** flags; `download` has **14**. The two missing were `--version`
  and `--yes` — *both* about the ambiguous-id safety stop — so the rotted rows were
  exactly the ones a reader who hits that stop needs. Every one of the 12 was
  confirmed present in the generated artifact before deleting.
- **zsh ate a variable:** `"$R:cli"` → the `:c` parsed as a history modifier,
  producing `/home/zach/workspace/civit/clili` with no error. Brace it: `${R}`.
- **Three mutants survived first runs, all the same shape** — `assertEnumerationsAgree`
  (correct but not wired into `buildArtifact`), the slot assertion (satisfied by a
  code *comment*), and "prefer live" (every test drove the pure helper, nothing
  reached `resolveBundle`). Component correct, wiring untested. **Mutate the CALL
  SITES, not just the function.**
- **A single-line grep over wrapped prose gives a confident wrong answer.** "Item 24
  is unreferenced" was wrong because `and item 24` / `covers the…` wraps across a
  newline. `agents_index_test.go` joins the region before matching for this reason.
- **A repeated failure is not a deterministic one.** `api/v1/articles/4797` returned
  500 four times over ~15 min, then recovered; the by-id route 404s correctly for
  missing ids and is healthy across the id space. No platform bug — do not re-file.
- **Read whole output, not `tail`.** Checking the new flag-table guard with `tail -3`
  showed only `apps/reference/cli.md` and read as "it doesn't cover the page it was
  built for". Full output shows both pages plus an 11-fixture detector control.

## How to verify

```bash
D=/home/zach/workspace/civit/civitai-developer-docs
R=/home/zach/workspace/civit/cli
git -C ${D} fetch origin -q && git -C ${R} fetch origin -q

# #42 — whole tree published, snapshot is the default source
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c CIVITAI_CLI_LIVE   # expect 3
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c globalOptions      # expect >=1

# #46 — no hand-maintained flag tables anywhere (reads BOTH pages + a detector control)
cd ${D} && node scripts/check-no-hand-flag-tables.mjs        # read all output, not the tail

# #44 — pin freshness is schedule-only
git -C ${D} show origin/main:.github/workflows/appblocks-bridge.yml | command grep -cE '^\s*run:.*check:pins'  # expect 0
git -C ${D} show origin/main:.github/workflows/appblocks-drift.yml  | command grep -c 'check:pins'             # expect 2

# #243 / #247 — both AGENTS.md guards green
cd ${R} && go test ./ -run 'TestEveryAgentsItemIsNamedByTheIndex|TestAgents'

# protection is real (never infer gating from a job existing)
gh api repos/civitai/civitai-developer-docs/branches/main/protection --jq '.required_status_checks.contexts'

# the scheduled drift sweep stays green
gh run list --repo civitai/civitai-developer-docs --workflow appblocks-drift.yml --limit 3 \
  --json conclusion,event,createdAt

# #253 — the NUL bug still ships (expect 1 until it is fixed)
${R}/bin/civitai login --help | tr -dc '\000' | wc -c
```
