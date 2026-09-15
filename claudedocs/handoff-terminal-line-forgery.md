# Handoff: terminal-line forgery (the `safeTerm` effort) — 2026-09-15

## Run this first — the index, one command
```bash
cairn recall --repo /home/zach/workspace/civit/cli
```
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading. `scope-absent`/`scope-empty` means nothing is recorded yet: ordinary, not an
error, and not a clean bill of health. Non-blocking: if it exits non-zero, print the
stderr line and carry on.

## Why this doc exists

**This effort accreted inside `handoff-agent-setup-onboarding.md` and is not part of
that arc.** It shares no code, no goal and no closing condition with agent-setup
onboarding; it rode in because that doc was the queue every `/resume` drew from, and
nothing refused it. That conflation is why "is this finished?" has only ever been
answerable by enumerating ranks. Splitting it out is the concrete form of that doc's
rank 29 "split by initiative", and this file is that split.

**Do not merge the two back together.** A close-check against either arc's condition
reads NOT ADDRESSED for reasons that have nothing to do with the other.

## Goal

Server-supplied text — an uploader's file name, a model name, an orchestrator error,
a server `message` — must not be able to forge lines of this CLI's own output. The
hazard is not defacement: it is a **counterfeit assertion by the CLI**, on surfaces
where the CLI is vouching for something. The two canonical payloads, both measured:

- a forged `Saved … (SHA256 verified)` above a transfer that never finished;
- a forged `Cost: … Buzz` line directly above `Generate? [y/N]:`, the last screen
  before an irreversible spend.

- **closing-condition:** `check` — #621 lands, AND a writer-keyed operand enumeration covers
  the money/verification paths (`generate.go`, `generate_wait.go`, `generate_output.go`,
  `download.go`), failing the suite on an ungated server-origin operand; proven by a negative
  control, a NON-ZERO positive control, and a retrospective at the parents of the **#566 and
  #612** fixes. **#624 is deliberately EXCLUDED from the retrospective** — it was an
  insufficiency-of-gate defect on an already-gated operand, which this instrument cannot see by
  construction. Detail, rationale and the two residuals directly below. ⚠ UNRATIFIED.

🔴 **THE FIELD ABOVE IS THE MACHINE-READABLE ONE, AND AN EARLIER DRAFT HAD ONLY THE
HEADING BELOW.** `handoff_doc.py` parses `closing-condition: <kind> — <…>` inside
`## Goal`; it does not parse a `### closing-condition` heading, so the first version of
this doc — whose PR headline was *"give it the closing condition it never had"* —
declared **none** to every reader that matters, and the write gate reported it as
GRANDFATHERED. That is this effort's own recurring defect (something that READS as
coverage while providing none) committed in the document that names it. Keep both: the
field is what is parsed, the section is what is read.

🔴 **AND THE REFUSAL THAT EXISTS FOR EXACTLY THIS WAS OUT OF REACH, BECAUSE THE DOC WAS
CREATED WITH A PLAIN `Write`.** `handoff_doc.py` rule (m) REFUSES a NEW doc with no
`closing-condition:` (`EXIT_UNDEFINED_DONE`, nothing written) — but "new" means no copy
tracked at HEAD, and this file was written and committed directly in `722cf29` before
`/handoff` ever saw it. By then it existed, so the gate took its **advisory**
grandfather arm instead and proceeded. The `/handoff` skill says plainly not to `Write`
the doc yourself and that step 5 is the only step that commits; doing it by hand is what
turned a refusal into a warning. **Create a handoff doc through the gate, even the first
time.**

### 🔴 closing-condition — `check`

**THE HAND ENUMERATION BECOMES MANDATORY AND AMORTISED, SO THE ABSENCE IS RECORDABLE.**

🔴 **AN EARLIER DRAFT SAID "THE HAND ENUMERATION IS RETIRED, BECAUSE AN INSTRUMENT FINDS
WHAT IT FOUND", AND ROUND 0 OF #632 KILLED BOTH HALVES OF THAT SENTENCE. Do not
re-derive it.** The retractions are recorded here because this doc's whole subject is
claims that read as coverage while providing none, and its own condition was one.

