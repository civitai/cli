# Handoff: doc-staleness guards + the v0.1.102 release — 2026-08-27

## Run this first — the index, one read-only command
```bash
python3 ~/workspace/devrc/scripts/lib/subsystem_recall.py --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Fix one wrong line in a release draft. It turned out to be **one defect shape,
four times: a document asserting a state its artifact had moved past.** The
session ended up shipping v0.1.102, building the first guard against that class,
and retiring the handoff doc that was itself two of the four instances.

## State now

- **`main` @ `0fed764`**, tree clean apart from untracked `node_modules/`.
  Base clone synced. **No claims held. Zero in-repo agent worktrees.**
- **`v0.1.102` published `2026-08-27T18:03:55Z`** and live on all three channels,
  each verified from the CONSUMER rather than the workflow that claims to update
  it: GitHub Release `isDraft:false` 14/14 assets, 1 changelog entry (#498); npm
  `dist-tags.latest = 0.1.102`; Homebrew cask `version "0.1.102"` (tap `2b51e6f`)
  with all four of its own URLs resolving 200 **unauthenticated** against a
  `v0.1.999` → 404 control. On the downloaded binary: `has("tier")` and
  `has("isMember")` true, `has("email")`/`has("emailVerified")` **both false**.
- **Merged this session:** #500 (restored the v0.1.101 page, opened v0.1.102's),
  #501 (this thread's predecessor doc — two shipped investigations still filed as
  OPEN), #502 (v0.1.102 closed as SHIPPED in the same session as its publish),
  #504 (`release_page_state_test.go`), #505 (two short-literal assertions),
  #506 (retired `handoff-whoami-capability-contract.md` as a CLOSED record).
- **All ten release pages are correctly headed**, enforced by #504.
- **IN FLIGHT, not mine:** `civitai/cli#503` `feat/listing-source-repo` —
  another session's work, several commits behind `main`. Do not touch; if it
  lands near other work, gate on the MERGED tree, not either branch's green.

## Open investigations — live diagnosis state

### Short numeric literals in `Contains` over command output — swept in `internal/cmd` ONLY

- **Symptom + exact repro:** an assertion like `strings.Contains(out, "137")`
  claims a rendered VALUE but tests only that those digits appear anywhere in the
  output. Any variable content in the same stream — a UUID, an ephemeral port, a
  temp path, a timestamp — can supply them.
- **Observed (with values):** measured on `generate_charge_seam_test.go` by
  mutating the charge to `999`, which must make the assertion impossible to
  satisfy, then running 2000 times: **8 / 2000 PASSED** = **0.40%**, ~1 in 250.
  The carrier is the **external-ID UUID** in the same stderr
  (`External ID 3c25011f-2282-402a-87ce-1be0849626c3`) — its hex digits spell
  `137`. After pinning `"Charged 137 Buzz"`: **0 / 2000**.
- **Ruled out:** that the carrier was `t.TempDir()`'s path — **wrong, and this
  was my predicted mechanism.** Probed directly: the stderr on that path is
  `"✓ Generation submitted — workflow wf_123\nCharged 137 Buzz\nExternal ID …"`
  and carries **no temp path at all**. Do not re-derive this.
  Also ruled out: the old `2 failures in 8 runs` of `go test ./internal/cmd`
  as a live mechanism — 70 and then 100 full-package runs at HEAD returned 0.
  Most likely a mid-session tree that no longer exists; **unfalsifiable, closed
  by decision**, and the 400-run capture probe that used to be prescribed for it
  is REJECTED as unable to distinguish "fixed" from "never existed here".
