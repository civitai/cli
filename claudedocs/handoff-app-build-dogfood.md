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

🔴 **SIX PRs MERGED 2026-09-27, all verified on `origin/main` BY CONTENT** (a squash merge never
makes the branch an ancestor, so ancestry is never the check): `#724` `7c57d3a` frontier drop ·
`#726` `e9c9d78` schema re-vendor · `#717` `721edae` v0.1.1-live · `#728` `8c4c631` oracle
verdict-forgery · `#718` `3c450ea` consent control + archive eviction · `#729` `9e9142e` ship
verdict-forgery. **Ranks closed: 13, 15, 16, 17, 18, 19.** `$0` of model spend all day.

🔴 **THE LIVE VERSION IS `v0.1.2`** — `civitai app status --json`: `approved`, `deployState:
live`, reviewed `2026-09-27T03:13:52.743Z`, deployed `03:15:10.039Z`. It answers a **second**
real-user report, *"Posting failed. Please try again."* ⚠ The report is operator-relayed; what is
MEASURED is the version, the dates and the controls below.
- **Two defects fixed, both confirmed in the SERVED bytes** (`/assets/index-CcmcNwgc.js`, HTTP
  200): Post was enabled for a workflow that produced NO images — `watch()` resolves on any of
  `succeeded|failed|canceled|expired` and only submit-time status was checked — now gated on
  `status==="succeeded"&&((Q=Bt.imageUrls)==null?void 0:Q.length)>0`; and every host refusal was
  flattened to one string — now all five closed-set codes plus the membership test
  `typeof(R?.code)=="string"&&th(R.code)?Dt(PS[R.code]||R.message||…)`. ⚠ **The minifier
  downlevels optional chaining**, so grepping the source spelling `imageUrls?.length>0` returns
  **0** on the bundle and is NOT evidence of absence — it cost me a near-miss report.
- 🔴 **`dogfood-ab-imgposter-fixed` NOW HOLDS `v0.1.2`, NOT `v0.1.1`** — bundle md5
  `c603fa854eba4d5206b73e04c14a1f3c`, **equal to the live one**. Any doc or probe reading it as
  "the v0.1.1 fix" is one version behind. The only `v0.1.1` control is the image
  `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` (md5 `b8a1936e5939e39aa3e26f6e9e777113`),
  snapshotted BEFORE the edit — without it `v0.1.2` has no attributable control at all.
- 🔴 **`v0.1.2` IS AN ERROR-PATH CHANGE AND THE ORACLE CANNOT GRADE ANY OF IT.**
  `"Please try again."` sites go **3 → 6**; `InlineTransport` rejects every request, so the error
  branch is the only one any arm reaches and a fix making failures legible is invisible to
  `RENDER=`. 🔴 **Auto-resume stays unverifiable by construction** (needs `TOKEN_REFRESH`, which
  `InlineTransport.onMessage` cannot deliver) — a human clicking Generate is the only check.
  **The per-string matrix, and the refutation of a first wrong answer to "what changed", are in
  the ARCHIVE** → *✅ MEASURED 2026-09-27 — what `v0.1.2` actually changed*. ⚠ `posts:write:self`
  4-vs-1 separates `v0.1.1`+`v0.1.2` from `v0.1.0` and says **nothing** about `v0.1.2`.

🔴 **THE TWO FORGERY FIXES ARE ONE RULE NOW, NOT TWO.** `#728` found that an app being graded
could **write its own grade**: manifest-derived values printed with a raw `%s`, where both
consumers take the FIRST `KEY=` match — measured `RENDER=yes` on an app that built **nothing**.
`#729` found the same hole in `ship.verdict.sh` and **extracted the rule to
`scripts/dogfood/_esc.sh`, sourced by both graders**, rather than copying it: verified on `main`,
zero open-coded `tok()` remains in either. 🔴 **MEASURED REASSURANCE: 0 of 8 recorded fixtures was
forgeable**, so no recorded verdict in this arc is void.
- ⚠ **`#718` WAS MERGED WITH ROUND 7 OF ITS AUDIT LADDER NOT RUN — accepted unaudited by a dated
  operator decision, 2026-09-27.** No round ever returned clean: round 6 found the forgeable
  `outputDir`, `#728` fixed it upstream, and a fix RESETS the gate. **Do not read "merged" as
  "audited"**; the residual is a subtly wrong fixture or an evicted block whose meaning changed in
  transit — the two things CI cannot see. Round 7 is now a post-merge option, not a gate.
