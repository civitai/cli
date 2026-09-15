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
| projected after phases 2–3 | ~3,400 | ~249,000 |

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

### 🔴 The unlock

The 60 Troubleshooting rows are pinned on **column one only** — the 68 symptom
strings must exist verbatim in non-test source.
`internal/cmd/readme_troubleshooting_test.go` states in its own comments that it
asserts nothing about the middle column. **That 35 KB of essay-length cause cells is
the largest unpinned region in the file**, and it is where phase 3 operates.

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

## Phase 3 — compress (~41,000 bytes, no facts deleted)

**Troubleshooting: 35,250 → ~13,000.** Sixty rows in a three-column table averaging
~350 bytes *per line of file*, with individual cause cells at 2,952, 2,299 and 1,636
bytes. A lookup table whose cells are essays is no longer a lookup table. Cap each
cause cell at ~2 sentences plus the anchor it already carries; relocate the detail
into the owning section. **The symptom column does not move** — it is the pinned half.

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
checking them. Convert all five to absolute URLs.

## How far to go

| option | result | cost |
|---|---|---|
| **A (chosen)** — cut, compress, reorder in place; maintainer content linked out | ~249,000 B (−20%) | README stays fully self-contained. No test changes beyond pinned prose being cut. 4–6 reviewable PRs. |
| B — A, plus deep reference to GitHub-only docs | ~180,000 B (−42%) | Relocates `Generate`'s five deep subsections (~22,900 B) and `Submit & auth`'s `####` blocks (~27,300 B). **Two of those are the golden-file and exact-stdout pins, so the tests must be repointed at the new path** — a docs edit becomes a code change. Tarball and cask readers lose the detail offline. |
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

Gate every step on `go test ./internal/cmd/ -run 'README|Readme|readme'` plus the
full suite. `build-test` is a required status check, so a broken pin blocks the merge
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
