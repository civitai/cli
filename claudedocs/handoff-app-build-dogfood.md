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

🔴 **WHY THE VERDICT MUST NOT BE `app validate`.** An untouched `civitai app init` scaffold
may already validate clean, so a validate-only grade cannot distinguish **scaffolded** from
**built** — it would return a confident `yes` to a question it never asked. The brief must
name a behaviour the scaffold does not have, and the browser must observe *that*. Operator
decision, 2026-09-20, chosen over a build-only oracle for exactly this reason.

## State now

- ✅ **Ranks 1, 2, 3, 5 SHIPPED; rank 4 DELETED by measurement.** `cli#678` (brief
  injection + the celsius brief), `cli#679` (unblocked both required checks), `cli#677`
  (this doc), `cli#681` (the render oracle, squash `9d7b5a2a`), `cli#680` (rank 3 measured,
  squash `39c2785`). All verified by content with negative controls.
- ✅ **The oracle works and BOTH controls are measured.** Positive: the live specimen grades
  `gate=pass gate_rc=0 observed=212 RENDER=yes`, re-run independently. Negative: an
  untouched `static` scaffold grades `gate=pass … RENDER=no` — the validator passes while
  the browser says nothing rendered.
- 🔴 **SCOPE CHANGED BY OPERATOR DIRECTION, 2026-09-21 — and it is WIDER than the frozen
  closing condition.** The ask is now: **one agent (`z-ai/glm-5.3-flash`), build an app that
  integrates GENERATION and POSTING, and SUBMIT IT FOR REVIEW.** Read carefully against the
  Goal above: a gen+posting app with its own assertion *would* satisfy the frozen condition,
  but **`app submit` is nowhere in it**. The submit is a NEW objective, not another round of
  this one. Recorded here rather than quietly folded in, because the condition is frozen and
  a later session must not read a submitted app as evidence the condition was met.
- 🔴 **BASELINE CAPTURED 2026-09-21, BEFORE ANY CREDENTIALED RUN — this is the control and
  it cannot be re-derived after the fact.** The `dogfood-3` precedent's discipline is to
  verify from the ACCOUNT, never from the agent's report.

  | | value |
  |---|---|
  | Buzz Blue | 1,305,157 |
  | Buzz Green | 947,618 |
  | Buzz Yellow | 1,849,047 |
  | **Buzz TOTAL** | **4,101,822** |
  | listings visible to `app list` | 13 |

  🔴 **If a credentialed run happens and this number was not read first, the run is
  UNGRADEABLE** — the same class as the pre-registration failures recorded elsewhere in
  this repo, where the grading instrument had no pre-period.
- 🔴 **BLOCKER FOUND — the harness cannot carry a credential without leaking it.** See the
  investigation block. A PR is in flight (`feat/dogfood-credentialed-trial`) adding a
  non-recording credential path, a second brief for generation + posting, and mechanical
  caps. **No trial has been run and nothing has been spent.**
- ⚠ **I UNDERSTATED THE BLAST RADIUS TO THE OPERATOR AND THEN CORRECTED IT.** The approval
  question said the account owned "two approved published apps" (`sensei`, `comfy`), taken
  from a truncated `app doctor`. `app list` shows **13 listings, ~10 owned by
  `zachlowdenzx`** — also `radio`, `cosmetic-studio`, `panorama-360`, `model-benchmarking`,
  `custom-generators`, `playable-collections`, `app-requests`, `vitrine`. The decision was
  taken on the smaller number; the bounds are unchanged and the precedent ran on this same
  account, but the correction is on the record.
- **Claims:** all released. `app-build-dogfood-1/-2/-3/-5` are done and free.
- **Carried forward — RANK 3'S TABLE.** Kept here deliberately: the appended investigation
  block "✅ ANSWERED 2026-09-21 — the harness CAN carry an app-build task" says *"see the
  State-now table"*, so deleting it would leave that block pointing at nothing.
  `xiaomi/mimo-v2.5`, `df-node-root`, brief delivered:

  | | measured | cap set |
  |---|---|---|
  | steps | **65** | 80 |
  | total cost | **$0.0145** | $0.50 |
  | prompt tokens | 1,564,496 | — |
  | completion tokens | 9,468 | — |
  | per-call prompt | 452 → **45,328** (~100× over 66 calls) | — |
  | `stop` | **`finished`** | — |

  The O(n²) growth is real and does not matter at this price; `stop=finished` is what makes
  it a completed task rather than a harness limit.
