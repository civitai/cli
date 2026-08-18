# AGENTS.md item 32 — what a submit may CLAIM about the source it was built from

Evidence for item 32 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements and the residuals, consulted when editing the code they are
about rather than on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## Why this item exists

`civitai app submit` used to package whatever was in the directory and record
**nothing** about where it came from, so deploy-vs-source drift could be
observed but never diagnosed. Five first-party apps were found behind their live
version, and `custom-generators` had never held its live `0.5.2` anywhere in git
history; reconciling them took a session of archaeology (`civitai/cli#411`).

The server half is `civitai/civitai#4061` — two nullable columns,
`source_commit` and `source_dirty`, on `app_block_publish_requests`. The CLI
half is `civitai/cli#471`.

## The two front doors — read this before touching the wire

🔴 **Two routes parse `submitVersionSchema`, and the CLI uses only one of them.**
`/api/blocks/submit-version` is the cookie/moderator-browser route;
`/api/v1/blocks/submit-version` is the bearer-token route, and it is the one
this CLI posts to (`appapi.DefaultSubmitPath`). Wiring provenance into only the
first would leave the real client's claim silently stripped — the exact
inert-feature shape the whole issue exists to close, with a fully green suite.
The generalisation: *"the server accepts a new field" is a question about every
parse site, not about the schema.*

## Why a malformed value must never be sent

The server validates `sourceCommit` against `^[0-9a-f]{40}$` and **hard-400s** a
malformed one — deliberately, because silently dropping it is the inert shape
above. That decision moves the burden to this client: a CLI that sends garbage
turns a working submit into a failed one. Hence `Provenance.sanitised()`, the
single gate before the wire, and the hostile-git-output tests behind it. Send
nothing rather than something bad.

## `source_dirty` is TRI-STATE

`null` = UNKNOWN (a pre-feature row, no git repo, an unborn HEAD, or a client
that sent nothing) · `false` = the client asserted the tree was CLEAN · `true` =
the client asserted it was DIRTY. `null` and `false` are different answers.
Never `?? false`, in the client or anywhere downstream — coercing turns "nobody
looked" into "someone looked and it was clean", which is the opposite of what
this feature is for.

## Residual: measured, and still open at the time of writing

The client half was proven correct against a capture server — correct route,
true HEAD sha, `sourceDirty: false` rather than `null` — while a real production
submit produced a row with `source_commit` **NULL**, because production had not
yet deployed the server half. `app status` showing no provenance is therefore
expected, not broken, until that deploy lands. The check that closes it: submit
a throwaway from a clean repo and confirm a non-null `source_commit` matching
local `HEAD`.

---

32. **Sending or showing the commit an app was built from — the 40-hex gate,
    `sourceDirty`'s null-vs-false, or the submit body's size?** An UNVERIFIED
    client claim: word none of it as verified, never `?? false`, and send
    nothing rather than a value `^[0-9a-f]{40}$` rejects — malformed is a 400
    that fails the submit (#411).
