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

🔴 **THE RESULT STANDS AND IS UNCHANGED: a cheap open model built a PASSING app.**
`xiaomi/mimo-v2.5`, blind, 73 steps, **$0.0237** — `brief=genpost brief_source=transcript-text
gate=pass viewer=signed-in observed=ready>generating RENDER=yes`, re-run independently, with
the negative control (unmodified `page-money` scaffold) still `gate=pass … RENDER=no`.
**The frozen condition is still NOT met** — it requires *"three cheap models with a frontier
control"* and we have **2 of 4 cells on 1 environment**. Rank 6 is the only item between
here and a gradeable verdict.

- ✅ **Six PRs merged in `civitai/cli`**, each verified by content with a negative control:
  `#683` `8603fc3` (credential path + `genpost` brief) · `#684` `7151ca3` (a truncated trial
  can no longer read as `finished`) · `#685` `bdeddef1` (agent-facing doc fixes) ·
  `#686` `0af4045` (oracle brief resolution + signed-in viewer) · `#682` `cadc69bb` (this
  doc). Plus `#679` `336eb9ed` earlier, which unblocked the repo.
- 🔶 **TWO PRs OPEN, and the merge ORDER matters — `#688` must land first.**
  - **`#688`** (`f9a88345`, `automation/bump-scaffold-pins`) — **13/13 green, 0 non-success,
    `CLEAN`.** Bumps pins `^0.48.0`/`^0.55.0` → `^0.49.0`/`^0.56.0` across all four literal
    sites. **Merge this first.**
  - **`#687`** (`34764e99`, `fix/ci-browser-launch`) — 13/13 completed, and its **only**
    non-success is `pins-vs-published`, i.e. the stale pins `#688` fixes. `BLOCKED` solely
    on that. Rebase onto the greened `main`, then merge.
- 🔴 **`main` IS RED RIGHT NOW** — `build-test failure` on `cadc69bb`, a **docs-only**
  commit. Fifth occurrence of the Chromium launch flake. `#687` is the fix; see the
  investigation block for what it can and cannot claim.
- 🔴 **`scaffold-currency` PASSED on `#688`, and that is the load-bearing fact about the
  pin bump.** That job installs `@latest` explicitly, so it reds on a **broken export**
  rather than a stale caret. Its green is the evidence that `blocks-react@0.56.0` did NOT
  move or remove anything the templates import — unlike `0.55.0`, which relocated
  `createLiveHost` to `/live` and forced a seven-file migration (`#679`). **Nothing else in
  the toolchain answers that question before merge.**
- 🔴 **A CHEAP, CONCRETE IMPROVEMENT FOUND BY ACCIDENT: a `workflow_dispatch` bump PR GETS
  CHECKS; the cron one does NOT.** `#676` (cron-authored, this morning) merged `CLEAN` with
  its checks never having run and left `main` red for a day —
  `.github/workflows/bump-scaffold-pins.yml:232-239` documents the loop-guard. `#688`,
  triggered by hand via `gh workflow run`, got all 13. **Making the nightly sweep dispatch
  its own validation would close the hole that bit us this morning.**
- **Account unchanged across both credentialed trials:** Buzz **4,101,822 → 4,101,822**
  (delta **0**), 13 listings, no `ab-*` reached the account, none of the ~10 operator-owned
  apps touched. `generations=0 submissions=0` in both runs.
- ⚠ **No `clawgate-task:` field** — `clawgate_handoff.sh resolve` exited **5**. An unknown
  session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id". Not a clean bill of health.

### Carried forward — durable values a `State now` replace would otherwise eat

🔴 **RANK 3'S TABLE, kept because an appended block points AT it** ("✅ ANSWERED 2026-09-21
— the harness CAN carry an app-build task" says *"see the State-now table"*). This is the
**third** update in which deleting it would have left that pointer dangling:

