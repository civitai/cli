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

- **closing-condition:** `check` — a ledger keyed on the WRITER, not the gate, fails the
  suite on an ungated server-origin operand; proven by a negative control, a NON-ZERO
  positive control, and a retrospective run at the parents of the #566/#612/#624 fixes
  that finds what the humans found. Detail and rationale directly below. ⚠ UNRATIFIED.

🔴 **THE FIELD ABOVE IS THE MACHINE-READABLE ONE, AND AN EARLIER DRAFT HAD ONLY THE
HEADING BELOW.** `handoff_doc.py` parses `closing-condition: <kind> — <…>` inside
`## Goal`; it does not parse a `### closing-condition` heading, so the first version of
this doc — whose PR headline was *"give it the closing condition it never had"* —
declared **none** to every reader that matters, and the write gate reported it as
GRANDFATHERED. That is this effort's own recurring defect (something that READS as
coverage while providing none) committed in the document that names it. Keep both: the
field is what is parsed, the section is what is read.

### 🔴 closing-condition — `check`

**THE HAND ENUMERATION IS RETIRED, BECAUSE AN INSTRUMENT FINDS WHAT IT FOUND.**

This is the condition because it is the one thing that would have stopped the effort
regenerating. Every defect this arc has closed was found by a **human** enumerating
operands: `#566`'s five, `#612`'s three, `#624`'s two. Its own title said so in
July; `#612`'s body said the same thing two months and nine PRs later. Every
instrument the repo owns keys on **the gate** — `scanSafeTermCallSites`,
`safeTermCoveredBy`, `TestSafeTermErrCallersAreLedgered`,
`TestSanitizerComposersAreLedgered` all enumerate functions that ALREADY call
`safeTerm` — so all of them are structurally incapable of finding an operand that
has no gate. That is the whole disease.

So the done-state is a ledger keyed on **the WRITER**, not the gate:

> Every `fmt.Fprint*` / `fmt.Errorf` / `fmt.Sprintf` site reaching a human-facing
> surface in `internal/cmd` carries a classification for each interpolated operand —
> **CLI-owned**, **user-typed**, or **server-origin** — and a server-origin operand
> with no gate FAILS the suite. A new writer call site cannot be added without
> classifying it.

That makes the absence **recordable**, which is exactly the property `#612` and
`#621` both say is missing today.

**Checked by — all three, because any one alone is walkable:**

1. **NEGATIVE CONTROL.** Plant an ungated server-origin operand in a *new* writer
   call; the check goes red **with its own message**. A check that cannot go red is
   testing nothing.
2. **POSITIVE CONTROL.** The check reports a **non-zero** count of operands actually
   classified on `main`, and the number is printed. A reassuring zero is
   indistinguishable from a check wired to nothing — report the pair, never the zero.
3. **THE RETROSPECTIVE.** Run it against the parents of the commits that fixed
   `#566`, `#612` and `#624`, and show it finds what the humans found. This is the
   one that distinguishes "an instrument exists" from "an instrument that would have
   worked". If it misses them, the condition is not met.

**Deliberately OUT of scope, so the condition is not read wider than it is:**
`#397` (runes are not display cells — nothing here owns a character-width table, so
every row count this arc quotes doubles for CJK) and the fact that a **bounded** tail
can still begin at column zero, which needs the terminal width the CLI cannot obtain.
Those are named residuals, not failures of this condition.

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

Every one verified by **CONTENT** with a working positive control, never by ancestry: a squash
merge makes the branch head a non-ancestor forever, so `merge-base --is-ancestor` returns false
after every one of these and means nothing.

