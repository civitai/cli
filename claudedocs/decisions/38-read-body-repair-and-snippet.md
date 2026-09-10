# The read path repairs a body only when it must, and strips only what it prints

**Item 38.** Code: `pkg/civitai/read.go` (`getInto`, `decodeBody`, `snippet`,
`readError`, `isDeepPagingCap`, `EscapeJSONStringControlChars`),
`pkg/civitai/hashes.go` (`postInto`), `internal/cmd/read_help.go`
(`readJSONNote`), `internal/saferune` (the class the snippet strip uses),
`cmd/civitai/main.go` (`errorLine`). Guards:
`pkg/civitai/read_repair_test.go`, `pkg/civitai/raw_doc_ledger_test.go`,
`internal/cmd/read_json_note_test.go`,
`cmd/civitai/read_error_stderr_test.go`, `saferune_callers_ledger_test.go`.

Born split: written straight into `claudedocs/decisions/` — AGENTS.md had 472
bytes of headroom when this was added and the body is three decisions, the
cross-tree measurement table in §3 and the enumerated residuals. See
`bornSplitItems` in `agents_split_preserved_test.go`. (Round 2 described this
as "measurement and mutation matrices"; there is one measurement table and no
mutation matrix — mutation results live in the PR body and in each guard's own
doc comment.)

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

**The fix is to classify on the WIRE message.** `readError` keeps `wireMsg`
before the `snippet` call; the display still gets the filtered string.
"Narrow" is a claim about what the SERVER sent, so the classifier is asked
about what arrived.

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
  - the deep-paging cap still exits 2 (same test, half (b)).
  Nothing asserts that the filter cannot move a code the CLI does not yet
  classify from text. Today there is no such code — §4 records the grep — but
  that is a measurement of the tree, not a guard on it.
- **`TestRawDocCommentsDisclaimByteIdentity` covers doc comments on struct
  fields, not prose.** The detail getters (`GetModel`, `GetArticle`,
  `GetCollection`, `GetApp`, `GetModelVersion`, `GetModelVersionByHash`) return
  raw bytes as a bare second `[]byte`, with no doc-comment slot the ledger can
  find, and narrative comments anywhere in the package are not read. So the
  byte-identity enumeration is OPEN, not closed — which is the whole lesson of
  round 2's count of three.
- **`TestItem38CommentsCiteTestsThatExist` is scoped to item 38's four files.**
  Measured across all of `internal/cmd`, 31 comment-cited test names do not
  resolve: cross-package citations, hypothetical examples (`TestFoo`), and real
  rot, unseparated. Widening the check to the package is a separate change with
  31 findings to triage; globbing it now would make the guard permanently red,
  which trains people to click through.
- **`TestSaferuneCallersAreLedgered` proves membership and naming, not
  CORRECTNESS.** It cannot tell whether a fourth caller is entitled to the
  unconditional strip — that judgement is `saferune`'s package doc (never what
  the user typed), and no test enforces it.
