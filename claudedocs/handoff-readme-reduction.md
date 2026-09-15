# Handoff: readme-reduction — 2026-09-15

## Run this first — the index, one command
```bash
cairn recall --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Make `README.md` high-level: trim stale/maintainer noise, compress the two reference
tables that have grown essay-length cells, reorganise so the reader paths stop
interleaving, and link out what does not belong in a shipped file.

closing-condition: check — `go test ./internal/cmd/ -run
'Attribution|Troubleshooting|README|Readme|readme' -count=1` exits 0 having run **>0**
tests, AND `go test ./...` is green, AND each remaining named phase has landed or been
withdrawn by a merged PR. Mechanical; no byte threshold.

🔴 **THE BYTE THRESHOLD WAS DELETED ON 2026-09-15, AND THIS IS THE THIRD AND FINAL
AMENDMENT TO IT — DO NOT RE-DERIVE IT.** Its history is the argument against it: set at
**250,000**, moved to **270,000** after the phase-3 trial edit, provisionally moved to
**276,000** after the phase-5 measurement, then dropped entirely when the change was run
through `the-algorithm`. It was set three times by estimate and missed twice. Step 1 of
that skill asks who *made* the requirement; the answer was a prior agent session, which
the skill names as "a department, not a maker". Nothing in the repo, no user and no
incident ever asked `README.md` to be a particular size — the arc's real goal, in this
doc's own words, is that it be *high-level*, and the byte count was a proxy for that
which drifted into being mistaken for the thing.

⚠ **The gate-filter amendment still stands and is NOT part of that retraction.** The
filter was `-run 'README|Readme|readme'`, which matches **zero** of the guards on the
Troubleshooting table's two frozen cells — measured, and confirmed by a live mutant the
narrow filter reported `ok` on. Use the wide filter above. Measured 2026-09-15 it runs
**76** tests; a run reporting `ok` with 0 tests is the failure this replaced.

🔴 **The full measured plan is `claudedocs/readme-reduction-plan.md`** — constraint
map, size map, cut list, compression targets, link-out policy, sequencing. Read it
before touching prose. This doc is state; that doc is the plan.

## State now

**Five PRs merged this session. README is 282,312 bytes on `main` — down 28,335 (−9.1%) from the arc's start. The byte threshold that used to sit here is GONE (see the closing condition above); size is still reported because it is the cheapest progress signal, but it is no longer a gate and no PR should be shaped to hit a number.**

| | bytes |
|---|---:|
| arc start (`426288f`) | 310,647 |
| after phase 1 — accuracy, #611 | 314,712 |
| after phase 3 — Troubleshooting, #625 | 305,329 |
| after phase 2 — cuts, #630 | 293,378 |
| after phase 3b — command table, #633 | 282,312 |
| **on `main` now** | **282,312** |
| phase 3c in flight (#635) | 280,571 |

### Merged
- **#611** (`f284d89`) phase 1, accuracy. Six corrections each pinned by a guard watched to fail, **plus a behaviour fix**: `civitai upgrade` now derives the release-asset extension per GOOS and works on Windows. Closed **#613** (Windows `upgrade`). Four audit rounds.
- **#625** Troubleshooting 35,250 → 19,182. Symptom column byte-identical.
- **#630** phase 2 cuts, −14,279, **give-back 0** — the PR that proved delete-first.
- **#633** command table 21,391 → 9,664 (−54.8%), **plus a new guard**: `TestREADMECommandSynopsesNameRealFlags` drives the live Cobra tree, 25 rows / 33 resolutions / 90 flag checks, floors 30/85.
- **#615** the automated pin bump (not this session's) — unfroze the repo and satisfied the previous arc's cli#530 closing condition.

### In flight
- **#635 `fix/readme-submit-preamble` @ `d1361c3`** — CLEAN, 13/13 green, **no audit round run yet**. Preamble 29,500 → 27,759 (−1,741), give-back 0. Repairs two claims that were false on `main` before this arc.
- **#623 `docs/readme-reduction-plan`** — the plan + this handoff. CLEAN, unmerged. 🔴 **The plan on `origin/main` is STALE; read the branch copy.**
- **#602** (another session's) — `CONFLICTING`/`DIRTY`, 17+ commits behind, conflicts with merged work *on its own*. A collision note is posted on it.
- **#614** open and optional — the `CIVITAI_NO_COLOR` value-parsing asymmetry, documented and guarded.

### Honest limits
- 🔴 **Nobody has run `civitai upgrade` on Windows.** Every branch is exercised against the real published v0.1.105 zip, and `minio/selfupdate` has a real Windows implementation, but the final self-replace is stubbed behind the `applyUpdate` seam — as it was for every platform before #611.
- **The estimates have under-delivered three times running**, each because the compressible fraction was smaller than a whole-section byte count suggested: Troubleshooting 13,000→20,229 (7,081 B of the section is not cause cells), phase 2 20,500→14,279, preamble 6,000→1,741 (35% of it is a golden file, another 20% pinned against live stdout).
- **`--session` sees almost nothing of this work** — every file went through subagent worktrees. Use `--pr` for the index window.

## Open investigations — live diagnosis state

🔴 **THIS SECTION IS APPEND-ONLY, SO A HEADING ALONE IS NOT A STATUS. READ THE
HEADING PREFIX.** When you append a RESOLVED block, retire the superseded heading in
the SAME edit.

### ⚠ OPEN — does a public model file actually require a token?
- as-of: 2026-09-15

`README.md` states every model-file download needs a token, "even a small public
embedding 401s anonymously" (three surfaces). `internal/cmd/download.go`'s `Long` and
its error string both say **most** do and some public files do not. The source file
also disagrees with itself: a comment near the error says "ANY model file".

- **Ruled out:** that one side is obviously stale — both were written deliberately and
  neither is pinned by a test. `via: code`
- **Next probe:** one live anonymous download of a small public embedding. `curl` the
  file URL with no token and read the status. That single measurement settles it.
- 🔴 Whichever way it resolves, fix the losing side **and** pin it, or the two will
  drift apart again. No test pins either side today.

## Next steps (ranked)

0. ~~**DECIDE THE CLOSING CONDITION.**~~ **CLOSED 2026-09-15** — the threshold was deleted, not re-set. See the closing condition at the top and the two retraction blocks under Gotchas. The measurement that killed option (a) is recorded below as "what phase 5 can actually yield".
1. **Trial-cut `## Set up your coding agent (agent-setup)` — the strongest surviving step-2 candidate, and it is UNMEASURED.** 16,971 bytes, 264 lines, **zero subheadings**, documenting ONE command — 55% the size of the whole Command reference, which covers ~25. Meanwhile 103,879 bytes of rationale for the same command already sit correctly placed in `claudedocs/decisions/34`, `/35` and `/36` (maintainer content, GitHub-only). 🔴 **Do not turn that into a byte estimate — that is the mistake this arc made three times.** Do the trial edit, then report the number. Two constraints known up front: it is read by `readme_command_synopsis_test.go`, and giving it 4–5 `###` is worth doing on its own merits (265 unbroken lines have no internal map).
   forcing: none — no gate rides on it now
