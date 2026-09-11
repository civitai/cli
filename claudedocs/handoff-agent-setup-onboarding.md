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

**Ranks 12 and 13 are DONE and MERGED. cli#399 was INVESTIGATED, NOT fixed — by explicit
operator decision (check-and-report).** This closes the arc: every item the operator
directed is complete, and the remaining queue is `forcing: none`.

- **Merged this pass:**

| PR | | commit |
|---|---|---|
| cli#558 | closing sweep: ranks 8/9 marked done, urllib 403 measured gone, ranks 12/13 filed | `3d17591` |
| docs#75 | ranks 12+13: deliver the sweep's red, and make the page's scope lists a checked claim | `d5b34c8` |

- **Claims RELEASED:** `agent-setup-onboarding-12`, `-13`. No agent-setup claim is held.
- **Both base clones synced**: cli `3d17591`, docs `d5b34c8`.
- 🔴 **No clawgate task recorded.** `resolve` exited **5**; an unknown session id also
  answers 200/empty, so that is not evidence no task exists.

### Verified independently, not accepted from the agent

- `check-example-apps.mjs --offline` → rc 0 on the merged tree.
- **Live scope-drift mutation** (added `models:read:public` to Sensei's page scopes) →
  **rc 1**, `✗ ZacxDev/civitai-app-sensei scopes — SCOPES DRIFTED … the page lists
  \`models:read:public\` which the manifest does not declare`. The guard names the repo,
  the scope, and the direction.
- **The two-host split is real and was demonstrated accidentally**: on a rate-limited
  run, `api.github.com` gave **3 of 8** repos verified live while
  `raw.githubusercontent.com` gave **8 of 8** scope lists verified — and the run exited
  **0 with a loud `5 … could not be reached and were NOT verified this run`** rather than
  a silent pass. That is the documented skip-loudly behaviour, observed rather than read.

### Honest limits on what shipped

- 🔴 **`scripts/drift-notify.mjs` HAS NEVER TALKED TO `api.github.com`.** All nine delivery
  scenarios were driven over real HTTP against a LOCAL FAKE issues API. **The first
  scheduled run is the first live exercise.** Two specific unknowns: whether GitHub
  auto-creates the `appblocks-drift` label under `issues: write` (there is a
  create-without-label fallback for the 403), and whether this repo's Actions policy
  permits the issue write at all.
- **Cross-run behaviour across two genuine cron ticks** is demonstrated against the fake,
  never observed live.
- **Hook lists are still unverified by machine.** The date stamp is the disclosure, not a
  substitute — see rank 14.
- **Neither `check-example-apps` nor the new self-test step is a required context.** The
  six required are `test-cli`, `test-messages`, `test-bridge`, `typecheck-snippets`,
  `build-site`, `test-md-regions`. Consistent with the operator's earlier "not now" on
  gating; re-deciding it is a maintainer call.

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

## Next steps (ranked)

🔴 **Ranks 1, 2, 3, 5, 6, 7, 8, 9, 12 and 13 are DONE — numbering is preserved
deliberately** so any live `claim-work` slug keeps pointing at the item it was taken for.

4. **P2 of the onboarding work**: `/.well-known/ai-catalog.json` +
   `/.well-known/agent-skills/index.json` with SHA-256 digests, advertised via
   `Link:` headers. 🔴 **Check `ai-catalog.yml` FIRST — it already exists** in the docs
   repo with a daily `cron: '37 6 * * *'` and a network half, and `check:ai-catalog` is a
   live script, so part of this may already be built. Read both before scoping.
   forcing: none
10. **cli#543** — the read `--help` rune budget: `users` sits **5 runes** under the
    1400 cap after #541, and `read_help_test.go:164-172` still names `images search` as
    the longest consumer, which stopped being true. Confirmed still OPEN 2026-09-11.
    forcing: none
11. **Residuals of decisions item 38**, in
    `claudedocs/decisions/38-read-body-repair-and-snippet.md`: four comment-citation rot
    sites (`download_ssrf_test.go:147`, `app_pull_not_approved_test.go:372`,
    `update_check_test.go:393`, `appblocks.go:1261`); the README documents no
    429 → exit 2 reclassification; and half (c) of
    `TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne`.
    forcing: none
14. **NEW — cli#399 is OPEN and UNCOVERED; the "one instrument fixes both" prediction is
    FALSIFIED.** Investigated 2026-09-11, deliberately not fixed (operator chose
    check-and-report). `safeTerm` is the CLI's one gate on server text reaching a
    terminal — **153 `safeTerm(` occurrences** in `internal/` non-test sources (my count;
    #399 says 150 sites in 56 functions) — and #399's audit found deleting the call at
    **20 of 25 sampled sites leaves the suite green**. cli#557's new guard does NOT reach
    it, for three structural reasons, each verified:
    `pkg/civitai/snippet_args_ledger_test.go:130` scans `os.ReadDir(".")` from
    `package civitai`, so `internal/cmd` is never walked; `:168` matches only
    `id.Name != "snippet"`, never `safeTerm`; and it is an **origin ledger** (where bytes
    come from), not a **coverage** assertion (would a test go red if the call vanished).
    Smallest instrument that would cover it: hang a coverage ledger on the AST walk
    already in `internal/cmd/safeterm_userinput_test.go`, keyed by **enclosing function**
    (that walk is currently flat over `file`, not `file.Decls`, so it has no enclosing-
    function attribution — that is the one structural change needed), asserted
    bidirectionally, with rows honestly reading NOT COVERED as the queue. 56 rows, not
    150 tests.
    forcing: none
15. **NEW — verify `drift-notify.mjs` against the REAL GitHub API on its first scheduled
    run.** It has only ever spoken to a local fake. Watch the first `appblocks-drift`
    cron tick and confirm: the issue is actually created; the `appblocks-drift` label
    either exists or the 403 fallback fires; the repo's Actions policy permits
    `issues: write`. **A notifier that cannot notify is the exact failure it exists to
    prevent, and its own failure is invisible by the same argument that motivated
    rank 12.** Closing condition: one real scheduled run observed end-to-end, or a
    deliberate `workflow_dispatch` exercising the notify path.
    forcing: gate — rank 12's closing condition is "a notify step merges"; it has merged,
    but its delivery is unproven against the real service, so the hole rank 12 named is
    not yet demonstrably closed.
16. **NEW — `#542`'s stated closing condition may not be discharged.** It named "any
    `saferune.*` call site outside `internal/cmd`". Measured 2026-09-11: there are
    **two** — `pkg/civitai/read.go` and `internal/genapi/status.go`. cli#557's guard
    matches `snippet`, not `saferune.*`, so it reaches neither as such, and the
    module-root `saferune_callers_ledger_test.go` pins **which packages import saferune**,
    not what they pass. Whether that discharges #542 is a judgement call — recorded so it
    is not silently assumed. Read with rank 14.
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

### Added 2026-09-11 (session 2, ranks 12–13 + the #399 check)

- 🔴 **GITHUB'S KEYWORD PARSER CLOSED AN ISSUE THE COMMIT EXPLICITLY SAID IT DID NOT
  CLOSE.** cli#557's commit `6fce127` body reads *"Does **NOT** close #399"*; GitHub
  matched `close #399`, ignored the negation, and closed it (auto-closed 16:56Z, reopened
  by ZacxDev 20:59Z). Both human-readable surfaces were correct; the automation was not.
  **Never write `close[s|d] #N` / `fix[es] #N` / `resolve[s] #N` in a commit or PR body
  unless you mean it — negation does not help.** Say "does not address #399" or reference
  it without a keyword. `via: measurement`
- 🔴 **A SCOPE-DRIFT GUARD ADDED ZERO API CALLS BECAUSE IT USES A DIFFERENT HOST, AND THE
  RATE LIMIT PROVED IT.** Manifests come from `raw.githubusercontent.com`; liveness from
  `api.github.com`. On a rate-limited run the sweep verified **3 of 8** repos live and
  **8 of 8** scope lists — the two numbers moved independently, which is the cleanest
  possible evidence that the scope check did not multiply the API budget. **When adding a
  network check, ask which HOST it hits before assuming it shares the old one's quota.**
  `via: measurement`
- 🔴 **THE GUARD AND THE NOTIFIER NEED OPPOSITE FAILURE POLICIES, AND SAYING SO IS THE
  DESIGN.** `check-example-apps.mjs` skips loudly and **exits 0** on an unreachable
  network — a false-fail makes a gate people click through. `drift-notify.mjs` **exits 1**
  when it cannot reach the API — a notifier that cannot notify is the failure it exists to
  prevent. Same repo, same sweep, deliberately inverted. Do not "harmonise" them.
- 🔴 **A TEST GREPPED THE WORKFLOW'S RAW `jobs:` TEXT — COMMENTS INCLUDED.** Correcting a
  stale claim in `cli-snapshot-refresh.yml` ("The ONLY elevation in this repo") tripped
  `test-refresh-cli-snapshot.mjs`, because the scope name written in a COMMENT matched its
  forbidden-scope grep. The fix was to move the comment to the file header, **not** to
  weaken the guard. **A guard that reads raw YAML cannot tell a comment from a
  declaration** — expect this when documenting permissions. `via: command`
- ⚠ **MY OWN GREP PIPELINE RETURNED A CONFIDENT `0` TWICE BEFORE I NOTICED.**
  `xargs -0 command grep` fails — `command` is a shell builtin, not an executable — and
  `which grep` here returns a shell FUNCTION body, which `find -exec` then cannot run.
  Both printed `0` matches for a pattern with 153 real hits. Fixed by using
  `/run/current-system/sw/bin/grep` directly **and running a positive control first**
  (`package cmd` → 67). **A zero from a pipeline you have not positive-controlled is a
  fact about the pipeline.** `via: command`
- ⚠ **DELIVERY VERIFIED AGAINST A FAKE IS NOT DELIVERY VERIFIED.** Nine scenarios over
  real HTTP against a local fake issues API is genuinely good engineering and still leaves
  the claim "it will notify" unproven — the real service can differ on label
  auto-creation, org policy, and token scope. Recorded as rank 15 rather than folded into
  rank 12's completion, because collapsing them is how "merged" becomes "working".

## How to verify

Ranks 12 + 13, on the merged docs tree:

```bash
cd /home/zach/workspace/civit/civitai-developer-docs
node scripts/check-example-apps.mjs --offline     # rc 0
node scripts/check-example-apps.mjs               # network half; reports BOTH counts
# expect: "N verified live … 0 rotted" AND "8 claimed … 8 verified … 0 drifted"
# 🔴 a rate-limited run legitimately exits 0 with "could not be reached and were NOT
#    verified" — read the COUNTS, never the exit code alone.

# the scope guard actually catches drift (reproduce in a detached copy):
#   add a scope to any "- **Scopes** —" line in apps/examples.md
#   -> rc 1, "SCOPES DRIFTED for github.com/<repo> — the page lists `<scope>` which the
#      manifest does not declare"
# 🔴 run `node --check` on any .mjs you mutate FIRST — a SyntaxError also exits 1.

node scripts/drift-notify.mjs --self-test          # rc 0 (fake API only — see rank 15)
```

Rank 14's finding, if you want to re-derive it rather than trust it:

```bash
cd /home/zach/workspace/civit/cli
grep -n 'os.ReadDir\|id.Name != "snippet"' pkg/civitai/snippet_args_ledger_test.go
# :130 scans "." from package civitai  ·  :168 matches only `snippet`
# neither reaches internal/cmd, and safeTerm appears once in the whole file
```

🔴 Read `rc` directly — `$?` after a pipe is the pipe's status. This repo has been misled
by that twice, and this session added a third shape: `xargs -0 command grep` and a
`find -exec` over a shell-function `grep` both return a confident, wrong `0`.
