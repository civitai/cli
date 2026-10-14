# `civitai feedback` reports platform friction to Civitai, and ships unadvertised

`civitai feedback send` lets a developer — in practice, their coding agent —
tell Civitai that the developer platform got in the way: the CLI, an SDK, the
docs, review, the sandbox, scopes, the MCP servers. `civitai feedback status`
reads back whether Civitai has acknowledged it, fixed it or decided not to.

Implementation: `internal/appapi/platform_feedback.go` (the wire contract, in
one file), `internal/redact` (what is removed before sending),
`internal/credscan/spans.go` (the existing matchers, with locations),
`internal/cmd/feedback.go` (the command). Guards:
`internal/appapi/platform_feedback_test.go`, `internal/redact/redact_test.go`,
`internal/redact/redact_audit_test.go`, `internal/redact/redact_lists_test.go`,
`internal/redact/redact_property_test.go`, `internal/redact/text_test.go`,
`internal/credscan/spans_test.go`, `internal/cmd/feedback_test.go`,
`internal/cmd/feedback_audit_test.go`, `cmd/civitai/feedback_exitcode_test.go`, three golden envelopes under
`internal/cmd/testdata/golden/feedback_*.json`, two rows in
`safeTermCoveredBy`, one in the atomic-write callers ledger.

Seven things here look wrong, or look like something to tidy. Each is
deliberate.

## 1. It is a second verb called "feedback", at the root, on purpose

`civitai app feedback` is an app OWNER reading what the app's USERS wrote.
`civitai feedback` is the developer writing to the PLATFORM. The data flows in
opposite directions, to different readers, about different subjects — so they
are not subcommands of one another, and this one is registered on the root
rather than under `app`.

The cost is two commands one word apart. It is paid in the help text: the first
line of `civitai feedback --help` says it is not `civitai app feedback`, both
subcommands repeat it, and `civitai app feedback --help` carries the
mirror-image clause in its opening paragraph. Those lines are pinned
(`TestFeedbackHelpStatesTheLaneOnItsFirstLine`,
`TestAppFeedbackHelpPointsAtThePlatformLane`). Do not "unify" the two under one
parent: an agent that files a platform bug into an app inbox, or looks for user
feedback in a platform report, gets a wrong answer from either merge.

## 2. Redaction runs before the request is built, and it never says "clean"

The message and the error excerpt go through `internal/redact` BEFORE the
request is assembled, so the wire, the `--dry-run` print and the `--json` echo
are all built from the redacted text.

🔴 **Bytes become text first, and a file is one encoding or it is refused.**
`--error-file` and `--error -` are read as bytes and decoded by
`redact.DecodeText` before any rule runs. The rules are byte-oriented matchers,
and a PowerShell-redirected log (UTF-16LE) is NUL-interleaved ASCII to them —
measured on the first build of this command, such a file went out with the
configured token in it and `redactions: []`. What `DecodeText` reads, exactly:

- **UTF-8**, with or without a byte-order mark — chosen when the input has no
  UTF-16 mark and contains no NUL byte.
- **UTF-16 announced by a byte-order mark**, when every code unit passes the
  unit test below for that byte order.
- **UTF-16 with no byte-order mark** — considered only when the input contains
  a NUL byte — when every code unit passes in exactly one of the two orders.

The unit test (`utf16UnitOK`), for a unit's high and low bytes: it passes when
the high byte is zero and the low byte is not (ASCII and Latin-1 — the
NUL-interleave); or when the high byte is non-zero, the two bytes are not BOTH
printable ASCII, and — if no mark established the order — it is not an ASCII
high byte over a zero low byte. Anything else is refused (exit `2`): an odd byte count, a UTF-32
mark, a failing unit, mark-less input that passes in both orders or neither,
and content that decodes to mostly control characters.

There is no "mostly UTF-16". The first version of this accepted mark-less input
once enough of its bytes fitted the pattern, and PowerShell 5.1's `>>` onto a
UTF-8 log produces a file that did: decoded whole as UTF-16, the UTF-8 head
became CJK-range characters that still encode its bytes, matched no rule, and
was sent with the token check silent. "Both bytes printable ASCII" is what a
UTF-8 region looks like, which is why one such unit refuses the file.

The cost, stated: genuine UTF-16 containing a character whose two bytes are
both printable ASCII (中, U+4E2D, is `2D 4E`; many common CJK characters are in
that range) is refused with or without a mark, and mark-less UTF-16 containing
a character below U+8000 with a zero low byte (一, U+4E00) is refused too. The error says to
save the log as UTF-8.

