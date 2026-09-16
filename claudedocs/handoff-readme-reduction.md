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

**Nine PRs merged across this arc; #641 is open and BLOCKED on a stale scaffold pin that is not its doing. README is 281,439 bytes on `main`. The byte threshold is GONE (see the closing condition); size is reported because it is the cheapest progress signal, never as a gate, and no PR should be shaped to hit a number.**

| | bytes |
|---|---:|
| arc start (`426288f`) | 310,647 |
| after phase 1 — accuracy, #611 | 314,712 |
| after phase 3 — Troubleshooting, #625 | 305,329 |
| after phase 2 — cuts, #630 | 293,378 |
| after phase 3b — command table, #633 | 282,312 |
| after phase 3c — preamble, #635 | 281,439 |
| **on `main` now (`079dc14`)** | **281,439** |
| *pending* — rank 1 agent-setup, #641 | *279,038* |

⚠ **#635 landed at −873, not the −1,741 its body advertises.** Four audit rounds added 868 B back, every byte of it buying a contract claim that is now true. Under the old threshold that would have read as the PR failing; it is the PR working.

### Merged
- **#611** (`f284d89`) phase 1, accuracy. Six corrections each pinned by a guard watched to fail, **plus a behaviour fix**: `civitai upgrade` derives the release-asset extension per GOOS and works on Windows. Closed **#613**. Four audit rounds.
- **#625** Troubleshooting 35,250 → 19,182. Symptom column byte-identical.
- **#630** phase 2 cuts, −14,279, **give-back 0** — the PR that proved delete-first.
- **#633** command table 21,391 → 9,664 (−54.8%), **plus a new guard**: `TestREADMECommandSynopsesNameRealFlags` drives the live Cobra tree, 25 rows / 33 resolutions / 90 flag checks, floors 30/85.
- **#615** the automated pin bump (not this session's) — unfroze the repo and satisfied the previous arc's cli#530 closing condition.
- **#623** (`868fff6`) the plan + this handoff, and the commit that **deleted the byte closing condition**.
- **#636** (`ada6699`) two comments that claimed coverage they did not have.
- **#635** (`c7ea087`) phase 3c, the preamble. Four audit rounds; the record is under Gotchas.
- **#639** (`079dc14`) **rank 3b** — pins the four contract claims #635's rounds repaired, ledgered against the switch they describe.

### Open
- 🔴 **#641 — rank 1, THIS SESSION. Open, MERGEABLE, and BLOCKED by a required check it did not break.** README-only: the `agent-setup` section 16,971 → 14,265 B (−2,706, −15.9%), 265 → 236 lines, 0 → 5 `###` subheadings each with a Contents entry. `pins-vs-published` is red because `@civitai/app-sdk` published **0.41.0** against the `^0.40.0` pin in `templates/page-money/package.json.tmpl`. **Control run: the same test fails identically on a clean `main` tree** (`git status` empty, HEAD `079dc14`), and `main`'s own run at 15:43Z today was green — so the pin went stale in between. It is a **required context**, so #641 cannot merge until `bump-scaffold-pins` runs. That is **operator-owned**; this session did not touch the pin. All four other required gates pass (`build-test`, `scaffold-currency`, `ready-ack-runtime`, `template-page-vite`), as do `lint` and `schema-drift`.
  🔴 **#641 has had NO adversarial audit round.** This session could not dispatch subagents. Given this arc's record — three of four findings on #635 were defects in the previous round's fix, all in compressed prose — treat #641 as unaudited compression and run `/audit-pr 641` before merging.
- **#640** — the handoff-close PR for rank 3b, another session's, open against **this same doc**. 🔴 It and the handoff PR carrying THIS update both edit `claudedocs/handoff-readme-reduction.md`; whichever merges second must rebase.
- **#637** — 🔴 **a CODE defect, not a docs one.** `printSubmitSizeDiagnosis` prints `What this CLI sent` for errors raised *before any HTTP request is built*, so the past tense can be false. Measured against a real `httptest` server: **0 requests received**, block printed, with a positive control on the hit counter. `appblocks.go:570` returns on `Token()` before `doOnceWith` builds anything; `auth/source.go:115` returns `persist refreshed tokens` untagged. A second comment on it carries the mirror case — `auth/source.go:90` tags `ErrUnauthorized` onto a purely *local* error, suppressing the block with no HTTP status anywhere. **Its closing condition includes reverting the two README surfaces #635 had to weaken.**
- **#602** (another session's) — now **22 commits behind** `origin/main` and still degrading. Not ours to rebase.
- **#614** open and optional — the `CIVITAI_NO_COLOR` value-parsing asymmetry, documented and guarded.

