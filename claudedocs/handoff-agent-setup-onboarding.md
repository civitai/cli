# Handoff: agent-setup-onboarding — 2026-09-08

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

## State now

**Rank 21 is DONE and merged — the #554/#564 collision is resolved.** Rank 20 (cli#566)
is CLAIMED and not started.

- **Branch `main`, clean, synced.** `civitai/cli` at `e3f2608`;
  `civitai/civitai-developer-docs` at `d5b34c8`.
- **Merged this pass:** **cli#569** (`e3f2608`) — rank 21. Builds on @xsvm's #554
  (their commit first, unmodified), rebased onto main after #564, plus the TAB column
  vector and the #564 ledger reconciliation. CI on `main` at `e3f2608`: **success**,
  both `CI` and `CodeQL`.
- **cli#554 is deliberately still OPEN.** It is superseded in content, not in credit;
  closing it is xsvm's call and they were told so on the PR. `Co-Authored-By: xsvm` is
  on the squash.
- **Issue #552 stays OPEN on purpose** and holds the remaining widened-ledger work:
  ~13 other tabwriter renderers share the same shape with no ledger detecting it, the
  README still does not describe the newline/tab-to-space change across the inline
  fields, and soft-wrap can still place attacker text at column zero with no control
  character involved.
- **Claims:** `cli-554-564-collision-resolution` released (by the parallel session);
  **`agent-setup-onboarding-20` HELD** — rank 20, not started.
- 🔴 **No clawgate task recorded.** `resolve` exited **5**. Its positive control answered
  **8** links for a different session, so the board is reachable and the token accepted —
  but a wrong id also answers 200/empty, so this is a real reading, **not** a clean bill
  of health.

### Carried forward — still true, keeps being dropped under this REPLACE heading

- **Rank 4 LIVE and verified**: `/.well-known/ai-catalog.json` → 200,
  `application/ai-catalog+json; charset=utf-8`; `check-ai-catalog.mjs` against production
  → 7 URLs, 0 failed, 0 skipped. Shipped NARROWED (catalog + `Link:` only) by operator
  decision.
- **Rank 6 verified against PROD**: `/apps/examples` → 200, `/apps/examples/` → 404 with
  the CLI shipping the no-slash spelling; `llms.txt` 215 → **192** lines.
- **The `Python-urllib` 403 is GONE** — root-caused to Cloudflare Browser Integrity
  Check, fixed by a Configuration Rule scoped to `developer.civitai.com`. The two
  `⚠ STILL OPEN` headings above its RESOLVED block are superseded; do not act on them.
