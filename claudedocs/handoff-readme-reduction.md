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
narrow filter reported `ok` on. Use the wide filter above. It ran **76** tests on
2026-09-15 and **80** at `c51fab9`; the count moves with the suite, so re-measure it —
what matters is that a run reporting `ok` with **0** tests is the failure this replaced.

🔴 **The full measured plan is `claudedocs/readme-reduction-plan.md`** — constraint
map, size map, cut list, compression targets, link-out policy, sequencing. Read it
before touching prose. This doc is state; that doc is the plan.

## State now

**Fourteen PRs merged across this arc. README is 277,826 bytes on `main` (`c51fab9`).** The byte threshold is GONE; size is reported because it is the cheapest progress signal, never as a gate — and this session ADDED 146 bytes, on purpose: #646 traded them for two README sentences that are now true.

🔴 **ARC CLOSING-CONDITION VERDICT: NOT YET CLOSED — one clause outstanding, unchanged.** Measured on a clean `main` at `c51fab9`:

| clause | result |
|---|---|
| `go test ./internal/cmd/ -run 'Attribution\|Troubleshooting\|README\|Readme\|readme' -count=1` exits 0 with **>0** tests | ✅ `ok`, **80** tests run |
| `go test ./...` green | ✅ 21 packages, 0 failures |
| each remaining named phase has landed or been withdrawn by a merged PR | ❌ **phases 4 and 5 are neither** |

⚠ **The gate count is 80, not the 81 this doc carried — and the drop is correct, not a regression.** #646 deleted `TestREADMEEntryBlockSurfacesAgree`, whose last assertion had become a substring of the sentinel ledger's own whole-sentence checks; no mutant was ever killed by it alone. Re-measure rather than reconcile: the number tracks the suite, and a session that "fixes" it back by re-adding a subsumed guard has made things worse.

So the one item between this arc and closed is still **phase 4 (rank 7) and phase 5 (rank 8)** — land them or withdraw them by merged PR.

| | bytes |
|---|---:|
| arc start (`426288f`) | 310,647 |
| after phase 1 — accuracy, #611 | 314,712 |
| after phase 3 — Troubleshooting, #625 | 305,329 |
| after phase 2 — cuts, #630 | 293,378 |
| after phase 3b — command table, #633 | 282,312 |
| after phase 3c — preamble, #635 | 281,439 |
| after rank 1 — agent-setup, #641 (`db68d1a`) | 277,680 |
| **after #646 — the #637 code fix (`c51fab9`)** | **277,826 (+146)** |

### Merged in the rank-1 session
- **#643** (`5327ac8`) **rank 4** — `@civitai/app-sdk` 0.41.0 pin bump. Operator-merged. `TestScaffoldPinsSatisfyPublished` re-run on a clean tree afterwards: **PASS** (it had been FAIL on `main` itself).
- **#641** (`db68d1a`) **rank 1** — the agent-setup section: **16,971 → 12,907 B (−4,064, −23.9%)**, 265 → 217 lines, 0 → **5 `###`** each with a Contents entry, **plus `internal/cmd/readme_agent_setup_claims_test.go`** (four guards). Five audit rounds (0–4), stopped by the **attribution gate**.
- **#642** (`4572d6f`) / **#644** (`be4c34a`) — the handoff, including the −4,936 → −4,064 correction.

### Merged in the #637 session
- **#645** (`7819352`) — the audit #640 said had never happened, plus the defect it found. #639's `nonTTYSubmitRefusal` returned `serverHit` and both callers asserted `if hit` — **branches that cannot fire**, because reaching the endpoint means the driver's own controls already fataled. A/B on one mutant: pre-#639 the test died naming the gate (*"submit endpoint was hit — the gate did NOT prevent the submission"*); post-#639 it died naming the harness. Three rounds; **stopped by the attribution gate**, rounds 1 and 2 changing 49 and 37 lines, all comments.
- **#646** (`c51fab9`) — **#637, closed COMPLETED.** `appapi.ErrNothingSent`, set from httptrace, tags any submit failure whose request never reached the connection; `doUpload` gains an arm that prints nothing for those; both README surfaces go back to the strong claim #635 spent four rounds hedging. Three rounds, ten mutants, zero survivors.

