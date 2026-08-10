# Handoff: dogfood-3 (first CREDENTIALED blind dogfood) — 2026-08-10

## Goal

Close the coverage gap both previous dogfood runs left: they were deliberately
un-credentialed, so `civitai generate` — the CLI's only irreversibly money-spending
surface, and the subject of AGENTS items 12–22 — was **structurally unreachable**. Run a
blind agent with a real credential and find what only a credentialed user can hit.

Prior runs: [`handoff-dogfood-154.md`](handoff-dogfood-154.md) (#223–#227),
[`handoff-dogfood-2.md`](handoff-dogfood-2.md) (#255–#260).

## State now

- **`main` @ `3156b80`.** Moved a lot during this session — a parallel workstream landed
  #333–#347. Do not assume any SHA in this doc is current.
- **The run is DONE.** 3 generations, 61 Buzz, 2 submissions, all within limits,
  **verified from the account** (balance 4,187,454 → 4,187,393) not from the agent's report.
- 🔴 **~13 findings are UNFILED.** They are in this doc (below) and nowhere else.
  Finding 1 is filed as **#350** (2026-08-10 17:12 CDT).
- ✅ **`dogfood3-20260810-money` v0.1.1 WITHDRAWN** — 2026-08-10 17:11:32 CDT,
  `pubreq_01KZPCQRKCBJ9GHQHQ1HFW08AK`, rc=0. Both versions now read `withdrawn` in
  `app status`. Nothing of this run's is in a moderator queue.
- **PR #296 (the sandbox harness) MERGED** at `2fe5336`, 2026-08-10T18:53. See the harness
  section — it landed with known 🔴 defects unfixed.

### The setup that produced this — reproduce it, do not rebuild the harness

```bash
D=/tmp/claude-1000/dogfood3
mkdir -p "$D/work" "$D/cfg/civitai"
cp <clean-build>/bin/civitai "$D/civitai"      # build from a WORKTREE, not git archive:
                                               # archive drops git metadata -> version "dev"
cp README.md "$D/README.md"
printf 'token: <key>\n' > "$D/cfg/civitai/config.yaml"; chmod 0600 "$_"
export XDG_CONFIG_HOME="$D/cfg"
```

Then a fresh agent with: work only in `$D/work`; use only `$D/civitai`; **do not read the
repo, `AGENTS.md`, `claudedocs/`**; `--max-cost` on every generate; a generation cap; a
submission cap; every app named with an identifiable prefix; keep a command log.

That is the whole harness. It cost minutes and produced 14 findings on the first run.

## Open investigations — live diagnosis state

### `app withdraw` → resubmit silently destroys the entire store listing

- **Symptom + exact repro** (all rc=0, no warning at any step):
  ```
  civitai app submit --yes                                    # pubreq_01KZPC3P55C105AYP7Z428RZWX
  civitai app listing set-icon ./assets/icon.png              # ~40s content scan
  civitai app listing set-cover ./assets/cover.png
  civitai app listing add-screenshot ./assets/shot1.png --caption "Main view"
  civitai app listing status                                  # Icon ✓ / Cover ✓ / Screenshots: 1
  civitai app withdraw pubreq_01KZPC3P55C105AYP7Z428RZWX      # rc=0
  # bump version 0.1.0 -> 0.1.1 in block.manifest.json
  civitai app submit --yes                                    # pubreq_01KZPCQRKCBJ9GHQHQ1HFW08AK
  civitai app listing status
  ```
- **Observed (verbatim):**
  ```
  App:             dogfood3-20260810-money
  Listing status:  draft
  Icon:            MISSING (required)
  Cover:           MISSING (required)
  Screenshots:     0

  ⚠ Not publishable yet — missing icon and cover.
  ```
  rc=0. Re-checked after 20s and via `--slug` — stable, not a cache artifact. Icon, cover
  **and the captioned screenshot** all gone.
- **The tool promises the opposite in THREE places** (this is what makes it data loss
  rather than a surprise):
  - README quickstart step 7: *"do it now, while the app is in review; **it carries forward
    on approval**"*
  - `app listing --help`: *"**the media you attach carries forward when a moderator approves
    it. Set it early** to clear the publish floor before you go live"*
  - `app submit`'s own success output: *"You can add them NOW, while the app is in review —
    **they carry forward on approval**"*

    …and the resubmit **reprinted that line over an empty listing**.
  - `app withdraw --help` presents withdraw as the normal repair path: *"Withdraw your own
    pending App submission **so you can resubmit a new bundle for the same slug**"*.
- **Ruled out:** caching (re-checked at +20s and through a different code path via `--slug`);
  a wrong-app lookup (`--slug` explicit, same result).
- ✅ **RESOLVED — the loss is AT WITHDRAW, and it deletes the LISTING, not just its media.**
  Measured 2026-08-10 17:11 CDT, at zero extra submission cost: the probe was run on the
  v0.1.1 listing that had to be withdrawn anyway. Transcript:
  ```
  17:11:22  app listing status  -> Icon set ✓ / Cover set ✓ / Screenshots: 1 (re-added)
  17:11:32  app withdraw pubreq_01KZPCQRKCBJ9GHQHQ1HFW08AK  -> rc=0, no warning
  17:11:36  app listing status  -> Error: no store listing found for this app (404)  rc=4
  ```
  Four seconds, three images, no warning. **Reuse this trick:** the probe is free whenever a
  submission is being withdrawn for other reasons — re-attach, verify, withdraw, re-check.
- 🔴 **CORRECTION to the report above — icon and cover SELF-HEAL, the screenshot does not.**
  Between 12:50 and 17:11 the v0.1.1 draft listing went `MISSING` → `set ✓` for icon and
  cover with **no user action**; the captioned screenshot stayed gone over the same ~4h20m.
  So the post-resubmit `MISSING` is partly a stale read that reconciles server-side over
  hours — but the CLI prints `⚠ Not publishable yet` throughout that window, and the caption
  is lost for good. The `404` after withdraw is unambiguous regardless. This is why the
  finding was NOT filed as written: live state contradicted "all gone, permanently".
- **Why it matters:** withdraw→fix→resubmit is the *only* documented repair path, and the
  docs actively push you to attach media *before* reaching it. A user with eight captioned
  screenshots loses all of them and has no reason to re-check, because the tool just told
  them it carried forward.

## The findings — UNFILED, ranked

Only #1–#5 have full reproductions above/below; the rest are one-liners with enough detail
to re-derive. **Check for duplicates before filing** — a parallel workstream filed #342–#347
on 2026-08-10 and some are adjacent (notably **#345**, the `--checkpoint` example in
`generate --help`, which is the same *class* as finding 3 but a different surface).

| # | severity | finding |
|---|---|---|
| 1 | 🔴 | ✅ **FILED #350.** **`app withdraw` deletes the whole store listing** (above). Silent, rc=0, on the documented repair path, contradicted by three promises. |
| 2 | 🟠 | **`app pull` gives a false diagnosis whose own remedy disproves it.** `app pull ./pulled --app <pending-app>` → `Error: no such app for your account — check the slug with 'civitai app status': App block not found` (rc=4). `app status` lists it twice. Real cause: `app pull --help` says it pulls *approved* apps; the app was `pending`. The README Troubleshooting table repeats the same false explanation. Compare `app metrics` on the identical precondition, which is exemplary: *"has no approved App Block yet — analytics only exist once a submitted version is approved; check where it is in review with `civitai app status <slug>`"*. |
| 3 | 🟠 | **The README's silent-model-substitution worked example cannot produce its own output.** `civitai generate "a cat" --checkpoint 999999999 --dry-run` → `Error: --checkpoint 999999999: not found (404): Model not found` (rc=4). The item-13 live `ResolveModelVersion` lookup now 404s before the substitution path is reachable. The section that exists to teach substitution demos it with a command that short-circuits. No invocation was found that produces the documented warning. |
| 4 | 🟠 | **Troubleshooting index attributes a string to the wrong command.** The README promises *"Every row's left column is a fragment of a string this CLI really prints"* and cites a test asserting it. The row `is this an App project?` is attributed to validate; `app validate ./emptydir` actually prints `block.manifest.json not found at project root ./emptydir`. The quoted string comes from `app listing`. The test checks **existence, not attribution**, so the invariant holds while the index is wrong. The message a new user really gets from validate appears nowhere in the index. |
| 5 | 🟠 | **`app dev-token`'s remedy block recreates the problem.** Three defects in eight lines: the final suggested command omits `--spend` (reproducing the state both warnings complain about); *"(or add it to `scopes` in block.manifest.json)"* is already satisfied per the warning four lines above; and the same warning prints twice, straddling the token on stdout. Consequence per the CLI's own text: `dev:live` "dead-ends silently". |
| 6 | 🟠 | `app listing status` before submit → `Error: no such submission` (rc=4). Three words, no next command, in a tool whose house style is always to name one. Fix: `— run 'civitai app submit' first; the draft listing is created at submit time`. |
| 7 | 🟠 | **`app pull`'s argument shape is undiscoverable.** Every other `app` subcommand takes the slug positionally; `app pull` takes it as required `--app` and uses the positional for the directory. Three natural attempts all fail without ever mentioning `--app`, one with a bare Cobra `accepts at most 1 arg(s), received 2` — the only unguided framework error found in the whole tool. |
| 8 | 🟡 | **`--version` is documented as global; it is root-only.** README "Global flags": *"accepted by **every** command"*. `app validate --version` / `models search --version` → `unknown flag` rc=2. The other four rows genuinely are global. (The colour-precedence contract in the same section was verified fully correct.) |
| 9 | 🟡 | **`scopeJustifications.<scope>` is documented as an emitted dotted field; it emits bare `scopeJustifications`.** Both trigger paths checked. Minor, but it is in the section whose whole point is that CI consumers group on `field` (AGENTS item 23). |
| 10 | 🟡 | `app status` prints 100 rows with no `--limit`, while its sibling `app list` has `--limit` and `--cursor`. |
| 11 | 🟡 | Internal procedure names leak into user errors: `appListings.setIcon rejected the request (400)`, and `workflows get <bad-id>` → `no such generation procedure or resource (404)`, which reads like the CLI is broken rather than the input. |
| 12 | 🟡 | `app metrics` with no argument → bare `accepts 1 arg(s), received 0`, while `app dev-token` and `app dev-tunnel` in the same situation are excellent. |
| 13 | 🟡 | An img2img job shows `Workflow: txt2img` on the confirmation screen you approve. Correct per item 19(a) (the server promotes it) but unexplained at the point of approval. |
| 14 | 🟢 | Smaller: "square-ish, not exactly square" reads as prohibiting 1:1 while recommending 512×512; README links to `AGENTS.md` twice for info a released-tarball user does not have; `generate --json` omits `externalId` while the human path prints it and the crash-recovery recipe needs it; scaffold `dev:harness`/`dev:tunnel` pin port 5186 with no knob and die with a raw Vite trace; `✓ Icon set ✓`; `app view` truncates mid-word without an ellipsis; the BLOCK_READY advisory is ~200 words as the first warning a newcomer sees; `.env.development` excluded from the bundle while `.env.production` is included, unstated. |

### What the run verified as WORKING — protect these

- **Exit-code contract holds across ~20 cases**, including the fussy 1-vs-2 filesystem split
  this session's own #263/#265 established (`--input <unreadable>` → 1, `<missing>`/`<dir>` → 2;
  `set-icon <unreadable>` → 1; `app validate /nope --json` → 2 with **zero bytes** on stdout).
- `--json` validate contract exact: a manifest broken six ways produced 9 findings, **all**
  with non-null dotted `field`s.
- The scaffold→build→test→validate→submit arc worked first try (148 tests green).
- The lockfile validation message (item 3) was singled out as the best in the tool, and its
  workspace-root clause "saved a monorepo user an afternoon".
- Both BLOCK_READY tiers behaved correctly (strong vs weak), and `--strict` gated (rc=1).
- Money safety: non-TTY refusal names the amount; `--max-cost` correctly described as not a
  cap; `workflows cancel --help` opens with "CANCELLING IS NOT A WAY TO SAVE MONEY";
  `--dry-run` genuinely spent nothing.

## The harness — merged, and this is the part to be careful about

🔴 **PR #296 MERGED at `2fe5336`** despite the decision to abandon it, and **the audit-5
findings were never fixed** (round 7 was stopped mid-flight). What is on `main` therefore
includes, measured:

1. **`scripts/dogfood-sandbox.sh selftest` against a live sandbox BRICKS IT** — go/no-go step
   3. 22 failures, then it deletes `observed_spend`, every counter, `meter_broken` and
   `calibration.ok`, strips `REQUIRE_CALIBRATION` from the policy and never restores it;
   afterwards every command returns rc=126 *"the sandbox policy is unusable"*. The offline
   fixture pins `REQUIRE_CALIBRATION=0`, the exact value the bug keys on, so 266 assertions
   and 64 mutants cannot see it.
2. **Any npm dependency's lifecycle script reads the credential and zeroes the meter** —
   measured via an ordinary `preinstall` hook. Residual 13 claims the jail has "no real
   credential store"; it contains the credential *and* the whole ledger, read-write.
3. **`rm ledger/meter_broken` un-latches the meter**, and the refusal message names that path.
4. `apply_settlement` treats an unreadable `observed_spend` as `0` — the one fallback the
   file forbids everywhere else, silently, after the money moved.

**Do not run `dogfood-sandbox.sh` against anything real without fixing those.** The
round-7 fixes exist as a patch at `/tmp/claude-1000/fix296g-scratch/round7-uncommitted.patch`
(~860 lines) — **volatile, dies at reboot.**

**What is genuinely worth keeping from it:** `internal/dogfoodguard` (a classifier that asks
cobra's real `Find`/`ParseFlags` instead of modelling it — four independent differentials at
45, 70 and 46 nodes plus a recording origin server found **zero** disagreements with the
binary), and `scripts/dogfood-mutate.sh` (64-mutant re-runnable battery).

🔴 **CORRECTION (measured 2026-08-10 17:2x) — "keep those two, delete the sandbox" does not
hold together.** The three pieces are coupled, so it is delete-the-complex or keep-and-fix:
- `scripts/dogfood-mutate.sh` line 3 declares itself *"the mutation battery for
  scripts/dogfood-sandbox.sh"* and line 33 targets that file. Delete the sandbox and it has
  nothing to mutate.
- `internal/dogfoodguard`'s **only** consumer is the sandbox, which builds it at line 440.
  Orphaned it is 291 lines of dev-only code with **no test file** (`internal/dogfoodguard/`
  contains `main.go` and nothing else).
- `make dogfood-check` (Makefile) runs `shellcheck` + `selftest --offline` against the
  sandbox, so removal is a Makefile change too, and `claudedocs/dogfood-3-sandbox.md`
  documents it.

Sizes, if that helps the call: sandbox 2,633 lines / 125 KB, mutate 178, dogfoodguard 291.

## Next steps (ranked)

1. ✅ ~~Withdraw `dogfood3-20260810-money` v0.1.1~~ — done 2026-08-10 17:11 CDT.
2. ✅ ~~File finding 1 (the listing wipe)~~ — **#350**, with the withdraw-vs-resubmit
   attribution resolved.
3. 🔄 **IN FLIGHT** — findings 2–14 are being re-verified against a binary built from current
   `main` and filed. 🔴 **They are STALE as written:** the run's binary was `dfdcc97`, **14
   commits behind `main`**, and #334/#336/#339/#347 changed `generate`, `workflows` and
   `scaffold`. Some may already be fixed — finding 6 is the worked example (the same binary
   gave a fully actionable "run `civitai app submit` first" message on a no-listing app, not
   the bare `no such submission` recorded). **Re-verify before filing anything from this
   table.** Filing plan: 2/3/4/5 individually, then three thematic issues (error-message
   quality 6+7+11+12; README contract accuracy 8+9; papercuts 10+13+14) rather than one
   9-item umbrella, which never gets actioned. Findings 2 and 13 cannot be re-verified
   without a live submission or Buzz — file with that caveat stated, do not spend.
   Confirmed NOT duplicates: finding 3 vs **#345** (that is a *real* checkpoint `128713`
   failing on ecosystem mismatch; finding 3 is a nonexistent id `999999999` 404ing before
   substitution is reachable).
