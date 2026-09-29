# Handoff: app-build-dogfood — 2026-09-20

## Run this first — the index, one command
```bash
cairn recall --repo /home/zach/workspace/civit/cli
```
Terse pointers this doc does not carry, curated by past sessions and outliving it.
🔴 RECALL, NOT LIVE OBSERVATION — every line is a pointer to VERIFY, never a current
reading, and it may describe a gotcha already fixed. `scope-absent`/`scope-empty` means
nothing is recorded yet: ordinary, not an error, and not a clean bill of health.
Non-blocking: if it exits non-zero, print the stderr line and carry on.

## Goal

Validate that the onboarding flow is good enough for **cheap open LLMs to BUILD APPS** —
not merely to install the CLI. This is the operator's original objective for the
`agent-setup-onboarding` arc, which that arc could not answer because its frozen condition
was about *setup reachability*.

- **closing-condition:** `check` — a BLIND agent, given only the hosted prompt URL and a
  one-line app brief, on a machine that did not build the CLI, produces an App Block that
  **renders and exhibits the specified behaviour**, observed by a headless browser driving
  the running block — **not** by the CLI's own validator. `civitai app validate` exiting 0
  is a fail-fast GATE, never the verdict. Graded per cell across the three cheap models,
  with a frontier control.
  This is frozen as the condition this arc was opened on.

🔴 **AMENDED 2026-09-26 — THE FRONTIER CONTROL IS DROPPED, BY A REAL OPERATOR DECISION, AND
THE ARC CLOSES WITHOUT IT.** The condition above is frozen and still reads *"with a frontier
control"*; that clause is now deliberately **unmet**. Asked directly, before any spend, the
operator chose it in these words:

> no frontier model run. if the cheap models can do it, then the frontier models will be able
> to, we dont need to waste money proving something we already konw

🔴 **This is not a capability or budget limit, and the distinction is the point** — measured
the same day, `anthropic/claude-sonnet-5` ($2.00/M prompt, $10.00/M completion) and
`openai/gpt-5.6-terra` ($2.00/M, $12.00/M) are both live on OpenRouter, and the harness key
holds **$48.97 of its $50 weekly limit**. A cell was configured, priced and ready; it was not
run because the operator judged the answer already known.

**What the drop costs, stated rather than waved through.** The control's original job was to
separate *"a cheap model cannot drive this task"* from *"the product is broken"* — and the
default arm has already done that job: 2 of 3 cheap models passed, which refutes
product-broken without a frontier cell. For that purpose the operator's inference holds. Two
things stay **unmeasured**, and neither is closed by it: (a) **"frontier ⊇ cheap" is an
assumption in this rig, never a measurement** — the one per-vendor prediction this arc made
from two data points was refuted on both halves, and the measured discriminator was **the
APP, not the model**; and (b) **no frontier app has ever been graded on the unconsented
arm** — the arm that caught the live defect, which 4 of 7 fixtures fail. Neither is a to-do:
the operator has decided the question is not worth the money. But a later reader must not
read the drop as evidence that a frontier cell *would* have passed.

🔴 **WHY THE VERDICT MUST NOT BE `app validate`.** An untouched `civitai app init` scaffold
may already validate clean, so a validate-only grade cannot distinguish **scaffolded** from
**built** — it would return a confident `yes` to a question it never asked. The brief must
name a behaviour the scaffold does not have, and the browser must observe *that*. Operator
decision, 2026-09-20, chosen over a build-only oracle for exactly this reason.

## State now

✅ **EVERYTHING FROM THIS ARC IS MERGED. Nothing is in flight.** Eight PRs landed, each verified by
content on `origin/main` (never by ancestry — a squash merge makes that permanently false):

| PR | commit | what |
|---|---|---|
| `cli#752` | `0f0777c4` | **rank 27** — the moved `contentRating` self-block, both sites. Claim released. |
| `cli#753` | `e602b3a` | README paste block → `civitai.com/agent-onboarding`, + a guard on the cross-repo mirror |
| `docs#127` | `6339220` | `SETUP_PROMPT` derives from `SHORT_PROMPT_URL`; `PROMPT_URL`/`PROMPT_PATH` stay canonical |
| `cli#754` | `ac840b6` | scaffold pin bump — **the repo's own automation had opened it**; I merged rather than duplicate |
| `cli#755` | `0bb7793` | the auth self-block five audit rounds walked past |
| `cli#756` | `94cad83` | `schema-drift` off `pull_request` — **observed firing and passing on the merge to `main`** |
| `cli#751` | `4e3570f4` | this doc consolidated onto `main`; the two-version split is over |

🔴 **THE ARC'S FROZEN CONDITION REMAINS CLOSED (2026-09-21) AND NO CODE ITEM REMAINS.** The only
live rank is **14**, which is blocked on the operator's API key and nothing else.

⚠ **No `clawgate-task:`** — `resolve` exited **5**, `NOTHING RESOLVED`; that zero cannot distinguish
"touched nothing" from "wrong id".

## Open investigations — live diagnosis state

🔴 **11 CLOSED blocks were EVICTED, not deleted — they are verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md).** The size ratchet refuses a doc that is over budget and growing; eviction is how a round lands without losing the eliminations a future session would otherwise re-derive. **Read the archive before re-running any probe this arc already ran** — every one of these is a resolved elimination, not current state:

- ✅ EXPLAINED 2026-09-21 — glm never wrote code, and "finished" was a truncation
- ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- ✅ FIXED 2026-09-25 — the page-money scaffold shipped a build that could not pass
- ✅ FIXED 2026-09-25 — all three harness refusals in one trial were wrong
- ✅ SHIPPED 2026-09-25 — the submission cap charged attempts, so a CLI-refused submit spent the budget
- ✅ SHIPPED 2026-09-25 — the oracle answered no host resource pick; a picker-gated app could never pass
- ✅ FIXED 2026-09-25 — the oracle graded only the consented viewer; a live app broken for every new user shipped green
- ✅ MEASURED 2026-09-26 — rank 15's premise was backwards, and so was my first account of why
- ✅ MEASURED 2026-09-27 — what `v0.1.2` actually changed, and the control that refuted my first answer
- ✅ RESOLVED 2026-09-28 — a BROKEN conflict resolution cached in `cli`'s rerere and replayed silently
- ✅ FIXED 2026-09-28 — `schema-drift` red repo-wide because the guard over the mirror was SELF-BLOCKING (fixed by `#745`; the current reading is the ✅ RESOLVED block below)
- ✅ RESOLVED 2026-09-28 — the schema-drift CLOSURE record: all three closing-condition halves, the rehearsal of the failing path, and why the "opens a PR unaided" half is unreachable BY DESIGN (an in-sync mirror correctly skips both validate and PR-creation). Its mechanism lesson is live in *Gotchas* → **a guard over a mirror could not accept the mirror's own update**.

### Evicted 2026-09-29 (rank 27 close) — two closed rank-27 blocks and the docs-cost block

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-29 (rank 27 close)*.** Rank 27 is merged as `cli#752`, so both of its
blocks are spent; the docs-cost block is 2026-09-21 measurement from a phase that has closed,
and its own next probe was superseded by the stale-CLI finding. **Read them there before
re-running any probe from those phases** — each is a resolved elimination. ⚠ One of the
rank-27 blocks contains a claim that was later REFUTED (its append-only repro cannot red the
`internal/cmd` site); the correction is in the ✅ RESOLVED 2026-09-29 block above.

