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

🔴 **ARC CLOSED 2026-10-02.** The closing condition's `check` is met in both halves —
both PRs MERGED, and each gate demonstrably refuses the defect at its own merge commit.

- **`civitai-app-starters#511` — MERGED**, squash `b4e3f00d4a493c54fb2b1493ca2c596ae06dd57e`,
  `mergedAt 2026-10-02T03:45:24Z`. Pre-merge CI re-read SHA-pinned at head `1980ad7`:
  **20/20 green**, including all five `Starter (…)` legs that were red from the JSDoc
  `import … from` regression — so the `1980ad7` fix held.
- **`civitai/cli#767` — MERGED**, squash `87ba43bd92e5e5c90c1e35f3dc909765924cfe16`,
  `mergedAt 2026-10-02T03:45:27Z`. Pre-merge CI SHA-pinned at head `837607d`: **12/12 green**,
  `build-test` completed.
- Both were `MERGEABLE`/`CLEAN` and based directly on `main` — not stacked. Merges verified
  by CONTENT on `origin/main` (`git cat-file -e` on all four new files), never by ancestry,
  since a squash merge never makes the branch head an ancestor.
- **Gate verification at the merge commits, not at the PR branches:**
  - `cli` @ `87ba43b`: `go test ./internal/validate/ -run StorageScope -count=1 -v` →
    **5 test functions PASS** (`…FollowsSymlinkedSource`, `…UnobservableScanStaysSilent`,
    `…DeclaresStore` 7 subtests, `…ToleratesABadScopesField` 3 subtests).
  - `starters` @ `b4e3f00`: install rc=0, build rc=0, full suite
    **98 files / 1761 tests / 0 failures** — reproducing the doc's own figures exactly.
  - 🔴 **Negative control, the part CI cannot give you:** neutering the guard condition at
    `packages/civitai-blocks-react/src/internal/mockHost.ts:1858`
    (`if (needed !== null && !declaredScopeSet.has(needed))` → `if (false && …)`) took the
    gate file to **6 failed / 6 passed of 12**, with failures naming
    `/apps:storage:shared:write/` — i.e. killed by **this** guard's own assertion, not a
    neighbour's error. Restored from a `cp` copy, re-ran **12/12 pass**, and confirmed the
    restore with `git diff` (clean on `mockHost.ts`), never by trusting the copy.
- **Release is now IN FLIGHT and is a NEW arc, not this one.** Merging `#511` landed the
  changeset `.changeset/polite-hosts-gate-storage.md` on `origin/main` and
  `changesets/action` opened the Version PR **`civitai-app-starters#514`**
  (`changeset-release/main`, *"chore(release): version packages"*).
  `npm view @civitai/blocks-react version` is still **`0.61.1`** — the dev-host gate reaches
  no app until that PR merges and publishes.
- **This doc is still on a branch, not mainline** — `civitai/cli#768`
  (`zach/handoff-appblocks-scope-enforcement`) is OPEN. Consequence measured this session:
  `handoff_search.py --exclude-slug appblocks-scope-enforcement` reported
  `in_scope_docs=533` equal to `indexed_docs=533`, i.e. the flag PARSED but MATCHED nothing,
  because the index deliberately skips a doc not committed to its repo's mainline. Land `#768`.
- **No `clawgate-task:` field, again.** `clawgate_handoff.sh resolve` exited **5** (unpiped),
  and `field <doc>` exited **1** (none present). A 0-task answer cannot distinguish "touched
  no task" from "wrong session id", so no field was written — not a clean bill of health.
- **Operator DECLINED fixing the two affected live apps** — *"Leave them; I'll handle it."*
  **Style Explorer (LIVE)** and **Prompt Lab (pending)** still declare one scope while using
  `useAppStorage()`, so their saves still 403 in production. Unchanged; recorded because it
  is a live user-facing defect.
- **Predecessor arc `appblocks-docs-pin-currency` is CLOSED and verified live**
  (`claudedocs/handoff-appblocks-docs-pin-currency.md`; `cli#765` still OPEN carrying it).

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

