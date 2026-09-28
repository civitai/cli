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

🔴 **TEN PRs MERGED, `v0.1.110` RELEASED TO npm + HOMEBREW, AND A LIVE APP FIXED — but the repo's
CI is RED for everyone right now and that is the first thing to deal with.** `$0` of model spend on
dogfood trials beyond **$0.031** across two ungradeable cells (see *Open investigations*).

**Merged and verified BY CONTENT on `origin/main`** (a squash merge never makes the branch an
ancestor, so ancestry is never the check). Tip is `3b98778`.

| PR | what | sha |
|---|---|---|
| `#735` | rank 20 — dead `apps/responsive` link + the liveness corpus that should have caught it | `6b5e381c7` |
| `#733` | rank 21 — the two app-controlled grader path reads (+ **7** injection sites) | `cf80b1d32` |
| `#734` | rank 22 — the post-path oracle arm | `0ab60e391` |
| `#732` | handoff | `329af2782` |
| `#737` | the T1 brief + floor grader | `aacfd71b5` |
| `#738` | `DOGFOOD_MAX_STEPS` pass-through | `5cdee3529` |
| `#739` | credential install VERIFIED + `civitai login` refused in a trial | `6c997049c` |
| `#741` | the false "pack ships no Slider" claim + `dev-token` states what it granted | `1dbe4295d` |
| `#740` | the auto-setup wizard surfaced + `--check` stops reporting `ok` over a broken state | `3b9877847` |

**`v0.1.110` is live on npm** (`npm view @civitai/cli version` → `0.1.110`, published
`19:46:05Z`). 🔴 **The release run reporting `success` is NOT the release** — `.goreleaser.yaml`
sets `draft: true` deliberately, and *publishing the draft* is what fires `release-npm.yml` and
`release-homebrew.yml` (both on `release: types: [published]`). The run was green with 14 assets
while npm still served `0.1.109`. Read the registry, never the run.

**`ab-img-poster` v0.1.3 is SUBMITTED and PENDING** — `pubreq_01M3K41081PXKNNHZK4S85X0ZX`,
submitted `2026-09-27 23:23 CDT`, confirmed server-side. **v0.1.2 (`index-CcmcNwgc.js`) is still
the serving revision** until v0.1.3 is approved, so nothing is broken while it waits. Source now
lives on the host at **`/home/zach/workspace/civit/ab-img-poster`** (v0.1.3, `node_modules`
installed); the v0.1.2 bundle stays preserved in the `dogfood-ab-imgposter-fixed` container as
evidence and was not touched.

**Two platform issues filed against `civitai/civitai`** (PUBLIC repo — no infra detail, no
credential, no operator quote; both bodies re-fetched and scanned with a positive control):
`#5181` the relative `CREATE_POST_RESULT.url`, `#5182` no generation metadata from a `workflow`
source and no way to ask for it. Both carry closing conditions; 16 duplicate queries over open AND
closed issues found none.

🔴 **`clawgate-task:` DELIBERATELY ABSENT.** `clawgate_handoff.sh resolve` printed
`NOTHING RESOLVED — 0 tasks`. ⚠ Its `rc` read `0` only because the status came through a pipe
(`| head`) — the TEXT is authoritative. An unknown session id answers 200 with an empty array, so
that zero cannot distinguish "touched no task" from "wrong id". Not a clean bill of health.

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

### 🔴 OPEN AND BLOCKING EVERYTHING — `schema-drift` is red repo-wide, and the automation that exists to fix it CANNOT
- as-of: 2026-09-28

- **Symptom + exact repro:** `schema-drift` fails on every open PR, including `#742`.
  `scripts/check-canonical-schema.sh` byte-compares the go:embedded mirror
  `schema/app-block.manifest.schema.json` against the schema published live at
  `https://civitai.com/schemas/app-block/v1.json`.
