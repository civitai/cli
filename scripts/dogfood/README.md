# `scripts/dogfood` — blind dogfood harness for the agent-setup entrypoint

Answers one question, repeatably: **can a real coding agent, given only
`https://developer.civitai.com/agent-setup/prompt.md`, reach a working setup on a
machine that did not build the CLI?**

One trial = one model driving one throwaway container. The model's only tool is a
`bash` that runs inside that container, and the container holds an OS, node/npm,
zsh and curl — nothing else. **Blindness is a mount namespace, not an
instruction:** this repo, its `AGENTS.md` and the operator's home directory are
not reachable from inside, so an agent cannot read the source even by accident.

No Civitai credential is involved at any point, and nothing in a trial can spend.
The only credential is the operator's OpenRouter key, read from
`OPENROUTER_API_KEY` and never written anywhere.

## Run it

```bash
export OPENROUTER_API_KEY=sk-or-...
cd scripts/dogfood
for e in node-root node-user ubuntu-apt stale-cli; do
  docker build -q -t "df-$e" -f "envs/$e.Dockerfile" .
done
bash driver.sh                        # 4 models x 4 environments, 4 at a time
bash grade.sh t-claude-noderoot root  # grade one finished trial
```

`driver.sh` skips a trial whose `runs/<id>/transcript.jsonl` already exists, so it
is resumable. Each trial writes a full transcript — every message, every command,
every result, and per-call token usage.

## Reading a verdict

`grade.sh` measures the CONTAINER. It never reads what the agent said it did,
because those are different claims — in the 2026-09-18 matrix, 5 of 8 agents
declared success on a setup that did not survive their own shell.

It reports the arc's frozen closing condition, which needs BOTH halves:

- **A** — `civitai agent-setup --check --json` reports `ok: true`
- **B** — `zsh -lic 'civitai --version'` prints that same version in the user's
  **login** shell

B exists because A alone has been green while the login shell still served the
old binary. A verdict of `CLOSING_CONDITION=yes` requires both.

## 🔴 Validate the grader before you read a verdict

Two controls, both of which have caught a real grader defect:

```bash
docker run -d --name dogfood-ctl-neg df-node-root sleep infinity
bash grade.sh ctl-neg root          # MUST report CLOSING_CONDITION=no

docker run -d --name dogfood-ctl-pos -e CLAUDECODE=1 df-node-root sleep infinity
docker exec -w /work dogfood-ctl-pos bash -lc \
  'npm install -g @civitai/cli && civitai agent-setup --track app'
bash grade.sh ctl-pos root          # MUST report CLOSING_CONDITION=yes
```

A grader that has not been watched to go red AND green is a claim about itself.

## Agent identity is a dimension, not a detail

`civitai agent-setup` branches hard on which agent it detects, and detection is by
environment variable. `--agent-env CLAUDECODE=1` makes a trial the `claude` path;
omitting it makes it `other`, which is what every agent outside the CLI's table
gets. **Vary it independently of the model** — the first matrix confounded the two
and needed three extra trials to separate them.

## Known limits

The harness gives each trial the true bytes of `prompt.md` via `curl`. A real
agent's WebFetch may summarise and drop content, so **fetch lossiness is out of
scope here and a green matrix says nothing about it.** Linux only: the Homebrew
branch of the prompt is never executed. Every project directory starts empty, so
the config-merge paths are unexercised.

Full method, results and the two defects the first run found:
`claudedocs/refs/agent-setup-dogfood-matrix-2026-09-18.md`.
