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

**This session closed ranks 4, 10 and two thirds of 11, and shipped 8 and 9 —
the entries below that credit 8 and 9 to "a parallel session" are describing
THIS one.** All five PRs are merged; nothing is in flight.

- **Branch `main`, clean, synced.** `civitai/cli` at `3d17591`;
  `civitai/civitai-developer-docs` at `11311ab`.
- **Merged this session:**

| PR | rank | |
|---|---|---|
| docs#73 (`c84684a`) | 4 | `.well-known/ai-catalog.json` + the `Link:` advertisement, and its guard |
| docs#74 (`11311ab`) | 8 | a byte budget on `prompt.md`, with a ceiling and a floor |
| cli#557 (`6fce127`) | 9 | `snippet()`'s "every argument is server bytes" ledger — closed #542 |
| cli#559 (`a33daed`) | 10 | read `--help` headroom, and a residual note that named the wrong command — closed #543 |
| cli#560 (`44a5b94`) | 11 | four rotted test citations + half (c)'s controls — 2 of 3 residuals |

- **Rank 4 is LIVE and verified against the origin**, not just merged:
  `https://developer.civitai.com/.well-known/ai-catalog.json` → 200,
  `application/ai-catalog+json; charset=utf-8`, and `/` carries the `Link:`
  header. `node scripts/check-ai-catalog.mjs` against production: **7 URLs
  checked, 0 failed, 0 skipped**, including "the live `Link` equals the config's".
- 🔴 **Rank 4 shipped NARROWED, by an explicit operator decision.** The original
  wording named `ai-catalog.json` **and** `agent-skills/index.json` with SHA-256
  digests. Only the catalog shipped. Measured reasons, both recorded in the docs
  repo's `CLAUDE.md` and `README.md`: an agent-skills index's every entry is a
  `tar.gz`/`SKILL.md` with a `sha256:` digest and this project publishes no skill
  bundles, so the index would have had zero or invented entries; and
  `ai-catalog.json` carries **no digests at all** — digests are an agent-skills
  field, so that half of the original wording described the wrong artifact.
- **Claims released:** `agent-setup-onboarding-4`, `-8`, `-9`, `-10`, `-11`.
  None held.
- 🔴 **No clawgate task recorded.** `clawgate_handoff.sh resolve` exited **5**
  (0 tasks for this session). Its positive control resolved **9** links for
  another session, so the board is reachable and the token is accepted — but a
  wrong session id also answers 200 with an empty array, so this is **not**
  evidence that no task exists.
- **Not done:** none of the five PRs received an adversarial audit. Each was
  offered and the operator chose to merge. See rank 17.
- **Recorded in the subsystem index this session** — `cli/civitai`: the stale
  `OPEN:` bullet about #513's server-side coercion rewritten as
  `RESOLVED d0d1805` (it is now IDENTIFIED and LIVE — Meilisearch dynamic JSON
  typing — and the earlier "not reproducible" was defeated by Cloudflare cache
  HITs, 38 of 38 flipping on a fresh cache key), plus a new bullet on the
  `snippet()` ledger. 🔴 **A new `ai-catalog` entry could NOT be written** — see
  the gotcha on the stranded store scopes.

**Carried forward from earlier sessions** (still true; keeps being dropped
because it sits under a REPLACE heading):

- **Rank 6 verified against PROD, not inferred** (2026-09-11, after the 16:24:59Z
  deploy): `/apps/examples` → **200** and `/apps/examples/` → **404**, with the
  CLI shipping the no-slash spelling; `llms.txt` → **192 lines** (was 215) with
  `### Examples` ×1, so the duplication is gone; the live page names all 8 repos.
- **Issue #530 is CLOSED and was verified by a real dispatched run**
  (run 34559798234), not by reasoning. The `cli/scaffold` index entry carries the
  2026-09-08 `OPEN:` bullet rewritten as `RESOLVED 5557b6a`, plus the
  sibling-workflow drift class.

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

