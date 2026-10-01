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

- Branch / PR: **`starters#505` OPEN** (`zach/components-chat-registry-drift`, commit
  **`ab23b80`** — rebased onto `d1a4d12` after main moved; the pre-rebase `fe50b77` was green
  20/20 and the guard re-passed on the moved base) — the release-pipeline fix.
  **`starters#502` OPEN and READY**, head **`35057f5c`**. **`cli#760` OPEN, still a DRAFT.**
  This doc rides `cli` branch `zach/handoff-appblocks-agent-dx` → **`cli#763`**.
- ✅ **7 of 8 PRs MERGED**, each verified by CONTENT on the target `main` (never by ancestry — a
  squash merge makes `git merge-base --is-ancestor` permanently false). Carried forward
  verbatim from the previous round, NOT re-derived this session:

  | PR | repo | sha | content check that passed |
  |---|---|---|---|
  | `#761` | cli | `d8e5fc0f` | scaffold pins `^0.53.0` / `^0.60.0` |
  | `#499` | starters | `1bad5de4` | guard present; 0 hardcoded `'SDXL'` in picker examples |
  | `#5262` | civitai | `e432c508` | comment reads "NOT a subset and NOT none" |
  | `#501` | starters | `08318a6c` | optional signature ×2; 0 × "currently REQUIRED"; host split ×5 |
  | `#759` | cli | `69f9c00d` | picker gotcha in the managed block; decision-36 exception recorded |
  | `#500` | starters | `55bc9aa4` | `withConsentRetry` present; `estimate()` NOT routed; changeset `major` |
  | `#762` | cli | `a355137b` | submit listing gate; `TaglineFromName`; `DOGFOOD_ALLOW_LISTING_TEXT` ×4 |

  **`cli` main: 12/12 green** at `a355137b`.
- ✅ **The release blocker is DIAGNOSED and the fix is up.** It was never npm auth. The tree on
  `starters` main carried `@civitai/components-chat@0.1.0` while the registry held only
  `0.1.1`, so `pnpm assert:published` correctly failed `PUBLISH DID NOT HAPPEN` on every push
  since 2026-09-30T16:50Z. `#505` records `0.1.1`. Full mechanism in the new investigation
  block below.
- 🔴 **`cli#760`'s pin is `^0.61.0`, NOT `^1.0.0` — and this line was corrected mid-session
  because the BASE MOVED under the measurement.** `starters#504`
  (*"release blocks-react consent-retry as a minor, not a major"*) merged at
  **2026-10-01T02:35:38Z**, downgrading `#500`'s changeset from `major` to `minor`. `#502` was
  recomputed and at head `35057f5c` now proposes `blocks-react 0.60.0 → **0.61.0**`,
  `app-sdk → 0.54.0`, `sdk → 0.10.0`. **So `cli#760`'s PR body (`0.61.0`) is now RIGHT, and
  both this doc's earlier `^1.0.0` and the kickoff that asserted it are WRONG.** Neither was
  wrong when written — a human re-graded the changeset in between.
- ✅ **What survives that correction, because it is a property of the published artifact
  rather than of a changeset grade:** the published `0.60.0` tarball's
  `dist/hooks/useCheckpointPicker.d.ts` declares `open: (opts: { baseModelGroup: string; … })`
  — required, and `opts` itself required — so the optional signature is unreleased and
  `^0.60.0` cannot typecheck the scaffold. **Whatever the next version NUMBER is, the pin must
  be the version that first carries the optional signature.** 🔴 Re-derive it from
  `npm view @civitai/blocks-react version` AFTER the publish lands — never from a changeset
  grade, a Version PR diff, or any number written in this doc.
- 🔴 **`cli#760`'s branch is BEHIND `cli` main** and needs a rebase, not a hand-edit: its
  `internal/scaffold/templates/page-money/package.json.tmpl` still reads `^0.52.0`/`^0.59.0`
  where main (post-`#761`) reads `^0.53.0`/`^0.60.0`.
- Deploy/verify status: **nothing is published yet.** `npm view @civitai/blocks-react version`
  → `0.60.0`. `#505` is verified only against the guard it fixes (red at `origin/main`, green
  at `fe50b77`) and its own CI — not against a completed release.
- ⚠ **No `clawgate-task:`** — `resolve` reported `NOTHING RESOLVED` (rc 5) again. An unknown
  session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id". It is not a clean bill of health.
- ⚠ **An uncommitted `0.1.1` bump is sitting in the `starters` PRIMARY clone**
  (`packages/civitai-components-chat/package.json`) — the residue of the by-hand publish.
  `#505` supersedes it. That clone is also mid-conflict on
  `packages/civitai-components/src/version.generated.ts` (`UU`); left untouched.

