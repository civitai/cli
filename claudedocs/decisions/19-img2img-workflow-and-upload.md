# AGENTS.md item 19 — img2img — txt2img plus images[], --ecosystem, and no credential

Evidence for item 19 of the *Intentional decisions that look wrong* list in
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

## 🔴 (a) IS UNCHANGED, BUT THE SPEND SCREENS NOW EXPLAIN IT — civitai/cli#365

**Read this before editing the workflow line on any generate surface.** Nothing
about the wire value moved. What moved is what the user is TOLD about it.

**What was found.** A blind credentialed dogfood run (2026-08-10, run 3) priced
an img2img job and read, on the screen it was being asked to approve a charge
from:

```
  Ecosystem:  Flux1Kontext
  Image:      ./probe.png (512x512) → https://…/blobs/…
If this ecosystem does not support image editing, the server IGNORES the images
above, generates from the prompt alone and still charges …
Workflow:         txt2img
```

`txt2img` is correct — (a) below is why — but sitting one line under a warning
that the server may IGNORE your images, it is the most alarming thing on the
screen, and nothing on that screen explained it.

**What was built.** One constant, `imagePromotionNote`, and one helper,
`workflowLines(built)`, used by the two surfaces that name the workflow: the
`--dry-run` quote's `Workflow:` row and `confirmGenerate`'s *"About to generate
with …"* line. With no `--image` the note is empty and those screens are
byte-identical to before.

🔴 **THE NOTE IS ITS OWN LINE, AND THAT IS A CORRECTION, NOT A PREFERENCE.** The
first revision returned one string and the confirmation interpolated it
mid-sentence:

```
About to generate with txt2img (image editing is requested AS txt2img plus --image; the server does the promotion, not this CLI) at https://civitai.com.
```

152 characters — at 80 columns the verb is split from its object and `at
https://civitai.com` orphans onto line 2, on the screen immediately before an
irreversible charge. `workflowLines` therefore returns *(label, note)* and each
caller prints the note on the following line; its doc comment carries the
measurement. Do not fold them back into one string.

**The constraint the wording is under, and it is item 28's.** The note states
the server's RULE — image editing is *requested as* `txt2img` plus `--image`,
and the server does the promotion — and never that the promotion FIRED. The CLI
cannot observe that, and (b) below records why no detector is possible. The
caveat `printImageDisclosure` prints on the same screen carries the other half.
🔴 Do not "improve" it into a claim, and do not rename the field: the wire value
must stay visible for (a)'s reason.

**The guards, in item 28's three shapes — and be precise about what each one
covers.** An earlier draft of this paragraph implied the two REAL screens were
golden-pinned with `--image`; they were not, and an audit found the 152-column
line in output no golden had ever seen. The inventory now, stated at the scope
actually measured:

- **One constant** — `imagePromotionNote`.
- **An asserted bidirectional call-site ledger** — `TestWorkflowLabelCallSiteLedger`:
  `workflowLines(built)` at exactly 2 sites, both of them guarding on a
  non-empty note (a caller that takes the pair and drops the second half
  restores the unexplained `Workflow: txt2img` while satisfying a
  call-site-only count), plus a ban on the two surfaces' old bare-constant
  format strings.
- **Golden pinning at two levels.** The helper's own two branches
  (`generate_workflow_label_with_image` / `_without_image`) AND — added by the
  audit fix — the two COMPOSED screens rendered with `--image`:
  `generate_confirmation_with_image`, `generate_dry_run_with_image_stdout`,
  `generate_dry_run_with_image_stderr`. The function-level pair was not a
  substitute: pinning a return value in isolation is exactly how a defect in
  the composition ships. The `t.TempDir()` image path is substituted out, and
  the substitution is asserted to have happened.
- **Two properties no golden can see**, because comparison is
  whitespace-collapsed: `TestGenerateImage_SpendScreenLinesStayReadable` bounds
  the confirmation sentence's column width, and the two
  `…ExplainsThePromotion` tests assert the note is the ADJACENT line rather
  than part of the value.

**Residuals, enumerated rather than implied:**

