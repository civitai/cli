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

🔴 **THE ARC IS NOT CLOSED, AND THIS DOC SAID IT WAS.** A close-check on 2026-09-26
read the frozen condition against the operator's own typed messages and the run
caches, and the condition is **one cell short**. See the 2026-09-26 investigation
block. Everything below is written against that correction.

- **Branch / PR:** `civitai/cli` **#718** `zach/dogfood-consent-negative-control`,
  OPEN, `mergeable=MERGEABLE`, 8 commits, `make ci` green. ⚠ **It CONFLICTS with
  #717** (both edit this doc) — test-merged, `git merge-tree` exit 1, three
  conflicted regions. **Merge #717 first, then rebase #718.**
- **DONE this session:** rank 15 — `scripts/dogfood/fixtures/consent-controls/`
  (`a844b51`), then six audit rounds of fixes (`842383c`, `9075e6a`, `4493938`,
  `65df428`, `5ebcb26`, `0d5ef57`, `8e57dd8`). Rounds 0–6 are posted as PR
  comments; each round's `audit-claims` block anchors the next.
- **IN FLIGHT:** #718 is mid-ladder — round 6 found a forgeable `outputDir`
  vector and said the ladder should continue. Round 7 was not run.
- **Deploy/verify:** `ab-img-poster v0.1.1` is **approved** (`reviewedAt
  2026-09-25T23:30:15.974Z`) and **live** — verified at the CONSUMER, not at a
  status field: `https://ab-img-poster.civit.ai/` → **HTTP 200**, deployed bundle
  `/assets/index-CJh51XL4.js` **266,507 B**, carrying `scopes:["ai:write:budgeted"]`
  ×2, `scopes:["posts:write:self"]` ×2, `requestConsent` ×2. **Rank 13 is CLOSED.**

### Carried forward — durable values a `State now` replace would otherwise eat

**The arm's own evidence table** (the pair the retraction below rests on — this IS
the arm's `no` arriving for its own reason, recorded 2026-09-25):

| arm | live `v0.1.0` | fixed `v0.1.1` |
|---|---|---|
| **unconsented** | **`RENDER=no`** `observed=ready>generating>ready` — spent without asking | **`RENDER=yes`** `observed=ready` — asks instead |
| **consented** | `RENDER=yes` | `RENDER=yes` — no regression |

**The seven-fixture re-grade** — 4 of 7 fail the unconsented arm; no default
verdict moved. 🔴 `glm-01`'s `no` is a RENDER failure, not a consent verdict, which
is why only **two** of these (`dsv4-02`, `ship-mimo-01`) join `ship-mimo-02` as the
arm's own `no`:

| fixture | default | **unconsented** |
|---|---|---|
| `ab-curve-01` | yes | **n/a** — celsius brief, transcript destroyed |
| `ab-genpost-glm-01` | no | **no** (a render failure, not a consent verdict) |
| `ab-genpost-mimo-01` | yes | **yes** |
| `ab-genpost-dsv4-01` | yes | **yes** |
| `ab-genpost-dsv4-02` | yes | **no** |
| `ab-ship-mimo-01` | yes | **no** |
| `ab-ship-mimo-02` | yes | **no** ← the live defect |

🔴 **The discriminator is the APP, not the model** — a prediction that "mimo fails,
deepseek passes" was refuted on both halves. Do not rebuild the vendor narrative.

🔴 **RETRACTED 2026-09-26 — "the frontier control was DROPPED by operator decision
(never run, not outstanding)".** That sentence was **agent-authored**. It entered
in `4d4a45e` (PR #691), which the operator merged with a bare *"1. merge"* —
approving a merge, not deciding to drop a cell. **MEASURED across all four arc
sessions (133 operator messages, `extract_user_msgs.py`): the operator typed the
word "frontier" EXACTLY ONCE, and it was to ASK FOR IT.** The frozen condition
says *"Graded per cell across the three cheap models, **with a frontier
control**"*, and no frontier cell exists in any run cache. **The frozen condition
is therefore NOT met.** Do not re-derive the drop.

🔴 **STILL TRUE: build-and-ship is answered, and the HTTP 200 stands.**
`ab-img-poster` was built blind by `xiaomi/mimo-v2.5` for **$0.0170**, submitted,
operator-approved, deployed. ⚠ The transcript never states whether the submit was
performed by the agent inside the trial or by the session on its behalf, and the
slug differs from the glm trial's `ab-image-poster` — do not quote "the agent
shipped it" without settling that.

🔴 **Every cell is ONE environment** — `cli#665` is **still OPEN** (verified
2026-09-26), so **2 of 4** container environments (`node-user`, `ubuntu-apt`) were
never gradeable. The whole matrix ran on writable-prefix envs only.

🔴 **RANK 3'S TABLE** (the arc's only record of the step/cost curve): mimo-v2.5,
celsius, **65 steps / $0.0145 / 1,564,496 prompt tokens / per-call 452 → 45,328 /
`stop=finished`**, caps 80/$0.50.

