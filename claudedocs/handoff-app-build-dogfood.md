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

🔴 **THE ARC IS CLOSED ON ITS CHEAP ARM, AND THE CLOSING VERDICT IS REPRODUCIBLE FROM
`main`.** `cli#690` (**`4e6214f`**) seeds the oracle's `token.scopes` from the block's own
manifest, so the instrument no longer grades a correct app `no`. All three live arms
re-run by a second party from a **clean checkout of the merge SHA**, not from the
implementing agent's tree:

```
ab-genpost-dsv4-01  deepseek  scopes=ai:write:budgeted,posts:write:self  observed=ready>generating>ready  RENDER=yes
ab-genpost-glm-01   scaffold  scopes=ai:write:budgeted                   observed=''                      RENDER=no   ← negative control holds
ab-genpost-mimo-01  mimo      scopes=ai:write:budgeted,posts:write:self  observed=ready>generating>ready  RENDER=yes
```

🔴 **VERDICT — `check`, answered: YES on the cheap arm, with ONE stated gap.**
**2 of 3 cheap open models** (`mimo-v2.5`, `deepseek-v4-pro`) took only the hosted prompt
URL plus a one-line brief, on a machine that did not build the CLI, and produced an App
Block a headless browser watched exhibit the specified behaviour. `glm-5.3-flash` failed,
explained: it wrote **zero** code in 66 steps and its `finished` was a truncation.

🔴 **THE GAP, RECORDED RATHER THAN ROUNDED AWAY — OPERATOR DECISION 2026-09-21: the
frontier control was DROPPED, and the condition is closed WITHOUT it.** The frozen text
reads *"Graded per cell across the three cheap models, **with a frontier control**"*. The
control arm was never run and is **not outstanding** — the operator decided on 2026-09-21
to close on the cheap arm and record the omission as a permanent limitation on the verdict.
**A later session must NOT "helpfully" run it to tidy the arc up**; re-opening it is a
deliberate new decision, and costs one trial.

- ⚠ **SECOND LIMITATION ON THE VERDICT: every cell is ONE environment.** `cli#665` is still
  OPEN and blocks 2 of the 4 trial environments, so *"2 of 3 models"* is a claim about
  `df-node-root` and nothing wider. Confirmed open 2026-09-21.
- ✅ **Four PRs merged this session**, each verified by content, not by ancestry
  (`--is-ancestor` is permanently false after a squash): **`#688`** `ef411fa` (pins
  `^0.49.0`/`^0.56.0`) · **`#687`** `3ffebca` (browser launch as its own failure domain) ·
  **`#689`** `91a93fc` (handoff) · **`#690`** `4e6214f` (the oracle scope seed).
- ✅ **`main` was red on the Chromium flake for five consecutive runs and is green again**
  from `3ffebca` onward. `#687`'s mechanical closing signal — a `BROWSER LAUNCH RETRY`
  `::warning` on `build-test` — **has not fired** across every launch since. 🔴 Still
  *"rarer, not provably gone"*: the condition stays armed and costs nothing.
- 🔴 **ACCOUNT UNCHANGED across all three credentialed trials.** Buzz
  **4,101,822 → 4,101,822** (delta **0**), `app list` byte-identical, **no `ab-*` listing**,
  `gen=0 sub=0` every time. ⚠ `civitai app create` **scaffolds locally and creates no
  listing** — a trial's `app create ab-…` is not an account mutation.
- ⚠ **No `clawgate-task:` field** — `clawgate_handoff.sh resolve` exited **5**. An unknown
  session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id". Not a clean bill of health.
- **Claims:** `app-build-dogfood-10` taken and **released**. None held.

### Carried forward — durable values a `State now` replace would otherwise eat

🔴 **RANK 3'S TABLE, kept because an appended block points AT it** ("✅ ANSWERED 2026-09-21
— the harness CAN carry an app-build task" says *"see the State-now table"*). **Sixth**
update in which deleting it would leave that pointer dangling:

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

**Per-cell cost — four points:**

