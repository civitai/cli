# Handoff: submit provenance — SHIPPED, LIVE AND VERIFIED IN PROD — 2026-08-18

## Run this first — the index, one read-only command
```bash
python3 ~/workspace/devrc/scripts/lib/subsystem_recall.py --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

The offsite thread's original goal is **met and closed** (kept below for provenance):
`civitai app listing` / `app status <slug>` were unreachable for **offsite** apps and told
their owners to run `civitai app submit` — a command that cannot succeed for an app that is
a registered URL rather than a block bundle (#422). Ship the honest refusal, and establish
whether reaching those apps is possible at all.

**`cli#411` — submit provenance — is DELIVERED and CLOSED.** `civitai app submit` recorded
nothing about where a bundle came from, so deploy-vs-source drift could be observed but never
diagnosed (five first-party apps were found behind their live version). It now stamps the commit
and the dirty flag, the server stores them, and `app status` reads them back — measured against
production, not inferred. **There is no live goal on this thread; pick from *Next steps*.**

## 🔴 READ THIS FIRST — the one-line state of #411

**CLOSED 2026-08-18T22:41Z, on a measurement against production, not on the code merging.**
Prod deployed `490b330f3b` at 22:19Z (deployment `a55fe05f3ca2`); a throwaway submit six minutes
later put `source_commit = 9fa0043f8fef741e62c3f572ab5e2e4f9014c0e0` — the local `HEAD` exactly —
and `source_dirty = f` on the row, and both `app status` surfaces read it back. The evidence is
the closing comment `civitai/cli#411#issuecomment-5335001032`; the recipe, with the values it
produced, is under *How to verify*.

**The earlier revision of this doc said "do not close #411 — the feature is inert in prod on
purpose". That was true until 22:19Z today and is now obsolete.** If you are reading a copy that
still says it, this is the correction.

## State now

- **`civitai/cli` `origin/main` @ `e7b9a9f`, base clone clean and level.** The base clone
  `/home/zach/workspace/civit/cli` is **shared and moves under you** — it went 1 behind
  mid-session twice on 08-17. `git -C … fetch && … merge --ff-only origin/main` when you own it;
  branch from `origin/main`, never from local `main`. One untracked `node_modules/` sits there,
  not mine and not ignored.
- **`civitai/civitai` base clone level with `origin/main` @ `a55fe05f3c` (`5.1.20`)** —
  `rev-list --left-right --count HEAD...origin/main` = `0 0`, re-measured 2026-08-18 22:4xZ.
  **That same sha is what production runs**, and it carries the provenance server half; the two
  being equal is timing, not a rule — re-read the deploy gate under *How to verify* rather than
  assuming `origin/main` is live. (Earlier revisions of this doc said local `main` held an
  unpushed `5.1.17` release bump belonging to another session; whoever owned it landed or dropped
  it, and nothing unpushed is stranded there.)
- **The offsite thread is CLOSED end to end.** Selector merged + deployed upstream, CLI half
  shipped, both #422 outcomes delivered, and every claim it produced that did not survive has been
  publicly retracted.
- **Merged across this thread** (08-17 above the rule, 08-18 below it):
  | what | PR | merge |
  |---|---|---|
  | four residuals: `assertListingTarget` ledger, README write-warning pin, exit-code rows, #427's false clause | cli#459 | `1695e1f` |
  | #389's measurement recorded; every "is open" surface updated | cli#462 | `de482c95` |
  | `getMyListingForApp` rate-limited 60/60 with an enforcement test | civitai#4050 | `3ff050f2` |
  | — | — | — |
  | handoff: thread moves to submit provenance | cli#469 | `cc3ce9a` |
  | **server half — provenance columns, both submit routes, read projection** | **civitai#4061** | **`490b330f3b`** |
  | **CLI half — stamp at submit, show in `app status`** | **cli#471** | **`8e51494c85`** |
  | AGENTS item 32 eviction — the ceiling had 12 bytes left | cli#472 | `c6c959354b` |
  | handoff: both halves merged, inert in prod on purpose | cli#473 | `87a147cc00` |
  | handoff: #472 merged, #4057 measured green | cli#474 | `e7b9a9f5b2` |
  | **prod deployed `490b330f3b`; provenance measured on the row; `cli#411` CLOSED** | — | — |
- **DATABASES MIGRATED BY HAND, both verified** (civitai migrations are hand-applied — no
  `prisma migrate deploy`): dev clone `cnpg-database-dev/cnpg-cluster-dev-1` and prod
  `cnpg-database/cnpg-cluster-nvme0-5`. Both show `source_commit text YES` / `source_dirty
  boolean YES`, no default, 23→25 columns; prod replicated to both replicas; re-apply is a clean
  no-op. **Prod was migrated BEFORE the code merged, deliberately** — see the RETURNING note.
