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

🔴 **`ab-img-poster v0.1.1` IS LIVE AND THE FIX IS VERIFIED IN THE BYTES THE SERVER SENDS.**
The previous revision of this doc recorded it as *pending operator review*; it was approved
`2026-09-25T23:30:15.974Z`, deployed, and confirmed at the consumer rather than at a status
field:

| check | result |
|---|---|
| `https://ab-img-poster.civit.ai/` | **HTTP 200** |
| deployed bundle re-fetched over the wire | HTTP 200, **266,508 B** |
| `scopes:["ai:write:budgeted"]` in the DEPLOYED js | **2** |
| `scopes:["posts:write:self"]` | **2** |
| **control** — same pattern in the old `v0.1.0` bundle | **0** |

The control is what makes that a reading: the pattern now present twice was **entirely
absent** from the build the operator originally hit. `v0.1.0` and `v0.1.1` are both
`approved`; `v0.1.1` is the serving one.

🔴 **WHAT IS STILL NOT VERIFIED, AND ONLY A HUMAN CAN DO IT.** The **auto-resume** —
generation starting by itself after the viewer grants consent — depends on the host's
`TOKEN_REFRESH` push. `InlineTransport.onMessage` returns a no-op unsubscribe, so that push
is **structurally undeliverable in the oracle**: no arm, present or future, can observe it.
The operator clicking Generate on the live app is the only available verification.

- ✅ **Rank 13 is CLOSED** — its closing condition (`civitai app status --json` shows
  `v0.1.1` no longer `pending`) is satisfied.
- **Account:** Buzz **4,074,196** unchanged; `pending: 0`. Eight `dogfood-*` containers plus
  the snapshot image `dogfood-fixture/ab-ship-mimo-02:pre-consent-fix`.
- ⚠ **`main` has moved under this arc twice since `#714`** (`a4307f5`, `58f98d5`, both other
  sessions). **Rebase before merging anything here** — four PRs this session were green on a
  base that had already moved.
- ⚠ **No `clawgate-task:`** — resolve returned 0 **with a positive control** (the same
  endpoint answered another session's link), so it is a real reading of the board.
- **Claims:** none held.

### Carried forward — durable values a `State now` replace would otherwise eat

🔴 **THE FROZEN CONDITION WAS CLOSED 2026-09-21 AND STAYS CLOSED**: 2 of 3 cheap models built
a passing App Block; `glm` failed, explained. Two permanent limits, **neither a to-do**: the
**frontier control was DROPPED by operator decision**, and **every cell is ONE environment**
(`cli#665`). ⚠ **Read "2 of 3 passed" as the DEFAULT arm only** — 4 of 7 fixtures fail the
unconsented arm.

🔴 **BUILD-AND-SHIP IS ANSWERED, AND NOW WITH A WORKING APP AT THE END OF IT.** Built blind
by `xiaomi/mimo-v2.5` for **$0.0170**, submitted, approved, deployed, HTTP 200 — then found
broken by a real user, fixed, resubmitted as `v0.1.1`, re-approved and re-verified. **The
word "working" is earned only at `v0.1.1`, and only for the paths the oracle can see.**

🔴 **RANK 3'S TABLE** (the arc's only record of the step/cost curve): mimo-v2.5, celsius,
**65 steps / $0.0145 / 1,564,496 prompt tokens / per-call 452 → 45,328 / `stop=finished`**,
caps 80/$0.50.

🔴 **BASELINES DRIFT — RE-READ, NEVER QUOTE.** Buzz 4,101,822 (09-21) → 4,074,196 (09-25).
The submissions listing is **at its 100-row cap, no cursor**.

🔴 **COST IS DEAD AS A CROSS-DAY INSTRUMENT** — 2.15× drift in $/1k on the same model and
task in four days. Grade on steps, prompt tokens and behavioural signals.

🔴 **EIGHT FIXTURES + ONE SNAPSHOT — DO NOT DESTROY.** `dogfood-ab-genpost-glm-01` is the
negative control; `dogfood-ab-imgposter-fixed` holds the fixed app with a synthetic trial dir
at `/tmp/fixed-runs/ab-imgposter-fixed`. Evidence lives OUTSIDE every worktree:
`~/.cache/dogfood-runs-2026-09-21/`, `~/.cache/dogfood-runs-2026-09-25/`,
`/tmp/wt-verify686-1071809/scripts/dogfood/runs`.