| cell | model | steps | stop | cost |
|---|---|---|---|---|
| `ab-curve-01` | mimo-v2.5 (celsius) | 65 | finished | **$0.0145** |
| `ab-genpost-glm-01` | glm-5.3-flash | 66 | (truncated, mislabelled) | ~$0.0918 |
| `ab-genpost-mimo-01` | mimo-v2.5 | 73 | finished | **$0.0237** |
| `ab-genpost-dsv4-01` | deepseek-v4-pro | 50 | finished | **$0.1859** |

🔴 **$0.1859 is ~7.8× the mimo cell in FEWER steps (50 vs 73)** — model price, not task
length. The *"cheap models are 5–20× under the frontier"* figure is from **SETUP** trials
of 7–11 steps. **Price an app-build cell from an app-build cell.**

**The prerequisite arc's baseline, which every cost figure here is read against:** 18
trials, 2026-09-19, the three cheap models reach SETUP on 4 of 6 grid rows,
model-independently, at **mean $0.0058/trial** (21 trials = $0.1211); frontier control
**5–20× dearer**. 🔴 That multiple is a SETUP figure and does **not** transfer to app-build
length — see the per-cell table above.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — 3 generations, 61 Buzz,
2 submissions, verified from the account (4,187,454 → 4,187,393). Records real
`submit`/`withdraw` defects incl. **permanent loss of captioned screenshots**.

🔴 **THE FOUR SPECIMEN CONTAINERS ARE THE ARC'S REGRESSION FIXTURES — DO NOT DESTROY.** All
four verified running 2026-09-21 after the merge: `dogfood-ab-curve-01` (celsius
`RENDER=yes observed=212`), `dogfood-ab-genpost-mimo-01` (`RENDER=yes`),
`dogfood-ab-genpost-glm-01` (unmodified scaffold `RENDER=no` — **the negative control**),
`dogfood-ab-genpost-dsv4-01` (the consent-gated app — **the fixture that proves `#690`**).
Re-creating any one costs a real trial.

**Where each trial's evidence lives** — none is in the primary clone, and every worktree
that held some has now been REMOVED:
`ab-genpost-mimo-01` + `ab-genpost-glm-01` → `/tmp/wt-verify686-1071809/scripts/dogfood/runs`
(both — the directory to pass as `DOGFOOD_RUNS`); **`ab-genpost-dsv4-01` →
`~/.cache/dogfood-runs-2026-09-21/ab-genpost-dsv4-01/`**, copied OUT of its worktree before
that worktree was removed, precisely because a routine cleanup already destroyed
`ab-curve-01`'s transcript.

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

### 🔴 OPEN — the oracle seeds a signed-in viewer with an EMPTY SCOPE LIST, so a consent-gated app can never pass
- as-of: 2026-09-21

🔴 **This is the `cli#686` viewer defect one layer deeper, and it is the third time this arc
has found the instrument wrong rather than the model.** `#686` fixed *"no viewer, so an
auth-gated app renders a branch production never exhibits"*. The same sentence is now true
of **capability**: a viewer is present, the token is inert, and an app that gates on the
token's scopes renders a branch production never exhibits.

- **Symptom + exact repro:** `bash oracle.sh ab-genpost-dsv4-01 root` →
  `observed=ready RENDER=no`, `render_reason=timed out after 15000ms waiting for the status
  to leave "ready" after clicking generate`, with `gate=pass` and the manifest declaring
  **both** `ai:write:budgeted` and `posts:write:self`.
- **Observed (with values), the full chain:**
  - `scripts/dogfood/briefs/_cdp.mjs` seeds the credential half as **`{ raw: '',
    scopes: [] }`**, described in-file as *"deliberately inert … it buys the block no
    capability"*.
  - The app: `App.tsx:41` `const granted = hasBudgetedScope(token.scopes)`;
    `:157-172` `handleGenerate` → `if (!granted) { consentPendingRef.current = true;
    requestConsent({ scopes: ['ai:write:budgeted'] }); return; }`; `:114`
    `setStatus('generating')` lives inside `doGenerate`, which is never reached.
  - The DOM produced **zero mutations** in the 15 s after the click — an old-value harvest
    added to the recorder returned `replacedValues=[]`.
  `via: measurement` + `via: code`
