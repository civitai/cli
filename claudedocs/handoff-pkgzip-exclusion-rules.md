# Handoff: pkgzip-exclusion-rules — 2026-08-17

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

Close the `pkgzip` exclusion-rule leak class — the family that started at #409
(`.git` excluded as a directory but not as a file) — and get each fix to users.
Two more shapes were closed this session and three releases shipped.

## State now

**Nothing is in flight.** `main` @ `01d6692`, clean, no open PRs of mine, no
worktrees, all my branches deleted from the remote.

| shipped | tag | commit | published |
|---|---|---|---|
| v0.1.96 | — | `53a925b` | 2026-08-16T21:45:53Z |
| v0.1.97 | — | `b2a9df6` | 2026-08-17T00:29:39Z |
| v0.1.98 | — | `71a374e` | 2026-08-17T19:07:22Z |

**#420 → PR #433 (`f8eb30f`), released in v0.1.97.** Dotenv-shaped and
archive-shaped **directories** are now excluded. Before it, a planted
`.env.d/db.env` holding `API_SECRET=` was packaged into a bundle that is
committed to Forgejo `civitai-apps/<slug>` and deployed.

**#435 (partial) → PR #442 (`4e35eaa`), released in v0.1.98.** Two more rules:
`*.env` **files** anywhere (`db.env`, `prod.env` — the shape tooling writes,
which no prefix rule could see), and case-insensitive matching on the two
*pattern* rules. Six leak paths measured present in v0.1.97 and absent in v0.1.98.

**Verified, not merely deployed.** Each release ran its arms against the
*previous released binary* as a negative control on the same fixture, before
publishing. v0.1.98's run: seven leak paths present in v0.1.97, all absent;
`src/environment.ts`, `src/app.env.ts`, `config.env/settings.ts` and
`.env-backup/.env.production` still packaged; secret count in the produced bytes
**0**.

**Issue state:** #409, #412, #420, #423 CLOSED. **#435 OPEN** (residual, below).
#411 open and measured as hygiene. #410/#422/#424/#427 are the concurrent
session's App-listing area — untouched on purpose.

## Open investigations — live diagnosis state

### #435 residual: the packager matches NAMES, and a dropped subtree is silent

- **Symptom + exact repro:** a secret still ships whenever no *name* rule
  matches, and an author is never told what was dropped. Repro:
  ```bash
  mkdir -p /tmp/x/src && cd /tmp/x
  printf '{"blockId":"x","version":"0.1.0"}' > block.manifest.json
  printf 'API_SECRET=leak\n' > secrets.json
  civitai app submit --package-only --skip-validate   # use a v0.1.98+ binary, NOT $PATH
  unzip -Z1 x-0.1.0.zip     # secrets.json is present
  ```
- **Observed (measured on the released v0.1.98 binary, 2026-08-17):** still
  packaged — `secrets.json`, `src/credentials.yaml`, `NODE_MODULES/react.js`,
  `Dist/app.js` (the *fixed-name* directory list is deliberately case-sensitive),
  `.env-backup/.env.production` (kept name in a surviving directory), and a file
  named `.GIT` (the VCS map is not case-folded — pinned as a documented residual
  in `internal/pkgzip/env_suffix_test.go`).
- **Observed, the cost side:** `.env` is **Babylon.js's environment-texture
  format**. A 3D block shipping `public/environment.env` loses it, and finds out
  **at runtime in the deployed app**, because `internal/cmd/app_submit.go` prints
  only `Packaged %d file(s)` and never names a skipped path. Measured 8 files → 6
  with no message. `sample.env`/`template.env` go the same way, and the
  allow-list has no suffix-shaped counterpart (`.env.sample` kept, `sample.env`
  not).
- **Ruled out:** blanket-excluding `envs/` or `env/` as directory names —
  `src/env/` is plausible app code, and silently deleting a subtree is the
  failure mode the directory rules are deliberately aimed away from (that
  asymmetry cost four review rounds on #433 and is documented at
  `internal/pkgzip/pkgzip.go` on `isDotenvShaped` / `isDotenvSuffixed`).