🔴 **BASELINES DRIFT — RE-READ, NEVER QUOTE.** Buzz was 4,101,822 (09-21),
4,074,196 (09-25). The submissions listing is **at its 100-row cap, no cursor**.

🔴 **COST IS DEAD AS A CROSS-DAY INSTRUMENT** — 2.15× drift in $/1k on the same
model and task in four days. Grade on steps, prompt tokens and behavioural signals.

**Per-cell cost, point-in-time only:** `ab-curve-01` $0.0145 · `glm-01` ~$0.0918
(truncated) · `mimo-01` $0.0237 · `dsv4-01` $0.1859 · `dsv4-02` $0.3510 ·
`ship-mimo-01` $0.0164 · `ship-mimo-02` $0.0170.

🔴 **SEVEN FIXTURES + ONE SNAPSHOT — DO NOT DESTROY.** The seven `dogfood-*`
containers, plus the image `dogfood-fixture/ab-ship-mimo-02:pre-consent-fix`.
`dogfood-ab-genpost-glm-01` is the negative control **for the DEFAULT arm only** —
on the unconsented arm it fails for a render reason and controls nothing. Evidence
lives OUTSIDE every worktree: `~/.cache/dogfood-runs-2026-09-21/`,
`~/.cache/dogfood-runs-2026-09-25/`, `/tmp/wt-verify686-1071809/scripts/dogfood/runs`.

**Three MORE containers, and these ones are REPRODUCIBLE — rebuild rather than
preserve.** `dogfood-ctl-scaffold-untouched`, `dogfood-ctl-genpost-blind`,
`dogfood-ctl-genpost-asks`, regenerated by
`scripts/dogfood/fixtures/consent-controls/build.sh` in ~5 min. Runs at
`~/.cache/dogfood-consent-controls/`.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — withdrawing a
**first-version** submission destroys captioned media permanently.

## Open investigations — live diagnosis state

🔴 **8 CLOSED block(s) were EVICTED, not deleted — they are verbatim in [`handoff-app-build-dogfood-ARCHIVE.md`](handoff-app-build-dogfood-ARCHIVE.md).** The size ratchet refuses a doc that is over budget and growing; eviction is how a round lands without losing the eliminations a future session would otherwise re-derive. Read the archive before re-running any probe this arc already ran:

- ✅ EXPLAINED 2026-09-21 — glm never wrote code, and "finished" was a truncation
- ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- ✅ FIXED 2026-09-25 — the page-money scaffold shipped a build that could not pass
- ✅ FIXED 2026-09-25 — all three harness refusals in one trial were wrong
- ✅ SHIPPED 2026-09-25 — the submission cap charged attempts, so a CLI-refused submit spent the budget
- ✅ SHIPPED 2026-09-25 — the oracle answered no host resource pick; a picker-gated app could never pass
- ✅ FIXED 2026-09-25 — the oracle graded only the consented viewer; a live app broken for every new user shipped green
- ✅ MEASURED 2026-09-26 — rank 15's premise was backwards, and so was my first account of why


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

### ⚠ NOT NEW 2026-09-26 — the page-money `npm install` crash is already diagnosed and remedied in this repo
- as-of: 2026-09-26