- **Observed (with values):** canonical now carries **4** `goods:` occurrences
  (`goods:read:self`, `goods:purchase:self`); `origin/main`'s vendored copy carries **0**.
  `via: measurement`
- **Ruled out — that this is `#742`'s defect.** `main`'s own `schema-drift` reads `success` at
  `2026-09-28T03:34:44Z`, i.e. BEFORE the upstream publish, so its green is stale rather than
  contradictory; `#742` touches no schema file and `check-canonical-schema.sh` gives byte-identical
  output at `#742`'s HEAD and at its base. `via: measurement`
- 🔴 **Ruled out — that the 6-hourly automation will heal it.** `revendor-canonical-schema.yml`
  (00:37/06:37/12:37/18:37 UTC, whose own header says that cadence *is* the bound on how long CI
  stays red) was dispatched manually as run **`36379098603`**: it **FAILED and opened no PR**. Its
  `validate — schema compiles + examples validate` step died on two tests in `internal/validate`:
  `--- FAIL: TestEnumFindingsKeepTheirExactWording` (`pattern_test.go:350`,
  *"enum finding on \"scopes[1]\" changed"* — it hardcodes the enum error message listing the
  allowed scopes, and the new schema legitimately adds two) and
  `--- FAIL: TestPatternRulesCoverTheVendoredSchema` (`pattern_test.go:449`,
  *"schema pattern \"^[a-z0-9][a-z0-9_-]*$\" has no glos…"*). `via: measurement`
- 🔴 **Leading hypothesis — it is SELF-BLOCKING, and that is the finding.** The guard that keeps
  the mirror in step is what prevents the mirror from ever being updated: it can never accept a
  canonical schema that adds a scope, an enum member or a pattern. Same shape as the base-clone
  refresh hook that worked exactly once per file and then silently stopped. So this does **not**
  self-heal at 06:37 — it stays red until a human fixes the two pins.
- ⚠ **`origin/automation/revendor-canonical-schema` is STALE, not the fix** — it sits on `4dd98b9`
  over `7c57d3a` and its vendored schema has **0** `goods:`. Do not merge it expecting a fix.
- ✅ **THE RE-VENDOR ITSELF IS ALREADY DONE AND PRESERVED.** A stopped agent left it uncommitted;
  I verified and saved it: **`~/.cache/schema-revendor-2026-09-28/app-block.manifest.schema.json.revendored`**
  is **byte-identical (`cmp`) to the live canonical** and carries the 4 `goods:` entries, alongside
  `canonical-as-fetched.json`. Both sha256'd. ⚠ The canonical URL is live and can move again —
  re-`cmp` before trusting the copy.
- **Next probe / the actual work:** fix the two pins, then commit the preserved schema.
  `TestEnumFindingsKeepTheirExactWording` is a legitimate user-facing **message-stability** guard —
  do not delete or loosen it; **derive the enum CONTENT from the vendored schema while pinning the
  message SHAPE**, and prove the shape half still bites by rewording the finding template and
  watching it fail. `TestPatternRulesCoverTheVendoredSchema` is **not** a test defect — it requires
  a human-readable gloss per pattern, so **add the gloss** for `^[a-z0-9][a-z0-9_-]*$` after
  finding which fields use it. Then sweep `internal/validate` for other hardcoded schema content
  and say whether the automation can now succeed unaided.
  🔴 **Do not merge anything through a red `schema-drift`** — that trains everyone to click through.

### 🔴 OPEN — rank 14's third T1 cell is blocked on a credential shape, not on code
- as-of: 2026-09-28

- **Symptom + exact repro:** two credentialed T1 cells ran; neither reached the publish floor, and
  **neither failed for a reason about the model or the product**. `$0.031` total model spend,
  **zero Buzz spent by the trial, zero account mutation attributable to it**.
