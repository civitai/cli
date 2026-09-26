# AGENTS.md item 25 — the listing-media dimension and aspect bounds are guidance, never a local check

Evidence for item 25 of the *Intentional decisions that look wrong* list in
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

> 25. **The listing-media DIMENSION and ASPECT bounds live in the README as prose,
>     and must NOT become a local check.** `civitai app listing set-icon` /
>     `set-cover` / `add-screenshot` validate the **format** and the **byte size**
>     of the source file and nothing else; the platform enforces a per-kind aspect
>     range and a minimum dimension at ATTACH time. This is item 4's argument
>     applied to a different constant set: stale *guidance* costs one round-trip,
>     while a stale *gate* refuses valid images and the author cannot override it.
>     🔴 Never scaffold a placeholder icon or cover — it passes every check and can
>     reach a public store listing.

## 2026-08-10 — the icon re-encode cap is REACHABLE, and routinely (#344, #295)

The body below already said the two icon byte caps measure different bytes.
What nobody had was evidence that the second one bites in ordinary use — #286
closed with the reachability recorded as **unmeasured**. A credentialed dogfood
run measured it. This section is appended ABOVE the pinned body on purpose:
`agents_split_preserved_test.go` digests the item verbatim from its `25. **`
heading down, so new evidence goes here, not into the body.

**Observed.** `civitai app listing set-icon ./icon1024_q15.jpg` on a 1024×1024
JPEG of **38,201 bytes** was refused by `appListings.setIcon` with a 400 naming
**1,202,233 bytes** against a **1,048,576** cap. Reproduced 3×. A second source
at the same ≤1024 px ceiling reported **2,201,537**. A 512×512 JPEG (79.2 KiB)
attached cleanly.

**What that rules out, and what it does not.** The two rejections sit at the
same ≤1024 px re-encode ceiling yet report **1.15** and **2.10 bytes per pixel**.
A raw decoded buffer is a *constant* 3 or 4 bytes/px, so a content-dependent
ratio rules raw decoding out: the quantity is a **compressed re-encode**, which
is what the body already reads `listing-meta.service.ts` as doing. That is the
**effect**, measured end-to-end through the public API. The encoder's exact
settings are **not** knowable from this repo — no server source is checked out
here — so nothing was verified about *how* the number is produced, only that it
tracks content as a compressed encode must. The 512×512 pass is **one point**,
not a bound.

**Why no local gate was added.** Predicting the number requires reproducing the
server's PNG encoder; a bound picked from these two samples would be exactly the
stale *gate* the body argues against — refusing valid images while still failing
to predict the case it was added for. `maxIconBytes` therefore stays at 2 MiB:
lowering it to 1 MiB would refuse a 1.5 MiB 512×512 source that re-encodes far
under the server's cap. What shipped instead — the decoded dimensions are
printed on the upload line (#295), and a BAD_REQUEST from a listing-media
ingest/attach is annotated with the source file's own measurements plus the
re-encode mechanism, so the server's byte count acquires visible units
(`attachRejectionAdvice`, `internal/cmd/app_listing.go`). Every number in that
annotation comes from the author's file; the platform's sentence is still
relayed verbatim ahead of it.

## 2026-09-25 — AMENDED: the LETTER was broader than the ARGUMENT, and the prose has left the README

**Operator decision, 2026-09-25.** The body below opens *"…live in the README as
prose"*, and every line of reasoning under it is about one thing: not turning
**guidance** into a **gate** — *"stale guidance costs one round-trip, while a
stale gate refuses valid images and the author cannot override it."* Relocating
prose does not do that. So the item's letter named a LOCATION where its argument
only ever constrained a MECHANISM, and the location half is retired.

**What is unchanged, and it is the whole load-bearing half.** No local dimension
or aspect check. No `LISTING_ICON_ASPECT_MIN` in `internal/cmd`. No promotion of
the table into `internal/validate`. The CLI still decodes width/height
(`appapi.DecodeImageInfo`) and still declines to gate on it; the omission is
still a decision, not a missing feature. Nothing in this amendment weakens that,
and the trigger in AGENTS.md was widened rather than narrowed — it now also
routes a reader who is about to MOVE the documentation, so this decision is
reachable from that situation too.

**What changed.** `README.md`'s `### Listing media requirements` section (4,180
bytes) is deleted. It was a duplicate:
`https://developer.civitai.com/apps/guide/store-listing` carries both tables,
the four behaviours, the megapixel ceiling and the same
guidance-not-a-gate note. This continues #707's phase 2, which had already moved
the rest of the `app listing` walkthrough to that page and left this section
behind.

