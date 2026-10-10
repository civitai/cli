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
- **Server (civitai/civitai), MERGED to `main` 2026-10-10 23:04Z, NOT yet in `release`** (latest release 5.1.224 `c3243bc3f4` at time of writing):
  - #5682 `9f1e5e089c` — `TokenScope.AppBlocksSubmit` (1<<25, carried by every `civitai login` token: `internal/appapi/oauth.go` `deviceScopeBase`) now admits a scoped token to `appFeedback.listForListing`, `countNewForMyListings`, `setOwnerStatus`, `flagAbusive`; `hasAnyForListing`, `getEligibility`, `create`, `mod*` stay session/Full-only. Consent label reworded to "Manage your Apps: submit versions, pull source, test locally, edit listings, view analytics and respond to user feedback". Existing CLI tokens gain the access on deploy without re-consent (operator accepted).
  - #5695 `58f03dab5a` — `getSessionFromBearerToken` only accepts `System|User|Access` key rows; an OAuth refresh token is no longer a bearer credential (found by #5682's audit; it predated it).
  - Audits: #5682 round 0 + rounds 1–2 (clean at 2); #5695 rounds 0–1 (clean at 1). Records are PR comments.
- **CLI: civitai/cli#802 OPEN, audit-clean** (rounds 0–3, clean at 3; records are PR comments), head `587209b0fe` on `zach/app-feedback-command`, worktree `/home/zach/workspace/civit/cli-app-feedback`. `civitai app feedback <slug>` (+ `set-status`, `flag`, `--count`), reporter hidden by default (`--with-reporter`), `untrustedMessage` + notice in a versioned `--json` envelope, terminal sanitising, triage recipe in `civitai agent setup`'s block. CI: everything green except `pins-vs-published`, which also fails on sibling PRs #799/#800 (npm published newer `@civitai/*` than the scaffold pins; #802 touches no pins).
- **Held on purpose:** merging #802 before the server change is deployed would make the agents-block recipe tell every project's agent to run a command a default login gets 403 on (round-0 finding). A live read with the OAuth login on 2026-10-10 returned the scope refusal (403, exit 3) — that is the expected pre-deploy state.

## Next steps (ranked)
1. **Wait for a civitai `release` containing `9f1e5e089c` to deploy to production**, then re-run the read: `civitai app feedback metadata-medic --status all --json` with the default login. Check: `git -C $CIVITAI merge-base --is-ancestor 9f1e5e089c origin/release`, then confirm the production deploy carries it (the operator's deploy-verification runbook). Cutting a release is the civitai team's call, not this arc's. forcing: deadline — #802 cannot be verified or merged until then
2. **Merge #802** once step 1's read returns the verification row; then the closing-condition check needs a RELEASED CLI build (check the cli repo's release process — `claudedocs/release-*-draft.md`). forcing: user
3. **Fix the cli `pins-vs-published` red** (separate PR; fails on every open PR). forcing: gate — every cli PR reports a red check for files it does not touch

## Defects (batched)
- 🟢 accepted (#5695 r1): the new bearer-token test's fake returns whole rows and ignores `select`; a mutant dropping `type: true` from the select survives. Predates the PR.
- 🟢 accepted (#5682 r0): existing CLI tokens gain inbox read/triage without re-consent; the weaker alternative (token may only set `acknowledged`) was not taken — operator chose full writes.

## Gotchas / decisions / dead-ends
- Operator decisions (2026-10-10): full writes with no confirmation flag; reporter hidden by default; reuse the analytics scope (no new bit, no re-login).
- Round-0 dispositions on #802: kept `--count`, the read-first lookup, the 2000-row cap, stricter `--json` escaping, the duplicated tRPC helpers; deleted two generic recipe bullets and trimmed decision-40/AGENTS.md weight (AGENTS.md 30,359 of 30,500 B).
- `set-status` past the 2000-row lookup cap advises a per-status `--all` read, then `--expect`; a row older than the newest 2000 of its status is reachable only with `--expect`.
- The Flipt flag gates only SENDING feedback; owner-side reads ignore it, so any owner with a deployed-scope CLI login can list their inbox.

## How to verify
```bash
git -C $CIVITAI fetch -q origin release && git -C $CIVITAI merge-base --is-ancestor 9f1e5e089c origin/release && echo "server change in release"
civitai app feedback metadata-medic --status all --json   # default `civitai login`; expect the abfb-verify row, untrustedMessage field
```