**Why the condition exists at all — this part survived.** Every defect this arc has
closed was found by a **human** enumerating operands. Every instrument the repo owns
keys on **the gate** — `scanSafeTermCallSites`, `safeTermCoveredBy`,
`TestSafeTermErrCallersAreLedgered`, `TestSanitizerComposersAreLedgered` all enumerate
functions that ALREADY call `safeTerm` — so all of them are structurally incapable of
finding an operand that has **no** gate. That is the disease, and it is still the thing
to fix.

So the done-state is **both** of:

> **(i)** `#621` lands — the ledger's free-text `why` can no longer scope a surface OUT
> without minting a tracked object. It is keyed on the gate and stays blind to an
> ungated operand, so it is not sufficient; it is the cheap half that hardens what
> exists, and (ii) inherits its defect if it ships without it.
>
> **(ii)** A writer-keyed operand enumeration covers the **money and verification
> paths** — `generate.go`, `generate_wait.go`, `generate_output.go`, `download.go`.
> Every `fmt.Fprint*` / `fmt.Errorf` / `fmt.Sprintf` site there classifies each
> interpolated operand **CLI-owned** / **user-typed** / **server-origin**, a
> server-origin operand with no gate FAILS the suite, and a new writer call site in
> those files cannot be added without classifying it.

🔴 **(ii) IS BOUNDED TO FOUR FILES ON PURPOSE, AND THE EARLIER DRAFT'S "all of
`internal/cmd`" WAS NOT COSTED.** Measured at #632's head: `internal/cmd` holds **1,023**
non-test, non-comment `fmt.Fprint*`/`Errorf`/`Sprintf` sites — against
`maxUncoveredSafeTermFuncs = 9` rows in the ledger it would replace. The four files above
are ~102 + `download.go`, and they are where a forged line is a **counterfeit assertion
by the CLI** rather than cosmetic: the `(SHA256 verified)` claim and the `Cost:` line
above an irreversible spend. Widening beyond them is a decision with a number attached,
not a tidy-up.

**Checked by — all three, because any one alone is walkable:**

1. **NEGATIVE CONTROL.** Plant an ungated server-origin operand in a *new* writer
   call; the check goes red **with its own message**. A check that cannot go red is
   testing nothing.
2. **POSITIVE CONTROL.** The check reports a **non-zero** count of operands actually
   classified, and the number is printed. A reassuring zero is indistinguishable from a
   check wired to nothing — report the pair, never the zero.
3. **THE RETROSPECTIVE.** Run it at the parents of the commits that fixed **`#566` and
   `#612`** and show it finds what the humans found. This is what distinguishes "an
   instrument exists" from "an instrument that would have worked". If it misses them,
   the condition is not met.

🔴 **`#624` IS EXCLUDED FROM CONTROL 3, AND AN EARLIER DRAFT INCLUDED IT — WHICH MADE
THE CONDITION PERMANENTLY UNMEETABLE.** Measured at `b4acda5^`: both #624 operands were
**already gated** (`safeTermSingle(p.name)`, `safeTermSingle(berr.Error())`). #624 was an
*insufficiency*-of-gate defect — the gate bounded the rune class, not length — so a
predicate keyed on "has a gate" reports **pass** and control 3 returns MISS. The draft's
own next sentence then read "if it misses them, the condition is not met", so the
condition refuted itself on one of its three named cases, forever, since #624 is closed
and the history is fixed.

**Two residuals this condition does NOT close, named so it is not read wider:**

- 🔴 **It cannot see a WRONG LABEL.** All three controls check that classification
  *happens*, never that a classification is *true*: a session that labels a
  server-origin operand `CLI-owned` passes all three. That is `#621`'s exact mechanism —
  free text nothing checks — which is why (i) is part of the condition rather than a
  nice-to-have, and it is still only a partial answer at the scale of (ii).
- **The insufficiency class is out of scope entirely** — #624's shape, and `#397` (runes
  are not display cells, so every row count this arc quotes doubles for CJK), and the
  fact that a **bounded** tail can still begin at column zero, which needs a terminal
  width the CLI cannot obtain. Named residuals, not failures of this condition.