**The page was verified by CONTENT before anything was deleted, not by URL.**
Fetched (HTTP 200, 120,341 B), tag-stripped to 23,196 B of text, then grepped
for every number the section carried: the three aspect ranges (`0.9`/`1.1`,
`1.3`/`2.4`, `0.4`/`2.6`), the three minimum dimensions (`128`, `640`, `320`),
the byte caps (`2 MiB` ×3, `4 MiB`), the re-encode ceiling (`1024`), and
`megapixel`. All present, and present as the same two tables rather than as
scattered digits — the stripped text was read, not only counted. Negative
control on the same corpus: `zzzznotpresent`, `9999 px` and `7.7 MiB` each
returned 0, so a non-zero above is a match and not a grep that matches anything.

**Five user-visible strings were repointed**, all of which named the deleted
section by title: the `Long` bodies of `set-icon`, `set-cover` and
`add-screenshot` (which is `--help` text a user reads), and the two
`attachRejectionAdvice` error strings. They now read `Platform bounds:
https://developer.civitai.com/apps/guide/store-listing`. That spelling is 71
bytes against the old 72, which is load-bearing:
`TestListingHelpStaysWithinTheBudget` caps `add-screenshot`'s `Long` at 1,400
characters and it stood at 1,396. The first, longer wording overran it by 31 and
the test caught it. `internal/cmd/app_listing.go` is now in
`docsURLSpellingLedger` — it is the most expensive row there, because these five
strings are compiled into released binaries and no docs edit reaches a version
already on somebody's PATH.

**The drift guards were re-pointed, not deleted.** Four test functions read the
deleted section, three of them code↔prose drift assertions. A hosted page cannot
be an oracle for a hermetic test, so they now read the copy of the same prose
that is still in this repository: the `assets/README.md` every template
scaffolds, which the body below already names as the second documented home.
That subject is wider than the README section was in one respect only — it is a
SHIPPED artefact, written into somebody else's project and not recallable.
🔴 **It is NOT wider for being three files**, and an earlier version of this
paragraph said "there are three copies" as though that multiplied coverage. The
three `assets/README.md.tmpl` are byte-identical (md5
`51ebc5538e3bccbd14b2fbd93a928228`, templates and rendered output alike) and
nothing pins them so: the nine subtests are THREE real comparisons run three
times. That is a count of declarations, not instances, and it is withdrawn —
collapsing those templates costs no coverage. What the move DID make stronger is
the cap assertions: `internal/scaffold/assets_dir_test.go` pinned the caps
as literals (`"2 MiB"`, `"4 MiB"`), so the two files agreed with each other and
neither was tied to the constant. The re-pointed guard derives the expected
string from `maxIconBytes` / `maxCoverBytes` / `maxScreenshotBytes`.
`TestREADMEIconAspectDoesNotForbidASquareIcon` moved with them; its own CONTROL
failure is what detected the relocation.

**The rule text below was corrected in place, through the digest mechanism.**
The body under the `---` is digested verbatim against `agentsSplitBaseWave3`, so
it normally cannot be edited at all. Two things in it had become false: the
HEADING said the bounds *"live in the README as prose"*, and the paragraph under
it pointed at *"README → Listing media requirements"* — a section this change
deleted. The canonical rule stated a location that no longer existed, and nothing
checked it.

Same class as item 3's false reason, resolved the same documented way: a
**recorded line-for-line delta** reversed before digesting (`item25OldLineA` /
`B` / `C` in `agents_split_preserved_test.go`), with both directions asserted
live. Restoring either dead line, rewording a corrected one, or neutering the
reversal are all RED by name — mutation-tested. The heading now reads *"…are
GUIDANCE wherever documented, and must NOT become a local check"*, which is the
amended rule stated where the rule is.

