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

- **Nothing is built. This doc is the scope**, produced 2026-09-20 from a read-only
  inspection of the existing rig. No harness change, no trial, no measurement yet.
- **Relationship to `agent-setup-onboarding`:** that arc is NOT superseded and NOT closed.
  Its condition (setup reachability) is a strict PREREQUISITE of this one — an agent that
  cannot get `civitai` on PATH cannot build anything. Its rank 34 / `cli#665` is therefore
  a live dependency here: **`node-user` and `ubuntu-apt` fail 3/3 for all three models**,
  because agents install to `~/.npm-global` which no login shell adds. Until that lands,
  this arc can only be graded on the writable-prefix environments.
  🔴 **That is 2 of 4 ENVIRONMENTS — 50%, not 2 of 6 — and an earlier draft got this wrong
  in the flattering direction.** The prerequisite arc's grid is **6 ROWS over 4 distinct
  envs**: `node-root` appears three times under three agent identities (claude / codex /
  other), plus `stale-cli`, `node-user`, `ubuntu-apt`. `scripts/dogfood/envs/` holds
  exactly **4** Dockerfiles. So the matrix rank 6 can actually run is **2 environments**,
  not 4, and `cli#665` blocks half of them. Agent identity is very likely irrelevant to an
  app-build grade, so the 6-row denominator should not be carried into this arc at all.
- **What the prerequisite arc already established, and it is load-bearing here** (measured
  2026-09-19, 18 trials): the three cheap models — `z-ai/glm-5.3-flash`,
  `xiaomi/mimo-v2.5`, `deepseek/deepseek-v4-pro` — all reach setup on 4 of 6 envs,
  model-independently, at **mean $0.0058/trial** (21 trials = $0.1211). Gemini was **5–20×
  dearer**. The capability confound was ruled out by measurement, not assumed: all 6
  failures stopped `"finished"`, hit `EACCES` 5–7× and attempted prefix remedies 8–14×.
- **What is reusable, measured by reading it:** `scripts/dogfood/runner.py` (225 lines,
  OpenRouter tool-loop, blind by mount namespace not by instruction), `driver.sh` (115),
  `grade.sh` (117), and 4 container envs (`node-root`, `node-user`, `stale-cli`,
  `ubuntu-apt`).
- **The CLI already has the whole app surface** — `app init`, `app create`, `app validate`,
  `app doctor`, `app dev-tunnel`, `app submit`, `app status`, `app pull`, `app listing`.
  🔴 **Corrected after round 0 — an earlier draft of this bullet cited a symbol COUNT and
  the count was measuring the wrong thing.** It read *"a grep for auth/network symbols
  returns 1 hit in `app_validate.go` and 3 each in `app_init.go`/`app_doctor.go"*; that
  tuple is not reproducible with any single pattern, and **every one of `app_doctor.go`'s
  `auth` matches is the substring inside "author"/"authorized"** in comments and help text.
  Read instead of counted, the real picture is: **`app validate` is local** (imports
  `fmt io ui validate cobra` — no client, no token), **`app init` has no client or token
  reference**, and 🔴 **`app doctor` IS auth-gated and network-bound** —
  `app_doctor.go:450-451` returns `ErrUnauthorized` when `cfg.Token() == ""`, then calls
  `newListingClient()` / `ListMyListings(...)` at `:453-461`. So an **offline,
  unauthenticated loop holds for the `{init, validate}` subset the closing condition
  actually uses, and NOT for `app doctor`** — do not put `doctor` on the trial path.
  ⚠ **And do not "confirm" this by denying the container network**: `runner.py:151-152`
  states egress is necessarily open because the trial must fetch `prompt.md` and reach npm,
  and the agent's whole task IS that URL (`runner.py:43`). A network-denied container
  cannot start a trial at all. Confirm by reading the command's code, as above.
- **Precedent for the credentialed variant if it is ever wanted:**
  `claudedocs/handoff-dogfood-3.md` is the *first CREDENTIALED blind dogfood* (2026-08-10).
  This arc deliberately does NOT need it.

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

## Next steps (ranked)

🔴 **REORDERED AND CUT after round 0 (`cli#677`), before any claim existed against this
list — so no `claim-work` slug was re-pointed.** Round 0 found that the old rank 1 could
not be executed at all (there is no way to give the agent a brief), and that two ranked
items only restated text the doc already carries. Old ranks 6 and 7 were DELETED, not
renumbered: 6's dependency is still stated twice below and in "State now", and 7 belongs to
the prerequisite arc, which already carries it as its own Next probe.

⚠ **On `forcing:` below — read this before quoting it.** The operator asked for the ARC
("we want to build apps", 2026-09-20). He did **not** ask for any individual item here; the
decomposition is agent-authored. An earlier draft tagged five items
*"the operator asked for this measurement"*, which was false of every one of them. The tag
now names the arc, which is the true forcing function, and this paragraph is the record
that the breakdown is not itself user-requested.