🔴 **Ranks 1–10 are DONE — numbering is preserved deliberately** so any live
`claim-work` slug keeps pointing at the item it was taken for.

1. ~~**Unfreeze `civitai/cli`**~~ — **DONE**, cli#529.
   forcing: none
2. ~~**Land the two draft agent-setup PRs**~~ — **DONE** (cli#528, docs#67).
   forcing: none
3. ~~**PR `civitai/cli#526`**~~ — **DONE**, merged `517fc76`.
   forcing: none
4. ~~**P2 of the onboarding work**~~ — **DONE**, docs#73 (`c84684a`), live and
   verified against the origin. Shipped NARROWED: catalog + `Link:` only, no
   agent-skills index — see State now for the measured reasons.
   forcing: none
5. ~~**Delete or gitignore `node_modules/`**~~ — **DONE**, cli#537.
   forcing: none
6. ~~**Example apps**~~ — **DONE**: docs#71 + cli#549, audit-fixed by docs#72 +
   cli#553.
   forcing: none
7. ~~**cli#530**~~ — **DONE**, cli#540, verified by run 34559798234.
   forcing: none
8. ~~**A hard byte ceiling on `prompt.md`**~~ — **DONE**, docs#74 (`11311ab`).
   The earlier entry credited this to "a parallel session"; it was this arc.
   Claim `agent-setup-onboarding-8` is RELEASED.
   forcing: none
9. ~~**cli#542**~~ — **DONE**, cli#557 (`6fce127`), issue CLOSED.
   🔴 **cli#399 was NOT closed with it — see rank 14.**
   forcing: none
10. ~~**cli#543** — the read `--help` rune budget~~ — **DONE**, cli#559
    (`a33daed`), issue CLOSED. `readJSONNote` trimmed 384 → 331 runes; `users`
    1395 → 1342. The binding constraint MOVED rather than disappearing:
    `images search` is longest again at 1386 (14 of headroom) — see rank 16.
    forcing: none
11. **Residuals of decisions item 38 — 2 of 3 DONE** (cli#560, `44a5b94`).
    Closed: the four comment-citation rot sites (repo-wide unresolved-name count
    **20 → 16**, sites 23 → 19, measured on both sides with the shipped
    instruments); and half (c)'s body-vs-message control confusion.
    **STILL OPEN: the README documents no 429 → exit 2 reclassification.**
    Left deliberately — BOTH README exit-code blocks are **generated** from
    `exitCodeDocs` in `internal/cmd/exitcodes_doc.go` (pinned by
    `TestREADMEExitCodeTableIsGenerated` / `…SectionsAreGenerated`), so writing
    the row means editing the generator and republishing the exit-code contract.
    Closing condition is stated in `claudedocs/decisions/38-…md`: the
    Troubleshooting row and the exit-code table both name the deep-paging case,
    and `readme_troubleshooting_test.go` still passes.
    forcing: none
12. **Nothing notifies on a red daily example-app drift run.**
    `civitai/civitai-developer-docs:.github/workflows/appblocks-drift.yml` has no
    notify/issue step. A red run and a fully-skipped "nothing was verified" run
    are indistinguishable unless someone opens the Actions tab. Adding a notify
    step is a workflow edit, which `AGENTS.md` puts behind "ask first".
    Closing condition: either a notify step merges, or the operator states in
    writing that Actions-tab-only visibility is accepted. **Untouched this
    session.**
    forcing: user — flagged to the operator 2026-09-11 and explicitly left as
    their call.
13. **The example page's per-repo SCOPE and HOOK lists have no rot guard.**
    `civitai/civitai-developer-docs:apps/examples.md` — `check-example-apps.mjs`
    guards only URL liveness. Either date-stamp those lists as-of, or extend the
    guard to fetch each `block.manifest.json` and compare. **Untouched this
    session.**
    forcing: none
14. 🔴 **cli#399 — REOPENED after this session closed it by accident.** The
    substance is untouched and unchanged: `safeTerm` is a security control called
    from ~150 sites, and deleting it at **20 of 25 sampled sites leaves the suite
    green**. cli#557 pinned a *ledger* in `pkg/civitai`; this is a *coverage*
    problem in `internal/cmd` and needs a different instrument (#399's own body
    proposes hanging a coverage ledger off the AST walk that
    `TestSafeTermIsNeverAppliedToUserTypedInput` already performs).
    Closing condition: deleting `safeTerm(...)` at a sampled site reddens the
    suite, demonstrated on ≥1 site that survives today.
    forcing: security — `safeTerm` is the CLI's only gate on server-supplied text
    reaching a terminal, and 20 of 25 sampled call sites are provably unpinned.
15. **The docs repo does not build from a pristine `main` locally.** See the
    Open investigations block for the full diagnosis state, including two
    retracted hypotheses. Blocks any local `npm run build` /
    `npm run check:built-site` in that repo; CI is unaffected.
    Closing condition: the local build exits 0 on a clean checkout, or the
    divergence from CI is explained in writing in that repo's `CLAUDE.md`.
    forcing: gate — `check:built-site` and `build-site` cannot be run locally
    before pushing, so that job's failures are only ever discovered in CI.
16. **`images search --help` sits 14 runes under the 1400 budget.** PRE-EXISTING
    (1386 both before and after #541) and untouched by rank 10's fix, because it
    does NOT interpolate `readJSONNote` — `images`, the parent, does. By this
    repo's own standard that is effectively frozen. Levers are
    `serverOwnedEnumNote`, `deepPagingNote`, or the body itself; the mechanism is
    now recorded in `read_help_test.go`'s residual note.
    forcing: none
17. **None of this arc's five PRs was adversarially audited.** Each audit was
    offered and the operator chose to merge. This is recorded as a known risk
    rather than a defect: rank 6's equivalent audit found a REAL hole (cli#553,
    "the docs-link gate could report ok without probing the link") in a PR that
    was also fully green and mutation-tested. The two with the most judgement in
    them are docs#74 (three derived byte constants) and cli#559 (published
    `--help` wording).
    forcing: none
18. **`.well-known/mcp/server-card.json` is not published.** The two MCP
    endpoints are deliberately absent from the catalog because a `GET` to
    `mcp.civitai.com/mcp` is **405** and `orchestration.civitai.com/mcp` is
    **401** — transports, not fetchable documents. A server card is the artifact
    that would represent them, and it would then be a catalog entry.
    forcing: none

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

## How to verify

Rank 4, against production — the half that could not exist before deploy:

```bash
cd /home/zach/workspace/civit/civitai-developer-docs
node scripts/check-ai-catalog.mjs --verbose     # expect: 7 URLs checked, 0 failed, 0 skipped
curl -sI https://developer.civitai.com/.well-known/ai-catalog.json | grep -iE 'HTTP/|content-type|^link'
```

Rank 8 and the repo's other offline guards:

```bash
cd /home/zach/workspace/civit/civitai-developer-docs
npm run check:agent-setup        # check 5 prints the byte count and the headroom
npm run check:ai-catalog -- --offline
```

Ranks 9, 10 and 11 in `civitai/cli`:

```bash
cd /home/zach/workspace/civit/cli
go test ./pkg/civitai ./internal/cmd ./internal/appapi -count=1
make ci                                         # 21 packages report ok
nix-shell -p golangci-lint --run "make lint"    # make ci does NOT run lint
```

Rank 11's closing measurement — the repo-wide unresolved-citation count, which
has no committed probe on purpose:

```bash
# scan every .go comment for Test[A-Za-z0-9_]{4,} and resolve against declared
# funcs; expect 16 distinct unresolved names, 19 sites. It was 20/23 at 44a5b94^.
```

🔴 Read `rc` directly — `$?` after a pipe is the pipe's status. This repo has
been misled by that three times now, most recently by a `git merge | tail`
that reported three failed merges as successes.
