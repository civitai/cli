# Handoff: cli-docs-consolidation — 2026-08-07 (r2)

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

## The Long/Annotations split — SCOPED AND PILOTED (2026-08-07, session 2)

The premise of next-step 1 below was **half wrong, and the half that was wrong is
the half that mattered.** Measured on the whole tree at `aeceb6b`, 53 command
nodes, via a scratch test that dumps the real cobra tree (`Short`/`Long`/
`Example`/`Annotations`) as JSON — not by parsing help text:

| Fact | Measured |
|---|---|
| Nodes carrying an `Annotations` map | **0 of 53** |
| Nodes with NO `Long` at all | 8 |
| Nodes with a `Long` under 400 chars | 16 more |
| README table cells, 16 matched rows | 6,255 chars total |
| `Long` on those same 16 nodes | **19,017 chars** total |

**`Long` is already 3× RICHER than the README table for the App-authoring
commands.** For 14 of the 16 matched rows the ratio is >1 (`app submit` 11×,
`app init` 9×). Only `whoami` is genuinely README-richer (590 vs 431). So
"migrate the README's prose into `Long`" is mostly ALREADY DONE where the handoff
assumed it was pending — and the real gap is the **public read-API group**
(`models`/`images`/`collections`/`creators`/`tags`/`users`/`model-versions`/
`articles`), which owns 19 of the 24 empty-or-thin nodes and has **no table row
at all**.

### 🔴 `Annotations` CANNOT reach the docs site. Measured, not inferred.

The generator captures exactly two channels per node — `civitai <path> --help`
and `civitai __complete <path> ""` — and parses the text
(`gen-appblocks-cli.mjs:13-15`). Cobra's default help template does not render
`Annotations`. Probe: set `Annotations{"probe:visible-in-help": "ZZPROBEZZ"}` on
`workflows get` and rebuild —

- `strings <binary> | grep -c ZZPROBEZZ` → **1** (the probe really applied)
- in `--help` → **0**; in `__complete` → **0**
- positive control, a word that IS in that help (`PRESIGNED`) → **1**
- `workflows get --help` is byte-identical with and without it (1512 both)

So an `Annotations` half is a no-op for docs unless (a) a custom help template
renders them — at which point they are `Long` with extra steps and cost the same
terminal space, or (b) a `cli.json` export seam, which the gotchas below
**disprove**. **Recommendation: drop the `Annotations` half.** The split that
survives is `Long` (prose, budgeted) vs the *guide* (everything that does not fit).

### 🔴 THE BLOCKER: the generator PARSES `Long` and then THROWS IT AWAY.

`gen-appblocks-cli.mjs:781` —
`description: short || parseLongDescription(help).split('\n')[0] || ''`

`parseLongDescription` captures the whole `Long` body, but it is only a
**fallback**, and even then only its **first line**. A subcommand always has a
`Short` from the parent's "Available Commands" block, so `short` always wins.
Measured end-to-end through the real generator with `CIVITAI_CLI_LIVE=1
CIVITAI_CLI_BIN=<pilot binary>`: the pilot added ~4,000 chars of `Long` and the
published `cli.json` gained **+926 chars, all of it `examples`**. The `Long`
reached `--help` and nothing else.

**Nothing about this migration reaches developer.civitai.com until that line
changes** (emit `longDescription: parseLongDescription(help)` alongside
`description`, and render it in `<CliReference />`). That is a `civitai-developer-docs`
PR and it is the **prerequisite**, not a follow-up — do it before migrating any
further group.

### Pilot — `civitai app listing`, 7 nodes, on branch `zach/help-long-pilot`

Chosen because it holds 3 of the 8 no-`Long` nodes, maps to the README's densest
single table row, and its missing facts (formats + per-kind byte caps) are **Go
constants**, so the help can be derived from the validator rather than duplicated.

- set-icon / set-cover / add-screenshot gained a `Long` + `Example` (had neither).
- status / rm-screenshot / reorder gained an `Example`.
- Formats + caps now stated, **computed** from `maxIconBytes`/`maxCoverBytes`/
  `maxScreenshotBytes` through the same `humanBytes` the refusal message uses, so
  `--help` predicts the error text.
- Four pre-existing lines over 80 columns rewrapped.

