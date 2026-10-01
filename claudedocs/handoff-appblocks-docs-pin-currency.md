# Handoff: appblocks-docs-pin-currency — 2026-10-01

## Run this first — the index, one command
```bash
$DEVRC/scripts/cairn-ops/read.sh recall --repo "/home/zach/workspace/civit/civitai-developer-docs"
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Make two already-identified `civitai-developer-docs` defects reach actual readers, both
unblocked by the 2026-10-01 publish of `blocks-react@0.61.0` / `app-sdk@0.54.0`: the
generated hooks reference still taught a hardcoded `baseModelGroup: 'SDXL'`, and the
published CLI troubleshooting page contradicted the shipped CLI's refusal list.

🔴 **This is a NEW arc.** `appblocks-agent-dx` CLOSED 2026-10-01 (its
`closing-condition` ADDRESSED; `cli#764` merged `3c7366404` at 22:06Z as its last PR).
These were its leftovers, not a continuation of it — see
`claudedocs/handoff-appblocks-agent-dx.md` in this same repo for that arc.

- **closing-condition:** `check` — both asks are LIVE on the public site, with a control
  proving the fetch is answering:
  ```bash
  curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c "baseModelGroup: 'SDXL'"        # 0
  curl -sS https://developer.civitai.com/site/guide/cli-troubleshooting.md | grep -c 'listing-completeness gate'  # >= 1
  curl -sS https://developer.civitai.com/apps/reference/hooks.md | wc -c                                   # CONTROL: ~45k, not ~200
  ```
  🔴 Merged is NOT live — this repo is branch-tracked (`ref: branch: main`) so a merge
  triggers a deploy that takes minutes. The predecessor arc measured twins still at
  21 B / 30 B ninety seconds after merge. FROZEN at round 1: the upstream README fix
  and the `@civitai/sdk` bump below are NEW items, not extensions of this line.

## State now

- **Branch / PR:** `civitai-developer-docs` `zach/docs-pin-bump-0610-and-cli-troubleshooting`
  → **#136**, head `09867e7`, `mergeable=MERGEABLE mergeState=CLEAN` (re-read after `#135`
  merged 22:26Z and moved the base). NOT merged, NOT deployed.
- **Upstream PR:** `civitai-app-starters` `zach/blocks-react-readme-retract-required` →
  **#508**, head `9fcb0c4`-era commit, patch changeset included. NOT merged.
- **DONE — `cli#764` merged** (`3c7366404`, 22:06:44Z), closing out the predecessor arc.
  It had read BLOCKED only because `build-test` was still `in_progress`; SHA-pinned, all
  12 checks were terminal `success`.
- **DONE — #136 commit 1 (`ecd284b`):** pins `blocks-react 0.59.0→0.61.0`,
  `app-sdk 0.52.0→0.54.0` + lock; regenerated `apps/reference/{hooks,messages,generation}.md`;
  added a `useCivitaiRoute` row to `apps/guide/porting.md`; applied `cli#762`'s one-clause
  listing-completeness-gate edit to `site/guide/cli-troubleshooting.md`.
- **DONE — #136 commit 2 (`09867e7`):** re-stamped **23** pin literals across 12 files
  that commit 1 staled. See the `check:ds-pins` gotcha below — this is the one real defect
  of the session and CI caught it, not me.
- **DONE — `starters#508`:** retracted `packages/civitai-blocks-react/README.md`'s
  `useCheckpointPicker` "currently **required**" clause AND restored its missing
  "omit the key" advice, + a patch changeset.
- **DONE — public correction posted** on #136
  (`#issuecomment-5942203718`): the commit message and PR body both called
  `apps/reference/generation.md` "bridge-table prose rewording only", which is FALSE.