**The credentialed precedent:** `claudedocs/handoff-dogfood-3.md` — withdrawing a
**first-version** submission destroys captioned media permanently.

## Open investigations — live diagnosis state

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

### ✅ SHIPPED 2026-09-21 — `cli#690` seeds the manifest's scopes; the cheap arm is 2 of 3
- as-of: 2026-09-21

🔴 **This CLOSES two now-PRUNED blocks — "🔴 OPEN — the oracle seeds a signed-in viewer
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

### ✅ FIXED 2026-09-25 — the page-money scaffold shipped a build that could not pass
- as-of: 2026-09-25

- **Symptom + exact repro:** `npm run build` (= `tsc -p tsconfig.json --noEmit && vite
  build`) failed on the scaffold's **own** shipped test files:
  `src/App.test.tsx(1,18): error TS2305: Module '"@testing-library/react"' has no exported
  member 'screen'` — plus `fireEvent` and `waitFor` in `e2e.test.tsx` and
  `responsive.test.tsx`.
- **Observed (with values):** all four failing files carry the scaffold's **creation mtime**
  (14:52:09); only `App.tsx` was agent-written (14:56:51). `@testing-library/react`'s types
  re-export those three names from `@testing-library/dom`, which the template **did not
  declare**. Cost: steps 29–39, **11 of 43 steps (26%)**, fighting code the agent never
  wrote. Hit independently by glm (2026-09-21) and deepseek (2026-09-25).
  `via: measurement`
- 🔴 **Ruled out — that this breaks EVERY install. It does not, and the stronger claim was
  MINE.** Measured both shapes at base: plain `npm install` → **exit 0** (npm ≥7 auto-installs
  the peer); `npm install --legacy-peer-deps` → **exit 2, 7 × TS2305**. The trial only reached
  the broken shape because its *plain* install died inside npm's arborist
  (`Cannot read properties of null (reading 'edgesOut')`) and its next command was
  `--legacy-peer-deps`. **The defect is install-shape-dependent.** `via: measurement`
- **Fixed in `cli#702`** (`dcf9da2`): `"@testing-library/dom": "^10.4.0"` added to
  `devDependencies`, resolved against all three sibling packages' peer ranges. The guard
  **really typechecks** a rendered scaffold and carries a positive control (`tsc --listFiles`
  must include the test files, so a clean verdict cannot be an artifact of `tsconfig`
  includes). Red at base with the exact 7 errors, green at HEAD.
- 🔴 **Still open, and stated rather than buried: CI DOES NOT RUN THAT GUARD.** It is gated on
  `CIVITAI_SCAFFOLD_TYPECHECK=1` because it needs network and ~6.5 s. `template-page-money`
  passing is **not** evidence the scaffold builds. An offline dependency-declaration guard
  runs in CI and is explicitly labelled as the weaker check.
- **Unexplained:** the npm arborist crash could not be reproduced. The fix makes it
  irrelevant, not diagnosed.
- **Next probe:** none. A future trial that reaches `--legacy-peer-deps` is the field test.

### ✅ FIXED 2026-09-25 — all three harness refusals in one trial were wrong
- as-of: 2026-09-25

- **Observed (with values), all three from `ab-genpost-dsv4-02`:**
  - **step 9** `app create civitai-image-generator --template static` — refused naming
    `'civitai-image-generator', 'static'`. Half right: the app name was wrong, `static` is
    the `--template` VALUE.
  - **step 11** `app create ab-image-generator --template static` — **refused naming only
    `'static'`**, with the remedy *"Rename the app and retry."* The app name was already
    correct. 🔴 **The agent abandoned `--template static` and took the 320 KB `page-money`
    default — so a false refusal silently chose the scaffold whose cost this arc measures.**
  - **step 25** writing its own `block.manifest.json` via heredoc — refused because the JSON
    **content** contained `civitai.com` (the `$schema` URL) and "generated" (inside a
    human-readable scope description). It then tried `python3`, absent from the container
    (rc=127) — a second step lost to the first refusal.
  `via: measurement`
- **Fixed in `cli#702`:** a `GATED_FLAGS` table gives every flag on a gated command a role —
  `slug` (its value is a prefix-cap candidate, both spellings), `value` (consumes its next
  token so it is not mistaken for a positional), `bool` (consumes nothing). Heredoc **bodies**
  are stripped before classification; opener lines, post-terminator text and inline redirect
  arguments are still classified. The refusal message now names the offending token's ROLE.