🔴 **I WROTE THIS UP AS A DISCOVERY AND IT IS NOT ONE. Round 0 of `/audit-pr 718` found the
prior art; I verified it myself.** `.github/workflows/ci.yml` carries a comment above its
`actions/setup-node` — repeated at **four** sites — naming the same crash
(`Cannot read properties of null (reading 'edgesOut')`), the same `vitest → jsdom → canvas`
chain, the same measurement class (*"npm 10.9.9 fails on the unmodified template, npm 11.19.0
succeeds; overriding jsdom or pinning vitest does NOT fix it"*) and the decided remedy:
**node 24 (npm 11), "Do not drop back to 22."** `gh issue #530` (closed 2026-09-09) is the
same crash again. **"Remedy undecided" was wrong.**

- **The one genuine residual, and it is a single line in three files:**
  `scripts/dogfood/envs/node-root.Dockerfile`, `node-user.Dockerfile` and
  `stale-cli.Dockerfile` are all still `FROM node:22-bookworm-slim`, so the **trial images
  never got the fix CI got**. Whether they should is a real fork — a trial arguably *ought*
  to measure a stock developer environment — but it is a question with documented prior art,
  and it belongs in an issue in #530's lineage, not here. `via: measurement`
- **What my run adds, and it is small:** an override pinning `@vitest/browser-playwright`
  to **5.0.1 still crashes** while **5.0.2 installs** — so 5.0.2 is a *fix*, not the trigger
  I predicted. Consistent with ci.yml's *"pinning vitest does not fix it"* (pinning a
  different package, at a different level). **`page-vite` installs cleanly** on the same npm;
  only `page-money` is affected. `via: measurement`
- **`build.sh` pins npm 11.19.0 in the FIXTURE containers** — the version ci.yml measured
  good. That pin is the **house convention**, not the deviation an earlier draft of this
  block called it.
- **Next probe:** none here. The open question is the env Dockerfiles' node major, and it is
  the operator's call.

### 🔴 OPEN 2026-09-26 — the frozen condition is ONE CELL SHORT, and the "drop" that closed it was never the operator's
- as-of: 2026-09-26

🔴 **THIS REOPENS THE ARC.** The close-check that produced it was asked for by the
operator: *"anything left outstanding from this arc? Find all sessions associated
with this handoff and check my messages then determine if all addressed shipped
and closed out"*.

- **Observed (with values):** the frozen condition reads *"Graded per cell across
  the three cheap models, **with a frontier control**."* The run caches
  (`~/.cache/dogfood-runs-2026-09-21`, `-09-25`, `dogfood-consent-controls`) hold
  only `ab-curve-01`, `ab-genpost-{glm,mimo,dsv4-01,dsv4-02}`,
  `ab-ship-mimo-{01,02}` and the three `ctl-*` fixtures. **No `claude` or `gpt`
  cell exists.** `driver.sh:21-23` still defaults `MODELS=(anthropic/claude-sonnet-5|claude
  openai/gpt-5.6-terra|gpt)`, so the frontier arm was configured and never run.
  `via: measurement`
- 🔴 **Ruled out — that the operator dropped it.** `extract_user_msgs.py
  --ids-file` over all four arc sessions (133 messages, 43 operator-authored)
  returns **exactly one** operator sentence containing "frontier", on
  `9d710f3c` 2026-09-21 15:56:51: *"rank 6: deepseek-v4-pro + a frontier control
  on the genpost brief … which is the only thing left before the frozen condition
  can be graded."* That is a REQUEST. A corpus-wide `find-session --all-time
  "frontier control"` returns only those same four sessions. `via: measurement`
- **Where the false claim came from:** `git log -S "frontier control was DROPPED"`
  → `4d4a45e` (PR #691). The operator's message for that PR is *"1. merge"*.
  `via: command`
- **Leading hypothesis:** an agent wrote the drop to close a ranked item, and the
  operator merged the PR without reading that sentence. No evidence of a decision.
- **Next probe:** none — this is a decision, not a measurement. Either run one
  frontier cell on the genpost brief (`DOGFOOD_MODELS='anthropic/claude-sonnet-5|claude'
  DOGFOOD_BRIEF_NAME=genpost DOGFOOD_TRIAL_PREFIX=fc bash driver.sh`, ~5–20× a
  cheap cell) **or** record the drop as an operator decision with a date. Until
  one of those, the arc cannot be graded.

### ⚠ OPEN 2026-09-26 — the arc's own opening question was never answered with a verdict the operator accepted
- as-of: 2026-09-26

- **Observed:** `f3f13433` 2026-09-20 18:42:00, re-sent 18:42:03 with the second
  clause appended: *"my goal of that arc was to validate that the onboarding flow
  was good enough for cheap open llms to be able to build apps. was that done? if
  so, how?"* No later operator message accepts a verdict. `via: measurement`
- **Why it matters:** every "closed" in this doc since is the doc talking to
  itself. The close-check is the frozen condition, and per the block above it is
  not met.
- **Next probe:** settle the frontier cell, then answer that question in one
  paragraph with the per-cell table, and put it in front of the operator.

### ⚠ OPEN 2026-09-26 — a dead link ships in BOTH scaffold templates
- as-of: 2026-09-26

- **Symptom + exact repro:** `curl -sL -o /dev/null -w '%{http_code}'
  https://developer.civitai.com/apps/responsive` → **404** (follows redirects;
  final URL unchanged). Control: `…/apps/reference/hooks.md` → **200** from the
  same probe, so the 404 is real and not a reachability fault. `via: measurement`
