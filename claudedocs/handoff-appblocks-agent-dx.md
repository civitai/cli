# Handoff: appblocks-agent-dx — 2026-10-01

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

Close three defects the operator observed while weaker LLMs built App Blocks: the resource
picker taught a hardcoded ecosystem, scope failures did not prompt-and-retry, and apps shipped
with no listing media and no description. All three turned out to be **teaching/sequencing**
defects, not missing capability.

- **closing-condition:** `check` — `cli#760` is merged AND a freshly scaffolded `page-money`
  app can reach every checkpoint ecosystem from its "Change model" button. Mechanically:
  `gh pr view 760 --repo civitai/cli --json state` reads `MERGED`, and
  `git -C <cli> show origin/main:internal/scaffold/templates/page-money/src/App.tsx.tmpl | grep -c 'baseModelGroup: checkpoint.baseModel'`
  returns **0** for the checkpoint call site (the LoRA site keeps its filter deliberately).
  This is frozen as the condition this arc was opened on.

## State now

✅ **7 of 8 PRs MERGED**, each verified by CONTENT on the target `main` (never by ancestry — a
squash merge makes `git merge-base --is-ancestor` permanently false):

| PR | repo | sha | content check that passed |
|---|---|---|---|
| `#761` | cli | `d8e5fc0f` | scaffold pins `^0.53.0` / `^0.60.0` |
| `#499` | starters | `1bad5de4` | guard present; 0 hardcoded `'SDXL'` in picker examples |
| `#5262` | civitai | `e432c508` | comment reads "NOT a subset and NOT none" |
| `#501` | starters | `08318a6c` | optional signature ×2; 0 × "currently REQUIRED"; host split ×5 |
| `#759` | cli | `69f9c00d` | picker gotcha in the managed block; decision-36 exception recorded |
| `#500` | starters | `55bc9aa4` | `withConsentRetry` present; `estimate()` NOT routed; changeset `major` |
| `#762` | cli | `a355137b` | submit listing gate; `TaglineFromName`; `DOGFOOD_ALLOW_LISTING_TEXT` ×4 |

**`cli` main: 12/12 green** at `a355137b`. **`starters` main: 20 checks, 1 red** — the
pre-existing release job (see the open investigation).

🔴 **`cli#760` is the ONLY open PR and it is correctly a DRAFT.** It cannot merge until
`@civitai/blocks-react` publishes. ⚠ **Its PR body names `0.61.0` and that is WRONG** — `#500`
is a `major`, so Version PR `starters#502` proposes `blocks-react 0.60.0 → 1.0.0` and
`app-sdk 0.53.0 → 0.54.0`. Its scaffold pin needs **`^1.0.0`**. Re-derive from the released
version, never from either PR's text.

⚠ **No `clawgate-task:`** — `resolve` reported `NOTHING RESOLVED`; an unknown session id answers
200 with an empty array, so that zero cannot distinguish "touched no task" from "wrong id".

## Open investigations — live diagnosis state

### 🔴 OPEN — the starters release pipeline fails on npm auth, so nothing can publish
- as-of: 2026-10-01

- **Symptom + exact repro:** the `Version + publish packages` check is RED on
  `civitai-app-starters` `main`. Reproduce by reading it on any recent main sha:
  `gh api repos/civitai/civitai-app-starters/commits/main/check-runs --jq '.check_runs[]|select(.name|test("Version"))|.conclusion'`
- **Observed (with values),** from job `110164556993` (run `36797615833`, 00:43:51→00:54:34Z),
  log fetched with `gh api .../logs --allow-escape-sequences` (83,912 bytes — 🔴 **without that
  flag it returns 0 bytes and any grep over it is meaningless**):
  - `WARN Issue while reading "/home/runner/work/_temp/.npmrc". Failed to replace env in config: ${NODE_AUTH_TOKEN}`
  - `ERROR: PUBLISH DID NOT HAPPEN — a package version in this tree is not on the registry.`
  - the job's own text: a failed publish and a version npm **STAGED** instead
    "produces exactly the same mix", and `E409 Cannot publish over previously staged version`
    can only be resolved by a human. `via: measurement`
- 🔴 **Ruled out — that this session's merges caused it.** Control: the same check is `failure`
  on `38ee13a`, the merge-base predating all 8 PRs, and on `1bad5de4` and `08318a6c`.
  Pre-existing. `via: measurement`
- **Leading hypothesis:** `NODE_AUTH_TOKEN` is not reaching the runner's `.npmrc`, so the
  publish cannot authenticate. The repo's own guard then correctly refuses to report success.
- **Next probe — needs a human, per the job's own instructions:** run `npm whoami` first
  (an `E401` there means not logged in and is an answer about the MACHINE, not the registry),
  then `npm stage list` to tell a failed publish from a staged version. Then re-run the release
  workflow. ⚠ `starters#502` currently shows `total=0` check-runs — the documented
  bot-authored-PR `action_required` state, which needs a workflow approval before it can merge.

### 🔴 OPEN — the submit gate cannot catch a FIRST submit, by construction
- as-of: 2026-10-01

