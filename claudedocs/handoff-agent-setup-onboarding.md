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

✅ **THE ARC IS CLOSED.** `cli#665` arm 1 is MET against the **live** hosted prompt and the issue
is closed. Nothing is in flight: no open PR, no running container, no worktree, no held claim.

- **The closing measurement:** 5 of 6 blind cells `CLOSING_CONDITION=yes`, every `yes` reading
  `login_version=0.1.113 agent_shell_version=0.1.113`, all 6 `stop=finished`, $0.058.
  🔴 **The provenance control ran BEFORE any verdict was read: 0 of 6 `start` records carry a
  `prompt_url` key**, so none was fed patched instructions. `grade.sh` reads the container, so
  that control is the only thing that can tell a patched run from a real one.
- **Merged, each verified BY CONTENT with a negative control** (never by ancestry — a squash
  makes that permanently false): `docs#152` `021ec58f` (the EACCES-branch edit — *this* is what
  moved the number) · `cli#784` `7df6e273` (doc, corrected before merge) · `cli#785` `efa1d6c2`
  (`--prompt-url` + the provenance mark) · `cli#787` (residuals, batched). Controls: docs `main~1` inverted (0/1), cli `main~2` `pf_auth`=0; `docs#152` probed
  SERVING and `cmp`-identical to the bytes the trials were fed.
- 🔴 **WHAT CLOSED IT WAS PROSE, AND THE RATIO IS THE POINT.** `cli#777` shipped `--fix-path` in
  `v0.1.112` and changed **nothing** (0 of 6). Same file, same cells: prose *asking* the agent to
  run it → **1 of 3**; the command **inside the fenced install block** → **5 of 6**. Across all 15
  patched-and-live trials the correlation never broke: every cell that executed `--fix-path`
  graded `yes`; every cell that did not graded `no`.
- ⚠ **5 of 6, not 6 of 6** — the live `no` (`mimo-v2.5` × `node-user`) is the cell that did not
  run the flag. Scope: 3 cheap models, `claude` identity, 2 envs, Linux only; the Homebrew branch
  never executes and **WebFetch-summarisation** is still untested, as `#665`'s scope note said.
- **`cli#785` ran audit rounds 0–3, each finding real defects** — three in code I had just called
  verified. Stopped after round 3 by operator decision; blockers fixed, residuals filed.
- **No `clawgate-task:` field**: `resolve` exited **5** (0 tasks) and `field` exited **1**. An
  unknown session id answers 200 with an empty array, so that zero cannot distinguish "touched no
  task" from "wrong id" — not a clean bill of health. 🔴 Captured directly; a `| head` ate the
  status first time, which is the trap the skill names.
- **Evidence** at `~/.cache/dogfood-runs-2026-10-04-{r38,r42,live}/`, each with controls
  (end-records matching trial count, nonsense-string control at 0); `-live/` carries
  `prompt-as-served.md`, `cmp`-identical to `origin/main`.
