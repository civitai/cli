# Handoff: submit-guards-and-app-drift — 2026-08-15

## Run this first — the index, one read-only command
```bash
python3 ~/workspace/devrc/scripts/lib/subsystem_recall.py --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

*(As of 2026-08-15 this returns `status=scope-absent` — the store has no `cli/` directory yet.)*

## Goal

Close the silent-regression hole in `civitai app submit` (issues #411, #412), ship it to
users, and reconcile the five first-party apps whose source repos had fallen behind their
live deployed version.

## State now

**All of it is merged, released and verified. Nothing is in flight.**

`civitai/cli` `main` @ `1b20b99`. Working tree clean. **Zero open PRs.**

Shipped in `v0.1.95` (published 2026-08-15T04:14:15Z, tagged at `fcd27fd`, npm
`@civitai/cli@0.1.95`, Homebrew cask 0.1.95, 14 assets):

| what | PR | merge |
|---|---|---|
| `app status` warns when local manifest is BEHIND highest approved | #413 | `3a53c2f` |
| `app submit` refuses version ≤ highest approved (`--allow-downgrade`) | #414 | `773c72e` |
| `app submit` refuses a dirty git work tree (`--allow-dirty`) | #415 | `fcd27fd` |
| release notes + changelog-filter regex fix | #416 | `e816594` |
| `.git`-as-a-FILE excluded from bundles (closes #409) | #418 | `7857a94` |
| one slug predicate instead of four | #421 | `7652b7d` |
| README ToC gate widened to `###` | #417 | `1b20b99` |
| scaffold pins → 0.35.0 | #419 | `d801fd7` |

**Issue state:** #412 **CLOSED** (both halves shipped). #411 **deliberately OPEN** — only the
dirty-tree half shipped; the provenance stamp needs a server field. #409 closed by #418.
**#420 filed** by this session (see below).

**The five drifted apps are reconciled and merged** — 24 files that existed nowhere in git are
now in git, **zero deletions** in any of the five:

| app | merge | main was → now |
|---|---|---|
| app-requests | `595abd1` | 0.2.0 → 0.2.2 |
| playable-collections | `3d0cdec` | 0.2.0 → 0.2.2 |
| model-benchmarking | `b015df2` | 0.1.3 → 0.2.2 |
| custom-generators | `51b18ea` | 0.4.0 → 0.5.2 |
| gen-matrix | `f699eab` | 0.7.3 → 0.8.2 |

**Verified, not just deployed:** the `linux_amd64` release artifact was downloaded, checksum-
verified against `checksums.txt` (`OK`), and driven through four arms *before* publishing —
clean repo packages (exit 0); dirty repo refuses (exit 1, names the path, **no network call**);
`--allow-dirty` passes through; **no git repo submits normally** (scaffold path intact). Homebrew
was checked the way it actually breaks: every archive URL the cask names answers **200
unauthenticated**. Post-merge, `app status custom-generators` from merged `main` reads `0.5.2`
and the drift warning is silent.

## Open investigations — live diagnosis state

### Sizing the `deployState: null` + `liveUrl` population (gates the #411 provenance stamp)

- **Why it matters:** an adversarial audit caught `app submit` telling authors a **serving** app
  was "not live" and recommending `--allow-downgrade` — the exact #412 accident, recommended by
  the guard built to prevent it. Fixed in #414's delta round (`rowIsServing()` reads `liveUrl`
  first). The *mechanism* is confirmed; the *blast radius* is unmeasured.
- **Observed (server source, read directly):**
  - `<civitai>/src/shared/constants/app-block-deploy.constants.ts:106-119` — `isApprovedAndServing`
    returns **true** for an approved row with `deployState === null` when *any* of: `deployUpdatedAt`
    is set, `reviewedAt` is null, or `reviewedAt < DEPLOY_STATE_TRACKING_EPOCH_MS`.
  - `<civitai>/src/pages/api/v1/blocks/submissions.ts:132,148` —
    `liveUrl: isLive ? "https://${row.slug}.${appsDomain}/" : null`, i.e. `liveUrl` is non-null
    **exactly** when that predicate holds.
  - `internal/cmd/app_status.go:216` gates `Live at:` on `s.LiveURL != nil && *s.LiveURL != ""` —
    so pre-fix, `app status` and `app submit` contradicted each other on the same row.
