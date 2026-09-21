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

- ✅ **Ranks 1 and 2 SHIPPED — `cli#678` merged** (squash `528b9777`). The harness takes
  `--brief`, and `scripts/dogfood/briefs/` carries `celsius.brief.txt`, `celsius.md` and
  `celsius.assert.mjs`. Verified by content with a negative control: `--brief` is in
  `runner.py` at `origin/main` and absent at `main~1`.
- ✅ **This doc's own scoping PR `cli#677` merged** (squash `69f3885`), so the arc has a
  durable home and `claim-work --slug-for` resolves against `main`.
- ✅ **The repo-wide block is GONE — `cli#679` merged** (squash `336eb9ed`). Both required
  checks were red on `main` and nothing could merge. They were **two different failures**:
  `pins-vs-published` was a stale caret (`^0.46.0` excludes `0.48.0`, because caret on a
  `0.x` locks the MINOR), and `scaffold-currency` was a **broken export** —
  `blocks-react@0.55.0` moved `createLiveHost` off `./testing` to `./live` and removed
  `mockParentMessage` outright. A pin bump alone fixed only the first. ⚠ `cli#676`, an
  earlier pin-only bump, merged looking clean and left `main` red for a day; see the
  Gotchas entry — the mechanism is worth knowing before the next one.
- ✅ **RANK 3 IS MEASURED — and it DELETES rank 4.** One real trial,
  `xiaomi/mimo-v2.5` (the cheapest of the three), `df-node-root`, brief delivered:

  | | measured | cap set |
  |---|---|---|
  | steps | **65** | 80 |
  | total cost | **$0.0145** | $0.50 |
  | prompt tokens | 1,564,496 | — |
  | completion tokens | 9,468 | — |
  | per-call prompt | 452 → **45,328** (~100× over 66 calls) | — |
  | `stop` | **`finished`** | — |

  **The O(n²) growth is REAL and it does not matter at this price.** Per-call prompt
  inflated ~100× and the run burned 1.56M prompt tokens — and still cost **1.5 cents**,
  hitting neither cap. `stop=finished` is the discriminating field, so this is a completed
  task and not a harness limit wearing a task's clothes.
- 🔶 **The arc's question has a PRELIMINARY yes, and the word is load-bearing.** The trial
  produced `/work/celsius-converter` with `data-testid="celsius"`, a `Convert` button,
  `data-testid="fahrenheit"`, and `(c * 9) / 5 + 32` rounded — so 100 yields exactly
  `"212"`. It ran `civitai agent-setup --track app` → `--check --json` → `civitai app
  create` → `civitai app validate`, i.e. the real onboarding chain, and produced a `dist/`.
  🔴 **This is SOURCE INSPECTION, not the oracle.** The closing condition requires a
  headless browser observing a RUNNING block; that is rank 5 and it has not run. A
  component that READS correctly and one that RENDERS correctly are different claims — and
  this arc exists precisely because the cheap oracle lies. **n=1**: one model, one
  environment, one brief, no repeat, no control model.
- ✅ **RANK 5 SHIPPED — `cli#681` merged** (squash `9d7b5a2a`), and the preliminary yes
  above is now a MEASURED one: the oracle grades the specimen `RENDER=yes observed=212`,
  re-run independently rather than accepted from a report. 🔴 **The negative control is the
  load-bearing half** — an untouched `static` scaffold grades `gate=pass … RENDER=no`, so
  the validator and the browser DISAGREE on a bare scaffold exactly as the arc predicted.
- ⚠ **A briefing error worth carrying: `BLOCK_INIT` cannot be POSTED to a block, and it
  fails SILENTLY.** The oracle brief said it must post `BLOCK_INIT`; a block's transport
  drops any message whose `event.origin` is outside the allowlist baked into the bundle,
  and a scaffold allowlists `https://civitai.com` only. The working route is seeding
  `window.__CIVITAI_BLOCK_CONTEXT__` before the first script — the branch the SDK's own
  detector takes first. Measured both ways on a real `page-money` build.
- **The specimen `dogfood-ab-curve-01` is still held** and was NOT mutated by the oracle
  (`/work` byte-identical afterwards). **Do not destroy it**; re-creating it costs a trial.
- **Claims:** `app-build-dogfood-1` / `-2` released on merge; `-3` and `-5` release when
  `cli#680` and `cli#681` land.
- **Carried forward — the prerequisite arc's baseline, which rank 3's number is read
  against.** Measured 2026-09-19 over 18 trials: the three cheap models
  (`z-ai/glm-5.3-flash`, `xiaomi/mimo-v2.5`, `deepseek/deepseek-v4-pro`) all reach SETUP on
  4 of 6 grid rows, model-independently, at **mean $0.0058/trial** (21 trials = $0.1211),
  with the frontier control **5–20× dearer**. So this arc's app-build trial at **$0.0145**
  is ~2.5× a setup trial, not the 10–30× the scoping round assumed.
