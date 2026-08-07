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

- **Branch / PR:** none open from this session. `civitai/cli` @ `aeceb6b` (main),
  `civitai-developer-docs` @ `9382513` (main). Both clean.
- 🔴 **Both repos are SHARED and BUSY.** `cli`'s tree shows
  ` M claudedocs/handoff-app-blocks-hardening.md` — **another session's edit.**
  Other sessions have **#267, #268, #269 open in `cli`** and **#5 in docs**, and
  landed #244–#254, #266 in `cli` plus #45/#47 in docs during this work.
  Always `git branch --show-current` before any write; work in a worktree.

### DONE — 14 PRs merged, 1 issue filed and closed

| PR | Repo | What |
|---|---|---|
| #243 | cli | 8 wrong `AGENTS.md` `item N` cross-refs + `agents_xrefs_test.go` |
| #247 | cli | `agents_index_test.go` — catches an item the file's own index never names |
| #252 / #261 | cli | this handoff (r1, r2) |
| #264 | cli | **fixed the `login --help` NUL byte** — closes issue #253 |
| #37 | docs | CLI snapshot 30 commits stale; `metrics` missing; reverse assertion; `__complete` enumeration |
| #38 | docs | `hostHandlerParity` drift was **semantic**, not cosmetic; `replyNote` split |
| #39 | docs | 69 example lines rendered; anchor slugs; heading depth |
| #40 | docs | 7 stale SDK stamps, a wrong `ModelSlotContext` cast, Nix install + 3 guards |
| #41 | docs | least-privilege `contents: read` on all 6 workflows |
| #43 | docs | removed `paths:` from the 4 required workflows (they deadlocked the gate) |
| #44 | docs | took the npm pin-freshness fetch out of the required `test-bridge` gate |
| #42 | docs | **CLI reference 19 → 52 commands + global flags** |
| #46 | docs | retired the hand-maintained flag tables from `site/guide/cli.md` |
| #48 | docs | re-captured the help snapshot at `civitai/cli@9cfe468` |

### Verified live (not inferred)
- `https://developer.civitai.com/apps/reference/cli` serves the widened reference —
  `cli-generate` and `cli-global-flags` both present. `/site/guide/cli` serves **zero**
  hand-maintained flag rows.
- **`llms-full.txt` now carries the CLI reference** — 15 `civitai generate` mentions,
  11 `--layout`. This was **0/0** before docs #45. The llms channel gap is CLOSED.
- Committed snapshot on docs `main`: **0 NUL bytes**, old sentinel gone,
  `Binary version: civitai v0.1.90-25-g9cfe468`, 106 `===CMD` blocks.
- `civitai-developer-docs` branch protection **created this session** — it had **none**
  before. Requires `test-cli`, `test-messages`, `test-bridge`, `typecheck-snippets`.
  `cli` protection unchanged (5 contexts).
- `appblocks-drift.yml` scheduled cron **green at 2026-08-07T07:52:57Z** — first green
  schedule since 07-27; #38 fixed the 10-day red streak.
- Forgejo downgrade (`civitai/civitai#3713`) **rolled out** — prod on `5.0.2249`, 6/6 pods.

## Open investigations — live diagnosis state

### The snapshot-freshness guard cannot detect between-release drift
- **Symptom:** `scripts/check-appblocks-cli-snapshot.mjs` verdicts `ok` while the
  committed snapshot is materially stale. It could not have flagged docs #48.
- **Observed (measured by driving the guard's own exported helpers):**
  ```
  OLD snapshot (12 CLI commits stale) -> tag v0.1.90 | ahead 13 | verdict: ok
  NEW snapshot                        -> tag v0.1.90 | ahead 25 | verdict: ok
  ```
  It parses `ahead` and `sha` and then **classifies on `tag` alone.** On a repo where
  `main` moves daily and tags are rare, that means it misses *most* drift.
- **Ruled out — the obvious fix is wrong:** comparing shas goes red on every upstream
  commit (8 of those 12 touched no help text at all) → a permanently-red daily gate,
  which this repo's own doctrine forbids.
- **Leading option:** content-hash against a freshly built binary. Needs a Go toolchain
  in the drift workflow, a decision on which bytes to hash (the header changes every
  commit), a SKIP contract for upstream build failures, and a new remedy string.
- **Cheap middle option (~15 lines):** report "N commits behind `main`" informationally
  and never fail. Full reasoning is a review comment on docs PR #48.
- **Next probe:** `node -e` against `check-appblocks-cli-snapshot.mjs`'s exports to
  re-confirm the `ahead` value is already available at classification time.

### `scripts/check-appblocks-pins.mjs:36-39` states a trigger that no longer exists
- **Observed, still present on docs `origin/main`:** *"It ALSO runs on a PR that
  touches package.json so a pin bump is verified against `latest` at review time."*
- **Why false:** #43 removed the `paths:` filter (so it ran on *every* PR, not only
  package.json ones), then #44 removed the PR invocation entirely. `check:pins` is now
  **schedule-only** via `appblocks-drift.yml:72-77`.
