# AGENTS.md item 30 — a live-listing change may be staged and left unpublished

Evidence for item 30 of the *Intentional decisions that look wrong* list in
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

## Why staging is the correct default for a removal

Curating a store listing is **N removals**, not one. If the first
`app listing rm-screenshot` submitted the shadow revision, it would open a
review cycle the author is not finished preparing — and every subsequent removal
would open another. So `rm-screenshot` writes into the shadow revision and stops
there, saying so, and naming `app listing submit-revision` as the command that
publishes.

#436 is the incident: the command printed a bare `✓ Screenshot removed`. That
sentence was **true of the revision and false of the listing** — the screenshot
was still live to every visitor. A success line that names the wrong object is
not a wording nit; it is a claim about what the user's site now looks like.

## Why the refusals differ between the two commands, on purpose

The below-floor refusal (a listing dropping under its required media minimum)
**FAILS** — exits non-zero — on `submit-revision`, where the attach path exits
0 for what looks like the same condition. That asymmetry is the point, and it
follows from what the job was:

- On the **attach** path the job was the attach. It succeeded. A listing that is
  not yet publishable is an expected intermediate state while curating, and
  reporting it as progress is honest.
- On **`submit-revision`** the submit IS the job. A revision that cannot be
  published has not been published, so exiting 0 would report a publish that did
  not happen — the #436 shape again, one command over.

Read a change to either exit code against that split before "harmonising" them.

---

30. **A live-listing change may be STAGED and left unpublished, on purpose.**
    `rm-screenshot` writes into the shadow revision and does NOT submit it: it
    says so and names `app listing submit-revision`, because curation is N
    removals and the first must not open a review cycle (#436 — the bare
    `✓ Screenshot removed` was true of the revision, false of the listing).
    That command's below-floor refusal therefore FAILS where the attach path
    exits 0: there the job was the attach; here the submit IS the job.