- 🔴 **Ruled out — that the empty harvest meant the probe was wired to nothing.** The
  identical probe on `ab-genpost-mimo-01` returned a **non-empty** list. That positive
  control is what makes the deepseek zero a reading rather than a silence.
  `via: measurement`
- **Ruled out — that a missing `data-testid="generate"` caused it.** The assertion locates
  the control by **label** (`labelExpr(GENERATE_LABEL)`, `genpost.assert.mjs:155,169`), and
  the brief only asks for *"a button labelled Generate"*. It was found; `generateDisabled`
  came back `true`/`false`, never `null`. `via: code`
- **Ruled out — that the status transition was a batched React transient the recorder
  missed.** It is a real weakness (`INSTALL_RECORDER`'s `push` reads `textContent` fresh at
  callback time, and MutationObserver batches per microtask checkpoint, so the in-file
  claim that it catches a machine passing *"through `generating` and out the other side"*
  is **narrower than it reads**) — but it is not what happened here: zero mutations means
  nothing was ever committed. `via: measurement`
- 🔴 **The grading consequence, which is the real finding:** mimo passes because it calls
  `setStatus('generating')` **before** `await requestConsent()`; deepseek fails because it
  checks consent **first**. **The assertion rewards an optimistic UI status and penalises
  the app that verifies it may generate before claiming it is generating.** That is
  backwards, and it is a property of the harness, not of either model.
- **Leading hypothesis for the fix:** seed a token whose `scopes` carry what the block's
  own manifest declares (the oracle already parses it — it prints `scopes=`), while leaving
  `raw` empty so the transport still rejects. That keeps *"nothing can complete here"* true
  — the property `#686` asserted from inside the page — while removing a branch production
  never exhibits. ⚠ It needs its own control: an app that spends without checking must
  still not appear to succeed.
- **Next probe:** re-grade `ab-genpost-dsv4-01` with `token.scopes` seeded from the
  manifest. If it flips to `RENDER=yes`, the cell is a PASS and the matrix reads 2/3 with
  one explained failure. **That single re-grade is free — the container is still up.**

### ✅ CLOSED 2026-09-21 — the CI Chromium launch flake: `#687` merged and `main` went green
- as-of: 2026-09-21

🔴 **This RETIRES the block above it, "⚠ OPEN — the CI Chromium launch flake: a fix is
proposed but the stall was never reproduced". Its measurements stand; its OPEN status does
not, and its "Next probe: none locally" instruction is obsolete** — the mechanical closing
condition it named has now been evaluated once and did not fire. (The tool appends and
cannot edit an earlier heading, so this paragraph is the retirement marker.)

- **Observed (with values):** `#687` merged as `3ffebca`; `main`'s check-runs at that SHA
  read **`MAIN_TOTAL=12 MAIN_FAILURES=0`**, `build-test success`. Four oracle runs this
  session all printed `attempt 1 of 2` (136–167 ms) with **no `BROWSER LAUNCH RETRY`**.
  `via: measurement`
- 🔴 **Ruled out — that this proves the flake is gone. It does not, and the block's own
  author already said so.** The honest position stays **"rarer, not provably gone"**: one
  green `main` run is one sample of an intermittent failure, and the four local launches
  were on this workstation, not the CI runner. The closing condition **stays armed** and
  costs nothing — `ci.yml` renders a retry as a `::warning`. `via: assumed`
- **Next probe:** none. Watch `build-test` warnings passively.

### 🔴 OPEN — pricing `cli#685` CANNOT ride along with rank 6; it is blocked on a release
- as-of: 2026-09-21

🔴 **This CORRECTS rank 6's own instruction** (*"⚠ Re-measure carry cost while running, to
price `cli#685`"*) **and the doc-cost block's** *"Next probe: re-measure carry cost on a
post-`cli#685` trial … **Free if folded into rank 6.**" Both sentences are wrong about the
same mechanism: it is neither free nor possible.