The message is passed through `redact.CleanText` (NUL removed, invalid UTF-8
replaced), because the server rejects both.

The rules, in the order they run:

| Order | Kind | What is removed | Left in its place |
|---|---|---|---|
| 1 | `credential` | in a URL token (scheme to the next white space): everything between `://` and its last `@` | `<credential>` |
| 1 | `url-query` | in a URL token: everything from its first `?` or `#` to its end | nothing |
| 2 | `credential` | the value of an `Authorization` header: everything after the `:` or `=` to the end of that line | `<credential>` |
| 3 | `token` | each configured credential value, matched exactly (the stored token, the OAuth access and refresh tokens, `$CIVITAI_TOKEN`) | `<token>` |
| 4 | `credential` | a PEM private-key block; an `x-api-key` / `api-key` value that has a digit; a bare `Bearer <value>` that has a digit; what `credscan.Spans` locates (known token formats, the value of a strict `KEY=value` assignment) | `<credential>` |
| 5 | `email` | an e-mail address | `<email>` |
| 6 | `home-path` | this machine's home directory | `~` |
| 7 | `user-path` | the user segment of `/home/<u>`, `/Users/<u>`, `C:\Users\<u>` | `<user>` |

🔴 **The URL rule names no character at which a query or a userinfo "ends", and
it is that rule's second rewrite.** Round 1 moved URL handling ahead of the
placeholder-producing rules (a `<token>` inside a query had stopped the strip
at its `<`) but kept a terminator class per part: a quote ended a query, the
first `@` ended a userinfo, and `/`, `?`, `#` could not be in one. Each was
defeated by that character: `?name='x'&sig=…` kept `&sig=…`; `user:p@ss@host`
kept `ss`; `user:a?b@host` kept `user:a` — every time under a count that said
the part was gone. White space is the only boundary now. One further cut
follows from it: when a `?` or `#` precedes the last `@`, the text cannot say
whether that is a password containing one or a query containing an `@`
(`host/p?to=x@SECRET`), so what follows the `@` is removed too and both a
credential and a url-query redaction are recorded.
`TestPropertyASecretInsideAURLTokenNeverSurvives` generates the surroundings
(3,000 seeded inputs, with surroundings drawn from the whole set of printable
ASCII punctuation) instead of enumerating them; against the round-1 rule it
failed on 1,358 of them.

Consequences for the counts, stated: a token inside a URL's query or userinfo,
or in an `Authorization` header, is counted under the rule that removed it, not
as a `token`; a URL that loses both its userinfo and its tail is two
redactions.

🔴 **The counts describe what is SENT.** Each replacement leaves a marker (a
Unicode noncharacter) in the working text, a replacement that removes text
holding markers re-emits them, and the counts are read off the text finally
returned — `ApplyClipped` counts nothing beyond its cut. The output reports
counts by kind, not the text removed.

