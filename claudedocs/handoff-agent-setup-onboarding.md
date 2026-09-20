# Handoff: agent-setup-onboarding — 2026-09-12

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

Give Civitai the Cloudflare-style agent onboarding entrypoint: one URL a user
pastes into any coding agent, which then sets that agent up to build Civitai
Apps. Concretely — `developer.civitai.com/agent-setup/prompt.md` (a thin router)
plus a `civitai agent-setup` command (all the real logic, in Go, tested).

- **closing-condition:** `check` — a BLIND dogfood run reaches a working setup:
  an agent given only the hosted URL, allowed to work from the tool's own errors
  and `--help` but **not** from reading source, ends with `civitai agent-setup
  --check` reporting `ok: true` **and** `zsh -lic 'civitai --version'` printing
  that same version in the USER's login shell, on a machine that did not build
  it. 🔴 Both halves, because the doc already records the failure where only the
  first held: `--check` returned `ok: true` from inside the agent's shell while
  the login shell still printed 0.1.101. The `authenticated` sub-check is
  reported and must NOT fail `ok` — stopping before auth is deliberate.
  This is frozen as the condition this arc was opened on.

- ✅ **MEASURED 2026-09-18 — SATISFIED on its own wording, and that wording is
  narrower than it reads.** It asks for **A** blind run to reach a working setup, not
  for all of them: **4 of 16 cells did**, each clearing every clause — blind (the
  container holds no repo), given only the hosted URL, on a machine that did not build
  the CLI, both halves green — and still passing under the CORRECTED arm B.
  🔴 **DO NOT CLOSE THE ARC ON THAT.** The same run found the entrypoint fails in
  **12 of 16** cells, which is the more useful fact and is bigger than what this
  condition asks. Per this doc's own rule a later finding opens a NEW arc rather than
  extending a frozen one — so: condition satisfied, arc deliberately left open.
  ⚠ **An earlier version of this bullet said "NOT MET", and every report built on it
  said so too.** That applied a stricter reading (*the entrypoint works generally*)
  than the frozen text, which is the same substitution this arc spent eleven audit
  rounds on — a better-sounding claim in place of the measured one.
  ⚠ The 4 passing cells are sourced to measurements recorded on 2026-09-18/19; the
  trial containers have since been deleted, so re-checking needs a re-run.
  The 12 failures are explained by exactly two causes (ranks 33 and 34 below — 34 is a
  defect; 33 was a design call, **DECIDED 2026-09-19** — see its entry), with
  a **model-independent VERDICT** across 19 trials — ⚠ stated at that width on
  purpose: the machine state did not depend on the model, but what the user is
  TOLD about it did (2 of 8 agents omitted the PATH-persistence warning, both the
  same model). "No model-dependent behaviour at all" is false. The condition is
  additionally **unreachable by construction on the `other` agent path** — a future
  close-check must land rank 33 first or it is grading an impossible bar. ✅ **That is
  no longer contested: decided 2026-09-19** (`refs/agent-setup-verdict-decision-2026-09-19.md`), the verdict
  moves. ⚠ Decided, NOT implemented — the bar stays impossible until the change merges. Method, grid, controls and limits:
  [`claudedocs/refs/agent-setup-dogfood-matrix-2026-09-18.md`](refs/agent-setup-dogfood-matrix-2026-09-18.md).
  The HARNESS is committed at `scripts/dogfood/` and is re-runnable. ⚠ **The
  READING is not** — transcripts are gitignored and the committed driver runs a
  different cell set than the one that produced these numbers, so every per-trial
  figure is a recorded measurement rather than a reproducible one. The evidence
  doc opens with that disclosure. ⚠ Every trial also fetched the true bytes via
  `curl`, so none of this speaks to the WebFetch-summarisation failure mode.

