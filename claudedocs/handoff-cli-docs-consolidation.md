# Handoff: cli-docs-consolidation — 2026-08-10 (r6, SHIPPED; the loop is closed and guarded)

Full pre-prune text (r5, 481 lines) is at `77bd422`. Nothing below was deleted for
length — only shipped STATUS was compressed. Every measured number survives.

## Goal and the answer

Should `civitai/cli`'s in-repo docs be dropped in favour of developer.civitai.com?
**No — relocate their content into the cobra command definitions.** 1,397 of
`README.md`'s 1,616 lines (86.4%) are prose no generator can produce, but the
*command-reference* prose can migrate into `Long`, where it becomes generatable AND
falls under a drift guard. Generating from `Short` instead would destroy ~80% of its
content: reference table cells measure 60–2,263 chars (median ~400) against `Short`
strings of 24–79.

## The scoping answer (next steps still depend on it)

Measured across all 53 command nodes by dumping the real cobra tree as JSON, not by
parsing help text:

| Fact | Measured |
|---|---|
| Nodes carrying an `Annotations` map | **0 of 53** |
| Nodes with NO `Long` at all | 8 → **now 0** (#274, #276) |
| Nodes with a `Long` under 400 chars | 16 → mostly closed |
| README table cells, 16 matched rows | 6,255 chars |
| `Long` on those same 16 nodes | **19,017 chars** |

`Long` was already ~3× richer than the README table for the App-authoring commands,
so that half of the "migration" was mostly already done. The real gap was the public
read-API group — no table row at all — which #276 closed.

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
space. **The surviving split is `Long` (prose, budgeted) vs the guide. Do not
re-propose an `Annotations` half.**

## What shipped — the pipeline is end-to-end and guarded

Twelve PRs, in pipeline order. Provenance is in git; these are kept only as pointers.

- **docs#49** publishes `longDescription` — the blocker that made everything else
  possible. **cli#274** (`app listing` pilot, 7 nodes) and **cli#276** (public
  read-API group, 21 nodes) are the migrated prose.
- **docs#51** made the build HERMETIC (OpenAPI spec committed, fetch opt-in);
  **docs#50** added the `build-site` job asserting the built CLI-reference HTML —
  now a **required** context.
- **docs#52** and **docs#56** are the two re-captures. Without a re-capture none of
  the CLI-side work is on the site (see the last mile, below). **docs#53** is the
  drift signal that would have told you.
- **cli#293** folded `cli#271`'s findings in and **auto-closed #271** via `Resolves`.
  **docs#54** fixed the IA wart, **docs#55** the two LOW `check-appblocks-pins.mjs`
  items, **docs#57** refreshed `check:spec-drift` (additive only, 18 insertions:
  `ComfyMiniMaxH3VideoGenInput.allOf[1].properties` gained `diffusionModel`/`loras`/
  `turbo`).

All three drift guards — `check:cli-snapshot`, `check:snapshots`, `check:spec-drift`
— now run clean against docs `main`. Every merge was verified **by content on
`origin/main`**, never by ancestry.

### 🔴 THE SNAPSHOT IS THE LAST MILE — and it has now failed TWICE for real

Writing `Long` in `civitai/cli` changes nothing on developer.civitai.com until the
docs repo re-captures `appblocks-snapshots/civitai-cli-help.txt`.

- **First failure, before #52.** The snapshot sat at `v0.1.90-25-g9cfe468` while
  both #274 and #276 had merged; `grep` for their prose returned **0 and 0** against
  a working positive control.
