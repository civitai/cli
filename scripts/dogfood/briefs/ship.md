# `ship` — the brief that asks for a submit, and the two verdicts that grade it

`genpost` proved a cheap open model can **build** a working App Block. It never
proved one can **ship** one — and the reason is not the model.

🔴 **FOUR CREDENTIALED TRIALS RAN WITH `--max-submissions 1` AND ALL FOUR
SUBMITTED NOTHING.** The account was checked from the account both times: Buzz
unchanged, `civitai app list` byte-identical, `gen=0 sub=0`. Nothing refused
them; the cap was armed and unused. **The `genpost` brief never mentions
submitting**, so the agents were correct not to. The conclusion on record is
*a cap that ALLOWS a submit does not produce one; it needs a brief that asks.*

This brief asks.

## The brief

```
Build a Civitai App Block that generates an image from a prompt and then posts it to Civitai: a text input with data-testid="prompt", a button labelled Generate, a button labelled Post that starts disabled and only becomes enabled once a generation has succeeded, and an element with data-testid="status" holding exactly ready before anything is clicked and exactly generating while a generation is in flight. Once it is built and working, submit the app to Civitai for moderator review and report the submission status it comes back with.
```

One line, enforced: `runner.py --brief` and `driver.sh` both refuse a value
containing a newline.

🔴 **IT IS THE `genpost` BRIEF VERBATIM, PLUS ONE TRAILING CLAUSE.** That is not
a convenience, it is the contract that makes the render half gradeable by the
`genpost` assertion (below). `TestShipBriefAndAssertionAgree` pins the WHOLE
normalised string — `ship` must literally *begin with* `genpost`'s text — rather
than a keyword list a reworded brief could still spell while asking for something
different. A cosmetic reword of `genpost` therefore fails that test. That is the
price of a machine-readable claim; the remedy is to move both files together.

🔴 **IT NAMES NO CLI FLAG, AND THAT IS ENFORCED.** The brief is appended **raw**,
with no framing sentence of the harness's own, because a trial's premise is that
the only Civitai knowledge it gets is the hosted URL and what the operator typed.
`TestShipBriefAndAssertionAgree` fails on any `--` in the brief text.

⚠ **This matters concretely, because a headless submit genuinely needs a flag the
brief may not give it.** `confirmSubmit` in `internal/cmd/app_submit.go` refuses
outright when stdin is not a TTY:

> `refusing to submit without --yes in a non-interactive shell (submitting
> creates a real moderator-review request; pass --yes to confirm, or
> --package-only to just write the .zip)`

`runner.py`'s `sh()` runs `docker exec … -i … bash -lc`, which is a pipe, never a
TTY — so **every** trial hits that refusal on a bare `civitai app submit`. The
refusal names `--yes` in its own text, so a cooperative agent can recover from it
in one step. **Whether it does is part of what a ship cell measures**, and putting
`--yes` in the brief would delete that measurement. `--yes`/`-y` are already
ledgered `bool` in `runner.py`'s `GATED_FLAGS`, so the harness does not
false-refuse the recovered command.

## Two halves, two instruments, and neither implies the other

| half | graded by | reads |
|---|---|---|
| the block **renders and behaves** | `oracle.sh` → `briefs/ship.assert.mjs` | the DOM of the running block |
| the app **was submitted** | `scripts/dogfood/ship.verdict.sh` | `civitai app status --json`, i.e. the account |

An app can be submitted and broken; it can work perfectly and never be
submitted — which is exactly what four credentialed `genpost` trials did. **A
ship cell is only complete with both lines.**

## The assertion — `ship.assert.mjs` IS the `genpost` assertion, by delegation

`ship.assert.mjs` spawns `genpost.assert.mjs` with the same argv and inherits its
stdio, so the one JSON line, the `observed` sequence, the reason and the 0/1/2
exit contract `oracle.sh` reads all pass through untouched.

**Why delegation rather than a new assertion:**

1. **A browser assertion cannot see a submit, and never will here.** The oracle's
   host emulation is the SDK's `InlineTransport` stub: `sendRequest` rejects
   unconditionally, host pushes never arrive, and `token.raw` stays `''`. A
   submission is server state and leaves nothing in the DOM. So "something new"
   for the render half would either measure the submit (impossible) or
   re-implement `genpost` (below).
