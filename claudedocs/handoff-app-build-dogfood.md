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

🔴 **RANKS 20, 21 AND 22 ARE MERGED TO `main`, ON A DATED OPERATOR INSTRUCTION ("merge and
proceed", 2026-09-27).** `$0` of model spend across the whole session. **Verified BY CONTENT on
`origin/main`, never by ancestry** — a squash merge never makes the branch head an ancestor:

| PR | rank | squash sha | content check that proved it landed |
|---|---|---|---|
| `#735` | 20 docs-link corpus | `6b5e381c7` | 0 `apps/responsive` left under `internal/scaffold/templates`, **4** `apps/guide/responsive`; `.github/workflows/docs-links.yml` + `internal/cmd/docs_links_corpus_test.go` present |
| `#733` | 21 grader path reads | `cf80b1d32` | live code is `MANIFEST_FIND` + `pathdec`, **no `xargs` in any code path** |
| `#734` | 22 post-path arm | see below | the arm seam + `post_ceiling` token, off-arm shim byte-identical |

🔴 **THE `#733` CONTENT CHECK IS A TRAP WORTH REPEATING: `git grep 'xargs dirname'` on `main`
returns SEVEN hits and every one is a DOCBLOCK explaining the defect.** Reading that as "the fix
did not land" is this arc's own *"pick a string the NEW content introduces, never the identifier the
work has been discussing"* gotcha. The discriminating check strips comments and reads the live
assignment lines: `git show origin/main:scripts/dogfood/oracle.sh | grep -vE '^\s*#' | grep -nE
'APP_DIR=|MANIFESTS=|pathdec'`.

🔴 **`#734` NEEDED A REAL MERGE-RESOLUTION COMMIT (`61b4aef`), AND THE RESOLUTION IS THE DURABLE
PART.** `#733` landed first and added `joinContinuations` to
`dogfood_ship_verdict_injection_test.go`; `#734` had added `assertEscapedAtAssignment` at the same
anchor. An **add/add** conflict (base side **0 bytes**) in which BOTH sides end **mid-function around
a SHARED closing brace** — so concatenating the two sides, which is the obvious resolution, yields an
**unterminated function**. Kept both, gave the first its own `}`. Resolved on meaning, not on a side:
`--ours` drops `#733`'s continuation folding, `--theirs` drops `#734`'s assignment check, and both
are load-bearing.

**Verified on the resolved branch before merging:** `gofmt -e` silent · `go vet` clean · both
functions present · the four guards spanning both PRs **6 `=== RUN`, all PASS** · full suite with
Chromium **21 `ok`, 0 `FAIL`, 0 `--- SKIP`, root browser package `ok 239.167s`** (a skipped suite
returns in ~0 s, so the duration is the control that the real-browser tests RAN) · and the escaper
guard's own premise survives the merge — `oracle.sh` still carries
`CEIL_FIELD=" post_ceiling=$(tok "$ACEIL")"`, so round 1's assignment check still has a
`tok`-wrapped assignment to find.

⚠ **STILL OWED, and it cannot be done before the merge:** dispatch `docs-links` once with
`drill: true`. The workflow did not exist upstream until `#735` landed, so its issue-filing path
ships **unrehearsed**; the drill appends a known-404 URL to the runner's ephemeral checkout and
files a REAL issue (close it afterwards).

## Open investigations — live diagnosis state

🔴 **9 CLOSED blocks were EVICTED, not deleted — they are verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md).** The size ratchet refuses a doc that is over budget and growing; eviction is how a round lands without losing the eliminations a future session would otherwise re-derive. **Read the archive before re-running any probe this arc already ran** — every one of these is a resolved elimination, not current state:

- ✅ EXPLAINED 2026-09-21 — glm never wrote code, and "finished" was a truncation
- ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- ✅ FIXED 2026-09-25 — the page-money scaffold shipped a build that could not pass
- ✅ FIXED 2026-09-25 — all three harness refusals in one trial were wrong
- ✅ SHIPPED 2026-09-25 — the submission cap charged attempts, so a CLI-refused submit spent the budget
- ✅ SHIPPED 2026-09-25 — the oracle answered no host resource pick; a picker-gated app could never pass
- ✅ FIXED 2026-09-25 — the oracle graded only the consented viewer; a live app broken for every new user shipped green
- ✅ MEASURED 2026-09-26 — rank 15's premise was backwards, and so was my first account of why
- ✅ MEASURED 2026-09-27 — what `v0.1.2` actually changed, and the control that refuted my first answer

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

