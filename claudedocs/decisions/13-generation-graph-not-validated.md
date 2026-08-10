# AGENTS.md item 13 — the CLI deliberately validates NOTHING about the generation graph

Evidence for item 13 of the *Intentional decisions that look wrong* list in
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

## The stub thesis this item's trigger replaced

Waves 1–3 of the evidence split (#290, #305, #310) left a multi-line STUB in
AGENTS.md here. That stub was prose written for the split — a compression of the
body below, not a slice of it — so the trigger index preserves it rather than
deleting it:

> 13. **The CLI deliberately validates NOTHING about the generation graph, and
>     that is the feature.** `internal/genapi/graph.go` models a handful of fields
>     and `--input` passes a caller's graph through byte-for-byte. It does not
>     vendor, and must never vendor, the ecosystem keys, the ~51 per-engine
>     graphs, the sampler enum, the buckets, the per-ecosystem defaults, the tier
>     limits or any cost table: the server re-derives every one of them from state
>     the CLI cannot see, so a vendored copy buys no correctness and starts
>     **refusing valid new inputs**. Live lookups are not mirrors, which is why
>     `ResolveModelVersion` is allowed.

---

13. **The CLI deliberately validates NOTHING about the generation graph, and
    that is the feature.** `internal/genapi/graph.go` models a handful of fields
    and `--input` passes a caller's graph through byte-for-byte. It does not
    vendor, and must never vendor: the ecosystem keys, the ~51 per-engine
    graphs, the sampler enum, the resolution/aspect-ratio buckets, the
    per-ecosystem defaults, the tier limits, or any cost table. The server
    re-derives every one of them at submit time from state the CLI cannot see —
    a caller's usable (non-disabled, non-memberOnly) ecosystem set, for one, so
    even the *default* model differs between a free and a member account — so a
    vendored copy buys no correctness, goes stale, and starts **refusing valid
    new inputs**, which is worse than the gap it closes. This is the same
    anti-mirror judgement as item 8, with money on it.
    Two consequences that look like missing features:
    (a) `KnownGraphKeys()` (`graph.go`) is derived by REFLECTION over the struct
    tags, never hand-listed, and it answers *"does the CLI model this key?"* —
    **not** *"does the server accept it?"*. `unknownKeyWarning`
    (`internal/cmd/generate_input.go`) is worded to say exactly that and must
    stay worded that way; a warning phrased as "invalid key" would be a
    validation claim this CLI has no authority to make.
    (b) The one bounded exception is `serverQuantityClamp` in
    `internal/cmd/generate.go` — a **warn-only** constant, currently 10, that
    **fails soft**: it warns and sends anyway. It exists because the server
    silently CLAMPS an out-of-range quantity (measured: 10000 → 10, −5 → 1) with
    no error, so a `--quantity 40` typo charges for 10 and gives the user no
    signal at all. Because it only warns, a stale number produces a needless
    note, never a blocked valid request. Do not promote it to validation, and do
    not add siblings for the other clamped fields without the same fail-soft
    shape.
    **Live lookups are not mirrors**, which is why one is allowed:
    `ResolveModelVersion` (`internal/genapi/versions.go`) resolves
    `--checkpoint` / `--lora` ids against the existing public
    `GET /api/v1/model-versions/{id}` before submitting. It is a round-trip, so
    it carries no drift cost, and it buys two things nothing else can. A
    **nonexistent** checkpoint id is otherwise accepted with HTTP 200, the
    ecosystem default silently substituted, and **billed** (measured on one
    ecosystem: the correct id priced 160, while a nonexistent id, a
    foreign-ecosystem id, and no model at all ALL priced 60 — the server's own
    comment says this correction is visible on-site and invisible through a
    non-browser path). And `Graph.Resources` entries **require** `model:{type}`,
    which is not derivable from a version id — a bare id and `{id}` alone are
    both 400s, while a *wrong* type is silently accepted. The type must come
    from the lookup, never a guess.
