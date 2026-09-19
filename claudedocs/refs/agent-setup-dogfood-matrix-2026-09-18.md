# Blind dogfood matrix — `developer.civitai.com/agent-setup/prompt.md`, 2026-09-18

19 blind trials: 4 models × 4 environments, plus 3 de-confounding controls. Each
trial is one model driving one throwaway Docker container whose only contents are
an OS, node/npm, zsh and curl. The harness is `scripts/dogfood/`.

**Headline: 4 of 16 matrix trials reached the arc's frozen closing condition, and
the 12 failures are fully explained by exactly TWO causes — the VERDICT being
model-independent in both.** ⚠ Only cause **B** is a defect; **A** is a contested
design call (a round-0 audit refuted this document's first argument about it — see
the retraction in §A). Every model read the prompt correctly and followed it. The
prompt is accurate and cheap; it is not sufficient on two axes.

## 🔴 THE GRID BELOW WAS PRODUCED BY AN EARLIER DRIVER THAN THE ONE COMMITTED

Read this before re-running anything. The 16-cell grid and the 3 controls were run
by a `driver.sh` that **bound agent identity to the model row** — `claude`/`gpt`
got a known identity, `gemini`/`grok` got none. That is the confound this document
then identifies as its own central methodological error, and the committed
`scripts/dogfood/driver.sh` no longer has it: identity is a crossed axis there.

**So the committed instrument does not reproduce this grid, and it should not.**
Re-running it yields a different and better-shaped matrix — 4 models × 4 envs ×
identity, with all three identities (`claude`, `codex`, `other`) crossed on the
cheapest env by default. Expect a
different headline number from the same product behaviour, because more cells run
under a known identity: mechanism **A** below is keyed to identity, so it fires in
fewer cells.

Two consequences, stated because a reader will otherwise treat a different number
as a regression:

- **The `4 of 16` headline is a property of THIS run's cell selection**, not a
  score for the entrypoint. What is invariant is the *rule* the cells obey, and
  that is what the de-confounding control establishes.
- **The per-trial figures here are not re-derivable from the repo.** Transcripts
  are gitignored (they are tens of thousands of lines of raw model output), and the
  committed driver runs a different cell set. Every count, cost and step figure
  below is therefore a **recorded measurement, not a reproducible one**. Treat them
  as evidence about what happened on 2026-09-18, and re-measure rather than re-cite
  if a decision turns on them.
- ⚠ **The cell selection is not the ONLY difference between this run and a re-run.**
  The committed runner also applies container resource limits (`--pids-limit 512`,
  `--memory 2g`, `--cpus 2`) that the trials below ran WITHOUT. Nothing observed
  here suggests a trial came close to any of them — the work is an `npm install` and
  a few short commands — but it is a second uncontrolled difference, and attributing
  a changed result to identity alone would be assuming what has not been checked.

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

Both were re-run after every change to the grader. **THREE grader defects have been
found, each of which would have produced a confident wrong verdict — but only the
first two were found BY the controls above.** The third was found by an audit round,
and the control that pins it (`ctl-profile`) lives in `scripts/dogfood/README.md` and
is deliberately NOT in the table above, because it postdates it. ⚠ **Running only the
two tabulated controls does not cover defect 3** — arm B's shell is unguarded on that
path. Validate from the README's control list, not from this table.

1. 🔴 **`--check --json` prints its error line to STDERR *ahead of* the JSON**, so
   a `2>&1` capture is unparseable — and `jq` failing then reads as "the command
   emitted no JSON at all", which is indistinguishable from "the CLI is not
   installed". The grader reads **stdout only**.
2. 🔴 **`jq '.ok // "absent"'` cannot see `ok: false`** — jq's `//` treats `false`
   as empty, exactly like `null`. A real failing setup was reported as `absent`,
   i.e. the instrument could not distinguish a failed setup from a missing CLI.
   Use `if has("ok") then (.ok|tostring) else "absent" end`.
