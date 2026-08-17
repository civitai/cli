# AGENTS.md item 29 — the OFFSITE refusal is a refusal, NOT a repair

Evidence for item 29 of the *Intentional decisions that look wrong* list in
[`AGENTS.md`](../../AGENTS.md). AGENTS.md carries only this item's TRIGGER —
one line naming the situations that mean you should be reading this file.
Everything below the rule is the item itself, moved here VERBATIM: the thesis,
the measurements, the retractions and the enumerated residuals, consulted when
editing the code they are about rather than on every session.

The list is append-only and never renumbered, so this file's number is stable.
Edit the body here, not in AGENTS.md; `agents_evidence_test.go` asserts the
pointer and the file agree, `agents_trigger_test.go` asserts the trigger is a
routing question rather than a label, and `agents_split_preserved_test.go` pins
the body against the text it was moved from.

## 🔴 SUPERSEDED FOR `app listing`: THE SERVER SELECTOR SHIPPED, AND SO DID THE CLIENT FALLBACK

**Read this first. The body's central claim is now FALSE for `app listing` and
still TRUE for `app status <slug>`, and treating the two as one item is the
mistake this section exists to prevent.** The body is byte-pinned against the
commit it was moved from, so it cannot be corrected in place; this is the
correction.

**What the body says.** *"No client change makes `app listing` /
`app status <slug>` reach an offsite app"*, because `getMyListingForApp` resolves
only by `appBlockId` and *"the slug selector 404s for every app, onsite controls
included"*.

**That measurement was right about the server it was taken against, and the
server changed.** `civitai/civitai#3989` widened the slug arm from
`where: { slug, kind: 'onsite', appBlockId: null, status: 'draft' }` to
`where: { slug, revisionOfId: null }`, and it is **deployed**.

**MEASURED (2026-08-17, live, credentialed, read-only, through
`client.GetMyListingForApp(ctx, "", slug)` against `https://civitai.com`):**

```
gen-matrix       BY-SLUG (onsite, approved) -> OK  apl_01KWFP4FEEJRWN27CJA49CDY4Q  approved
radio            BY-SLUG (OFFSITE)          -> OK  apl_01KYNB77D490DM6YS0C5Z7KYT7  approved
comfy            BY-SLUG (OFFSITE)          -> OK  apl_01KYNC19W1M2020K1BS2JTA4VN  approved
cosmetic-studio  BY-SLUG (OFFSITE)          -> OK  apl_01KYAZSZPSX8BNSYNTDPD4JCTP  approved
vitrine          BY-SLUG (OFFSITE)          -> OK  apl_01KWYD9DA34CYKQNC91W2T3SYN  approved
NEGATIVE CONTROL definitely-not-an-app-zzz  -> 404
```

The negative control is not decoration: a selector that resolved *everything*
would produce the same five OKs and mean nothing.