- **Symptom + exact repro:** step 5 of every trial is `npm install -g @civitai/cli`. A
  trial runs the **published** CLI, never this repo's tree, so a merged-but-unreleased doc
  fix is invisible to every cell.
- **Observed (with values), three agreeing reads:** `npm view @civitai/cli version` →
  **`0.1.106`** (`time.modified` 2026-09-19T19:41Z); `git tag --contains bdeddef` →
  **empty**, `git describe origin/main` → **`v0.1.106`**; and the running container's own
  artefact, `head -c 600 /work/ab-prompt-to-post/README.md | grep -c useCreatePostFromApp`
  → **0**, at the byte offset (388) where `#685` places the 36-hook index.
  `via: measurement`
- **Ruled out — that the deepseek cell prices the fix.** `civitai --version` inside its
  container reads **0.1.106**. `via: measurement`
- 🔴 **The consequence is load-bearing for the matrix, and it is favourable:** glm, mimo and
  deepseek all ran the **same pre-`#685` doc stack**, so the three cheap cells differ on the
  **model axis alone**. A release landing mid-matrix would have confounded exactly the
  comparison the frozen condition asks for.
- **Observed — the cost is still being paid:** the deepseek trial spent steps 26–31
  grepping `node_modules` for `useCreatePostFromApp` / `BlockCreatePostRequest` /
  `firstImageUrl`, the same reverse-engineering that cost glm **706,371 carry tokens,
  28.2% of its run**. `via: measurement`
- **Next probe:** cut a release containing `bdeddef`, then re-run ONE genpost cell on the
  same model and diff carry cost. Until then the doc-stack block's figures are a claim
  about `0.1.106` and nothing newer.

### ✅ MEASURED 2026-09-21 — the inert token WAS the cause; deepseek passes, and the fix is controlled
- as-of: 2026-09-21

🔴 **This RESOLVES the block above it, "🔴 OPEN — the oracle seeds a signed-in viewer with an
EMPTY SCOPE LIST". Its diagnosis was right and its status is now closed; its "Leading
hypothesis" and "Next probe" have BOTH been run — do not re-run them.** (The tool appends
and cannot edit an earlier heading, so this paragraph is the retirement marker.)

- **Observed (with values), the experiment.** One knob (`token.scopes`), four arms, same
  oracle build, same containers:

  | arm | scopes | `observed` | verdict |
  |---|---|---|---|
  | deepseek | empty | `ready` | `RENDER=no` |
  | deepseek | `ai:write:budgeted,posts:write:self` | `ready>generating>ready` | **`RENDER=yes`** |
  | glm scaffold (neg control) | seeded | `''` | `RENDER=no` |
  | mimo (pos control) | seeded | `ready>generating>ready` | `RENDER=yes` |

  `via: measurement`
- 🔴 **Ruled out — that the patch itself changed the verdict.** The patched file with the
  env var UNSET reproduced `observed=ready RENDER=no` byte-for-byte. That control ran FIRST,
  before the arm anyone wanted to believe. `via: measurement`
- 🔴 **Ruled out — that seeding scopes makes the oracle permissive.** The unmodified
  `page-money` scaffold still grades `RENDER=no` with scopes seeded. This is the arm that
  decides whether the fix is shippable, and it holds. `via: measurement`
- **Ruled out — that it perturbs the known-good cell.** mimo stays `RENDER=yes`. Its
  `observed` lengthens `ready>generating` → `ready>generating>ready` because with consent
  granted it reaches the submit, which the stub rejects — the verdict is
  `seq.includes('generating')`, so it is unaffected. **Expect that string to change when
  rank 10 lands; it is not a regression.** `via: measurement`
- 🔴 **What the fix must preserve, and the evidence it still does:** `raw` stayed **empty**
  in every arm, so `InlineTransport.sendRequest` still rejects unconditionally and
  *"nothing can complete here"* — the property `#686` asserted from inside the page —
  remains true. Scopes buy the block a *branch*, never a capability.