- 🔴 **THE ALLOWLIST IS `{--slug, --from, --name, --dir}` — AND A BRIEF THAT SAID `{--slug}`
  WOULD HAVE SHIPPED A BYPASS.** `--from` takes *"an existing published app slug"*, `--name`
  is slugified into the blockId when `--slug` is absent, `--dir` selects the directory whose
  manifest supplies the blockId. A literal grep for `"slug"` finds three `StringVar` sites and
  misses all three of those. `via: code`
- 🔴 **Ruled out — that narrowing the check reopened `#698`'s bypass.** Verified by direct
  `Caps.judge()` calls on the real commands: `--slug=some-other-app` refused, `--slug
  some-other-app` refused, `--dir=sensei` refused, `--template static` **allowed**, own-app
  attached slug allowed, wrong-prefix create refused, `eval "civitai app submit"` refused.
  `via: measurement`
- **Guarded by a seam guard, not a comment:** `TestDogfoodGatedFlagLedgerCoversTheCLI` derives
  the flag set from `internal/cmd` cobra source and fails in BOTH directions, with positive
  controls (≥15 names, ≥5 files; actual 25/7). Validated by **mutation**: adding an unledgered
  `--app-slug` to `app_create.go` turned exactly that guard red.
- 🔴 **`c=civitai; $c app submit` IS NOT REFUSED — and never was, at base or HEAD.** A comment
  in `runner.py` asserted it as a genuine fail-closed case; the comment was **false**.
  `segments()` splits on `;`, so segment 1 names the CLI without a dangerous verb and segment 2
  names a verb without the CLI. The comment is corrected in `#702`; **the gap is open.**
  `via: measurement`
- **Next probe:** none for the fixes. Closing the `c=civitai` gap is a separate decision — the
  module's own threat model says these caps stop the ordinary accident, not an adversary.

### ✅ SHIPPED 2026-09-25 — the submission cap charged attempts, so a CLI-refused submit spent the budget
- as-of: 2026-09-25

- **Observed (with values), trial `ab-ship-mimo-01`:** step 49 `civitai app submit` → the
  CLI refused **pre-flight** (*"refusing to submit without --yes in a non-interactive
  shell"*, exit 1, nothing contacted) — and `runner.py:851` incremented `sub` 0→1 anyway.
  Steps 50/51/54 (`--yes`, `--package-only`, `--yes`) were then all refused by the harness.
  Account confirmed 0 pending, 0 `ab-*`, Buzz unchanged. `via: measurement`
- 🔴 **The model recovered correctly and we blocked it** — it read the refusal and went to
  `--yes` on its very next step. That was the open question about whether a blind model
  would adapt; it did, immediately.
- **Fixed in `cli#705`** (`37e85ba`) — and 🔴 **the DIRECTION was the design decision.** The
  obvious fix (decide pre-flight: only charge a submit carrying `--yes`) **fails OPEN**: it
  makes the harness depend on the CLI's non-TTY refusal, so a config that auto-confirms or a
  TTY appearing would silently uncap **real** submissions. Shipped instead: charge
  unconditionally, **refund** only when the result proves nothing was contacted. A reworded
  CLI message then stops the refund and the harness over-refuses — the safe direction.
  `--package-only` is the one pre-flight exemption (contacts nothing by construction).
- **Ruled out — that the refund can return a REAL submission's charge.** Verified by driving
  `judge()→settle()→judge()` directly: a successful submit is not refunded and the next
  `--yes` is correctly refused. `via: measurement`
- **Next probe:** none. Proven live by `ab-ship-mimo-02`, whose log shows
  `verdict=run` → `verdict=refund` → `--yes` executing.

### ✅ SHIPPED 2026-09-25 — the oracle answered no host resource pick; a picker-gated app could never pass
- as-of: 2026-09-25

- **Observed (with values):** `ab-ship-mimo-02` graded `RENDER=no observed=ready
  generateDisabled=true`. Cause, from the app's own source: `openPicker({resourceType:
  'Checkpoint'})` at `App.jsx:36`, `disabled={… || !model}` at `:154`. The stub rejects
  every request and delivers no host pushes, so `model` stays null forever.
  `via: measurement` + `via: code`
- **Fixed in `cli#708`** (`d239fcd`): the oracle answers `OPEN_RESOURCE_PICKER` /
  `OPEN_CHECKPOINT_PICKER` and nothing else. Four control arms after the change:
  `ab-ship-mimo-02` **yes** · `ab-genpost-glm-01` **no** (negative control holds, verified
  independently) · mimo-01 and dsv4-01 **unchanged**.
