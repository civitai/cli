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

closing-condition: check — `README.md` is at or below 250,000 bytes on `origin/main`
AND `go test ./internal/cmd/ -run 'README|Readme|readme' -count=1` exits 0. Both are
mechanical; run them and the arc is answerable without a judgement call.

🔴 **The full measured plan is `claudedocs/readme-reduction-plan.md`** — constraint
map, size map, cut list, compression targets, link-out policy, sequencing. Read it
before touching prose. This doc is state; that doc is the plan.

## State now

**Phase 1 (accuracy) is MERGED as #611 (`f284d89`). Phases 2–4 are unstarted.**

🔴 **The README is currently BIGGER than when this arc started** — 314,712 bytes
against 310,647 at `426288f`. Phase 1 was an accuracy pass, not a reduction. Do not
read #611 as progress toward the goal; it is prerequisite work that made the file
trustworthy enough to cut safely.

- **#611 merged** after four audit rounds (0, 1, 2, and a fix round). Six README
  corrections, each pinned by a guard watched to fail on revert, plus a real
  behaviour fix: `civitai upgrade` now derives the release-asset extension per GOOS
  and can self-update on Windows. Verified against the real published `v0.1.105`
  zip and the 16,036,352-byte `civitai.exe` inside it.
- **#613 CLOSED** by that merge (Windows `upgrade`). **#614 OPEN** and deliberately
  optional — the `CIVITAI_NO_COLOR` value-parsing asymmetry is now accurately
  documented *and* guarded, so it is a papercut with no drift risk.
- **#615 merged at 01:50** (automated pin bump), which unfroze the repo. That also
  satisfies the closing condition the previous arc recorded for cli#530: a real
  scheduled run that produced a PR. cli#540's fix works.
- **#602 is OPEN, green, and 12 commits behind main** — the only open PR touching
  README. It edits 1671-1712 and three Troubleshooting rows, colliding with phases
  2 and 3 equally. It is another session's PR.

### Honest limits

- 🔴 **Nobody has run `civitai upgrade` on a Windows machine.** Every branch is
  exercised against the real published zip, and `minio/selfupdate` carries a genuine
  Windows implementation, but the final self-replace is stubbed behind the
  `applyUpdate` seam — as it already was for every platform before #611.
- **`TestFilesystemErrorsExitGenericEndToEnd` failed once** during a revert matrix
  and then passed 5/5. Called a flake on repetition alone, not on wall-time analysis,
  which is the weaker diagnosis. Unrelated to #611. Worth knowing if it resurfaces.
- **The audit base rate for this arc was 4 for 4** — every round found something,
  and rounds 1, 2 and 3 each found a defect the previous round's own fix introduced.

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

1. **Merge #602, or have its owner do so.** The single unblock: it is green,
   mergeable and 23h stale, and it collides with both remaining phases. Cheap now,
   more expensive as main moves.
2. **Phase 3 — Troubleshooting compression, 35,250 → ~13,000 bytes.** The largest
   single win and the lowest risk: the middle column is pinned by **nothing**
   (`readme_troubleshooting_test.go` says so in its own comments). The symptom column
   is pinned and does not move.
   forcing: gate — this is the arc's closing condition
3. **Phase 2 — the cuts, ~20,500 bytes.** Six unrelated regions, all maintainer-facing,
   none pinned. Listed with byte counts in the plan doc.
   forcing: gate
4. **Phase 3b — command-reference compression, 21,812 → ~8,500.** One cell is 3,794
   bytes and inlines a table that already exists 230 lines earlier.
   forcing: gate
5. **Phase 4 — the reorder.** Last, when the pieces are their final size; a reorder
   diff over uncompressed text is unreviewable.
   forcing: none
6. **Settle the `download` auth contradiction** (see Open investigations). One live
   request.
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
