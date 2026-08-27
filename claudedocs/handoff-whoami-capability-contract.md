# CLOSED — Handoff: whoami capability contract + the AGENTS/CLAUDE consolidation (2026-08-26 → 2026-08-27)

🔴 **THIS DOC IS A RECORD, NOT A QUEUE. Nothing in it is waiting for you.**
The work shipped (#492, #494, #498, #500, #501, #502, #504, #505) and released as
**v0.1.102**. Read it for the reasoning and the gotchas; do not read it for
something to do. New work gets a NEW handoff doc — see the residuals section for
why re-adding a ranked list here is actively harmful.

**Every "state" claim below was true when written and is not re-verified.** A
sha, a branch list, a "no open PRs" — all decay. Re-measure before acting on any
of them; that is the exact failure this thread produced four times.

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
`AGENTS.md`/`CLAUDE.md` were two docs claiming to be one. Both were fixed and
**shipped as v0.1.101**; the deliberately-unfinished piece (#377 option b)
shipped in **#498** and released as **v0.1.102**.

The doc then outlived its own goal by two days, which is how it became a case
study in the defect it was recording. What it is now good for is the reasoning:
the gotchas, the retracted theories, and the measurements — none of which are
re-derivable from the diff.

## State as of closing (2026-08-27) — a SNAPSHOT, already decaying

🔴 This section used to be headed "State now" and carried a `main` sha. It was
wrong within hours, twice. It is kept for provenance only; **re-measure anything
you intend to act on.**

- **`main` @ `d1afcbc`** at the time of writing, tree clean apart from untracked
  `node_modules/`. **`v0.1.102` published and live on all three channels.**
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
  `2026-08-26T03:56:29Z`; `79ed55d` landed after it. **#498 ships in v0.1.102**,
  whose notes are now written: `claudedocs/release-v0.1.102-draft.md`.
- **The release drafts are correct and the v0.1.101 page is CLOSED** (PR #500,
  squash `b04501d`). `release-v0.1.101-draft.md` is headed `SHIPPED` with a
  measured outcome table; the false #498 row and prose section are gone. 🔴 **Do
  not edit a `SHIPPED` page** — that is exactly the mistake #500 corrected.
- 11 commits on the branch, 10 audit rounds. Every round produced findings;
  rounds 1–2 were shipped behaviour, 3–10 were guard quality.

## Resolved since this doc was written — do NOT re-derive these

🔴 **Both entries below were filed under "Open investigations — live diagnosis
state" and have SHIPPED.** They are kept because the reasoning is worth reading,
not because there is work in them. A resolved investigation left under an "open"
heading is the same defect as a shipped release page still headed `DRAFT` — and
this doc had both at once.

### ✅ `whoami --json` drops four modellable fields (#377 option b) — SHIPPED in #498

`tier`, `status`, `isMember`, `subscriptions` are modelled on `appapi.Identity`
and published by `--json`. `email` / `emailVerified` stay unmodelled **on
purpose**: they are PII the command does not print, so passing the body through
would be a privacy regression dressed as a bug fix. The rationale lives in a 🔴
block on the struct field an editor must touch, with two tests that go red.
Verified live: `tier:"silver"`, `status:"active"`, `isMember:true`,
`subscriptions:["yellow"]`, `has("email")==false`.

**Two leaks were found in review, neither in the original diff** — `WhoAmI`'s
parse-failure branch echoed the whole `/api/v1/me` body (email included) to
stderr, and `Subscriptions` was briefly a `json.RawMessage` that republished
server bytes verbatim. Both closed; see the `[]string` note under Gotchas.

### ✅ `release-v0.1.101-draft.md` described #498 as part of a release that shipped without it — FIXED in #500

The tag published `2026-08-26T03:56:29Z`; `79ed55d` merged
`2026-08-27T00:03:20Z`, **20 hours later**. The published body carries exactly
two changelog entries (#492, #494) and no #498 — measured, not inferred.

**Cause, and it is the reusable part:** an audit reported the release notes were
stale about #498, and the fix went into the v0.1.101 page without anyone asking
whether that release had already closed. It had. The page still said `DRAFT` /
"Not yet tagged" while being live on all three channels, which is what made it
look editable.

🔴 **Before writing into any release page, check whether it is closed:**

```bash
gh release view vX.Y.Z --json isDraft,publishedAt   # isDraft:false ⇒ CLOSED
git merge-base --is-ancestor <pr-sha> vX.Y.Z        # non-zero ⇒ NOT in that tag
```

The fix restored the page (`git checkout edc1299 -- <path>`; only two commits
ever touched it, so the revert is exact), headed it `SHIPPED` with a measured
table, and opened `release-v0.1.102-draft.md` carrying the #498 content where it
is true. That page's mechanics step 5 flips its own heading at tag time, so the
loop closes itself next time.

## Residuals — nothing here is queued work

🔴 **CLOSED. There is no ranked list any more, and its absence is deliberate.**
This doc's queue drained on 2026-08-27; every item either shipped or was decided.
A handoff is a SNAPSHOT, and this one went stale twice in two days while people
kept patching it — each patch made it read more authoritative as it decayed at
the same rate. The residuals below are recorded so nobody re-derives them, not
so somebody picks them up.

🔴 **Do not re-add a ranked list to this file.** `claim-work` derives its slug
from doc + RANK, so a new list silently re-points `whoami-capability-contract-N`
at whatever you wrote. That already happened once here: rank 1 meant "fix the
release-draft error", then "tag v0.1.102", under the same slug. New work gets a
NEW handoff doc.

### ✅ Decided: `AGENTS.md` gets NO item for the unmodelled `email`/`emailVerified`

Decided 2026-08-27 against adding one. The guard is **structural and stronger
than prose**: `appapi.Identity` has nine fields — `Username, ID, TokenScope,
BuzzLimit, Subject, Tier, Status, IsMember, Subscriptions` — and **no `Email`
field exists anywhere in `internal/appapi`**. Adding `"email": id.Email` to the
`--json` map does not leak PII; it fails to compile. Reaching a leak means first
adding the field to `Identity`, which is exactly where the 🔴 rationale block
sits, backed by two tests that go red.

A trigger line costs ~310 bytes of AGENTS.md's ~950-byte headroom **every
session**, to restate what the compiler already enforces at the one place an
editor must touch. An auditor argued for adding it; that argument loses to the
compiler.

### ◻︎ Closed by decision: the 2-in-8 `internal/cmd` failures were never attributed

**And they are not going to be.** The observation was 2 failures in 8 runs of
`go test ./internal/cmd`, output never captured. Later rounds ran 70 and then
100 full-package runs at HEAD with **0** failures, so whatever it was is not
reproducible at that rate now; the most likely explanation is that it was seen
on a mid-session tree that no longer exists — which no amount of re-running can
confirm or refute.

The 400-run capture probe this section used to prescribe was **rejected**: it
hunts one unreproducible instance and cannot distinguish "fixed" from "never
existed here". The **sweep** was done instead, because it finds the CLASS:

- `generate_charge_seam_test.go` — `Contains(errOut, "137")` was a silent green
  at a **measured 0.40%** (8 survivals / 2000 runs with the charge mutated to
  999). The carrier is the random external-ID **UUID**, whose hex digits spell
  `137` about 1 run in 250 — *not* the temp path that was predicted. Fixed by
  pinning `"Charged 137 Buzz"`; re-measured **0 / 2000**.
- `app_listing_status_json_test.go` — `Contains(text, "389")` was a spelled
  guard: replacing the citation with "a payload of 3891 bytes" left it GREEN
  with the citation gone. Now boundary-matched, with its own negative and
  positive controls.

Both in #505, alongside the three earlier instances (`whoami_test.go`'s `4242`,
`cmd_test.go`'s `Contains(out,"99")`, and the `workflow_settlement_regression`
false claim). **Assume more exist** — the sweep covered `internal/cmd` only.

### ◻︎ Passive: the additive `--json` keys in the wild

No code pending. If a consumer reports breakage it will be a strict-schema
decoder (`DisallowUnknownFields` / `additionalProperties:false`); the four new
keys are the only difference.

## Historical record — the original diagnosis, kept for its reasoning

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

## What shipped, in order

| PR | What |
|---|---|
| #492 | #377 option (a) — the false "raw JSON" claim in help + flag usage |
| #494 | the credential TYPE is not a capability — split the section in two |
| #498 | #377 option (b) — `--json` carries `tier`/`status`/`isMember`/`subscriptions`, still withholds the PII |
| #500 | restored the v0.1.101 page and opened v0.1.102's, after #498's notes were written into a release that had already shipped |
| #501 | this doc — two investigations had shipped but were still filed as OPEN |
| #502 | v0.1.102 closed as SHIPPED in the same session as its publish |
| #504 | `release_page_state_test.go` — a shipped release page can no longer keep saying DRAFT; found v0.1.99 stale for nine days |
| #505 | two assertions that passed on digits appearing rather than values rendered |

**`v0.1.102` published 2026-08-27T18:03:55Z** and verified from each consumer,
not from the workflow claiming to have updated it: GitHub Release `isDraft:false`
with 14/14 assets and one changelog entry; npm `dist-tags.latest = 0.1.102`;
Homebrew cask `version "0.1.102"` (tap `2b51e6f`) with all four of its own URLs
resolving 200 **unauthenticated** against a `v0.1.999` → 404 control; and, on the
downloaded binary, `has("tier")`/`has("isMember")` true with
`has("email")`/`has("emailVerified")` both **false**.

🔴 **THE THREAD'S ACTUAL DEFECT WAS NOT ABOUT `whoami`.** It was one shape,
four times: **a document asserting a state its artifact had moved past.** The
v0.1.101 release page, this handoff twice, and `release-v0.1.99-draft.md` sitting
`DRAFT` for nine days. Prose asking people to remember failed every time —
including an explicit numbered step written for exactly that purpose. #504 is the
only durable fix, and it covers release pages **only**; the same class remains
open for every other doc in this repo.

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