### Honest limits
- 🔴 **Nobody has run `civitai upgrade` on Windows.** The final self-replace is stubbed behind the `applyUpdate` seam.
- **The estimates under-delivered four times running** — Troubleshooting 13,000→20,229, phase 2 20,500→14,279, preamble 6,000→1,741, and now rank 1: a 16,971-byte section yielded **2,706**. Every time the cause was the same and it is now measured rather than inferred (see Gotchas).
- **`--session` sees almost nothing of this work** — files went through worktrees. Use `--pr`.

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

🔴 **Ranks 0, 1, 2, 3 and 3b are CLOSED and are deliberately no longer ranks** — a closed item is not work, and carrying it as a numbered rank inflates the `forcing: none` count that the write gate ratchets on. Live numbering below is UNCHANGED so existing `claim-work` slugs keep resolving; the closures are recorded in **State now**, not here.

- **0** closing condition — threshold deleted 2026-09-15, not re-set.
- **1** trial-cut `## Set up your coding agent (agent-setup)` — **CLOSED 2026-09-16, measured not estimated: −2,706 B (−15.9%), PR #641.** Do not re-open it as a size item; the Gotchas entry explains why the remaining 11,518 B of prose is contract rather than fat. What is NOT done: #641 is unaudited (rank 5) and blocked (rank 4).
- **2** the #635 ladder — merged `c7ea087` after four rounds.
- **3** #623 — merged `868fff6`; #636 also merged (`ada6699`).
- **3b** pin the prose #635 repaired — merged as **#639** (`079dc14`); #640 carries its handoff close and is still open.

4. 🔴 **Bump the stale scaffold pin — it is the ONLY thing blocking #641, and it blocks every other PR in this repo too.** `@civitai/app-sdk` 0.41.0 vs `^0.40.0` in `templates/page-money/package.json.tmpl`; `pins-vs-published` is a required context and is red on `main` itself. Fix is `go run ./internal/scaffold/cmd/bump-pins` (and the matching assertion in `scaffold_test.go`), normally raised by `bump-scaffold-pins.yml`. **Operator-owned by standing decision** — an agent may not merge those. Closing condition: `pins-vs-published` green on `main`.
   forcing: gate
5. **Audit and merge #641.** `/audit-pr 641` round 0 first, then the correctness axes; it is a prose-compression PR with zero audit rounds, which is the exact shape that produced #635's four-round ladder. Closing condition: a round returns no high-severity findings AND the required contexts pass (needs rank 4 first).
   forcing: gate
6. **#602** — operator's. Green on its own checks but `DIRTY` against main and 22+ commits behind; its inclusive-ceiling correction must survive the rebase.
   forcing: gate
7. **Phase 4 — the reorder.** Byte-neutral. Fixes the measured reader paths: the CI/`--json` reader travels 81% of the file to reach the exit-code contract, and the `listing status`-is-not-a-read warning sits ~36% in where a `--json` reader never passes it. **This is the highest-value remaining phase** — with the byte target gone the arc's goal is legibility, which is what a reorder buys and a byte count never measured. Rank 1 is now direct evidence for that reading: its durable win was 5 subheadings, not 2,706 bytes.
   forcing: none
8. **Phase 5 — error messages.** A QUALITY item, not a size item; ceiling measured at ~5,465 B realistic. Do it because the error should carry its own remedy, not to move a number. Needs its own PR and audit round: it changes behaviour.
   forcing: none
9. **#614** — `CIVITAI_NO_COLOR` value parsing. Genuinely optional.
   forcing: none