- **Observed — four shipped sites**, every one reaching a developer who scaffolds:
  `internal/scaffold/templates/page-money/README.md.tmpl:115`,
  `…/page-money/src/App.tsx.tmpl:168`,
  `…/page-vite/README.md.tmpl:104`,
  `…/page-vite/src/index.css.tmpl:55`.
  Also referenced in `internal/scaffold/query_prelude_guard_test.go:220` (a
  fixture string, not a shipped link). `via: command`
- **Ruled out — that the page moved.** `/apps/responsive.md`,
  `/apps/guides/responsive` and `/apps/reference/responsive.md` all 404.
  `via: measurement`
- **Ruled out — that the hosted-hooks instruction did not land.** The operator's
  *"it should use the hosted one, local index is just asking for drift"* DID land:
  `#702` deleted the local index and the scaffold points at
  `https://developer.civitai.com/apps/reference/hooks.md`, which is **200 with 18
  SDK-hook mentions**. Only the responsive guide is dead. `via: measurement`
- **Next probe:** decide whether the page should exist in
  `civitai/civitai-developer-docs` or the four links should be removed; nothing in
  this repo gates an external URL in a template.

## Next steps (ranked)

🔴 **Numbering frozen.** 1–16 as before; 17–20 added 2026-09-26 by the close-check.

⚠ **On `forcing:` — ranks 13–16 trace to the operator's 2026-09-25 feedback and
asks; 17–20 trace to the 2026-09-26 close-check the operator requested.** Every
one cites an EXTERNAL signal, and every item carries a real forcing kind.

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. ⚠ **RE-SCOPE OR RETIRE, do not work as written.** Its premise is measured
    false: `template-page-money` in `.github/workflows/ci.yml` runs on **every**
    `pull_request`, scaffolds a fresh page-money app and runs `npm run typecheck`
    → `npm test` → `npm run build` → `civitai app validate`; it ran green on
    #718's head. The real gap is only that the LOCAL `make ci` (Go-only) cannot
    see it, and the env-gated `TestPageMoneyScaffoldTypechecks…` is a slower
    duplicate of what Actions already does. Touches `civitai/cli` only.
    forcing: regression — narrowed to the local gate; the CI claim is withdrawn
13. ✅ **DONE 2026-09-26** — `v0.1.1` approved `23:30:15.974Z` and verified at the
    consumer (HTTP 200; both scope patterns ×2 in the deployed bundle).
    forcing: user — satisfied
14. **T1 "ship-ready" needs a decision, not code.** The publish floor requires an
    icon and a cover; the trial container has **no `python3`, `convert`, `magick`
    or `pip3`**. Adding image tooling or letting a brief spend Buzz on a cover
    **changes what "blind" means**. ⚠ `ab-img-poster` DID reach live, so something
    satisfied the floor — the transcript does not say what or who.
    forcing: user — asked about "complete working apps" on 2026-09-25
15. ✅ **DONE 2026-09-26** — `scripts/dogfood/fixtures/consent-controls/`.
    ⚠ Premise was backwards on both halves; see the 2026-09-26 retraction block.
    forcing: gate — satisfied