- **Leading hypothesis:** more instances exist in the packages never swept.
  Counted 2026-08-27 (`strings.Contains(x, "<1-3 digits>")`):

  | package | test files | with `Contains` | short numeric literals |
  |---|---|---|---|
  | `internal/cmd` | 168 | 157 | **18** (2 fixed in #505) |
  | `internal/appapi` | 26 | 18 | **2** — never examined |
  | `pkg/civitai` | 15 | 12 | **1** — never examined |
  | `internal/validate` / `genapi` / `scaffold` / `pkgzip` | — | — | 0 |

- **Next probe:** list the survivors, then judge each by whether its haystack
  carries variable content:
  ```bash
  cd /home/zach/workspace/civit/cli
  grep -nE 'strings\.Contains\([^,]+, "[0-9]{1,3}"\)' \
    internal/cmd/*_test.go internal/appapi/*_test.go pkg/civitai/*_test.go
  ```
  For each hit, the decisive test is the mutation, not reading: change the
  asserted value to something else and run `-count=2000`; **any** PASS is a
  silent green. A literal in a haystack with no variable content (a static help
  string, a fixed fixture) is fine and needs no change.

## Next steps (ranked)

1. **Sweep the 3 unexamined short-literal candidates** — repo `civitai/cli`,
   files `internal/appapi/*_test.go` (2) and `pkg/civitai/*_test.go` (1).
   Command and the mutation method are in the investigation block above. Small,
   self-contained; may well find nothing, which is a fine result to record.
2. **Decide whether the doc-staleness class deserves a guard beyond release
   pages** — repo `civitai/cli`, `release_page_state_test.go` is the only one and
   it covers `claudedocs/release-v*-draft.md` ONLY. The other instances were
   handoff docs, which are judgment-shaped ("is ranked item 1 done?") and
   probably NOT mechanically checkable. 🔴 Do not build a guard that reads as
   coverage while providing none — that is worse than no guard. A cheap, real
   option if one is wanted: fail when a doc headed `CLOSED` regains a
   `## Next steps` section.
3. **Re-baseline `minReleasePages` if old release pages are ever pruned** —
   `release_page_state_test.go` requires ≥8 pages as its positive control; there
   are 10. Pruning below 8 fails the guard loudly (by design), so the constant
   moves in the same commit as the pruning.

## Gotchas / decisions / dead-ends

- 🔴 **A `git tag`-based guard is VACUOUS in this repo's CI, and that is
  measured.** `.github/workflows/ci.yml` sets no `fetch-depth`, so
  `actions/checkout@v4` uses depth 1, and **a depth-1 clone carries zero tags**:
  `git clone --depth 1 file://<repo>` → `git tag -l` returns 0 entries and
  `refs/tags/v0.1.99` will not resolve. Any guard keyed on tags passes silently
  in the tier that gates a merge while looking authoritative locally. This is why
  `release_page_state_test.go`'s primary invariant is tag-free.
- 🔴 **A mutation SURVIVED and the fix was a third test.** Swapping
  `release_page_state_test.go`'s numeric semver sort for a string compare leaves
  the primary guard GREEN with a stale page present — lexically
  `"0.1.99" > "0.1.102"`, so the stale page sorts last and is exempted as
  "newest". Only the tag tier caught it, and that tier cannot run in CI.
  `TestReleasePageOrderingIsNumericNotLexical` kills it directly.
- 🔴 **A `git checkout -- <path>` in a mutation battery DESTROYED an uncommitted
  test, and the commit message then CLAIMED that test existed.** Caught only
  because a depth-1 clone ran 2 tests instead of 3. **Commit before mutating**,
  and verify a commit's contents with `git show HEAD:<path>`, never by reading
  the worktree.
- **`gh pr merge` prints NOTHING on success.** An empty result cannot
  distinguish success from failure — verify by CONTENT (`git show
  origin/main:<path>`) plus `gh pr view --json mergedAt,mergeCommit`. Never by
  `merge-base --is-ancestor`: a squash merge makes that permanently false.
- **`git cherry` is equally useless after a squash** — it reported all 7 commits
  of two long-merged branches as "missing upstream" because a squash changes the
  patch-id. Both branches' work WAS in `main`; proven by the PR's own squash
  commit being an ancestor, and by comparing test-function sets.
- **The retired predecessor doc is `claudedocs/handoff-whoami-capability-contract.md`.**
  It is headed `CLOSED` and has no ranked list ON PURPOSE: `claim-work` derives
  its slug from doc + RANK, so re-adding one silently re-points
  `whoami-capability-contract-N`. That already happened — rank 1 meant "fix the
  release-draft error", then "tag v0.1.102", under one slug.
- **No `AGENTS.md` item was added for the unmodelled `email`/`emailVerified`,
  deliberately.** `appapi.Identity` has nine fields and **no `Email` field exists
  anywhere in `internal/appapi`**, so a `--json` leak does not compile. A trigger
  line would cost ~310 bytes of per-session budget to restate what the compiler
  enforces. Decided 2026-08-27; do not re-open without new evidence.
- **`make ci` is not a superset of CI** — it omits `golangci-lint`. Run
  `make lint` separately, and `./scripts/ci-shallow.sh` for the depth-1 tier
  (which reads COMMITTED state only, so commit first).

## How to verify

The guards this session added, on a clean checkout of `main`:

```bash
cd /home/zach/workspace/civit/cli
go test -run 'ReleasePage|NewestRelease|TaggedRelease' -v .
go test -run 'TestGenerate_WaitingPathPrintsTheRealizedCharge$|TestTheShadowWriteWarningIsOnBothPublishedSurfaces' -v ./internal/cmd
```

All five must PASS. The release-page tier logs its own coverage — expect
`checked 10 release pages` and `ordering is numeric: … (a lexical sort would have
said v0.1.99)`.

Prove the release-page guard can still FIRE (a guard nobody has watched fail
proves nothing):

```bash
sed -i '1s/SHIPPED/DRAFT/' claudedocs/release-v0.1.99-draft.md
go test -run 'TestOnlyTheNewestReleasePageMayBeDraft' .   # must FAIL naming v0.1.99
git checkout -- claudedocs/release-v0.1.99-draft.md
```

Full gate — `make ci` is **not** a superset of CI:

```bash
make ci && nix-shell -p golangci-lint --run "make lint"
./scripts/ci-shallow.sh
```

Expect 21 packages ok / 0 FAIL, 0 lint issues, 21/21 shallow.