- **Carried forward — the prerequisite arc's baseline, which rank 3's number is read
  against.** Measured 2026-09-19 over 18 trials: the three cheap models reach SETUP on 4 of
  6 grid rows, model-independently, at **mean $0.0058/trial** (21 trials = $0.1211), with
  the frontier control **5–20× dearer**. So an app-build trial at **$0.0145** is ~2.5× a
  setup trial.
- **Carried forward — the credentialed precedent, now directly load-bearing for rank 7.**
  `claudedocs/handoff-dogfood-3.md` is the *first CREDENTIALED blind dogfood* (2026-08-10):
  3 generations, 61 Buzz, 2 submissions, verified from the account
  (4,187,454 → 4,187,393). Read it before running rank 7 — it also records real
  `submit`/`withdraw` defects, including permanent loss of captioned screenshots.

## Open investigations — live diagnosis state

### 🔴 The harness cannot run a task this long — O(n²) prompt growth against a 40-step cap
- as-of: 2026-09-20

- **Symptom + exact repro:** read `scripts/dogfood/runner.py:118-131`. `--max-steps`
  defaults to **40**; `--max-cost` defaults to **$1.0**; `MAX_OUT = 12000` bytes of tool
  output are handed back per command (`:61`), and every turn resends the entire history.
- **Observed (with values):** the module's own comment states it —
  *"cumulative prompt tokens grow O(n^2) in steps. Observed runs took 3-8 steps and cost
  ~$0.02-0.10; at the 40-step cap that is roughly 25x, and a model that loops on a failing
  install — exactly the failure being measured — is the case that reaches it. This is the
  only bound on money."* The setup task finished in **7–11 steps**. An app build is
  plausibly **30–80**.
- 🔴 **Why this is an EARLY item and not a tuning detail** (it is rank 3, behind the two
  things that make an app-build trial exist at all)**:** a trial that hits the step cap
  or the cost cap emits `CLOSING_CONDITION=no`, which is **indistinguishable from the
  product being broken**. That is precisely the capability confound the prerequisite arc
  went to the trouble of ruling out by measurement; here it returns harder, because a
  longer task reaches the cap on the happy path rather than only when looping.
- **Ruled out — that raising `--max-steps` alone fixes it.** The growth is quadratic in
  steps *because the whole history is resent*; raising the cap raises the cost it is the
  only bound on. `via: code` (`runner.py:122-129`, the comment and the loop at `:179`)
- **Leading hypothesis:** the runner needs history management — trimming or summarising
  older tool output — before any app-build trial is graded. `MAX_OUT` bounds a single
  command's output but nothing bounds the accumulation.
- **Next probe:** run one app-build trial on the CHEAPEST model with
  `--max-steps 80 --max-cost 2.0` and read steps, cumulative prompt tokens and cost per
  step out of `transcript.jsonl`, which already records per-call `usage` (`runner.py:190`)
  — no instrumentation needed. That curve decides whether trimming is required or merely
  nice. Do this BEFORE building the render oracle — if the curve is fatal, the oracle
  grades nothing. ⚠ **Blocked until ranks 1 and 2 land**: there is no app-build trial to
  run until the runner can receive a brief.

### ✅ ANSWERED 2026-09-21 — the harness CAN carry an app-build task; rank 4 is not needed
- as-of: 2026-09-21

🔴 **This RESOLVES the block above it, "The harness cannot run a task this long — O(n²)
prompt growth against a 40-step cap" (as-of 2026-09-20). Its measurements stand; its
CONCLUSION does not.** That block said history management was needed "before any app-build
trial is graded", and made it rank 1. Measurement says otherwise. **Do not act on its Next
probe — it has been run, and this is the result.** (The tool appends here and cannot edit
an earlier heading, so this paragraph is the retirement marker.)