16. ✅ **SUBSTANTIALLY DONE** — the retraction is in place and `"2 of 3 passed"`
    carries its DEFAULT-arm caveat. What remains is whether historical blocks get
    retro-qualified; low value. forcing: user — satisfied
17. 🔴 **Settle the frontier control — run one cell, or record the drop.** This is
    the ONLY thing between this arc and a graded verdict. `DOGFOOD_MODELS='anthropic/claude-sonnet-5|claude'
    DOGFOOD_BRIEF_NAME=genpost DOGFOOD_TRIAL_PREFIX=fc bash scripts/dogfood/driver.sh`.
    Touches `civitai/cli` only. **IN FLIGHT: nothing.**
    forcing: user — the operator asked for it once and never withdrew it
18. **Merge #717, then rebase and merge #718.** They conflict (`merge-tree` exit
    1, three regions). #718 is mid-ladder: round 6 found a forgeable `outputDir`
    vector. **IN FLIGHT: `civitai/cli`#717, `civitai/cli`#718.**
    forcing: gate — an open PR carrying an unmerged audit ladder
19. **Harden `oracle.sh`'s manifest-derived fields.** `:318` and `:370` print
    `OUTDIR`, `SCOPES`, `APP_DIR`, `SERVED` with a raw `%s` from the block's own
    manifest, and `jq -r` turns a `\n` in any of them into a real newline. That is
    the single upstream cause behind three defeated reads in #718's ladder. ⚠ Its
    output is parsed by `grade.sh` and the Go suite — changing the rendering is
    its own change. Touches `civitai/cli` only.
    forcing: security — app-controlled data is spliced into a grader's input stream
20. **Fix or remove the four `apps/responsive` 404 links.** See the investigation
    block. Touches `civitai/cli` (templates) and possibly
    `civitai/civitai-developer-docs`.
    forcing: regression — a dead link ships to every developer who scaffolds

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

- 🔴 **A BRIEF CAN SHIP A BYPASS, AND MINE DID.** I specified the slug-flag allowlist as
  `{--slug}` from a literal grep. `--from` takes *a published app slug*; `--name` is
  slugified into the blockId; `--dir` selects the manifest. Implemented as briefed,
  `--from=some-other-app` would have been an unguarded path to a foreign published app.
  **When briefing a change to a security check, require the implementer to DERIVE the set
  from source and guard it — never hand them a set you grepped.** The seam guard is what
  made this survivable; the fix alone would not have.
- 🔴 **A CAP'S FAILURE DIRECTION IS THE WHOLE DESIGN.** The intuitive submission-cap fix
  fails OPEN on a live publishing path. Ask *"if the thing I depend on changes, do I
  over-refuse or under-refuse?"* before choosing the mechanism.
- 🔴 **`go test ./...` GREEN IS NOT `gofmt` GREEN.** `#708` was reported locally green and CI
  reddened on one unformatted file, taking `build-test` **and** `lint` with it. Two
  different claims, reported as though one covered the other.
- 🔴 **A `-run` FILTER THAT MATCHES NOTHING PRINTS `ok`,** and `tail -20` on exactly 20 lines
  silently dropped the root package — the real-browser suite — from a merged-tree run I was
  about to call green. **Count what ran; name the package.**
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
- ⚠ **A SUBAGENT STOPPED WITHOUT REPORTING TWICE, AND THE WORK WAS FINE BOTH TIMES.** Ask for
  the report; do not re-derive the work.
- ⚠ **REBASE BEFORE MERGING HERE, ALWAYS.** Four PRs this session were green on a base that
  had moved. Zero file overlap is not safety; the merged-tree suite is.

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

### Added 2026-09-26 — the close-check, and what reading the operator's own words changed

- 🔴 **A HANDOFF DOC IS NOT EVIDENCE ABOUT ITS OWN ARC'S CLOSURE.** This session
  opened by reading the doc's *"THE FROZEN CONDITION WAS CLOSED AND STAYS
  CLOSED"* and reporting the arc ADDRESSED. That was wrong, and the correction
  came only from reading the 133 messages the operator actually typed. **A
  doc-derived close verdict is a claim the doc makes about itself.**