3. 🔴 **A FALSE GREEN, found on audit round 4 — the grader was measuring a
   different shell than the condition names.** Arm B ran `bash -lc "zsh -lic '…'"`;
   the outer bash login sources `~/.profile`, whose stock
   `if [ -d "$HOME/.local/bin" ]` block prepends a directory **zsh never reads**.
   So a CLI installed under `$HOME/.local` satisfied the wrapper and not the shell
   the frozen condition names. Measured in one container: wrapped → `0.1.105`,
   `CLOSING_CONDITION=yes`; direct `zsh -lic` → `command not found`,
   `CLOSING_CONDITION=no`. Arm B now runs zsh directly.
   ⚠ **The grid below is UNAFFECTED — by complete enumeration, not sampling.** All
   19 surviving trial containers were re-graded under both arm-B shapes: **19/19
   identical**, and `~/.local/bin` is absent in all 19. 🔴 **The invariant is "no
   trial installed under a prefix that ONLY `~/.profile` adds" — i.e. `~/.local/bin`
   or `~/bin`** — not "every trial used `$HOME/.npm-global`", which an earlier draft
   said and which is false for 11 of the 19: the root trials install to
   `/usr/local/bin` and have no `~/.npm-global` at all. The defect was latent.

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
| claude / gpt | node-user | claude / codex | **no** | ❌ no (2/2) | **B** |
| gemini / grok | node-user | other | **no** | ❌ no (2/2) | **B** |
| claude / gpt | ubuntu-apt | claude / codex | **no** | ❌ no (2/2) | **B** |
| gemini / grok | ubuntu-apt | other | **no** | ❌ no (2/2) | **B** |

⚠ **The last four rows were written as two rows reading `agent: —`**, i.e. the
identity was left unrecorded for 8 of the 16 cells — on the axis this document's
own de-confounding section proves is one of the two determinants. The identities
above are recovered from the run's model→identity binding, which is exactly the
confound; they are correct for this run but they are a *derivation*, not an
independent record. **Mechanism B is reported for all 8 on the strength of the
container measurement** (`civitai` absent from every fresh shell), which does not
depend on identity — but the document cannot independently demonstrate that A was
not also firing in the four `other` cells, because B alone is sufficient to
produce `CLOSING_CONDITION=no` and the grader stops there.

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
writable?). **The VERDICT is model-independent** — and that is the most useful
single result here: it means each cause can be addressed at its source rather than
papered over with prose aimed at weaker models. ⚠ Not "both defects are fixable in
the product" — an earlier draft said that, and §A's own retraction concludes the
opposite for cause A, whose likelier fix is prompt-side and whose owner is whoever
holds the `--json` contract.

🔴 **State that at the width it was measured, not wider. "No model-dependent
behaviour at all" is FALSE, and an earlier draft of this file said it.** Two
model-dependent differences were observed, both in *reporting* rather than in the
machine state:

- **Steps to completion ranged 3–8.** gpt-5.6-terra chained install+setup+verify
  into a single command; claude-sonnet-5 took 7–8 discrete steps everywhere.
- **The persistence warning is model-dependent.** 6 of 8 agents on the `--prefix`
  path told the user the PATH change would not survive the shell and named the
  profile file; the 2 that did not were **both gpt-5.6-terra** — i.e. the model
  with the fewest steps. A small sample, and the direction is worth a second look
  before anyone leans on it.

So: *what the machine ends up in* did not depend on the model across 19 trials.
*What the user is told about it* did.

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

🔴 **RETRACTED — "this is the THIRD instance of a shape the file's own comments
have already recognised twice", and the exclusion fix that rested on it. A
round-0 audit refuted both; the retraction is kept in place rather than deleted
because the argument was persuasive and someone will re-derive it.**

