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

**The entrypoint is LIVE, dogfooded twice, and repaired. This effort is essentially
complete** — what remains is listed under Next steps and none of it blocks a user.

- **`https://developer.civitai.com/agent-setup/prompt.md`** — HTTP 200,
  `text/markdown; charset=utf-8`, `nosniff`, `Vary: Accept-Encoding`, no redirect,
  **byte-identical to `origin/main`** (6,949 B) verified through Cloudflare.
- **`@civitai/cli@0.1.104`** on npm and Homebrew. Shipped artifact verified:
  checksum OK, reports `civitai 0.1.104`, and `agent-setup` run *from the
  downloaded binary* produces a correct per-project block.
- **`civitai/cli` README leads with the paste string** (#536), byte-identical to
  the docs repo's `SETUP_PROMPT` constant — four surfaces, one source.

### Merged this session

| PR | |
|---|---|
| cli#529 | pin bump → **unfroze the repo** (`pins-vs-published` had been red, `enforce_admins`) |
| cli#527 | the first handoff doc |
| cli#528 | `civitai agent-setup` — 5 audit rounds, clean final round |
| cli#533 | per-project `AGENTS.md` block (no npm commands in a `static` project) |
| cli#534 | `create` not `init`; `--check`'s `mcp-*` wording; the token source |
| cli#536 | README one-liner |
| cli#537 | gitignore `/node_modules/` |
| docs#67 | `prompt.md` route, landing page, `.md` content-type, the guard |
| docs#69 | tag-strip rewrite (CodeQL + a real command-deleting bypass) |
| docs#70 | round-2 doc fixes |

Releases **v0.1.103** and **v0.1.104** both tagged, published, and confirmed on
npm by polling the registry rather than trusting the workflow.

### What was NOT done, and why

- **Example apps were never covered.** They were in the original ask
  ("design, components, site host messaging, api, example apps") and are the one
  named surface with no pointer anywhere. Design/components/messaging are
  reachable via `/apps/reference/`; the API track was deferred by operator choice.
  Nothing points at the seven example-app repos or `/apps/showcase`.
- 🔴 **The design contract was deliberately NOT committed**, retiring rank 2's
  second clause. Decisions 34 (27 KB), 35 (52 KB) and 36 (12 KB) now own its
  durable content with tests behind them, and the cross-repo seam it defined is
  enforced *mechanically* by `check-agent-setup.mjs`. Committing it would have
  added a fourth copy of rules that already have owners — the drift class this
  session kept finding. It dies with the session on purpose.

### Recorded in the subsystem index (`cli/scaffold`, 7 bullets)

- The 2026-09-08 `OPEN:` bullet was **rewritten, not closed**: the freeze itself is
  cleared (cli#529) and the SDK and node 24 are both exonerated with the
  measurements, but the nightly's inability to land its own bump is still open
  (cli#530), so the marker stays.
- Appended the `init` vs `create` fact — including the one site that must KEEP
  saying `init`, because ready-ack advice needs the static template's
  `civitai-host.js` and page-money does not ship it.
- ⚠ My first rewrite wrote `OPEN (narrowed …):` — a parenthetical **before** the
  colon, which is the documented near-miss grammar. The badge flipped to
  `1 NEAR-MISS` and the openness became invisible. Fixed to `OPEN: (narrowed …)`;
  the row reads `🔴 1 OPEN` again. **Read the badge after any marker edit.**

### Deploy/verify honesty

Everything above was verified against the **served** artifact, not the merge:
npm was polled until it served the new version; the live doc was `cmp`'d against
`origin/main`; the release binary was downloaded, checksum-verified and run.

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

## Next steps (ranked)

🔴 **Ranks 1, 2 and 5 are DONE — numbering is preserved deliberately** so any live
`claim-work` slug keeps pointing at the item it was taken for.

1. ~~**Unfreeze `civitai/cli`**~~ — **DONE**, cli#529.
   forcing: none
2. ~~**Land the two draft agent-setup PRs**~~ — **DONE** (cli#528, docs#67). The
   second clause, "move the design contract out of the scratchpad", is
   **deliberately retired** — see State now.
   forcing: none
3. **PR `civitai/cli#526`** (external contributor `xsvm`, fixes #513's sibling
   issue #525) — **still has zero CI ever run**; fork PRs need a maintainer to
   approve the workflow. Cannot be synced from here (pushing to a fork branch is
   not ours). Approve a run, then review.
   forcing: user — an external contributor has been waiting since 2026-09-06.
4. **P2 of the onboarding work**: `/.well-known/ai-catalog.json` +
   `/.well-known/agent-skills/index.json` with SHA-256 digests, advertised via
   `Link:` headers. Cloudflare ships the artifacts and advertises none; Mintlify
   advertises. Nobody does both.
   forcing: none
5. ~~**Delete or gitignore `node_modules/`**~~ — **DONE**, cli#537. It was not a
   stray npm install: it held a test RSA keypair with a `privatePem`.
   forcing: none
6. **Example apps are the one named surface with no pointer.** The shipped
   `AGENTS.md` links `/apps/guide/`, `/apps/reference/` and `/llms.txt` — nothing
   reaches the seven example-app repos or `/apps/showcase`. Decide whether the
   block should name them, and where the list lives so it cannot rot.
   forcing: user — it was in the operator's original scope and was never
   explicitly deferred, unlike the API track.
7. **cli#530** — make the nightly open its bump PR even when step 9 fails, and
   diagnose the `edgesOut` crash. Until then the repo re-freezes on every
   upstream publish and a human has to bump by hand.
   forcing: regression — `pins-vs-published` is required with `enforce_admins`,
   so this takes the whole repo down, and it has already happened once.
8. **A hard byte ceiling on `prompt.md`, enforced by `check-agent-setup.mjs`.**
   It went 2,798 → 6,949 B across two fix rounds with every addition justified,
   and nothing measures the total. The measured failure mode is length: a real
   agent's `WebFetch` returned an LLM *summary* that silently dropped the whole
   install-failure section, both MCP URLs and **all of step 5**.
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

## How to verify

The whole entrypoint, from the outside:

```bash
# 1. the served contract
curl -sI https://developer.civitai.com/agent-setup/prompt.md   # 200, text/markdown, nosniff
diff <(curl -s https://developer.civitai.com/agent-setup/prompt.md) \
     <(git -C /home/zach/workspace/civit/civitai-developer-docs show origin/main:public/agent-setup/prompt.md)

# 2. the published CLI, installed the way a user would
T=$(mktemp -d); npm install -g --prefix "$T" @civitai/cli --silent
"$T/bin/civitai" --version          # want 0.1.104 or later
for c in agent-setup "app create" upgrade login; do "$T/bin/civitai" $c --help >/dev/null && echo "ok $c"; done

# 3. the per-project block is honest in BOTH shapes
P=$(mktemp -d); cd "$P"
"$T/bin/civitai" app init probe-static  && "$T/bin/civitai" agent-setup --dir probe-static
grep -c npm probe-static/AGENTS.md      # want 0 — a static project has no package.json
"$T/bin/civitai" app create probe-money && "$T/bin/civitai" agent-setup --dir probe-money
grep -oE 'npm run [a-z:]+' probe-money/AGENTS.md   # every one must exist in its package.json
```

🔴 **Read `rc` directly — `$?` after a pipe is the pipe's status.** This repo has
been misled by that twice.

🔴 **A bare `civitai` on the operator's machine resolves to a stale 0.1.101** at
`~/.local/bin/civitai` (and `~/go/bin/civitai`), which has no `agent-setup`. Use a
full path, or run `civitai upgrade` on it.

The guard that keeps the doc and the CLI in agreement:

```bash
cd /home/zach/workspace/civit/civitai-developer-docs && npm run check:agent-setup; echo "rc=$?"
```
