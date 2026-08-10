# Handoff: cli-docs-consolidation — 2026-08-09 (r4, SHIPPED + VERIFIED)

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

## State now (2026-08-09, end of session)

- **Both repos clean; nothing of this work is in flight.** `civitai/cli` @
  `c5c3817`, `civitai-developer-docs` @ `2426938`. Both moved MANY times during
  the session — **pin a SHA before measuring anything.**
- **8 PRs merged** (table below). `build-site` is now a **required** context on
  the docs repo.
- **Open, neither mine to close:** `cli#271` (DIRTY — see the investigation
  block) and `docs#5` (CLEAN, unblocked, another session's content).

### 🔴 THREE PARALLEL SESSIONS ARE EDITING `internal/cmd/app_listing.go`

Live worktrees, all touching the file `cli#274` rewrote:

| worktree | branch | also touches |
|---|---|---|
| `cli-270-listing-docs` | `zach/270-listing-media-docs` | `app_listing.go`, `listing_media_docs_test.go` |
| `cli-270-attach-order` | `zach/270-attach-before-scan` | `app_listing.go`, `appapi/listing.go` |
| `cli-270-docrot-humanbytes` | `zach/270-readme-generalise-and-humanbytes` | **merged as #282** |

**Do not edit `app_listing.go` without checking those first**, and never in the
base clone. `cli#271`'s branch is checked out in `/tmp/wt-handoff3`, which is why
its conflict was left to its owner rather than resolved.

### The derived-not-typed rule PAID OFF, measured

`#282` (another session) relabelled `humanBytes` from `MB` to `MiB`. Because
`listingSourceRule` computes its cap from `maxIconBytes` through that same
function, `civitai app listing set-icon --help` **followed automatically** —
it now reads `at most 2.0 MiB` with no edit to any help body. Verified on
`c5c3817`: all five listing guards still exist and **86 subtests pass**. A
hand-typed "2 MiB" would have silently disagreed with the code instead.

## The scoping answer (kept — next steps still depend on it)

Measured across all 53 command nodes by dumping the real cobra tree as JSON, not
by parsing help text:

| Fact | Measured |
|---|---|
| Nodes carrying an `Annotations` map | **0 of 53** |
| Nodes with NO `Long` at all | 8 → **now 0** (#274, #276) |
| Nodes with a `Long` under 400 chars | 16 → mostly closed |
| README table cells, 16 matched rows | 6,255 chars |
| `Long` on those same 16 nodes | **19,017 chars** |

`Long` was already ~3× richer than the README table for the App-authoring
commands, so that half of the "migration" was mostly already done. The real gap
was the public read-API group — no table row at all — which #276 closed.

### 🔴 `Annotations` CANNOT reach the docs site — measured, not inferred

The generator captures exactly two channels per node (`civitai <path> --help` and
`civitai __complete <path> ""`) and parses the text. Cobra's default help template
renders `Annotations` in neither. Probe on `workflows get` with
`Annotations{"probe:visible-in-help": "ZZPROBEZZ"}`:

- `strings <binary> | grep -c ZZPROBEZZ` → **1** (the probe really applied)
- in `--help` → **0**; in `__complete` → **0**
- positive control, a word that IS in that help (`PRESIGNED`) → **1**
- `workflows get --help` byte-identical with and without it (1512 both)

So an `Annotations` half is a no-op for docs unless a custom help template renders
them — at which point they are `Long` with extra steps and cost the same terminal
space. **The surviving split is `Long` (prose, budgeted) vs the guide.**

## SHIPPED — the pipeline is end-to-end (2026-08-09)

Eight PRs merged; the loop this workstream was about is closed AND guarded.

| PR | Repo | What |
|---|---|---|
| #49 | docs | generator publishes `longDescription` — the blocker |
| #274 | cli | `app listing` pilot, 7 nodes |
| #51 | docs | build is HERMETIC — OpenAPI spec committed, fetch opt-in |
| #276 | cli | public read-API group, 21 nodes |
| #50 | docs | `build-site` job asserting the built CLI-reference HTML |
| #52 | docs | re-capture the snapshot — **without this none of the above is on the site** |
| #277 | cli | handoff r3 |
| #53 | docs | commits-behind drift signal (below) |

Also: `build-site` **promoted to a required context**; `docs#5` was collaterally
blocked by that and unblocked again (see the investigation block).

Every merge verified **by content on `origin/main`**, never by ancestry — a
squash merge makes `git merge-base --is-ancestor` false forever, so the ancestry
check reads as "not merged" and is wrong.

🔴 **THE SNAPSHOT IS THE LAST MILE AND IT IS EASY TO FORGET.** Writing `Long` in
`civitai/cli` changes nothing on developer.civitai.com until the docs repo
re-captures `appblocks-snapshots/civitai-cli-help.txt`. Measured before #52: the
snapshot sat at `v0.1.90-25-g9cfe468` while both #274 and #276 had merged, and
`grep` for their prose returned **0 and 0** against a working positive control.
Capture from a **clean build pinned by SHA, with real version ldflags** — a bare
`go build` off a `git archive` stamps `version=dev`, the base clone is dirty, and
`~/.local/bin/civitai` has silently produced **47 commands instead of 52**.

🔴 **AND UNTIL #53 NOTHING TOLD YOU.** `check-appblocks-cli-snapshot.mjs` parses
`ahead` and `sha` out of the header and then classifies on the **tag alone**;
`main` moves daily and tags are rare, so a snapshot 30 commits stale at the
current tag verdicts `ok`. It said `ok` throughout the failure above. #53 adds
`commitsSinceSnapshot`, printed on BOTH passing verdicts — **reported, never
failed**, because failing on every upstream commit is a permanently-red daily
gate. Measured live: the stale sha reports **12**, the current one 3, a bogus sha
`null`. Watch that line in the scheduled `appblocks-drift` run.

## Open investigations — live diagnosis state

### `cli#271` conflicts with `#277`; its branch is checked out in a LIVE worktree
- **Symptom + repro:** `gh pr view 271 --repo civitai/cli --json mergeStateStatus`
  → `DIRTY`. `git merge-tree --write-tree pr271 origin/main` → **rc=1** (branch on
  the EXIT CODE, never a marker grep — `merge-tree` prints only a tree OID on
  success and emits no `<<<<<<<`).
- **Observed:** #271 was cut from `aeceb6b`, the same base #277 started from, and
  rewrites 144/131 lines of `claudedocs/handoff-cli-docs-consolidation.md`.
  Its branch `docs/handoff-consolidation-r3` is checked out at **`/tmp/wt-handoff3`**
  — `git worktree add` refuses it, and writing there would clobber a live session.
- **Content of #271 that this file does NOT have** (must survive the merge):
  (1) the snapshot-freshness gap — **already extracted and shipped as docs#53**;
  (2) `llms-full.txt` carries the CLI reference now, **0 → 15** `civitai generate`
  mentions after docs#45, so the llms concern is CLOSED, not open/INFERRED;
  (3) the IA finding is narrower than recorded here — #42 fixed the frontmatter,
  only `apps/reference/cli.md:23-24` and `:36` still presume App authoring.
- **Ruled out:** resolving it here. Not a technical block — a concurrency one.
- **Next probe:** once `/tmp/wt-handoff3` is free,
  `git -C /tmp/wt-handoff3 merge origin/main`, take `main`'s version as the base
  and splice in (2) and (3). Do NOT take #271's "State now"/"DONE" tables — they
  predate all eight merges. Analysis is posted as a comment on the PR.

### `check:built-site` cannot see a defect in the predicate it shares
- **Symptom:** the guard added by docs#50 passes while the rendered page is wrong.
- **Observed:** mutate `cliLongBody` itself to `return String(command?.description ?? '')`,
  rebuild → the page renders **52** `.ab-long` blocks each containing the one-line
  summary, and `check:built-site` reports **all 9 checks ok, rc=0**. Structural, not
  a bug: `want` is recomputed through the same function the component calls, so
  the two agree by construction.
- **Ruled out:** "the gate set is broken" — the already-required `test-cli` kills
  that mutant with 3 failures (`cliLongBody SUPPRESSES only…`, `synthetic controls
  for each branch`, `STRUCTURAL — the committed fences equal cliLongBody…`).
- **Also measured:** `grep -rn "vitepress build|npm run build" .github/workflows/`
  returned nothing before #50 — no job built the site at all. Two mutants #50 does
  kill that nothing else did: `{{ longText(c) }}` → `{{ c.description }}` (still 44
  blocks, so a count assertion misses it) and `v-html` (15 of 44 emit raw `<slug>`).
- **Leading hypothesis:** acceptable as-is; the risk is a reader believing
  `check:built-site` covers the predicate.
- **Next probe:** none needed to decide — the choice is whether to add a Vue test
  runner. If not, add one sentence at the check saying what it structurally cannot
  see.

### `README.md`'s `.modelVersions[]` and `poi` claims are wrong and UNVERIFIED
- **Symptom:** `README.md` ~741 says a `models search` hit "already embeds
  `.modelVersions[]` — every version's files, hashes and trained words"; ~747 says
  `.model` is "only {name, type, nsfw, poi}".
- **Observed:** `ModelListItem` (`pkg/civitai/models.go:23-30`) has **no**
  `modelVersions` field; `ModelVersionSummary` (:36-41) carries only
  `ID/Name/BaseModel/Files`; `TrainedWords` exists only on `ModelVersionDetail`
  (`model_versions.go:85`). `ModelVersionModel` is `{Name, Type, NSFW}` — `poi`
  appears **nowhere in the repo except that README line**.
- **Ruled out — do not "fix" by inference.** The CLI's structs are declared
  SUBSETS of the API payload, so a field's absence in Go is NOT evidence the API
  omits it. `cli#276` removed the claims from `--help` (where they had been
  retyped) and deliberately left the README alone.
- **Next probe, verbatim:**
  `civitai models search --limit 1 --json | jq '.items[0] | {hasVersions: has("modelVersions")}'`
  and `civitai model-versions get <id> --json | jq '.model | keys'`.

## Next steps (ranked)

1. 🔴 **Resolve `cli#271`** — see the investigation block. It carries two measured
   findings this file lacks, and it is the only thing left that can lose work.
2. **Decide the `.vue` question** — add a Vue test runner, or document what
   `check:built-site` structurally cannot see. One sentence either way.
3. **Settle the two README claims** with the one live `--json` call above.
4. **The next migration bucket needs a BUDGET, not more prose.** After the pilot
   (7 nodes) and the read-API group (21), what remains is ~17 MID and 11 RICH —
   and the RICH ones are the problem: `generate` 5,726 chars, root 4,203,
   `download` 3,742. Now that `Long` is published these land on BOTH surfaces, so
   the question changed: no longer "migrate prose in" but "what belongs in
   `--help` versus the guide". Answer that before writing more.
5. **Do NOT add an `Annotations` half.** Measured disproof above.
6. **Decide the IA wart** (below) — cheapest is option (c), and #271 narrows it to
   two sentences in the page BODY (the frontmatter was fixed by #42).
7. The two LOW items below (pins header comment; `parseSemver` build metadata).
8. **Prune `claudedocs/`** — this file is 400+ lines and other sessions write here
   too. Coordinate before pruning.

## Gotchas / decisions / dead-ends

### From the 2026-08-09 shipping session — every one cost a round

- 🔴 **A guard that compares a constant against itself cannot fail, and it LOOKS
  like coverage.** Three separate instances shipped and were caught only by
  audit: `want := limitRule(max)` (mutating `max`→`max+1` rendered "1–101" while
  the CLI enforced 100 — suite green); `Contains(Long, readAnonNote)` where the
  Long interpolates `readAnonNote` (the exact blanket auth claim #268 removed
  could be pasted back — green); and `Contains(Long, listingImageFormats)`
  likewise (narrowing to "png or jpeg" AND widening to claim avif both survived).
  The fix is always the same shape: assert against a LITERAL, or against
  BEHAVIOUR, never against the producer.
- 🔴 **A mutant killed by the WRONG guard reads as coverage.** An auth mutant
  looked dead; it was the BUDGET guard firing, because the rewrite pushed a body
  to 1413 runes. The length-neutral version survived. Always check WHICH
  assertion fired.
- 🔴 **Two guards can share a blind spot when they share a cap.**
  `humanBytes(maxIconBytes) == humanBytes(maxScreenshotBytes) == "2.0 MB"`, so
  three `Contains` checks over three strings enforced TWO of three caps. Pair
  each value with its label. And the icon↔screenshot swap is a declared
  EQUIVALENT mutant — different binary, byte-identical help — proven with `cmp`
  plus the differing binaries as the negative control.
- 🔴 **Count RUNES, not bytes, in a column guard.** Em-dashes are 3 bytes and one
  column; a byte count failed four bodies that render inside 80 columns.
- 🔴 **A test fixture must not mutate global state.** Importing `image/gif` for a
  negative control ran its `init()`, registered the decoder process-wide, changed
  how `generate --image` refuses a GIF, and reddened three unrelated tests. Build
  the bytes by hand.
- 🔴 **A gate can be satisfied by following its own remedy.** Blanking the `.md`
  renderer's call site failed `check:md-regions`, whose message says "regenerate
  and commit the page diff" — do that and EVERYTHING goes green while 41,109
  bytes vanish from the LLM channel. A floor on the COMMITTED artifact is what
  closes it.
- **Instrument errors that produced confident wrong answers, all mine:** an
  overlap check that ran on two failed `git diff` commands and printed "no
  overlap" from empty input; a "before" binary built from the base clone instead
  of the worktree; `grep -c 'poi'` matching "check**poi**nt"; `npm run
  check:openapi-drift` exiting 1 because **the script is named
  `check:spec-drift`** — an rc=1 that reads exactly like a failing check; and a
  mutation harness whose `open(SRC,"w")` truncated the file BEFORE the mutation
  function ran, so a raising mutation left it empty. **Checksum-gate every
  mutation**, and pair every zero with a positive control.
- **The shared base clone's `node_modules` drifts ahead of `package-lock.json`.**
  It makes `gen-appblocks-messages` / `gen-appblocks-bridge` fail with an SDK
  inventory error that looks like real drift. `npm ci` in your worktree is the
  discriminating control.
- **A five-minute hang was measured as forty-five seconds** because the measurer
  stopped waiting. `copy-spec`'s fetch had no `AbortSignal.timeout` — the only
  `fetch(` in that repo without one — so a degraded host burned the job's full
  20-minute budget and reported a TIMEOUT. Run the failure to completion before
  quoting its duration.

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

# the help guards, both groups (merged)
git -C ${R} fetch origin -q
go test ./internal/cmd -run TestListingHelp -count=1 -v | grep -c -- '--- PASS'   # app listing
go test ./internal/cmd -run TestReadAPI     -count=1 -v | grep -c -- '--- PASS'   # read API

# 🔴 THE LAST MILE: is the SITE actually carrying the CLI's current help?
# A zero here means the snapshot is stale, not that the prose is missing.
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -m1 'Binary version'
git -C ${R} describe --tags origin/main    # if this is ahead, re-capture (see #52)
# positive control for the grep above:
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -c 'Manage your App store listing'  # expect 1

# 🔴 Annotations are invisible to the docs pipeline — re-measure before believing
# any plan that relies on them. Set one on any command, rebuild, then:
#   strings <bin> | grep -c ZZPROBEZZ      -> 1   (the probe applied)
#   <bin> <cmd> --help | grep -c ZZPROBEZZ -> 0   (it never reaches the channel)
#   <bin> <cmd> --help | grep -c <a word actually in that help> -> 1  (control)

# 🔴 RETRACTED — "the generator drops Long" was true and is FIXED by docs#49.
# It now emits `longDescription` alongside an untouched `description`:
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c longDescription  # expect >=1

# the site actually carries this session's prose (0 here = stale snapshot)
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -c 'Set your store listing'  # expect 2  (#274)
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -c 'No login is needed'      # expect 8  (#276)

# the drift signal #53 added — the tag check alone CANNOT see staleness
cd ${D} && npm ci --ignore-scripts >/dev/null && node scripts/check-appblocks-cli-snapshot.mjs
#   expect rc=0 and a line reading "N civitai/cli commit(s) have landed since…"
#   positive control that the number is real, not a constant:
cd ${D} && node -e "import('./scripts/check-appblocks-cli-snapshot.mjs').then(async m => {
  console.log('stale sha 9cfe468 ->', await m.commitsSinceSnapshot('9cfe468'));  // a LARGE number
  console.log('bogus sha         ->', await m.commitsSinceSnapshot('0000000'));  // must be null
})"

# build-site IS required now (re-measure; never infer gating from a job existing)
gh api repos/civitai/civitai-developer-docs/branches/main/protection \
  --jq '.required_status_checks.contexts'   # expect […, "build-site"]

# 🔴 LOCAL-RUN TRAP: the shared base clones' node_modules have drifted ahead of
# package-lock.json. Three docs tests fail identically on PRISTINE origin/main
# until you `npm ci` in your own worktree. Run the discriminating control before
# believing a local red.
```
