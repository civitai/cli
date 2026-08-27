# Handoff: the App Listing source-repository link — 2026-08-27

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

`civitai/civitai#4314` (merged 2026-08-23) added an optional public
source-repository link to an App Listing — one `Source` row on the `/apps`
**detail** page. It has two authoring paths and this CLI covered only one. Close
the gap, then set the link on every app that has a public repo.

## State now

- **Branch `main` @ `7b5284f`, clean, in sync with origin. No open PRs. Nothing in flight.**
- **`civitai app listing set-source-repo` shipped** — PR #503, squash `7b5284f`,
  +1914/−45 across 9 files. Worktree removed, claim `listing-source-repo-cli-1` released.
- **8 of 11 live listings now serve `sourceRepoUrl`.** Verified through the public
  API (`app view <slug> --json`), not inferred:

  | | |
  |---|---|
  | 7 onsite (sensei, app-requests, gen-matrix, custom-generators, model-benchmarking, playable-collections, panorama-360) | live, via the manifest `repository` key |
  | vitrine (offsite) | live, via the new command → revision `apl_01M12EDABXV9MG4TSP2D7C456V` → publish request `alpr_01M12EGCMB4WA47S1PN8Z52MR3` → approved |
  | comfy, radio, cosmetic-studio | **unset — every candidate repo is private** |
  | 10 delisted apps | unreachable via the public detail endpoint |

- **Both halves of #4314 are now exercised against production.** The offsite chain
  (listing write → material staging → moderator revision → approve carrying the
  value back to the parent) had never run end to end anywhere before today, in
  this repo or in tests. `applyApprovedRevision` does carry `sourceRepoUrl` back —
  measured, not assumed.

## Open investigations — live diagnosis state

### `listingError`'s 403 arm appends a remedy that is false for a moderator takedown

Not mid-diagnosis — root cause and evidence are below and nothing is unknown.
Recorded because it is unfixed, it now has a **documented destination** (the new
README exit-code table publishes `a moderator removed the listing → 3`), and the
next person to hit it will be told to fix access that is already fine.

- **Symptom + exact repro:** any `civitai app listing` write against a listing a
  moderator removed. The server's own sentence is correct; the CLI appends a
  false diagnosis.
- **Observed (with values):** `internal/appapi/listing.go:1003` —

  ```go
  return fmt.Errorf("not permitted for your account (403): %s — managing store listings needs Apps-author access (invite-only beta)", msg)
  ```

  With the server's real 403 body (`offsite-listing.service.ts:1242`,
  `this listing has been removed by a moderator and can no longer be edited`) the
  user sees both sentences. The caller's Apps-author access is **not** the problem.
  `README.md:3560` repeats the same wrong diagnosis for that fragment.
- **Ruled out:** that it is specific to `set-source-repo` — it is not; the arm is
  shared by every listing write. That it is new — it predates PR #503, which does
  not touch that arm. That the scope arm is also wrong — `:1000-1001` gates on
  `isInsufficientScopeMsg(msg)` and is correct.
- **Leading hypothesis:** one 403 arm answering for N causes — the shape this repo
  catalogs as #374/#391, one level out. The takedown message is distinguishable by
  its text, exactly as the scope case already is.
- **Next probe:** none needed for diagnosis. The change is to branch `:999-1003`
  on the takedown message the way `:1000` branches on the scope message, and to
  split `README.md:3560` into two rows. Confirm the exact server string first:
  `git -C $CIVITAI show origin/main:src/server/services/blocks/offsite-listing.service.ts | sed -n '1240,1244p'`

## Next steps (ranked)

1. **Narrow `listingError`'s 403 arm** — repo `civitai/cli`, files
   `internal/appapi/listing.go:999-1003` and `README.md:3560`. Evidence and the
   exact change are in the open investigation above. Pre-existing; deliberately
   not folded into a listing-feature branch.
2. **Set the remaining 3 offsite links, IF their repos go public** — `comfy`,
   `radio`, `cosmetic-studio`. Blocked, not forgotten: `civitai/ai-radio`,
   `civitai/Cosmetic-Studio` and every `comfy` candidate are **private**
   (re-verified 2026-08-27). A link to a private repo is a 404 on a public store
   page. Command once unblocked:
   `civitai app listing set-source-repo <url> --slug <slug>` then
   `civitai app listing submit-revision --slug <slug>` — it is a MATERIAL change,
   so it stages a revision and needs moderator approval.
3. **Decide whether `vitrine` should point at the civitai org's repo.** The listing
   is yours; `github.com/civitai/vitrine` is not. Live now. Reversing it is
   `civitai app listing set-source-repo --clear --slug vitrine` — another material
   change, another review cycle, so it is a decision rather than a cleanup.
4. **Consider porting `splitItemsFloor`'s membership shape to other count floors.**
   `agents_split_preserved_test.go` — a count floor was defeated by add-one/
   delete-one (see gotchas); any other `len(x) >= N` guard in this repo has the
   same hole. Not surveyed.

