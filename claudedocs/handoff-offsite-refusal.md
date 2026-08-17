# Handoff: offsite-refusal — 2026-08-16

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

`civitai app listing` / `app status <slug>` were unreachable for **offsite** apps and told
their owners to run `civitai app submit` — a command that cannot succeed for an app that is
a registered URL rather than a block bundle (#422). Ship the honest refusal, and establish
whether reaching those apps is possible at all.

## State now

- **`civitai/cli` `main` @ `bb7d7f2`**, clean, in sync. One open PR of mine, **#442**
  (`*.env` anywhere / case-insensitive pkgzip patterns), unrelated to this thread.
- **The server-side selector is MERGED: `civitai/civitai#3989` → `a15b4fb7b1`.** One clause:
  `where: { slug, kind:'onsite', appBlockId:null, status:'draft' }` → `where: { slug, revisionOfId: null }`
  in `getMyListingForApp` (`offsite-listing.service.ts:~1746`). No schema change (the `slug`
  selector was already declared), no migration, no index — `AppListing.slug` is `@unique`
  across both kinds.
- 🔴 **MERGED IS NOT DEPLOYED, AND IT IS NOT DEPLOYED.** Measured 2026-08-17 04:35Z against
  `https://civitai.com`: slug-only `getMyListingForApp` still 404s for `gen-matrix`, `radio`
  and `comfy`. **Positive control in the same run:** `app listing status --slug gen-matrix`
  resolves (exit 0) via the appBlockId path, so the 404s are about the selector, not the
  probe. **Until this flips, the CLI half must not ship** — a client calling a selector the
  server drops is the inert-feature shape.
- **Shipped in `civitai/cli`:**
  | what | PR | merge |
  |---|---|---|
  | AGENTS item 29 split to an evidence file + "no client path" retracted | #432 | `b1213a3` |
  | `appBlockId` is not a kind discriminator — second retraction | #441 | `483441e` |
  | README scoped to *this CLI* (published contract) | #446 | `bb7d7f2` |
- **Filed upstream:** **#3984** (the ask) · **#4003** (unbounded existence oracle, deliberately
  out of #3989's scope) · **#4008** (`globSync` breaks a convention scanner incl. its own
  positive control).
- **Verified, not just merged:** every merge confirmed by CONTENT on `origin/main`, never by
  ancestry — a squash makes `merge-base --is-ancestor` false forever.

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

## Next steps (ranked)

1. 🔴 **Poll for the #3989 deploy, then ship the CLI half.** The one command that settles it
   (read-only, both controls built in) is under *How to verify*. When slug-only resolves:
   `resolveListing` passes the slug for offsite, and the refusal **narrows to a fallback for
   older servers rather than being deleted**. Then `README.md` and
   `claudedocs/decisions/29-offsite-refusal.md` both need the "merged but not yet deployed"
   note removed — grep for `3989`.
2. **#442** — the `*.env` pkgzip PR, open and unrelated to this thread.
3. **#420** — `.env.d/db.env` packaged; credential-shaped, per-name decision not a blanket
   `excludedDirs ⇄ excludedFiles` promotion.
4. **#424's remaining question is now narrow.** Its point 3 is resolved by #3989. What is still
   unmeasured: a genuinely **pending** first-version *onsite* app — the only shape the OLD clause
   served. Nothing depends on it; the widening left that path untouched.
5. **Two 🟢 nits on #3989**, now post-merge so they need a follow-up PR: a comment at
   `app-collaborator.permission-matrix.test.ts:284` restating the very absolute its own commit
   softened, and the PR body's mutation counts being wrong in both directions (the matrix mock
   is not Prisma-filter-aware, so `{not:…}` mutants die from a mock artefact).
6. **#427** — the refusal asserts ownership it never checked. **#411 second half** — provenance
   stamp, still needs the server field.

## Gotchas / decisions / dead-ends

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

## How to verify

```bash
# 1. Is the #3989 selector live yet? THE one question gating the CLI half.
#    Throwaway Go test in internal/cmd (read-only: getMyListingForApp is a .query and
#    since the lazy-shadow change does NOT mint a revision — that is getMyListingForEdit,
#    which DOES write). Delete the file afterwards.
#      c := appapi.NewWithSource(cfg.BaseURL(), auth.New(cfg), "")
#      c.GetMyListingForApp(ctx, "", "radio")     // 404 today; resolves once deployed
# 🔴 The positive control is NOT optional — four 404s alone are indistinguishable from a
#    broken probe, which is what invalidated #422's first attempt AND my own first draft:
cd /home/zach/workspace/civit/cli && make build
./bin/civitai app listing status --slug gen-matrix   # MUST resolve, exit 0 — route+auth work
./bin/civitai app listing status --slug radio        # offsite refusal, exit 4 (until deploy)

# 2. The retractions are on main
git -C /home/zach/workspace/civit/cli show origin/main:README.md | grep -c "no client-side route"   # 0
git -C /home/zach/workspace/civit/cli show origin/main:internal/cmd/app_offsite.go | grep -c "MEANS" # 0

# 3. Item 29's pinned body is intact (see Gotchas — this is the one that breaks silently)
git -C /home/zach/workspace/civit/cli show origin/main:claudedocs/decisions/29-offsite-refusal.md \
  | awk '/^29\. \*\*/{f=1} f' | grep -v '^[[:space:]]*$' | sha256sum
#   must equal df86c7a851e2397db48eebd2f4b9d17e91565128ec4550510a62b785f552828d
```

🔴 **Do not use the `civitai` on `PATH`** — stale build, predates all of this.
