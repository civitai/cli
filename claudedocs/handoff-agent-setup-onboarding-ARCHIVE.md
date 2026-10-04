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

## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-04

Closed investigation bodies from the agent-setup-onboarding arc, evicted when rank 34 shipped end-to-end (cli#777 -> v0.1.112 -> docs#126). Each heading and as-of stamp stays in the handoff as a pointer. Read a block here before re-deriving a diagnosis in the same area: the dogfood matrix readings, the rank-33 verdict decision and its three plan corrections, and the pins-vs-published recurrences all live in this file now.

From `Open investigations — live diagnosis state`:

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
  D=/home/zach/workspace/civit/cli-dogfood36/scripts/dogfood
  cd "$D" && DOGFOOD_MODELS='x-ai/grok-4.6|grok' DOGFOOD_ENVS='df-node-root|noderoot|root df-stale-cli|stale|root' \
    DOGFOOD_IDENTITIES='other|' bash driver.sh
  🔴 Then the `gemini × stale × other` cell separately — and note the driver's
  non-crossed envs run the FIRST identity in the list, which is why `DOGFOOD_IDENTITIES`
  must name `other` first or the stale cell silently runs a different identity.

## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-04

Evicted because they are CLOSED, not because they are worthless — keep reading them for the raw values, which are still the baseline later blocks compare against. Two cautions. The two cli#665 blocks were each wrong about the SHAPE of the gap (one called it release sequencing, one an environment problem) while their measurements were sound, so adopt their numbers and not their framings. And the rank-36 cost findings stand on their own even where their status lines are superseded.

From `Open investigations — live diagnosis state`:

🔴 **NOT OPEN, AND ITS CLOSING BULLET IS NOW WRONG.** The block below ends *"Next
probe: none … what is missing is a DECISION"* — the decision was made **and
implemented**; `checkCountsTowardVerdict` now excludes the `mcp-*` rows when
`agentTargets[agent]` is unknown. Read the IMPLEMENTED block at the bottom of this
section for what actually shipped and for the **three corrections the decision
record's plan needed**. Measurements below are kept — they are still the baseline.
  a round-0 audit refuted its original framing — read the RETRACTED bullet below
  before acting on any part of this block**
For `agent == other` — every agent with no entry in the CLI's table — `--check`
returns `ok: false` and exit 1 after a **completely correct** setup, forever.
  docker run -d --name x node:22-bookworm-slim sleep infinity
  docker exec -w /work x bash -lc \
    'mkdir -p /work && npm install -g @civitai/cli && civitai agent-setup --track app
     civitai agent-setup --check --json; echo "rc=$?"'
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
🔴 **ITS "THREE REMAINING STEPS" AND ITS `Next probe` ARE BOTH WRONG NOW AND HAVE BEEN
DELETED rather than preserved** — steps (1) and (2) landed (#777, v0.1.112), step (3) was
RETIRED by an operator decision the same day, and the probe has been RUN. Why it was
wrong is the transferable part: it modelled the gap as **release sequencing**, so every
step it listed was a shipping step. The measured gap is **delivery** — the hosted
`prompt.md` never names the flag and forbids the action — which no amount of shipping
reaches. The measurements below are kept; they are still the baseline.
- **Symptom + exact repro:** #665's closing condition arm 1 is graded mechanically by
  `scripts/dogfood/grade.sh` from a blind container trial — `CLOSING_CONDITION=yes` requires
  `zsh -lic 'civitai --version'` to print the version in the user's login shell. Nothing in #777
  exercises that path: `--fix-path` appears in nine files, all implementation, tests, README, help
  text or ledger, and `grep -c 'fix-path' scripts/dogfood/grade.sh` returns **0**.
- **Observed (with values):** the three remaining steps, in order, none started — (1) merge #777;
  (2) cut a CLI release carrying the flag (npm + Homebrew), because the hosted prompt must not name
  a flag the published CLI lacks; (3) a PR to `civitai/civitai-developer-docs` editing
  `agent-setup/prompt.md` §2's EACCES branch to run `civitai agent-setup --fix-path` and verify in a
  NEW login shell. Only then can the matrix be re-run and arm 1 graded.
- **Ruled out:** that #777 alone could close it — the author said so and the audit measured it.
  `via: measurement` · That a `--prefix="$HOME/.local"` remedy would do instead — all 6 failing
  containers (complete enumeration) installed to `~/.npm-global`, which **neither** login shell
  finds. `via: measurement` · That a `GOOS` gate was the right Windows answer — Git Bash/MSYS is
  `GOOS=windows` running a POSIX bash that genuinely reads `~/.bash_profile`. `via: code`
- **Leading hypothesis:** not a defect — ordinary release sequencing. The risk is that the arc is
  read as finished at merge, when the measurable benefit begins two steps later.
- **Next probe:** after the release ships, `npm view @civitai/cli version` to confirm the flag is
  published, then the docs PR, then `bash scripts/dogfood/driver.sh` against `df-node-user` or
  `df-ubuntu-apt` and read `grade.sh`'s `CLOSING_CONDITION=`.
🔴 **ITS `Leading hypothesis` WAS WRONG AND IS THE REASON TO KEEP THIS BLOCK.** It read
*"the remedy is sound and the grading is purely an environment problem"* — the first
half is now MEASURED TRUE and the second is FALSE. The environment was never the
obstacle: 6 of 6 blind container cells graded `no`, and the obstacle is that the hosted
`prompt.md` never names `--fix-path` and tells the agent *"do not edit their shell
profile yourself"*. Reading "purely an environment problem" as "a container will flip
it" is exactly the inference the grid refutes. Its `Next probe` is kept — it was right,
and running it is what produced the finding.
- **Symptom + exact repro:** #665's closing condition wants a BLIND machine — no prior `civitai` —
  where `civitai agent-setup --check` reports `ok: true` AND `zsh -lic 'civitai --version'` prints
  the same version. The issue is CLOSED (auto-closed by #777's merge) while that is unmeasured.
- **Observed (with values):** `gh issue view 665 --json state,closedAt` → `CLOSED COMPLETED
  2026-10-04T01:56:10Z`; `#777` merged `01:56:08Z`. On THIS host `command -v civitai` →
  `/home/zach/.local/bin/civitai`, plus `/home/zach/go/bin/civitai` (`v0.1.80`) and the npm copy —
  three on PATH, so "blind" is unsatisfiable. With `ZDOTDIR` unset and the competing copies removed
  from PATH, both probes DO pass against the published binary: `zsh -lic` and `bash -lc` each print
  `civitai 0.1.112` resolving `…/@civitai/cli/lib/binaries/civitai`; with the block deleted, both
  report not-found.
- **Ruled out:** that `--fix-path` is inert on the npm `--prefix` path #665 is about — the postinstall
  writes `lib/binaries/civitai`, the naming guard passes, and `--fix-path --dry-run` through the Node
  wrapper reports it WOULD write both files. `via: measurement`
- **Ruled out:** that a NixOS zsh login shell clobbers `~/.zshenv`'s PATH prepend — a synthetic
  marker survived `zsh -lic` with `ZDOTDIR` unset. The earlier "NixOS clobber" theory is RETRACTED.
- **Leading hypothesis:** the remedy is sound and the grading is purely an environment problem.
- **Next probe:** re-run the dogfood matrix in a container. 🔴 Copy `runs/` aside first — `runner.py`
  opens each transcript `"w"` and DESTROYS existing ones, and `grade.sh` refuses a stopped container.

## Evicted from `claudedocs/handoff-agent-setup-onboarding.md` — 2026-10-04

Closed blocks, evicted for size and kept for their VALUES. Two cautions: the cli#665 blocks were each wrong about the SHAPE of the gap while their measurements were sound — adopt their numbers and not their framings; and anything here quoting "0 of 6 CLOSING_CONDITION=no" is a statement about ARM 1 of a two-arm condition, which no block below says because nobody had read the condition yet.

From `Open investigations — live diagnosis state`:

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
  for t in "$D"/runs/t-{glm,mimo,dsv4}-*/; do
    id=$(basename "$t"); u=root
    case "$id" in *-nodeuser-*|*-ubuntu-*) u=dev ;; esac
    printf '%-34s ' "$id"; bash "$D/grade.sh" "$id" "$u" 2>&1 | tail -1
  done
