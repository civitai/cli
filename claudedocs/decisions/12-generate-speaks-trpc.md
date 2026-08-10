# AGENTS.md item 12 — civitai generate calls tRPC, not REST — because there is no REST

Evidence for item 12 of the *Intentional decisions that look wrong* list in
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

12. **`civitai generate` calls tRPC, not REST — because there is no REST
    generation route to call.** Exactly the situation item 5 documents for
    `app metrics`, and it will attract the same "fix". Generation exists only as
    the tRPC procedures `orchestrator.whatIfFromGraph` (the cost estimate, a
    GET) and `orchestrator.generateFromGraph` (the submit, a POST) in
    `civitai/civitai → src/server/routers/orchestrator.router.ts`; there is no
    `/api/v1` equivalent, and the same is true of the reads behind
    `civitai workflows list|get|cancel`. So `internal/genapi` speaks tRPC — path
    constants in `generate.go`, and the same `authedDo` + envelope-unwrap shape
    (`?input={"json":{…}}`, success unwrapping `result.data.json`) that
    `internal/appapi` established for item 5.
    Don't "simplify" any of it into a REST call that does not exist.
    Two things in there are load-bearing and look like oversights. `unwrapTRPC`
    rejects a literal `null` payload as malformed rather than unmarshalling it:
    `null` decodes CLEANLY into a zero `WhatIfResult`, which renders as
    **total 0** — a fabricated free quote, on the one screen a user reads before
    approving a charge. And `CancelWorkflow` deliberately does NOT go through
    that unwrap, because the server's successful cancel reply has no
    `result.data.json` at all (`workflows.ts`) — routing it through the standard
    path would turn **every successful cancel** into "unexpected response". HTTP
    200 is the success signal there.
