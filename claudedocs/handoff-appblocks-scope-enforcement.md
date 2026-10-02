# Handoff: appblocks-scope-enforcement — 2026-10-01

## Run this first — the index, one command
```bash
$DEVRC/scripts/cairn-ops/read.sh recall --repo "/home/zach/workspace/civit/civitai-app-starters"
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Stop an App Block shipping with an undeclared storage scope. Operator report
2026-10-01: an agent built a character-sheet app from
`https://developer.civitai.com/agent-setup/prompt.md`, saved everything through
`useAppStorage()`, declared only `ai:write:budgeted`, and **passed 198 unit tests, the
dev harness, `civitai app validate` and a full submit** — then every save 403'd in
production, surfaced to the viewer as *"Saving failed for an unknown reason… try again"*.

- **closing-condition:** `check` — both deterministic fixes are MERGED and each
  demonstrably refuses the defect:
  ```bash
  gh pr view 511 --repo civitai/civitai-app-starters --json state --jq .state   # MERGED
  gh pr view 767 --repo civitai/cli --json state --jq .state                    # MERGED
  # and the gates still fire at their merge commits:
  (cd <starters> && direnv exec . pnpm --filter @civitai/blocks-react test -- mockHostStorageScopes)
  (cd <cli> && go test ./internal/validate/ -run StorageScope -count=1)
  ```
  🔴 FROZEN at round 1. The two live apps below, the `agent-setup`/`AGENTS.md` teaching
  gap and the SDK error-copy fix are each a NEW arc, not an extension of this line.

## State now

🔴 **ARC CLOSED 2026-10-02, VERDICT RE-CONFIRMED BY A FULL CLOSE-CHECK.** The frozen
closing condition (both fixes MERGED, each demonstrably refusing the defect) was met, and
the follow-on release, the fleet rollout and the teaching layer all shipped on top of it.
**26 PRs merged, every one verified by CONTENT on its default branch, not by ancestry.**

- **The three deterministic fixes:** `starters#511` (`b4e3f00`, mock host gates storage on
  manifest-declared scopes) · `cli#767` (`87ba43b`, `validate` warns on an undeclared
  scope) · `starters#517` (`c13383d`, the idempotency-key rule vendored once and enforced
  at the hook boundary AND in the mock host).
- **Published and verified independently of CI** — resolvable, `_npmUser = GitHub Actions
  <npm-oidc-no-reply@github.com>`, `dist.attestations` present, never a version string:
  `@civitai/blocks-react` **0.62.0** (`starters#514`) then **0.63.0** (`starters#515`,
  `70ec100`), `@civitai/app-sdk` **0.55.0**.
- **The scaffold admits it** (`cli#769`, `60e0b31`) so a new app is born WITH the gate —
  without it every future `civitai app init` reproduced the original defect.
- **All 7 fleet consumers on `^0.63.0` + `^0.55.0`**, verified by reading each default
  branch: gen-matrix#29, model-benchmarking#81, playable-collections#52, sensei#88 (on
  `trunk`), yt-thumbnail#40, panorama-360#17, developer-docs#142.
- **The docs no longer teach the defect** (`developer-docs#141`, `#142`): the three
  colon-bearing examples are gone (colon-keys **0**, hyphen-keys **1** per file), and
  `generation.md`'s generated `idempotencyKey?` row went **1007 → 2425 chars**, gaining the
  charset, the 400 shape and why colons are banned. That row is inside a generated region
  and was structurally unable to carry the constraint until `#517` put it in the JSDoc.