### Evicted 2026-09-29 (rank 27 close) — the `cli#685` pricing block

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-29 (rank 27 close)*.** Its partner, the docs-stack carry-cost block, was
evicted in the same round; both belong to the phase that closed with the frozen condition on
2026-09-21. Its reusable half: **a trial installs the PUBLISHED CLI (`npm install -g`), so a
merged-but-unreleased fix is invisible to every cell** — and that is what made the three cheap
cells differ on the model axis alone, which was favourable. Read it there before re-running any
probe that assumes a merged fix is live.

### Evicted 2026-09-29 (rank 14 launch) — the rank-14 credential-shape block

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-29 (rank 14 launch)*.** Its blocking condition is DISCHARGED: the operator
delivered the key and cell `at3` launched, so its *Next probe — needs the operator* instruction is
spent. **Its eliminations are NOT** — it is what rules out a missing credential, a destroyed
credential and a broken install path, each by measurement. Read it there before re-running any
credential probe. The live reading is the IN FLIGHT block below.


### 🔴 OPEN — spend can no longer be graded by balance delta, because the operator uses the same account
- as-of: 2026-09-28

- **Observed (with values):** during cell `at2`'s window (21:52–22:15) Buzz moved
  **4,074,007 → 4,073,892 (−115)** in two steps at ~22:11 and ~22:12, and a new submission
  `yt-thumbnail` v0.1.0 appeared at `22:01:19Z`. **None of it was the trial's.** `via: measurement`
- **Ruled out — that the trial spent it**, on three independent lines: the trial invoked
  `civitai generate` **zero** times; **zero** image files exist anywhere under `/work` in its
  container; and at 22:11–22:12 it was provably inside step 71's five-minute sleep loop. It also
  created no `ab-t1-*` app on the account at all. `via: measurement`
- 🔴 **Consequence:** the `dogfood-3`-era method of grading spend as a balance delta is **void**
  while the operator works the same account. **Attribute by blockId** — which
  `ship.verdict.sh` already does, reading the trial's own manifests out of the container.
  Reporting that delta as trial spend would have been a false claim about the operator's money.

### 🔴 OPEN — the CLI's sensitive-scope mirror is not derived from anything, and it was already wrong
- as-of: 2026-09-28

- **Symptom:** `civitai app validate` accepted a manifest declaring `goods:purchase:self` with **no**
  `scopeJustifications` entry; the server rejects it at submit, so the author ate a 400 after a full
  package+submit round trip. Fixed in `#745`; the *class* is what stays open.
- **Observed (with values):** `SENSITIVE_BLOCK_SCOPES` in `internal/validate/semantic.go` held **6**
  entries; the authoritative constant `SENSITIVE_BLOCK_SCOPES` in
  `<civitai>/src/shared/constants/block-scope.constants.ts` holds **7** on **`origin/release`** —
  `ai:write:budgeted`, `social:tip:self`, `buzz:read:self`, `collections:read:private`,
  `apps:storage:shared:write`, `posts:write:self`, `goods:purchase:self`. `via: measurement`
- 🔴 **Read `origin/release`, NOT `origin/main`** — the site serves `release`, and this doc's own
  `revendor` notes record that a canonical change can sit on `main` unpublished for days. Both
  branches happened to agree here; that is luck, not method.
- 🔴 **Ruled out — that `goods:read:self` is also sensitive.** It is deliberately absent upstream: the
  server scopes that reply to `claims.appBlockId`, so an app sees only what it itself sold to that
  viewer. Adding it would demand a justification the server never asks for. A test row in `#745` now
  fails if anyone adds it. `via: code`
- 🔴 **Why nothing caught it, and this is the durable half:** every guard in `internal/validate` keys
  on the **vendored schema**, and this set mirrors a **separate server constant** that is in no schema
  — so `schema-drift` going red over `goods:` said nothing about it, and the re-vendor automation
  would have landed the schema with the mirror still wrong. **A hand-kept mirror with no drift-guard
  is invisible to a drift-guard suite.**
- **Next probe:** decide whether this set can be machine-checked at all. It is cross-repo, so the
  options are a generated fixture or a scheduled check that reads the deployed constant — both
  bigger than `#745`. Until then, the rule is in `semantic_test.go`'s docstring: when the schema
  gains a scope, re-read the deployed constant by hand.

### Evicted 2026-09-29 (close) — the round-5 record and the two follow-ups

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-29 (close)*.** Both are closed: the follow-ups merged as `#755`/`#756`, and
the round-5 record's DISTILLED lessons are kept live in *Gotchas* (ask whether the work should
exist before auditing whether it is correct; measure a guard's own failure cost first; a spelling
guard on a provenance property is walkable). **Read the archive before re-adding any guard over the
vendored schema** — it carries why `schema-drift` is not a required check, that `contentRating` has
never changed, and which run logs the cited incidents actually name.

### Evicted 2026-09-29 (rank 14 launch) — the round-5 follow-up closure record

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-29 (rank 14 launch)*.** `#755`/`#756` are merged and the workflow move was
verified LIVE; its one live instruction (the next PR should show 12 checks, not 13) is carried in
*Gotchas*. Read it there before re-verifying either follow-up.


## Next steps (ranked)

🔴 **Numbering frozen and cumulative.** 1–13, 15–22, 25–29 closed; **14** live — and it is the only
one.

1–13, 15–22, 25–29. ✅ **CLOSED** — 27 as `#752`, 29 as `#755`/`#756`/`#751`
    forcing: gate — satisfied
14. 🔴 **BLOCKED ON THE OPERATOR — the key, then it runs.** T1 "ship-ready"; every other
    precondition verified 2026-09-28. The command, its three ⚠ caveats and the verified
    preconditions are in the rank-14 open block above — do not restate them. Mint a full-scope
    personal API key at `https://civitai.com/user/account` → API Keys.
    forcing: user — asked 2026-09-25, decided 2026-09-27

## Gotchas / decisions / dead-ends

- 🔴 **A VALIDATOR SHIPPED BY THE TOOL UNDER TEST CANNOT BE THAT TOOL'S GRADE.**
  `civitai app validate` is the obvious oracle and is the wrong one twice over: a bare
  scaffold may pass it, and a defect in the validator grades itself green. It is kept as a
  fail-fast gate because it is cheap and offline — never as the verdict.
- 🔴 **THE CAPABILITY CONFOUND IS THE HOUSE HAZARD OF THIS WHOLE FAMILY.** A model that
  cannot drive the task, a harness that runs out of steps, and a broken product all emit
  the same `CLOSING_CONDITION=no`. The prerequisite arc beat it by scanning transcripts for
  `EACCES` counts and remedy attempts — i.e. by finding a signal the three explanations
  DISAGREE about. 🔴 **For two of the three, the rig ALREADY emits that signal and an
  earlier draft framed it as open design work:** `runner.py` writes
  `stop` ∈ {`finished`, `max-steps`, `max-cost (...)`} into the `end` record of
  `transcript.jsonl` (`:176`, `:181`, `:198`, `:219`), which separates "harness ran out"
  from the rest. **Read that field; do not re-invent it.** What is genuinely still open is
  separating "model gave up" from "product broken" *within* `stop=finished` — that is what
  the prerequisite arc's transcript scan did, and it is the part to design.
- ⚠ **`--max-steps` is not a spend cap and the module says so.** `--max-cost` is the only
  bound on money. Set both deliberately on every run.