- **`AGENTS.md` is 28,489 bytes against `agentsMaxBytes = 28_758` — 269 bytes of headroom**,
  re-measured 2026-08-18 23:0xZ at `e7be70f`. **Not the 145 an earlier revision of this doc
  recorded**: another session evicted item 31 to its evidence file and added `internal/credscan`
  (#470) in between. Items 2, 4 and 30 remain inline; only 30 has a base commit. 🔴 **Re-measure
  before quoting this** — the file moves under you from threads that have nothing to do with
  yours, in both directions.
- **Nothing of mine is in flight.** All four worktrees removed (the `civitai` one needed
  `worktree remove --force` — it holds the `event-engine-common` submodule — done only after
  confirming it was byte-clean and its work was in `origin/main`). Open cli PR `#470` and the
  open `civitai` PRs under this account belong to other sessions.
- **Issues closed:** `cli#422`, `cli#389` (outcome A, measured), `civitai#3984`, `civitai#4003`,
  `civitai#4008`, **`civitai#4059`** (delivered by #4061).
- **Issues open on purpose:** `cli#424`, `cli#427` (both narrowed), `civitai#3893`.
  **`cli#411` and `civitai#4057` are both CLOSED** — see the top of this doc, and #4057's block
  below.
- 🔴 **The prod-deployed revision IS readable, and the previous revision of this doc said it was
  not.** The GitHub deployments API on `civitai/civitai` carries a `do-prod` environment whose
  successful statuses name `https://civitai.com`. Command + the trap under *How to verify*. This
  retires the "inferred = the deploy gap" caveat below and, with it, the malformed-`sourceCommit`
  probe that was being saved to settle it — no submit needs to be spent on that question again.

## The live proof — round 1 (22:0x, NULL) and round 2 (22:38, CLOSED)

**Round 2 is the one that closed the issue**; round 1 is kept because it is what proved the
client half correct while the server was still undeployed, and because the pair is the control:
the same recipe returned NULL and then the sha, with only the prod deployment changing between.

### Round 2 — 2026-08-18 22:38Z, against prod running `a55fe05f3ca2`

```
local HEAD:  9fa0043f8fef741e62c3f572ab5e2e4f9014c0e0   (tree clean, branch `probe`, no remote)
row:         source_commit = 9fa0043f8fef741e62c3f572ab5e2e4f9014c0e0 · source_dirty = f
app status:  Source commit: 9fa0043f8fef741e62c3f572ab5e2e4f9014c0e0 (reported clean)
--json:      {"sourceCommit": "9fa0043f…", "sourceDirty": false}
```

- **`source_dirty` is `f`, not NULL** — the tri-state survives client → wire → column intact.
- **Count control:** prod went from **0 of 145** rows carrying provenance to **1 of 146**. The
  number moved by exactly one and it was mine, which is what makes the earlier "0 of 145" a
  reading rather than a query wired to nothing.
- **The no-remote warning fired correctly:** *"the work tree is clean, but HEAD is on no remote —
  this bundle's source exists only on this machine."*
- **Cleanup:** `pubreq_01M0BGE04Z5YWMWNK1V694ETP9` withdrawn, `status: withdrawn` confirmed on the
  row; provenance survives the withdrawal; nothing left in the moderator queue.
- **Residual, stated rather than counted as coverage:** the `source_dirty = true` path was never
  exercised live — one submit was spent, from a clean tree. Unit tests on both halves only.

### Round 1 — earlier the same day, against prod running `60d8087647cf` (5.1.18)

Ran the real thing end to end: scaffolded a throwaway, committed it, `civitai app submit --yes`
against production, then read the row.

- **Production row: `source_commit` NULL.** Across all of prod, **0 of 145 rows** carry
  provenance.
- **The CLI is NOT at fault.** Pointing the shipped binary at a capture server (the step that
  distinguishes "client didn't send" from "server didn't store" — an empty result cannot):
  ```json
  {"path": "/api/v1/blocks/submit-version",
   "keys": ["bundleBase64", "sourceCommit", "sourceDirty"],
   "sourceCommit": "8d80b901e00961df3ddeccf020cd07d13db60d5e",
   "sourceDirty": false, "bodyBytes": 12209}
  ```
  Correct route, true HEAD sha, `sourceDirty: false` rather than `null` — the tri-state survives
  the client. `bodyBytes` matches the CLI's own printed submit-body size exactly, which
  independently validates the `SubmitBodySize(zipLen, prov)` signature change.
- **Diagnosis:** production had not deployed `490b330f3b`. Recorded then as **measured** = the
  two facts above, **inferred** = the deploy gap. 🔴 **The inference was right and is now
  measured**: the last successful `do-prod` deployment at that moment was `60d8087647cf` (5.1.18,
  2026-08-17 22:47Z), which does not contain `490b330f3b`. The malformed-`sourceCommit` probe
  that was being held in reserve to settle this is **no longer needed** — the deployments API
  answers it for free.
- **The ordering is safe, and this was verified not assumed:** the real prod submit returned
  exit 0 and created a real row, because the old handler drops unknown keys silently.
- **Cleanup done:** `pubreq_01M0B6ZFKM8EB0R8G3BS22RBSQ` withdrawn (`status: withdrawn` confirmed in
  the DB), nothing left in the moderator queue.

## `civitai/civitai#4061` — the server half (MERGED `490b330f3b`)

Two nullable columns on `app_block_publish_requests`, accepted at submit and returned on read:
`source_commit` (40-hex, lowercase) and `source_dirty` (boolean).

- 🔴 **THE WRITE FAILS UNCONDITIONALLY AGAINST AN UNMIGRATED TABLE, VIA `RETURNING` — NOT JUST
  THE READ.** The migration comment originally claimed the write break was "SECONDARY and
  CONDITIONAL — only a submit that actually CARRIES provenance names a column the table lacks".
  **That was false, in the reassuring direction.** Prisma does omit an `undefined` field from the
  INSERT column list, but `submitVersion`'s `create` passes **no `select`**, so Prisma reads the
  row back — `INSERT … RETURNING <every scalar in the model>` — and every submit raises P2022,
  including from clients that send no provenance. Observed, not theorised: it took down the
  preview smoke suite (`preview / smoke-tests`, 1 failure, the only smoke spec that submits) on
  two successive commits until the dev clone was migrated, while a control PR passed 65/65.
  Corrected in the migration header rather than deleted.
- **🔴 THE SCOPING BRIEF NAMED THE WRONG ROUTE, AND THAT NEARLY SHIPPED AN INERT FEATURE.** There
  are TWO submit front doors: `src/pages/api/blocks/submit-version.ts` (cookie / mod browser) and
  `src/pages/api/v1/blocks/submit-version.ts` (bearer token). **The CLI posts to the v1 one** —
  `internal/appapi/appblocks.go:281`, `DefaultSubmitPath = "/api/v1/blocks/submit-version"`. Wiring
  only the first would have left the actual client's provenance silently stripped: the exact
  inert-feature failure #4059 exists to close, reproduced by the fix for it. **Both are wired and
  both are pinned.** Generalise: *when a feature is "the server accepting a field", enumerate every
  front door that parses that schema before writing a line.*
- **Design decisions, all stated in the source:** client-claimed and never server-verified; NOT
  `forgejo_commit_sha` (a server-side sha in the platform's own repo, written on approve — never
  aliased or fallen back between); both fields optional so this is never a submit gate; a malformed
  `sourceCommit` **400s rather than being dropped**; and `NULL != false` — `source_dirty` is
  tri-state (null = unknown, false = client asserted clean, true = client asserted dirty), with no
  backfill and no DEFAULT, because either would turn "nobody looked" into "someone looked and it
  was clean". `recordPendingFromPush` deliberately leaves both NULL.
- **Verified independently of the implementing agent** (all re-run by hand, not taken on its word):
  `pnpm typecheck` → 0 errors · 6 test files → **255 passed** · the **anti-strip mutant** (delete
  `sourceCommit` from the schema) → **killed by 14 tests across 3 files**, which is what proves the
  feature is not inert · the worktree restored byte-identical from a `cp -a` copy afterwards.
- **CI: every test gate SUCCESS** (Unit / Package unit / App unit / tekton typecheck / ESLint +
  Prettier / Schema drift / event-engine-common pin; one SKIPPED). `preview / deploy` stayed
  PENDING; `mergeStateStatus: UNSTABLE` is that alone.
- **Residual:** mutant (f) — dropping `sourceCommit` from the read `SELECT` — has only a structural
  kill. The read suite mocks `findMany`, so no behavioural instrument exists at that layer. Stated
  in the PR body rather than counted as coverage. 7 of 40 new assertions are **invariant guards**
  (green at base by construction), listed as such; 33 were red at base.
- **`preview / component-tests` never appeared in the rollup** — it presumably runs after
  `preview / deploy`. So this PR neither clears nor contradicts `civitai#4057`.

**Next:** merge #4061 → apply the migration → *then* build `cli#411`'s stamping half.

## Open investigations — live diagnosis state

### #424 — does the `slug` selector on `appListings.getMyListingForApp` EVER resolve?

- **Symptom + exact repro:** `internal/cmd/app_listing.go:186-192` carries a 🔴 comment
  asserting a pre-approval draft listing has `appBlockId = NULL` and is *"resolvable ONLY
  BY SLUG"*, with the slug argument *"load-bearing when it is absent"*. Live, the slug
  selector resolves nothing. Reproduce with a Go test in `internal/cmd` calling
  `appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")` then
  `client.GetMyListingForApp(ctx, "", "<slug>")`.

- **Observed (with values)** — as `zachlowdenzx` against `https://civitai.com`, varying
  only the selector:

  | app | `appBlockId` only | `slug` only | both |
  |---|---|---|---|
  | `gen-matrix` (onsite, approved+live) | ✅ `apl_01KWFP4FEEJRWN27CJA49CDY4Q` approved | ❌ 404 | ✅ same id |
  | `custom-generators` (onsite, approved+live) | ✅ `apl_01KXKAJF85NQ8X702AHKX2MMAW` approved | ❌ 404 | ✅ same id |
  | `etag-probe-05773` (`appBlockId = nil`) | n/a | ❌ 404 | n/a |
  | `dogfood-test-palette` (`appBlockId = nil`) | n/a | ❌ 404 | n/a |
  | `radio`, `comfy` (offsite) | n/a — none exists | ❌ 404 | n/a |
  | `definitely-not-an-app-zzz` | n/a | ❌ 404 | n/a |

  The 404 body is `no store listing found for this app (404): no listing found for app
  <slug> — a store listing is created when you run 'civitai app submit'…`.
  **The `appBlockId` does 100% of the resolution work.**

- **Ruled out:**
  - *"The probe is malformed"* — the `appBlockId`-only calls return real listing ids in the
    **same run**, so auth, tRPC `{"json":…}` envelope encoding and route path are all correct.
    (#422's original probe was discarded for exactly this reason: it 404'd on its own
    positive control. My own first round did too — see *Gotchas*.)
  - *"Only nonexistent slugs 404"* — `gen-matrix` and `custom-generators` are approved and
    live, and 404 on slug-only.
  - *"The tests prove the server supports it"* — they do not.
    `internal/appapi/listing_test.go:69` and `internal/cmd/app_listing_test.go:~888` are
    `httptest` fakes returning `{"appListingId":"apl_draft",…}` **unconditionally**. They
    pin the wire shape the CLI *sends*, never what the server *answers*.

- ✅ **Hypothesis CONFIRMED from server source** (`civitai/civitai@07a1c537dc`,
  `offsite-listing.service.ts:1705-1710`): the slug arm is real but scoped
  `where: { slug, kind: 'onsite', appBlockId: null, status: 'draft' }`. Every row in the
  table above is accounted for — the two approved onsite apps carry an `appBlockId` and fail
  clauses 2 and 3; the offsite apps fail clause 1. So the comment is neither right nor
  wrong: it is **over-general**. Suggested edit in the #424 thread — keep the slug argument
  and both tests, narrow the prose to name the three conditions.

- **🔴 The confound I could NOT eliminate:** the only two `appBlockId = nil` submissions I
  could find (`etag-probe-05773`, `dogfood-test-palette`) are **`withdrawn`**, not
  **`pending`**. A withdrawn submission may have had its listing removed server-side, which
  would explain their 404 without the pending path being broken. So the honest state is
  *"unverified and contradicted by every app reachable from this account"*, **not**
  *"proven broken"*.

- **Next probe** (settles it in one measurement): scaffold a throwaway app, `civitai app
  submit` it, and **before it is reviewed** call `GetMyListingForApp(ctx, "", "<slug>")`.
  If that resolves, the comment is right and only approved listings are slug-unreachable.
  If it 404s, the pre-approval media flow has never worked from the CLI and #422's
  "no CLI path to attach media" is the general case, not offsite-only.

### #422 outcome 1 — reaching offsite apps at all

- **Symptom:** offsite apps have no store-listing management path from the CLI. All four on
  this account (`radio`, `comfy`, `cosmetic-studio`, `vitrine`) serve a live icon + cover.
- **Observed:** `getMyListingForApp` is addressable **only** by `appBlockId`; none of the
  four offsite apps measured here has one. Dropping the submission lookup and falling
  through with an empty `appBlockId` just moves the 404 one call later.
- 🔴 **"— that is what `kind: offsite` means" is RETRACTED (2026-08-16).** `appBlockId` is
  **not** a kind discriminator. `civitai/civitai`,
  `packages/civitai-db-schema/prisma/schema.full.prisma:2849-2853`: it is "Set for EVERY
  backfilled row — on-site AND the #2821 off-site rows (both come from an AppBlock) … Only
  a natively-created off-site listing (no backing AppBlock) leaves it NULL." The
  `kind:'offsite'` + non-null `appBlockId` shape is **0 rows in production** (measured
  2026-08-11, `src/components/Apps/appListingEditorTabs.ts`). So: empirically zero today,
  structurally possible, and a backfilled class exists in the schema. The observation above
  and the refusal are unaffected — all four apps here are natively-created — but
  **discriminate on `kind`, never on `appBlockId` nullness.** Full account:
  `claudedocs/decisions/29-offsite-refusal.md`.
- 🔴 **"Ruled out: any client-side fix" is RETRACTED (2026-08-16).** It was true of
  `getMyListingForApp` and false as a claim about the CLI, because that proc is not the only
  route to an `AppListing.id`. **Measured, live, unauthenticated:**
  `GET /api/v1/apps/radio` → `200 {"id":"apl_01KYNB77D490DM6YS0C5Z7KYT7","kind":"offsite",…}`.
  That is the listing row id, and **this CLI already decodes it** —
  `civitai.AppDetail.ID`, off the call `offsiteApp` already makes on the refusal path.
  Server-side (`civitai/civitai@07a1c537dc`, read not measured) `getMyListingForEdit` and
  every listing-keyed asset proc gate on **ownership only, no kind check**, and the
  `listingMedia:false` capability cell is read solely by a client-side tab gate.
  So: **a client path probably exists for an APPROVED offsite app, and is unverified.**
  Not a drop-in — `/api/v1/apps/{slug}` is approved-only and scope-gated (a draft or pending
  offsite listing is invisible to it), and `getMyListingForEdit` **writes** (mints a shadow
  revision), so it cannot sit on the read-only `app listing status` path. Full account,
  with the caveats and the probe that would settle it:
  `claudedocs/decisions/29-offsite-refusal.md`.
- **Two vendored comments say the row exists** (claims, not measurements):
  `pkg/civitai/apps.go:10-13` — `/api/v1/apps` serves BOTH kinds *"from the durable
  AppListing record"*; `internal/appapi/listing.go:351` — `persistListingAssetImage` is
  documented against **`offsite-listing.service.ts`**. The `id` measurement above turns the
  first of these from a claim into a confirmed one.
- **DONE — the upstream issue is filed: `civitai/civitai#3984`**, asking for a slug/app-id
  selector on `appListings.getMyListingForApp`. It also reports the mechanism that answers
  #424: the slug arm exists but is scoped
  `where: { slug, kind: 'onsite', appBlockId: null, status: 'draft' }`
  (`offsite-listing.service.ts:1705-1710`), so `kind: 'offsite'` excludes every offsite app
  on the first clause and an approved onsite app fails the other two. Cross-linked from
  #422, #424 and `civitai/civitai#3893`.

### Is the #3989 slug selector deployed to production? (the only thing gating the CLI half)

- **Symptom:** nothing is broken; this is a readiness question. `civitai app listing set-icon
  --slug radio` cannot work until the merged server selector is live.
- **Observed (2026-08-17 04:35Z, `https://civitai.com`, slug-only `getMyListingForApp`):**
  `gen-matrix` → 404 · `radio` → 404 · `comfy` → 404 · `definitely-not-an-app-zzz` → 404.
  **Positive control, same session/auth/base URL:** `app listing status --slug gen-matrix`
  → resolves, exit 0.
- **Ruled out:** *"the probe is malformed"* — the appBlockId path resolved a real listing in the
  same run. *"the CLI refusal proves anything"* — it does NOT: `app listing status --slug radio`
  prints the refusal whether or not the selector is deployed, because the CLI short-circuits at
  the submissions lookup and never calls the route. That first probe was worthless and is the
  empty-result trap.
- **Leading hypothesis:** merged 04:20Z, probed 04:35Z — production simply has not rolled yet.
- **Next probe:** re-run *How to verify* step 1. A single resolve on `radio` flips this.

### `civitai/cli#424` — does the slug arm serve a genuinely PENDING first-version onsite app?

- **Resolved from server source** (`civitai/civitai@07a1c537dc`): the old arm was
  `where: { slug, kind:'onsite', appBlockId:null, status:'draft' }`, which accounts for every row
  in #424's measured table — approved onsite apps carry an `appBlockId` and fail clauses 2-3;
  offsite fails clause 1. So the `app_listing.go:186-192` comment is **over-general, not wrong**.
- **Still unmeasured:** the pending-onsite case itself. #424's two `appBlockId = nil` submissions
  were **withdrawn**, and the clause keys on the *listing's* status, not the submission's.
- **Next probe:** submit a throwaway app; before review, `GetMyListingForApp(ctx, "", "<slug>")`.
- **Note:** #3989 widened past this shape entirely, so the answer now changes nothing operationally.

### `civitai#4057` — `preview / component-tests` red on `main`, ImageCard overrun tolerance exceeded

- **Symptom:** `preview / component-tests` fails on every `main`-based branch. **Report-only, not
  blocking**, which is why it went unnoticed.
- **Observed (verbatim, from the Tekton run for `pr-preview-4050-5f657`):**
  ```
  FAIL  component (chromium)
    src/components/Cards/ImageCard.browser.test.tsx
      > ImageCard action row at four-digit reaction counts
      > still overruns the narrow column, by no more than it does today
  AssertionError: expected 57 to be less than or equal to 50
    ❯ src/components/Cards/ImageCard.browser.test.tsx:173:23
  ```
  **1 failed of 1670** (157 files, 156 green). A failure screenshot is emitted at
  `src/components/Cards/__screenshots__/ImageCard.browser.test.tsx/ImageCard-action-row-…-1.png`.
- **What the test is:** a **characterization test with a tolerance** — the name says *"still
  overruns … by no more than it does today"*, and line 172 asserts the three-digit case does **not**
  overrun. It pins a known-bad layout at a 50px budget; the overrun is now 57px.
- **Window (measured, REST statuses API):** #4004 ✅ 19:34Z → **#4043 ❌ 20:49Z** → #4050 ❌ 21:26Z.
  Green earlier the same day on #3986 (00:38Z) and #3988 (04:46Z).
- **Ruled out:** *"it is #4050's rate limit"* — #4050 has no UI change and inherits the failure from
  its base. *"the whole suite is broken"* — 1669 of 1670 pass. *"a flake"* — not established either
  way; the assertion is a measured pixel budget, not a timing one.
- **NOT established:** the cause. #4043 (`4288b08efc`, a notification-db dev-fixture fix) is only the
  **earliest failing run**, and has no plausible path to card layout. **Do not attribute it.**
- **Next probe:** open the emitted screenshot, then
  `git -C /home/zach/workspace/civit/civitai log --since="2026-08-17T19:34Z" --until="2026-08-17T20:49Z" -- src/components/Cards/`
  and the shared card CSS. If nothing in that window touches the card, widen the bisect.

### `civitai#4057` — RESOLVED: fixed by `civitai#4052`, not intermittent (CLOSED 2026-08-18)

- **The previous revision's leading hypothesis — "either something in the window changed the
  layout back, or the check is intermittent" — is REFUTED.** The fix is `cd6d1e9041` (#4052,
  merged 2026-08-17 21:35Z), which replaced the bare literal with a documented constant:
  `expect(fourDigits).toBeLessThanOrEqual(MAX_NARROW_OVERFLOW_PX)`, **64**, up from 50.
- **The mechanism the issue was missing, stated in #4052's own comment:** `reactionBarOverflow()`
  is a raw pixel distance driven by the WIDTH OF RENDERED TEXT, so unlike the height assertions
  beside it, it moves with font metrics and therefore with the environment — 50px measured off-CI
  when #4034 shipped it, 57px in the CI browser. It shipped red, and `preview / component-tests`
  being report-only is why nothing said so.
- 🔴 **What killed the intermittency theory: the outcome sorts by ANCESTRY, not by the clock.**

  | PR | run | carries `cd6d1e9041`? | result |
  |---|---|---|---|
  | #4056 | 08-17 22:12Z | **no** | ❌ |
  | #4061 / #4075 / #4086 / #4082 | 08-18 18:53–21:23Z | yes | ✅ |

  #4056 is the run that *looked* like intermittency — it failed AFTER the fix merged, because its
  branch was cut before it. **A "red after the fix" is a claim about the branch's base, not about
  the fix.** Check ancestry before reaching for flakiness.
- **The spec still RUNS, not skipped** — the thing a green could otherwise be hiding:
  `✓ component (chromium) src/components/Cards/ImageCard.browser.test.tsx (6 tests) 418ms`.
- **NOT fixed, on purpose:** #4052 raised the bound, it did not close the overrun. The
  characterization test still pins a known-bad layout, now at 64px, deliberately above the largest
  observed value — a real regression moves it by tens of px, a font-stack change by a few.
- Closed with the evidence: `civitai/civitai#4057#issuecomment-5335314159`.

### NEW, unrelated: a `MySubmissionsList` component flake (no issue filed yet)

`preview / component-tests` failed on **#4087** at 20:50Z, which is what made #4057 look
unresolved on first read. Different test, unrelated cause:

```
FAIL  src/components/Apps/MySubmissionsList.browser.test.tsx
  > the History button opens the moderation timeline (actions + verbatim reasons)
VitestBrowserElementError: Cannot find element with locator: getByText('Reported for policy')
```

Reads as a flake: #4087 is comment-only and does not touch that component, the same 39-test file
passed on #4086, #4082 and #4075 either side of it, and **the file took 21.4s failing against
6.0s green — a ~3.6× inflation confined to ONE file while the whole run moved only 1.26×.** That
is a locator waiting out its timeout, not load spread across the suite.

🔴 **How to read these runs at all** (the previous revision had no route to the failing test name —
the GitHub status only says "Component suite failed"): the Tekton pipelines run in the DataPacket
prod cluster and the logs are readable directly.

```bash
export KUBECONFIG=$KC_DPPROD
kubectl get pipelineruns -n tekton-builds --sort-by=.metadata.creationTimestamp | grep <pr-number>
kubectl logs -n tekton-builds pr-preview-<pr>-<suffix>-component-tests-pod --all-containers \
  | sed -e 's/\x1b\[[0-9;]*m//g' | sed -n '/Failed Tests/,/Test Files/p'
```

The taskrun shows `Succeeded` even when the suite fails — it is a report-only suite, so the task's
own status says nothing about the tests. **Read the pod log, not the taskrun condition.**

## Next steps (ranked)

**Nothing on this thread is blocked or in flight.** `cli#411` closed the provenance work; what
follows is the leftover list, and none of it is urgent.

🔴 **Retired from this list, having been carried on it while already done:** *"`AGENTS.md` item
29's trigger still describes a blanket refusal"*. #453 (`fbefd64`) narrowed it at the time it
shipped; the trigger reads *"the by-slug fallback, the narrowed refusal behind it, or `app
status`'s?"* today. It survived several revisions of this doc — including one merged 20 minutes
before it was caught — because each revision re-ranked the list without re-reading the file it
names. **Verify a next-step against the tree before carrying it forward; a list item is a
hypothesis with a timestamp, not a fact.**

1. **`civitai#3893`** — rescoped to four touch points, no proc re-key. `ListingMediaEditor` uses
   `appBlockId` in exactly 4 places, all query-key/invalidation; the slug is already in hand at the
   page. The real blocker is `editorTabsFor`'s `appBlockId != null`, not the resolver.
2. **`AGENTS.md` eviction wave** for item 30 when someone needs the room — 269 bytes is still
   about one ordinary edit away from failing the ceiling.
3. **`internal/devtunnel` flake** — `TestSSHDialerProxyLocalHostUnreachableNamesHost` failed once
   under full-suite load, 8 clean reruns. Remove the timing dependency; do not re-run it away.
4. **Shadow drafts on `radio` and `gen-matrix`** — still no server-side discard path.
5. **`source_dirty = true` has no live proof** — if a submit is being spent anyway, spend it from a
   dirty tree and read the row; today's round 2 only exercised the clean case.
6. **File the `MySubmissionsList` flake** if it recurs — one occurrence, diagnosed above, no issue
   opened for it yet.

## Gotchas / decisions / dead-ends

### From closing #411 (2026-08-18, evening) — new

- 🔴 **A FAILED DEPLOY LEAVES A DEPLOYMENT RECORD THAT NAMES THE SHA IT TRIED TO SHIP.** Reading
  "the latest deployment" therefore reports a feature live at the moment its deploy *started*, not
  when it landed — here that was a 2-hour window in which the naive read was confidently wrong, and
  the deploy in it never succeeded at all. Filter on a `success` status, and treat the
  `environment_url` on that status as the only thing tying the environment name to production.
- **A shared-tree guard can judge the WRONG REPO when the tree it should judge does not exist
  yet.** `git init "$D" && git -C "$D" commit …` in one compound command was blocked as a commit to
  `main` — of the `cli` clone, because `$D` was not a worktree at evaluation time. The guard said so
  in its last paragraph. **Split creation from use across Bash calls** rather than reading the
  refusal as being about the repo you were thinking of.
- **The count control is what turns "0 rows" into a reading.** Prod held 0-of-145 provenance rows
  before and 1-of-146 after, and the 1 was mine. Without the second number the 0 is
  indistinguishable from a query pointed at the wrong table.

### From the provenance build (2026-08-18) — new

- 🔴 **A ZERO FROM A TOOL RUN FROM THE WRONG DIRECTORY LOOKS EXACTLY LIKE A CLEAN RUN.**
  `golangci-lint run <abs-path>/...` from outside the module printed **`0 issues.`** *together
  with* a `typechecking error: directory prefix … does not contain main module`. Re-run from
  inside the module: `0 issues.` and no error. Same reassuring number, one of them meaningless.
  Read the whole output, not the count — and use the two runs against each other as the control.
- 🔴 **`preview / smoke-tests` LIVES IN COMMIT STATUSES, NOT THE CHECK ROLLUP, AND IS CREATED
  ONLY AFTER `preview / deploy` FINISHES.** I read `gh pr view --json statusCheckRollup` while
  deploy was still pending, saw everything terminal and green, and reported "every test gate
  green". It was false — smoke-tests failed and did not yet exist to be seen. **Poll
  `gh api repos/…/commits/<sha>/status` too, and require the status you care about to be
  PRESENT, not merely for everything present to be terminal.** The check SET also grows mid-poll
  (8 → 9 → 15 here), so a minimum-count assertion made at t=0 is a moving target.
- 🔴 **`make ci` TELLS YOU WHEN IT IS NOT ENOUGH — READ ITS TAIL.** It prints "this is a FULL
  clone. CI checks out at depth 1, where tests that read git history cannot resolve their base
  blobs. Before you push a change that touches those, run: `make ci-shallow`." Any `splitItems`
  work touches exactly those. `ci-shallow` reads **committed** state, so commit first.
- **`build-image` flakes, and the honest retry is an amended commit with an IDENTICAL TREE.**
  `preview / deploy` failed at `build-image` on a comment-only `.sql` delta from a fully-green
  commit. `git commit --amend --no-edit` + `--force-with-lease` gives a new SHA with the same tree
  (verify: tree hash unchanged), so the retry is a real experiment rather than a hope. It went
  green. **Do NOT use the `preview-db/prod` label to retrigger** — that repoints the preview at
  the PRODUCTION database.
- 🔴 **The stash stack in these repos is NON-EMPTY and holds other sessions' work** (three
  entries: `fix/cleanup-s3-on-model-file-delete` ×2, `feat/deploy-status-skill`). Live proof the
  no-stash rule is not theoretical. `cp -a` to a **per-agent** path; there is a stale
  `offsitemod.orig.ts` (2026-08-11, zero provenance content) sitting at bare `/tmp/claude-1000/`
  that looks exactly like a backup and is not.