4. **Decide the harness's fate on `main`** — fix the four defects above, or delete the whole
   complex. See the CORRECTION above: the three pieces are coupled, so "keep dogfoodguard +
   dogfood-mutate.sh" is not an option that survives deleting the sandbox.
5. **Rotate the API key used for this run** (it was pasted into a chat transcript, and lives
   at `/tmp/claude-1000/dogfood3/cfg/civitai/config.yaml`). Verified 2026-08-10: the token
   string appears **0 times** in the repo and **0 times** in its full git history, so nothing
   here needs scrubbing — rotate because of the transcript and the on-disk copy, then
   `rm -rf /tmp/claude-1000/dogfood3`.

## Gotchas / decisions / dead-ends

- 🔴 **The harness was the dead end.** 7 rounds, 5 blind audits, ~24 🔴 — and it found
  **nothing about the CLI**. Every money control was *convention*: a shell script running as
  the agent's own uid, beside a credential it could read and a binary it could execute
  directly. That was true in round 1 and stated in the very first residual list. The bound
  people wanted needs **a second uid, a container, or a platform-capped credential** — not a
  better shell script. The minimal setup at the top of this doc replaced all of it.
- **The dogfood agent must be a FRESH agent.** The agent that builds or fixes tooling has read
  the source and cannot be the subject.
