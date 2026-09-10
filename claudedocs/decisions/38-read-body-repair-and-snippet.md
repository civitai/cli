# The read path repairs a body only when it must, and strips only what it prints

**Item 38.** Code: `pkg/civitai/read.go` (`getInto`, `decodeBody`, `snippet`,
`readError`, `isDeepPagingCap`, `classifyWindow`,
`EscapeJSONStringControlChars`), `pkg/civitai/hashes.go` (`postInto`),
`internal/cmd/read_help.go` (`readJSONNote`), `internal/saferune/saferune.go`
(the class the snippet strip uses), `cmd/civitai/main.go` (`errorLine`).
Guards: `pkg/civitai/read_repair_test.go`, `pkg/civitai/raw_doc_ledger_test.go`,
`internal/cmd/read_json_note_test.go`, `internal/cmd/read_help_test.go`,
`internal/cmd/read_r2_test.go`, `cmd/civitai/read_error_stderr_test.go`,
`saferune_callers_ledger_test.go`.
This list and `item38CommentedFiles` — the set whose comments are checked for
dangling test citations — are pinned equal by
`TestItem38FileLedgerMatchesTheDecisionHeader`. They disagreed in both
directions until round 4: the ledger held four files, two of which this header
did not name, and none of the `pkg/civitai` or `cmd/civitai` ones.

Born split: written straight into `claudedocs/decisions/` — AGENTS.md had 472
bytes of headroom when this was added, and the body is five decisions, three
tables and the enumerated residuals. See `bornSplitItems` in
`agents_split_preserved_test.go`.

🔴 **That sentence has been wrong twice, in opposite directions, so the geometry
is now pinned.** Round 2 wrote "measurement and mutation matrices"; there has
never been a mutation matrix here, and that half was right to correct. Round 3
then wrote "one measurement table" — in the same commit that added the second and
the third. The three are: a clause-by-clause table and a cross-tree measurement
table in §3, and a cross-tree exit-code measurement table in §4.
`TestItem38EvidenceTableGeometry` asserts where they are and how many; the
adjectives are not asserted and remain a reader's judgement. Mutation results
live in the PR body and in each guard's own doc comment, never here.

This is the follow-up to the adversarial audit of civitai/cli#526 (squash
`517fc76`). #526's own change was correct and its regression test was watched
red at base; what this item records is the set of OUR claims that its correct
change falsified, and the two design choices taken while fixing them.

## 1. The repair is a RETRY on the decode, never a pre-pass

#526 shipped, in `getInto`:

```go
if !json.Valid(raw) {
    if fixed := EscapeJSONStringControlChars(raw); json.Valid(fixed) {
        raw = fixed
    }
}
if out != nil {
    if err := json.Unmarshal(raw, out); err != nil {
        return nil, fmt.Errorf("unexpected response from %s (status %d): %s", path, status, snippet(raw))
    }
}
```

It is now unmarshal-first: decode, and only on failure sanitize and decode
again, keeping the repaired bytes only if the second decode succeeded. Three
reasons, in descending order of how load-bearing they are.

**(a) The snippet.** `snippet(raw)` is the only thing a user is shown when a
`200` will not decode, and the pre-pass reassigned `raw` to the repaired bytes
*before* it ran. So a server that sent one `0x0D` was reported as having sent a
backslash and an `r`, and README's Troubleshooting row — "the text after the
colon is the server's own body, truncated" — was false. Under the retry shape
the reassignment happens only on the branch that succeeded, so a failure
message still quotes what actually arrived.
Pinned by `TestGetIntoErrorSnippetQuotesTheWireBytesNotTheRepairedOnes`.

**(b) The unrepairable case.** A body invalid for a reason the sanitizer cannot
fix must reach the same error with the ORIGINAL bytes. Nothing exercised that:
a mutant assigning the sanitized bytes unconditionally SURVIVED a full green
`pkg/civitai` + `internal/cmd` run. Pinned by
`TestGetIntoUnrepairableBodyReportsTheOriginalBytes`, which is labelled in
source as an **invariant guard** — it passes at `517fc76` too, so it is not
regression coverage; its value is the mutant.