- ⚠ **`#718`'s DISCARDED retraction is still reachable** in that PR's own commits and in
  `refs/remotes/pr/718`. It concluded the frozen condition was *unmet and still outstanding*,
  which the 2026-09-26 operator drop supersedes. **Do not re-import it, and do not read its
  survival there as a second open question.**

**Account:** `pending: 0`; the `$50/wk` OpenRouter key was at `$48.97` before any of this and no
trial ran. ⚠ **Buzz: RE-READ, never quote** — `civitai buzz` reports per-pool (Blue `1,305,245` /
Green `919,826` on 2026-09-27) and does NOT reconcile with the single `4,074,196` figure older
entries carry. **11 fixture containers + BOTH snapshot images intact; nothing destroyed.**
⚠ **No `clawgate-task:`** — `resolve` exited **5**; an unknown id answers 200 with an empty array,
so that cannot distinguish "touched no task" from "wrong id". Not a clean bill of health.

### Carried forward — durable values a `State now` replace would otherwise eat

🔴 **THE FROZEN CONDITION WAS CLOSED 2026-09-21 AND STAYS CLOSED**: 2 of 3 cheap models built
a passing App Block; `glm` failed, explained. Two permanent limits, **neither a to-do**: the
**frontier control is DROPPED as of 2026-09-26** — see the dated amendment under *Goal* for
the operator's own words and what the drop leaves unmeasured — and **every cell is ONE
environment** (`cli#665`). ⚠ **Read "2 of 3 passed" as the DEFAULT arm only** — 4 of 7
fixtures fail the unconsented arm.

🔴 **CORRECTED 2026-09-26 — the sentence that stood here for five days was AGENT-AUTHORED AND
FALSE WHEN WRITTEN.** It read *"the frontier control was DROPPED by operator decision (never
run, not outstanding)"*, and it closed the frozen condition's last open clause by fiat. The
drop above is real; that one was not. Provenance and the reusable lesson are in *Gotchas* →
**Added 2026-09-26**. Do not let the two be confused: a claim that became true later is not a
claim that was true.

🔴 **BUILD-AND-SHIP IS ANSWERED, AND NOW WITH A WORKING APP AT THE END OF IT.** Built blind
by `xiaomi/mimo-v2.5` for **$0.0170**, submitted, approved, deployed, HTTP 200 — then found
broken by a real user, fixed, resubmitted as `v0.1.1`, re-approved and re-verified — **then
broken again, on a DIFFERENT path, by a second real-user report, and shipped as `v0.1.2`
(live 2026-09-27T03:15:10Z).** **The word "working" is earned only at `v0.1.2`, and only for
the paths the oracle can see.** 🔴 **Two user-found defects on two different paths, after the
oracle passed the app both times, is the durable reading** — see *Gotchas* → **Added 2026-09-25
(feedback)**, whose lesson this is the second instance of, not a new one.

🔴 **RANK 3'S TABLE** (the arc's only record of the step/cost curve): mimo-v2.5, celsius,
**65 steps / $0.0145 / 1,564,496 prompt tokens / per-call 452 → 45,328 / `stop=finished`**,
caps 80/$0.50.

🔴 **BASELINES DRIFT — RE-READ, NEVER QUOTE.** Buzz 4,101,822 (09-21) → 4,074,196 (09-25).
The submissions listing is **at its 100-row cap, no cursor**.

🔴 **COST IS DEAD AS A CROSS-DAY INSTRUMENT** — 2.15× drift in $/1k on the same model and
task in four days. Grade on steps, prompt tokens and behavioural signals.

🔴 **ELEVEN FIXTURES + TWO SNAPSHOTS — DO NOT DESTROY.** ⚠ **Re-enumerated 2026-09-27; the count
in this heading has been wrong three ways at once** — it read "EIGHT + ONE" while `Account` below
said 11 containers and `fixtures/consent-controls/README.md` cross-referenced a *"SEVEN
FIXTURES"* heading that no longer existed. Measured: **11** `dogfood-*` containers (8 `ab-*` +
the 3 `ctl-*` that landed with rank 15) and **2** images —
`dogfood-fixture/ab-ship-mimo-02:pre-consent-fix` and the new
`dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix`. 🔴 **Derive it, do not quote it:**
`docker ps -a --format '{{.Names}}' | grep -c '^dogfood-'`.
`dogfood-ab-genpost-glm-01` is the negative control; `dogfood-ab-imgposter-fixed` holds the
fixed app with a synthetic trial dir at `/tmp/fixed-runs/ab-imgposter-fixed` — ⚠ **and as of
2026-09-27 that is `v0.1.2`, byte-exact to the live bundle; see *State now*.** Evidence lives
OUTSIDE every worktree:
`~/.cache/dogfood-runs-2026-09-21/`, `~/.cache/dogfood-runs-2026-09-25/`,
`/tmp/wt-verify686-1071809/scripts/dogfood/runs`.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — withdrawing a
**first-version** submission destroys captioned media permanently.

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