- **Next probe:** none for the diagnosis. The remaining work is shipping it (rank 10):
  seed `token.scopes` from the block's own manifest, which the oracle already parses (it
  prints `scopes=`), and update `fxGenpostBootstrapProbe` in `dogfood_oracle_test.go`,
  which asserts the seeded object from inside the page and will go red.

### ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- as-of: 2026-09-21

🔴 **This CLOSES the two blocks above it — "🔴 OPEN — the oracle seeds a signed-in viewer
with an EMPTY SCOPE LIST" and "✅ MEASURED — the inert token WAS the cause". Their evidence
stands; their OPEN status and their Next-probe instructions are both spent. Do not re-run
the four-arm experiment to confirm this** — it is now the shipped behaviour of `main`, and
the three-arm check under *How to verify* is the cheaper successor.

- **Observed (with values), from a CLEAN checkout of the merge SHA `82d9d83` — deliberately
  not the implementing agent's tree:** deepseek `RENDER=yes observed=ready>generating>ready`
  · scaffold `RENDER=no observed=''` · mimo `RENDER=yes`. CI `13/13`, 0 non-success, and the
  branch already contained the current `main` tip, so the green is about the merged tree.
  `via: measurement`
- **What shipped:** `HOST_BOOTSTRAP` became `hostBootstrap(scopes)`; `oracle.sh` reads
  `block.manifest.json` **once** and passes the scope CSV to the assertion **as argv[3]**
  (no env var), and carries a **seam guard** that exits 2 when the manifest and the scopes
  the assertion actually presented disagree — a mismatch means nobody knows what was
  graded, so refusing beats reporting.
- 🔴 **`raw` STAYS EMPTY, and that is the load-bearing invariant.**
  `InlineTransport.sendRequest` still rejects unconditionally, so *"nothing can complete
  here"* — the property `#686` asserted from inside the page — survives. **Scopes buy the
  block a BRANCH, never a capability.** Any future change here must re-assert that.
- ⚠ **Stated limit on the new coverage, reported by the implementer rather than found by
  review:** of the four rows in `TestOracleSeedsTheBlocksDeclaredScopes`, **only row 1 was
  watched to fail at base** (`RENDER=no, want yes` — the exact deepseek signature). Rows
  2–4 are green at base: they are **controls, not regression coverage** — row 2 (same app,
  manifest declaring nothing → `no`) is what stops a hardcoded scope list passing row 1.
  **Do not cite rows 2–4 as regression evidence.** `via: measurement`
- ⚠ **The seam guard was demonstrated by a HAND mutation, not a committed one** (mutating
  the reported `hostScopes` to `MUTANT` produced the mismatch and exit 2). The committed
  test pins only the happy path. A cheap hardening if anyone touches this again.
- 🔴 **EXPECTED STRING CHANGE, not a regression:** mimo's `observed` lengthened
  `ready>generating` → **`ready>generating>ready`**, because with consent granted it now
  reaches the submit, which the stub rejects, settling back. The verdict is
  `seq.includes('generating')`, so it is unaffected. **Any doc still asserting the short
  string is stale** — `README.md` and `genpost.md` were updated in `#690`.
- **Next probe:** none.

## Next steps (ranked)

🔴 **Numbering frozen.** The arc's own condition is answered; 6 and 10 closed 2026-09-21.

⚠ **On `forcing:` — the operator asked for the ARC, not for individual items; the
decomposition is agent-authored. Rank 7 is the exception (asked for in those words on
2026-09-21); rank 9 traces to an ask made the same day.**

1. ✅ **DONE** — brief-injection path (`cli#678`). forcing: user — satisfied
2. ✅ **DONE** — the celsius brief and assertion (`cli#678`). forcing: user — satisfied
3. ✅ **DONE** — the step/cost curve. forcing: user — satisfied
4. ❌ **DELETED** — history management not required; rank 3 measured it. forcing: gate — retired
5. ✅ **DONE** — the render oracle (`cli#681`) + two defect fixes (`cli#686`).
   forcing: user — satisfied