- **An agent cut off mid red/green leaves the tree looking "partially done" when it is REVERTED.**
  One died having checked a source file back out to base for a red check; the test file survived
  as modified, the source change was gone, and nothing said so. **Re-derive the tree state before
  resuming an interrupted agent** — and do not hand it your inference as fact: I told one its red
  result "stood" when it had never run it, and it correctly re-ran rather than accept my number.
- **IDE diagnostics from a worktree are mostly gopls noise.** `undefined: Client`, `use of
  internal package not allowed`, "not included in your workspace" — all false; `go build ./...`
  returned rc=0. Check with the real toolchain before reporting a diagnostic as a finding. But
  note the converse bit too: a genuinely broken `_test.go` (`not enough arguments in call to New`)
  made `internal/appapi` **fail to build so none of its tests ran**, including nine pre-existing
  files. Read per-package lines.
- **The `civitai/civitai` worktree recipe from this harness** — `cd` is blocked and the toolchain
  is direnv-only, so every command needs `direnv exec <wt> …`; `pnpm`/`node` are not otherwise on
  PATH. There is **no `wt add`** (only `wt stale|rm`). Setup:
  `git worktree add` → `git submodule update --init event-engine-common` →
  `printf 'use flake\n' > <wt>/.envrc && direnv allow <wt>` → `direnv exec <wt> pnpm install`
  (~13s warm). 🔴 **Do NOT copy the base `.envrc`** — it carries live S3 credentials and a
  `DATABASE_URL` the task does not need.