## Gotchas / decisions / dead-ends

- 🔴 **`set-source-repo` is a SEPARATE command from `set-text`, and the reason is
  measured.** `sourceRepoUrl` is in `MATERIAL_PATCH_FIELDS`, and on an approved
  listing the server writes the **FULL** patch (material + trivial) to a shadow as
  soon as any material field differs
  (`offsite-listing.service.ts:1318-1339`, quoted from its own comment). So a
  `set-text --tagline X --source-repo Y` would stage BOTH — the tagline edit would
  stop applying in place. A flag would have changed what the existing flags do,
  conditional on an unrelated flag. Enforced structurally: `appapi.ListingPatch`
  is a closed interface with a field ledger (`internal/appapi/listing_patch_ledger_test.go`).
- 🔴 **The CLI deliberately does NOT pre-validate the repo URL** — AGENTS item 33,
  `claudedocs/decisions/33-source-repo-url-is-not-mirrored.md`. The one mirror this
  repo ships (the vendored schema `pattern`) is measurably wrong in BOTH
  directions: it accepts all 7 values in the server's own drift-guard corpus that
  the server refuses, and rejects `https://GITHUB.COM/o/r/` which it accepts.
- 🔴 **`beginListingRevision` is IDEMPOTENT.** A material edit joins an
  already-open shadow rather than minting its own, so "send it for review"
  publishes everything staged there. The command distinguishes the two, and the
  distinction is only observable BEFORE the write — afterwards every staged edit
  has a `shadowId`. `hadOpenRevision(ref)` is the one predicate, shared with
  `set-text` (it was open-coded at 4 sites).
- 🔴 **`app listing status` can MINT a revision on a live listing.** Do not reach
  for it to check whether one exists — that creates the state you are checking
  for. `set-source-repo`'s own read (`getMyListingForApp`) is side-effect-free and
  reports `openRevision`.
- 🔴 **Five audit rounds, and FOUR of them refuted a claim I had made about my own
  work** — each proven with a surviving mutant, never argued. A closed interface
  stops another *package*, not this one widening a struct. A born-split guard
  searched for a heading string AGENTS.md has never contained, so every real move
  passed. A ledger promised "a third implementation cannot appear unnoticed" and
  enumerated nothing. A README table told scripters two states "both exit 1" when
  they are 2 and 3. **Assume more of this exists.**
- 🔴 **A COUNT floor is defeated by add-one/delete-one.** `minSplitRows = 31`
  rested on "append-only, so any deletion drops below the floor" — but an eviction
  wave is *the* commit that adds rows, so it is the natural vehicle for the
  deletion. Replaced with a membership floor over item numbers.
- 🔴 **A mutation battery run against a DIRTY tree destroys uncommitted work.**
  Its `git checkout --` restore ate an entire round of edits; caught only because
  the file was missing from `git status` before committing. The battery must
  refuse to run dirty. Commit first, always.
- **Exit codes for the refused states are NOT uniform**, and prose said they were:
  owner-unpublished → `MATERIAL_CHANGE_BLOCKED` → mapped explicitly to
  `BAD_REQUEST` → **2**; moderator-removed → 403 → **3**; migration not applied →
  412, unclassified → **1**. Pinned by
  `cmd/civitai/set_source_repo_exitcode_test.go`, which **reads** the README table
  rather than restating it (editing the table to publish `9` was green before).
- **`make lint` caught a deprecated `parser.ParseDir` that `make ci` passed over** —
  exactly as AGENTS.md says it will. Run both.
- **The `Long` help field is a backtick-delimited raw string.** A backticked
  command inside it terminates the literal; use `+ "` … `" +` concatenation.

## How to verify

The shipped command, and the state of every app:

```bash
cd /home/zach/workspace/civit/cli && make build
./bin/civitai app listing set-source-repo --help
for s in sensei app-requests gen-matrix custom-generators model-benchmarking \
         playable-collections panorama-360 vitrine comfy radio cosmetic-studio; do
  printf '%-22s %s\n' "$s" "$(./bin/civitai app view $s --json | jq -r .sourceRepoUrl)"
done
```

Expect 8 URLs and 3 `null` (comfy, radio, cosmetic-studio — private repos).

The refusal paths, which cost no write:

```bash
./bin/civitai app listing set-source-repo https://github.com/x/y --slug sensei; echo "rc=$?"   # 1, on-site verdict
./bin/civitai app listing set-source-repo --slug vitrine >/dev/null 2>&1; echo "rc=$?"          # 2, usage
```

🔴 Read `rc` directly — `$?` after a pipe is the pipe's status, which reported a
misleading `0` for these during the session.

Full gate — `make ci` is **not** a superset of CI:

```bash
make ci && nix-shell -p golangci-lint --run "make lint"
```
