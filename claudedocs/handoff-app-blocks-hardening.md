# Handoff: app-blocks-hardening — 2026-08-06

## Goal
Close out the App Blocks CI/test-integrity work and the two real production bugs it
uncovered (a silent host message drop, and a silent Forgejo privilege downgrade).
All work landed in `civitai/civitai`, not `civitai/cli`.

## State now

- **Branch / PR: nothing open from this session.** All 9 PRs merged to `civitai/civitai@main`.
- Base clones at session end: `civitai` on `main` (clean); `cli` on
  `docs/login-comment-scanner-reflow` (**another session's branch** — do not assume it is yours).
- Shared stash stack: **58 entries, untouched**. Never `git stash` in either repo.

### Merged this session (verified by CONTENT, not ancestry — ancestry is always false after a squash)

| PR | merge commit | what |
|---|---|---|
| #3658 | `d705fbd97d` | shared client-IP predicate for rate-limit bucket keys |
| #3675 | `27431d86d9` | image-upload flake — React commit/effect gap |
| #3680 | `5c76917c24` | **PROD**: 5 status gates read a stale closure; `statusRef` moved to render body + NACKs |
| #3688 | `21e388d6b5` | token-terminal flake — `driveToReady` gated on commit, not effect |
| #3689 | `8db68a394b` | manual-Retry flake — driver click racing a 2s real backoff (virtual clock) |
| #3690 | `41213ed4b1` | "Pull from site is GONE" guard now catches a PARTIAL render |
| #3700 | `f2b954d9c3` | spend-column write-surface guard: 60s timeout → untimed |
| #3701 | `73cae3561a` | NUL byte blinding recursive ripgrep to a 168KB service + repo-wide guard |
| #3713 | `0ee780b043` | **PROD**: Forgejo `addCollaborator` grant-at-least (was silently downgrading) |

Also filed: **issue #3685** (IframeHost divergence, deliberately not fixed — see below).

- **Deploy/verify status (updated 2026-08-06, post-deploy pass):**

| PR | in prod (`release`)? | verified live? |
|---|---|---|
| #3680 | **YES** | ✅ **VERIFIED on civitai.com** — see below |
| #3713 | **NO — never released** | ❌ not deployed, so not verifiable |

  🔴 **`main` IS NOT PRODUCTION.** Prod (`civitai.com`) is built from the **`release`**
  branch → `ghcr.io/civitai/civitai-prod`; `main` builds `civitai-web` → **`next.civitai.com`**
  only. Getting `main` → prod is a deliberate human step (`pnpm run release`). The whole
  merged-this-session table above was "content-verified on `main`", which says **nothing**
  about prod. Everything below #3680 in that table is still sitting in `next` only.

  Image tag format is `{yyyymmddHHMMSS}-{sha7}`, so the tag names the deployed commit:
  ```bash
  git -C $DATAPACKET show origin/trunk:clusters/production/apps/civitai-dp-prod/deployment.yaml | command grep 'image: "ghcr.io/civitai'
  git -C $DATAPACKET show origin/trunk:clusters/production/apps/civitai-next/deployment.yaml   | command grep 'image: "ghcr.io/civitai'
  ```
  Measured 2026-08-06: prod `20260806232003-0c310aa` (= `0c310aadd7`, "5.0.2248", release HEAD);
  next `20260807012800-0dd0e8e` (= `0dd0e8e57f`, main HEAD). **Verify by CONTENT at that sha,
  not by ancestry** — `release` is produced by a REBASE, so `merge-base --is-ancestor` is false
  for changes that ARE present (same class as the squash-merge trap).

#### ✅ #3680 — VERIFIED live on production civitai.com

Vehicle: `civitai app dev-tunnel`, which frames a local page in the **real production
`PageBlockHost`** at `civitai.com/apps/dev/<blockId>` (confirmed: that route imports
`PageBlockHost`, not the still-unfixed `IframeHost` of #3685). Probe posted
`OPEN_BUZZ_PURCHASE` with a `requestId` **before** acking `BLOCK_READY`, i.e. host
status `'loading'`.

    [  7702ms] POSITIVE CONTROL ok — inbound channel live (BLOCK_INIT from https://civitai.com)
    [  7702ms] SENT OPEN_BUZZ_PURCHASE pre-ready (requestId=probe-preready-pdz7tvs9)
    [  7702ms] RECEIVED BUZZ_PURCHASE_RESULT purchased=false
    [  7952ms] SENT BLOCK_READY (ack, after the probe request)

Reply arrived in the same millisecond bucket, and **250ms before** the block acked ready —
so it was genuinely the pre-ready refusal path. Old code at `5c76917c24^` is
`if (requestId == null || !raw) return;` — a bare return, no reply, so the block hangs to
the SDK's 30s `DEFAULT_REQUEST_TIMEOUT_MS`. New code sends `purchased:false`.

Two controls make this non-vacuous:
- **Positive control**: `BLOCK_INIT` observed, so "no reply" would have been a real
  observation rather than a listener wired to nothing.
- **False-pass control**: the pre-existing `reviewNack` branch *also* replies
  `purchased:false`. It could not have fired — `/apps/dev/[blockId]` never passes
  `reviewMode` (**0 occurrences** in the file), so it defaults `false` → `reviewNack` false.

Spends nothing in either code path: a pre-ready request is refused by both; only whether
the refusal is ANSWERED differs. Costs one dev tunnel and a static page — repeatable.

**This also validated the deployed-sha method**: reading source at `0c310aadd7` predicted
prod's observable behaviour correctly, which is what licenses the #3713 conclusion below.

#### ❌ #3713 — NOT deployed; verification is blocked, not merely skipped

Content at the deployed prod commit `0c310aadd7`: `permissionRank` **0 occurrences**, and
the old false comment `"422 if already a collaborator"` **still present**. Prod runs the
original bare-PUT `addCollaborator`. `next` (`0dd0e8e57f`) has the fix (`permissionRank` ×2,
false comment gone).

🔴 **So the recipe this doc previously recommended would have CAUSED the bug, not verified
the fix.** `civitai app pull` against prod today calls `getMyForgejoCloneInfo` → asks `read`
→ unfixed SET → **silently downgrades your own `write` to `read`**. That is live right now.
Prod and next share ONE Forgejo (`FORGEJO_BASE_URL: http://forgejo-http.forgejo.svc.cluster.local:3000`
in **both** manifests), so the damage is to the real repo either way. Recover by loading the
app's repo page on the web (`getMyAppRepo` asks `write`).

Verifying on `next` instead was attempted and is blocked on auth, not on method: the CLI
gets `401` there (oauth2-proxy 302s `next.civitai.com`; an API key does not pass it) and the
browser lands on `/login?returnUrl=%2F`. Driving that login was not attempted.

**Post-release check (run once #3713 is on `release`), in this order — step 2 is the
positive control, without it step 4 proves nothing:**
1. Load the app's repo page on the web (`getMyAppRepo`, grants `write`).
2. `git push --dry-run` to the Forgejo remote → **must succeed**. If it fails, stop; you
   never had `write` and the rest is meaningless.
3. `civitai app pull --app <slug>` (`getMyForgejoCloneInfo`, asks `read`).
4. `git push --dry-run` again → must **still** succeed. On unfixed code it fails with
   `not allowed to push to branch 'main'`.

## Open investigations — live diagnosis state

### `orchestrator-token-cache` TTL test flakes
- **Symptom + exact repro:** intermittent, in `preview / unit-tests`. Not reliably reproducible.
  `src/server/orchestrator/__tests__/orchestrator-token-cache.test.ts >
  orchestrator-token-cache — env-driven module init > expires cached entries after the
  configured TTL (re-mints on next call)`
- **Observed (with values):** `AssertionError: expected 'token-2' to be 'token-1' // Object.is equality`,
  test time **64ms**. Its 8 sibling tests in the same file all passed (1ms–106ms) in the same run,
  so this is a single-test failure, **not load** (load inflates every test in a run).
  Seen in `pr-preview-3701-qgbkc-unit-tests`; did NOT fire in the later
  `pr-preview-3701-vh54x-unit-tests` (849 files / 12679 tests, 0 failures).
- **Ruled out:** load (whose-time-moved: only this test moved). Not caused by any PR in this
  session — it touches none of these files.
- **Leading hypothesis:** a real-timer TTL race — the test asserts a cached value is still
  `token-1` after a delay that is not deterministically shorter than the TTL. Same family as
  #3689, whose fix was a virtual clock. Unowned.
- **Next probe:** read the test and check whether it uses real timers:
  `command grep -n "useFakeTimers\|setTimeout\|TTL\|advance" src/server/orchestrator/__tests__/orchestrator-token-cache.test.ts`
  If real timers, port the #3689 pattern (`vi.useFakeTimers({toFake:['setTimeout','clearTimeout','setInterval','clearInterval']})`
  + `afterEach(() => vi.useRealTimers())` — an `afterEach`, NOT a `finally`, which does not run on test timeout).

### `preview / smoke-tests` was red on EVERY PR, then recovered unexplained
- **Symptom + exact repro:** every PR in the repo, ~5 consecutive, red on one assertion.
- **Observed (with values):** `tests/preview-image-feed.spec.ts:53:7 › images browse feed renders
  real content (tester) › image.getInfinite (the /images feed via meili) returns a non-empty page`,
  failing all 3 retries with `Error: the /images feed (meili-backed) should render >= 1 image`
  → `expect(received).toBeGreaterThan(expected)`. Confirmed byte-identical on #3690, #3700,
  #3708, #3707, #3695 — PRs sharing no code.
  Then on #3713 (`891cf751be`): `preview / smoke-tests=success — 61 passed, 0 flaky`.
- **Ruled out:** any PR's own changes (identical failure across unrelated PRs).
  **Ruled out:** that it is the same problem as #3711 — that is a Playwright/Chromium
  revision skew in the COMPONENT suite, a different failure entirely (see Gotchas).
- **Leading hypothesis:** the preview environment's Meilisearch index was empty and was
  reseeded by something upstream. **Nobody diagnosed the cause; nobody owns it.**
- **Next probe:** if it recurs, check the preview env's meili index doc count before
  assuming it is a test bug. The assertion is a real product signal — a permanently-red
  version of it is worse than none.

### #3685 — `OPEN_BUZZ_PURCHASE` has two behaviours depending on host
- **Symptom:** after #3680, `PageBlockHost` NACKs a pre-ready request (settles in a tick) while
  `IframeHost.tsx` (~:1196, same closed-over-`status` shape at ~:1072/:1094) still **silently drops**
  it, hanging the block's promise for the SDK's full 30s. `useBuzzPurchase` is host-agnostic.
- **Ruled out as a non-issue:** `hostHandlerParity.ts` records only request/reply **types**, so
  nothing in the repo will catch this divergence.
- **Next step:** not a mystery — it is scoped work. Filed as #3685 with a suggested fix
  (extend parity to "refusal path answers on the same requestId").

## Next steps (ranked)

1. ~~**Post-deploy verify #3680 and #3713.**~~ **#3680 DONE** (verified live on prod, above).
   **#3713 is BLOCKED on a release** — it is not in prod, so there is nothing to verify yet.
   The real next step is to decide whether to cut a release: the Forgejo downgrade is live in
   production today, and every `civitai app pull` an author runs strips their own push access.
   Recipe for the check once released is above; do NOT run it against prod before then.
2. **#3685** — fix the `IframeHost` side, or accept the divergence explicitly.
3. **`orchestrator-token-cache`** flake — see probe above.
4. **Worktree sweep**: `git -C <civitai> worktree list | wc -l` → **251**. Four hold real
   uncommitted work including two edited `.claude/skills/*/SKILL.md` files
   (`buzz-debezium-kafka-replication`, `civitai-axiom-loki-phase4`,
   `civitai-blocks-impl` untracked `drain-wildcard-audit-orphans.ts`, `civitai-cli-helpnotice`
   modified `internal/cmd/root.go` + `update_notice.go`). Other sessions' — ask before touching.
5. **#3711** (another session's, DO NOT MERGE as-is) needs a companion infra pin — see Gotchas.

## Gotchas / decisions / dead-ends

- 🔴 **"Merged to `main`" ≠ "in production" in this repo, and the earlier revision of this
  doc silently assumed it did.** Prod = `release` branch → `civitai-prod` image; `main` →
  `civitai-web` → `next.civitai.com` (which runs `IS_PREVIEW=true` against the **production
  database**). PR previews at `pr-<N>.civitaic.com` are torn down within the hour by a
  cronjob, so after a merge there is nothing left to verify against except next/prod.
  Deploy chain is Tekton → GHCR → Flux image automation → Flagger canary (prod only);
  the operator doc is `civitai/.claude/skills/deploy-status/SKILL.md`.
- **`civitai app dev-tunnel` is the cheapest way to test the REAL prod host.** It frames your
  local page inside production `PageBlockHost` with your real session — no app submission, no
  review. Two traps hit this session: a bare `python3 -m http.server` is CORS-blocked (the
  tunnel's own embeddability preflight, item 10, caught it — serve
  `Access-Control-Allow-Origin: *`), and **do not wrap the tunnel in `timeout N`** — it killed
  the tunnel mid-test.
- **`pgrep -f <pat>` matched ONLY this session's own shell** when hunting the tunnel process —
  the documented self-match trap, reproduced verbatim. Resolve PIDs via `/proc/<pid>/cmdline`.
- **zsh ate a git pathspec again:** `git show $D:src/...` → `bad substitution`, because `:s`
  parses as a history modifier. It printed `0` for every content check — a **wrong answer that
  looks like a measurement**, not an error. Brace it: `${D}:src/...`.
- **CI migrated to Tekton mid-session.** GitHub Actions check-runs are gone. Statuses are now
  `tekton / typecheck` + `preview / {deploy,render-check,unit-tests,component-tests,auth-tests,smoke-tests}`.
  A 38-deep GH Actions queue looked like a capacity problem; it was a **dead queue on a
  decommissioned system**. PRs predating the cutover need a push to pick up Tekton.
- **`git checkout -B <branch>` is REFUSED if the branch is checked out in another worktree —
  and a following `git merge` then runs against the WRONG HEAD and still prints "merged clean".**
  Two of four branches were silently no-ops this way. Always verify `behind_main=0` per branch
  before pushing.
- **A Tekton webhook can be silently dropped.** #3689's push landed (remote head correct, PR
  `updated_at` matching the others to the second) but produced no pipelinerun while 4 siblings did.
  Fix: empty commit to re-fire.
- **Reading CI logs:** `KUBECONFIG=$KC_DPPROD kubectl logs -n tekton-builds <taskrun>-pod --all-containers`.
  Traps: `tail -1` on a taskrun list does NOT sort by age — sort by `.metadata.creationTimestamp`.
  The taskrun reports `Succeeded` even when a report-only step FAILED. Logs age out.
- **`grep` in the Claude Code tool shell is a shell FUNCTION** (from `~/.claude/shell-snapshots/`)
  routing to an **embedded ugrep with `-I`**, so it returns empty + rc=1 on a file containing a
  NUL — indistinguishable from "not present". `command grep` bypasses it (GNU grep 3.12, which
  is loud: `binary file matches` on stderr, rc=0). **Recursive ripgrep silently omits such a file
  entirely.** `ugrep` is not on PATH; it lives inside the claude binary.
- **#3711 is NOT the meili smoke problem.** It is a Playwright/Chromium revision skew:
  `playwright-core@1.57.0`→chromium **1200**, `@1.61.1`→**1228**; the NixOS host ships 1228.
  Failure mode is `Test Files (130) / Tests no tests / Duration 153ms` — **0 of 130 files ran,
  reported as not-a-failure**. Merging it alone breaks the CI smoke job, whose container image
  is still pinned to the 1.57 line; needs a companion pin in a private infra repo.
- **Why `git ls-files` was abandoned in the #3701 guard:** the Tekton workspace `/workspace/source`
  is not a usable git checkout, so the guard collected **0 tests** and reported a collection error,
  not a test failure. Replaced with a filesystem walk (single path, no git-available fallback —
  two paths drift and the fallback would be the one CI exercises and nobody tests).
- **Rejected:** granting `write` on an unobservable permission read in #3713. Measured:
  `admin`→`write` is a 204 that **lowers**, so it does not close the TOCTOU window, and it
  silently escalates a `read` caller to push access.
- **Rejected:** an ESLint rule flagging one-shot postMessage stimuli in browser tests. After
  #3680 fixed the root cause and `test/commitWindow.tsx` gave a deterministic drive, the rule
  would gate against a shape the codebase no longer produces.
- **Do not re-derive:** #3659's add/add conflict on `src/server/utils/client-ip.ts` was resolved
  by ANOTHER session and merged. Both `resolveClientIp` (attribution label, always returns) and
  `getTrustedClientIp` (validated address or `null`, for enforcement) are on `main`, deliberately.

### The pattern worth carrying forward
Every PR audited this session had a real defect, and in **every** case it was in the EVIDENCE,
not the code: a mutation result asserted rather than run; a positive control validating a
re-implementation instead of the real code path (`Buffer.from('a\0b').includes(0)` vs the actual
scan function); a test fake that keyed state by user alone so it structurally could not
distinguish repos; an "environment can't run this" excuse that dissolved when someone fixed the
worktree (16/16 passed; typecheck baseline 6, not 40). Four different authors. Audit the proof,
not just the diff.

## How to verify

```bash
R=/home/zach/workspace/civit/civitai
git -C $R fetch origin main -q

# #3680 — statusRef in the RENDER BODY (not an effect)
git -C $R show origin/main:src/components/AppBlocks/PageBlockHost.tsx | command grep -B2 "statusRef.current = status"

# #3701 — guard present, service clean
git -C $R show origin/main:src/__tests__/source-nul-bytes.test.ts | command grep -c "walkSourceFiles"
# NB: block-registry.service.ts is under services/ ; only forgejo.service.ts is under services/blocks/
python3 -c "import subprocess;print(subprocess.run(['git','-C','$R','show','origin/main:src/server/services/block-registry.service.ts'],capture_output=True).stdout.count(b'\x00'))"  # expect 0

# #3713 — grant-at-least present, false 422 comment gone
git -C $R show origin/main:src/server/services/blocks/forgejo.service.ts | command grep -c "permissionRank"
git -C $R show origin/main:src/server/services/blocks/forgejo.service.ts | command grep -c "422 if already a collaborator"  # expect 0
```

Note `command grep` above is deliberate — see the ugrep gotcha.