- **`civitai/civitai` `main` has NO required status checks** (measured: `contexts: null`,
  `checks: []`, only a `deletion` ruleset). Green CI there is advisory; local verification is the
  load-bearing evidence.
- **Hand-applying a civitai migration** (they are never auto-applied):
  `KUBECONFIG=$KC_DPPROD kubectl exec -i -n <ns> <primary-pod> -c postgres -- psql -U postgres -d civitai -v ON_ERROR_STOP=1 < <migration.sql>`.
  dev = `cnpg-database-dev` / `cnpg-cluster-dev-1`; prod = `cnpg-database` / `cnpg-cluster-nvme0-5`.
  🔴 **Confirm the PRIMARY two ways** (`cnpg.io/instanceRole` label AND
  `.status.currentPrimary`) before writing DDL, and check no OTHER cluster carries the table
  (`cnpg-cluster-3` does not; `cnpg-cluster-apps-1` has no `civitai` db) so the apply is complete
  rather than partial.
- **A throwaway app for a live probe:** `civitai app create`, `git init -b <not-main>` (a fresh
  `main` trips the never-commit-to-main guard), stage explicit paths, commit, `app submit --yes`
  (it refuses non-interactively without `--yes`), then **`civitai app withdraw <pubreq> --yes`** so
  nothing sits in the moderator queue.