## Next steps (ranked)

1. **Merge the Version PR and verify the publish.** `civitai-app-starters#514`
   (`changeset-release/main`) is already open. Verify by RESOLVABILITY plus the OIDC
   discriminators, never a version string: `npm install --dry-run @civitai/blocks-react@<v>`
   rc=0, `_npmUser = "GitHub Actions"`, `dist.attestations` present.
   🔴 `#511` is BREAKING (minor, pre-1.0) — the published default is EMPTY `declaredScopes`,
   so every consumer whose dev harness saves without declaring a scope starts refusing.
   Closing condition: `npm view @civitai/blocks-react version` is past `0.61.1` and resolves.
   IN FLIGHT: civitai-app-starters#514
   forcing: gate — the dev-host gate reaches no app until it is published
2. **Teach the storage scopes in the generated `AGENTS.md`.** `civitai/cli`,
   `internal/cmd/templates/agents-app.md` — name `apps:storage:read`/`write` beside the
   existing `ai:write:budgeted` mention, keyed on the persistence trigger. The only surface
   an agent is actually routed to. 🔴 `agents_size_test.go` ratchets that file's size.
   Closing condition: `grep -c apps:storage internal/cmd/templates/agents-app.md` ≥ 1 on origin/main.
   forcing: none
3. **Classify authorization failures in the SDK's storage error copy.** The viewer saw
   *"Saving failed for an unknown reason… try again"*; `appStorageErrors.ts` already documents
   that an authz failure classifies `null` and that *"please try again" is the WRONG copy for
   that arm* — nothing enforces it.
   Closing condition: a block can distinguish a scope denial from a transient without parsing host prose.
   forcing: none

## Defects (batched)

- 🔴 **THIS DOC'S OWN CLOSING-CONDITION COMMAND IS NOT A POSITIVE CONTROL — it does not
  narrow.** `pnpm --filter @civitai/blocks-react test -- mockHostStorageScopes` returned
  **98 files / 1761 tests**, byte-identical to the unfiltered run: pnpm/vitest never received
  it as a file filter, so a reassuring green says nothing about whether the gate file ran.
  The real control is `npx vitest run --project unit test/mockHostStorageScopes.test.tsx`
  → **1 file / 12 tests**, a DIFFERENT and EXPECTED count. Measured 2026-10-02; the "How to
  verify" block below is corrected. Same family as the prettier-quoted-glob trap: a filter
  that matches nothing is a PASS.
- ⚠ **`--reporter=basic` does not exist in vitest 4** and fails as `ERR_LOAD_URL` /
  `loadCustomReporterModule` with a ~20-line vite module-runner stack — which reads as a
  broken repo, not a bad flag. Drop the flag.
- 🔴 **`tests/guards/blocks-react-entry-directory-names.test.mjs` treats any `from '…'`
  in a COMMENT as a module edge.** `allSpecifiers` is `/\bfrom\s*['"]([^'"]+)['"]/g` over raw
  source, so documenting an import in a JSDoc example fails the entry-layout guard with
  `unresolvable specifier`, naming a path in no real import list.
  `blockproto.StripCommentsForExt` already exists and the sibling `cli` check uses it.
  Reported on `#511`; **still not fixed** — it survived the merge.
- ⚠ **`pnpm test:guards` has one PRE-EXISTING local failure**:
  `manifest-sandbox-tokens.test.mjs` rule 2 walks into `.direnv/flake-inputs/…` and scans its
  OWN source. `origin/main` fails identically; CI never has `.direnv`. Local-environment
  artifact of the guard's corpus, not shipped content.
- ⚠ **`starters`' `pnpm typecheck` EXCLUDES `test/`**, so a type error in a test file is
  invisible to it. Do not dismiss the editor checker's output as noise.