- **Build the dogfood binary from a git WORKTREE**, not `git archive` — archive drops git
  metadata and `git describe` yields version `dev`.
- **Blindness is convention now, and that is fine** — the threat model is a *cooperative*
  agent. A cooperative agent told "you have exactly three sources of truth" complies.
- **Verify spend from the account, not the agent's log.** `civitai buzz` before and after.
  Note the account has concurrent activity from other workstreams, so a balance delta is no
  longer solely attributable to one run.
- Nine "guards that could not fail" from the previous workstream are catalogued in
  [`handoff-dogfood-2.md`](handoff-dogfood-2.md); the same shapes recurred throughout this
  session's harness work.

## How to verify

```bash
# the run's artifacts, from the account (not from any agent's report)
export XDG_CONFIG_HOME=/tmp/claude-1000/dogfood3/cfg
/tmp/claude-1000/dogfood3/civitai app status | grep dogfood3-20260810
/tmp/claude-1000/dogfood3/civitai buzz | grep -i total

# the operator's real config was never touched
md5sum ~/.config/civitai/config.yaml     # b508d46aa048… at session end

# the harness defect, if you doubt it (use a THROWAWAY root)
scripts/dogfood-sandbox.sh init --root /tmp/probe … && scripts/dogfood-sandbox.sh selftest
# expect: FAIL=22, then every subsequent command rc=126
```
