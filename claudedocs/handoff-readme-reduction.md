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

**Phase 1 (accuracy) is MERGED as #611 (`f284d89`). Phase 3 (Troubleshooting compression) is IN FLIGHT. Phases 2 and 4 unstarted.**

🔴 **The README is still BIGGER than when this arc started** — 314,712 bytes against 310,647 at `426288f`. No reduction has shipped yet. Phase 3 is the first PR that will move that number.

- **#611 merged** (`f284d89`) after four audit rounds. Six README corrections each pinned by a guard watched to fail on revert, plus a behaviour fix: `civitai upgrade` derives the release-asset extension per GOOS and can self-update on Windows. Verified against the real published `v0.1.105` zip and its 16,036,352-byte `civitai.exe`.
- **#623 OPEN** — `claudedocs/readme-reduction-plan.md` + this handoff. Docs only, `make ci` green. The plan doc is the arc's measured basis; read it before touching prose.
- **Phase 3 IN FLIGHT: `civitai/cli` branch `fix/readme-troubleshooting-compression`**, worktree `/home/zach/workspace/civit/cli-trouble`, based on `f284d89`. Target: `## Troubleshooting`, measured at **lines 4004-4104, 101 lines, 35,249 bytes** — ~350 bytes per line of file — down to ~13,000. No PR yet.
- **#613 CLOSED** by #611. **#614 OPEN** and optional — the `CIVITAI_NO_COLOR` asymmetry is accurately documented *and* guarded, so it has no drift risk.
- **#615 merged 01:50** (automated pin bump) unfroze the repo; it also satisfied the closing condition the previous arc recorded for cli#530 — a real scheduled run that produced a PR. cli#540's fix works.
- **#602 still OPEN**, green, now 12+ commits behind main. Only open PR touching README; edits 1671-1712 and three Troubleshooting rows, so it collides with phases 2 and 3 equally. Another session's PR.
- **Worktrees**: `cli-readme-fix` removed after #611 merged. Live: `cli-docs` (#623), `cli-trouble` (phase 3).
- **Claim**: `cli-readme-reduction-1` held by this session (rc 12 on re-check). Release it when the arc closes or is abandoned.
- **No clawgate task recorded** — `clawgate_handoff.sh resolve` returned rc 5 (nothing resolved). Its positive control showed the board reachable, but a wrong session id also answers 200 with an empty array, so this is NOT a statement that no task exists.

### Honest limits

- 🔴 **Nobody has run `civitai upgrade` on a Windows machine.** Every branch is exercised against the real published zip and `minio/selfupdate` carries a genuine Windows implementation, but the final self-replace is stubbed behind the `applyUpdate` seam — as it was for every platform before #611.
- **`TestFilesystemErrorsExitGenericEndToEnd` failed once** during a revert matrix, then passed 5/5. Called a flake on repetition alone, not wall-time analysis — the weaker diagnosis. Unrelated to #611.
- **The audit base rate for #611 was 4 for 4.** Rounds 1, 2 and 3 each found a defect the previous round's own fix introduced.

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

1. **Merge #602, or have its owner do so.** The single unblock: green, mergeable, 12+ commits behind, and it collides with both remaining prose phases. Cheaper now than later. `IN FLIGHT: civitai/cli#602` (another session's).
   forcing: gate
2. **Phase 3 — Troubleshooting compression, 35,249 → ~13,000 bytes.** `IN FLIGHT: civitai/cli` branch `fix/readme-troubleshooting-compression`. Largest win, lowest risk: the middle column is pinned by nothing, which `internal/cmd/readme_troubleshooting_test.go` states in its own comments. The ~68 symptom strings in column one ARE pinned and must stay byte-identical. Must test-merge against #602, which edits three of these rows.
   forcing: gate
3. **Phase 2 — the cuts, ~20,500 bytes.** Six unrelated regions, all maintainer-facing, none pinned. Byte counts per region in `claudedocs/readme-reduction-plan.md`.
   forcing: gate
4. **Phase 3b — command-reference compression, 21,812 → ~8,500.** One cell is 3,794 bytes and inlines a per-agent table that already exists ~230 lines earlier.
   forcing: gate
5. **Phase 4 — the reorder.** Last, when the pieces are their final size; a reorder diff over uncompressed text is unreviewable. Fixes the measured reader paths (the CI/`--json` reader currently travels 81% of the file to reach the exit-code contract).
   forcing: none
6. **Settle the `download` auth contradiction** — one live anonymous download. See Open investigations.
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
