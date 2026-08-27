# RECORD: the App Listing source-repository link — 2026-08-27

🔴 **This is a RECORD, not a live handoff. Its queue is DRAINED — see "Next
steps".** Two items shipped (#509, #510) and two were decided by Zach; nothing in
here is waiting on anyone. Read it for the measured facts and the gotchas, which
outlive the work. Do not resume from it expecting a task, and do not re-open the
decisions in items 2 and 3 without new information — they were answered, not
deferred.

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

### ✅ FIXED — `listingError`'s 403 arm appended a remedy that was false for a moderator takedown

**CLOSED by `fix/listing-403-moderator-takedown` (PR #509).** `listingError`'s
403 arm now branches on `isModeratorTakedownMsg` (declared beside
`isInsufficientScopeMsg` in `internal/appapi/appblocks.go`) before the
Apps-author-access fallback, and says the account's access is not the problem
while naming "ask a Civitai moderator to relist it". Exit code unchanged at `3`.
`README.md`'s Troubleshooting row was split in two, the source-repo section's
"the message in each case is the server's" sentence corrected, and a bullet added
to exit code 3's generated `Detail` (`internal/cmd/exitcodes_doc.go`) saying not
every `403` there is about the credential. Regression:
`internal/appapi/listing_forbidden_test.go` — red at `d2a635c`, green at HEAD.

The diagnosis below is kept as the record of what was wrong and why.

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

## Next steps — 🔴 NONE. The queue is DRAINED and this doc is a RECORD.

All four ranked items are resolved: two shipped, two decided by Zach on
2026-08-27. **Nothing here is waiting on anyone.** Do not treat the list below as
a queue — it is the disposition of each item, kept so the decisions are not
silently re-opened by the next session.

1. ~~**Narrow `listingError`'s 403 arm**~~ — **DONE**, PR #509
   (`fix/listing-403-moderator-takedown`). See the closed investigation above.
2. ~~**Set the remaining 3 offsite links**~~ — **DECIDED: leave them unset.**
   `comfy`, `radio`, `cosmetic-studio`. Not blocked-and-waiting; a decision.
   `civitai/ai-radio`, `civitai/Cosmetic-Studio` and every `comfy` candidate are
   **private** (re-verified 2026-08-27), and a link to a private repo is a 404 on
   a public store page. 🔴 **This does NOT become actionable merely because a repo
   later goes public** — it needs someone to want the link. If that happens:
   `civitai app listing set-source-repo <url> --slug <slug>` then
   `civitai app listing submit-revision --slug <slug>` — a MATERIAL change, so it
   stages a revision and needs moderator approval.
3. ~~**Whether `vitrine` should point at the civitai org's repo**~~ — **DECIDED:
   keep it, because the repo is public.** No write was made; the link was already
   live and still is. Verified 2026-08-27 two ways, because a `gh` token can read
   private repos and the store page is anonymous: `repos/civitai/vitrine` reports
   `private: false`, AND unauthenticated `GET https://github.com/civitai/vitrine`
   answers `200`. `app view vitrine --json` serves
   `sourceRepoUrl: https://github.com/civitai/vitrine`. The org-vs-owner asymmetry
   that made this a decision (the listing is Zach's, the repo is the org's) is
   accepted, not overlooked. Reversing it would be
   `civitai app listing set-source-repo --clear --slug vitrine` — another material
   change and another review cycle.
4. ~~**Port `splitItemsFloor`'s membership shape to other count floors**~~ —
   **DONE**, PR #510. ~130 guards surveyed, **5** genuinely had the hole
   (`humanbytes_test.go`, `fs_not_network_test.go`, `exitcodes_claims_test.go`,
   `readme_troubleshooting_test.go`, `read_help_test.go`). Each converted with the
   proof pair: old count guard GREEN under the trade, new membership guard RED by
   name. 🔴 The loudest find: `exitcodes_claims_test.go` guarded a **ten-row**
   ledger with `len(claims) < 5`, so five rows — including the #241 row that stops
   a script retrying a permissions error forever — were deletable outright with
   nothing going red.

### Known and UNCLAIMED — deliberately not minted as work items

Neither has an owner or a closing condition, so neither is a queue entry. They are
recorded because they were found, not because anyone is on them.

- **Four other 403 arms answering for N causes.** `submissionsError`
  (`internal/appapi/appblocks.go:1943`) and `devTunnelError` have the same
  one-arm-answers-many-causes shape PR #509 fixed. A listing takedown cannot reach
  either, so nothing is currently wrong — the ground is simply unexamined.
- **Five tables with NO collection guard at all** — including
  `internal/cmd/app_listing_advice_ledger_test.go:121`. A *missing* guard is a
  different defect from a tradeable count and was left alone on purpose; PR #510's
  body lists all five.

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
  deletion. Replaced with a membership floor over item numbers. **Swept repo-wide
  in PR #510**: ~130 guards examined, 5 more had the hole. The discrimination that
  decides it — a walker/corpus positive control like `neterr_ledger_test.go:131`
  (`len(files) < 60`) is NOT this defect; it asserts the instrument RAN, which is
  a count question by nature. Convert only guards whose purpose is "this protected
  set must not shrink" AND where losing A while gaining B is a real loss.
  🔴 **What a membership floor still does not buy:** deleting the floor entry and
  its row *together* stays green, and protection is opt-in per entry — a new row
  nobody names in the floor is permanently tradeable, and nothing reddens at the
  moment of the omission.
- 🔴 **A broken sweep regex reports a confident, quiet ZERO.** Twice in one
  session. `len\([A-Za-z0-9_.\[\]]*\)` looks fine and is not: inside an ERE
  bracket expression a backslash is LITERAL, so the class closed early and the
  pattern demanded a trailing `]`. It was "positive-controlled" against a fixed
  string that never exercised it, so the control passed while the sweep found
  almost nothing. Caught only because a hit named in the brief
  (`neterr_ledger_test.go:131`) was missing from the results. **Control a search
  pattern against a KNOWN hit in the real corpus, never against a synthetic
  string** — and note the repo-local trap that compounds it: `grep -r` here is a
  ugrep wrapper honouring `.gitignore`, so prefer
  `find … -print0 | xargs -0 grep …`.
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