### Working in `civitai/civitai` from this harness (new — 2026-08-17)

- 🔴 **`cd` is BLOCKED by this harness, and the monorepo's toolchain is direnv-only.** `pnpm`,
  `node` and `npx` are NOT on PATH; they come from the flake. Every command must be wrapped:
  `direnv exec /path/to/worktree <cmd>`. A bare `pnpm --version` returns *"command not found"*,
  which reads like a broken worktree and is not.
- **Worktree setup recipe that worked, in full** (the repo's own `wt` subcommand has only
  `stale` and `rm` — there is **no `wt add`**):
  ```bash
  git -C /home/zach/workspace/civit/civitai worktree add <wt> -b <branch> origin/main
  git -C <wt> submodule update --init event-engine-common
  printf 'use flake\n' > <wt>/.envrc && direnv allow <wt>
  direnv exec <wt> pnpm install --prefer-offline      # 13s warm
  ```
  🔴 **Do NOT copy the base clone's `.envrc` verbatim** — it carries live S3 credentials and a
  `DATABASE_URL` the task does not need. `use flake` alone is enough for typecheck/test.
  Node is v22 against `engines: >=24`; it warns and works.
- **`civitai/civitai` `main` has NO required status checks.** Measured, not assumed:
  `repos/civitai/civitai/branches/main/protection` → `contexts: null, checks: []`, and the only
  ruleset on `main` is `deletion`. **Green CI there is advisory** — local verification is the
  load-bearing evidence, and the AGENTS.md instruction to re-measure before calling any job a gate
  applies to this repo too.