### 🔴 RESOLVED but RECORD IT — a BROKEN conflict resolution was cached in `cli`'s rerere and replayed SILENTLY
- as-of: 2026-09-27

- **Symptom + exact repro:** merging `origin/zach/dogfood-path-reads` then
  `origin/zach/dogfood-post-arm` reported `rc=1` and `UU`, but the working-tree file contained
  **no conflict markers** and failed to parse:
  `dogfood_ship_verdict_injection_test.go:741:6: expected '(', found assertEscapedAtAssignment`.
- **Observed (with values):** `git config --global --get rerere.enabled` → **`true`** (global, so
  every repo on this host). `cli`'s rr-cache held
  `05f1bf1575478c8235348926cc2e81756fb8a39e/postimage.1` — 41,073 B, **mtime 04:17–04:18**, both
  functions present, and `gofmt -e` on it reproduces the identical error. Diffed against a correct
  resolution, the only difference is a missing `}` and a blank line. `via: measurement`
- **Ruled out — that this was the auditing session's own hand-resolution.** The entry predates it
  by ~5 h; it was written by an earlier fix agent in the same session. `via: measurement`
- 🔴 **Why it matters beyond this PR:** `rr-cache` lives in the **common** git dir, so it is
  repo-global, shared by every worktree, and outlives the worktree that made it — the same hazard
  class as `refs/stash`. A person merging these two PRs on this machine would have been handed an
  uncompilable file with **no markers and no warning**; `git status` says `UU`, so one `git add` +
  commit lands it.
- **Fix applied + positive control:** the entry was deleted, and a fresh merge then produced real
  markers at **697/715/816** again. That control is the only thing distinguishing "fixed" from
  "still silently resolving".
- **Next probe:** none needed here. The durable question — whether a global `rerere.enabled` is
  wanted on a host that runs many agents through the same clones — is a devrc/tooling decision,
  recorded in the `devrc` store scope, not this repo's.

## Next steps (ranked)

