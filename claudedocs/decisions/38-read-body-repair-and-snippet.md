# The read path repairs a body only when it must, and strips only what it prints

**Item 38.** Code: `pkg/civitai/read.go` (`getInto`, `decodeBody`, `snippet`,
`EscapeJSONStringControlChars`), `pkg/civitai/hashes.go` (`postInto`),
`internal/cmd/read_help.go` (`readJSONNote`), `cmd/civitai/main.go`
(`errorLine`). Guards: `pkg/civitai/read_repair_test.go`,
`internal/cmd/read_json_note_test.go`,
`cmd/civitai/read_error_stderr_test.go`.

Born split: written straight into `claudedocs/decisions/` — AGENTS.md had 472
bytes of headroom when this was added and the body is three decisions plus their
measurement and mutation matrices. See `bornSplitItems` in
`agents_split_preserved_test.go`.

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

`readJSONNote`'s doc comment carried a MEASURED claim that #526 falsified in
every clause — that a raw C0 byte inside a string exits 1 with empty stdout, so
the repair "is not a behaviour of this group". Re-measured on the same fixture
shape: exit 0, the table renders, `emitJSON` IS entered.

The measurement across the merge, for the record:

| tree | `models search` over a body with a raw CR in `name` |
| --- | --- |
| `5557b6a` (before #526) | exit 1, `unexpected response from /api/v1/models (status 200)`, empty stdout |
| `517fc76` (#526) and after | exit 0, table renders, `--json` prints the document with `\r` |

The paragraph survived a full green suite because nothing asserted on it: the
only test touching the constant asked a different question (`claimsByteIdentity`).
`TestReadJSONNoteDescribesTheRepair` now re-measures the behaviour through the
real command tree AND pins the constant verbatim against that measurement, so a
reword fails on purpose; `TestReadJSONNoteCommentNamesItsGuard` keeps the doc
comment pointing at it.

The same false byte-identity claim was in two result-type doc comments
(`CollectionSearchResult`, `ArticleSearchResult` — "the raw response body") and
in `numeric_username_test.go` ("a raw passthrough of the server's own bytes").
All three now say what is true; the single accurate statement lives on
`EscapeJSONStringControlChars`, which is exported and therefore visible to a
godoc reader of the public package.

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
- **The exit-code contract is unchanged** and asserted: a 404 read still exits
  4 with the filtered message (`TestReadErrorReachesStderrWithoutTerminalControlRunes`).