| | measured | cap set |
|---|---|---|
| steps | **65** | 80 |
| total cost | **$0.0145** | $0.50 |
| prompt tokens | 1,564,496 | — |
| per-call prompt | 452 → **45,328** | — |
| `stop` | **`finished`** | — |

🔴 **THE PRE-RUN BASELINE — cannot be re-derived after the fact.** Buzz **Blue 1,305,157 ·
Green 947,618 · Yellow 1,849,047 · TOTAL 4,101,822**; **13** listings. Spend is graded as a
DELTA against this, never from an agent's report.

**The prerequisite arc's baseline:** 18 trials, 2026-09-19, three cheap models reach SETUP
on 4 of 6 grid rows, model-independently, **mean $0.0058/trial**, frontier control **5–20×
dearer**. A genpost app-build cell at $0.0237 is ~4× a setup trial.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — 3 generations, 61 Buzz,
2 submissions, verified from the account (4,187,454 → 4,187,393). Records real
`submit`/`withdraw` defects incl. **permanent loss of captioned screenshots**.

**The three specimen containers are regression fixtures — do not destroy:**
`dogfood-ab-curve-01` (celsius `RENDER=yes observed=212`), `dogfood-ab-genpost-mimo-01`
(genpost **`RENDER=yes`**), `dogfood-ab-genpost-glm-01` (unmodified scaffold `RENDER=no`,
the negative control). 🔴 `runs/ab-curve-01/transcript.jsonl` was **destroyed by a routine
worktree cleanup** — copy `runs/<trial>/` out before removing a worktree.

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

### ✅ EXPLAINED 2026-09-21 — glm never wrote code, and "finished" was a truncation
- as-of: 2026-09-21

🔴 **This CLOSES the question "why did the genpost trial fail", and the answer changes what
the arc concluded.** Two defects compounded — one model-side, one harness-side.

