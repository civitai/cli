# AGENTS.md item 25 — the listing-media dimension and aspect bounds stay prose in the README

Evidence for item 25 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md keeps the stub — the thesis, plus
enough to tell you whether this item bears on what you are about to change.
Everything below the rule is that item's body, moved here VERBATIM: the
measurements, mutation matrices, retractions and residuals are consulted when
editing the code they are about, not on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, and `agents_split_preserved_test.go` pins the body
against the text it was moved from.

---

25. **The listing-media DIMENSION and ASPECT bounds live in the README as prose,
    and must NOT become a local check.** `civitai app listing set-icon` /
    `set-cover` / `add-screenshot` validate the **format** and the **byte size**
    of the source file (`maxIconBytes` / `maxCoverBytes` /
    `maxScreenshotBytes`) and nothing else. The platform additionally enforces a
    per-kind aspect range and a minimum dimension at ATTACH time
    (`civitai/civitai → src/server/schema/blocks/app-listing.schema.ts`,
    `validateListingImage`), returning a `BAD_REQUEST` that names the bound and
    the measured value. Those numbers are documented in README →
    *Listing media requirements* and in the scaffolded `assets/README.md`, and
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
