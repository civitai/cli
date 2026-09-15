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

closing-condition: check — `README.md` is at or below **270,000** bytes on `origin/main`
AND `go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme'
-count=1` exits 0. Both are mechanical; run them and the arc is answerable without a
judgement call.

⚠ **Two operator amendments, 2026-09-15, both recorded so the original is not
re-derived.** (1) The threshold was **250,000** and was moved *after* the phase-3 trial
edit showed Option A lands at ~266,261 — changed deliberately rather than quietly
missed; the arithmetic is in `claudedocs/readme-reduction-plan.md`. (2) The gate filter
was `-run 'README|Readme|readme'`, which matches **zero** of the guards on the
Troubleshooting table's two frozen cells — measured, and confirmed by a live mutant the
narrow filter reported `ok` on.

🔴 **The full measured plan is `claudedocs/readme-reduction-plan.md`** — constraint
map, size map, cut list, compression targets, link-out policy, sequencing. Read it
before touching prose. This doc is state; that doc is the plan.

## State now

**Five PRs merged this session. README is 282,312 bytes on `main` — down 28,335 (−9.1%) from the arc's start. The closing condition (≤270,000) is 12,312 away and NO named item closes it.**

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

1. **DECIDE THE CLOSING CONDITION — this blocks calling the arc finished.** 282,312 now, ≤270,000 required, gap 12,312. Phase 4 is byte-neutral by definition; phase 5 is unmeasured. Three options, recommendation first: **(a) measure phase 5 first** — sample how many Troubleshooting rows can actually be deleted once their error messages carry the remedy, then re-set the condition from a measurement rather than a fourth estimate; **(b) take Option B** — move `Generate`'s deep subsections and the `####` blocks to GitHub-only docs, reaches ≤270,000 but requires repointing the golden-file and stdout pins; **(c) move the condition to ~278,000** and record that it was set twice by estimate and missed twice.
   forcing: gate — the arc's own closing condition is unreachable as written
2. **Run the ladder on #635, then merge.** Round 0 + round 1; merge when a round returns no high-sev. Same grant as #611/#625/#630/#633.
   forcing: gate
3. **Merge #623** so the plan and this handoff stop living only on a branch. No audit round has run on it (docs-only, no test surface — stated, not skipped silently).
   forcing: gate
4. **#602** — operator's. Green on its own checks but `DIRTY` against main and degrading; its inclusive-ceiling correction must survive the rebase (see the note posted on it).
   forcing: gate
5. **Phase 4 — the reorder.** Byte-neutral. Fixes the measured reader paths: the CI/`--json` reader currently travels 81% of the file to reach the exit-code contract, and the `listing status`-is-not-a-read warning sits ~36% in where a `--json` reader never passes it.
   forcing: none
6. **Phase 5 — error messages.** Where the README says more than the binary, improve the Go error string and delete the row. 7 rows sit on an incident floor and may not be deleted; column 1 is pinned to the source string, so a message change moves both in one commit. Needs its own PR and audit round — it changes behaviour.
   forcing: none
7. **#614** — `CIVITAI_NO_COLOR` value parsing. Genuinely optional.
   forcing: none

## Gotchas / decisions / dead-ends

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

- 🔴 **There is no README size ceiling and roughly a dozen assertions are FLOORS.**
  `agents_size_test.go` caps AGENTS.md; nothing caps README. Meanwhile
  `len(raw) > 10_000`, `links >= 40`, `subs >= 25`, `symptoms >= 15` all fire if it
  shrinks. The suite punishes this file for shrinking and never for growing — that
  asymmetry is most of how 429 lines became 4,234 in ten weeks.
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

```bash
# the arc's closing condition, both halves
git -C <repo> show origin/main:README.md | wc -c          # target: <= 250000
go test ./internal/cmd/ -run 'README|Readme|readme' -count=1

# the guards phase 1 shipped, proven by reverting a correction
#   each revert must fail on its OWNING guard, not a neighbour's

# lint is a SEPARATE CI job; make ci does not run it
nix-shell -p golangci-lint --run "golangci-lint run"
```