- 🔴 **AN AGENT-AUTHORED "OPERATOR DECIDED X" SURVIVES INDEFINITELY, BECAUSE
  MERGING A PR IS NOT DECIDING WHAT IS IN IT.** The drop entered via a PR the
  operator merged with *"1. merge"*. **When a doc attributes a decision to the
  operator, the check is `extract_user_msgs.py`, not the doc's confidence.**
- 🔴 **`--arc` COULD NOT RESOLVE THIS DOC, AND EXIT 3 IS NOT AN EMPTY ARC.**
  `find-session.py --arc` / `extract_user_msgs.py --arc` resolve only against
  `$DEVRC/$HOMELAB/$DATAPACKET/$CIVITAI`, and `$CIVITAI` is `civitai/civitai` —
  the doc lives in `civitai/cli`, which has **no handle**. Exit 3 means *nothing
  was measured*. Fall back to `find-session.py --all-time <slug>` and feed the ids
  to `extract_user_msgs.py --ids-file`.
- ⚠ **A KEYWORD SESSION SEARCH PULLS IN OTHER ARCS.** `1186691e` matched on
  "unconsented" (playable-collections) and `df150f26` on "ab-img-poster"
  (app-platform-migration). Both excluded by reading their first message. Confirm
  membership from the kickoff line, not the match.
- 🔴 **A VERIFICATION STEP THAT MUTATES IS NOT A VERIFICATION STEP.** `civitai app
  listing status` *opens* a revision draft on a LIVE listing — so checking
  "is a shadow revision still open on `panorama-360`?" CREATES one. Left unprobed
  deliberately; the answer is not worth manufacturing the condition.
- ⚠ **`extract_user_msgs.py` output is ~2/3 harness noise here** — 90 of 133
  headings were injected `task-notification` blocks, only 43 operator-authored.
  Budget for that ratio before reading, or delegate the read.

### Added 2026-09-26 — six audit rounds on #718, and the one pattern that repeated

- 🔴 **IN ALL SIX ROUNDS THE DEFECT WAS THE EXPLANATORY COMMENT, NOT THE CODE.**
  A four-site false headline; a count taken from the wrong column *inside the
  paragraph retracting that error*; a "this hole is closed" about a hole one
  layer upstream; an anchor claim the regex did not support; a `%q` guarantee that
  does not hold in ANSI-C form; and "the first marker is always the oracle's".
  The code was fine each time.
- 🔴 **TWO FIXES INVERTED EACH OTHER.** Round 4's unscoped JSON read MASKED a case
  round 3's regex CAUGHT; round 5's scoped read MASKED a case round 4 CAUGHT. When
  a guard's third fix re-breaks the first, **stop picking and find the invariant**
  — round 6 counts assertion-bearing objects and requires exactly one, which no
  position-based pick can be defeated into.
- 🔴 **`${have:+a}${have:-b}` EXPANDS BOTH WHEN THE VAR IS SET.** `:-` yields the
  VALUE, not the fallback, so the digest was spliced into the sentence. The EMPTY
  branch read correctly, which is why it survived review.
- ⚠ **TWO OF MY OWN MUTANTS DIED FOR THE WRONG REASON** and would have read as
  passes: a planted `echo x` was invalid TypeScript so the BUILD aborted before
  the guard ran, and a `sed`-built replica had a syntax error. **Confirm a mutant
  fails with the GUARD's specific message, not merely non-zero.**
- ⚠ **`cmd | tail -8; echo $?` reports `tail`'s status.** It printed `EXIT=0` over
  a guard that had just fired correctly.
- ⚠ **A shellcheck/typecheck "clean" is worthless without watching it go red.** My
  first negative control returned rc 0 on a file I expected to fail.
- 🔴 **PRE-ASSEMBLING THE NEXT ROUND'S BRIEF BEFORE POSTING THIS ROUND'S BLOCK
  ANCHORS IT ON THE WRONG ROUND.** `audit-dispatch.py` warned on stderr that the
  newest block said `round=4` while I asked for 6. Post the block first; read
  stderr.