## Open investigations — live diagnosis state

### ~~🔴 OPEN — the starters release pipeline fails on npm auth, so nothing can publish~~ RESOLVED 2026-10-01 — it is NOT auth
- as-of: 2026-10-01

🔴 **The heading and the leading hypothesis below were both WRONG, and the measurements are
kept because they are still the baseline.** The cause is a tree-vs-registry version drift on
`@civitai/components-chat` — see *the starters release blocker was a stale version number* below.
**Why the wrong reading was reachable:** the `NODE_AUTH_TOKEN` warning is the loudest line in
the log and reads as a cause, but it appears in **SUCCESSFUL** runs too (5 occurrences in run
`36745692381`, which published, against 7 in the failing one). `release.yml` ships no
`NPM_TOKEN` on purpose and publishes via OIDC, while `actions/setup-node` with `registry-url`
writes `_authToken=${NODE_AUTH_TOKEN}` unconditionally — so the warning is expected on every
run and discriminates nothing. **The probe that would have separated them in one command is a
control, not a deeper read of the failing log: fetch a SUCCESSFUL run's log and grep it for
the same line.**
The `Next probe` bullet that stood here has been deleted rather than struck through: it has
been carried out (both answers are in the new block) and re-running it would send the next
session back down the refuted path.

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
- **~~Leading hypothesis~~ REFUTED:** `NODE_AUTH_TOKEN` is not reaching the runner's `.npmrc`,
  so the publish cannot authenticate. Killed by the successful-run control above.
- ⚠ **Also wrong, and it was never the blocker it was recorded as:** *"`starters#502`
  currently shows `total=0` check-runs — the documented bot-authored-PR `action_required`
  state, which needs a workflow approval before it can merge."* Re-read 2026-10-01 at its head
  sha `acde776e`: **20 check-runs, all `success`**, `mergeStateStatus: CLEAN`,
  `mergeable: MERGEABLE`, not a draft. The zero was an **unregistered rollup read too soon**
  after the changesets action force-pushed the branch — CLAUDE.md's own fourth/sixth
  check-rollup shapes. Read it SHA-pinned via
  `gh api repos/<o>/<r>/commits/$SHA/check-runs`, never seconds after a push. `via: measurement`

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

### ✅ RESOLVED — the starters release blocker was a stale version number, not npm auth
- as-of: 2026-10-01

- **Symptom + exact repro:** `Version + publish packages` RED on `civitai-app-starters` `main`
  on every push since 2026-09-30T16:50:59Z. First failing run **`36747192297`** at `38ee13a8`,
  the `chore(release): version packages (#492)` merge; the run immediately before it
  (`36745692381`, `38d09078`) **succeeded**, which brackets the cause to that commit.
- **The cause, with values.** Tree-vs-registry version drift on exactly one package:

      @civitai/components-chat   tree origin/main 0.1.0   registry: 0.1.1 ONLY
      other 6 packages           tree == registry         all present

  `curl -o /dev/null -w '%{http_code}' https://registry.npmjs.org/@civitai%2Fcomponents-chat`
  → **200** (name known) and `…/components-chat/0.1.0` → **404**, against a positive control
  `…/theme/0.4.0` → **200**. That is `assert-published-versions.mjs`'s own
  *"name known, version absent → FAIL"* arm, i.e. the failed-publish signal, firing correctly.
  `via: measurement`
- **How the drift was created, in two steps.** (1) `0.1.0` was this package's **first ever**
  publish and it failed: `🦋 error an error occurred while publishing @civitai/components-chat:
  E404 Not Found - PUT https://registry.npmjs.org/@civitai%2fcomponents-chat`. Under npm OIDC
  trusted publishing a 404 on PUT means *unauthorized*, and it is unavoidable on a first
  publish — a trusted publisher cannot be configured for a package the registry has never
  heard of, so the OIDC handshake had nothing to match. `release.yml` names this exact
  signature in its own `setup-node` comment. The other five packages in that run published
  normally (`🦋 success packages published successfully:` lists app-sdk 0.53.0,
  blocks-react 0.60.0, components 0.9.0, components-react 0.9.1, sdk 0.9.0) because
  `changeset publish` continues past a failing package. (2) `0.1.1` was then published **by
  hand**: registry metadata gives `_npmUser.name = devzacx`, `time["0.1.1"] =
  2026-09-30T18:01:58.001Z`, and **no `dist.attestations`** — against `attestations=YES` +
  `_npmUser = "GitHub Actions"` on all six others, which is the discriminator between a CI
  OIDC publish and a laptop one. The matching version bump was never committed.
  `via: measurement`