⚠ **A round-1 version of this section said the heading could NOT be corrected,
because doing so needed the paragraph re-wrapped.** That was a reason composed to
justify a residual rather than derived from the mechanism, and an audit
falsified it: the delta already replaces a 78-column line with a 59-column one,
so raggedness is plainly tolerated, and a single replacement line that still
flows into the next one is a line-for-line swap like any other. It is, at 85
columns. The re-wrap rule itself is unchanged and still true — a multi-line
re-flow is a *sequence* match and that is the loosening — it just never applied
here.

The historical STUB quotation near the top of this file still carries the old
sentence, deliberately: it is a record of what waves 1–3 left behind, and it sits
above the `---` where the digest never reaches.

**Two further residuals, stated rather than glossed.**

1. The quotation ban in `TestListingRequirementsDocDoesNotPinAServerSentence`
   was a REGRESSION guard against the README (red on the tree that shipped the
   stale *"That icon couldn't be read"* paragraph). On its new subject it is an
   INVARIANT guard: the scaffolded docs never carried that defect. The rule is
   the same and worth pinning where the prose now lives, but the historical
   redness belongs to a file that no longer exists.
2. The pixel-ceiling bullet (roughly 16 megapixels, ~4096 × 4096, refused
   regardless of file size) was in the README and not in the scaffolded docs, so
   re-pointing the invariant half required ADDING it to the three
   `assets/README.md.tmpl` copies. That is a content change made to preserve a
   guard, and it is also the right place for it — it is the one rejection the
   byte-cap table cannot explain, and the author reading that file is the one
   who hits it.

---

25. **The listing-media DIMENSION and ASPECT bounds are GUIDANCE wherever documented,
    and must NOT become a local check.** `civitai app listing set-icon` /
    `set-cover` / `add-screenshot` validate the **format** and the **byte size**
    of the source file (`maxIconBytes` / `maxCoverBytes` /
    `maxScreenshotBytes`) and nothing else. The platform additionally enforces a
    per-kind aspect range and a minimum dimension at ATTACH time
    (`civitai/civitai → src/server/schema/blocks/app-listing.schema.ts`,
    `validateListingImage`), returning a `BAD_REQUEST` that names the bound and
    the measured value. Those numbers are documented in the platform's Store
    listing guide and in the scaffolded `assets/README.md`, and
    that is deliberately as far as they go.
    - **Why prose and not a check.** This is item 4's argument, applied to a
      different constant set: stale *guidance* costs one round-trip carrying the
      server's current bound, while a stale *gate* refuses valid images and the
      author cannot override it. A local dimension check would also have to
      re-derive the icon rescale below to avoid being wrong on day one. So do
      not add a `LISTING_ICON_ASPECT_MIN` to `internal/cmd`, and do not "fix"
      the docs by promoting the table into `internal/validate`. The CLI already
      decodes width/height (`appapi.DecodeImageInfo`) — the omission is a
      decision, not a missing feature.
    - **The icon byte cap and the server's icon byte cap measure DIFFERENT
      bytes, and the docs must not conflate them.** `maxIconBytes` (2 MiB) is
      the SOURCE file, mirroring the server's `INLINE_ICON_MAX_DECODED_BYTES`
      on the data-URI path the icon rides. The listing schema's
      `MAX_LISTING_ICON_SIZE_BYTES` (1 MiB) is checked against
      `Image.metadata.size`, which for that path is the byte length of the
      **re-encoded** PNG the server produces after downscaling to ≤1024 px on
      the longer side (`listing-meta.service.ts`) — not the file the author
      passed. Cover and screenshot take the full-res path, where the CLI sends
      `sizeBytes: len(data)`, so there the two caps DO describe the same bytes.
    - **Never scaffold a placeholder icon or cover.** A placeholder passes every
      format and byte check and uploads cleanly, so it can reach a public store
      listing; a missing file fails loudly at the step that can still fix it.
      `assets/` therefore ships with a README and no images, and
      `internal/scaffold/assets_dir_test.go` fails if an image file appears
      under it.