**`--help` cost, base vs the committed pilot, all 54 captured nodes:** 47 nodes
byte-identical, 7 changed; tree 100,451 → 104,190 bytes (**+3.7%**) and
1,987 → 2,065 lines (**+3.9%**). Per node the group runs 16→32 lines (set-icon),
17→40 (add-screenshot, the largest).

Three guards, none subsuming the others — caps-vs-constants (incl. cross-kind,
since icon and screenshot share a 2 MiB cap), completeness (Long+Short+Example on
every node, tree-walked with a count floor), budget (1400 chars / 80 columns).
**9/9 mutants killed by the intended guard, checksum-gated; a comment-only null
mutant survived.** `make ci` green, 18/18 packages.

🔴 **Count RUNES, not bytes, in a column guard.** The first version used `len()`
and failed four PRE-EXISTING bodies that render inside 80 columns — em-dashes are
3 bytes, one column.

### Where the remaining ~46 nodes stand

| Bucket | Count | Nodes |
|---|---|---|
| No `Long` | 5 left | `collections get`, `creators search`, `model-versions get`, `models get`, `tags search` |
| Thin (<400) | 14 left | the rest of the read-API group |
| Mid (400–1200) | 17 left | mostly fine |
| Rich (>1200) | 11 | `generate` (5,726), `download` (3,742), root (4,203) — these need a BUDGET, not more prose |

## Open investigations — live diagnosis state

### ~~`civitai login --help` emits a raw NUL byte — #253~~ — FIXED
Closed by `9cfe468` (#264). Re-verified on a clean build at `aeceb6b`:
`civitai login --help | tr -dc '\000' | wc -c` → **0** (positive control:
`printf 'a\000b' | tr -dc '\000' | wc -c` → 1, so the pipe is wired).
The original diagnosis is kept below for the mechanism.

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

1. 🔴 **Publish `Long` from the generator** (`civitai-developer-docs`) — emit
   `longDescription: parseLongDescription(help)` at `gen-appblocks-cli.mjs:781`
   and render it in `<CliReference />`. **Everything else in this workstream is
   dead weight until this lands** — measured above, the pilot's 4,000 chars of
   `Long` reached `--help` and 0 chars of `cli.json`. Cheap: the parser already
   exists and is already called.
2. **Merge the pilot** (`zach/help-long-pilot`, 1 commit) — it stands on its own
   for terminal users even before step 1.
3. **Then migrate the read-API group** (`models`/`images`/`collections`/
   `creators`/`tags`/`users`/`model-versions`/`articles`, 19 empty-or-thin nodes)
   using the pilot's three guards as the template. Its source prose is
   README lines 414–498, which has **no table row** — so unlike the App commands
   this really is a migration rather than a re-statement.
4. **Do NOT add an `Annotations` half.** Measured disproof above.
5. **Decide the IA wart** (below) — cheapest is option (c).
6. The two LOW items below (pins header comment; `parseSemver` build metadata).
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

# #253 — FIXED in #264; expect 0 now. The positive control matters: a pipe that
# reports 0 because it is wired to nothing looks identical to a fixed binary.
${R}/bin/civitai login --help | tr -dc '\000' | wc -c   # expect 0
printf 'a\000b' | tr -dc '\000' | wc -c                 # expect 1 (control)

# the app-listing help pilot (branch zach/help-long-pilot)
git -C ${R} fetch origin -q
go test ./internal/cmd -run TestListingHelp -count=1 -v | grep -c -- '--- PASS'  # expect 14

# 🔴 Annotations are invisible to the docs pipeline — re-measure before believing
# any plan that relies on them. Set one on any command, rebuild, then:
#   strings <bin> | grep -c ZZPROBEZZ      -> 1   (the probe applied)
#   <bin> <cmd> --help | grep -c ZZPROBEZZ -> 0   (it never reaches the channel)
#   <bin> <cmd> --help | grep -c <a word actually in that help> -> 1  (control)

# 🔴 The generator drops Long. This is the blocker, not a nicety.
command grep -n 'description: short ||' ${D}/scripts/gen-appblocks-cli.mjs
# End-to-end proof: generate from a binary with a fattened Long and diff cli.json —
# only `examples` moves.
CIVITAI_CLI_LIVE=1 CIVITAI_CLI_BIN=<your build> node ${D}/scripts/gen-appblocks-cli.mjs
```
