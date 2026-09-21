# `scripts/dogfood` — blind dogfood harness for the agent-setup entrypoint

Answers one question, repeatably: **can a real coding agent, given only
`https://developer.civitai.com/agent-setup/prompt.md`, reach a working setup on a
machine that did not build the CLI?**

One trial = one model driving one throwaway container. The model's only tool is a
`bash` that runs inside that container, and the container holds an OS, node/npm,
zsh and curl — nothing else. **Blindness is a mount namespace, not an
instruction:** this repo, its `AGENTS.md` and the operator's home directory are
not reachable from inside, so an agent cannot read the source even by accident.

By default no Civitai credential is involved at any point, so no trial can spend
Buzz or touch a real account. The only credential is then the operator's
OpenRouter key, read from `OPENROUTER_API_KEY` and never written anywhere.
`--credential-file` opts one trial into a **credentialed run** — see
[Credentialed trials](#credentialed-trials-and-the-run-caps) for what that
changes, what it bounds, and what it does not.

### 🔴 What is isolated, and what is NOT

Read this before running it on your own workstation — you are handing a language
model an unsandboxed root shell in half of these images.

| | bounded? | by what |
|---|---|---|
| filesystem — repo, `$HOME`, credentials | **yes** | mount namespace; the container has none of them |
| Civitai account / Buzz | **yes, unless `--credential-file`** | no Civitai credential exists in a default trial; with one, see the caps below |
| processes | yes | `--pids-limit 512` |
| memory / CPU | yes | `--memory 2g --cpus 2` |
| money | yes | `--max-cost` (default $1/trial) — a step cap is not a spend cap |
| **network** | **NO** | egress is open and must be: the trial has to fetch `prompt.md` and reach npm |
| **disk** | **NO** | see below |

**The network line is the one that matters.** A model-authored command runs with
reachability to your LAN, to anything bound on a routable address, and to the
internet. The container bounds what a trial can *read of yours*; it does not bound
what it can *reach*. If that is not acceptable, run this on an isolated host.

**Disk is unbounded, deliberately.** `--storage-opt size=` was tried and removed:
Docker accepts it *"only for overlay over xfs with `pquota`"*, so on an ordinary
daemon it does not cap the write — it refuses to start the container, turning
every trial into a failed one. A bound that breaks the harness on most hosts is
worse than a declared gap. A runaway `npm install` can fill the host's Docker
storage; watch `docker system df` on a long matrix.

⚠ A `subprocess` timeout kills the local `docker exec` client, not the process it
started inside the container — which is why the resource limits above exist rather
than relying on the timeout.

## Run it

```bash
export OPENROUTER_API_KEY=sk-or-...
cd scripts/dogfood
for e in node-root node-user ubuntu-apt stale-cli; do
  docker build -q -t "df-$e" -f "envs/$e.Dockerfile" .
done
bash driver.sh                                  # the matrix, 4 trials at a time
bash grade.sh t-claude-noderoot-claudeid root   # grade one finished trial

# a cheap smoke run — one model, one env, all three identities (3 trials):
DOGFOOD_MODELS='google/gemini-3.8-flash|gemini' DOGFOOD_ENVS='df-node-root|noderoot|root' bash driver.sh
```

## Two kinds of trial: setup, and app-build

By default a trial's whole task is the hosted URL — *can an agent reach a
working setup?* Give it a **brief** and the task becomes the URL plus one
operator-typed line, and the question becomes *can an agent BUILD something?*

```bash
# preview the exact task. Starts no container, calls no API, spends nothing.
python3 runner.py --print-task --brief "$(cat briefs/celsius.brief.txt)"

# one app-build trial
python3 runner.py --model "$MODEL" --image df-node-root --trial ab-01 \
  --brief "$(cat briefs/celsius.brief.txt)"

# the whole matrix as app-build trials, in their own trial-id namespace
DOGFOOD_BRIEF="$(cat briefs/celsius.brief.txt)" DOGFOOD_TRIAL_PREFIX=ta bash driver.sh
```

- **With no brief the task is BYTE-IDENTICAL to what it has always been** — not
  "equivalent". An app-build trial is a setup trial plus one appended
  paragraph, so every setup grid already measured stays comparable.
  `--print-task` exists so that can be diffed rather than asserted.
- **The brief is appended raw**, with no framing sentence of the harness's own.
  Framing would be Civitai knowledge injected by the rig, and the trial's
  premise is that the only such knowledge is the URL and what the operator
  typed. Blindness is a mount namespace; this keeps the message channel honest
  about the same thing.
- **One line, enforced.** Both `runner.py --brief` and `DOGFOOD_BRIEF` refuse a
  value containing a newline: a multi-line brief is nearly always a file that
  got splatted onto the command line, which is the one route by which repo
  content could reach a blind trial.
- **`brief` is written into the transcript's `start` record, always** — empty
  string included. A graded cell has to be traceable to the brief it was run
  with, and the trial id cannot carry that (same reason `grade.sh` reads the
  agent identity out of the container rather than out of the filename).
- 🔴 **`driver.sh` REFUSES to run a brief under the default `t-` prefix.** Trial
  ids are `<prefix>-<model>-<env>-<identity>` and the resume guard skips any id
  whose transcript already has an `end` record — so an app matrix in the setup
  matrix's namespace would skip every cell, each "complete" from a *different
  task*, while printing `MATRIX COMPLETE`. Set `DOGFOOD_TRIAL_PREFIX`.

## Credentialed trials, and the run caps

A default trial has no Civitai credential, so `civitai generate` — the CLI's only
irreversibly money-spending surface — and every `app` command that reaches the
account are **structurally unreachable**. `--credential-file` opts one trial into
reaching them.

```bash
python3 runner.py --model "$MODEL" --image df-node-root --trial c-01 \
  --brief "$(cat briefs/genpost.brief.txt)" \
  --credential-file ~/.config/civitai/config.yaml \
  --app-prefix dogfood4- --max-generations 3 --max-submissions 1

# the whole matrix, credentialed (driver.sh refuses without DOGFOOD_APP_PREFIX)
DOGFOOD_CREDENTIAL_FILE=~/.config/civitai/config.yaml \
DOGFOOD_APP_PREFIX=dogfood4- DOGFOOD_MAX_GENERATIONS=3 DOGFOOD_MAX_SUBMISSIONS=1 \
DOGFOOD_BRIEF="$(cat briefs/genpost.brief.txt)" DOGFOOD_TRIAL_PREFIX=tc bash driver.sh
```

### 🔴 The secret does not reach the artifacts

The flag takes a **path**, never a value, and the file is `docker cp`'d in and
installed by a fixed script. That keeps the credential off the three surfaces a
run leaves behind:

| surface | what keeps it off |
|---|---|
| **process argv** (`ps`) | only the path is ever an argument. `docker run -e CIVITAI_TOKEN=…` would put the value in argv for every process on the host; `TestDogfoodCredentialIsInjectedByPathNotByValue` fails if anyone reintroduces it |
| **`transcript.jsonl` / `commands.log` / stdout** | every value written goes through a redactor keyed on the credential's own strings, so even a trial in which the model `cat`s the config file records `[REDACTED:<sha8>]` |
| **the container's shell history** | `docker exec … bash -lc` is non-interactive, so bash writes no history file — and nothing in the install path holds the value anyway |

What is recorded is `credentialed: true` plus `credential_sha256`, a 12-hex
prefix of the file's digest: not reversible, not a substring, and enough to tell
two runs apart.

⚠ **What this does NOT do.** The credential is a real file inside the container
and the model runs shell as a user who can read it — it must, or the CLI cannot
authenticate. The guarantee is about what leaves the container and lands in the
artifacts you keep, not about hiding the credential from the trial.

**Prove it rather than trusting this table:**
`go test . -run TestDogfoodCredential -v`. `TestDogfoodCredentialNeverLeaks`
plants a canary, runs a trial that `cat`s the config, greps all three surfaces
for zero hits — and runs the same grep over the un-redacted file first, because a
grep that finds nothing because its PATTERN is wrong is indistinguishable from
one that finds nothing because there is nothing there.

### The caps — what is mechanical and what is not

| cap | flag | mechanical? |
|---|---|---|
| app-name prefix on anything the trial creates or mutates | `--app-prefix` | **yes** — a mismatched `app init` / `create` / `submit` / `listing` is refused and never executed. With no slug on the command line it reads every `block.manifest.json` under `/work` and requires all of them to carry the prefix |
| generation cap | `--max-generations N` | **yes** — the N+1th `civitai generate` is refused before the `docker exec` |
| submission cap | `--max-submissions N` | **yes** — same, for `civitai app submit` |
| `app withdraw` | `--allow-withdraw` to permit | **yes** — refused by default. It permanently destroys a listing's captioned screenshots and names a publication-request id no prefix check can resolve to an app |
| command log | always on | **yes** — `runs/<trial>/commands.log`, one line per command with its verdict and the counters |

🔴 **They classify COMMAND TEXT, and that is the boundary.** A model that writes
a command into a file and runs the file, or builds it at runtime out of pieces
the classifier does not see, is not stopped. The fail-closed rule narrows this —
a segment naming the CLI *and* a spending or publishing verb, with no invocation
the harness can parse (`eval "civitai app submit"`, `c=civitai`), is refused —
and it does not close it. The threat model is the one
`claudedocs/handoff-dogfood-3.md` settled on: a **cooperative** agent. These stop
the ordinary accident, not an adversary. The bound people want for an adversary
needs a second uid, a container with no credential in it, or a platform-capped
token; a better classifier is not that.

**The caps arm themselves when a credential is present**, so an operator does not
have to remember three flags for the bound to exist. With no credential and no
cap flag nothing is judged at all, and the transcript is byte-for-byte what it
has always been — pinned by `TestDogfoodUncredentialedRunIsUnchanged`.

## The render oracle — the verdict for an app-build trial

`oracle.sh` is what turns "the agent produced files" into "a browser watched the
block do the thing". It serves the built block **inside the trial container**,
drives it from a headless browser **on the host** (a trial image ships no
Chromium), runs the brief's assertion, and prints one summary line.

```bash
bash oracle.sh <trial-id> <container-user> [brief]     # default brief: celsius
bash grade.sh  <trial-id> <container-user> [brief]     # setup arms + the render arm
```

A graded cell then carries both verdicts on one line:

```
agent=… check_ok=true … CLOSING_CONDITION=yes render_brief=celsius validate_gate=pass scopes=none observed=212 RENDER=yes
```

- 🔴 **`civitai app validate` is a GATE, not the verdict, and it does not even
  short-circuit.** It runs first because it is cheap and offline, and its result
  is reported as `validate_gate=`. It decides nothing: an untouched
  `civitai app init` scaffold PASSES it (measured, `✓ <dir> is valid`), so a
  validate-keyed grade cannot tell *scaffolded* from *built* and would return a
  confident `yes` to a question it never asked. Nor does a failing gate abort the
  render — that would substitute the validator for the verdict in exactly the
  case where the verdict matters, and would throw away `observed`.
- 🔴 **`observed=` is on the cell on purpose.** The assertion is strict — `212 °F`
  fails where `212` passes — so without the value a working converter with a unit
  suffix is indistinguishable from a block that rendered nothing.
- 🔴 **`RENDER=unmeasured` is a third state, and it is not `no`.** `oracle.sh`
  exits **2** when it measured nothing at all (no browser, no such container, a
  stopped container, a server the host could not reach, an assertion that could
  not run). Folding that into `no` would report *"the model did not build the
  app"* about a run in which no block was ever loaded — the capability confound,
  arriving through the grader.
- 🔴 **It emulates a host before the bundle runs.** A block built from the
  `page-money` template renders nothing but *"Connecting to host…"* until a host
  delivers its runtime context, so the oracle seeds
  `window.__CIVITAI_BLOCK_CONTEXT__` (the branch the SDK's own transport detector
  takes) before navigation. Measured both ways in `briefs/celsius.md`. Set
  `CIVITAI_ASSERT_NO_HOST=1` for the control arm.
- **It never mutates the trial.** One file into the container's `/tmp`, one node
  process, killed by its reported PID on the way out. `/work` is read only, and
  no `docker rm`/`stop`/`commit` appears in `oracle.sh` — a graded container is
  evidence, and re-creating one costs a real trial.
- **Not bounded:** the in-container server binds `0.0.0.0` so the host browser
  can reach it over the bridge. That is inside the network exposure the isolation
  table above already declares open; it closes nothing and opens nothing new.

Tests: `go test . -run 'TestOracle|TestGrade|TestServeBlock'`. Docker is stubbed,
so they need no daemon — but they do drive a **real** browser, and they FAIL
rather than skip under `$CI` (a skip and a pass read the same in a merge log).
`ci.yml`'s `build-test` resolves one into `CIVITAI_CHROME` before `go test ./...`.

- 🔴 **`scopes=` is REPORTED and decides nothing**, exactly like the validate
  gate. A manifest is a declaration, not a behaviour — a block can declare
  `posts:write:self` and post nothing, and the platform grants scopes at review
  rather than at manifest time. It is on the cell because a generate-then-post
  app declaring only `ai:write:budgeted` is worth seeing next to the render
  verdict. Pinned in both directions by `TestOracleReportsScopesWithoutDeciding`.

### The briefs

| brief | what a green cell means | doc |
|---|---|---|
| `celsius` | the agent built *something that runs* — a converter wired to prescribed hooks | `briefs/celsius.md` |
| `genpost` | the agent wired a *generate-then-post* flow: prompt, a Post control gated shut until a generation succeeds, and a status machine the Generate click drives | `briefs/genpost.md` |

🔴 **`genpost` cannot see a generation or a post happen, and never will here.**
The oracle's host emulation is the SDK's `InlineTransport`, a v1 stub whose
`sendRequest` rejects and which delivers no host pushes — so an assertion that
waited for an image or a post id would time out against a perfect app. It grades
the block's own state machine on the near side of the request. Read a green cell
as *"the flow is wired and gated correctly"*, never as *"the money path works"*.

`briefs/` holds each brief and the behavioural assertion that grades it;
`_cdp.mjs` is the browser plumbing they share. 🔴 **An assertion is
only worth running once an untouched `civitai app init` scaffold has been
watched to FAIL it**; a brief the scaffold already satisfies makes every cell
green while measuring nothing. Both briefs' docs record that control for all
three templates — and for `genpost` the scaffold control is not enough on its
own, because `page-money` already ships a prompt field and a Generate button, so
`briefs/genpost.md` also records a complete *generate-only* app failing it.
The tests for the injection path itself are
`dogfood_brief_test.go` in the repo root (`go test -run Dogfood .`), and
`TestEveryBriefHasASiblingGuard` there is the ledger: a new `<name>.brief.txt`
needs `<name>.assert.mjs`, `<name>.md` and a `Test<Name>BriefAndAssertionAgree`.

Trial ids are `<prefix>-<model>-<env>-<identity>`, `t-` by default. The three identities are `claudeid`
(`CLAUDECODE=1`), `codexid` (`CODEX_SANDBOX=1`) and `other` (no signal), and
`DOGFOOD_IDENTITIES` overrides them the same way `DOGFOOD_MODELS` does. Set
`FULL_CROSS=1` to run every model × env × identity combination instead of the
default, which crosses all three only on the cheapest environment — so the
default matrix is **24** trials, not 16.

`driver.sh` is resumable: it skips a trial whose transcript carries a `"kind":
"end"` record, and **re-runs one that does not**. 🔴 It deliberately does NOT key
on the file merely existing — `runner.py` opens that file before it creates the
container, so a crashed or timed-out trial leaves a non-empty transcript with no
`end` record, and an existence check would skip it forever while the matrix
reported COMPLETE. Each trial writes a full transcript — every message, every
command, every result, and per-call token usage.

🔴 **A re-run DESTROYS the partial evidence.** `runner.py` opens the transcript
`"w"` (truncating) and `docker rm -f`s the container before it starts, so
re-running `driver.sh` after a matrix with timed-out trials deletes exactly the
partial transcripts and half-built containers you would want to read to find out
*why* they timed out. **Copy `runs/` aside before re-running** if the failures are
what you are investigating. The resume gate trades "skipped forever" for
"overwritten on the next run"; that is the better default, not a free one.

## 🔴 How a trial ends — the `stop` vocabulary

A trial's `end` record and the one-line `.out` summary both carry `stop`, and a
truncated trial used to be indistinguishable from a finished one. Measured on
`ab-genpost-glm-01` (z-ai/glm-5.3-flash): the last assistant message had
`content: null`, no tool calls, and `completion_tokens: 8000` — **exactly the
`max_tokens` the harness sent** — of which **7,992 were reasoning. The model
exhausted its output budget inside its reasoning channel and returned nothing,
and the harness recorded `stop: "finished"`.** `finish_reason` appeared nowhere
in `runner.py`, `grade.sh` or `oracle.sh`; OpenRouter had been sending it all
along and the harness dropped it. That is a **harness limit reported as a task
outcome**, the one confound this harness exists not to introduce.

`finish_reason` is now recorded on every `assistant` record and on the `end`
record, and the terminal state is split:

| `stop` | means | what the provider said |
|---|---|---|
| `finished` | the model stopped on its own and left a report | `finish_reason` in `stop`/`end_turn`/`stop_sequence`/`eos`/`complete`, content non-empty |
| `truncated` | **a harness limit, not a result** — the output budget ran out | `length` (or a provider-native `max_tokens`/`model_length`/`max_output_tokens`) |
| `empty-reply` | the model stopped on its own and said nothing | a natural stop, content empty/null/whitespace |
| `stopped-unknown:<value>` | the harness has **no evidence** the reply completed | anything else, `none` when the field was absent |
| `max-steps` / `max-cost (…)` | the harness stopped the loop | unchanged |

🔴 **An absent or unrecognised `finish_reason` does NOT become `finished`.**
`finished` is a positive claim that the provider said the model chose to stop,
so it is only made when the provider actually did. Defaulting the unknown case
to success is precisely the defect above, one provider vocabulary later. The raw
value rides in the string so a new word is diagnosable from the `.out` line
alone. `stopped-unknown:*` is a statement about the *instrument*, not the model.

**Grading is unaffected.** `grade.sh` and `oracle.sh` measure the container and
never open `transcript.jsonl`, so no cell's `CLOSING_CONDITION` changes value
because of the new vocabulary — pinned by `TestGradersDoNotReadTheStopVocabulary`,
which goes red if either script starts reading it. `driver.sh`'s resume guard
greps for `"kind": "end"` and is likewise indifferent. **What changes is what a
human reads**: a `truncated` cell must not be counted as a model failure.

### The output ceiling, and the model's own reasoning

- `--max-tokens` (default **32000**, `DOGFOOD_MAX_TOKENS` in `driver.sh`).
  8,000 left that model **8 tokens** after its reasoning. Its per-turn reasoning
  burn ran 2,141 → 4,238 → 2,021 → 7,992; only the first three are uncensored
  observations, since the fourth *is* the cap. 32000 is 4× the budget that was
  exhausted and ~7.5× the largest burst we have seen complete. It is a
  **ceiling, not a spend cap** — tokens are billed as generated and `--max-cost`
  (still $1) is the only bound on money.
- The runner now sends the provider's **`reasoning_details`** (verbatim, so
  signatures survive) and `reasoning` back in the assistant history. Previously
  it appended only `content` + `tool_calls`; for a model whose `content` was
  `null` on 66 of 67 turns, its entire contribution to its own history was the
  text of the shell commands it ran — **87.8% of its output discarded every
  turn**, then re-derived inside the budget that truncated it. Turn it off with
  `--no-carry-reasoning` (history grows faster; prompt cost is already O(n²) in
  steps). A provider that returns neither field sends exactly the message shape
  it always did — `TestDogfoodAssistantHistoryIsUnchangedWithoutReasoning`.