- ⚠ **The three cheap models are ~5–20× cheaper than the frontier control**, so the
  frontier arm dominates the bill. Run it as a control, not as a cell in every sweep.
  🔴 **That "5–20×" is from the SETUP grid and UNDERSTATES the app-build gap on completion —
  measured list prices 2026-09-26:** sonnet-5 $2.00/M prompt + $10.00/M completion against
  mimo $0.14/$0.28 (**14× / 36×**) and deepseek-v4-pro $0.35/$0.70 (**5.7× / 14.3×**). Pricing
  moves, so re-read `/api/v1/models` rather than quoting these. Projected from the comparands'
  own token counts (0.95M–2.7M prompt, 12k–18k completion), one frontier genpost cell is
  **~$0.50–$2.50** — which is the number the 2026-09-26 drop decided against paying.
- ⚠ **This repo lands handoff docs via PR** — every `docs(handoff):` commit in
  `claudedocs/` carries a `(#N)`. Do not push one straight to `main`.
- ⚠ **Three older dogfood arcs exist and are NOT this one** —
  `claudedocs/handoff-dogfood-154.md` (2026-08-07), `handoff-dogfood-2.md` (2026-08-09),
  `handoff-dogfood-3.md` (2026-08-10). All are CLI-usability rounds; none grades app
  building and none has a render oracle. Checked before minting this slug, so a future
  session does not re-litigate whether this was a duplicate.

### Evicted 2026-09-27 (resume) — four closed 2026-09-21 blocks

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md) under *Evicted
2026-09-27 (resume)*.** The size ratchet refuses a doc that is over budget and growing, and this
round needed 7,116 B; these four are 2026-09-21 detail from a phase that has closed, and their
conclusions survive in **Added 2026-09-21 (close) — what this arc actually taught**, which is kept.
**Read them there before re-running any probe from that phase** — each is a resolved elimination:

- rank 3's measurement, and three inferences that did not survive it (the $0.06–0.18 estimate that
  came in at $0.0145; a real mechanism whose consequence was still wrong; the pin-bump PR that
  merged CLEAN because no checks ran)
- the credentialed turn (capture the account baseline BEFORE the run; `--agent-env` records its
  value in the transcript; a leak test needs a positive control)
- the pin treadmill, and a control that was not one (upstream publishes faster than the sweep; pick
  a discriminating string the NEW content introduces)
- the merge round, and a verdict that was about the instrument (the third instrument defect; a
  probe's zero needs the arm that makes it non-zero; a merged fix is not a shipped fix)

### Evicted 2026-09-28 — five blocks whose lessons this session's entries carry forward

🔴 **MOVED, NOT DELETED — verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md)
under *Evicted 2026-09-28*.** The size ratchet refuses a doc that is over budget and growing; this
round needed 14,853 B. Each of these is either superseded by an entry added below it or already
generalised elsewhere — **read the archive before re-running any probe from those phases:**

- *the instrument was wrong more often than the models were* (2026-09-21) — summarised by
  **Added 2026-09-21 (close)**, which is kept
- *the instrument, the brief, and three of my own claims* + *(ship) what shipping taught*
  (2026-09-25) — the cost-instrument death and the brief-can-ship-a-bypass rule are restated in
  the 2026-09-28 blocks
- *what merging a doc can break* (2026-09-27) — the test-merge-before-landing rule
- *four cwd-resolution hits, and a tool's verdict read as a fact* (2026-09-27) — **superseded by
  *Added 2026-09-28 — process traps hit while investigating traps of the same class***, which
  carries the same traps plus rerere and the scratchpad collisions. The `audit-dispatch --repo`
  lesson from it also lives in the `devrc` store's `scripts` entry.

### Evicted 2026-09-28 (schema-drift round) — the 2026-09-21 "what this arc actually taught" block

Verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md). Five
lessons, every one still live and each restated at its own site in the sections below: the instrument
was wrong more often than the models; a grader can penalise the MORE correct implementation; a
probe's zero needs the arm that makes it non-zero; run the control arm BEFORE the arm you want to
believe; a merged fix is not a shipped fix when the harness installs from npm.

### Evicted 2026-09-28 (audit batch) — two blocks: *the instrument was too LENIENT* (2026-09-25) and *a handoff doc is not evidence about its own arc* (2026-09-26)

Both verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md).

### Added 2026-09-27 (resume) — durable facts RELOCATED here out of `State now`

🔴 **These were living under a REPLACE heading, where every future update deletes them.** The write
gate flagged 18 such lines; these are the ones worth keeping, moved to an APPEND section so the next
`State now` rewrite cannot eat them. **Nothing here is new this session** — it is 2026-09-27
(morning) evidence, re-homed.

- **The six PRs of 2026-09-27, verified on `origin/main` BY CONTENT** (a squash merge never makes
  the branch an ancestor, so ancestry is never the check): `#724` `7c57d3a` frontier drop · `#726`
  `e9c9d78` schema re-vendor · `#717` `721edae` v0.1.1-live · `#728` `8c4c631` oracle
  verdict-forgery · `#718` `3c450ea` consent control + archive eviction · `#729` `9e9142e` ship
  verdict-forgery. Ranks closed: 13, 15, 16, 17, 18, 19.
- **`v0.1.2` is live** — reviewed `2026-09-27T03:13:52.743Z`, deployed `03:15:10.039Z`, answering a
  second real-user report (*"Posting failed. Please try again."*). ⚠ That report is
  operator-relayed; what is MEASURED is the version, the dates and the controls.
- **Both defects confirmed in the SERVED bytes** (`/assets/index-CcmcNwgc.js`, HTTP 200): Post was
  enabled for a workflow that produced NO images — `watch()` resolves on any of
  `succeeded|failed|canceled|expired` and only submit-time status was checked — now gated on
  `status==="succeeded"&&((Q=Bt.imageUrls)==null?void 0:Q.length)>0`; and every host refusal was
  flattened to one string — now all five closed-set codes plus
  `typeof(R?.code)=="string"&&th(R.code)?Dt(PS[R.code]||R.message||…)`.
  ⚠ **The minifier DOWNLEVELS optional chaining**, so grepping the source spelling
  `imageUrls?.length>0` returns **0** on the bundle and is NOT evidence of absence — it cost a
  near-miss report. Pair with: `grep -c` on a minified bundle counts LINES, and a minified bundle
  is one line.
- **The bundle md5s, which are the only attributable controls:**
  `dogfood-ab-imgposter-fixed` = `v0.1.2` = **`c603fa854eba4d5206b73e04c14a1f3c`**, equal to live;
  the pre-fix image `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` =
  **`b8a1936e5939e39aa3e26f6e9e777113`**. Without that image `v0.1.2` has no control at all. ⚠ Any
  doc or probe reading the container as "the v0.1.1 fix" is one version behind.
- 🔴 **`v0.1.2` IS AN ERROR-PATH CHANGE AND THE ORACLE CANNOT GRADE ANY OF IT.** `"Please try
  again."` sites go 3 → 6; `InlineTransport` rejects every request, so the error branch is the only
  one any arm reaches and a fix making failures legible is invisible to `RENDER=`. **Auto-resume
  stays unverifiable by construction** (needs `TOKEN_REFRESH`, undeliverable by
  `InlineTransport.onMessage`) — a human clicking Generate is the only check. ⚠ `posts:write:self`
  4-vs-1 separates `v0.1.1`+`v0.1.2` from `v0.1.0` and says **nothing** about `v0.1.2`.
  **This is precisely what rank 22 exists to fix.**