- **Carried forward — the credentialed precedent, if the arc ever needs auth.**
  `claudedocs/handoff-dogfood-3.md` is the *first CREDENTIALED blind dogfood* (2026-08-10).
  This arc deliberately does NOT need it: `app validate` and `app init` are local, and
  `app doctor` is the auth-gated one that stays off the trial path.

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

## Next steps (ranked)

🔴 **NUMBERING IS FROZEN — `app-build-dogfood-3` and `-5` are LIVE claim slugs and a
renumber would silently re-point them.** Rank 4 is therefore marked deleted IN PLACE rather
than closed up. Ranks 1–3 are done; do not re-take them.

⚠ **Carried forward from round 0 — on `forcing:` below, read this before quoting it.** The
operator asked for the ARC ("we want to build apps", 2026-09-20). He did **not** ask for
any individual item here; the decomposition is agent-authored. An earlier draft tagged five
items *"the operator asked for this measurement"*, which was false of every one of them.
The tag names the arc, which is the true forcing function, and this paragraph is the record
that the breakdown is not itself user-requested.

1. ✅ **DONE — brief-injection path.** Shipped in `cli#678`.
   forcing: user — satisfied
2. ✅ **DONE — the brief and its behavioural assertion.** Shipped in `cli#678`, with the
   scaffold control failing on all three templates.
   forcing: user — satisfied
3. ✅ **DONE — the step/cost curve.** Measured 2026-09-21; see the investigation block.
   forcing: user — satisfied
4. ❌ **DELETED, NOT DEFERRED — history management in `runner.py` is NOT required.**
   Rank 3 measured the curve this item was conditional on: 65 steps and $0.0145 against
   caps of 80 and $0.50. The item existed only "if rank 3 says so", and rank 3 says no.
   Recorded rather than silently dropped so nobody re-derives the O(n²) worry from the
   superseded block and rebuilds it.
   forcing: gate — retired; the condition it was gated on measured NO
5. ✅ **DONE — the render oracle is built and BOTH controls are measured.** Shipped in
   `cli#681` (squash `9d7b5a2a`): `scripts/dogfood/oracle.sh` + `serve-block.mjs`, wired
   into `grade.sh`, with `civitai app validate` demoted to a reported fail-fast GATE.
   **Positive control** — re-run independently against the live specimen, not taken from a
   report: `gate=pass gate_rc=0 observed=212 RENDER=yes`, i.e. a real browser typed `100`
   into the model-authored app and read back exactly `212`.
   🔴 **Negative control, and it is the arc's whole thesis as an observed fact:** an
   untouched `static` scaffold grades **`gate=pass … RENDER=no`** — the validator passes
   while the browser says nothing rendered. That disagreement is why the verdict is the
   browser. Operating detail: `scripts/dogfood/README.md`.
   forcing: user — satisfied
6. **Run the matrix** on `glm-5.3-flash`, `mimo-v2.5`, `deepseek-v4-pro` plus a frontier
   control, on the **2 writable-prefix environments**. Report per-cell, and report the
   environments that could NOT be run.
   ⚠ **Budget, now grounded rather than guessed:** one cheap-model trial measured
   **$0.0145**. A 3-model × 2-env matrix is therefore ~$0.09, plus a frontier control at
   5–20×. The cost objection to this arc is effectively gone.
   forcing: user — the operator asked for this arc on 2026-09-20

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

## How to verify

**Rank 3's curve — re-derive from the transcript, do not trust the table above:**

```bash
TRIAL=runs/ab-curve-01/transcript.jsonl   # under scripts/dogfood/
python3 - "$TRIAL" <<'PY'
import json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
end = [r for r in rows if r.get("kind") == "end"][-1]
print("stop:", end["stop"], "steps:", end["steps"], "usage:", end["usage"])
a = [r for r in rows if r.get("kind") == "assistant" and r.get("usage")]
print("first per-call prompt:", a[0]["usage"]["prompt_tokens"])
print("last  per-call prompt:", a[-1]["usage"]["prompt_tokens"])
PY
```
🔴 The per-step numbers live on `assistant` records, NOT on a `step` kind — there is no
`step` record and a scan for one returns a confident empty curve.

**The brief actually reaches the model — free, spends nothing:**

```bash
cd scripts/dogfood
python3 runner.py --print-task                                           # the bare setup task
python3 runner.py --print-task --brief "$(cat briefs/celsius.brief.txt)" # URL + blank line + brief, raw
```
The first must be byte-identical to the task every prior setup trial received.

**What the specimen actually contains (while it lives):**

```bash
docker exec dogfood-ab-curve-01 sh -c 'grep -rn "data-testid" /work/celsius-converter/src'
```
🔴 This is SOURCE INSPECTION and is **not** the closing condition. The verdict is rank 5's
headless assertion against a RUNNING block.
