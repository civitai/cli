## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-03

Closed investigations from the agent-setup-onboarding arc, evicted when rank 34 shipped. Each kept its measurements and its ruled-out theories, so read the relevant block before re-deriving a diagnosis in the same area: the repo-freeze/pins-vs-published recurrences and why a stored CI green expires when the guard queries npm; the developer.civitai.com 403 traced to Cloudflare Browser Integrity Check rather than the docs nginx.conf; the bump-scaffold-pins node-22/npm-10 arborist crash; and the rank-33 verdict decision with the three corrections its plan needed.

From `Open investigations — live diagnosis state`:

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
🔴 **STILL OPEN AS AN ISSUE, BUT ITS SECOND-ORDER HARM IS FIXED.** The measured
defect below is unchanged — nothing shipped installs the CLI anywhere a later
shell can reach, and cli#665's own closing condition is NOT met. What HAS shipped
(cli#671, in v0.1.106) is the `AGENTS.md` half: the block now tells a reader what
to do when `civitai` is not on PATH, which addresses the **0 of 8** finding below.
🔴 **Do NOT read the "Next probe: none" line as current** — the remedies were
re-opened, two probe-based drafts were built and DELETED, and the reasoning is in
the shipped block. Measurements below stand.
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


## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-04

The remaining CLOSED investigation blocks, evicted when `cli#777` merged. Rank 29 estimated six; there were seven. Each heading stays in the handoff as a pointer, so the append-only history still reads in order there. Eviction is a MOVE: every measurement, every ruled-out theory and every `via:` attribution below is the original text, unedited.

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

### ✅ RESOLVED — was cli#596 audited? (the previous handoff said no)

- as-of: 2026-09-14