- **IN FLIGHT — the nine-axes audit of #136 had NOT reported when this was written.**
  Round 0 DID report, twice (it re-ran at `09867e7` after being told the head moved). Verdict:
  `deletion candidate — D4 + D7`, 7 candidates, `requirements: 5 (unattributed: 0)`. It
  independently reproduced the stale-stamp regression with a **pin-revert control** — PR pins →
  exit 1 / 23 violations, base pins with nothing else changed → exit 0 — so the diff, not the
  base, staled them. All candidates are in `## Defects (batched)` and ranks 3–5.
- **CI at `09867e7`: 16/16 success**, all terminal, `completed_at` 22:44–22:46Z, same check count
  as `ecd284b` so nothing is unregistered. `mergeable=MERGEABLE mergeState=CLEAN` re-read after
  `#135` moved the base. 🔴 `CLEAN` is a conflict verdict, NOT a CI-settle signal.
- **Deploy/verify status:** nothing merged, nothing deployed, **nothing verified against the live
  site.** Every green is a LOCAL run plus CI; the closing condition is unrun by construction.
- **No `clawgate-task:` field:** `clawgate_handoff.sh resolve` exited **5** (nothing
  resolved). That cannot distinguish "touched no task" from "wrong session id", so no field
  was written — it is not a clean bill of health.

## Open investigations — live diagnosis state

### `apps/reference/generation.md` gained three un-reviewed money/consent claims via regeneration
- as-of: 2026-10-01
- **Symptom + exact repro:** the regenerated region adds behavioural claims about when a
  viewer is CHARGED, which no human in this arc reviewed, and which the PR body actively
  told reviewers to skip. `git -C <docs> diff cb2d43c..09867e7 -- apps/reference/generation.md`
- **Observed (with values):** 1,205 chars of pure insertion, **zero** removals, 107 distinct
  new tokens, at three sites — on `estimate`: *"🔴 NO AUTOMATIC CONSENT PROMPT HERE…"* (388
  chars); on `submit`: *"🔴 CONSENT IS HANDLED FOR YOU… re-sends the submit ONCE — with the
  SAME `idempotencyKey`, so the two attempts are one reservation, not two… Opt out with
  `autoRequestConsent: false`"* (530 chars); on `idempotencyKey?`: *"…an error you receive may
  already be a second attempt's"* (287 chars). `via: measurement`
- **Ruled out — that the CONTENT is wrong or hand-written.** It is generator output from
  `blocks-react@0.61.0`'s own type declarations and `check:md-regions` is rc 0. The defect was
  the DESCRIPTION, now corrected publicly. `via: command`
- **Leading hypothesis:** nothing is broken; the claims are probably correct. But "probably"
  is doing work here — a money/consent claim published to developer.civitai.com deserves one
  person reading it against `node_modules/@civitai/blocks-react/dist/hooks/useBuzzWorkflow.d.ts`
  before it ships, and nobody has.
- **Next probe:** read those three inserted blocks against that `.d.ts` and confirm each
  claim, in particular that the auto-retry really does reuse the minted `idempotencyKey`
  (the "two attempts are one reservation" claim is the expensive one to be wrong about).

## Next steps (ranked)

1. **Read the nine-axes audit of #136, fix what it finds, then merge and verify live.**
   Repo `civitai-developer-docs`, PR **#136**. The audit was dispatched against `ecd284b`
   and told that the head moved to `09867e7`. After merging, the closing condition above is
   the gate — and merged is not live.
   IN FLIGHT: civitai-developer-docs#136
   forcing: gate — the hardcoded-`SDXL` defect is live on developer.civitai.com until this merges and deploys
2. **Merge `starters#508`, confirm the Release run publishes `blocks-react@0.61.1`, then
   re-bump the docs pin and regenerate.** Until that round-trip completes, #136 publishes a
   `useCheckpointPicker` description that (a) falsely says `baseModelGroup` is required and
   (b) never tells the reader it can be omitted. Operator decided 2026-10-01 to ship #136
   anyway and fix upstream after; this is the "after".
   Repo `civitai-app-starters` → then `civitai-developer-docs`.
   Closing condition: `npm view @civitai/blocks-react version` ≥ `0.61.1`, and
   `curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c 'currently \*\*required\*\*'` → 0.
   IN FLIGHT: civitai-app-starters#508
   forcing: regression — #136 knowingly publishes a false claim about a parameter's optionality until this lands