- **Observed (with values):** cell `at1` — `stop: max-steps` at **40** steps, `generations: 0`,
  `submissions: 0`, `$0.0113`, last three calls `npx tsc --noEmit` / `npm run build` /
  `civitai app validate` (it had BUILT the app and was cut off before the submit-and-media phase).
  Cause: `driver.sh` had no `--max-steps` pass-through, so every trial was pinned to `runner.py`'s
  default 40 while a build-only `celsius` cell takes **65** — fixed and merged as `#738`.
  Cell `at2` — `--max-steps 140`, died `rc=124` (wall-clock timeout) at step **74**, having spent
  steps **56–74** (18 of 23 minutes) on `civitai login` attempts with 5-minute hangs.
  `via: measurement`
- 🔴 **Ruled out — that the credential was MISSING.** Step 60's `cat ~/.config/civitai/config.yaml`
  recorded `result: "exit code: 0\n[REDACTED:480d887c]\n"` — the read SUCCEEDED and printed the
  credential. The string *"No config file found"* is in the COMMAND (`… || echo "No config file
  found"`), never in the output. **I reported the fallback string as the result; that was wrong.**
  `via: measurement`
- 🔴 **CAUSE ESTABLISHED — the credential was EXPIRED, not absent.** Steps 55 and 62 both returned
  `device login failed: Invalid grant: refresh token is invalid (invalid_grant)`. The operator's
  credential is an **OAuth login** whose `config.yaml` carries `refresh_token` / `token_expiry`;
  **OAuth refresh is stateful, so a copy inside a container cannot refresh.** `via: measurement`
- **Ruled out — that `civitai login` destroyed the installed credential.** A cancelled/timed-out
  `civitai login` leaves an existing stored credential intact, measured directly on the host.
  `via: measurement`
- **Ruled out — that the install path is broken.** A control in a FRESH container reproduced the
  installer exactly: 232 bytes, mode `600`, readable as the trial user. `via: measurement`
- **Next probe — needs the operator.** Mint a **full-scope personal API key** at
  `https://civitai.com/user/account` → API Keys (a personal key has no refresh to invalidate), put
  it in a file (`install -m600 /dev/stdin /tmp/t1-key`), and build an ISOLATED trial credential
  without touching the live login — `os.UserConfigDir()` honours `XDG_CONFIG_HOME`, verified:
  `XDG_CONFIG_HOME=/tmp/t1cfg civitai whoami` reports *no token configured* while the real login
  still works. Then:
  ```bash
  XDG_CONFIG_HOME=/tmp/t1cfg civitai login --token "$(cat /tmp/t1-key)"
  cd /home/zach/workspace/civit/cli/scripts/dogfood   # a worktree off origin/main
  OPENROUTER_API_KEY="$(tr -d '\r\n' < ~/.config/openrouter/key)" \
  DOGFOOD_CREDENTIAL_FILE=/tmp/t1cfg/civitai/config.yaml \
  DOGFOOD_APP_PREFIX=ab-t1- DOGFOOD_TRIAL_PREFIX=at3 DOGFOOD_BRIEF_NAME=t1 \
  DOGFOOD_MAX_STEPS=140 DOGFOOD_MAX_GENERATIONS=4 DOGFOOD_MAX_SUBMISSIONS=1 \
  DOGFOOD_MODELS='xiaomi/mimo-v2.5|mimo' DOGFOOD_ENVS='df-node-root|noderoot|root' \
  DOGFOOD_IDENTITIES='claudeid|CLAUDECODE=1' bash driver.sh
  ```
  ⚠ `DOGFOOD_TRIAL_PREFIX` must be NEW (`at3`) or the resume guard skips the cell as complete.
  ⚠ `runner.py` increments the generation counter for **any** `generate` — **`--dry-run` is NOT
  exempt** (`runner.py:942`), so a price-check costs a slot; 4 buys 2 images.
  ⚠ `--max-cost` defaults to **$1.00** and the driver has no pass-through.

### 🔴 OPEN — spend can no longer be graded by balance delta, because the operator uses the same account
- as-of: 2026-09-28