2. **Run the ladder on #635, then merge.** Round 0 + round 1; merge when a round returns no high-sev. Same grant as #611/#625/#630/#633.
   forcing: gate
3. **Merge #623** so the plan and this handoff stop living only on a branch. No audit round has run on it (docs-only, no test surface — stated, not skipped silently).
   forcing: gate
4. **#602** — operator's. Green on its own checks but `DIRTY` against main and degrading; its inclusive-ceiling correction must survive the rebase (see the note posted on it).
   forcing: gate
5. **Phase 4 — the reorder.** Byte-neutral. Fixes the measured reader paths: the CI/`--json` reader currently travels 81% of the file to reach the exit-code contract, and the `listing status`-is-not-a-read warning sits ~36% in where a `--json` reader never passes it. **This is now the highest-value remaining phase**, because with the byte target gone the arc's goal is legibility, which is what a reorder buys and what a byte count never measured.
   forcing: none
6. **Phase 5 — error messages.** ⚠ **Re-scoped by measurement: it is a QUALITY item, not a size item.** Where the README says more than the binary, improve the Go error string and delete the row. Ceiling measured at ~5,465 bytes realistic (see below), so do it because the error should carry its own remedy — `AGENTS.md`: *"Make errors actionable — name the next command to run"* — not to move a number. Needs its own PR and audit round: it changes behaviour.
   forcing: none
7. **#614** — `CIVITAI_NO_COLOR` value parsing. Genuinely optional.
   forcing: none
8. **23 stale agent worktrees under `.claude/worktrees/`** (41 total in `git worktree list`). Noticed, not acted on: they make a whole-repo `grep` return a dozen copies of every hit, which is how one measurement in this session nearly got read off the wrong tree. `git worktree prune` plus removing the dead ones. Closing condition: `git worktree list | wc -l` returns a number the operator recognises as live-only.
   forcing: none

## Gotchas / decisions / dead-ends