3. **`@civitai/sdk` 0.7.0 → 0.10.0 in `civitai-developer-docs`.** 🔴 NOT a number bump:
   `scripts/check-appblocks-pins.mjs`'s own header says a lagging pin here is the signal to
   RE-READ the release-scoped prose in `apps/guide/porting.md`, which asserts things like
   *"That is new in 0.5.0. `0.4.0` threw from `initialize()` instead"*. Compounding it,
   `scripts/check-design-system-pins.mjs:212` scopes `SDK_PACKAGES` to `app-sdk` +
   `blocks-react` only, so **`@civitai/sdk` literals are UNGUARDED** — `porting.md:6` stamps
   `npm:@civitai/sdk@0.5.0` against a 0.7.0 pin today and nothing flags it.
   Closing condition: `npm run check:pins` reports 0 lagging, AND every `@civitai/sdk`
   version claim in `apps/guide/porting.md` re-read against the 0.10.0 `.d.ts`.
   forcing: gate — `check:pins` is red on this and the scheduled drift sweep (issue #130) cannot go green until it lands
4. **Make `gen-appblocks-md.mjs` own the `sources:` stamp for the pages it generates.**
   Round 0's D3, and the root cause of this session's only real defect: for a page whose body
   is generated from the pin, the stamp IS the pin and carries no human judgement, yet only a
   maintainer's memory keeps it current. Keep `check:ds-pins` for hand-authored pages, where a
   stamp is a real provenance claim.
   Closing condition: a pin bump + `npm run gen:appblocks:md` leaves `check:ds-pins --offline` rc 0 with no hand edit.
   forcing: regression — the next pin bump reddens `typecheck-snippets` exactly as this one did
5. **Correct the stale `check:ds-pins` claim in the talos-infra skill.**
   `<talos-infra>/.claude/skills/manage-appblocks-docs/reference/doc-audit-lessons.md:170`
   asserts the `--offline` variant "is the PR-gating one and **is green**" — true of `main`,
   false of any pin-bump branch, and it reads as licence not to run the gate this session
   broke. Also add the missing `sources:`-stamp step to that skill's pin-bump recipe at
   `SKILL.md:142`.
   Closing condition: `git grep -n 'is green' <talos-infra>/.claude/skills/manage-appblocks-docs/reference/doc-audit-lessons.md` returns nothing for that claim.
   forcing: regression — the doc actively mis-routes the next agent doing this exact task

## Defects (batched)

- **Round 0's D3** — the 20 hand-maintained `sources:` stamps on the three GENERATED pages are
  duplicated state; generator should own them. (Promoted to rank 4 because it is the root cause
  of the session's defect, not a nit.)
- **Round 0's D4** — `@civitai/sdk` in `TRACKED_PACKAGES` is a permanently-red gate shape: its
  red cannot be cleared by a pin bump (its header says the lag is a signal to re-read prose), so
  the scheduled sweep and its aggregate issue **#130** stay red until rank 3 lands. Consider an
  explicit dated deferral, the pattern `check-design-system-pins.mjs` already ships as
  `HISTORICAL_LITERALS`.
- **Round 0's D5** — the stale "is green" sentence (rank 5).
- **`cli-troubleshooting.md` ↔ `publishedTroubleshootingRows` is unguarded on BOTH sides.** Zero
  scripts/workflows in `civitai-developer-docs` reference either name, and the cli-side ledger's
  own header says *"Nothing in this repository can prove the ledger still matches the live page."*
  The hand sync in #136 is the only option today but it is UNGUARDED, not guarded elsewhere — it
  will desynchronise again silently.
- **`apps/reference/hooks.md`'s nested ```` ```md ```` fences went 2 → 3.** Pre-existing generator
  shape, NOT introduced here (2 at the merge-base, 3 at head, same shape). Unjudged: whether the
  published page renders the `useCivitaiNavigate` description as prose or as a code block.
- **Round 0's D6** — the pin-bump recipe lives in TWO places and both are wrong: fold the
  `sources:`-stamp step into `<talos-infra>/.claude/skills/manage-appblocks-docs/SKILL.md:142` and
  correct `reference/doc-audit-lessons.md:170`. Rank 4 (D3/D4) makes both unnecessary.
- **Round 0's D7** — `check-design-system-pins.mjs:212` `SDK_PACKAGES` omits `@civitai/sdk`, so
  `apps/guide/porting.md:6` stamps 0.5.0 against a 0.7.0 pin and three body claims scoped to
  0.5.0/0.4.0 are pinned by nothing. This is the deferred sdk work's actual content (rank 3).
- 🔴 **Two false claims in my own commit messages, both now public and unfixable in place**
  (amending would force-push and void a green 16/16 CI run): `ecd284b` called
  `apps/reference/generation.md` "bridge-table prose rewording only" when the regeneration added
  three substantive consent/money claim blocks; and `09867e7` says the generated region is
  "lines 61-832" — **832 is the region's LINE COUNT, as printed by `gen:appblocks:md`, not its end
  line**, so that figure is true at no ref. Both are corrected in the PR comment thread. The
  substantive claim 832 supported (that `:906` sits outside the region) is correct at both refs.

## Gotchas / decisions / dead-ends

- 🔴 **`typecheck-snippets` RUNS TWO GATES AND ONLY ONE IS THE SNIPPET TYPECHECK.**
  `.github/workflows/appblocks-snippets.yml` runs `npm run test:snippets:appblocks` (44/44 green
  locally) AND, in the same job, `npm run check:ds-pins -- --offline` — a different script. A
  local green on the first says nothing about the second. **On any pin bump, run
  `check:ds-pins -- --offline` before pushing.** This cost one red CI run.
- 🔴 **A pin bump stales every provenance stamp that quotes the pin — 23 of them here.** Fix them
  LINE-TARGETED from the gate's own violation list, never a global sed:
  `check-design-system-pins.mjs` deliberately exempts 4 HISTORICAL literals by registry
  (`responsive.md:183` components@0.4.0; `generation.md:45,243` app-sdk@0.30.0; `generation.md:283`
  app-sdk@0.43.0), each naming a real ARRIVAL version that a blanket replace would make false.
  The helper used: `scratchpad/bump_stamps.py`, which parses the gate's own output and asserts
  each replacement lands exactly once on its own line or aborts before writing.
- 🔴 **A frontmatter `sources:` stamp is NEVER exemptible, and the gate says so.** It declares
  which published version the page was written against — provenance, not a changelog fact. The
  only valid action is to set it to the pin. Adding a `HISTORICAL_LITERALS` row will not silence it.
- 🔴 **Re-stamping a version on hand-authored prose makes it a claim about the NEW version.**
  `hooks.md:906` (OUTSIDE the generated region, which is lines **61–896** at `09867e7` and
  61–825 at the merge-base) asserts *"three `kind`
  values… four members in all"*. Re-derived at `app-sdk@0.54.0`: `dist/blocks/types.d.ts:1262`
  gives `WorkflowBody = …TextToImage | …CustomComfy | …Step | …PassThroughStep`, `CustomComfy =
  Recipe | Inline` (`:1049`), two step arms (`step: string` `:1105` vs `step?: undefined` `:1182`)
  — three `kind` values, four table rows. Claim held; only the version moved. Do this check, do
  not carry the count over.
- 🔴 **`grep -oE '\{\+[^}]*\+\}'` over `git diff --word-diff` UNDER-COUNTS when the inserted text
  contains `}`** — the character class terminates on it. It reported **1** addition in
  `generation.md` where there were **3**, and that wrong count nearly shipped as a public
  correction. Use `difflib` over the two blobs, or isolate each changed row's common
  prefix/suffix.
- 🔴 **A claim that WRAPS across lines is invisible to a line-based grep, which returns a
  confident ZERO on a file you just fixed.** Hit while verifying `starters#508`: the restored
  "omit the key, or pass a family…" spans a line break, so `grep -c` said 0 on correct text.
  Verify prose fixes over whitespace-NORMALISED text (`perl -0777 -ne 's/\s+/ /g'`).
- 🔴 **A grep cannot tell an assertion from a quoted retraction.** After fixing `starters#508`
  the tree still had one `currently **required**` hit — the changeset QUOTING the retracted
  claim so nobody re-derives it. Read the operative text, not a count.
- **`gen:appblocks` does NOT rewrite the committed markdown — `gen:appblocks:md` does.** The
  umbrella writes the gitignored `public/appblocks/*.json`; the committed regions need the
  separate `gen:appblocks:md` step. Running only the umbrella leaves `hooks.md` untouched and
  looks like the bump changed nothing.
- **`check:snapshots` is red and is NOT yours.** It diffs `appblocks-snapshots/` against upstream
  GitHub and reads neither `node_modules` nor any file a pin bump touches — pre-existing by
  construction. Round 0 independently confirmed this disposition.
- **Decision: ship #136 with a known-false sentence, fix upstream after.** Operator's call,
  2026-10-01. The trade: 6 hardcoded-`SDXL` examples that produce visibly broken pickers, against
  one false sentence in a paragraph whose actionable advice is correct. Rank 2 is the other half.
- **`useCivitaiRoute` porting verdict** = **Keep** — it reads the host-owned sub-path from the
  handshake and `ROUTE_CHANGED`; off the bridge your own router owns the URL, so there is nothing
  to replace. Mutation-verified by round 0 both directions (row deleted → exit 1 naming the hook).
- **Both audits were dispatched with `--repo`-style cross-repo worktrees, NOT
  `isolation: "worktree"`** — the cwd is `datapacket-talos`, a different repo, so that flag would
  have worktreed the wrong one.

## How to verify

```bash
D=~/workspace/civit/civitai-developer-docs; ST=~/workspace/civit/civitai-app-starters

# 0. THE ARC'S CLOSING CONDITION — only meaningful AFTER #136 merges AND deploys
curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c "baseModelGroup: 'SDXL'"               # 0
curl -sS https://developer.civitai.com/site/guide/cli-troubleshooting.md | grep -c 'listing-completeness gate'   # >= 1
curl -sS https://developer.civitai.com/apps/reference/hooks.md | wc -c                                          # CONTROL ~45k

# 1. the repo-local half, in a CLEAN worktree off the PR head (not the primary clone)
WT=/tmp/v136; git -C $D worktree add --detach $WT origin/zach/docs-pin-bump-0610-and-cli-troubleshooting
(cd $WT && npm ci && npm run build)
(cd $WT && npm run check:ds-pins -- --offline)     # rc 0 — the gate that caught this session
(cd $WT && npm run check:md-regions && npm run check:porting-hooks && npm run test:snippets:appblocks)
(cd $WT && npm run check:agent-md && npm run check:built-site)
git -C $D worktree remove --force $WT

# 2. the premise, measured on the PUBLISHED tarball rather than inferred
cd /tmp && npm pack @civitai/blocks-react@0.61.0 >/dev/null 2>&1 && tar xzf civitai-blocks-react-0.61.0.tgz
find package -type f -print0 | xargs -0 grep -ac "baseModelGroup: 'SDXL'" | grep -v ':0$' | wc -l   # 0
find package -type f -print0 | xargs -0 grep -ao 'baseModelGroup' | wc -l                          # 39 — CONTROL

# 3. the upstream retraction (normalised — the claim WRAPS, a line grep lies)
git -C $ST show origin/zach/blocks-react-readme-retract-required:packages/civitai-blocks-react/README.md \
  | perl -0777 -ne 's/\s+/ /g; print scalar(()=/omit the key, or pass a family/g), "\n"'            # 1
```
