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

🔴 **ARC CLOSED — `closing-condition` ADDRESSED, and the last open PR of the arc is now merged.**
Evaluated 2026-10-01T05:03Z:

- `gh pr view 760 --repo civitai/cli --json state` → **`MERGED`** (`67112782`, 03:29:19Z)
- `grep -c 'baseModelGroup: checkpoint.baseModel'` on `App.tsx.tmpl@origin/main` → **`0`**
- the LoRA site keeps its filter deliberately (`:535` `const requestedFamily = checkpoint.baseModel;`
  → `:539` `baseModelGroup: requestedFamily,`), so the zero is the intended change

⚠ **The prose half was never driven live.** "A freshly scaffolded app can reach every checkpoint
ecosystem from its Change-model button" rests on the scaffold typechecking against `0.61.0`'s
optional signature plus the host's documented `baseModels: []` ⇒ no-narrowing behaviour. The
registered MECHANICAL test is the grep, and it passes. Nobody clicked the button.

### Final merges

| PR | repo | merged | what |
|---|---|---|---|
| `#505` | starters | `a560b3dc` 03:20Z | release-pipeline fix — `components-chat` `0.1.0`→`0.1.1` |
| `#760` | cli | `67112782` 03:29Z | scaffold pin `^0.61.0`/`^0.54.0` + two `#759` guard fixes |
| **`#131`** | developer-docs | **`25a8e835` 05:03Z** | the two empty `.md` twins + a guard — **ask #6's PR, which this doc had never recorded** |
| **`#763`** | cli | **`4a26d79c` 05:03Z** | this handoff doc |

- ✅ **`starters` main Release run GREEN** — `36810029698` at `a560b3dc`, first success since
  `2026-09-30T16:38:35Z`; its assert step logged `OK: 7/7 publishable package version(s)
  confirmed on the registry, 0 missing` (a COUNT of 7, so not a vacuous pass).
- ✅ **Published and resolvable** (verified by `npm install --dry-run`, with `0.60.0` as the
  control proving the probe can pass): `blocks-react@0.61.0`, `app-sdk@0.54.0`, `sdk@0.10.0`,
  all `_npmUser = GitHub Actions` + attestations. No partial publish.
- ✅ **The npm trusted publisher for `@civitai/components-chat` was set by the operator**
  (2026-10-01, ask closed on their word). 🔴 **NOT independently verifiable from this machine
  and not claimed as verified** — the registry doc exposes no trusted-publisher field
  (`[k for k in doc if 'trust' in k]` → `[]`) and `npm access` has no such subcommand. It
  resolves on the NEXT `components-chat` publish; see `## Next steps`.
- 🔴 **`#131` is MERGED but NOT LIVE.** Measured 05:04Z, ~90 s after the merge:
  `developer.civitai.com/apps.md` = **21 B**, `orchestration.md` = **30 B**, against a control
  `apps/reference/hooks.md` = **42,265 B**. `civitai-developer-docs` is branch-tracked
  (`ref: branch: main`), so the merge triggers the deploy; it had not landed yet. **Merged ≠
  live** — the harm is still on the public site until the twins pass the guard's 200-char floor.

### `#131` was gated on the MERGED tree, not its own branch

Its 16 checks were green at **16:48Z against a base that moved** — `#132` merged at 17:01Z —
and it wires `check:agent-md` into the **required** `build-site` job, so a merged-tree failure
would have reddened a required check for everyone. Built and ran it before merging:

- merged tree (`origin/main` + `refs/pull/131/head`): clean merge, 5 files, build exit 0
- `check:agent-md` → **`OK — 129 .md twin(s), all carry a body`**, rc 0 (128 → 129: `#132`'s
  new `apps/guide/earning.md` added one, and it has a body; 129 ≫ the `MIN_TWINS=50` floor)
- `check:built-site` (same job) → `all built-site checks passed`, rc 0
- 🔴 **negative control**: planting the *exact shipped defect* (`dist/apps.md` = the live
  21-byte frontmatter-only value) → rc **1**, naming `/apps.md — 0 body chars`; restoring
  returned green. The guard can still fire on the merged tree.
- ⚠ A **third** `layout: home` page exists — the site root `index.md`, body **0 chars** — and
  it does NOT trip the guard, because the plugin emits no twin for the root. That is measured
  (129 twins, all bodied), not assumed; if a future plugin change starts emitting one, this
  guard goes red on `main` with no code change.

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

