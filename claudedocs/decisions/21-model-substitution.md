# AGENTS.md item 21 — a reported model substitution is a WARNING

Evidence for item 21 of the *Intentional decisions that look wrong* list in
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

> 21. **A reported model substitution is a WARNING by default, not a failure — and
>     that is a deliberate product decision, not laziness.** The server accepts a
>     checkpoint version id that is not valid for a `modelLocked` ecosystem,
>     substitutes the ecosystem default, runs the job and bills for what ran
>     (civitai/civitai#3665), reporting each swap as
>     `modelSubstitutions: [{requested, applied, reason}]`. Warn-and-continue
>     keeps working automation working; `--fail-on-substitution` is the opt-in for
>     callers who would rather fail than get a different model.

---

21. **A reported model substitution is a WARNING by default, not a failure — and
    that is a deliberate product decision, not laziness.** The server accepts a
    checkpoint version id that is not valid for a `modelLocked` ecosystem,
    substitutes the ecosystem default, runs the job and bills for what ran
    (civitai/civitai#3665). It now records each swap and returns it as
    `modelSubstitutions: [{requested, applied, reason}]`, `reason` ∈
    `{wrong-workflow, unrecognized, gated}`. `internal/genapi/substitution.go`
    models it; `internal/cmd/generate_substitution.go` renders it.

    (a) **Three carriers, and they differ.** `whatIfFromGraph` → TOP-LEVEL only
    (nothing is persisted on that path, so the reply is the only copy).
    `generateFromGraph` → top-level AND `metadata`. Any later read
    (`getWorkflow`) → `metadata` only. `SubmitResult.Substitutions()` prefers the
    top-level copy and falls back to metadata; it must never CONCATENATE them, or
    one swap is reported as two.

    (b) 🔴 **ABSENCE IS AMBIGUOUS. The key is OMITTED when nothing was
    substituted, so "no record" means EITHER "no substitution" OR "a server
    older than the field" — the same bytes.** Nothing may render that as "no
    substitutions": a reassuring negative would be wrong against every server
    predating civitai/civitai#3692. The renderer prints *nothing at all* when the
    record is empty, and there is a test asserting the CLI never claims the
    negative. This is also why the field is a plain slice and not a pointer —
    unlike item 9's `AppAnalytics.Views` there is no third state to preserve, and
    no Go type can resolve an ambiguity that is inherent to the protocol.

    (c) **Why warn-and-continue is the default.** The substitution is a
    deliberate graceful degradation: a script pinned to a version that was later
    retired keeps producing images instead of breaking. Making it a hard CLI
    failure by default would override that server-side decision and break working
    automation on a CLI upgrade. So the default is the fail-soft shape of item
    13(b) — warn loudly, proceed — and the protection for a human is that the
    warning prints BEFORE the confirmation prompt, where they can still say no.
    `--fail-on-substitution` is the opt-in for callers who would rather fail than
    get a different model; it mirrors `--max-cost` (an opt-in pre-flight refusal
    evaluated on the ESTIMATE, so it spends nothing) and is tagged
    `ErrModelSubstituted`.

    (d) 🔴 **It is checked on the estimate and deliberately does NOT re-fire
    after the submit.** By then the money is gone, and aborting would strand
    outputs the caller has paid for and still needs to collect. A post-submit
    substitution is always REPORTED and always present in `--json`; it just does
    not change the exit code.

    (e) **The report goes to STDERR in every mode, `--json` included** — the raw
    passthrough already carries the record on stdout, so warning there would
    corrupt the stream for exactly the callers automating a spend. And the
    server's `reason` token is printed RAW per item 8, with the CLI's advice on a
    separate line; the advice supplements the token, it never replaces it. An
    UNRECOGNISED reason still warns with the ids intact — a fourth reason from a
    newer server must not make the CLI go quiet, which is the original defect
    reintroduced via a `switch`.

    (f) 🔴 **The PHASE a call site passes is part of the contract, and asserting
    it by keyword does NOT work.** `reportModelSubstitutions` takes a
    `substitutionPhase`, and the difference between them is whether the money has
    already moved — so a wrong argument makes `--dry-run` announce "HAS BEEN
    CHARGED", or makes `workflows get` tell someone whose workflow was billed that
    "Nothing has been submitted or charged yet". A test asserting
    `Contains(out, "charged")` cannot see any of that: it was satisfied by an
    unrelated line from the *quote* renderer, so every call-site mutation survived
    a green suite. Assert the phase STRUCTURALLY — derive the expected lead from
    the constant via `substitutionLead(phase)` and require the other phases' leads
    to be ABSENT (`assertPhase` in generate_substitution_test.go), and keep
    `TestSubstitutionLead_AllPhasesNonEmptyAndPairwiseDistinct`, because an empty
    or duplicated lead silently disarms every one of those assertions
    (`Contains(x, "")` is always true).

    (g) 🔴 **A mutation matrix scoped to the renderer proves nothing about the
    call sites.** The first round of this feature reported 17/17 mutants killed
    and was still missing all of the above: every mutant targeted
    `reportModelSubstitutions`/`substitutionRefusal` internals, so the ARGUMENTS
    passed to them, the third phase, and the refusal message's operand ORDER were
    structurally outside the battery. A second, independently-built battery found
    11 survivors — including a swap of `Applied`/`Requested` that made the
    money-refusal line state the substitution backwards. When you mutation-test a
    reporter, mutate the CALL SITES too, not just the function.

    (h) 🔴 **The post-spend report runs AFTER the handle reaches the user, and
    that ordering is the thing to preserve — not the line's position in the
    file.** By the submit reply the job is CHARGED and the workflow id is the
    user's only way back to what they paid for; the report explains a charge that
    has already happened, so it is advisory. It used to be emitted BEFORE the id,
    which was harmless only because it does no I/O — and the obvious next
    improvement (resolving the substituted ids to NAMES through
    `ResolveModelVersion`, which the estimate path already does for
    `--checkpoint`) would have inherited `getWithRetry`'s **4 attempts**
    (`readMaxAttempts`, `pkg/civitai/retry.go`) against a **30s**-timeout client
    (`defaultTimeout`, `pkg/civitai/api.go`) plus backoff — minutes per id, two
    ids per record, holding the handle for a cosmetic gain. `emitSubmitHandle`
    exists so all four branches (--no-wait, --json, a reply with no workflow id,
    and the waiting path) emit the handle from ONE place before any advisory, and
    `generate_handle_order_test.go` pins it by BLOCKING the advisory's own write
    and reading what the user already has — no wall-clock sleep is involved. If
    you do add name enrichment, bound it and keep the ids as the fallback: the
    report must still appear with ids rather than go quiet, which is the silence
    the whole feature exists to end.
    Two traps that cost a mutation round: extracting the handle emission moved
    three control-flow decisions with it, and `||`→`&&` on either branch test
    turns a `--no-wait` run into a POLLING run — detected at first only as a
    **nil-seam panic**, which aborts the binary and stops the guard that would
    have named the defect from ever running. So the `--no-wait` cases wire a
    working poll seam they never use (`wireIdlePoll`), and the poll-count
    assertion is deliberately NOT fatal-gated on the returned error, which every
    one of those mutants also changes.

    (i) 🔴 **The approval summary annotates the superseded checkpoint; it never
    swaps it.** The summary echoes the checkpoint as a NAME rather than an
    integer (item 13) so the user approves something recognisable — but with a
    substitution in play that made the LAST model line before `Generate? [y/N]`
    the name the server had already said it would discard, with the warning
    scrolled above it. `substitutionCheckpointNote` appends
    `[SUPERSEDED — the server will run version N instead; …]` to the requested
    label at BOTH pre-spend surfaces (the confirm prompt and the `--dry-run`
    quote), computed from one call site so the two cannot disagree. Printing the
    applied id in the requested one's place would be a second silence, not a
    fix: the user asked for their version and must still see it was overridden.
    The match is on `requested`, so a substitution of anything the line does not
    name leaves it alone — a false mark on a correct line teaches authors to
    ignore the real one. The tense comes from `substitutionVerb(phase)`, the same
    constant the lead uses, and is asserted structurally per (f): a call site
    passing `substitutionAfterSubmit` would tell someone deciding whether to
    spend that the substitute already ran.