- **Release provenance and the arm-2 evidence** (`v0.1.112` publish time, Homebrew tap
  `46ebc701`, and the agents' own "it is **not** permanent" finals) are verbatim in `#665`'s
  closing comment — deliberately not re-copied into a doc 2.2x its ceiling.
- **Cleanup verified:** 25 containers removed by EXACT resolved name (the unrelated app-author
  arc's 15 untouched), 3 worktrees by exact path + prune, no listener on `:8099`.

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

### ❌ SUPERSEDED (2026-09-12) — "`developer.civitai.com` returns 403 to `Python-urllib` — IN FLIGHT, delegated"

### ✅ RESOLVED — the repo freeze (`pins-vs-published`)

### ❌ SUPERSEDED (2026-09-12) — "🔴 STILL OPEN — `bump-scaffold-pins` cannot land its own fix (cli#530)"

### ❌ SUPERSEDED (2026-09-12) — "⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge" (1 of 2)

### ✅ RESOLVED — `bump-scaffold-pins` could not land its own bump (cli#530)

### ❌ SUPERSEDED (2026-09-12) — "⚠ STILL OPEN — `Python-urllib` 403 at the Cloudflare edge" (2 of 2)

### ✅ RESOLVED — `developer.civitai.com` 403 to `Python-urllib`

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

### ~~✅ DECIDED (2026-09-19) — an agent the CLI does not know can never reach `ok: true` (rank 33)~~ IMPLEMENTED 2026-09-19 — see "✅ IMPLEMENTED — rank 33 shipped as cli#669" below
- as-of: 2026-09-18, from 19 blind trials; **argument rewritten the same day after

### ~~⚠ OPEN — the documented `--prefix` remedy leaves the CLI unreachable (rank 34)~~ PARTLY ADDRESSED 2026-09-19 — see "✅ SHIPPED — ranks 33/34/35 and v0.1.106" below
- as-of: 2026-09-18, from 19 blind trials

### ✅ IMPLEMENTED — rank 33 shipped as cli#669, and the decision record's plan needed three corrections
- as-of: 2026-09-19

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

### ~~⏳ IN FLIGHT — rank 36: the first post-ship dogfood measurement, and a 25× cost error in this doc~~ ✅ MEASURED 2026-09-19 — the matrix completed; see "✅ MEASURED — rank 36" below. The cost and control findings here STAND; only the "in flight" status is superseded.
- as-of: 2026-09-19

### ✅ MEASURED — rank 36: mechanism A is closed, mechanism B is not, and agents do not take the documented remedy
- as-of: 2026-09-19

### ~~⚠ OPEN — `cli#665`'s arm 1 is ungraded, and three sequenced steps stand between #777 and grading it~~ SUPERSEDED 2026-10-04 — arm 1 is now GRADED; see "⚠ OPEN — `cli#665` arm 1 is GRADED and NOT MET" below
- as-of: 2026-10-04

### ⚠ OPEN — five fixes in `cd1a5db` are FIXED-BUT-UNAUDITED, with the gaps enumerated
- as-of: 2026-10-04

- **Symptom + exact repro:** round 5's two 🟡 and three 🟢 were fixed without a round 6. The fixes
  are mutation-checked individually but no independent pass read the result. `gh pr view 777
  --comments` carries the full list.
- **Observed (with values):** the gaps the fix round stated rather than hid — (a) **no pin exists
  for "every outer error of `pathFixTargets`"**, so a *fourth* early return would again be invisible;
  only the row's shape is guarded. (b) **The third cause has no automated test** — reaching
  `filepath.Abs(home)` failing needs `os.Chdir` into a deleted directory, process-global and racy
  against parallel tests, and a flaky guard was judged worse than none; it was measured once with
  the real binary (`HOME=relhome` from a deleted cwd → empty-path `blocked` row, exit 1).
  (c) The template prose guard does not catch a contradiction that never names "Windows".
  (d) `multiShellMinDirShapes`' count floor cannot fail alone, being implied by five distinct names.
  (e) **`ineffassign` did not fire** on a `got := ""` immediately-overwritten initialiser despite
  being enabled — unexplained; staticcheck is demonstrably live, so this is an instrument property
  nobody has diagnosed.
- **Ruled out:** that the enumeration should have been corrected to "three" rather than deleted —
  this ladder rotted **three** hand-maintained counts (file-corpus totals, `### Round N` sections,
  the cause count), so the fix names `pathFixTargets` as the authority instead. `via: measurement`
  · That the third cause was recent drift — its message dates to `667c59c`, round 1's own R6/M23, so
  round 4's "TWO" was short by one **on arrival**. `via: command`
- **Leading hypothesis:** (a)–(d) are accepted costs with stated reasons; (e) is the only one that
  is genuinely unknown.
- **Next probe:** for (e), `golangci-lint run --disable-all --enable=ineffassign` over a minimal
  reproducer of `got := ""` followed by an unconditional assignment, at v2.12.2 — that separates a
  config problem from a linter behaviour.

### ~~⚠ OPEN — `cli#665` arm 1 is ungraded, and the issue's closure hides that~~ GRADED 2026-10-04 — see "⚠ OPEN — `cli#665` arm 1 is GRADED and NOT MET" below
- as-of: 2026-10-04

### ⚠ OPEN — `--fix-path` writes `$HOME/.zshenv`, which zsh ignores under `ZDOTDIR` (cli#781)
- as-of: 2026-10-04
- **Symptom + exact repro:** on a host with `ZDOTDIR` set, `--fix-path` writes `$HOME/.zshenv`,
  reports success and names the file, and no later zsh ever reads it.
- **Observed (with values):** a synthetic `~/.zshenv` prepending a marker dir was LOST under
  `zsh -lic`, `-ic` and `-c` with `ZDOTDIR=/home/zach/.config/zsh` set, and SURVIVED with it unset
  or pointed at the test HOME. End-to-end, with `ZDOTDIR` set the npm binaries dir was absent from
  `$path` entirely.
- **Ruled out:** that the suite would catch it — `agent_setup_fix_path_shell_test.go` **strips
  `ZDOTDIR`**, with a comment saying otherwise "every assertion below would be vacuous". So the
  suite is blind to the axis by construction. `via: code`
- **Ruled out:** that it is already disclosed — `ZDOTDIR` appears nowhere in
  `internal/cmd/agent_setup_fixpath.go` or in decision 39's "WHAT IT DOES NOT REACH". Decision 39
  discusses whether zsh is INSTALLED, a different question. `via: code`
- **Leading hypothesis:** two legitimate fixes, and choosing is a product call — honour it
  (`${ZDOTDIR:-$HOME}/.zshenv`) or disclose it. See cli#781 for both.
- **Next probe:** none needed to diagnose; the decision is the blocker.

### ⚠ OPEN — `cli#665` arm 1 is GRADED and NOT MET: the fix works, the hosted prompt forbids it
- as-of: 2026-10-04

**Supersedes both earlier `cli#665` blocks**, whose headings are retired above — one modelled the
gap as release sequencing, the other as an environment problem. Both are measured wrong.

- **Symptom + exact repro:** on a machine where npm's global prefix is not writable, a blind agent
  given only the hosted URL installs the CLI successfully and leaves it unreachable from every
  later shell, so `zsh -lic 'civitai --version'` fails and arm 1 never goes green. Reproduce:
  ```bash
  WT=/home/zach/workspace/civit/cli-r38; cd "$WT"/scripts/dogfood
  for e in node-user ubuntu-apt; do docker build -q -t "df-$e" -f "envs/$e.Dockerfile" .; done
  set -a; . ~/.config/repo-cos/env; set +a
  DOGFOOD_MODELS='z-ai/glm-5.3-flash|glm xiaomi/mimo-v2.5|mimo deepseek/deepseek-v4-pro|dsv4' \
  DOGFOOD_ENVS='df-node-user|nodeuser|dev df-ubuntu-apt|ubuntu|dev' \
  DOGFOOD_IDENTITIES='claudeid|CLAUDECODE=1' DOGFOOD_TRIAL_PREFIX=r38 bash driver.sh
  ```
- **Observed (with values) — the grid.** Every cell `agent=unknown check_ok=parse-error
  failed_checks=[] mcp_rows=unreadable login_version=none agent_shell_version=none`:

  | cell | stop | steps | cost | `fix-path` in transcript | verdict |
  |---|---|---|---|---|---|
  | glm × node-user | finished | 8 | $0.00246 | 0 | ❌ no |
  | glm × ubuntu-apt | finished | 10 | $0.00381 | 0 | ❌ no |
  | mimo × node-user | finished | 9 | $0.00252 | **7** | ❌ no |
  | mimo × ubuntu-apt | finished | 9 | $0.00117 | 0 | ❌ no |
  | dsv4 × node-user | finished | 10 | $0.01258 | 0 | ❌ no |
  | dsv4 × ubuntu-apt | finished | 10 | $0.01371 | 0 | ❌ no |

- 🔴 **Observed — the mechanism, in the agent's OWN WORDS.** `mimo × node-user` found the flag from
  `agent-setup --help` and refused to use it: *"(`civitai agent-setup --fix-path` can also append a
  marker-guarded block to your startup files for you — **I didn't run it since editing your shell
  profile is yours to decide.**)"* The hosted `prompt.md` EACCES branch ends: *"Report it verbatim
  in step 5 so the user can make it permanent — **do not edit their shell profile yourself.**"*
  The agent obeyed the prompt. `grep -c 'fix-path' /tmp/prompt.md` → **0** (http 200, 7,433 B).
- **Observed — `AGENTS.md` is the wrong channel, measured.** `grep -ac 'AGENTS.md'` is non-zero in
  the transcripts only as `agent-setup`'s own `✓ create /work/AGENTS.md` line and the agent's final
  report; **no cell read the file.** The flag's only prose home is its line 35.
- **Ruled out — that the installs failed.** Complete enumeration, 6 of 6 containers:
  `installed=/home/dev/.npm-global/bin/civitai`, `bash_login=none`, `zsh_login=none`,
  **no `~/.zshenv` written**. Identical to the pre-#777 state. `via: measurement`
- **Ruled out — that cheap models cannot drive the task.** All 6 `stop=finished` in 8–10 steps,
  5–8 `EACCES` hits and 14–20 `npm-global` mentions each; every one installed the CLI and reported
  a verified setup. `via: measurement`
- **Ruled out — that `--fix-path` is inert on this path.** New control `ctl-reach`, published
  v0.1.112 in a blind `df-node-user` container, reproducing the `~/.npm-global` install 6/6 agents
  choose: BEFORE → `bash -lc` and `zsh -lic` both `command not found`; AFTER
  `civitai agent-setup --fix-path` → **both print `civitai 0.1.112`**, having written
  `/home/dev/.zshenv` and `/home/dev/.profile`. The "before" reading is the negative control.
  `via: measurement`
- **Ruled out — that the CLI should have warned at runtime and did not.** It is deliberate:
  `TestEveryBlockTellsYouWhatToDoWhenTheCLIIsNotOnPATH` is *"deliberately UNCONDITIONAL"* and
  decision 36 forbids a per-machine claim — two earlier drafts that detected the condition were
  each measured wrong. So `fix-path in agent-setup stdout = 0` is correct behaviour, NOT a gap.
  🔴 Do not re-derive "add a runtime PATH warning" from the grid; it was already tried and rejected.
  `via: code`
- **Ruled out — that the grader is at fault.** All three documented controls were run first, on
  v0.1.112: `ctl-neg` → `no`/`parse-error`/`unreadable`; `ctl-pos` → `yes`, both arms `0.1.112`;
  `ctl-profile` → `check_ok=true` with `login_version=none` ⇒ `no`, i.e. arm A green while arm B
  red, which is the discriminating one. `via: measurement`
- **Leading hypothesis:** the remaining gap is one sentence of prose in another repo. Editing the
  `prompt.md` EACCES branch to run `civitai agent-setup --fix-path` — and deleting the
  "do not edit their shell profile yourself" instruction that contradicts it — is predicted to flip
  these cells. ⚠ **PREDICTED, NOT MEASURED**: it rests on one agent's stated reason plus `ctl-reach`.
- **Next probe:** make the prediction testable before shipping the wording. `runner.py` hardcodes
  the prompt URL at line 51 and has no override, so add a `--prompt-url` flag (plumbed through
  `driver.sh`), serve a patched `prompt.md` locally, and re-run 3 cells at ~$0.02. That converts
  the inference into a measurement and is the only probe that can.

### ⚠ OPEN — arm 1 is unmeasured against the LIVE entrypoint; every number so far used a patched copy
- as-of: 2026-10-04

- **Symptom + exact repro:** arm 1 is graded from a blind trial fed the **hosted** `prompt.md`.
  Every measurement in this arc was fed a locally-served patched copy — evidence about proposed
  wording, not about the shipped entrypoint.
- **Observed (with values):** `docs#152` merged 20:36:52Z, squash `021ec58f`; `origin/main` has
  `--fix-path` 1 / forbidding sentence 0, `main~1` exactly inverted. Patched grid 5 of 6 `yes`.
  Hosted grid: **not run**.
- **Ruled out — that patched runs substitute.** They are marked `prompt_url`, and `grade.sh`
  reads the container so its verdict cannot tell them apart. `via: code`
- **Ruled out — that merging is serving.** `docs#147` merged while the live `.md` twins still
  served the old text with `build-site` queued. `via: measurement`
- **Leading hypothesis:** the live grid reproduces 5 of 6, because the served bytes are
  `cmp`-identical to what was measured. A prediction, not a result.
- **Next probe:**
  ```bash
  curl -sS -A 'Mozilla/5.0' https://developer.civitai.com/agent-setup/prompt.md \
    | grep -c 'agent-setup --fix-path'     # 1 => serving
  D=/tmp/wt-prompturl/scripts/dogfood
  cd "$D" && DOGFOOD_TRIAL_PREFIX=live DOGFOOD_ENVS='df-node-user|nodeuser|dev' \
    DOGFOOD_IDENTITIES='claudeid|CLAUDECODE=1' \
    DOGFOOD_MODELS='z-ai/glm-5.3-flash|glm xiaomi/mimo-v2.5|mimo deepseek/deepseek-v4-pro|dsv4' \
    bash driver.sh                        # NO DOGFOOD_PROMPT_URL — that is the point
  for id in live-{glm,mimo,dsv4}-nodeuser-claudeid; do bash grade.sh "$id" dev | tail -1; done
  ```
  🔴 Each `start` record must carry **no** `prompt_url`. If one does, the run is patched and
  cannot close `#665`.

### ~~⚠ OPEN — arm 1 is unmeasured against the LIVE entrypoint~~ ✅ RESOLVED 2026-10-04 — 5 of 6 live, `#665` CLOSED
- as-of: 2026-10-04

🔴 **ITS `Next probe` WAS RUN AND ITS PREDICTION HELD** — 5 of 6 live, provenance control clean,
served bytes `cmp`-identical to what was measured. Nothing in it is open; its measurements stay as
the baseline the live run was compared against.

## Next steps (ranked)

🔴 **THE ARC IS CLOSED. Everything below is either DONE or was always independent of it.**
Numbering preserved so live `claim-work` slugs keep resolving.

15. **The docs repo does not build from a pristine `main` locally.** NOT re-verified.
    forcing: gate
16. **`images search --help` 14 runes under budget.** ⚠ RETIRE.
    forcing: none
17. **Audit at MERGE time.** ⚠ CONVERT, not work.
    forcing: none
25. **cli#579 — `saferune.Strip` doc claims a byte-for-byte subsequence.** Inert.
    forcing: none
26. **cli#575 R2–R4.** ⚠ R2/R3 DEFER; R4 has the coverage value.
    forcing: none
27. **cli#586 — three near-identical AST expression renderers.**
    forcing: none
29. ⚠ **PARTLY DONE** (eviction run 4×). ❌ Left: the `## Gotchas` split, and demoting dated
    evidence to `claudedocs/refs/`.
    forcing: gate
34. ✅ DONE — `cli#777` → `v0.1.112` → `docs#126`.
    forcing: gate — satisfied
38. ✅ DONE — arm 1 graded 6/6 `no`; cause was delivery, not the CLI.
    forcing: gate — satisfied
39. 🔴 **THE ONLY OPEN ITEM WITH URGENCY, AND NOT THIS ARC'S.** Decide `cli#781` (ZDOTDIR):
    honour `${ZDOTDIR:-$HOME}/.zshenv` or disclose. `--fix-path` writes `$HOME/.zshenv`, which zsh
    ignores under `ZDOTDIR`, and `agent_setup_fix_path_shell_test.go` **strips** the variable — so
    the suite is blind to the axis by construction and a fix needs one probe that does not strip
    it. No container that closed `#665` set `ZDOTDIR`, so this arc's green says nothing about it.
    forcing: gate — cli#781 is open
40. **The app-author onboarding arc — its own doc when it next grows.** `docs#151` carries the
    three remaining routing pointers.
    forcing: user — an external app author reported being blocked and the second half is unfixed
41. ✅ DONE — `docs#147` verified SERVING.
    forcing: gate — satisfied
42. ✅ DONE — `docs#152` merged (`021ec58f`), operator decision for arm 1.
    forcing: gate — satisfied
43. ✅ DONE — arm 1 MET live, 5 of 6, control clean; `#665` CLOSED.
    forcing: gate — satisfied
44. ✅ DONE — `cli#785` merged (`efa1d6c2`); residuals are `cli#787`.
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

- 🔴 **2026-10-04 — A HAND-MAINTAINED COUNT IN PROSE ROTS; DELETE THE ENUMERATION AND NAME THE
  AUTHORITY.** Three separate counts rotted inside one audit ladder on cli#777: a sweep's file-corpus
  totals (745/746, reproducible under no corpus definition), the decision record's `### Round N`
  section count ("grows by one per round" — returned 3 after five rounds), and `pathFixTargets`'
  cause count ("TWO CAUSES" where the code had three, and short by one **on arrival** — the third
  return dated to the same ladder's round 1). Each was *corrected* once and rotted again. The fix
  that held was deleting the number and naming the function that decides. **A count beside the thing
  it counts will drift; a pointer to the authority cannot.**
- 🔴 **2026-10-04 — AN AUDIT FINDING IS A POINTER TO VERIFY, NOT A SPEC TO BRIEF.** On cli#777 I
  relayed audit findings into fix briefs without re-measuring and authored **four** wrong
  instructions that way: "add `:` to the refusal" (a literal `:` refuses every Windows path, so the
  agent correctly deviated to `os.PathListSeparator` — which was *also* wrong, because the emitted
  block is POSIX `sh` and always splits on `:`); "no `GOOS` gate, it would break MSYS" (the colon
  refusal already refuses MSYS, through a different line); "say two causes" (there are three); and
  "add the phrase to the `forbidden` list" (a **spelled** guard — the fix agent proved by mutation
  that a reworded contradiction survives it, and pinned a *relationship* instead). Every one was
  caught by the next round, which is the only reason they are not in `main`. **Re-measure a finding
  before you brief it, and when a subagent deviates with a measurement, read the measurement.**
- 🔴 **2026-10-04 — A GUARD THAT PINS A WORD IS WALKABLE BY THE HOUSE IDIOM.** cli#777's
  `strings.Count(code, "runtime.GOOS") != 1` was defeated by `env.GOOS == "windows"` — the package's
  own injected-OS seam, used in exactly that shape elsewhere in the same package. The mutant survived
  the **entire module**. A count of one spelling cannot be repaired by counting another: the cure was
  switching the gate to the injected seam so the refusal became **behaviourally** testable, then
  asserting the behaviour. Same lesson one level up: a prose claim must be pinned as the whole
  normalised sentence *and* by a relationship (the block may mention "Windows" only inside the
  pinned claim), because `strings.Contains` proves a claim is PRESENT, never UNCONTRADICTED.
- ⚠ **2026-10-04 — THIS LADDER ENDED ON ZERO-EXECUTABLE-PAYLOAD, NOT ON CONVERGENCE, AND THE
  ATTRIBUTION GATE COULD NOT FIRE.** Payload ran 161 → 151 → 189 → 189 → 71 lines with executable
  sub-splits 60 → 18 → 35 → 35 → **0**. The gate needs two consecutive payload-free rounds; this
  PR's diff is *mixed* rather than purely prose, so neither that gate nor the prose-regime escape
  hatch applied, and by the letter the ladder ends only by converging. It was stopped by operator
  decision on the executable-zero signal instead — recorded as fixed-but-unaudited. **When a ladder's
  executable payload hits zero while findings keep arriving, the findings are about the ladder.**

### Added 2026-10-04 — shipping rank 34, and four rounds of audit on a 29-line docs PR

- 🔴 **A HEALTHY NIGHTLY THAT RAN RECENTLY IS NOT EVIDENCE THE NEXT ONE IS NEAR.** `pins-vs-published`
  blocked the whole repo for a ~19h window: the bot swept at `12:34Z` and correctly opened nothing
  (`0.55.0` was still latest), then `app-sdk` published `0.56.0` at `16:48Z` and `0.56.1` at `23:43Z`.
  The previous handoff's "the next scheduled run opens the bump PR" was right in mechanism and wrong
  in timing. **Read the publish timestamp against the cron, and `gh workflow run` it** — the workflow
  carries `workflow_dispatch`, so dispatching is the bot authoring its own bump, not the hand-authored
  PR this doc forbids.
- 🔴 **A `GITHUB_TOKEN`-OPENED PR GETS NO CHECKS, AND SHOWS **ZERO** — WHICH READS AS "NOTHING TO
  WORRY ABOUT" ON THE ONE PR NOBODY VERIFIED.** `docs#126` sat open from 2026-09-28 while the
  committed CLI-help snapshot stayed three releases behind. One human empty commit made the checks
  dispatch and three went red immediately. **The empty commit did not cause them, it revealed them.**
- 🔴 **A DOCS-SNAPSHOT BUMP CAN RED CHECKS THAT ARE NOTHING TO DO WITH IT — ESTABLISH A `main`
  BASELINE BEFORE ATTRIBUTING ANY OF IT.** Of three reds on #126, two were introduced (one phrase:
  `app create`'s long description changed `app developers only` → `page tokens only`) and one was
  pre-existing. The discriminator was dispatching the three required workflows against `main` via
  `workflow_dispatch`; they are `pull_request`-only, so CI history has no baseline to read.
- 🔴 **`git checkout -- <file>` TO UNDO A PLANTED CONTROL REVERTED THREE REAL EDITS IN THE SAME FILE.**
  Caught immediately, but RULES already says copy aside and restore by copying back. Second time this
  session a cleanup step destroyed adjacent work.
- 🔴 **I RELAYED AN AGENT'S FINDING WITHOUT CHECKING IT AND IT WAS WRONG, IN THE DIRECTION THAT SHIPS
  A FALSE DOC.** A research pass said "only `dev-tunnel` is gated"; measured at `origin/release`,
  `dev-token` and `submit-version` gate on the SAME two flags and the tunnel adds a third. So
  quickstart's "both are invite-gated" was LITERALLY TRUE and I nearly "fixed" it into something
  false. Retracted publicly on `docs#147`.
- 🔴 **THE SAME DEFECT SHAPE FOUR TIMES IN ONE PR: A CLAIM NARROWED IN ONE PLACE AND LEFT STANDING IN
  ITS TWIN.** Round 0 found the PR reproducing the defect class it was written to fix (two sentences
  asserting builder access is SUFFICIENT to generate, when a default `civitai login` cannot spend);
  round 1 found the same shape displaced onto `dev-tunnel`, where the unsubmitted-spend gate is
  **mods-only** so an authoring tester gets an app that renders and cannot generate; round 2 found two
  more; round 3 found a claim I had read off an adjacent **docstring's parenthetical** instead of the
  code (`ENV_KEYS` is `['VITE_LIVE_BLOCK_TOKEN','CIVITAI_HOST_KEY']` and PRESERVES the origins key).
  **The fix is the SWEEP, never the reported line.**
- 🔴 **I PUT `<slug>` IN A ```bash FENCE CARRYING `>>`** — an input redirect, so the binary never runs
  and the error names nothing in the doc. `CLAUDE.md` warns about exactly this. Promoting a command
  from prose-inline to a fence is what arms it.
- 🔴 **THE PROSE STOP-CRITERION IS A MEASUREMENT, AND IT INVERTED MY OWN NARRATIVE.** I called round
  2's findings "the ladder auditing itself"; the ladder-authored share of its pre-image lines measured
  **0.375**, so it was still converging on the PR's own content — and round 3 measured **0.714**, which
  is what actually ended the ladder. Compute it (`git diff -U0 -w -M` hunks → `git blame -w -M` at the
  round's `<from>` → `merge-base --is-ancestor <sha> <round-1 tip>`) rather than narrating it.
- **Two `audit-dispatch.py` refusals, both correct:** a `round=0` claims block (exit 4 — it would
  mis-anchor the next delta onto a tip that fixed nothing; record round 0 as PROSE), and passing a
  RANGE to `--audited` where it wants the single sha the round READ. And the payload reading **fails
  open cross-repo** — re-run the assembly from a checkout holding the commits to get it MEASURED.
- **`sops`/`gh` aside:** `gh pr update-branch <n>` defaults to a MERGE, which is what preserves an
  audit ladder's anchor shas; verify with `git log -1 --format='%p'` showing two parents.
- 🔴 **CARRIED FORWARD off a REPLACE heading so it survives: THE DOCS CHAIN IS RELEASE → SNAPSHOT →
  PROSE, NEVER RELEASE → PROSE.** `civitai-developer-docs`' `scripts/check-agent-setup.mjs` check 1
  requires every flag named in a `prompt.md` code block or command-shaped inline span to appear in
  `appblocks-snapshots/civitai-cli-help.txt`, and `cli-snapshot-refresh.yml` captures that file by
  `gh release download` of a **published release asset**, deliberately never from `main` — so the
  site cannot document a binary nobody can install. A snapshot-refresh PR therefore sits between any
  CLI release and any prose that names a new flag, and the guard is repo-local so it cannot be waited
  out. Also homed in the subsystem store at `civitai-developer-docs/guards`.

### Added 2026-10-04 (second session) — grading rank 38, and three instruments checked before one verdict

- 🔴 **A SHIPPED FLAG IS NOT A DELIVERED FLAG, AND THE TWO FAILED IN OPPOSITE DIRECTIONS HERE.**
  `--fix-path` was merged, published to npm, tapped in Homebrew, and documented in the generated CLI
  reference — four green shipping signals — and **changed nothing measurable**, because the agent it
  exists for is never told to run it and is told not to. The transferable test: for any remedy aimed
  at an agent, name the ONE document that agent reads and check the remedy is IN it. `AGENTS.md`
  looked like that document and is not: 0 of 6 agents opened the file they had just created.
- 🔴 **THE CONTRADICTION IS WORSE THAN THE OMISSION, AND ONLY A TRANSCRIPT SHOWS IT.** A missing
  mention would make the flag merely undiscovered; *"do not edit their shell profile yourself"*
  makes a discovering agent **refuse**. 1 of 6 found the flag via `--help` and declined in those
  words. A grid of verdicts cannot see this — the cell reads `no` either way. **Read the transcript
  of a `no` before attributing it to anything.**
- 🔴 **MY OWN FIRST READING OF THE REACHABILITY CONTROL WAS AN ARTEFACT OF MY SETUP, AND THE FIX
  WAS TO READ THE CODE, NOT TO RE-RUN.** `fix-path in agent-setup stdout = 0` looked like a missing
  runtime warning — but I had exported `PATH` before that step, so the CLI could legitimately have
  suppressed one. The discriminator was not another container: it was `agent_setup_path_note_test.go`,
  whose docstring says the delivery is *"deliberately UNCONDITIONAL"* prose and records that two
  detection drafts were each measured wrong. **A behavioural absence that a design decision already
  explains is not a finding.**
- ⚠ **THE README's GRADER-CONTROL ASSERTION IS STALE — it now under-reports the exempt set.** It
  pins `failed_checks=[authenticated]` and `agent_shell_version=0.1.105` for `ctl-pos`/`ctl-profile`;
  live on v0.1.112 both read `failed_checks=[authenticated,agent-token]`. A new `agent-token` check
  exists and is also verdict-exempt. The controls still pass on SHAPE; a reader asserting the
  documented literals would score three healthy controls as failures.
- **A `--max-cost` sanity note, third recurrence of the same lesson:** 6 cells cost **$0.036** total
  against a $6 cap. Key budget after the run: `$49.579` remaining of `$50`. The cheap models remain
  5–20× cheaper than a frontier model for this task and all three resolved live with `tools`.
- **Operational:** `runs/` lives inside whichever checkout you run from, so running the matrix in a
  dedicated worktree isolates it from the primary clone's transcripts for free — `runner.py` opens
  each transcript `"w"`, and this is the cheapest way to make that harmless.

### Added 2026-10-04 (third session) — a two-arm condition, and four self-inflicted lessons

- 🔴 **A CLOSING CONDITION WITH AN `either/or` CAN BE HALF-SATISFIED BY THE THING YOU ARE ABOUT
  TO DELETE.** This arc graded arm 1, reported `0 of 6`, and proposed removing a sentence that
  was arm 2's implementation, already working 6 of 6. The grid could not show it: `grade.sh`
  implements arm 1 only, so arm 2 appeared in no verdict line. **Read the condition's text before
  quoting a grader at it**, and report every arm or name the one you measured.
- 🔴 **"THIS REVERSES A DELIBERATE POSTURE" WAS REFUTED BY THE FILE I WAS EDITING.** Two
  pre-existing lines in the same document said the opposite. **Grep the artifact for the intent**
  before asserting a change reverses it — the removed sentence was an *exception* to the
  document's standing instruction, and the measured agent behaviour was agents resolving that
  contradiction.
- 🔴 **A STRUCTURAL FIX BEAT A PROSE FIX 5-OF-6 AGAINST 1-OF-3 ON THE SAME FILE.** Asking an
  agent to run a command gets it relayed to the user; putting the command in the fenced block
  gets it run. The house preference for deterministic over prompt-tuning, with a number.
- 🔴 **I PUBLISHED TWO FALSE AUDIT DISPOSITIONS AND A LATER ROUND CAUGHT THEM.** `D3=deleted`
  when the literal was still present byte-identically; `D4=kept: collapsed` when it had gone
  5→6, i.e. grown. Both were one `grep` away. **A disposition is a measurement, not a summary of
  intent** — run the count before writing the word.
- 🔴 **A SURVIVING MUTANT IS A CLAIM ABOUT THE MUTANT FIRST.** Removing only the leading half of
  a two-line trim left the trailing half, which still emptied a blank value — the guard looked
  uncovered and was not. Conversely two of my own guards *did* survive a green run because the
  reachability preflight killed their tests instead: **a different guard's error passing your
  test is indistinguishable from coverage**, and the fix is asserting the guard's own message,
  never a non-zero exit.
- ⚠ **`dispositions=` is COMMA-separated; a comma inside a reason breaks it and semicolons are
  not a separator.** Three postings before it parsed. Never a blocker (`4 recorded, 0 unrecorded`
  throughout) but the `UNREADABLE` list accumulates every attempt — get it right once.
- ⚠ **The attribution gate cannot measure payload for a CROSS-REPO ladder:** `git log
  --remerge-diff <range>` exits 128 because the assembly checkout is another repository, so every
  `payload=` falls back to the author's own classification. Structural. Say so when quoting one.
- **A 0 ms connection failure to a container you just started is a RACE, not a block.** `docker
  run -d` returns before the server binds, and that failure reads exactly like a firewall drop —
  it cost one false "inter-container networking is blocked" diagnosis.

### Added 2026-10-04 (fourth session) — arc closed; the lessons are in the STORE, not here

🔴 **A pointer, not a narrative — deliberately.** This doc is 147 KB against a 65 KB ceiling, so a
CLOSED arc's lessons belong where they are recalled on demand. Both entries were written this
session, and the two earlier `OPEN:` bullets rewritten `RESOLVED efa1d6c2:` / `RESOLVED 021ec58f:`
in the same turn: `$DEVRC/scripts/cairn-ops/read.sh recall --repo /home/zach/workspace/civit/cli
--ref dogfood` (and `--ref agent-setup`).

- 🔴 **A delivery fix can be worth more than the feature, and the ratio was measured** — merged,
  published, tapped, documented, and **0 of 6**; one prose edit took it to **5 of 6**. Prose
  *asking* an agent to run a command scored 1 of 3; the command **in the fenced block**, 5 of 6.
- 🔴 **Calling a guard structural does not make it structural**, and **two of my own guards passed
  for the wrong reason** — another guard's error killed their tests.
- 🔴 **An `either/or` closing condition can be half-satisfied by the sentence you are deleting.**
- 🔴 **I published two false audit dispositions.** A disposition is a measurement, not intent.

## How to verify

```bash
# 1. #665 closed, and read BOTH arms — it is an either/or
gh issue view 665 --repo civitai/cli --json state,stateReason --jq '"\(.state)/\(.stateReason)"'
gh issue view 665 --repo civitai/cli --json body --jq .body | sed -n '/## Closing condition/,+6p'

# 2. The docs change is SERVING (branch-tracked => merge is deploy, but not instant)
curl -sS -A 'Mozilla/5.0' https://developer.civitai.com/agent-setup/prompt.md > /tmp/p.md
grep -c 'agent-setup --fix-path' /tmp/p.md            # 1
grep -c 'do not edit their shell profile' /tmp/p.md   # 0

# 3. The cli merge, BY CONTENT with a negative control (a squash makes ancestry false)
C=/home/zach/workspace/civit/cli
git -C "$C" show origin/main:scripts/dogfood/driver.sh   | grep -c 'pf_auth'   # 2
git -C "$C" show origin/main~2:scripts/dogfood/driver.sh | grep -c 'pf_auth'   # 0

# 4. The closing evidence and its provenance control — the 0 is the load-bearing half
R=~/.cache/dogfood-runs-2026-10-04-live/runs
grep -l '"prompt_url"' "$R"/live-*/transcript.jsonl | wc -l   # 0 — none fed a patched prompt
grep -o '"stop": "[a-z-]*"' "$R"/live-*/transcript.jsonl | sort | uniq -c   # 6 x finished
```
## Defects (batched)

- `cli#787` — the test stub answers `--print-task` regardless of script path, so the default-URL
  **derivation** is untestable; and the bash trim misses U+00A0. Both measured and bounded; the
  issue carries the evidence and a mechanical closing condition.