- **Observed (with values), the harness defect:** the final assistant record carried
  `content: null`, `tool_calls: []`, `completion_tokens: 8000` — **exactly** the
  `max_tokens` the harness sent — of which **7,992 were reasoning**. `runner.py` branched on
  absence of tool calls alone and **never read `finish_reason`** (it appeared nowhere in
  `runner.py`, `grade.sh` or `oracle.sh`). So a cell that ran out of output budget was
  indistinguishable from one that completed with an empty report. **Fixed in `cli#684`**;
  the stop vocabulary is now `finished` / `truncated` / `empty-reply` /
  `stopped-unknown:<value>`, and an unknown value deliberately does NOT become `finished`.
  `via: measurement` (the transcript's own usage block)
- **Observed, the model-side fact:** **zero writes across 66 steps**, established two
  independent ways — a regex over all 65 commands (no `cat >`, `tee`, `sed -i`, redirect
  into a file) and the container filesystem (every `src/` file carrying the scaffold's
  creation mtime). It engaged the brief — it researched `useCreatePostFromApp`, `scopes`,
  `useRequestConsent`, `Textarea`, `Button` in order — and never implemented any of it.
  `via: measurement`
- **Ruled out — that it was blocked.** 1 of 65 commands exited non-zero (a `civitai
  --version` before install). It hit one real blocker, a scaffold missing
  `@testing-library/dom`, and **fixed it at step 32**, then read for 34 more steps.
  `via: measurement`
- **Ruled out — that the brief was evicted from context.** The harness never prunes; the
  brief was `messages[1]` on all 67 requests. It was **diluted 1:800 by tool output**, not
  evicted. `via: code` + `via: measurement`
- **Ruled out — that documentation volume caused it.** Docs were **13.1%** of bytes read;
  our own scaffold source and `node_modules` were **68.9% of carry cost**. `MAX_OUT`
  crowding was not the mechanism (4 of 65 commands hit the cap). `via: measurement`
- 🔴 **REFUTED BY EXPERIMENT — that the doc gap caused the failure.** `mimo-v2.5`, given the
  **identical** brief, scaffold, harness and unfixed docs, started writing at step 40 and
  produced a passing app. **The doc stack is a COST problem, not the failure cause.**
  `via: measurement` (the mimo trial)
- **Leading hypothesis for the remaining gap:** a planning failure specific to glm —
  87.8% of its completion tokens went to reasoning the harness then **discarded** between
  turns, so it re-derived its situation 67 times. `cli#684` now carries `reasoning_details`
  back. **NOT ESTABLISHED**: the transcript stores no reasoning text, and proving it needs a
  re-run on the fixed harness (~$0.09).
- **Next probe:** none required for this arc. If anyone wants the planning hypothesis
  settled, re-run glm on the fixed harness and compare.

### ⚠ OPEN — the documentation stack is a measured cost problem
- as-of: 2026-09-21

- **Observed (with values):** carry cost = bytes × steps remaining, because the harness
  resends the whole history each turn. Ranked that way over glm's run: scaffold source
  **40.7%**, `node_modules` **28.2%**, scaffold README **12.8%**, the hosted prompt **5.1%**
  (4th-highest single read, because it is first and is resent 66 times).
- **The single largest attributable waste:** `useCreatePostFromApp` was documented **nowhere**
  in the local stack — not the 40 KB scaffold README, not `AGENTS.md`, not the hosted prompt,
  not 5,908 lines of scaffold source. Reverse-engineering it from `node_modules` cost
  **706,371 carry tokens, 28.2% of the run**. **Fixed in `cli#685`** — a complete 36-hook
  index now lands at byte 388 of the README, single-sourced from `internal/scaffold/hooks.go`
  with a guard that fails in both directions.
- **Also fixed in `cli#685`:** `AGENTS.md` was stale for **58 of 66 steps** (`app create`
  never re-ran `agent-setup`, so it said "No Civitai App has been scaffolded"); the docs
  links sat at 90% depth as a bare list and the agent made **exactly one HTTP request in 66
  steps**; `--template` was absent from the commands table while the default is the 37-file
  `page-money`.
- 🔴 **A briefing error of mine, corrected by measurement:** I named
  `hostHandlerParity.ts` as the source for the mock-host capability table. It covers the
  three REAL hosts, not `createMockHost`. The published SDK answers directly — and
  `CREATE_POST_FROM_APP` **is** mocked, which is what glm spent ~380,000 carry tokens
  discovering by reading `mockHost.js` line by line.
- **Still open:** 96.3% of glm's prompt tokens were **cache reads**, so quoting 2.5M as
  fresh overstates it; and ~4 of the 6.3× cost gap between the runs is **model price**, not
  tokens. The doc stack owns the 1.6× volume difference, not the 6.3×.
- **Next probe:** re-measure carry cost on a post-`cli#685` trial to price the fix. Free if
  folded into rank 6.

### ⚠ OPEN — the CI Chromium launch flake: a fix is proposed but the stall was never reproduced
- as-of: 2026-09-21

- **Symptom + exact repro:** `build-test` fails with **no failing test name at all**.
  Measured on five runs — `35557830290`, `35563154222`, `35567753483`, `35616180491`, and
  `main`'s current `cadc69bb` — across `main` itself and three PRs including a **docs-only**
  one. In a 31,141-byte log there are **zero `--- FAIL:` lines** and every other package
  reports `ok`:
  ```
  {"assertion":"celsius","pass":false,"reason":"harness error: browser never printed a
   DevTools endpoint:\n[ERROR:dbus/bus.cc:405] Failed to connect to the bus: Could not
   parse server address: Unknown address type ..."}
  oracle: the assertion could not run (exit 2) — nothing was measured (this is NOT a failing trial)
  FAIL github.com/civitai/cli
  ```
- 🔴 **The oracle behaves CORRECTLY** — it exits 2 and says *nothing was measured* rather
  than emitting a false `RENDER=no`. The job fails because the render-oracle tests
  deliberately refuse to skip under `$CI`. **Do not "fix" this by loosening them to a
  skip**; a skip here is a green that checked nothing, and `ci.yml` forbids it in-line.
- **Observed (with values) — the number that did not exist until `#687`'s smoke step
  shipped:** a healthy cold launch on that runner is **`[cdp] browser launched in 3546ms`**
  against a **15,000 ms** budget — ~4× headroom on a machine with **3.4× run-to-run
  spread**. And the 15 s was never chosen for *launching*: it is the wait for a React tree
  to mount, reused for an unrelated quantity.
- **Ruled out — that clearing `DBUS_SESSION_BUS_ADDRESS` is the mechanism.** Null result.
  `via: measurement`
- **Ruled out — that the error string alone reproduces it.** `DBUS_SESSION_BUS_ADDRESS=bogus:path=/nope`
  reproduces the CI stderr **byte for byte** and Chromium still reaches a DevTools endpoint
  in ~110 ms. `via: measurement`
- **Ruled out — that `chromium --version` latency predicts failure.** 1.9–6.5 s in failing
  jobs vs 0.02 s locally, but 2.2 s passed and 2.8 s failed — an environment speed class,
  not a predictor. `via: measurement`
- **Leading hypothesis:** a thin timeout against a slow cold launch, plus one blocking
  session-bus round trip. `--password-store=basic` removes exactly one (`4 dbus errors → 3`),
  and **three of four failures stalled after exactly three**.
- 🔴 **Counter-evidence, stated rather than buried: the FOURTH failure stalled after ONE
  dbus error**, which that flag cannot explain. So the fix is **"rarer, not provably gone"**
  — the author's own words, and the honest position.
- **Next probe:** none locally; the stall was never reproduced. **The closing condition is
  MECHANICAL and needs nobody to remember it** — a retried launch prints
  `BROWSER LAUNCH RETRY`, which `ci.yml` renders as a `::warning` on the job. **If that
  warning appears in `build-test` after `#687` merges, the launch is still sick.**

## Next steps (ranked)

🔴 **Numbering frozen.** Ranks 1–5 settled; 4 deleted by measurement.

⚠ **On `forcing:` — the operator asked for the ARC, not for individual items; the
decomposition is agent-authored. Rank 7 is the exception (asked for in those words on
2026-09-21).**

1. ✅ **DONE** — brief-injection path (`cli#678`). forcing: user — satisfied
2. ✅ **DONE** — the celsius brief and assertion (`cli#678`). forcing: user — satisfied
3. ✅ **DONE** — the step/cost curve. forcing: user — satisfied
4. ❌ **DELETED** — history management not required; rank 3 measured it. forcing: gate — retired
5. ✅ **DONE** — the render oracle (`cli#681`) + two defect fixes (`cli#686`).
   forcing: user — satisfied
6. 🔴 **THE ONLY ITEM BEFORE THE FROZEN CONDITION CAN BE GRADED — run the matrix.**
   Outstanding: **`deepseek-v4-pro`** and the **frontier control**, on the genpost brief.
   Cheap cells measured $0.0145–$0.0918, so ~$0.05 for the remaining cheap cell, 5–20× for
   the frontier. ⚠ `cli#665` still blocks 2 of the 4 environments. ⚠ Re-measure carry cost
   while running, to price `cli#685`.
   forcing: user — the operator asked for this arc on 2026-09-20
7. 🔶 **PARTIALLY DONE — the credentialed run happened; NOTHING WAS SUBMITTED.**
   `generations=0 submissions=0` in both trials. The app mimo built passes the brief and was
   never submitted. 🔴 **`app submit` is OUTSIDE the frozen condition** — closing rank 6 does
   not close this, and closing this does not close rank 6. Decide explicitly whether a
   submit is still wanted before spending another credentialed cell.
   forcing: user — the operator asked for this on 2026-09-21
8. **Merge `#688`, then rebase and merge `#687`** — in that order. `#688` is green;
   `#687`'s only red is the stale pins `#688` fixes. Until both land, `main` stays red on
   `build-test` and every PR in the repo looks unmergeable.
   forcing: gate — `main` is red on a required check

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

### Added 2026-09-21 — the instrument was wrong more often than the models were

- 🔴 **I GRADED AN APP AGAINST THE WRONG ASSERTION AND NEARLY REPORTED IT AS THE RESULT.**
  `oracle.sh` took the brief as an **optional third positional arg defaulting to `celsius`**.
  Run against a genpost trial without that arg, it ran the celsius assertion, timed out on
  `[data-testid="celsius"]`, and printed `RENDER=no`. The app was correct the whole time.
  **Fixed in `cli#686`:** the brief is now derived from the trial's own transcript, and an
  argument that DISAGREES with it is refused rather than silently preferred — a disagreement
  means someone is confused and a verdict either way is worthless.
- 🔴 **A NEGATIVE CONTROL WAS PASSING VACUOUSLY, AND THE MECHANISM IS REUSABLE.** In the
  oracle's own tests, `stubOracleEnv` built fixtures under `t.TempDir()`, whose path carries
  the **test name**, which was spliced into an unquoted `find /work …`. A subtest name with a
  shell metacharacter produced a syntax error → `manifests=0` → `RENDER=no` — **the expected
  value of every negative case**. So the control passed for entirely the wrong reason. Found
  and fixed inside `cli#686`. **Ask what value a broken harness returns; if it equals your
  expected failure value, the control proves nothing.**
- 🔴 **AN ORACLE'S ENVIRONMENT IS PART OF ITS VERDICT.** Seeding no signed-in viewer made
  every auth-gated app fail on a branch production never exhibits — civitai's block-scope
  middleware **hard-rejects** `posts:write:self` for an anonymous subject, so the brief's
  behaviour cannot exist there. `cli#686` seeds a viewer shaped byte-for-byte like
  production's `withSignedInFlag()`, while the stub transport still rejects every request —
  asserted from inside the page, not by reading source, so "nothing can complete here"
  remains true.
- 🔴 **A ROUTINE WORKTREE CLEANUP DESTROYED THE CONTROL RUN'S TRANSCRIPT.** `runs/` lived
  inside the worktree; `git worktree remove` took it. Two separate analyses then had no
  per-step data for the one successful run they most needed to compare against. **Copy
  `runs/<trial>/` out before removing a worktree** — the container survives a cleanup, the
  transcript does not.
- ⚠ **A WATCHDOG GREP OVER A WHOLE JSON RECORD RAISES FALSE ALARMS FROM DOCUMENTATION.** My
  spend monitor matched `civitai generate|app submit` anywhere in a record, and fired four
  times on the CLI's **own help text and an `AGENTS.md` table** appearing in tool *results*.
  The authoritative signal is the runner's own `generations`/`submissions` counters, which
  are enforced before `docker exec`. **Match the command field, not the record.**
- ⚠ **`main` is intermittently red on a Chromium launch flake** (`dbus` address failure →
  `browser never printed a DevTools endpoint`). The oracle correctly reports `exit 2 —
  nothing was measured (this is NOT a failing trial)`, and `build-test` fails anyway because
  the tests refuse to skip under CI. Seen on at least three runs; clears on re-run. The CI
  config asks for the fix by name (*"Add a browser install step here (the tests themselves
  must NOT be loosened into a skip)"*). **Unfixed, and it will keep training people to merge
  through a required check.**

### Added 2026-09-21 (late) — the pin treadmill, and a control that was not one

- 🔴 **UPSTREAM PUBLISHES FASTER THAN THE SWEEP, AND IT BIT THREE TIMES IN ONE DAY.** npm
  published `app-sdk@0.49.0` / `blocks-react@0.56.0` at **15:33:32Z** — after `main`'s green
  pins run (15:19:22Z) and before `#687`'s (15:40:50Z). Nothing anyone did caused it.
  `bump-scaffold-pins.yml:64-69` already measured the cause: **six minors in 19 days, a
  ~3.6-day mean** against what was then a 7-day sweep. They moved to daily; daily is still
  slower than upstream. **Every window between a publish and the next sweep reds every open
  PR on a required check for reasons unrelated to its content.**
- 🔴 **MY NEGATIVE CONTROL WAS MUDDIED BY THE ARC'S OWN EARLIER TEXT — THIRD TIME TODAY.**
  Verifying `#682` I grepped `RENDER=yes` and got **2 hits at `main~1`**, because the
  celsius specimen already graded that way. The discriminating string was
  `ready>generating` (**2 on main, 0 at `main~1`**). **Pick a string the NEW content
  introduces, never the identifier the work has been discussing all along.** Same shape as
  the `local/share/opencode/auth.json` path count earlier in the day.
- ⚠ **A `count=1` replace in a mutation sweep can hit a DOC COMMENT instead of the code.**
  `#687`'s author had a mutant report SURVIVED for exactly that reason; re-run correctly it
  was killed. **A survivor is a claim about the sweep before it is a claim about the guard.**
- ⚠ **`gh api … check-runs | jq` can die on control characters** in a check's output
  (`Invalid string: control characters from U+0000 through U+001F must be escaped`). Use
  `gh api --jq` (server-side) rather than piping into `jq`. A failed parse printed empty
  counts beside a reassuring echo — read the counts, not the banner.

## How to verify

**The headline result — free, no trial, re-grade the specimen:**

```bash
CLI=/home/zach/workspace/civit/cli
(cd "$CLI/scripts/dogfood" && nix-shell -p chromium --run \
   'CIVITAI_CHROME=$(command -v chromium) bash oracle.sh ab-genpost-mimo-01 root')
```
Expect `brief=genpost brief_source=transcript-text … observed=ready>generating RENDER=yes`.
🔴 **Always re-run the NEGATIVE control beside it** — `ab-genpost-glm-01` must stay
`RENDER=no` with `gate=pass`. A change that makes both pass has broken the oracle.
⚠ The oracle now DERIVES the brief from `runs/<trial>/transcript.jsonl`; that directory must
be present or it refuses rather than defaulting.

**The account — the only trustworthy evidence about spend:**

```bash
civitai buzz      # TOTAL must equal 4101822 unless a generation was intended
civitai app list  # 13 listings; every pre-existing slug unchanged; no ab-* present
```

**The merge order, and why `main` is red:**

```bash
gh pr view 688 --repo civitai/cli --json mergeStateStatus   # green -> merge FIRST
gh pr view 687 --repo civitai/cli --json mergeStateStatus   # blocked only by stale pins
gh api repos/civitai/cli/commits/$(git -C "$CLI" rev-parse origin/main)/check-runs \
  --jq '.check_runs[]|select(.conclusion=="failure")|.name'
```
🔴 **Attribute any `build-test` red by FAILING TEST NAME, never by the verdict.** The flake
produces **zero** `--- FAIL:` lines; if you see named tests, it is not the flake. And assert
the log is non-empty before believing any grep over it — `gh api … /logs` can return 0 bytes,
which greps as "no failures".
## Defects (batched)

- `bump-scaffold-pins.yml` validates `pins-vs-published` in-job but has **no in-job
  equivalent for `scaffold-currency`** — the half that actually broke on `#676`. A pin bump
  cannot catch a moved export; only `scaffold-currency` can, and it does not run on a
  cron-authored bump PR.
- The nightly sweep's PRs do not trigger checks (GitHub loop-guard); a hand `workflow_dispatch`
  does. Making the sweep dispatch its own validation closes it.
- `cli#665` open — blocks 2 of the 4 trial environments.