**(c) Cost.** The pre-pass scanned every successful body in full, on top of the
`json.Unmarshal` that walks the same bytes — on the happy path of an importable
SDK, not a one-shot CLI. The load-bearing half of this is STRUCTURAL: the retry
shape does no extra work at all on a body that decodes. The size of what was
removed is one host's measurement and is not portable — a synthetic 1.50 MB /
2185-item models page, go1.25, `-benchtime 300x -count=5`: `json.Valid`
4.9–6.4 ms against `json.Unmarshal` 12.9–26.4 ms. The scratch benchmark is not
in the tree; re-measure rather than quoting this.

`decodeBody` exists so `getInto` and `postInto` cannot answer this differently.
`postInto`'s doc comment enumerates what it mirrors from `getInto`, and the
repair was, for one release, a fourth axis on which it silently did not — a
divergence no test could see, because `HashMatch` is two ints and a 64-char hex
with no free-text field. Sharing the body makes the enumeration true by
construction. `TestDecodeBodyIsSharedByGetIntoAndPostInto` is a bidirectional
ledger of its callers; `TestPostIntoRepairsControlBytesLikeGetInto` is the
behavioural half.

## 2. `snippet` strips; `errorLine` does not — decision (a), and why not (b)

`cmd/civitai/main.go` prints `Error: <err>` straight to stderr. `internal/cmd`'s
`safeTerm` sits on the HUMAN RENDERERS, not on the error path, and `snippet`
embeds up to 500 bytes of a server body verbatim. So a `200` or a `404` body
carrying `ESC[1A ESC[2K` could erase the CLI's own preceding output from inside
an error message. That hole PRE-DATES #526; fixing (a) above would have widened
it back to the 2xx path as well, because restoring the wire bytes to the snippet
restores the wire's escapes too.

**Chosen: filter inside `snippet`, using `internal/saferune`.** Every argument
`snippet` is ever given is the server's own bytes — the undecodable 2xx body,
the API's `{"error":…}`/`{"message":…}` text, a zod issue out of a 400, a
retry-exhaustion body, a `FlexString` value. None of it is anything the user
typed, which is what makes an unconditional strip correct at this one function.
`snippet` was already a terminal-concern helper: its existing job is bounding
length "so a huge response can't flood the terminal".

**Rejected: filter at the print site (`errorLine`).** It would apply the class
to EVERY error, including the many that echo what the user typed — a slug, a
path, a prompt. That is exactly the regression civitai/cli#393 exists to
prevent, and `TestSafeTermIsNeverAppliedToUserTypedInput` exists because it
already happened once.

**Rejected: re-derive the class inside `pkg/civitai`.** That is the OTHER half
of #393 — two tables that disagree. `saferune`'s package doc is emphatic that
the class is a PROPERTY with one enumerated exception, and that a summary of it
is a copy of it.

🔴 **THE COST, STATED RATHER THAN HIDDEN.** This makes `pkg/civitai` — the
public read/download SDK — the first thing under `pkg/` to import `internal/`,
and it means an SDK consumer that is not a terminal reads an error string with
those runes already removed. Two things bound that: the strip touches only the
human-readable tail of an error message, and the byte path an SDK consumer would
actually parse (`Raw`) is returned unstripped.

Pinned by `TestReadErrorSnippetStripsTerminalControlRunes` (unit) and
`TestReadErrorReachesStderrWithoutTerminalControlRunes` (the SEAM: a real HTTP
reply, the real SDK, the real `errorLine`, plus the published exit code for a
404). The seam test exists because each side was already tested and neither side
owned the join.

## 3. What `--json` and `Raw` may CLAIM

`readJSONNote`'s doc comment carried a MEASURED claim that #526 falsified —
that a raw C0 byte inside a string exits 1 with empty stdout, so the repair "is
not a behaviour of this group". Re-measured on the same fixture shape: exit 0,
the table renders, `emitJSON` IS entered.

**What #526 falsified is the CONCLUSION, not every premise**, and round 2 wrote
this up as "falsified in every clause" — an over-claim of exactly the kind this
item exists to close. Clause by clause, measured at HEAD:

| clause | at HEAD |
| --- | --- |
| "every read command reaches `emitJSON` only AFTER `getInto` has unmarshalled the same bytes into a typed struct" | **still true** — every `emitJSON` call site in the read group is handed `res.Raw` or the `raw` return of a `getInto`-backed getter |
| "`encoding/json` rejects a raw C0 byte inside a string literal there first" | **still true** — measured: `invalid character '\r' in string literal`. That rejection is now what TRIGGERS the repair |
| "a body carrying a literal CR … exits 1 … and an EMPTY stdout — `emitJSON` is never entered" | **false** |
| "it is not a behaviour of this group, so it is not described as one" | **false**, and it is the conclusion the two true premises stopped supporting |