⚠ **This condition was written 2026-09-15 by the session that closed ranks 28 and 31,
and it is the FIRST one this effort has ever had.** It is a proposal until an
operator either ratifies it or replaces it — the immediately preceding lesson on this
arc is that a fork resolved without an operator author of record is a finding, so
this one says plainly that it has not been ratified.

## State now

**Closed:** `#574`, `#604`, `#605`, `#612`, `#624` — and `#609` as a duplicate of `#606`.

| PR | rank | merged | what |
|---|---|---|---|
| cli#596 | 23 | `752bf50` | the blob-download forgery; closed `#574` |
| cli#608 | 30 | `a29abb7` | 16 server operands on `generate`; closed `#604` |
| cli#619 | 31 | `7c5a39c` | three operands with NO gate at all; closed `#612` |
| cli#628 | 28 | `b4acda5` | the LENGTH class — bounded two operands; closed `#605` + `#624` |
| cli#632 | 29 | `b017353` | this doc; the split, and the effort's first closing condition |

Every one verified by **CONTENT** with a working positive control, never by ancestry: a squash
merge makes the branch head a non-ancestor forever, so `merge-base --is-ancestor` returns false
after every one of these and means nothing.

**Nothing is in flight.** All claims released and verified absent (`agent-setup-onboarding-23`,
`-28`, `-29`, `-30`, `-31`). The two open PRs in the repo (#623, #602) belong to other efforts.

### 🔴 THE ONE BLOCKING DECISION

**The closing condition in `## Goal` is UNRATIFIED, and round 0 of #632 already broke one
version of it.** It was rewritten in `00c5fec` and is better — bounded, costed, with #624
excluded — but it is still a proposal by the session that wrote it.

🔴 **The recommendation on the table is NOT "ratify it". It is "run one measurement first",**
because the measurement can make the condition unnecessary:

- If the server's `message` never echoes caller-supplied text, the whole class is
  **defence-in-depth against a backend bug** — exploitation needs a compromised backend — and
  the right closure is plausibly *accept the residual in writing and stop*, which RETIRES ranks
  2 and 3 instead of working them.
- If it does echo, this is **uploader → victim with no server compromise**, the severity on
  #629/#620 moves from a floor to a measurement, and the instrument is clearly worth building.

The probe is free and the machinery is already here — see the open investigation below.

### Honest limits

<!-- CARRIED FORWARD deliberately: `## State now` is a REPLACE section, so this subsection is
     deleted by any update that omits it. Caught twice now by reading the `-` lines; the write
     gate printed no durable-drop warning either time. -->

- 🔴 **EVERY CLOSURE SO FAR HAS PRODUCED ITS OWN SUCCESSOR: `#604` → `#612` → `#624` → `#629`.**
  Four rewrites of the same "not wholly gated" sentence. That chain is the strongest argument
  for having a closing condition at all, and the reason a fifth point-fix should not start
  before the decision above is made.
- 🔴 **The threat model is OPEN** — the blocking decision above, and the open investigation
  below. Severity on #629/#620 is a **floor, not a measurement**.
- 🔴 **The audit base rate is EIGHTEEN FOR EIGHTEEN.** Every round that ran found something.
  Ranks 28, 29 and 31 each ran audit rounds and **not one came back clean**.
- 🔴 **Three consecutive rounds found the PREVIOUS round's own prose** — not code, but sentences
  a fix wrote to explain itself. Twice a false absolute was replaced by a narrower absolute that
  was also false.
- 🔴 **THIS DOC'S OWN CLOSING CONDITION HAS NOW FAILED TWICE, IN TWO DIFFERENT WAYS**, and both
  were caught by instruments rather than by reading: (1) it shipped **unparseable** — written as
  a `###` heading, which `handoff_doc.py` does not read, so the doc declared none while its PR
  headline said it added one; (2) rewritten, it was **unmeetable** — control 3 tested #624
  against a predicate structurally incapable of seeing it. Assume the third version is also
  wrong somewhere and look for it.

## How to verify

**Finding the NEXT ungated operand** — and note this is the instrument the closing
condition exists to RETIRE. It is a human enumeration; `git grep 'safeTerm('` is NOT
it, because that only finds operands which already have a gate:

```bash
# enumerate every rendered operand, then TRACE EACH ONE to its origin
command grep -nE 'fmt\.(Fprint[^(]*|Errorf|Sprintf)\(' internal/cmd/<file>.go
```

🔴 **Do not pipe it to `wc -l` and compare against a remembered number.** The
enumeration counts its own prose: on `generate.go` it read 102 → 103 → 102 again
across one PR's three commits, purely from comment lines quoting `fmt.Errorf(` being
added and removed. That tripwire falsified itself inside a single PR and was deleted
rather than renumbered.

**The forgery guards, after any change:**

```bash
go test ./internal/cmd -count=1 -run 'ForgeALine|IsLengthBounded|BoundsTheOperand|GeometryIsNotServerChosen|RowCountIsNotServerChosen|StaysMultiLine|PreservesClassification|CallSitesAreCovered|CallersAreLedgered'
# ⚠ every alternative above MATCHES a real test — an earlier draft carried `SoftWrap`, which
# matches NONE (the file is softwrap_forgery_test.go; its funcs are BoundsTheOperand and
# IsLengthBounded), and a -run alternative that silently matches nothing is the class this
# doc exists to catch. Verify with -v and count the RUN lines, never the exit code.
# then revert ONE gate and confirm the named test dies with ITS OWN message
```

⚠ **Three traps in that loop, all measured:**
- A **TAB assertion is inert** on a `ui`-styled surface — lipgloss expands `\t` to
  four spaces before the bytes reach the writer — and live only on bare
  `fmt.Fprintf`.
- `TestClassifyGenerateErrorFallThroughPreservesClassification` is an **invariant
  guard that cannot fail alone**: nine pre-existing tests catch the same mutant. Do
  not cite it alone in a mutation matrix.
- `swMaxRowsAt80` pins **ROWS, not the cap**. Boundary measured: cap 135 green, 136
  red. "The cap is pinned" is the wrong sentence.

**The ledger will NOT demand a row for the next one.** GREW iterates the functions
that already CALL `safeTerm`, so one call plus one row satisfies it however many
further ungated operands that function grows. That is `#621`, and it is why the
closing condition is keyed on the writer instead.

## Gotchas specific to this effort

- 🔴 **A guard can be SPELLED rather than STRUCTURAL.** Assert the STATE — geometry,
  an escape class, a row count — never a word another feature can spell. Both `#605`
  and `#624` named a word-guard as the one unacceptable closure.
- 🔴 **A guard that derives its expectation from the thing it constrains is blind to
  that thing changing.** Measured on `#628`: every geometry assertion derived from
  `maxServerLineRunes`, so at cap 5000 both stayed GREEN while the surfaces occupied
  63 and 64 display rows — the forgery essentially restored. One absolute anchor
  closes it.
- 🔴 **Prove a guard REACHABLE, not merely breakable.** On `#619` a probe for a new
  predicate died at an EARLIER control, which proves nothing about the new one; a
  second payload that got past it was needed.
- 🔴 **A figure carried in prose beside the thing it counts WILL drift**, and this
  arc proved it three times in two PRs ("~16" vs 131 call sites; "148 runes" vs 155;
  "226" vs 225; "2 rows at 100" vs 3). Nothing asserts on them, so they are unpinned
  by construction. **Either pin it or stop quoting it** — compute and print at run
  time instead.
- 🔴 **A row count quoted beside a surface it does not belong to.** "7 rows directly
  above `Generate? [y/N]:`" was the *download* line's figure; that surface costs 8.
  Same defect, one file over, one round later.
- ⚠ **A scripted replace that edits a QUOTED LITERAL needs its escaping checked**,
  not only its match count — one broke the build here; `make ci` caught it.
- ⚠ **Counting headers beats reading them back.** An edit to a 112 KB handoff left
  two `32.` headers, one running off mid-sentence; `grep -c '^32\. '` caught it.

### Added 2026-09-15 — ranks 28 and 31, and three rounds that found the previous round's prose

- 🔴 **A GUARD THAT DERIVES ITS EXPECTATION FROM THE THING IT CONSTRAINS IS BLIND TO THAT THING
  CHANGING.** Measured on #628: every geometry assertion derived from `maxServerLineRunes`, so
  at cap 5000 both stayed GREEN while the surfaces occupied 63 and 64 display rows — the
  forgery essentially restored. The only red was a FIXTURE-SIZE control whose message pointed at
  the fixture, not the cap. One absolute anchor closes it; `swMaxRowsAt80` is that anchor, and
  it pins **ROWS, not the cap** (boundary: 135 green, 136 red).
- 🔴 **PROVE A NEW GUARD REACHABLE, NOT MERELY BREAKABLE — AND WATCH WHICH CHECK KILLS IT.** A
  probe for a new predicate on #619 died at an EARLIER control, which proves nothing about the
  new one. A second payload crafted to get past that control was needed before the predicate
  could be called reachable.
- 🔴 **A FIGURE CARRIED IN PROSE BESIDE THE THING IT COUNTS WILL DRIFT, AND THIS ARC PROVED IT
  FOUR TIMES IN TWO PRs:** "~16" vs 131 call sites; "148 runes" vs 155; "226" vs 225; "2 rows at
  100" vs 3. Nothing asserts on them, so they are unpinned by construction. **Either pin it or
  stop quoting it** — compute and print at run time.
- 🔴 **A ROW COUNT QUOTED BESIDE A SURFACE IT DOES NOT BELONG TO.** "7 rows directly above
  `Generate? [y/N]:`" was the *download* line's figure; that surface costs 8 (overhead 104 vs
  21). Same defect, one file over, one round later — written by the fix for the first one.