## Next steps (ranked)

🔴 **Numbering frozen and cumulative** (`#718` published 17–20 first; 21–22 added 2026-09-27):
1–11 settled; 13, 15, 16, 17, 18, 19 closed; 12, 14, 20, 21, 22 live.

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. ⚠ **RE-SCOPE OR RETIRE, do not work as written.** `CIVITAI_SCAFFOLD_TYPECHECK` is in **ZERO**
    workflows at `origin/main` — only in `internal/scaffold/page_money_typecheck_test.go` and this
    doc — so `make ci` still cannot see the defect that cost 26% of a trial.
    forcing: regression — the defect shipped once and CI still cannot see it
13. ✅ **CLOSED 2026-09-25** — `v0.1.1` approved and verified; superseded by `v0.1.2` on 09-27.
    forcing: user — satisfied
14. 🔴 **THE ONLY ITEM WAITING ON THE OPERATOR. T1 "ship-ready" needs a decision, not code.** The
    publish floor requires an icon AND a cover; the trial container has **no `python3`, `convert`,
    `magick` or `pip3`**. Adding image tooling, or letting a brief spend Buzz on a cover, **changes
    what "blind" means.** 🔴 **New constraint, from `app submit`'s own output 2026-09-27:** listing
    media *"carry forward on APPROVAL only — withdrawing the submission, or a moderator REJECTING
    it, deletes the listing and everything on it"*. A rejection destroys attached media, which is
    stronger than the withdrawal-only hazard recorded before.
    forcing: user — asked about "complete working apps" on 2026-09-25
15. ✅ **CLOSED 2026-09-27** — `scripts/dogfood/fixtures/consent-controls/` on `main` via `#718`.
    forcing: gate — satisfied
16. ✅ **CLOSED 2026-09-27** — the re-read landed with `#718`'s eviction. forcing: user — satisfied
17. ✅ **CLOSED 2026-09-26** — frontier control dropped by a dated operator decision (`#724`). See
    *Goal*. **Do not re-open to "tidy the arc up"** — that instruction is what hid the fabrication
    for five days; if you re-open it, say who asked.
    forcing: user — decided by the operator, in their own words
18. ✅ **CLOSED 2026-09-27** — `#726` → `#717` → `#718` all merged, in that order.
    forcing: gate — satisfied
19. ✅ **CLOSED 2026-09-27** — `#728` + `#729`, one shared escaper. forcing: security — satisfied
20. **Fix or remove the four `apps/responsive` 404 links.**
    `https://developer.civitai.com/apps/responsive` is a 404 shipped at four scaffold template
    sites. Touches `civitai/cli` templates and possibly `civitai/civitai-developer-docs`.
    forcing: regression — a dead link ships to every developer who scaffolds
21. 🔴 **Replace the two app-controlled PATH READS — a wrong read, not a wrong render.**
    `scripts/dogfood/oracle.sh:313` is `APP_DIR=$(printf '%s\n' "$MANIFESTS" | head -1 | xargs
    dirname)`, and `xargs` splits on whitespace: reproduced 2026-09-27, `/work/my
    app/block.manifest.json` yields **two** lines (`/work`, `app`) against a one-line control.
    `ship.verdict.sh`'s `APP_COUNT` has the same shape (a newline in a path counts as two
    manifests). Deliberately NOT folded into `#729`: `APP_DIR` feeds `civitai app validate`, the
    `outputDir` resolution and the directory that gets **served**, so fixing it can move a render
    verdict and needs its own app-discovery cases. **Closing condition:** one PR replacing both
    reads whose test asserts `app_dir=/work/my app`, shown red at that PR's own base.
    forcing: security — app-controlled path data still steers a grader's reads
