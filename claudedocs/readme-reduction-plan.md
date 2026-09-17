# README reduction plan

**Status as of 2026-09-15.** Phase 1 (accuracy) is merged as #611. Phases 2–4 are
planned and unstarted.

🔴 **The README is currently BIGGER than when this arc started** — 314,712 bytes
against 310,647. Phase 1 was an accuracy pass, not a reduction: it added six
corrections, a behaviour fix and four guards. No reduction has shipped yet. Phases
2 and 3 are where it happens.

| | lines | bytes |
|---|---:|---:|
| arc start (`426288f`) | 4,179 | 310,647 |
| after phase 1 (`f284d89`) | 4,234 | 314,712 |
| **after phase 3 (`e3c5bd0`, MERGED)** | 4,123 | **305,329** |
| projected end of Option A | — | ~266,261 |
| **closing condition (amended)** | — | **≤ 270,000** |

## 🔴 The method changed after phase 3 — DELETE-FIRST, not relocate

**Operator decisions, 2026-09-15.** Recorded here because the original method is what a
later reader would otherwise re-derive from the phase descriptions below.

**1. Relocation is no longer the default — it is the exception.** Phase 3 removed 15,021
bytes from Troubleshooting and **6,377 came back in five other sections** (Submit & auth
+1,932, Download +1,796, Submission status +1,179, Scripting +859, Global flags +616) —
a **39.7% give-back**, against an ask that said *link-out*. The method preserved the
*string*, not the *reader*: a cause cell is read at the moment of failure, a paragraph
three sections away is read by someone browsing. **The order is now: DELETE what the
binary already says or what is maintainer history → LINK OUT with an absolute
`https://github.com/civitai/cli/blob/main/<doc>` URL → relocate only when a fact is
user-facing, unique, and has a section that genuinely owns it.** The evidence that this
works is the D1 trim: **−1,103 bytes, zero give-back, and MORE correct**, because an
unpinned prose mirror of an error string can drift while the source string cannot.

**2. 🔴 THE BYTE CLOSING CONDITION IS DELETED (2026-09-15). This item is kept only so
the number is not re-derived from the arithmetic below.** It moved 250,000 → 270,000 →
(briefly) 276,000 → gone. Option A's projection was ~266,261: 305,329 − 20,500 (phase 2)
− 13,312 (command reference) − 6,000 (Submit & auth preamble); the 250,000 bar was set
before any trial edit and missed by 16,261, and ⚠ **the projection is itself optimistic**
— it subtracts phases 2 and 4 *gross* while using phase 3's *net*.

**What killed it was step 1 of `the-algorithm`: name the maker.** No user, no incident
and nothing in the repo ever asked `README.md` to be a particular size. The requirement
was self-issued by a prior agent session — which that skill names as *"a department, not
a maker"* — and the arc's stated goal is that the file be **high-level**, for which a
byte count was a proxy that got mistaken for the thing. Set three times by estimate,
missed twice. The gate is now the test filter alone; see the handoff. **A README byte-
ceiling test was designed as the replacement and then discarded** — `README.md` is not
`@`-imported the way `AGENTS.md` is, so the per-session cost that justifies *that*
ceiling does not exist here, and step 5 forbids answering over-growth with a ratchet.

**3. CLI error messages were brought IN SCOPE as phase 5 — and phase 5 is WITHDRAWN
(2026-09-17, see its own section).** The reasoning here still stands as far as it goes:
round 0 on #625 measured that **14 of 19 sampled cause cells were near-verbatim copies
of strings the binary already prints**, and `AGENTS.md` says *"Make errors actionable —
name the next command to run."* 🔴 **But the conclusion drawn from it was backwards.**
That measurement says the cells RESTATE the binary; the phase read it as the binary
lacking what the cells hold, which is the opposite claim and does not follow. Measured
row by row, the binary already carries the remedy in 20 of 21 rows of the target band —
because that `AGENTS.md` rule has been in force the whole time. **The one row where it
did not is shipped.**

## How it got here

Measured from git history, not inferred:

| date | lines |
|---|---:|
| 2026-07-01 | 429 |
| 2026-08-01 | 768 |
| 2026-08-15 | 2,944 |
| 2026-09-01 | 3,772 |
| 2026-09-15 | 4,234 |

