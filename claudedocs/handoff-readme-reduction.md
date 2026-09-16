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

**Nine PRs merged across this arc; five of them this session. README is 281,439 bytes on `main` (`079dc14`) — down 29,208 (−9.4%) from the arc's start. There is no byte gate (see the closing condition above); size is reported because it is the cheapest progress signal, and for no other reason.**

| | bytes |
|---|---:|
| arc start (`426288f`) | 310,647 |
| after phase 1 — accuracy, #611 | 314,712 |
| after phase 3 — Troubleshooting, #625 | 305,329 |
| after phase 2 — cuts, #630 | 293,378 |
| after phase 3b — command table, #633 | 282,312 |
| after phase 3c — preamble, #635 | 281,439 |
| **on `main` now (`079dc14`)** | **281,439** |

⚠ **#635 landed at −873, not the −1,741 its body advertises.** Four audit rounds added 868 B back, every byte buying a contract claim that is now true. Under the deleted threshold that would have read as the PR failing; it is the PR working, and it is the clearest single vindication of deleting it.

### Merged
- **#611** (`f284d89`) phase 1, accuracy — six corrections each pinned by a guard watched to fail, plus the Windows `civitai upgrade` behaviour fix. Closed #613. Four audit rounds.
- **#625** Troubleshooting 35,250 → 19,182. Symptom column byte-identical.
- **#630** phase 2 cuts, −14,279, **give-back 0** — the PR that proved delete-first.
- **#633** command table 21,391 → 9,664 (−54.8%), plus `TestREADMECommandSynopsesNameRealFlags`.
- **#615** the automated pin bump (not this arc's) — satisfied the previous arc's cli#530 closing condition.
- **#623** (`868fff6`) the plan + this handoff, and the commit that **deleted the byte closing condition**.
- **#636** (`ada6699`) two comments that claimed coverage they did not have.
- **#635** (`c7ea087`) phase 3c, the preamble. **Four audit rounds** — the ladder record is under Gotchas and is the most transferable thing this arc produced.
- **#639** (`079dc14`) **rank 3b — the guards.** Four tests over the paragraph #635 repaired and over `doUpload`'s switch, all mutation-verified. **Two audit rounds, four findings, every one of them my own prose overreaching my own code.**

### Open
- **#637** — 🔴 a **CODE** defect, not a docs one. `printSubmitSizeDiagnosis` prints `What this CLI sent` for errors raised *before any HTTP request is built*, so the past tense can be false. Measured against a real `httptest` server: **0 requests received**, block printed, with a positive control on the hit counter. `appblocks.go:570` returns on `Tokens.Token()` before `doOnceWith` builds anything; `auth/source.go:115` returns `persist refreshed tokens` untagged. A second comment carries the mirror case — `auth/source.go:90` tags `ErrUnauthorized` onto a purely *local* error, suppressing the block with no HTTP status anywhere. **Its closing condition includes reverting the two README surfaces #635 had to weaken**, and `TestREADMEEntryBlockSurfacesAgree`'s failure message points at it by name so a future fixer is not told to keep the weakened wording.
- **#602** (another session's) — `DIRTY` against main and **22+ commits behind**. Not ours to rebase.
- **#614** open and optional — the `CIVITAI_NO_COLOR` value-parsing asymmetry, documented and guarded.

### Honest limits
- 🔴 **Nobody has run `civitai upgrade` on Windows.** Every branch is exercised against the real published v0.1.105 zip, but the final self-replace is stubbed behind the `applyUpdate` seam.
- **The estimates under-delivered three times running**, which is what eventually killed the byte threshold. Phase 5 is now measured rather than estimated (below).
- 🔴 **The `app_submit_r2_test.go` consolidation in #639 was never audited.** It landed in the round-1 fix commit, and no round read it. Operator merged knowing this. It is one call into `nonTTYSubmitRefusal`, and that helper fatals if the refusal path was not the one taken — so the exposure is small and named, not hand-waved.

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

**Closed and removed from the queue** (kept as one line so nobody re-derives them): the byte closing condition decision, the #635 ladder, merging #623, and rank 3b. 3b merged as #639 (`079dc14`) — the four claims are pinned, and the ledger catches something nothing else in the tree does (measured: removing the `ErrRateLimited` case is caught *only* by that file).

