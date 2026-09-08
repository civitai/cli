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

- **Branch `main` @ `d9b4e29`**, clean apart from an untracked `node_modules/`
  that is NOT gitignored (a stray npm install in the base clone — delete it or
  ignore it; it is a live `git add -A` footgun).
- **The repo is FROZEN and this blocks everything below.** See the investigation.
- **Two draft PRs in flight**, written by subagents in isolated worktrees:
  - `civitai/cli` — branch `feat/agent-setup`, the new command.
  - `civitai/civitai-developer-docs` — branch `feat/agent-setup`, the hosted
    `prompt.md` route, a landing page, two serving-bug fixes, the anti-rot guard.
  Neither was reviewed by this session; both reports land after this doc.
- 🔴 **The design contract is currently ONLY in an ephemeral scratchpad** —
  `…/7a9918ba-…/scratchpad/agent-setup-contract.md`. It is the frozen seam
  between the two repos (command surface, exit codes, `--check --json` shape, the
  per-agent MCP config table, the `AGENTS.md` template verbatim, `prompt.md`
  verbatim). **If neither PR carries it into a repo, it is lost when the session
  ends.** Next step 2 exists to close that.
- Work claimed: `claim-work --release agent-setup-onboarding-p1` when done.
- No clawgate task recorded: `clawgate_handoff.sh resolve` exited 5 (nothing
  resolved). The board WAS reachable — its positive control resolved 8 links for
  another session — but a wrong session id also answers 200/empty, so this is not
  evidence that no task exists.

### Decisions taken this session (settled by the operator, do not relitigate)

- Audience: **both tracks, branched** — but **P1 ships the app track only**; the
  API/orchestration branch is deferred to P2.
- Side effects: **install + configure, stop before auth.** Nothing runs
  `civitai login`.
- Beta gate: **assume Apps-author access and fail loudly** — no capability branch.
- Instruction file: **`AGENTS.md`**, plus a one-line `CLAUDE.md` → `@AGENTS.md`
  shim, because Claude Code does not read `AGENTS.md`
  (code.claude.com/docs/en/memory). No `.cursor/rules`, no `GEMINI.md` in P1.

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

## Next steps (ranked)

1. **Unfreeze `civitai/cli` — land the scaffold pin bump to `^0.39.0` /
   `^0.49.0`.** Run the Next probe in the first investigation to decide whether
   it is an npm crash or a real SDK break, then fix accordingly. Touches
   `internal/scaffold/templates/page-money/package.json.tmpl`,
   `internal/scaffold/scaffold_test.go`, and probably
   `.github/workflows/bump-scaffold-pins.yml`.
   forcing: gate — `pins-vs-published` is a required context with
   `enforce_admins: true` and is red on `main`; nothing in the repo can merge,
   including both PRs from item 2.
2. **Review and land the two draft agent-setup PRs**, and in the same pass
   **move the design contract out of the scratchpad into a repo** — the natural
   home is `civitai/cli:claudedocs/decisions/` or a doc beside the command.
   IN FLIGHT: `civitai/cli` `feat/agent-setup`, `civitai/civitai-developer-docs`
   `feat/agent-setup`.
   forcing: none
3. **PR `civitai/cli#526`** (external contributor `xsvm`, opened 2026-09-06,
   +105/−52 across `internal/cmd/read.go`, `pkg/civitai/read.go`,
   `read_test.go`; fixes issue #525). **Zero checks have ever run** — it is a
   fork PR needing maintainer approval to trigger workflows. Approve the run so
   there is a signal, then review.
   forcing: user — an external contributor has been waiting on a first response
   since 2026-09-06.
4. **P2 of the onboarding work**: `/.well-known/ai-catalog.json` +
   `/.well-known/agent-skills/index.json` with SHA-256 digests, advertised via
   `Link:` headers the way Mintlify does. Cloudflare ships the artifacts and
   advertises none of them; nobody currently does both.
   forcing: none
5. **Delete or gitignore `node_modules/`** in the `civitai/cli` base clone.
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

## How to verify

The freeze, and whether item 1 cleared it:

```bash
cd /home/zach/workspace/civit/cli
CIVITAI_CHECK_PUBLISHED_PINS=1 go test ./internal/scaffold \
    -run TestScaffoldPinsSatisfyPublished -count=1   # must PASS
gh api repos/civitai/cli/branches/main/protection \
    --jq '.required_status_checks.contexts'
```

The two serving bugs (read the CONTENT and the header, not the status alone):

```bash
curl -sI -A 'Python-urllib/3.11' https://developer.civitai.com/ | head -1
curl -sI https://developer.civitai.com/apps/guide/quickstart.md | grep -i content-type
```

The onboarding entrypoint end to end, once both PRs land:

```bash
curl -s https://developer.civitai.com/agent-setup/prompt.md | head -5   # must be markdown, not HTML
civitai agent-setup --track app --dir /tmp/probe-app
civitai agent-setup --check --json | jq .
```

🔴 Read `rc` directly rather than `$?` after a pipe — `$?` reports the pipe's
status, which this repo has already been misled by once.

Full gate — `make ci` is **not** a superset of CI (it omits golangci-lint):

```bash
make ci && nix-shell -p golangci-lint --run "make lint"
```
