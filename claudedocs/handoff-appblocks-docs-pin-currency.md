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

🔴 **This is a NEW arc.** `appblocks-agent-dx` CLOSED 2026-10-01 (`cli#764` merged
`3c7366404` as its last PR). These were its leftovers — see
`claudedocs/handoff-appblocks-agent-dx.md` in this same repo for that arc.

- **closing-condition:** `check` — both asks are LIVE on the public site, with a control
  proving the fetch is answering:
  ```bash
  curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c "baseModelGroup: 'SDXL'"        # 0
  curl -sS https://developer.civitai.com/site/guide/cli-troubleshooting.md | grep -c 'listing-completeness gate'  # >= 1
  curl -sS https://developer.civitai.com/apps/reference/hooks.md | wc -c                                   # CONTROL: ~45k, not ~200
  ```
  🔴 Merged is NOT live. FROZEN at round 1: the upstream README fix and the `@civitai/sdk`
  bump are NEW items, not extensions of this line.

🔴 **VERDICT: ADDRESSED — ARC CLOSED 2026-10-01T23:31Z.** `#136` merged (`cbfab1e3`,
23:23:38Z) on 16/16 terminal-green checks, deployed, and verified LIVE by polling:
`hooks.md` 42,265 B / sdxl=**2** → 46,590 B / sdxl=**0**, `cli-troubleshooting.md`
21,192 B / gate=**0** → 21,223 B / gate=**1**, control `messages.md` answering at
17,377 B. Independently re-read after the poll loop exited. **Anything outstanding below
is a NEW arc, not another round of this one.**

## State now

- 🔴 **ARC CLOSED.** `civitai-developer-docs#136` **MERGED** `cbfab1e3` 23:23:38Z,
  squash, branch deleted; **DEPLOYED AND VERIFIED LIVE** (numbers in `## Goal`).
  Both closing-condition halves measured on the public site, with a size control and a
  pre-merge positive control (`sdxl=2, gate=0`) proving the probe could see the defect.
- 🔴 **THE DEPLOY LAG IS MEASURED, NOT ASSUMED: ~8 MINUTES.** 23 consecutive polls at
  20 s intervals returned the OLD page unchanged after the merge; `hooks.md` flipped on
  poll 24 and `cli-troubleshooting.md` on poll 25, ~20 s apart. A session reporting this
  closed off the merge alone would have been wrong for that whole window.
- **`civitai-app-starters#508` — STILL OPEN**, now two commits: `6cea118` (prose
  retraction) and **`9f1bd12`** (the EXAMPLE, which the audit caught). Patch changeset
  included. `claim-work` slug `appblocks-agent-dx-5` is still HELD for it.
- **`cli#765` — STILL OPEN**, carries this handoff doc.
- **`cli#764` merged** `3c7366404` 22:06:44Z; `main`'s copy of the predecessor handoff is
  now the authoritative 519 lines (it was a stale 455 before).
- **Claims released:** `appblocks-agent-dx-2` and `-3` (both landed in `#136`).
- **Two audits ran on `#136`** — round 0 (requirements/deletion, reported twice, re-run at
  `09867e7`) and the nine axes. Round 0 verdict `deletion candidate — D4 + D7`, 7
  candidates, `requirements: 5 (unattributed: 0)`. Nine-axes verdict **merge after fixing
  🔴 1**, which was the upstream example — fixed in `#508` `9f1bd12` before the merge.
- **Two public corrections posted on `#136`** (`#issuecomment-5942203718`,
  `#issuecomment-5942313185`) for false claims in my own commit messages. Neither is
  amendable in place — that would force-push and void a green 16/16 CI run.
- **No `clawgate-task:` field:** `clawgate_handoff.sh resolve` exited 5. That cannot
  distinguish "touched no task" from "wrong session id"; it is not a clean bill of health.
- ⚠ **`cairn.civitai.com` is returning HTTP 503 (no available server)**, served from an
  18-minute-old cache. Both store writes LANDED on the `personal` instance (verified by
  reading the synced cache) but their automated post-write validation was UNCONFIRMED
  twice. Not this arc's problem; recorded because the next writer will hit it.

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

🔴 **Rank 1 is CLOSED and is kept in place so the numbering stays stable** — a rank is
half a `claim-work` claim's identity, so renumbering re-points every live claim.

1. ✅ **CLOSED 2026-10-01T23:31Z — `#136` merged, deployed and verified live.** Evidence
   in `## Goal` and `## State now`. Re-check with `## How to verify` §0 if ever in doubt.
   forcing: gate — the hardcoded-`SDXL` defect was live on developer.civitai.com until this merged and deployed; it is now measured gone
