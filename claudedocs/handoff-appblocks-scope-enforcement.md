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

- **`civitai-app-starters#511` — OPEN, head `1980ad7`.** Dev mock host now gates storage
  on manifest-declared scopes. 🔴 BREAKING (minor, pre-1.0 — `major` would mean 1.0.0;
  operator confirmed "we are still v0").
  - `packages/civitai-blocks-react/src/internal/mockHostScopes.ts` (new): the op→scope
    table for 15 messages, the refusal prose, the per-reply-type payload builder.
  - `.../src/internal/mockHost.ts`: `declaredScopes?: string[]` defaulting to EMPTY, and
    ONE guard ahead of the message switch (not 15 handler copies).
  - `.../test/mockHostStorageScopes.test.tsx` (new, 12 cases) + 25 migrated call sites
    across the 2 storage suites.
  - Verified: build rc=0, typecheck rc=0, **98 files / 1761 tests / 0 failures**, and the
    migration **watched red first** at 26 failed / 1723 passed.
- **`civitai/cli#767` — OPEN, head `837607d`.** `validate` warns when source calls a
  storage hook and the manifest declares no scope for that store.
  - `internal/validate/storagescope.go` + `storagescope_test.go` (new), wired in
    `validate.go` beside `readyAckChecks`, plus rows in `finding_contract_test.go` and
    `finding_fields_test.go`, plus the `app validate --help` text.
  - Verified: build/vet rc=0, `internal/validate` + `internal/cmd` ok, 20 packages ok.
    **7-mutant sweep, each killed by its OWN guard's test**, `-count=1` throughout,
    restores digest-verified, positive control in the batch.
- **NEITHER IS MERGED.** The hole is open in every shipped surface until both land.
- **CI at the time of writing:** `#511` had 5 red `Starter (…)` legs from MY regression
  (a JSDoc `import … from` line — see Gotchas), fixed in `1980ad7` and re-pushed; CI not
  re-read after that push. `#767` had `build-test` `in_progress`, 0 failures.
- **Operator DECLINED fixing the two affected live apps** — *"Leave them; I'll handle
  it."* **Style Explorer (LIVE)** and **Prompt Lab (pending)** declare the same single
  scope and also use `useAppStorage()`, so their saves 403 in production the same way.
  Not mine to touch; recorded because it is a live user-facing defect.
- **No `clawgate-task:` field:** `clawgate_handoff.sh resolve` exited **5**. ⚠ Its first
  read looked like rc=0 because the command was PIPED through `tail` — the documented
  pipe-eats-the-status trap. Unpiped it is 5, which cannot distinguish "touched no task"
  from "wrong session id", so no field was written and that is not a clean bill.
- **Predecessor arc `appblocks-docs-pin-currency` is CLOSED and verified live**
  (`claudedocs/handoff-appblocks-docs-pin-currency.md`, `cli#765` still OPEN carrying it).

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

## Next steps (ranked)

1. **Re-read CI on `#511` (head `1980ad7`) and `#767`, then merge both.**
   `civitai-app-starters#511` and `civitai/cli#767`. `#511`'s five `Starter (…)` legs were
   red from my JSDoc regression and were NOT re-read after the fix push; `#767` had
   `build-test` still running. SHA-pin the read (`gh pr view <n> --json headRefOid` then
   `gh api repos/<o>/<r>/commits/$SHA/check-runs`) — a rollup resolved at call time can
   hand back the pre-push commit's verdict.
   IN FLIGHT: civitai-app-starters#511, civitai/cli#767
   forcing: regression — an undeclared storage scope still ships silently in every surface until both land
2. **After `#511` merges, cut the release and verify the publish.** It is a changeset
   repo: merging the PR only lands the changeset, and `changesets/action` then opens a
   Version PR whose merge is what publishes. Verify by RESOLVABILITY plus the OIDC
   discriminators, never a version string: `npm install --dry-run @civitai/blocks-react@<v>`
   rc=0, and `_npmUser = "GitHub Actions"` with `dist.attestations` present.
   Closing condition: `npm view @civitai/blocks-react version` is past `0.61.1` and resolves.
   forcing: gate — the dev-host gate reaches no app until it is published
