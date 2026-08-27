# Handoff: whoami capability contract + the AGENTS/CLAUDE consolidation — 2026-08-26

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

`civitai whoami` reported capabilities dishonestly in three independent ways, and
`AGENTS.md`/`CLAUDE.md` were two docs claiming to be one. Both are fixed and
**shipped as v0.1.101**. This doc exists so the next session does not re-derive
the evidence, and so the one deliberately-unfinished piece (#377 option b) is
picked up with its measurements intact.

## State now

- **`main` @ `79ed55d`, tree clean** (only untracked `node_modules/`, pre-existing).
  Base clone synced with `merge --ff-only`. **No open PRs. Nothing in flight.**
- **#377 is CLOSED — option (b) shipped as PR #498** (squash `79ed55d`,
  merged `2026-08-27T00:03:20Z`, branch deleted, claim
  `whoami-capability-contract-1` released).
- **`whoami --json` now carries `tier`, `status`, `isMember`, `subscriptions`.**
  Additive: four keys added, none removed or retyped, every pre-existing key's
  value byte-identical. Only a strict-schema consumer
  (`DisallowUnknownFields` / `additionalProperties:false`) can notice.
- **Verified against the LIVE production API** through the command's own
  sanitised surface (not a fixture, not a curl of `/me`):
  `./bin/civitai whoami --json` → `tier:"silver"`, `status:"active"`,
  `isMember:true`, `subscriptions:["yellow"]`, `has("email")==false`.
  This also settles the one uncertainty three auditors flagged: `subscriptions`
  really is an array of **strings** on the live server, so `[]string` is
  measured, not inferred from the single recorded capture.
- **`v0.1.101` DOES NOT CONTAIN #498.** The tag sits on `edc1299` and published
  `2026-08-26T03:56:29Z`; `79ed55d` landed after it. #498 ships in the NEXT
  release. See the open investigation below — the release draft is wrong about
  this and it is my error.
- 11 commits on the branch, 10 audit rounds. Every round produced findings;
  rounds 1–2 were shipped behaviour, 3–10 were guard quality.

## Open investigations — live diagnosis state

### `whoami --json` still drops four modellable fields (#377 option b)

- **Symptom + exact repro:** `civitai whoami --json` omits fields the server
  demonstrably sends. Reproduce by pointing the binary at a fake `/api/v1/me`
  serving the real-capture body kept verbatim in `internal/appapi/api_test.go`.
- **Observed (with values):** six keys have nowhere to land in `appapi.Identity` —
  `tier`, `status`, `isMember`, `subscriptions`, `email`, `emailVerified`.
  Confirmed live against the released v0.1.101 binary, not inferred from code.
- **Ruled out:** *"just pass the raw body through"* — rejected. `email` /
  `emailVerified` are PII the command does not print today; making the output
  genuinely raw is a privacy regression dressed as a bug fix.
  Also ruled out: that this is a wording problem only. Option (a) — the false
  `"raw JSON"` claim in help + flag usage — **shipped in #492** and is pinned so
  it cannot regress. The dropped fields are a separate, still-live defect.
- **Leading hypothesis:** not a bug hunt — the work is known and scoped. Model
  `tier`, `status`, `isMember`, `subscriptions` on `appapi.Identity` and add them
  to the `--json` map; leave `email`/`emailVerified` unmodelled **and say so in
  the struct doc comment**, so the reasoning lives in the code.
  `isMember`/`tier`/`subscriptions` are not idle: AGENTS item 13 records that a
  caller's usable ecosystem set differs between a free and a member account, so
  the CLI is discarding the server's own answer to "is this a member account".
- **Next probe:** none needed — go straight to implementation. Note that
  `TestWhoAmIJSONShapeIsPinnedWhole` **will fail the moment the fields land**;
  that is the guard working, and its expected key set is what you update.

- 🔴 **Why this was nearly lost:** #492's body read
  `Closes #377 partially — option (a) only; (b) remains open.`
  **GitHub has no partial-close keyword** — it saw `Closes #377` and closed the
  whole issue one second after the merge, while the author was saying the
  opposite in the same sentence. Reopened `2026-08-26` with the full evidence.
  **For a half-fix, reference the issue WITHOUT a closing keyword.**

