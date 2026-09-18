# Blind dogfood matrix — `developer.civitai.com/agent-setup/prompt.md`, 2026-09-18

19 blind trials: 4 models × 4 environments, plus 3 de-confounding controls. Each
trial is one model driving one throwaway Docker container whose only contents are
an OS, node/npm, zsh and curl. The harness is `scripts/dogfood/`.

**Headline: 4 of 16 matrix trials reached the arc's frozen closing condition, and
the 12 failures are fully explained by exactly TWO defects — neither of which is
model-dependent.** Every model read the prompt correctly and followed it. The
prompt is accurate and cheap; it is not sufficient on two axes.

## Method, and what makes it blind

- The model's ONLY tool is `bash`, and that bash runs `docker exec` into the trial
  container. The repo, this file, `AGENTS.md` and the operator's home directory
  are not in that container's filesystem, so blindness is a **mount namespace,
  not an instruction** — the weakness every previous dogfood harness had.
- The model is given the bare URL as the user message and nothing else. The system
  prompt says "you are a coding agent with a shell", and says nothing about
  Civitai. It fetches `prompt.md` itself with `curl`.
- No Civitai credential exists anywhere in the trial. Nothing can spend.
- 🔴 **`curl` is the BEST CASE for the fetch, deliberately.** A real agent's
  WebFetch summarises through a small model and drops content (measured earlier in
  this arc). Giving every trial the true bytes isolates *prose* defects from
  *fetch-lossiness* defects. Fetch lossiness is therefore **not covered here.**

### Instrument validation — run before any verdict was read

| control | expectation | measured |
|---|---|---|
| untouched container | grader must go RED | `CLOSING_CONDITION=no`, `check_ok=parse-error` |
| hand-built success (install + `agent-setup` by hand) | grader must go GREEN | `CLOSING_CONDITION=yes`, `check_ok=true` |

Both were re-run after every change to the grader. Two grader defects were found
and fixed by these controls, and both would have produced confident wrong verdicts:

1. 🔴 **`--check --json` prints its error line to STDERR *ahead of* the JSON**, so
   a `2>&1` capture is unparseable — and `jq` failing then reads as "the command
   emitted no JSON at all", which is indistinguishable from "the CLI is not
   installed". The grader reads **stdout only**.
2. 🔴 **`jq '.ok // "absent"'` cannot see `ok: false`** — jq's `//` treats `false`
   as empty, exactly like `null`. A real failing setup was reported as `absent`,
   i.e. the instrument could not distinguish a failed setup from a missing CLI.
   Use `if has("ok") then (.ok|tostring) else "absent" end`.

## The grid

Agent identity is set with the environment variable the real agent exports, since
`civitai agent-setup` branches hard on it. `other` = no signal, which is what
every agent outside the CLI's table gets (Gemini CLI, Aider, Cline, Continue, …).

| model | env | agent | prefix writable | closing condition | mechanism |
|---|---|---|---|---|---|
| claude-sonnet-5 | node-root | claude | yes | ✅ **yes** | — |
| claude-sonnet-5 | stale-cli 0.1.101 | claude | yes | ✅ **yes** | upgraded to 0.1.105 |
| gpt-5.6-terra | node-root | codex | yes | ✅ **yes** | — |
| gpt-5.6-terra | stale-cli 0.1.101 | codex | yes | ✅ **yes** | upgraded to 0.1.105 |
| gemini-3.8-flash | node-root | other | yes | ❌ no | **A** |
| gemini-3.8-flash | stale-cli | other | yes | ❌ no | **A** |
| grok-4.6 | node-root | other | yes | ❌ no | **A** |
| grok-4.6 | stale-cli | other | yes | ❌ no | **A** |
| all 4 models | node-user | — | **no** | ❌ no (4/4) | **B** |
| all 4 models | ubuntu-apt | — | **no** | ❌ no (4/4) | **B** |

### The de-confounding control — model vs agent identity

The grid above confounds the two: `claude`/`gpt` carried a known agent identity
and `gemini`/`grok` did not. Three extra trials swap them, on the identical
environment:

| trial | identity | closing condition |
|---|---|---|
| claude-sonnet-5, node-root | **other** | ❌ no — mechanism **A** |
| gemini-3.8-flash, node-root | **claude** | ✅ **yes** |
| grok-4.6, node-root | **claude** | ✅ **yes** |

**The outcome inverts with the identity and not with the model.** Across all 19
trials the verdict is a pure function of (agent identity ∈ CLI table?, npm prefix
writable?). **No model-dependent behaviour was observed at all** — and that is the
most useful single result here, because it means both defects are fixable in the
product rather than papered over with prose aimed at weaker models.

## Defect A — an agent the CLI does not know can never reach `ok: true`