- **Ruled out:** that `deployState == "live"` is sufficient — it is not; it is a strict subset of
  what the server treats as serving.
- **Unknown:** how many production rows are actually in that state. Three separate agents flagged
  they had no DB access; nobody has measured it.
- **Next probe** (read-only, CNPG nvme0 — recipe in the `app-blocks` skill):
  ```sql
  SELECT count(*) FILTER (WHERE deploy_updated_at IS NOT NULL) AS legacy_transitioned,
         count(*) FILTER (WHERE reviewed_at IS NULL)           AS no_anchor,
         count(*) FILTER (WHERE reviewed_at < '2026-06-15')    AS pre_epoch,
         count(*)                                              AS total
  FROM app_block_publish_requests
  WHERE status = 'approved' AND deploy_state IS NULL;
  ```
  A large legacy population ⇒ the #414 liveness fix was load-bearing and the stamp is urgent.
  Near-zero ⇒ the stamp is hygiene and can queue behind cheaper work.

## Next steps (ranked)

1. **Size the `deployState: null` population** (query above), then pitch #411's provenance stamp
   to the server side as a **two-column migration** — `source_commit` (nullable text) +
   `source_dirty` (boolean) on `app_block_publish_requests`, surfaced through `shapeRow`.
   Deliberately *not* branch/remote/tree-hash: the CLI already computes commit + dirtiness for
   #415's guard, so the client half is nearly free, and every extra field is another migration
   for information that does not answer "which commit is this?".
   🔴 **Hold the CLI half until the server field exists** — a CLI stamping into a field the API
   drops is the inert-feature shape.
2. **#420** — `.env.local` / `*.zip` excluded as FILES but not DIRECTORIES; a planted
   `.env.d/db.env` containing `API_SECRET=leak` **was packaged**. Mirror of #409, credential-shaped.
   Deliberately not fixed in #418 (different blast radius: a directory rule would start dropping
   `.environment/` whole).
3. **`app status` ignores its parent context** — `internal/cmd/app_status.go:98` builds its own
   `context.Background()` and never reads `cmd.Context()`, so the command is uncancellable by its
   caller. Pre-existing, unrelated to #412, small and well-defined.
4. **#410** — a withdrawn newest row hides an approved+deploying one in `app status`.
5. Storage-key coverage gap in gen-matrix's recovered suite: mutating `RUN_STORAGE_KEY`'s **value**
   survives, because tests import the constant symbolically rather than asserting a literal.
   Pre-existing, not introduced by the sync.

## Gotchas / decisions / dead-ends

🔴 **`grep -r` here is a SHELL FUNCTION that silently skips `.git/`.** From the Claude Code shell
snapshot; `type -a grep` confirms. Measured with a token planted in **both** `.git/config` and
`src/planted.txt`: the function returns **only** the `src/` hit; `/run/current-system/sw/bin/grep`
returns both. Worse than a zero — it produces a *positive* result, so the scan looks like it works.
Matters because **`civitai app pull` embeds an access token in the clone's `.git/config`** (it warns,
then does it). Three agents reproduced this independently. Full write-up:
`<claude-project-memory>/feedback_grep_r_is_a_shell_function_that_skips_git.md`.

🔴 **`gh run rerun` replays the SAME merge SHA** — re-running a check after merging its fix does
nothing. It re-tested stale pins and failed identically. Use **`gh pr update-branch`**.

🔴 **`pins-vs-published` is time-dependent, not flaky.** It compares scaffold pins against **live
npm**, so it goes red with no commit when a new version publishes (it did, mid-session:
`@civitai/app-sdk` 0.34.0 → 0.35.0). Fix is `gh workflow run bump-scaffold-pins`, not a re-run.