### Instrument traps hit again this session

- 🔴 **A vitest run whose path filter matches nothing EXITS 0 AND IS OMITTED SILENTLY.** I passed
  6 test paths and got `Test Files 6 passed`… no: `5 passed (5)`, because one path guessed a
  `__tests__/` directory that did not exist. **Nothing named the missing file.** Caught only by
  comparing the file COUNT to the number of paths passed. Always assert the count equals what you
  asked for — `Tests N passed` alone cannot see an absent file.
- 🔴 **The empty-conclusion CI trap fired exactly as documented, again.** `Unit tests` sat with a
  **blank** conclusion (not `PENDING`) for ~14 minutes before resolving SUCCESS. A `PENDING`-only
  poll declares ALL SETTLED against it. The check count also GREW mid-poll (8 → 9, `tekton /
  typecheck` appeared late), so a minimum-count assertion made at t=0 is itself a moving target —
  require a terminal conclusion on **every** row and re-read the row set each poll.
- 🔴 **A subagent's self-reported gate results were accurate here — and re-running them still paid
  for itself.** The re-run is what surfaced the 5-of-6 filter miss (mine, not the agent's) and
  independently confirmed the anti-strip mutant. Mid-run IDE diagnostics showed a dozen type errors
  and a stray `mutate-4059.py`; both were **mutation-battery artifacts**, and `git status` on the
  committed worktree was clean. **Check the tree state before believing a diagnostic snapshot taken
  during a mutation run.**

### Scoping

- 🔴 **"The server accepts a new field" is a question about EVERY front door that parses the
  schema, not about the schema.** Two routes parse `submitVersionSchema`; the CLI uses the one the
  brief did not name. Enumerate the parse sites (`git grep <schemaName>`) before writing a line —
  the failure mode of missing one is a feature that is inert on exactly the path that matters,
  with a full green suite.

- 🔴 **A probe that fires identically on its own positive control attributes nothing.**
  This bit three separate attempts on the same question — #422's author, and my first two
  rounds. The fix that worked: **vary the selector, not the app**, and report the pair.
- 🔴 **My NBSP fixture was contaminated.** Checking whether the URL guard rejected U+00A0,
  my first fixture also contained ASCII spaces, so the ASCII branch killed it first and
  reported a false *"rejected"*. Re-run with `\u`-escaped fixtures it rendered. **Write
  fixtures whose only distinguishing feature is the thing under test.**
- **`make ci` is not CI** — it omits lint. `golangci-lint` is not on this host's PATH; run
  CI's pin via `nix-shell -p golangci-lint --run "golangci-lint run ./..."` (v2.12.2).
- **Poll `gh pr view` only when every check holds a TERMINAL conclusion.** A mid-flight read
  returned `MERGEABLE/UNSTABLE`; re-read after settling gave `CLEAN`. Same trap AGENTS.md
  documents.
- **Verify a squash merge by CONTENT, never ancestry** — `merge-base --is-ancestor` is false
  after every squash, forever.
- **Decision: allowlist over `unicode.IsSpace`** for the external-URL guard. `IsSpace` is
  still a denylist; it closes the seven known runes and leaves the next one open. Control:
  mutant M14 (swap to `IsSpace`) dies *only* on the U+FF0F fullwidth-solidus row, so the
  weaker option demonstrably admits a hazard the shipped one rejects. Residual, documented
  in the source: pure-ASCII, so an IDN host or unencoded UTF-8 path is dropped (message
  says less, stays whole).
- **Decision: item 29 was INLINE in AGENTS.md** — and that was only ever true for one merge.
  A new decisions file needs a `splitItems` sha pinned to a base commit where the body
  already lived in AGENTS.md, which a first-appearance item cannot have. #426 WAS that first
  PR, so the base existed the moment it merged and the split became ordinary
  (`agentsSplitBaseWave5`). **Generalise it: a new item is inline until its first merge, not
  forever.**
- **This checkout is shared.** `main` moved under me mid-session (1b20b99 → 961050e) from
  another session, and a stale agent worktree held `fix/offsite-refusal` checked out,
  blocking a normal `git checkout -B`. Check `git worktree list` before branching.
- **Four review rounds, each finding what the previous round's fix left or introduced:**
  a whitespace guard that banned four ASCII bytes while claiming to ban all whitespace →
  fixing it left the allowlist's *accept* side unpinned so a re-spelled denylist passed
  green → the test written to catch dangling citations couldn't see its own walk shrinking.
  **CI was green at every step.**

- 🔴 **`claudedocs/decisions/29-offsite-refusal.md` has TWO regions with different rules.**
  Everything from the `29. **…` heading to EOF is **byte-pinned** — `agents_split_preserved_test.go`
  holds sha `df86c7a851e2397db48eebd2f4b9d17e91565128ec4550510a62b785f552828d` (8 non-blank lines,
  base `53a925bf7810f2e0dadd64ddc9f77f2e390ae8b4`). Corrections go **above the `---` rule**, in the
  free-form header, which is what both retractions did. **If an edit fails
  `TestSplitItemBodiesArePreservedVerbatim/item_29`, do NOT edit the sha or the test.**
- 🔴 **A first-appearance AGENTS item is inline only until its FIRST MERGE, not forever.** #426's own
  comment said a new item cannot carry a `splitItems` provenance row — true, but #426 *was* the first
  PR of that two-PR dance, so the base existed the moment it merged (`agentsSplitBaseWave5`).
- 🔴 **THREE claims retracted this session, two of them mine.** (a) *"No client change makes `app
  listing` reach an offsite app"* — `GET /api/v1/apps/{slug}` returns the `apl_` listing id for an
  offsite app and the CLI **already decodes it** (`AppDetail.ID`) on the refusal path itself;
  `getMyListingForEdit` and the asset procs gate on ownership only, **no kind check**. (b) *"an
  offsite app has no appBlockId — that is what `kind: offsite` MEANS"* — the schema says the
  opposite and warns against exactly that inference; the backfilled class exists but the backfill
  service is **DARK**, which is why production measures 0 rows. (c) my #3984 disclosure
  justification — *"store slugs are already public"* holds only for `approved`; the argument that
  actually works is that `submitExternalListing` already leaks a coarser oracle over the whole
  namespace.
- 🔴 **A `PENDING`-only CI poll reports ALL SETTLED before anything starts.** A freshly-created check
  has an **empty** conclusion, so it is not `PENDING`. My poll printed `SETTLED (failing=0)` on a
  rollup of 7 where 4 had blank conclusions and the two Tekton checks were **absent entirely** —
  one step from merging on it. Require a **terminal** conclusion
  (`SUCCESS|FAILURE|CANCELLED|TIMED_OUT|NEUTRAL|SKIPPED|ERROR`) for **every** check **and** assert a
  minimum count. AGENTS.md documents this trap; knowing it did not prevent it.
- 🔴 **Three CI failures on `civitai/civitai#3989`, none of them the code, each diagnosed not
  assumed.** (1) `build-image` — a Turbopack/PostCSS fault on CSS modules the PR never touched;
  a retry passed on the identical SHA. (2) `preview / unit-tests` — `globSync` is not a function,
  killing a convention scanner **including its own positive control**, so it reported nothing in
  either direction; also on #3988 → filed as **#4008**. (3) `tekton / typecheck` — **cancelled by a
  1h pipeline timeout** (59m40s, `TaskRunCancelled`), while six other PRs passed the same task in
  the same window, so *not* a degraded cluster; an empty commit re-ran it green in **4 minutes**.
  **A `preview` label toggle retriggers only when no run is in flight**; an empty commit always works.
- **The `civitai/civitai` worktree recipe is not the generic one** — submodule `event-engine-common`
  + a `.envrc` (`use flake`) that a worktree never inherits. Remove with
  `node .claude/skills/dev-server/cli.mjs wt rm <path>`, which **crashed on a pnpm symlink**
  (`ENOTDIR`); `git worktree remove --force` then `worktree prune` cleaned it up.
- **Do NOT consolidate `deleteOnsiteDraftListingForSlug`'s look-alike clause**
  (`publish-request.service.ts`). It is a `deleteMany` with no owner predicate and nothing
  downstream to refuse it — **the narrowness IS the authorization**. A cross-reference comment now
  says so at the site.
- **The audit→fix→re-audit cycle converged in one round** and was worth it: the first audit found
  the function JSDoc still documenting the deleted clause, a **surviving mutant** (selector
  precedence — `if (slug)` passed 44/44), and an access-control matrix whose `ONSITE_ONLY`
  *classification* was stale rather than merely mis-worded. The fix round then caught **its own**
  fix being vacuous (a permissive mock meant the new offsite cells passed under the old clause too).
  Delta re-audit: 17/17 mutants killed, zero production-code change proven by comment-stripped diff.

- 🔴 **`civitai app listing status` IS NOT A READ, and this is the single most expensive thing
  learned here.** It calls `getMyListingForEdit`, which for an APPROVED parent calls
  `beginListingRevision` and mints a shadow revision server-side
  (`offsite-listing.service.ts:1539-1541`). Every `app listing` subcommand does. **It caught
  three separate actors in one day** — including a verification step whose whole purpose was to
  avoid touching production — and left shadow drafts on `radio` and `gen-matrix`. **There is no
  server-side discard path** (grepped: no `discardRevision`/`abandonRevision`/`deleteRevision`/
  `cancelRevision`); clearing them is a web-UI action. Impact is bounded: the shadow is a hidden
  draft clone with a synthetic `rev-<ulid>` slug, nothing is submitted for review
  (`hasPendingRevision` keys on a pending publish REQUEST, not shadow existence), and the live
  listing is untouched. Tracked as `civitai/cli#389`. **Use `app view` or the fakes to verify.**
- 🔴 **A DECISION FILE CLAIMED A HAZARD WAS RETIRED WHILE THE CODE WALKED INTO IT.** #453's
  first draft added *"the shipped repair answers none of those constraints because it does not
  need to"* — 70 lines above the same file's own statement that `getMyListingForEdit` writes and
  *"cannot sit on the read-only `app listing status` path"*. Constraint 2 is a property of the
  PROC, not of how the listing id was obtained, so changing the resolver retired nothing. Caught
  by the adversarial audit, not by any gate. **When a change retires *some* of a documented
  constraint set, say which ones — never "none of them apply now".**
- 🔴 **A TEST KILLED BY A FIXTURE ARTIFACT LOOKS EXACTLY LIKE A TEST KILLED BY COVERAGE.** The
  audit reasoned that because a mutant returning `slugErr` was killed by 15 tests, the
  error-swallow was *pinned* and therefore deliberate. It was not: three fixtures answered
  **every** path with `{"submissions":[]}`, so the fallback received an undecodable body, and
  the CORRECT fix was killed by that artifact alone. Found only by running it. **A kill count is
  not evidence of intent.**
- 🔴 **`--project unit`-style blind spots have a CI-poll twin: a `PENDING`-only poll reports ALL
  SETTLED before anything starts**, because a freshly-created check has an **empty** conclusion.
  Require a terminal conclusion for EVERY check AND assert a minimum count.
- **GitHub's GraphQL API 503'd for hours** while REST stayed up: `gh pr view/diff/checks` all
  fail, `gh api repos/...` works. It also failed CodeQL's `init` action, producing an
  `Analyze (go)` FAILURE that was an outage, not a finding — and therefore not a clearance
  either. Re-run and confirm before reading a security gate as green.
- **Three CI failures on `civitai/civitai#3989`, none the code**, each diagnosed rather than
  assumed: a Turbopack/PostCSS fault at `build-image` (retry passed, same SHA); the `globSync`
  scanner (#4008); and `tekton / typecheck` **cancelled by a 1h pipeline timeout** while six
  other PRs passed the same task in that window — so not a degraded cluster. A `preview` label
  toggle retriggers only when no run is in flight; **an empty commit always works**.
- **`git worktree remove` is not the monorepo's tool** — use
  `node .claude/skills/dev-server/cli.mjs wt rm <path>`, which nonetheless crashed on a pnpm
  symlink (`ENOTDIR`); `git worktree remove --force` + `worktree prune` cleaned up. In THIS repo
  a plain worktree is fine.
- **`main` is protected here.** The handoff tooling's `--push` lands a commit on local `main`
  and then fails the push, leaving it stranded. Preserve onto a branch → **verify with
  `ls-remote`** → only then `reset --keep`.

- 🔴 **GREPPING FOR A STRING YOU DELIBERATELY QUOTED READS AS A REGRESSION. This fired TWICE.**
  A content check for `MERGED IS NOT DEPLOYED` after retiring it returned 1 — the hit was the
  *retraction bullet quoting it*. A check for `always works in the App-store` after fixing that
  clause returned 2 — both were comments recording the retired wording. **Read the surrounding
  lines; a grep count cannot distinguish a live claim from its own obituary.**
- 🔴 **A MUTATION THAT DOES NOT APPLY PRINTS THE UNMUTATED RESULT, WHICH READS AS "SURVIVED".**
  Verifying #4050's limiter, a regex missed because a new doc comment had shifted the block; the run
  printed 5/5 green. **Assert the mutation LANDED before reading the verdict** — `rateLimit({`
  count 16 → 15 is what caught it. Same class as the stale-bytecode trap, different mechanism.
- 🔴 **AN AUDIT CAN BE CONFIDENTLY WRONG ABOUT A SURVIVOR.** The #453 delta re-audit reported the
  #389 write-warning unpinned on **both** surfaces. False for the `status` `Long` half —
  `TestAppListingStatusJSONHelpWarnsAboutTheShadowSideEffect` reddens on deletion. Only the README
  half genuinely survived. A later checker caught it **by running both**, which the audit had not.
- 🔴 **`mapOffsiteError` does NOT send "everything else" to 400** — `NOT_FOUND`→404,
  `NOT_OWNED`**or**`FORBIDDEN`→**403**, `ALREADY_REPORTED`→409, remainder→400
  (`app-listings.router.ts:252`). I relayed the oversimplified version into a brief without checking
  that clause; corrected publicly on #389. The conclusion (all gates precede the write) was unaffected
  — but *"the conclusion survives"* is not *"the fact I stated was true"*.
- 🔴 **zsh ate a git ref.** `git show $B:src/...` — the `:s` is a **history modifier**, so the ref
  expands to a well-formed WRONG string and git errors confusingly. **Brace it: `"${B}:${path}"`.**
- 🔴 **`git checkout -- <file>` to revert a deliberate mutation takes your REAL edits with it.**
  An agent hit this on `README.md` mid-run, caught it via `git diff --stat`, re-applied. Restore
  from a `cp -a` copy, never from git.
- 🔴 **A HELP-TEXT BUDGET IS A GUARD: SATISFY IT, DO NOT RAISE IT.** `helpBodyBudget = 1400` runes
  (`internal/cmd/app_listing_help_test.go:329`) blocked a first draft at 1608. The fix was to
  compress to *"civitai/cli#389 settled that a FAILED call writes nothing; a successful one still
  does."* Raising a budget to fit your text is how the budget stops meaning anything.
- **`fs.globSync` needs Node 22+.** `civitai#4008`'s scanner failure was `node:20-bookworm`
  (v20.20.2, `typeof fs.globSync === "undefined"`) against a repo declaring `engines.node: ">=24"`.
  Fixed in `datapacket-talos@5135b849`, **not** in application code — and the code fix I had
  suggested would have papered over an unsupported runtime.
- **The three CLI surfaces citing an issue are held consistent by three guards** — a whole-string
  README pin, a help-text content check, and a **seam ledger** that fails if the warning drops off
  either published surface. When you change one, expect all three to redden. That is them working.

## How to verify

### 🔴 IS A GIVEN COMMIT LIVE ON `civitai.com`? — read it, don't infer it

```bash
gh api 'repos/civitai/civitai/deployments?per_page=20' --jq '.[]|select(.environment=="do-prod").id' |
  while read -r id; do gh api "repos/civitai/civitai/deployments/$id/statuses" --jq '.[].state' |
    grep -qx success && { gh api "repos/civitai/civitai/deployments/$id" --jq .sha; break; }; done
# then: git -C $CIVITAI merge-base --is-ancestor <commit> <that sha>
```

🔴 **Read the newest *successful* deployment, not the newest deployment — a FAILED deploy still
creates a deployment record carrying the sha it tried to ship.** On 2026-08-18 the naive read
would have called provenance live at 20:20Z; that deploy went `failure` at 21:50Z ("Pipeline
failed", `tekton-dp.civitai.com`) and the feature did not actually land until `a55fe05f3ca2`
succeeded at 22:19Z. **Positive control:** `3ff050f2` (#4050, merged 08-17) returns LIVE, so the
check can say yes — a bare "not live" from an instrument never seen to say otherwise is worth
nothing. Successful statuses carry `environment_url = https://civitai.com`, which is what ties
`do-prod` to production rather than to a name that merely looks like it.

### THE MEASUREMENT THAT CLOSED `cli#411` — re-runnable as a regression probe

Ran 2026-08-18 22:38Z; values it produced are inline. Costs one real submit, so re-run it only
when the provenance path itself is in question.

```bash
cd /home/zach/workspace/civit/cli && make build      # never the `civitai` on PATH — stale

S=prov-probe-$(od -An -N3 -tx1 /dev/urandom | tr -d ' ')
D=<scratchpad>/$S
./bin/civitai app create "$S" "$D" --template static
git init -q -b probe "$D"                            # NOT `main` — the guard blocks committing there
git -C "$D" add <the 8 scaffolded paths, explicitly>  # never -A/. — the guard blocks that too
git -C "$D" -c user.email=… -c user.name=… commit -q -m scaffold
git -C "$D" rev-parse HEAD                           # <- the sha that MUST land in the row

./bin/civitai app submit "$D" --yes                  # refuses non-interactively without --yes
```

🔴 **`git init` and the `git commit` must be SEPARATE Bash calls.** Chained in one compound
command the branch guard evaluates before `git init` has run, finds `$D` is not a worktree yet,
falls back to judging the *caller's* cwd — the `cli` clone, on `main` — and blocks the whole
thing with a message about the wrong repo.

Then read the row directly — the CLI's own output is not the proof, the row is:

```bash
export KUBECONFIG=$KC_DPPROD
kubectl exec -i -n cnpg-database cnpg-cluster-nvme0-5 -c postgres -- \
  psql -U postgres -d civitai -x -c \
  "select id, slug, status, source_commit, source_dirty from app_block_publish_requests where slug='$S';"
#   source_commit | 9fa0043f8fef741e62c3f572ab5e2e4f9014c0e0   <- == local HEAD
#   source_dirty  | f                                          <- f, NOT NULL
```

**Passes when** `source_commit` equals the local `HEAD` **and** `source_dirty` is `f`, not NULL
(the tree was clean — a NULL there means the tri-state collapsed somewhere). Worth also running
`./bin/civitai app status "$S"` and `--json` before withdrawing: that exercises the read
projection, which the row alone does not.

```bash
./bin/civitai app withdraw <pubreq_id> --yes         # ALWAYS — don't leave it in the mod queue
# confirm on the row: status = withdrawn, and the provenance survives the withdrawal
```

**If `source_commit` comes back NULL,** do not guess. Check the deploy gate above first; if the
commit IS live, the discriminating step is whether the CLI sent it — point the binary at a
capture server (`CIVITAI_BASE_URL=http://127.0.0.1:<port>`, `CIVITAI_TOKEN=dummy`) and log the
POST body. On 2026-08-18 round 1 that showed the client perfect and the server undeployed.

```bash
# The offsite repair, WITHOUT writing (see Gotchas — every `app listing` subcommand WRITES).
cd /home/zach/workspace/civit/cli && make build
./bin/civitai app view radio          # read-only

# The shipped pieces are on main:
git -C /home/zach/workspace/civit/cli show origin/main:internal/cmd/app_listing.go \
  | grep -c 'errors.Is(slugErr, civitai.ErrNotFound)'      # 1  — the by-slug fallback gate
git -C /home/zach/workspace/civit/cli show origin/main:internal/cmd/app_listing.go \
  | grep -c 'which is open'                                # 0  — #389 framing retired
git -C /home/zach/workspace/civit/civitai show origin/main:src/server/routers/app-listings.router.ts \
  | grep -c 'Too many listing lookups'                     # 1  — the rate limit

# Item 29's pinned body (breaks silently; corrections go in the HEADER, above the `---`):
git -C /home/zach/workspace/civit/cli show origin/main:claudedocs/decisions/29-offsite-refusal.md \
  | awk '/^29\. \*\*/{f=1} f' | grep -v '^[[:space:]]*$' | sha256sum
#   must equal df86c7a851e2397db48eebd2f4b9d17e91565128ec4550510a62b785f552828d
```
All four re-measured 2026-08-17 and still holding: `1`, `0`, `1`, sha matches.

### `civitai#4061` (the provenance server half) — worktree at `civitai-4059-provenance`

```bash
WT=/home/zach/workspace/civit/civitai-4059-provenance

# 🔴 EVERY command needs the direnv wrapper — pnpm/node are not on PATH otherwise.
direnv exec $WT pnpm typecheck                       # OK — 0 type errors

# Count the FILES, not just the tests: a path that matches nothing exits 0 and is
# omitted in silence. Six paths in, "Test Files 6 passed (6)" out, or you have a typo.
direnv exec $WT npx vitest run --project 'unit*' \
  src/tests/api/blocks/submit-version.test.ts \
  src/tests/api/v1/blocks/submit-version.test.ts \
  src/tests/api/v1/blocks/submissions.test.ts \
  src/server/services/blocks/__tests__/publish-request.service.test.ts \
  src/server/services/blocks/__tests__/publish-request.orchestration.test.ts \
  src/server/schema/blocks/submit-version-provenance.schema.test.ts
#   Test Files 6 passed (6) · Tests 255 passed (255)

# BOTH front doors must pass provenance through — the CLI uses the v1 one.
grep -c sourceCommit $WT/src/pages/api/blocks/submit-version.ts      # >=1
grep -c sourceCommit $WT/src/pages/api/v1/blocks/submit-version.ts   # >=1
grep -n DefaultSubmitPath /home/zach/workspace/civit/cli/internal/appapi/appblocks.go
#   -> "/api/v1/blocks/submit-version"

# The anti-strip control. Delete the sourceCommit field from submitVersionSchema and
# re-run the six files: 14 tests across 3 files MUST go red. Restore from a `cp -a`
# copy, NEVER `git checkout --`. If it stays green, the feature is inert.
```
🔴 **The migration is authored and UNAPPLIED.** Nothing above touches a database, and no
command here can tell you whether the SQL applies cleanly — that check does not exist yet.

🔴 **Do not use the `civitai` on `PATH`** — stale build.
🔴 **An earlier version of this section recommended `app listing status --slug radio` as the
verification command. That is a WRITE** — it mints a shadow revision on an approved listing. The
recommendation stood for one revision of this doc and is recorded here because it was replaced,
not struck through.