### Added 2026-09-15 — what phase 5 can actually yield (the measurement that closed rank 1)

Measured against the LIVE section on `main` at 282,312, not estimated. `## Troubleshooting`
is 20,372 B / 61 data rows. Banded by cause-cell size, with the two floors re-derived
from the tests rather than trusted from prose:

| band | rows | row-bytes | phase 5 applies? |
|---|---:|---:|---|
| floor-protected | 9 | 2,732 | **no** — 7 from `…CoversTheRefusalsAuthorsActuallyHit`, 2 from `symptomAttributionsFloor`. Both are named-MEMBERSHIP guards, not counts, so add-one/delete-one does not defeat them |
| cause cell <150 B | 17 | 3,047 | **no** — phase 3 already cut these to the bone; the only content left is the **exit code**, which the binary never prints as text |
| 150–250 B | 19 | 5,465 | **partly** — the realistic target |
| >250 B | 16 | 7,966 | **no** — see below |

🔴 **The big cells are big for reasons a single error string structurally CANNOT
absorb, and that is the finding.** Checked against live source: `SHA256 mismatch for`
(737 B) and `could not read your Buzz balance` (446 B) — **both Go strings already carry
their remedy** — `deleted the partial download`, and `verify with civitai buzz`. The
README's extra bytes are *threat-model rationale*: why the uploader-supplied filename is
sanitised, why the progress line is cut at 120 chars, what a wrap would forge at column
zero. That is documentation of a security property and it does not belong in an error
message. Others aggregate across call sites (`is an OFFSITE app` = 4 commands,
`no such submission` = 3, `the server rejected this store-listing change (400)` = seven
routes) or branch **two** exit codes off one message (`rate limited (429)`) — a table can
say that and a per-site string cannot.

**So: realistic yield ≤5,465 B.** The absolute ceiling is 16,478 B (delete all 52
non-floor rows), capped by `len(symptoms) >= 15` to ~14,600 — which means deleting **46
of 61 published rows**. That is precisely the repair
`TestREADMETroubleshootingCoversTheRefusalsAuthorsActuallyHit`'s own comment forbids:
*"Deleting a row to make the sibling green is exactly the repair this forbids."* It was
offered as an option and rejected.

⚠ **The crude instrument is recorded so nobody quotes it.** A regex pass scored "25 of
49 located rows have no remedy-shaped token on the error line". It **overcounts** — it
flags `SHA256 mismatch`, which we then showed by hand already carries its remedy. Use
the band table, not the 25.

### Added 2026-09-15 — Option B, measured live instead of estimated

Recorded because it was offered and declined, so the numbers are not re-derived.
`## Generate`'s five deep subsections = **26,289 B** (`Silent model substitution` 6,035,
`Raw graphs` 5,632, `Waiting, downloading, re-attaching` 5,142, `What the server says
went wrong` 4,968, `Listing and cancelling workflows` 4,512). `## Submit & auth`'s
**un-pinned** `####` blocks = **10,490 B** (`What looks like a credential` 4,696,
`How big can a bundle be?` 3,355, `What the packager left out` 2,439) — the dotenv block
(10,353, golden-file) and the whoami blocks (5,841, exact-stdout) are excluded because
moving them repoints a test and turns a docs edit into a code change. Declined on the
link-out policy: `.goreleaser.yaml` ships only `README.md` and `LICENSE`, so every byte
moved out is lost to tarball, Homebrew-cask and npm readers offline.

### Added 2026-09-15 — four audit rounds on #611

- 🔴 **Every finding across four rounds was the same shape: a sentence claiming more
  than its code does.** Round 0: the PR title said six guards and there were four.
  Round 1: five 🟡, including a claim the PR itself had just published. Round 2: three
  *new* unguarded prose claims written into the surfaces the PR was cleaning. Round 3
  fixed an attribution comment its own first commit had got wrong. The defect class a
  PR is fixing reproduces itself inside the fix.
- 🔴 **A guard's failure MESSAGE is part of the guard.** One reverse branch fired on
  the wrong predicate and told the reader "The tables were merged… Re-derive it." A
  maintainer following that instruction would have deleted the qualifier and
  republished the very defect the guard existed to catch.
- 🔴 **A mutation control can be green for an unrelated reason.** A `default:`
  fall-through mutant survived because the control fed it a *zip*: the error came from
  the tar reader rejecting the zip, not from the dispatch, so the test would have
  stayed green with the branch deleted. A one-directional control needs both shapes.