- **Observed (with values):** see the State-now table — 65 steps against an 80 cap,
  $0.0145 against a $0.50 cap, `stop=finished`.
- **Ruled out — that the step or cost cap would bite on the happy path.** It did neither,
  with ~19% step headroom and ~34× cost headroom. `via: measurement`
- **Ruled out — that O(n²) prompt growth is a cost problem HERE.** The growth is real and
  visible (452 → 45,328 per call, 1.56M total), and at this model's price the entire run
  is 1.5 cents. The mechanism was correctly identified; the consequence was overestimated.
  `via: measurement`
- **Still open, and the caps should STAY:** a model that LOOPS is the case that reaches a
  cap, and that case has not been observed. Keep `--max-steps` and `--max-cost` set
  deliberately per run. Also unmeasured: a frontier model on the same task would spend
  5–20× more per the prerequisite arc's figures — still small, but not re-derived here.
- **Next probe:** none for rank 4. It is deleted, not deferred.

### 🔴 The harness cannot carry a credential — `--agent-env` writes its value into the transcript
- as-of: 2026-09-21

- **Symptom + exact repro:** read `scripts/dogfood/runner.py:237-238`:
  ```python
  rec("start", trial=a.trial, model=a.model, image=a.image, user=a.user,
      agent_env=a.agent_env, brief=a.brief, container=container)
  ```
  `--agent-env VAR=VALUE` (`:162`, applied at `:223-224`) is the ONLY mechanism for getting
  a variable into the trial container.
- **Observed (with values):** the `start` row of `runs/<trial>/transcript.jsonl` carries
  `agent_env` **verbatim**. Confirmed by reading the existing
  `runs/ab-curve-01/transcript.jsonl`, whose `start` row carries `brief` in full by the
  same call — the identical code path.
- 🔴 **Why this is a security defect and not an inconvenience:** the only available
  credential route would write a **live Civitai account token in plaintext** into a file
  that persists on disk, is read by humans and agents afterwards, and is routinely quoted
  into reports. The token in question authorises spending Buzz, submitting, and mutating
  ~10 published listings.
- **Ruled out — that the secret could simply be redacted at read time.** The value is
  written at trial start and the file is the durable artefact; redacting a reader does not
  unwrite the token. `via: code` (`runner.py:237`)
- **Leading hypothesis:** the injection must happen through a path that never passes the
  secret to `rec()` — e.g. copying the credential into the container after create, and
  recording only a non-reversible marker (a boolean plus a short digest prefix).
- **Next probe:** none needed to diagnose; the fix is in flight on
  `feat/dogfood-credentialed-trial`. 🔴 **The thing to CHECK when it lands is the leak test's
  POSITIVE CONTROL** — a grep that finds nothing because its pattern is wrong is
  indistinguishable from a grep that finds nothing because the secret is absent. Require the
  test to demonstrate it DOES find a planted secret.

## Next steps (ranked)

🔴 **Numbering stays stable.** Ranks 1–5 are settled; the operator's new direction is added
as rank 7 rather than rewriting rank 6, so the two objectives stay separable — one is inside
the frozen closing condition and the other is not.

⚠ **Carried forward from round 0 — on `forcing:` below, read this before quoting it.** The
operator asked for the ARC ("we want to build apps", 2026-09-20). He did **not** ask for any
individual item here; the decomposition is agent-authored. An earlier draft tagged five items
*"the operator asked for this measurement"*, which was false of every one of them. The tag
names the arc, which is the true forcing function, and this paragraph is the record that the
breakdown is not itself user-requested. ⚠ **Rank 7 is the one exception** — the operator
asked for that item specifically, in those words, on 2026-09-21.

1. ✅ **DONE — brief-injection path.** `cli#678`.
   forcing: user — satisfied
2. ✅ **DONE — the celsius brief and its assertion.** `cli#678`.
   forcing: user — satisfied
3. ✅ **DONE — the step/cost curve.** 65 steps, $0.0145.
   forcing: user — satisfied
