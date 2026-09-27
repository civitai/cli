# ARCHIVE — handoff-app-build-dogfood

Closed investigation blocks evicted from `handoff-app-build-dogfood.md` so the
live doc stays under the size ratchet. These are RESOLVED: read them to avoid
re-deriving an elimination, **not** as current state.

🔴 **Two provenances, and the distinction matters when you cite one.** The seven
blocks under *Evicted 2026-09-26* that the live doc **did** carry are
**byte-identical to its copies**, `as-of:` stamps included — asserted
mechanically at eviction time, not by eye, and the eviction script refuses rather
than writes if any copy differs. ⚠ The remaining blocks (*rank 15's premise was
backwards*, and everything under *Evicted 2026-09-27*) were written **straight
into this file**; the live doc never carried them, so "byte-identical to the copy
the doc carried" does not apply — there is no such copy.

🔴 **Do not quote a block count from this preamble** — blocks get appended.
Derive it: `grep -c '^### ' claudedocs/handoff-app-build-dogfood-ARCHIVE.md`.

## Evicted 2026-09-26

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

### ✅ MEASURED 2026-09-26 — rank 15's premise was backwards, and so was my first account of why
- as-of: 2026-09-26

🔴 **THIS CLOSES RANK 15, AND IT RETRACTS A CLAIM I SHIPPED IN FOUR PLACES WHILE CLOSING IT.**
"An untouched scaffold fails the consent arm" is **true but vacuous**, and the reason is
not the one the rank assumed.

🔴 **RETRACTED — "the arm's `no` had never been watched arrive for the arm's OWN reason."**
`cli#718` led with that sentence in `build.sh`, the fixture README, the `scripts/dogfood/README.md`
block and the PR body. **It is false, and it was false when written — this document contradicts
it two screens up.** `ab-ship-mimo-02` (the live `ab-img-poster v0.1.0`) already graded
`RENDER=no observed=ready>generating>ready` — *"spent without asking"* — against `v0.1.1`'s
`RENDER=yes observed=ready`; `ab-genpost-dsv4-02` and `ab-ship-mimo-01` also grade `no` on the
arm; and the **How to verify** section below runs precisely that cell, calling the pair "the
point". Caught by round 0 of `/audit-pr 718`, which read the table I did not re-read.
**What was actually missing is narrower:** not *observation* of the arm's own `no`, but
**attribution** — a pair differing in ONE controlled variable rather than a version bump — and
**reproducibility**, since the seven fixtures are preserved containers and these rebuild from
git. That is the whole increment. Do not re-inflate it.

- **Ruled out — that `glm-01` failed for a render reason only because it was never
  built.** `ctl-scaffold-untouched` is a `civitai app init --template page-money`
  scaffold **installed AND built** — strictly further along than `glm-01`'s — and it
  fails **both** arms with the byte-identical `render_reason=timed out after 15000ms
  waiting for [data-testid="prompt"] to appear` and `observed=''`. Building changes
  nothing: page-money ships `pm-*` testids and its own UI, so it can never reach the
  genpost assertion's consent step. `via: measurement`
- 🔴 **And the intuition is INVERTED — the scaffold gets consent RIGHT.** Its `App.tsx`
  `proceed()` gates on `hasBudgetedScope(token.scopes)` and calls
  `requestConsent({ scopes: ['ai:write:budgeted'] })` — the exact pattern
  `ab-img-poster v0.1.1` was fixed *to*. **The apps that fail this arm failed by
  REPLACING the scaffold's generation path, not by keeping it.** `via: code`
- **The control that works — a scaffold-derived pair differing in ONE file.** Both reach
  the Generate click; `build.sh` asserts the one-file delta and exits 2 otherwise.

  | fixture | consented | unconsented | unconsented reason |
  |---|---|---|---|
  | `ctl-scaffold-untouched` | `no` | `no` | render — identical on both arms |
  | `ctl-genpost-blind` | `yes` | **`no`** | *"never asked the host for consent … Generate was clicked and sent none"* |
  | `ctl-genpost-asks` | `yes` | `yes` | — |

- 🔴 **THE ATTRIBUTION IS MEASURED, NOT ARGUED.** On the unconsented arm both fixtures
  report `generateClicked=true`, `generateDisabled=false`, `initialStatus=ready`,
  `postDisabled=true` — every render precondition passed on BOTH sides, so the verdicts
  can differ only on the consent axis. `blind` carries `hostRefused=REQUEST_TOKEN,
  ESTIMATE_WORKFLOW` (it went for the money path unconsented) against `asks`'s
  `REQUEST_TOKEN` alone. `via: measurement`
- 🔴 **`asks` IS THE POSITIVE CONTROL FOR `blind`'s `no`.** `messageShim=sites=1` on both
  means the instrument was wired into each bundle, and `asks` proves an ask **is
  observable on this exact SDK build** — which is what separates a real `no` from the
  `consentBlind()` case the assertion reports as `unmeasured` (`unmeasured=false` on
  both). Without that half the negative control would be the very thing this arc keeps
  finding: a control that cannot produce the discriminating input. `via: measurement`