22. **Build the post-path oracle arm.** No arm can see `createPost` — `InlineTransport` rejects
    every request, so "post works" and "post fails for everyone" are indistinguishable, which is
    how TWO user-found defects shipped green on one app. `CREATE_POST_FROM_APP` **is** answerable
    by `createMockHost` (2 hits in `mockHost.js`), so this is buildable, unlike auto-resume.
    ⚠ It proves the BRANCH exists, never that the host accepts the payload.
    forcing: gate — the instrument is blind on the path that produced both live defects

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

### Added 2026-09-25 — the instrument, the brief, and three of my own claims that did not survive

- 🔴 **A BRIEF CAN SHIP A BYPASS. Mine nearly did.** I specified the slug-flag allowlist as
  `{--slug}` on the strength of a literal grep for `"slug"`, which finds three `StringVar`
  sites and misses `--from` (takes a published app slug), `--name` (slugified into the
  blockId) and `--dir`. Implemented literally, `--from=some-other-app` would have become an
  unguarded path to a foreign published app. **When briefing a change to a security check,
  require the implementer to DERIVE the set from source and guard it — do not hand them a
  set you grepped.** The seam guard is what makes this survivable; the fix alone would not.
- 🔴 **I OVERSTATED THE SCAFFOLD DEFECT AS UNIVERSAL AND IT IS INSTALL-SHAPE-DEPENDENT.**
  "Every scaffolded page-money app fails its own build out of the box" is false under plain
  `npm install`. **State the install shape, or any "it's broken for everyone" claim is
  unfalsifiable.**
- 🔴 **I PASSED ALONG A FALSE CLAIM FROM A CODE COMMENT** — that `c=civitai; $c app submit`
  is refused. It never was. **A comment asserting a guard is not evidence the guard exists,
  and quoting one into a brief launders it into a requirement.**
- 🔴 **COST DIED AS AN INSTRUMENT AND NOTHING WARNED US.** Two runs of the same model on the
  same task four days apart differed **2.15× in $/1k** with cached share nearly unchanged.
  Any cross-day dollar comparison in this family confounds provider pricing with the thing
  under test. **Grade on steps, tokens and behavioural signals.**
- 🔴 **THE FIX WORKED THROUGH A HALF NOBODY WAS MEASURING.** `#685`'s headline was a local
  36-hook index; the index was never read. Its *docs-links promotion* is what drove the agent
  to the hosted reference (1 → 5 HTTP requests). **Ask which half of a two-part change did the
  work before crediting either.**
- ⚠ **A `-run` FILTER THAT MATCHES NOTHING PRINTS `ok`.** A scoped `go test -run '<pattern>'`
  returned `ok` with no test names — indistinguishable from a vacuous pass. Re-running with
  `-v` and counting `=== RUN` lines showed 7 tests / 38 lines. **Count what ran.**
- ⚠ **`tail -20` ON EXACTLY 20 LINES SILENTLY DROPPED THE PACKAGE THAT MATTERED.** A merged-tree
  suite read as "all green" had the root package — the real-browser oracle suite, 133 s — cut
  off the top of the window. **Grep for the package by name and count `FAIL`.**
- ⚠ **A SUBAGENT STOPPED WITHOUT REPORTING, TWICE, AND THE WORK WAS FINE.** Both times the
  final message was a status line, not the report; the PR and tests existed. **A missing report
  is not a missing result — ask for the report rather than re-deriving the work.**
- ⚠ **REBASE BEFORE MERGING IN THIS REPO, ALWAYS.** Three PRs this session were green on a base
  that had moved (`#687` on stale pins, `#691` 3 commits behind, `#702` 2 commits behind while
  another session released `v0.1.108`). File overlap was zero for `#702`, which is not safety —
  the merged-tree suite is. It was green; the check is what makes that a fact.

### Added 2026-09-25 (ship) — what shipping taught that building did not

⚠ **Four bullets were DEDUPED out of this block on 2026-09-27** — they were near-verbatim
repeats of bullets in the block immediately above (*a brief can ship a bypass*; *a `-run`
filter that matches nothing prints `ok`* + `tail -20`; *a subagent stopped without
reporting*; *rebase before merging*). Nothing was lost: read them there.

- 🔴 **A CAP'S FAILURE DIRECTION IS THE WHOLE DESIGN.** The intuitive submission-cap fix
  fails OPEN on a live publishing path. Ask *"if the thing I depend on changes, do I
  over-refuse or under-refuse?"* before choosing the mechanism.