- 🔴 **Ruled out — npm auth, the previous round's leading hypothesis.** The
  `WARN … Failed to replace env in config: ${NODE_AUTH_TOKEN}` line appears in the
  **SUCCESSFUL** run `36745692381` too — `grep -ac NODE_AUTH_TOKEN` gives **5** there against
  **7** in the failing run. The workflow ships no `NPM_TOKEN` by design (its env block says
  so) and publishes via OIDC, while `actions/setup-node` with `registry-url` always writes
  that `.npmrc` line. Expected on every run; discriminates nothing. `via: measurement`
- 🔴 **Ruled out — a STAGED version,** the other cause the guard states it cannot tell apart
  from a failed publish. `npm whoami` → **`devzacx`** (so not an `E401` machine answer), and
  `npm stage list @civitai/components-chat` / `@civitai/blocks-react` / `@civitai/app-sdk` all
  return `No staged versions of package name "…"`. ⚠ **`npm stage ls` is not a valid
  subcommand** — it prints `EUSAGE … Unknown subcommand` and **still exits 0**, so an `ls`
  spelling reads as a clean "nothing staged". Use `list`. `via: command`
- 🔴 **Ruled out — that the current red is a failing publish attempt.** The recent runs are
  **version** runs, not publish runs: with changesets pending the action only updates `#502`,
  and run `36804684761`'s log contains no `Publishing "@civitai/components-chat"` line at all.
  The sole failure is the assert step: `@civitai/components-chat@0.1.0 -> HTTP 404 after 60
  attempt(s)`. So "re-run the release workflow" could never have fixed it. `via: measurement`
- **Fix + the red→green matrix.** `starters#505` records `0.1.1` in `package.json` plus a
  CHANGELOG entry carrying this mechanism. 🔴 **`scripts/assert-published-versions.mjs` reads
  `package.json` from `HEAD`, not the working tree** — it prints `7 package(s) from HEAD
  (<sha>)` — so an uncommitted fix is invisible to it and the first run after editing looks
  like a failed fix. Committed:

      origin/main (2895c67):  @civitai/components-chat@0.1.0 -> HTTP 404   RC=1
      fe50b77:                OK 7/7 confirmed on the registry, 0 missing  RC=0

  `via: command`
- 🔴 **What `#505` does NOT fix, and it is the durable half.** `@civitai/components-chat` still
  has **no GitHub Actions trusted publisher** on npmjs.com. The next changeset that bumps it
  fails to publish with the same E404. Now that the package exists on the registry one can
  finally be configured — owner `civitai`, repo `civitai-app-starters`, workflow `release.yml`,
  environment blank, matching the six that work. **Operator-only** (npmjs.com UI + 2FA).
- **Next probe:** merge `#505`, then confirm main's Release run is green AND that the
  changesets action has recomputed `#502` off the fixed main — i.e. `#502`'s diff now carries
  `components-chat` at `0.1.1`. If it still carries `0.1.0`, merging it re-introduces the drift
  and main goes red again on the very next push.

## Next steps (ranked)

1. **Merge `starters#505`** (`zach/components-chat-registry-drift`). Then confirm main's
   Release run goes green, and that `#502` has been recomputed off the fixed main.
   IN FLIGHT: civitai/civitai-app-starters#505
   forcing: gate — `starters` main is red on this and nothing can publish until it is fixed
2. **Merge `starters#502`,** then verify at the **REGISTRY**, never the PR:
   `npm view @civitai/blocks-react version` → `0.61.0`, `npm view @civitai/app-sdk version` →
   `0.54.0`, `npm view @civitai/sdk version` → `0.10.0`. 🔴 A merged Version PR is not a
   publish. 🔴 **Re-read `#502`'s own diff first rather than trusting those three numbers** —
   they changed once already this arc (`#504`), and a later changeset re-grade moves them
   again. ⚠ This is the step that puts packages on the PUBLIC registry and moves `latest`;
   an unpublish is impossible after 72 h, and this repo has twice stranded consumers on a
   PARTIAL publish (`ETARGET` on install). IN FLIGHT: civitai/civitai-app-starters#502
   forcing: gate — `cli#760` cannot merge until `blocks-react` publishes
3. **Configure the npm trusted publisher for `@civitai/components-chat`** on npmjs.com (owner
   `civitai`, repo `civitai-app-starters`, workflow `release.yml`, environment blank).
   🔴 **OPERATOR-ONLY** — npmjs.com UI behind 2FA; no agent can do it.
   forcing: gate — without it the next `components-chat` changeset fails to publish with the
   same E404 that caused this whole arc