- **Observed (with values):** during cell `at2`'s window (21:52–22:15) Buzz moved
  **4,074,007 → 4,073,892 (−115)** in two steps at ~22:11 and ~22:12, and a new submission
  `yt-thumbnail` v0.1.0 appeared at `22:01:19Z`. **None of it was the trial's.** `via: measurement`
- **Ruled out — that the trial spent it**, on three independent lines: the trial invoked
  `civitai generate` **zero** times; **zero** image files exist anywhere under `/work` in its
  container; and at 22:11–22:12 it was provably inside step 71's five-minute sleep loop. It also
  created no `ab-t1-*` app on the account at all. `via: measurement`
- 🔴 **Consequence:** the `dogfood-3`-era method of grading spend as a balance delta is **void**
  while the operator works the same account. **Attribute by blockId** — which
  `ship.verdict.sh` already does, reading the trial's own manifests out of the container.
  Reporting that delta as trial spend would have been a false claim about the operator's money.

## Next steps (ranked)

🔴 **Numbering frozen and cumulative.** 1–11 settled; 13, 15–22 closed; **12, 14, 25, 26 live.**

1–11. ✅ **DONE** — the arc through build-and-ship. forcing: user/gate — satisfied
12. **RE-SCOPED 2026-09-27, ready to implement.** `CIVITAI_SCAFFOLD_TYPECHECK` is set by **ZERO**
    workflows. 🔴 Wire it into the **daily `bump-scaffold-pins.yml`**, NOT `ci.yml`'s
    `scaffold-currency` job — that one is REQUIRED + `enforce_admins`, and a network-dependent
    assertion there has frozen every open PR **twice**. Copy the neighbouring positive-control
    shape (`-count=1`, grep the log for the test's own `--- (PASS|SKIP|FAIL):` line, a distinct
    `guard-did-not-run` state). ⚠ *"every scaffolded page-money app fails its own build"* was
    **overstated and install-shape-dependent** — this is regression protection, not a live break.
    forcing: regression — the defect shipped once and CI still cannot see it
14. 🔴 **BLOCKED ON THE OPERATOR — one action, then it runs.** T1 "ship-ready". The operator
    selected *spend Buzz on a generated cover* from a four-option question on 2026-09-27 — **a
    SELECTION, not a quote; do not attribute words to them.** Measured: **no image tooling is
    needed** (icon aspect 0.9–1.1 ← `--aspect-ratio 1:1`; cover 1.3–2.4 ← `16:9`; the CLI reads
    geometry with the Go stdlib in `internal/appapi/imageinfo.go` and ATTACH refuses a bad shape in
    seconds with a legible message, so the retry loop closes with no local measurement capability). The brief and floor
    grader are MERGED (`#737`). What is left is a **personal API key** (see *Open investigations*)
    plus the run. 🔴 A moderator **REJECTION deletes the listing and every attached asset**, and a
    run including `submit` **cannot be credential-bounded** (`app create`/`submit`/`listing` need
    the account token; only generation can be scoped via `app dev-token`).
    **Closing condition:** one blind cheap-model trial reaches `approved`/live with an icon AND a
    cover it generated itself, per `app status --json`; the operator confirms the spend was theirs.
    ⚠ Do **not** grade on reaching `approved` inside the trial — measured review latency over 8
    submissions is median ~5.6 min with a tail to **741 min**. `#737`'s grader correctly stops at
    "submitted with the floor met".
    forcing: user — asked about "complete working apps" 2026-09-25, decided 2026-09-27
13, 15–22. ✅ **CLOSED** — 20/21/22 merged 2026-09-27 (`#735`/`#733`/`#734`).
    forcing: gate/user/security/regression — satisfied