- **Symptom:** `cli#762`'s listing gate blocks an incomplete listing from the **second** submit
  onward only.
- **Observed:** a first submit is what MINTS the listing, so before it `listMine` returns no row
  and the gate passes. The measured trial (`at3`) submitted first. `via: code`
- 🔴 **Ruled out — that blocking onsite text would fix it.** An onsite tagline is re-derived from
  the manifest **only at approve time**, so refusing the submit refuses the one act that fixes
  it — permanently. The gate asks the local manifest whether the fix is already in the bundle.
  `via: code`
- **What reaches a first-time app instead:** `#762`'s scaffold tagline (`TaglineFromName`), which
  makes the manifest-governed field non-empty from `civitai app create` onward.
- **Next probe:** decide whether `app doctor` should be wired into the scaffold's own
  `npm run` flow (pre-submit), which is the only remaining surface that reaches a first submit.
  Not started; no owner.

## Next steps (ranked)

1. **Unblock the release.** Resolve the npm-auth failure above, approve and merge
   `starters#502`, then confirm BOTH versions are actually on npm
   (`npm view @civitai/blocks-react version` → `1.0.0`, `npm view @civitai/app-sdk version` → `0.54.0`).
   🔴 A merged Version PR is not a publish — read the registry, not the PR.
   forcing: gate — `cli#760` cannot merge until this lands, and `starters` main is red on it
2. **Then land `cli#760`.** Bump its scaffold pin to the ACTUAL published major (`^1.0.0`, not
   the `0.61.0` its body names), re-run `CIVITAI_SCAFFOLD_TYPECHECK=1 go test ./internal/scaffold
   -run TestPageMoneyScaffoldTypechecksItsOwnShippedTests` (it PASSES on main and FAILED on #760's
   head — that inversion is the gate), undraft, merge. Files: `package.json.tmpl`, `App.tsx.tmpl`,
   `models.ts.tmpl`. IN FLIGHT: civitai/cli#760
   forcing: gate — the arc's closing condition names this PR
3. **Fix the published CLI troubleshooting page.** `cli#762` updated the vendored mirror in
   `publishedTroubleshootingRows`; the live page needs the same one-clause edit in
   `civitai/civitai-developer-docs` (`site/guide/cli-troubleshooting`).
   forcing: regression — the published page now contradicts the shipped CLI's refusal list
4. **Make `#499`'s fix reach actual readers.** Merging it did NOT change the published page.
   `civitai-developer-docs` pins `@civitai/blocks-react` EXACT at `0.59.0`, the generator reads
   the PUBLISHED tarball (README primary, `@example` fallback), and
   `apps/reference/hooks.md:583,597` is a COMMITTED generated region still carrying both
   `baseModelGroup: 'SDXL'` literals. Chain: publish → bump that pin → `npm run gen:appblocks:md`
   → commit → deploy. Closing condition: `git grep "baseModelGroup: 'SDXL'" -- apps/reference/hooks.md`
   in `civitai-developer-docs@origin/main` returns nothing.
   forcing: gate — the defect is live on developer.civitai.com until this completes

## Defects (batched)

- `cli#759`'s picker-call ledger growth half is SPELLING-dependent: a picker reached through an
  alias not ending in `Picker`, or a call whose argument object starts on the next line, both
  survive a full `go test ./internal/cmd/`. The shrink half is sound.
- Nothing ratchets the size of the generated `AGENTS.md` block. The repo owns the mechanism
  (`agents_size_test.go`, with a ceiling AND a can-still-fire test) and points it at its own
  `AGENTS.md`, not at the block written into every user's project. `AGENTS.md` item 36's trigger
  list does not name the Gotchas list, so the next gotcha addition routes to no decision doc.
- `civitai`'s `component` (browser) test tier is UNGATED in CI — no project selector matches it;
  its only home is the report-only `preview / component-tests`, which needs a `preview` label.
  `#5262`'s new assertion is mutation-validated locally and runs in no CI job.
- `starters`' `pnpm typecheck` EXCLUDES `test/`, and vitest's oxc strips types without checking,
  so every test file in `civitai-blocks-react` is only syntax-checked.
- `packages/civitai-components/src/version.generated.ts` is stale in git on `starters` main
  (committed `0.8.1`, its `package.json` says `0.9.0`), so any `pnpm build` dirties the tree.
  Three independent agents hit it.