4. **Then land `cli#760`.** REBASE onto `cli` main first (the branch is behind: its
   `package.json.tmpl` still reads `^0.52.0`/`^0.59.0`), then set `@civitai/blocks-react` and
   `@civitai/app-sdk` to the versions `npm view` reports **after step 2's publish** — expected
   `^0.61.0` / `^0.54.0` as of `#502`@`35057f5c`, but 🔴 **derive, do not copy these**: they
   were `^1.0.0` / `^0.54.0` earlier in this same arc until `#504` re-graded the changeset.
   Re-run
   `CIVITAI_SCAFFOLD_TYPECHECK=1 go test ./internal/scaffold -run TestPageMoneyScaffoldTypechecksItsOwnShippedTests`
   and watch the FAIL→PASS inversion; undraft; merge. Files: `package.json.tmpl`,
   `App.tsx.tmpl`, `models.ts.tmpl`. IN FLIGHT: civitai/cli#760
   forcing: gate — the arc's closing condition names this PR
5. **Fix the published CLI troubleshooting page.** `cli#762` updated the vendored mirror in
   `publishedTroubleshootingRows`; the live page needs the same one-clause edit in
   `civitai/civitai-developer-docs` (`site/guide/cli-troubleshooting`).
   forcing: regression — the published page contradicts the shipped CLI's refusal list
6. **Make `#499`'s fix reach actual readers.** `civitai-developer-docs` pins
   `@civitai/blocks-react` EXACT at `0.59.0`, the generator reads the PUBLISHED tarball, and
   `apps/reference/hooks.md:583,597` is a committed generated region still carrying both
   `baseModelGroup: 'SDXL'` literals. Chain: publish → bump that pin → `npm run
   gen:appblocks:md` → commit → deploy. Closing condition:
   `git grep "baseModelGroup: 'SDXL'" -- apps/reference/hooks.md` in
   `civitai-developer-docs@origin/main` returns nothing.
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

### The release-blocker round — four traps, all of which returned a confident wrong answer

- 🔴 **A WARNING THAT APPEARS IN THE SUCCESSFUL RUN TOO EXPLAINS NOTHING — and fetching a
  successful run's log is a one-command control that the failing log cannot substitute for.**
  The whole previous round's hypothesis rested on a `.npmrc`/`NODE_AUTH_TOKEN` warning that is
  present in every run of this workflow, failing or not. No amount of re-reading the failing
  log distinguishes a cause from a constant; only the control does.
- 🔴 **`npm stage ls` EXITS 0 WHILE DOING NOTHING.** It prints
  `npm error code EUSAGE … Unknown subcommand: ls` and the shell sees **rc=0**, so a wrapper
  or a `|| echo` guard around it reports success having measured nothing. The valid spellings
  are `npm stage list [<pkg>]` / `view` / `approve` / `reject`. Same family as CLAUDE.md's
  "a reassuring zero is indistinguishable from a probe wired to nothing".
- 🔴 **A GUARD MAY READ `HEAD`, NOT YOUR WORKING TREE — check before concluding your fix
  failed.** `assert-published-versions.mjs` says so in its own first output line
  (`7 package(s) from HEAD (2895c67)`). The edit had to be committed before the guard could
  see it; read the banner rather than the verdict.
- 🔴 **npm PROVENANCE ATTESTATIONS ARE THE DISCRIMINATOR BETWEEN A CI PUBLISH AND A LAPTOP
  ONE.** `curl https://registry.npmjs.org/<pkg>` → `versions[<v>].dist.attestations` plus
  `_npmUser.name`: `GitHub Actions` + attestations present ⇒ the OIDC path ran;
  a human username + no attestations ⇒ someone published by hand, and whatever git bump went
  with it may never have been committed. That single field is what turned "why is this version
  missing" into "who put the other one there".
- ⚠ **OIDC TRUSTED PUBLISHING HAS A FIRST-PUBLISH CHICKEN-AND-EGG.** A brand-new package
  cannot have a trusted publisher configured (the package does not exist yet), so its first
  CI publish gets `E404 Not Found - PUT`. Expect to bootstrap the name by hand **at the
  version the tree declares** — not at the next one — and commit that bump, or the registry
  and the tree diverge and the whole monorepo's release goes red.

### 🔴 A CHANGESET GRADE IS NOT A FACT ABOUT THE CODE — it is a line in a file a human can edit, and one did, mid-session