- The `.out` summary now carries `finish_reason`, a `reasoning_tokens` total and
  a **per-turn `turns` array** (`completion_tokens`, `reasoning_tokens`,
  `finish_reason`). The escalation above was plainly visible in the transcript
  and completely invisible in the summary an operator actually reads; a total
  cannot show an escalation.

## Reading a verdict

`grade.sh` measures the CONTAINER. It never reads what the agent said it did,
because those are different claims — in the 2026-09-18 matrix, **0 of 8** agents
on the `--prefix` path noticed that the `AGENTS.md` they had just written names a
binary no later shell can find. Most of them reported the situation accurately as
far as they went (8/8 relayed the PATH line, 6/8 warned it would not persist);
what none of them reported is the thing only the container can tell you.

It reports the arc's frozen closing condition, which needs BOTH halves:

- **A** — `civitai agent-setup --check --json` reports `ok: true`
- **B** — `zsh -lic 'civitai --version'` prints that same version in the user's
  **login** shell

B exists because A alone has been green while the login shell still served the
old binary. A verdict of `CLOSING_CONDITION=yes` requires both.

## 🔴 Validate the grader before you read a verdict

Three controls. ⚠ **None of them FOUND a defect** — an earlier draft of this line
claimed each had, and it was wrong. Defect 3 was found by an audit round;
**what found defects 1 and 2 is not recorded and is no longer guessed at** (three
drafts gave three answers). `ctl-profile` was written afterwards to pin defect 3.
The controls prove the instrument can go red AND green — still not ceremony, since
one never watched to do both is a claim about itself, but a weaker claim than "each
caught something", and weaker again than coverage:

```bash
docker run -d --name dogfood-ctl-neg df-node-root sleep infinity
bash grade.sh ctl-neg root          # MUST report CLOSING_CONDITION=no

docker run -d --name dogfood-ctl-pos -e CLAUDECODE=1 df-node-root sleep infinity
docker exec -w /work dogfood-ctl-pos bash -lc \
  'npm install -g @civitai/cli && civitai agent-setup --track app'
bash grade.sh ctl-pos root          # MUST report CLOSING_CONDITION=yes
```

A grader that has not been watched to go red AND green is a claim about itself.

🔴 **A third control, pinning a false GREEN that an audit round found** (it did not
find it — see the retraction above). Arm B used to run
`bash -lc "zsh -lic '…'"`. The outer bash login shell sources `~/.profile`, whose
stock `if [ -d "$HOME/.local/bin" ]` block prepends a directory **zsh never sees** —
so a CLI installed under `$HOME/.local` was visible to the wrapper and invisible to
the shell the closing condition actually names. Build that state and the grader must
say **no**:

```bash
docker run -d --name dogfood-ctl-profile -u dev -e CLAUDECODE=1 df-node-user sleep infinity
docker exec -u dev -w /work dogfood-ctl-profile bash -lc \
  'npm install -g --prefix="$HOME/.local" @civitai/cli
   export PATH="$HOME/.local/bin:$PATH"; civitai agent-setup --track app'
bash grade.sh ctl-profile dev       # see the assertion below
```