- 🔴 **THREE CONSECUTIVE ROUNDS FOUND THE PREVIOUS ROUND'S OWN PROSE**, not its code. Twice a
  false absolute was replaced by a narrower absolute that was also false. The cure that worked
  was to stop asserting the form: delete the figures, state the property, and let the assertion
  print today's numbers.
- 🔴 **AN INVARIANT GUARD THAT CANNOT FAIL ALONE MUST BE LABELLED AS ONE.**
  `TestClassifyGenerateErrorFallThroughPreservesClassification` is green at its own base and is
  caught by nine pre-existing tests; a mutation matrix citing it alone over-attributes. Kept for
  documentation — it is the only test whose NAME states the property — not for detection.
- ⚠ **A SCRIPTED REPLACE THAT EDITS A QUOTED LITERAL NEEDS ITS ESCAPING CHECKED**, not only its
  match count. One put unescaped quotes inside a Go string and broke the build; `make ci` caught
  it pre-commit.
- ⚠ **COUNTING HEADERS BEATS READING THEM BACK.** An edit to a 112 KB handoff left two `32.`
  headers, one running off mid-sentence. `grep -c '^32\. '` caught it; reading did not.
- ⚠ **An issue number is not reserved until the issue exists.** `#623` was written into two
  source comments and taken by another session before the file was created; the real number was
  #624. Assert the count, then verify the number resolves to what you think it does.