✅ **THE SECOND EFFORT THAT ACCRETED HERE HAS BEEN SPLIT OUT —
[`handoff-terminal-line-forgery.md`](handoff-terminal-line-forgery.md).**
The `safeTerm` / terminal-line-forgery work (ranks 23, 28, 30, 31 **and 32**; issues #574,
#604, #605, #612, #620, #621, #622, #624, #627, #629; PRs #596…#628) shares no
code, no goal and no closing condition with agent-setup onboarding. It rode in
because this doc was the queue every `/resume` drew from, and nothing refused it.

🔴 **It now has a closing condition of its own — the first it has ever had** — and
that is the thing to read there, not the rank list: *the hand enumeration is
retired, because an instrument finds what it found.* Every defect that arc closed
was found by a HUMAN enumerating operands, because every instrument the repo owns
keys on the GATE and is therefore structurally incapable of finding an operand
that has no gate. ⚠ That condition is **unratified** — a proposal by the session
that wrote it, flagged as such there.

- **A close-check against THIS doc's condition and one against that doc's are
  different questions**, and each reads NOT ADDRESSED for reasons belonging to the
  other. Do not resolve that by widening either — a later audit or ask opens a NEW
  arc, it does not extend a frozen one.
- **Do not merge them back together.** This split is rank 29's "split by
  initiative" step, done; the prune was never only about bytes.
- ⚠ **The `## Gotchas` section below was NOT split** — it is filed by date and
  session, not by initiative, so forgery-specific and agent-setup lessons are
  interleaved there. The forgery-specific ones are restated in the new doc; the
  originals are left in place rather than moved, because a partial move would make
  this section's dates lie. Splitting it is the remaining half of rank 29.

## State now

- ✅ **RELEASED — `v0.1.106` is live on npm and Homebrew**, re-confirmed independently:
  a container that never built the CLI installed it from the PUBLIC registry and
  completed setup end-to-end (the `ctl-pos` grader control).
- ✅ **Rank 33 — `cli#669` MERGED** (`d0b79ae`) and **MEASURED**: mechanism A is CLOSED.
  All four `other`-identity cells measured post-ship grade `CLOSING_CONDITION=yes`,
  against 4-of-4 failing on 2026-09-18.
- ✅ **Rank 35 — `civitai-developer-docs#89` MERGED AND LIVE** (7433 B, step 4 keys on `ok`).
- ✅ **Rank 36 — the matrix RAN and is graded: 18/18, 12 `yes` / 6 `no`.** The verdict is
  now a function of **prefix-writable alone**; identity no longer determines anything.
  Full grid and limits in the investigation block below. ⚠ Its comparison clause is
  **partly unmet** — see rank 36.
- ⚠ **Rank 34 / `cli#665` — UNFIXED, and the remedy analysis is aimed at the wrong
  path.** 6 of 6 failing containers (complete enumeration) installed the CLI at
  `$HOME/.npm-global/bin/civitai`, reachable from NEITHER login shell. The issue's
  candidate (b) is `--prefix="$HOME/.local"`; **no agent took it.**
- ✅ **`cli#673` MERGED** (`9588fb0`, squash) — grader identity in the verdict line, and a
  stale stderr-ordering claim corrected. Verified by CONTENT with a negative control:
  `AGENT_ID` 3× at `origin/main`, 0× at `origin/main~1`. 13/13 checks green SHA-pinned.
- **Deploy/verify status:** nothing new deployed. `#673` touches only the harness.
- ⚠ **No `clawgate-task:` field**: `clawgate_handoff.sh resolve` exited **5**. An unknown
  session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id". Not a clean bill of health.
- ⚠ **Live worktrees**, remove by EXACT path: `cli-dogfood36` (holds `runs/` and `logs/` —
  the ONLY copy of this session's 21 transcripts), `cli-ho674` (this doc), `cli-rank33`
  and `cli-cc-fix` (both merged — safe to remove).
- ⚠ **21 `dogfood-*` trial containers + 3 `dogfood-ctl-*` controls are RUNNING**, plus four
  `df-*` images at 342 MB each. 🔴 `grade.sh` can only read a RUNNING container and
  refuses a stopped one by name, so tearing these down means any re-grade needs a
  **re-run** — and `runner.py` opens each transcript `"w"`, so a re-run DESTROYS the
  transcripts too. Copy `runs/` aside before any re-run.

## Open investigations — live diagnosis state

🔴 **THIS SECTION IS APPEND-ONLY, SO A HEADING ALONE IS NOT A STATUS. READ THE
HEADING PREFIX.** Blocks are never deleted — a corrected reading is worth more than a
deleted one — so a superseded diagnosis stays in place with its heading rewritten to
`❌ SUPERSEDED`. **TWO investigations are open below**, and one is DECIDED: "⚠ OPEN —
The docs repo cannot be built locally from a pristine main" (rank 15) and "⚠ OPEN — the
documented `--prefix` remedy leaves the CLI unreachable" (rank 34, filed as cli#665);
"✅ DECIDED — an agent the CLI does not know can never reach `ok: true`" (rank 33) is
settled but NOT implemented. Everything else here is `✅ RESOLVED` or
`❌ SUPERSEDED`, and a reader who stops at the first matching heading was previously
getting the OPPOSITE of the truth — three blocks still said `STILL OPEN` above their
own resolutions until this delta retired them (2026-09-12).

**When you append a RESOLVED block, retire the superseded heading in the SAME edit.**
It is one `Edit` call and nobody ever comes back for it.

### ❌ SUPERSEDED (2026-09-12) — "The repo is frozen: `pins-vs-published` is red on `main`"

🔴 **RESOLVED. Superseded by "✅ RESOLVED — the repo freeze (`pins-vs-published`)" below
(cli#529).** Do NOT run this block's "Next probe": it was written to separate two
mechanisms, and the answer is already known — the npm/arborist crash was real and the
SDK was innocent. Kept for its measurements only.

- **Symptom + exact repro:** nothing merges in `civitai/cli`. Every PR shows
  `mergeStateStatus: BLOCKED`. Reproduce the cause directly:
  ```bash
  cd /home/zach/workspace/civit/cli
  CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold \
      -run TestScaffoldPinsSatisfyPublished -count=1
  ```
- **Observed (with values):**
  - Guard output, verbatim: `STALE SCAFFOLD PIN — templates/page-money/package.json.tmpl`;
    `@civitai/app-sdk` pinned `^0.37.0` vs published **0.39.0**;
    `@civitai/blocks-react` pinned `^0.46.0` vs published **0.49.0**. Pre-1.0
    caret locks the minor, so neither pin admits the published version.
  - Branch protection: `gh api repos/civitai/cli/branches/main/protection` →
    required contexts `["pins-vs-published","scaffold-currency","build-test","ready-ack-runtime","template-page-vite"]`,
    `enforce_admins: true`, `strict: false`.
  - `bump-scaffold-pins` nightly: **failed 09-06 (34030852777), 09-07
    (34127760960), 09-08 (34224413110)**; last green **09-05 (33962879680)**;
    09-04 also failed.
  - In run 34224413110 the bump itself WORKS — steps 4 `bump stale @civitai/*
    scaffold pins`, 6 `validate — pins now satisfy published (network guard)` and
    7 `validate — full scaffold suite (offline)` all **succeed**. It dies at step
    9 `validate — scaffold builds against the bumped SDK` with:
    `npm error Cannot read properties of null (reading 'edgesOut')`.
    Step 10 `open PR if pins changed` is therefore **skipped** — which is why no
    bump PR ever appears.
  - The last full `ci.yml` run on `main` (33909583100, 09-04, `d9b4e29`) was
    green on all 8 jobs including `pins-vs-published` — a **stored green with an
    expiry date**, since the guard queries live npm and upstream published after.
- **Ruled out:** that the bump logic is broken — steps 4/6/7 pass, including the
  network guard against the NEW pins. `via: measurement` (`gh run view
  34224413110 --json jobs`, per-step conclusions). · That `pins-vs-published`
  might be a stale/false alarm — the guard was run locally against live npm and
  fails for real. `via: command`. · That this is caused by anything in the
  agent-setup work — it predates it by three days. `via: measurement` (run dates).
- **Leading hypothesis:** an **npm/arborist install crash** on the scaffolded
  app under node 24 / npm 11 (`aebd2fb` moved CI to node 24), NOT a removed or
  renamed SDK export. 🔴 **Issue #524's body says the opposite** — *"Pins were
  rewritten and satisfy npm, but a fresh `civitai app init … --template
  page-money` then failed to install, typecheck or build against them… this is
  the removed/renamed SDK export case, and it is a real upstream break rather
  than a bot bug."* That body is **rewritten by every failing run** and its
  classification contradicts the log line above. Do not adopt it without
  reproducing.
- **Next probe:** reproduce the install locally against the bumped pins, which
  separates the two mechanisms in one command:
  ```bash
  cd $(mktemp -d) && civitai app init probe --template page-money && cd probe
  sed -i 's/\^0\.37\.0/^0.39.0/; s/\^0\.46\.0/^0.49.0/' package.json
  npm install 2>&1 | tail -30      # edgesOut crash here ⇒ npm bug, not an SDK break
  ```
  If it crashes: the fix is an npm pin / `--legacy-peer-deps` in the workflow,
  and the SDK is innocent. If it installs and then a typecheck fails naming a
  missing export: #524 is right and 0.46→0.49 really did break `page-money`.
  Per the `cli/scaffold` cairn entry, the method that clears a `blocks-react`
  bump is `npm pack` both versions and diff `dist/` recursively, reading the
  verdict off `internal/transport`, `internal/liveHost`, `internal/mockHost`,
  `hooks/useBuzzWorkflow`, `hooks/useAppWorkflows` by name.

### ❌ SUPERSEDED (2026-09-12) — "`developer.civitai.com` returns 403 to `Python-urllib` — IN FLIGHT, delegated"

🔴 **RESOLVED. Superseded by "✅ RESOLVED — `developer.civitai.com` 403 to
`Python-urllib`" below.** Its leading hypothesis (a UA denylist in the docs repo's
`nginx.conf`) is **REFUTED** — the cause was Cloudflare Browser Integrity Check, fixed
by a Configuration Rule. **Do NOT run its "Next probe" and do NOT look in
`nginx.conf`.** Re-measured live 2026-09-12: `Python-urllib/3.11` → **200**,
`python-requests/2.31` → **200** on `/agent-setup/prompt.md`.

- **Symptom + exact repro:**
  ```bash
  curl -sI -A 'Python-urllib/3.11' https://developer.civitai.com/ | head -1   # 403
  curl -sI -A 'python-requests/2.31' https://developer.civitai.com/ | head -1 # 200
  ```
- **Observed (with values):** 403 for `Python-urllib/3.11`; 200 for default
  curl, `python-requests`, `ClaudeBot/1.0`, and browsers. Naive agent fetchers
  using urllib are hard-blocked — which breaks the pasted-URL pattern before it
  starts. Separately, per-page `.md` URLs return 200 but with
  `Content-Type: text/plain`, not `text/markdown`.
- **Ruled out:** nothing yet — this was measured, not diagnosed. `via: assumed`
  that it is in `nginx.conf` (the docs repo ships one); the subagent was briefed
  to check there FIRST and to say plainly if it turns out to be upstream in a
  CDN/WAF rather than fake a fix.
- **Leading hypothesis:** a UA denylist in the docs repo's own `nginx.conf`.
- **Next probe:** read the docs PR's report. If it says upstream, the fix is not
  in this repo and needs whoever owns the edge config.

### ✅ RESOLVED — the repo freeze (`pins-vs-published`)

**Closed by cli#529.** Pins bumped to `^0.39.0` / `^0.49.0`; the guard passes and
four queued PRs merged. The diagnosis recorded below was **half right**: the
`edgesOut` crash is real and still open (see below), but issue #524's
classification — *"the removed/renamed SDK export case … a real upstream break"* —
was **wrong**.

- **Ruled out:** that 0.46→0.49 broke the scaffold. Scaffolded page-money,
  hand-bumped, `npm install` → 142 packages rc 0, `typecheck` clean, `build`
  clean. `via: command`
- **Ruled out:** that it was a contract inversion like 0.44's. `npm pack`ed both
  `blocks-react` versions and diffed `dist/` by name: `useBuzzWorkflow` and
  `useAppWorkflows` **byte-identical**; `transport`/`liveHost`/`mockHost` differ
  by **zero removed lines** — purely additive. `via: measurement`
- **Ruled out:** that node 24 was the cause. `template-page-money` and
  `scaffold-currency` scaffold and build against the bumped pins on the same
  node-24 runners and **pass**. `via: measurement`

### ❌ SUPERSEDED (2026-09-12) — "🔴 STILL OPEN — `bump-scaffold-pins` cannot land its own fix (cli#530)"

🔴 **NOT OPEN. Superseded by "✅ RESOLVED — `bump-scaffold-pins` could not land its own
bump (cli#530)" below** — closed by cli#540 (`5557b6a`), verified 2026-09-11 by run
34559798234. Issue **#530 is CLOSED**, re-verified 2026-09-12. Kept for the control
pair (node 22/npm 10 vs node 24/npm 11), which is the durable evidence.

The nightly bumps correctly (steps 4/6/7 green, incl. the network guard against
the NEW pins) then dies at step 9 `validate — scaffold builds against the bumped
SDK` with `npm error Cannot read properties of null (reading 'edgesOut')`, which
**skips step 10 `open PR if pins changed`**. So no bump PR is ever opened and the
repo re-freezes on its own the next time upstream publishes.

- 🔴 **DIAGNOSED 2026-09-08 — the two hypotheses below this line were BOTH
  WRONG; do not re-derive either.** The cause is that
  `bump-scaffold-pins.yml` still pinned `node-version: "22"` in both its jobs.
  `ci.yml` moved to node 24 in `aebd2fb` (PR #520) **for this exact crash**; the
  sibling nightly was never updated. Measured on a fresh
  `civitai app init … --template page-money` at the pins of the day
  (`@civitai/app-sdk ^0.39.0`, `@civitai/blocks-react ^0.49.0`), a separate
  clean directory per arm:
  - node **22.23.2 / npm 10.9.8** (the runner's exact versions, from the failing
    run log) → `npm install --no-audit --no-fund` exits **1** with
    `npm error Cannot read properties of null (reading 'edgesOut')`.
  - node **24.19.0 / npm 11.17.0** → install + typecheck + build all exit **0**,
    116 modules transformed.

  Run-date corroboration: green 09-01/02/03/05 (pins already current ⇒ step 9
  skipped by its `if:`), red 09-04/06/07/08 (the bumper changed something ⇒ step 9
  ran).
- ❌ **RETRACTED — "the nightly installs into a tree its own earlier steps just
  rewrote."** Refuted: step 9 scaffolds into a fresh `$RUNNER_TEMP/bump-validate`,
  and the crash reproduces on a fresh scaffold outside CI entirely.
- ❌ **RETRACTED — #524's "a removed/renamed SDK export … a real upstream break
  rather than a bot bug."** That sentence was **generated by this workflow's own
  `classify` job**, not by a human diagnosis, and it was false. The classifier
  has been rewritten to state the measurement and enumerate the causes without
  picking one.
- 🔴 **Independent of the cause:** a step-9 failure should not silently suppress
  step 10. Opening the PR as a draft, or letting its own CI carry the red,
  surfaces the break without taking the repo down. **Done** — the PR step now
  runs under `always()`, downgrading to a draft on a build failure while pins/suite
  failures still withhold it.
- **Closing condition is still the operator's to observe:** a real scheduled run
  that either goes green or fails step 9 and STILL produces a PR. Nothing here
  can run the workflow.

### ❌ SUPERSEDED (2026-09-12) — "⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge" (1 of 2)

🔴 **NOT OPEN.** Superseded by "✅ RESOLVED — `developer.civitai.com` 403 to
`Python-urllib`" below. The 403 is **GONE**, re-measured live 2026-09-12 (200 for both
`Python-urllib/3.11` and `python-requests/2.31`). This is the FIRST of two identical
stale headings; the second is a few blocks down.

Re-measured after every deploy, unchanged: `Python-urllib/3.11` → **403**
(`error code: 1010`, Browser Integrity Check), `python-requests` / `curl` /
`ClaudeBot` → 200. Case-sensitive and prefix-anchored.

- **Ruled out:** that it is in this repo. The same `nginx.conf` run locally
  returns 200 to the blocked UA, and there is no `_headers`/`_redirects`/
  `wrangler.*` anywhere in the docs repo. `via: measurement`
- **Next step is not a probe:** it needs a Cloudflare dashboard change by someone
  with zone access. Not actionable from a repo.

### ✅ RESOLVED — `bump-scaffold-pins` could not land its own bump (cli#530)

🔴 **SUPERSEDES the "🔴 STILL OPEN — `bump-scaffold-pins` cannot land its own fix"
block above. That block's "Leading hypothesis" and "Next probe" are both WRONG and
must not be run** — the hypothesis it names was refuted, and the probe it suggests
(checking for a reused workspace between steps) investigates a mechanism that does
not exist here.

- **Closed by cli#540** (`5557b6a`), **verified 2026-09-11** by an actual run.
- **Root cause: node 22 / npm 10.** `ci.yml` moved to node 24 in `aebd2fb`
  (cli#520) *for this exact arborist crash*; the sibling nightly was left at
  `node-version: "22"` in **both** jobs.
- **Ruled out — the step-order hypothesis** ("the nightly installs into a tree its
  own earlier steps just rewrote"): step 9 scaffolds into a fresh
  `$RUNNER_TEMP/bump-validate`, and the crash reproduces on a fresh scaffold
  outside CI entirely. `via: measurement`
- **Ruled out — #524's "removed/renamed SDK export … a real upstream break"**:
  that body is generated by this workflow's own `classify` job, and the sentence
  has since been rewritten so it no longer asserts a cause it cannot know.
  `via: measurement`
- **Observed (control pair, same scaffold and pins, fresh dir per arm):**
  node 22.23.2 / npm 10.9.8 → `npm error Cannot read properties of null (reading
  'edgesOut')`, rc **1**. node 24.19.0 / npm 11.17.0 → install + typecheck +
  build clean, rc **0**. `via: command`
- 🔴 **A green scheduled run could NOT have proved this.** Both runs after #540
  merged went green with **steps 6–12 skipped** — pins current ⇒ `detect changes`
  false ⇒ step 9 never executes. Exercised deliberately instead
  ([run 34559798234](https://github.com/civitai/cli/actions/runs/34559798234)):
  step 9 **success**, step 11 **success**, opened PR #546 (closed, branches
  deleted). `via: measurement`

### ❌ SUPERSEDED (2026-09-12) — "⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge" (2 of 2)

🔴 **NOT OPEN.** Superseded by the `✅ RESOLVED` block immediately below. It WAS
actionable and it WAS actioned — a Configuration Rule on the `civitai.com` zone
disabling BIC for `developer.civitai.com`.

Unchanged this session and **not actionable from a repo**. Needs a Cloudflare
dashboard change by someone with zone access. `via: measurement` (ruled out as
ours: the same `nginx.conf` returns 200 locally to the blocked UA, and there is no
`_headers`/`_redirects`/`wrangler.*` anywhere in the docs repo).

### ✅ RESOLVED — `developer.civitai.com` 403 to `Python-urllib`

🔴 **The block below titled *"returns 403 to `Python-urllib` — IN FLIGHT, delegated"*
is RETIRED. Do NOT run its "Next probe", and do NOT look in `nginx.conf`.** Its
leading hypothesis — *"a UA denylist in the docs repo's own `nginx.conf`"*, tagged
`via: assumed` — is **REFUTED**. The block is left intact above only because a
corrected reading is worth more than a deleted one.

**Root cause: Cloudflare Browser Integrity Check (BIC)**, a zone-level setting on
the `civitai.com` zone. Never the origin, never the docs repo.

- **Symptom + exact repro (now fixed — this reproduces nothing today):**
  ```bash
  curl -s -o /dev/null -w '%{http_code}\n' -A 'Python-urllib/3.12' \
    https://developer.civitai.com/agent-setup/prompt.md      # was 403, now 200
  ```
- **Observed (with values), measured 2026-09-09 → 2026-09-11:**
  - Cloudflare firewall events (GraphQL `firewallEventsAdaptive`, zone
    `civitai.com`, host `developer.civitai.com`):
    `action=block  source=bic  ruleId=bic` on `/agent-setup/prompt.md` for
    `Python-urllib/3.11`, `Python-urllib/3.12`, `Python-urllib`, `libwww-perl/6.0`.
  - Block body was **17 bytes**, `content-type: text/plain`: `error code: 1010`
    — Cloudflare's BIC error code.
  - The 403 carried **none** of the origin's headers (no `etag`, no
    `x-content-type-options`, no `cf-cache-status`), while the 200 carried all
    three. The request never reached nginx.
  - Zone settings: `browser_check = "on"` (zone-wide, `editable: true`),
    `security_level = "essentially_off"`. BIC is independent of security level.
  - 🔴 **The denylist is exact-string and case-sensitive, and mostly porous:**
    `Python-urllib/3.11` → 403 but `python-urllib/3.11` → **200**;
    `MyAgent (urllib inside)` → 200; `<empty UA>` → **200**; `Wget/1.21`,
    `Go-http-client/1.1`, `node-fetch`, `axios`, `okhttp`, `Mozilla/5.0` → all 200.
    Only `Python-urllib*` and `libwww-perl*` were blocked. Cloudflare publishes no
    list, no versioning and no changelog for it.
- **Ruled out:** that it is in the docs repo's `nginx.conf` — the 403 carries no
  origin headers at all and Cloudflare's own events attribute it to `bic`.
  `via: measurement` · That a WAF **managed** ruleset did it — events name `bic`,
  not a ruleset id, and BIC is a zone product a managed-rules skip cannot reach.
  `via: measurement` · That a **custom** firewall rule did it — all 14 read; none
  matches `urllib`/`libwww`, and the host-scoped ones target `civitai.com` and
  `image.civitai.com` only. `via: command` · That **AI Crawl Control** could fix
  it — it manages known, self-identifying crawlers; `Python-urllib` is neither
  recognised nor verifiable. `via: doc`
  (https://developers.cloudflare.com/ai-crawl-control/features/manage-ai-crawlers/)
- **Resolution:** a **Configuration Rule** on the `civitai.com` zone:
  `(http.host eq "developer.civitai.com")` → `action: set_config`,
  `action_parameters: {"bic": false}`. `browser_check` stays **`on`** zone-wide,
  so `civitai.com` and `image.civitai.com` keep BIC.
- **Verified (content, not just status):** `Python-urllib/3.12` now receives
  byte-identical content to a known-good UA on `/`, `/index.html`, `/llms.txt`,
  `/apps/` and `/agent-setup/prompt.md` — `prompt.md` 6,949 B and `llms.txt`
  13,066 B, `cmp` clean both, `content-type: text/markdown`, origin `etag` present.

### ⚠ OPEN — The docs repo cannot be built locally from a pristine `main`, while CI builds it green

**Still open, and NOT re-verified on 2026-09-12** — this session did not touch the docs
repo. Rank 15. The block below is the diagnosis as of 2026-09-11.

- **Symptom + exact repro:**
  ```bash
  cd /home/zach/workspace/civit/civitai-developer-docs   # clean, at origin/main
  npm ci --no-audit --no-fund && npm run build
  ```
- **Observed (with values):** `prebuild` dies in `gen-appblocks-messages.mjs`:
  `Error: gen-appblocks-messages: 1 INVENTORY reply value(s) do not name a
  published SDK host->block message. … Unresolved: SET_COLLECTION_FOLLOW ->
  "COLLECTION_FOLLOW_RESULT"`. `npm run check:snapshots` is red on the same tree
  and asks for a re-snapshot of `hostHandlerParity.ts`. `check:cli-snapshot`,
  `check:manifest-parity` and `check:md-regions` are red on pristine `main` too.
- **Ruled out:** that it is caused by any change made this session — reproduced
  with the working tree byte-identical to `origin/main`, zero edits applied.
  `via: command` · That it is node-version dependent — identical failure on node
  **22.23.2** and node **26.8.1**; the first hypothesis was "local node 26 vs
  CI's node 20" and it is **RETRACTED**. `via: measurement` · That the four red
  `check:*` guards are related to this session's files — none of the four
  references any file this session touched. `via: code`
- **Leading hypothesis:** the committed `appblocks-snapshots/` have drifted from
  the published SDK, which is what those scheduled drift guards exist to report;
  the generator turns the same drift into a hard build failure. Why CI's
  `build-site` is **green on the same commit** is NOT explained — that is the
  open part.
- **Next probe:** diff what CI installs against what a local `npm ci` installs —
  `gh run view <build-site-run> --log | grep -A3 'added .* packages'` against a
  local `npm ls @civitai/blocks-react @civitai/app-sdk`. If the resolved SDK
  versions differ, the snapshots are fine and the lockfile/registry state is the
  variable; if they match, the difference is in the runner image and the
  generator is reading something outside the repo.

### ✅ RESOLVED — was cli#596 audited? (the previous handoff said no)
- as-of: 2026-09-14

Resolved by measurement, not by argument: it had been, twice, and the handoff was written
before those rounds' commits were read back.

- **Observed (with values):** `gh pr view 596 --json commits` returns **three** commits, not
  one — `2d509d3`, `382c7fc`, `19455c0`. `382c7fc`'s message opens *"Round 0 on #596"*;
  `19455c0`'s answers a nine-axis pass. A fenced `audit-claims round=1
  audited=2d509d3..19455c0c` block with 14 claims is posted as an issue comment on the PR,
  timestamped 2026-09-14T04:26:02Z.
- **Ruled out:** that a round-0 dispatch was still owed — `audit-dispatch.py 596 --round 2`
  parsed the round-1 block and emitted a delta range with **silent stderr**, i.e. no
  `the newest claims block says round=N` widening warning, which is what fires when an
  intermediate round posted nothing. `via: command`
- **Ruled out:** that the handoff's named worktree was where the work was — `/home/zach/workspace/civit/cli-574`
  does not exist; `git worktree list` shows `cli-596b` on `fix/generate-blob-forgery-r0` at
  `19455c0`. `via: command`

### ~~✅ DECIDED (2026-09-19) — an agent the CLI does not know can never reach `ok: true` (rank 33)~~ IMPLEMENTED 2026-09-19 — see "✅ IMPLEMENTED — rank 33 shipped as cli#669" below

🔴 **NOT OPEN, AND ITS CLOSING BULLET IS NOW WRONG.** The block below ends *"Next
probe: none … what is missing is a DECISION"* — the decision was made **and
implemented**; `checkCountsTowardVerdict` now excludes the `mcp-*` rows when
`agentTargets[agent]` is unknown. Read the IMPLEMENTED block at the bottom of this
section for what actually shipped and for the **three corrections the decision
record's plan needed**. Measurements below are kept — they are still the baseline.
- as-of: 2026-09-18, from 19 blind trials; **argument rewritten the same day after
  a round-0 audit refuted its original framing — read the RETRACTED bullet below
  before acting on any part of this block**

For `agent == other` — every agent with no entry in the CLI's table — `--check`
returns `ok: false` and exit 1 after a **completely correct** setup, forever.

- **Symptom + exact repro:**
  ```bash
  docker run -d --name x node:22-bookworm-slim sleep infinity
  docker exec -w /work x bash -lc \
    'mkdir -p /work && npm install -g @civitai/cli && civitai agent-setup --track app
     civitai agent-setup --check --json; echo "rc=$?"'
  ```
- **Observed (with values):** `{"agent":"other","ok":false,…}`, rc **1**, with
  `mcp-site` and `mcp-orch` both false and detail *"agent other has no config file
  this CLI knows — register … by hand"*. `authenticated` is correctly excluded
  (the error says "2 check(s) failed", not 3). 4 of 4 such trials hit it; the
  three de-confounding trials show it follows the IDENTITY, not the model —
  claude-sonnet-5 on `other` fails, gemini and grok on `claude` pass.
- **Ruled out:** that this is model behaviour — see the swap above. `via: measurement`
  · That `authenticated` is the cause — it is already excluded by
  `checkCountsTowardVerdict`. `via: code`
- **Mechanism, located:** `internal/cmd/agent_setup.go` `checkCountsTowardVerdict`
  excludes `authenticated` and `claude-md` and nothing else, so the two MCP rows
  count even for an agent the CLI has no config target for.
- ❌ **RETRACTED — "this is the same shape the file's own comments have already
  recognised TWICE; third instance, not a new class."** Refuted by a round-0
  audit. The two existing exclusions apply where **no work remains** (auth is out
  of scope; the CLAUDE.md shim is inert for a non-Claude agent); on `other` work
  remains and the user must paste. 🔴 **And `agent_setup.go:739-742` — the
  docstring of the function emitting these rows — records the opposite decision
  in words: *"An agent this CLI has no target for … are both genuinely unfinished
  setups"*.** The first draft read a switch as an omission while a sibling
  function stated the intent. **Do not re-derive the exclusion fix from the
  switch alone.**
- **What bites the entrypoint, and is not contested:** `prompt.md` step 4 says
  *"Do not report success if any check fails"* while this path fails one
  permanently by design — those cannot both stand; and the CLI's own remediation
  line, "re-run `civitai agent-setup` to fix what it can write", names an action
  that can never change the outcome here.
- 🔴 **Which surface moves is a DESIGN CALL, not a diagnosis.** `prompt.md`'s
  wording or the verdict semantics — and the `--json` contract has ledgered
  consumers (`readme_agent_setup_claims_test.go`), so it belongs to whoever owns
  that contract. This block reports; it does not decide.
- 🔴 **A2 — the escape hatch exists and the entrypoint never mentions it.**
  `prompt.md` contains `--agent` **zero** times, while
  `civitai agent-setup --track app --agent cursor` yields `{"ok":true}` on the spot.
  The one trial that recovered gracefully did so by **asking the user** which editor
  they use — the one thing the prompt tells the agent not to do — because nothing it
  was given mentions the flag. ⚠ **`--agent` is NOT a blanket fix**: it is right when
  detection merely FAILED for an agent that IS in the table, and wrong when the agent
  genuinely is not (it writes a config the running agent never reads, and `--check`
  then reports green on a setup that does not work). Any prompt change here has to
  separate those two cases, and it does not remove the need to fix A.
- **Next probe:** none. Nothing here is undiagnosed — what is missing is a DECISION,
  and this block does not get to make it. 🔴 **Neither candidate is recommended here.**
  An earlier version of this very bullet said the verdict fix was *"recommended,
  matches precedent"* — the exact two words the retraction above kills — and left it
  in the ACTION bullet, which is where the next session looks. That is the failure
  this block exists to prevent, committed inside the block that prevents it.

### ~~⚠ OPEN — the documented `--prefix` remedy leaves the CLI unreachable (rank 34)~~ PARTLY ADDRESSED 2026-09-19 — see "✅ SHIPPED — ranks 33/34/35 and v0.1.106" below

🔴 **STILL OPEN AS AN ISSUE, BUT ITS SECOND-ORDER HARM IS FIXED.** The measured
defect below is unchanged — nothing shipped installs the CLI anywhere a later
shell can reach, and cli#665's own closing condition is NOT met. What HAS shipped
(cli#671, in v0.1.106) is the `AGENTS.md` half: the block now tells a reader what
to do when `civitai` is not on PATH, which addresses the **0 of 8** finding below.
🔴 **Do NOT read the "Next probe: none" line as current** — the remedies were
re-opened, two probe-based drafts were built and DELETED, and the reasoning is in
the shipped block. Measurements below stand.
- as-of: 2026-09-18, from 19 blind trials

- **Symptom + exact repro:** on any machine where npm's global prefix is not
  writable, follow `prompt.md` §2 "If the install fails" exactly, then open a new
  shell:
  ```bash
  npm install -g --prefix="$HOME/.npm-global" @civitai/cli
  PATH="$HOME/.npm-global/bin:$PATH"; civitai --version   # works
  bash -lc 'command -v civitai'                           # nothing
  zsh -lic 'civitai --version'                            # command not found
  ```
- **Observed (with values):** **8 of 8** trials on the two non-writable-prefix
  environments ended this way, across all four models. The binary is present at
  `~/.npm-global/bin/civitai` the whole time. What the agents reported: **8/8**
  relayed the PATH line as step 5 requires, **6/8** also warned it is not
  persistent and named the profile file, **2/8** (both gpt-5.6-terra) gave no
  persistence warning, and **0/8** connected it to the `AGENTS.md` just written.
- ⚠ **CORRECTED — an earlier draft of this block said "5 of 8 still declared
  success", from a keyword regex, and that overstated the defect.** Read by hand,
  most agents relay a usable manual remedy. **The defect is not that agents lie;
  it is that step 3's durable artifact is written against a binary that only
  exists in the installing shell.** `AGENTS.md` tells every future agent session
  to run `civitai …`, and every future session gets a new shell.
- **Ruled out:** that the agents skipped or garbled the documented remedy — every
  one ran it verbatim and relayed the PATH line. `via: measurement`
- **Why step 4 cannot catch it:** step 4's `civitai --version` runs in the *same*
  shell as the install, which is the one shell where it works.
- 🔴 **The second-order cost is the real one:** step 3 writes an `AGENTS.md` that
  tells every future agent session to run `civitai …`. Those sessions get a new
  shell, so the file the setup exists to produce names a binary the setup left
  unreachable.
- 🔴 **This is "success measured in an environment the user does not have" on a NEW
  operand.** The recorded instance was a *stale* binary and was fixed with
  `civitai upgrade`; this is an *unreachable* binary produced by the prompt's own
  remedy. A fix aimed at the earlier operand did not generalise.
- **Next probe:** none — the remedies and what is measured about them live in ranked
  item 34, which is the ONE place they are maintained. 🔴 **Do not restate them here.**
  A copy of the ranking lived in this bullet through four audit rounds and was missed by
  a sweep note that named only three surfaces; ranked item 34, the evidence doc and
  issue cli#665 are the other three.

### ✅ IMPLEMENTED — rank 33 shipped as cli#669, and the decision record's plan needed three corrections
- as-of: 2026-09-19

**Supersedes the `✅ DECIDED (2026-09-19)` block above, whose "Next probe: none —
what is missing is a DECISION" is now wrong.** The decision was made *and*
implemented. The durable rule lives in
`claudedocs/decisions/35-agent-setup-merges-a-users-file.md`
§"The `mcp-*` rows for an agent this CLI has no target for".

🔴 **Each correction below was MEASURED, not reasoned. The decision record
(`claudedocs/refs/agent-setup-verdict-decision-2026-09-19.md`) has been updated with
all three, and its header retracted.**

- **Correction 1 — the prescribed code does not compile.** The record's sketch reads
  `case checkMCPSite, checkMCPOrch:`; **those constants do not exist**. The check
  names are bare literals on the `civitaiMCPServers` table (`agent_setup_mcp.go`).
  Shipped as a derived `isMCPCheckName` lookup against that table, which is also what
  `TestREADMEVerdictExemptionsAreLedgeredAgainstTheCode`'s own docstring demands so a
  third server is covered the moment it is added. `via: command` (build failure)
- **Correction 2 — coupled edit 3 predicted the WRONG DIRECTION, and the guard was
  BLIND.** The plan said `readme_agent_setup_claims_test.go` "goes red until the
  README sentence names the exemption". Measured, in this order:
  1. code change alone → `go test ./internal/cmd` **ok**, whole package green, with
     the README sentence left false;
  2. then adding `` `mcp-site` `` / `` `mcp-orch` `` to that sentence → **FAIL**:
     *"the README's exemption sentence names \"mcp-site\" as excluded from `ok`, but
     checkCountsTowardVerdict COUNTS it."* — **which is itself false.**

  Cause: the guard derives `exemptForOther` with `"cursor"` — an agent that **IS** in
  `agentTargets` — so "other" there has only ever meant *not claude*, never *not in
  the table*. There are **three** agent classes and it sampled two. Following its
  failure message leads to de-backticking the names (round 3 of #641's measured wrong
  fix) or to reverting correct code. Widened to sample the unknown class, assert that
  identity is absent from `agentTargets`, and pin the clause whole. `via: measurement`
- **Correction 3 — coupled edit 6's `AGENTS.md` item and its forced eviction were NOT
  NEEDED.** Item **35** already routes here: its trigger sentence names *"`--check`'s
  verdict"* verbatim, its Code header already lists `checkCountsTowardVerdict`, and
  decision 35 already carries the verdict-exemption table this change extends.
  `AGENTS.md` is **unchanged at 30,493 bytes** and no eviction was paid.
  ⚠ **This is the ONE place the implementation departs from the decision as written.**
  It is reversible; the `--json` contract's owner can overrule it by minting a new
  numbered item and paying the eviction. `via: code`
- **Ruled out — that the change is a blanket exemption.** Negative control with the
  MCP config removed, same binary: `claude` ok=false rc=1 · `cursor` ok=false rc=1 ·
  `vscode` ok=false rc=1 · `other` ok=true rc=0. `via: measurement`
- **Verification matrix** for the new behavioural guard
  `TestMCPRowsDoNotFailTheVerdictForAnAgentWithNoConfigTarget`: **RED at
  `origin/main` (`07e9031`)** on `a correct setup for an unknown agent is ok and
  exits 0`; green at HEAD; a blanket-exemption mutant dies on its own distinct arm
  (`a KNOWN agent with no MCP config still fails`). The widened README ledger kills
  three mutants, each with its own message. `make ci-shallow` (depth-1 tier,
  committed state) **21/21**. `via: measurement`
- **Next probe:** none for the mechanism. What remains is **merge sequencing** —
  #668 then #667 then the doc branch then rebase #669.

### ⚠ OPEN — `pins-vs-published` is red on cli#669, and it is the repo freeze recurring a THIRD time

- as-of: 2026-09-19
- **Symptom + exact repro:** `pins-vs-published` fails on `cli#669`. Reproduce
  against live npm, from any checkout:
  ```bash
  CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold \
      -run TestScaffoldPinsSatisfyPublished -count=1
  ```
- **Observed (with values):** `STALE SCAFFOLD PIN — every app created from this
  template is born stale.` `pinned: ^0.43.0` vs `published: 0.44.0`
  (`@civitai/app-sdk`). `@civitai/blocks-react ^0.52.0` is still current.
- **Ruled out — that cli#669 caused it.** That commit touches **no** scaffold or pin
  file (`git show --name-only` lists six files, none under `internal/scaffold` or
  `templates/`), and `git diff origin/main -- internal/scaffold templates` is
  **empty**. `via: command`
- 🔴 **Ruled out — that main's GREEN is evidence the gate is healthy.** `main`'s
  `pins-vs-published` reads `success` at `2026-09-19T05:05:39Z`, but the guard queries
  **live npm**, so that is a *stored green with an expiry date* — upstream published
  0.44.0 after it ran. The discriminating control is running the guard locally NOW,
  which fails on a tree whose pin files are byte-identical to `main`. `via: measurement`
- **The fix is ALREADY OPEN — do not author a second one.** **cli#668**
  (`automation/bump-scaffold-pins`, opened 11:56:29Z by the nightly) bumps
  `^0.43.0 → ^0.44.0`, is `MERGEABLE/CLEAN` and **13/13 green**. Found by the
  `gh pr list --state open` sweep, which is the only thing that sees an unclaimed
  duplicate.
- **Leading hypothesis:** not a defect — this is the designed behaviour of a
  network-querying guard plus a fast-moving upstream. Third recurrence this arc
  (#664, #666, now #668). It is also **positive evidence for cli#530's closing
  condition**: a scheduled run produced a bump PR on its own, which is what #540 was
  meant to restore.
- **Next probe:** merge #668, then re-run #669's CI and confirm
  `pins-vs-published` goes green.

### ✅ SHIPPED — ranks 33/34/35 and v0.1.106, and the three-round arc on rank 34
- as-of: 2026-09-19

**Supersedes the `⚠ OPEN — the documented --prefix remedy` block above**, whose
"Next probe: none" is no longer current.

- **Rank 33 shipped as decided**, with three corrections the decision record needed:
  its Go snippet **did not compile** (`checkMCPSite`/`checkMCPOrch` are not constants —
  the names are bare literals on `civitaiMCPServers`); its coupled-edit-3 prediction was
  **inverted** (the README ledger guard derives its exempt set with `"cursor"`, an agent
  that IS in `agentTargets`, so it sampled two of THREE agent classes and stayed green
  while the README was false — and went red, falsely, once the rows were named); and its
  `AGENTS.md` eviction was **unnecessary** because item 35 already routes there.
- 🔴 **RANK 34 TOOK THREE ATTEMPTS, AND TWO OF THEM REINTRODUCED THE CLASS THE PREVIOUS
  AUDIT RAISED.** Both drafts detected the unreachable-CLI condition by running a login
  shell and wrote the binary's ABSOLUTE PATH into `AGENTS.md`:
  1. stripping `PATH` alone left the profile's idempotence sentinel
     (`__NIXOS_SET_ENVIRONMENT_DONE`) set, so the profile built **no PATH at all** and a
     correctly-installed CLI read as unreachable — on the maintainer's own platform;
  2. stripping sentinels fixed that, and the `cmd.WaitDelay` added for an unrelated hang
     introduced a THIRD inversion: `exec.ErrWaitDelay` returns **with the shell exited 0
     and the resolved path already on stdout**, which the probe scored "not reachable"
     while discarding that answer. Measured: 2s deadline, **30.0s** elapsed, `err=nil`.
  **Neither fix had a guard that could catch its removal** — deleting `WaitDelay` left
  the suite green; reverting the sentinel strip left the suite green. Both guards
  re-implemented their subject instead of calling it, and one asserted the opposite in
  its own comment.
- 🔴 **THE FIX WAS DELETING THE PROBE, NOT PATCHING IT A THIRD TIME.** The recurring
  fault was never a single bug: it was a **per-machine signal whose only consumer was a
  write into a COMMITTED file**, so every inversion channel put a developer's home
  directory and a bold-red false directive into a repo other people pull. 631 insertions
  became 116. Durable rule recorded in
  `claudedocs/decisions/36-agents-block-per-project.md`: this block may depend on the
  PROJECT, never on the MACHINE.
- **Ruled out — that the row was worth keeping.** Measured baseline: 8/8 trials already
  relayed the PATH line, 6/8 already warned it would not persist. The row's marginal
  value was ~0 against that. `via: measurement`
- **Next probe:** the dogfood matrix — see ranked item 36.

### ~~⏳ IN FLIGHT — rank 36: the first post-ship dogfood measurement, and a 25× cost error in this doc~~ ✅ MEASURED 2026-09-19 — the matrix completed; see "✅ MEASURED — rank 36" below. The cost and control findings here STAND; only the "in flight" status is superseded.
- as-of: 2026-09-19

- **Symptom + exact repro:** the arc shipped ranks 33 and 35 to move the 12-of-16
  failing cells recorded on 2026-09-18, and nobody had re-measured. Preconditions were
  met for the first time this session (CLI published, hosted prompt live).

- **Observed (with values) — the grader was validated in BOTH directions FIRST**, on the
  merged `#673` code, asserting the FIELDS and not just the verdict word:

  | control | verdict line |
  |---|---|
  | `ctl-neg` (bare container) | `agent=unknown check_ok=parse-error mcp_rows=unreadable login_version=none agent_shell_version=none CLOSING_CONDITION=no` |
  | `ctl-pos` (public npm install) | `agent=claude check_ok=true failed_checks=[authenticated] mcp_rows=2 login_version=0.1.106 agent_shell_version=0.1.106 CLOSING_CONDITION=yes` |
  | `ctl-profile` (`--prefix=$HOME/.local`) | `agent=claude check_ok=true failed_checks=[authenticated] mcp_rows=2 login_version=none agent_shell_version=0.1.106 CLOSING_CONDITION=no` |

  `ctl-profile` is the one that carries weight: arm A GREEN while arm B is RED, which is
  the false-green shape the README pins, and it reproduces mechanism B directly —
  `bash -lc 'civitai --version'` → `0.1.106`, `zsh -lic 'civitai --version'` →
  `command not found`.

- **Observed (with values) — the 3-trial smoke matrix** (`gemini-3.8-flash`, `node-root`,
  all three identities), graded from the containers:

  | cell | recorded 2026-09-18 | measured 2026-09-19 on v0.1.106 |
  |---|---|---|
  | `gemini × node-root × claude` | ✅ yes | ✅ `yes` |
  | `gemini × node-root × codex` | *(never run)* | ✅ `yes` |
  | `gemini × node-root × other` | ❌ **no** (mechanism A) | ✅ **`yes`** |

  The `other` cell verbatim: `agent=other check_ok=true
  failed_checks=[mcp-site,mcp-orch,authenticated] mcp_rows=2 login_version=0.1.106
  agent_shell_version=0.1.106 CLOSING_CONDITION=yes` — both MCP rows still PRESENT and
  still `false`, which is rank 33's design, and both halves of the closing condition
  green on a machine that never built the CLI. All three trials stopped `"finished"` in
  9–10 steps.

- 🔴 **`#673` EARNED ITS KEEP ON ITS FIRST RUN.** The `agent=` field is read from the
  container's own `--check` JSON, and it matched the trial-id label in all three cells —
  which is the first time identity has been *measured* rather than asserted by whoever
  named the trial.

- 🔴 **Ruled out — that a full matrix costs ~$24.** `--max-cost` is a per-trial CAP, not
  an estimate, and this doc's rank 36 quoted "24 trials at $1/trial" as though it were
  one. The 3 smoke trials cost **$0.083 total** ($0.028/trial), and this repo's own
  evidence doc records the entire 19-trial matrix of 2026-09-18 at **$0.66**. A full
  24-cell run is ~$1. `via: measurement` (the `end` records' `usage.cost`, and
  `claudedocs/refs/agent-setup-dogfood-matrix-2026-09-18.md` line 469).

- **Ruled out — that a stale model slug would kill the run.** All four default slugs and
  all three operator-chosen ones resolve live on OpenRouter, and each advertises `tools`
  + `tool_choice`, which `runner.py` requires. `via: command`
  (`curl -s https://openrouter.ai/api/v1/models`, 447 models enumerated).

- ⚠ **`~/.config/repo-cos/env` holds a LIVE PAID OpenRouter key** — $49.69 of a $50
  weekly limit remaining, no expiry — while `<devrc>/SECRETS.md` documents that file as
  belonging to a service retired 2026-09-07 and says it is safe to delete. That entry is
  wrong about the key being dead. `via: measurement` (the free `/api/v1/key` endpoint).

- **Leading hypothesis:** mechanism A is closed by the shipped v0.1.106 and mechanism B
  (rank 34 / `cli#665`) is not, so the post-ship grid should show the four `other` cells
  flipping to `yes` while every unwritable-prefix cell stays `no`. One of the four is now
  measured; three are not.

- 🔴 **Stated limit — the model axis was CHANGED mid-run at the operator's direction**,
  from the recorded `claude-sonnet-5 / gpt-5.6-terra / gemini-3.8-flash / grok-4.6` to
  `z-ai/glm-5.3-flash / xiaomi/mimo-v2.5 / deepseek/deepseek-v4-pro`. The two sets are
  DISJOINT, so rank 36's "compare the `other` cells against the recorded 4-of-4 failure"
  cannot be done cell-for-cell — the in-flight run tests whether the recorded
  MODEL-INDEPENDENCE holds on three models never tried, which is a different and arguably
  stronger question. 🔴 **The confound to watch when reading it: a cheap model that
  simply cannot drive the task produces the same `CLOSING_CONDITION=no` as a broken
  product.** Only the transcript separates those two; the verdict line cannot.

- **Next probe:** grade every finished container and tabulate by (identity, prefix
  writable), then read the transcript of any `no` cell before attributing it to the
  product:
  ```bash
  D=/home/zach/workspace/civit/cli-dogfood36/scripts/dogfood
  for t in "$D"/runs/t-{glm,mimo,dsv4}-*/; do
    id=$(basename "$t"); u=root
    case "$id" in *-nodeuser-*|*-ubuntu-*) u=dev ;; esac
    printf '%-34s ' "$id"; bash "$D/grade.sh" "$id" "$u" 2>&1 | tail -1
  done
  ```

### ✅ MEASURED — rank 36: mechanism A is closed, mechanism B is not, and agents do not take the documented remedy

- as-of: 2026-09-19

**Supersedes the "⏳ IN FLIGHT — rank 36" block above** (its status only; its cost and
grader-control findings stand).

- **Observed (with values) — the full grid.** 18 trials, all stopped `"finished"` in 7–11
  steps, graded from the containers with the `#673` grader:

  | env | prefix writable | identity | glm-5.3-flash | mimo-v2.5 | deepseek-v4-pro |
  |---|---|---|---|---|---|
  | node-root | yes | claude | ✅ yes | ✅ yes | ✅ yes |
  | node-root | yes | codex | ✅ yes | ✅ yes | ✅ yes |
  | node-root | yes | **other** | ✅ **yes** | ✅ **yes** | ✅ **yes** |
  | stale-cli 0.1.101 | yes | claude | ✅ yes | ✅ yes | ✅ yes |
  | node-user | **no** | claude | ❌ no | ❌ no | ❌ no |
  | ubuntu-apt | **no** | claude | ❌ no | ❌ no | ❌ no |

  Every `yes` reads `login_version=0.1.106 agent_shell_version=0.1.106 mcp_rows=2`; every
  `other` cell reads `failed_checks=[mcp-site,mcp-orch,authenticated]` with both MCP rows
  PRESENT and `false`, which is rank 33's design. Every `no` reads
  `agent=unknown check_ok=parse-error mcp_rows=unreadable`.

- 🔴 **MECHANISM A IS CLOSED.** The `other` identity passes on all three models. With the
  `gemini × node-root × other` smoke cell — which IS one of the recorded four — that is
  **four `other` cells measured post-ship, all `yes`**, against 4-of-4 failing before.

- 🔴 **MECHANISM B IS UNFIXED, AND THE REMEDY ANALYSIS IS AIMED AT A PATH NOBODY WALKS.**
  Enumerated, not sampled — **6 of 6** failing containers:
  `installed_at=/home/dev/.npm-global/bin/civitai bash_login=none zsh_login=none`.
  The install SUCCEEDS; nothing makes it reachable. The hosted prompt documents candidate
  **(b)** as `--prefix="$HOME/.local"`, and this doc's measured objection to (b) is that
  the stock `~/.profile` block serves bash but not zsh — **but all six agents
  independently chose `~/.npm-global`, which `~/.profile` does not add at all**, so
  neither shell finds it. That is one step WORSE than the modelled case, and the
  `ctl-profile` control reproduces (b), not what agents do. `via: measurement`

- **Ruled out — that the six failures are a capability failure of cheap models.** This was
  the confound flagged before the run: a model that cannot drive the task yields the same
  `CLOSING_CONDITION=no` as a broken product. It does not apply — every trial stopped
  `"finished"`, and all six failures hit `EACCES` (5–7 occurrences each) and attempted
  prefix remedies (8–14 mentions each). `via: measurement` (transcript scan)

- **Ruled out — that the verdict is model-dependent.** Identical outcomes across three
  models absent from every prior grid. `via: measurement`

- 🔴 **Stated limit — the comparison clause is PARTLY UNMET.** Rank 36 asks that the
  `other` cells be compared against the recorded 4-of-4 failure. **One** of those four
  (`gemini × node-root × other`) was re-measured directly and flipped; the other three
  (`gemini × stale`, `grok × node-root`, `grok × stale`) were NOT re-run, and the three
  new `other` cells are on models absent from the record. The claim "mechanism A is
  closed" therefore rests on 1 direct re-measurement plus model-independence, not on 4
  direct ones. Closing that gap is three trials at ~$0.08.

- **Observed — cost, against the `--max-cost` ceiling that framed this item.** 21 trials
  totalled **$0.1211** (mean $0.0058) against a $21 cap. Per model: mimo $0.0009–0.0013,
  glm $0.0013–0.0026, deepseek $0.0029–0.0054, **gemini $0.0248–0.0297** — the frontier
  model was 5–20× dearer than any of the three.

- **Next probe:** the three missing recorded cells, which is the whole remaining gap:
  ```bash
  D=/home/zach/workspace/civit/cli-dogfood36/scripts/dogfood
  cd "$D" && DOGFOOD_MODELS='x-ai/grok-4.6|grok' DOGFOOD_ENVS='df-node-root|noderoot|root df-stale-cli|stale|root' \
    DOGFOOD_IDENTITIES='other|' bash driver.sh
  ```
  🔴 Then the `gemini × stale × other` cell separately — and note the driver's
  non-crossed envs run the FIRST identity in the list, which is why `DOGFOOD_IDENTITIES`
  must name `other` first or the stale cell silently runs a different identity.

## Next steps (ranked)

🔴 **Ranks 1–14, 18–24 are DONE — numbering preserved** so live `claim-work` slugs keep
pointing at what they were taken for. **33, 35, 36 and 37 are DONE; 34 is the only
product work left.**

➡ **Ranks 23, 28, 30, 31 and 32 were the FORGERY effort and have MOVED** to
[`handoff-terminal-line-forgery.md`](handoff-terminal-line-forgery.md), which carries its own
closing condition. Their numbers are retired HERE rather than reused.

🔴 **THE SPLIT MINTS A SECOND SLUG FOR ONE ITEM, AND `claim-work` LOCKS PER SLUG — FORWARD IT
BY HAND BEFORE TAKING ANY OF THESE.** `claim-work --slug-for` derives the slug from the DOC, so
the same work has two canonical names and **both compare-and-swaps succeed independently**.

| retired slug (here) | now | live slug |
|---|---|---|
| `agent-setup-onboarding-32` | rank 1 of the forgery doc | `terminal-line-forgery-1` |
| `agent-setup-onboarding-23` / `-28` / `-30` / `-31` | CLOSED (#574, #605+#624, #604, #612) | none — do not take |
| `agent-setup-onboarding-33` / `-35` / `-36` / `-37` | released | free to re-take |

⚠ The only MECHANICAL backstop is the `gh pr list --state open` sweep, which is the one thing
that catches an UNCLAIMED duplicate.

15. **The docs repo does not build from a pristine `main` locally.** Unchanged, NOT
    re-verified. `/home/zach/workspace/civit/civitai-developer-docs`.
    forcing: gate
16. **`images search --help` sits 14 runes under the 1400 budget.** ⚠ RECOMMENDED FOR
    RETIREMENT.
    forcing: none
17. **Audit at MERGE time.** ⚠ RECOMMENDED FOR CONVERSION, not work.
    forcing: none
25. **cli#579 — `saferune.Strip`'s doc claims a byte-for-byte subsequence.** Explicitly inert.
    forcing: none
26. **cli#575 R2–R4.** ⚠ R2/R3 RECOMMENDED FOR DEFERRAL. R4 has the real coverage value.
    forcing: none
27. **cli#586 — three near-identical AST expression renderers.**
    forcing: none
29. ⚠ **HALF DONE — the gotcha split is still not done.** The `## Gotchas` section is
    filed by date, so forgery and agent-setup lessons remain interleaved.
    ❌ The other two playbook steps are also untouched: **evicting what has CLOSED**, and
    **demoting dated evidence to `claudedocs/refs/`** behind a pointer.
    🔴 Do NOT satisfy this by deleting an open investigation, a gotcha or a ruled-out theory.
    ⚠ The doc is now **158 KB against the tool's 65 KB ceiling** — over by 92 KB. No gate
    enforces it; it is purely what the next session must read.
    forcing: none
33. ✅ **DONE — merged (`d0b79ae`), RELEASED in v0.1.106, and MEASURED**: mechanism A is
    closed across four post-ship `other` cells. Claim released.
    forcing: gate — satisfied
34. 🔴 **THE ONLY PRODUCT WORK LEFT — `cli#665` is OPEN and its remedy is aimed at the
    wrong path.** `cli#671` shipped the `AGENTS.md` half; the install half did not.
    🔴 **NEW, MEASURED 2026-09-19 (enumerated 6 of 6, not sampled):** agents resolve
    `EACCES` by installing to **`~/.npm-global`**, NOT the `--prefix="$HOME/.local"` the
    hosted prompt documents as candidate (b). `~/.profile` does not add `~/.npm-global`,
    so **neither** `bash -lc` nor `zsh -lic` finds the binary — worse than (b), whose
    known limit was bash-only. Any fix must be designed against `~/.npm-global`.
    ⚠ An earlier draft of this item said "5 of 8 reported success anyway" — from a keyword
    regex, RETRACTED. The number that carries the item is **0 of 8** (agents that connected
    the failure to the `AGENTS.md` they had just written). 8/8 relayed the PATH line and
    6/8 warned it would not persist — the agents were never the weak link.
    forcing: gate — cli#665 is open and its closing condition is unmet
35. ✅ **DONE — docs#89 merged and verified LIVE** (7433 B, new step 4 present, old
    wording gone). Claim released.
    forcing: gate — satisfied
36. ✅ **DONE — the matrix ran, 18/18 graded, grid recorded in the investigation block.**
    ⚠ **Its comparison clause is PARTLY UNMET and that is deliberate, not an oversight:**
    3 of the recorded 4 `other` cells (`gemini × stale`, `grok × node-root`,
    `grok × stale`) were not re-run — the operator declined the ~$0.08 gap-filling run.
    So "mechanism A is closed" rests on ONE direct re-measurement plus model-independence.
    Anyone wanting the literal clause should run the `Next probe` in that block.
    closing-condition: `scripts/dogfood/grade.sh` reports the per-cell verdicts for a
    full matrix run, and the `other`-identity cells are compared against the recorded
    4-of-4 failure.
    forcing: gate — satisfied for the grid; 3 comparison cells outstanding
37. ✅ **DONE — `cli#673` merged** (`9588fb0`), verified by content with a negative
    control at `main~1`. Claim released.
    forcing: gate — satisfied

## Gotchas / decisions / dead-ends

⚠ **THIS SECTION IS NOT SPLIT BY INITIATIVE — it is filed by date and session, so
forgery and agent-setup lessons are interleaved here.** Two consequences a reader needs:
a rank number cited below (e.g. "rank 30") may name a rank that has MOVED to
[`handoff-terminal-line-forgery.md`](handoff-terminal-line-forgery.md) and no longer
appears in this file's ranked list; and the forgery-specific lessons are RESTATED there,
so finding one in both places is expected rather than a duplicate to reconcile. Splitting
this section is the remaining half of rank 29, deliberately not attempted here — moving
half of a date-keyed section makes its dates lie.

### Added 2026-09-13 — rank 19, four audit rounds, and a reduction

- 🔴 **A `str.replace` WITH NO ASSERT IS A SILENT NO-OP THAT GETS LAUNDERED INTO A
  CLAIM.** A fix-round edit did not match (`"rather than being silently treated"`
  against a file saying `"rather than silently treated"`), the script printed `ok`,
  and the next round's claims block asserted the fix as done. The audit caught it by
  `git grep` at three shas: byte-identical. **Every scripted replace must assert and
  exit non-zero on a miss** — one did on the very next round and caught a second miss.
- 🔴 **A GUARD THAT KEEPS FAILING ITS OWN PROPERTY IS EVIDENCE ABOUT THAT PROPERTY'S
  COST.** Ask what it defends against and whether that has ever happened, BEFORE
  paying for the fourth attempt. `/audit-pr`'s round 0 asked exactly this on day one —
  *"the half that found something is 302 lines, the half that found nothing is 494"* —
  and three rounds of findings landed in the half it named before anyone acted.
  Round 0's value here was not a defect; it was a sentence nobody read for two days.
- 🔴 **A NAME BLOCKLIST CANNOT BE COMPLETED BY THINKING HARDER.** Three attempts at
  "no two sites share a row" enumerated names; the next shape was always one
  identifier away. What worked was asking the STATE — *does more than one declaration
  claim this key?* — and later, not needing the property at all.
- 🔴 **A POSITIVE CONTROL THAT SHARES THE WALK IT CHECKS IS NOT A CONTROL.** A ledger's
  site-count floor was computed from the same restricted traversal that missed the
  sites, so both were blind together. The fix is a second traversal built differently,
  whose DISAGREEMENT is the failure.
- 🔴 **A CONTROL PLACED BEFORE THE ARMS IT GUARDS CAN MASK THEM.** A `t.Fatalf` on
  "this package resolved zero tests" was a strict subset of the dangling-pin arm below
  it, so a real finding was relabelled *"CONTROL failure, not a finding"* and later
  arms never ran. Split by WHICH cases are empty: all ⇒ instrument, some ⇒ finding.
- 🔴 **`git worktree prune` DOES NOT REMOVE WORKTREES.** It only clears administrative
  entries whose directories are already gone. 29 remain in this repo, each pinning a
  branch repo-globally — the stale-local-ref hazard recorded earlier in this doc.
  Remove by path: `git worktree remove --force <path>`.
- ⚠ **A BATTERY SCRIPT IN THE SCRATCHPAD HARD-CODED A LIVE WORKTREE PATH AND RAN
  `git checkout --` AGAINST IT** — which its own second line forbade. An auditor
  re-ran it and wrote into the PR's tree. Two lessons: a script's header is a claim
  like any other, and **do not re-run another round's scratch scripts** — brief
  auditors to build their own probes.
- ⚠ **A MUTANT CAN DIE OF A COMPILE ERROR FOR TWO ROUNDS WITHOUT ANYONE NOTICING.**
  The SHRANK-arm mutant referenced a constant a previous round had deleted, so it
  failed with `undefined:` rather than firing the arm it was named for. **Read the
  failure TEXT, never just rc=1.**
- ⚠ **`gh pr merge` RETURNS rc 0 WITHOUT PRINTING ANYTHING.** Verify by CONTENT —
  `git diff <the head you verified> origin/main` empty — because a squash merge never
  makes the branch head an ancestor, and read back the merge commit's own body.

- 🔴 **A hosted instruction file is a lossy channel, and this is measured, not
  theoretical.** Claude Code's WebFetch is *"lossy by design"* (per
  code.claude.com/docs/en/tools-reference): HTML→Markdown, truncate at 100 KB,
  then summarize with a small model — Claude receives the summary. Reproduced
  during this session's research: the first fetch of Cloudflare's own
  `prompt.md` came back with **invented headings** and both the OpenCode and
  Windsurf config blocks **missing**; `curl` returned the true 4,900 bytes.
  There is a public case (`oh-my-openagent#1401`) where the summarizer dropped
  one of four install flags and the install reported success. **This is the
  entire reason the logic lives in the Go binary and `prompt.md` stays short.**
  ⚠ **This said "~60 lines" and that was never true of the SERVED file** — measured
  across its whole history in `civitai-developer-docs`: 89 lines at `a94fee3`
  (2026-09-08, the first commit served verbatim), then 155, 165, and **169 lines /
  7,187 bytes today** (`8c2a6e2`, 2026-09-18). Pre-existing rot, not a claim this
  arc made; corrected here because the number is the whole point of the sentence.
  🔴 **Do not restate a line count here — it rots in another repo.** Measure it:
  `curl -s https://developer.civitai.com/agent-setup/prompt.md | wc -l`.
- 🔴 **Do not pin `@civitai/*` versions in any hosted or embedded file.** Pins
  belong in `civitai app init`, which `pins-vs-published` guards. The current
  freeze is the live demonstration: a version literal with CI behind it still
  went three minors stale; one without CI rots silently.
- 🔴 **Cloudflare's authenticity footer is a no-op and must not be copied.** It
  ends *"published at <url> so you can re-verify their authenticity"* — verified
  this session: `prompt.md.sig`, `prompt.md.sha256`, `prompt.sig` and
  `SHA256SUMS` **all 404**. The sentence lives inside the document it claims to
  authenticate. Its worst property is that it READS as an integrity control and
  thereby stops anyone asking. Ours says the file is unsigned instead.
- **Copy `tracing.md`'s shape, not `prompt.md`'s.** Cloudflare's other prompt
  (`/agent-setup/tracing.md`, 7,387 B) has an explicit non-goal, a detection
  phase with a "continue only when…" gate, a consent stop, and a real verify
  section ending *"Do not report success if validation fails"*. `prompt.md`
  itself has **no verification step at all** — its only closure is printing an
  ASCII banner.
- **Two measured constraints shaped the `AGENTS.md` template, and they cut
  against being thorough.** ETH Zurich/LogicStar (4 agents, SWE-bench Lite +
  CTXbench): repository overviews appear in 95–100% of generated context files
  and produce **zero** benefit; named tools are obeyed **1.6×** more and
  repo-specific ones **2.5×** more. So the template is commands and gotchas, and
  deliberately contains no architecture prose.
- **One level of indirection, not a tree.** Measured (arXiv 2607.17598): a flat
  index → leaf scored **0.462** vs raw **0.257**, while hierarchical
  *"consistently underperforms or harms accuracy"* (0.267). The existing
  `llms.txt` already IS the flat index, which is why no skills bundle is planned
  — adding one would build the hierarchy the data says to avoid.
- **Sizing that argues against loading docs blindly:** the `/apps` doc surface is
  ~412 KB ≈ **103k tokens across 20 pages**, and `/apps/reference/cli.md` alone
  is ~106 KB ≈ **26k tokens**. Live `llms-full.txt` is ~1.15 MB. That is
  `references/` leaf material, never something to inline.
- **`--track api` refuses with exit 2 rather than falling back to the app
  track.** "Deferred" must not read to an agent as "the API is unsupported".
- **The `authenticated` check is reported but must NOT fail `ok`.** We stop
  before auth on purpose, so a fresh unauthenticated setup is a success. This is
  the easiest thing in the feature to get backwards; it has its own test.
- **The docs base clone was `behind 18`** when this session started. Both
  subagents were briefed to `fetch` and branch from `origin/main` rather than the
  local ref. Check this before any future work there.
- **Cross-repo subagents must NOT get `isolation: "worktree"`** — that flag
  worktrees the CWD's repo, not the one the task names. The docs agent was told
  to run `git -C <docs-repo> worktree add` itself.

### Added 2026-09-09 — from two blind dogfood runs and five audit rounds

- 🔴 **BLIND DOGFOODING BEAT THE AUDIT LADDER, and the reason generalises.** Five
  adversarial rounds on cli#528 found real defects, then converged on auditing
  scaffolding the ladder itself had written (rounds 2 and 3 on docs#67 changed
  **zero** published-payload lines). Two blind runs found **seven** defects the
  ladder structurally could not: every one was a **prose claim that only fails on
  contact with a real environment** — a NixOS npm prefix, a months-old shadowing
  binary, an OAuth token that cannot be exported, a template written against the
  wrong scaffold. An auditor reading a diff cannot see any of those. **When the
  artifact is instructions, dogfood it; do not audit it.**
- 🔴 **The brief that made the dogfood work:** it may work things out from the
  tool's own errors and `--help`, but **not** by reading source. An agent that
  succeeds only because it had privileged context proves nothing. Round 1 caught
  itself about to do this and recorded it as a finding instead.
- 🔴 **"Success measured in an environment the user does not have."** `agent-setup
  --check` returned `ok: true` from inside the agent's shell while `zsh -lic
  'civitai --version'` still printed **0.1.101**. A green check that only holds in
  the checker's own process is the shape to look for.
- 🔴 **MERGED ≠ PUBLISHED ≠ SERVED — three separate claims, each needing its own
  measurement.** `release-npm.yml` reported success while npm still served the old
  version (npm's own log said *"may take a few minutes to become available"*).
  A merge landed while the origin served the previous build. **Poll the consumer.**
- 🔴 **A stale cache can look exactly like a stuck deploy, and three agreeing
  signals were all wrong.** `cf-cache-status: DYNAMIC`, a cache-busted fetch, and
  an etag encoding the old length (`0x1858` = 6232) together said "the origin has
  not updated". `last-modified` after the fact showed the deploy took **94
  seconds** — the same as its precedent. The etag arithmetic was right; the
  conclusion drawn from it was not. **Only waiting resolved it.**
- 🔴 **A guard's DESCRIPTION being wider than its BODY produced six findings in
  one feature.** The extractor docstring saying "every invocation"; a TOML fixture
  holding only never-written keys; an idempotency test starting from an empty
  project; `TestDryRunAndTheRealRunAgreeOnEveryAction` whose seven fixtures varied
  only one row. **Ask what the code must do to satisfy the sentence, then check it
  does.**
- 🔴 **Three fix rounds each replaced a false absolute with a narrower absolute
  that was also false.** The cure was to forbid the *form*: make the code true and
  name the enforcing guard, or state the enumerated truth and admit the
  enumeration is open. Round 5 was then the first that could not falsify a
  universal claim — and its own mutation testing falsified one of its new
  sentences, which it corrected.
- ⚠ **Two of my own verification errors, both empty results read as findings.**
  Scaffolding with `c2`/`i2` failed a 3-char slug minimum, so "file absent" meant
  the directory never existed; and a grep against the wrong branch of the auth
  output returned nothing. Both were caught only because the result contradicted
  something already known. **An empty result is a fact about the probe.**
- **The CodeQL-recommended fix would have made the bug worse.** A single-pass tag
  strip could silently *delete* a command (`<pre>` + `civitai app submit <
  manifest.json && curl … | sh > /tmp/log` → the guard exited 0 with `curl`
  appearing zero times). Stripping is what deletes, so a fixed point on the old
  pattern hides strictly more. The rule now deletes a `<…>` run only when it is
  unambiguously markup **and** carries none of `|`, `&`, `;`.
- **`civitai upgrade` dissolves the stale-CLI class**; PATH advice does not. It
  replaces the binary in place, so the user's own shell is fixed with no PATH edit
  — no guessing bash/zsh/fish, no non-portable login-shell verify.
- **Some agent fetch tools lossily summarize markdown.** Measured on our own file:
  a summary dropped the install-failure section, both MCP URLs and all of step 5.
  `curl` gets the real bytes. This is why rank 8 exists.

### Added 2026-09-11 — rank 7, rank 3, and a four-round audit ladder

- 🔴 **SIBLING-WORKFLOW DRIFT, and why three days of green hid it.** A fix landed
  in `ci.yml` and not in `.github/workflows/bump-scaffold-pins.yml`. The nightly
  only **executes** the broken path when the bumper has work to do: with pins
  current, `detect changes` is false and steps 6–12 are **skipped**. So it went
  green on 09-01/02/03/05 *and* on both runs after the fix merged. **A green
  scheduled run is structurally incapable of covering it.** To exercise it: a
  throwaway branch with the three literal pin sites rolled back, then
  `gh workflow run bump-scaffold-pins.yml --ref <branch>`. **Do NOT roll back
  `testdata/design-tokens.txt`** — it is provenance-stamped, `bump-pins`
  regenerates it as site 4 of 4, and step 4 resolves the drift before step 7 runs.
  Guard: `workflow_node_version_test.go`.
- 🔴 **Three claims in the previous handoff were wrong, in the reassuring
  direction.** (a) "#526 has zero checks ever run" — CI ran 2026-09-06, all 8 jobs.
  (b) "cannot be synced from here (pushing to a fork branch is not ours)" —
  `maintainer_can_modify: true`. (c) The `edgesOut` step-order hypothesis.
  **A handoff's open-investigation block reads as current forever.**
- 🔴 **`gh api --method PUT /pulls/<n>/update-branch` RE-ARMS the fork approval
  gate.** The new commit creates a run with conclusion `action_required`. So
  "there is nothing to approve" can become false as a *result* of unblocking the
  PR. Approve with `POST /actions/runs/<id>/approve`.
- 🔴 **A monitor that only emits on COMPLETED checks cannot see checks that never
  START.** One sat silent 20 minutes on `total_count: 0` (blocked at the approval
  gate) and exited 0 — indistinguishable from "still running". Emit on stall, on
  the gate re-arming, and on a run ending with no checks recorded.
- 🔴 **`audit-dispatch.py --round N` REFUSES without an `audit-claims` block, and
  the block is what makes a delta round possible.** Briefing auditors "do not
  comment on the PR" is right for findings and **wrong for the claims block** —
  each round then reconstructs context from prose. Post it as an **issue** comment;
  `gh pr view --json comments` cannot see review comments.
- 🔴 **The audit ladder on #541: four rounds, ZERO runtime defects, ~16 unsupported
  claims.** Each round's fixes generated the next round's findings. Ended on the
  stated criterion, not on convergence — rationale posted on the PR, because a
  report that ends on the escape hatch is otherwise indistinguishable from one that
  converged. Across the whole PR only **~25 executable non-test lines**.
- **A guard whose DESCRIPTION is wider than its BODY was the single most common
  finding.** Examples: a ledger advertising rename-safety while substring-matching
  prose; a matrix header asserting "measured, not reasoned" that disagreed with the
  measurement; a positive control folded into a shared counter so one arm could
  never observe a zero.
- ⚠ **My own instruments were wrong three times** — a `grep -c /dev/stdin` that
  reported empty at HEAD and near-total at base; a mutation that edited a *comment*
  quoting `node-version: "24"`; and `$B:AGENTS.md` eaten by zsh's `:A` history
  modifier (brace it: `${B}`). **Validate the instrument before reading its verdict.**
- **`subsystem_touch.py --session` returned `looked-at-nothing`** because every
  edit was made by a subagent in a worktree. `--pr 540,526,541` found the 26 real
  paths. The better a session follows the delegate-and-isolate defaults, the
  blinder that window is.

- 🔴 **A Page Rule target without a trailing `*` matches ONE path.** The first
  attempt at the fix above landed as a Page Rule targeting
  `developer.civitai.com/` (operator `matches`). Result: `/` → 200 while
  `/index.html`, `/llms.txt`, `/apps/` and `/agent-setup/prompt.md` all stayed
  403. It reads as "the rule didn't work"; it worked, on exactly one path. Needs
  `developer.civitai.com/*`.
- 🔴 **Prefer a Configuration Rule over a Page Rule for this class.** Page Rules
  are **(deprecated)**; Cloudflare's BIC page says *"To use this feature on
  specific hostnames—instead of across your entire zone—use a configuration
  rule"*, and Configuration Rules take **precedence over** Page Rules, so keeping
  both makes the interaction ambiguous.
  (https://developers.cloudflare.com/waf/tools/browser-integrity-check/)
- 🔴 **`PUT /zones/{z}/rulesets/{id}` REPLACES the entire rules array — and the
  official docs only demonstrate `PUT`.** This zone has 5 other Configuration
  Rules, two of them active `security_level` overrides; following Cloudflare's
  own example verbatim would have silently deleted all five. **Append with
  `POST /zones/{z}/rulesets/{id}/rules`.**
- 🔴 **A Configuration Rule DOES override the zone-level `browser_check`, and no
  Cloudflare doc says so.** Searched: the settings page states a precedence rule
  only for Disable RUM. Confirmed **empirically instead** — `browser_check` reads
  `"on"` while the exempted host serves 200 to a UA that setting blocks. If this
  ever regresses, re-probe rather than re-reading the docs.
- **`set_config` is non-terminating, so within `http_config_settings` LAST MATCH
  WINS.** Position matters if another rule ever sets `bic` for an overlapping host.
- **The `action` field is mandatory and is `set_config`** — a Configuration Rule
  body carrying only `expression` + `action_parameters` 400s.
  (https://developers.cloudflare.com/rules/configuration-rules/create-api/)
- **Reading Cloudflare state needs four separate token scopes**, and they fail
  differently: Zone Settings read (`9109 Unauthorized`), Config Rules read and
  Page Rules read (`request is not authorized`). 🔴 **A `pagerules` call without
  the scope returns a body whose `.result` is null — read as a count it yields a
  confident `0` for a zone that has 13.** Check `.success` before any count.
- **Related, not done:** Cloudflare's **Markdown for Agents**
  (`"content_converter": true`) sits on the same Configuration Rules surface and
  serves the same "agents fetch our docs" goal; Cloudflare runs it on their own
  docs. Worth its own decision.
  (https://developers.cloudflare.com/fundamentals/reference/markdown-for-agents/)

### Added 2026-09-11 (second session) — rank 6, and four premises that were wrong

- 🔴 **RANK 6's OWN PREMISE WAS WRONG IN THREE PLACES, AND THE PRIOR HANDOFF ASSERTED
  ALL THREE.** Measured this session:
  - **`/apps/showcase` is the COMPONENT showcase** — design tokens, `TokenGallery.vue`,
    `apps/showcase.md` — **not** an example-app gallery. The prior text read
    *"nothing reaches the seven example-app repos or `/apps/showcase`"*, which
    conflates two unrelated surfaces. There was **no** example-app page on the docs
    site at all; `/apps/examples/` 404s. `via: command`
  - **The seven repos are under `ZacxDev/`, a PERSONAL account — not the `civitai`
    org.** All seven public, unarchived. An **eighth**, `civitai/app-panorama-360`,
    *is* org-owned and was missing from the list entirely. `via: measurement`
    (`gh api repos/<owner>/<name>` on each; remotes read from the local clones)
  - **GitHub topics are NOT a usable enumeration.** `topic:civitai-app-block` returns
    **6** repos and a **different set**: `civitai-app-sensei` and
    `civitai-block-generate-from-model` carry no topics at all, while
    `civitai/app-panorama-360` is in. A topic query was the obvious "cannot rot"
    mechanism and it is already wrong. `via: measurement`
- 🔴 **AN ALL-404 SWEEP TOLD ME NOTHING, AND I NEARLY BUILT ON IT.** I measured
  `civitai.com/apps/run/<slug>` → 404 for all seven and read it as "these apps are not
  published". The positive control killed it: **`civitai.com/apps` itself 404s** while
  `civitai.com/models` returns 200 — the whole `/apps` route family is closed to an
  unauthenticated fetcher. So the 404s are a fact about the ROUTE, not about the apps,
  and no run-URL can be verified from here. **The page therefore links repos (which
  are independently verifiable) and asserts no run-URL.** `via: command`
- 🔴 **A STATUS-CODE CHECK CANNOT DETECT THE ROT IT EXISTS TO CATCH: GitHub
  301-REDIRECTS A RENAMED REPO.** `curl`/API against the old URL returns **200** for a
  repo that no longer lives there. Any example-app link guard must compare the API's
  **`full_name`** against the owner/repo in the URL, and fail on mismatch — as well as
  on 404 and `archived: true`. This is the single most important property of
  `scripts/check-example-apps.mjs`. `via: code`
- **WHY THE LIST IS NOT EMBEDDED IN THE CLI.** The managed block is **written to disk
  in users' projects**, so a repo literal that rots there cannot be recalled — only a
  re-run of `civitai agent-setup` rewrites it. A CI guard in `civitai/cli` could only
  ever protect UNSHIPPED copies. One URL under Civitai's control keeps the block's rot
  surface at exactly one link, and adding/removing an example becomes a docs edit
  rather than a CLI release every user must then re-run. This also preserves the
  measured flat-index-one-hop shape (flat index → leaf 0.462 vs hierarchical 0.267).
- 🔴 **`AGENTS.md` HAD 58 BYTES OF HEADROOM — measure before planning an item.**
  `wc -c AGENTS.md` = 30,442; `agents_size_test.go:229` `agentsMaxBytes = 30_500`;
  `:254` `agentsMaxBytesCeiling = 30_600` bounds any raise, and the guard's own failure
  message forbids raising it. My initial "append a brand-new numbered item" instruction
  was **unaffordable** and was overturned mid-flight; extending item 36's trigger cost
  **+1 byte**. Measure the ceiling before briefing anyone to add an item. `via: measurement`
  🔴 Note the spelling: naming the next free number in the `item N` form makes
  `agents_xrefs_test.go` fail, because that number does not exist yet. The guard's own
  source comment records the same constraint about itself.
- ⚠ **BOTH SUBAGENTS STOPPED WITH EVERYTHING UNCOMMITTED.** The parent process exited;
  neither had committed, so `apps/examples.md`, `check-example-apps.mjs`,
  `agent_setup_docs_test.go` and two sets of file edits were sitting in worktrees, one
  stray `checkout` from silent deletion. **On resume, the first instruction to each was
  "commit before anything else" — and it worked.** A long-running file-modifying agent
  should be told to commit incrementally, not only at the end.
- ⚠ **A subagent "stopped by user" notification did NOT mean the user rejected it.**
  Both agents died together because the parent Claude Code process exited. Read the
  sibling notification before concluding intent.

### Added 2026-09-11 (session 2) — the audit ladder on rank 6

- 🔴 **A GUARD I HAD PERSONALLY WATCHED WORK WAS UNPROTECTED, AND MY CONTROLS COULD NOT
  HAVE TOLD ME.** I ran three controls on `check-example-apps.mjs` and reported the
  rename check verified: injected `facebook/jest` → red, stripped all links → red,
  baseline → 8 live. All true. The auditor then set the `full_name` comparison to
  `if (false && …)` — the exact simplification the file's own 30-line banner forbids —
  and **every one of those controls still passed**: PR gate rc 0, daily half rc 0
  reporting `8 verified live · 0 rotted`, the jest fixture printing `✓`. **Running a
  guard proves it works TODAY; only mutating it proves anything about it STAYING
  working.** Those are different claims and I reported the weaker one as the stronger.
  Fixed in docs#72 with an embedded must-FAIL fixture table that runs on every
  invocation, plus a degeneracy guard — verified by me: mutant → rc 1
  (`expected verdict "fail", got "ok"`), fixtures deleted → rc 1
  (`SELF-TEST DEGENERATE — … 0 RENAMED row(s)`). `via: measurement`
- 🔴 **MY OWN DEGENERACY CONTROL WAS A BROKEN INSTRUMENT AND I NEARLY REPORTED ITS
  rc=1 AS A PASS.** Deleting fixture rows with a line-grep produced a `SyntaxError`, so
  node exited 1 before the guard ran at all. rc=1 from a syntax error and rc=1 from the
  guard firing are indistinguishable by exit code. The fix was mechanical: `node --check`
  FIRST, then read the verdict. **Validate the instrument before reading its verdict
  applies to the controls you write while validating someone else's instrument.**
  `via: command`
- 🔴 **A `t.Skipf` ON THE FIRST TRANSPORT ERROR TURNED THE MERGE GATE INTO A PASS.**
  `agent_setup_docs_test.go`'s link probe abandoned the whole test on the first
  unreachable URL — so one DNS hiccup on `/apps/guide/` meant `/apps/examples`, the link
  the gate exists for, was **never fetched** and the run reported `ok`. Reproduced by me
  both directions: pre-fix (`7b7d5f1`) `--- SKIP` → `ok`, *"skipping, not failing"*;
  post-fix (cli#553) `--- FAIL … DEAD LINK` preceded by `checked 3 of 4 link(s); 1
  unreachable, 2 live, 1 gone`. **A guard that gives up on the first error is a guard
  that reports success about work it did not do.** `via: measurement`
- 🔴 **TWO ENVIRONMENTAL "PRE-EXISTING FAILURE" CLAIMS I RELAYED WERE WRONG, AND I HAD
  NOT RE-DERIVED EITHER.** I briefed a fix agent that `gen:appblocks:messages` fails
  because a sibling `../civitai` checkout is ABSENT — the mechanism is the **opposite**
  (it fails because the sibling IS present, so the script prefers the drifted live file
  over the snapshot; under `APPBLOCKS_SNAPSHOT_ONLY=1`, what CI grades with, it exits 0).
  And `check:built-site` failing on a stale `public/appblocks/cli.json` **did not
  reproduce** — `prebuild` regenerates it. Both came from an earlier subagent's report
  that I passed along verbatim. **A subagent's environmental diagnosis is a hypothesis,
  and relaying it launders it into an assertion.** `via: measurement`
- 🔴 **`/apps/showcase` IS THE COMPONENT SHOWCASE — the prior handoff conflated it with
  an example gallery**, and that conflation is what made rank 6 read as "a pointer is
  missing" rather than "the page does not exist". Also wrong in the same item: the seven
  repos are under **`ZacxDev`**, a personal account, not the `civitai` org; an eighth
  (`civitai/app-panorama-360`) is org-owned and was missing entirely; and a GitHub topic
  query is NOT a usable enumeration (`topic:civitai-app-block` → 6, a different set; two
  of the seven carry no topics). `via: measurement`
- 🔴 **AN ALL-404 SWEEP IS AN EMPTY RESULT AND IDENTIFIES NOTHING.** I measured
  `civitai.com/apps/run/<slug>` → 404 for all seven and nearly concluded "not published".
  The positive control killed it: **`civitai.com/apps` itself 404s** while
  `civitai.com/models` → 200. The whole `/apps` route family is closed to an
  unauthenticated fetcher, so the 404s are a fact about the ROUTE. The page therefore
  links repos and asserts no run-URL. `via: command`
- 🔴 **GITHUB 301-REDIRECTS A RENAMED REPO, SO A STATUS-CODE CHECK CANNOT SEE THE ROT IT
  EXISTS FOR.** `api.github.com/repos/facebook/jest` → **200** with
  `full_name: jestjs/jest`. Verified independently. Any link guard must compare
  `full_name`, not status. `via: measurement`
- ⚠ **A URL PARSER THAT FALSE-FAILS IS WORSE THAN ONE THAT MISSES.** The first guard
  reported `✗ RENAMED` and exit 1 on a link ending `?tab=readme-ov-file` — literally what
  GitHub's own "copy link" button produces — with a remedy telling the maintainer to
  change the URL to the one they already had. Fixed structurally (enumerate what GitHub
  CAN issue, so it cannot be out-spelled by the next punctuation mark). `via: code`
- ⚠ **A DEPLOY THAT LOOKS STUCK FOR 5½ MINUTES.** Merged 16:19:20Z; `llms.txt` still
  served the OLD 215-line body through six cache-busted polls and flipped to 192 lines at
  **16:24:59Z**. The precedent in this doc was 94 s. **Do not diagnose from the first few
  polls** — this surface has a recorded case where three agreeing signals all said "the
  origin has not updated" and only waiting resolved it. `via: measurement`
- ⚠ **BOTH SUBAGENTS ONCE STOPPED WITH EVERYTHING UNCOMMITTED** (parent process exited).
  `apps/examples.md`, `check-example-apps.mjs` and `agent_setup_docs_test.go` sat in
  worktrees, one stray `checkout` from deletion. On resume the first instruction to each
  was "commit before anything else" and it recovered all of it. **Brief long-running
  file-modifying agents to commit incrementally, not only at the end.** A "stopped by
  user" notification did NOT mean rejection — both died from the same process exit; the
  sibling notification is what showed it.
- ⚠ **THE QUEUE WAS WORKED CONCURRENTLY AND I DID NOT NOTICE UNTIL AFTER THE FACT.**
  cli#549 and cli#548 were merged by another actor while my audits ran, and a cli#550 I
  never saw landed on `main` reporting the `Python-urllib` 403 root-caused (Cloudflare
  Browser Integrity Check). The `agent-setup-onboarding-6` claim was **already released**
  when I went to release it. The merge ORDER I had committed to was respected anyway
  (docs 05:42Z, cli 05:52Z) — by luck of timing, not by the lock. **`claim-work` failing
  open means a taken claim is not a guarantee; re-read `gh pr list` and the claim state
  before assuming you are the only writer.** `via: measurement`
- **AGENTS.md headroom is the binding constraint on adding an item, and it is tiny.**
  30,442 B at session start against `agentsMaxBytes = 30_500`; `agentsMaxBytesCeiling =
  30_600` bounds any raise and the guard's own message forbids it. A new item needs
  ~180 B (trigger + pointer) because `agents_evidence_test.go` asserts SET EQUALITY
  between `→ evidence:` pointers and `claudedocs/decisions/` files — so a new decisions
  file REQUIRES a new item. Extending item 36's trigger cost +1 B, then +10 B for the
  audit fix. Now **30,453 / 30,500**. Measure before briefing anyone to add an item.

### Added 2026-09-11 (session 2, closing sweep)

- 🔴 **THE RANKED LIST WENT STALE WITHIN MINUTES BECAUSE PARALLEL SESSIONS WERE DRAINING
  IT, AND A STALE LIST IS A DUPLICATE-WORK GENERATOR, NOT A COSMETIC PROBLEM.** Between
  this session writing "ranks 8–11 untouched" and re-checking ~20 minutes later, **rank 8
  shipped** (docs#74, `11311ab`) and **rank 9 shipped** (cli#557, `6fce127`, issue #542
  CLOSED) — neither by this session. A `/resume` reading the list as written would have
  claimed and re-done both. **Re-verify every ranked item against live state immediately
  before writing the list, not when you formed it** — `gh issue view <n> --json state`
  and `gh pr view` are one command each. `via: measurement`
- 🔴 **THREE RESOLVED INVESTIGATIONS STILL CARRY "STILL OPEN" HEADINGS, because the
  Open-investigations section is APPEND-only and retiring the old heading is a separate
  manual step nobody does.** Live now: two `⚠ STILL OPEN — Python-urllib 403` headings
  (lines ~212, ~255) sit ABOVE the `✅ RESOLVED` block that supersedes them, and a
  `🔴 STILL OPEN — bump-scaffold-pins` sits above its own RESOLVED block. **Measured
  2026-09-11: the urllib 403 is GONE** — `Python-urllib/3.11`, `python-requests` and
  `ClaudeBot` all return **200** from `developer.civitai.com`. A reader who stops at the
  first matching heading gets the opposite of the truth. The handoff skill's own rule
  says to retire the superseded heading in the SAME delta; the append semantics make that
  easy to skip. **When you append a RESOLVED block, say so in the REPLACE'd status
  section too** — that one is overwritten and cannot accumulate contradictions.
  `via: command`
- ⚠ **`claim-work` released itself out from under this session.** The
  `agent-setup-onboarding-6` claim was **already gone** when this session went to release
  it (`nothing to release — ref does not exist`), and `agent-setup-onboarding-8` was
  taken by a different session mid-arc. The lock FAILS OPEN by design, so a held claim is
  not a guarantee of exclusivity and an absent one is not proof the work is unowned.
  **Sweep `gh pr list --state open` as well** — that is the only thing that sees an
  UNCLAIMED duplicate. `via: measurement`

### Added 2026-09-11 (third session) — ranks 4, 8, 9, 10, 11

- 🔴 **"Does NOT close #399" CLOSED #399. GitHub's parser does not read
  negations.** A commit body sentence — *"Does NOT close #399 (safeTerm unpinned
  at ~20 of 25 sampled sites)"* — contains the literal `close #399`, and the
  closing-keyword parser matched it. The PR body said the same thing in words, so
  **both human-readable surfaces were correct and the issue closed anyway**; no
  reviewer could have caught it. Reopened, with the mechanism recorded on the
  issue. **To reference an issue without closing it, do not use the keyword at
  all** — "see #399", "related to #399". `close`/`closes`/`fixes`/`resolves`
  followed by `#N` closes it regardless of the surrounding sentence.
- 🔴 **A handoff doc written on a branch describes the `main` it was CUT from,
  not the one it MERGES into.** cli#558 was created 20:19:20Z and merged
  20:58:23Z — after cli#559 (20:23) and cli#560 (20:33). It replaced the ranked
  list with a pre-merge snapshot: rank 10 read *"Confirmed still OPEN"* when it
  was closed, rank 4 read open when it was live, and the doc contained **zero**
  mentions of docs#73, cli#559 or cli#560. It also credited ranks 8 and 9 to "a
  parallel session" — they were the same session it was sweeping up after. The
  merged-tree rule applies to DOCS, not just code.
- 🔴 **`default_type` does not set a Content-Type for an extension nginx already
  knows.** It is the FALLBACK for an unresolvable extension — which is why it
  works for `.md`, absent from `mime.types`. `.json` IS present, so the map wins
  and `application/ai-catalog+json` came back as `application/json`. An empty
  `types { }` block clears the map so `default_type` applies. It reads as a
  no-op; deleting it silently reverts the declared type.
- 🔴 **One server-level `add_header Link` reaches almost none of a site.** nginx
  inherits `add_header` only into blocks that declare none of their own.
  Measured under a real nginx: with a single server-level copy, **four of five
  document routes served NO `Link`** — `/apps/guide.html`,
  `/agent-setup/prompt.md`, `/.well-known/ai-catalog.json` and **`/` itself**
  (resolved through an internal redirect into `location ~* \.html$`), while
  `/llms.txt` kept it and is the control proving the directive worked at all.
  **Spot-checking the homepage would have passed that config.**
- 🔴 **The docs deploy takes 83–384 s, not ~90 s — an early 404 is not a
  failure.** Measured three times this session on the same pipeline: 83 s (#72),
  340 s (#73's predecessor) and 384 s (#73). The handoff's earlier "~94 s"
  precedent is the FAST end. Mid-run I called a deploy "stalled" on the strength
  of it and was wrong; only waiting resolved it. `last-modified` on any page is
  the origin's own value and is the honest signal — a CDN read is not.
- 🔴 **Registering a guard in a bidirectional ledger: do ONE side first and watch
  it fail.** Adding `pkg/civitai/snippet_args_ledger_test.go` to item 38 means
  editing `decisions/38`'s header AND `item38CommentedFiles`, which
  `TestItem38FileLedgerMatchesTheDecisionHeader` pins equal. Updating the header
  alone was done deliberately and the pin caught it by name. That is the cheapest
  possible proof the registration is real rather than asserted.
- 🔴 **A "previously called X" note re-arms the rot it documents.** The first
  draft of cli#560 recorded each fixed citation as "previously
  `TestSomethingThatDoesNotExist`" — which keeps that identifier in the
  repo-wide unresolved count the fix exists to reduce. It would have left the
  count at 20. `decisions/38` warns about exactly this and it was still walked
  into; the measurement caught it, not the reading.
- **When a verbatim-pinned constant must change, the pin tells you the order.**
  `readJSONNote` is pinned against `wantReadJSONNote`. The sequence that
  satisfies it: change the constant, watch the pin FAIL, re-run the behavioural
  halves that the wording is supposed to describe, and only then re-type the
  expectation. Pasting the new string into both places passes the test while
  proving nothing.
- ⚠ **Four of my own instruments were wrong, each in the reassuring direction.**
  `git merge … | tail && echo ok` tested `tail`'s status, not the merge's, and
  reported three failed merges as successes. `pgrep -f nginxprobe` matched the
  shell running it and killed its own caller (exit 144). zsh ate `$b:refs/…` as
  an `:r` history modifier, producing `…ledgerefs/remotes/…` — brace it,
  `${b}`. And `awk '$2=="pending"'` over `gh pr checks` misread `Analyze (go)`,
  whose NAME contains a space, so the status column is not `$2`; use
  `--json statusCheckRollup` instead of parsing the table.
- **A single-branch clone silently fetches nothing.** The docs repo's
  `remote.origin.fetch` is `+refs/heads/main:refs/remotes/origin/main`, so
  `git fetch origin` leaves every other branch unresolvable and a test-merge
  "succeeds" against refs that do not exist. Fetch explicit refspecs, or
  `refs/pull/<n>/head` for a fork-based PR.
- **A bare `git worktree add` has no toolchain.** `go` was not on PATH in a fresh
  worktree of `civitai/cli` even though it is in the base clone; the full test
  suite silently did not run. Use an absolute path or copy the environment before
  reading any verdict from a worktree.
- 🔴 **TWO SUBSYSTEM-INDEX SCOPES ARE STRANDED LOCAL-ONLY AND WILL NEVER SYNC —
  `civitai-developer-docs` and `civitai-app-requests`.** They exist in the frozen
  mirror (`~/.claude/analyze-service-index/`, files `0444`) and in NEITHER the
  synced cache (`~/.cache/subsystem-store/`) nor the pod. The docs one holds
  `apps.md`, 4,037 B, dated **2026-09-02** — the Cairn cutover day, which is the
  likely mechanism. Consequences, both live: `cairn create --scope
  civitai-developer-docs` is REFUSED `[not-found]`, so nothing new can be
  recorded for that repo at all; and a `/resume` in it reads an EMPTY scope while
  a whole entry sits on this host's disk. **Do not "fix" it by writing locally —
  that is the failure, not the remedy.** It needs the pod-seeding path in the
  `cairn` skill.

### Added 2026-09-11 (session 2 close) — ranks 12–13, the #399 check, and a real merge race

- 🔴 **TWO SESSIONS WROTE THE SAME HANDOFF DOC AND GITHUB CAUGHT IT — `gh pr view` IS WHY.**
  This session's handoff PR came back `CONFLICTING / DIRTY` because a parallel arc had
  merged #561 into the same file. Forcing would have **regressed their closures of ranks
  4, 10 and 11** — their update was newer and strictly better on those items. The fix was
  to fast-forward, re-read THEIR version, and rebuild this delta on top of it.
  🔴 **The numbering collided too**: they filed 14–17, this session had drafted its own
  14–16 for different substance. **The second merger renumbers its OWN items** — hence
  18–19 here, and their 14 (cli#399) absorbed this session's structural evidence rather
  than becoming a duplicate entry. `via: measurement`
- 🔴 **GITHUB'S KEYWORD PARSER CLOSED AN ISSUE THE COMMIT EXPLICITLY SAID IT DID NOT
  CLOSE.** cli#557's commit `6fce127` reads *"Does **NOT** close #399"*; GitHub matched
  `close #399`, ignored the negation, and closed it (auto-closed 16:56Z, reopened 20:59Z).
  Both human-readable surfaces were right; the automation was not. **Never write
  `close[s|d] #N` / `fix[es] #N` / `resolve[s] #N` in a commit or PR body unless you mean
  it — negation does not help.** Write "does not address #399", or reference it with no
  keyword. `via: measurement`
- 🔴 **A NETWORK CHECK ADDED ZERO API BUDGET BECAUSE IT USES A DIFFERENT HOST, AND A RATE
  LIMIT PROVED IT.** Manifests come from `raw.githubusercontent.com`; liveness from
  `api.github.com`. On a rate-limited run the sweep verified **3 of 8** repos live and
  **8 of 8** scope lists — the numbers moved independently. **When adding a network check,
  ask which HOST it hits before assuming it shares the old one's quota**; and note this
  was learned from an accident, not a designed experiment. `via: measurement`
- 🔴 **THE GUARD AND THE NOTIFIER NEED OPPOSITE FAILURE POLICIES, AND SAYING SO IS THE
  DESIGN.** `check-example-apps.mjs` skips loudly and **exits 0** on an unreachable
  network — a false-fail makes a gate people click through. `drift-notify.mjs` **exits 1**
  when it cannot reach the API — a notifier that cannot notify is the failure it exists to
  prevent. Same repo, same sweep, deliberately inverted. **Do not "harmonise" them.**
- 🔴 **A TEST GREPPED A WORKFLOW'S RAW `jobs:` TEXT — COMMENTS INCLUDED.** Correcting a
  stale claim in `cli-snapshot-refresh.yml` ("The ONLY elevation in this repo") tripped
  `test-refresh-cli-snapshot.mjs`, because the scope name written in a COMMENT matched its
  forbidden-scope grep. The fix was to move the comment to the file header, **not** to
  weaken the guard. **A guard that reads raw YAML cannot tell a comment from a
  declaration** — expect this when documenting permissions. `via: command`
- ⚠ **MY OWN GREP PIPELINE RETURNED A CONFIDENT `0` TWICE BEFORE I NOTICED.**
  `xargs -0 command grep` fails — `command` is a shell BUILTIN, not an executable — and
  `which grep` on this host returns a shell FUNCTION body, which `find -exec` then cannot
  run. Both printed **0** matches for a pattern with **153** real hits. Fixed by calling
  `/run/current-system/sw/bin/grep` directly **and running a positive control first**
  (`package cmd` → 67). **A zero from a pipeline you have not positive-controlled is a
  fact about the pipeline.** This is a third distinct shape of the same repo-recorded
  hazard, after `$?`-after-a-pipe and ugrep's `.gitignore` blindness. `via: command`
- ⚠ **DELIVERY VERIFIED AGAINST A FAKE IS NOT DELIVERY VERIFIED.** Nine scenarios over
  real HTTP against a local fake issues API is good engineering and still leaves "it will
  notify" unproven — the real service can differ on label auto-creation, org policy and
  token scope. Filed as rank 18 rather than folded into rank 12's completion, **because
  collapsing them is how "merged" becomes "working".** `via: assumed` (the gap is
  structural and stated by the implementer; nothing was measured against the real API)

### Added 2026-09-12 — ranks 14 and 18, and a PR this session broke

- 🔴 **MERGING A LEDGER PR BROKE AN EXTERNAL CONTRIBUTOR'S LEDGER PR, AND THE COLLISION
  HAD ALREADY BEEN WRITTEN DOWN.** cli#564 and cli#554 both edit
  `internal/cmd/safeterm_userinput_test.go`; #564 rewrote its AST walk. A parallel
  session recorded the `#564`/`#554` collision in a *different* handoff doc
  (`handoff-external-issue-513-numeric-username.md`) **before** #564 merged, and this
  session did not read that doc before merging. **A sibling handoff doc is part of the
  pre-merge check when two PRs touch one package** — `gh pr list --state open` showed
  #554 the whole time and I read it as unrelated because its title is about `images`.
  `via: measurement`
- 🔴 **GITHUB'S KEYWORD PARSER CLOSED #399 A SECOND TIME — FROM THE TEXT WARNING ABOUT
  THE FIRST.** cli#562's body **quoted** the string `close #399` while documenting the
  earlier accident; the parser matched the quote. Closed 21:49:58Z with `commit_id: null`
  and nothing on `main` fixing it. **Quoting the keyword is writing the keyword.** The
  discipline that worked: brief every agent never to write it *even inside quotes*, then
  close the issue **by hand** with the evidence. #564 shipped and #399 stayed open until
  deliberately closed. `via: measurement`
- 🔴 **AN AUDIT FOUND TWO DEFECTS INSIDE THE GUARD BUILT TO PREVENT THEM, AND BOTH WERE
  "DESCRIPTION WIDER THAN BODY".** (a) `formatFileList`'s ledger row said the test asserts
  "name **and type**"; the fixture never set `Type`, so deleting
  `safeTerm(dashIfEmpty(f.Type))` at `download.go:658` left the **full suite green**.
  (b) The RATCHET's comment said "the number cannot go UP" while the body only checked
  `>`, so covering a row without lowering the cap banked headroom a later commit could
  spend silently. Both reproduced by me before and after the fix; (a) now fails naming
  `ffltype` with the name column still rendering correctly, (b) now fails
  `RATCHET HEADROOM: 24 … but maxUncoveredSafeTermFuncs is 25. Lower it to 24 in this
  commit.` `via: measurement`
- 🔴 **A LEDGER CAN ONLY DEMAND ROWS FOR FUNCTIONS THAT ALREADY CALL THE THING IT
  GUARDS.** #564's `GREW` fires per enclosing function *that calls `safeTerm`*. A renderer
  that never calls it is invisible — which is precisely where the remaining download-path
  holes live (rank 20). **When a guard is keyed on the presence of the call, it cannot see
  its absence.** The harder instrument keys on *rendering a server-supplied struct field*;
  that is deliberately NOT part of #566's closing condition. `via: code`
- ⚠ **A TRANSIENT EDITOR DIAGNOSTIC REPORTED A COMPILE ERROR THAT WAS NEVER COMMITTED.**
  An unused `bytes` import was flagged in `safeterm_renderers_test.go`; the committed file
  had no such import, the worktree was clean at the pushed head, and CI was 13/13. **An
  LSP diagnostic describes the buffer, not the commit** — check `git show HEAD:<path>`
  before treating one as a finding. `via: command`
- ⚠ **MY OWN ISSUE-FILING WAS REFUSED BY A HOOK, CORRECTLY.** I wrote
  `## Suggested closing condition` — *leading* text, where the gate allows only trailing —
  and my section restated the remedy rather than naming an observable end state. The gate
  says plainly that it cannot check the latter and that passing is a floor, not a verdict.
  Rewrote with a command that exits non-zero until the fix lands. `via: command`
- ⚠ **A THIRD BROKEN-PIPELINE ZERO, THIS TIME IN A CHECK-SETTLING LOOP.**
  `gh pr checks | awk '{print $2}'` reads `(actions)` — not the status — for any check
  whose NAME contains a space, so my terminal-state counter never fired while all 13
  checks had been green for minutes. After `xargs -0 command grep` and the shell-function
  `grep`, that is three distinct shapes in one session, all returning a confident wrong
  answer instead of an error. **Positive-control every counting pipeline before reading
  its verdict.** `via: command`

### Carried forward from a REPLACE heading — two measurements worth keeping

These sat under `State now` / `Next steps` and would have been deleted by the next status
rewrite. They are the evidence behind two closures, and re-deriving either costs an hour.

- **cli#399's closing condition, measured both arms** (rank 14, cli#564). The condition was
  *deleting `safeTerm(...)` at a sampled site reddens the suite, demonstrated on ≥1 site
  that survives today*. Verified with the mutant compiling in each arm, so the red is the
  guard and not a build break: baseline `cf5e4a8`, full `internal/cmd` suite →
  **rc 0, `ok`, 41.3s**; after #564 → **FAIL** naming `U+200B, U+202E, U+2800`.
  The issue was then closed **by hand**, never by a commit keyword.
- **The scale #564 actually moved, re-derived by full sweep, not asserted**: **36 of 59**
  functions were deletable-while-green at base; **0 of 60** are now. All 35 covered rows
  verified by deletion, 35/35 red under only the test their row names. Counted with a
  positive-controlled pipeline (`package cmd` → 67 before quoting): **153 `safeTerm(`
  occurrences** in `internal/` non-test sources = 151 calls + 1 declaration + 1 comment.

### Added 2026-09-12 (rank 21 close-out) — a merge race and a fourth keyword shape

- 🔴 **A CLOSING KEYWORD CAN RIDE IN ON A COMMIT NEITHER PR SURFACE SHOWS — a fourth
  shape, and the first that no review of the PR could catch.** cli#569's title and body
  are clean, and its body states explicitly that the residual work *stays tracked on*
  issue #552. But its first commit — carried unmodified from xsvm's #554 — is subjected
  `fix(images): … (closes #552)`. **A squash merge's default body is the concatenation of
  the branch's commit messages**, so merging on the default would have shut the issue the
  PR depends on staying open. The three prior shapes were all about text *someone in this
  repo wrote*; this one is inherited. **Before squash-merging any branch carrying commits
  you did not author, read `git log origin/main..<head>` subjects and pass an explicit
  `--body`.** Then re-read the issue state. `via: measurement`
- 🔴 **TWO SESSIONS ON THE SAME ACCOUNT MERGED THE SAME PR, AND `gh pr merge` REPORTED
  `already merged` WITH rc 0.** A parallel session merged #569 at 04:36:15Z; this
  session's `gh pr merge … --body-file` ran seconds later and returned
  `! Pull request civitai/cli#569 was already merged`, **exit 0**. The merged body is not
  the one this session wrote. Outcome was fine — the other session had independently
  caught the keyword hazard and sanitized its own body — but **rc 0 from `gh pr merge`
  does not mean your merge, or your body, landed.** Read `gh pr view --json mergeCommit`
  and diff the merge against the head you verified before claiming either. `via: command`
- 🔴 **`claim-work` said "THIS SESSION (you already hold it)" for work a DIFFERENT session
  was actively finishing.** The owner-id is host-scoped, not session-scoped, so two
  concurrent sessions on one host read each other's claims as their own — the lock is
  invisible in exactly the configuration where it is most needed. The claim had also
  already been released by the other session when this one went to release it. **Treat a
  matching owner-id as "this HOST", and sweep `gh pr list` / the PR's own timeline before
  assuming you are the only writer.** `via: measurement`
- 🔴 **A VERIFICATION IS A CLAIM ABOUT A SHA, AND THE MERGE IS A SEPARATE CLAIM.** This
  session verified `e2a7dd6`; the merge produced `e3f2608` from a parallel actor. The two
  were reconciled by `git diff --quiet e2a7dd6 origin/main` → **empty**, which is what
  licenses "what landed is what I verified". Without that diff the verification would have
  described a head nobody merged. **Run it whenever you did not perform the merge
  yourself.** `via: command`
- ⚠ **A PIPED `$?` MISREAD THE CLAWGATE FIELD PROBE — the fourth instance in this arc.**
  `clawgate_handoff.sh field <doc> | head -3; echo "rc=$?"` printed **0** ("a field is
  already there, leave it alone") when the tool's real status was **1** ("no field, add
  one"). `head` succeeded; the tool did not. The skill warns about this for `resolve` and
  the same trap sits one line below on `field`. **`out=$(cmd 2>&1); rc=$?` — always.**
  `via: command`
- **The production split in cli#569 is the durable decision, not the guards.** Inline and
  tabular fields (`username`, `baseModel`, `nsfwLevel`, `url`, `model`, `sampler`, `cfg`,
  `steps`, `seed`, resource type/name/weight/hash) route through **`safeTermSingle`**;
  multi-line free text (`prompt`, `negative`) stays on **`safeTerm` + `indentContinuation`**.
  The rule is *one line per terminal line-break rune AND one tabwriter cell* for anything
  in a column or after a label, and *indented multi-line* for anything that is prose. A
  tab is never legitimate inside a single-line cell: the delimiter cannot also be content.
- **`sanitizerComposers` is a LEDGER, not a file exemption, and the difference is the
  point.** It names functions *in `safeterm.go`* that may delegate to `safeTerm`, keyed by
  **enclosing function** and requiring the file match too. The obvious alternative —
  allowlisting the argument NAME `"s"`, which #554 proposed — is wrong invisibly, because
  `s` is the commonest local name in Go and one entry blinds the harness across ~67 files.
  Measured both directions this session. **Do not "simplify" it back to a name or a file.**

### Added 2026-09-12 (rank 20 close-out) — a five-round ladder, a semantic merge conflict, and a live tooling defect

- 🔴 **ROUND 0 FOUND A GUARD THAT SHOULD NOT EXIST — INSIDE A PR WHOSE ENTIRE PURPOSE
  WAS ADDING GUARDS.** #572 shipped `safeTerm(dir)` on the `create output directory`
  error. It is **unreachable for server bytes**: `filepath.Base` puts the uploader's
  name in the **leaf**, so `filepath.Dir(target)` yields only user-typed `--out` /
  `--out-dir` / `--root` or a `layoutFolders` constant — and stripping user-typed input
  is precisely what `internal/saferune`'s rule **forbids**. 🔴 **It had already passed a
  20-mutant sweep.** The killing subtest handed `downloadOne` a target with the hostile
  name as a **directory component**, a shape `targetPath` cannot produce. **BREAKABLE IS
  NOT REACHABLE** — a mutation sweep proves a guard can go red, never that production
  can reach it. Removing it *restored consistency*: `generate_output.go:413` carries the
  same ungated `create output directory %s: %w` line, so the gate was the outlier, not
  the exception. **Ask of every new guard: what production input reaches this line?**
  `via: measurement`
- 🔴 **ROUNDS 1→4 EACH RETRACTED A JUSTIFICATION THE PREVIOUS ROUND HAD WRITTEN WHILE
  FIXING THE ROUND BEFORE IT.** The chain, in full, because the shape is the lesson:
  round 1 wrote a false claim (*"`%q` already escapes the class"*) → round 2 retracted
  it, **but the retraction landed in a test header while the authoritative ledger row
  kept the false sentence** → round 3 found round 2's retraction contained its own false
  clause (*"the https/parse refusals are not `*url.Error` at all"* — the **parse**
  refusal IS one, and the two classify to **different published exit codes, 5 vs 1**) →
  round 4 corrected that and swept the shape everywhere. **The cure was to STOP
  SUPPLYING REPLACEMENT JUSTIFICATIONS**: state the measured fact, and name which
  clauses of the old sentence survive. A fix that reaches for a fresh explanation is how
  round N+1 gets its finding. `via: measurement`
- 🔴 **THE LADDER WAS STOPPED ON THE PROSE-PAYLOAD CRITERION, NOT ON A CLEAN ROUND — AND
  SAYING SO IS THE POINT.** Round 3's range shipped **zero** executable change
  (`download.go`'s 39 changed lines were **100% comments**); round 4's shipped **one
  test-fixture line**. When the payload is prose, the attribution gate **structurally
  cannot fire** — there is nothing for a delta audit to attribute. The rationale was
  posted to the PR, because **a report ending on that escape hatch is otherwise
  indistinguishable from one that converged.** No round ever returned zero findings.
  `via: measurement`
- 🔴 **A PREDICTED MERGE COLLISION HAPPENED, AND THE CONFLICT WAS SEMANTIC, NOT
  TEXTUAL.** #573 merged mid-flight. File-level fencing held for 20 of 21 paths — but
  both PRs edited `internal/cmd/safeterm_coverage_test.go`, and
  `maxUncoveredSafeTermFuncs` is an **equality** assertion. The three trees read **25
  (base) / 22 (main) / 24 (PR)**. 🔴 **Taking either side's number would have been
  wrong, and git would not have said so.** Resolved by **union of the rows** plus
  reading the correct value — **21** — off the assertion's own failure output.
  **MEASURED, NOT COMPUTED**: with a ratchet, the test already knows the answer, so make
  it tell you rather than doing the arithmetic. `via: measurement`
- 🔴 **THE LITERAL-`\uXXXX`-IN-COMMENTS DEFECT IS LIVE, NOT HISTORICAL.** The editing
  tooling reproduced it **mid-round, on freshly typed characters**. `gofmt`, `go vet`
  **and** `golangci-lint` are all **blind** to it — confirmed green with the escapes
  present, so the repo's whole lint stack is not a control for this.
  **Diff-review for `\uXXXX` before committing prose**, every time. `via: measurement`
- 🔴 **GREPPING FOR A RETRACTED PHRASE RETURNS HITS FROM THE RETRACTION QUOTING IT.**
  Twice this session a count of **1** read as "still broken", and reading the context
  showed a correct retraction doing its job. **A COUNT CANNOT DISTINGUISH A CLAIM FROM
  ITS RETRACTION** — and in a repo whose convention is to keep refuted text visible with
  a correction above it, that is the *normal* case, not an edge case. Read the context
  before acting on any count over this doc or these test headers. `via: command`
- 🔴 **A LOCAL BRANCH REF WAS STALE BECAUSE AN AGENT WORKTREE HELD IT, AND BRANCHING
  FROM THE LOCAL NAME WOULD HAVE SILENTLY LOST COMMITS.** `fix/download-path-safeterm`
  read **`036151b`** locally while the PR head was **`35c7213`** — **10 commits** behind,
  and `036151b` is **not an ancestor of `origin/main`**. The holder was
  `.claude/worktrees/agent-a937852b…`. This is the concrete harm behind the 16 stale
  worktrees noted in *State now*: a worktree pins a branch **repo-globally** at whatever
  commit it stopped on. **Always `git checkout -b <new> origin/<branch>`, never the bare
  local name** — the repo's own `AGENTS.md` says this and the failure is silent.
  `via: measurement`
- **`audit-dispatch.py` REFUSED to emit a claims block for round 0** — `round 0 has no
  fixes to claim`. **Correct behaviour, recorded so nobody "fixes" it**: the claims block
  starts at the round that first *fixes* something, because a claims block is a record of
  what a fix asserted. Round 0 is a pure findings round and has nothing to assert.
- ⚠ **`gh issue create` WAS BLOCKED TWICE BY THE CLOSING-CONDITION GATE, AND BOTH
  REFUSALS WERE LITERALLY ACCURATE.** (a) The body arrived via a **shell substitution**
  the gate could not read, so *"could not read the body-file path"* was exactly true.
  (b) A compound `heredoc && gh` command meant the **PreToolUse block prevented the
  heredoc from ever writing the file** — so the gate then correctly reported a body file
  that did not exist. **Write the body to a real file in a SEPARATE call, then invoke
  `gh` in its own call.** Chaining the two makes the gate's diagnosis describe a file
  your blocked command never created. `via: command`

### Added 2026-09-12 (rank 20 close, post-merge sweep)

- **`audit-dispatch.py --round 0 --emit-claims` REFUSES, correctly, and it is worth knowing
  before you reach for it.** `🔴 REFUSING TO EMIT an audit-claims block: round 0 has no fixes
  to claim.` Round 0 reports requirements and deletion candidates and does not move the
  ladder, so a `round=0` block would be anchorable and the next delta round would diff FROM
  the tip round 0 merely READ — attributing the whole change to a round that fixed nothing.
  **The claims block starts at the round that first FIXES something.** Round 0's verdict goes
  on the PR as prose instead. Exit 4. `via: command`
- **A handoff doc's own `State now` SHA is stale by exactly one commit the moment it merges,
  and that is structural, not rot.** `deacd19` (this doc's merge) advanced `main` past the
  `f0cb748` the block names. Every handoff has this property. **Do not "fix" it by rewriting
  `State now`** — that heading REPLACES, and its `Carried forward` subsection is the one the
  file itself records as "keeps being dropped under this REPLACE heading". Re-verify the SHA
  live instead; it is one `git log -1` away. `via: measurement`
- ⚠ **`clawgate_handoff.sh resolve` answered `rc=5` for this session** — 0 tasks, with its
  positive control showing 10 links for a *different* session, so the board is reachable and
  the token accepted. **That zero is a real reading and NOT a clean bill of health**: an
  unknown session id also answers `200` with an empty array. No `clawgate-task:` field is
  recorded on this doc, and none should be invented to fill the blank. `via: command`

### Added 2026-09-14 — eight PRs, five audit rounds, and a reduction that deleted coverage

- 🔴 **A DELETION JUSTIFIED BY A MEASUREMENT INHERITS THAT MEASUREMENT'S SCOPE.** cli#583's
  reduction removed a guard's owner key on the measurement *"every package has had exactly
  ONE saferune call site, always"* — true, and about COLLISIONS only. The key also carried
  RELOCATION. Renaming `safeTerm`'s body to delegate to a new function then walked
  `o.aspectRatio` — which the repo's own ledger names as must-never-be-stripped — into
  `saferune.Strip` with `go test ./...` **fully green**. Red pre-reduction, green after.
  **Before removing a mechanism, enumerate what it DOES — not what the measurement covers.**
- 🔴 **A `str.replace` WITH NO ASSERT IS A SILENT NO-OP THAT GETS LAUNDERED INTO A CLAIM.**
  An edit did not match (`"rather than being silently treated"` vs `"rather than silently
  treated"`), the script printed `ok`, and the next round's claims block asserted it as
  done. Caught by `git grep` at three shas: byte-identical. **Assert every scripted
  replace and exit non-zero on a miss** — one did on the very next round and caught a
  second miss.
- 🔴 **A GUARD THAT KEEPS FAILING ITS OWN PROPERTY IS EVIDENCE ABOUT THAT PROPERTY'S COST.**
  Three attempts at "no two sites share a row" each enumerated NAMES and each was beaten by
  the next shape (`vs.Names[0]` → a multi-name spec; `_` as a key SENTENCE → a row spelling
  it; `_` as a FLAG in one branch → `func _()` and `func init()`). What ended it was asking
  whether the hazard had ever occurred: it had not, and the property was replaced by a
  COUNT. **Ask before paying for the fourth attempt.**
- 🔴 **ONE VALUE, PRINTED TWICE, GATED ON ONE HALF — THREE TIMES IN ONE SESSION.** #566's
  original shape; then `safeTermErr` leaving the `%w` cause raw while the `%s` operand was
  gated; then **in a test**, where `Contains(err, userDir)` was satisfied by the wrapped
  cause and the mutant SURVIVED. The fix in each case is to COUNT occurrences, not to
  `Contains`.
- 🔴 **A POSITIVE CONTROL THAT SHARES THE WALK IT CHECKS IS NOT A CONTROL**, and **a control
  placed BEFORE the arms it guards can MASK them** — a `t.Fatalf` on "this package resolved
  zero tests" was a strict subset of the arm below it, so a real finding was relabelled
  *"CONTROL failure, not a finding"*. Split by WHICH cases are empty: all ⇒ instrument,
  some ⇒ finding.
- 🔴 **`make ci` GREEN while `make lint` RED, twice** — once on two dead struct fields, once
  on staticcheck **ST1018** (a raw U+200B in a fixture). ST1018 is the exact rule AGENTS.md
  records as invisible to `make ci`. **Run both, every time.**
- 🔴 **A MUTANT CAN DIE OF A COMPILE ERROR FOR TWO ROUNDS.** A SHRANK-arm mutant referenced
  a constant a previous round had deleted, so it failed `undefined:` rather than firing the
  arm it was named for. **Read the failure TEXT, never just rc=1.**
- 🔴 **`grep -c` COUNTS LINES, `grep -o | wc -l` COUNTS OCCURRENCES.** A count of the same
  thing came back 27 and 31 from the two spellings, and the number was about to be written
  into a comment. Third counting pipeline to mislead in this arc.
- 🔴 **`git worktree prune` DOES NOT REMOVE WORKTREES** — it only clears entries whose
  directories are already gone. Remove by path: `git worktree remove --force <path>`.
- 🔴 **THE BASE CLONE WAS ON ANOTHER SESSION'S BRANCH, AND `--ff-only` IS WHAT CAUGHT IT.**
  Without that flag the merge would have landed `origin/main` **into their branch**. Their
  commit was pushed, so nothing was at risk — but the class is live in this repo.
- 🔴 **A PR MERGED OUT FROM UNDER A RUNNING AUDIT.** cli#590 was merged by a parallel
  session, which had run its own delta round 2 and fixed two of three findings post-range.
  The third was still live on `main` and became cli#594. **A delta round's range can be
  overtaken; re-read the PR's state before acting on the report.**
- ⚠ **A SCRATCH BATTERY SCRIPT HARD-CODED A LIVE WORKTREE PATH AND RAN `git checkout --`
  AGAINST IT** — which its own second line forbade. An auditor re-ran it and wrote into the
  PR's tree. **Brief auditors to build their own probes and never re-run another round's
  scratch scripts.**
- ⚠ **A TIMING TEST FLAKED UNDER FIVE CONCURRENT `go test` PROCESSES**
  (`TestAppStatusDriftLookupGetsADeadlineOfItsOwn`). Passes in isolation, at base, and on
  re-run. **Discriminate load from an assertion by running the control at base**, not by
  re-running.
- ⚠ **`gh pr merge` RETURNS rc 0 SILENTLY.** Verify by CONTENT — and when the base has
  MOVED, `git diff <verified-head> origin/main` is the WRONG check (it shows every parallel
  change). Verify the payload's own markers are present instead.

### Added 2026-09-14 — round 2 on cli#596, and a handoff that was two rounds stale

- 🔴 **A HANDOFF'S "NOT AUDITED" IS A CLAIM ABOUT WHEN IT WAS WRITTEN, NOT ABOUT THE PR.**
  This session was told to run round 0; two rounds had already run and their fixes were
  pushed. The tell was cheap and was not in the doc: `gh pr view <n> --json commits` showing
  three commits where the doc implied one. **Read the PR's commit list and its comments before
  believing any ladder state a doc asserts** — `audit-dispatch.py --round N` will happily
  assemble the round you ASK for.
- 🔴 **TWO OPERATOR DECISIONS, RECORDED SO THE NEXT ROUND DOES NOT RE-LITIGATE THEM.**
  (a) **Fix scope stays NARROW**: #596 fixes only what its own delta broke or left unpinned;
  `generate.go`'s six sibling sites go to a new issue (rank 30). (b) **The `j.target` gate at
  `generate_output.go:517` STAYS**, with the decision written down: gating a mixed-origin
  target path is the documented `saferune` exception and `downloadBlobTo:420` already does it.
  The residual is real and must be stated, not closed — a `--out-dir` carrying a stripped rune
  is printed without it, so the refusal names a directory that is not the directory.
- 🔴 **A LEDGER CAN BE STRUCTURALLY BLIND TO THE VALUE IT EXISTS TO CLASSIFY.** `bareIdentArgs`
  keys **bare identifiers**, so a selector expression like `j.target` is invisible to it and no
  row was ever demanded for the round-1 gate. The mutation proving it: the auditor's mutant
  listed `target` in five functions and `downloadOutputs` was not among them. **When a ledger
  reports nothing about a site, ask whether its parser can SEE that site's expression shape.**
- 🔴 **A MUTATION SWEEP RUN FROM A COPY INSIDE THE GO MODULE ROOT SCORES FAKE KILLS.** A
  pristine `package cmd` copy left in the module root made all three mutants return
  `[setup failed]` — three deaths that proved nothing. Put the `cp -a` copy OUTSIDE the module
  root, and `rm -f <copy>/.git` first (a worktree's `.git` is a FILE, so a commit in the copy
  lands on the real branch).
- ⚠ **`git status -sb` IS HOW YOU CATCH A PUSH THAT WOULD MISS ITS PR.** `cli-596b`'s branch
  is `fix/generate-blob-forgery-r0` tracking a remote that reads `[gone]`, while the PR's head
  is `fix/generate-blob-forgery`. A bare `git push` there creates a second remote branch and
  the PR never moves — silently, with rc 0.
- ⚠ **The base clone was switched to another session's branch for the SECOND session running**
  (`test/pin-429-header-before-message` this time, `docs/handoff-sweep-scope` last). Neither
  was noticed by a survey — both by a command that happened to print the branch.

### Added 2026-09-14 — a gate that does not exist, asserted for several sessions

- 🔴 **RANK 29 CLAIMED A RED GATE THAT NOTHING RUNS, AND THE CLAIM CAME FROM A TOOL'S WARNING
  TEXT.** `handoff_doc.py` prints *"`test_no_handoff_doc_exceeds_its_budget` will go RED on
  `main`, and it fails for EVERYONE"* whenever a doc exceeds 65,536 B — but that test is
  **devrc's**, rooted at devrc via `Path(__file__)...parent.parent.parent`, with a corpus
  function named `this_repos_corpus()`. A handoff doc living in `civitai/cli` is outside its
  corpus entirely. The warning is correct about the BYTES and wrong about the CONSEQUENCE, and
  the consequence is the half that got written into the ranked list as `forcing: gate`.
  **Measured three ways before retracting:** the test runs green (9 passed); `command grep -r`
  plus `find` over the `civitai/cli` checkout find no such test; and cli#603 took the 97 KB doc
  through all 13 civitai/cli checks SUCCESS. `via: command`
- 🔴 **THE TRANSFERABLE SHAPE: a tool's warning is a claim about the TOOL'S OWN REPO unless it
  says otherwise.** This one names a test by function name, which reads as a specific,
  checkable fact and is exactly why nobody checked it. A cross-repo handoff inherits the
  warning verbatim and the falsity is invisible at the point of writing.
- ⚠ **Consequence for the queue, stated because it is not obviously good:** `forcing: none`
  items are declared not eligible to be worked, so correcting this makes rank 29 LESS likely to
  be picked up, not more. That is the honest state — the cost is real (~24k tokens of context
  on every `/resume`) but no external signal is asking for it. Do not re-mint a fake gate to
  raise its priority.

### Added 2026-09-14 — five audit rounds, and three of them found the previous round's prose

- 🔴 **AN ENUMERATION IN A COMMENT CANNOT BE COMPLETED BY THINKING HARDER — DELETE IT, DO NOT
  EXTEND IT.** A paragraph naming "the surfaces reachable from this function" went 3 sites → 6
  → still incomplete across three successive corrections, each one written to fix the last, and
  each reading as exhaustive to the next reader. Round 5 found `printOutputURLs` and the poll
  reporters still missing. What ended it was deleting the list, stating the property, naming the
  ledger + issue as authoritative, and writing **"do not add a fourth list"** into the comment.
  Same shape as the name-blocklist lesson already in this doc — it recurs because each attempt
  looks like it is *almost* complete.
- 🔴 **THE AUDIT LADDER'S ATTRIBUTION GATE CANNOT FIRE WHEN THE FIXES ARE COMMENTS IN PAYLOAD
  FILES.** It needs two consecutive rounds changing zero PAYLOAD lines; a comment edit in
  `generate_output.go` counts as payload. The series here was 6, 0, 14 — never two zeros —
  while **zero executable lines moved after `a3a8ca0`**. Switching to an "executable payload"
  count would make it fire instantly and would be re-deciding the payload class mid-ladder,
  which is exactly how that gate gets disarmed without anyone choosing to. **Name the condition
  and stop on the stated criterion instead.**
- 🔴 **A ROUND'S "SAFE TO MERGE" VERDICT IS NOT THE STOP SIGNAL, AND ITS "I CALL THIS CLEAN" IS
  NOT EITHER.** Round 3 reported two 🟢 findings, said each *"changes what a reader
  concludes"*, and then declared itself clean. By the ladder's own rule that makes them
  findings. Both were real: one of them was a coverage row crediting another function's lines.
  **Read the findings, not the verdict.**
- 🔴 **`gh pr merge` RETURNS rc 0 AND PRINTS NOTHING — AND THE OBVIOUS CONTENT CHECK IS WRONG
  WHEN `main` HAS MOVED.** `git diff <head> origin/main` is non-empty because *other people's*
  work landed, not because yours did not. **Diff only the files you touched, and run a positive
  control** (a file you did NOT touch that differs) so an empty result is not indistinguishable
  from a broken comparison. Ancestry is useless here: a squash merge never makes the branch head
  an ancestor.
- 🔴 **A HANDOFF'S LADDER STATE IS A CLAIM ABOUT WHEN IT WAS WRITTEN.** This doc said #596 was
  "not audited, next action round 0"; two rounds had already run and their fixes were pushed.
  The tell was one command the doc did not carry: `gh pr view <n> --json commits` showing three
  commits where the doc implied one. `audit-dispatch.py --round N` will happily assemble
  whichever round you ASK for. **Read the PR's commits and comments before believing any ladder
  state.**
- ⚠ **A `git worktree`'s local branch can track a remote branch that no longer exists, and a
  bare `git push` then silently creates a SECOND remote branch instead of updating the PR**
  — rc 0, no warning, PR head unchanged. `cli-596b`'s branch is `fix/generate-blob-forgery-r0`
  while the PR's head is `fix/generate-blob-forgery`; `git status -sb` showed
  `...origin/fix/generate-blob-forgery-r0 [gone]`. Push with an explicit refspec and confirm
  `gh pr view --json headRefOid` moved.
- ⚠ **A MUTATION SWEEP RUN FROM A COPY INSIDE THE GO MODULE ROOT SCORES FAKE KILLS** — a stray
  `package cmd` copy there makes every mutant return `[setup failed]`. Copy OUTSIDE the module
  root, and `rm -f <copy>/.git` first (a worktree's `.git` is a FILE, so a commit in the copy
  lands on the real branch).

### Added 2026-09-18 — 19 blind trials, and two instruments that lied first

- 🔴 **A `2>&1` CAPTURE OF `--check --json` IS UNPARSEABLE, AND THE FAILURE READS AS
  "NO JSON AT ALL".** The command prints its `Error: agent setup incomplete…` line
  to **stderr, AHEAD of** the JSON on stdout, so a merged capture makes `jq` fail —
  and a `jq` failure is indistinguishable from "the CLI is not installed". My first
  grader scored a *correctly installed, genuinely failing* setup identically to a
  bare container. **Read stdout only.** Anything consuming that payload — the
  hosted prompt included — has the same exposure.
- 🔴 **`jq '.ok // "absent"'` CANNOT SEE `ok: false`.** jq's `//` treats `false` as
  empty exactly like `null`, so the alternative fires and a real failure is reported
  as a missing field. `if has("ok") then (.ok|tostring) else "absent" end`. The tell
  is a boolean field that never once reports `false` across a run you know contains
  failures.
- 🔴 **BLINDNESS SHOULD BE A MOUNT NAMESPACE, NOT AN INSTRUCTION — AND A CONTAINER
  MAKES IT ~40 LINES.** Every earlier dogfood harness in this repo enforced
  blindness by telling the agent not to look, then spent rounds arguing about what
  that did and did not bind. A model whose only tool shells into a throwaway
  container cannot read the repo because the repo is not in its filesystem. The
  deleted `dogfood-sandbox.sh` complex (7 rounds, ~24 findings, none about the CLI)
  was the credentialed version of this problem; **un-credentialed, the whole thing
  is cheap** — no token, no spend meter, no ledger, no jail.
- 🔴 **I CONFOUNDED THE DIMENSION I WAS THERE TO MEASURE, AND THE GRID LOOKED
  CLEAN.** The matrix gave `claude`/`gpt` a known agent identity and `gemini`/`grok`
  none, so a 16-cell result that partitioned perfectly by model was equally well
  explained by identity. **A result that looks decisive is when to ask what else
  predicts it.** Three swap trials settled it in ten minutes and inverted the
  reading: identity decides, model does not. Had I shipped the first grid, "Gemini
  and Grok fail the onboarding" would have been the finding — and it is false.
- 🔴 **AN AGENT'S FINAL REPORT IS NOT EVIDENCE ABOUT THE MACHINE** — and my first
  attempt to quantify that was itself a bad instrument. A regex over the final
  reports matching `success|complete|all set|done` scored *"Setup is complete"* the
  same as a report that merely lists what it did, and scored a persistence WARNING
  as nothing at all; it produced "5 of 8 declared success", **which I put in a
  commit message before checking it.** Read by hand: 8/8 relayed the PATH line,
  **6/8** also warned it does not persist, 2/8 did not. **The defect survived the
  correction and got sharper — it is not that agents lie, it is that `AGENTS.md`
  is written against a binary that only exists in the installing shell — but the
  number was wrong and the framing it supported was wrong with it.** The grader,
  which measures the container, was right throughout; the prose instrument I
  reached for to describe *why* was not. **Validate the cheap instrument too.**
- ⚠ **A "measured on a real machine" prose remedy can be correct AND still not
  work.** `prompt.md`'s `--prefix` fallback is accurate, the agents executed it
  verbatim, and it still ended in an unusable install 8 times out of 8. Accuracy of
  an instruction and sufficiency of the outcome are different claims — the whole
  reason this arc's condition has a login-shell half.
- **The `other` agent path is not an edge case, and treating it as one is how it
  went unnoticed.** Gemini CLI, Aider, Cline, Continue and anything shipped after
  the table was written all land there. It was 8 of 19 trials here purely by
  accident of how I assigned identities.

### Added 2026-09-15 — rank 30, and three ways a guard can be wrong

- 🔴 **A GUARD CAN BE WALKABLE BY THE VERY OPERAND IT GUARDS.** To stop a one-line assertion
  falsely accusing a deliberately multi-line suffix, I split the rendered error on
  `". The server reported:"` and asserted on the head. `workflowID` is server-origin and lands
  BEFORE that marker, so an id of `"wf1. The server reported: ok\nSaved …"` moves the real
  newline into the discarded tail. MEASURED: with the gate reverted, the test PASSED while a
  fully forged line survived. `assertNoControlEscapes` could not see it either — it matches the
  two-character `\n` that `%q` emits, not a raw newline. **The cure was structural, not a
  better spelling:** the no-reason case asserts one-line over the WHOLE string (nothing to
  split) with a `HasSuffix` control — the suffix is appended LAST, so it cannot be displaced by
  anything spelled earlier — and the reason case SUBTRACTS the suffix it injected rather than
  searching for a marker.
- 🔴 **AN INSTRUMENT THAT ENUMERATES THE GUARDED CANNOT FIND THE UNGUARDED.** #604's closing
  condition said to check with `git grep 'safeTerm('`. That finds operands that ALREADY have a
  gate, so it is structurally incapable of finding one with none — and three such operands
  existed, one on the same `fmt.Fprintf` as a gate the PR was adding. What found them:
  enumerate every value interpolated into a writer or `fmt.Errorf` (102 sites across the two
  files) and trace each origin. **Write the closing-condition CHECK against the hazard, never
  against the helper's name.**
- 🔴 **A REQUIREMENT AUTHORED BY A PRIOR AUDIT ROUND IS THE HIGHEST-SCRUTINY CLASS.** Round 0
  attacked #604's condition — which I had written — and was right twice: the blind instrument
  above, and a row-count invariant that would have fired a FALSE forgery on a valid response
  (`Deliverable` does not require a URL, so nil-URL outputs legitimately leave numbering gaps).
  Round 0 also declined to manufacture a deletion (4 examined, 0 cut) and argued the one
  plausible cut should stay.
- 🔴 **THE FREEZE CLASS RECURRED, AND A DISPATCHED RUN IS NOT THE SCHEDULED ONE.**
  `pins-vs-published` (REQUIRED, `enforce_admins: true`) went red between the nightly's 14:03
  run and 22:29 CI — app-sdk 0.39→0.40, blocks-react 0.49→0.50 — freezing every open PR.
  Unfrozen by dispatching the repo's own `bump-scaffold-pins` via `workflow_dispatch`, which is
  better than hand-editing pins because it builds the scaffold against the new SDK on a clean
  runner. It opened #615 NOT as a draft, which per #530/#540 means that validation passed.
  ⚠ This says nothing about whether the nightly will catch the next publish.
- ⚠ **I FILED A DUPLICATE ISSUE (#609) BECAUSE I SKIPPED THE OPEN-PR SWEEP.** #606 was already
  in flight on the same schema drift and landed while I wrote it. `claim-work`'s own rule says
  the `gh pr list --state open` sweep is the only thing that catches an UNCLAIMED duplicate.
  I then nearly filed a second duplicate for the revendor bot's failure — **#607 already
  existed, auto-filed by the bot**. Searching first is what caught that one.
- ⚠ **A "POSITIVE CONTROL" AGAINST AN IDENTICAL TREE PROVES NOTHING.** Verifying a merge by
  `git diff <head> origin/main` over the touched files, my control sha happened to hold the
  same content, so it returned empty too — an empty control read as confirmation. Pick a sha
  that genuinely differs (97 insertions, 9/9) before believing the zero.

### Added 2026-09-19 — the first blind multi-model dogfood, and an 11-round ladder

- ⚠ **WHERE THE RANK-33 DELIBERATION LIVES, now that the ranked entry is a one-liner.**
  The contested framing — the refuted "third instance of a shape" argument, the
  `agent_setup.go:739-742` docstring that says the opposite, and the internal
  contradiction with §A2's rejection of `--agent cursor` — is preserved in TWO places
  and was deliberately not deleted: the `## Open investigations` block headed
  "✅ DECIDED (2026-09-19) — an agent the CLI does not know can never reach `ok: true`",
  and in full in `claudedocs/refs/agent-setup-verdict-decision-2026-09-19.md`. A ranked
  item is a work queue entry, not an argument record; do not reconstruct the argument
  there.

- 🔴 **A CONTROL THAT CANNOT PRODUCE THE DISCRIMINATING INPUT CANNOT FIND THE DEFECT** —
  and three drafts of this repo's own docs claimed otherwise. `ctl-neg` yields no JSON and
  `ctl-pos` yields `ok: true`, so neither can ever produce `ok: false`, which is the ONLY
  input on which the buggy `jq '.ok // "absent"'` differs from the correct one. Under the
  bug both controls still match their tabulated expectations exactly. **Validating an
  instrument in both directions proves it can DISCRIMINATE; it is not coverage of any
  particular defect.**
- 🔴 **THE GRADER MEASURED A DIFFERENT SHELL THAN THE CONDITION NAMED — a FALSE GREEN.**
  Arm B ran `bash -lc "zsh -lic '…'"`. The outer bash login sources `~/.profile`, whose
  stock `if [ -d "$HOME/.local/bin" ]` block prepends a directory **zsh never reads**.
  Same container, same install: wrapped → `0.1.105`, direct → `command not found`. The
  frozen condition names the zsh form. **When a condition names a shell, run THAT shell —
  a convenience wrapper is a different measurement.**
- 🔴 **A MEASUREMENT OF A PRECONDITION THE REMEDY ITSELF ALTERS IS NOT EVIDENCE ABOUT THE
  REMEDY.** "`~/.local/bin` does not exist, therefore that prefix is unavailable" — the
  remedy is what creates it. Cost: a whole round, and a wrong conclusion published to an
  issue.
- 🔴 **CONFOUNDING IS INVISIBLE WHEN THE GRID LOOKS CLEAN.** The first matrix bound agent
  identity to the model row; the result partitioned perfectly by model and was equally
  well explained by identity. **A result that looks decisive is exactly when to ask what
  else predicts it.** Three swap trials inverted the reading.
- 🔴 **AN ABBREVIATION IS NOT A PREFIX YOU MAY EXTEND.** I took 7-char shas from
  `git commit` output and appended a guessed 8th character — four of five did not resolve.
  Then, correcting that, I padded an already-wrong 8-char prefix to **40 characters with
  invented hex** and certified it because it was "full-length". **A fabricated identifier
  gets MORE credible as it gets longer, because length is what gets checked instead of
  resolution.** `git rev-parse` it; `git cat-file -t` it before publishing it.
- 🔴 **CORRECTING THE COUNT OF A THING IS NOT DOING THE THING — and the corrected count is
  what makes the miss invisible.** One round changed a sweep note from "three surfaces" to
  "four" and then swept three. The next found a FIFTH (the PR body) that no list had ever
  named. The one after found a SIXTH — the cairn store, the only surface that outlives the
  PR. **Sweep by SEARCHING for the claim (repo + `gh pr view --json body` +
  `gh issue view` + the store), never by walking an enumeration.**
- 🔴 **FIXING THE SWEEP INSTRUMENT WHILE LEAVING ITS BOUNDARY UNCHANGED YIELDS A CLEAN
  RESULT OVER AN INCOMPLETE SET, WHICH READS EXACTLY LIKE COVERAGE.** That is how the
  store survived a search-based sweep that had just replaced a list-based one.
- 🔴 **A STORE ENTRY OUTLIVES THE PR; CORRECT IT IN PLACE.** `cairn put`, not an appended
  correction bullet — a reader who stops at the first matching bullet must not be left
  with the false version. This arc's own handoff documents that failure mode for
  append-only sections.
- 🔴 **WHEN A CLAIM HAS BEEN WRONG THREE TIMES, STOP WRITING A BETTER ONE.** Three drafts
  of one remedy list, each replacing a false claim with a differently false one. The cure
  was to abandon the RANKING and state the measurement, with all three retracted drafts
  recorded so a fourth is not derived. Same for an attribution the record could not
  settle: state the proved negative and leave the positive unstated.
- 🔴 **BLINDNESS SHOULD BE A MOUNT NAMESPACE, NOT AN INSTRUCTION.** A container whose
  filesystem lacks the repo cannot be read from, whatever the agent decides. The deleted
  `dogfood-sandbox.sh` complex was the CREDENTIALED version of this problem (7 rounds, ~24
  findings, none about the CLI); un-credentialed it is ~250 lines.
- 🔴 **AN AGENT'S FINAL REPORT IS NOT EVIDENCE ABOUT THE MACHINE.** Grade the container.
  Separately: the cheap regex I reached for to *describe* why was itself a bad instrument
  and produced a number I put in a commit message before checking it.
- ⚠ **`prompt.md` is ~60 lines was never true** — 89 → 155 → 165 → **169 lines / 7,187
  bytes**. Do not restate a line count for a file in another repo; measure it.
- ⚠ **`gh pr merge` returns rc 0 silently.** Verify by CONTENT with a negative control —
  a squash merge never makes the branch head an ancestor.
- ⚠ **Merge `main` in; do NOT rebase this branch.** The ladder's `audit-claims` blocks
  reference commit shas, and a rebase rewrites every one of them.
- ⚠ **An unquoted heredoc executes backticks.** I posted a claims comment whose sentence
  about *how to sweep for claims* had its two backticked command names executed and
  deleted — `across repo +  + .` — with the shell's errors interleaved into unrelated
  output. Use `<<'EOF'`, and read back what you published.
- ⚠ **`claudedocs/decisions/` is mechanically 1:1 with a numbered `AGENTS.md` item**
  (`TestEvidencePointersAndFilesAreTheSameSet`, `TestSplitTableCoversEveryEvidenceFile`),
  and `AGENTS.md` has **7 bytes** of headroom against its 30,500 ceiling. A decision that
  is made but not implemented belongs in `claudedocs/refs/`; the implementing PR adds the
  item, pays the eviction, and moves the file. 🔴 **A cross-reference guard reads prose of
  the form `item <N>` as a pointer INTO that numbered list** — so writing the number you
  intend to add dangles against a list that does not have it yet. It caught me twice: once
  in the decision record, and again HERE after I had fixed it there. Name it "the entry I
  was adding", never by number.

### Added 2026-09-19 (close-out) — where the ladder was NOT pointed

- 🔴 **ELEVEN AUDIT ROUNDS WERE POINTED AT THE MECHANISM, AND NOTHING WAS POINTED AT THE
  VERDICT.** The ladder's recurring finding was one substitution — a better-sounding
  claim standing in for the measured one — and it kept catching it in guards, counts,
  attributions and shas. It did not catch the same substitution in **the single line
  that says whether the arc is done**: the frozen condition asks for **a** blind run to
  reach a working setup, 4 of 16 did, and every report said "NOT MET" because a stricter
  reading *sounded* more honest. **A close-check reads the CONDITION'S OWN WORDS; an
  audit of the work cannot supply that, because the condition is not in the diff.**
- 🔴 **"STRICTER" IS NOT A SYNONYM FOR "MORE HONEST".** Reporting NOT MET felt
  conservative and was simply wrong about the text. When a verdict is stricter than its
  written criterion, that is still a mis-report — and it is the direction nobody
  challenges, which is exactly why it survived eleven rounds and a merge.
- ⚠ **The frozen condition and the useful question had drifted apart, and only the
  measurement exposed it.** It asks *"can an agent do this at all?"* (a feasibility
  question, answerable by one success). The dogfood answered a different one: *"does it
  work across agents and environments?"* — 4 of 16. Both are worth knowing; conflating
  them is what produced the wrong verdict. Per this doc's own rule the second opens a
  NEW arc rather than extending the frozen one.
- ⚠ **A guard that fires on a READ can be right when nothing it names is wrong.** The
  handoff write-back guard fired on `handoff-terminal-line-forgery.md` — a doc this
  session read while sweeping and deliberately did not touch (its "condition is not met"
  belongs to a different arc). The guard's point was not that doc; it was that real work
  had happened since the last handoff write. Answering the guard's *reason* rather than
  its *object* is the difference between dismissing it and using it.
### Added 2026-09-19 — implementing rank 33

- 🔴 **A DECISION RECORD IS A PLAN, NOT A MEASUREMENT — AND THIS ONE WAS WRONG IN
  THREE OF ITS SIX COUPLED EDITS.** Its Go snippet did not compile, its test
  prediction was inverted, and its `AGENTS.md` eviction was unnecessary. None of the
  three is a criticism of the *decision*, which stands; all three are the difference
  between writing down what a change will require and running it. **Run each coupled
  edit's own check before treating the list as a spec.**
- 🔴 **A GUARD NAMED FOR A DISTINCTION IT DOES NOT SAMPLE.**
  `TestREADMEVerdictExemptionsAreLedgeredAgainstTheCode` calls its set
  `exemptForOther` and derives it with `"cursor"` — an agent that IS in the table. So
  "other" meant *not claude*, never *not in the table*, and a third agent class was
  invisible. Its docstring says **"THIS IS A LEDGER, NOT A COUNT … adding a third
  exemption without saying so in the README is red"** — a third exemption was added
  and it stayed **green**. The variable NAME is what made the gap unreadable.
  **Ask which values a parameterised guard actually feeds, not what the parameter is
  called.**
- 🔴 **THE GUARD'S FAILURE MESSAGE WAS FALSE, AND FOLLOWING IT DESTROYS CORRECT
  WORK.** Once the README named the rows, it reported *"the README names `mcp-site`
  as excluded from `ok`, but checkCountsTowardVerdict COUNTS it"* — the code does not
  count it on that path. The two remedies that message invites are de-backticking the
  names (round 3 of #641 measured that exact wrong fix) and reverting the code. This
  file already warns *"A guard whose failure message sends a maintainer to the wrong
  file is the hazard RULES.md names"* — here the guard was the one that had not been
  hardened.
- 🔴 **A STORED CI GREEN EXPIRES WHEN THE GUARD QUERIES THE NETWORK.** `main`'s
  `pins-vs-published` was `success` at 05:05Z and the same guard failed at 12:00Z with
  no commit in between, because npm published `@civitai/app-sdk 0.44.0`. **Reading
  `main`'s stored conclusion is NOT a control for "did my branch break this".** Run
  the guard locally, or diff the pin files against `origin/main` (here: empty).
- ⚠ **`gh pr list --state open` found the fix already open (#668) before any was
  authored.** The sweep is the only mechanism that sees an *unclaimed* duplicate, and
  it cost one command.
- ⚠ **A Go binary copied into a slim container needs `CGO_ENABLED=0`.** Without it:
  `cannot execute: required file not found`, rc **127** — indistinguishable at a
  glance from "the binary is not there", and it cost a round trip.
- ⚠ **The `git add` provenance hook reports against the SESSION'S CWD, not the repo
  being committed.** Committing in a `cli` worktree from a `datapacket-talos` cwd drew
  a warning naming `claudedocs/refs/r2-b2-tiering-thrash.md`, a file in the *other*
  repo that the commit does not contain (`git show --name-only | grep -c r2-b2` → 0).
  Verify against the commit before acting on it.
- ⚠ **An "item N" reference in prose is CHECKED, ACROSS THE WHOLE REPO — and writing
  the gotcha down is not the same as obeying it.** `TestAgentsItemCrossReferencesResolve`
  fails on a prose phrase naming an item number above `AGENTS.md`'s highest (currently
  1..38), in ANY file, not just `AGENTS.md`. It fired twice in one session: first on a
  `claudedocs/refs/` file, then **on this very bullet**, whose original wording quoted
  the offending phrase verbatim as the example. A guard that reads prose cannot tell a
  citation from a claim. **Say "a new numbered item", and do not quote the number even
  when explaining the rule.**

### Added 2026-09-19 — shipping the arc, and six instrument misreadings

- 🔴 **A CLOSING KEYWORD INSIDE A NEGATION STILL CLOSES THE ISSUE.** The PR body read
  *"Deliberately **not** `Closes #665`"*; GitHub matched the substring, put it in the
  squash commit, and auto-closed an issue whose condition was explicitly unmet. Same
  shape as this repo's item-N xref guard, which fired on the very bullet documenting it.
  **A parser reads words, not meaning — do not quote the form you are refusing.**
- 🔴 ⚠ **THE COUNT IN THIS BULLET IS WRONG — IT WAS EIGHT, NOT SIX. See the close-out
  correction at the end of this section; two more happened AFTER this was written, and
  the eighth was a false SECURITY finding.** The six below are accurate as instances;
  only the total is stale — which is precisely the rot this bullet is about.
- 🔴 **SIX INSTRUMENT MISREADINGS IN ONE SESSION, ALL THE SAME SHAPE: the tool answered
  confidently about the WRONG OBJECT.** (1) `gh pr checks` served the pre-push rollup
  after a force-push. (2) `npm view` reported the OLD version minutes after a successful
  publish — the registry's own JSON had the new one. (3) A Tekton poll read "newest
  existing PipelineRun" and graded a PRE-rotation failure as the new run, which I
  reported as "failed again". (4) The same poll matched reason `Succeeded` when Tekton
  reports **`Completed`**, so it timed out on a build that had already passed.
  (5) `python3 tool.py $FILES` in zsh passed 66 paths as ONE argument (`${=FILES}`).
  (6) `handoff_doc.py` was run with `--confirm` twice and its GUIDANCE text was read as
  success; the actual `status=` line said `behind`, then `failed`. **Read the field that
  carries the verdict, not the nearest reassuring text.**
- 🔴 **THE `pins-vs-published` FREEZE RECURRED TWICE MORE IN ONE SESSION** (`#668`
  `app-sdk ^0.43→^0.44`, then `#672` `^0.44→^0.45` plus `blocks-react ^0.52→^0.53`).
  Upstream publishes fast enough that any PR sitting a few hours hits it. The diagnostic
  is always the same two controls — pin files identical to `main`, and the guard failing
  locally against live npm — and the fix is the nightly, which can be TRIGGERED on demand
  (`gh workflow run bump-scaffold-pins.yml`) rather than waited for.
- 🔴 **PINS SHIP IN THE RELEASE.** A stale scaffold at tag time means every app created
  from that version is born against a package set that does not resolve. Bump BEFORE the
  tag, not after.
- ⚠ **A DRAFT RELEASE IS THE GATE BETWEEN REVERSIBLE AND IRREVERSIBLE.** goreleaser sets
  `draft: true`; publishing the draft is what fires npm AND Homebrew. Verify the ARTIFACT
  — download it, checksum it against the published `checksums.txt`, run it — before
  publishing, because the draft is deletable and the publish is not.
- ⚠ **RESTORING A MUTATION WITH `git checkout --` REVERTED UNCOMMITTED WORK.** A
  mutation-test restore silently discarded the real change in that file; the end-to-end
  suite caught it as a template error. **Restore from a `cp` backup, never from git,
  when the file carries uncommitted work.**

### Added 2026-09-19 (close-out) — the count was wrong, and the worst misread came last

- 🔴 **CORRECTION: the bullet above says SIX instrument misreadings. It was EIGHT, and
  the count was written while two more were still ahead.** This is the exact rot that
  bullet is about — a number asserted in prose, stale before the session ended. The two
  it missed:
  - **(7) `handoff_doc.py` refusals read as success.** `--confirm --push` was run twice
    and its GUIDANCE text was tailed as if it were a verdict; the real `status=` line
    said `behind` (the primary clone was behind a trunk I had myself moved), then
    `failed` (detached HEAD, no `--branch`). Nothing was written either time. **Read the
    field that carries the verdict, not the nearest reassuring text.**
  - **(8) 🔴 `git check-ignore -q <DIRECTORY>` PRODUCED A FALSE SECURITY FINDING.**
    `.gitignore` carries `.secrets/*` — the pattern matches the CONTENTS, not the
    directory entry — so `check-ignore .secrets` correctly reports "not matched", and
    that was read as "the private keys are not ignored". It was reported to the operator
    TWICE and committed into a handoff before being checked. Measured afterwards:
    `check-ignore -v` resolves every key file to `.gitignore:9`, and `git add .secrets`
    exits `fatal: pathspec … did not match any files`. **A gitignore question is about
    FILES — test a path INSIDE the directory, and use `-v` (which names the matching
    rule) over `-q` (a status you then interpret).**
- 🔴 **AN ASSERTED VULNERABILITY THAT DOES NOT EXIST COSTS MORE THAN A MISSED ONE.** It
  spends the operator's attention and devalues every other finding in the same document.
  Retracted in place at `<talos-infra>` `bec009e81`, struck through rather than deleted so
  anyone who read the earlier version sees it was withdrawn.
- ⚠ **All eight have ONE shape: the instrument answered confidently about the WRONG
  OBJECT** — a stale rollup, a cached registry read, a pre-rotation PipelineRun, a
  reason string that was `Completed` not `Succeeded`, an unsplit zsh variable, guidance
  text, and a directory-vs-contents pattern. Seven cost time. The eighth cost
  credibility. **The cure is the same every time: name the object you are asking about,
  and read the field that answers for THAT object.**

## How to verify

**Rank 33's closing condition, verbatim — a fresh machine with no agent env var:**

```bash
docker run -d --name v33 node:22-bookworm-slim sleep 300
CGO_ENABLED=0 go -C <cli-worktree> build -o /tmp/civitai-static ./cmd/civitai
docker cp /tmp/civitai-static v33:/usr/local/bin/civitai
docker exec v33 bash -lc 'mkdir -p /work && cd /work && civitai agent-setup --track app >/dev/null 2>&1; cd /work && civitai agent-setup --check --json; echo "rc=$?"'
docker rm -f v33
```
Expect `"agent": "other"`, `"ok": true`, **rc 0**, and both `mcp-site` / `mcp-orch`
rows PRESENT with `"ok": false`. 🔴 **Build with `CGO_ENABLED=0`** — a cgo binary in
that image fails with `cannot execute: required file not found`, rc 127, which reads
like a missing binary rather than a link error.

**🔴 VALIDATE THE GRADER BEFORE READING ANY VERDICT FROM IT — three controls, $0 of API
spend, and assert the FIELDS, not the verdict word.** The full commands are in
`scripts/dogfood/README.md`; the values they must produce on v0.1.106 are in the rank-36
investigation block above. `ctl-profile` is the one that matters: arm A green while arm B
is red. A `CLOSING_CONDITION=no` alone is also what a grader broken on arm A produces.

**The guard matrix (what makes the change more than a green suite):**

```bash
# RED at base: revert ONLY the isMCPCheckName branch, keep the test, then
go -C <cli-worktree> test ./internal/cmd -run TestMCPRowsDoNotFailTheVerdictForAnAgentWithNoConfigTarget -count=1
# blanket mutant: make that branch `return false` unconditionally — the
# "a KNOWN agent with no MCP config still fails" arm must go red, on its own message
```

**The tier CI actually runs — reads COMMITTED state, so commit before running it:**

```bash
cd <cli-worktree> && make ci-shallow      # expect: 21/21, 0 failures, 0 timeouts
```
🔴 `make ci` is **not** what CI runs — it is a full clone. `make ci-shallow` is the
depth-1 tier, and it grades committed state only.

**The freeze, if `pins-vs-published` is red:**

```bash
CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold -run TestScaffoldPinsSatisfyPublished -count=1
gh pr list --repo civitai/cli --state open   # a bump PR probably already exists — do not author a second
```
