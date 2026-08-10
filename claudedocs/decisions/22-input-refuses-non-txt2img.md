# AGENTS.md item 22 — `--input` refuses every workflow but `txt2img`

Evidence for item 22 of the *Intentional decisions that look wrong* list in
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

> 22. **`--input` refuses every workflow but `txt2img`, and that refusal is NOT the
>     local validation item 13 forbids — it is a CONTENT-AUDIT gate over a
>     CONFIRMED server-side gap.** Item 13 forbids reproducing the server's
>     *judgement* about which graphs are valid; this claims nothing of the kind,
>     and is the same shape as item 19(b) — a flag combination the CLI owns.
>     🔴 The gap is confirmed, not suspected: two shipped ecosystems declare
>     prompt nodes the audit never runs over (civitai/civitai#3667). Keep the
>     claim that size — the gate stops accidents, not adversaries — and lift it
>     when the server closes the coverage, not when the next workflow looks like
>     it would work.

---

22. **`--input` refuses every workflow but `txt2img`, and that refusal is NOT the
    local validation item 13 forbids — it is a CONTENT-AUDIT gate over a
    CONFIRMED server-side gap.** This is the item most likely to be read as an
    unfinished feature, because item 13 says in bold that the CLI validates
    nothing about the graph and `--input` exists precisely to pass one through
    untouched. The reconciliation: item 13 forbids reproducing the server's
    *judgement* about which graphs are valid, and this check makes no such
    claim — it is the same shape as item 19(b), a flag combination the CLI owns,
    asserting nothing about which ecosystems exist or what any of them allows.

    🔴 **THE GAP IS CONFIRMED, NOT SUSPECTED, AND A PRIOR SESSION RECORDED THE
    OPPOSITE.** The server audits at `'prompt' in data && typeof data.prompt ===
    'string'` (`orchestration-new.service.ts:1460`) over a `data` REBUILT FROM
    DECLARED GRAPH NODES ONLY. An earlier revision of the code comment called
    exploitability an open question; a later handoff went further and wrote
    "ruled out — no known bypass", reasoning that graphs lacking a `prompt` node
    *compose* a shared `promptGraph`. True of the image graphs, and false as a
    generalisation — it says nothing about graphs that opt out of the shared node
    names. Verified at `civitai@a7e0bcd668`, two shipped ecosystems do:
    **Hunyuan3D declares no `prompt` node at all**, only `hunyuanPrompt`
    (`hunyuan3d-graph.ts:71`, prefixed because the bare names "collide with the
    standard image Controllers in `GenerationForm.tsx`"), so the audit never runs
    — and the handler then maps it back for the generator,
    `prompt: hunyuanPrompt ? hunyuanPrompt : undefined`
    (`hunyuan3d-graph.handler.ts:58`). **PolyGen's `texturePrompt`**
    (`polygen-graph.ts:162`) is covered by no audit block at all, and on
    `img2model3d` is the only text in the request because that workflow's
    `prompt` node is gated `when: workflow.startsWith('txt')` and is deleted.
    Sweep basis: all 79 declared node keys under
    `src/shared/data-graph/generation`, every node whose input is a bare
    `z.string()`. Note both are MULTI-LINE `.node(\n  'name',` declarations that
    a single-line grep misses. Tracked at civitai/civitai#3667.

    🔴 **KEEP THE CLAIM THIS SIZE — the overclaim and the dismissal are both
    available and both wrong.** The CLI is not what holds the line: the same
    fields are reachable from the first-party form and from any direct tRPC
    caller with a personal API key, so the gate stops accidents, not adversaries.
    That is an argument for fixing the platform, **not** for opening `--input`.
    What the gate buys is that we do not add a second client to a confirmed
    unaudited path while it is open. Equally, no live generation probe has been
    run — this is reachability by code path, not a demonstrated exploit, and
    writing it up as a shipped vulnerability would be the overclaim.

    **Lift it when the server closes the coverage, not when the next workflow
    looks like it would work.** Unblocking the other ~50 ecosystems through
    `--input` is the single biggest capability unlock left in this command, which
    is exactly why the bar is written down rather than left to judgement.