1. **Trial-cut `## Set up your coding agent (agent-setup)` — the largest surviving step-2 candidate, still UNMEASURED.** 16,971 bytes, 264 lines, **zero subheadings**, documenting ONE command — 55% the size of the whole Command reference, which covers ~25. 103,879 bytes of its rationale already sit correctly placed in `claudedocs/decisions/34`, `/35`, `/36` (maintainer content, GitHub-only). 🔴 **Measured 2026-09-16: only ~168 of its 16,971 bytes are pinned** by any Go test literal ≥30 B — three Docs-section URLs and one sentence; `agent_setup_docs_test.go` does not read the README at all. So it is almost entirely unpinned prose, the same property that made Troubleshooting's middle column tractable. 🔴 **Do not turn that into a byte estimate — that is the mistake this arc made three times.** Do the trial edit, then report the number. Repo: `civitai/cli`; files: `README.md`, and `internal/cmd/readme_command_synopsis_test.go` reads that section.
   forcing: none
2. **Phase 4 — the reorder.** Byte-neutral, and **now the highest-value remaining phase**: with the byte target gone the arc's goal is legibility, which is what a reorder buys and what a byte count never measured. Fixes the measured reader paths — the CI/`--json` reader travels 81% of the file to reach the exit-code contract, and the `listing status`-is-not-a-read warning sits ~36% in where a `--json` reader never passes it. Repo: `civitai/cli`; files: `README.md`.
   forcing: none
3. **Phase 5 — error messages.** ⚠ **Re-scoped by measurement: a QUALITY item, not a size one.** Realistic yield ≤5,465 B (band table under Gotchas), so do it because an error should carry its own remedy — `AGENTS.md`: *"Make errors actionable — name the next command to run"* — never to move a number. Changes behaviour, so it needs its own PR and audit round. Repo: `civitai/cli`; files: `internal/cmd/*.go`, `README.md`.
   forcing: none
4. **#602** — operator's. Green on its own checks but `DIRTY` against main and degrading; its inclusive-ceiling correction must survive the rebase (note posted on the PR).
   forcing: gate
5. **#637** — fix the code so the tense stops lying, then **revert the two README surfaces #635 weakened, in the same PR**. `TestREADMEEntryBlockSurfacesAgree` will go red on that revert and its failure message says so explicitly — that is intended, not a blocker. Repo: `civitai/cli`; files: `internal/cmd/app_submit.go`, `internal/appapi/appblocks.go`, `internal/auth/source.go`, `README.md`, `internal/cmd/readme_submit_entry_block_test.go`.
   forcing: none
6. **#614** — `CIVITAI_NO_COLOR` value parsing. Genuinely optional.
   forcing: none
7. **41 worktrees on this clone — 23 agent (`.claude/worktrees/`) + 18 named.** ⚠ **`git worktree prune` was RUN and removed NOTHING**: every directory exists, so they are live entries and the cheap fix does not apply. **Not acted on, deliberately** — several hold *other sessions'* branches, and removing 18 is the operator's call. Cost measured, not asserted: it blocked `--delete-branch` on **all five** merges this session, and it produced a false 🔴 in #635's round 3 (an auditor grepped a worktree standing on `main` rather than the PR head). Closing condition: `git worktree list | wc -l` returns a number the operator recognises as live-only.
   forcing: none
8. 🔴 **`README.md:1679`/`:1728` state a body size that is arithmetically impossible**, under a sentence asserting *"That number is exact, not an estimate."* `base64.EncodedLen(8201270)` + envelope = **10935047** (no provenance) or **10935125** (40-hex commit + `sourceDirty:false`); the README prints **10935065**, which is neither. Pre-existing since `509babc` (#452); `10935065` appears only in `README.md`, so it is unpinned by construction. Closing condition: the printed figure equals `SubmitBodySize` for a stated provenance case, and a test asserts it.
   forcing: none

## Gotchas / decisions / dead-ends

### Added 2026-09-15 — #635's four-round ladder, and what it measured about ladders

🔴 **THREE OF THE FOUR FINDINGS WERE DEFECTS IN THE PREVIOUS ROUND'S FIX, ALL IN ONE
PARAGRAPH.** Not a restatement of the skill's warning — a fresh instance, and the
cleanest one this repo has.

| round | finding | authored by |
|---|---|---|
| 0 | the sentence over-claimed by one case (the ceiling refusal prints a *different* block) | the PR |
| 1 | still over-claimed, by **nine** more early-`return err` sites; **and** the Troubleshooting row now disagreed | round 0's fix |
| 2 | the fix over-corrected into a **bytes-shaped** claim the code does not model | round 1's fix |
| 3 | clean — ladder stopped | — |

**The direction of travel is the lesson, and it is counter-intuitive.** The *base* wording
was a claim about how the error **classifies**, which is what `app_submit.go:672` actually
switches on, and it was **true** of the pre-contact case. Round 1 "narrowed" it into a
claim about whether bytes left the machine — a property the code never computes. **A fix
that makes a sentence more specific can make it false**, and it will read as an
improvement in review because specificity looks like rigour.

- 🔴 **When a claim loses its reason, DELETE the reason — do not source a new one.** Round
  2's fix dropped *"because those never reach the server to fail against"* (false for the
  version guard, which calls `ListSubmissions`) rather than substituting a better because.
  Reaching for a fresh justification under pressure is what regenerates the error; the
  enumeration did not need a why.