25. 🔴 **UNBLOCK `schema-drift`, AND IT GATES EVERYTHING ELSE IN THIS REPO.** Full diagnosis,
    the two failing tests with their exact messages, and the preserved byte-identical re-vendor are
    in *Open investigations*. The work: fix the two pins in
    `internal/validate/pattern_test.go` (derive enum CONTENT from the schema, pin the message
    SHAPE; add the gloss for `^[a-z0-9][a-z0-9_-]*$`), commit
    `~/.cache/schema-revendor-2026-09-28/app-block.manifest.schema.json.revendored` into
    `schema/app-block.manifest.schema.json`, then sweep `internal/validate` for other hardcoded
    schema content. ⚠ A branch `zach/revendor-schema-unblock` and a worktree
    `/tmp/wt-schema-revendor-2053628` exist from a STOPPED agent — 0 commits ahead of main, one
    uncommitted schema edit, already preserved; safe to remove.
    **Closing condition:** `schema-drift` green on `origin/main` AND on `#742`, and one manual
    `gh workflow run revendor-canonical-schema` completes and opens a PR unaided.
    forcing: gate — a required check is red on every open PR in the repo
26. **MERGE `#742` once 25 lands.** `MERGEABLE`/`UNSTABLE`; its only non-success is the inherited
    `schema-drift`. It adds `post_link=` to the post arm (`absolute` / `relative` / `none` /
    `unmeasured`, classified by where the href RESOLVES, so a protocol-relative href is correctly
    not a finding). Escaper, inertness and byte-identical-row guards all re-verified; mutation
    10/10. Re-read its CI SHA-pinned after 25 merges — do not merge on the stale rollup.
    forcing: gate — the arm cannot see the defect class a real user found until this lands

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

### Added 2026-09-28 — a real user's session is worth more than the whole synthetic rig

- 🔴 **THE OPERATOR'S OWN opencode SESSION FOUND MORE THAN EVERY DOGFOOD CELL COMBINED, AND IT
  SUCCEEDED.** `ses_f1b9a7d40ffeqlEuYZlMEN0U2L` ("Civitai YouTube thumbnail generator"), model
  `z-ai/glm-5.3-flash`, **$0.18**, 110 messages / 3 user turns / 115 tool calls. It shipped
  `yt-thumbnail` v0.1.0 (submitted `22:01:19Z`, pending). Span 2 h 17 m but **agent working time
  was 45.6 min** — 67% was operator-idle, so ranking time sinks against the span understates every
  one by ~3×. Store: `~/.local/share/opencode/opencode-stable.db`, `data` is a JSON blob in
  `message`/`part`. ⚠ **`find-session.py` cannot find a session by id** — ids live in
  `session.id`, that tool searches message CONTENT; it returned only THIS session, because the id
  appeared in the prompt. Query the DB directly.
- 🔴 **THE 110-MINUTE BLOCKER WAS A FALSE PREREQUISITE, AND THE TOOLING MANUFACTURED IT.**
  `agent-setup` printed *"export CIVITAI_TOKEN=…"*; `prompt.md` §5 requires the agent to relay it
  **verbatim without explaining it**; the operator could not find the credential and returned 110
  minutes later. **`CIVITAI_TOKEN` appears in 24 parts of discussion and 0 of the 50 bash
  commands** — the app scaffolded, built, tested, validated and **submitted** without it. That
  block is about the MCP servers, which the session never touched (0 MCP calls). Compounding it,
  `--check` reported `"ok": true` on a row whose own detail said *"CIVITAI_TOKEN is NOT set … 401s
  until you export"*, under an instruction to read `ok` alone. Fixed in `#740`.
- 🔴 **THE SCAFFOLD SHIPPED THE ANSWER AND NOTHING POINTED AT IT.** `vite-plugin-civitai-setup.ts`
  (5,547 B) + `src/setup-dev-live.ts` (15,666 B) implement a **"Set up automatically"** button that
  mints the token, merges `.env.development.local` in place, and takes a pasted key — *never asking
  anyone to export anything*. The generated README mentioned `dev-token` **7 times** and the wizard
  **0**: it documented the harder path seven times over. Fixed in `#740`.