- 🔴 **THE INVARIANT, REWORDED RATHER THAN QUIETLY BROKEN:** the stub no longer rejects
  *unconditionally* — it rejects everything but two **discovery** calls. `token.raw` stays
  empty and `ESTIMATE_WORKFLOW` was measured still-refused on three of four real arms.
  **A pick buys a BRANCH, never a capability.** Any future change here must re-assert that.
- 🔴 **A SILENT-FAILURE PATH WAS FOUND IN REVIEW AND CLOSED BEFORE MERGE.** The shim patches
  the SDK's **minified** stub by regex, and the first needle was wrong on the second real
  bundle (`blocks-react` 0.53.1 emits `Error(\`…\`)` with no `new`) — it patched **0 sites**
  while the cell stayed green, green only because that app needed no pick. The mirror case
  regrades the very defect being fixed: `sites=0` on an app that *does* pick → `RENDER=no`,
  attributed to the model. Now a pick the shim could not answer reports **`unmeasured`**
  (exit 2), never `no`, with a fixture that **defeats the needle on purpose** watched to
  fire, plus an over-refusal control. `via: measurement`
- ⚠ **Stated limit:** when the patch fails the shim is never called, so "a pick was
  requested" is **inferred** from the gate still being shut after prerequisite clicks — the
  signature of the harm, not the cause. An app needing a pick but *not* gating Generate on
  it still grades `no`. Unguarded, disclosed.
- ⚠ **Two bundles is not a general claim about minifier spellings.**
- **Next probe:** none.

### ✅ FIXED 2026-09-25 — the oracle graded only the consented viewer; a live app broken for every new user shipped green
- as-of: 2026-09-25

- **Symptom, from a REAL USER on the live app:** Generate → *"Generation failed. Please try
  again."*; consent had to be granted manually via "review permissions".
- **Observed (with values):** `App.jsx` generate path calls `estimate`/`submit` with no
  consent request; the catch has no missing-scope branch. `handlePost` used
  `requestConsent('posts:write:self')` — a bare string — and `await`ed a `void`
  fire-and-forget call before `createPost`. `via: measurement` + `via: code`
- 🔴 **Why the oracle could not see it:** `InlineTransport` rejects every request, so
  *"generation fails for everyone"* and *"generation works"* produce the identical trace
  `ready>generating>ready`; the assertion grades the status word. **And `#690`'s scope
  seeding actively masked it** — with scopes granted, an app that never asks looks identical
  to one that asks correctly. `via: code`
- 🔴 **Ruled out — that the oracle could answer a consent request.** `useRequestConsent`
  calls `transport.sendMessage({type:'REQUEST_CONSENT'})` — a **`sendMessage`, not a
  `sendRequest`** — and `InlineTransport.sendMessage` is an intentional v1 no-op, with
  `onMessage` returning a no-op unsubscribe. **A `TOKEN_REFRESH` grant is structurally
  undeliverable**, so neither "granted" nor "declined" is reachable and OBSERVING THE ASK is
  necessary *and* sufficient. That makes the arm strictly safer than the default one: it
  removes capability (`scopes: []`, `raw: ''`) and adds none. `via: code`
- 🔴 **Ruled out — rewording the briefs.** Measured by running the real `oracle.sh` over a
  reworded copy: **6 of 6** transcript-bearing fixtures go exit-2. Two of my premises were
  wrong — `ab-genpost-mimo-01` *also* resolves by text, and `brief_name` fixtures are **NOT**
  safe, because the self-consistency check compares recorded prose against the named file.
  **The requirement lives in the assertion; `arm=` labels every cell.** `via: measurement`
- **A mutation that SURVIVED round 1, and what it means:** widening `consentBlind` to
  `messageSites === 0` was invisible because no fixture had a bundle without an inline
  transport — an unreachable-guard case. A `fxNoSdkNeverAsksApp` fixture was added; the
  mutant then died to only that row, M0 still survived, and a delta re-sweep found nothing
  new. **The ladder ended on a clean round, not on a count.**