- **A comment can name the wrong mechanism.** `upgrade.go` claimed a corrupt zip
  member "surfaces as a CRC failure on Close". Measured on Go 1.25.14: `io.ReadAll`
  returns `zip: checksum error` and `Close()` returns nil. The branch guarding it was
  dead. Load-bearing, because it invited dropping the check that actually works.
- **`audit-dispatch.py --round N` REFUSES without a posted claims block**, and the
  refusal is correct: an empty "what was claimed fixed" section silently turns a delta
  into a blind full audit that then reads as covered. Post the block as an **issue**
  comment — `gh pr view --json comments` does not return review comments.
- **`gh issue create` is gated on a closing condition** and cannot read a body passed
  via shell substitution. Use `--body-file <real path>` with a literal title.

### Added 2026-09-15 — the README's structural cause

- 🔴 **RETRACTED 2026-09-15 — "the suite punishes this file for shrinking" WAS FALSE,
  and it was load-bearing: it is the sole argument that motivated adding a README byte
  ceiling.** The original claim read: *"There is no README size ceiling and roughly a
  dozen assertions are FLOORS. `len(raw) > 10_000`, `links >= 40`, `subs >= 25`,
  `symptoms >= 15` all fire if it shrinks… that asymmetry is most of how 429 lines
  became 4,234 in ten weeks."* The first sentence is true. **The causal half is not.**
  Every one of those numbers is a **positive control on an EXTRACTOR**, not a floor on
  content volume, and each says so in its own failure message — `links < 40` prints
  *"the link regex is reading the wrong text"*; `resolved < 30` prints *"the synopsis
  parser is not finding command paths"*; `symptoms < 15` prints *"extracted only %d
  symptom strings"*; `rows < 20` and `checks < 85` both print *"the assertions above are
  vacuous at this count"*. They fire when a PARSER stops seeing text, which is exactly
  what RULES.md's instrument-validation rule demands, and they are defended.
  **The arc itself is the disproof**: four PRs cut 28,335 bytes — 9.1% — and **not one
  floor fired**. A suite that punished shrinking would have gone red long before.
  ⚠ So the structural cause of the growth is still **unexplained**. Do not fill that
  gap with the next plausible mechanism; it is an open question, not a solved one.
- 🔴 **AND THEREFORE: DO NOT ADD A README BYTE-CEILING TEST.** One was designed and
  discarded the same day. Three independent reasons, each sufficient. (1) The floors
  argument above is retracted, so the guard has no incident to point at — nobody has
  ever reported harm from README size. (2) **The analogy to `agents_size_test.go` does
  not transfer.** That ceiling is justified by a *mechanical* per-session cost:
  `CLAUDE.md` line 1 is `@AGENTS.md`, so every byte is paid by every agent in every
  session. **Nothing `@`-imports `README.md`** — verified by grep — so the cost it would
  ration does not exist. (3) `the-algorithm` step 5 forbids it in terms: *"The fix for
  over-guarding is NEVER another guard. No ratchet on test growth."* The design WAS a
  ratchet. Note also what the AGENTS.md ceiling actually costs, from its own comment: it
  needed a second constant (`agentsMaxBytesCeiling`) to stop the first being raised
  instead of obeyed, plus a third test to assert that bound — the honest price of that
  guard is three artifacts, not one.
- **Only `README.md` and `LICENSE` ship** (`.goreleaser.yaml` archives). So "move it
  to AGENTS.md" loses user-reachable content. The sanctioned link-out is the absolute
  `https://github.com/civitai/cli/blob/main/<doc>` URL, which
  `readme_contributor_links_test.go`'s normaliser deliberately exempts — its own
  failure message prescribes exactly that.

### Added 2026-09-15 — operator decisions that change how this arc runs

- 🔴 **The ladder stop rule here is "audit until a round returns no HIGH-SEVERITY findings"**, not "until a round is clean" — operator correction, 2026-09-15. Under it, #611's round 1 (five 🟡, zero 🔴) was already a stopping round and the fix round after it was optional. Do not re-derive the stricter rule from the skill text.
- 🔴 **Standing merge authority, scoped:** merge PRs this session opened once all required contexts pass AND a round returns no high-sev findings. **Never** another session's PR, never a release draft, never a tag. The automated `bump-scaffold-pins` PRs were explicitly NOT included — those still go to the operator.
- **Preferred fix direction: fix the code, not the prose.** Asked whether to document the Windows `upgrade` defect or fix it, the operator chose fix. Applied generally: when a README claim is false because the code is wrong, prefer correcting reality.

### Added 2026-09-15 — process traps hit this session

