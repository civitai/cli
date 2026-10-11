# Handoff: app-feedback-cli — 2026-10-10

## Run this first — the index, one command
```bash
$DEVRC/scripts/cairn-ops/read.sh recall --repo "/home/zach/workspace/civit/cli"
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal
Operator ask (2026-10-10): "wire the cli so app owners can use agents to pull feedback and use it to improve their apps". Per-app feedback itself shipped and was verified end to end on 2026-10-10 (that arc is closed; its handoff lives in the operator's infra repo).
- **closing-condition:** `check` — civitai/cli#802 is merged AND a released `civitai` CLI, logged in with a default `civitai login` (OAuth, not a Full personal key), runs `civitai app feedback metadata-medic --status all --json` against production and the output contains the row whose `untrustedMessage` is `abfb-verify page-host desktop 20261010T225306Z — automated verification, safe to ignore` — run and read by a session, not inferred from code.

## State now
- **Server: DEPLOYED and serving.** civitai release 5.1.225 (`2e71928c9c`) contains `9f1e5e089c` (`merge-base --is-ancestor` rc=0); dp-prod canaries `civitai-dp-prod` + `-api` both `Succeeded` 2026-10-11T00:04:35Z; 155 serving pods on `civitai-prod:20261010234137-2e71928`, 8 old `c3243bc` pods draining at read time.
- **Live read VERIFIED with the PR-head build (not yet a released CLI).** `go build ./cmd/civitai` from #802 head `ef142c9`, default OAuth login (`whoami` → "Credential Type: OAuth login"), `civitai app feedback metadata-medic --status all --json` → exit 0, envelope `schemaVersion 1`, `fetched 1`, row id 92 with `untrustedMessage` = `abfb-verify page-host desktop 20261010T225306Z — automated verification, safe to ignore`. The pre-deploy 403 is gone. Installed `civitai` is 0.1.113, which lacks `app feedback`.
- **#802 merge blocked by the REQUIRED check `pins-vs-published`** (branch protection on cli `main` requires it, plus scaffold-currency, build-test, ready-ack-runtime, template-page-vite). Operator chose (2026-10-11): land #797 first, then update #802 and merge. No admin-merge.
- **#797 (`automation/bump-scaffold-pins`) refreshed:** its 2026-10-09 green went red on re-run (npm published `@civitai/app-sdk` 0.63.0, `@civitai/blocks-react` 0.67.1). Pushed `42aa9bc` (`go run ./internal/scaffold/cmd/bump-pins`: app-sdk ^0.63.0, blocks-react ^0.67.0; token set unchanged, 36). `CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1` → `--- PASS` (not skip) locally; red at `47bf9e0` in CI re-run 2026-10-11T00:45:44Z. IN FLIGHT: civitai/cli#797 CI on `42aa9bc`. Worktree `/home/zach/workspace/civit/cli-pins-797` (remove after #797 merges).

## Next steps (ranked)
1. **Merge #797 once CI on `42aa9bc` is all-terminal green** (≥13 check-runs; re-read `mergeStateStatus` = CLEAN). If pins-vs-published is red again, npm moved again: re-run `bump-pins` in the 797 worktree. IN FLIGHT: civitai/cli#797. forcing: gate — the required `pins-vs-published` check blocks every cli PR, #802 included
2. **`gh pr update-branch 802 -R civitai/cli`, wait for its checks on the new head SHA, merge #802** (squash; keep the default body so trailers survive). forcing: user — operator asked for #802 to merge once the abfb-verify row came back (it did, 2026-10-11)
3. **Closing-condition check needs a RELEASED CLI**: after #802 merges, cut/await a cli release per the cli repo's release process (`claudedocs/release-*-draft.md`), install it, and re-run the read with the default login. forcing: user — it is this arc's own frozen closing condition

## Defects (batched)
- 🟢 accepted (#5695 r1): the new bearer-token test's fake returns whole rows and ignores `select`; a mutant dropping `type: true` from the select survives. Predates the PR.
- 🟢 accepted (#5682 r0): existing CLI tokens gain inbox read/triage without re-consent; the weaker alternative (token may only set `acknowledged`) was not taken — operator chose full writes.

## Gotchas / decisions / dead-ends
- Operator decisions (2026-10-10): full writes with no confirmation flag; reporter hidden by default; reuse the analytics scope (no new bit, no re-login).
- Round-0 dispositions on #802: kept `--count`, the read-first lookup, the 2000-row cap, stricter `--json` escaping, the duplicated tRPC helpers; deleted two generic recipe bullets and trimmed decision-40/AGENTS.md weight (AGENTS.md 30,359 of 30,500 B).
- `set-status` past the 2000-row lookup cap advises a per-status `--all` read, then `--expect`; a row older than the newest 2000 of its status is reachable only with `--expect`.
- The Flipt flag gates only SENDING feedback; owner-side reads ignore it, so any owner with a deployed-scope CLI login can list their inbox.

- Server side (carried from the superseded State now; deployed in 5.1.225): #5682 `9f1e5e089c` — `TokenScope.AppBlocksSubmit` (1<<25, on every `civitai login` token via `internal/appapi/oauth.go` `deviceScopeBase`) admits `appFeedback.listForListing`, `countNewForMyListings`, `setOwnerStatus`, `flagAbusive`; `hasAnyForListing`, `getEligibility`, `create`, `mod*` stay session/Full-only. #5695 `58f03dab5a` — `getSessionFromBearerToken` accepts only `System|User|Access` key rows (OAuth refresh tokens are no longer bearer credentials). Audits: #5682 clean at r2, #5695 clean at r1 (PR comments).
- #802 was held until deploy on purpose: merging earlier would have the agents-block recipe send every project's agent to a 403 (round-0 finding).
- `pins-vs-published` is REQUIRED by branch protection on cli `main`, so its npm-driven red is a hard merge block, not cosmetic; the operator declined an admin-merge past it (2026-10-11).

## How to verify
```bash
gh pr view 797 -R civitai/cli --json state,mergeStateStatus
gh pr view 802 -R civitai/cli --json state,mergeStateStatus,mergedAt
civitai --version   # must be a release that includes #802
civitai app feedback metadata-medic --status all --json   # default `civitai login`; expect the abfb-verify row (id 92)
```