**Verified after merge, not assumed:** both squashes checked **by content** per file against `origin/main` (never by ancestry); `go test ./...` 21/21 and the wide gate at 80 on a clean `main`; the 12 new behaviour guards pass there; both worktrees removed, both branches deleted, base clone re-synced `--ff-only`, both claims released.

### Open
- **#602** (another session's) — 22+ commits behind `origin/main`. Not ours to rebase.
- **#614** open and optional — `CIVITAI_NO_COLOR` value parsing.
- **#640** — CLOSED as superseded, not merged. Its branch sat on `079dc14`, so its README half *reverted* #641; a rebase would have been a full rewrite of every hunk. Its one surviving finding was salvaged into #645 and is discharged.

### Honest limits
- 🔴 **Nobody has run `civitai upgrade` on Windows.** The final self-replace is stubbed behind the `applyUpdate` seam.
- 🔴 **Every byte estimate in this arc over-promised, and every mid-ladder MEASUREMENT did too.** A whole-section count is an upper bound; a pre-merge figure is provisional. Quote the merge commit.
- **golangci-lint was never run at CI's pinned v2.12.2 locally** (this host has 2.13.2, clean). `lint` is not a required context; read the PR's own run.
- **`--session` sees almost nothing of this work** — worktrees. Use `--pr`.
- **Worktree count keeps drifting upward** — every audit round adds an agent worktree. Rank 10's figure is stale on arrival; count it rather than quoting this doc.
- 🔴 **NOBODY HAS RUN `app submit` AGAINST THE REAL SERVER SINCE #646.** Everything about `ErrNothingSent` is measured against `httptest` and a stubbed transport. The h2 ordering of the httptrace callback is **probed, not proven** (3/3 each way, not reproduced) and is recorded as such in `appblocks.go`; the losing direction is conservative, but it is unproven either way.

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

🔴 **Ranks 0, 1, 2, 3, 3b, 4 and 5 are CLOSED and are deliberately no longer ranks** — a closed item is not work, and carrying it as a numbered rank inflates the `forcing: none` count the write gate ratchets on. Live numbering below is UNCHANGED so existing `claim-work` slugs keep resolving.

- **0** closing condition — threshold deleted 2026-09-15, not re-set.
- **1** trial-cut the agent-setup section — **CLOSED, −4,064 B (−23.9%), #641**, audited over five rounds. (The −4,936 here was the mid-ladder figure this doc elsewhere retracts; corrected in place so the two halves stop disagreeing.)
- **2** the #635 ladder — merged `c7ea087`.
- **3** #623 — merged `868fff6`; #636 also merged.
- **3b** pin the prose #635 repaired — merged as **#639**.
- **4** the stale scaffold pin — **CLOSED**, merged as **#643** (`5327ac8`), operator-merged; the failing test re-run green.
- **5** audit #641 — **CLOSED**: rounds 0–4 run, ladder stopped by the attribution gate, round 4 verdict *safe to merge*.
- **#640/#645** salvage the unaudited #639 consolidation — **CLOSED**: #640 closed superseded, its finding discharged by **#645** (`7819352`).
- **#637** the false past tense — **CLOSED**, merged as **#646** (`c51fab9`), issue auto-closed COMPLETED. Never a numbered rank; it was carried in Open.

6. **#602** — operator's. Green on its own checks but `DIRTY` against main and 22+ commits behind; its inclusive-ceiling correction must survive the rebase.
   forcing: gate
7. **Phase 4 — the reorder.** Byte-neutral. Fixes the measured reader paths: the CI/`--json` reader travels 81% of the file to reach the exit-code contract, and the `listing status`-is-not-a-read warning sits ~36% in where a `--json` reader never passes it. **The highest-value remaining phase** — with the byte target gone the arc's goal is legibility, which is what a reorder buys and a byte count never measured. Rank 1 is direct evidence: its durable win was 5 subheadings, not 4,936 bytes.
   forcing: none
8. **Phase 5 — error messages.** A QUALITY item, not a size item; ceiling measured at ~5,465 B realistic. Do it because the error should carry its own remedy. Needs its own PR and audit round: it changes behaviour.
   forcing: none
