# ARCHIVE — handoff-app-build-dogfood

Closed investigation blocks evicted from `handoff-app-build-dogfood.md` so the
live doc stays under the size ratchet. **Nothing here was deleted or edited** —
each block is byte-identical to the copy the doc carried, including its
`as-of:` stamp. These are RESOLVED: read them to avoid re-deriving an
elimination, not as current state.

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