- **The harness was validated in both directions before any verdict was quoted.**
  `build.sh`'s twin-drift guard was mutation-tested — a planted `src/drift.ts` in one
  twin gives **exit 2** with its own message naming the file; removed, **exit 0** and
  `twins verified`. The whole matrix then reproduced byte-for-byte from a clean
  script-built set. ⚠ The first exit-code read was wrong (`… | tail -8; echo $?` reports
  `tail`'s status — gotcha #8) and said `EXIT=0` over a guard that had just fired.
- **Next probe:** none. `fixtures/consent-controls/` is the durable artifact; the
  containers are evidence and the runs are at `~/.cache/dogfood-consent-controls/`.


## Evicted 2026-09-27

### ✅ MEASURED 2026-09-27 — what `v0.1.2` actually changed, and the control that refuted my first answer
- as-of: 2026-09-27

🔴 **THIS CLOSES "what did `v0.1.2` fix", AND IT RETRACTS MY OWN FIRST ANSWER TO IT — caught by a
control, not by review.** Written while reconciling `#718`; the live-doc summary is in *State now*.

- **Observed (with values), three revisions of the same app:** `v0.1.0` =
  `dogfood-ab-ship-mimo-02` (`index-CkrWgKXb.js`); `v0.1.1` = the image
  `dogfood-fixture/ab-imgposter:v0.1.1-pre-post-fix` (`index-CJh51XL4.js`, md5
  `b8a1936e5939e39aa3e26f6e9e777113`, manifest `"version": "0.1.1"`); `v0.1.2` = the LIVE bundle
  `index-CcmcNwgc.js` (md5 `c603fa854eba4d5206b73e04c14a1f3c`), byte-identical to
  `dogfood-ab-imgposter-fixed`, whose manifest reads `0.1.2`. `via: measurement`
- 🔴 **RETRACTED — "`posts:write:self` 4 vs 1, so the POST path is where `v0.1.2` moved."** I read
  that count against the `v0.1.0` control only, and it looked decisive. **The `v0.1.1` snapshot
  reads 4 as well** — as it does for budgeted-scope (4/4) and `requestConsent` (2/2) — so all
  three counts separate `v0.1.1`+`v0.1.2` from `v0.1.0` and **none** of them discriminates the
  revision I was trying to characterise. This is this doc's own lesson landing on its author:
  *pick a string the NEW content introduces, never the identifier the work has been discussing
  all along.* **The fix was to find the control that brackets the change, not to reason harder.**
  `via: measurement`
- **The discriminating read — failure-message vocabulary.** `"Please try again."` sites go
  **3 → 6**, and the new tokens are reads of an error's `code` / `message` / `status`:

  | string | v0.1.0 | v0.1.1 | v0.1.2 |
  |---|---|---|---|
  | `Generation failed. Please try again.` | 2 | 2 | 2 |
  | `Posting failed. Please try again.` | 1 | 1 | **2** |
  | `Generation was refused. Please try again.` | 0 | 0 | **1** |
  | `That generation expired before it finished.` | 0 | 0 | **1** |
  | `so there is nothing to post` | 0 | 0 | **2** |
  | `still connecting to Civitai` | 0 | 0 | **1** |
  | `lost its Civitai session` | 0 | 0 | **1** |
  | `may already have been created` | 0 | 0 | **1** |

  So the generic post failure the user reported gains a **second distinguishable branch**, plus
  five new specific messages — including *"Your post may already have been created — check your
  Civitai profile before trying again."* `via: measurement`
- **Ruled out — that `v0.1.2` added a capability or a scope.** The manifest scope set and the
  `requestConsent` call count are unchanged from `v0.1.1`; only message strings and error-field
  reads differ. `via: measurement`
- ⚠ **NOT VERIFIED — that a real user reported *"Posting failed. Please try again."*** The string
  is present in `v0.1.0`/`v0.1.1` at **1** site, which is consistent with the report, but the
  report itself is operator-relayed and this session had no access to it. Do not upgrade it.
- 🔴 **THE STRUCTURAL POINT, and it is the third instance: the oracle cannot see this class at
  all.** `InlineTransport` rejects every request, so the app's *error* branch is the only one the
  rig ever reaches; a change that makes failures legible cannot move `RENDER=`. Two user-found
  defects (`v0.1.1` generate, `v0.1.2` post) now sit in the gap between "the oracle passes it" and
  "a person can use it". See *Gotchas* → **Added 2026-09-25 (feedback)**.
- ⚠ **A METHOD TRAP worth carrying:** a greedy `grep -o '"[^"]\{6,70\}...[^"]\{0,40\}"'` over a
  267 KB **single-line** minified bundle backtracked past a 120 s timeout and returned nothing —
  which reads as "the string is absent". A double-quote-only extractor then **missed
  `Posting failed. Please try again.` entirely**, and had I stopped there I would have reported it
  absent from every revision. **Count exact needles in Python; never infer absence from a regex
  sweep over minified output.**
- **Next probe:** none. The three bundles are preserved; the two images are in the do-not-destroy
  set.

## Evicted 2026-09-27 (resume)

🔴 **These four blocks were carried by the live doc and are byte-identical to its copies** —
asserted mechanically at eviction time, not by eye. They are 2026-09-21 detail from a closed
phase; the live doc keeps *Added 2026-09-21 (close) — what this arc actually taught*, which
summarises them, plus a pointer here.

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