- 🔴 **Stop on the FINDINGS, and say out loud that the streak is not evidence.** Round 3
  volunteered: *"I felt the pull to sustain the three-round streak and am declining it."*
  Three consecutive productive rounds create real pressure to manufacture a fourth finding.
- 🔴 **A FOURTH edit to that paragraph was deliberately NOT made.** Round 3's 🟢 was real
  and went to #637 instead. Rationale worth reusing: after three of my own edits each
  introduced a defect, an unaudited fourth resets the verification gate with **no round
  left to catch it**. Filing beat fixing.
- **The auditor RETRACTED a would-be 🔴 before reporting it** — it had grepped `README.md`
  from a worktree standing on `main` rather than the PR head, and "found" a second unfixed
  row. Re-measured via `git show <head>:README.md`: exactly one row, already fixed. Same
  family as the `ci-shallow.sh` trap in this repo's `AGENTS.md`. **Resolve a range in a tree
  that contains the PR head**, and note the retraction is worth more than the findings kept.
- 🔴 **THE COVERAGE GAP THAT MADE ALL OF THIS POSSIBLE, AND IT IS STILL OPEN.** **No test
  pins the prose of that paragraph or that table row.**
  `TestREADMETroubleshootingSymptomsExistInTheSource` guards only the **left-hand symptom
  column**. Every behavioural sentence rounds 0–3 rewrote is unguarded, which is exactly how
  each round shipped a defect with the suite green. The arc's phase-1 pattern — pin each
  repaired claim with a guard watched to fail — was **not** applied here, and that is why it
  took four rounds instead of one.

### Added 2026-09-15 — two numbers nothing asserts on

Found by #635's once-per-ladder sweep, **neither attributable to that PR**; both still live
on `main`.

