# Handoff: listing-revision-and-submit-size — 2026-08-17

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

Close #430 (`app listing reorder` 400s on every live listing) and everything it dragged
in: the false-success sibling it exposed, the copy that survived both, the predicate they
share, the missing machine-readable read, and — separately — #423's unreleasable-app
papercut. Verify against the LIVE API, not only fakes.

## State now

- **Branch:** `main` @ `509babc`, clean, in sync with `origin/main`.
- **Six PRs merged this session**, each verified by CONTENT in `origin/main` (never by
  ancestry — a squash merge makes `merge-base --is-ancestor` false forever):

  | sha | PR | issue | what |
  |---|---|---|---|
  | `b3a35c6` | #434 | #430 | `reorder` targets the shadow revision, not the parent |
  | `c27b368` | #437 | #436 | `rm-screenshot` stops claiming a change it only staged; adds `app listing submit-revision` |
  | `a5cb1ac` | #444 | #443 | the publish line stops telling you to remove MORE screenshots |
  | `474a113` | #445 | #440 | last open-coded publish-floor check folded into `floorMet` |
  | `d3d2798` | #449 | #447 | `app listing status --json` |
  | `509babc` | #452 | #423 | report the base64 JSON submit-body size; say what was sent when a submit fails |

- **LIVE-VERIFIED (not just fakes)** against `model-benchmarking` on `v0.1.97-3-gc27b368`:
  - `reorder` on a live listing → exit 0, `New screenshot order staged on a revision —
    pending moderator review (alpr_01M0687DVPXCT0NPDNPA0JM126)`. Previously a hard
    `400: orderedIds must be exactly the listing's current screenshot ids`. Confirmed
    end-to-end: after moderator approval the reorder reached the PUBLIC gallery.
  - `rm-screenshot` → `✓ Screenshot removal staged on an open revision — not submitted for
    review yet.` (no bare `✓ Screenshot removed`).
  - `submit-revision` → returned `alpr_01M0687DVPXCT0NPDNPA0JM126`, the SAME id a previous
    session had to obtain with a hand-rolled tRPC call. Idempotent; nothing moved.
  - `--json` and the submit-body size were smoke-tested on the merged binary (below).
- **Verification side effect, now RESOLVED.** Testing shrank `model-benchmarking` from 4
  screenshots to 2; the restore was staged and has since been APPROVED. Measured
  2026-08-17: public = 4, staged = 4, order identical to the original (uncaptioned first,
  then *Every model…*, *Vote on checkpoint…*, *Prompts are voted on…*). Nothing outstanding.
  Note `status --json` still reports `hasPendingRevision: true` — see the open block below.
- **Worktrees:** 20 → 17. Removed three clean detached `/tmp` release-check leftovers
  (`cli-v194`, `wt-cli-v0190`, `wt-cli-v0192`), each re-checked immediately before removal
  and each an ANCESTOR of `origin/main`. **No branch was deleted.**

## Open investigations — live diagnosis state

### #423 — is the server's submit ceiling a ~10 MB request-BODY limit?

- **Symptom + exact repro:** `civitai app submit --yes` on a bundle around 8 MB compressed
  → `Error: server returned 400: Invalid JSON`. Naming nothing size-shaped.
- **Observed (with values).** `SubmitVersion` (`internal/appapi/appblocks.go`) base64-encodes
  the zip into a JSON body, so the server sees ~4/3 of the zip:

  | bundle | zip | JSON body sent | submit |
  |---|---|---|---|
  | known-good control | 2.32 MB | ~3.09 MB | ✅ accepted |
  | `gen-matrix@0.8.3` | 8.20 MB | **~10.94 MB** | ❌ `400: Invalid JSON` |
  | after trimming | 0.18 MB | ~0.23 MB | ✅ accepted |

- **Ruled out:**
  - *"The CLI is swallowing a better error"* — it is not. `SubmitVersion` → `serverError` →
    `fmt.Errorf("server returned %d: %s", status, msg)` with `msg` the server's own body.
    **`Invalid JSON` is what the SERVER said.** `serverError` is untouched, with a comment
    on the failure path recording why, so it is not re-attempted.
  - *"File count"* — the accepted control carries MORE files (90 > 68).