🔴 **The credential shapes are `internal/credscan`'s own matchers, not a copy.**
`credscan.Spans` returns where the two detectors `Scan` already runs would
fire, and `TestSpansAgreeWithMatch` pins that the two agree line by line. A
second table would drift; `internal/saferune` records that failure (#393).

🔴 **So redaction inherits credscan's deliberate silences**, and it matches
SHAPES. That is why the zero case reads "Redaction matched nothing … that is
not a statement that no secret is in them", and why `--dry-run` exists. Do not
reword either into a claim that the text is safe (item 28: no claim about a
state the CLI cannot observe).

🔴 **There is no prompt-injection detector, and one must not be added** — here
for the outbound text, or for the note that comes back. It would be a phrase
list; item 28 and decision 40 record how those lose.

`--ref`, `--app` and `--doc` are validated rather than redacted, and a token is
made of exactly the characters a site key allows. So the last gate does not
reason about fields: `feedbackRefuseIfTokenInBody` searches the request for
each credential value the redactor knows — the marshalled bytes, and each
string of the document after JSON decoding, since JSON escapes `<`, `&` and
quotes — and refuses to send if it finds one. It finds a credential that is
present as itself; one that is present transformed is a known miss, below.

The two lists that follow are kept identical, entry for entry, here, in
`internal/redact`'s package doc and in `internal/redact/redact_lists_test.go`,
where each entry is demonstrated by an input
(`TestKnownListsAreTheSameInEveryDoc`).

KNOWN MISSES — sent as typed:

- a bare `Bearer <value>` with no `Authorization` header in front of it, when
  the value has no digit
- an `x-api-key` / `api-key` value that has no digit
- the query or fragment of a URL written without a scheme
  (`cdn.example/f?sig=…`)
- a credential that appears transformed: URL-encoded, base64 of itself, or
  split across lines
- a configured credential shorter than 8 characters (stderr says when one was
  skipped)

KNOWN OVER-REDACTIONS — removed although not a secret. Each is in the safe
direction and none gets a rule of its own:

- everything after `Authorization:` to the end of that line, whatever it is —
  prose included, and anything else that shares the line
- an `api-key` / `apiKey` assignment whose value has a digit, even when the
  value is a variable name
- any URL userinfo, even a bare user name (`ssh://git@host/…` is counted as a
  credential)
- everything between `://` and the last `@` of a URL token — so an `@` in a
  path or query takes the host with it — and, when a `?` or `#` came before
  that `@`, the rest of the token too
- a URL's `#fragment`, and whatever is glued to a URL after its first `?` or
  `#` with no white space: closing quotes, brackets, a trailing full stop, the
  rest of a minified JSON line
- a Windows user segment runs to the next backslash on the line, so
  `copy C:\Users\ingrid .\dist\out` loses `ingrid .`
- `/home/<name>` and `/Users/<name>` wherever they appear, including inside a
  URL path
- the words of a private-key header (`BEGIN … PRIVATE KEY`) in prose

## 3. Almost nothing is read from the project

Auto-attached: the CLI version, OS, architecture, the agent detected from the
ENVIRONMENT, and the version each `@civitai/*` package is DECLARED at in
`./package.json` (at most four, sorted by name). Nothing else.

- **The agent** is `agent-setup`'s `detectAgent` with the marker-file half
  switched off (`Exists` left nil). The marker files say which agents have been
  used in a directory before, not who is running the command, and reading them
  would be reading the project. No environment signal → the field is omitted.
- **The SDK versions** are the ranges written in `package.json`, not what is
  installed. A value that is not a version at all — `file:../sdk`, a git URL —
  is dropped, because that is a path or a host from the developer's machine.
- **The app slug is never inferred.** `block.manifest.json` is not opened;
  `--app` is the only way a slug is sent
  (`TestFeedbackSendDefaultsAndAttachesOnlyWhatItSays`).

A dependency KEY is held to the npm name shape (`@civitai/` then lower-case
letters, digits, `.`, `_`, `-`): package.json is a JSON object, and a key is as
free-form as its value.

An auto-detected value that does not fit its bound is OMITTED rather than
clipped, and is not a reason to refuse the report — with one exception: the
CLI's own version is required by the contract, so it is reduced to the allowed
characters and length instead. A value the user typed that does not
fit is refused (exit `2`). The error excerpt alone is clipped — to the first
500 characters that remain AFTER redaction.

## 4. The server owns dedupe

`duplicate`, `reporters` and `status` are the server's words about a report.
The CLI computes no fingerprint, normalises nothing for matching, and never
decides two reports are the same — it could only compare against what THIS
machine sent, which is the wrong population. When the server says
`duplicate: true` the output says Civitai matched the report to one it already
has and that the ref printed is that report's. The local ledger moves a
repeated ref to the front rather than listing it twice; that is bookkeeping,
not dedupe.

## 5. What comes back is data, including Civitai's own note

`note` is Civitai-authored, which makes it more trustworthy than a site user's
message and changes nothing about how it is rendered: sanitised (`safeTerm`),
capped at 600 runes in both views, printed behind the same `|` gutter as any
other server-supplied message, under a label that says it is information and
not an instruction. `--json` names the field `civitaiNote` and escapes the
runes a terminal would act on (`writeFeedbackJSON`, decision 40 §2). Every
single-line server cell — the ref, the status word, `fixedIn`'s component and
version, the timestamps — goes through one function (`feedbackServerCell`).

The server-issued ref is echoed into a command the output suggests running, so
it is held to a conservative shape (`ValidPlatformFeedbackStatusRef`) before it
is suggested, stored or sent back. One that is not ref-shaped is printed
sanitised and nothing else.

`feedback status` builds its output from the refs that were ASKED FOR, in that
order. A ref the response does not mention is "not found"; a row for a ref
nobody asked about is dropped.

## 6. The exit codes follow the published table, and one 403 is not an auth error

| What the server answers | Meaning | Exit |
|---|---|---|
| `200` | sent — a new report or a duplicate alike | `0` |
| `403` with the token-scope sentence | the credential's scope does not cover the procedure | `3` |
| `403` with the collection-off sentence | Civitai is not collecting platform feedback right now | `1` |
| any other `403` | refused; the CLI reports the server's words and names no cause | `3` |
| `404` on the procedure | this server has no such API (tRPC's answer for an unknown procedure) | `4` |
| `429` | rate limited | `6` |
| `400` | the server's schema rejected what `Validate` accepted | `2` |

🔴 **Collection-off is HTTP 403 and exits `1`, not `3`.** Nothing about the
caller's credential is wrong, and `3` is the code a script answers with
`civitai login`. `platformFeedbackError` therefore leaves that one arm
UNTAGGED. It is recognised by the stable core of the server's sentence, exactly
as the scope refusal is (`isInsufficientScopeMsg`), because tRPC gives both the
same code — and it is deliberately narrow: a 403 that says neither is not
called "collection is off".

🔴 **The 404 and 400 messages name no cause.** A 404 says the procedure is not
there; it is not worded as a server that "predates" the API or will have it
"yet" — this CLI shipped before any server had it, so either would be a claim
the status does not carry. A 400 passed the CLI's own checks, and its message
does not guess that limits changed or that an upgrade would help. One mapper
serves both verbs, so no arm tells a `status` caller to re-run `send`
(`TestErrorsClaimOnlyWhatAStatusCodeCarries`).

`feedback status <ref…>` exits `4` when EVERY ref named was not found — the
same miss `civitai app status <unknown-slug>` reports, on the same code, per
the README's exit-code table. A partly-found list exits `0`, and so does the
no-argument read (the user named no ref).

## 7. The verb is unadvertised until the server side is on

The server endpoint did not exist when this shipped. So the verb is genuinely
unadvertised, as a set of states rather than an intention:

- the root `feedback` command is `Hidden`, so `civitai --help` does not list it;
- the README does not document it — the section and the command-table rows ship
  with the PR that turns the verb on;
- the managed AGENTS.md template is untouched, no other command's error
  suggests it, and `agent-setup`'s output is unchanged.

`civitai feedback --help` itself works, for whoever is told the verb exists, and
`civitai app feedback --help` keeps its one mirror-image clause (§1).

A pointer on any of those surfaces would send every agent that reads it at an
endpoint that answers `404`. `TestPlatformFeedbackVerbIsUnadvertised` asserts
each state — the command's `Hidden` field, the RENDERED root help, the README,
the template — rather than the absence of a spelling in some directory. The PR
that turns the verb on flips them together.

`--error` takes only `-` (stdin). It briefly also accepted the error text
itself, which made `--error ./submit.log` send the path string; any other value
is now a usage error naming `--error -` and `--error-file <path>`.

## Not established

- **Nothing here was run against a live server.** `platformFeedback.create` and
  `platformFeedback.getStatus` did not exist. Every request shape, field name,
  enum, bound and status code was taken from a written contract and pinned as a
  literal; the fakes in the tests are that contract as this CLI understood it.
  A server that differs will be refused by those literals, and the fix is meant
  to be one file (`internal/appapi/platform_feedback.go`).
- **The collection-off sentence is assumed.** The contract gives it as "like
  `Feedback is not being collected here right now.`". The CLI matches
  `not being collected` / `not collecting`, case-insensitively. If the shipped
  sentence contains neither, that state exits `3` with the generic refusal
  instead of `1` — truthful text, wrong code.
- **The scope requirement is assumed** to be the Apps submit scope that a
  default `civitai login` requests. No scope refusal was observed.
- **The server-issued ref's alphabet is assumed** from one example
  (`5123-3f9a01bc`). A ref outside `[A-Za-z0-9._-]{1,80}` would be printed but
  not stored, suggested or accepted by `feedback status`.
- **`report.command`, `report.exitCode` and `report.httpStatus` are in the wire
  type and validated, and no flag sets them.** Kept on purpose: the follow-up
  that attaches them from a failing command is then not a contract change.
- **The server's rejection of NUL and unpaired surrogates is taken from the
  contract**, not observed. `CleanText` and `Validate` remove and refuse them.
- **String bounds are counted in UTF-16 code units**, on the assumption that
  the server's schema is zod (`.max()` is JavaScript's `.length`). If it counts
  code points instead, the CLI is merely stricter.