- **`bump-scaffold-pins` is fixed** (cli#540, node 22 → 24), exercised deliberately by
  run 34559798234. A green *scheduled* run could not have proved it.
- **Subsystem index**: `cli/civitai`'s stale `OPEN:` bullet reads `RESOLVED d0d1805`.

### Honest limits

- **The notifier's cross-run paths are still fake-only**: update-in-place,
  comment-on-fingerprint-change, comment-then-close. The **create** path is real.
- **#564's ledger still has the structural blind spot** — rank 20. It is why #399 could
  close honestly while the same defect shape survives on the download path.
- **The `1010` Cloudflare UA block was never re-provoked**; the 403/301 arms of the CLI
  link probe remain locally simulated.
- **cli#569 was merged by a parallel session, not by the session that verified it** —
  see the gotcha below. The verification stands (`git diff e2a7dd6 origin/main` empty);
  the attribution in any earlier draft does not.

## Open investigations — live diagnosis state

### The repo is frozen: `pins-vs-published` is red on `main` and the bump automation cannot land the fix

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

### `developer.civitai.com` returns 403 to `Python-urllib` — IN FLIGHT, delegated

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

### 🔴 STILL OPEN — `bump-scaffold-pins` cannot land its own fix (cli#530)

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

### ⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge

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

### ⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge

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

### The docs repo cannot be built locally from a pristine `main`, while CI builds it green

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

## Next steps (ranked)

🔴 **Ranks 1–14, 18 and 21 are DONE — numbering is preserved deliberately** so any live
`claim-work` slug keeps pointing at the item it was taken for. New items continue from 22.

1. ~~**Unfreeze `civitai/cli`**~~ — **DONE**, cli#529.
   forcing: none
2. ~~**Land the two draft agent-setup PRs**~~ — **DONE** (cli#528, docs#67).
   forcing: none
3. ~~**PR `civitai/cli#526`**~~ — **DONE**, merged `517fc76`.
   forcing: none
4. ~~**P2 of the onboarding work**~~ — **DONE**, docs#73 (`c84684a`), live and verified.
   forcing: none
5. ~~**Delete or gitignore `node_modules/`**~~ — **DONE**, cli#537.
   forcing: none
6. ~~**Example apps**~~ — **DONE**: docs#71 + cli#549, audit-fixed by docs#72 + cli#553.
   forcing: none
7. ~~**cli#530**~~ — **DONE**, cli#540, verified by run 34559798234.
   forcing: none
8. ~~**A hard byte ceiling on `prompt.md`**~~ — **DONE**, docs#74 (`11311ab`).
   forcing: none
9. ~~**cli#542**~~ — **DONE**, cli#557 (`6fce127`), issue CLOSED.
   forcing: none
10. ~~**cli#543** — the read `--help` rune budget~~ — **DONE**, cli#559 (`a33daed`).
    The binding constraint MOVED rather than disappearing — see rank 16.
    forcing: none
11. **Residuals of decisions item 38 — 2 of 3 DONE** (cli#560, `44a5b94`). **STILL OPEN:
    the README documents no 429 → exit 2 reclassification.** Left deliberately — both
    README exit-code blocks are **generated** from `exitCodeDocs` in
    `internal/cmd/exitcodes_doc.go`, so writing the row means editing the generator and
    republishing the exit-code contract. Closing condition in
    `claudedocs/decisions/38-…md`.
    forcing: none
12. ~~**Nothing notifies on a red daily example-app drift run**~~ — **DONE**, docs#75.
    forcing: none
13. ~~**Per-repo SCOPE/HOOK rot guard**~~ — **DONE for scopes**, docs#75. Hooks are
    date-stamped, not machine-verified, deliberately.
    forcing: none
14. ~~**cli#399 — `safeTerm` unpinned at most call sites**~~ — **DONE**, cli#564
    (`4dbcda3`), issue CLOSED with the condition demonstrated.
    🔴 **Does NOT cover the download path — see rank 20.**
    forcing: none
15. **The docs repo does not build from a pristine `main` locally.** Blocks any local
    `npm run build` / `check:built-site`; CI unaffected. Repo:
    `/home/zach/workspace/civit/civitai-developer-docs`. Closing condition: the local
    build exits 0 on a clean checkout, or the divergence is explained in that repo's
    `CLAUDE.md`.
    forcing: gate — `check:built-site` and `build-site` cannot be run locally before
    pushing, so that job's failures are only ever discovered in CI.
16. **`images search --help` sits 14 runes under the 1400 budget.** PRE-EXISTING (1386
    before and after #541), untouched by rank 10's fix because it does not interpolate
    `readJSONNote`. Files: `internal/cmd/images.go`. Levers: `serverOwnedEnumNote`,
    `deepPagingNote`, or the body.
    forcing: none
17. **Most of this arc's PRs were not adversarially audited.** The ones that WERE
    (rank 6's two, #564's, and #569's) each found real defects in PRs that were fully
    green and mutation-tested. Unaudited with the most judgement in them: docs#73,
    docs#74, cli#559.
    forcing: none
18. ~~**`drift-notify.mjs` has never talked to the REAL GitHub API**~~ — **DONE**,
    verified live by `gh workflow run appblocks-drift.yml`: ALERT path, docs#76 opened,
    label applied. **Residual**: the cross-run paths are still fake-only.
    forcing: none
19. **`#542`'s stated closing condition may not be discharged.** It named "any
    `saferune.*` call site outside `internal/cmd`". There are **two** —
    `pkg/civitai/read.go` and `internal/genapi/status.go` — and cli#557's guard matches
    `snippet`, not `saferune.*`. Read with rank 20.
    forcing: none
20. 🔴 **cli#566 — three uploader-controlled surfaces on the download path reach the
    terminal raw, and #564's ledger STRUCTURALLY CANNOT demand a row for them.**
    **CLAIMED `agent-setup-onboarding-20`, not started.** `GREW` only fires for functions
    that **already call `safeTerm`**; these call it zero times. Surfaces, all in
    `internal/cmd/download.go`: `(*progressWriter).line()` (`:1043,1045` — raw
    `files[].name` inside a `\r`-rewritten progress line, every download);
    `checkTargetCollisions` (`:702` — the **literal twin** of the sanitized `:658`, same
    fields, same shape, 44 lines apart); and `fmt.Errorf` at `:787,:810` reaching
    unfiltered stderr via `cmd/civitai/main.go:57`. First two **reproduced**; the third
    **read, not driven**. Verified pre-existing:
    `git diff --stat cf5e4a8 8063caa -- internal/cmd/download.go` empty, with
    `git cat-file -e cf5e4a8:internal/cmd/download.go` proving the path existed.
    Closing condition is in the issue.
    forcing: security — uploader-controlled text reaches a terminal through a `\r`-rewrite
    and through a message asserting an integrity result; the same defect shape #564's
    headline fix closed one function away.
21. ~~**cli#554 was broken by merging #564**~~ — **DONE**, cli#569 (`e3f2608`), merged
    with a squash body that deliberately drops the inherited closing keyword. #554 left
    OPEN for xsvm to close; issue #552 left OPEN and holding the residuals.
    forcing: none
22. **The widened-ledger work on issue #552 — a ledger keyed on RENDERING a
    server-supplied struct field, not on the presence of a `safeTerm` call.** This is the
    instrument rank 20's issue deliberately excludes from its own closing condition, and
    it is what would cover the ~13 other tabwriter renderers in one move rather than
    one function at a time. Read rank 19 and rank 20 first — all three are the same seam.
    forcing: security — the same uploader-controlled-text class as rank 20, at the ~13
    renderers no current guard can see.

## Gotchas / decisions / dead-ends

- 🔴 **A hosted instruction file is a lossy channel, and this is measured, not
  theoretical.** Claude Code's WebFetch is *"lossy by design"* (per
  code.claude.com/docs/en/tools-reference): HTML→Markdown, truncate at 100 KB,
  then summarize with a small model — Claude receives the summary. Reproduced
  during this session's research: the first fetch of Cloudflare's own
  `prompt.md` came back with **invented headings** and both the OpenCode and
  Windsurf config blocks **missing**; `curl` returned the true 4,900 bytes.
  There is a public case (`oh-my-openagent#1401`) where the summarizer dropped
  one of four install flags and the install reported success. **This is the
  entire reason the logic lives in the Go binary and `prompt.md` is ~60 lines.**
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

## How to verify

Rank 21, against the merged tree — the controls that matter, each re-derivable:

```bash
cd /home/zach/workspace/civit/cli
go test ./internal/cmd -count=1                 # expect: ok, ~35s
# the two tab guards, by name
go test ./internal/cmd -count=1 -v \
  -run 'TestSafeTermSingleNeutralisesTheTabColumnVector|TestTabForgeryIsNeutralisedInTheRealImagesTable'
# the composer ledger, both directions
go test ./internal/cmd -count=1 -run 'TestSanitizerComposersAreLedgered'
```

🔴 **The mutation controls are the real verification and they are NOT committed.** Run
them in a detached worktree, restoring from a `cp` of the file rather than
`git checkout --`:

| mutant | expected |
|---|---|
| `safeTermSingle` back to `\n`-only (drop `\t` from the `ContainsAny` guard AND the `NewReplacer`) | compiles; reddens **exactly** the two tab guards, each with its own message; the behavioural one reports the forged row at **14 columns against an 8-column header** |
| add `"s": …` to `bareIdentArgs`, then append `func zzProbeRenderer(u string) string { s := u; return safeTerm(s) }` to `images.go` | **PASSES** — the injection survives (this is #554's shape) |
| same probe, `"s"` absent | **FAILS** naming `images.go:<line>: safeTerm(s)`; scanner reports **154** = 153 real calls + the probe |
| append an unledgered func calling `safeTerm` to `safeterm.go` | `UNLEDGERED COMPOSER` |
| add a `sanitizerComposers` entry for a function that does not exist | `STALE COMPOSER` |

Ranks 9–11 and the repo gates:

```bash
cd /home/zach/workspace/civit/cli
make ci                                         # 21 packages report ok
nix-shell -p golangci-lint --run "make lint"    # make ci does NOT run lint
```

Rank 4, against production:

```bash
cd /home/zach/workspace/civit/civitai-developer-docs
node scripts/check-ai-catalog.mjs --verbose     # expect: 7 URLs, 0 failed, 0 skipped
```

🔴 Read `rc` directly — `$?` after a pipe is the pipe's status. This repo has been
misled by that four times now; the newest instance is in the gotchas below.
