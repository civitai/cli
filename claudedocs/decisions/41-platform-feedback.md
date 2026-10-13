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
`internal/credscan/spans_test.go`, `internal/cmd/feedback_test.go`,
`cmd/civitai/feedback_exitcode_test.go`, three golden envelopes under
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
request is assembled, so there is no un-redacted copy in anything that reaches
the wire, the `--dry-run` print or the `--json` echo. The rules, in order:

| Kind | What is removed | Left in its place |
|---|---|---|
| `token` | each configured credential value, matched exactly (the stored token, the OAuth access and refresh tokens, `$CIVITAI_TOKEN`) | `<token>` |
| `credential` | a PEM private-key block; a `Bearer <value>` header value; everything `credscan.Spans` locates (known token formats, a URL with an embedded password, the value of a strict `KEY=value` assignment) | `<credential>` |
| `email` | an e-mail address | `<email>` |
| `url-query` | a URL's `?query` (the fragment is kept) | nothing |
| `home-path` | this machine's home directory | `~` |
| `user-path` | the user segment of `/home/<u>`, `/Users/<u>`, `C:\Users\<u>` | `<user>` |

The output reports counts by kind and never the text removed.

🔴 **The credential shapes are `internal/credscan`'s own matchers, not a copy.**
`credscan.Spans` returns where the two detectors `Scan` already runs would
fire, and `TestSpansAgreeWithMatch` pins that the two agree line by line. A
second table would drift; `internal/saferune` records that failure (#393).

🔴 **So redaction inherits credscan's deliberate silences**, and it matches
SHAPES: a placeholder-looking or low-entropy value, a Google-API-key shape, a
headerless key body, a secret in a format nobody listed — all are sent as
typed. That is why the zero case reads "Redaction matched nothing … that is not
a statement that no secret is in them", and why `--dry-run` exists. The reader
is the last check. Do not reword either line into a claim that the text is
safe (item 28: no claim about a state the CLI cannot observe).

🔴 **There is no prompt-injection detector, and one must not be added** — here
for the outbound text, or for the note that comes back. It would be a phrase
list; item 28 and decision 40 record how those lose.

Two fields are validated rather than redacted (`--ref`, `--app`) plus `--doc`,
and a token is made of exactly the characters a site key allows. So the last
gate does not reason about fields: `feedbackRefuseIfTokenInBody` searches the
MARSHALLED body for every credential value the redactor knows and refuses to
send if one is there.

Residuals, stated: the exact-token rule skips a credential shorter than 8
characters (it is a substring replace, and a two-character "token" would shred
ordinary words) and says so on stderr; a token that appears URL-encoded or
otherwise transformed is not matched; `/home/<u>` is rewritten wherever it
appears, including inside a URL path; a bare prose mention of a private-key
header loses those words to credscan's pattern.

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

An auto-detected value that does not fit its bound is OMITTED, never clipped
and never a reason to refuse the report. A value the user typed that does not
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
| `404` on the procedure | a server without the API (tRPC's answer for an unknown procedure) | `4` |
| `429` | rate limited | `6` |
| `400` | the server's schema rejected what `Validate` accepted — the contract moved | `2` |

🔴 **Collection-off is HTTP 403 and exits `1`, not `3`.** Nothing about the
caller's credential is wrong, and `3` is the code a script answers with
`civitai login`. `platformFeedbackError` therefore leaves that one arm
UNTAGGED. It is recognised by the stable core of the server's sentence, exactly
as the scope refusal is (`isInsufficientScopeMsg`), because tRPC gives both the
same code — and it is deliberately narrow: a 403 that says neither is not
called "collection is off".

`feedback status <ref…>` exits `4` when EVERY ref named was not found — the
same miss `civitai app status <unknown-slug>` reports, on the same code, per
the README's exit-code table. A partly-found list exits `0`, and so does the
no-argument read (the user named no ref).

## 7. The verb is unadvertised until the server side is on

The server endpoint did not exist when this shipped. So nothing outside the
command's own help and README section points at it: the managed AGENTS.md
template is untouched, no other command's error suggests it, and
`agent-setup`'s output is unchanged. A pointer would send every agent that
reads it at an endpoint that answers `404`.

`TestPlatformFeedbackVerbIsUnadvertised` makes that a set assertion over
`internal/cmd`'s non-test sources and its templates. The PR that turns the verb
on widens that set on purpose; until then a new mention is red.

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
  type and validated, and no flag sets them.** The decided flag set has none;
  they are there so adding one later is not a contract change.
- **String bounds are counted in UTF-16 code units**, on the assumption that
  the server's schema is zod (`.max()` is JavaScript's `.length`). If it counts
  code points instead, the CLI is merely stricter.