### Added 2026-09-15 (second half) — round 0 on the doc itself

- 🔴 **A CLOSING CONDITION CAN BE ARRANGED TO FAIL ON ITS OWN EVIDENCE.** #632's first
  condition said *"run it at the parents of the #566/#612/#624 fixes … if it misses them, the
  condition is not met"* while its predicate keyed on the ABSENCE of a gate. Measured at
  `b4acda5^`: both #624 operands were **already gated** — #624 was an *insufficiency*-of-gate
  defect. So control 3 returned MISS by construction and the condition refuted itself,
  permanently, since #624 is closed and history is fixed. **When a condition names historical
  cases, check each one is of a kind the predicate can SEE.**
- 🔴 **"RETIRE THE MANUAL PROCESS" IS A CLAIM ABOUT COST THAT NEEDS A COUNT.** The same draft
  said the hand enumeration was "retired"; measured, the instrument it proposed spans **1,023**
  non-test non-comment writer sites in `internal/cmd` against the **9** rows of the ledger it
  would replace. Bounded to the four money/verification files it is ~150. **Count the sites
  before writing the verb.**
- 🔴 **A GUARD'S SCALE CAN REINSTATE THE DEFECT IT REPLACES.** None of the three controls checks
  whether a classification is TRUE — a wrong label passes all of them, which is #621's mechanism
  at 150 rows instead of 9. Ask of any new ledger: *what happens if a row is simply wrong?*