- **The two forgery fixes are ONE RULE, not two.** `#728`: an app being graded could write its own
  grade (manifest values printed with a raw `%s`; both consumers take the FIRST `KEY=` match —
  measured `RENDER=yes` on an app that built nothing). `#729` found the same hole in
  `ship.verdict.sh` and **extracted the rule to `scripts/dogfood/_esc.sh`, sourced by both
  graders**, rather than copying it. 🔴 **MEASURED REASSURANCE: 0 of 8 recorded fixtures was
  forgeable**, so no recorded verdict in this arc is void.
- ⚠ **`#718` WAS MERGED WITH ROUND 7 OF ITS AUDIT LADDER NOT RUN** — accepted unaudited by a dated
  operator decision, 2026-09-27. No round ever returned clean: round 6 found the forgeable
  `outputDir`, `#728` fixed it upstream, and a fix RESETS the gate. **Do not read "merged" as
  "audited".** The residual is a subtly wrong fixture or an evicted block whose meaning changed in
  transit — the two things CI cannot see. Round 7 is a post-merge option, not a gate.
- ⚠ **`#718`'s DISCARDED retraction is still reachable** in that PR's own commits and in
  `refs/remotes/pr/718`. It concluded the frozen condition was *unmet and still outstanding*, which
  the 2026-09-26 operator drop supersedes. **Do not re-import it, and do not read its survival
  there as a second open question.**

### Evicted 2026-09-28 (schema-drift round) — the 2026-09-27 stale-CLI / reconciliation block

Verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md). Its
reusable tells, so you know whether you need it: a LOCAL `civitai` binary can be older than npm
`latest` and silently answer a version question wrongly; a "reconciliation" that reads only one side
is not one; and a recommendation of mine was refuted by measuring it.

### Added 2026-09-27 (ladder) — the CARRIED-FORWARD durable values, RELOCATED here for good

🔴 **These lived under `## State now`, a REPLACE heading, and were flagged as dropped on TWO
consecutive updates. They are now in an APPEND section so no future `State now` rewrite can eat
them.** Nothing here is new; it is the arc's accumulated ground truth, re-homed. The previous
`### Carried forward` subsection is retired — do not re-create it under `State now`.

- 🔴 **THE FROZEN CONDITION WAS CLOSED 2026-09-21 AND STAYS CLOSED**: 2 of 3 cheap models built a
  passing App Block (`xiaomi/mimo-v2.5` blind for **$0.0170**; `glm` failed, explained). Two
  permanent limits, **neither a to-do**: the **frontier control is DROPPED as of 2026-09-26** by a
  dated operator decision (their own words are under *Goal*), and **every cell is ONE environment**
  (`cli#665`). ⚠ Read "2 of 3 passed" as the **DEFAULT arm only** — **4 of 7** fixtures fail the
  unconsented arm.
- 🔴 **AN AGENT-AUTHORED "THE OPERATOR DECIDED X" SURVIVED FIVE DAYS** — the sentence closing the
  frozen condition's last open clause was **fabricated**, and `4d4a45e` (#691) deleted an accurate
  *"still NOT met"* line in the same hunk. Full account under *Added 2026-09-26*. **This is why
  rank 14's decision is recorded as a SELECTION from a four-option question, not as a quote.**
- 🔴 **RANK 3'S TABLE — the arc's only record of the step/cost curve:** mimo-v2.5, celsius,
  **65 steps / $0.0145 / 1,564,496 prompt tokens / per-call 452 → 45,328 / `stop=finished`**,
  caps 80/$0.50.
- 🔴 **COST IS DEAD AS A CROSS-DAY INSTRUMENT** — **2.15×** drift in $/1k on the same model and task
  in four days, cached share nearly unchanged. Grade on steps, prompt tokens and behavioural
  signals.
- 🔴 **BASELINES DRIFT — RE-READ, NEVER QUOTE.** Buzz **4,101,822** (09-21) → **4,074,196** (09-25)
  → **4,074,007** (09-27, = Blue **1,305,224** + Green **919,796** + Yellow **1,848,987**, which
  reconciles exactly — the older "does not reconcile" note was a missing column). The submissions
  listing is **at its 100-row cap, no cursor**.
- 🔴 **ELEVEN FIXTURES + TWO SNAPSHOTS — DO NOT DESTROY.** Derive, never quote:
  `docker ps -a --format '{{.Names}}' | grep -c '^dogfood-'` (11 on 2026-09-27: 8 `ab-*` + 3
  `ctl-*`); images `dogfood-fixture/ab-ship-mimo-02:pre-consent-fix` and
  `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix`. `dogfood-ab-genpost-glm-01` is the negative
  control; `dogfood-ab-imgposter-fixed` holds **`v0.1.2`** (bundle md5
  `c603fa854eba4d5206b73e04c14a1f3c`, byte-equal to live) with a synthetic trial dir at
  `/tmp/fixed-runs/ab-imgposter-fixed`; the only `v0.1.1` control is that image (md5
  `b8a1936e5939e39aa3e26f6e9e777113`). Evidence lives OUTSIDE every worktree:
  `~/.cache/dogfood-runs-2026-09-21/`, `~/.cache/dogfood-runs-2026-09-25/`,
  `/tmp/wt-verify686-1071809/scripts/dogfood/runs`.
- 🔴 **`v0.1.2` IS AN ERROR-PATH CHANGE THE ORACLE COULD NOT GRADE** — `InlineTransport` rejects
  every request, so the error branch was the only one any arm reached. **Auto-resume stays
  unverifiable by construction** (needs a `TOKEN_REFRESH` push, which `InlineTransport.onMessage`
  cannot deliver) — a human clicking Generate is the only check. `#734` closes the *post* half of
  this blindness; auto-resume remains open and is **not** a to-do.
- **The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — withdrawing a **first-version**
  submission destroys captioned media permanently; and `civitai app listing status` has left a
  shadow revision open on **`panorama-360`**, operator-only to clear.

### Added 2026-09-28 — a real user's session is worth more than the whole synthetic rig

- 🔴 **THE OPERATOR'S OWN opencode SESSION FOUND MORE THAN EVERY DOGFOOD CELL COMBINED, AND IT
  SUCCEEDED.** `ses_f1b9a7d40ffeqlEuYZlMEN0U2L` ("Civitai YouTube thumbnail generator"), model
  `z-ai/glm-5.3-flash`, **$0.18**, 110 messages / 3 user turns / 115 tool calls. It shipped
  `yt-thumbnail` v0.1.0 (submitted `22:01:19Z`, pending). Span 2 h 17 m but **agent working time
  was 45.6 min** — 67% was operator-idle, so ranking time sinks against the span understates every
  one by ~3×. Store: `~/.local/share/opencode/opencode-stable.db`, `data` is a JSON blob in
  `message`/`part`. ⚠ **`find-session.py` cannot find a session by id** — ids live in
  `session.id`, that tool searches message CONTENT; it returned only THIS session, because the id
  appeared in the prompt. Query the DB directly.
- 🔴 **THE 110-MINUTE BLOCKER WAS A FALSE PREREQUISITE, AND THE TOOLING MANUFACTURED IT.**
  `agent-setup` printed *"export CIVITAI_TOKEN=…"*; `prompt.md` §5 requires the agent to relay it
  **verbatim without explaining it**; the operator could not find the credential and returned 110
  minutes later. **`CIVITAI_TOKEN` appears in 24 parts of discussion and 0 of the 50 bash
  commands** — the app scaffolded, built, tested, validated and **submitted** without it. That
  block is about the MCP servers, which the session never touched (0 MCP calls). Compounding it,
  `--check` reported `"ok": true` on a row whose own detail said *"CIVITAI_TOKEN is NOT set … 401s
  until you export"*, under an instruction to read `ok` alone. Fixed in `#740`.