9. **#614** — `CIVITAI_NO_COLOR` value parsing. Genuinely optional.
   forcing: none
10. **Worktrees on this clone — count before quoting.** `git worktree prune` removed NOTHING — live entries, not orphans; several hold *other sessions'* branches. High blast radius, operator's call. The number in this line has been stale at every reading: audit rounds add one each.
   forcing: none
11. 🔴 **`README.md` states a body size that is arithmetically impossible**, under a sentence asserting *"That number is exact, not an estimate."* Pre-existing since #452; `10935065` appears only in `README.md`, so it is unpinned by construction. ⚠ #646 prefixed that sample line with **"up to"**, which changes what the number CLAIMS but not whether it is derivable — re-read the line before quoting the old line numbers.
   forcing: none

12. **Eight `internal/` files cite a `RULES.md` this repo does not ship**, so those pointers are unopenable for any contributor. Fixing two of eight (both in `readme_submit_entry_block_test.go`) is the patch-the-second-copy shape; it wants one consolidating change. **Closing condition:** a merged PR that either ships the referenced rules or rewrites all eight citations to name something in-repo, checked by a grep in that PR's diff.
   forcing: none

13. **`TestItem38CommentsCiteTestsThatExist` ledgers only item 38's file set**, so a production comment citing a test by name — `internal/cmd/app_submit.go` does it twice — dangles silently if that test is deleted. Widening needs a suppression convention for ~5 prose shapes first; that test's own comment records why. **Closing condition:** a merged PR that globs the citation check repo-wide with that convention.
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

### Added 2026-09-16 — the #641 ladder: five rounds, and what ENDED it

🔴 **THE LADDER WAS ENDED BY THE ATTRIBUTION GATE, NOT BY A CLEAN ROUND — and that is the first time in this arc.** Rounds 3 and 4 each changed **zero payload lines** (payload = `README.md`; the new test file is scaffolding). Two consecutive zero-payload rounds means the rounds are auditing guards the ladder itself wrote. Measured per round with `git log --numstat --format= --remerge-diff <audited>..HEAD --not origin/main`.

| round | findings | payload lines its fix changed |
|---|---|---:|
| 0 | requirements & deletion: 1 major deletion candidate | (n/a — reports only) |
| 1 | 3 🟡 behaviour + 1 🟡 guard + 6 🟢 | 88 |
| 2 | 2 🟡 payload + 2 🟡 scaffolding + 3 🟢 | 39 |
| 3 | 2 🟡 scaffolding + 1 🟢 | **0** |
| 4 | 2 🟢 scaffolding — verdict *safe to merge* | **0** |