🔴 **Leftovers, not a continuation — the arc is closed.** Items 2 and 3 are a NEW arc in
`civitai-developer-docs` and want their own `closing-condition`. All premises re-verified on
`origin/main` at 2026-10-01T05:0xZ, after `#131`/`#132` moved the tree twice.

1. **Confirm `#131` actually went live.** One command, and it is the only thing that closes the
   harm this arc's ask #6 identified:
   `curl -sS https://developer.civitai.com/apps.md | wc -c` → must exceed the guard's 200-char
   body floor (it was 21 B at merge+90 s). A watch was armed in-session; if it never fired,
   check the deploy rather than assuming.
   forcing: regression — an agent following `llms.txt` to `/apps.md` gets a blank page until
   this deploys, which is the defect the merged PR exists to fix
2. **Make `#499`'s fix reach actual readers.** RE-VERIFIED on current `origin/main`:
   `package.json:57` pins `"@civitai/blocks-react": "0.59.0"` EXACT (line moved from 56 — `#132`
   shifted the file, so read the grep not the number), and `apps/reference/hooks.md` still
   carries **2** × `baseModelGroup: 'SDXL'` in a committed generated region. Unblocked by
   tonight's publish. Chain: bump the pin → `npm run gen:appblocks:md` → commit → deploy.
   Closing condition: `git grep "baseModelGroup: 'SDXL'" -- apps/reference/hooks.md` on
   `civitai-developer-docs@origin/main` returns nothing.
   forcing: gate — the defect is live on developer.civitai.com until this completes
3. **Fix the published CLI troubleshooting page.** `cli#762` updated the vendored mirror in
   `publishedTroubleshootingRows`; the live page needs the same one-clause edit. VERIFIED:
   `site/guide/cli-troubleshooting.md` exists on `origin/main`. Same repo as item 2 — one PR.
   forcing: regression — the published page contradicts the shipped CLI's refusal list
4. **Close the trusted-publisher loop on the next `components-chat` publish.** Nothing to do
   now; this is a READ to perform when that package next ships:
   `curl -sS https://registry.npmjs.org/@civitai%2Fcomponents-chat | python3 -c '…latest…'` must
   show `GitHub Actions` + `attestations: True`. While it still reads `0.1.1 / devzacx / False`,
   the configuration is unconfirmed rather than wrong.
   forcing: gate — if the publisher is not in fact configured, the next changeset bumping that
   package fails with the same `E404 Not Found - PUT` that opened this whole arc

## Defects (batched)

- 🔴 **`#759`'s picker-call-ledger GROWTH half is still spelling-dependent.** The sibling
  DERIVATION half was fixed in `#760` (`derivesFromChosenCheckpoint`, one hop through a local);
  the growth half still misses a picker reached through an alias not ending in `Picker`, and a
  call whose argument object opens on the next line. Not fixed in `#760` — touching it again
  would have reset a green 12/12 CI run. Same one-hop treatment applies.
- 🔴 **The `page-money` scaffold pin lives at SEVEN literal sites across FOUR files** and only
  `TestDesignTokenLedgerMatchesTemplatePin` notices divergence. `bump-pins` owns all seven; a
  hand-edit of `package.json.tmpl` leaves six stale. A `.github/workflows/bump-scaffold-pins.yml`
  exists and presumably automates it — nobody checked whether it would have sufficed unaided.