- 🔴 **`go test ./...` GREEN IS NOT `gofmt` GREEN.** `#708` was reported locally green and CI
  reddened on one unformatted file, taking `build-test` **and** `lint` with it. Two
  different claims, reported as though one covered the other.
- 🔴 **THE FIX WORKED THROUGH A HALF NOBODY MEASURED.** `#685`'s headline was a local 36-hook
  index; **the index was never read in either run**. Its *docs-links promotion* is what drove
  the agent to the hosted reference (1 → 5 HTTP requests). **Ask which half of a two-part
  change did the work before crediting either.** Operator decision 2026-09-25: hosted is
  canonical, the local index was drift — retired in `#702`.
- 🔴 **A MERGED FIX IS NOT A SHIPPED FIX when the harness installs from npm.** Nearly ran the
  ship cell against a CLI missing `#702`; cut `v0.1.109` first and verified the fix in a
  **generated scaffold** (`tsc --noEmit` exit 0 under `--legacy-peer-deps`, the exact shape
  that had cost 11 steps).
- ⚠ **RE-READ A BASELINE IMMEDIATELY BEFORE SPENDING, NOT WHEN YOU PLANNED THE RUN.** Doing
  so caught `oauth-probe` appearing mid-setup and killed a shortcut ("0 pending ⇒ any pending
  row is ours") I would otherwise have graded against.
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

### Added 2026-09-27 — what merging a doc can break

- 🔴 **REBASING YOURSELF PROVES NOTHING ABOUT THE OTHER OPEN PRs ON THE SAME FILE.** I rebased
  `#724`, merged it, and put `#717` and `#718` into conflict — both clean minutes earlier. **The
  check is one command BEFORE merging:** `gh pr list --state open --json number,files` filtered
  to the path you are landing. This doc already said *"two changes touching one file: TEST-MERGE
  them"*; it does not say "your own branch", and I read it as if it did.
- 🔴 **A DOC-ONLY MERGE CAN MAKE THE CANONICAL DOC WRONG ABOUT PRODUCTION.** While `#717` sat
  conflicted, `main` asserted `v0.1.1` was *"pending operator review"* and *"`v0.1.0` is live and
  broken"* — both false since `2026-09-25T23:30Z`. **Delaying a doc PR is a correctness change.**
- 🔴 **RESOLVE A DOC CONFLICT ON MEANING, NOT ON SIDES** — here `--ours` wholesale would have
  resurrected a retracted falsehood. Verify with a per-claim grep of BOTH sides; absent conflict
  markers prove only that git is satisfied. **Never rebase a branch another worktree holds** —
  detached-worktree recipe in the `cli` store scope (`cairn recall --ref dogfood`).
- ⚠ **The close-check's own coverage is a FLOOR, not an enumeration:** the arc walk reported **4
  sessions** and its own gaps — **3 of 11** commits on this doc carry no session id, and the
  opencode corpus was not searched. (`--arc` cannot resolve a doc in this repo at all; that is a
  devrc tooling gap, recorded in the `devrc` store scope, not here.)

## How to verify

Every check reports a CONTROL beside its result; a bare count or zero is not a measurement.

**Rank 13 — the live fix at the consumer** (expect `2`, control `0`). ⚠ **The live revision is
`v0.1.2` as of 2026-09-27, not `v0.1.1`** — `2` vs `0` does NOT discriminate between them, so
read the version from the API (below) rather than inferring it from this count:
```bash
curl -s -o /tmp/live.html -w 'page http=%{http_code}\n' https://ab-img-poster.civit.ai/
B=$(grep -o '/assets/index-[A-Za-z0-9_-]*\.js' /tmp/live.html | head -1)
curl -s "https://ab-img-poster.civit.ai$B" -o /tmp/live.js -w "bundle http=%{http_code}\n"
grep -o 'scopes:\["ai:write:budgeted"\]' /tmp/live.js | wc -l     # expect 2
docker exec dogfood-ab-ship-mimo-02 bash -lc \
  'grep -o "scopes:\[\"ai:write:budgeted\"\]" /work/ab-img-poster/dist/assets/index-*.js | wc -l'   # CONTROL: 0
```

**Which revision is serving, and a byte-exact local control for it** (measured 2026-09-27).
🔴 **`dogfood-ab-imgposter-fixed` holds `v0.1.2`, and its bundle is md5-equal to the live one** —
so it is the control for what is serving now, not for `v0.1.1`. The `sh -c` is load-bearing:
`kubectl`-style unquoted globs aside, `docker exec` gets no shell of its own.
```bash
md5sum /tmp/live.js                                  # c603fa854eba4d5206b73e04c14a1f3c
docker exec dogfood-ab-imgposter-fixed bash -lc \
  'md5sum /work/ab-img-poster/dist/assets/index-CcmcNwgc.js;
   grep -o "\"version\"[^,]*" /work/ab-img-poster/block.manifest.json'   # 0.1.2, SAME md5
# discriminating counts vs the v0.1.0 control (dogfood-ab-ship-mimo-02): 4 vs 1
docker exec dogfood-ab-imgposter-fixed bash -lc \
  'grep -o posts:write:self /work/ab-img-poster/dist/assets/index-*.js | wc -l'   # expect 4
docker exec dogfood-ab-ship-mimo-02 bash -lc \
  'grep -o posts:write:self /work/ab-img-poster/dist/assets/index-*.js | wc -l'   # CONTROL: 1
```

**Rank 15 — the consent-control twins, now on `main`.** Expect the three-row matrix in
`scripts/dogfood/fixtures/consent-controls/README.md`: `ctl-scaffold-untouched` `no`/`no`,
`ctl-genpost-blind` `yes`/**`no`**, `ctl-genpost-asks` `yes`/`yes`. 🔴 **`asks` passing is the
positive control** — without it `blind`'s `no` cannot be told from the `unmeasured` case. ⚠ A
`ctl-scaffold-untouched` row that MOVES means the template gained the genpost shape.
```bash
CTL=/home/zach/workspace/civit/cli/scripts/dogfood/fixtures/consent-controls
(cd "$CTL" && bash build.sh --keep && nix-shell -p chromium --run 'bash grade-controls.sh')
```

**The defect and the fix, on the arm that sees them** — expect `RENDER=no`
(`observed=ready>generating>ready`) then `RENDER=yes` (`observed=ready`). 🔴 **On this arm a
status machine that MOVES is the defect.** 🔴 **Auto-resume is NOT verifiable here** (needs
`TOKEN_REFRESH`, undeliverable by `InlineTransport`) — only a human clicking Generate live.
```bash
CLI=/home/zach/workspace/civit/cli
git -C "$CLI" worktree add --detach /tmp/wt-v origin/main
D=/tmp/wt-v/scripts/dogfood
(cd "$D" && CIVITAI_ASSERT_UNCONSENTED=1 DOGFOOD_RUNS="$HOME/.cache/dogfood-runs-2026-09-25" \
  nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) \
  CIVITAI_ASSERT_UNCONSENTED=1 bash oracle.sh ab-ship-mimo-02 root' | tail -1)
(cd "$D" && CIVITAI_ASSERT_UNCONSENTED=1 DOGFOOD_RUNS=/tmp/fixed-runs \
  nix-shell -p chromium --run 'CIVITAI_CHROME=$(command -v chromium) \
  CIVITAI_ASSERT_UNCONSENTED=1 bash oracle.sh ab-imgposter-fixed root' | tail -1)
git -C "$CLI" worktree remove --force /tmp/wt-v
```

**Rank 17 — that no frontier cell exists.** Expect NO row naming `anthropic/` or `openai/`,
**and** non-zero rows for the three cheap ids; that second half is the control, and a run
reporting only the zero has not measured anything. ⚠ Caches live outside every worktree — a
`/tmp` glob going empty means the evidence moved, not that a frontier cell appeared.
```bash
for f in ~/.cache/dogfood-runs-*/*/transcript.jsonl \
         /tmp/wt-*/scripts/dogfood/runs/*/transcript.jsonl \
         /tmp/fixed-runs/*/transcript.jsonl; do
  [ -f "$f" ] && head -1 "$f"
done | python3 -c 'import sys,json,collections
c = collections.Counter(json.loads(l).get("model", "<none>") for l in sys.stdin)
for k, v in sorted(c.items()): print(v, k)'
```

**The account:**
```bash
civitai app status --json   # ab-img-poster v0.1.2 approved/live (2026-09-27T03:15:10Z); pending 0
civitai buzz                # 4,074,196 on 2026-09-25 — RE-READ, it drifts
```
⚠ **The submissions listing is AT its 100-row cap with no cursor**, so a blockId absent from
`app status --json` is not evidence it has no submission — look it up directly with
`civitai app status <blockId>`.

## Defects (batched)

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