🔴 **IN FLIGHT — `civitai/cli#632`, branch `docs/split-forgery-initiative`, commit `722cf29`.
OPEN, UNMERGED, and NO audit round has run on it.** It is the PR that created this doc: it
splits the effort out of `handoff-agent-setup-onboarding.md` (rank 29's "split by initiative")
and writes the closing condition in `## Goal`. `make ci` green, 21 packages.

🔴 **THE CLOSING CONDITION IS UNRATIFIED, AND THAT IS THE BLOCKING QUESTION FOR THIS EFFORT.**
It was written by the session that closed ranks 28 and 31, not by the operator. The immediately
preceding lesson on this arc is that a fork resolved without an operator author of record is
itself an audit finding — so this one is flagged rather than presented as settled. **Ratify it,
replace it, or reject it before starting a fifth point-fix**, because every rank below is
otherwise worked under a done-state nobody agreed to.

- **Claim:** `agent-setup-onboarding-29` is HELD at the time of writing (taken for the split).
  Release it when #632 lands. Ranks 23/28/30/31/32 released and verified absent.
- **Not established:** whether an operator agrees the writer-keyed ledger is the right
  done-state; whether it is buildable at acceptable cost. No prototype exists.

### Honest limits

<!-- CARRIED FORWARD deliberately: `## State now` is a REPLACE section, so this subsection
     is deleted by any update that omits it. The write gate printed no durable-drop warning
     for it, and that silence is not evidence — it was caught by reading the `-` lines. -->

- 🔴 **EVERY CLOSURE SO FAR HAS PRODUCED ITS OWN SUCCESSOR: `#604` → `#612` → `#624` →
  `#629`.** Four rewrites of the same "not wholly gated" sentence. That chain is the
  strongest argument for the closing condition above, and the reason a fifth point-fix
  should not start before someone decides whether that condition is right.
- 🔴 **The threat model is OPEN**, recorded on `#612`. Nobody has established whether the
  server's `message` ever carries uploader-controlled text. The classifier matching on
  `has("unknown ecosystem")` implies the server echoes requested values, which would make
  this uploader→victim with **no server compromise**. The cheap probe is a **`whatIf`/quote**
  call's raw error body — **never a submit, which charges**. Severity on those issues is a
  **floor, not a measurement**.
- 🔴 **The audit base rate is FIFTEEN FOR FIFTEEN.** Every round that ran found something.
  Ranks 28 and 31 each ran three rounds and **not one came back clean**; both ladders ended
  on the **attribution gate** (two consecutive rounds whose fixes changed zero payload
  lines), never on convergence.
- 🔴 **Three consecutive rounds found the PREVIOUS round's own prose** — not code, but
  sentences a fix wrote to explain itself. Twice a false absolute was replaced by a
  narrower absolute that was also false.
- ⚠ **This doc's own closing condition shipped UNPARSEABLE** in its first commit — written
  as a `###` heading, which `handoff_doc.py` does not read, so the doc declared NONE while
  its PR headline said it added one. Fixed in `02f4795`; kept here because it is the
  effort's own defect class (reads as coverage, provides none) committed in the document
  that names it.

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
go test ./internal/cmd -count=1 -run 'ForgeALine|IsLengthBounded|SoftWrap|GeometryIsNotServerChosen|RowCountIsNotServerChosen|StaysMultiLine|PreservesClassification|Ledgered'
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
## Next steps (ranked)

🔴 **Re-verify every item against live state before trusting it** — and `claim-work` the rank
before acting on it, in EITHER runtime, plus a `gh pr list --state open` sweep, which is the
only thing that catches an UNCLAIMED duplicate.

1. **civitai/cli#629 — `civitai buzz` is COMPLETELY ungated on the error the bounded warning
   points users at.** `internal/cmd/buzz.go` has **zero** `safeTerm` calls and returns the error
   unwrapped; `cmd/civitai/main.go`'s `errorLine` prints `"Error: " + err.Error()` raw, ESC
   included. Reached via `internal/appapi/appblocks.go:1185`, which interpolates the entire raw
   body on a non-envelope 200. This is the **raw-ANSI** class, not the length class — and the
   surface that was *just* bounded names this command in its own text as the recovery path.
   It is the other half of `#624`'s option (a), which asked to cap the operand **and** `:1185`;
   only the first was done. Closing condition is on the issue.
   ⚠ **Decide the effort's own closing condition first** — this is the fifth successor in the
   chain #604 → #612 → #624 → #629, and starting it before that decision is the pattern the
   whole arc documents.
   forcing: security — raw ANSI on the path a bounded surface directs users to.

2. **civitai/cli#620 — seven `internal/genapi` sites interpolate an UNPARSED HTTP body.**
   `blobs.go:73`/`:155`, `generate.go:214`/`:265`/`:288`, `status.go:489`, `workflows.go:221`.
   A non-envelope 200 puts raw ANSI on stderr. `classifyGenerateError`'s `!errors.As` early
   return is where some of them surface, and #619 deliberately left it ungated because gating
   there closes a SUBSET of one class — `blobs.go`'s two reach `resolveImages` instead. Fix at
   the seven interpolations, not at the one return.
   forcing: security — same raw-ANSI class, on the dominant generate error path.

## Defects (batched)

Fixed as a BATCH, never one rank per finding. Closing one buys room for one rank.

- **#621** — a `safeTermCoveredBy` `why` can still scope a surface OUT in free text, with nothing
  checking it beyond non-empty. **This is the mechanism that concealed `#612` F2 for a full PR
  cycle**, and it is the nearest existing thing to this doc's closing condition — a structural
  check on a RELATIONSHIP rather than a grep for a phrase.
- **#627** — `pkg/civitai`'s `snippet` truncates on a BYTE index; 3 of 4 alignments return
  invalid UTF-8. Measured on the real function with synthetic bodies; reachability on a live
  endpoint is NOT verified.
- **#622** — this arc has no `claudedocs/decisions/` entry, so its rationale is inlined across
  ~6,600 lines of `internal/cmd`. `AGENTS.md`'s own policy says the body belongs in a decisions
  file with a one-line trigger.
- **#397** — runes are not display cells; a double-width (CJK) rune takes two columns, so every
  row count this arc quotes doubles. Explicitly OUT of the closing condition's scope.
- **The `## Gotchas` section of `handoff-agent-setup-onboarding.md` is still not split** — filed
  by date, so forgery and agent-setup lessons interleave. The forgery ones are restated here;
  the originals are deliberately left there. Remaining half of rank 29.