1. **Give `runner.py` a way to receive the app brief — nothing can run until this exists.**
   `runner.py:43` is a module constant, `USER = """<the hosted prompt URL>"""`, and the
   flag list at `:116-131` (`--model --image --trial --user --agent-env --max-steps
   --max-cost --out`) has **no brief argument**. The closing condition requires the agent
   be given the URL *and a one-line brief*; today there is no app-build trial to run, only
   a setup trial. Add the injection path and keep the blind-by-mount-namespace property.
   forcing: user — the operator asked for this arc on 2026-09-20
2. **Write the app brief and its behavioural assertion.** One line the agent receives
   alongside the hosted URL, naming a behaviour an untouched `app init` scaffold does NOT
   have, and a single deterministic assertion the browser can make about the rendered
   block. 🔴 Write the assertion FIRST and check the bare scaffold FAILS it — an assertion
   the scaffold already satisfies makes every cell green while measuring nothing.
   forcing: user — the operator asked for this arc on 2026-09-20
3. **Run ONE app-build trial and read the step/cost curve** — see the investigation block.
   Cheapest model, writable-prefix env, `--max-steps 80 --max-cost 2.0`. ⚠ **This is a
   run-and-analyse task, NOT an instrumentation one**: `runner.py:190` already records
   per-call `usage` into `transcript.jsonl`, so prompt tokens and cost per step are
   captured today. The curve decides whether rank 4 is required.
   forcing: user — the operator asked for this arc on 2026-09-20
4. **Give `runner.py` history management** if rank 3 says so — bound cumulative prompt
   growth, not just per-command output.
   forcing: user — the operator asked for this arc on 2026-09-20
5. **Build the render oracle**: serve the built block inside the trial container and drive
   it headless, asserting the rank-2 behaviour. `civitai app validate` runs first as a
   fail-fast gate and is reported, never as the verdict. 🔴 **Related tooling that may serve
   as the driver rather than being rebuilt lives in ANOTHER repo** —
   `<talos-infra>/.claude/skills/app-blocks/` and `<talos-infra>/.claude/skills/app-capture/`.
   An earlier draft pointed at `~/.claude/skills/...`, where **neither exists**; a session
   following that finds nothing and rebuilds a headless driver it did not need to.
   forcing: user — the operator asked for this arc on 2026-09-20
6. **Run the matrix** on `glm-5.3-flash`, `mimo-v2.5`, `deepseek-v4-pro` plus a frontier
   control, on the **2 writable-prefix environments** (see the corrected denominator in
   "State now"). Report per-cell, and report the environments that could NOT be run.
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

## How to verify

**The closing condition, once ranks 1–6 exist** — the verdict is the browser, the
validator is only a gate. 🔴 **`--model` and `--image` are `required=True`
(`runner.py:116-117`); an earlier draft omitted both, so the command argparse-failed with
exit 2 before doing anything.** And every placeholder is a `$VAR`, never `<name>`: inside a
```bash fence `<` and `>` are shell REDIRECTIONS, so a pasted `<cell>` makes the shell read
a file named `cell` and the command never runs.

```bash
CLI=/home/zach/workspace/civit/cli
MODEL=z-ai/glm-5.3-flash ; IMAGE=dogfood-node-root ; TRIAL=cell-01 ; CUSER=root
# per cell: blind trial -> built block -> gate -> render assertion
(cd "$CLI" && python3 scripts/dogfood/runner.py \
   --model "$MODEL" --image "$IMAGE" --trial "$TRIAL" \
   --max-steps 80 --max-cost 2.0)
# grade.sh takes a trial-id then a container-user; the 2nd arg defaults to root, which is
# WRONG the moment node-user becomes gradeable — pass it explicitly.
(cd "$CLI" && bash scripts/dogfood/grade.sh "$TRIAL" "$CUSER")
```

**Before trusting any green cell, run both controls — the rig already has this shape and
it is what made the prerequisite arc's numbers believable:**
- **negative control**: a bare container that cannot possibly succeed must grade `no`.
- **positive control**: a hand-built, known-good app must grade `yes`. 🔴 Until the
  positive control has been watched to pass, a `no` across the matrix is a claim about the
  harness, not about the models.
- 🔴 **Scaffold control (specific to this arc):** an untouched `civitai app init` scaffold
  must FAIL the render assertion. If it passes, the assertion measures nothing and every
  cell is vacuously green.

**The prerequisite that gates 2 of the 4 environments:**

```bash
gh issue view 665 --repo civitai/cli --json state --jq .state   # OPEN = node-user/ubuntu-apt ungradeable
```