- **Leading hypothesis:** the remaining exposure is not closable by more name
  rules. **Drop-messaging is the high-value piece** — it converts every current
  and future over-broad rule from a silent runtime break into a visible one, and
  it is the only fix that helps the Babylon case at all.
- **Next probe:** decide the surface, then implement. The one-line version is a
  `skipped N path(s)` summary after `Packaged %d file(s)`; the fuller version is
  `--verbose` listing them. `Build` already has the list — `Result.Files` is what
  it kept, so the skipped set needs collecting in the walk at
  `internal/pkgzip/pkgzip.go` (`filepath.WalkDir`, the `SkipDir` and
  `isExcludedFile` branches).

### #411: provenance stamp — measured as hygiene, blocked server-side

- **Observed (CNPG nvme0 replica, read-only, 2026-08-15 and again 2026-08-16 —
  identical both days):** of 22 slugs with approved rows, **20** have
  `deploy_state='live'` on their newest approved row; exactly **2** are in the
  `deployState: null`-but-serving state — `generate-from-model@0.2.7` and
  `who-am-i@0.1.0`, both via the pre-epoch legacy branch. The other two
  null-serving branches (`deploy_updated_at` set; `reviewed_at` null) hold
  **zero** rows.
- **Ruled out:** that this is urgent. The #414 liveness fix was load-bearing for
  two May-era dogfood apps, not for any first-party app.
- **Blocked on:** a server column. `appapi.Submission` has no commit/tree/dirty
  field, so the CLI cannot send what the API will not accept. 🔴 Hold the CLI
  half until the server field exists — a CLI stamping into a dropped field is the
  inert-feature shape.
- **Next probe (re-measure before acting — this decays):**
  ```sql
  SELECT count(*) FILTER (WHERE deploy_state = 'live') AS live,
         count(*) FILTER (WHERE deploy_state IS NULL)  AS null_state,
         count(*) AS total
  FROM (SELECT DISTINCT ON (slug) slug, deploy_state
        FROM app_block_publish_requests WHERE status='approved'
        ORDER BY slug, submitted_at DESC) t;
  ```
  Run it on a replica: `kubectl exec -i -n cnpg-database cnpg-cluster-nvme0-4 -c
  postgres -- psql -U postgres -d civitai`.

### Bonus finding, not yet acted on: `generate-from-model` is stuck behind its own guard

`generate-from-model` has an approved **1.0.0** (submitted 2026-05-29) followed
by seven approved **0.2.x** rows — the #412 accident already in production data,
from before the guard existed. With #414 live, every future submit for that slug
is refused unless it exceeds 1.0.0 or passes `--allow-downgrade`. Working as
designed, permanent for that app, and a ready-made real-data test case.

## Next steps (ranked)

1. **Drop-messaging for #435** — the design call above. Highest value left in
   this class; it is the only thing that makes an over-broad rule visible.
2. **`internal/antipattern/antipattern.go:198-202`** keeps its own 8-name
   `skipDirs` against `excludedDirs`' 18, and now also lacks the dotenv/archive
   rules — so `validate` can report findings in files the packager drops. One
   rule, one place. Pre-existing; impact limited by `scannedExts`.
3. **#411's stamp** — only after the server column lands.
4. Leave #410/#422/#424/#427 to the concurrent session; #453 is open against
   exactly that area.

## Gotchas / decisions / dead-ends

🔴 **"Does not close #N" CLOSES #N.** GitHub's keyword parser ignores negation,
so `close #435` inside *"Does not close #435"* closed the issue on merge. Cost:
the residual was marked COMPLETED for ~4 h. To reference without closing, write
"#435 stays open" or "the residual tracked in #435" — never the keyword.

🔴 **Five false results came from ONE glob.** The fixtures legitimately contain
**directories** named `x.ZIP`, `X.ZIP`, `config.env`, `artifact.zip`, `.env.d`.
`rm ./*.zip` refuses them and `ls -t *.zip | head -1` hands a *directory* to
`unzip`, printing errors that read exactly like a packager failure. **Target the
bundle by its exact name — `<blockId>-<version>.zip`.**

