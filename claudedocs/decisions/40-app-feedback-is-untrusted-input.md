# `civitai app feedback` prints text any site user wrote, to a reader that is often an agent

`civitai app feedback <slug>` reads the private feedback users send an app's
developer from inside the app, and `set-status` / `flag` act on it. The server
side is tRPC only (`appFeedback.*`, `civitai/civitai →
src/server/routers/app-feedback.router.ts`), so the transport follows item 5.

Implementation: `internal/appapi/feedback.go`, `internal/cmd/app_feedback.go`.
Guards: `internal/cmd/app_feedback_test.go`,
`internal/cmd/agent_setup_feedback_triage_test.go`, the two golden envelopes
under `internal/cmd/testdata/golden/app_feedback_list*.json`, and four rows in
`safeTermCoveredBy`.

Six things here look wrong, or look like something to tidy. Each is deliberate.

## 1. The message is untrusted third-party input, and the defence is structural

Every other server string this CLI prints was written by the platform or by the
person running the command. A feedback message is written by **any signed-in
site user**, and the developer's coding agent is what pulls and triages it. So
the text is never interpreted, and each renderer makes its origin a property of
the output's SHAPE:

- **Human view.** The message goes through `safeTerm`, is hard-wrapped
  (`wrapServerText`, 72 runes), and every line of it is printed behind a `  | `
  gutter that no CLI-authored line carries. The row header is the only
  column-zero line per row. A message therefore cannot forge a header, a second
  row or `set-status`'s success line — with a control byte or with nothing but a
  newline — and a long unbroken token is split by the CLI, inside the gutter,
  instead of being left to the terminal's soft wrap.
- **`--json`.** The text sits in a field NAMED `untrustedMessage`, last in each
  row, inside a versioned envelope (`schemaVersion`) whose top-level `notice`
  states the rule on every read.

`--since` / `--version` are client-side filters (the server has none). Without
`--all` they see one page, and when the server reports more, stderr says the
filter covered only the first page — "nothing matched" over a sample is not
"nothing matches".

🔴 **There is no heuristic that tries to DETECT a prompt injection, and one must
not be added.** It would be a phrase list. Item 28 records phrase lists losing
twice to a paraphrase and once to an appended sentence; an injection detector
loses the same way, and its green would read as "this message is safe". The
labelling is the mechanism.

The header's own server-supplied cells (time, status, version, sha, surface,
and with `--with-reporter` the username — the one a third party chooses) are
single-line (`safeTermSingle`) and each capped at 40 runes, so a hostile cell
cannot turn the header into an unbounded line.

Residuals, stated: the wrap budget and the cell cap count runes, not display
cells (the gap `wrapServerText` already records, civitai/cli#397); a terminal
narrower than 76 columns still soft-wraps message lines; and the header cap
BOUNDS a soft-wrapped header rather than preventing one — a header built from
several maximal cells is longer than 80 columns. None of that changes what a
line of message text is prefixed with at the moment it is emitted.

## 2. `--json` here escapes more than the rest of the CLI's `--json`

`encoding/json` escapes C0 only, leaving DEL, C1 and the invisible/bidi class
raw. For text any user can write, printed by a command an agent runs in a
terminal, `writeFeedbackJSON` escapes every rune `safeTerm` would strip. It is
escaping, not sanitising — decoding returns the identical string — and the
reason is in its doc comment. Do not collapse it into `writeJSON`.

## 3. Reporter identity is absent by default, not null

Operator decision. `--with-reporter` opts in; without it the `reporter` key does
not exist, because "absent" is the only shape a test can assert without also
accepting a leaked-but-empty value.

## 4. The CLI does not say feedback is "disabled", because an owner cannot observe that

Collection is behind a server-side flag, but the flag gates only who may WRITE
feedback (`resolveAppFeedbackTarget` → `isFeedbackAreaEnabled`,
`src/server/services/blocks/app-feedback.service.ts`). None of the four
owner-side procedures consults it. An owner whose users are not offered the form
reads `{items: []}` — byte-identical to an app nobody has written to.

So the empty state prints that it cannot tell the two apart (item 28's rule: no
claim about a state the CLI cannot observe). The states that ARE observable
each get their own message and code:

| What the server answers | Meaning | Exit |
|---|---|---|
| `200`, no rows | empty — cause unknowable | 0 |
| `403` with the token-scope message | the credential's scope does not cover the procedure | 3 |
| any other `403` | not the owner or an accepted editor — 🔴 **or the listing does not exist**: `resolveInboxListingId` returns one error for both on purpose, so the CLI does not distinguish them | 3 |
| `404` on the procedure | a server with no feedback API (tRPC's answer for an unknown procedure) | 4 |
| `409` on a write | the write's precondition matched no row | 1 |

## 5. A `409` is not worded as only "the status changed"

Both writes are an `updateMany` whose `WHERE` is the owner-visibility predicate
plus the precondition, and the server throws the same `CONFLICT` whenever that
matches zero rows. So it is also what an id that belongs to another app, a row a
moderator has hidden, and a reporter who has since been banned look like — and,
for `flag`, a row that is already flagged. The message leads with "status
changed since you read it" and names the others. `409` carries no
classification sentinel, so it is the generic exit `1`; that is pinned, not a
fallthrough.

`set-status` always sends `expectedOwnerStatus` — the status the row was READ
as, with `new` travelling as JSON `null` (the field is `.nullable()`, not
`.optional()`, so the key must be present). It reads the row first unless
`--expect` supplies the value. When the row is already in the target status —
as read, OR as `--expect` says — nothing is sent: the compare-and-set would
match, re-stamp the status and re-run the reporter notification for no change.
The `--expect` case sends no request at all (round-1 audit finding: it used to
POST).

## 6. `--count` is a flag here, not a line in `civitai app status`

So that a failed count can never fail `app status`; the reasoning is on
`runAppFeedbackCount`.

## Not established

- **No SUCCESSFUL read or write was observed against the live server.** Every
  request shape was read out of the server's zod schemas and pinned as a
  literal. Two read-only calls were made against production on 2026-10-10 with
  an OAuth `civitai login` token — `civitai app feedback --count` and a
  one-page list of an owned app — and both were refused `403` with the
  token-scope message, exit `3`. That establishes three things and no more: the
  procedures exist there (not `404`), the slug resolved through the shared
  listing resolution, and the real scope refusal is the string the CLI
  classifies on. The success envelope — in particular whether `nextCursor` is
  absent or `null` on the last page — was not observed; the client accepts both.
- **A scoped `civitai login` token is refused until the server annotates the
  owner-side procedures with the Apps submit scope**, as measured above. Until
  then only a full-scope personal API key works. The CLI's scope-refusal message
  is worded to be true in both states.
- An app whose slug is literally `set-status` or `flag` cannot be listed: those
  words name the subcommands. Stated in `--help`; no such app was looked for.