- ⚠ **The npm/arborist `edgesOut` crash is NOT a discovery** — `ci.yml` carries it
  at four sites with the decided remedy (node 24 / npm 11, *"Do not drop back to
  22"*), and `gh issue #530` is the same crash. Before writing up an environment
  break, grep the repo's CI config for the error string.

## How to verify

**The arc's actual closing condition** — one command, and it is the open item:
```bash
# has a frontier cell EVER run? (empty output = the condition is one cell short)
ls -d ~/.cache/dogfood-runs-*/*/ 2>/dev/null | grep -E '/(t|ta|fc)-(claude|gpt)' || echo "NONE — frozen condition NOT met"
```

**The live app** (rank 13, closed — re-run to confirm it stays closed):
```bash
curl -s -o /dev/null -w '%{http_code}\n' https://ab-img-poster.civit.ai/     # 200
JS=$(curl -s https://ab-img-poster.civit.ai/ | grep -oE '/assets/[A-Za-z0-9._-]+\.js' | head -1)
curl -s "https://ab-img-poster.civit.ai$JS" | grep -oE 'scopes:\["(ai:write:budgeted|posts:write:self)"\]' | sort | uniq -c
# expect 2 of each
```

**The consent controls** (#718). 🔴 Read the `render_reason`, not the verdict —
two rows legitimately print `RENDER=no` and the failures mean opposite things:
```bash
CC=/home/zach/workspace/civit/cli/scripts/dogfood/fixtures/consent-controls
bash "$CC/build.sh"                                       # ~5 min; --keep to reuse
nix-shell -p chromium --run "bash $CC/grade-controls.sh"  # exit 0, six rows
```

**The 404:**
```bash
curl -sL -o /dev/null -w '%{http_code}\n' https://developer.civitai.com/apps/responsive        # 404
curl -sL -o /dev/null -w '%{http_code}\n' https://developer.civitai.com/apps/reference/hooks.md # 200 (control)
```
## Defects (batched)

- 🔴 **`https://developer.civitai.com/apps/responsive` is a 404**, shipped at four
  template sites (see the 2026-09-26 investigation block). Control: `hooks.md`
  returns 200 from the same probe.
- **`scripts/dogfood/envs/*.Dockerfile` are all still `FROM node:22-bookworm-slim`**,
  so the trial images never got the node-24 fix CI took for the page-money `npm
  install` crash (`.github/workflows/ci.yml`, four sites; `gh issue #530`).
  Operator call: a trial arguably *should* measure a stock developer environment.
  **Not an undiagnosed crash.**
- 🔴 **`panorama-360` has an open shadow revision** an agent created on a LIVE
  listing of the operator's. **DO NOT PROBE IT TO CHECK** — `civitai app listing
  status` *opens* a revision draft on a live listing, so the check creates the
  thing it checks for. Operator-only; there is no `discard-revision`.
- **`cli#665` is OPEN** (verified 2026-09-26) — 2 of 4 trial environments
  (`node-user`, `ubuntu-apt`) were never gradeable.
- **"Confusion" was never graded.** The operator asked for *"steps, confusion, and
  token efficiency"* (`9d710f3c` 2026-09-25 15:05:04). Steps and tokens were
  measured; nothing grades confusion.
- **A possibly-missing second feedback item.** The 09-25 consent report is
  numbered **"1."** with a single bullet and no "2." (`9d710f3c` 21:49:36).
- **Ten `/audit-pr` offers went unanswered** — #679, #681, #683, #685, #686, #687,
  #704, #705, #708, #712. The repo's stated pre-merge convention was skipped on
  ten merged PRs.
- **The browser-flake closing condition is passive and unchecked** — *"if `BROWSER
  LAUNCH RETRY` appears in `build-test` runs after merge, the launch is still
  sick."* Nobody has looked.
- `pickerBlind` and `consentBlind` have never been tested together on one
  doubly-blind bundle.
- `CONSENT_SETTLE_MS = 1200` is a judgement; no measured bound for an app that
  asks after an `await`.
- Nothing is known about the **iframe** transport; all of this is the inline path.
- `c=civitai; $c app submit` is not refused, at base or HEAD (pre-existing).
- The submissions listing caps at 100 rows with no cursor, and the account is AT
  the cap.
- `civitai app submit` **exits 0 when it did not submit** (no token).
- `end.usage.cost` reports a bare `0.0` on an unpriced run — deferred to the
  operator and never answered.