- ⚠ **`pnpm build` leaves `packages/civitai-components/src/version.generated.ts` dirty** in a
  fresh worktree at `b4e3f00` — a build-regenerated tracked file, so a post-build
  `git status` shows one modification that is NOT yours. That drift is the subject of open
  **`starters#513`**. Do not mistake it for your own edit when checking a restore.

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

## How to verify

```bash
ST=~/workspace/civit/civitai-app-starters; CLI=~/workspace/civit/cli

# 0. THE CLOSING CONDITION — both MERGED (both were, 2026-10-02T03:45Z)
gh pr view 511 --repo civitai/civitai-app-starters --json state --jq .state   # MERGED
gh pr view 767 --repo civitai/cli --json state --jq .state                    # MERGED
#    and by CONTENT, because a squash merge breaks ancestry:
git -C $ST cat-file -e origin/main:packages/civitai-blocks-react/src/internal/mockHostScopes.ts
git -C $CLI cat-file -e origin/main:internal/validate/storagescope.go

# 1. the cli gate, at its merge commit
WC=/tmp/v767; git -C $CLI worktree add --detach $WC 87ba43bd92e5e5c90c1e35f3dc909765924cfe16
(cd $WC && go test ./internal/validate/ -run StorageScope -count=1 -v)   # 5 funcs PASS
(cd $WC && go build ./... && go vet ./...)                               # rc=0, rc=0
git -C $CLI worktree remove --force $WC
#    ⚠ whole-repo `go test ./...` has 3 PRE-EXISTING TestOracle* failures, all
#    `no Chromium on PATH` — identical to main. 🔴 NEVER set CIVITAI_CHROME here: it
#    drives a real browser and once left 637 orphaned processes.

# 2. the dev-host gate, at its merge commit (direnv allow it FIRST — per-directory)
WT=/tmp/v511; git -C $ST worktree add --detach $WT b4e3f00d4a493c54fb2b1493ca2c596ae06dd57e
direnv allow $WT
(cd $WT && direnv exec . pnpm install --frozen-lockfile && direnv exec . pnpm build)
(cd $WT && direnv exec . pnpm --filter @civitai/blocks-react test)   # 98 files / 1761 tests
#    🔴 THE POSITIVE CONTROL — `--filter … test -- <name>` DOES NOT NARROW (it returns the
#    same 98/1761). Use a real path filter and read the COUNT, which must differ:
(cd $WT/packages/civitai-blocks-react && direnv exec $WT npx vitest run --project unit \
   test/mockHostStorageScopes.test.tsx)                              # 1 file / 12 tests
#    🔴 THE NEGATIVE CONTROL — prove it can go red. Edit src/internal/mockHost.ts:1858
#    `if (needed !== null && …)` → `if (false && needed !== null && …)`, re-run the line
#    above: expect 6 failed / 6 passed naming /apps:storage:shared:write/. Then restore and
#    confirm with `git -C $WT diff --stat -- packages/civitai-blocks-react/src/internal/mockHost.ts`
#    being EMPTY — not with the `cp` exiting 0. One dirty file (components/version.generated.ts)
#    is expected after a build and is starters#513, not your edit.
git -C $ST worktree remove --force $WT

# 3. the defect itself, end to end — the fastest proof either gate works
#    scaffold an app, make it save, declare no storage scope:
#      civitai app init ./probe && cd ./probe
#      (add a useAppStorage() call; leave `scopes` as scaffolded)
#      civitai app validate          # expect the storage-scope WARNING (exit 0; --strict fails)
#      pnpm test                     # expect storage ops to REJECT naming apps:storage:write
#    ⚠ the `pnpm test` arm needs a PUBLISHED blocks-react carrying #511 — rank 1. Until
#    then a scaffolded app installs 0.61.1, which has no gate, and this arm passes vacuously.

# 4. the release (rank 1, NOT this arc) — resolvability + OIDC, never a version string
npm view @civitai/blocks-react version          # was 0.61.1 at close; must advance
npm view @civitai/blocks-react --json | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("_npmUser"), bool(d.get("dist",{}).get("attestations")))'
```