`internal/cmd/agent_setup.go`, `checkCountsTowardVerdict`.

For `agent == other`, `agentSetupMCPChecks` returns `mcp-site` / `mcp-orch` with
`OK: false` and the detail *"agent other has no config file this CLI knows —
register … by hand"*. Both rows count toward the verdict, so:

```
$ civitai agent-setup --check --json      # after a completely correct setup
{"agent":"other","ok":false, …}           # exit 1, forever
Error: agent setup incomplete: 2 check(s) failed — … re-run `civitai agent-setup` to fix what it can write
```

Three consequences, all observed:

1. **`prompt.md` step 4 says "Do not report success if any check fails."** So a
   correctly-completed setup instructs the agent to report a failure. 4 of 4 such
   trials hedged in their final report; the setup was in fact complete.
2. **The remediation text cannot work.** "re-run `civitai agent-setup` to fix what
   it can write" — on `other` there is nothing it can write, so the advice is an
   infinite loop.
3. **The arc's frozen closing condition is unreachable on this path**, because it
   requires `ok: true`.

🔴 **This is the THIRD instance of a shape the file's own comments have already
recognised twice.** `authenticated` is excluded from the verdict because *"a
fresh, correct, unauthenticated setup is a SUCCESS"*; `claude-md` is excluded for
an agent that does not read it because otherwise *"the hosted prompt reports that
as a failed setup"*. The `other` case is the same sentence a third time, and the
exclusion list does not cover it:

```go
func checkCountsTowardVerdict(name, agent string) bool {
	switch name {
	case checkAuthenticated: return false
	case checkClaudeMD:      return agent == agentClaude
	default:                 return true          // ← mcp-site / mcp-orch, always
	}
}
```

### A2 — the escape hatch exists and the entrypoint never mentions it

`prompt.md` contains the string `--agent` **zero** times. The CLI's own `--help`
documents it, and it works immediately:

```
$ civitai agent-setup --track app --agent cursor && civitai agent-setup --check --json
{"ok":true,"agent":"cursor"}
```