6. ✅ **CLOSED 2026-09-21 — 2 of 3 cheap models PASS**, reproducible from `main` since
   `4e6214f`. Closed WITHOUT the frontier control by operator decision, and on ONE
   environment (`cli#665` blocks two more). Both limits are stated on the verdict above and
   are **not** to-do items. forcing: user — satisfied
7. 🔶 **STILL OPEN AND NOT ADVANCED BY RANK 6 — `app submit` is OUTSIDE the frozen
   condition.** 🔴 **Three credentialed trials have now run with a submission PERMITTED and
   all three used none (`sub=0`).** A cap that ALLOWS a submit does not produce one:
   **this needs a BRIEF that asks for a submit, not a permission.** That is the whole
   remaining design, and it is one line of brief text plus a decision about what a
   submitted listing costs to withdraw (see the `dogfood-3` precedent — withdraw
   permanently destroys captioned screenshots).
   forcing: user — the operator asked for this on 2026-09-21
8. ✅ **DONE** — `#688` then `#687` merged in that order; `main` green.
   forcing: gate — satisfied
9. **Cut a release containing `bdeddef` (`cli#685`), then re-run one genpost cell to price
   the doc fix.** Blocked until a release exists — trials install from npm, and `#685` is in
   no tag. Touches `civitai/cli` only. forcing: user — the operator asked to price `cli#685`
   as part of rank 6; measurement showed it cannot be done there
10. ✅ **DONE** — `cli#690` `4e6214f`, verified from a clean checkout by a second party.
    forcing: gate — satisfied

🔴 **THERE IS DELIBERATELY NO RANK FOR THE FRONTIER CONTROL.** Re-running it
(`gemini-3.8-flash`, one genpost cell, ~$0.03–0.30) would satisfy the frozen text as
written — and it is **not queued**, because the operator decided on 2026-09-21 to close
without it. A ranked item here would read as outstanding work and invite a later session to
tidy the arc up, which is the opposite of the decision. It is written down in the verdict
above so the option stays findable; taking it is a NEW decision, not a continuation.

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

### Added 2026-09-21 (late) — the merge round, and a verdict that was about the instrument

- 🔴 **THE THIRD INSTRUMENT DEFECT OF THE ARC, AND THE PATTERN IS NOW UNMISTAKABLE: when a
  cell grades `no`, the harness has been wrong more often than the model.** `#686` fixed an
  anonymous viewer and a defaulted brief; this session found an inert token. **Before
  recording a `RENDER=no` as a model result, find the branch the app actually took** — here
  it was three source reads (`_cdp.mjs` seed → `granted` → `handleGenerate`) and cost
  minutes, against a wrong headline that would have stood indefinitely.
- 🔴 **A PROBE'S ZERO NEEDED ITS POSITIVE CONTROL AND THE CONTROL CHANGED THE READING.** The
  old-value harvest returned `[]` on deepseek — which is what a probe wired to nothing also
  returns. Running the **same** probe on the passing mimo cell returned a non-empty list,
  and only then was the zero evidence. **Never quote a probe's zero without the arm that
  makes it non-zero.**
- 🔴 **A MERGED FIX IS NOT A SHIPPED FIX WHEN THE HARNESS INSTALLS FROM npm.** Rank 6 said
  pricing `cli#685` was *"free if folded into rank 6"*; it is impossible until a release.
  **Check the fix is in the ARTEFACT the measurement loads** — npm's `latest`, not
  `origin/main`.
- ⚠ **DEEPSEEK IS NOT A CHEAP MODEL AT APP-BUILD LENGTH: $0.1859 in 50 steps, ~7.8× the
  mimo cell in FEWER steps.** The "5–20× under the frontier" figure comes from the SETUP
  grid, where a trial is 7–11 steps. **Price an app-build cell from an app-build cell.**