- 🔴 **A DUPLICATED RECIPE IS A SECOND THING TO KEEP TRUE, AND THIS ONE WAS NOT KEPT TRUE FOR
  THREE COMMITS.** `## How to verify` existed in both handoff docs; the older copy's `-run`
  filter omitted `IsLengthBounded`, the guard #628 added for #605/#624, so a reader following it
  would have reported the forgery guards green **having never run the newest one**. Deleted in
  favour of a pointer.
- 🔴 **A `-run` FILTER ALTERNATIVE THAT MATCHES NO TEST IS SILENT.** `SoftWrap` matched zero —
  the file is `softwrap_forgery_test.go` but its funcs are `BoundsTheOperand` and
  `IsLengthBounded`. Verify a filter by counting `=== RUN` lines, never by its exit code; the
  corrected filter yields 57.
- 🔴 **SPLITTING A HANDOFF MINTS A SECOND `claim-work` SLUG FOR THE SAME ITEM**, and the lock is
  per-slug: `agent-setup-onboarding-32` and `terminal-line-forgery-1` both resolve to #629 and
  both compare-and-swaps succeed. A forwarding map is the human half; the `gh pr list` sweep is
  the only mechanical one.
- 🔴 **CREATE A HANDOFF DOC THROUGH THE GATE, EVEN THE FIRST TIME.** `handoff_doc.py` rule (m)
  REFUSES a new doc with no `closing-condition:` — but "new" means untracked at HEAD, and this
  file was `Write`-ten and committed by hand first, so the gate took its **advisory** arm
  instead. That is why the condition shipped unparseable. The skill says step 5 is the only step
  that commits; doing it by hand converted a refusal into a warning.
- ⚠ **`… | head; echo "rc=$?"` READS THE PIPE'S STATUS, NOT THE SCRIPT'S** — the exact trap the
  `/handoff` skill documents, hit anyway in this session on
  `clawgate_handoff.sh field`. It printed `rc=0` ("a field is already there") for a real
  `rc=1` ("there is none"). Use `out=$(cmd 2>&1); rc=$?`.
## Next steps (ranked)

🔴 **`claim-work` the rank before acting on it, in EITHER runtime, plus a
`gh pr list --state open` sweep** — the sweep is the only thing that catches an UNCLAIMED
duplicate. ⚠ **Rank 1 of this doc is also reachable as the retired slug
`agent-setup-onboarding-32`** — see the forwarding map in `handoff-agent-setup-onboarding.md`;
two slugs, one item, and both compare-and-swaps succeed.

