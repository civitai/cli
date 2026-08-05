# Handoff: app-analytics-followups — updated 2026-08-05

## Goal

Finish the App-analytics follow-up stream.

**Status: the stream is essentially closed.** Every ranked item that was actionable has
shipped — including the one called "the real remaining product gap" (#6) and the last
data-quality gap (#5). What remains is a grab-bag of web-only polish, two leads nobody has
ever measured, and one decision that is `.github`-gated and therefore not an agent's to make.
See **Next steps**.

Predecessor doc (history + the original ranked list):
`claudedocs/2026-08-03-app-analytics-handoff.md`.

## State now

This doc now lives on `main`; there is no branch or worktree behind it.

**DONE — merged this stream (15 PRs):**

| PR | what |
|---|---|
| civitai#3561 | bound `block_scope_invocations.endpoint` so `topEndpoints` aggregates |
| civitai#3557 | dark-flag fabricated-zero fix (`unavailable` discriminator on analytics) |
| civitai#3572 | scope-gate `getMyAppAnalytics` **and** `getMyForgejoCloneInfo` (`AppBlocksSubmit`) |
| civitai#3574 | humanise BOTH analytics top-N cards (`analytics-bucket-labels.ts`) |
| civitai#3581 | `getMyRevenue` undiscriminated zero + extract shared `RevenuePanel` |
| cli#190 | `civitai app metrics <slug>` |
| cli#191 / cli#192 | `AGENTS.md` items 5–7 / item 8 (CLI-vs-web label divergence) |
| civitai#3606 | anchor the `Generations` locator — fixes the component-suite oscillation this stream caused |
| cli#194 | bump the stale scaffold pins (unblocked cli#193; see the recurrence note below) |
| cli#193 | land both handoff docs on `main` |
| civitai#3613 | **follow-up #6** — `blockRenders`' first reader + the "App loads" card |
| cli#195 | the matching `App loads` section in `civitai app metrics` |
| civitai#3626 | **follow-up #5** — bound the REST half of `block_scope_invocations.endpoint` |
| civitai#3627 | record App loads as untrusted input + unknown-app detection (no ingest change) |

**IN FLIGHT: nothing.** All PRs above are merged; no branch or worktree from this stream is
outstanding. Historical detail on the cli#193/#194 unblock is in git history — the operative
lesson is the recurrence note immediately below.

⚠️ **This will recur, and the automation cannot prevent it** — measured, not assumed. The
`bump-scaffold-pins.yml` workflow is NOT broken: it ran on schedule Mon 2026-08-03 at
**10:51 UTC** and correctly no-op'd, because both packages published *later that same day* —
`blocks-react` 0.38.0 at **20:15 UTC**, `app-sdk` 0.30.0 at **22:13 UTC** (npm registry `time`
field). It is a **weekly cron** (`17 7 * * 1`), so any `@civitai/*` minor publish leaves
`pins-vs-published` — a REQUIRED check — red on `main` and on every open PR for up to ~7 days
regardless of that PR's content. We happened to catch this one ~1 day in.

Two things follow: (a) when a PR is blocked by `pins-vs-published`, the fix is always
`go run ./internal/scaffold/cmd/bump-pins` + a PR — don't re-diagnose it; you can also just
`gh workflow run bump-scaffold-pins.yml` (someone already did exactly that on 2026-07-29,
49s after the 0.28.0 publish). (b) If the ~7-day window is judged too wide, the lever is cron
frequency (daily) — not the bumper, which works. ⚠️ `.github/workflows/*` is ask-first per
`AGENTS.md`, so that is a decision to raise, not to make.

🔴 **It recurred within a day, exactly as predicted.** cli#194 bumped to app-sdk `0.30.0` /
blocks-react `0.38.0` on 2026-08-04; by 2026-08-05 `main` carries **cli#203**, bumping the same
two pins again to `0.31.0` / `0.39.0`.

✅ **RESOLVED — the (b) lever was raised and approved, and the cron is now DAILY** (`17 7 * * *`,
cli#205, 2026-08-05). The window is bounded at ~24h instead of ~168h. The publish cadence that
justified it, measured off the npm registry `time` field: app-sdk 0.26.0 Jul 17 → 0.27.0 Jul 28
→ 0.28.0 Jul 29 → 0.29.0 Aug 3 20:15 → 0.30.0 Aug 3 22:13 → 0.31.0 Aug 5 03:54, i.e. six minors
in 19 days (~3.6-day mean) against a 7-day sweep — upstream published faster than the bot swept.
**Do not re-raise this as an open decision.** Point (a) still stands unchanged: a blocked PR is
still fixed by `go run ./internal/scaffold/cmd/bump-pins` + a PR, or by
`gh workflow run bump-scaffold-pins.yml` — daily narrows the window, it does not close it, so
pin drift remains a standing property of this repo rather than an incident to diagnose.

Minor, left alone deliberately: the bumper also rewrites a README prose line to read
"`accountType` / … require `@civitai/app-sdk@^0.30.0`", but those APIs arrived in the *older*
version — post-bump the sentence overstates the minimum. Over-strict, not harmful; rewriting
that site is the bumper's documented design. Flagged on cli#194, not changed.

**Deploy/verify status: CONFIRMED at every tier, including in production.**

- `preview / component-tests` — the open question in the previous revision — reported
  **success** on #3606 and on every subsequent PR in this stream (1259/1259). #3606's anchor
  fix holds. The mechanism was checked directly, not inferred: PRs still carrying the bare
  `getByText('Generations')` failed, PRs carrying `{ exact: true }` passed.
- **Verified live in prod, not just deployed.** `civitai app metrics gen-matrix --from
  2026-06-01` printed `App loads — Impressions 7 · Unique viewers 4 · Signed-out loads 0`.
  Cross-checked two ways: a hand-written ClickHouse query reproduced 7/4/0 for that app id
  exactly, and the reader's own query appears in `system.query_log` at **20ms** against its
  10s bound. The prod image tag was confirmed to carry the merge commit first.
- One caveat worth keeping: that 20ms was against ~125 rows in a single partition, so it
  proves the query is correct and fast **today** — it does not prove the `createdDate`
  partition pruning works at scale.

**Prod facts established (don't re-query):** `OauthClient.allowedScopes` for `civitai-cli`
= `100663297` (bit 25 live, so #3572 takes effect). Of 6,685,302 `ApiKey` rows: **0** are a
strict superset of `Full` lacking bit 25 (the allow→deny flip class is empty). **331** live
tokens were 403ing the analytics proc; **30** of them carry `33554433` and lack bit 26 —
which is why `AppBlocksSubmit` and not `AppBlocksDevTunnel` was correct.
Query via: `KUBECONFIG=$KC_DPPROD kubectl exec -n cnpg-database $(kubectl get cluster cnpg-cluster-nvme0 -n cnpg-database -o jsonpath='{.status.currentPrimary}') -- psql -U postgres -d civitai -c "<SQL>"`

---

## Open investigations — live diagnosis state

### ~~`preview / component-tests` is RED on #3574~~ — RESOLVED, fixed in civitai#3606

- **Symptom + exact repro:** the external `preview / component-tests` check went
  **success → failure** exactly at my audit-fix round on #3574, and stayed failed through
  merge. It is report-only (`"Component suite failed (report-only, not blocking)"`) so it
  did not block. Locally it only reproduces in a **full-project** run:
  `direnv exec <W> env -C <W> PLAYWRIGHT_BROWSERS_PATH=/tmp/claude-1000/pw-browsers ./node_modules/.bin/vitest run --project component`
  Single-file runs of the same file pass (7 tests), cold `optimizeDeps` cache included.

- **Observed (with values):**
  - Check history on #3574's commits:
    `075519d380` → `success`; `8e75826616` → `failure`; `928273e4dd` → `failure`.
  - `src/components/AppBlocks/AppAnalyticsPanel.browser.test.tsx:105` asserts
    `await expect.element(page.getByText('Generations')).toBeInTheDocument();`
  - `src/components/AppBlocks/AppAnalyticsPanel.tsx:263` renders a stat whose Tooltip label is
    `tooltip="Generations run through your app within the selected range, and the viewer Buzz burned doing so."`
  - Collision confirmed by computation: `'generations' in tooltip.lower()` → **True**;
    the PREVIOUS label `'Generation submits'` → **False**. So the rename introduced it.
  - `getByText` in vitest browser mode is **substring + case-insensitive** (established by
    civitai#3593, which fixed the identical defect in #3557's discriminator test).
  - civitai#3593's commit body: *"vitest browser mode shares ONE browser page across every
    `.browser.test.tsx` file, so in the full-suite run the pointer position left behind by
    an earlier file can already sit over the stat at mount. The locator then resolves to 2
    elements and the assertion dies with a strict mode violation after the matcher timeout."*
  - Local full-project runs (my host, non-canonical browser) fail **both** with the merge
    (`Error: [vitest] Browser connection was closed while running tests`) and **without** it
    (`exit=124`, timed out at 1500s). Run the without-merge baseline at `e4d23679c2^`.

- **Ruled out:**
  - *Cold `optimizeDeps` flake* — cold cache + single file passes (7 tests). `vitest.config.mts:98-124`
    documents the FIX for that, not an open flake. (Consequence: `lint.yml`'s unit-job comment
    citing it as the reason to exclude browser tests is **stale** — worth fixing.)
  - *My changes break the suite universally* — **PR 3591 PASSES `component-tests` on a base
    containing the #3574 merge**; PR 3594 fails on the same base. Verified with
    `git merge-base --is-ancestor e4d23679c2 <head>` → YES for both.
  - *Local repro will settle it* — it will not; the full-project run fails on this NixOS host
    regardless of the change (see values above).

- **Leading hypothesis (high confidence):** `getByText('Generations')` at
  `AppAnalyticsPanel.browser.test.tsx:105` resolves to 2 elements whenever the "Runs (range)"
  tooltip happens to be mounted — which the shared-page/leftover-pointer behaviour makes
  possible in a full-suite run but never in a single-file run. Strict-mode violation →
  matcher timeout → suite red. This is exactly #3593's defect class, introduced by my
  vocabulary rename, at exactly the commit where the check turned red.

- **FIXED — civitai#3606 (merged 2026-08-04).** Anchored with `{ exact: true }`, plus a
  regression guard that REPRODUCES the breaking state (hovers the tooltip's real target) so
  reverting the anchor fails a test rather than contradicting a comment.

  🔴 **The trap when reproducing this class:** the tooltip target is the 14px
  `IconInfoCircle`, NOT the label. Hovering the "Runs (range)" text does **not** mount it —
  measured `tooltip_mounted=false`. My first attempt hovered the label, saw the old assertion
  pass, and would have shipped an unproven fix; a positive control on the PROBE is what caught
  it. Hovering the icon gives `before=1 after=2 tooltip_mounted=true`.

  Proof matrix (all mutations checksum-verified, file restored byte-identical): old assertion
  + icon hovered → FAILS with `strict mode violation: getByText('Generations') resolved to 2
  elements`; new assertion + icon hovered → passes; drop the anchor from the new guard →
  FAILS; hover a non-mounting target → the guard's POSITIVE CONTROL fails, so it cannot pass
  vacuously.

  ✅ **CONFIRMED by the authority (2026-08-04).** `preview / component-tests` reported
  **success** on #3606's head and on every subsequent PR in this stream (1259/1259). The
  mechanism was verified directly rather than inferred: reading
  `AppAnalyticsPanel.browser.test.tsx` at each open PR's head SHA, the ones still carrying the
  bare `getByText('Generations')` failed and the ones carrying `{ exact: true }` passed.

- ~~Next probe / fix (apply verbatim):~~ historical — mirror #3593's fix in
  `src/components/AppBlocks/AppAnalyticsPanel.browser.test.tsx`:
  1. anchor the ambiguous assertions with `{ exact: true }` —
     `page.getByText('Generations', { exact: true })` at line 105 (and audit lines 129/130/144
     the same way, plus the `'App-local storage writes'` and `'AI workflow submits'` asserts);
  2. prove it by mutation the way #3593 did: park the pointer over the stat so the tooltip is
     open, run the file, confirm the OLD assertion fails and the new one passes;
  3. also re-check `expect(page.getByText('pending').elements()).toHaveLength(0)` (line ~146)
     — case-insensitive substring, so any future copy containing "spending"/"Pending" turns it
     into a false failure. `AppAnalyticsPanel.tsx` has no such copy today (grepped), so it is
     latent, not live.

  The preview component log IS reachable, and this is the durable trick: the `preview / *`
  GitHub statuses point at the deployed environment, not at logs — the output lives in the
  Tekton taskrun. `KUBECONFIG=$KC_DPPROD kubectl logs -n tekton-builds -l
  tekton.dev/taskRun=pr-preview-<PR>-<id>-component-tests --tail=60 --all-containers`. Note
  the taskrun reports `Succeeded` even when the suite fails, because the step is report-only —
  read the log's test counts, never the taskrun status.

---

## Next steps (ranked)

Everything actionable has shipped. What is left, honestly ranked:

1. ~~Fix the `getByText('Generations')` ambiguity~~ — DONE, civitai#3606.
2. ~~Unblock cli#193 by bumping the stale scaffold pins~~ — DONE, cli#194.
3. ~~Merge cli#193~~ — DONE.
4. ~~**Follow-up #6 — `blockRenders` has zero readers**~~ — DONE, civitai#3613 + cli#195.
   🔴 **The dedup advice this doc used to give was WRONG, and would have shipped a broken
   metric.** It said "per-mount, so unique views need query-side dedup". `blockInstanceId` is
   NOT per-mount — it is `page_apb_<ULID>`, roughly one per PLACEMENT. Measured on prod: 124
   rows carried 28 distinct `blockInstanceId` across 27 distinct `appBlockId`, ~1:1 with the
   app, so deduping on it reports **~1 unique viewer per app**. The correct identity is
   `userId`, and anonymous rows all carry `userId = 0`, so they need `ip` instead — hence
   `uniqExactIf(userId, isAnon=0) + uniqExactIf(ip, isAnon=1)`. Querying prod BEFORE building
   is what caught this.
5. ~~**Follow-up #5 — `normalizeEndpoint` writes unbounded values**~~ — DONE, civitai#3626.
   Note the fix is allowlist-first, not shape-based: the static route vocabulary is itself
   slug-shaped (`generation-resources`, `tip-allowance`), so any heuristic that templates
   `my-cool-collection` also templates those. A drift test walks `src/pages/api` so a new
   wrapped route fails CI rather than silently recording `:seg`.
6. **Follow-up #3** (owns-zero-apps unflagged all-zero; web-only) and **#7** (minor grab-bag).
   The only genuinely open feature work, and it is polish.
7. **`lint.yml`'s stale unit-job comment**, and possibly a GH-native component runner as
   `continue-on-error: true`. ⚠️ `.github/workflows/*` is ask-first per `AGENTS.md` — raise it,
   don't do it. Do NOT frame it as "adding a missing tier": the tier exists
   (`preview / component-tests`) and is now proven reliable.
8. **Two leads that have never been measured** — unchanged, and still worth a measurement
   before any code:
   - `addCollaborator` is a PUT that *sets* permission, so `civitai app pull` granting `read`
     where the web flow granted `write` may downgrade it and break a later push. #3572 made it
     reachable by the CLI's default credential; nobody has checked Forgejo's live PUT
     semantics.
   - `installs: 0` remains **unverified, not measured** — no positive control has ever shown
     that counter move.
9. **A latent flake worth someone's attention, not mine to fix:**
   `AppListingsMarketplaceBody.browser.test.tsx` → "the search box does NOT write the URL per
   keystroke" races two `await fill()` calls against a **300ms** debounce. Measured locally at
   **338ms** for the case — the margin is thinner than the test's own comment assumes. It
   failed once on civitai#3627's preview run and passed on the sibling PR ten minutes earlier;
   driving the debounce with fake timers would remove the timing dependency outright.

**Security:** a finding touching this stream was reported privately to security@civitai.com
per `SECURITY.md` (which forbids public issues, and PRs that demonstrate the issue). The
mechanism is deliberately **not** recorded in this repo or in code comments. `app-views.service.ts`
carries the operational consequence — treat App loads / unique viewers as untrusted input, and
do not build payouts, ranking, discovery, or leaderboards on them — and points maintainers at
that report before changing the ingest path.

---

## Gotchas / decisions / dead-ends

**Environment traps — every one produced a false GREEN in this session:**
- Node tooling needs **both** direnv and a real cwd: `direnv exec <W> env -C <W> ./node_modules/.bin/vitest …`. `--root` alone does not set `process.cwd()` and silently suppressed ~78 cwd-sensitive tests.
- A `node_modules` predating main's `@civitai/db-queries` package **silently removes ~1,126 tests** (10,131 passed vs 11,257 after `pnpm install --frozen-lockfile`) while still printing a plausible total. Filed on [civitai#3579](https://github.com/civitai/civitai/issues/3579).
- Bare `tsc --noEmit` OOMs and then reports **zero** errors. Always `NODE_OPTIONS="--max_old_space_size=8192"`.
- `prettier --check` prints its all-clean message even when it matched **zero** files (zsh passes a newline-joined list as one arg). Use `xargs` + positive-control against `src/components/Apps/AppAnalyticsInline.tsx`, which violates on main.
- Strip ANSI before grepping vitest output (`sed 's/\x1b\[[0-9;]*m//g'`) or greps match nothing and read as success.
- Backticks inside a double-quoted `gh ... --body "..."` get **command-substituted** by zsh and mangle the comment. Always `--body-file`.
- The `component` tier cannot run on this NixOS host with Playwright's vendored `chrome-headless-shell`. Overlay used: `PLAYWRIGHT_BROWSERS_PATH=/tmp/claude-1000/pw-browsers` → nixpkgs Chrome for Testing 149. **Non-canonical** — label any result from it.

**Decisions worth not re-litigating:**
- `#3572` uses `AppBlocksSubmit`, not `AppBlocksDevTunnel`: prod data shows 30 of 331 affected tokens lack bit 26. Not a style call.
- `#3581` declares ONE discriminator value (`notEntitled`), not two: `getRevenueForOwner` never *computes* ownership (it scopes by `appOwnerUserId` in the WHERE clause), so a not-owned request is a truthful zero and `notOwned` is unproducible.
- `#3574` does **not** reuse `humaniseScopeEndpoint`/`humaniseScopeInvocation` — measured, they return `'(no workflow id)'`, `''`, and wrong-register strings for these buckets.
- The CLI deliberately keeps printing raw tokens where the web humanises them (cli#192, `AGENTS.md` item 8).

**Dead ends / errors I made — recorded so they aren't repeated:**
- 🔴 **I claimed "CI does not run the `component` project at all". FALSE.** Every status query I wrote filtered for `Unit tests|Typecheck|ESLint|event-engine`, so `preview / component-tests` was never in a result set and I read its absence as nonexistence. Retracted on cli#193, civitai#3574, civitai#3581.
- 🔴 I diagnosed `get-models-raw.transient-503`'s failures as "tests pinning a widening the implementation lacks" from **test names alone**. Wrong — `model.service.ts:420` calls `isTransientMeiliError`. Real cause is the incomplete wholesale `vi.mock` of `~/server/db/pgDb` ([#3579](https://github.com/civitai/civitai/issues/3579)), which explains **both** of main's failing unit files.
- I reported "145 flip-risk credentials" from `(mask & Full) = Full`, which is also true when `mask == Full`. The strict-superset form needs `AND mask <> Full` → the real answer is **0**.
- Across 12 adversarial audit rounds, **every fix round found a defect in the previous fix**, and the mechanical gate caught none — the suite and typecheck were green at every tip. The recurring finds: a comment asserting a mechanism the code doesn't implement (5+ times); a test pinning the author's belief rather than the contract (including one that *certified a known-wrong label as intended*); a claim broader than its evidence; an unreachable guard counted as coverage.

---

## How to verify

```bash
W=/home/zach/workspace/civit/civitai-regate   # or any worktree; pnpm install FIRST
git -C $W fetch origin main && git -C $W checkout --detach origin/main
direnv exec $W env -C $W pnpm install --frozen-lockfile      # else ~1,126 tests vanish

# unit tier — expect 2 failed files (BOTH main's, see #3579), ~11,258 passed
direnv exec $W env -C $W ./node_modules/.bin/vitest run --project unit --reporter=dot 2>&1 \
  | sed 's/\x1b\[[0-9;]*m//g' | grep -E "Test Files|Tests "

# typecheck — expect exactly 1 error, main's generated updated-at-tables.ts
direnv exec $W env -C $W NODE_OPTIONS="--max_old_space_size=8192" ./node_modules/.bin/tsc --noEmit 2>&1 | grep -c "error TS"

# the analytics/revenue guards specifically — expect 40 passed
direnv exec $W env -C $W ./node_modules/.bin/vitest run --project unit \
  src/server/services/blocks/__tests__/buzz-attribution.service.test.ts \
  src/components/AppBlocks/__tests__/revenue-panel-wiring.test.ts \
  src/server/routers/__tests__/blocks.router.getMyAppAnalytics.test.ts

# component tier — SINGLE FILE ONLY on this host; full-project does not complete here
timeout 600 direnv exec $W env -C $W PLAYWRIGHT_BROWSERS_PATH=/tmp/claude-1000/pw-browsers \
  ./node_modules/.bin/vitest run --project component \
  src/components/AppBlocks/AppAnalyticsPanel.browser.test.tsx
```

The CLI command itself: `civitai app metrics <slug>` (needs a login token; the fix in #3572
is what makes that work). `--json` passes the payload through raw and still exits 0, so a
script must branch on `notOwned` / `unavailable` itself.