- **Second failure, before #56 — this is the proof, not a hypothetical.** The live
  site was serving `2.0 MB` in **10 occurrences across 8 lines** where the CLI says
  `MiB` (cli#282's `humanBytes` relabel). Re-captured at tag **v0.1.92**; the
  snapshot now carries 0 × `2.0 MB` and 10 × `MiB`.

**Prove a re-capture dropped nothing** with two checks, not one: the `===CMD` floor
(**106** at v0.1.92) and command-label-set IDENTITY (the set of command labels
before vs after must be equal — a count alone cannot see a swap).

Capture from a **clean build pinned by SHA, with real version ldflags** — a bare
`go build` off a `git archive` stamps `version=dev`, the base clone is dirty, and
`~/.local/bin/civitai` has silently produced **47 commands instead of 52**.

### 🔴 AND UNTIL #53 NOTHING TOLD YOU

`check-appblocks-cli-snapshot.mjs` parses `ahead` and `sha` out of the header and
then classifies on the **tag alone**; `main` moves daily and tags are rare, so a
snapshot 30 commits stale at the current tag verdicts `ok`. It said `ok` throughout
the first failure above. #53 adds `commitsSinceSnapshot`, printed on BOTH passing
verdicts — **reported, never failed**, because failing on every upstream commit is a
permanently-red daily gate. Measured live: the stale sha reports **12**, the current
one 3, a bogus sha `null`.

**Caveat as of now:** that line currently prints
`⊘ commits-behind unavailable (compare API unreachable) — tag verdict stands`
(`check-appblocks-cli-snapshot.mjs:213`). Degrading rather than failing is by design,
but it means the signal is **not giving a number right now** — do not read its
silence as freshness.

## Open investigations — live diagnosis state

### `check:built-site` cannot see a defect in the predicate it shares
- **Symptom:** the guard added by docs#50 passes while the rendered page is wrong.
- **Observed:** mutate `cliLongBody` itself to
  `return String(command?.description ?? '')`, rebuild → the page renders **52**
  `.ab-long` blocks each containing the one-line summary, and `check:built-site`
  reports **all 9 checks ok, rc=0**. Structural, not a bug: `want` is recomputed
  through the same function the component calls, so the two agree by construction.
- **Ruled out:** "the gate set is broken" — the already-required `test-cli` kills that
  mutant with 3 failures (`cliLongBody SUPPRESSES only…`, `synthetic controls for
  each branch`, `STRUCTURAL — the committed fences equal cliLongBody…`).
- **Also measured:** `grep -rn "vitepress build|npm run build" .github/workflows/`
  returned nothing before #50 — no job built the site at all. Two mutants #50 does
  kill that nothing else did: `{{ longText(c) }}` → `{{ c.description }}` (still 44
  blocks, so a count assertion misses it) and `v-html` (15 of 44 emit raw `<slug>`).
- **Leading hypothesis:** acceptable as-is; the risk is a reader believing
  `check:built-site` covers the predicate.
- **Next probe:** none needed to decide — the choice is whether to add a Vue test
  runner. If not, add one sentence at the check saying what it structurally cannot see.

### `README.md`'s `.modelVersions[]` and `poi` claims are wrong and UNVERIFIED
- **Symptom:** `README.md` ~741 says a `models search` hit "already embeds
  `.modelVersions[]` — every version's files, hashes and trained words"; ~747 says
  `.model` is "only {name, type, nsfw, poi}".
- **Observed:** `ModelListItem` (`pkg/civitai/models.go:23-30`) has **no**
  `modelVersions` field; `ModelVersionSummary` (:36-41) carries only
  `ID/Name/BaseModel/Files`; `TrainedWords` exists only on `ModelVersionDetail`
  (`model_versions.go:85`). `ModelVersionModel` is `{Name, Type, NSFW}` — `poi`
  appears **nowhere in the repo except that README line**.
- **Ruled out — do not "fix" by inference.** The CLI's structs are declared SUBSETS
  of the API payload, so a field's absence in Go is NOT evidence the API omits it.
  `cli#276` removed the claims from `--help` (where they had been retyped) and
  deliberately left the README alone.
- **Next probe, verbatim:**
  `civitai models search --limit 1 --json | jq '.items[0] | {hasVersions: has("modelVersions")}'`
  and `civitai model-versions get <id> --json | jq '.model | keys'`.
  (Line numbers above are claims about a tree that has moved — re-measure them.)

### `REQUEST_CONSENT` is NOT new, and must never join the dead-message denylist
Corrects a natural wrong assumption. Measured on docs `main`:
`hostHandlerParity.ts:177` gives `REQUEST_CONSENT` `PageBlockHost: 'required'` — the
**opposite** of `RESIZE_IFRAME`'s N/A-for-page row that `internal/antipattern`'s
`resize-iframe-page` rule is built on, so denylisting it would flag a live, required
message. The actual new thing is a host→block **push**,
`CONSENT_UNAVAILABLE { reason, scopes }` (`:170-176`), sent when requested scopes are
proven un-grantable; uncorrelated by design (no `requestId`, hence `reply: ''`).
**No CLI change needed** — `internal/blockproto`'s emitter registers no inbound
handlers, so only an SDK-backed app can consume it.

## Next steps (ranked)

1. **Decide the `.vue` question** — add a Vue test runner, or document what
   `check:built-site` structurally cannot see. One sentence either way.
2. **Settle the two README claims** with the one live `--json` call above. Needs a
   token; that is the only reason it is still open.
3. **The next migration bucket needs a BUDGET, not more prose.** After the pilot
   (7 nodes) and the read-API group (21), what remains is ~17 MID and 11 RICH — and
   the RICH ones are the problem: `generate` 5,726 chars, root 4,203, `download`
   3,742. Now that `Long` is published these land on BOTH surfaces, so the question
   changed: no longer "migrate prose in" but **"what belongs in `--help` versus the
   guide"**. Answer that before writing more.
4. **Prune the REST of `claudedocs/`** — 4,772 lines across 12 files, several stale.
   🔴 Other sessions actively own most of them (four were touched today by
   workstreams that are not this one). This needs coordination, **not unilateral
   deletion**.

### Residuals (known, deliberate, not bugs)
- `apps/reference/cli.md:11` still carries an App-authoring presumption — *"page's
  primary audience: App authors reach `app submit` more often than `civitai tags`"* —
  but it is a **frontmatter comment**, non-rendered. Left deliberately by docs#54,
  which fixed the two rendered body sentences.
- `check:cli-snapshot`'s commits-behind line is degrading to `⊘` (above).
- `docs#5` is OPEN and is another session's content — not mine to close.
- `docs/handoff-consolidation-r3` still exists as a branch (local and remote).
  `/tmp/wt-handoff3` is **gone**; r5 recorded it as live, and that is now false.

## Gotchas / decisions / dead-ends

### Measurement and instrument traps

- 🔴 **A SQUASH MERGE MAKES "unpushed commits" A FALSE ALARM.** Gating a worktree
  removal on `git log origin/main..HEAD | wc -l` "must be 0" returned **2** for a
  branch that HAD merged — a squash is a new commit with different parents, so that
  count can never reach 0 after one. **Verify by CONTENT** (diff the touched files
  against `main`), never by ancestry or commit count. Same rule as
  `git merge-base --is-ancestor`, false forever after a squash.
- 🔴 **`grep -c` counts LINES, not occurrences**, and the two disagree whenever a
  line repeats the token. It produced **"5"** and **"10"** for the same MiB fix from
  two people both measuring correctly. Use `grep -o … | wc -l` when the claim is
  about occurrences, and **say which one you counted**.
- 🔴 **zsh ate a variable AGAIN.** `$ref:scripts/foo.mjs` — the `:s` parsed as a
  history modifier, so `git show` failed and the check reported a confident
  **`0 occurrences`** assembled from failed commands. r5 already recorded this trap
  from `"$R:cli"` (which produced `/home/zach/workspace/civit/clili` with no error).
  **Knowing it did not prevent it.** Brace: `${ref}`.
- **"New diagnostics" can be pre-existing.** Two unused helpers were reported the
  moment a file entered analysis; both were byte-identically unused on `main` too.
  Compare against the base ref before attributing dead code to a change.
- **Instrument errors that produced confident wrong answers:** an overlap check that
  ran on two failed `git diff` commands and printed "no overlap" from empty input; a
  "before" binary built from the base clone instead of the worktree; `grep -c 'poi'`
  matching "check**poi**nt"; `npm run check:openapi-drift` exiting 1 because **the
  script is named `check:spec-drift`** — an rc=1 that reads exactly like a failing
  check; and a mutation harness whose `open(SRC,"w")` truncated the file BEFORE the
  mutation function ran, so a raising mutation left it empty. **Checksum-gate every
  mutation**, and pair every zero with a positive control.
- **A single-line grep over wrapped prose gives a confident wrong answer.** "Item 24
  is unreferenced" was wrong because `and item 24` / `covers the…` wraps across a
  newline. `agents_index_test.go` joins the region before matching for this reason.
- **Read whole output, not `tail`.** Checking the flag-table guard with `tail -3`
  showed only `apps/reference/cli.md` and read as "it doesn't cover the page it was
  built for". Full output shows both pages plus an 11-fixture detector control.
- **The obvious grep lies about workflow wiring.** `grep -c 'check:pins'
  appblocks-bridge.yml` returns **1**, which reads as "still wired to a PR" — it is a
  **comment**. Count `run:` lines instead (`grep -cE '^\s*run:.*check:pins'` → **0**).
  Scope to `run:` whenever the question is "is it actually wired".
- **A five-minute hang was measured as forty-five seconds** because the measurer
  stopped waiting. `copy-spec`'s fetch had no `AbortSignal.timeout` — the only
  `fetch(` in that repo without one — so a degraded host burned the job's full
  20-minute budget and reported a TIMEOUT. Run the failure to completion before
  quoting its duration.
- **A repeated failure is not a deterministic one.** `api/v1/articles/4797` returned
  500 four times over ~15 min, then recovered; the by-id route 404s correctly for
  missing ids and is healthy across the id space. No platform bug — do not re-file.
- **The shared base clone's `node_modules` drifts ahead of `package-lock.json`.** It
  makes `gen-appblocks-messages` / `gen-appblocks-bridge` fail with an SDK inventory
  error that looks like real drift. `npm ci` in your worktree is the discriminating
  control.

### Guards that look like coverage and are not

- 🔴 **A guard that compares a constant against itself cannot fail.** Three separate
  instances shipped and were caught only by audit: `want := limitRule(max)` (mutating
  `max`→`max+1` rendered "1–101" while the CLI enforced 100 — suite green);
  `Contains(Long, readAnonNote)` where the `Long` interpolates `readAnonNote` (the
  exact blanket auth claim #268 removed could be pasted back — green); and
  `Contains(Long, listingImageFormats)` likewise (narrowing to "png or jpeg" AND
  widening to claim avif both survived). Fix is always the same shape: assert against
  a LITERAL, or against BEHAVIOUR, never against the producer.
- 🔴 **A mutant killed by the WRONG guard reads as coverage.** An auth mutant looked
  dead; it was the BUDGET guard firing, because the rewrite pushed a body to 1413
  runes. The length-neutral version survived. Always check WHICH assertion fired.
- 🔴 **Two guards can share a blind spot when they share a cap.**
  `humanBytes(maxIconBytes) == humanBytes(maxScreenshotBytes) == "2.0 MB"`, so three
  `Contains` checks over three strings enforced TWO of three caps. Pair each value
  with its label. The icon↔screenshot swap is a declared EQUIVALENT mutant —
  different binary, byte-identical help — proven with `cmp` plus the differing
  binaries as the negative control.
- 🔴 **Count RUNES, not bytes, in a column guard.** Em-dashes are 3 bytes and one
  column; a byte count failed four bodies that render inside 80 columns.
- 🔴 **A test fixture must not mutate global state.** Importing `image/gif` for a
  negative control ran its `init()`, registered the decoder process-wide, changed how
  `generate --image` refuses a GIF, and reddened three unrelated tests. Build the
  bytes by hand.
- 🔴 **A gate can be satisfied by following its own remedy.** Blanking the `.md`
  renderer's call site failed `check:md-regions`, whose message says "regenerate and
  commit the page diff" — do that and EVERYTHING goes green while 41,109 bytes vanish
  from the LLM channel. A floor on the COMMITTED artifact is what closes it.
- **Mutate the CALL SITES, not just the function.** Three mutants survived first
  runs, all the same shape — `assertEnumerationsAgree` (correct but not wired into
  `buildArtifact`), the slot assertion (satisfied by a code *comment*), and "prefer
  live" (every test drove the pure helper, nothing reached `resolveBundle`). Component
  correct, wiring untested.
- **A guard was deliberately WEAKENED, and that trade is recorded rather than
  hidden.** In docs `scripts/test-appblocks-cli.mjs` the structural separator check
  went from `examples.length === 11` to `length > 0 && blanks >= 3`, because
  `app create` grew a fifth scenario (cli#267's `--slug`) and two literals had to move
  together. The exact pin still lives in the sibling CONTENT check (14 lines =
  10 content + 4 blank separators). Net coverage preserved, but this is a
  **loosening**, not a neutral refactor.

### Pipeline decisions and disproofs

- 🔴 **The `cli.json` export command is DISPROVED — do not re-propose it.** The
  widening it was meant to enable works with the existing scraper (53 nodes, 0
  enumeration disagreements, 0.79s). `Long` recovers **54/54 byte-exact**; defaults
  **0 mismatches over 180 flags** — and an export would be *worse* there, because
  **24 flags carry their real effective default in authored prose that `DefValue`
  does not** (`--dir` → `./<slug>` vs `DefValue=""`). Cobra v1.8.1 → v1.10.2 help
  output: **0 byte-differences across 53 nodes**. `Annotations`/`GroupID`/`Deprecated`
  counts are **0/0/0**. A `go:generate` program *can* import `internal/cmd` (negative
  control: an external module cannot), so the published-binary-surface question was
  moot.
- 🔴 **`paths:` filters + required contexts = deadlock.** A required check whose
  workflow never triggers blocks a PR forever at `MERGEABLE/BLOCKED`. Self-inflicted
  here (protection added before checking the workflows always run); it blocked #42.
  Fixed in #43 by removing the filters. **Rejected**: the common "dummy job that
  always succeeds with the same name" pattern — it makes a required check report
  green without running anything.
- 🔴 **The generator prefers a live `civitai` on PATH — AND IT BIT US AGAIN TODAY.**
  `~/.local/bin/civitai` is stale (`v0.1.89-20-g4018e2c`) and silently produced
  **47 commands instead of 52**, dropping `generate` and `workflows`. #42 made the
  snapshot the default with `CIVITAI_CLI_LIVE=1` opt-in **plus** a live-vs-snapshot
  diff. This is a **recurring** trap, not a closed one. Also: `cli/bin/civitai` is
  rebuilt by other sessions and is often `-dirty` — a binary built from a dirty tree
  is evidence about that working copy and about no commit.
- 🔴 **The docs-side NUL repair at the capture seam MUST STAY**, even though cli#264
  fixed it at source. The generator can capture from *any* binary, and `v0.1.90`-era
  builds still emit the NUL. Its test now covers a **historical fixture**, not current
  output — the header in `gen-appblocks-cli.mjs` says so. Do not "clean it up" on the
  strength of the source fix.
- **A re-capture is never "just the one row".** docs#48's diff carried **7 hunks**
  across three CLI PRs (#242, #251, #264). Attribute every hunk to a commit, and
  cross-check the other direction
  (`git diff <old>..<new> -- internal/cmd/ ':!*_test.go'`) so a *missing* change is
  caught too — a re-capture that silently drops a change looks identical to one that
  had nothing to carry.
- **#46's deletion was better-justified than it looked.** The `### Download flags`
  table listed **12** flags; `download` has **14**. The two missing were `--version`
  and `--yes` — *both* about the ambiguous-id safety stop — so the rotted rows were
  exactly the ones a reader who hits that stop needs. Every one of the 12 was
  confirmed present in the generated artifact before deleting.
- **The derived-not-typed rule PAID OFF, measured.** cli#282 relabelled `humanBytes`
  from `MB` to `MiB`. Because `listingSourceRule` computes its cap from
  `maxIconBytes` through that same function, `civitai app listing set-icon --help`
  **followed automatically** — `at most 2.0 MiB` with no edit to any help body; all
  five listing guards still exist and 86 subtests pass. A hand-typed "2 MiB" would
  have silently disagreed with the code. (The docs snapshot did NOT follow
  automatically — that is what docs#56 had to fix.)
- **`parseSemver` turned someone else's npm publish into `exit 2`** (fixed in
  docs#55). Mechanism measured, and it is not the one an earlier write-up gave:
  `parseSemver("0.31.0+build.7")` **returned `null`** (it did not throw); the throw
  was `compareSemver`'s explicit `if (!pa || !pb) throw`; and `compareSemver` is
  called **outside** `fetchLatest`'s try/catch, so it escaped to `main().catch` →
  `process.exit(2)`. A scheduled job could therefore go red purely because a third
  party published `X.Y.Z+meta`, which is valid SemVer 2.0.0. The pin side could never
  reach it — `readPinnedVersion` rejects a non-exact pin first. **The fix has its own
  trap:** build metadata is stripped with `-([^+]+)` for the prerelease *plus* a
  `(?:\+.+)?` tail — the greedy `-(.+)` swallows `+build.7` into `pre` and mis-sorts
  two builds of one prerelease. Both halves load-bearing.
- **A handoff's line numbers are claims about a tree that has moved.** Folding
  `cli#271` in, every line number was re-measured and **three had drifted**
  (`apps/reference/cli.md:23-24`→`:24`, `:36`→`:37`; `parseSemver` `:59`→`:58`).
  Re-measure; do not transcribe. `cli#271` itself was resolved by **extraction**, not
  merge — its branch was checked out in a worktree this session did not own, and
  taking `main` as the base reaches the same file contents with no shared-checkout
  write. It auto-closed on cli#293's `Resolves` keyword.

## How to verify

```bash
D=/home/zach/workspace/civit/civitai-developer-docs
R=/home/zach/workspace/civit/cli
git -C ${D} fetch origin -q && git -C ${R} fetch origin -q

# 🔴 THE LAST MILE: is the SITE actually carrying the CLI's current help?
# A zero here means the snapshot is stale, not that the prose is missing.
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -m1 'Binary version'   # v0.1.92
git -C ${R} describe --tags origin/main    # if this is ahead, re-capture (see #52 / #56)
# floors that prove a re-capture dropped nothing (a count alone cannot see a swap —
# also compare the SET of command labels before vs after):
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -c '===CMD'            # expect 106
# positive control for the greps above:
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -c 'Manage your App store listing'  # expect 1

# the cli#282 MiB relabel actually reached the site. NOTE grep -o | wc -l:
# `grep -c` would answer 8 (lines), not 10 (occurrences).
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -o '2\.0 MB' | wc -l   # expect 0
git -C ${D} show origin/main:appblocks-snapshots/civitai-cli-help.txt | command grep -o 'MiB'    | wc -l   # expect 10

# the generator still publishes Long — the whole pipeline's blocker (docs#49)
git -C ${D} show origin/main:scripts/gen-appblocks-cli.mjs | command grep -c longDescription  # expect >=1

# the drift signal #53 added — the tag check alone CANNOT see staleness.
# Currently prints "⊘ commits-behind unavailable" when the compare API is down;
# that is a DEGRADED signal, not a fresh one.
cd ${D} && npm ci --ignore-scripts >/dev/null && node scripts/check-appblocks-cli-snapshot.mjs
# positive control that the number is real, not a constant:
cd ${D} && node -e "import('./scripts/check-appblocks-cli-snapshot.mjs').then(async m => {
  console.log('stale sha 9cfe468 ->', await m.commitsSinceSnapshot('9cfe468'));  // a LARGE number
  console.log('bogus sha         ->', await m.commitsSinceSnapshot('0000000'));  // must be null
})"

# all three drift guards (they run clean on docs main as of 2026-08-10)
cd ${D} && npm run check:cli-snapshot && npm run check:snapshots && npm run check:spec-drift
#   NB the script is `check:spec-drift`. `check:openapi-drift` does not exist and
#   exits 1 — an rc=1 that reads exactly like a failing check.

# 🔴 Annotations are invisible to the docs pipeline — re-measure before believing
# any plan that relies on them. Set one on any command, rebuild, then:
#   strings <bin> | grep -c ZZPROBEZZ      -> 1   (the probe applied)
#   <bin> <cmd> --help | grep -c ZZPROBEZZ -> 0   (it never reaches the channel)
#   <bin> <cmd> --help | grep -c <a word actually in that help> -> 1  (control)

# the NUL at the capture seam — FIXED in cli#264; expect 0 now. The positive control
# matters: a pipe that reports 0 because it is wired to nothing looks identical to a
# fixed binary. (The docs-side repair still MUST stay — see gotchas.)
${R}/bin/civitai login --help | tr -dc '\000' | wc -c   # expect 0
printf 'a\000b' | tr -dc '\000' | wc -c                 # expect 1 (control)

# the help guards, both migrated groups
cd ${R} && go test ./internal/cmd -run TestListingHelp -count=1 -v | grep -c -- '--- PASS'   # app listing
cd ${R} && go test ./internal/cmd -run TestReadAPI     -count=1 -v | grep -c -- '--- PASS'   # read API

# protection is real (never infer gating from a job existing)
gh api repos/civitai/civitai-developer-docs/branches/main/protection \
  --jq '.required_status_checks.contexts'   # expect […, "build-site"]

# 🔴 LOCAL-RUN TRAP: the shared base clones' node_modules have drifted ahead of
# package-lock.json. Three docs tests fail identically on PRISTINE origin/main
# until you `npm ci` in your own worktree. Run that discriminating control before
# believing a local red.
```