2. **Merge `starters#508`, confirm the Release run publishes `blocks-react@0.61.1`, then
   re-bump the docs pin and regenerate.** 🔴 The live page TODAY still carries the false
   "currently **required**" sentence AND an example that always passes the filter — the
   operator shipped `#136` knowing this, on the measured grounds that it is strictly
   better than the hardcoded `'SDXL'` it replaced. This is the other half.
   Closing condition: `npm view @civitai/blocks-react version` ≥ `0.61.1`, AND
   `curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c 'currently \*\*required\*\*'` → 0,
   AND that page's checkpoint example no longer passes `baseModelGroup` unconditionally.
   IN FLIGHT: civitai-app-starters#508
   forcing: regression — the published page teaches a filter trap the hook's own `.d.ts` names as a trap
3. **`@civitai/sdk` 0.7.0 → 0.10.0 in `civitai-developer-docs`.** 🔴 NOT a number bump:
   `scripts/check-appblocks-pins.mjs`'s header says a lagging pin here is the signal to
   RE-READ `apps/guide/porting.md`'s release-scoped prose. And
   `scripts/check-design-system-pins.mjs:212` scopes `SDK_PACKAGES` to `app-sdk` +
   `blocks-react` only, so **`@civitai/sdk` literals are UNGUARDED** — `porting.md:6`
   stamps `npm:@civitai/sdk@0.5.0` against a 0.7.0 pin and nothing flags it, while `:33`
   and `:361` are changelog prose needing `HISTORICAL_LITERALS` rows.
   Closing condition: `npm run check:pins` reports 0 lagging, AND every `@civitai/sdk`
   version claim in `apps/guide/porting.md` re-read against the 0.10.0 `.d.ts`.
   forcing: gate — `check:pins` is red on this and the scheduled drift sweep (issue #130) cannot go green until it lands
4. **Fix the generator truncation that drops most of every hook's safety caveats.**
   🔴 The nine-axes audit's highest-leverage structural finding, and it is NOT this PR's
   regression: `scripts/gen-appblocks-hooks.mjs:37` takes
   `prose = text.slice(0, text.indexOf('```tsx'))`, so everything after a README section's
   FIRST fence is discarded — measured **~65,327 chars and 31 🔴 markers across 20 of 33
   hook sections** at 0.61.0 (worst: `useDomainMaturity`, 3,628 B published against a
   29,261 B section). `useCivitaiRoute` now publishes with 4 of its 5 caveats gone,
   including "PAGE SLOT ONLY" and the `''`-is-both-a-route-and-the-pre-init-sentinel trap.
   Closing condition: `curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c 'Page slot only'` ≥ 1.
   forcing: regression — the published reference omits documented footguns the `.d.ts` carries, on 20 of 33 hooks
5. **Give the new consent-retry text a field table and a live `{@link}` target.**
   `ConsentRetryOptions` occurs EXACTLY ONCE in the whole docs corpus — the `{@link}` that
   names it — and `SubmitWorkflowOptions extends ConsentRetryOptions` while the generated
   table lists only `idempotencyKey?`, because `describeType` collects own members, not
   inherited ones. So `submit(body, { autoRequestConsent: false })` appears NOWHERE on the
   site, and the exclusion contract (one retry never a loop; a timeout with no idempotency
   key is NOT retried because that would be a real second write) is dropped. MONEY PATH.
   Fix is not one line: `REACT_TYPES` in `scripts/gen-appblocks-bridge.mjs` resolves
   against `dist/hooks/useBuzzWorkflow.d.ts` only (`:158`, `:187`) and
   `ConsentRetryOptions` lives in `dist/hooks/consentRetryOptions.d.ts`, so the source file
   must widen too. That generator's own comment at `:69-75` states the rule being broken.
   Closing condition: `curl -sS https://developer.civitai.com/apps/reference/generation.md | grep -c autoRequestConsent` ≥ 1 with a field row, not only a `{@link}`.
   forcing: regression — a documented money-path opt-out is unreachable from the published docs
6. **Correct the stale `check:ds-pins` claim in the talos-infra skill.**
   `<talos-infra>/.claude/skills/manage-appblocks-docs/reference/doc-audit-lessons.md:170`
   asserts the `--offline` variant "is the PR-gating one and **is green**" — true of `main`,
   false of any pin-bump branch, and it reads as licence not to run the gate that caught
   this session. Also add the missing `sources:`-stamp step to `SKILL.md:142`.
   Closing condition: that sentence no longer asserts the gate is green.
   forcing: regression — the doc actively mis-routes the next agent doing this exact task

## Defects (batched)

- 🔴 **The nine-axes audit's 🔴: the disclosed residual was NARROWER than the defect, and
  my `#508` fix initially missed the half that matters.** `#508` corrected the README prose
  and left the section's sole EXAMPLE unconditionally passing
  `baseModelGroup: context.checkpoint.baseModel` under the comment *"Derive the family …
  never a literal"* — precisely the trap `useCheckpointPicker.d.ts:6-11` names (*"Passing
  the family you are already in is therefore a trap — it makes the picker offer only the
  ecosystem the user is trying to leave"*). The `.d.ts`'s FIRST `@example` is the
  unconstrained default; the derived form is its SECOND, conditional one. Fixed in
  `9f1bd12`. 🔴 **The lesson: in an arc whose finding is "a weak model copies the nearest
  example", fixing the prose and leaving the example is fixing the wrong half.**
- 🔴 **Two false claims in my own `#136` commit messages**, both corrected publicly and
  both unfixable in place: `ecd284b` called `apps/reference/generation.md` "bridge-table
  prose rewording only" when the regeneration added THREE substantive consent/money claim
  blocks (1,205 chars of pure insertion, 0 removals); `09867e7` wrote the generated
  region's LINE COUNT (832, as printed by `gen:appblocks:md`) as its END LINE — the region
  is 61–896 at that head and 61–825 at the merge-base, so 832 is true at no ref.
- 🔴 **Round 0's D4 + D7** (its verdict): the 20 hand-maintained `sources:` stamps on the
  three GENERATED pages are duplicated state the generator should own (ranks 4/6 territory
  — it is the root cause of this session's only real defect), and `SDK_PACKAGES` omits
  `@civitai/sdk` entirely (rank 3).
- **Round 0's D5/D6** — the pin-bump recipe lives in two places and both are wrong
  (rank 6).
- **`cli-troubleshooting.md` ↔ `publishedTroubleshootingRows` is unguarded on BOTH sides.**
  Zero scripts/workflows in `civitai-developer-docs` reference either name, and the
  cli-side ledger's own header says *"Nothing in this repository can prove the ledger still
  matches the live page."* The hand sync is the only option today but it will desynchronise
  again silently. ⚠ And it is currently **AHEAD of every shipped CLI** — see Gotchas.
- 🟢 **`test:snippets:appblocks`'s 44/44 is not coverage of this change** and I quoted it
  without that qualifier. Fences inside `<!-- BEGIN GENERATED -->` are deliberately
  excluded (`scripts/typecheck-appblocks-snippets.mjs:213-240`); the two covered `hooks.md`
  snippets are at `:930` and `:946`, outside the region. The count is true and says nothing
  about the regenerated examples.
- 🟢 **`blocks-react@0.61.0` wants `@civitai/components ^0.9.0` against the repo's `0.8.1`
  pin**, so `npm ci` installs a nested duplicate. Probed and benign: `styles.css` is
  byte-identical between 0.8.1 and 0.9.0, the `data-civitai-ui` selector set is identical,
  and the site imports only `@civitai/components/styles.css`. Noted because `check:ds-pins`
  grades literals against `package.json` and is structurally blind to a transitive
  requirement the pin no longer satisfies.
- 🟢 **`apps/reference/hooks.md:716` ships a markdown link as literal syntax.** The island
  interpolates `{{ h.description }}`, so the first hook description to carry a link renders
  as visible `[...](...)` in the built HTML — and it is the only cross-reference between
  the two halves of the new routing pair.

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

- 🔴 **THE DEPLOY LAG IS ~8 MINUTES AND IT IS MEASURED.** 23 polls at 20 s after the
  `#136` merge returned the old page byte-for-byte unchanged; `hooks.md` flipped on poll 24
  and `cli-troubleshooting.md` on poll 25. **Take a PRE-merge baseline as the positive
  control** — here `sdxl=2, gate=0` — so the later zero is a measurement rather than a
  hope, and **gate on page SIZE too**, or an error page reads as success.
- 🔴 **THE TROUBLESHOOTING PAGE IS NOW AHEAD OF EVERY SHIPPED CLI, and the PR body said the
  opposite.** `cli#762`'s merge `a355137` is **not** an ancestor of `v0.1.111`
  (`git describe` → `v0.1.111-11-ga355137`, i.e. 11 commits past the tag), so the
  listing-completeness gate the clause documents is UNRELEASED. Compounding it,
  `apps/reference/cli.md` is generated from a **0.1.109** snapshot and
  `git grep allow-incomplete-listing -- '*.md'` returns **0**, so a reader following the
  clause to the `app submit` reference finds neither the gate nor its opt-out flag.
  Operator decision 2026-10-01: ship as-is — the clause is true about the code and keeps
  the page in sync with the vendored mirror. 🔴 **"The mirror already carried it" is NOT
  evidence the behaviour shipped** — that was the reasoning error.
- 🔴 **A README's PROSE and its EXAMPLE are two separate artifacts and a fix to one is not
  a fix to the other.** See Defects. The generated public page takes the DESCRIPTION from
  the README and the EXAMPLE from the README, independently; correcting one leaves the
  other teaching the old thing, and the example is what gets copied.
- 🔴 **`grep -oE '\{\+[^}]*\+\}'` over `git diff --word-diff` UNDER-COUNTS when the inserted
  text contains `}`** — the character class terminates on it. Reported **1** addition in
  `generation.md` where there were **3**, and that wrong count nearly shipped as a public
  correction. Use `difflib` over the two blobs, or isolate each changed row's common
  prefix/suffix.
- 🔴 **`${PIPESTATUS[0]}` IS EMPTY IN zsh** — it is `$pipestatus` (lowercase array) here. A
  gate's rc read that way comes back blank, which reads as neither pass nor fail.
- 🔴 **A `cairn append` can LAND while its post-write validation is UNCONFIRMED, and the
  cause can be an unrelated instance.** `hygiene.sh` refuses to check when its pre-check
  `cairn sync` exits 4, and the aggregate sync exits 4 if ANY configured instance fails —
  here `cairn.civitai.com` answered HTTP 503 while `personal` (the instance actually
  written) was live and current. **Confirm by reading the synced cache for the bullet's own
  text; do NOT retry the write.**
- 🔴 **A store bullet asserting a fix "retracted in #N" when #N is UNMERGED reads as done
  forever.** Caught on my own append and corrected with a `cairn put` adding `OPEN:` — the
  measured failure is an entry serving a remedy as current for 22 days.
- **Decision: ship `#136` with two known-stale upstream claims, fix upstream after.**
  Operator's call, re-confirmed 2026-10-01 AFTER the audit widened the residual from "one
  false sentence" to "the headline instruction plus the committed example". The measured
  grounds: every step strictly improves the page — hardcoded `'SDXL'` (pins every viewer to
  one family regardless of context) → derived-from-context (the milder trap) → omit-by-
  default once rank 2 lands.
- **Both audits were dispatched WITHOUT `isolation: "worktree"`**, deliberately: the cwd is
  `datapacket-talos`, a different repo, so that flag would have worktreed the wrong one.
  Each agent ran the cross-repo recipe itself against a `refs/audit/` ref.

## How to verify

```bash
D=~/workspace/civit/civitai-developer-docs; ST=~/workspace/civit/civitai-app-starters

# 0. THE ARC'S CLOSING CONDITION — MET 2026-10-01T23:31Z, re-runnable any time
curl -sS https://developer.civitai.com/apps/reference/hooks.md | grep -c "baseModelGroup: 'SDXL'"               # 0
curl -sS https://developer.civitai.com/site/guide/cli-troubleshooting.md | grep -c 'listing-completeness gate'   # 1
curl -sS https://developer.civitai.com/apps/reference/messages.md | wc -c                                       # CONTROL ~17k

# 1. the repo-local half, in a CLEAN worktree off the merge commit
WT=/tmp/v136; git -C $D worktree add --detach $WT cbfab1e3
(cd $WT && npm ci && npm run build)
(cd $WT && npm run check:ds-pins -- --offline)     # rc 0 — the gate that caught this session
(cd $WT && npm run check:md-regions && npm run check:porting-hooks && npm run test:snippets:appblocks)
git -C $D worktree remove --force $WT

# 2. the premise, measured on the PUBLISHED tarball rather than inferred
cd /tmp && npm pack @civitai/blocks-react@0.61.0 >/dev/null 2>&1 && tar xzf civitai-blocks-react-0.61.0.tgz
find package -type f -print0 | xargs -0 grep -ao "baseModelGroup: 'SDXL'" | wc -l   # 0
find package -type f -print0 | xargs -0 grep -ao 'baseModelGroup' | wc -l           # 39 — CONTROL

# 3. rank 2's state (normalised — the claim WRAPS, a line grep lies)
git -C $ST show origin/zach/blocks-react-readme-retract-required:packages/civitai-blocks-react/README.md \
  | perl -0777 -ne 's/\s+/ /g; print scalar(()=/omit the key, or pass a family/g), "\n"'   # 1
git -C $ST show origin/zach/blocks-react-readme-retract-required:packages/civitai-blocks-react/README.md \
  | grep -c 'DEFAULT — pass no baseModelGroup'                                              # 1
```