### 🔴 `claudedocs/release-v0.1.101-draft.md` now describes #498 as part of a release that shipped without it

- **Symptom + exact repro:** the draft's "Predicted contents" table carries a
  row `| 3 | feat(whoami): --json carries the account profile … (#498) |`, and a
  prose section headed *"Third `--json` change, and this one is ADDITIVE, not
  breaking (#498)"*. Both are false: v0.1.101 was already tagged and published
  when #498 merged.
  ```bash
  git rev-list -n1 v0.1.101 | cut -c1-7          # edc1299
  git merge-base --is-ancestor 79ed55d v0.1.101  # non-zero -> NOT in the tag
  git log --oneline v0.1.101..origin/main        # 79ed55d, 5d897d0
  ```
- **Observed (with values):** `gh release list` shows `v0.1.101  Latest
  2026-08-26T03:56:29Z`. `79ed55d` merged `2026-08-27T00:03:20Z` — 20 hours
  after the tag.
- **Ruled out:** *"the draft is just ahead of the tag"* — no; the previous
  handoff records v0.1.101 as **tagged, published and verified on all three
  channels** (GitHub Release 14/14 assets, npm `dist-tags.latest=0.1.101`,
  Homebrew cask `2beec20`). The draft is describing the past, not the future.
- **Leading hypothesis:** not a hypothesis — a mistake with a known cause. I
  updated the v0.1.101 draft because the audit told me the release notes were
  stale about this PR, and I never checked whether that release had already
  shipped. The `has("email")` smoke probe I added to it is good content in the
  wrong document.
- **Next probe:** none needed. The fix is mechanical, on a feature branch
  (never `main`): restore `release-v0.1.101-draft.md` to its as-shipped state
  (`git show edc1299:claudedocs/release-v0.1.101-draft.md` is the pre-#498
  text — diff it, do not blind-restore, since #497 may have touched it), and
  open `claudedocs/release-v0.1.102-draft.md` carrying the #498 section, the
  table row and the `jq 'has("tier"), has("isMember"), has("email")'` probe.

### Two test failures observed during the audit that were never attributed

- **Symptom + exact repro:** an early flake-hunting loop in an audit round saw
  **2 failures in 8 runs** of `go test ./internal/cmd` whose output was not
  captured.
- **Observed (with values):** one flake mechanism WAS found and fixed —
  `TestWhoAmIJSONNeverPublishesUnmodelledSubscriptionContent` scanned raw stdout
  for the literal `"4242"`, and stdout carries `base_url` with the httptest
  ephemeral port. Measured **5 failures / 6000 runs** pre-fix, **0 / 6000**
  after. Structural bound: 13 of the 28232 ephemeral ports contain `4242`
  (42420-42429, 34242, 44242, 54242) = 1 in 2172.
- **Ruled out:** that the 2 uncaptured failures are explained by that mechanism
  — **not established.** The measured rate is ~1 in 1200; 2 in 8 is far too hot
  for it. A later round ran **70** full-package runs at HEAD with **0**
  failures, and another ran 100 — neither can see a ~1/100 mechanism reliably.
- **Leading hypothesis:** most likely the same class (a short literal an
  ephemeral port / temp path / timestamp can spell) in a test nobody has swept.
  Two instances of that class were found and fixed this session
  (`whoami_test.go`'s `4242`, `cmd_test.go`'s `Contains(out,"99")`), and a third
  false-claim instance in `workflow_settlement_regression_test.go`.
- **Next probe:** run the package to failure and CAPTURE the output —
  ```bash
  cd /home/zach/workspace/civit/cli
  for i in $(seq 1 400); do
    go test ./internal/cmd -count=1 >/tmp/ic-$i.log 2>&1 || { echo "FAIL run $i"; break; }
  done; tail -40 /tmp/ic-$i.log
  ```
  Then grep `internal/cmd` for `strings.Contains` over command output with a
  literal shorter than ~4 chars.

## Next steps (ranked)

1. **Fix the release-draft error** — repo `civitai/cli`, files
   `claudedocs/release-v0.1.101-draft.md` (restore) and a new
   `claudedocs/release-v0.1.102-draft.md`. Feature branch + PR; evidence and the
   exact commands are in the open investigation above. **This is a correction of
   something this session got wrong**, not new scope.
2. **Tag v0.1.102 when the maintainer wants it** — `main` is green and carries
   one user-visible change since v0.1.101 (#498, additive `--json` keys). 🔴
   `AGENTS.md` makes tagging and publishing SEPARATE consents, and publishing the
   draft fires npm + the Homebrew tap on the same click. Do not do either without
   being asked. Do step 1 first or the notes will be wrong.
3. **Reap two stale in-repo agent worktrees** — `git worktree list` shows
   `.claude/worktrees/agent-a06dc1f0c76f4f7c7` (holding `fix/offsite-refusal`)
   and `.claude/worktrees/agent-a84febaf8c91e65fe` (holding `audit392`). They
   live INSIDE the repo, are gitignored, and hold those branches repo-globally;
   they also polluted an auditor's `grep` with stale copies of `whoami.go` until
   it switched to `find | xargs grep`. Check for unique work before removing.
4. **Decide on `AGENTS.md` item 33** — "`email`/`emailVerified` are unmodelled
   on purpose". I did NOT add it: the rationale sits in a 🔴 block at the exact
   struct field an editor must touch, backed by two tests that go red, and
   `AGENTS.md` has ~950 bytes of headroom (28,650 / 29,600). An auditor
   independently argued FOR adding it. ~310 bytes for a trigger + a
   `claudedocs/decisions/33-*.md`. Judgement call, still open.
5. **Watch the additive `--json` change in the wild** — no code pending. If a
   consumer reports breakage it will be a strict-schema decoder; the four new
   keys are the only difference.

## Gotchas / decisions / dead-ends

- 🔴 **Five guards in this repo were green while testing nothing**, all found by
  mutation rather than reading. Worth assuming more exist:
  - `whoami_test.go`'s glyph list would have gone **vacuous on a header rename** —
    it checks `--json` does not leak human markers; a renamed marker is a string
    the payload can never carry. Proven: the old 3-entry list **survives** a
    `Credential:` leak; the 4-entry list kills it.
  - `agents_size_test.go`'s eviction playbook named the wrong inline set for
    months (`"2 and 4"` after item 30 went inline). Now **computed**, plus a guard
    that crosses slicers so it cannot agree with itself.
  - The README's `whoami` fenced block was a **silent excerpt** of its own
    command's output. Now byte-locked by a bidirectional seam guard.
  - `whoamiRow` nearly ate a colon, which would have left a Troubleshooting guard
    satisfied **by a comment**.
  - `Closes #N partially` — see above.
- **"Granted Capabilities" was proposed and rejected.** It is *less* accurate than
  plain "Capabilities": `Credential type` is not a capability at all, and a
  personal key's `Submit Apps: yes` is **not a grant** — the backend does not
  scope-gate submit for personal keys, so the gate simply does not apply (for
  OAuth it genuinely *is* the `ScopeAppBlocksSubmit` bit). The section also prints
  `no` and `unknown` rows, so it is a checklist with verdicts, not a list of
  grants. Resolved by **splitting** into `Credential:` + `Capabilities:`.
- **`refactor(` is not filtered from the changelog.** The exclude list in
  `.goreleaser.yaml` is exactly `docs`/`test`/`chore` with an optional scope
  group. So #494 appears in the published notes — judged correct (it is a
  user-visible change) rather than accidental. Same class as `ci(` last release.
  Widening the filter was considered and rejected: untested edit to a file whose
  failure mode is invisible until a release ships.
- **`AGENTS.md` item 4 was false the day it was written.** The bitmask landed
  `908f981` (2026-06-25, #36); item 4 denying it landed `b0968d7` (2026-06-29,
  #70) — whose own subject is *"correct false 'vendored token-scope bitmask'
  claim"*. The correction introduced the falsehood.
- **`agentsMaxBytes` was raised deliberately, once**, 28,758 → 29,600, and it is
  **not** drift: #493 made the two docs *smaller together* (29,823 → 28,661), so
  holding the old ceiling charged `AGENTS.md` for bytes the session stopped paying
  next door. `agentsMaxBytesCeiling` stayed 30,600 and did not need re-deriving —
  the achieved size rose, so its property got *stricter* untouched.
- **Publishing a release fires npm AND the Homebrew tap on the same click**, and
  npm unpublish is restricted. `AGENTS.md` makes it a separate consent from
  tagging for exactly that reason. Tag → draft → verify artifacts → ask.
- **A feature probe reading an EXIT CODE reports a false PRESENT on this CLI.**
  Cobra falls back to *parent* help on an unknown subcommand, so `rc=0` says
  nothing. Read the **first line of help**. Control: `app definitelynotacommand
  --help` also exits 0 with the same fallback text.

- 🔴 **A `git checkout HEAD -- <path>` restore DESTROYS uncommitted edits in
  that path, and a mutation battery runs it every iteration.** Two real edits
  were lost this way mid-session and then CLAIMED in a commit message; the suite
  stayed green through the whole round because an error string and a struct tag
  have no tests. The durable fix is in the battery: refuse to run when the
  mutation targets are dirty. Commit before sweeping, always.
- 🔴 **`Subscriptions` is `[]string`, NOT `json.RawMessage` — and RawMessage was
  tried first and was wrong twice.** It published server bytes verbatim to
  `--json` (an object-shaped element carrying a billing email would ship with no
  code change), and it did not even buy the drift-resilience it was chosen for,
  because a drift in `tier`/`status`/`isMember` blanks all four regardless. Do
  not "restore" it.
- 🔴 **A GUARD'S DESCRIPTION IS A COVERAGE CLAIM, AND THIS SESSION WROTE FOUR
  THAT WERE WIDER THAN THEIR CODE.** In order: a banned-substring ledger that a
  paraphrase (`Member: yes`) and a case-change (`ACTIVE`) both walked past; a
  golden that closed addition on 1 of 6 render paths while a 🔴 comment said
  four; a pair table named "every rendered value" that was organised per value
  *source*; and an injectivity test that was blind to any widening keyed on what
  it had already normalised away. Each was found by mutation, never by reading.
- 🔴 **AGENTS item 28's phrase-ledger lesson is now 4-for-4.** The recorded fix
  is golden-output pinning. But a golden only closes ADDITION on the branches
  its FIXTURES EXECUTE — a body without profile fields cannot see a row behind
  `if id.Tier != nil`. That is the half item 28 does not say.
- 🔴 **A short literal in a `Contains` over command output is a coin flip in
  BOTH directions.** `"4242"` gave a false RED ~1/1200 (ephemeral port);
  `Contains(out,"99")` would have given a silent GREEN ~1/35 (2.88% of ephemeral
  ports contain `99`). The silent green is worse. Three instances found; assume
  more exist.
- **`make lint` caught a `gofmt` violation that `make ci` passed over**, live,
  this session — exactly as `AGENTS.md` says. Run both.
- **A mutant that fails to COMPILE is not a kill.** Recorded a KILLED verdict
  off a missing import once; caught by requiring `go vet` to pass first.
- **`gh pr merge --squash` breaks ancestry forever** — verify a merge landed by
  CONTENT (`git grep <symbol> origin/main`) plus `gh pr view --json mergedAt`,
  never `merge-base --is-ancestor`.
- **`Closes #N partially` does not exist** — GitHub closed #377 one second after
  #492 merged, while the sentence said the opposite. For a half-fix, reference
  the issue WITHOUT a closing keyword. (#377 is now legitimately closed.)

## How to verify

The shipped behaviour, against a local build of `main`:

```bash
cd /home/zach/workspace/civit/cli && make build
./bin/civitai whoami --json | jq '{tier, status, isMember, subscriptions,
  leaked_email: has("email"), leaked_emailVerified: has("emailVerified")}'
```
Expect the four profile fields populated and **both `leaked_*` false**. Use the
command's own `--json` (not `curl /api/v1/me`) — it is the curated projection by
design, so the probe cannot print the PII even by accident.

For the degraded path, serve a fixed `/api/v1/me` on loopback:

| body | `--json` must show |
|---|---|
| no profile keys | all four `null` |
| `"subscriptions":[{"billingEmail":"…"}]` | all four `null`, no PII anywhere |
| `"tier":3` (drift in ONE field) | all four `null`, command still succeeds |

Full gate — and `make ci` is **not** a superset of CI:

```bash
make ci && nix-shell -p golangci-lint --run "make lint"
./scripts/ci-shallow.sh     # the depth-1 tier CI actually uses
```