- **Next probe:** none for the instrument. The open item is whether an untouched scaffold
  fails this arm — see the limits below.

## Next steps (ranked)

🔴 **Numbering frozen.** 1–11 settled; 13 closed this session; 12, 14–16 carried.

⚠ **On `forcing:` — ranks 13–16 trace to the operator's 2026-09-25 feedback and asks.**

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. **Make CI able to see a broken scaffold build.** `TestPageMoneyScaffoldTypechecks…` is
    env-gated on `CIVITAI_SCAFFOLD_TYPECHECK=1`, so `make ci` cannot catch the defect that
    cost 26% of a trial. `template-page-money` passing is NOT that check. Touches
    `.github/workflows/ci.yml`, `internal/scaffold/page_money_typecheck_test.go`.
    forcing: regression — the defect shipped once and CI still cannot see it
13. ✅ **CLOSED 2026-09-25** — `v0.1.1` approved, live, and verified in the deployed bundle
    against the `v0.1.0` control. forcing: user — satisfied
14. **T1 "ship-ready" needs a decision, not code.** The publish floor requires an icon and a
    cover; the trial container has **no `python3`, `convert`, `magick` or `pip3`**. Adding
    image tooling or letting a brief spend Buzz on a cover **changes what "blind" means**.
    forcing: user — asked about "complete working apps" on 2026-09-25
15. 🔴 **THE SHARPEST OPEN ITEM, AND IT IS A GAP IN OUR OWN FIX: the unconsented arm has NO
    NEGATIVE CONTROL.** `ab-genpost-glm-01` fails it, but for an unrelated *render* reason —
    it never renders `[data-testid="prompt"]`, so the consent predicate never runs. **"An
    untouched scaffold fails the consent arm" is UNMEASURED.** Build a scaffold-derived app
    that reaches the Generate click and watch the arm fail it for its own reason. Touches
    `civitai/cli` (`scripts/dogfood/briefs/genpost.assert.mjs`,
    `dogfood_oracle_consent_test.go`).
    forcing: gate — an arm watched to pass and to fail, but never to fail for its own reason,
    is the exact defect class this arc kept finding
16. **Re-read every "the model built a working app" claim against the unconsented arm.** 4 of
    7 fixtures fail it, including cells previously cited as successes. The default-arm
    verdicts stand; the WORD "working" does not.
    forcing: user — the operator's feedback refuted the claim as written

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

## How to verify

**The live fix, at the consumer — the check that closed rank 13. Always include the control:**

```bash
curl -s -o /tmp/live.html -w 'page http=%{http_code}\n' https://ab-img-poster.civit.ai/
B=$(grep -o '/assets/index-[A-Za-z0-9_-]*\.js' /tmp/live.html | head -1)
curl -s "https://ab-img-poster.civit.ai$B" -o /tmp/live.js -w "bundle http=%{http_code}\n"
grep -o 'scopes:\["ai:write:budgeted"\]' /tmp/live.js | wc -l     # expect 2
docker exec dogfood-ab-ship-mimo-02 bash -lc \
  'grep -o "scopes:\[\"ai:write:budgeted\"\]" /work/ab-img-poster/dist/assets/index-*.js | wc -l'   # CONTROL: 0
```
🔴 **The control is the point** — a count of 2 means nothing without the 0 beside it.

**The defect and the fix on the arm that can see them:**

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
Expect **`RENDER=no`** (broken: `observed=ready>generating>ready`) then **`RENDER=yes`**
(fixed: `observed=ready`). 🔴 **On this arm a status machine that MOVES is the defect** — the
correct app stays at `ready` because it asks instead of spending.

🔴 **The auto-resume cannot be verified here** — it needs the host's `TOKEN_REFRESH`, which
`InlineTransport` cannot deliver. **A human clicking Generate on the live app is the only
check**, and it is the one that closes the user-facing loop.

**The account:**
```bash
civitai app status --json   # v0.1.1 approved/live; pending 0
civitai buzz                # 4,074,196 on 2026-09-25 — RE-READ, it drifts
```
## Defects (batched)

- `ab-img-poster v0.1.0` is **live and broken for first-time viewers** until `v0.1.1` is
  approved.
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