🔴 **Two individually-green PRs produced a tree that did not compile,** and `git merge` reported
no conflict. #413 and #414 each wrote their own `highestApprovedVersion` in package `cmd` with
incompatible signatures, plus an identically-named test. **Always build the merged tree before
merging siblings that touch one package.** Consolidated into `internal/cmd/approved_version.go`;
a cross-command contract test now pins that `app status` and `app submit` name the same version
for the same rows (that contract had no test on either side, which is why two agents could
disagree about it).

**Reconciliation policy (used for all five apps), and why:**
- **Overlay, never mirror.** `rsync -a --exclude='/.git/' --exclude='/block.manifest.json'`, no
  `--delete`. The bundles **exclude** repo files (`brand/`, `docs/`, sometimes `LICENSE`), so a
  mirror deletes CI config and licenses. Assert `git diff --diff-filter=D --name-only` is **empty**.
- **The manifest is FIELD-MERGED, never copied.** `iframe.src` is **server-owned** and the validator
  rejects a dev-set value → a verbatim copy leaves the repo **unsubmittable**. Two server-owned
  fields: `iframe.src`, `trustTier`.
- **Drift is BIDIRECTIONAL.** Repos hold post-deploy commits (taglines, brand/, a test *improvement*
  in model-benchmarking that replaced a rotting literal with a lockstep invariant). A blind overlay
  reverts them. Check per file whether the repo's last commit postdates the deploy.
- 🔴 **That date check is BLIND to a DELETION made in the deployed line** — it asks about the repo's
  history when the fact lives in the bundle's. custom-generators v0.5.0 deleted the dev-tunnel
  wiring; since rsync never deletes, taking the deployed `vite.config.ts` would leave dead code plus
  a broken dev flow. **Tell: a differing file whose deployed side adds nothing of substance is a
  deletion wearing a modification's clothes.** Check with
  `git -C <bundle> log --all --diff-filter=A -- <path>`.
- **Acceptance test:** the drift warning must **fire on pristine main and go silent on the branch**,
  plus control A (force a lower version → must warn, proving it reads your file) and control B
  (manifest absent → silent, proving the false-negative mode exists and yours isn't it). A silent
  run alone proves nothing.

**Evidence the root cause was incomplete trees, not just uncommitted ones:** gen-matrix's bundle
deleted `.github/workflows/ci.yml` **in the same commit that added five test files**, and dropped a
`pnpm-workspace.yaml` line that breaks CI on pnpm 9 (CI's version; local pnpm 11 passes either way).
playable-collections' deployed bundle is byte-identical to `oss/zach/playcol-0.2.2-ux-polish` except
the server-injected `iframe.src` — independent confirmation the bundles are genuine repo work.

**Release mechanics:** goreleaser cuts a **DRAFT**; publishing the draft is what fires **npm AND the
Homebrew tap** (both trigger on `release: published`), and npm unpublish is restricted. Verify the
artifact *before* publishing. The changelog `exclude` patterns needed an optional scope group —
`^docs:` never matched `docs(release):`, which is why three releases counted "leaking commits" by
hand. Fixed in #416 and pinned by `changelog_filter_ledger_test.go`.

## How to verify

```bash
# 1. the guards are live in the released binary
D=/tmp/v195; mkdir -p $D
gh release download v0.1.95 --repo civitai/cli --pattern 'civitai_0.1.95_linux_amd64' --dir $D --clobber
chmod +x $D/civitai_0.1.95_linux_amd64 && $D/civitai_0.1.95_linux_amd64 version   # 0.1.95 / fcd27fd

# 2. the drift is closed — each must print its deployed version and NO warning
R=/home/zach/workspace/civit/civitai-app-custom-generators
git -C $R worktree add --detach /tmp/vcg origin/main
env -C /tmp/vcg $D/civitai_0.1.95_linux_amd64 app status custom-generators   # Version: 0.5.2, no ⚠
git -C $R worktree remove --force /tmp/vcg

# 3. the cli repo itself
git -C /home/zach/workspace/civit/cli log --oneline -1 origin/main            # 1b20b99
```

🔴 **Do not use the `civitai` on `PATH`** for any of this — it is a stale `v0.1.89-…` dev build that
predates the drift warning entirely, so the check would be vacuous.