The one trial that recovered gracefully — claude-sonnet-5 forced onto `other` —
did so by **asking the user** which editor they use, which is the one thing the
prompt tells the agent not to do ("Do not ask the user to run any of these
commands"; it works autonomously otherwise). It could not offer `--agent` because
nothing it was given mentions it.

🔴 **But `--agent` is NOT a blanket fix, and the prompt would have to say which
case it is for.** Two situations are being conflated:

| situation | right action | `ok: true` reachable? |
|---|---|---|
| detection FAILED for an agent that IS in the table (no env signal, e.g. the CLI run from a plain terminal) | `--agent <name>` | yes |
| the agent is genuinely NOT in the table (Gemini CLI, Aider, Cline, …) | paste by hand | **no — defect A** |

Forcing `--agent cursor` in the second case writes a Cursor config that the
running agent will never read, and `--check` then reports a green setup that does
not work. So the fix for A2 is a *conditional* instruction, and it does not remove
the need to fix A.

**Recommended fix (structural, matching the existing precedent):** the two MCP
rows must not count when `agentTargets[agent]` is unknown — the rows STAY, because
they are true and the user does need to paste, exactly as `authenticated` stays.
The alternative — teaching `prompt.md` to distinguish "a check that failed" from
"a check that reports work only you can do" — pushes a product invariant into
prose, and prose is the channel this arc has repeatedly measured to be lossy.

## Defect B — the documented `--prefix` remedy produces a setup that does not survive the shell

`prompt.md` §2 "If the install fails" tells the agent, on `EACCES`/`ENOENT`:

```bash
npm install -g --prefix="$HOME/.npm-global" @civitai/cli
PATH="$HOME/.npm-global/bin:$PATH"
```

…and correctly adds *"That `PATH` line lasts only for this shell. Report it
verbatim in step 5 … do not edit their shell profile yourself."*

**All 8 trials on a non-writable prefix followed this correctly, and all 8 ended
with `civitai` unreachable from every shell but the one that installed it.**
Measured in the finished containers:

```
$ command -v civitai            → (nothing)      # a fresh non-login bash
$ zsh -lic 'civitai --version'  → command not found
$ ls ~/.npm-global/bin          → civitai        # it is right there
```

What the agents actually reported, counted by hand after a keyword regex
**overstated this and had to be retracted** (see the correction note below):

| | count | detail |
|---|---|---|
| relayed the PATH line, as step 5 requires | **8 / 8** | the prompt's instruction is obeyed |
| *also* warned it is not persistent and named the profile file | **6 / 8** | e.g. *"add it to `~/.bashrc` or `~/.zshrc` to make it permanent"* |
| gave no persistence warning at all | **2 / 8** | both gpt-5.6-terra |
| opened by declaring the setup complete/successful | 4 / 8 | claude ×2, gemini-ubuntu, gpt-ubuntu |
| stated that a LATER shell — including a later **agent** session reading the `AGENTS.md` just written — would not find `civitai` | **0 / 8** | nobody connected the two |

🔴 **So the defect is NARROWER than "agents falsely report success", and saying it
that way would have been wrong.** The prompt's prose works; most agents relay a
usable manual remedy. What no agent noticed, and what no step checks, is that
**step 3's durable artifact is written against a binary that only exists in the
installing shell**: `AGENTS.md` tells every future agent session to run `civitai
…`, and every future session gets a new shell. The gap is closed only if the
human performs the profile edit before anything else runs.

Step 4 cannot catch it because its `civitai --version` runs in the *same* shell as
the install — the one shell in which it works.

> ⚠ **Correction, recorded because the instrument was mine.** The first version of
> this section said *"5 of 8 still declared the setup a success"*, from a regex
> matching `success|complete|all set|done`. That regex scores *"Setup is
> complete"* and *"all steps succeeded except authentication"* identically to a
> report that merely lists what it did, and it counted a persistence WARNING as
> nothing at all. Read by hand, 6 of 8 warned properly. The number was quoted in a
> commit message before it was checked.

The second-order cost is the one that matters: step 3 writes an `AGENTS.md` that
tells every future agent session to run `civitai …`. Those sessions get a new
shell, so **the file the setup exists to produce instructs the agent to run a
binary the setup left unreachable.**

🔴 **This is "success measured in an environment the user does not have" — the
lesson already recorded in this arc — recurring on a NEW operand.** The recorded
instance was a *stale* binary and was fixed with `civitai upgrade`; this one is an
*unreachable* binary produced by the prompt's own remedy, and no step checks for
it. A fix aimed at the earlier operand did not generalise, which is the reusable
part.

**Candidate fixes, none yet chosen:**
- **(a) Verify in a NEW shell.** Step 4 runs `bash -lc 'civitai --version'` (or
  `zsh -lic`) rather than a bare `civitai --version`. This does not fix the
  install; it makes the failure *visible* instead of silent, which is the minimum.
- **(b) Prefer a prefix that is already on PATH.** `npm config set prefix` writes
  npm's own config, not a shell profile — but it still does not put `bin` on PATH,
  so it is not sufficient alone.
- **(c) Record the absolute path in `AGENTS.md`** when the CLI was reached through
  a non-PATH prefix, so future sessions work regardless.
- (a) is the smallest honest change and is independent of the others.

## Token efficiency — the third question the trials were run to answer

`prompt.md` is 7,187 bytes (~1.8k tokens) and is fetched once.

| model | trials | steps (tool calls) | cumulative prompt tok | cost/trial |
|---|---|---|---|---|
| claude-sonnet-5 | 4 | 7–8 | 27.8k–36.6k | $0.076–0.100 |
| gpt-5.6-terra | 4 | **3**–6 | 7.6k–14.2k | $0.017–0.023 |
| gemini-3.8-flash | 4 | 7–8 | 18.6k–20.2k | $0.018–0.021 |
| grok-4.6 | 4 | 5–6 | 14.5k–17.2k | $0.021–0.023 |

**Verdict: token-efficient, and no model needed a second document.** Nothing
fetched `llms.txt`, the `/apps` reference, or any other page — the ~103k-token doc
surface was never touched, which is the outcome the one-level-of-indirection
design was aiming for. The whole 19-trial matrix cost **$0.66**.

The fastest complete run was GPT in **3** tool calls (fetch, probe, then one
chained install+setup+verify), and it passed. There is no evidence the prompt is
too long for any current model.

## What this run does NOT cover

Stated so an absence is not read as a clean bill of health.

1. **Lossy fetch.** Every trial used `curl`, i.e. the true bytes. The measured
   WebFetch-summarisation failure mode is untested here.
2. **The `authenticated` half.** No credential existed in any trial; the login
   path and the post-login `agent-setup` re-run are unexercised.
3. **macOS / Homebrew.** Linux containers only, so §2's brew branch and the
   "cask is macOS-only" caveat were never executed.
4. **Windows.**
5. **Agents with a config file but an unusual root** (`CODEX_HOME`, Zed on
   Windows) — the identity was set by env var, but every container had a normal
   `$HOME`.
6. **A pre-existing `CLAUDE.md` / `AGENTS.md` / populated MCP config.** Every
   project directory was empty, so the merge paths are untested.
7. **Only 4 of the 7 agents in the CLI's table** were exercised as identities
   (claude, codex, and `other` twice over). `cursor`, `opencode`, `vscode`,
   `windsurf` and `zed` were not.