- 🔴 **THE SCAFFOLD SHIPPED THE ANSWER AND NOTHING POINTED AT IT.** `vite-plugin-civitai-setup.ts`
  (5,547 B) + `src/setup-dev-live.ts` (15,666 B) implement a **"Set up automatically"** button that
  mints the token, merges `.env.development.local` in place, and takes a pasted key — *never asking
  anyone to export anything*. The generated README mentioned `dev-token` **7 times** and the wizard
  **0**: it documented the harder path seven times over. Fixed in `#740`.
- ⚠ **Docs-vs-`node_modules` is still 1:1, and this CORROBORATES the synthetic measurement.** 4
  `webfetch` calls versus 4 `node_modules` reverse-engineering probes, and **all four doc fetches
  land in the first 44 seconds** — zero during the 14.7-min stall or the 15.8-min fix phase, the
  two windows where it was guessing at SDK surface. `apps/examples` and `llms.txt`, which
  `AGENTS.md` advertises, were never fetched.
- ⚠ **67% of agent working time was ONE FILE.** Six aborted 12.5k-token `App.tsx` writes (880 s)
  plus the fallout (951 s). Provider-side, but the monolithic `page-money` `App.tsx` is why a UI
  change reads as "rewrite the file".

### Added 2026-09-28 — four of my own claims that did not survive, and the pattern in them

- 🔴 **I SAID A BLOCK CANNOT LEARN THE HOST ORIGIN. `useHostOrigin()` IS A FIRST-CLASS EXPORT**
  (`@civitai/blocks-react/dist/index.d.ts:25`, *"the validated host origin, or `undefined` before
  `BLOCK_INIT` lands"*), present since 0.20.0 and already in the live bundle. My search pattern was
  `(baseUrl|origin)\??:` — it can only match **field declarations**, and this is a **hook**. I then
  called that *"I checked the whole SDK surface"*. **Sampling reported as enumeration**, and it was
  the load-bearing claim in a brief; the agent refused to assert it and filed on the contract
  mismatch instead. It is also why v0.1.3's fix needs no hardcoded domain.
- 🔴 **I READ A COMMAND AS ITS OUTPUT.** *"No config file found"* was the `||` fallback in step 60's
  command line; the recorded result was `exit code: 0` plus the credential. That single misreading
  sent the whole credential diagnosis at the wrong fix — `#739`'s read-back would have PASSED that
  run. **The `part` JSON carries both; read the result field.**
- 🔴 **I PRESENTED AN INFERENCE AS A MEASUREMENT AND BLAMED THE WRONG AGENT.** I said a sibling had
  deleted `scratchpad/pristine/`; it had `rm -rf`'d it itself after its PR opened, and its battery
  ran **nine minutes before** the sibling's write. Two agents corrected me independently and
  consistently. The warning I sent was still right to send — but it should have named the
  alternative explanation instead of asserting a cause.
- 🔴 **I BRIEFED THE WRONG CLI VERSION** (`0.1.109`) for the opencode session after measuring
  `0.1.105` myself earlier the same day. The agent caught it from the transcript.
- **The pattern, stated once:** every one was a narrow probe reported in the register of a
  measurement. The cure that worked each time was a **positive control** — and three of the four
  were caught by an agent declining to repeat my claim, which is the reason to keep asking
  subagents to verify rather than to comply.

### Evicted 2026-09-28 (close) — the 2026-09-28 process-traps block

Verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md). Its tells,
so you know whether to open it: a backtick inside a double-quoted `echo` EXECUTED `civitai login`;
`pkill -f` killed its own reporting shell and produced an empty probe that read as a result; and
`docker exec -d sh -c` had no login path, so the probe returned a zero about itself.

### Added 2026-09-28 — a guard over a mirror could not accept the mirror's own update

- 🔴 **A DRIFT-GUARD THAT PINS CONTENT CANNOT EVER ACCEPT AN ADDITIVE UPSTREAM CHANGE, AND THE
  AUTOMATION THAT EXISTS TO LAND THAT CHANGE IS WHAT IT BLOCKS.** `TestEnumFindingsKeepTheirExactWording`
  spelled out all thirteen scopes; the canonical added two; `revendor-canonical-schema.yml` runs the
  suite on its own output, so it failed and opened no PR. **The cure is to split the claim: pin the
  SHAPE, derive the CONTENT.** Same shape as the base-clone refresh hook that worked exactly once per
  file and then silently stopped.
- 🔴 **BUT DERIVING BOTH HALVES IS THE TRAP ON THE OTHER SIDE, AND IT IS SILENT.** If the expected
  message is built entirely from the schema, a reword of the library's lead-in moves `want` and `got`
  together and **nothing fails**. **Anchor the template against a hand-written literal over the one
  enum that is not expected to grow**, and assert that literal against real output too. The mutant
  that proves it is a reword injected where the code CONSUMES the library message — not a change to
  the test.
- 🔴 **A HAND-KEPT MIRROR OF A CONSTANT IN ANOTHER REPO IS INVISIBLE TO A DRIFT-GUARD SUITE THAT KEYS
  ON THE SCHEMA.** `SENSITIVE_BLOCK_SCOPES` was one entry behind production and no gate could see it,
  because it mirrors a TypeScript constant and every guard reads the vendored JSON. **When a
  re-vendor adds a capability, ask what ELSE mirrors that capability by hand.**
- 🔴 **`CDPATH` IS EXPORTED ON THIS HOST, SO `$(cd "$(dirname "$0")/.." && pwd)` RETURNS TWO LINES.**
  bash's `cd` echoes its target when it resolves via `CDPATH`, so `check-canonical-schema.sh` and
  `revendor-canonical-schema.sh` build a `$VENDORED` with an embedded newline. `jq -S . "$VENDORED"`
  then fails **inside a process substitution**, which `set -euo pipefail` does not catch, so
  `diff <(empty) <(canonical)` prints the whole canonical as `>` lines and the script announces
  **`FAIL: … has DRIFTED`** for a file `cmp` calls identical. **The tell is that EVERY diff line is
  `>` — one operand is empty, not different.** CI has no `CDPATH` and is unaffected, which is worse:
  it contradicts a green CI for reasons unrelated to the code. **Run repo scripts with
  `env -u CDPATH`; do NOT "fix" the scripts — the idiom is correct and the host is the anomaly.**
- ⚠ **A CLOSING CONDITION CAN NAME AN OUTCOME THE SYSTEM CORRECTLY REFUSES TO PRODUCE.** Rank 25's
  *"…and opens a PR unaided"* is unreachable once the thing it would PR is already in sync. **A
  condition that only holds while the bug is still present is not a closing condition** — rehearse the
  failing path instead, and say which half you rehearsed.
- ⚠ **`gh run view --log-failed` was the fastest route to the real cause** — the check-run's
  `output.summary` was **empty**, so the SHA-pinned `check-runs` API said only `failure`. The diff and
  the two test names were in the job log.

### Added 2026-09-28 (close) — ⚠ RETRACTED HEADING: the bot had been blocked THREE times, not five, and two instrument traps