4. ❌ **DELETED, NOT DEFERRED — history management is not required.** Rank 3 measured the
   condition it was gated on and the answer was no.
   forcing: gate — retired
5. ✅ **DONE — the render oracle.** `cli#681`, both controls measured.
   forcing: user — satisfied
6. **The matrix — 3 cheap models × 2 writable-prefix environments, celsius brief.** This is
   the item that grades the FROZEN closing condition. ~$0.09 plus a frontier control at
   5–20×. ⚠ Still gated by `cli#665` for the other two environments, and its brief touches
   no SDK surface (`"scopes": []`), so a green matrix means "can scaffold and wire UI plus
   arithmetic", NOT "can build an app that spends Buzz".
   forcing: user — the operator asked for this arc on 2026-09-20
7. 🔶 **IN FLIGHT — the CREDENTIALED run: one agent (`glm-5.3-flash`), an app integrating
   generation and posting, submitted for review.** Operator direction 2026-09-21, approved
   under the `dogfood-3` precedent's bounds: identifiable app prefix, generation cap,
   submission cap, `--max-cost` per generate, command log, **and verification from the
   account balance above rather than the agent's report**.
   🔴 **`app submit` is OUTSIDE the frozen closing condition** — do not report a successful
   submit as the arc closing. 🔴 **The no-touch rule for the ~10 existing listings is PROSE
   in a brief and nothing enforces it**; an account token is an account token, and
   `app withdraw` permanently destroys captioned screenshots (`handoff-dogfood-3.md`).
   Blocked on the credential-leak fix landing first.
   forcing: user — the operator asked for this on 2026-09-21

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
- ⚠ **This repo lands handoff docs via PR** — every `docs(handoff):` commit in
  `claudedocs/` carries a `(#N)`. Do not push one straight to `main`.
- ⚠ **Three older dogfood arcs exist and are NOT this one** —
  `claudedocs/handoff-dogfood-154.md` (2026-08-07), `handoff-dogfood-2.md` (2026-08-09),
  `handoff-dogfood-3.md` (2026-08-10). All are CLI-usability rounds; none grades app
  building and none has a render oracle. Checked before minting this slug, so a future
  session does not re-litigate whether this was a duplicate.

### Added 2026-09-21 — rank 3's measurement, and three inferences that did not survive it

- 🔴 **I ESTIMATED THE TRIAL AT $0.06–0.18 AND IT CAME IN AT $0.0145 — 4–12× CHEAPER.** The
  estimate was "setup cost × step ratio", which is the right shape and still wrong, because
  it priced every step at the average of a SHORT run. **Where a cheap measurement exists,
  take it instead of extrapolating** — this one cost 1.5 cents and deleted a ranked item.
- 🔴 **A MECHANISM CAN BE REAL AND ITS CONSEQUENCE STILL WRONG.** The O(n²) prompt growth
  is exactly as documented — per-call prompt went 452 → 45,328 and the run consumed 1.56M
  prompt tokens for 9,468 completion tokens. The *conclusion* drawn from it ("the harness
  cannot carry this task") was false at this price point. **Naming a mechanism correctly is
  not the same as pricing it.**
- 🔴 **A PIN-BUMP PR THAT MERGES CLEAN CAN LEAVE `main` RED, AND THE `CLEAN` IS AN ABSENCE
  OF CHECKS, NOT A PASS.** `cli#676` was bot-authored, and
  `.github/workflows/bump-scaffold-pins.yml:232-239` documents that a PR opened with the
  default `GITHUB_TOKEN` does **not** trigger this repo's other workflows (GitHub's
  loop-guard). So `pins-vs-published` / `scaffold-currency` never ran on it, its
  `mergeStateStatus: CLEAN` reported nothing, and `main` went red on merge. **Treat a
  missing check as UNMEASURED and assert a minimum check count before believing any
  rollup.** ⚠ Still open: that workflow validates `pins-vs-published` in-job but has **no
  in-job equivalent for `scaffold-currency`**, which is the half that actually broke. It
  will recur.