10. **41 worktrees on this clone.** `git worktree prune` removed NOTHING — they are live entries, not orphans. Not acted on: several hold *other sessions'* branches; high blast radius, operator's call. Costs: whole-repo `grep` returns a dozen copies of every hit (#635's round 3 produced a false 🔴 that way), and a worktree holding a merged branch blocks `--delete-branch`.
   forcing: none
11. 🔴 **`README.md:1679`/`:1728` state a body size that is arithmetically impossible**, under a sentence asserting *"That number is exact, not an estimate."* Pre-existing since #452; `10935065` appears only in `README.md`, so it is unpinned by construction. Either re-derive it from `SubmitBodySize` and pin it, or stop quoting an exact number.
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

### Added 2026-09-16 — rank 1 measured: what a 16,971-byte section actually yields

🔴 **NOTHING PINS THE `agent-setup` SECTION'S BODY — measured by maximal mutation, not inferred.** Deleting all 264 body lines and keeping only the heading leaves `go test ./...` **fully green** (21 packages, 0 failures). The handoff's prior "~168 bytes are pinned" overstates it: only the heading and its `## Contents` entry are pinned.

- **The instrument was validated before its verdict was believed**, and the first one failed. Matching test-file string literals against the section produced 78 "pinned" strings — and its positive control failed (a string known to be pinned elsewhere lives in a test but not in README), proving it detects **coincidence, not pinning**. Discarded. The mutation instrument's own positive control does hold: changing the heading turns `readme_nav_test.go` red in **both** directions (`…CoversEverySection` and `…ListsNothingElse`), which is what proves the suite reads the mutated file at all.
- 🔴 **THE SECTION IS 84.8% PROSE (14,385 B), NOT TABLES — so the small yield is not a structural floor, it is a judgement.** Banding it: prose 14,385 B (84.8%), tables 1,503 B (8.9%), code fences 1,007 B (5.9%). After the cut the tables and fences are **byte-identical**; all 2,867 B came out of prose, leaving 11,518 B. A deeper cut is available and was **declined**: what remains is published contract (exit codes, the three `--json` shapes, the no-credential rule, the per-agent spellings), and with the byte threshold deleted there is no reason to trade contract for bytes.
- **So the fourth under-delivery has a different cause from the first three.** Those three were "the compressible fraction was smaller than the section looked". This one is "the compressible fraction was large and most of it is contract we chose to keep". Recording the distinction because treating them as the same lesson would wrongly imply phase 4/5 are also capped.
- **The durable win was structural, and it is the one the arc's goal actually names**: 265 unbroken lines with no internal map became five `###` subsections, each reachable from `## Contents`. 🔴 **Adding a `###` is not free** — `readme_nav_test.go` requires a Contents line for every `##`/`###` (except children of Exit codes/Troubleshooting) *and* requires every Contents anchor to resolve. Both directions fire; budget the Contents entries as part of the edit (+305 B here, which is why the section fell 2,706 B and the file only 2,401).
- **Link-out worked here where Option B was declined, and the difference is worth keeping.** Option B was declined because moving bytes out loses them to tarball/Homebrew/npm readers. Rank 1 **deleted duplicated rationale that already existed** in `claudedocs/decisions/34`, `/35`, `/36` (103,879 B) and linked it by absolute URL — delete-first, not relocate. 🔴 **Verify link targets exist: `git cat-file -e origin/main:<path>` for each. No test checks that**, and a published dead link is a defect no gate would catch.
- 🔴 **One false claim found while grounding sentences against source, and it had shipped.** The section said *"Three rows are **reported by `--check` and never fail its verdict**"*, naming `authenticated`, an absent `Authorization` header, and `claude-md`. `agent_setup.go`'s `checkCountsTowardVerdict` excludes exactly **two** names (`authenticated` always; `claude-md` unless the agent is `claude`). An absent header is **not a check row at all** — it is a condition inside `mcp-*` rows that are `ok: true` anyway (decision 34; `TestCheckDoesNotFailOnAnAbsentHeader`). Same defect class as every other finding in this arc: a sentence claiming more than its code does. Now stated as the predicate computes it.

### Added 2026-09-16 — a required check went red on `main` without anyone touching it

🔴 **`pins-vs-published` is a REQUIRED context and it depends on npm, so `main` can go red with no commit.** Measured today: main's run at `079dc14` (15:43Z) was **green**; ~50 minutes later #641 went red on it. `@civitai/app-sdk` published **0.41.0** against the `^0.40.0` pin (pre-1.0 caret locks the minor).

- **The control is what makes this attributable, and it cost one command.** Running `CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1` on a **clean `main` tree** (`git status` empty, HEAD = `origin/main`) reproduces the failure with zero local changes. Without it the plausible story — "my PR broke a check" — is indistinguishable from the true one, and a README-only PR would have been debugged for nothing.
- **Required contexts, measured today** (`gh api repos/civitai/cli/branches/main/protection`): `pins-vs-published`, `scaffold-currency`, `build-test`, `ready-ack-runtime`, `template-page-vite`. **`lint` is NOT among them** — consistent with `AGENTS.md`, and a reason to keep running it locally.
- **Do not fix it in a feature PR.** The bump touches a vendored scaffold pin and the matching assertion; `bump-scaffold-pins.yml` raises it and those PRs are operator-owned by standing decision.

### Added 2026-09-16 — the shared-queue lock worked, and the sweep is what made it cheap

`claim-work --slug-for claudedocs/handoff-readme-reduction.md 1` → `readme-reduction-1`, claimed before any edit. The `gh pr list --state open` sweep beside it is what surfaced **#640 already editing this very doc** — an unclaimed, invisible-to-the-lock overlap. Recording it because the sweep is the only half that sees a duplicate nobody claimed, and here it changed the plan: the handoff update had to go to its own branch rather than be written casually.

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