- **`gh issue create` is gated on a closing condition and cannot read a body passed via shell substitution.** Use `--body-file <real path>` with a literal title; a `$(...)` title blocks the whole invocation. Both of this session's first two attempts were refused, correctly — neither draft named what would end the issue.
- **`audit-dispatch.py --round N` REFUSES without a posted claims block**, and the refusal is right: an empty "what was claimed fixed" section turns a delta into a blind full audit that then reads as covered. Post the block as an **issue** comment — `gh pr view --json comments` does not return review comments.
- **`pgrep -f 'go test'` matches its own shell.** A leaked-process sweep reported a PID that was the sweep itself; by the time it was resolved via `/proc`, it was gone. Resolve before killing, always.
- **Agent mutation copies produce alarming diagnostics.** Compile errors in `upgrade.go` were reported three separate times from `cp -a` copies under the scratchpad — deliberately-broken trees are the point of a mutation copy. Check whether the live worktree builds before treating a diagnostic as real.

### Added 2026-09-15 (session 2) — what five merged PRs and eleven audit findings taught

- 🔴 **EVERY audit finding across five PRs was the same shape: a sentence claiming more than its code does.** Eleven of them. Two were 🔴 in published contract text, *introduced while compressing*: a row said "every change opens a revision" while naming `set-text`, which applies **in place, immediately, publicly** — a reader could have made an irreversible public write; the fix then narrowed it to "a **media** change", silently reassigning `set-source-repo` to the wrong side. **A compression pass must open the Go source for every behavioural sentence, and check what the shortened version implies about cases it no longer names.**
- 🔴 **DELETE-FIRST beats relocate, measured on the same file.** Phase 3 relocated: removed 15,021, gave back 6,377 (39.7%) into five other sections. Phases 2/3b/3c deleted: gave back **0**. Relocation preserves the *string*, not the *reader* — a cause cell is read at the moment of failure, a paragraph three sections away is read by someone browsing.
- 🔴 **A guard's failure MESSAGE is part of the guard.** One reverse branch fired on the wrong predicate and told the reader "The tables were merged… Re-derive it." — which, followed, republishes the defect it guards.
- 🔴 **A mutation control can be green for an unrelated reason.** A `default:` fall-through survived because the control fed it a *zip*: the error came from the tar reader rejecting the zip, not from the dispatch. A format-dispatch control needs BOTH well-formed shapes under a bogus format name.
- 🔴 **Floors must sit ABOVE the regression they guard, not on it.** The new synopsis guard's floors were 25/60 while the documented regression produces exactly 25/25/75 — it cleared them and passed. Raised to 30/85; the mutant now fails naming its own signature.
- **The plan contained two traps, both retracted after measurement**: "convert all five relative links to absolute" would drive `checkedTargets` to 0 and `t.Fatal` (5 survive, floor 3 — and the same control is breakable by *deleting* a link too); and treating the per-cluster byte figures as targets rather than upper bounds, which would have cut two passages `claudedocs/decisions/13` and `/25` require to exist.
- **`-run 'README|Readme|readme'` MISSES the Attribution tests** — the only guards on the Troubleshooting table's two frozen cells. Use `-run 'Attribution|Troubleshooting|README|Readme|readme'` **plus `go test ./...`** (9 other packages hold README-reading tests, including the dotenv golden in `internal/pkgzip`).
- **`git merge-tree` exit code answers a different question from `gh pr view`.** #602 conflicts with `origin/main` *on its own*; a control row proved the conflict was not caused by any of this session's branches.
- **`printf` in a commit message eats a bare `%`** — one commit shipped truncated at "39.7". Write the message to a file and `-F` it.

## How to verify

🔴 **This block was STALE and is the reason to distrust any copy of the gate you find
elsewhere in this doc.** It still named `<= 250000` — a threshold already superseded
twice at the top of the file and now deleted outright — and the NARROW `-run` filter the
amendment above exists to replace. Both are corrected here; if you find a third copy,
that one is stale too.

```bash
# the arc's closing condition. There is NO byte threshold — see the top of this doc.
# Report the size if you like, but nothing gates on it:
git -C <repo> show origin/main:README.md | wc -c

# the gate, WIDE filter, with its own positive control.
# `ok` with 0 tests run is the failure this replaced — measured 76 on 2026-09-15.
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # must be > 0
go test ./...                                            # 9 other packages read README

# the guards phase 1 shipped, proven by reverting a correction
#   each revert must fail on its OWNING guard, not a neighbour's

# lint is a SEPARATE CI job; make ci does not run it
nix-shell -p golangci-lint --run "golangci-lint run"
```