3. **Teach the storage scopes in the generated `AGENTS.md`.** `civitai/cli`,
   `internal/cmd/templates/agents-app.md` — name `apps:storage:read`/`write` beside the
   existing `ai:write:budgeted` mention, keyed on the persistence trigger. Cheap, and the
   only surface an agent is actually routed to. 🔴 `agents_size_test.go` ratchets that
   file's size — budget for it.
   Closing condition: `grep -c apps:storage internal/cmd/templates/agents-app.md` ≥ 1 on origin/main.
   forcing: none
4. **Classify authorization failures in the SDK's storage error copy.** The viewer saw
   *"Saving failed for an unknown reason… try again"*, and `appStorageErrors.ts` already
   documents that an authz failure classifies `null` and that *"please try again" is the
   WRONG copy for that arm* — nothing enforces it. Operator did not select this.
   Closing condition: a block can distinguish a scope denial from a transient without parsing host prose.
   forcing: none

## Defects (batched)

- 🔴 **`tests/guards/blocks-react-entry-directory-names.test.mjs` treats any `from '…'`
  in a COMMENT as a module edge.** `allSpecifiers` is
  `/\bfrom\s*['"]([^'"]+)['"]/g` over raw source, so documenting an import in a JSDoc
  example — an ordinary thing to do — fails the entry-layout guard with
  `unresolvable specifier`, naming a path that appears nowhere in the real import list.
  `blockproto.StripCommentsForExt` already exists for exactly this and is what the
  sibling `cli` check uses. Reported on `#511`; not fixed there.
- ⚠ **`pnpm test:guards` has one PRE-EXISTING failure when run locally**:
  `manifest-sandbox-tokens.test.mjs` rule 2. It walks into `.direnv/flake-inputs/…` and
  scans its OWN source, so the "documented sandbox literals" it flags are its own
  fixtures. `origin/main` fails identically; CI never has `.direnv`. Not a defect in the
  repo's shipped content — a local-environment artifact of the guard's corpus.
- ⚠ **`starters`' `pnpm typecheck` EXCLUDES `test/`**, so a type error in a test file is
  invisible to it; the editor's checker is the only one that sees them. It caught a real
  one here (`SharedListItem.value.title`, not `.title`) while its "cannot find module"
  errors were phantoms from a missing install. Do not dismiss all of its output as noise.

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

## How to verify

```bash
ST=~/workspace/civit/civitai-app-starters; CLI=~/workspace/civit/cli

# 0. THE CLOSING CONDITION — both merged
gh pr view 511 --repo civitai/civitai-app-starters --json state --jq .state   # MERGED
gh pr view 767 --repo civitai/cli --json state --jq .state                    # MERGED

# 1. the dev-host gate, in a worktree (direnv allow it FIRST — per-directory)
WT=/tmp/v511; git -C $ST worktree add --detach $WT origin/main
direnv allow $WT
(cd $WT && direnv exec . pnpm install --frozen-lockfile && direnv exec . pnpm build)
(cd $WT && direnv exec . pnpm --filter @civitai/blocks-react test)   # 98 files / 1761 tests
git -C $ST worktree remove --force $WT

# 2. the validate check, with its mutation sweep re-runnable
WC=/tmp/v767; git -C $CLI worktree add --detach $WC origin/main
(cd $WC && go test ./internal/validate/ -run StorageScope -count=1 -v)   # 5 funcs PASS
(cd $WC && go build ./... && go vet ./...)                               # rc=0, rc=0
git -C $CLI worktree remove --force $WC
#   ⚠ whole-repo `go test ./...` has 3 PRE-EXISTING TestOracle* failures, all
#   `no Chromium on PATH` — identical to main. NEVER set CIVITAI_CHROME here: it
#   drives a real browser and once left 637 orphaned processes.

# 3. the defect itself, end to end — the fastest proof either gate works
#    scaffold an app, make it save, declare no storage scope:
#      civitai app init ./probe && cd ./probe
#      (add a useAppStorage() call; leave `scopes` as scaffolded)
#      civitai app validate          # expect the storage-scope WARNING (exit 0; --strict fails)
#      pnpm test                     # expect storage ops to REJECT naming apps:storage:write
```