🔴 **Assert the FIELDS, not just the verdict.** `CLOSING_CONDITION=no` alone is
satisfied by `ctl-neg` too, and by a grader that broke arm A instead — this control
is only meaningful if arm A is GREEN while arm B is red. Require:

```
check_ok=true  failed_checks=[authenticated]  mcp_rows=2
login_version=none  agent_shell_version=0.1.105  CLOSING_CONDITION=no
```

Measured against the same container: `bash -lc 'civitai --version'` → `0.1.105`;
`zsh -lic 'civitai --version'` → `command not found`. The wrapped grader reported
**yes**; the corrected one reports **no**. ⚠ The 2026-09-18 grid is unaffected — no
trial installed under `$HOME/.local` — so the defect was latent, not triggered.

## Agent identity is a dimension, not a detail

`civitai agent-setup` branches hard on which agent it detects, and detection is by
environment variable. `--agent-env CLAUDECODE=1` makes a trial the `claude` path;
omitting it makes it `other`, which is what every agent outside the CLI's table
gets.

🔴 **Identity is CROSSED with the model in `driver.sh`, never bound to it.** The
first version of this harness gave `claude`/`gpt` a known identity and
`gemini`/`grok` none. The resulting grid partitioned perfectly by model — and was
equally well explained by identity. Read the wrong way it says *"Gemini and Grok
fail the onboarding"*, which is **false**: swap the identities and the outcome
swaps with them. The default cross runs all three identities on the cheapest
environment for exactly this reason, and a per-model claim is only readable
between two cells whose identity matches.

## Known limits

The harness gives each trial the true bytes of `prompt.md` via `curl`. A real
agent's WebFetch may summarise and drop content, so **fetch lossiness is out of
scope here and a green matrix says nothing about it.** Linux only: the Homebrew
branch of the prompt is never executed. Every project directory starts empty, so
the config-merge paths are unexercised.

Full method, results, and the two causes the first run found (one a defect filed as
cli#665; one a design call, DECIDED 2026-09-19 —
`claudedocs/refs/agent-setup-verdict-decision-2026-09-19.md`):
`claudedocs/refs/agent-setup-dogfood-matrix-2026-09-18.md`.