- **Ruled out:** not the comment #44 *did* fix — that was the sibling
  `check-design-system-pins.mjs:172-181`.
- **Next probe:** one comment edit pointing at `appblocks-drift.yml`. LOW.

### `check-appblocks-pins.mjs` exits 2 on a SemVer build-metadata `latest`
- **Observed (mechanism corrected — do not restate the old version):** `parseSemver`
  **returns `null`**, it does not throw. Regex unchanged on `main` at `:59`:
  `/^(\d+)\.(\d+)\.(\d+)(?:-(.+))?$/`. Measured:
  `parseSemver("0.31.0+build.7")` → `null`; `parseSemver("0.31.0")` → `{major:0,…}`.
  The throw is an explicit guard in `compareSemver` — `if (!pa || !pb) throw` — whose
  doc comment says "Unparseable -> throws". Intentional, not an oversight. It
  propagates to the catch at ~`:190` → **exit 2**.
- **Why it matters:** exit 2 is triggerable purely by *someone else's* publish, and the
  in-file comment lists the red paths without saying an upstream publish causes that one.
  `X.Y.Z+meta` is valid SemVer 2.0.0.
- **Next probe:** strip `+…` before compare, or document it. LOW.

### IA: the reference page is under `/apps/` and its body still presumes App authoring
- **Symptom:** `site/guide/cli.md` (public-API guide — searching/downloading models)
  links flag reference to `apps/reference/cli.md`, under `/apps/` (App *authoring*).
- **Observed — narrower than previously recorded.** The **frontmatter** was fixed by
  #42 and is now audience-neutral: *"The whole civitai CLI command tree — commands,
  flags, examples and the global flags…"*. The **body** was not
  (`apps/reference/cli.md:23-24`): *"is the canonical tool for **authoring Civitai
  Apps**"*, and `:36` *"App authors usually do"*.
- **Ruled out:** the llms-channel concern that used to ride along here — docs #45
  closed it (measured above).
- **Options, none chosen:** (a) reword the body's opening two sentences — cheapest, and
  now the *only* thing wrong with the framing; (b) alias/render `<CliReference />` under
  `/site/` too; (c) move the reference somewhere audience-neutral.
- **Next probe:** `git -C <docs> show origin/main:apps/reference/cli.md | sed -n '21,40p'`

## Next steps (ranked)