- A NEW surface that prints its own workflow line without touching either symbol
  is invisible to all of them (item 28's residual, unchanged).
- **`--input` with a hand-written graph carrying an `images` array prints a bare
  `Workflow: txt2img` and no note.** The note keys off `built.images`, which the
  `--input` path never populates because the CLI deliberately does not interpret
  that graph (see the `Graph: … (sent as-is; this CLI did not interpret it)`
  line right beside it). Reading the array to decide would be interpreting it.
  Known and accepted, not overlooked.
- The goldens are whitespace-collapsed, so they pin words and order, never line
  breaks — which is why the width bound and the adjacency assertions above are
  separate guards rather than redundant ones.

## The stub thesis this item's trigger replaced

Waves 1–3 of the evidence split (#290, #305, #310) left a multi-line STUB in
AGENTS.md here. That stub was prose written for the split — a compression of the
body below, not a slice of it — so the trigger index preserves it rather than
deleting it:

> 19. **img2img sends `workflow: "txt2img"` PLUS `images[]`, requires
>     `--ecosystem`, and uploads with NO credential — three things that each read
>     like a bug.** The server promotes `txt2img` + non-empty `images` to
>     `img2img:edit` itself, so sending the edit workflow would mean vendoring
>     which ecosystems offer it — item 13's prohibition exactly. `--image`
>     without `--ecosystem` is a hard error because the promotion reads the
>     ecosystem off the RAW request body, so an absent one silently skips it and
>     the flag becomes a guaranteed no-op THAT STILL CHARGES. The presigned upload
>     carries no credential, for item 17's reason. 🔴 And the CLI still cannot
>     tell you whether the promotion actually fired — do not add a detector.

---

19. **img2img sends `workflow: "txt2img"` PLUS `images[]`, requires
    `--ecosystem`, and uploads with NO credential — three things that each read
    like a bug.**

    (a) **The workflow value stays `txt2img`.** There is no `--workflow` flag and
    `generateWorkflow` is still the constant `"txt2img"` even when `--image` is
    present. The server does the promotion itself: `normalizeImageWorkflow`
    (`civitai/civitai → src/server/services/orchestrator/orchestration-new.service.ts`)
    rewrites `txt2img` + non-empty `images` to `img2img:edit` *before* graph
    validation. Sending `img2img:edit` ourselves would mean vendoring which
    ecosystems offer it (18 of them, gated by `isWorkflowAvailable`) — item 13's
    prohibition exactly. `--input` still refuses a non-`txt2img` workflow, and
    that refusal must stay.

    (b) 🔴 **`--image` REFUSES to run without `--ecosystem`, and that refusal is
    not the local validation item 13 forbids.** It checks a FLAG COMBINATION the
    CLI owns; it asserts nothing about which ecosystems exist or what any of them
    allows, and `--ecosystem` itself is passed through unexamined. It is a hard
    error because without it the flag is a **guaranteed no-op that still
    charges**: `normalizeImageWorkflow` reads `ecosystem` off the RAW request
    body, *before* the graph applies its own ecosystem default, so an absent
    ecosystem skips the promotion — and then the default ecosystem
    (`ZImageTurbo`) has no `images` node at all, so the graph engine
    (`data-graph.ts` `_evaluate` → `if (def.when === false)`) **deletes the key
    with zero diagnostics** and `_buildValidationResult` records no error.
    Measured against civitai.com: `{workflow:txt2img, images:[<real image>]}`
    with no ecosystem priced **8** with factors `{base,pixels,steps,quantity}` —
    byte-identical to the same graph carrying no images, HTTP 200 throughout.

    🔴 **And the CLI still cannot tell you whether the promotion actually
    fired.** The whatIf reply carries `{ready, cost, transactions,
    allowMatureContent}` — plus, since item 21, `modelSubstitutions` — and no
    detector — "did a `factors.images` key appear?" — is WRONG, and a
    differential estimate ("does the price change when I add the images?") is
    wrong the same way: measured, `Flux1Kontext`, `NanoBanana` and `Seedream`
    are all genuinely edit-capable and all price **byte-identically with and
    without images**, because their cost model is a flat `base`. Only `Qwen`
    grows an `images` factor. So both detectors would refuse valid img2img on the
    three most obvious edit ecosystems. **Do not add either one.** The command
    prints an explicit caveat at the confirmation instead, which is the honest
    answer: an ecosystem with no images node drops them and bills a plain
    txt2img, and nothing observable distinguishes that from a real edit job.

    (c) **The reference-image cap is ONE global ceiling (7), not a per-ecosystem
    table — and the sub-ceiling gap is deliberate, not unfinished.** Real limits
    span 1..7 and live only inside the per-engine graph files. The server's own
    helper for this (`src/shared/data-graph/generation/images-limit.ts`) says in
    its header that a flat constant "would simultaneously over-allow the 1-image
    ecosystems and under-allow the 7-image ones" and that copying the spread
    yields "a parallel table that rots the moment a graph file changes" — so it
    instantiates the real graph. The CLI cannot, and `getImagesLimit` is not
    exposed over any API, so there is no live lookup to make either. Above 7 the
    array is truncated for *every* ecosystem, so refusing there cannot block a
    valid request; below it the CLI genuinely does not know and warns instead.
    🔴 The truncation is what makes this matter: `imagesNode`'s input transform
    does `arr.slice(0, max)` **before** the output schema's `.max()` check, so an
    over-limit list can never trip the server's own "Maximum N images allowed"
    message — the extras are simply gone and the truncated job is billed.
    Measured on `Qwen` (max 3): 4, 5, 6 and 12 images all priced 60 with an
    identical `images: 1.2` factor, byte-identical to 3.

    (d) **`GraphImage.Width`/`Height` are VALUE ints with no `omitempty`, which
    contradicts item 14 on every other field.** Opposite situation: the server
    *requires* both. The images node takes them as optional on INPUT and then
    validates the transformed array against an output schema where both are
    required numbers, so omitting them is a hard 400 — measured:
    `images:[{"url":"…"}]` returns `Validation failed: images: Invalid input:
    expected number, received undefined`. A **bare URL string** fails identically
    even though the input union accepts one, because the transform turns it into
    `{url}` with no dimensions. There is no server-side dimension probe on this
    path (the only `getImageDimensions` is a browser `Image()` loader used by
    React components). So there is no "unset" state to preserve, and `omitempty`
    would additionally erase a legitimate 0.
    Dimensions come from `image.DecodeConfig` — **header only, never `Decode`**,
    which would materialise every pixel to learn two integers — and the upload's
    Content-Type comes from the DECODED format, not the filename extension.

    (e) 🔴 **The presigned upload carries NO credential, and item 17 carries the
    full argument — read it there rather than re-deriving it here.**
    `UploadPresigned` (`pkg/civitai/upload.go`) is a near-duplicate of
    `DownloadPresigned` and will attract the same "fold it back in behind a
    bool". The upload URL is **server-supplied** and lives on a `*.civitai.com`
    host (observed: `orchestration-new.civitai.com`), which
    `isTrustedDownloadHost` **matches**, so a token-carrying path would hand a
    25-scope personal API key to a request its own signature already authorizes.
    The WILDCARD is the load-bearing fact and the subdomain is incidental. Here
    the safety is structural rather than conditional: the interface has no token
    parameter and consults no `TokenSource`. `genapi.UploadImageBlob`'s two hops
    differ on purpose — hop 1 (presign) is authed, hop 2 (upload) is not — and
    the test asserts BOTH, since "hop 2 had no auth" alone cannot distinguish
    correctness from a recorder wired to nothing.
    Everything genuinely shared IS shared: `UploadPresigned` reuses
    `downloadHTTPClient()` wholesale (SSRF dial guard, redirect policy,
    `ResponseHeaderTimeout`) and the https check is the single
    `requireHTTPSTransfer` predicate the download path also calls — the verb is a
    message parameter, not a second implementation. Two details that are not
    style: the method is **POST** (`POST /v2/consumer/blobs`, not PUT), and the
    body is a `[]byte` rather than a reader so the request carries a real
    `Content-Length` and is replayable.
    One more: `getConsumerBlobUploadUrl` is **REST, not tRPC** — the one
    generation-adjacent route that is. It is a plain GET returning a bare
    `{uploadUrl, expiresAt}`, so it must never go through `unwrapTRPC`, and
    reading item 12 as "everything here is tRPC" produces a 404.

    (f) **`--dry-run` and `--print-input` DO upload local `--image` files.** An
    estimate built on a graph with no `images[]` prices a plain txt2img, and
    `--print-input` must emit a document `--input` can submit, which means real
    blob URLs rather than local paths. An upload spends no Buzz, but it is a
    network write — so `--print-input`'s "reaches no money seam" claim is still
    true while its "no request at all" claim is not, with `--image`.