- ⚠ **The rebase of `#687` was the whole fix for its `BLOCKED` state** — its only
  non-success was `pins-vs-published`, a check about its **base**, not its diff. A PR can be
  red on a fact that is true of `main`; rebasing, not editing, is the remedy. (The store's
  `scaffold` entry already records the general case from 2026-09-01.)
- ⚠ **`civitai app create` scaffolds LOCALLY and creates no listing.** A credentialed
  trial's `app create ab-…` at step 11 is not an account mutation — checked against
  `app list`, which was byte-identical before and after.
- ⚠ **Both PRs' landings were confirmed by `mergedAt` + `mergeCommit.oid` and the new
  `origin/main` tip**, never by `git merge-base --is-ancestor`, which is permanently false
  after a squash merge.

### Added 2026-09-21 (close) — what this arc actually taught

- 🔴 **THE INSTRUMENT WAS WRONG MORE OFTEN THAN THE MODELS WERE — four times, and every
  time it produced a confident, plausible verdict.** A brief defaulting to `celsius`; an
  anonymous viewer; a vacuous negative control passing off a shell metacharacter; and an
  inert token. **Three of the four would have been reported as a model failure.** The
  working discipline that caught the fourth: *before recording a `RENDER=no` as a result,
  find the BRANCH the app actually took.* Three source reads did it.
- 🔴 **A GRADER CAN PENALISE THE MORE CORRECT IMPLEMENTATION, AND NOTHING ABOUT THE VERDICT
  SAYS SO.** mimo set its status optimistically and passed; deepseek verified it held a
  budgeted scope before claiming to generate, and failed. **Ask what an app must do to
  satisfy your assertion, and whether that is the behaviour you actually want to reward.**
- 🔴 **A PROBE'S ZERO NEEDS THE ARM THAT MAKES IT NON-ZERO.** The old-value harvest returned
  `[]` on deepseek — indistinguishable from a probe wired to nothing. The same probe on the
  passing cell returned non-empty, and only then was the zero evidence.
- 🔴 **RUN THE CONTROL ARM BEFORE THE ARM YOU WANT TO BELIEVE.** The scope experiment ran
  patched-file-with-the-knob-OFF first. Had that flipped the verdict on its own, the
  headline arm would have meant nothing — and it is the arm nobody thinks to run.
- 🔴 **A MERGED FIX IS NOT A SHIPPED FIX when the harness installs from npm.** Pricing
  `cli#685` was recorded as *"free if folded into rank 6"* and is impossible until a
  release. **Check the fix is in the ARTEFACT the measurement loads.**
- ⚠ **A SUBAGENT'S SELF-REPORT WAS ACCURATE AND STILL WORTH RE-DERIVING.** `#690`'s three
  live verdicts were re-run from a clean checkout of the merge SHA rather than the agent's
  tree; they matched. The point is not that it lied — it did not, and it volunteered its own
  coverage gap — but that "verified in the tree that built it" is a different claim.
- ⚠ **`runner.py --out` defaults to `.`; only `driver.sh` passes `--out runs`.** A
  hand-rolled trial lands outside where the graders look and is ungradeable until moved or
  `DOGFOOD_RUNS` names its parent. Nothing errors.

## How to verify

**The whole arc's verdict, ~3 minutes, no trial, no spend. ALWAYS run all three** — a change
that makes the middle one pass has broken the oracle:

```bash
CLI=/home/zach/workspace/civit/cli
git -C "$CLI" worktree add --detach /tmp/wt-verify origin/main
D=/tmp/wt-verify/scripts/dogfood
R=/tmp/wt-verify686-1071809/scripts/dogfood/runs      # mimo + glm
DS=~/.cache/dogfood-runs-2026-09-21                   # deepseek
(cd "$D" && DOGFOOD_RUNS="$DS" nix-shell -p chromium --run \
   'CIVITAI_CHROME=$(command -v chromium) bash oracle.sh ab-genpost-dsv4-01 root' | tail -1)
(cd "$D" && DOGFOOD_RUNS="$R"  nix-shell -p chromium --run \
   'CIVITAI_CHROME=$(command -v chromium) bash oracle.sh ab-genpost-glm-01  root' | tail -1)
(cd "$D" && DOGFOOD_RUNS="$R"  nix-shell -p chromium --run \
   'CIVITAI_CHROME=$(command -v chromium) bash oracle.sh ab-genpost-mimo-01 root' | tail -1)
git -C "$CLI" worktree remove --force /tmp/wt-verify
```

