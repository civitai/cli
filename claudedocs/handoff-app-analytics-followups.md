# Handoff: app-analytics-followups — 2026-08-04

## Goal

Finish the App-analytics follow-up stream. The feature work is **done and merged**; what
remains is one self-inflicted broken test (diagnosed below, fix is known), and the
lower-priority follow-ups #3/#5/#6/#7 from the original handoff.

Predecessor doc (history + the original ranked list):
`claudedocs/2026-08-03-app-analytics-handoff.md` — same branch, same PR.

## State now

- **Branch / PR:** `zach/handoff-app-analytics` → [cli#193](https://github.com/civitai/cli/pull/193) (OPEN, `MERGEABLE`/`BLOCKED` on checks). This doc rides on it.
- **Worktree for it:** `/home/zach/workspace/civit/cli-handoff-analytics`

**DONE — merged this stream (8 PRs):**

| PR | what |
|---|---|
| civitai#3561 | bound `block_scope_invocations.endpoint` so `topEndpoints` aggregates |
| civitai#3557 | dark-flag fabricated-zero fix (`unavailable` discriminator on analytics) |
| civitai#3572 | scope-gate `getMyAppAnalytics` **and** `getMyForgejoCloneInfo` (`AppBlocksSubmit`) |
| civitai#3574 | humanise BOTH analytics top-N cards (`analytics-bucket-labels.ts`) |
| civitai#3581 | `getMyRevenue` undiscriminated zero + extract shared `RevenuePanel` |
| cli#190 | `civitai app metrics <slug>` |
| cli#191 / cli#192 | `AGENTS.md` items 5–7 / item 8 (CLI-vs-web label divergence) |

**IN FLIGHT:** cli#193 only (two docs). Nothing else uncommitted anywhere.

**Deploy/verify status:** all merged to `civitai/main` and `civitai-cli/main`. Verified at
the unit tier and via prod SQL (below). **NOT verified** at the component/browser tier —
see the open investigation; that tier is report-only and currently red on #3574.

**Prod facts established (don't re-query):** `OauthClient.allowedScopes` for `civitai-cli`
= `100663297` (bit 25 live, so #3572 takes effect). Of 6,685,302 `ApiKey` rows: **0** are a
strict superset of `Full` lacking bit 25 (the allow→deny flip class is empty). **331** live
tokens were 403ing the analytics proc; **30** of them carry `33554433` and lack bit 26 —
which is why `AppBlocksSubmit` and not `AppBlocksDevTunnel` was correct.
Query via: `KUBECONFIG=$KC_DPPROD kubectl exec -n cnpg-database $(kubectl get cluster cnpg-cluster-nvme0 -n cnpg-database -o jsonpath='{.status.currentPrimary}') -- psql -U postgres -d civitai -c "<SQL>"`

---

## Open investigations — live diagnosis state

### `preview / component-tests` is RED on #3574 — ROOT CAUSE FOUND, fix not yet written

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

- **Next probe / fix (apply verbatim):** mirror #3593's fix in
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

  Then get the preview pipeline's component log for a post-fix commit to confirm; that log is
  the only authority and I could not reach it.

---

## Next steps (ranked)

1. **Fix the `getByText('Generations')` ambiguity** (above) and open a small PR. Self-inflicted,
   diagnosed, ~5 lines. Do this first.
2. **Merge cli#193** (this doc + the updated predecessor). Docs-only.
3. **Fix `lint.yml`'s stale unit-job comment** and consider adding the component project to
   GitHub Actions as `continue-on-error: true` — the same report-only posture the unit job
   uses, and the only way to get canonical-browser signal. ⚠️ `.github/workflows/*` is
   ask-first per `AGENTS.md`. Do NOT frame this as "adding a missing tier" — the tier exists
   (`preview / component-tests`); this is about a second, GH-native runner.
4. **Follow-up #6 — ClickHouse `blockRenders` has zero readers.** The real remaining product
   gap: written since 2026-06-22, carries `appBlockId`/`slotId`/`isAnon`, and is the only
   signal covering anonymous viewers and static blocks. Per-mount, so unique views need
   query-side dedup.
5. **Follow-up #5 — `normalizeEndpoint` still writes unbounded values**
   (`block-scope.middleware.ts`): templates only `^\d+$` and ULIDs, so a slug/UUID segment
   passes through verbatim into `topEndpoints`.
6. **Follow-up #3** (owns-zero-apps unflagged all-zero; web-only) and **#7** (minor grab-bag).
7. **Two unverified leads:** `addCollaborator` is a PUT that *sets* permission, so
   `civitai app pull` granting `read` where the web flow granted `write` may downgrade it and
   break a later push (#3572 made it reachable by the CLI's default credential; nobody checked
   Forgejo's live PUT semantics). And `installs: 0` remains **unverified, not measured** — no
   positive control has ever shown that counter move.

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