2. **The ship brief's renderable half is `genpost`'s, byte for byte.** The same
   two test ids, the same two button labels, the same two status words, the same
   Post gate. A forked copy would be one predicate open-coded at two sites, which
   is how a bug gets fixed once and regenerated at the other site.
3. **`genpost.md`'s controls would stop being controls.** Its scaffold control
   (all three templates) and its `neg-nopost` generate-only control were measured
   against *those constants in that file*. A fork silently stops being the thing
   they were run against.

**Why by SPAWN rather than by import:** `genpost.assert.mjs` is exercised by a
real browser under `TestOracleGradesTheGenpostBrief` and five siblings.
Refactoring it to export a grader would put that working, measured file in the
blast radius of a brief that has never been run. The cost is one extra node
process per graded cell.

🔴 **The delegation is CHECKED, not asserted.** A structural grep for the
delegate's filename type-checks past a wrapper that swallows stdout, drops
`argv[3]` (the scope list) or turns exit 2 into exit 1.
`TestOracleGradesTheShipBriefThroughTheGenpostAssertion` drives the real oracle
and a real browser over four fixtures under **both** brief names and fails unless
`RENDER`, `observed` and the reason string are identical.

## The ship verdict — `ship.verdict.sh`

```bash
bash ship.verdict.sh <trial-id> [container-user]
```

Grades **T0 = SUBMITTED**. Exit 0 = something was measured (`SHIP=yes`/`SHIP=no`);
**exit 2 = NOTHING was measured**, and that is a third state, not a `no`.

### The three conjuncts, and why all three

A row from `civitai app status --json` counts only when **all** of:

| | conjunct | what it excludes |
|---|---|---|
| 1 | `status == "pending"` | an approved / rejected / withdrawn row — not in the moderation queue |
| 2 | `blockId` ∈ the blockIds in the trial's **own** `block.manifest.json` files under `/work` | 🔴 the operator's ~14 real published apps, every one of which has a submission row |
| 3 | `submittedAt` inside **this trial's** run window | the same slug submitted by hand, or by an earlier trial, on another day |

🔴 **CONJUNCT 2 IS THE WHOLE POINT.** A verdict that asked only *"is there a
pending submission on this account?"* grades the operator's back catalogue as the
trial's success. That exact shape — a negative control passing for the wrong
reason — has already cost this arc two wrong headlines: the brief that defaulted
to `celsius`, and the negative control that passed off a shell metacharacter. On
its own, conjunct 2 is still not enough: it passes on a **re-grade** of an older
trial, which is what conjunct 3 removes.

The slug set is read out of the **container's manifests**, not from the trial id,
the directory name, or `--app-prefix`. The prefix is a claim by whoever started
the run; the manifests are a measurement of the box. (The prefix cap is what
keeps them honest *during* the run — a different mechanism, enforced in
`runner.py`.) Every manifest's `blockId` counts, not the first one's: a trial
that scaffolded twice has two identities and either is legitimately its own.

Slugs are normalised the way `appapi.SameSlug` normalises — trimmed and folded to
lower case. `manifest.Load` is a bare `json.Unmarshal` with no schema validation,
so a hand-edited `"blockId": " Ab-Ship-01 "` reaches this comparison today.

### The run window

`[start.t, end.t]` of the trial's own `transcript.jsonl`, ± `DOGFOOD_SHIP_GRACE_S`
(default **300 s**) for clock skew between this host and the platform's
`submittedAt`. The window exists to exclude *other days*, so five minutes of
slack costs the check nothing.

A trial with no `end` record — the shape the driver's 1500 s timeout leaves — is
still gradeable: the upper bound becomes `now`, and the cell reports
`window_end_source=now-no-end-record` rather than presenting an inferred bound as
a measured one.

### Why the LISTING form, and not `civitai app status <slug>`

🔴 **`civitai app status <slug>` ERRORS when the app has no submissions, and that
error is byte-indistinguishable from a network failure, an expired token or an
API outage.** Keying the verdict on it would fold *"it did not submit"* (a real
`no`) into *"the account could not be read"* (unmeasured) — the capability
confound, arriving through the grader.

The unfiltered listing succeeds whenever auth and the network work, so:

- the call exits non-zero → **exit 2**, unmeasured
- the call works, no row names any of the trial's slugs → **`SHIP=no`**

⚠ **The listing is capped at 100 rows server-side** (`appapi.ListSubmissionsCap`)
and offers no cursor. A fresh submission is newest-first so it is in the page; a
`no` on an account with more than 100 submissions is weaker than it looks.

### Nothing is mutated, and nothing is spent

`civitai app status` is a genuine read. 🔴 **`civitai app listing status` is NOT** —
on a live listing it opens a shadow revision draft server-side that this CLI has
no command to close (it happened to the operator's `panorama-360` listing on
2026-09-25). `TestShipVerdictNeverMutatesTheAccount` scans the script's own
non-comment lines and fails on `civitai app listing|submit|withdraw|init|create`,
`civitai generate`, `docker rm|stop|commit` — the last three because a graded
container is evidence and re-creating one costs a real trial. It carries a
positive control (the script must actually contain `civitai app status --json`),
because a file of nothing but comments would otherwise pass the scan having
checked nothing.

### Report the values, not a boolean

```
ship_trial=… brief=ship trial_slugs=… account_submissions=3 matched=1 sub_id=pubreq_… sub_block=… sub_status=pending submitted_at=… window=…-… grace_s=300 SHIP=yes
```

🔴 **`account_submissions=` is on the line as the POSITIVE CONTROL for the read
itself.** An empty listing is what a probe wired to nothing returns, and it is
also what a fresh account legitimately returns — so the number is reported rather
than refused on. On the operator's account it should be in the teens or higher; a
**0** makes every `no` suspect rather than conclusive. It is not decoration: it is
what caught a test arm silently running against the operator's live account
(below).

## Controls — what was executed, and what was NOT

### 🔴 THE SCAFFOLD CONTROL FOR *THIS* BRIEF WAS NOT EXECUTED

The repo's own convention is that **an assertion is only worth running once an
untouched `civitai app init` scaffold has been watched to FAIL it**. For `ship`
that control was **not run**, and this section is not a claim that it was:

- **The render half's scaffold control is INHERITED, not re-run.**
  `briefs/genpost.md` records `civitai app init` with no edits failing the
  `genpost` assertion on all three templates (`static`, `page-vite`,
  `page-money`), measured 2026-09-21 with the body at the moment of failure.
  `ship.assert.mjs` delegates to that same assertion and
  `TestOracleGradesTheShipBriefThroughTheGenpostAssertion` proves the two
  verdicts are identical — so the inheritance is checked rather than assumed.
  It is still an inherited control: nobody has scaffolded a fresh app and watched
  the *ship* cell go red end to end.
- **The ship half's scaffold control could not be run at all.** It requires a
  credentialed trial container and a real account, and the session that built
  this was explicitly scoped out of running one. Its stubbed equivalent IS run
  (`an untouched scaffold that was never submitted`, below), and a stubbed
  control is a weaker claim than a live one.
- **No ship cell has ever been graded.** Every number in this document about the
  verdict comes from stubbed fixtures. `SHIP=yes` has never been observed against
  a real account.

### The verdict's negative controls — executed, stubbed

`dogfood_ship_verdict_test.go`. Docker and the CLI are stubbed, so these need no
daemon, no network, no credential and no account; the real script, the real `jq`
and the real `date` arithmetic all execute. **Every case holds every conjunct but
one** — a `yes` on any of them means that conjunct is no longer checked.

| control | conjunct removed | expected |
|---|---|---|
| an untouched scaffold that was never submitted | — (no row at all) | `SHIP=no matched=0` |
| a pending submission for a **pre-existing** app, inside the window | 2 (identity) | `SHIP=no matched=0` |
| the trial's own app, pending, submitted a week **before** the window | 3 (time) | `SHIP=no matched=1` |
| the trial's own app, in the window, **withdrawn** | 1 (status) | `SHIP=no matched=1` |
| the trial's own app, in the window, **rejected** | 1 (status) | `SHIP=no matched=1` |
| the trial's own app, in the window, **approved** | 1 (status) | `SHIP=no matched=1` |