- **THREE live production defects closed, only ONE of which was reported:** the storage
  403 (operator's report), the `idempotencyKey` 400 (the docs taught it), and 🔴
  **`TipButton`'s every-tip 400** — publicly exported, always supplied a 5-segment colon
  key, measured `_r_0_:123:50:Image:99` → `keyClears: false`. Nobody had reported it.
- **The predecessor session's three asks are all accounted for** (they were in
  `9a8d045a`'s kickoff, never handed to this session — recovered with
  `extract_user_msgs.py --arc`): (a) docs pin + `baseModelGroup: 'SDXL'` in a generated
  region → **CLOSED**, grep returns **0** on `origin/main` with a positive control of 3
  `baseModelGroup` matches in that same file, pin now `0.63.0` (was `0.59.0`); (b)
  `site/guide/cli-troubleshooting.md` one-clause edit → **CLOSED** in `cbfab1e`
  (developer-docs#136); (c) `@civitai/components-chat` trusted publisher → **still
  `0.1.1 / devzacx / attestations False`, published 2026-09-30**, in-tree also `0.1.1`, so
  no changeset touched it. Its closing condition is that package's NEXT publish, which has
  not happened. 🔴 **NOT a work item — there is no action, only an event to wait for**;
  the operator's own framing is *"unconfirmed, not wrong"*.
- **Index hygiene:** both `OPEN:` bullets this arc wrote on
  `civitai-app-starters/blocks-react` are now `RESOLVED` with shas (`c13383d`,
  `9edaf20`); that entry is at **0 OPEN**. The 4 remaining opens in that scope are dated
  2026-09-03/04 and 10-01 — predecessor arcs. Claims `appblocks-scope-enforcement-1`,
  `-pinbump`, `-idempotency-key-guard` and `appblocks-fleet-bump-063` all released.
- **No `clawgate-task:` field.** `clawgate_handoff.sh resolve` exits **5** and
  `field <doc>` exits **1**. A 0-task answer cannot distinguish "touched no task" from
  "wrong session id", so none was written — not a clean bill of health.
- **Sibling arcs:** `appblocks-docs-pin-currency` is CLOSED and verified live
  (`claudedocs/handoff-appblocks-docs-pin-currency.md`; `cli#765` still OPEN carrying it).
  `appblocks-agent-dx` is CLOSED — do not re-open it; its own doc's authoritative copy is
  the one `cli#764` merged.
- 🔴 **ONE THING IS LIVE-BROKEN AND IT IS MINE: `devrc#1987` is correct in the repo and
  INERT on this machine.** The handle still resolves to the dormant clone until a
  `home-manager switch`, and the `20:19:19Z` sync tick wrote ~25 docs / 277 sections under
  the now-orphaned label `civitai-cli`. **I armed that** — my `git merge --ff-only` on the
  devrc base clone (run to prove the index behaviour) put the five-handle tuple into the
  exact working-tree file the timer executes, while the unit env still pointed elsewhere.
  Rank 1 closes it.

## Open investigations — live diagnosis state

### 14 of the 15 op→scope rows in `#511` are an inference no test in that repo can falsify
- as-of: 2026-10-01
- **Symptom + exact repro:** the gate refuses a storage op whose scope is not declared.
  Which scope each op needs is a claim about the SERVER, asserted in
  `packages/civitai-blocks-react/src/internal/mockHostScopes.ts`.
- **Observed (with values):** the incident gives exactly ONE pair directly — the server's
  own refusal named both sides: `"storage set requires the apps:storage:write scope"` on
  `"path": "apps.storage.set"`. `via: measurement`
- **Ruled out — that an authoritative table already exists to copy.** `BLOCK_SCOPES` in
  `@civitai/app-sdk` carries the scope NAMES and states the mechanism ("the server gates
  them by presence in the block's approved scope set, not a bitmask") but maps no
  operation to a scope; a grep for `APPS_STORAGE_*` across app-sdk and blocks-react
  returns nothing outside `scopes.ts`, and the server's table is not vendored.
  `via: measurement`
- **Leading hypothesis:** the read/write split is right, because it is the only split the
  scope names admit. The risk is per-row, not systemic.
- **Next probe:** `SHARED_REPORT` is the row to doubt first — `useSharedStorage().report()`
  writes a row in the shared store, so it is a write by construction, but a server may
  treat an abuse report as a moderation path outside that store's gate. Confirm against
  the host's own router (`civitai`: the `apps.storage.*` / shared-storage tRPC
  procedures) before trusting it. A WRONG row fails a CORRECT app, so the table is the
  suspect in that case, not the app.

### The teaching surfaces still say nothing about storage scopes
- as-of: 2026-10-01
- **Symptom + exact repro:** an agent following the documented onboarding has no signal
  that persistence needs a manifest scope. Both fixes here are enforcement; neither
  teaches.
- **Observed (with values):** `curl -sS https://developer.civitai.com/agent-setup/prompt.md`
  → `storage` **0**, `scope` **0** (7,433 B; it is a setup doc and routes to AGENTS.md).
  `git -C <cli> show origin/main:internal/cmd/templates/agents-app.md` (168 lines, the
  block written into EVERY project) → `apps:storage` **0**, `persist` **0**,
  `useAppStorage` **0**, while `ai:write:budgeted` is named explicitly.
  `internal/scaffold/templates/page-money/block.manifest.json.tmpl` declares
  `"scopes": ["ai:write:budgeted"]` WITH a matching `scopeJustifications` entry — it
  demonstrates the exact mechanism, for generation only. No scaffold template declares a
  storage scope or calls a storage hook. `via: measurement`
- **Ruled out — that the capability is undocumented.** `apps/reference/scopes.md`
  documents all four storage scopes among 15. The defect is routing, not absence.
  `via: measurement`
- **Leading hypothesis:** naming the storage scopes in `agents-app.md` beside the
  existing `ai:write:budgeted` mention, keyed on the trigger ("anything that must survive
  a reload"), is the cheap half. Operator did NOT select it this session.
- **Next probe:** decide whether it is worth a third surface at all now that two
  deterministic gates exist — the arc's own finding is that prose surfaces failed four
  times here.

### A LIVE app already declares all four storage scopes — it is the falsifier for the 14-of-15 inference, and it is in another repo's handoff
- as-of: 2026-10-02
- **Symptom + exact repro:** the op→scope table in
  `packages/civitai-blocks-react/src/internal/mockHostScopes.ts` is now SHIPPED (merged
  `b4e3f00`, publishing via `#514`), and 14 of its 15 rows remain an inference no test in
  that repo can falsify. A wrong row fails a CORRECT app.
- **Observed (with values):** `handoff_search.py --offline` surfaced
  `datapacket-talos/claudedocs/handoff-app-taste-rollout.md` investigation #10 — *"Do the
  App Blocks storage scopes actually reach the token now that an approved version declares
  them?"* It records `gen-matrix` **0.8.9** as `approved` / `deployState live`,
  `sourceCommit 267e80f`, whose `block.manifest.json` declares all four:
  `['ai:write:budgeted','apps:storage:read','apps:storage:write','apps:storage:shared:read','apps:storage:shared:write']`.
  `via: doc` — this is RECALL from a 2026-09-03 handoff, re-derived against nothing.
- **Ruled out — that an authoritative table exists to copy.** `BLOCK_SCOPES` in
  `@civitai/app-sdk` carries scope NAMES and states the mechanism but maps no operation to a
  scope; `APPS_STORAGE_*` greps to nothing outside `scopes.ts`; the server's table is not
  vendored. `via: measurement`
- **Leading hypothesis:** a live app declaring ALL four scopes cannot discriminate the rows
  either — it satisfies every row by construction. The discriminating observation is an app
  declaring a STRICT SUBSET, or the host router itself.
- **Next probe:** read the host's own router rather than any app —
  `<civitai>` `src/server/.../apps.storage.*` and the shared-storage tRPC procedures — and
  check `SHARED_REPORT` first, since `useSharedStorage().report()` is a write by construction
  but a server may route an abuse report outside that store's gate. Confirm the 2026-09-03
  `gen-matrix` figures are still live before leaning on them.

### pnpm and the npm registry disagree about when a version was published, by 60–90s, and nobody knows which pnpm reads
- as-of: 2026-10-02
- **Symptom + exact repro:** the pnpm release-age gate quotes a publish instant in its
  refusal; `npm view <pkg> time --json` quotes a different one for the same version. Both
  land inside the 24h window so no decision turned on it, but a future decision could.
- **Observed (with values):** TWO independent measurements, hours apart, same direction —
  pnpm EARLIER than npm. (1) `@civitai/blocks-react@0.62.0`: pnpm
  `2026-10-02T04:00:25.000Z`, `npm view … time` `04:02:03.809Z` — **98 s apart**. (2) the
  0.63.0/0.55.0 set: pnpm `17:38:42–43Z` for all four packages, `npm view` `17:39:39Z –
  17:42:54Z`. `via: measurement`
- **Ruled out — that it is noise or one bad reading.** Two separate agents measured it in
  two separate repos on two separate releases and the sign never flipped; pnpm also quoted
  a single instant for four packages where npm quoted four distinct ones, which is a
  different SHAPE, not jitter. `via: measurement`
- **Leading hypothesis:** pnpm reads a different registry field than `time[<version>]` —
  plausibly a packument-level or cached `modified`/`publishTime`, or its own metadata
  cache — so the two are answering different questions rather than one being wrong.
- **Next probe:** `curl -s https://registry.npmjs.org/@civitai/blocks-react | python3 -c "import json,sys; d=json.load(sys.stdin); print({k:v for k,v in d.get('time',{}).items() if '0.63' in k}); print(d.get('modified'))"` and compare both against what pnpm prints in a forced `ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`. 🔴 Record which FIELD matches rather than which tool is "right".

## Next steps (ranked)

1. **`home-manager switch`, then prune the orphaned index label.** Operator's call: a
   switch activates EVERY devrc change merged since generation 864 (2026-10-02 13:34), not
   just the handle. Until then `devrc#1987` (`46e8dbc`) is inert and the index reads 25
   docs from the dormant clone instead of 27. Then the prune — proven available, all five
   `REPO_ENV_HANDLES` entries resolve and a real `--rebuild --prune` **dry-run returned
   rc 0 with no refusal**. 🔴 The prune cannot meaningfully run BEFORE the switch: pre-switch
   the unit still NAMES `civitai-cli`, so there is no orphan from its perspective.
   Closing condition: `PostgresSectionStore.repos()` returns labels exactly
   `civitai, cli, datapacket-talos, devrc, homelab-talos`.
   forcing: regression — the index now serves ~25 stale docs under a label no config names, created by this arc
2. **Fix `find-session.py`'s hardcoded handle list.** `scripts/find-session.py:1148` loops
   `("DEVRC","HOMELAB","DATAPACKET","CIVITAI")` while its own docstring at `:1139` says it
   *"Searches every repo in `handoff_index.REPO_ENV_HANDLES`"*. `devrc#1982` widened that
   tuple to five and this tool silently did not follow — a description wider than its
   implementation. It is why `extract_user_msgs.py --arc` exited **3 (nothing measured)**
   on this very doc and needed a `CIVITAI=<cli-path>` override to answer the operator's
   question. Make it read `REPO_ENV_HANDLES` (one rule, one place) and correct `:255`'s
   error text, which also enumerates the four.
   Closing condition: `python3 $DEVRC/scripts/session-analysis/extract_user_msgs.py --arc handoff-appblocks-scope-enforcement` exits 0 with NO env override.
   forcing: user — the operator asked for this arc's sessions and the tool could not answer without a hand override
3. **Teach the storage scopes in the generated `AGENTS.md`.** `civitai/cli`,
   `internal/cmd/templates/agents-app.md` — name `apps:storage:read`/`write` beside the
   existing `ai:write:budgeted`, keyed on the persistence trigger. The only surface an
   agent is routed to. 🔴 `agents_size_test.go` ratchets that file's size.
   Closing condition: `grep -c apps:storage internal/cmd/templates/agents-app.md` ≥ 1 on origin/main.
   forcing: none
4. **Classify authorization failures in the SDK's storage error copy.** The viewer saw
   *"Saving failed for an unknown reason… try again"*; `appStorageErrors.ts` already
   documents that an authz failure classifies `null` and that *"please try again" is the
   WRONG copy for that arm* — nothing enforces it.
   Closing condition: a block can distinguish a scope denial from a transient without parsing host prose.
   forcing: none

## Defects (batched)

- 🔴 **THIS DOC'S ORIGINAL CLOSING-CONDITION COMMAND WAS NOT A POSITIVE CONTROL — it did
  not narrow.** `pnpm --filter @civitai/blocks-react test -- mockHostStorageScopes` returned
  **98 files / 1761 tests**, byte-identical to the unfiltered run: pnpm/vitest never took it
  as a file filter, so a reassuring green said nothing about whether the gate file ran. The
  real control is `npx vitest run --project unit test/<file>` from inside the package →
  **1 file / 12 tests**, a DIFFERENT and EXPECTED count. Fixed in `## How to verify`.
- 🔴 **`scripts/find-session.py:1148` hardcodes four repo handles while `:1139` claims it
  reads `handoff_index.REPO_ENV_HANDLES`** (now five). Rank 2 fixes it. Also `:255`'s error
  text enumerates the same four.
- 🔴 **FOUR separate repos carried a WRONG in-file note about the pnpm release-age gate**,
  each corrected in place by its bumping agent: gen-matrix and sensei said the exclude list
  was "INERT" (measured against pnpm 10.28.1/11.28.0, neither the 11.25.0 the flakes pin);
  model-benchmarking said `@civitai/theme` should not be added "for symmetry" — wrong in
  the CI-breaking direction. The root claim is false: `pnpm config get minimumReleaseAge`
  → `undefined` means the **built-in 24h default applies**, not that the feature is off.
- 🔴 **`tests/guards/blocks-react-entry-directory-names.test.mjs` treats any `from '…'` in
  a COMMENT as a module edge** — `allSpecifiers` regexes raw source, so a JSDoc example
  fails the entry-layout guard naming a path in no real import list.
  `blockproto.StripCommentsForExt` exists and the sibling `cli` check uses it. **Still unfixed.**
- ⚠ **`starters`' `pnpm typecheck` EXCLUDES `test/`**, so a type error in a test file is
  invisible to it; `pnpm test` does not typecheck either. Run both separately.
- ⚠ **`pnpm test:guards` has one PRE-EXISTING local failure** —
  `manifest-sandbox-tokens.test.mjs` rule 2 walks into `.direnv/` and scans its own
  fixtures. Identical at `origin/main`; CI has no `.direnv`.
- ⚠ **`pnpm build` leaves `packages/civitai-components/src/version.generated.ts` dirty** —
  a build artifact, not your edit. Do not commit it, and do not mistake it for a failed restore.
- ⚠ **`--reporter=basic` does not exist in vitest 4** — fails as `ERR_LOAD_URL` /
  `loadCustomReporterModule` under a ~20-line vite stack that reads as a broken repo.
- ⚠ **playable-collections' `test:browser` tier cannot run on this host at either end** —
  nix pins `playwright-browsers` rev 1228, `playwright@1.63.0` wants 1243. Baselined as
  environmental; CI does exercise it.
- ⚠ **playable-collections installs TWO copies of `components` and `theme`**, predating
  this arc (`origin/main` already had `0.4.1`+`0.9.0` / `0.3.1`+`0.4.0`). It has no
  `@civitai` lockstep guard, which is why it passed where sensei failed. Deliberately not
  folded into a dependency bump.

## Gotchas / decisions / dead-ends

- 🔴 **`--filter @civitai/blocks-react test` DOES NOT RUN THE STARTER PACKAGES, and a
  blast-radius claim measured through it understates CI.** `#511`'s commit said "26 tests
  across 2 files"; CI then went red on five `Starter (…)` legs from a defect the filtered
  run never executes. Measure a breaking change with the suite CI runs, or say which
  filter produced the number.
- 🔴 **A `from '…'` INSIDE A DOC COMMENT is a real module edge to that guard** — the
  regression above. The mirror-image irony is worth keeping: the sibling `cli` check
  strips comments precisely *because a doc mention is not a call*.
- 🔴 **`direnv allow` IS PER-DIRECTORY, and a blocked `.envrc` makes four steps report
  `rc=1` with no output** — which reads as four failures and is one cause
  (`.envrc is blocked`). Every new worktree of `starters` needs its own `direnv allow`;
  🔴 do NOT `cp` an `.envrc` in and do NOT `rm` one — it is TRACKED there, so a worktree
  already has it and removing it stages a deletion.
- 🔴 **`go test` CACHES RESULTS — a mutation sweep without `-count=1` scores mutants
  SURVIVED without executing them.** Go's version of the stale-bytecode trap.
- 🔴 **TWO of my own mutants were INVALID before they were valid, and the sweep is the
  only reason I know.** (a) Replacing a call left its import unused → BUILD ERROR, which
  kills a test for the wrong reason. (b) Passing a non-markup ext to
  `StripCommentsForExt` was a NO-OP, because it runs `StripJSComments`
  UNCONDITIONALLY and `MarkupExts` gates only the HTML pass. The valid mutant calls
  `StripHTMLComments` alone.
- 🔴 **A FIXTURE'S FILE NAMES DECIDED WHETHER A TEST COULD FAIL.** The unobservable-scan
  test named the oversized file `aaa-bundle.js` and the hook file `zzz-App.tsx`;
  `os.ReadDir` returns entries SORTED, so the scan went `partial` BEFORE the hook was
  read and the assertion held whether or not the guard existed. The mutant SURVIVED,
  which is what exposed it. Reversed and documented as being the test.
- **`internal/antipattern.ScanDir` is called from a TEST ONLY** — it guards the scaffold
  templates, never a user's app. Worth knowing before assuming `validate` or packaging
  runs it.
- **Why `storagescope.go` is not a refactor of `ackScanner`:** that walk carries a
  documented MASKING PAIR of `found || partial` short-circuits whose asymmetry is load
  bearing — three separate deletions survived the whole repo until
  `TestPartialScanNeverBecomesFound` was written for them. This scan also must not stop
  at the first hit. Unifying the two walks is a reasonable follow-up, not a passing edit.
- **Field choice is deliberate and opposite to `readyack`'s:** `scopes`, not
  `FieldProject`. Both read `src/`, but readyack's remedy is in SOURCE while here the
  source is CORRECT and the manifest needs two strings. `findingFieldLedger` carries the
  contrast as a row, because that repo's meta-guard demands the reason.
- **Two `cli` meta-guards caught omissions before any review did:**
  `TestEveryCheckEmitsAField` (new function in no coverage corpus) and
  `TestEveryFindingCarriesItsDocumentedField` (field pinned by no ledger row).
- **Decision: enforce-by-default (breaking) over opt-in**, operator's call. A permissive
  default leaves the old behaviour for every app that does not opt in — precisely the
  apps that did not know the scopes existed.
- ⚠ **`scaffold boot theme` was a SEPARATE defect from the same session and is already
  fixed upstream** — `cli#766` and `starters#509`, both merged: the boot skeleton now
  takes the theme from the HOST rather than `prefers-color-scheme`. Do not re-open it.

- 🔴 **A HANDOFF ON A BRANCH IS INVISIBLE TO THE RESUME TOOLING IN TWO SEPARATE WAYS, and
  both present as reassuring output.** (a) `resume-state.sh "<path>"` on this doc printed
  `handoff: (none found — git-only)` plus `requested handoff … NO SUCH FILE` — it reconciled
  NOTHING, and the whole digest (`WORKLOAD`, `ALERTS`, `CLAWGATE`, `INVESTIGATIONS`, `DOD`)
  was about no document at all. The doc was never on `main`; `git log --all --oneline --`
  found it on `zach/handoff-appblocks-scope-enforcement` and `branch -a --contains` named the
  branch. (b) `handoff_search.py --exclude-slug` reported `in_scope_docs == indexed_docs`,
  the flag PARSED but MATCHED nothing — same cause. **Read the pair of counts, and when a
  named handoff is "missing", ask `git log --all` before concluding anything.**
- 🔴 **`resume-state.sh` does NOT expand a leading `~`** — pass an absolute path. It also
  anchors on the session's cwd repo, so a doc in a sibling client repo is a cross-repo read,
  not a local one.
- 🔴 **Verify a squash merge by CONTENT, never ancestry.** `git merge-base --is-ancestor`
  returns false after every squash merge, forever. `gh pr view --json state,mergeCommit` plus
  `git cat-file -e origin/main:<path>` on each new file is the check that works.
- 🔴 **A restore after a mutation must be proved by `git diff`, not by the `cp` succeeding.**
  Here the post-restore tree had exactly ONE dirty file and it was a build artifact
  (`version.generated.ts`), while `git diff --stat -- …/mockHost.ts` was empty — which is the
  only statement that the mutation is gone. A `cp` that silently wrote the wrong file looks
  identical.
- **`direnv allow` IS PER-DIRECTORY** — every new `starters` worktree needs its own, and a
  blocked `.envrc` makes install/build report `rc=1` with no output. 🔴 Do NOT `cp` an
  `.envrc` in and do NOT `rm` one — it is TRACKED there.
- **`go test` CACHES RESULTS** — a mutation sweep without `-count=1` scores mutants SURVIVED
  without executing them.
- **Decision: enforce-by-default (breaking) over opt-in**, operator's call. A permissive
  default leaves the old behaviour for every app that did not know the scopes existed.
- ⚠ **`scaffold boot theme` was a SEPARATE defect already fixed upstream** — `cli#766` and
  `starters#509`, both merged. Do not re-open it.

- ✅ **PUBLISHED AND VERIFIED 2026-10-02: `@civitai/blocks-react@0.62.0`** — a pre-1.0 MINOR
  for the breaking default, which is correct (a `major` would mean 1.0.0). `#514` bumped
  **only** that package, so the partial-publish/ETARGET class could not fire this time:
  `@civitai/components@0.9.0` and `@civitai/theme@0.4.0` were each already on the registry at
  exactly the in-tree version, and the published dep strings are RANGES (`^0.9.0`, `^0.4.0`,
  `>=0.49.0 <1.0.0`) — not the EXACT pin that made `0.45.0` uninstallable. Verified without
  trusting CI: `npm install --dry-run @civitai/blocks-react@0.62.0` **rc=0** (added 11
  packages, no `ETARGET`), `_npmUser = GitHub Actions <npm-oidc-no-reply@github.com>`,
  `dist.attestations` **present**, registry `time` `2026-10-02T04:02:03.809Z`.
  🔴 **The gate now reaches real apps** — a consumer on `^0.61.x` does NOT get it
  (`^0.61.0` resolves `>=0.61.0 <0.62.0`), so every block repo needs an EXPLICIT bump before
  its dev harness starts refusing undeclared storage scopes. That bump is a NEW arc.
- 🔴 **A GREEN `gh run` CONCLUSION IS NOT A PUBLISH VERDICT, AND THIS SCOPE'S INDEX HAS FOUR
  BULLETS ABOUT IT — READ THEM BEFORE READING ANY RELEASE RUN.** The two claims are
  independent: step 8 `Create release PR or publish` can SUCCEED while step 9
  `Assert the published versions actually exist on npm` goes RED purely from registry
  propagation, and that red is NOT proof the publish failed. Read the two steps separately
  (`gh api repos/<o>/<r>/actions/runs/<id>/jobs --jq '.jobs[].steps[]'`), then the registry's
  own `time` field. Here BOTH were `success` — run **36962569820** — which the index records
  as the pairing every earlier release in this repo lacked.
- 🔴 **The propagation budget was already fixed, and its measured window has only grown.**
  `release.yml` runs the assert at `PUBLISH_CHECK_TRIES: '60'` / `PUBLISH_CHECK_DELAY: '10000'`
  = **600s** under `timeout-minutes: 40`, landed in `827eccc` (#421) against a measured
  **369s** — itself larger than the **157s** measured in September, which was larger than the
  "~60s" before that. 🔴 **Treat every one of those figures as a LOWER BOUND; the window has
  not converged.** The workflow's own comment says TRIES and DELAY are COUPLED to the
  `timeout-minutes` cap (worst case `2 x 59 x 10000` per failing package) — re-derive both if
  you touch either. This release's green end-to-end run was the stated close-check for that
  index item, now `RESOLVED 827eccc:`.
- 🔴 **If a publish ever 404s, the staged-publishing check is `npm stage list` — the verb is
  `list`, NOT `ls`.** `npm stage ls` prints `EUSAGE Unknown subcommand` and **exits 0**, so an
  `ls` spelling reads as a clean "nothing staged" while telling you nothing. A staged version
  returns `E409 Cannot publish over previously staged version` forever and needs the owner's
  2FA (`npm stage approve`); a silent failure is cured by a plain re-run. On a FIRST attempt
  the log cannot separate the two — the re-run IS the discriminator, and it is only safe when
  one package remains and its deps are already live.
- ⚠ **`@civitai/components-chat` still has NO npm trusted publisher** (operator-only, npmjs.com
  UI + 2FA). It was not in this release, so it did not bite — but any future release whose
  changeset set includes it will take an `E404 PUT` on that package while every other package
  publishes fine. Still OPEN in the index.
- 🔴 **A non-terminal check is not a failure, and `mergeStateStatus=BLOCKED` can mean exactly
  that.** `#514` read `BLOCKED` with 19 of 20 checks terminal-green and
  `README snippets (typecheck)` still `in_progress` — a slow leg that finished ~12 min after
  the other 19 (03:46:58Z → 03:58:57Z), the same lag it showed on `#511`. Gate on a
  **tallied** count of terminal conclusions at a **pinned** SHA, re-confirm the head has not
  moved, and report the tally (`{"success":20}`) rather than an absence of failures.

- 🔴 **THE SESSION'S ONE REUSABLE LESSON: nearly every failure here was a GREEN OR A ZERO
  THAT MEANT NOTHING, and not one was caught by reasoning — every single one was caught by
  a control.** The complete list, because the pattern is the point: a `pnpm --filter … test
  -- <name>` that DID NOT NARROW (same 98 files/1761 tests, so the "positive control" was a
  fact about the glob); `pnpm config get minimumReleaseAge` → `undefined` read as
  "feature off" when it means the 24h DEFAULT APPLIES; an EMPTY `check-runs` rollup on a
  repo that gates on four Tekton COMMIT STATUSES; a `--timeout=300` flag typo that ran
  **zero tests and exited 0**; `${PIPESTATUS[0]}` in zsh (it is lowercase `pipestatus`)
  reporting **rc 0 over 127 failures**; a config change live on `origin/main` and INERT in
  the process reading it, because the clone was one commit behind; and a `grep -c` that
  counted my own QUOTED RETRACTION as a surviving instance of the claim it retracts.
- 🔴 **THE PEER FLOOR IS THE REAL WORK IN ANY blocks-react BUMP, AND IT FAILS SILENTLY ON
  pnpm AND LOUDLY ON npm.** `0.63.0` peers `@civitai/app-sdk >=0.55.0 <1.0.0`; a pre-1.0
  caret locks the minor so `^0.54.0` = `>=0.54.0 <0.55.0` and EXCLUDES it. On pnpm the
  install merely warns and the break lands at MODULE EVALUATION — suites fail to IMPORT
  and collect ZERO tests while the summary reports NO failures. Measured across four repos
  on 0.63.0: gen-matrix 729→502, model-benchmarking **1027→352**, playable-collections
  809→434, sensei 868→356 — **1,789 tests silently unrun**. On npm (yt-thumbnail,
  panorama-360, developer-docs) the same mistake is a hard `ERESOLVE` and nothing runs.
  🔴 **ONLY A THREE-POINT COUNT SEES IT** — baseline, blocks-react ALONE, then +app-sdk.
  First-vs-last hides it completely, and the middle reading shows **only a passed count**,
  so read the FILE count too.
- 🔴 **THE RELEASE-AGE GATE SCORES EVERY LOCKFILE ENTRY, NOT JUST THE RANGES YOU EDIT.**
  Four tokens were needed, not two: the two direct bumps plus `@civitai/theme@0.5.0` and
  `@civitai/components@0.9.1`, which arrive only as nested deps of `0.63.0` while
  `package.json` still pins `^0.3.1`/`^0.4.1`. pnpm appended them itself. Control each
  token INDIVIDUALLY from a clean tree — model-benchmarking did and found
  `@civitai/sdk@0.10.0` was NOT load-bearing. 🔴 **APPEND-vs-REPLACE IS PER-REPO**, stated
  in each file's own header (gen-matrix and playable-collections append;
  model-benchmarking and sensei replace), and **REPLACE has an ordering trap**: swapping
  `0.62.0`→`0.63.0` then installing FAILS, because at gate time the lockfile still
  resolves `0.62.0` and the error names the version you just deliberately removed. Allow
  both during resolution, then prune.
- 🔴 **A VERSION PR IS A SNAPSHOT OF THE CHANGESETS THAT EXISTED WHEN IT WAS COMPUTED.**
  `starters#515` was OPEN, green and mergeable while bumping `blocks-react` to **0.62.1 as
  a PATCH** from three unrelated theme/docs changesets — `idempotency-key-format-guard.md`
  was still unconsumed on `main`. Merging it would have published a version without the
  guard under a plausible number. The one-command discriminator is
  `git ls-tree --name-only origin/main .changeset/`.
- 🔴 **A GUARD THAT FIRES ON ITS OWN DEADLINE BEATS A NOTE, AND THIS ARC PROVED IT TWICE.**
  `#517` ledgered three symbols against the app-sdk version its own branch would publish —
  unavoidable pre-release — and pinned the prediction so `PREDICTION HAS COME TRUE` goes
  RED the moment that version appears. It blocked five `Starter (…)` legs on the Version
  PR. My equivalent was a sentence in a handoff. Its remedy text also named the plausible
  WRONG fix and why: raise the floor and you exclude a good release forever. **Follow such
  a remedy verbatim rather than reasoning from scratch.**
- 🔴 **A DESCRIPTION WIDER THAN ITS IMPLEMENTATION READS AS COVERAGE AND PROVIDES NONE —
  three instances, one of them mine.** (a) `find-session.py`'s docstring claims it reads
  `REPO_ENV_HANDLES`; the code hardcodes four (rank 2). (b) The `transport-helpers.test.ts`
  comment named the idempotency hazard correctly while asserting only against the SDK's own
  generator, which **already conformed and never could not** — the caller-supplied path,
  the one that broke, had no guard at any of three layers. (c) I wrote "the floor stays
  `>=0.49.0`" into a guard file when it is `>=0.55.0`, copied from an adjacent note that
  says `0.49.0` correctly about a DIFFERENT conversion (`#520` corrected it).
- 🔴 **THE STORAGE GATE REACHES ALMOST NO EXISTING APP, AND ONLY A TWO-ARM PROBE REVEALS
  THAT.** 5 of 6 apps ported `useAppStorage`/`useSharedStorage` onto their OWN REST runtime
  (`./lib/sdk-runtime.js`), so storage never crosses `createMockHost` and the gate is
  structurally UNREACHABLE; panorama-360 uses no storage at all. Only **yt-thumbnail**
  exercises it (removing `declaredScopes` fails 9 tests in `src/responsive.test.tsx`). A
  green suite there is NOT the gate approving. 🔴 And do NOT pre-thread `declaredScopes`
  where no path observes it — that pre-silences the gate for a future port back.
- 🔴 **THE DOCS WERE UPSTREAM OF THE MOCK HOST, WHICH REFRAMES THE WHOLE ARC.** I first
  diagnosed the 400 as mock-host permissiveness — the layer I happened to be looking at.
  The public docs *taught* the colon key in four places (`generation.md:594`,
  `text-to-image.md:349`, `earning.md:394`, and `hooks.md:820` recommending `React.useId()`,
  which returns `":R0:"` on React 18.3.1) while stating no charset anywhere. The app did
  what it was told. Four layers failed in order: docs taught it → SDK forwarded it
  unvalidated → mock host never saw the field (**zero** occurrences in `mockHost.ts`) →
  live host rejected it.
- 🔴 **THE COLON IS BANNED FOR A REASON, SO A DIFFERENT SEPARATOR IS NOT A FREE CHOICE.**
  `<civitai>/src/server/utils/block-gen-idempotency.ts:59` says "colon-free" because
  `block-tip-rate-limit.ts:215` composes its rate-limit key as `<userId>:<key>`. Use `-`;
  `.` and `/` are also outside `[A-Za-z0-9_-]`.
- 🔴 **REFUSE, NEVER SANITISE, a malformed idempotency key** — sanitising breaks the
  identity the key carries, so two logical submits could collapse into one charge or a
  retry could mint a second reservation. `#517` added `InvalidIdempotencyKeyError` rather
  than reusing `WorkflowSubmitError`, whose every code is money-AMBIGUOUS by design. An
  anti-sanitise test pins it.
- 🔴 **A MUTATION CAN DIE FOR THE WRONG REASON, AND A SUITE CAN BE BLIND TO THE DIMENSION
  THE BUG LIVES ON.** In `#517`'s 13-mutant sweep, M6 was killed by a NEIGHBOUR's error
  with the new assertion never executing (M6b — same mutant plus the guard disabled —
  proved reachability), and **M7 SURVIVED 37/37** because the suite pins ONE React whose
  `useId()` already conforms while the peer range declares `^18 || ^19`. Closed by
  extracting `composeTipIdempotencyKey` and feeding each version's real seed.
- 🔴 **devrc GATES ON FOUR TEKTON COMMIT STATUSES, NOT CHECK-RUNS.**
  `gh api …/commits/<sha>/check-runs` returns `total_count: 0` there and reads as "no CI".
  Poll `…/commits/<sha>/status` and read `.state`; `.conclusion` is always null for
  statuses. Contexts: `tekton/devrc-{pytests,nodetests,gotests,cairn-client-runs}`.
  ⚠ And the repo genuinely has no `.github`, so "0 checks is by design" is a TRUE sentence
  about Actions that is a WRONG answer about the gate.
- 🔴 **THE RESUME TOOLING HAS TWO SEPARATE REPO-HANDLE LISTS AND I ONLY WIDENED ONE.**
  `handoff_index.REPO_ENV_HANDLES` (fixed in `#1982`, now five) and `find-session.py`'s
  hardcoded four. Symptom: `extract_user_msgs.py --arc <this doc>` exits **3 — NOTHING
  MEASURED**, which is NOT "the arc is empty" (that is exit 4). It had TWO stacked causes:
  the handle gap, and the `cli` clone being 7 commits behind so the doc was not on disk at
  all. Workaround that answered the question: `CIVITAI=/home/zach/workspace/civit/cli`.
- ⚠ **`extract_user_msgs.py` counts harness `<task-notification>` blocks as "typed".** It
  reported 52 messages for this arc; **10** were genuine operator input. Filter on the body
  before quoting a count.
- 🔴 **A PRIMARY CLONE BEING BEHIND IS A LIVE HAZARD, NOT HYGIENE — measured twice today.**
  The `cli` clone was 7 commits behind, so the handoff doc was absent from disk and the arc
  resolver could not see it. And the playable-collections/sensei agent found BOTH its
  primary clones stale, noting *"`ls`/`grep` in those clones would have reported the wrong
  pins"* — it read `origin/<branch>` refs instead, which is the only reason its pin check
  held.
- **Decision: enforce-by-default (breaking) over opt-in**, operator's call, for both gates.
  A permissive default leaves the old behaviour for exactly the apps that did not know the
  constraint existed.
- ⚠ **`components-chat`'s identical-looking `const NAME` regex was deliberately NOT
  consolidated** — it governs LLM TOOL NAMES, a different grammar, and merging them would
  let a money-key policy change silently alter which chat tools register.

## How to verify

```bash
ST=~/workspace/civit/civitai-app-starters; CLI=~/workspace/civit/cli
DD=~/workspace/civit/civitai-developer-docs

# 0. THE CLOSING CONDITION — both original fixes merged (they are)
gh pr view 511 --repo civitai/civitai-app-starters --json state --jq .state   # MERGED
gh pr view 767 --repo civitai/cli --json state --jq .state                    # MERGED
#    by CONTENT, because a squash merge never makes the branch head an ancestor:
git -C $ST cat-file -e origin/main:packages/civitai-blocks-react/src/internal/mockHostScopes.ts
git -C $CLI cat-file -e origin/main:internal/validate/storagescope.go

# 1. the whole chain is published and reaches apps
npm view @civitai/blocks-react version      # 0.63.0
npm view @civitai/app-sdk version           # 0.55.0
cd /tmp && npm install --dry-run @civitai/blocks-react@0.63.0   # rc=0, no ETARGET
npm view @civitai/blocks-react@0.63.0 --json | python3 -c 'import json,sys; d=json.load(sys.stdin); u=d.get("_npmUser"); print(u.get("name") if isinstance(u,dict) else u, bool(d.get("dist",{}).get("attestations")))'
#    -> "GitHub Actions" True.  🔴 NEVER verify a publish by a version string alone:
#       this package once had a `latest` that npm view reported happily while
#       npm install died with ETARGET.

# 2. every fleet consumer is on it (expect ^0.63.0 / ^0.55.0; docs repo exact)
for r in civitai-app-gen-matrix civitai-app-model-benchmarking \
         civitai-app-playable-collections civitai-app-yt-thumbnail app-panorama-360; do
  git -C ~/workspace/civit/$r show origin/main:package.json | python3 -c 'import json,sys; d=json.load(sys.stdin); m={**d.get("dependencies",{}),**d.get("devDependencies",{})}; print(m.get("@civitai/blocks-react"), m.get("@civitai/app-sdk"))'
done
git -C ~/workspace/civit/civitai-app-sensei show origin/trunk:package.json | grep -a blocks-react   # sensei is on TRUNK

# 3. the docs no longer teach it — read the PAIR, never the zero alone
for f in apps/reference/generation.md apps/guide/text-to-image.md apps/guide/earning.md; do
  echo "$f colon=$(git -C $DD show origin/main:$f | grep -acE 'idempotencyKey: `[a-z]+:')" \
       "hyphen=$(git -C $DD show origin/main:$f | grep -acE 'idempotencyKey: `[a-z]+-')"
done   # expect colon=0 hyphen=1 each

# 4. the predecessor arc's item (a), with its positive control
git -C $DD show origin/main:apps/reference/hooks.md | grep -ac "baseModelGroup: 'SDXL'"  # 0
git -C $DD show origin/main:apps/reference/hooks.md | grep -ac 'baseModelGroup'          # 3 = the grep CAN match

# 5. RANK 1's closing condition — is the handle repoint live yet?
systemctl --user show handoff-index-sync.service -p Environment | tr ' ' '\n' | grep CIVITAI_CLI
#    still .../civitai-cli  => the switch has NOT happened and #1987 is inert
python3 ~/workspace/devrc/scripts/lib/handoff_search.py --offline --query x --limit 1 2>&1 | grep repos=
#    want a `cli` label and indexed_docs reflecting 27, not 25

# 6. RANK 2's closing condition — no env override needed
python3 $DEVRC/scripts/session-analysis/extract_user_msgs.py --arc handoff-appblocks-scope-enforcement
#    exit 3 today (handle gap). Want exit 0 with NO CIVITAI= override.
#    🔴 exit 3 = NOTHING MEASURED; exit 4 = measured and genuinely empty. Different facts.
```