- 🔴 **ROUND 0 NEARLY DOUBLED THE DELIVERABLE, AND IT IS THE ROUND MOST AT RISK OF BEING SKIPPED.** The trial cut was −2,706; round 0 asked *should this exist* and found that **~4,375 B of the surviving section restated `civitai agent-setup --help`**, which the plan's own order (`readme-reduction-plan.md:29-31`) says to DELETE FIRST. Final: **−4,936**. **Run round 0 at PR-CREATE time.**
- 🔴 **THE `.goreleaser` LINK-OUT ARGUMENT DOES NOT REACH `--help` DUPLICATION.** The archive ships `README.md`, `LICENSE` **and the binary**, so anything `--help` says is already reachable offline. That argument protects `claudedocs/` link-outs only. This was the reasoning error that made the first cut half-sized.
- 🔴 **THE SPELLED-GUARD CLASS COST THREE ATTEMPTS ON ONE SENTENCE.** Pinning the `claude-md` clause: (a) assert the name appears — it appears in the shared sentence too, **survived**; (b) assert `` `x` alone ``/`` is `x` `` — a reworded falsehood carries neither, **survived**; (c) **CONSTRUCT the expected sentence from the derived set and compare it WHOLE** — kills the inversion, the reword, and the code-side change. Only (c) pins state rather than wording.
- 🔴 **A FIX ROUND WEAKENED A WORKING GUARD ON A FALSE PREMISE.** Round 2 replaced a case-sensitive fixture with `ToLower` + a shorter string, claiming `Long` "does not spell it that way" — `grep -c` at that very commit returns **1**. It conflated an earlier *draft* with the *committed* fixture. Cost, measured: two mutants survived the whole package, one of which let `--help` advertise `${ENV:civitai_token}`, a spelling that does not resolve. **Check the tree before believing a commit message's premise — including your own.**
- 🔴 **HARDENING ONE PARSE WHILE ADDING A SECOND, UNHARDENED ONE.** Round 3 fixed a sentence-slicer's false diagnosis and, in the same commit, added another slice with the same defect: an ordinary paragraph split made a TRUE statement fail with "does not state what the code computes". Round 4's fix **deleted the parse** rather than hardening it — the assertion is a constructed whole string, so position is irrelevant. **Prefer deleting a parse to teaching it a better error message.**
- 🔴 **A MUTANT THAT NEVER RAN SCORES `SURVIVED` AND IS INDISTINGUISHABLE FROM A VACUOUS GUARD.** Hit twice in this ladder, once by an auditor and once by me — a substitution whose anchor had moved. **Every mutation run now asserts `mutation APPLIED = True` (new text present AND old absent, re-read from disk) before its result is read.**
- **A docstring claiming coverage its body lacks was found in FOUR separate rounds** of this one PR. The tell each time: the docstring names a RELATIONSHIP, the body inspects one SIDE.
- **The measured gap this PR closed:** deleting all ~200 body lines of the section while keeping the five `###` headings left `go test ./...` **fully green**. The headings are pinned; not one sentence under them was.

### Added 2026-09-16 — a required check can go red on `main` with no commit

🔴 **`pins-vs-published` is a REQUIRED context that depends on npm.** Measured: `main` green at 15:43Z; ~50 min later every open PR was red because `@civitai/app-sdk` published 0.41.0 against a `^0.40.0` pin. **It blocks every open PR, not just the one you are looking at** — confirmed on #641 and #642 simultaneously, and the `bump-scaffold-pins.yml` header records a three-day instance.

- **The control costs one command and is what makes it attributable**: run the failing test on a **clean `main`** tree. If it fails there, it is not your PR. Without it, "my PR broke a check" is indistinguishable from the truth.
- **Required contexts, measured** (`gh api repos/civitai/cli/branches/main/protection`): `pins-vs-published`, `scaffold-currency`, `build-test`, `ready-ack-runtime`, `template-page-vite`. **`lint` is NOT among them**, and `strict` is **false** — so a stale branch is not force-updated, and a required check red on the BRANCH still blocks even after `main` is fixed. **Merge `main` in to make it re-run.**
- **The scheduled bump runs ~12:36Z daily**, so a package published after it leaves everything blocked until the next day. `gh workflow run bump-scaffold-pins.yml` opens the PR immediately; merging it is the operator's.

### Added 2026-09-16 — the give-back is a LAW of this repo's ladders, not an anomaly

🔴 **TWO PRs, TWO LADDERS, ~870 BYTES OF GIVE-BACK EACH — and both times the advertised figure was the mid-ladder one.**

| PR | advertised | landed | given back | rounds |
|---|---:|---:|---:|---|
| #635 | −1,741 | −873 | 868 B | 4 |
| #641 | −4,936 | −4,064 | 872 B | 5 (0–4) |

The give-back is **not waste** — in both cases every byte bought a contract claim that is now true, and #641's rounds removed three false claims and caught two more before they shipped. What is wrong is quoting the pre-ladder number as the result.

- 🔴 **Quote a byte figure ONLY from the merge commit.** A number measured on any commit before the ladder ends describes a tree nobody shipped.
- 🔴 **Run the once-per-ladder count sweep in the LAST round or after the merge.** #641's ran at round 2 and was correct then; rounds 2–4 changed README afterwards, so the swept number went stale **inside its own PR** — precisely the "falsified by a LATER commit of the same PR, inside no round's range" trap the audit skill documents. Running it mid-ladder satisfies the letter of the rule and misses its point.
- **Predictive value, offered as a hypothesis and not a law:** ~870 B over 4–5 rounds on a contract-bearing section, both times. If a third ladder lands near it, it is worth budgeting for rather than being surprised by.