Why it fails. The two existing exclusions are exclusions because **no work
remains**: `authenticated` is out of scope by design (*"a fresh, correct,
unauthenticated setup is a SUCCESS"*), and `claude-md` is **inert** for an agent
that does not read it. On `other`, work remains and has not been done — the MCP
servers genuinely are unregistered and the user genuinely must paste. Not the
same sentence.

🔴 **And the code already decided this case, explicitly, in the docstring of the
function that emits the rows — `internal/cmd/agent_setup.go:739-742`:**

> *"An agent this CLI has no target for, and a user-scoped target with no
> resolvable home directory, **are both genuinely unfinished setups** — but a row
> reading "not registered in " with an empty path is an answer with none of the
> content."*

The first draft of this section did not quote that comment. It read the
`checkCountsTowardVerdict` switch as an omission when a sibling function records
the decision in words.

🔴 **Worse, the recommended fix contradicted this document's own reasoning ~40
lines below.** §A2 rejects forcing `--agent cursor` on the not-in-table path
*because "`--check` then reports a green setup that does not work"* — and
excluding the rows from the verdict produces exactly that outcome by a different
route. The same consequence was disqualifying in one paragraph and recommended in
another.

**What is left standing, below, is the part that needs no verdict change at all.**
For reference, the switch as it is today:

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

### What is uncontested, and what is a design call

**Uncontested, and fixable without touching verdict semantics:**

1. The remediation string *"re-run `civitai agent-setup` to fix what it can
   write"* names an action that can never change the outcome on this path. It
   should say what is actually left to do: paste the printed config.
2. `prompt.md` mentions `--agent` **zero** times (§A2), so an agent landing on
   `other` has no documented recovery.

**The design call, which this document does NOT get to make:** `prompt.md` step 4
says *"Do not report success if any check fails"*, and on this path a check fails
permanently by design. Those cannot both stand. Either the prompt learns to
distinguish "a check that failed" from "a check that reports work only YOU can
do", or the verdict does. 🔴 **The first draft of this file picked the second and
argued it from a precedent that does not exist** (see the retraction above); the
first is now the likelier answer, because it is the one the code's own docstring
is consistent with — but the `--json` contract has ledgered consumers
(`readme_agent_setup_claims_test.go`), so whoever owns that contract decides, not
this report.

⚠ **Consequence for the arc's closing condition:** it requires `ok: true`, so it
is unreachable on this path. That is a fact about the CONDITION. It must not be
resolved by quietly widening the condition until it passes.

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

**Candidate fixes. 🔴 THE RANKING IS ABANDONED — three drafts of it, here and in
the handoff, were each wrong. What follows is a measurement; the enumeration is
open.**

- **(a) Verify in a NEW shell.** Step 4 runs `bash -lc 'civitai --version'` (or
  `zsh -lic`) rather than a bare `civitai --version`. Does not fix the install; it
  makes the failure *visible* instead of silent.
- **(b) Install into a prefix already on PATH.** ⚠ Earlier drafts of this bullet
  described (b) AS `npm config set prefix`, which is a different action — pointing
  npm at a *new* prefix, which indeed does not put `bin` on PATH. Stated properly,
  (b) means a prefix whose `bin` is *already* on PATH.
- **(c) Record the absolute path in `AGENTS.md`** when the CLI was reached through
  a non-PATH prefix, so future agent sessions work regardless.

🔴 **MEASURED, in both environments this defect's closing condition is graded on**
(`df-node-user`, `df-ubuntu-apt`, as the container's own non-root user):

```
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin   # all six: writable=no
~/.local/bin: does not exist      ~/bin: does not exist
```

❌ **An earlier draft concluded from this that "(b) has no implementable target"
and that "arm 1 is unreachable on the graded environments by any listed
candidate". BOTH ARE WITHDRAWN — the measurement above records a precondition the
remedy itself alters.** Both images' stock `~/.profile` carries a block conditional
on EXISTENCE:

```sh
if [ -d "$HOME/.local/bin" ] ; then PATH="$HOME/.local/bin:$PATH" ; fi
```

So `npm install -g --prefix="$HOME/.local"` creates that directory and puts the CLI
on a **bash** login shell's PATH with no profile edit — measured in both images:
`civitai 0.1.105`. 🔴 **zsh does not read `~/.profile`** (it reads `.zshenv`,
`.zprofile`, `.zshrc`), and the closing condition names `zsh -lic`. Same container,
same install: `zsh -lic 'civitai --version'` → **`command not found`**.

**The two shells disagree, and no reachability claim is made here.** That
disagreement is itself the finding: it means the remedy's success depends on which
login shell the condition means, which nobody has decided.

⚠ **FOUR surfaces describe these same three remedies**, and they disagreed for two
rounds in opposite directions: this list, the handoff's **ranked item 34**, the
handoff's **Defect-B investigation block**, and public issue **cli#665**. A sweep
note that named only three is how the fourth copy survived four audit rounds. Rank
34 is the one place they are maintained; the others point at it.

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
7. **Only 2 of the 7 agents in the CLI's table** were exercised as identities
   (⚠ this said *"4 of 7"* and was wrong by 2×, in the one section whose job is to
   not overstate coverage — the same sentence names 5 as unexercised, and 7−5=2.
   `other` is not a table entry: `agentTargets` has no row for it, which is the
   entire mechanism of defect A.) Exercised: **claude** and **codex**, plus the
   out-of-table `other` case. `cursor`, `opencode`, `vscode`, `windsurf` and `zed`
   were not. ⚠ All five have identical coverage — zero — so none is "least
   covered"; an earlier draft said `windsurf` was, on the grounds that it spells
   the URL key `serverUrl`. That is a different KEY, not a different write path
   (it is `formatJSON`, the same routine as claude and cursor). ⚠ A second draft
   then prioritised `zed` for its JSONC config — but `AllowsComments` is set on
   **vscode, opencode AND zed**, so that property does not discriminate either.
   **No ranking is offered.** If one is wanted, `opencode` has the structurally
   distinct row — a `PreferExisting` target-selection branch no other agent has,
   whose absence once wrote the servers into a second file.