- **Leading hypothesis:** a ~10 MB request-body limit in front of the app. A body truncated
  or refused at that boundary fails JSON parsing, producing an error about the PARSE —
  downstream of the real cause, which is why it names nothing about size. A 10 MB body
  limit implies a zip ceiling of ~7.5 MB, inside the measured bracket.
- 🔴 **The bracket is all that is known: `(2.32 MB, 8.20 MB]`.** `MaxBundleSizeBytes` is
  deliberately UNCHANGED at 50 MiB and no server ceiling is vendored — a cap guessed from
  inside that bracket would refuse bundles the server accepts, unappealably, with nothing to
  distinguish the CLI's guess from a real limit. Recorded as **AGENTS.md item 31**;
  `internal/pkgzip/caps_claim_test.go` bans the "mirrors the server" phrasing from returning.
- **Next probe** (settles it, but costs real submissions): bisect between 2.32 MB and
  8.20 MB compressed with throwaway apps, reading the new `Packaged … bytes as the base64
  JSON submit body` line to know the body size at each step. Cheaper alternative: read
  `civitai/civitai` for the body-parser/proxy limit on the submit route — no submissions at all.

### #389 — `getMyListingForEdit` is classified a read while it opens a revision

- **Why it is live now:** `app listing status --json` shipped (#449) on top of that route, so
  a script can now poll a "read" that mutates. The flag's `--help`, its usage string, the
  README command row and a README bullet all say so and name #389. **The contradiction is
  documented, not resolved.**
- **Observed today:** after the restore was approved, `status --json` reports
  `hasPendingRevision: true` and the human view prints `A revision is currently under
  moderator review`, while the public gallery already serves the restored 4. Consistent with
  a shadow opened by the status read itself — but `hasPendingRevision` is documented to mean
  SUBMITTED, not "a shadow exists" (`internal/appapi/listing.go` doc comment).
  **Unexplained; do not assume which.**
- **Next probe:** read `model-benchmarking`'s revision list server-side, or call
  `getMyListingForApp` (which does NOT open a shadow) and compare its
  `hasPendingRevision` against `getMyListingForEdit`'s in the same minute. If they disagree,
  the flag is being set by the read.

### Are the remaining 13 worktree branches merged? — UNRESOLVED, instrument failed

- **Why it matters:** ~17 worktrees remain; several likely hold merged work, and stale ones
  blocked `gh pr merge --delete-branch` three times this session.
- **Ruled out — three instruments, all inconclusive:**
  - `gh pr list --head <branch> --state all` → **no PRs for any of the 16** (they predate the
    current flow). Not evidence of anything.
  - Full commit-SUBJECT matching vs `main` → mostly `PARTIAL 1/3`, which is exactly what a
    squash merge looks like. Cannot separate "fully merged" from "half-merged".
  - Reverse-applying each branch's diff to `main` (`git diff main...br | git apply --check
    --reverse`) → **identical answer for all 16**. 🔴 **POSITIVE CONTROL FAILED:**
    `feat/pkg-civitai-sdk` reported "not in main" while `pkg/civitai/` holds 30 files in
    `main` and `refactor: promote internal/api to importable pkg/civitai SDK (#172)` is in
    the log. Cause: the branches are 26–245 commits behind, so their diffs no longer share
    context and the patch fails for reasons unrelated to whether the change landed.
- **Next probe:** per-branch, by hand — for each, read what it changed and grep `main` for
  that specific change today. ~13 small judgements; no script found that does it reliably.
- 🔴 **Do NOT touch two of them:** `cli-offsite-listing` is ACTIVE — another session
  committed `a410ce3 feat(listing): reach an OFFSITE app's store listing by slug (#422
  outcome 1)` on top of `509babc` while this session ran. `cli-oidc-draft` holds an untracked
  `OIDC-PR-BODY.md` that exists nowhere else.

## Next steps (ranked)

1. **#422 outcome 1 is being built RIGHT NOW by another session** in
   `/home/zach/workspace/civit/cli-offsite-listing` (`feat/offsite-listing-slug-fallback` @
   `a410ce3`). Coordinate before touching `app_listing.go` / `app_offsite.go` — this session
   merged six PRs into those files.
2. **#389** — now load-bearing because `--json` invites polling a mutating read. The probe
   above is one comparison, no submissions.
3. **#424** — `resolveListing`'s slug-only lookup. Server source already explains the 404s
   (`where: { slug, kind:'onsite', appBlockId:null, status:'draft' }`); the open question is
   only whether the genuinely-pending onsite path works.
4. **#427** — the offsite refusal asserts ownership it never checked; its probe has no off switch.
5. **#411** — submit provenance (commit SHA + dirty flag). The dirty-tree refusal already shipped.
6. **The 13-worktree merge review** — lowest value; the three that caused real friction are gone.

## Gotchas / decisions / dead-ends

- 🔴 **`grep -c` answers a different question than "is it gone".** Verifying #444 landed,
  `grep -c 'remove the rest first'` returned **1** and looked like a failed merge. The one hit
  was the doc comment recording what the line USED to say; the live `Fprintf` was correct.
  **Read the hit, never the count.**
- 🔴 **A `-run` filter that excludes the killing test reports SURVIVED.** Verifying #445 I ran
  `-run 'Floor|Ledger|Predicate'` against tests actually named
  `TestPublishFloorHasOneDefinition` / `TestFloorQuestionScannerSeesShapesNotInTree`. The
  mutant looked survived until I read the real `func Test` names.
- 🔴 **`golangci-lint run ./...` lints the shell's CWD, not the worktree.** My first lint of a
  worktree reported `0 issues` having run in the base clone. Use
  `nix-shell -p golangci-lint --run "make -C <abs-worktree> lint"` and **confirm the
  `Entering directory` line**. A second attempt with an absolute `<path>/...` pattern printed
  a typechecking error AND `0 issues` — the zero was still meaningless.
- 🔴 **Python's `json.dumps` inserts a space after the colon; Go's `json.Marshal` does not.**
  Independently checking the new submit-body size, my figure was 1 byte higher (`{"bundleBase64": ""}`
  = 20 vs Go's 19). The code was right. An unexplained off-by-one in a size calc is exactly
  what gets waved through as rounding — resolve it, don't round it.
- **Verify a squash merge by CONTENT, never ancestry** — false forever after a squash.
- **`mergeable` goes `UNKNOWN` right after a push.** Poll until it settles; `gh pr view
  --json mergeable,mergeStateStatus` is the only authority.
- **Gate on a MINIMUM check count, not just "none pending".** On two PRs the first poll saw
  11 checks; the 12th had not been created yet.
- **This checkout is shared and moved repeatedly mid-session** — `main` advanced from
  `b1213a3` → `ebe5315` → `483441e` → `bb7d7f2` → `4e35eaa` → `d3d2798` under me, with #420,
  #435, #441, #446, #448 landing from other sessions. Always branch off a FETCHED
  `origin/main`, never a local ref.
- **Decision: `reorder` auto-submits, `rm-screenshot` does not.** A reorder is one complete
  operation; curation is N removals, and auto-submitting on the first would open a review
  cycle mid-edit. Recorded as **AGENTS.md item 30**. The asymmetry is deliberate and open to
  revisit — the attach path also gates on `--changelog`/`--yes` in a non-TTY while `reorder`
  does not gate at all.
- **Decision: `editTargetId` re-asked and re-refused** (#447). `ListingRef`'s doc comment
  records that the refusal now has a user-visible consequence: adding the decode means adding
  the field to the `--json` payload too.
- **Two subagent mutants SURVIVED and were reported, not patched** — #437's 400-gate (fixed by
  adding a `503` control row) and #449's M11 (`parentId` from either read; the two name the
  same listing, so forcing a fixture apart would pin a distinction the server does not make).
  Both correct calls.

## How to verify

```bash
cd /home/zach/workspace/civit/cli && make build

# #449 — the read that was missing (⚠ opens a shadow revision on a live listing)
./bin/civitai app listing status --slug model-benchmarking --json | jq '{parentId, shadowId, status, hasPendingRevision}'

# #423 — the size the server actually sees, on any project dir
./bin/civitai app submit <dir> --package-only --skip-validate --out /tmp/x.zip
#   -> "Packaged N file(s) (… bytes compressed, … decompressed; … bytes as the base64 JSON submit body)"
#   check: EncodedLen(zip) + 19  == the reported body size

# full gate — make ci is NOT a superset of CI (it omits lint)
make ci && nix-shell -p golangci-lint --run "make -C /home/zach/workspace/civit/cli lint"
```

Live behaviour of #434/#436 is already confirmed (see **State now**); re-running it costs a
real moderator review cycle and is not needed.
