# Handoff: pkgzip-exclusion-rules — 2026-08-18

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
**The arc is closed and shipped.**

## State now

**Nothing is in flight.** `main` @ `42002af`, clean, no open PRs of mine, no
worktrees, all my branches deleted from the remote.

**v0.1.99 published 2026-08-18T23:20:40Z**, tagged at `42002af`, 14 assets.
npm `@civitai/cli` `dist-tags.latest` = 0.1.99. Homebrew cask v0.1.99 (tap commit
`ac5f97d5`, 6 s after publish). The `linux_amd64` artifact was downloaded,
**checksum-matched against the release's own `checksums.txt`**, and exercised
end-to-end — the binary that was tested is the one users get.

| PR | commit | what shipped |
|---|---|---|
| #458 | `728d558` | packager names every skipped path, with the rule that matched |
| #460 | `8617a68` | `antipattern` scans exactly what the packager ships |
| #461 | `c1684bb` | dotenv allow-list scoped to the project root |
| #467 | `075bf06` | vendorHash bot no longer commits a `/nix/store` out-link |
| #468 | `e4051c2` | depth-aware exclusion; residual population widened |
| #470 | `e7be70f` | credential warning on what IS packaged |

Issues **#464, #465, #466 CLOSED**; **#450 closed** as superseded by #467.
**#435 stays OPEN** — Row B only, below.

## What an author sees now

```
Packaged 4 file(s) (…)
Skipped 4 path(s): .env-backup/.env.production (.env*), public/environment.env (*.env),
  node_modules/, dist/
⚠ 1 packaged file(s) look like they hold credentials:
    src/db.js:1  URL with an embedded password
```

Both lines are new. The first reports what did **not** ship; the second what
**did**. 🔴 They are opposite residuals and neither substitutes for the other —
do not assume drop-messaging narrowed the leak direction. It is structurally
silent there.

## Open — one item, low value

### #435 Row B: the fixed-name directory maps are case-sensitive

`NODE_MODULES/`, `Dist/` and a file named `.GIT` still package. Bloat and VCS
plumbing, **not credentials**. The original objection to case-folding was that
`Build/`/`Dist/` are plausible content names and a silent subtree loss is
unacceptable — **that argument is now weaker**, because since #458 the loss is
printed by name on the run that causes it. Worth revisiting on its own merits;
nothing here is credential-bearing.

Rows A and C are closed: A by #461, C by #470.

## Judgement calls worth not re-litigating

- **`.local` is in the reserved-host set** and is the weakest entry — an mDNS
  name *does* resolve on a LAN. Documented at the code; first thing to revisit
  if a real report contradicts it.
- **`db.test.internal` reports.** `.internal` is a private-use TLD naming real
  infrastructure, so that is a real machine's test environment. Reserving the
  *label* `test` at any depth would silence it by exactly the argument that
  would silence `db.dev.civitai.com`. RFC 6761 reserves the `.test` **TLD**,
  handled separately.
- **The credential warning is advisory** — it never changes the exit code and
  cannot stop an upload. On the real submit path it prints as the bundle goes
  up. Enforcement, if ever wanted, belongs server-side at review time.
- **`B49`'s termination guard is unreachable by construction** and is kept and
  labelled rather than deleted: with the guard removed and the regex widened,
  the suite stops terminating. Evidence: a 38,425-line sweep found the smallest
  operator-end offset is exactly 6, the theoretical minimum.

## Gotchas / dead-ends

🔴 **A fix round introduces defects at roughly the rate an original
implementation does.** Every audit round after the first found something the
previous round had certified clean, and each was *created by the fix for the
previous finding*:

| round found | in the fix for |
|---|---|
| a golden re-approved with a sentence the PR's own test contradicted | the original feature |
| an **830×** quadratic scan (0.032 s → 26.5 s on 64 KB minified) | the label-leak fix |
| `maxLineBytes` tolerating a **1024×** change | the quadratic fix |
| a hostname-substring hole silencing real credentials | the host-blindness fix |

That is the whole argument for resetting the verification gate on every fix
rather than trusting it. Budget for it.

🔴 **Three instrument traps produced confident wrong readings in one session,
all of them documented rules that were known and still hit:**
- mutating the **first** of two identically-shaped functions (`placeholderPrefixes`
  appears in both `valueLooksLikeSecret` and `urlPasswordLooksReal`) — the wrong
  function, reported as a vacuous test;
- a `-run` keyword filter that **excluded the killing test**, hiding the above;
- `go build … | head && echo OK` printing **BUILD OK** on a compile failure,
  because the pipeline's exit status is `head`'s.

Each returned a plausible answer. The cure that worked every time: a **build
gate before any verdict**, no `-run` filter, and counted output rather than exit
codes.

🔴 **A killed run leaves a mutant in the tree.** A harness that mutates the real
worktree scored a 47-mutant battery against a poisoned baseline. Mutate in a
`cp -a` copy and `rm -f <copy>/.git` immediately — a worktree's `.git` is a FILE
pointing at the real gitdir, so a commit inside a naive copy lands on the live
branch.

🔴 **CI gates the MERGED tree; a clean textual merge is not a clean merge.**
`AGENTS.md` was 28,548 on the branch and 28,613 on main — both under the 28,758
ceiling — and **28,832 merged**. Git resolved it happily because each side
appended to a different region. Verify with a real test-merge into a detached
worktree at `origin/main`, not on the branch.

🔴 **A shared scratchpad path collides.** A stale `msg1.txt` made a `git commit -F`
silently reuse another commit's message. Use per-run unique filenames.

**The corpus figure that gates any detector change:** 244 project dirs / 3,917
packaged files / **1 project firing = 0.41%**. Re-measure it on every detector
change — a URL pattern once took it to **5.33%**, 12 of 14 findings being one
starter-template README line.

🔴 **The credential warning's false-positive rate is measured; its true-positive
rate is NOT.** The corpus holds no real credential, so detection is evidenced
only against planted fixtures. Justified by consequence — an unrecallable
credential published to Forgejo and a human reviewer — never by demonstrated
catch-rate. Do not let 0.41% read as proof it works.

## How to verify

```bash
D=/tmp/v199; mkdir -p $D
gh release download v0.1.99 --repo civitai/cli --pattern 'civitai_0.1.99_linux_amd64' --dir $D --clobber
chmod +x $D/civitai_0.1.99_linux_amd64 && $D/civitai_0.1.99_linux_amd64 version   # 0.1.99 / 42002af

A=/tmp/v199check; rm -rf $A; mkdir -p $A/src $A/public $A/.env-backup
printf '{"blockId":"x","version":"0.1.0"}' > $A/block.manifest.json
printf 'keep' > $A/src/App.tsx
printf 'TEXTURE' > $A/public/environment.env
printf 'API_SECRET=leak\n' > $A/.env-backup/.env.production
printf 'DATABASE_URL=postgres://svc:Kq9Zx2Lm5Rb8@db.prod.internal/app\n' > $A/src/db.js
(cd $A && $D/civitai_0.1.99_linux_amd64 app submit --package-only --skip-validate -o /tmp/v199.zip)
# expect: Skipped names environment.env and .env-backup/.env.production;
#         ⚠ names src/db.js:1; NO secret bytes anywhere in the output; exit 0
unzip -Z1 /tmp/v199.zip | sort    # .env-backup/.env.production must be ABSENT
```