1. **Migrate README command-reference prose into `Long` / `Annotations`** — the actual
   answer to this session's question.
   🔴 **It is not a straight move, and the handoff previously under-stated this.**
   `Long` renders in `--help`; README reference cells run to **2,263 chars**. Dumping
   them into `Long` fixes the docs surface by wrecking the CLI's own. Decide the split
   FIRST — terminal-appropriate → `Long`; structured docs-only (the `--json` shapes,
   gh's `Annotations["help:json-fields"]` precedent) → `Annotations`, invisible to
   `--help`; narrative/tutorials → hosted pages, never generated. Then **pilot one
   command group** (`download` or the read commands) and measure `--help` before/after
   before touching ~50 files.
2. **Reword `apps/reference/cli.md:23-24`** — option (a) above, now the only remaining
   IA defect and a two-sentence change.
3. **Decide the freshness-guard gap** — the cheap informational option, or commit to
   content-hashing.
4. The two LOW items in `check-appblocks-pins.mjs` (stale header; `parseSemver`).
5. **Prune `claudedocs/`** last — other sessions actively write here; coordinate.

## Gotchas / decisions / dead-ends

- 🔴 **The `cli.json` export command is DISPROVED — do not re-propose it.** The widening
  it was meant to enable works with the existing scraper (53 nodes, 0 enumeration
  disagreements, 0.79s). `Long` recovers **54/54 byte-exact**; defaults **0 mismatches
  over 180 flags** — and an export would be *worse* there, because **24 flags carry
  their real effective default in authored prose that `DefValue` does not** (`--dir` →
  `./<slug>` vs `DefValue=""`). Cobra v1.8.1 → v1.10.2 help output: **0 byte-differences
  across 53 nodes**. `Annotations`/`GroupID`/`Deprecated` counts are **0/0/0**. A
  `go:generate` program *can* import `internal/cmd` (negative control: an external
  module cannot), so the published-binary-surface question was moot.
- 🔴 **`paths:` filters + required contexts = deadlock.** A required check whose workflow
  never triggers blocks a PR forever at `MERGEABLE/BLOCKED`. Self-inflicted here
  (protection added before checking the workflows always run); it blocked #42. Fixed in
  #43. **Rejected**: the common "dummy job that always succeeds with the same name"
  pattern — it makes a required check report green without running anything.
- 🔴 **The generator prefers a live `civitai` on PATH.** `~/.local/bin/civitai` is
  `v0.1.89-20-g4018e2c` (stale) and silently produced **47 commands instead of 52**,
  dropping `generate` and `workflows`. #42 made the snapshot the default with
  `CIVITAI_CLI_LIVE=1` opt-in **plus** a live-vs-snapshot diff. **Always build the
  binary from `origin/main` in a detached worktree when re-capturing**, and confirm it
  is not `-dirty` — a dirty build is evidence about a working copy and about no commit.
- 🔴 **The docs-side NUL repair at the capture seam MUST STAY** even though #264 fixed
  the source. The generator can capture from any binary, and `v0.1.90`-era builds still
  emit the NUL. Its test now covers a *historical fixture*, not current output — the
  header in `gen-appblocks-cli.mjs` says so; do not "clean it up".
- **#46's deletion was better-justified than it looked.** The `### Download flags` table
  listed **12** flags; `download` has **14**. The two missing were `--version` and
  `--yes` — *both* about the ambiguous-id safety stop — so the rotted rows were exactly
  the ones a reader who hits that stop needs.
- **A re-capture is never "just the one row".** Docs #48's diff carried 7 hunks across
  three CLI PRs (#242, #251, #264). Attribute every hunk to a commit, and cross-check
  the other direction (`git diff <old>..<new> -- internal/cmd/ ':!*_test.go'`) so a
  *missing* change is caught too.
- **zsh ate a variable:** `"$R:cli"` → the `:c` parsed as a history modifier, producing
  `/home/zach/workspace/civit/clili` with no error. Brace it: `${R}`.
- **Three mutants survived first runs, all the same shape** — `assertEnumerationsAgree`
  (correct but not wired into `buildArtifact`), the slot assertion (satisfied by a code
  *comment*), and "prefer live" (every test drove the pure helper, nothing reached
  `resolveBundle`). Component correct, wiring untested. **Mutate the CALL SITES.**
- **A single-line grep over wrapped prose gives a confident wrong answer.** "Item 24 is
  unreferenced" was wrong because `and item 24` / `covers the…` wraps across a newline.
  `agents_index_test.go` joins the region before matching for this reason.
- **A repeated failure is not a deterministic one.** `api/v1/articles/4797` returned 500
  four times over ~15 min, then recovered; the by-id route 404s correctly for missing ids
  and is healthy across the id space. No platform bug — do not re-file.
- **Read whole output, not `tail`.** Checking the flag-table guard with `tail -3` showed
  only one of the two pages it scans and read as "it doesn't cover the page it was built
  for". Full output shows both plus an 11-fixture detector control.

## How to verify

```bash
D=/home/zach/workspace/civit/civitai-developer-docs
R=/home/zach/workspace/civit/cli
git -C ${D} fetch origin -q && git -C ${R} fetch origin -q

# #264 — the NUL is gone at source, and the guard is in
git -C ${R} show origin/main:internal/cmd/login.go | command grep 'tokenFlagNoValue = '   # "(no value)"
cd ${R} && go test ./internal/cmd/ -run 'TestLoginHelp|TestTokenFlagNoValue' -count=1

# #48 — the committed snapshot matches, and is plain text
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt > /tmp/s.txt
tr -dc '\000' < /tmp/s.txt | wc -c            # expect 0
command grep -c 'civitai-token-no-value' /tmp/s.txt   # expect 0
command grep -c '^===CMD' /tmp/s.txt          # expect 106

# #46 — no hand-maintained flag tables (reads BOTH pages + a detector control)
cd ${D} && node scripts/check-no-hand-flag-tables.mjs   # read ALL output, not the tail

# #42 / #45 — the whole tree is published AND reaches the LLM channel
curl -s https://developer.civitai.com/llms-full.txt | command grep -c 'civitai generate'  # expect >0

# #243 / #247 — both AGENTS.md guards green
cd ${R} && go test ./ -run 'TestEveryAgentsItemIsNamedByTheIndex|TestAgents'

# protection is real (never infer gating from a job existing)
gh api repos/civitai/civitai-developer-docs/branches/main/protection --jq '.required_status_checks.contexts'

# the scheduled drift sweep stays green
gh run list --repo civitai/civitai-developer-docs --workflow appblocks-drift.yml --limit 3 \
  --json conclusion,event,createdAt
```