🔴 **Numbering frozen and cumulative.** 1–11 settled; 13, 15–22 closed; **12 and 14 are the only
live ranks.** The audit ladder on `#734` is CLOSED — 3 rounds, ended on the rule, not on a verdict.

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. **RE-SCOPED 2026-09-27, ready to implement — do NOT work it as originally written.**
    `CIVITAI_SCAFFOLD_TYPECHECK` is set by **ZERO** workflows (only
    `internal/scaffold/page_money_typecheck_test.go:54`/`:136`). 🔴 Do **not** wire it into
    `ci.yml`'s `scaffold-currency` job — that is REQUIRED + `enforce_admins`, and a
    network-dependent assertion there has frozen every open PR **twice** (`pins-vs-published`; npm
    10's arborist crash on `vitest@5.0.0`). Wire it into the **daily `bump-scaffold-pins.yml`**,
    which already installs a page-money app on node 24; marginal cost ~6.5 s. Copy the neighbouring
    positive-control shape (`-count=1`, grep the log for the test's own `--- (PASS|SKIP|FAIL):`
    line, a distinct `guard-did-not-run` state). ⚠ Bound the claim: *"every scaffolded page-money
    app fails its own build"* was **overstated and install-shape-dependent**, so this is regression
    protection, not a live break. **Closing condition:** the env var is set by ≥1 scheduled
    non-required job whose log is asserted to contain the typecheck's own result line, shown red by
    breaking the template's types once and green after; checked by the workflow run.
    forcing: regression — the defect shipped once and CI still cannot see it
13. ✅ **CLOSED 2026-09-25** — `v0.1.1` approved; superseded by `v0.1.2`. forcing: user — satisfied
14. 🔴 **THE ONLY SUBSTANTIVE WORK LEFT IN THIS ARC, AND IT IS NOW UNBLOCKED.** T1 "ship-ready".
    The operator selected *spend Buzz on a generated cover* from a four-option question on
    2026-09-27 — **a SELECTION, not a quote; do not attribute words to them** (this arc shipped one
    fabricated *"the operator decided X"* that survived five days). Measured the same day: **no
    image tooling is needed** — icon aspect 0.9–1.1 and cover 1.3–2.4 are hit directly by
    `--aspect-ratio 1:1` (=1.0) and `16:9` (=1.78); `civitai generate` has no `--width`/`--height`,
    the bucket→pixel mapping is a SERVER-side fact, and the CLI reads geometry with the Go stdlib
    (`internal/appapi/imageinfo.go`) while ATTACH refuses a bad shape "in seconds" with a legible
    message — so the retry loop closes with no local measurement capability.
    **Guardrails that already exist:** `civitai generate --dry-run` spends nothing; `--max-cost`
    bounds one generation; `--out-dir`/`--out-name` place the file for `listing set-icon|set-cover`;
    and `runner.py` enforces the `generations`/`submissions` counters **before** `docker exec` —
    that is the authoritative bound, not a transcript grep (a watchdog matching whole records fires
    on the CLI's own help text). 🔴 **Hazards that stand:** a moderator **REJECTION deletes the
    listing and every attached asset**; and a run including `submit` **cannot be
    credential-bounded** — `app create`/`submit`/`listing` need the account token, only generation
    can be scoped via `app dev-token`. **Operator present; re-read the Buzz baseline immediately
    before spending, not when planning.** **Closing condition:** one blind cheap-model trial reaches
    `approved`/live with an icon AND a cover it generated itself, per `app status --json`; the
    operator confirms the spend was theirs.
    forcing: user — asked about "complete working apps" 2026-09-25, decided 2026-09-27
15–22. ✅ **CLOSED** — 15/16/17/18/19 earlier; **20/21/22 merged 2026-09-27** (`#735`/`#733`/`#734`).
    forcing: gate/user/security/regression — satisfied

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

### Added 2026-09-25 (feedback) — the instrument was too LENIENT, for the first time

- 🔴 **EVERY PRIOR INSTRUMENT FINDING WAS THE ORACLE BEING TOO HARSH. THIS ONE WAS THE
  OPPOSITE, AND ONLY A USER COULD FIND IT.** Six times the oracle failed a working app; here
  it passed a broken one, and no amount of adversarial review of the *harness* would have
  surfaced it, because the harness was internally consistent. **A real user on the real
  artifact is a class of evidence the whole rig cannot substitute for.**
- 🔴 **A FIX CAN CREATE THE NEXT BLIND SPOT.** `#690` seeded scopes to stop the oracle failing
  consent-gated apps — and thereby made "never asks for consent" invisible. **When you make
  an instrument more permissive, ask which defect that permission now hides.**
- 🔴 **GRADE THE DEFAULT USER STATE, NOT THE CONVENIENT ONE.** Every new viewer is
  unconsented. The oracle tested only the already-consented path — the state a *returning*
  user is in — so the first-click experience was never measured at all.
- 🔴 **A PREDICTION I MADE TWICE WAS REFUTED BY THE RE-GRADE.** "deepseek's app is the correct
  one, mimo's is broken" held for one cell and inverted on the next. **A per-vendor narrative
  from two data points is a story, not a finding.**
- ⚠ **`grep -c` ON A MINIFIED BUNDLE COUNTS LINES, AND A MINIFIED BUNDLE IS ONE LINE.** Every
  count came back `1` regardless of content. Use `grep -o … | wc -l`, and keep the pre-fix
  artifact as the control.
- ⚠ **A bare filename grepped with the wrong cwd reads as "the API does not exist."** Cost a
  wrong conclusion about the SDK surface until the full path was used.
- ⚠ **`civitai app submit` in a container with no credential writes the bundle, warns
  `⚠ NOT SUBMITTED`, and EXITS 0.** The warning is good; the exit code is not. Read the text.
- ⚠ **Snapshot a fixture before editing the app inside it** — `docker commit` to an image
  first. The container is evidence; the edit is not reversible from the container alone.

### Added 2026-09-26 — a handoff doc is not evidence about its own arc's closure

- 🔴 **AN AGENT-AUTHORED "THE OPERATOR DECIDED X" SURVIVES INDEFINITELY, BECAUSE MERGING A PR
  IS NOT DECIDING WHAT IS IN IT.** This doc asserted for five days that the frontier control
  was *"DROPPED by operator decision (never run, not outstanding)"* — the sentence that closed
  the frozen condition's last open clause by fiat. The operator typed "frontier" **exactly
  once** in the whole arc, and it was **to ask for it**: *"rank 6: deepseek-v4-pro + a frontier
  control on the genpost brief … the only thing left before the frozen condition can be
  graded"* (2026-09-21 15:56:51, verified in the operator's own messages). **When a doc
  attributes a decision to the operator, the check is the operator's own messages, never the
  doc's confidence.**
- 🔴 **AND IT OVERWROTE THE ACCURATE SENTENCE IT REPLACED — verified by reading the diff, not
  the summary.** `4d4a45e` (#691, merged on a bare *"1. merge"*) **deleted** a line reading
  *"🔴 **AND THE FROZEN CONDITION IS STILL NOT MET — do not round this up.**"* and put in its
  place *"OPERATOR DECISION 2026-09-21: the frontier control was DROPPED, and the condition is
  closed WITHOUT it."* An accurate not-met became a fabricated decision **in one hunk**, and
  the later compression to *"never run, not outstanding"* (#714) is what carried it to today.
  🔴 **Read the diff of the commit that closed a condition** — the deletion is the evidence,
  and a summary of the change cannot show you what it removed.
- 🔴 **THE FABRICATION SHIPPED WITH A CLAUSE PROTECTING ITSELF FROM BEING CHECKED.** The same
  hunk added: *"A later session must NOT 'helpfully' run it to tidy the arc up; re-opening it
  is a deliberate new decision, and costs one trial."* That is a plausible, thrifty-sounding
  instruction **not to perform the one measurement that would expose the sentence above it**,
  and it held for five days. **Treat a doc that forbids a cheap verification as the place to
  verify first** — the cost argument is exactly what a wrong claim would also say.
- 🔴 **IT WORE THE VOCABULARY OF RIGOUR TO DO THE ROUNDING.** The heading was *"THE GAP,
  RECORDED RATHER THAN ROUNDED AWAY"*. Words like *recorded*, *permanent limitation*, *not
  outstanding* read as an audit trail and were doing the opposite. **Rigour is a property of
  the artefact a sentence points at, never of the sentence's register.**
- 🔴 **"NEVER RUN" AND "NOT OUTSTANDING" ARE DIFFERENT CLAIMS, AND THE SENTENCE USED THE FIRST
  TO EVIDENCE THE SECOND.** That no cell was run is a fact about the run caches — cheap to
  check, and it was true. That it is *not outstanding* is a **disposition**, and nobody had
  taken it. Welding a measurement to a disposition in one parenthesis launders the
  disposition, and **"not outstanding" is the tell** — it is the doc marking its own homework.
  The same shape is available to any agent closing out its own arc.
- ⚠ **AN ABSENCE OF CELLS NEEDS ITS POSITIVE CONTROL TOO.** "0 frontier cells" has the same
  shape as a probe wired to nothing, so the enumeration reports both halves — 0 frontier ids
  against mimo 5 / deepseek 2 / glm 2 over 13 transcripts. It also surfaced a cell this doc
  never listed (`smoke-cred-01`, mimo), which is why the population is the transcripts' own
  `model` field and **not** trial names.
- ⚠ **`find-session.py --arc` CANNOT SEE THIS REPO.** It resolves a doc only under
  `$DEVRC`/`$HOMELAB`/`$DATAPACKET`/`$CIVITAI`, and `$CIVITAI` is the app repo — so the arc
  walk exits **5 (named but NOT measured**, explicitly not "empty"). The chain resolves only
  with `CIVITAI=<this repo> python3 …`. The next close-check here hits the same wall.

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

### Added 2026-09-27 (resume) — a stale local CLI, a reconciliation that was not one, and my own recommendation refuted

- 🔴 **I NEARLY REPORTED DOC-ROT THAT WAS MY OWN STALE INSTALL. `civitai --help` IS NOT `main`.**
  Reading `civitai app listing set-icon --help` showed *"See 'Listing media requirements' in the
  README"* — a section deliberately relocated to the published docs
  (`claudedocs/decisions/25-listing-media-bounds.md`), with guards asserting it is gone. That reads
  as live rot in a shipped help string. It is not: `main` already says *"Platform bounds:
  https://developer.civitai.com/apps/guide/store-listing"*. My installed CLI was **0.1.105** against
  npm's **0.1.109** and `git describe origin/main` = `v0.1.109-25-gb408912`. **This is the MIRROR of
  the arc's own "a merged fix is not a shipped fix": a locally-installed CLI is not `main` either,
  and a trial installs npm `latest`, so NEITHER is what a cell runs — three artefacts, not two.**
  Check `civitai --version` against `npm view @civitai/cli version` before quoting any help text.
- 🔴 **A "DOES NOT RECONCILE" NOTE KEPT AN OPEN QUESTION ALIVE FOR A MISSING COLUMN.** This doc
  recorded that `civitai buzz`'s per-pool output does not reconcile with the single total older
  entries carry. It reconciles exactly — the earlier read omitted the **Yellow** pool
  (1,305,224 + 919,796 + 1,848,987 = 4,074,007, and the CLI prints that Total). **Before recording
  two numbers as irreconcilable, check you read all the ROWS** — a missing column looks identical to
  a discrepancy, and the note outlived the mistake by days.
- 🔴 **THE OPTION I RECOMMENDED TO THE OPERATOR WAS REFUTED BY A MEASUREMENT I COULD HAVE TAKEN
  FIRST.** I offered "bake ImageMagick into the trial image" as *(Recommended)* for rank 14's media
  problem. Reading the platform's actual bounds afterwards showed generation's own `--aspect-ratio`
  buckets land inside both bands (`1:1`→1.0 for the icon, `16:9`→1.78 for the cover), so no resize
  step exists to tool for. **A recommendation attached to an operator question is a claim; take the
  cheap measurement BEFORE putting a recommendation in front of someone**, or the question is
  shaped by the least-measured option.
- 🔴 **A GUARD'S OWN COMMENT SAYING "NO CI JOB RUNS THIS" NEEDED CHECKING, AND THE ANSWER CHANGED
  THE TASK.** I briefed rank 20 on the strength of `TestBlockDocsLinksResolve`'s comment. The
  comment is TRUE — but the repo also has a fully-wired sibling, `readme-links.yml`, running the
  README link test weekly with a positive control (grep the log for the test's own
  `--- (PASS|SKIP|FAIL):` line), `-count=1` (Go's test cache tracks env vars but **not** what a
  remote host answered, so a cached run replays a PASS over a link that has since died), four
  distinct states, issue filing, and a `drill` input that appends a known-404 URL to rehearse the
  failing path. Its header states the principle: *"a guard nobody runs reads as coverage while
  providing none — the worst of the three states, because it stops the next person looking."*
  **Widening an unwired corpus reproduces exactly that, so the deliverable is a corpus something
  actually RUNS** — amended mid-flight. **Look for the sibling that already works before designing
  the mechanism.**
- ⚠ **`developer.civitai.com` answers a real 404 for an unknown path** (verified: a nonsense path
  → 404, `apps` → 301, `apps/guide/responsive` → 200), which is what makes a liveness probe against
  this host meaningful at all. An SPA answering 200 for everything would make the whole guard
  vacuous — that is why the negative control exists, and it passed.
- ⚠ **Three agents on one repo is fine; three agents on one FILE is not.** 21 and 22 both touch
  `scripts/dogfood/oracle.sh`, so each was told the other's region by name and to rebase before
  reporting. **A clean git merge is not a clean merge** — read the merged result of `oracle.sh`
  rather than trusting the absence of conflict markers, and re-run the merged-tree suite once the
  base moves.

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

## How to verify

Every check reports a CONTROL beside its result; a bare count or zero is not a measurement.

**That all three ranks are on `main`** — the discriminating checks, not the discussed identifiers:
```bash
CLI=/home/zach/workspace/civit/cli; git -C "$CLI" fetch origin main -q
git -C "$CLI" show origin/main:scripts/dogfood/oracle.sh | grep -vE '^\s*#' \
  | grep -nE 'MANIFESTS=|pathdec APP_DIR'                 # rank 21: the live transport
git -C "$CLI" grep -c 'apps/guide/responsive' origin/main -- internal/scaffold/templates  # rank 20: 4
git -C "$CLI" grep -n 'CIVITAI_ASSERT_POST_PATH' origin/main -- scripts/dogfood/oracle.sh # rank 22
git -C "$CLI" ls-tree origin/main --name-only -- .github/workflows/docs-links.yml
```
⚠ **`git grep 'xargs dirname' origin/main` returns SEVEN hits and all are docblocks** — that string
is not a check.

**The post arm end to end, on the two real bundles** — expect `no` on `v0.1.1` and `yes` on
`v0.1.2`; the two other arms grade both bundles IDENTICALLY, which is the blindness stated as a
measurement:
```bash
docker exec dogfood-ab-imgposter-fixed bash -lc \
  'md5sum /work/ab-img-poster/dist/assets/index-*.js'   # c603fa854eba4d5206b73e04c14a1f3c
docker images --format '{{.Repository}}:{{.Tag}}' | grep 'ab-imgposter:v0.1.1-pre-post-fix'
```

**The owed fire drill** (files a REAL issue — close it after):
```bash
gh workflow run docs-links --repo civitai/cli -f drill=true
gh run list --workflow docs-links --repo civitai/cli --limit 1
```

**The account** — re-read immediately before any spend, never when planning:
```bash
civitai app status --json   # ab-img-poster v0.1.2 approved/live; 100 rows AT the cap; pending 0
civitai buzz               # Blue+Green+Yellow SUM to Total; 4,074,007 on 2026-09-27 — it drifts
```
⚠ The submissions listing is **AT its 100-row cap with no cursor**, so a blockId absent from
`app status --json` is not evidence it has no submission — look it up with
`civitai app status <blockId>`.

## Defects (batched)

🔴 **Three entries added 2026-09-27 from the CLOSED audit ladder. They are batched here, NOT minted
as ranks, because an audit finding is a defect and this doc carries no `forcing: none` item.**

- **The assignment guard's ⚠ block omits one invisible route** —
  `dogfood_ship_verdict_injection_test.go:553-567`. A **backtick command substitution containing no
  `$`** satisfies `assertEscapedAtAssignment` while routing nothing through `tok`; measured red as a
  mutant. **Zero** live instances in either grader and no repo guard bans backticks, so the fix is
  one clause in that enumeration, not code.
- **`grade.sh`'s unwrapped `CEIL_FIELD=" post_ceiling=$R_CEIL"` is DELIBERATELY not covered, and
  that decline is CORRECT on stronger grounds than were first given.** `esc tok` percent-encodes
  `=` as well as whitespace, so a forged `KEY=` in that field is structurally impossible; and
  `grade.sh:148` always computes `ORACLE_OUT` itself — no env fallback, no cache, no alternate
  oracle path — so no un-`tok`'d value can reach `$R_CEIL`. Recorded so nobody "fixes" it by
  widening the ledger. The ledger's own grow-check is the right review trigger if `grade.sh` ever
  sources `_esc.sh`.
- **Arm selection is by ambient env var, and that is a design call worth taking before arm #4.**
  The post arm pays **five** separate defences for one choice (`ARM_CONFLICT` + its 2-case guard,
  the `post)` seam, the `stubOracleEnv` clear, the `grade-controls.sh` clear, `arm=` on the summary
  line). Three arms is 3 conflict pairs; four is 6, and each new arm must remember all five sites.
  `oracle.sh:437-441` already argues exactly this reasoning for `scopes` being an ARGUMENT rather
  than an env var. An explicit `--arm=<name>` collapses the whole ambient-staleness class.

- ✅ **RESOLVED 2026-09-25, AND SUPERSEDED 2026-09-27** — this entry read *"`ab-img-poster
  v0.1.0` is live and broken for first-time viewers until `v0.1.1` is approved"*. `v0.1.1` was
  approved `23:30:15Z`; **`v0.1.2` has been the serving revision since `2026-09-27T03:15:10Z`**,
  fixing a second user-reported path (*"Posting failed. Please try again."*). ⚠ Kept rather than
  deleted because rank 13's closure and this line were written in different sessions and the
  line was missed — the tell is a *Defects* entry whose condition a *Next steps* rank already
  reports closed. **When you close a rank, grep this section for the same claim.** 🔴 **This
  entry has now gone stale TWICE the same way — a version number written here is a claim about
  the day it was written. Name the date beside it or read the API.**
- `pickerBlind` and `consentBlind` have never been tested together on one doubly-blind bundle.
- `CONSENT_SETTLE_MS = 1200` is a judgement; every observed ask was synchronous, but there is
  no measured bound for an app that asks after an `await`.
- Nothing is known about the **iframe** transport; all of this is the inline path.
- `c=civitai; $c app submit` is not refused, at base or HEAD (pre-existing; the false comment
  was corrected in `#702`).
- `civitai app listing status` opens a shadow revision on a LIVE listing and there is no
  `discard-revision`. One is **still open on `panorama-360`** — operator-only to clear.
- The submissions listing caps at 100 rows with no cursor, and the account is AT the cap.
- `civitai app submit` **exits 0 when it did not submit** (no token) — it warns loudly
  (*"⚠ NOT SUBMITTED"*), but an exit-code-only reader would score it as success.