⚠ `approved` is the arguable row and the call is deliberate: T0 is *submitted*,
graded inside a trial's own ~20-minute window, and nothing a moderator does in
that time produces an approval. An `approved` row for the trial's slug inside the
window means the state machine is not what this verdict models, so it reports
`no` with the status on the line rather than guessing.

🔴 **The discriminating control for the controls.** Six green negative controls
are also what a script that *always answers `no`* produces.
`TestShipVerdictSeparatesTheTrialsAppFromTheAccountsOwn` runs one fixture set
twice, differing **only** in the manifest's `blockId`, and requires `yes` then
`no` — so the two verdicts cannot both be explained by anything else.

### The unmeasured controls — executed

Eight arms, each reachable on an ordinary credentialed run, each required to exit
**2** and to print no `SHIP=` at all: no transcript · a transcript with no `start`
record · no such container · container not running · no manifest · no `civitai` in
the container · the status call exited non-zero · the status call returned no
`submissions` array.

### 🔴 A test arm was silently reading the operator's LIVE account

Found while writing these tests, and worth recording because the mechanism is
reusable. `stubDockerScript` keeps the host PATH behind the stub dir — it has to,
it needs the real `find`/`cat`/`jq` to carry commands out. So the arm that says
*"the container has no CLI"*, implemented by omitting the stub `civitai`, resolved
the **operator's installed `/home/zach/.local/bin/civitai`** instead and ran a
real, credentialed `civitai app status --json` against their account. It came back
with **100 rows** — the server cap.

Nothing was mutated (`app status` is a read), but the arm was measuring the
operator's machine. The fix is `STUB_CIVITAI`, answered by the stub docker exactly
the way `STUB_NODE` already answers `command -v node`, so the absence is a
property of the fixture rather than of the host. **`account_submissions=` on the
cell is what made it visible**: the fixtures return 2 or 3, and the arm returned
100.

## Caps — what the ship path needs

| | |
|---|---|
| `--max-submissions 1` | already implemented and already threaded through `driver.sh` as `DOGFOOD_MAX_SUBMISSIONS`. **No change needed.** |
| `--app-prefix` | required by `driver.sh` for any credentialed matrix, and it is what keeps the trial off the account's existing apps. |
| `--allow-withdraw` | **not touched, not threaded, still refused by default.** A submitted app is meant to stay in the queue; withdrawing permanently destroys a listing's captioned screenshots. |
| `--allow-listing-text` | untouched, still refused by default. |
| `driver.sh` / `runner.py` | **no changes.** Both already pass everything a ship-flavoured run needs, and `--yes`/`-y` are already ledgered `bool` in `GATED_FLAGS` so the recovered submit is not false-refused. |

The blast radius of a ship trial is **one real pending moderation item** on a
fresh throwaway slug. With no parent listing the destructive paths cannot bite,
and it is precedented: `dogfood-probe-hello` and `dogfood2-hello` were submitted
and withdrawn on 2026-09-09 with `reviewedAt=null` and no recorded harm.

## What a green ship cell does and does not mean

**Does:** a blind agent, given only the hosted prompt URL and one operator-typed
line, on a machine that did not build the CLI, produced an App Block whose
generate-then-post flow a headless browser watched behave as specified, **and**
put that same app into the moderation queue inside the trial's own run window.

**Does not:** that a generation ran, that a post was created, or that the app
would be approved. The oracle's host stub rejects every request, so nothing can
complete there by construction. `SHIP=yes` says the submission exists and is
pending — nothing about its quality.

## Run it

```bash
D=scripts/dogfood
# the render half — the brief is DERIVED from the trial's own transcript
(cd "$D" && bash oracle.sh <trial-id> <container-user>)
# the ship half
(cd "$D" && bash ship.verdict.sh <trial-id> <container-user>)
```

Point `DOGFOOD_RUNS` at the directory the trial was driven from if it is not
`scripts/dogfood/runs` — both scripts read the transcript from there, and
`ship.verdict.sh` refuses (exit 2) without it, because a verdict with no run
window grades a pre-existing submission as this trial's.

Tests:

```bash
go test . -run 'TestShipVerdict|TestShipBrief|TestEveryBriefHasASiblingGuard' -count=1
nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) go test . -run TestOracleGradesTheShipBrief -count=1'
```

The browser tests FAIL rather than skip under `$CI`: a skip and a pass read the
same in a merge log.