- ⚠️ **RETRACTED 2026-09-28 (same day, in `#749`) — THIS BULLET ORIGINALLY READ "BLOCKED AT LEAST FIVE
  TIMES … A RECURRING CLASS, NOT AN INCIDENT", AND THAT WAS WRONG.** It was assembled from an issue
  *title* search, which is the trap: the title is written by whichever `classify` arm fired, so one
  headline covers unrelated causes. Corrected from the issues' bodies and run logs:
  | issue | what actually failed |
  |---|---|
  | **#323** (08-10) | a **hand-dispatched FIRE DRILL** (`drill=true`) — its body says *"NOT a real failure of the bot"*, and its title is *"could not re-vendor"*, a **different** title (the `revendor-failed` arm) |
  | **#486** (08-24) | `TestPatternRulesCoverTheVendoredSchema` — the pattern guard, on the `repository` regex |
  | **#607** (09-15) | `TestEnumFindingsKeepTheirExactWording` only — the enum half, fixed by `#745` |
  | **#695** (09-25) | `TestBuildExcludesGitFileInSubmodule` in **`internal/pkgzip`** — nothing to do with the schema (run 36063482611) |
  | **#743** (09-28) | **both** halves |

  So **three** schema-shape blockages (#486, #607, #743), of which **two** were the pattern guard —
  **both true positives, cleared fast (#486 open 31 min, 2026-08-24 18:45→19:15Z; #743 eleven hours,
  2026-09-28 04:51→15:51Z), zero false positives**. 🔴 **Two lessons survive the correction, and they
  are the durable ones.** First: **the validate gate is `go test ./...`, so
  ANY red test anywhere in the repo blocks the bot under the headline "the canonical schema changed in
  a way the CLI cannot accept"** — that is #695, and it means the headline is a claim about which ARM
  fired, never about the cause. Second: **the case for `#749` does not rest on the count.** `ci.yml`
  runs `schema-drift` on `pull_request:` with **no path filter**, so it runs on **every PR in the
  repo** and stays red while the vendored mirror is stale — a single blockage reddens a check on every
  unrelated human PR until a human writes a gloss. That argument holds at N=2 and is what to lead with.
- 🔴 **A green run AUTO-CLOSES the failure issue, so `gh issue list --state all --search 'revendor
  in:title'` is the health history — and the closed ones are the evidence.** ⚠️ But that search is
  exactly what produced the retracted "five" above: it enumerates **titles**, and the title names the
  `classify` ARM, not the cause. **Read each body and its run log before counting anything as an
  instance of one class** — two of those five were a fire drill and an unrelated packaging test.

- ⚠ **The original claim, kept STRUCK rather than deleted so the false text stays visible — the
  correction and the per-issue evidence are the bullet directly above:**
  ~~**THE VENDORED-SCHEMA GUARDS HAVE BLOCKED THE RE-VENDOR BOT AT LEAST FIVE TIMES — A RECURRING
  CLASS, NOT AN INCIDENT … roughly monthly.**~~ 🔴 **The half that STANDS: a green run AUTO-CLOSES the
  failure issue, so `gh issue list --state all` is the health history and the CLOSED ones are the
  evidence.** What was wrong was counting those titles as five instances of ONE mechanism.
- 🔴 **`gh run rerun` REPLAYS THE EVENT'S PINNED SHA, SO IT CANNOT CLEAR A RED A PR INHERITED FROM A
  MOVED BASE.** After `#745` merged and `main` went green, rerunning the failed `schema-drift` job on
  `#742` and `#744` returned **`failure` in 6 seconds** — replaying the OLD merge commit, which still
  carried the stale schema. It reads exactly like *"the fix did not work"*. **The tells:** an
  implausibly short rerun, and `main` green on the same check while the PR is red. **The remedy is a
  NEW EVENT, not a replay — `gh pr update-branch <n>`** (non-destructive, no force-push). 🔴 **And the
  corollary that matters more: once the base moves, the PR's OTHER green checks are stale too** —
  `build-test` passed against the old base and nothing re-reds it.
- ⚠ **Two GitHub API lags misread as state, both in one minute:** `gh pr view --json headRefOid`
  served the **pre-update** head right after `update-branch` reported success, and `mergeStateStatus`
  returned `UNKNOWN`. Neither is evidence; re-read, or `git fetch` the branch and compare.
- ⚠ **The `CDPATH` two-line-path lesson was stated TWICE in this section; the duplicate is evicted**
  to [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md) under *Evicted
  2026-09-28 (rank 27 close) — duplicate CDPATH bullet*. The surviving statement is above, and
  `## How to verify` carries the operational form. Nothing was lost.
- ⚠ **A zsh monitor printed `PR7 PR7` instead of a check tally** — `PR$n[$term/$tot]` is ARRAY
  SUBSCRIPTING in zsh, so the display was garbage while the arithmetic was fine. The verdict happened
  to be right; it was re-read directly before acting, because a mangled instrument does not get
  believed. Brace it (`${n}`) — same family as the `$VAR:` history-modifier trap.

### Added 2026-09-28 (audit batch) — I COUNTED ISSUES BY TITLE AND GOT THE PREMISE WRONG, plus four instruments that lied

- 🔴 **RETRACTION. "THE GUARDS HAVE BLOCKED THE RE-VENDOR BOT AT LEAST FIVE TIMES, ROUGHLY MONTHLY,
  ALL FIVE UNDER ONE TITLE" IS FALSE. I WROTE IT, IN FIVE PLACES.** Found by `#749`'s round-0 audit,
  then verified against the run logs myself. **TRUE FIGURES: 3** schema-shape blockages — `#486`
  (`TestPatternRulesCoverTheVendoredSchema`, the `repository` regex), `#607` (the ENUM half only),
  `#743` (both) — of which **2** were this guard, **both TRUE POSITIVES cleared in under a day**, and
  **0 false positives**. `#323` is a **hand-dispatched FIRE DRILL** whose body says *"NOT a real
  failure of the bot"* and whose title is *"could not re-vendor"* — a **different** title, so the
  one-title claim is false too. `#695` failed on `TestBuildExcludesGitFileInSubmodule` in
  **`internal/pkgzip`** (run `36063482611`) — no schema guard involved. `via: measurement`
- 🔴 **THE REUSABLE ERROR, AND IT IS THE POINT: I COUNTED ISSUES BY TITLE AND NEVER OPENED ONE.** A
  bot that files under ONE marker and **REWRITES the body on every failure** makes every failure look
  like the same failure — `failure-issue.yml` keeps exactly one open issue and rewrites it, so **the
  title is a MARKER, not a diagnosis**. A title-keyed count of a bot's issues measures how many times
  the BOT FAILED, never how many times one MECHANISM fired. **Read the run log per occurrence before
  counting anything as an instance of a mechanism.** ⚠ It propagated into 5 sites — this doc, the
  `cairn cli/manifest` entry, two `#749` files and my own brief to the agent that built it, which is
  how a wrong premise ends up in code comments. **A retraction is a tree-wide SWEEP.**
- 🔴 **AND THE JUSTIFICATION THAT SURVIVES IS STRONGER THAN THE ONE I INVENTED**, which is the part
  worth keeping: `ci.yml` runs `schema-drift` on **EVERY PR in the repo**, and it stays red until a
  human writes the gloss. So a blockage is not "the bot stalls" — it is a red check on every
  unrelated human PR. **Use that framing; never the five-times one.**

- 🔴 **THREE HARNESS BUGS IN ONE MUTATION SESSION, EACH WOULD HAVE PRODUCED A CONFIDENT WRONG CLAIM.**
  (a) `grep -cF` **cannot count a MULTILINE pattern** — each line becomes its own pattern and it
  returns lines matching ANY; mine read "12 occurrences" for a unique 3-line target. Use a Python
  multiline replace asserting `count==1`. (b) An extracted test file **missing its imports** fails to
  BUILD and `go test` says `[build failed]` — nearly filed as "red at base", the opposite claim.
  (c) A grep with the wrong indentation returned a **false zero**, because a YAML block scalar dedents
  its content. **All three caught by reading CONTENT, not the count.**
- 🔴 **A MUTANT SCORED ON A BASELINE THAT IS NOT GREEN HAS MEASURED NOTHING.** The goods battery
  aborted on its own positive control: two of this repo's OWN guards had refused the change —
  `TestEveryCheckEmitsAField` (a new finding-producing function the corpus never reached) and
  `TestEveryFindingCarriesItsDocumentedField` (findings with no ledger entry). **Both right.** A
  corpus fixture must trip ALL branches of a function that stops at the first hit per entry.
- 🔴 **NEAR-MISS: a `/tmp/wt-*` worktree that is NOT YOURS can be the only copy of graded evidence.**
  `/tmp/wt-verify686-1071809` holds the sole per-step transcripts for `ab-genpost-glm-01` and
  `ab-genpost-mimo-01`. A glob over `/tmp/wt-*` destroys them — the class that already cost this arc a
  control run. **Remove the EXACT paths you created, never a pattern.**
- ⚠ **`gh pr update-branch` reports success while `headRefOid` still serves the PRE-update head**, and
  `mergeStateStatus` returns `UNKNOWN` (lazy computation). Neither is evidence; re-read, or `git fetch`
  the branch and diff it. After an update, also confirm the PR's diff vs `main` is still ONLY what you
  intended — that is the cheap check that the merge brought in payload and nothing else.
- ⚠ **Adding `schema-drift` to the required contexts is a real trade, not a free win.**
  `check-canonical-schema.sh` hard-exits 1 on ANY non-200, so a civitai outage would block every merge,
  and `enforce_admins: true` means nobody can click through. The gap it closes is narrow — a
  hand-edited mirror — because a canonical-side change is caught by the daily bot regardless. The
  `gh api` call REPLACES the context list, so all six must be spelled or one is silently un-required.

### Added 2026-09-28 (close) — every mistake I made had ONE cause: I asserted where I should have derived

- 🔴 **FIVE FALSE ZEROS AND ONE FALSE PREMISE, ONE ROOT CAUSE.** (a) "blocked five times" — counted
  issues by TITLE, never opened one. (b) "2 sites" for a retraction sweep — it was **6**; I used one
  pattern shape, the agent used three plus a positive control. (c)/(d) two greps that reported a
  present claim MISSING because the phrase **wraps** and the grep is line-based. (e) a guessed path
  (`internal/validate/revendor_owed_signal_test.go`) when the file is at the repo ROOT — the diffstat
  one line above already said so. (f) `len(open(p).read())` = CHARACTERS against a BYTE ceiling, an
  890 B gap. **Every one was caught by a subagent or by reading CONTENT. Derive the path, vary the
  pattern shape, and use a positive control — then the count is worth quoting.**
- 🔴 **AN AUDIT'S REPRODUCTION CAN BE MECHANICALLY WRONG WHILE ITS FINDING IS RIGHT.** The `owed`
  defect was reported as *"`set -uo pipefail` with no `-e`, so any `jq` failure goes silent"*. GitHub
  runs every `run:` block as **`bash -e {0}`** — errexit IS on, and **that workflow's own header says
  so, citing two prior incidents**. The defect is real with a NARROWER trigger: `jq` exits **0
  printing nothing** on EMPTY input, which is what a truncated ledger write produces. **Re-derive a
  mechanism before relaying it; the file often already contains the answer.** `via: measurement`
- ⚠ **`rerere` is ENABLED here and CACHED my conflict resolution** (`Recorded resolution for …`). Mine
  was verified per-claim on both sides, so replay is safe — but anyone re-doing that merge gets my
  answer without being asked. This arc has already been bitten by a cached BAD resolution.
- ⚠ **`gh pr merge` can report `already merged` for a merge it just performed.** Confirmed benign:
  `mergedBy` was this account and `autoMergeRequest` was null. Read those two fields before treating
  it as a third party.

- 🔴 **A `Contains` ASSERTION AND AN `==` ASSERTION NEED THEIR MUTANTS IN DIFFERENT PLACES, AND THE
  DIFFERENCE IS INVISIBLE IN THE SOURCE.** Two guards spelled the same literal; the `==` one reddened on
  any change to the enum, the `Contains` one only on a change landing inside the matched prefix. So the
  documented repro for a defect at BOTH sites demonstrated only ONE, and a fix verified with it would
  have shipped half-done reading as complete. **Ask what SHAPE of assertion each site uses before
  choosing where to mutate; for an ordered list, vary POSITION, not presence.**
- 🔴 **A GUARD OVER A SELF-BLOCK CAN ITSELF SELF-BLOCK, AND THE CURE IS TO FREEZE ITS NEEDLE.**
  `content_rating_literal_ledger_test.go` searches for a hand-typed rating list. Deriving that needle
  from the schema would have been the obvious "DRY" choice and would have re-created the bug one level
  up: a canonical that grows the ratings moves the needle, it stops matching the frozen anchor, and the
  ledger reports a SHRINK. **A guard over a frozen specimen must itself be frozen** — and it then
  matches its own source file, so exclude that file by path. ⚠ It instantly caught two rating literals
  in my own COMMENTS — prose quoting a set looks identical to an assertion. **Describe, don't quote.**
- ⚠ **`go test ./...` buffers per package, so a long root package looks hung for ~6 min**; two runs here
  were killed only because the tree changed under them after launch. **`sha256sum` before a long run,
  then leave the tree alone.** `golangci-lint` is absent here (`nix-shell -p golangci-lint`) and its
  `0 issues` means nothing until an unused func in your new file is seen to red it.

### Carried forward 2026-09-28 (rank 27) — durable lines rescued from `State now` / `Next steps`

🔴 **These sat under REPLACE headings; this round's rewrite would have deleted them.** Re-homed, not new.

- **The eight PRs of 2026-09-28, verified on `origin/main` BY CONTENT** (never by ancestry): `#745`
  `a8a1338` · `#742` `f4958df` · `#744` `64426e8` · `#746` `9052238` · `#750` `7d333a2` · `#748`
  `8d1733e` · `#747` `b98585f` · `#749` `2105a33`. Ranks 12/25/26/28 closed, claims `-12`/`-25`
  released. Earlier: 20/21/22 as `#735`/`#733`/`#734` (2026-09-27).
- 🔴 **AUDIT PROVENANCE — "merged" ≠ "audited", and only ONE was.** `#745` is the only PR ever audited
  for CORRECTNESS (it found three defects, **two introduced by the fix itself**). `#749` had round 0
  only; `#748`/`#747` merged on mutation batteries plus CI by dated operator decision; **`#752` none.**
- 🔴 **MEASURED — the vendored enum's CONTENT is guarded by nothing that can block a merge:** injecting
  `evil:total:takeover` into the scope enum leaves `internal/validate` and `internal/manifest` **GREEN**.
  The gap the `schema-drift` required-check would close.
- ⚠ **`#675`/`#602` were never red on `schema-drift`** — their green dates from **2026-09-20**, before
  the canonical change, so they never re-ran. **A stale green is not a pass, nor a red.**

- 🔴 **TWO `handoff_doc.py` CEILING TRAPS. (a) AN EVICTION IS TWO FILES; `--confirm` COMMITS ONE PATH** —
  the archive half dies with the worktree, and a dead stub reads like a clean eviction. Commit the archive
  side separately BEFORE the confirm, then `grep -c` the stub's section title in BOTH files on the pushed
  ref (want 1 and 1) — `dd1de77`. **(b) The ceiling is BYTES**: `len(open(p).read())` counts characters,
  and `🔴 ⚠ ✅ —` made that gap 890 B — it read 64,728 for 65,618 B. **`wc -c`.**

- 🔴 **ASK WHETHER THE WORK SHOULD EXIST BEFORE AUDITING WHETHER IT IS CORRECT.** Rounds 0–4 found
  23 real defects, every one in the guards the ladder was itself writing; round 5, run as a
  REQUIREMENT round, refuted the premise and ended in a −270-line deletion. **The tell it was time:
  each round's fix was the next round's finding, three times running.** One requirement round was
  worth more than four correctness rounds.
- 🔴 **MEASURE A GUARD'S OWN FAILURE COST BEFORE HARDENING IT — three one-command checks.** Is the
  check REQUIRED (`gh api repos/<r>/branches/main/protection`), has the guarded thing ever MOVED
  (`git log -p -- <file>`), which run log actually failed (`gh run view <id> --log-failed`)? All
  three would have stopped this at round 0.
- 🔴 **A SPELLING GUARD ON A PROVENANCE PROPERTY IS WALKABLE, AND VALUE CHECKS CANNOT HELP** —
  "hand-written" and "schema-derived" are EQUAL at rest, which is why the original defect shipped
  unseen; only a syntax-tree check sees it. The AST version worked and was deleted anyway, because
  **a guard whose sole purpose is guarding a deleted guard goes with it.**
- 🔴 **SWEEP FOR IN-FLIGHT WORK BEFORE OPENING A PR, EVEN WHEN AUTHORISED TO CREATE ONE.** A branch
  name collision was the only thing that stopped a duplicate pin-bump PR racing the repo's own
  automation against a REQUIRED check. `gh pr list --state open` is the cheap half.
- ⚠ **`git checkout -- <file>` reverts a mutant AND your uncommitted edits in it, together** — cost a
  round here. Restore from `cp -a` and `cmp`. ⚠ **A pipe eats the exit status**; capture
  `out=$(cmd 2>&1); rc=$?`. ⚠ **`grep -E -- '--- …'`** needs the `--`. ⚠ **`gh pr update-branch`
  reports success while the API serves the PRE-update head**, and after the base moves every prior
  green is stale — confirm git and the API agree, then require every check TERMINAL plus a minimum
  count (13 here).
- ⚠ **AN EVICTION IS TWO FILES AND `handoff_doc.py --confirm` COMMITS ONE PATH.** Commit the archive
  side yourself, in the same commit as the doc stub, BEFORE the confirm run — the reverse order
  destroyed an eviction here and left the doc pointing at a section that did not exist.

- 🔴 **VERIFY A WORKFLOW CHANGE BY WATCHING IT RUN, NOT BY PARSING IT.** `#756` moved a check
  between workflows; the YAML parsing, the trigger shape and the required-context set were all
  confirmed statically — and none of that would have caught a trigger that never fires. The merge to
  `main` firing the new workflow, and passing, is the only evidence that mattered, and it was one
  `gh run list --workflow` away.
- 🔴 **A GREP FOR A LITERAL MATCHES THE COMMENT THAT RETIRES IT.** Checking `#755` landed, my own
  grep for the hardcoded auth list returned `1` — it had matched the comment I wrote explaining that
  the literal was removed. **Classify each hit as code or comment before believing a count**; the
  one-liner that does it is `case "$(sed 's/^[[:space:]]*//')" in "//"*)`.

## How to verify

🔴 **Run repo scripts with `env -u CDPATH`** — else `check-canonical-schema.sh` reports a confident
`FAIL: … has DRIFTED` over a byte-identical file. **The tell: EVERY diff line is `>`** (one operand
empty). Do not "fix" the scripts; the host is the anomaly.

```bash
CLI=/home/zach/workspace/civit/cli
git -C "$CLI" worktree add --detach /tmp/wt-v origin/main
(cd /tmp/wt-v && env -u CDPATH bash scripts/check-canonical-schema.sh); echo "rc=$?"   # 0 + OK
# negative control: bump "maxItems": 32 -> 33 in schema/ and re-run; want rc=1
(cd /tmp/wt-v && env -u CDPATH go test ./internal/validate/ ./internal/cmd/ ./internal/manifest/ -count=1)
git -C "$CLI" worktree remove --force /tmp/wt-v
```

**Rank 27 on `main`:** append a `contentRating` value → `internal/validate` and `internal/cmd` both
green; retire one → also green now (shrink guard deleted, see above); re-add the literal to
`internal/cmd` → ledger reds `GREW`. **#755:** grow the `auth` enum → `internal/manifest` green with
the fix, **FAIL** at `origin/main`; below 2 entries → vacuity floor fires.

⚠ **The root package needs a browser** (render-oracle tests refuse to skip):
`nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) env -u CDPATH go test ./... -count=1'`
— ~334 s and **buffers silently**, so `sha256sum` first and leave the tree alone. ⚠ **Grep wrapped prose on NORMALISED text**
(`tr '\n' ' ' | tr -s ' '`); a `grep -oE` with two wide `{0,220}` bounds over a 100 KB doc
backtracks for minutes — use Python.

## Defects (batched)

🔴 **Added 2026-09-28 — from the live-app feedback and the onboarding diagnosis.**

- **`ab-img-poster`'s metadata gap is NOT fixable in the app.** `BlockCreatePostRequest` is exactly
  `{sources, title?, detail?, tags?, modelVersionId?}` — no metadata field — and
  `BlockPostSource` (workflow) is `{kind, workflowId, imageIndexes?}` — no flag. The host creates
  the `Image` rows for a workflow source, so only the host can attach generation metadata. Tracked
  as `civitai/civitai#5182`; nothing to do in this repo or the app.
- **v0.1.3's URL fix is verified in the bundle, NOT against the real host.** `new URL(` goes 1 → 2
  versus the live v0.1.2 bundle, the minified call reads `new URL(i,r||void 0)`, and all six helper
  cases were unit-tested in node — but no live post has exercised it. The first post through v0.1.3
  after approval is the real test.
- **The post arm still cannot see a block that faithfully renders a malformed host URL** on the
  ordinary arm — deliberate, since defaulting the mock to the platform's relative shape would move
  every recorded verdict. `#742` catches an app that MANGLES the url; the malformed-host case needs
  `CIVITAI_ASSERT_POST_MALFORMED_URL=1`.
- **`#742`'s arm reads only `a[href]`** — a button calling `location.assign`, a copy-to-clipboard
  control, or a link in a shadow root is invisible; no navigation is attempted, so an absolute href
  pointing at the WRONG post passes.
- **`agent-setup --check`'s stale-CLI row is verdict-EXEMPT by decision**, so an agent that reads
  only `ok` — which the hosted prompt instructs — still will not notice a stale CLI. Fixing that
  means changing `prompt.md`'s *"read `ok`, not the rows"* instruction, which lives outside this
  repo. Recorded in `#740`'s own output and pinned by a test.
- `pickerBlind` and `consentBlind` have never been tested together on one doubly-blind bundle.
- `CONSENT_SETTLE_MS = 1200` is a judgement; every observed ask was synchronous.
- Nothing is known about the **iframe** transport; all of this is the inline path.
- `civitai app listing status` opens a shadow revision on a LIVE listing and there is no
  `discard-revision`. One is **still open on `panorama-360`** — operator-only to clear.
- The submissions listing caps at 100 rows with no cursor, and the account is AT the cap.
- `civitai app submit` **exits 0 when it did not submit** (no token) — it warns `⚠ NOT SUBMITTED`.