- 🔴 **THE PRIMARY CLONE'S WORKING TREE IS STALE AND IT COST ME A WRONG CONCLUSION.** I read
  `scripts/dogfood/driver.sh` from `/home/zach/workspace/civit/cli` and concluded the brief
  plumbing did not exist — the clone sat at `4f1df8c`, behind the `528b977` that had merged
  #678 twenty minutes earlier. **Read from `origin/main` or a fresh worktree before
  concluding a feature is missing.** The tell is concluding that work you just merged is
  absent.
- ⚠ **The agent used `civitai app create`, not `app init`.** A scan for `app init` returned
  zero and read as "it skipped scaffolding"; the real chain was
  `agent-setup --track app` → `--check --json` → `app create` → `app validate`. **Grep for
  the command the CLI actually ships**, not the one you assumed it ships.
- ⚠ **The specimen container is a consumable.** `dogfood-ab-curve-01` holds the only
  agent-built app in existence for this arc and re-creating it costs a trial. Snapshot with
  `docker commit` before doing anything mutating to it.

### Added 2026-09-21 — the credentialed turn

- 🔴 **CAPTURE THE ACCOUNT BASELINE BEFORE THE RUN, NOT AFTER — IT CANNOT BE RECOVERED.**
  The `dogfood-3` precedent verified its spend as a balance delta (4,187,454 → 4,187,393 for
  3 generations / 61 Buzz) rather than believing the agent. That only works with a
  pre-reading. Taken here as Buzz **4,101,822** across 13 listings. A run that starts without
  one is ungradeable on spend no matter how well it goes.
- 🔴 **THE ONLY CREDENTIAL PATH INTO THE HARNESS WAS A LEAK.** `--agent-env` records its
  value in the transcript, so the obvious way to authenticate a trial writes a live account
  token to disk in plaintext. **Before wiring a secret through any harness, grep where its
  parameters get RECORDED** — the argv, the transcript, the logs, the shell history. This one
  was two lines from the flag definition and would have been invisible until someone read a
  transcript months later.
- 🔴 **A GREP THAT FINDS NOTHING IS NOT EVIDENCE OF ABSENCE — a leak test needs a POSITIVE
  CONTROL.** Requested explicitly for the fix: the test must demonstrate its pattern DOES
  find a planted secret before its zero on the real transcript means anything. This is the
  same shape that has bitten repeatedly across this repo's history.
- ⚠ **I QUOTED THE BLAST RADIUS FROM A TRUNCATED COMMAND AND IT WAS WRONG BY 5×.**
  `civitai app doctor 2>&1 | head -4` showed two apps; `civitai app list` shows 13 listings,
  ~10 operator-owned. The approval was taken on the smaller number. **When a number bounds a
  risk someone is consenting to, read the whole output.**
- ⚠ **An account-scoped capability cannot be narrowed for this task.** `app dev-token` mints
  a budgeted `ai:write:budgeted` JWT (clamped against the bearer's `AIServicesWrite` bit), so
  GENERATION can be scoped — but `app create`, `app submit` and `app listing` need the
  account token. A run that includes submit therefore cannot be credential-bounded, only
  behaviour-bounded, and behaviour bounds on a blind model are prose.

## How to verify

**The credentialed run, graded from the ACCOUNT — the only method that does not trust the
agent's own report:**

```bash
civitai buzz        # compare TOTAL against the baseline recorded in State now (4,101,822)
civitai app list    # compare against 13 listings; every pre-existing slug must be UNCHANGED
```
🔴 Run both **before** any credentialed trial as well, in case the baseline above has aged.
The delta is the spend; the listing set is the blast-radius check.

**The credential must not appear in any artefact the run leaves behind:**

```bash
T=scripts/dogfood/runs/<trial>/transcript.jsonl
grep -c 'access_token\|refresh_token\|eyJ' "$T"        # expect 0
```
🔴 **A zero here is worthless without a positive control.** Plant a known string in a scratch
copy and confirm the same grep DOES find it; only then does the zero mean the secret is
absent rather than the pattern being wrong.

**Rank 3's curve, and the brief reaching the model** — unchanged; see the commands recorded
in the previous revision of this section.