Expect **`RENDER=yes`** / **`RENDER=no`** / **`RENDER=yes`**, all three `gate=pass`, and
`observed=ready>generating>ready` on the two passes. 🔴 **`RENDER=no` on the middle line is
the PASSING state of the negative control** — it is the unmodified `page-money` scaffold,
and its disagreement with its own validator (`gate=pass`) is the arc's founding thesis.

🔴 **Read `stop` before believing any cell** — `truncated` is a harness limit, not a model
failure, which is what `#684` exists to make visible. The three graded cells read
`stop=finished`; `glm`'s reads truncated, which is why it is a FAIL-explained and not a
data point about the product.

**The account — the only trustworthy evidence about spend:**

```bash
civitai buzz      # TOTAL must equal 4101822 unless a generation was intended
civitai app list  # 13 listings; every pre-existing slug unchanged; no ab-* present
```
## Defects (batched)

- `bump-scaffold-pins.yml` validates `pins-vs-published` in-job but has **no in-job
  equivalent for `scaffold-currency`** — the half that actually broke on `#676`. A pin bump
  cannot catch a moved export; only `scaffold-currency` can, and it does not run on a
  cron-authored bump PR.
- The nightly sweep's PRs do not trigger checks (GitHub loop-guard); a hand `workflow_dispatch`
  does. Making the sweep dispatch its own validation closes it. ⚠ **Still true after this
  session** — `#688` was hand-dispatched and got all 13 checks.
- `cli#665` open — blocks 2 of the 4 trial environments. Confirmed still OPEN 2026-09-21.
- ~~🔴 **`_cdp.mjs` seeds `token: { raw: '', scopes: [] }`, so a block that gates generation
  on a budgeted scope can never reach `generating`.** The assertion consequently rewards
  setting a UI status optimistically and penalises checking consent first. Rank 10.~~
  **FIXED — rank 10 shipped.** `oracle.sh` reads the manifest once and hands the declared
  scope list to the assertion as an argument (`node <brief>.assert.mjs <url> <csv>`), which
  seeds it as `token.scopes`; `raw` stays `''`, so scopes buy a *branch* and never a
  *capability*. Regression coverage is `TestOracleSeedsTheBlocksDeclaredScopes` (red at
  `origin/main` with `RENDER=no observed=ready`, green at HEAD) plus the updated
  `fxGenpostBootstrapProbe`, which now asserts the seeded scopes match the manifest from
  inside the page. ⚠ **`observed` on a scope-gated cell lengthens `ready>generating` →
  `ready>generating>ready`** — with consent granted the app reaches the submit, which the
  stub rejects. The verdict is `seq.includes('generating')`, so it is unaffected; any doc
  quoting the old string is stale, not a regression.
- ⚠ **`genpost.assert.mjs`'s `INSTALL_RECORDER` claims more than it delivers.** Its comment
  says polling cannot catch a machine passing *"through `generating` and out the other
  side"* and a MutationObserver can; but `push` reads `textContent` **fresh at callback
  time** and MutationObserver batches per microtask checkpoint, so a transition and its
  revert inside one React commit read as the final value only. Harvesting
  `characterDataOldValue` / `removedNodes` closes it. **Not what broke the deepseek cell** —
  recorded so the comment is not trusted beyond what it does.
- ⚠ **`runner.py --out` defaults to `.` while `oracle.sh`/`grade.sh` read `runs/<trial>/`.**
  Only `driver.sh` passes `--out runs`, and the README's own one-trial example omits it, so
  a hand-rolled trial is ungradeable until the dir is moved or `DOGFOOD_RUNS` names its
  parent. Nothing errors.