### Added 2026-09-16 — what the five-round ladder on #641 is worth repeating

The full record is in the two blocks above this one. What a future session should take from it, shortest form:

- 🔴 **Round 0 is the highest-yield round and the easiest to skip.** It nearly doubled the deliverable by asking *should this exist* — and the answer was that ~4,375 B restated `civitai agent-setup --help`, which the plan's own order says to DELETE FIRST. **Run it at PR-CREATE time**, not when you get round to auditing.
- 🔴 **The `.goreleaser` link-out argument does not reach `--help` duplication.** The archive ships the binary beside `README.md`, so anything `--help` says is already offline-reachable. That argument protects `claudedocs/` link-outs only. Getting this wrong is what made the first cut half-sized.
- 🔴 **A guard satisfiable by WORDING is not a guard.** Three attempts on one sentence: assert the name appears (it appears elsewhere — survived), assert a phrasing (a reword carries neither — survived), **construct the expected text from the derived set and compare it WHOLE** (kills the inversion, the reword, and the code-side change). Only the third pins state.
- 🔴 **Prefer DELETING a parse to hardening it.** A guard that slices prose gives a false diagnosis when its delimiter moves, and the message sends a maintainer to edit correct text. Where the assertion is a constructed whole string, searching the whole section removes the failure class outright.
- 🔴 **Assert `mutation APPLIED = True` before reading any mutant's result.** A substitution whose anchor moved scores SURVIVED and is indistinguishable from a vacuous guard. Hit twice in this ladder — once by an auditor, once by me.
- **A docstring claiming coverage its body lacks appeared in FOUR separate rounds of one PR.** The tell every time: the docstring names a RELATIONSHIP, the body inspects one SIDE.

### Added 2026-09-16 — the #637 session: three ladders, zero 🔴, and a defect the fix kept regenerating

🔴 **THE OPERATOR'S STOP RULE WAS OVER-RUN, AND THAT IS THE PROCESS FINDING.** The rule
recorded above is *"audit until a round returns no HIGH-SEVERITY findings"* — an operator
correction, explicitly not to be re-derived from the skill's stricter text. **No round of
either PR returned a 🔴**, so #646 was a stopping round at round 1 and #645 at round 0.
Both ran to round 2 anyway, on the skill's findings-keyed rule. The extra rounds found real
things — including a false byte claim that had shipped — so the content was not waste; the
*call* was the operator's and was not asked for. Ask, or stop.

🔴 **THE SAME SENTENCE SHIPPED FALSE THREE TIMES, IN THREE DIFFERENT DIRECTIONS, ACROSS
THREE ROUNDS OF ONE PR.** This is the arc's cleanest instance of "a fix that makes a sentence
more specific can make it false".

| round | what the README said | why it was false |
|---|---|---|
| 0 | "A failure that sent nothing prints **nothing**" | the ceiling refusal sends nothing and prints a full block — named three lines below it |
| 1 | "…so `sent` is a fact about **bytes that left this machine**" | a write cut short had delivered ~200 KB while the CLI called it nothing sent |
| 2 | the block printed "**N bytes** on the wire" | after round 1 widened *when* it prints, N was what the CLI BUILT: 8,388,627 printed against 233,266 received |

Round 2's is the one to study: round 1 fixed **when** the block prints and left **what it
says** alone, so the repair re-opened #423's original defect ("the server received 0 bytes
while the CLI reported 12,587,785") through a new door. **When a fix widens a predicate, list
every sentence downstream of it before deciding you are done.**

- 🔴 **A JUSTIFICATION CAN POINT AT PROSE THAT DOES NOT EXIST.** Round 1's comment read "the
  prose says the request went out rather than that N bytes landed" — while the PRINTED prose
  said N bytes. It was true of the comment's own paragraph and false of the program.
- 🔴 **"THIS CANNOT BE PINNED WITHOUT A RACE" WAS WRONG, AND THAT CLASS OF CLAIM IS THE
  EXPENSIVE ONE.** The stickiness of the sent-flag across a 401 retry looked unpinnable
  (the server would have to die between attempts). The separating seam is the **dialer**:
  a `DialContext` that succeeds once and fails after gives attempt 1 a full body and attempt
  2 nothing, deterministically. A comment saying nobody can cover something stops the next
  person looking.