A 10× expansion in ten weeks, +2,176 lines in the first half of August alone. This
is not a document that was designed long; it accreted. The fix is an edit, not a
rewrite.

**The structural cause is in the test suite, and it is worth fixing first in your
head before touching prose:** there is no README size ceiling anywhere in the repo
(`agents_size_test.go` caps AGENTS.md; nothing caps README), while roughly a dozen
assertions are *floors* — `len(raw) > 10_000`, `links >= 40`, `headings >= 20`,
`subs >= 25`, `symptoms >= 15`, `checkedTargets >= 3`. The suite punishes this file
for shrinking and never for growing.

## Accuracy: what three audits found

~390 claims checked against source and the built binary. **Zero dead commands, zero
dead flags, zero broken links** — all 205 in-document anchors resolve, and every one
of the 95 distinct flags exists and is valid on the command it is shown with.

Six defects were found and are fixed in #611. Two had Go bugs behind them, filed as
#613 (fixed, Windows `upgrade`) and #614 (open, `CIVITAI_NO_COLOR` value parsing).

🔴 **One contradiction remains UNRESOLVED and is deliberately untouched.** README
states every model-file download needs a token ("even a small public embedding 401s
anonymously"); `internal/cmd/download.go`'s `Long` and its error string both say
*most* do and some public files do not. Settling it needs a live anonymous download
— a measurement, not an edit. No test pins either side.

## The constraint map — what is NOT free to move

19 Go test files read `README.md` by path. **~98,772 bytes — 31.8% of the file at
measurement time — sits inside a section-scoped assertion.** Moving pinned text to
another file makes a test FAIL, not silently pass.

### Do not touch without a corresponding code change

- **~15,808 B — the exit-code table and its six detail subsections.** Generated from
  `internal/cmd/exitcodes_doc.go` and asserted **byte-identical**. There is no
  generator command in the repo — no `go:generate`, no make target. The failing test
  prints the correct text to paste.
- **~10,353 B — the dotenv `####` block.** A whitespace-collapsed **golden file**
  (`internal/pkgzip/testdata/readme_dotenv_section.golden.txt`). Re-approving with
  `-update` rewrites the golden, never the README. Its heading is
  `strings.Index`-searched, so renaming it fails.
- **~1,370 B — the `#389` shadow-write bullet.** Pinned as an **exact normalised
  string**; its own comment says a cosmetic reword fails the test. Deliberate.
- **~880 B — the two `whoami` blocks and the `--json` example.** Compared
  byte-for-byte against live stdout.
- **10 literal headings** are `strings.Index`-searched and cannot be renamed. Two
  heading texts must match exactly *including the 🔴*.

### The Contents block is a bidirectional ledger

Every `##` and every `###` must have a Contents entry linking its anchor, and every
Contents anchor must resolve to an existing in-scope heading. Moving a section out
means deleting its Contents line too. The subsection floor is 25 against ~36 in
scope — about eleven moves of headroom.

Two exemptions exist, keyed by parent `##` name: `Exit codes` (6 generated children)
and `Troubleshooting` (5 lookup buckets). That map is itself ledgered both ways.

### 🔴 The unlock — and its two CORRECTIONS, measured by the phase-3 trial edit

The 60 Troubleshooting rows are pinned on **column one** — the 68 symptom strings must
exist verbatim in non-test source — and the cause cells are *mostly* free. That is what
phase 3 operated on, and it worked: **−15,021 bytes, −42.6%** (#625).

🔴 **CORRECTION 1 — "the middle column is pinned by NOTHING" was FALSE.** This section
said so on the strength of a comment in `readme_troubleshooting_test.go`. **Rows 19 and
20 ARE pinned.** Row 20's cause cell carries **nine** exact `strings.Replace` spans that
`t.Fatal` when absent — including `"because validation reports the row above first. "`,
*with a trailing space* — and both cells are additionally prose-parsed for positive and
negative command attribution. They were left verbatim and are 969 bytes of the result.
**Do not re-derive the original claim from that comment; the comment is narrower than
the file.**

🔴 **CORRECTION 2 — the gate this doc and every brief specified is TOO NARROW.**
`go test ./internal/cmd/ -run 'README|Readme|readme'` matches **zero** of the guards on
row 20's cell (`TestAttributionProseParser`,
`TestAttributionProseCheckAcceptsCorrectProseAndRejectsMisattribution`) — verified by
counting matches, which is 0. The narrow filter reports green while those frozen spans
are unguarded. Use `-run 'Attribution|Troubleshooting|README|Readme|readme'`.

## Phase 2 — cut (~20,500 bytes)

None of this is pinned. All of it is maintainer-facing rather than user-facing.

| what | bytes | why it goes |
|---|---:|---|
| `## Development` + `## Releasing` | 6,926 | Maintainer-only: names `HOMEBREW_TAP_GITHUB_TOKEN` and commands only a maintainer can run. The README already says "see AGENTS.md for the full process" and then restates 88 lines of it. Four copies of the 2026-08-09 tap incident exist across the repo. |
| 5 `<sub>` changelog footnotes | 1,712 | All of the form "Until `civitai/cli#NNN` this was discarded at parse time…". One is an explicit retraction of an earlier paragraph. This is a changelog wearing a documentation costume. |
| PR triage in the listing reference | 3,076 | "#422 retired the 'cannot be addressed' refusal, and #389 retired nothing"; "measured 2026-08-17: four offsite apps and one onsite control". |
| bundle-size retraction essay | 2,251 | "That reverses what this section used to say"; "an earlier version of this section overstated it", plus a predicted-vs-actual table captioned "a sanity check, not evidence". The conclusion is user-facing; the reasoning is not. |
| 10 scattered maintainer asides | 4,445 | Includes one addressed to a **contributor, not a user**: "please don't 'helpfully' promote these numbers into a local check (see AGENTS.md item 25)". Others point users at `*_test.go` files and `internal/` paths they cannot open. |
| 4 duplication clusters | ~4,700 | The offsite "best-effort" caveat appears **4×** near word-for-word; the 429 exit-code taxonomy is re-derived **6×** (two hand-written, four generated); the two `app submit` guards **4×** each; the `app doctor` code list **4×**. |

**Not on this list, deliberately:** the decision-rationale AGENTS.md sanctions as a
double statement — the 10485760-byte submit ceiling, the listing media bounds — stays.

🔴 **TWO PASSAGES ARE PERMANENTLY KEEP — do not re-propose them in a later phase.**
Phase 2 deferred both as "behind an AGENTS.md trigger"; round 0 then read the decision
files and found the triggers do **more** than require a read:
- **`### Raw graphs`'s retraction paragraph.** `claudedocs/decisions/13-*.md` records that
  a false *money* claim survived every green suite because this surface was unpinned. The
  paragraph is the user-facing record that `"priority": "high"` priced at **28** against
  **8** — an unmodelled graph key can more than triple a spend, and `--dry-run` is where
  you would see it. User-facing money evidence, not archaeology.
- **The icon re-encode measurement.** `claudedocs/decisions/25-*.md` states the CLI's icon
  byte cap and the server's measure **different bytes** and that "the docs must not
  conflate them". This paragraph is what prevents the conflation — item 25 requires it,
  it does not merely permit it.

⚠ **Phase 2's ~20,500 figure was a target and should not have been.** This doc labels its
own per-cluster byte counts "judgement over sampled reads and **upper bounds**", so being
"short" against one is meaningless — and chasing the last 6,100 would have cut both
passages above. **The only binding number in this arc is the closing condition.**

## Phase 3 — compress (~41,000 bytes, no facts deleted)

**Troubleshooting: 35,250 → 20,229. DONE (#625), and the ~13,000 target this doc
originally carried was BELOW THE FLOOR.** Sixty rows averaging ~350 bytes *per line of
file*, cause cells at 2,952, 2,299 and 1,636. Each cell was capped at ~2 sentences plus
the anchor it already carries, detail merged into the owning section. Symptom column
byte-identical.

🔴 **Why 13,000 was unreachable — the arithmetic this doc lacked until a trial edit
produced it.** **7,081 bytes of the section are not cause cells at all**: the pinned
symptom column, the link column, 60 row delimiters, five bucket headers, the preamble
and the footer. A 13,000 target therefore allows 5,919 bytes across 60 cells = 99 bytes
each — and rows 19–20 are frozen at 969 of that, leaving **85 bytes** for the other 58.
That is one clause, not "what happened and what you do next". **At two sentences the
floor is ~19–20 KB.** Every other projection below is of the same kind — derived from
section sizes, not from a trial edit — so treat them as optimistic until one is run.

**Command reference: 21,812 → ~8,500.** Twenty-five rows in which one cell is
**3,794 bytes** (`agent-setup`, which inlines a per-agent MCP key table that already
exists ~230 lines earlier) and another is 3,328 (`app listing`). One line of purpose,
the flag synopsis, the anchor. `readme_nav_test.go` already *requires* each row to
carry a resolving anchor link, so the guard favours this design. The `app listing`
row must keep naming every subcommand — that part is pinned.

**Submit & auth preamble: ~6,000.** 30,128 bytes sit before the section's first
`###`, holding five `####` blocks the Contents block cannot see. Promote them to
`###` (which also helps the subsection floor) and tighten the connective prose. Two
of the five are pinned and keep their exact text.

**Also worth fixing while in there:** four real subcommands — `set-text`,
`rm-screenshot`, `reorder`, `submit-revision` — appear **zero times inside any of the
66 code fences**. The table guard enforces presence, which is not usability.

## Phase 4 — reorganise (byte-neutral; fixes the reader paths)

Two things are simply in the wrong place.

1. **App-authoring subsections are nested under `## Command reference`** — the
   blockId, Templates, the host handshake, the local dev loop, dev-tunnel and
   Examples all live inside it, while the Contents block lists them flat under
   "Author an App". The outline Contents implies is not the outline of the file.
2. **Read-path material is sandwiched inside the authoring track** — Browse the
   public API, Download model files and Scripting with `--json` sit between the
   authoring sections and Validate fidelity.

Measured reader cost before the change:

| reader | must scroll | to reach |
|---|---:|---|
| wants to download a model file | 80,126 B (25.8%) | `## Download model files` |
| building a first App Block | 100,175 B (32.2%) | `## Validate fidelity` |
| scripting `--json` in CI | 251,849 B (**81.1%**) | `## Exit codes` — the contract they must branch on |

The CI reader is worst served: 52% of the entire file sits between the section they
were sent to and the exit-code contract.

🔴 **The single most consequential misplacement**: the warning that `listing status`
**is not a pure read** — "so do not poll it in a loop" — sits ~36% in, inside *After
you submit* under *Submit & auth*. It is the one bullet telling a CI author their
read is a write, and a `--json` reader following Contents never passes it. It is also
the most tightly pinned prose in the document, so it cannot be reworded — but it can
and should be **cross-linked** from the scripting section.

`## Set up your coding agent` is 16,972 bytes with **zero subsections** — 265 unbroken
lines, no internal map, no Contents sub-entries. Giving it 4–5 `###` also raises the
subsection count, which helps the floor.

🔴 **RETRACTED 2026-09-15, IN BOTH HALVES. The original read: "Phase 4 must fix a
contradiction at TWO sites, not one" — there is no contradiction, and there is one site.**

**The count.** Measured on `main`: `degrades rather than enforcing` appears at
**`README.md:3961`** and nowhere else in the file, plus its generator
`internal/cmd/exitcodes_doc.go:146`. The second copy this item sent phase 4 to fix was in
the `app submit` command-reference row, and **#633's command-table rewrite removed it** —
so the instruction outlived the thing it pointed at.

**The contradiction.** The clause was read as the opposite of the no-commits-yet refusal
under `##### What the dirty-work-tree guard counts as a change`. Read end to end, they are
**two different cases and both sentences are true**. "Degrades" is scoped by its own
em-dash clause to the *absence of git context*: `app_submit_dirty_guard.go:250-257` runs
`rev-parse --is-inside-work-tree` and returns early when it errors or prints anything but
`true` — no repo, a bare repo, or no `git` on `PATH`. A `git init` repo with no commits
prints **`true`**, so the guard proceeds: `headCommit` fails leaving provenance empty
(`:278`), `git status --porcelain` reports every file as `??`, and it takes the **ordinary
refusal** branch. That is not a degrade.

🔴 **So phase 4 must NOT "fix" this.** Doing so would mean editing byte-pinned generated
text in `exitcodes_doc.go` to disambiguate something that is not ambiguous — and the
former argument against it (*"in a PR whose requirement is give-back 0"*) is itself void
now that the byte condition is deleted. The reason to leave it alone is that **it is
correct**, which is the durable one.

## Phase 5 — WITHDRAWN 2026-09-17, measured row by row

🔴 **WITHDRAWN. Do not re-open it on the numbers below — they are the numbers that
killed it.** The phase said: *"for each row where the README still says more than the
binary does, move the difference into the Go error string and delete the row."* Measured
against the live section, **the premise is false for 20 of 21 rows in its own target
band**: the binary already says it.

**The instrument mattered, and the first one was wrong.** A one-line regex over
non-test Go called 12 rows "no remedy in the binary". Every one hand-checked was a
**false negative** — these messages are `+`-joined across many lines, and a single-line
read truncates them. That is the same overcounting the handoff already recorded for the
crude 25-of-49 pass. The classification below comes from reading the WHOLE statement.

The 21 rows in the 150–250 B band:

| | rows | why it is not phase-5 work |
|---|---:|---|
| the binary already carries the remedy in full | **11** | the README cell restates it |
| the cell holds what a per-site string structurally cannot | **3** | two exit codes off one message (`rate limited (429)`); "the two rows below are the 403s that are not about your grant"; an exit code the binary never prints as text |
| not an error message at all | **6** | a `whoami` row label, a `workflows list` legend, an `errors.New` sentinel, a note, two output labels — and one string **your app prints at runtime**, which the CLI cannot reach |
| genuine | **1** | `not logged in (401)` — shipped, see below |

Three examples, because the pattern is uniform rather than marginal:

- **`HEAD is on no remote`** — README: *"Push the branch."* Binary already: *"Push the
  branch so the submitted version can be traced back to a commit."*
- **`nothing index.html loads reaches it`** — README: *"Copying `civitai-host.js` in is
  only half the fix — it has to be referenced too."* `readyAckRemedy` already: *"Copying
  the file in is not enough on its own — a browser never fetches a file nothing
  references."*
- **`no lockfile is committed`** — README: *"Generate it with the package manager."*
  Binary already: *"Run `%s` and commit the %s it writes."*

⚠ **AND ITS CEILING WAS STALE IN BOTH DIRECTIONS.** The ≤5,465 B figure was measured
when `README.md` was 282,312 B, three PRs ago. Live: the section is **20,834 B / 61
rows**, the non-floor 150–250 band is **18 rows / 3,480 cause-bytes**, and the incident
floor now protects **10** rows, not 7. Re-measure before quoting any of it.

🔴 **The one genuine row was shipped, and it turned into a consolidation.** The 401
message existed at **five** sites in `internal/appapi`; four spelled the remedy
`run `civitai login` (or set CIVITAI_TOKEN)` and the fifth — `submissionsError` — said
`run `civitai login``, dropping the environment-variable route for the command group
most likely to run in CI. Nobody chose that; it is what an open-coded predicate does at
N sites. All five now call `unauthorizedError`, which also names the personal-API-key
route the README had and **no user-facing surface did**. `TestUnauthorizedCallersAreLedgered`
pins the caller set in both directions.

**The transferable finding:** this repo's error strings were already carrying their
remedies, because `AGENTS.md` has said *"make errors actionable — name the next command
to run"* the whole time. A phase premised on them NOT doing so was mis-specified from
the start, and only a row-by-row read could show it. **Where a README cell is longer
than the error, read the error before assuming the cell adds something.**

## Link-out policy

🔴 **`.goreleaser.yaml` archives only `README.md` and `LICENSE`.** npm ships a
separate `npm/README.md`. So AGENTS.md, CONTRIBUTING.md, `claudedocs/` and any
hypothetical `docs/` reach **github.com readers only**. "Move it to another file" is a
real loss of user-reachable content, not a free win.

The repo already encodes the mechanism: `readme_contributor_links_test.go` bans
*relative* links to unshipped docs and its own failure message prescribes the fix —
the absolute `https://github.com/civitai/cli/blob/main/<doc>` URL, which its
normaliser deliberately exempts. That degrades honestly: a tarball reader gets a
working URL instead of a 404.

So the rule is about **audience**, not file size:

- **Maintainer content moves out freely** — anyone who needs it has a checkout.
- **User contract content never moves out** — exit codes, `--json` shapes, the
  command reference, Troubleshooting. These stay in the shipped file whatever they cost.
- **Deep reference is the judgement call** — see "How far to go".

⚠ **Five of the six existing relative links already 404 for a non-checkout reader**
(`internal/scaffold/…`, `examples/`, `schema/…`). The guard covers only three named
files by design, and its own control comment names the uncovered ones without
checking them.

🔴 **AND THE SAME HAZARD IS REACHABLE BY DELETION, NOT ONLY BY CONVERSION.** Phase 2
deleted one relative link as an unopenable `internal/` pointer — sanctioned by this
doc's own phase-2 row — taking the control from **6 to 5 against a floor of 3**. The
four remaining deletable ones are `examples/` ×3 and `schema/…`; deleting them leaves
only `LICENSE` and the control `t.Fatal`s with *"only 1 relative FILE link target(s)
survived normalisation"*, which reads as a broken normaliser rather than as your own
cut. **Re-measure `checkedTargets` before deleting any relative link**, or widen the
control first.

🔴 **DO NOT "convert all five to absolute URLs" — this doc said to, and that would
BREAK the guard enforcing the absolute-link rule.** `readme_contributor_links_test.go`
carries a `checkedTargets >= 3` CONTROL that counts relative **file** link targets
surviving normalisation; it exists to catch a normaliser that has stopped seeing
anything, so an empty set is indistinguishable from a broken instrument and it
`t.Fatal`s. **Measured 2026-09-15: exactly 5 survive, floor 3** — converting all five
drives it to 0 and reddens the suite. Fixing the 404s therefore requires widening that
control *first*, which is a code change, and it must keep a positive control that can
still fail. Until then, leave the relative links alone.

## How far to go

| option | result | cost |
|---|---|---|
| **A (chosen)** — cut, compress, reorder in place; maintainer content linked out | **~266,261 B (−14%)** — revised from ~249,000 after the phase-3 trial edit | README stays fully self-contained. No test changes beyond pinned prose being cut. 4–6 reviewable PRs. ⚠ **The "does NOT meet the ≤250,000 closing condition" line that stood here is VOID — there is no byte closing condition any more** (2026-09-15). The figure is a projection, which is all it ever was. |
| B — A, plus deep reference to GitHub-only docs | ~180,000 B (−42%) | **Offered 2026-09-15 and DECLINED.** Measured live rather than estimated: `Generate`'s five deep subsections = **26,289 B**; `Submit & auth`'s **un-pinned** `####` blocks = **10,490 B** (the dotenv golden-file block, 10,353, and the whoami exact-stdout blocks, 5,841, are excluded — moving them repoints a test and turns a docs edit into a code change). Declined on the link-out policy: `.goreleaser.yaml` ships only `README.md` and `LICENSE`, so every byte moved out is lost offline to tarball, Homebrew-cask and npm readers. |
| C — split the read path into its own document | ~120,000 B (−61%) | Not recommended. That track is what a shipped README is for; moving it makes the offline binary's only documentation an App-authoring guide. |

A and B are sequential, not exclusive.

## Sequencing

1. ~~Accuracy fixes~~ — **merged as #611.**
2. Any open PR touching README must land or be rebased onto first. At time of
   writing that is **#602** (edits 1671-1712 and three Troubleshooting rows) — it
   collides with phases 2 and 3 equally.
3. Troubleshooting compression — largest win, lowest risk (unpinned middle column).
4. The cuts.
5. Command-reference compression.
6. The reorder **last**, when the pieces are their final size. A reorder diff over
   uncompressed text is unreviewable.

Gate every step on `go test ./internal/cmd/ -run
'Attribution|Troubleshooting|README|Readme|readme'` — 🔴 **the WIDE filter; the narrow
one this line used to name matches zero of the guards on the Troubleshooting table's two
frozen cells, and reports `ok` over a live mutant** — plus the full suite. `build-test` is a required status check, so a broken pin blocks the merge
rather than slipping through. `make ci` does **not** run lint — that is a separate CI
job, and golangci-lint is not on the bare PATH here (`nix-shell -p golangci-lint`).

## Stated limits

- Whether a public model file really requires a token was **not measured**.
- Paraphrase-level duplication has no exact metric. The 4-gram overlap of 5.0% bounds
  the *lexical* ceiling, not the semantic one, so the per-cluster byte figures in
  phase 2 are judgement over sampled reads and are **upper bounds**.
- The post-edit byte projections are derived from measured section sizes, not from a
  trial edit.
- The constraint map's 31.8% was measured at 310,647 bytes, before phase 1.
