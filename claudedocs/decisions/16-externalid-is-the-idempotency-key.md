# AGENTS.md item 16 — externalId is minted unconditionally on every submit, and must never be

Evidence for item 16 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements, the mutation matrices, the retractions and the enumerated
residuals, consulted when editing the code they are about rather than on every
session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

---

16. **`externalId` is minted unconditionally on every submit, and must never be
    sent on a whatIf.** `generateInput.ExternalID` deliberately has **no**
    `omitempty`, and `GenerateFromGraph` mints a UUIDv4 when the caller passes
    none. This looks like a field that should be optional. It is the idempotency
    key, and without it a single user action can be charged up to **three
    times**: the platform's submit wrapper retries 3× on a 5xx *and* on a network
    error / no response, reusing the same body, adds no idempotency key of its
    own, and has no server-side minting fallback. The orchestrator dedupes on
    `(userId, externalId)`. The web client is safe only because it always mints
    one.
    It is also what makes the CLI's own 401-refresh replay in
    `genapi/client.go` safe to point at a mutation — the key is minted *before*
    the body is marshalled, so a replay re-sends byte-identical bytes and gets
    the pre-existing workflow back. **Do not add a mutation to that package that
    omits it.**
    🔴 **Never send it on `whatIfFromGraph`.** A matching key returns the
    pre-existing workflow, so a quote would BURN the key the subsequent submit
    needs — which is why the server's own whatIf body omits it. And because a
    duplicate key returns **HTTP 200 with the pre-existing workflow** rather than
    a 409, re-attachment has to be inferred locally: that is why the key is
    written to a local record *before* the POST goes out
    (`internal/cmd/generate_state.go`) and why `--external-id` exists at all. It
    overrides; it does not enable.
    A `--input` file supplying its own `externalId` is REFUSED, not honoured —
    see the envelope whitelist below.