The measurement across the merge, for the record:

| tree | `models search` over a body with a raw CR in `name` |
| --- | --- |
| `5557b6a` (before #526) | exit 1, `unexpected response from /api/v1/models (status 200)`, empty stdout |
| `517fc76` (#526) and after | exit 0, table renders, `--json` prints the document with `\r` |

The paragraph survived a full green suite because nothing asserted on it: the
only test touching the constant was `TestReadJSONNoteDoesNotClaimByteIdentity`
(`internal/cmd/read_help_test.go`, via its `claimsByteIdentity` helper), which
asks a different question — whether the note promises byte identity, not whether
it describes the repair.
`TestReadJSONNoteDescribesTheRepair` now re-measures the behaviour through the
real command tree AND pins the constant verbatim against that measurement, so a
reword fails on purpose; `TestReadJSONNoteCommentNamesItsGuard` keeps the doc
comment pointing at it.

🔴 **AND THE COMMENT SAYING SO NAMED A TEST THAT HAS NEVER EXISTED.** Round 2
wrote the sentence above into `read_json_note_test.go` citing
`Test`+`ReadJSONNoteWordingMatchesEmitJSON` — an identifier that appears nowhere
in this repository and in no commit of its history. The comment written to stop
comments going stale was itself unverifiable, and
`TestReadJSONNoteCommentNamesItsGuard` could not see it: that test pins only the
FORWARD direction (the doc comment contains the guard's name), so a wrong name
in the same file is structurally invisible to it. The reverse direction is now
`TestItem38CommentsCiteTestsThatExist` — every `Test…` identifier in a comment
in item 38's files must resolve to a test function declared somewhere in the
module. Resolution is repo-wide on purpose: these comments legitimately cite
guards in other packages.

The same false byte-identity claim was spread much wider than one commit's
sweep found.

🔴 **ROUND 2 CLOSED THIS AS A COUNT OF THREE AND THE COUNT WAS WRONG.** It read:
"the same false claim was in two result-type doc comments … and in
`numeric_username_test.go`. All three now say what is true." At that point at
least seven more sites were live, including the two that matter most —
`ModelSearchResult` and `ImageSearchResult`, the exact types civitai/cli#525
bites, since the raw byte arrives in a model `name` or an image `prompt`. And
`getInto`'s own doc comment said "see the note on Raw in the result types",
pointing at eight referents of which two carried the note.

**A count is the wrong instrument for a sweep**, which is why the fix is not a
better count. `TestRawDocCommentsDisclaimByteIdentity` is a bidirectional
ledger of every type with a `Raw []byte` field, and it requires the disclaimer
on each: a ninth result type cannot be added without it, and a ledgered type
cannot silently lose the field. Corrected in the same pass, outside what that
ledger can see: `ArticleDetail.Content`'s "`--json` still returns the raw body
untouched" (its point is that `--json` does not do the HTML rendering
`--content` does — that survives; the byte word does not) and
`flexstring_test.go`'s "Raw … must still be the server's own bytes" / "`--json`
is a raw passthrough", which is verbatim the comment #526's follow-up corrected
in `numeric_username_test.go` and missed here.

The single accurate statement lives on `EscapeJSONStringControlChars`, which is
exported and therefore visible to a godoc reader of the public package.

Deliberately NOT rewritten: "preserved for `--json` via the raw body" on
`ModelDetail`, `ArticleDetail`, `ImageItem` and their neighbours. That is a
claim about FIELD preservation — a key the struct does not model survives into
`--json` — which is true, and which the repair does not touch.

## 4. The strip is a DISPLAY filter and must not become a CLASSIFIER

Round 3, and the only behavioural finding in it. §2 put `internal/saferune`
inside `snippet`. `readError` then handed `snippet`'s output to
`isDeepPagingCap`, whose own comment says the match is "deliberately narrow so a
real rate-limit 429 is never misclassified as a usage error".

Removing runes can only WIDEN a substring match, and it widened it in exactly
that direction. Same 429 body on both trees — one U+00AD SOFT HYPHEN, which is
`Cf` and therefore in the class, inside the word "many":

| tree | `{"message":"… too ma<U+00AD>ny pages …"}` on a 429 |
| --- | --- |
| `517fc76` | `ErrRateLimited` — **exit 6** |
| `f936cfc` (round 2) | `ErrBadRequest` — **exit 2** |
| HEAD | `ErrRateLimited` — **exit 6** |

Contrived for Civitai's own backend, which does not emit soft hyphens; not
contrived for the proxy, CDN and captive-portal 429s `readError` also handles.
AGENTS.md items 7 and 24 make the exit code a published contract.

**The fix is to classify on the WIRE message.** `readError` takes `classifyMsg`
before the `snippet` call; the display still gets the filtered string.
"Narrow" is a claim about what the SERVER sent, so the classifier is asked
about what arrived.

🔴 **"ANY TRANSFORMATION CAN ONLY WIDEN IT" WAS FALSE, AND `isDeepPagingCap`'S
OWN COMMENT SAID IT.** `snippet` applies TWO transformations that pull in
opposite directions, and this PR knew both: the **strip** can only widen (the
table above), the **500-byte bound** can only narrow (the paragraph below, and
half (c) of the guard). Three other sites in this PR stated it correctly; the
one that generalised was the comment a maintainer reads before feeding the
classifier a processed string. Corrected in round 4 to name both directions.

🔴 **THAT FIX MOVES A SECOND EXIT CODE, DELIBERATELY, AND IN THE OPPOSITE
DIRECTION — SAID HERE RATHER THAN DISCOVERED LATER.** `snippet` also truncates
at 500 bytes, which is the same hazard in a second shape: before this change a
genuine cap message whose phrase fell past the display budget was invisible to
the classifier and exited 6, so a scripter's backoff loop span forever on a
structurally doomed request. Classifying pre-`snippet` removes the truncation
from the match as well as the strip, so such a message now exits 2. That is the
intended reading — a display budget is not a statement about what the server
said — and it is pinned as half (c) of the same test rather than left as prose.
Reachability is not hypothetical (a >500-byte error body is ordinary) but no
such 429 has been observed from the API; the case is constructed.

🔴 **THE SAME EDIT ALSO UN-BOUNDED THE MATCH AGAINST AN ARBITRARY BODY, WHICH
ROUND 3 DID NOT SAY.** The paragraph above frames the change as re-widening the
match for a legitimate cap message. It is also a change in the SIZE of what the
matcher scans, in the other direction: `readError`'s `msg` is the server's
`error`/`message` field only when the body is JSON and carries one. When a 429
body is not JSON, or is JSON without either key, `msg` is the **entire body**,
bounded only by `maxResponseBody` (64 MiB). Before round 3 the matcher inherited
`snippet`'s 500 bytes; after it, `strings.ToLower` (a full copy) plus three
`Contains` ran over whatever arrived.

Round 4 bounds it with `maxClassifyMessage` (8 KiB) via `classifyWindow` — a
budget deliberately separate from the display budget, since conflating the two
is the defect this section exists for. **No false positive was demonstrated;
this is containment, not a fix for an observed defect**, and the number is
generous rather than derived: the API's own cap message is ~60 bytes, no
measurement in this repo pins a largest legitimate 429 message, and none of the
proxy/CDN/captive-portal 429s `readError` also handles has been sampled for
length. 8 KiB is ~130x that observed message and ~16x the display bound. The
visible consequence is stated rather than hidden: a 429 whose cap phrase sits
past 8 KiB is not reclassified and keeps exit 6. Pinned by
`TestDeepPagingCapClassificationIsBounded`, which is red on an unbounded
`classifyMsg` and carries a positive control — the same fixture shape with the
phrase inside the window must still reclassify, or half (a) would pass against a
classifier that never matches.

This is the ONLY site in `pkg/civitai` where the text of an error picks a
classification; `readError`'s other branches classify from the status alone
(`statusKind`), and `snippet`'s other callers are display-only. Verified by
grep over every `kind =` assignment and every `snippet(` call site.

Pinned by `TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne` (the
sentinel, plus a positive control that the cap is still reclassified, plus that
the display is still filtered) and
`TestThrottle429KeepsExitSixThroughTheMessageFilter` (the published number, at
the `cmd/civitai` seam).

## 5. Two prose statements of the caller set, both stale at once

`pkg/civitai`'s `snippet` became the THIRD non-test caller of
`internal/saferune`. AGENTS.md's Layout entry still said `safeTerm` and
`hasPrintableContent` "both" call it, and `saferune`'s package doc still opened
"BECAUSE TWO PACKAGES ASK THE SAME QUESTION". Both survived a green suite, a
lint run and a round-2 audit of the branch that falsified them.

The third caller asks a THIRD question — "what may an ERROR STRING carry",
because `cmd/civitai/main.go` prints `Error: <err>` to stderr with no renderer
in front of it — so the package doc states the question, not just a bigger
number. `TestSaferuneCallersAreLedgered` is the bidirectional ledger: it fails
when the importer set grows or shrinks, requires both texts to name every
ledgered call site, and derives the count WORD from the ledger's own length so
the numeral cannot drift from the membership.

## Residuals, enumerated

- **The README's prose is not machine-checked against the behaviour.**
  `readme_troubleshooting_test.go` is a drift detector over symptom STRINGS, and
  no symptom string changed, which is why it did not catch the false
  "server's own body" clause. The BEHAVIOUR the row describes is pinned (§1a,
  §2); that the row still describes it is not. Widening the detector to compare
  a row's explanation against runtime behaviour is a much larger guard than the
  one that exists, and was not attempted.
- **`snippet` still truncates by BYTE at 500**, so it can cut a multi-byte rune
  in half and emit invalid UTF-8. Pre-existing, unchanged here, and not a
  terminal-control hazard — the strip runs before the bound.
- **`saferune` keeps `\n` and `\t` deliberately**, so a server body can still
  put a newline in an error line. `internal/cmd`'s `indentContinuation` is the
  answer to that on the renderers it covers; the error path has no equivalent.
- 🔴 **The exit-code contract was NOT unchanged**, and round 2 recorded here
  that it was — "unchanged and asserted", on the evidence of one 404 test. That
  is a description wider than its body twice over. A 404 is classified from the
  STATUS alone, so `TestReadErrorReachesStderrWithoutTerminalControlRunes`
  cannot see a message filter moving a code no matter what the filter does; and
  the one code the filter COULD move, it did (§4). What is asserted now, stated
  at the width actually measured:
  - a 404 read exits 4 with the filtered message
    (`TestReadErrorReachesStderrWithoutTerminalControlRunes`);
  - a throttle 429 whose message carries a `Default_Ignorable` rune exits 6, at
    the seam (`TestThrottle429KeepsExitSixThroughTheMessageFilter`) and as a
    sentinel (`TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne`);
  - the deep-paging cap still exits 2 (same test, half (b));
  - a cap message whose phrase falls past `snippet`'s 500-byte DISPLAY bound
    exits 2 (same test, half (c)) — the deliberate move in the opposite
    direction;
  - a cap message whose phrase falls past `maxClassifyMessage` (8 KiB) exits 6
    (`TestDeepPagingCapClassificationIsBounded`), with the same fixture shape
    inside the window still exiting 2 as its positive control.
  Nothing asserts that the filter cannot move a code the CLI does not yet
  classify from text. Today there is no such code — §4 records the grep — but
  that is a measurement of the tree, not a guard on it.
- **The README does not document the 429 → exit 2 reclassification at all**, so
  neither the round-3 change nor round 4's bound has a published counterpart to
  keep in sync. Its Troubleshooting row reads "`rate limited (429)` | Throttled;
  exit `6`", and §4 above is the only statement anywhere that some 429s exit 2.
  This gap PRE-DATES this PR — `isDeepPagingCap` and its reclassification are
  live at `517fc76` — and is recorded rather than fixed here, because writing the
  row is a change to the published exit-code contract's wording and belongs with
  whoever owns AGENTS.md item 7's table, not with a follow-up audit. Closing
  condition: the Troubleshooting row and the exit-code table both name the
  deep-paging case, and `readme_troubleshooting_test.go` still passes.
- **`TestRawDocCommentsDisclaimByteIdentity` covers doc comments on struct
  fields, not prose.** The detail getters (`GetModel`, `GetArticle`,
  `GetCollection`, `GetApp`, `GetModelVersion`, `GetModelVersionByHash`) return
  raw bytes as a bare second `[]byte`, with no doc-comment slot the ledger can
  find, and narrative comments anywhere in the package are not read. So the
  byte-identity enumeration is OPEN, not closed — which is the whole lesson of
  round 2's count of three.
- 🔴 **`TestItem38CommentsCiteTestsThatExist` is scoped to item 38's OWN files
  — twelve of them plus this document since round 4 — and the number that
  justified the scope limit was measured with an instrument this PR does not
  ship.** Round 3 wrote "measured across all of `internal/cmd`, 31 comment-cited
  test names do not resolve", and listed *cross-package citations* as part of the
  31 — but `repoTestFuncNames` is repo-WIDE precisely so cross-package citations
  DO resolve. That 31 came from a package-local resolver. Re-measured on this
  tree with the shipped instruments (`repoTestFuncNames` + `testIdentRe`) over
  all 471 `.go` files in the module, at this commit: **2096 citations, 23
  unresolved sites, 20 distinct names.** (A measurement of this tree, not an
  invariant: the citation total moves with any comment edit.)

  All 20 were triaged. **Sixteen names at nineteen sites are correct prose** in
  five shapes a regex cannot separate from rot: a name **wrapped across a comment
  line-break, or elided with an ellipsis**; a **glob** naming a family; a
  **subtest path** (`Parent_Sub`), which no `func` declares; a **fixture
  identifier quoted from the test's own source**; and a **deliberate citation of
  a test that was deleted, renamed or replaced**, which is this repo's own
  convention for recording what a test used to assert. Globbing the check today
  reports 23 sites of which 19 are not defects — a permanently-red gate.
  Widening it needs a suppression convention for those five shapes first, which
  is a separate change. The per-shape file:line index is in
  `TestItem38CommentsCiteTestsThatExist`'s doc comment; neither that comment nor
  this document may quote the identifiers, because both are now scanned and would
  flag themselves.
- 🔴 **Four genuine rot findings came out of that triage, and are NOT fixed
  here.** Each is a doc comment, or a "pinned by" pointer, naming a test nothing
  declares — the same defect §3 of this document exists for, four more instances
  of it, in files this PR did not write. Site, then what is actually declared
  there:

  - `pkg/civitai/download_ssrf_test.go:147` → the function under it is
    `Test`+`DownloadFileRedirectTargetSchemeChecked`.
  - `internal/cmd/app_pull_not_approved_test.go:372` → the function under it is
    `Test`+`AppPullForbiddenIsNotDisambiguated`.
  - `internal/cmd/update_check_test.go:393` → the function under it is
    `Test`+`VersionCommandEnvOptOut`.
  - `internal/appapi/appblocks.go:1261` → the guard it means is
    `Test`+`SubmissionSelectorPrecedenceAgreesWithTheParse`.

  (The real names are written split across a `+` for the same reason as the dead
  identifier in §3: this document is scanned, and it must not resolve names for
  the check by accident.) They are named rather than fixed because a name-only
  correction leaves a comment that reads truer and may still be wrong — the first
  row is the proof: its body describes a redirect-to-**loopback** case while the
  function under it builds a redirect-to-**plain-http** case, so the sentence
  needs rewriting from the function, not renaming. Doing that for four guards on
  evidence this PR has not gathered is how a follow-up audit introduces its own
  false claims. Closing condition: each comment is rewritten from what its
  function does, and the repo-wide unresolved-name count drops from 20 to 16.
- **`TestSaferuneCallersAreLedgered` proves membership and naming, not
  CORRECTNESS.** It cannot tell whether a fourth caller is entitled to the
  unconditional strip — that judgement is `saferune`'s package doc (never what
  the user typed), and no test enforces it.
- 🔴 **And until round 4 it did not prove NAMING in the direction its own
  comment advertised.** The comment on the ledger's `question` field said
  "asserting the IDENTIFIER rather than a phrase … the function can be renamed
  and the texts must move with it". `question` was a plain `string` used as a
  substring needle; nothing bound it to a declaration. Measured: renaming
  `safeTerm` across `internal/cmd` at `31d5ba3` left `go test ./...` **fully
  green** while AGENTS.md's Layout entry and `saferune`'s package doc both named
  a function that no longer existed. `checkQuestionsResolve` now resolves every
  ledgered `question` to a top-level `FuncDecl` in its own package before any
  prose is searched, and all three rows were mutation-tested by an actual rename
  (`safeTerm`, `hasPrintableContent`, `snippet`), each failing with the ledger's
  own RENAMED message. It reads top-level, receiver-less funcs only: a question
  moved onto a method is a change to the class's shape and must widen the
  resolver deliberately.
