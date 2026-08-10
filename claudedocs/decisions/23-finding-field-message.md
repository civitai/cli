# AGENTS.md item 23 — a validation finding is a Finding{Field, Message}

Evidence for item 23 of the *Intentional decisions that look wrong* list in
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

> 23. **A validation finding is a `Finding{Field, Message}`, and the Field is
>     carried from the CHECK — never re-derived at the printer.** `validate.Result`
>     holds `[]Finding`, not `[]string`. This looks like ceremony around what used
>     to be a string; it is the fix for issue #225, where 7 of 11 findings on a
>     six-ways-broken manifest came back `field: null` — precisely the semantic
>     ones, the checks a local pre-check exists for. 🔴 DO NOT close a future gap
>     with more prefix heuristics at the printer: the field has to come from the
>     producer, and `newFinding` is the only constructor.

---

23. **A validation finding is a `Finding{Field, Message}`, and the Field is
    carried from the CHECK — never re-derived at the printer.** `validate.Result`
    holds `[]Finding`, not `[]string`, and every check function in
    `internal/validate` returns `[]Finding`. This looks like ceremony around what
    used to be a string; it is the fix for issue #225, and the string version is
    what made that bug unfixable in place.
    - **What broke.** `--json` promised `errors`/`warnings` each with
      `field`/`message`. The field was recovered in `internal/cmd/app_validate.go`
      by string-parsing a schema-style `"<path>: <reason>"` prefix. Schema errors
      parse; the ported semantic checks of item 1 emit PROSE, so they parsed to
      nothing. Measured on a six-ways-broken manifest at `e9a44c4`: **7 of 11
      findings came back `field: null`, and they were precisely the semantic ones**
      — the checks a local pre-check exists for. Grouping by field in CI, the one
      thing `--json` is for, did not work for the findings that matter most.
    - 🔴 **DO NOT "FIX" A FUTURE GAP HERE WITH MORE PREFIX HEURISTICS.** A parser
      at the printer regenerates the bug at every check added afterwards: the new
      check emits prose, the parser finds no path, the null is back, and nothing
      fails. The field has to come from the producer. `newFinding(field, message)`
      is the only constructor, and `Finding{…}` composite literals outside
      `finding.go` are rejected by `TestFindingsAreConstructedWithAField`.
    - **ONE NOTATION: DOTTED.** `blockId`, `iframe.sandbox`, `scopes[1]`,
      `targets[0].slotId`. Three notations used to coexist — `(root)` and JSON
      Pointer `/blockId` in `--json`, dotted `iframe.sandbox` in the text output.
      Dotted won because it is what every other surface already speaks (the
      semantic messages' own prose, this file, the README, the schema
      descriptions), so unifying on JSON Pointer would have meant rewriting every
      author-facing message to `/iframe/sandbox` to buy escaping rigour a manifest
      whose keys are all identifiers does not need. **This changed the wire
      values** (`/scopes/1` → `scopes[1]`) and the text output's leading path;
      it is a deliberate break, called out in the README.
    - **TWO SENTINELS, NOT ONE, AND THEY ARE NOT INTERCHANGEABLE.** `(root)`
      (`FieldDocument`) is the manifest DOCUMENT — absent, unparseable, or a
      schema violation the library locates at the top-level object. `(project)`
      (`FieldProject`) is repository state OUTSIDE the manifest — the committed
      lockfile, the source tree the item-20 advisory reads. Collapsing them would
      send a CI job looking in `block.manifest.json` for a missing lockfile. Note
      the lockfile remedy *does* offer a `buildCommand` edit as one option; the
      finding still is not ABOUT that field, and pinning it there would mis-group
      every project that takes the other remedy.
    - **Field assignment is mechanical, not taste.** A finding names the location
      it is ABOUT — the field whose presence, absence or value the rule reports
      on. For an "X is set but Y is missing" pair rule that is **Y**, which reads
      straight off the sentence and stays symmetric across the pair. Per-element
      and per-key rules carry the index or key (`targets[0].slotId`,
      `scopeJustifications.<scope>`), because `targets` alone is useless on the
      only manifests where those rules fire more than once. All three clauses are
      ASSERTED, not merely stated: the pair-rule convention by
      `TestPairRuleNamesTheMissingField` (which also requires the message to name
      BOTH sides, so "the missing one" is a claim about a pair that exists), the
      per-element/per-key ones by rows in `findingFieldLedger()`. It was stated
      here and asserted nowhere for one release, and an inverted pair survived a
      green suite.
    - 🔴 **`dedupe` KEYS ON THE (field, message) PAIR.** The value being deduped
      grew a second axis and collapsing on either alone loses real findings: an
      iframe is routinely wrong several ways at once (same field, different
      messages), and `scopeJustifications` emits one finding per offending key
      (same message shape, different fields). Pinned by
      `TestDedupeFindingsKeysOnTheFieldMessagePAIR`.
    - 🔴 **`findingSiteHook` IS A TEST SEAM IN PRODUCTION CODE, AND IT IS THERE
      BECAUSE "EVERY FINDING I SAW HAD A FIELD" IS A CLAIM ABOUT THE CORPUS, NOT
      ABOUT THE CHECKS.** A corpus that never trips the sandbox rule reports a
      serene pass for a sandbox rule that emits nothing — the reassuring-zero
      shape. `TestEveryCheckEmitsAField` therefore AST-enumerates every
      finding-producing function in the package and, through the hook, requires
      the corpus to have REACHED each one; a check added without a fixture fails
      by name and source line. Measured: deleting the `targets` fixture failed
      with `targetChecks (targets.go:89) was never reached`. The hook is nil in
      every production run — one nil compare per finding, and `runtime.Caller` is
      not entered unless a test installed it. Do not delete it to "clean up"; the
      coverage claim becomes vacuous the moment it goes.
    - 🔴 **`make fmt` ONCE DISARMED GUARD A, AND THE FIX IS THAT THE FUNNEL RULE
      IS SPELLING-INDEPENDENT.** The first version of the AST scan matched only a
      composite literal that NAMES ITS OWN TYPE (`Finding{…}` — an
      `*ast.CompositeLit` whose `Type` is an `*ast.Ident`). An element with its
      type ELIDED — `[]Finding{{Message: …}}` — has `Type == nil`, so the scan
      skipped it. That is not an exotic spelling: **`gofmt -s` REWRITES the
      caught form into the uncaught one**, and this repo runs `gofmt -s -w .` in
      `make fmt` and enforces `gofmt -s -l .` in CI. The repo's own formatter
      converted every literal the guard could see into one it could not.
      Measured on the merged tree: a new check returning
      `[]Finding{{Message: …}}` with NO Field, wired into the real pipeline and
      with no corpus fixture, passed the ENTIRE suite (18/18 packages `ok`,
      `gofmt -s -l .` clean over 278 files) — guard B could not see it either,
      because its ledger is built from `newFinding` call sites and this check
      never calls `newFinding`. `findingLitDepth` now resolves the element type
      from the ENCLOSING literal (`[]Finding`, `[]*Finding`, `map[k]Finding`,
      `[][]Finding`), and `TestFindingFunnelScannerSeesEveryLiteralSpelling`
      drives the scanner against a control corpus of 15 spellings — both the
      forms that MUST be flagged and the forms that must NOT — so a future
      narrowing of the guard fails loudly instead of silently.
      **Do not describe the funnel rule as being about `Finding{…}`**; an earlier
      revision of this item and of the test's own doc comment both did, and both
      were true only for the spelling gofmt deletes.
      Related hole closed in the same change: the scan iterated `f.Decls` and
      skipped everything that was not an `*ast.FuncDecl`, so a package-level
      `var x = newFinding("", …)` was invisible to BOTH guards. It now walks the
      whole file and rejects any construction outside a function body by name —
      such a site runs at init, before a test can install `findingSiteHook`, so
      the reachability ledger can never attribute it.
    - **Four guards, and none subsumes the others.**
      `TestFindingsAreConstructedWithAField` (structural, AST) fires on the
      SOURCE, so it catches a new check nobody wrote a test for — but cannot see a
      field computed to `""` at runtime. `TestEveryCheckEmitsAField`
      (behavioural + ledger) catches exactly that — but only for checks a fixture
      reaches. `TestEveryFindingCarriesItsDocumentedField` +
      `TestPairRuleNamesTheMissingField` (`finding_fields_test.go`) pin the
      field's VALUE — see the bullet below. `TestValidateJSONEveryFindingCarriesAField` /
      `…SemanticChecksAreFieldTagged` (in `internal/cmd`) assert on the DECODED
      `map[string]any` of real `--json` stdout, per AGENTS item 14's reasoning: a
      `strings.Contains(out, "field")` is satisfied by the word appearing inside a
      MESSAGE and cannot see `"field": null` at all. Mutation results: a literal
      `""` field kills guard A at its site; a runtime-computed `""` kills guard B
      and the per-check cmd guard; an elided-type `[]Finding{{…}}` literal kills
      guard A's funnel rule with its own message; a package-level `newFinding`
      kills guard A's attribution rule; reverting `fieldPath` to JSON Pointer
      kills all four notation guards; deleting one corpus fixture kills guard B's
      ledger by name.
    - 🔴 **A NON-EMPTY FIELD IS NOT THE CONTRACT — THE SPECIFIC FIELD IS, AND
      MUTATING ONE TO A *WRONG BUT PLAUSIBLE* VALUE IS THE SHAPE THE FIRST SWEEP
      NEVER TRIED.** Every guard above answers "is the field non-empty / in the
      right notation?", and the original sweep only ever mutated a field to `""`
      or to JSON Pointer — both of which those guards see. It never mutated one
      to another real field name, which is what a refactor actually produces. An
      audit found three such mutants surviving the full green suite:
      `buildCoherence`'s two pair rules INVERTED; the budgeted-scope-no-page
      warning moved `page` → `scopes`; and **both lockfile findings moved
      `(project)` → `(root)`**, i.e. exactly the sentinel collapse this item
      already marked 🔴, undetected. Note the near miss that let the last one
      through: guard B's positive control asserts the corpus produced *a*
      `(project)` finding, and `readyack.go` still produced one — a control
      satisfied by a bystander.
      `findingFieldLedger()` closes it with a BIDIRECTIONAL ledger over every
      finding the corpus produces: each finding must match exactly ONE entry (a
      new check with no row fails, naming itself), and each entry must match at
      least one finding (a stale row cannot sit there looking like coverage).
      Each row records the field AND `why` that field — derived from what the
      check MEANS, never copied from the implementation, or the test is a
      restatement of the thing it constrains.
    - **The residual `dedupe`'s second axis knowingly ships with.** Two
      `targets[]` entries carrying the SAME unknown `slotId` now print two
      identical-looking lines in the human output, because the message
      interpolates the slot id but not the index, and the (field, message) key
      keeps both. Measured. Under the old message-keyed dedupe they collapsed to
      one. `--json` is unaffected and is arguably *more* correct — the two
      findings carry `targets[0].slotId` and `targets[1].slotId` and a consumer
      can point at both lines. Fixing the text would mean interpolating the index
      into the message, which changes author-facing copy for a cosmetic
      duplicate; it is left alone deliberately rather than by oversight.
    - **The `issues()` empty-field fallback in `app_validate.go` maps `""` →
      `(root)`, and it is DEFENCE IN DEPTH, not a working path.** It upholds the
      README's "never null" for a case the guards missed. 🔴 State the cost with
      it, because it is what makes a per-SHAPE assertion insufficient: measured on
      the runtime-empty mutant, `TestValidateJSONEveryFindingCarriesAField`
      **passed** — the fallback supplied a non-null value — while
      `TestValidateJSONSemanticChecksAreFieldTagged`, which asks for the SPECIFIC
      field, failed on `(root)`. Assert the field a check must carry, never merely
      that it carried one. The unrestricted claim over every check stays with
      `TestEveryCheckEmitsAField`, which sees findings before the fallback.
    - **The human-readable output is `Finding.Message` verbatim**, which is why
      `Message` is the complete line rather than a reason fragment to compose with
      `Field`: the semantic messages name their field inside prose
      ("iframe.sandbox MUST NOT combine …") and composing would stutter. Sorting
      is by Message first, Field second, so the text output's order is unchanged
      from the string era. `validate.Messages()` is the projection the three text
      consumers (`app validate`, `app submit`, `app init`'s self-check) use.
