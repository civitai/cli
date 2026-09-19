# `scripts/dogfood` — blind dogfood harness for the agent-setup entrypoint

Answers one question, repeatably: **can a real coding agent, given only
`https://developer.civitai.com/agent-setup/prompt.md`, reach a working setup on a
machine that did not build the CLI?**

One trial = one model driving one throwaway container. The model's only tool is a
`bash` that runs inside that container, and the container holds an OS, node/npm,
zsh and curl — nothing else. **Blindness is a mount namespace, not an
instruction:** this repo, its `AGENTS.md` and the operator's home directory are
not reachable from inside, so an agent cannot read the source even by accident.

No Civitai credential is involved at any point, so no trial can spend Buzz or
touch a real account. The only credential is the operator's OpenRouter key, read
from `OPENROUTER_API_KEY` and never written anywhere.

### 🔴 What is isolated, and what is NOT

Read this before running it on your own workstation — you are handing a language
model an unsandboxed root shell in half of these images.

| | bounded? | by what |
|---|---|---|
| filesystem — repo, `$HOME`, credentials | **yes** | mount namespace; the container has none of them |
| Civitai account / Buzz | **yes** | no Civitai credential exists in a trial |
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

Trial ids are `t-<model>-<env>-<identity>`. The three identities are `claudeid`
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

Full method, results, and the two causes the first run found (one a defect, one a
contested design call):
`claudedocs/refs/agent-setup-dogfood-matrix-2026-09-18.md`.
