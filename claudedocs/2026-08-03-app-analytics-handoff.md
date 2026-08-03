# Handoff — App analytics: CLI command + two platform fixes

**Session date:** 2026-08-02 → 2026-08-03
**Repos touched:** `civitai/cli`, `civitai/civitai`, `devrc`
**Status at handoff:** 3 PRs open and audited, 1 re-gate in flight, nothing merged.

---

## What this was

Started as "tell me about the CLI", became: *can I see analytics for my published
Apps?* The answer turned out to be **yes on the platform, no from the CLI** — a
complete owner-scoped analytics service (`blocks.getMyAppAnalytics`, shipped
2026-06-21) existed behind tRPC with a web UI panel, and the CLI simply never
called it. Building that call surfaced two real platform defects.

---

## Open PRs — none merged

| PR | repo | what | state |
|---|---|---|---|
| [#190](https://github.com/civitai/cli/pull/190) | cli | `civitai app metrics <slug>` | 3 commits, CI green, audited ×2, **verified live** |
| [#3557](https://github.com/civitai/civitai/pull/3557) | civitai | dark-flag fabricated-zero fix | 8 files, audited (8/8 mutants), `MERGEABLE` |
| [#3561](https://github.com/civitai/civitai/pull/3561) | civitai | endpoint cardinality fix | 10 files, audited, `CLEAN` |

**Agreed merge order: #3561 → #3557 → cli#190.** Not a correctness constraint
(the regions are disjoint) — #3561 first because it is behind main and its
browser tests had never run against main's `component-setup.tsx` change until the
gate; cli#190 last because its `notOwned` guard only becomes meaningful once
#3557 deploys.

### RESOLVED — the re-gate came back clean

**The blocker below is cleared; it is kept for context.** A re-gate ran against
`main@dd888989fb` and, when main advanced again mid-run, re-ran the whole thing
against `fdf8e13bed`. Both bases give identical results: merged tree **752 files
/ 10896 passed / 1 skipped / 0 failed**, typecheck **0 errors** (validated with
an injected type error proving tsc reports exactly 1), and
`blocks.router.workflow.test.ts` collects **311** tests. The +10 delta lands in
exactly 5 PR-owned files — nothing stopped collecting.

**#3561's endpoint assertions survive main's rewrite intact and non-vacuously**,
proven by two discriminating mutations rather than asserted: reverting the three
router sites turns exactly 5 tests red on *this* guard's own error
(`expected 'workflow:submit:pending' to be 'workflow:submit'`); deleting the
`detail.workflowId` spread turns exactly 4 red, with the `:pending` test
correctly staying green because it asserts absence.

One real blocker surfaced and was fixed: #3557 failed `prettier --check` on a
file it created (`AppAnalyticsPanel.browser.test.tsx:48-51`). Fixed in
`b77f849e9f` — pure line-wrap, no behaviour change. The 9 other prettier
failures on that tree are pre-existing on main. **#3557's CI needs to re-green
after that push before merging.**

Caveats carried forward: the component-suite numbers came from a
**non-canonical browser build** — Playwright's vendored `chrome-headless-shell`
is a generic-linux binary NixOS refuses to exec, so the gate shimmed nixpkgs
chromium 1228 into the `-1200` names. Unit results are unaffected and are the
stronger evidence. The gate also ran prettier only, not ESLint proper.

Gate worktrees left in place: `/home/zach/workspace/civit/civitai-regate` and
`civitai-regate-ctl`.

### Original blocker (historical — resolved above)

A gate merged #3557 + #3561 off `main@ed6e17ac7d` and **passed** (751 files /
10852 passed / 1 skipped / 0 failed, typecheck 0 with a positive control).

Then `main` advanced 3 commits to `dd888989fb`. One of them —
`dd720a8078` *"feat(app-blocks): tie a step's moderation posture to its
orchestrator $type (#3554)"* — rewrote **779 lines of
`src/server/routers/blocks.router.ts`** and **483 lines of
`src/server/routers/__tests__/blocks.router.workflow.test.ts`**. That is the file
both PRs touch and the test file #3561 edits.

GitHub still reports both `MERGEABLE` — no *textual* conflict. Verified:
`dd720a8078` does **not** touch the `workflow:submit` / `recordScopeInvocation`
lines, and all three endpoint sites survive in current main at `:4449`, `:6328`,
`:7107`. So source-level risk is low.

**Unresolved:** whether #3561's endpoint assertions survive main's test-file
rewrite *non-vacuously*. A re-gate against `dd888989fb` was dispatched and had
not reported at handoff. Its worktree: `/home/zach/workspace/civit/civitai-regate`.

**Do not merge until that re-gate reports.** If #3561's assertions were hollowed
out, that is a rebase-and-re-verify on #3561 before anything lands.

---

## The measured data (owner's own apps, 2026-05-01 → 2026-08-03)

Reachable via `blocks.getMyAppAnalytics`. Totals: **62 runs · 1,819 Buzz spent ·
343 API calls · 0 installs · 0 Buzz purchased**, max 3 active users.

| app | runs | Buzz | API calls | err |
|---|---:|---:|---:|---:|
| gen-matrix | 20 | 65 | 26 | 0% |
| dogfood-manual | 11 | 35 | 17 | 0% |
| panorama-360 | 9 | 0 | 9 | 0% |
| custom-generators | 8 | **1,672** | 19 | 0% |
| seed-explorer | 6 | 18 | 8 | 0% |
| model-benchmarking | 5 | 20 | 5 | 0% |
| w6-ui-dogfood | 2 | 6 | 5 | 0% |
| buzz-generator | 1 | 3 | 5 | 0% |
| playable-collections | 0 | 0 | **245** | 2% |
| app-requests | 0 | 0 | 4 | 0% |
| 6 others | 0 | 0 | 0 | — |

Dark: `continuous-gen`, `df-qwen-canvas`, `lora-weight-lab`, `model-notes`,
`notepad`, `prompt-library`.

**`custom-generators` is unexplained and worth a look** — 1,672 Buzz over 8 runs
is ~209/run against ~3/run everywhere else, i.e. 92% of all spend.

**Treat `installs: 0` as unverified, not measured.** Uniform across all 16 apps;
no positive control was ever obtained proving that counter can move.

Submission state: 16 apps with a live deploy, 100 submissions (2026-06-17 →
2026-08-01), 79 approved / 18 withdrawn / 2 rejected / 1 pending (`sensei`).

---

## Follow-ups not done (ranked)

1. **`getMyAppAnalytics` has no `.meta({ requiredScope })`** → defaults to
   `TokenScope.Full`, so a personal API key works but a `civitai login` OAuth
   token **403s**. Since `login` with no flags is the default auth path, most
   users cannot use the command. Precedent: `stopDevTunnel`
   (`blocks.router.ts:1375`). *Decide which scope is right — do not copy blindly;
   the proc exposes installs, Buzz spend, endpoint names, active-user counts.*
   This was explicitly deferred as the one item where the right answer was not
   already determined by evidence.
2. **`topEndpoints` renders raw in `AppAnalyticsPanel.tsx:343-348`.** A
   cross-PR interaction only the merged tree shows: after #3561 the bounded
   `workflow:submit` / `storage:set` tokens collapse into single top-ranked
   buckets, so the panel will now *prominently* display raw internal tokens.
   `AppActivityPanel` has `humaniseScopeEndpoint`; `AppAnalyticsPanel` has no
   humaniser. Cosmetic.
3. **Owns-zero-apps returns unflagged all-zero counters**
   (`app-analytics.service.ts:187-188`). Flagged independently by two agents. Web
   only — unreachable from the CLI, which always sends a concrete `appBlockId`
   (`metrics <slug>`, `cobra.ExactArgs(1)`). Defensible (an aggregate over an
   empty set genuinely is zero) but wants a recorded decision.
4. **`getMyRevenue` has the identical undiscriminated-zero bug**
   (`blocks.router.ts:5152` → `emptyRevenue()`, no discriminator at all).
   `/apps/revenue` renders both panels, so that page still shows one fabricated
   zero surface.
5. **`normalizeEndpoint` still writes unbounded values**
   (`block-scope.middleware.ts:930`) — templates only `^\d+$`→`:id` and ULIDs;
   a slug or UUID segment passes through verbatim, and the row is written from
   `res.on('finish')` registered *before* the handler, so a 400 still logs.
6. **ClickHouse `blockRenders` has zero readers.** Written since 2026-06-22
   (`Tracker.blockRender()` → `/api/track/block-render`), carries `appBlockId`,
   `slotId`, `isAnon`. It is the **only** signal covering anonymous viewers and
   static/no-scope blocks — exactly where the Postgres engagement metrics go
   flat. Biggest remaining product gap; per-mount, so unique views need
   query-side dedup.
7. Minor: `deployDetail: 'Build None'` gave zero diagnostic when `gen-matrix`
   0.7.2 failed to build · `GET /api/v1/blocks/submissions` caps at
   `MAX_ROWS = 100` with no cursor (owner is already at exactly 100, so history
   before 2026-06-17 is unreachable) · five structurally dead `AppListingMetric`
   columns (`openCount`/`visitCount`/`connectCount`/`tipped*`) · pflag
   back-quote trap still live at `app_dev_tunnel.go:395`.

---

## Session learnings (a doc-update PR set was dispatched for these)

- **`isolation: "worktree"` binds to the CURRENT repo**, not the target. Two
  agents aimed at the monorepo got worktrees of the Go repo. Cross-repo dispatch
  must create its own worktree explicitly.
- **Scope process kills by `/proc/<pid>/cwd` when agents run in parallel.** A
  system-wide `chrome-headless-shell|vitest|steam-run` kill took out ~15 PIDs
  including a sibling's in-flight gate.
- **A gate is evidence about the base it ran on.** Re-check how far behind main
  you are immediately before merging.
- **Submodule missing → `blocks.router.workflow.test.ts` collects 0 tests** and
  reads as a pass. Validate every worktree run against a nonzero collect count.
- **The `.envrc` trap fired twice more:** system Node 26.5.0 vs flake 22.22.2
  (7 spurious failures), and a wrong cwd that let **77 tests silently not run**
  while the output looked normal.
- **`prettier --check --stdin-filepath` prints `(stdin)`**, not the path — a
  grep for the path gave a false CLEAN.
- **A DTO field that is never branched on is not a guard.** `notOwned` was
  declared in the panel's type and never read; the not-owned case had been
  fabricating zeros on the web all along.
- **Every audit round found something real** — three rounds on #190, and the fix
  round introduced its own regression (a genuinely nonzero error rate rendering
  as `0.0%`).

---

## Verification notes

- cli#190 was exercised against the **live** API with a personal key: `2.0%` /
  `0.0%` error rates matching independent `curl`, exit 4 / 2 / 0, `--json` raw
  passthrough intact. The `<0.1%` band is fake-server + mutation only — no app of
  the owner's has a tiny-nonzero error rate to point at.
- The re-gate's harness must self-validate: nonzero collect on
  `blocks.router.workflow.test.ts`, and an injected type error proving `tsc` can
  see one before trusting a `0`.