1. **Settle the threat model, then ratify / narrow / RETIRE the closing condition.** One free
   `--dry-run` probe (the open investigation above), then a decision that is the operator's and
   not a session's. 🔴 **"Retire" is a live outcome, not a courtesy option**: if server messages
   do not echo caller input, the honest closure may be *accept the residual in writing* — which
   removes ranks 2 and 3 rather than scheduling them. Do this BEFORE any fifth point-fix.
   forcing: security — two open issues (#629, #620) whose severity is an unmeasured floor, and
   the measurement that would settle it costs nothing.

2. **civitai/cli#629 — `civitai buzz` is COMPLETELY ungated on the error the bounded warning
   points users at.** `internal/cmd/buzz.go` has **zero** `safeTerm` calls and returns the error
   unwrapped; `cmd/civitai/main.go`'s `errorLine` prints `"Error: " + err.Error()` raw, ESC
   included. Reached via `internal/appapi/appblocks.go:1185`. The **raw-ANSI** class, not the
   length class — and the surface just bounded by #628 names this command in its own text as the
   recovery path. The other half of #624's option (a).
   forcing: security — raw ANSI on the path a bounded surface directs users to.

3. **civitai/cli#620 — seven `internal/genapi` sites interpolate an UNPARSED HTTP body.**
   `blobs.go:73`/`:155`, `generate.go:214`/`:265`/`:288`, `status.go:489`, `workflows.go:221`.
   Fix at the seven interpolations, not at `classifyGenerateError`'s `!errors.As` return — #619
   left that deliberately ungated because gating there closes a SUBSET of one class
   (`blobs.go`'s two reach `resolveImages` instead).
   forcing: security — same raw-ANSI class, on the dominant generate error path.

## Defects (batched)

Fixed as a BATCH, never one rank per finding. Closing one buys room for one rank.

- **#621** — a `safeTermCoveredBy` `why` can still scope a surface OUT in free text with nothing
  checking it. It is **part (i) of the closing condition**, not an aside.
- **#627** — `pkg/civitai`'s `snippet` truncates on a BYTE index; 3 of 4 alignments return
  invalid UTF-8. Measured on the real function with synthetic bodies; reachability on a live
  endpoint NOT verified.
- **#622** — no `claudedocs/decisions/` entry for the arc.
- **#397** — runes are not display cells; every row count this arc quotes doubles for CJK.
  Explicitly OUT of the closing condition's scope.
- **The `## Gotchas` section of `handoff-agent-setup-onboarding.md` is still not split** — filed
  by date, so forgery and agent-setup lessons interleave. Remaining half of rank 29.
- 🔴 **The writer-keyed ledger's labels would be UNCHECKED FREE TEXT** — round 0's F2 residual,
  left standing. All three controls check that classification *happens*, never that it is
  *true*; #621 hardens the OLD gate-keyed ledger, not the new one. Same disease at ~150 rows
  instead of 1,023. If the condition is ratified, this belongs IN it.
## Open investigations — live diagnosis state

### Does the orchestrator's error `message` ever echo CALLER- or UPLOADER-supplied text?

- as-of: 2026-09-15

This is not a bug report — it is the unresolved question that sets the **severity** of every
remaining item in this effort, and it has been open since #612 was filed. It is recorded here
with its live state because the next session should run the probe, not re-derive the question.

- **Symptom + exact repro:** there is no failing command. The question is whether a hostile
  value can reach the CLI's error surfaces from an **uploader or a caller**, or only from a
  **compromised backend**. Everything downstream — whether #629 is urgent or is defence in
  depth — turns on it.
- **Observed (with values):**
  - `classifyGenerateError` (`internal/cmd/generate.go`) matches on
    `has("unknown ecosystem")` as one of five needles, gated `apiErr.Status >= 500`. A server
    message containing the phrase "unknown ecosystem" **only makes sense if the server names
    the ecosystem it did not recognise** — i.e. echoes the value the caller sent.
  - The other four needles are status-agnostic and name server-side conditions
    (`enough funds`, `prompt was flagged`, `generation is disabled`,
    `account has been restricted`) — none implies an echo.
  - `appapi.serverMessage` (`internal/appapi/appblocks.go`) returns the server's `message`
    **verbatim**, and falls back to `strings.TrimSpace(string(raw))` — the whole body — when
    neither `message` nor `error` is present.
  - `genapi.serverMessage` (`internal/genapi/errors.go`) unwraps the tRPC envelope and
    likewise ends `return strings.TrimSpace(string(raw))`.
- **Ruled out:** that it can be settled from this repository — the orchestrator's message
  construction is server-side and no part of it is vendored here. `via: code`
- **Ruled out:** that settling it requires a SUBMIT, which would charge Buzz irreversibly.
  `generate --dry-run` routes through `deps.whatIf` (`internal/cmd/generate.go:1201`) and
  returns at `:1230` before the spend; the flag's own help text reads *"print the cost estimate
  and exit without submitting (spends nothing)"*. `via: code`
- **Ruled out:** that no credential is available to run it — `~/.config/civitai/config.yaml`
  exists on this host. `via: command`
- **Leading hypothesis:** the server **does** echo caller-supplied values, because the
  `unknown ecosystem` needle is hard to explain otherwise. If so the severity on #629 and #620
  is higher than filed, and neither is merely defence in depth.
- **Next probe — one command, spends nothing:**
  ```bash
  civitai generate "a cat" --ecosystem ZZPROBE-DISTINCTIVE-123 --dry-run
  ```
  Read the error text. **If it contains `ZZPROBE-DISTINCTIVE-123`, the server echoes caller
  input and the question is settled in the direction that raises severity.** If it names no
  such value, try the same with a nonexistent `--checkpoint <id>` before concluding — one
  endpoint's behaviour is not the surface's.
  ⚠ This hits the live API with the operator's credentials. It is outward-facing, so it is the
  operator's call to run it, not a session's to run unasked.
