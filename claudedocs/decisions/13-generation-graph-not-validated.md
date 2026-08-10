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

## 🔴 (a)'s CLAIM ABOUT `unknownKeyWarning` WAS FALSE WHEN WRITTEN — civitai/cli#343

**Read this before wording anything about an unmodelled key.** The body below is
byte-pinned against the commit it was moved from, so it cannot be edited in
place; this header is where the correction is recorded. The RULE in (a) stands
unchanged and is now actually implemented.

(a) says `unknownKeyWarning` "is worded to say exactly that and must stay worded
that way", where *that* is *"does the CLI model this key?"* rather than *"does
the server accept it?"*. It was not. The shipped warning ended:

> But the server SILENTLY IGNORES keys it does not declare: an unrecognised key
> returns HTTP 200, prices the same, and simply has no effect, so a typo here
> costs Buzz and produces a job that ran without it.

That is a claim about the server's node registry — the very thing this item
forbids vendoring — and it was **wrong**. Measured on a credentialed run: a
graph carrying `"priority": "high"` drew that warning and then priced at **28**
with a `fixed → priority 20` component three lines below it, against **8** for
`normal` and **8** for `low`; the job also cleared in ~40s rather than the
60–90 minutes the low-priority queue was quoting. The key was honoured, it more
than tripled the price, and the CLI's own cost breakdown disproved its own
warning on the same screen.

**The generalisation is the lesson, not the sentence.** The false claim came
from ONE measurement (`foobar:123` returned 200, priced identically and
vanished) written up as a rule covering every undeclared key. That is this
item's own failure mode in prose form: a vendored assertion about the server's
graph nodes, phrased as a warning instead of a table, and therefore invisible to
every guard aimed at tables. **The item's negative examples named "invalid key";
the direction that actually shipped was the opposite one, "the server ignores
it".** Both are verdicts on the server. Neither is available to this CLI.

The warning now says only: the key is not modelled here, it is sent exactly as
written, and what it does — including what it costs — is the server's answer;
`--dry-run` prices the graph with the key included, so that is where a price
effect would show. It is pinned by the golden
`internal/cmd/testdata/golden/generate_input_unknown_key_warning.txt`, because
this surface being unpinned is why a false money claim survived every green
suite.

## The sibling defect, and the REFUSAL THAT WAS DRAFTED AND WITHDRAWN

civitai/cli#342 is the same defect pointing the other way: on the same run a
graph carrying `resources:[{modelVersionId:128713,type:"Checkpoint"}]` **plus**
`--fail-on-substitution` was charged 28 Buzz and ran model version 2442439, with
no warning, no substitution report and no refusal. So the CLI warned about the
key the server honoured and stayed silent about the one it dropped.

**Detecting that `resources` was dropped is exactly what this item forbids** —
it needs a per-workflow table of which keys the server honours — and it was not
attempted, then or now.

🔴 **The first fix was a REFUSAL of the `--input` + `--fail-on-substitution`
combination, and it was withdrawn before merge. Do not reinstate it.** The
premise was that the flag "structurally cannot fire" on a raw graph. It is false.
`substitutionRefusal` reads `quote.ModelSubstitutions` — the record returned by
the **estimate** — not `o.checkpoint`, so on `--input` the flag is a **working
pre-spend guard**: it refuses before the submit, and nothing is charged.
Refusing the combination would have deleted a guard that works in order to close
a gap somewhere else, on the CLI's only irreversible-spend path.

The defect was never "the flag is broken". It is that its coverage is **partial
and undocumented** — and the fix for undocumented is documentation.
`inputSubstitutionCoverageNote` is that documentation: an unconditional warning
whenever the two flags appear together, pinned by the golden
`generate_input_fail_on_substitution_coverage_note.txt`, and behaviourally
backed by `TestGenerateInput_FailOnSubstitutionStillFiresOnARawGraph`, which
proves the "LIVE" claim rather than asserting it.

🔴 **The note is UNCONDITIONAL on the two flags and never reads the graph, and
that is what makes it wordable.** A version that inspected the file — "does it
name a model at the top level, or under `resources`?" — would be the per-key
registry this item forbids. Firing always makes a claim about the FLAG's
coverage; firing selectively would make a claim about the FILE's contents.

🔴 **The tempting wrong sentence, and why it is this item's own error.** It is
natural to write *"covers top-level model substitutions only; nested `resources`
are not reported"*, because item 21(a) says `whatIfFromGraph` carries
**TOP-LEVEL** substitutions. But item 21(a) is about **where in the REPLY the
record is carried** — the reply's own top level, versus nested under
`metadata` — a fact about this CLI's transport. It says nothing about **which
model reference in the REQUEST graph** the server substitutes and reports on.
Reading the first as the second manufactures a per-key rule about the server's
graph registry out of one paid observation, which is precisely the mistake the
`unknownKeyWarning` retraction above documents. `generate_input_test.go` carries
a retraction check for those phrasings.

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