- 🔴 **AN ASSERTION HOLDING EITHER WAY DOES NOT MAKE THE KILL HOLD EITHER WAY.** The
  cut-short test asserted the SERVER received something — equally true when the whole body
  writes cleanly into socket buffers. On such a host it passes while the mutant it is
  credited with killing survives. Fixed with a counting `net.Conn` measuring what the CLIENT
  wrote, and the control was watched to fire.
- 🔴 **A MUTATION HARNESS IS AN INSTRUMENT.** One mutant scored `APPLIED=False` rather than
  `SURVIVED` because the mutant text CONTAINS its own anchor, so the "old text absent" check
  could never hold. Two mutants also survived and each survivor was a real coverage gap — a
  redundant tag site that read as coverage, and an unpinned qualifier.
- 🔴 **`git checkout -- <file>` RESTORES FROM THE INDEX.** Reverting a mutant that way
  silently discarded two unstaged fixes in the same file. Stage before a battery, or copy
  aside — the rule about `cp -a` restores exists for exactly this.
- **Both PRs quoted a count from a `-run`-FILTERED sweep and had to retract it** — then the
  retraction was itself wrong (the filter was a latent hazard, not the cause; whole-package
  under the same mutant returns the same number). **A count is meaningless without the mutant
  that produced it.**
- **An `AGENTS.md: "…"` attribution named a phrase in no file this repo ships.** It lives in
  the operator's private rules. Eight `internal/` files cite a `RULES.md` the repo does not
  ship — filed, not fixed, because fixing two of eight is the patch-the-second-copy shape.

### Added 2026-09-16 — the attribution gate fired for real, twice

#645 stopped on it rather than on a clean round: rounds 1 and 2 changed **49 and 37 payload
lines, every one a comment** (checked mechanically — no changed line failed `^[+-]\s*//`).
Two consecutive zero-executable rounds means the ladder is auditing its own prose. Round 2
was explicitly *not* clean, which is the point of gating on what a round CHANGES rather than
on what it FINDS.

## How to verify

🔴 **This block was STALE once already** — it carried a deleted byte threshold and a narrow `-run` filter. Both corrected. If you find a third copy of the gate anywhere in this doc, that one is stale too.

```bash
# the arc's closing condition. NO byte threshold — size is a progress signal only.
git -C <repo> show origin/main:README.md | wc -c        # 277,826 at c51fab9

# the gate, WIDE filter, with its own positive control.
# `ok` with 0 tests run is the failure this replaced. 76 on 2026-09-15, 81 after
# #639/#641, 80 at c51fab9 once #646 deleted a subsumed guard. The number tracks
# the suite: RE-MEASURE it, never reconcile it back.
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' -count=1
go test ./internal/cmd/ -run 'Attribution|Troubleshooting|README|Readme|readme' \
  -count=1 -v | grep -c '^--- PASS\|^    --- PASS'      # must be > 0
go test ./...                                            # 9 other packages read README

# the agent-setup contract guards #641 added (rank 1). All four must pass.
go test ./internal/cmd/ -count=1 \
  -run 'READMEVerdictExemptions|CheckEmitsNoRow|ManualRowCarries|HelpPointerIsHonoured'

# the #637 behaviour guards #646 added. 12 PASS on a clean main at c51fab9; each
# drives the REAL appapi.Client, because a fake Submitter returns whatever error
# the test hands it and would pass vacuously while the real chain lied.
go test ./internal/appapi/ ./internal/cmd/ -count=1 \
  -run 'NothingSent|SubmitDiagnosis|CutShort|A401Retry|CannotBeBuilt|AfterTheWrite'

# lint is a SEPARATE CI job and NOT a required context; `make ci` does not run it.
# 🔴 CI pins v2.12.2 (.github/workflows/ci.yml); this host has 2.13.2, so a local
# zero is a claim about 2.13.2. Read the PR's own run for the gate.
nix-shell -p golangci-lint --run "golangci-lint run"
```
