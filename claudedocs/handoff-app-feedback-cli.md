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
- **ARC CLOSED 2026-10-11** — closing condition met: released `civitai 0.1.118` (installed from the published release asset `civitai_0.1.118_linux_amd64`, `checksums.txt` OK), default OAuth login (`whoami` → "OAuth login"), `civitai app feedback metadata-medic --status all --json` → exit 0, output contains row id 92 with `untrustedMessage` = `abfb-verify page-host desktop 20261010T225306Z — automated verification, safe to ignore`.
- Landed: civitai/civitai #5682 `9f1e5e089c` + #5695 `58f03dab5a` (production release 5.1.225, canaries Succeeded 2026-10-11T00:04:35Z); civitai/cli #797 (squash `eb11c31`, scaffold pins app-sdk ^0.63.0 / blocks-react ^0.67.0) and #802 (squash `67b82f1`); release `v0.1.118` published 2026-10-11T01:50:45Z — GitHub (14 assets), npm `@civitai/cli` latest = 0.1.118, Homebrew cask version 0.1.118.

## Next steps (ranked)
1. **Merge civitai/cli#803** (automated re-vendor of the embedded manifest schema; canonical gained `analytics`). `main`'s `schema-drift` check is red until it lands, and the CLI's local manifest validation does not know `analytics` (same gap in v0.1.117 — not a regression of v0.1.118). Per its body, decide whether `internal/validate/` should track the new field. forcing: gate — `schema-drift` red on cli `main`

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

- `npm view @civitai/cli version` read 0.1.117 for a few minutes after `Release npm` reported success; the publish step had logged `Publishing to https://registry.npmjs.org/ with tag latest`. Registry propagation lag, not a failed publish — re-read `https://registry.npmjs.org/@civitai/cli` dist-tags before diagnosing.
- In-app "Send feedback to developer" is a ⋮-menu item (`src/components/AppBlocks/ChromeFeedbackEntry.tsx` in civitai), not a dedicated chrome icon button; hidden for owners/collaborators, unapproved listings, muted users, users without Apps-store access, and when the app-feedback Flipt area flag is off for the user.

## How to verify
```bash
civitai --version   # 0.1.118 or later
civitai app feedback metadata-medic --status all --json   # default `civitai login`; expect the abfb-verify row (id 92)
```