- ⚠ **`cli`'s browser-driving Oracle tier is red on `main` under `CIVITAI_CHROME`**, and
  differently red than on a feature branch (1 failure on main vs 10 on `#760`'s branch). Without
  the env var the three `TestOracle*` tests fail everywhere with `no Chromium on PATH`. Nobody
  owns this; recorded because the tier's state is unknowable from CI.
- ⚠ **The site root `index.md` is a `layout: home` page with a 0-char body** and is invisible to
  `#131`'s guard only because the plugin emits no twin for the root. Measured, not assumed.

**Carried forward from earlier rounds — NOT re-verified this session.** `Defects` is a REPLACE
heading, so these would be silently deleted by any update that omits them:

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
  Three independent agents hit it; seen again as a `UU` conflict in the `starters` PRIMARY clone.
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

### 🔴 THE MERGED-TREE BREAK IS NOT HYPOTHETICAL — `#759` + `#760` hit it exactly

- `#759` added `TestThePickerGuidanceMatchesWhatTheScaffoldActuallyDoes` to `main`; `#760`
  changed the scaffold the guard inspects. **Disjoint files, no textual conflict, both green on
  their own branches, red the moment `#760` was rebased onto `main`.** Nothing but running the
  suite on the MERGED tree could have seen it — and `#760` would have merged red had the rebase
  not been done first.
- Two fixes, both in the guard rather than the template, because the template was right:
  - `wantPickerCallSites["openCheckpointPicker"]` was `true`, **encoding the belief this whole
    arc exists to refute.** Holding a chosen checkpoint is NOT the exception; needing to STAY
    INSIDE its family is. LoRA keeps `true`; the Change-model button becomes `false`.
  - The "must be DERIVED" half was a per-line `strings.Contains(value, "checkpoint.baseModel")`
    and the scaffold legitimately derives through a named local (the family asked for has to be
    recorded on the resulting LoRA). **The guard failed code that follows its own guidance
    exactly, and the obvious way to green it was to inline the expression and lose the recorded
    family** — a guard correct code cannot satisfy is worse than none.
- Mutation-tested, each mutant killed by **its own** error rather than another assertion, with
  the template restored byte-identical afterwards: `'SDXL'` → "hardcoded ecosystem";
  `const requestedFamily = someOtherState.family` → "derives … from something other than" (this
  one proves the one-hop resolution does not blindly accept any identifier);
  `openCheckpointPicker({ baseModelGroup: … })` → "records that this call site holds no chosen
  checkpoint" (proves the flipped ledger entry is REACHABLE, not just changed).

### ⚠ A browser-driving test tier is not a harness to widen casually

- Chasing a fully-green suite, this session set `CIVITAI_CHROME` to the operator's own Brave
  binary and ran `go test ./...`. That **widened the harness into a tier that launches a real
  browser**: failures went 3 → 10, `main` fails *differently* under the same variable, and the
  runs left **637 orphaned Brave processes** across two worktrees. SIGTERM did not take;
  SIGKILL on a freshly re-resolved PID list did.
- 🔴 **The cleanup rule that made this safe to undo: resolve PIDs and verify `/proc/<pid>/cwd`
  against the EXACT worktree paths plus `/proc/<pid>/exe`, never a `pkill -f` pattern.** 32
  Brave processes belonging to another session (`/tmp/wt-rank21`, Sept 27) were present
  throughout and had to survive — a pattern kill would have taken them.
- **The correct comparison was the one already in hand.** Without the env var the suite matched
  `origin/main` exactly: three `TestOracle*` failures, every one `no Chromium on PATH`. A change
  that touches none of the browser machinery is not answerable for that tier, and on an
  operator's workstation it should not be run at all.

### 🔴 A HANDOFF'S PR LEDGER IS BUILT FROM ITS RANKS, SO AN ASK OUTSIDE THE RANKS VANISHES

- This doc's `## Goal` framed the arc as **three defects** and its table listed **8 PRs**. A
  transcript sweep of the arc's two sessions found a **FOURTH operator ask**, 38 minutes before
  the three-defect message, whose PR — `civitai-developer-docs#131` — appeared in **no table, no
  rank and no defect line** of this document. It sat `CLEAN`/`MERGEABLE` with 16 green checks for
  **12.5 hours** while the defect it fixes was live in production.
- **Why it was invisible:** the `/handoff` was invoked with `topic: appblocks-agent-dx`, scoped
  to the three-defect messages. Ask #6 (*"read and evaluate the manage-appblocks-docs skill …
  are there agent-first markdown pages for every docs page"*) produced a pushed skill commit
  **and** a PR, and only the skill half landed. The genesis session even OFFERED `/audit-pr 131`;
  the next operator message opened a new topic, so the offer was never answered and the PR was
  never audited.
- 🔴 **The reusable rule is the one `supersede.md` already states for ranks, applied to ASKS:
  enumerate from what the SESSION DID, not from what the ranked list records.** The ranked list
  is what someone thought to write down; it is not a coverage map. A `gh pr list --state all
  --author <me>` windowed to the arc's first message is the cheap complete enumeration, and it
  is what found this.
- ⚠ **A bare `#N` grep over a session transcript CANNOT attribute a PR.** `grep -c '#131'` over
  the `.jsonl` returned **434** and the identical number for four unrelated PRs — the file is a
  handful of enormous lines, so `-c` counts lines, not hits, and every line matched. The sound
  instrument is the **creating act**: `grep -o 'gh pr create[^"]*'`, which named
  `zach/agent-md-empty-home-pages` unambiguously. *Identical counts across unrelated candidates
  is the tell.*

### ⚠ `find-session --arc` CANNOT measure an arc whose doc lives outside the four repo handles

- `--arc handoff-appblocks-agent-dx.md` exits **5**: *"no repo handle holds …"*. The handles are
  `$DEVRC`, `$HOMELAB`, `$DATAPACKET`, `$CIVITAI`; this doc lives in `civit/cli`, which is none
  of them. Exit 5 is explicitly **NOT** "the arc is empty" — nothing is read at all, and the
  `NEXT —` footer that would have named `extract_user_msgs.py --arc` never prints.
- The manual substitute, which worked: WRITERS from the doc's own commit trailers
  (`git log --format='%(trailers:key=Claude-Session-Id,valueonly)' -- <doc>`), READERS from
  `find-session.py <slug> --all-time`, then
  `extract_user_msgs.py --session <id> --session <id>`. Two sessions, 45 messages, 172 KB — of
  which only **9** were operator-typed; the rest were task-notifications and relayed subagent
  reports, so filter by size and by `<task-notification>` before reading.
- Worth fixing upstream: either add a `cli` handle, or let `--arc` take a path outside the
  handle set. Until then, any arc in this repo is unmeasurable by that flag.

## How to verify

```bash
CLI=~/workspace/civit/cli; ST=~/workspace/civit/civitai-app-starters; D=~/workspace/civit/civitai-developer-docs

# 1. THE ARC'S CLOSING CONDITION — both halves
gh pr view 760 --repo civitai/cli --json state --jq .state            # MERGED
git -C $CLI show origin/main:internal/scaffold/templates/page-money/src/App.tsx.tmpl \
  | grep -c 'baseModelGroup: checkpoint.baseModel'                    # 0
# a 0 achieved by DELETING the LoRA filter is the wrong 0 — it must survive:
git -C $CLI show origin/main:internal/scaffold/templates/page-money/src/App.tsx.tmpl \
  | grep -nE 'const requestedFamily|baseModelGroup:'                  # :535 and :539

# 2. 🔴 the one thing still unclosed — did #131 DEPLOY? (merged != live)
curl -sS https://developer.civitai.com/apps.md | wc -c                # > 200, was 21 at merge
curl -sS https://developer.civitai.com/orchestration.md | wc -c       # > 200, was 30
curl -sS https://developer.civitai.com/apps/reference/hooks.md | wc -c  # ~42k — the CONTROL;
#   if this is small too, the site is not answering and the other two numbers mean nothing

# 3. the release pipeline — the RUN, then the REGISTRY
gh run list --repo civitai/civitai-app-starters --workflow release.yml -L 1 \
  --json headSha,conclusion --jq '.[]|"\(.headSha[0:8]) \(.conclusion)"'   # success
for p in blocks-react app-sdk sdk components-chat; do
  printf '%-16s %s\n' "$p" "$(npm view @civitai/$p version)"
done                                       # 0.61.0 / 0.54.0 / 0.10.0 / 0.1.1

# 4. the trusted publisher — resolves only on the NEXT components-chat publish
curl -sS https://registry.npmjs.org/@civitai%2Fcomponents-chat \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); l=d["dist-tags"]["latest"]; v=d["versions"][l]; print(l, v["_npmUser"]["name"], bool(v["dist"].get("attestations")))'
#   0.1.1 devzacx False  = still the by-hand bootstrap; configuration UNCONFIRMED, not wrong

# 5. the scaffold gate — SELF-SKIPS without the env var and `make ci` never sets it,
#    so a green suite does NOT prove the rendered scaffold builds. -v: PASS, never SKIP.
(cd $CLI && CIVITAI_SCAFFOLD_TYPECHECK=1 go test ./internal/scaffold \
  -run TestPageMoneyScaffoldTypechecksItsOwnShippedTests -v)

# 6. ⚠ do NOT set CIVITAI_CHROME to run cli's suite on a workstation — it drives a real
#    browser and left 637 orphaned processes. Baseline without it: 3 TestOracle* failures,
#    all `no Chromium on PATH`, identical to main.
(cd $CLI && go test ./... -count=1 2>&1 | grep -c 'no Chromium on PATH')   # 5 lines / 3 tests

# 7. the arc's own completeness check — the sweep that found #131
for r in cli civitai-app-starters civitai civitai-developer-docs; do
  gh pr list --repo civitai/$r --state all --author ZacxDev --limit 80 --json number,title,createdAt,state,headRefName \
  | jq -r --arg r "$r" '.[]|select(.createdAt>="2026-09-29T21:00:00Z")|"\($r)#\(.number)\t\(.state)\t\(.headRefName)"'
done | sort -k2
```