- 🔴 **`README.md:1679`/`:1728` — the transcript's body size is arithmetically
  unreachable.** The lines state the zip as `8201270` B.
  `base64.EncodedLen(8201270)` + envelope = **10935047** (no provenance) or **10935125**
  (40-hex commit + `sourceDirty:false`). The README prints **10935065** — neither — directly
  under a paragraph asserting *"That number is exact, not an estimate."* Pre-existing since
  `509babc` (#452). **`10935065` appears only in `README.md`**, so it is unpinned by
  construction and no test can ever catch it drifting.
- **`d1361c3`'s commit message claims `10485760` "still appears exactly once".** It appears
  **twice**, and did so already at the PR base. Commit-message only.

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

### Added 2026-09-16 — #639's ladder, and the pattern across BOTH ladders this session

🔴 **ACROSS #635 AND #639, SIX OF EIGHT AUDIT FINDINGS WERE "A SENTENCE CLAIMING MORE THAN ITS CODE DOES" — AND HALF OF THOSE WERE WRITTEN WHILE BUILDING THE GUARD AGAINST THAT EXACT CLASS.** The arc has now reproduced its own defect inside its own fix, twice, in two different shapes.

**The mechanism is nameable, and it is not carelessness.** Every one came from writing the **doc comment first** and the implementation after. `RULES.md` already says *"Write the claim AFTER the code, from what the function does"* — it was read, in session, and the opposite was done three times in one file. The fix that worked was mechanical: on the last round every comment was rewritten **from the code**, and that round produced no new prose findings.

Concrete instances, so the shape is recognisable rather than abstract:

- 🔴 **"EXTRACTED, NOT COPIED", citing `AGENTS.md` "One rule, one place" — in a ONE-FILE diff.** Extracting is not something a one-file diff can do. `app_submit_r2_test.go` still held its own verbatim copy, so the PR took the count from one site to **two** while its comment claimed the reverse *and named the rule it was breaking*. The tell was free: `git diff --stat` said one file.
- 🔴 **A ledger that covered one of the two surfaces it existed to keep in sync.** `entryBlockSentinel` carried only what the paragraph must say; the Troubleshooting row's exception list — *the half that actually drifted in #635* — was two keyword checks. Rewriting the cell to *"including a `401`/`403`/`429`"*, the exact inverse of `doUpload`'s second case, left the package green.
- 🔴 **A "grows" direction that was SPELLED, not structural.** The extractor regex was `(?:appapi|civitai)\.(\w+)`, so `case errors.Is(err, os.ErrDeadlineExceeded)` was invisible while the docstring said flatly that a new sentinel is red. The `len(found) >= 2` control could not see it either — the three *known* sentinels still matched. **A positive control sized to today's set cannot detect an addition.**
- **A comment describing its own bounds wrongly in both halves**, repeated verbatim in the function's header doc.

### Added 2026-09-16 — two mutation-testing errors of mine, both in the "green for the wrong reason" family

- 🔴 **A MUTANT THAT TRIPS TWO ASSERTIONS TELLS YOU NOTHING ABOUT EITHER.** My M5 deleted both words the row check looked for; the sibling `classifies` assertion went red first, and I scored the *other* one as killed. It was **vacuous**: it matched the whole table line, whose left-hand symptom column contains the same string — and that column is frozen independently by `TestREADMETroubleshootingSymptomsExistInTheSource`, so it could never leave the line. A cause cell saying verbatim what my own error message called false **passed**. Round 0 built the isolated mutant I should have. **Mutate the narrowest expression that can be wrong, and confirm the failure names the guard you were testing.**
- **Making it reachable immediately found a second thing** — the cell writes the tense as `**would have** sent`, so the literal substring never occurred. The first reachable run failed on that. That failure is the cleanest evidence the fix changed something real.
- 🔴 **A positive control floor proves the instrument is reading, not that it is reading WIDELY enough.** `len(found) >= 2` was satisfied by the pre-existing sentinels throughout the `os.ErrDeadlineExceeded` mutant. A floor keyed to today's population is blind to growth by construction.

### Added 2026-09-16 — the gate filter trap, hit AGAIN, by me

🔴 **A NEW GUARD WHOSE NAME DOES NOT MATCH THE GATE FILTER RUNS ONLY UNDER A FULL `./...`, AND THE DOCUMENTED GATE REPORTS GREEN OVER IT.** `TestSubmitEntryBlockSentinelLedger` did not match `-run 'Attribution|Troubleshooting|README|Readme|readme'`. This is Correction 2 in the plan doc, reproduced by the person who had just read it. Both new behavioural/structural tests now carry the `README` prefix deliberately — **name a guard for the filter that is supposed to run it.**

⚠ **And a counting slip worth the line:** the gate count was quoted as 42 once, from `grep -c '^--- PASS'`, against a 76 baseline taken with `'^--- PASS\|^    --- PASS'`. Two patterns, compared as if one. **Name the pattern beside the number** — a round-1 auditor independently failed to reproduce 76→80 for exactly this reason. Correct figures: **76 → 80** with the subtest-inclusive pattern, 38 → 42 top-level.

### Added 2026-09-16 — what audit round 0 is FOR, demonstrated twice

Round 0 (requirements & deletion) changed the outcome on **both** PRs it ran on this session, which is not what its own trial record predicted (`ran: 6 · changed the outcome: 3`).

- On **#639** it was pointed explicitly at the question *"should these guards exist at all"*, because a README byte-ceiling test had been designed and discarded the day before under `the-algorithm` step 5. It **upheld** the requirement, and the discriminator it gave is worth reusing: **the byte threshold had no incident — it was a proxy a session invented; rank 3b has a measured one. The incident is the maker; the session only wrote it down.**
- It also **deflated the PR's own framing** rather than agreeing with it: at most **two** of #635's four defects would have been caught *at introduction*; the rest catch **regression** of already-repaired claims. And it found that `app_submit_size_test.go` already pinned the 401/403 suppression **behaviourally** — so "nothing asserted any of it" was wrong: the behaviour was pinned, only the prose was not.

**Dispatch it blind and let it attack the premise.** A framed audit confirms the frame.

## How to verify

🔴 **This block was stale once already** — it named a deleted threshold and the narrow `-run` filter. If you find a third copy anywhere, that one is stale too.

```bash
# The arc's closing condition. There is NO byte threshold — see the top of this doc.
# Report the size if you like; nothing gates on it:
git -C <repo> show origin/main:README.md | wc -c          # 281,439 at 079dc14

# The gate, WIDE filter, with its own positive control.
# `ok` with 0 tests run is the failure this replaced. Measured 80 on 2026-09-16.
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # 80 — NAME this pattern when quoting it
go test ./...                                            # 21/21 packages, 9 others read README

# The guards #639 added, and the one mutation that proves the ledger earns its place:
go test ./internal/cmd/ -run 'EntryBlock|PreUploadRefusal' -count=1 -v
#   then, in a cp -a copy with .git REMOVED FIRST (a worktree's .git is a FILE):
#   delete `&& !errors.Is(err, civitai.ErrRateLimited)` from doUpload's switch
#   -> TestREADMESubmitEntryBlockSentinelLedger is the ONLY test in the tree that fails.

# lint is a SEPARATE CI job; make ci does not run it
nix-shell -p golangci-lint --run "golangci-lint run ./internal/cmd/..."
```