- ⚠ **Docs-vs-`node_modules` is still 1:1, and this CORROBORATES the synthetic measurement.** 4
  `webfetch` calls versus 4 `node_modules` reverse-engineering probes, and **all four doc fetches
  land in the first 44 seconds** — zero during the 14.7-min stall or the 15.8-min fix phase, the
  two windows where it was guessing at SDK surface. `apps/examples` and `llms.txt`, which
  `AGENTS.md` advertises, were never fetched.
- ⚠ **67% of agent working time was ONE FILE.** Six aborted 12.5k-token `App.tsx` writes (880 s)
  plus the fallout (951 s). Provider-side, but the monolithic `page-money` `App.tsx` is why a UI
  change reads as "rewrite the file".

### Added 2026-09-28 — four of my own claims that did not survive, and the pattern in them

- 🔴 **I SAID A BLOCK CANNOT LEARN THE HOST ORIGIN. `useHostOrigin()` IS A FIRST-CLASS EXPORT**
  (`@civitai/blocks-react/dist/index.d.ts:25`, *"the validated host origin, or `undefined` before
  `BLOCK_INIT` lands"*), present since 0.20.0 and already in the live bundle. My search pattern was
  `(baseUrl|origin)\??:` — it can only match **field declarations**, and this is a **hook**. I then
  called that *"I checked the whole SDK surface"*. **Sampling reported as enumeration**, and it was
  the load-bearing claim in a brief; the agent refused to assert it and filed on the contract
  mismatch instead. It is also why v0.1.3's fix needs no hardcoded domain.
- 🔴 **I READ A COMMAND AS ITS OUTPUT.** *"No config file found"* was the `||` fallback in step 60's
  command line; the recorded result was `exit code: 0` plus the credential. That single misreading
  sent the whole credential diagnosis at the wrong fix — `#739`'s read-back would have PASSED that
  run. **The `part` JSON carries both; read the result field.**
- 🔴 **I PRESENTED AN INFERENCE AS A MEASUREMENT AND BLAMED THE WRONG AGENT.** I said a sibling had
  deleted `scratchpad/pristine/`; it had `rm -rf`'d it itself after its PR opened, and its battery
  ran **nine minutes before** the sibling's write. Two agents corrected me independently and
  consistently. The warning I sent was still right to send — but it should have named the
  alternative explanation instead of asserting a cause.
- 🔴 **I BRIEFED THE WRONG CLI VERSION** (`0.1.109`) for the opencode session after measuring
  `0.1.105` myself earlier the same day. The agent caught it from the transcript.
- **The pattern, stated once:** every one was a narrow probe reported in the register of a
  measurement. The cure that worked each time was a **positive control** — and three of the four
  were caught by an agent declining to repeat my claim, which is the reason to keep asking
  subagents to verify rather than to comply.

### Added 2026-09-28 — process traps hit while investigating traps of the same class

- 🔴 **A BACKTICK INSIDE A DOUBLE-QUOTED `echo` EXECUTED `civitai login`** and hung for two
  minutes — while I was investigating whether `civitai login` destroys credentials. Single-quote it,
  or use `git commit -F <file>`.
- 🔴 **`pkill -f "civitai login"` KILLED ITS OWN REPORTING SHELL**, producing an empty probe I
  nearly read as a result.
- 🔴 **`docker exec -d sh -c 'civitai …'` HAS NO LOGIN PATH** — `civitai: not found`. A probe that
  cannot run the binary returns a zero about itself.
- 🔴 **`gofmt -l` REPORTS NOTHING ABOUT A FILE THAT DOES NOT PARSE** — it lists *misformatted*
  files. I read a clean `gofmt -l` over an uncompilable file because I had sent stderr to
  `/dev/null`. Use `gofmt -e <file>` and read stderr.
- 🔴 **`git rev-parse --git-common-dir` RETURNS A RELATIVE `.git`**, so a `find` over it searched
  the WRONG repo from this dispatch hub — the 71-entry `rr-cache` listing I quoted was
  `datapacket-talos`'s; `cli` has 49. Use `--absolute-git-dir`.
- 🔴 **A BROKEN CONFLICT RESOLUTION IS CACHED BY `git rerere` IN THE COMMON GIT DIR AND REPLAYED
  WITH NO MARKERS.** `rerere.enabled` is **global** on this host. `git merge` returned rc 1 and
  `UU` while the file had zero markers and would not compile. `git rerere forget` REFUSES from that
  state; the remedy was `rm -rf` of the cache entry, and **`git -c rerere.enabled=false merge` is
  the one-command discriminator** between "rerere resolved it" and "no conflict". Full record in
  the `devrc` store's `rules` entry.
- 🔴 **I DISPATCHED TWO AGENTS INTO ONE SCRATCHPAD TWICE**, after being bitten by it earlier the
  same day. They collided on `pristine/`, `mutate.py` and `commitmsg.txt`. **Name every scratch
  path per-agent in the brief** — the agents cannot know about each other.
- ⚠ **#733 and #734 conflicted in one add/add hunk** whose two sides each end mid-function around a
  **shared closing brace**, so concatenating both sides — the obvious resolution — yields an
  unterminated function. Keep both and give the first its own `}`.

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

🔴 **Added 2026-09-28 — from the live-app feedback and the onboarding diagnosis.**

- **`ab-img-poster`'s metadata gap is NOT fixable in the app.** `BlockCreatePostRequest` is exactly
  `{sources, title?, detail?, tags?, modelVersionId?}` — no metadata field — and
  `BlockPostSource` (workflow) is `{kind, workflowId, imageIndexes?}` — no flag. The host creates
  the `Image` rows for a workflow source, so only the host can attach generation metadata. Tracked
  as `civitai/civitai#5182`; nothing to do in this repo or the app.
- **v0.1.3's URL fix is verified in the bundle, NOT against the real host.** `new URL(` goes 1 → 2
  versus the live v0.1.2 bundle, the minified call reads `new URL(i,r||void 0)`, and all six helper
  cases were unit-tested in node — but no live post has exercised it. The first post through v0.1.3
  after approval is the real test.
- **The post arm still cannot see a block that faithfully renders a malformed host URL** on the
  ordinary arm — deliberate, since defaulting the mock to the platform's relative shape would move
  every recorded verdict. `#742` catches an app that MANGLES the url; the malformed-host case needs
  `CIVITAI_ASSERT_POST_MALFORMED_URL=1`.
- **`#742`'s arm reads only `a[href]`** — a button calling `location.assign`, a copy-to-clipboard
  control, or a link in a shadow root is invisible; no navigation is attempted, so an absolute href
  pointing at the WRONG post passes.
- **`agent-setup --check`'s stale-CLI row is verdict-EXEMPT by decision**, so an agent that reads
  only `ok` — which the hosted prompt instructs — still will not notice a stale CLI. Fixing that
  means changing `prompt.md`'s *"read `ok`, not the rows"* instruction, which lives outside this
  repo. Recorded in `#740`'s own output and pinned by a test.
- `pickerBlind` and `consentBlind` have never been tested together on one doubly-blind bundle.
- `CONSENT_SETTLE_MS = 1200` is a judgement; every observed ask was synchronous.
- Nothing is known about the **iframe** transport; all of this is the inline path.
- `civitai app listing status` opens a shadow revision on a LIVE listing and there is no
  `discard-revision`. One is **still open on `panorama-360`** — operator-only to clear.
- The submissions listing caps at 100 rows with no cursor, and the account is AT the cap.
- `civitai app submit` **exits 0 when it did not submit** (no token) — it warns `⚠ NOT SUBMITTED`.