🔴 **`raw.githubusercontent.com` is CACHED.** It reported the Homebrew cask at the
previous version for five minutes after the tap was updated (workflow succeeded
19:07:24, tap commit `b23ef87` landed 19:07:32). Poll `gh api
repos/civitai/homebrew-tap/commits`, not the raw CDN.

🔴 **A URL check must read URLs OUT of the cask**, not construct them. While the
cask still read `0.1.97`, hand-built `v0.1.98` URLs answered 200 — a green that
could not detect a cask pointing at the wrong version, which is the one failure
it exists to catch.

🔴 **`gh pr checks --watch` can show a PREVIOUS run's results.** Merge was
correctly refused as `BLOCKED` while the rollup showed 11 checks, all
non-terminal. Poll the rollup for a **terminal conclusion on every check AND a
minimum count of 12**, never a keyword scan.

🔴 **GitHub's action CDN failed four jobs across three runs** (`codeload … 429/503`
in `Set up job`, nothing executed). `gh run rerun` **refuses CodeQL runs**;
`gh pr update-branch` no-ops when the branch is current; an empty commit is the
remaining re-trigger. Tagging was held deliberately — the Release workflow needs
the same endpoint, and a tag is the hardest step to take back.

🔴 **Squash merges make ancestry useless for "is it merged".** `git branch -r
--merged origin/main` reported **2** merged branches repo-wide and none of the 8
that had just landed. Verify by PR state and by content on `main`.

**Six adversarial audit rounds on #433, four on #442 — no 🔴 in any of them, and
the code was correct from the first commit of each.** Every finding was *prose
about* the code, but two mattered: a comment that argued for deleting a guard
which is the only thing stopping `.env.d/id_rsa` from being uploaded, and
case-folding with **zero** coverage through `Build` (a walk that skipped the
predicates passed 20/20 while packaging `X.ZIP/` and `DB.ENV`). One round found a
path in a comment labelled "measured" that existed only in a throwaway shell
fixture. **Read measured values off the tool's own output, never off your summary
of it.**

**Design decisions worth not re-litigating:** the `*.env` suffix rule is
**files-only** (a directory match would drop a subtree silently); the
`keptEnvFiles` allow-list stays **exact** while the match around it folds, so
`.ENV.PRODUCTION` is dropped rather than uploaded; the **fixed-name** directory
maps are deliberately **not** case-folded (`Build/`, `Dist/` are plausible
content names). Each is pinned by a test and by a killing mutant.

## How to verify

```bash
# 1. the released binary has the rules (do NOT use the `civitai` on $PATH — stale)
D=/tmp/v198; mkdir -p $D
gh release download v0.1.98 --repo civitai/cli --pattern 'civitai_0.1.98_linux_amd64' --dir $D --clobber
chmod +x $D/civitai_0.1.98_linux_amd64 && $D/civitai_0.1.98_linux_amd64 version   # 0.1.98 / 71a374e

# 2. the leak is closed — plant both shapes, package, and read the exact bundle
A=/tmp/v98check; rm -rf $A; mkdir -p $A/src $A/envs $A/.env.d $A/x.ZIP $A/config.env
printf '{"blockId":"x","version":"0.1.0"}' > $A/block.manifest.json
printf 'keep' > $A/src/App.tsx; printf 'keep' > $A/config.env/settings.ts
for f in db.env envs/prod.env .env.d/credentials.json x.ZIP/a.txt; do printf 'API_SECRET=leak\n' > "$A/$f"; done
(cd $A && $D/civitai_0.1.98_linux_amd64 app submit --package-only --skip-validate >/dev/null)
unzip -Z1 $A/x-0.1.0.zip | sort
# expect exactly: block.manifest.json  config.env/settings.ts  src/App.tsx

# 3. the suite
git -C /home/zach/workspace/civit/cli log --oneline -1 origin/main   # 01d6692
```