**WHAT SHIPPED IN THIS CLI (civitai/cli#422 outcome 1).** `resolveListing`
(`internal/cmd/app_listing.go`) now treats the block-submission lookup as the
ONSITE path and falls back to `GetMyListingForApp(ctx, "", slug)` when it returns
`ErrNotFound`. Every `app listing` subcommand funnels through that one helper,
and the listing-keyed procs downstream gate on OWNERSHIP rather than kind, so all
seven reach an offsite app. Verified live: `civitai app listing status --slug
radio` refused before the change and printed the listing after it.

**WHAT DID NOT CHANGE, and must not be "tidied" into agreement:**

- **`app status <slug>` is untouched and its refusal is still TRUE.** It reads
  the block-submission pipeline; an offsite app has no block submissions, and no
  listing selector repairs that. `offsiteStatusRefusal` stays as it is, and the
  two messages stay different.
- **The `app listing` refusal NARROWED, it was not deleted.** It is what a
  caller gets when the by-slug lookup ALSO answers NOT-FOUND: a Civitai without
  #3989 (older or self-hosted), or an app with no listing row. Its copy was
  rewritten — it no longer says the listing "cannot be addressed from this CLI",
  which is now a claim the code contradicts.
- 🔴 **"A listing this account does not own" is NOT one of those cases, and the
  first cut of this file said it was.** On a server carrying #3989 the by-slug
  arm RESOLVES a stranger's row and then refuses it: `getMyListingForApp` has no
  user predicate in its `where` (`{ slug, revisionOfId: null }`), and the
  ownership gate is the next statement —
  `resolveListingRole(listing.id, userId) === null → NOT_OWNED`
  (`civitai/civitai → src/server/services/blocks/offsite-listing.service.ts:1772-1774`,
  the `findFirst` at :1744-1749), mapped NOT_OWNED → FORBIDDEN by
  `app-listings.router.ts:637-654`. So it is a **403 → exit 3**, not a
  not-found, and it reaches the refusal only on a server predating #3989 whose
  narrower clause makes it 404 like an absent row. Verified from source rather
  than assumed: the refusal copy and the "downstream gates on ownership"
  argument both depend on this selector being session-scoped, and nothing in the
  widened `where` says so.
- **Only `ErrNotFound` falls through — from EITHER lookup.** A 403 from the
  invite-gated submissions route, a 5xx or a transport failure keep their own
  error and their own exit code; none of them is evidence that no submission
  exists. 🔴 The first cut applied that rule to the SUBMISSIONS lookup only: it
  wrote `if ref, slugErr := …; slugErr == nil` and DISCARDED `slugErr`, so a
  403/500/503 from the FALLBACK fell through carrying the submission's 404 and
  exited **4** under a message naming three causes, none of which was "the
  server errored". Both arms are now gated on `errors.Is(…, ErrNotFound)`.
  Measured exit codes, both lookups, end-to-end
  (`cmd/civitai/app_listing_lookup_exitcode_test.go`): 401/403 → 3, 429 → 6,
  502/503/504 → 5, and a plain **500 → 1** (no sentinel), which is why the
  published contract's "keeps its own code (`3`, `5`)" was also wrong.
- **The onsite happy path still costs exactly two requests.** The fallback sits
  on the error path, the same argument the kind probe rests on. The ERROR path
  now costs three bounded requests, not two: submissions + fallback + kind
  probe. The fallback deliberately keeps the client's own 30 s per-request
  budget rather than the probe's 5 s — for an offsite app it is the ANSWER, not
  a diagnostic, so capping it at a diagnostic's deadline would fail the repair
  on a slow-but-working server. Stated in `resolveListing`'s own comment.

**THE THREE CONSTRAINTS LISTED FURTHER DOWN ARE NOT ALL RETIRED, AND THIS
PARAGRAPH USED TO SAY THEY WERE.** It read: those constraints "applied to the
WORKAROUND route through `GET /api/v1/apps/{slug}` … the shipped repair … answers
none of those constraints because it does not need to." That is true of **1** and
**3** and **false of 2**, which is the one that can cost a user something.

- **Constraint 1 (approved-only catalog visibility) — RETIRED.** It was a
  property of the public `GET /api/v1/apps/{slug}` hop, and the shipped repair
  does not use it: `getMyListingForApp` is an authenticated owner read with no
  publication gate, so a DRAFT or PENDING offsite listing resolves too.
- **Constraint 3 (two hops through a public catalog is fragile) — RETIRED.** The
  repair is one hop against the owner route. That was the whole point.
- 🔴 **Constraint 2 (`getMyListingForEdit` performs a write) — SURVIVES, AND IS
  NOW LIVE ON THE OFFSITE PATH.** It is a property of `getMyListingForEdit`,
  **not** of how the listing id was obtained, so changing the resolver cannot
  retire it. Read the constraint as stated at the bottom of this file: "for an
  approved parent it calls `beginListingRevision` and mints a shadow revision".
  That is still exactly what happens — `offsite-listing.service.ts:1540-1541` in
  `civitai/civitai`, guarded by `if (listing.status === 'approved')`.

**So the repair MOVED constraint 2 rather than answering it.** Before #422
outcome 1, `app listing` refused for an offsite app and therefore could not write
anything; now every subcommand resolves, and `app listing status` — a command
that reads like a read — calls `getMyListingForEdit` exactly once, which on an
**approved** listing mints a shadow revision on a production listing. Every
offsite app measured on civitai.com (`radio`, `comfy`, `cosmetic-studio`,
`vitrine`) is approved, so this is the NORMAL state there, not an edge case.

**NOT HYPOTHETICAL: IT HAPPENED TWICE DURING THIS PR'S OWN DEVELOPMENT**, to
`radio` and to `gen-matrix`, each time by running `app listing status` against a
real app to check the repair worked. Tracked as **civitai/cli#389** ("is this a
read at all?"). #422 does not close it, and neither does the measurement below:
#389's own open question was whether a call that FAILS could leave a revision
behind, and reading `getMyListingForEdit` at `civitai/civitai@3ff050f` settles
that it cannot — every refusal (`loadOwnedEditableListing` at :1494, the
`revisionOfId` check at :1496, the status switch at :1502) is raised above the
`if (listing.status === 'approved') beginListingRevision(...)` at :1540, and the
proc's zod input and `appDeveloperProcedure` middleware reject before the
resolver runs at all. **That is the ERROR arm. The write on SUCCESS is exactly
as described above and is unchanged** — an approved offsite listing still gets a
shadow minted by a command that reads like a read. The README says so for
`app listing status` with and without `--json`, and the command's own `--help`
says it too.

**The rule for anyone testing this path: use the `httptest` fakes.** If you need
live shape, `civitai app view <slug>` is genuinely read-only. Do not reach for a
real slug to "just check" — that is the exact move that left the two shadows.

## 🔴 "NO CLIENT CHANGE MAKES … REACH AN OFFSITE APP" IS RETRACTED IN PART

**Read this before repeating the body's central claim, and before treating the
refusal as proof that no client path exists.** The body below is byte-pinned
against the commit it was moved from, so it cannot be edited in place; this
header is where the correction lives. The *refusal itself* still ships and is
still right for what it does — what does not survive is the absolute.

**What the body says.** *"No client change makes `app listing` /
`app status <slug>` reach an offsite app."* The support offered is that
`getMyListingForApp` resolves only by `appBlockId`, and an offsite app has none.

**The first half is measured and true. The second half is true of every offsite
app this CLI has measured, but NOT as the absolute the body states it as** — see
the section below. **And even granting both, the conclusion does not follow**,
because `getMyListingForApp` is not the only route to an `AppListing.id`.

**MEASURED (2026-08-16, live, unauthenticated, read-only):**

```
$ curl -s https://civitai.com/api/v1/apps/radio
200  {"id":"apl_01KYNB77D490DM6YS0C5Z7KYT7","slug":"radio","kind":"offsite",
      "iconUrl":"https://image.civitai.com/…","coverUrl":"https://image.civitai.com/…"}
```

That `id` is the `AppListing` row id — server-side it is `row.id` off
`dbRead.appListing.findFirst` (`projectListingDetail`,
`src/server/services/blocks/app-listing.service.ts:402` in `civitai/civitai`).
**This CLI already calls that exact route and already decodes that exact field:**
`offsiteApp` in `internal/cmd/app_offsite.go` calls `client.GetApp`, and
`civitai.AppDetail.ID` (`pkg/civitai/apps.go:136`) is the `apl_` id. So the
handle the body says cannot be obtained is *already in a variable on the refusal
path*.

**READ FROM SOURCE, NOT MEASURED** (`civitai/civitai@07a1c537dc`) — the rest of a
plausible client path:

- `getMyListingForEdit` takes a `listingId` and gates on **ownership and status
  only** — `loadOwnedEditableListing`, then a `removed`/`rejected`/shadow switch.
  There is **no kind check**.
- The asset procs (`setIcon` / `setCover` / `addScreenshot` / `removeScreenshot` /
  `reorderScreenshots` / `updateScreenshotCaption`) are listing-keyed and gate on
  `loadOwnedListing`. Again **no kind check**.
- `listingMedia` — the capability cell that is `false` for offsite — is read
  **only** by `src/components/Apps/appListingEditorTabs.ts`, a CLIENT tab gate.
  No service enforces it. (`earnings` and `submitVersion`, the two STRUCTURAL
  false cells, *are* enforced server-side. `listingMedia` is not.)

So the honest state is **"a client path probably exists for an APPROVED offsite
app, and is unverified"** — not "ruled out", which is what the body and
`claudedocs/handoff-offsite-refusal.md` both said.

**WHY IT IS STILL NOT A DROP-IN REPAIR** — three constraints any attempt must
answer, and the reason the refusal was not simply reverted:

1. `GET /api/v1/apps/{slug}` returns **approved** listings only
   (`app-listing.service.ts:672`) and is further gated by
   `resolveStoreVisibilityScope`, the `public-external` kind filter and the
   mature-rating check (:678, :688). A **draft or pending** offsite listing is
   invisible there — which is exactly the window an author wants to attach an
   icon and cover in. An owner can also be refused their own approved app when a
   visibility or maturity gate excludes it, producing a "no listing" that really
   means "not publicly visible to you here".
2. `getMyListingForEdit` **performs a write**. For an approved parent it calls
   `beginListingRevision` and mints a shadow revision (deliberately — see the
   🔴 SECURITY note at `offsite-listing.service.ts:1526`). It therefore cannot sit
   on the read-only `app listing status` path, and it is why hop 2 was NOT probed
   live: doing so would have minted a shadow on a real production listing.
3. Two hops through a **public catalog** route to reach your own private listing
   is a fragile shape to build a command on. It is a workaround for a missing
   selector, not the selector.

**THE UPSTREAM ASK: `civitai/civitai#3984`** — a slug (or app-id) selector on
`appListings.getMyListingForApp` that actually resolves offsite listings. One hop,
works pre-approval, needs no public read. The mechanism it reports, which also
answers civitai/cli#424: the slug arm **exists** but is scoped
`where: { slug, kind: 'onsite', appBlockId: null, status: 'draft' }`
(`offsite-listing.service.ts:1705-1710`), so `kind: 'offsite'` excludes every
offsite app on the first clause and an approved onsite app fails the other two.
That is why the body's *"the slug selector 404s for every app"* measurement is
correct and why it is a scoping fact rather than a missing feature.
Related upstream: `civitai/civitai#3893` (same resolver, the web Media-tab half —
it proposes an `appListingId` key, which serves the website but leaves every
non-web client on constraint 1 above).

**WHAT WOULD SETTLE IT.** One measurement: call `getMyListingForEdit` with an
offsite app's `apl_` id and see whether it returns assets or refuses on kind.
Do it against a **throwaway** offsite app, not `radio` / `comfy` / `cosmetic-studio`
/ `vitrine` — constraint 2 means the probe writes. Until then, do not restate the
body's absolute, and do not build on the workaround either.

## 🔴 "AN OFFSITE APP HAS NO `appBlockId`" IS ALSO RETRACTED — IT IS NOT A KIND DISCRIMINATOR

**Read this before writing any predicate over `appBlockId`.** The body says
`getMyListingForApp` resolves "ONLY by `appBlockId`, which an offsite app has
none of". The measurement is right; the *"which an offsite app has none of"* is a
false absolute, and the same absolute was in `internal/cmd/app_offsite.go`
("that is what `kind: offsite` MEANS") until 2026-08-16. Both were corrected
upstream the same night.

**READ FROM SOURCE** — `civitai/civitai`,
`packages/civitai-db-schema/prisma/schema.full.prisma:2849-2853`, on
`AppListing.appBlockId`:

> 1:1 backing AppBlock (UNIQUE) + idempotency key. Set for EVERY backfilled row —
> on-site AND the #2821 off-site rows (both come from an AppBlock). **It is NOT a
> kind discriminator: discriminate on `kind`, never on appBlockId nullness.**
> Only a natively-created off-site listing (no backing AppBlock) leaves it NULL.

`app-listing-backfill.service.ts:32-40` says the same thing about the writer, and
`resolveLegacyAppRedirect.ts:170-183` carries the *same* retraction, of the same
absolute, in the same repo — the sizing claim "every listing that carries an
`appBlockId` is `kind='onsite'`" was withdrawn there for exactly this reason.

**MEASURED (upstream, production, 2026-08-11)** — the `kind:'offsite'` +
non-null-`appBlockId` shape is **0 rows**
(`src/components/Apps/appListingEditorTabs.ts:104-108`, via
`resolveAccessibleAppBlockIds`), which is why the two-clause tab arms there are
each individually killable rather than redundant.

**So the honest state is three claims, not one:** the shape is **empirically zero
today**, **structurally possible**, and **a backfilled class for it exists in the
schema**. "None, by definition" is none of those.

**WHAT DOES NOT CHANGE, stated so this is not read as a reopening:**

- This CLI's own live measurement stands — 4/4 offsite apps fail the submission
  lookup, 7/7 onsite pass (#422). `kind` predicts it exactly.
- The refusal is correct behaviour and its wording is unaffected: it is derived
  from `kind`, never from `appBlockId` nullness (`offsiteApp` branches on
  `d.Kind` alone).
- All four measured apps are **natively-created** offsite listings, which
  genuinely have no backing AppBlock. The absolute is wrong; the instance is not.

**THE OPERATIONAL RULE:** discriminate on `kind`. If you ever need "does this
listing have a backing block?", ask that question directly — it is a *different*
question from "is this offsite?", and treating them as one is what both retracted
sentences did.

---

29. **The OFFSITE refusal is a refusal, NOT a repair.** No client change makes
    `app listing` / `app status <slug>` reach an offsite app:
    `getMyListingForApp` was measured to resolve ONLY by `appBlockId`, which an
    offsite app has none of — the slug selector 404s for every app, onsite
    controls included (civitai/cli#422, #424). Dropping the submission lookup
    just moves the 404 one call later. `internal/cmd/app_offsite.go` detects
    `kind: offsite` on the ERROR PATH ONLY; reaching these apps needs a
    SERVER-side selector.