- The kickoff for this round said, in bold: *"cli#760's own PR body names the WRONG
  precondition: it says 0.61.0, but #500 is a major so it needs `^1.0.0`."* That was true when
  written. **`starters#504` then merged at 2026-10-01T02:35:38Z** — *"release blocks-react
  consent-retry as a minor, not a major"* — and `#502` recomputed to `blocks-react 0.61.0`.
  So the PR body became right, and both the kickoff and this doc's own first correction became
  wrong, **without anybody being wrong at the time**.
- 🔴 **The reusable shape: "re-derive it" is only safe advice if you also say WHICH SOURCE.**
  Both readings obeyed "don't trust the PR body" and still landed on a number that moved,
  because the source they re-derived from — a `.changeset/*.md` grade, then a Version PR diff
  — is *upstream of a human decision*. The only source no re-grade can move is the **published
  artifact**: `npm view <pkg> version`, and the shipped `.d.ts` for the signature itself. The
  property that actually gates `cli#760` is *"the first version whose `useCheckpointPicker`
  declares `opts?:` / `baseModelGroup?:`"* — version-number-free, and checkable in one command.
- ⚠ **It was caught by accident, which is the part to fix.** `main` moving showed up only
  because a pre-merge `git log origin/main` for an unrelated reason (confirming the repo's
  squash-merge style) printed `#504` at the top. Nothing in the flow asks "has the base moved
  since I measured?" — `RULES.md` says to re-run a merged-tree test when the base moves, and
  this is the same rule applied to a *claim* rather than a test. `starters#505` was rebased
  onto the new `main` (`fe50b77` → `ab23b80`) and its guard re-run green there rather than
  assuming disjoint files made it safe.

## How to verify

```bash
CLI=~/workspace/civit/cli; ST=~/workspace/civit/civitai-app-starters; CIV=~/workspace/civit/civitai

# 1. the release blocker — the guard itself, from a CLEAN worktree (it reads HEAD, not the tree)
git -C $ST worktree add --detach /tmp/vap origin/main && (cd /tmp/vap && node scripts/assert-published-versions.mjs)
git -C $ST worktree remove --force /tmp/vap
#   RED  until #505 lands:  @civitai/components-chat@0.1.0 -> HTTP 404 ... RC=1
#   GREEN once it has:      OK: 7/7 publishable package version(s) confirmed ... RC=0

# 2. the publish actually happened — ask the REGISTRY, never the PR.
#    🔴 The expected numbers come from `gh pr diff 502`, NOT from this comment: #504 moved
#    blocks-react from 1.0.0 to 0.61.0 mid-arc, and another re-grade moves it again.
npm view @civitai/blocks-react version   # 0.61.0 as of #502@35057f5c — re-derive
npm view @civitai/app-sdk version        # 0.54.0
npm view @civitai/sdk version            # 0.10.0

# 3. the trusted publisher for components-chat exists (proxy: a CI publish, not a laptop one)
curl -sS https://registry.npmjs.org/@civitai%2Fcomponents-chat \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); l=d["dist-tags"]["latest"]; v=d["versions"][l]; print(l, v["_npmUser"]["name"], bool(v["dist"].get("attestations")))'
#   want: <next version> "GitHub Actions" True   (0.1.1 devzacx False = still the hand publish)

# 4. cli#760's pin is right — read the PUBLISHED TYPE, which no changeset re-grade can move
V=$(npm view @civitai/blocks-react version)     # whatever actually published
npm pack "@civitai/blocks-react@$V" --pack-destination /tmp >/dev/null
tar -xzf "/tmp/civitai-blocks-react-$V.tgz" -C /tmp
grep -n -A3 'open:' /tmp/package/dist/hooks/useCheckpointPicker.d.ts   # want `opts?:` and `baseModelGroup?:`
#   0.60.0 has `opts:` + `baseModelGroup: string` (both REQUIRED) — that is the control

# 5. the scaffold gate — it SELF-SKIPS without the env var, and `make ci` never sets it
(cd $CLI && CIVITAI_SCAFFOLD_TYPECHECK=1 go test ./internal/scaffold \
  -run TestPageMoneyScaffoldTypechecksItsOwnShippedTests -v)   # PASS, never SKIP

# 6. check rollups — SHA-PINNED, never seconds after a push (a 0 or a stale green both lie)
SHA=$(gh pr view 502 --repo civitai/civitai-app-starters --json headRefOid --jq .headRefOid)
gh api "repos/civitai/civitai-app-starters/commits/$SHA/check-runs" \
  --jq '"total=\(.total_count)", (.check_runs[]|"\(.name) \(.status) \(.conclusion) \(.completed_at)")'
```