- `cli#762`'s `dropIncompatibleLoras` family comparison is a SNAPSHOT deny-list; the SDK/host
  affords no media-type filter (pinned by the host's own ledger test).

## Gotchas / decisions / dead-ends

### The pattern behind all three defects — the SDK was right and the TEACHING was wrong

- 🔴 **Every one of the three reported defects was a docs/sequencing defect, not missing
  capability.** `useResourcePicker`'s `baseModelGroup` was ALREADY optional and documented as
  "omit for an unconstrained pick"; `requestConsent` and `useConsentUnavailable` already existed;
  `app doctor` already detected all five listing problems and named each remedy; `civitai generate`
  already supports NanoBanana (Gemini) and OpenAI image models. **A weak model does not read API
  surface and infer a flow — it copies the nearest example.** So the highest-leverage surface is
  examples and scaffold, judged by whether a model copying them verbatim gets working code.
- 🔴 **A FALSE COMMENT IN THE HOST WAS THE ROOT CAUSE OF THE WHOLE CHECKPOINT CHAIN.**
  `PageBlockHost.tsx`'s resource-picker and checkpoint-picker handlers were BYTE-IDENTICAL —
  `const groupKey = baseModelGroup ? getBaseModelGroup(baseModelGroup) : null` — while their
  comments contradicted: the checkpoint one claimed `baseModels:[]` yields "no checkpoints rather
  than all families". Three layers prove `[]` means NO narrowing. That false comment justified the
  SDK's `required`, which locked every scaffolded app into its starting ecosystem, and `#499` was
  one merge from publishing the same reasoning as settled fact. Fixed in `#5262`.
- 🔴 **An empty string is NOT "unconstrained", and the mechanism is HOST-SPECIFIC.**
  `getBaseModelGroup('')` returns `'Other'` — a REAL ecosystem key with non-empty members — so on
  the **model slot** `''` NARROWS. On the **page host** `pageBlockHostLogic.ts:343-346` drops the
  field on `length === 0`, so `''` ≡ omission. A WHITESPACE-ONLY string narrows on BOTH. Always
  OMIT the key. ⚠ An earlier version of this sentence stated the model-slot behaviour as universal
  and shipped it; it was corrected in `#499` (README) and `#501` (JSDoc).

### Process traps this session actually hit

- 🔴 **A rebase reported "Successfully rebased" while SILENTLY DROPPING 2 of 3 commits.**
  Resolving a conflict with `git checkout --theirs <file>` on commit 1 of 3 made the later two
  vanish, including a correction routed specifically to that file. **Reading the merged region is
  what caught it; the rebase's exit status said nothing.** Resolve by writing the branch TIP's
  content, then verify the final file is byte-identical to that tip.
- 🔴 **A MUTANT CAN BE TOO WEAK, and the survival then reads as isolation.** Moving an idempotency
  key mint inside a retried closure as `options?.idempotencyKey ?? generate()` PRESERVES a
  caller-supplied key, so only one test died and that was misread as "the guard is isolated".
  The honest mutant (always mint fresh) kills 2 on the bridge rail and 3 on the tip rail.
- 🔴 **A GREP CANNOT TELL AN ASSERTION FROM A QUOTED RETRACTION.** After `#5262` merged, grepping
  for the retracted string returned 1 and read as "the fix did not land" — it was the fix QUOTING
  the dead claim so nobody re-derives it. Read the operative text, not a count.
- 🔴 **Distinguish a slow CI job from a stuck one by its OWN history, not by re-polling.**
  `README snippets (typecheck)` takes 9.7/11.1/11.5 min; at 4.2 min it is simply not done.
  One wall-time comparison answers it; re-polling leaves you guessing.
- ⚠ **`gh pr view` reports the PRE-PUSH head for minutes.** After a force-push it showed
  `CONFLICTING/DIRTY` at the old sha. `git ls-remote` is authoritative on the ref; poll until the
  PR's `headRefOid` actually refreshes before reading any rollup.
- ⚠ **`merge-tree --write-tree` is the only reliable conflict check** — branch on its EXIT CODE.
  It returned 1 for `#501` while GitHub still said `UNKNOWN`.

## How to verify

```bash
CLI=~/workspace/civit/cli; ST=~/workspace/civit/civitai-app-starters; CIV=~/workspace/civit/civitai

# all 7 merged, by CONTENT (ancestry is permanently false after a squash)
git -C $ST show origin/main:packages/civitai-blocks-react/src/hooks/useCheckpointPicker.ts \
  | grep -cE 'baseModelGroup\?: string;'                      # want 2
git -C $ST show origin/main:packages/civitai-blocks-react/src/hooks/useCheckpointPicker.ts \
  | grep -c 'currently REQUIRED'                               # want 0
git -C $CLI cat-file -e origin/main:internal/cmd/app_submit_listing_gate.go && echo gate-present
git -C $CLI show origin/main:internal/scaffold/templates/page-money/block.manifest.json.tmpl | grep -c tagline  # want 1

# the release blocker
gh api repos/civitai/civitai-app-starters/commits/main/check-runs \
  --jq '.check_runs[]|select(.name|test("Version"))|"\(.name) \(.conclusion)"'
npm view @civitai/blocks-react version      # 1.0.0 once the publish succeeds
npm view @civitai/app-sdk version           # 0.54.0

# the scaffold typecheck gate — it SELF-SKIPS without the env var, and `make ci` never sets it,
# so a green suite does NOT prove the rendered scaffold builds
(cd $CLI && CIVITAI_SCAFFOLD_TYPECHECK=1 go test ./internal/scaffold \
  -run TestPageMoneyScaffoldTypechecksItsOwnShippedTests -v)   # PASS, never SKIP
```
