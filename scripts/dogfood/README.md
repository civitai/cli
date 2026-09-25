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
| money | yes | `--max-cost` (default $1/trial) — a step cap is not a spend cap, and a turn the provider does not price ends the trial (`unpriced-turn`) rather than counting as $0 |
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
  --brief "$(cat briefs/celsius.brief.txt)" --brief-name celsius

# the whole matrix as app-build trials, in their own trial-id namespace
DOGFOOD_BRIEF_NAME=celsius DOGFOOD_TRIAL_PREFIX=ta bash driver.sh
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
- 🔴 **`brief_name` rides alongside it, on the same contract, because the PROSE
  IS NOT AN IDENTIFIER.** The render oracle has to map a graded cell back to
  `briefs/<name>.assert.mjs`, and from prose alone the only route is an exact
  match against `briefs/*.brief.txt` — which stops resolving every already-run
  trial the moment a brief file is reworded, and cannot resolve an ad-hoc brief
  at all. `--brief-name` / `DOGFOOD_BRIEF_NAME` records it, and `driver.sh` also
  derives it when you paste a committed brief's text, so the two cannot
  disagree. `runner.py` refuses a name with no assertion behind it, or one whose
  committed text is not the text being sent, **before** a container or an API
  call exists — a mislabelled name is recorded as a fact and then believed by
  every later grade. An ad-hoc brief still runs and simply records no name.
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
  --brief "$(cat briefs/genpost.brief.txt)" --brief-name genpost \
  --credential-file ~/.config/civitai/config.yaml \
  --app-prefix dogfood4- --max-generations 3 --max-submissions 1

# the whole matrix, credentialed (driver.sh refuses without DOGFOOD_APP_PREFIX)
DOGFOOD_CREDENTIAL_FILE=~/.config/civitai/config.yaml \
DOGFOOD_APP_PREFIX=dogfood4- DOGFOOD_MAX_GENERATIONS=3 DOGFOOD_MAX_SUBMISSIONS=1 \
DOGFOOD_BRIEF_NAME=genpost DOGFOOD_TRIAL_PREFIX=tc bash driver.sh
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
| app-name prefix on anything the trial creates or mutates | `--app-prefix` | **yes** — a mismatched `app init` / `create` / `submit` / `listing` is refused and never executed. It reads the positional app-name argument plus the value of any flag `GATED_FLAGS` (runner.py) marks slug-bearing — `--slug`, `--from`, `--name`, `--dir` — in both the `--slug x` and `--slug=x` spelling. With no slug on the command line it reads every `block.manifest.json` under `/work` and requires all of them to carry the prefix. **Other flags' values are not app names:** `--template static` used to be refused as if `static` were a slug, and the agent that believed it measured the wrong scaffold |
| generation cap | `--max-generations N` | **yes** — the N+1th `civitai generate` is refused before the `docker exec` |
| submission cap | `--max-submissions N` | **yes** — same, for `civitai app submit` |
| `app withdraw` | `--allow-withdraw` to permit | **yes** — refused by default. It permanently destroys a listing's captioned screenshots and names a publication-request id no prefix check can resolve to an app |
| `app listing set-text` | `--allow-listing-text` to permit | **yes** — refused by default. It rewrites the listing's public tagline/description/category **in place on every listing status** — not a "material" change, so no revision and no moderator review — and this CLI has no command that restores the previous value. Neither flag is threaded through `driver.sh`, so a matrix run cannot reach either |
| command log | always on | **yes** — `runs/<trial>/commands.log`, one line per command with its verdict and the counters |

🔴 **They classify COMMAND TEXT, and that is the boundary.** A model that writes
a command into a file and runs the file, or builds it at runtime out of pieces
the classifier does not see, is not stopped. The fail-closed rule narrows this —
a segment naming the CLI *and* a spending or publishing verb, with no invocation
the harness can parse (`eval "civitai app submit"`), is refused —
and it does not close it. **A HERE-DOCUMENT BODY IS NOT COMMAND TEXT** and is
stripped before any of this runs (`strip_heredoc_bodies`): it is stdin for the
opener's command, the shell never executes it, and classifying it refused an
agent's own `block.manifest.json` for quoting "Civitai" next to the word
"generated". ⚠ Inline redirect content that is an *argument* (`echo '…' > f`) IS
still classified, because to a shell it genuinely is command text. ⚠ And
`c=civitai; $c app submit` is **not** refused, at this ref or before it: segment 1
names the CLI with no verb, segment 2 names a verb with no "civitai", so neither
trips the rule. The threat model is the one
`claudedocs/handoff-dogfood-3.md` settled on: a **cooperative** agent. These stop
the ordinary accident, not an adversary. The bound people want for an adversary
needs a second uid, a container with no credential in it, or a platform-capped
token; a better classifier is not that.

🔴 **`--slug=NAME` used to walk straight past the prefix cap, and `--slug NAME`
did not.** The classifier dropped every token starting with `-`, so the attached
form of a flag offered no slug candidate at all; `_prefix_ok` then fell through
to its "read every `block.manifest.json` under `/work`" branch, which PASSES —
the workspace manifests were all created by the trial and all carry the prefix,
while `--slug` pointed at a real listing on the account. cobra treats the two
spellings as identical and this did not. It defeated the cap for **every** gated
verb (`app init`, `app create`, `app submit`, the whole `app listing` group), not
only the one it was found on. An `--opt=value` token now contributes its VALUE;
`TestDogfoodAttachedSlugFlagDoesNotBypassThePrefixCap` is red at `4d4a45e` on
seven commands and green here, with
`TestDogfoodAttachedSlugOnTheTrialsOwnAppStillRuns` as the over-refusal control.

🔴 **`civitai app listing status` is NOT a read, and the comment in `runner.py`
used to say it was.** On a LIVE listing it calls `getMyListingForEdit`, which
idempotently **opens a shadow revision draft** server-side — the CLI's own
`--help` says so — and there is no `discard-revision` command anywhere in this
CLI to close it. It happened to the operator's `panorama-360` listing on
2026-09-25. It submits nothing and destroys nothing, so it stays inside the
prefix gate rather than being promoted to a refused-by-default verb: refusing it
would take away the trial's only way to observe its own listing, which changes
what the harness measures, for a side effect that costs nothing irreversible.
⚠ The old comment listed `status` among the ungated reads. That is true of
`civitai app status` and false of `civitai app listing status` — `listing` is in
`APP_MUTATING`, so the whole group including `status` goes through the prefix
gate. A report written off that comment asserted the opposite of what the code
does; `TestDogfoodAppStatusAndAppListingStatusAreGatedDifferently` pins the two
behaviours so the next reader does not have to trust the prose.

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
bash oracle.sh <trial-id> <container-user> [brief]     # brief DERIVED from the trial
bash grade.sh  <trial-id> <container-user> [brief]     # setup arms + the render arm
```

A graded cell then carries both verdicts on one line:

```
agent=… check_ok=true … CLOSING_CONDITION=yes render_brief=celsius brief_source=transcript-name validate_gate=pass scopes=none viewer=signed-in observed=212 RENDER=yes
```

- 🔴 **The brief is DERIVED from the trial, and the argument is only a
  cross-check.** It used to be an optional positional defaulting to `celsius`,
  and the cost was measured: `oracle.sh ab-genpost-mimo-01 root`, against a
  trial built from the `genpost` brief, ran the CELSIUS assertion, timed out
  waiting for `[data-testid="celsius"]` and printed `RENDER=no` — byte-identical
  to the verdict a model that built nothing earns. `runner.py` records the brief
  in the trial's `start` record (`brief` = the prose, `brief_name` = the name),
  so `oracle.sh` reads it out of `<runs>/<trial>/transcript.jsonl` and **refuses
  (exit 2) when an argument disagrees** rather than picking one. `brief_source=`
  says how it was resolved: `transcript-name`, `transcript-text`,
  `argument-unverified`, `argument-trial-recorded-no-brief`, or
  `default-trial-recorded-no-brief` (a setup cell, which built no app). Point
  `DOGFOOD_RUNS` at the directory the trial was driven from if it is not
  `scripts/dogfood/runs`.
- 🔴 **The oracle presents a SIGNED-IN viewer, reported as `viewer=`.** It used
  to seed `viewer: null`, and an auth-gated app — the shape the CLI's own
  `page-money` scaffold ships, `const anon = ready && !viewer` and a sign-in
  CTA — then rendered its signed-out branch and graded `RENDER=no`. That verdict
  was about the harness. The seeded object is byte-for-byte what civitai.com's
  `withSignedInFlag()` emits (`{ id, username, signedIn: true }`, no `status`),
  and the token stays empty so it buys the block no capability. Set
  `CIVITAI_ASSERT_ANON_VIEWER=1` for the control arm, which is also how you
  grade a block's signed-out branch deliberately.

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

#### Launching the browser (`launch()` in `briefs/_cdp.mjs`)

🔴 **THE LAUNCH IS ITS OWN FAILURE DOMAIN AND IT MADE `build-test` FLAKY ON
EVERY PR** — measured 2026-09-21 over four runs, on `main` and on three PRs, one
of them a docs-only change to a single markdown file. The browser printed its
dbus startup noise, no DevTools endpoint arrived inside the deadline, the
assertion reported a `harness error`, `oracle.sh` correctly returned exit 2 /
*nothing was measured*, and the job went red **with zero `--- FAIL:` lines in a
31 KB log**. Two of those runs went green on re-run with no change. Everything
downstream was working; nothing upstream was watching the browser.

What the launch does about it, and the standing of each part:

| | |
|---|---|
| **Its own budget.** `CIVITAI_ASSERT_LAUNCH_MS` (default 30 s), separate from `CIVITAI_ASSERT_WAIT_MS` (15 s). | A cold browser binding a port and a React tree mounting are unrelated quantities. Measured in the failing jobs themselves, `chromium --version` took **1.9–6.5 s** on the runner against **0.02 s** on a dev box — 100–300× slower, 3.4× spread. ⚠ It does not separate the passing runs from the failing ones, so it sizes the budget; it does not diagnose the stall. |
| **Flags that skip startup work.** `--password-store=basic` plus the no-background-network group. | The runner's `DBUS_SESSION_BUS_ADDRESS` is unparseable — reproduce the exact CI stderr with `DBUS_SESSION_BUS_ADDRESS=bogus:path=/nope`. Measured: the flag removes one session-bus round trip (4 `dbus/bus.cc` errors → 3), the keyring probe, which is the last one before the DevTools line. Three of the four failures stalled after exactly three; **the fourth stalled after one, so this is not the whole mechanism.** ⚠ Null result worth not re-deriving: *clearing* the env var changes nothing (4 either way). |
| **Release builds preferred.** `google-chrome` before `chromium`. | `ubuntu-latest` ships both, and `/usr/bin/chromium` there is a raw `chromium-browser-snapshots` build (152.0.7977.0) while `google-chrome` is a release (152.0.7977.82). ⚠ A determinism argument, **not** a measurement. |
| **The process is killed when a launch fails.** | The old code walked away from it; all four CI logs end with the runner's own `Terminate orphan process: pid (…) (chrome)`. |
| **A bounded retry** — `CIVITAI_ASSERT_LAUNCH_ATTEMPTS`, default 2. | The backstop, not the fix. 🔴 **It is loud on SUCCESS**: a launch that needed a second attempt prints `BROWSER LAUNCH RETRY` with the first attempt's diagnostics, and `ci.yml`'s browser-smoke step turns that string into a `::warning`. That is the only thing keeping it from being a way to stop noticing a browser that has started failing half the time. A browser that never works still fails, after exactly N attempts, with every attempt's output in the message. |

⚠ **The stall was never reproduced locally.** `TestCdpLaunch*` in
`dogfood_cdp_launch_test.go` are therefore invariant guards on the launch's
behaviour, driven by a fake browser, and only the orphan one is a regression
test. Do not read them as evidence the flake is gone.

`ci.yml` gained a **browser smoke step** that runs `launch()` before `go test`,
carrying both controls: it fails a binary that exits 3 without printing (so it
can still go red) and it fails the job by name if the resolved browser cannot
bind a debugging port, instead of that surfacing eighty lines into a Go test.

- 🔴 **`scopes=` decides no verdict**, exactly like the validate gate. A manifest
  is a declaration, not a behaviour — a block can declare `posts:write:self` and
  post nothing, and the platform grants scopes at review rather than at manifest
  time. It is on the cell because a generate-then-post app declaring only
  `ai:write:budgeted` is worth seeing next to the render verdict. Pinned in both
  directions by `TestOracleReportsScopesWithoutDeciding`.
- 🔴 **…but it is not inert: the same list is SEEDED as the bootstrap's
  `token.scopes`, and an empty list was never the neutral default.** The oracle
  seeded `scopes: []` unconditionally until 2026-09-21. Measured on
  `ab-genpost-dsv4-01`: a correct generate-then-post app whose Generate handler
  reads `hasBudgetedScope(token.scopes)` and asks the host for consent when that
  is false never reached `generating`, and the cell read `RENDER=no
  observed=ready` — the verdict a model that built nothing earns, about the
  harness. An empty list grades the **refused** branch, rewarding an app that
  flips a status optimistically over one that checks consent first. `oracle.sh`
  reads the manifest once and hands the list to the assertion as an argument;
  `token.raw` stays `''`, so scopes buy a *branch* and never a *capability*, and
  a disagreement between the cell's `scopes=` and the list the assertion reports
  seeding is exit 2 (nothing measured). Four-arm measurement and the shippability
  control — an untouched `page-money` scaffold still grades `no` with both scopes
  seeded — in `briefs/genpost.md`; pinned by
  `TestOracleSeedsTheBlocksDeclaredScopes`.

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
| `unpriced-turn (…)` | **the money cap became inoperable** — the provider did not price a turn | `usage.cost` absent, null, or not a number |

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

### 🔴 The money cap could silently never fire

`--max-cost` is a comparison against a running total that comes **entirely** from
the provider's `usage.cost`. There is no price table in `runner.py` and no
`/api/v1/models` fallback — `grep -c 'api/v1/models\|price\|pricing'` over the
file returns 0 — and the accumulation was a single line:

```
usage_total["cost"] += u.get("cost", 0.0) or 0.0
```

So a model, or a route, whose usage payload **omits** `cost` (or returns it
null) accumulated **$0 forever**: the cap never tripped, and `--max-steps` was
the only remaining bound — which the flag's own comment says in so many words is
*not* a spend cap. The `end` record's `cost: 0.0` was the money-shaped twin of
the `stop: "finished"` defect above: an absent value rendered as a successful
measurement.

**An unpriced turn now ends the trial.** `stop` becomes
`unpriced-turn (turn N returned no usable usage.cost, …)`, a `kind: "unpriced"`
record carries the usage block the provider actually sent, and `finish_reason`
is still on the `end` record and the summary so the terminal state is not lost.

- **A stated `$0` is a price; an absent figure is not.** A free route keeps
  running. Only absence, null, and a non-numeric value stop the trial.
- **Exactly one call is allowed to complete, and that is the floor.** You cannot
  know a provider omits `cost` until a response arrives, so the first request is
  spent no matter what. There is deliberately **no grace period** past it: N
  turns of unknown cost is still unbounded spend, bounded only by a number
  nobody can convert into money. The stop happens before the unpriced turn's
  tool calls run and before the next `call()`.
- **`end.usage.cost` is a LOWER BOUND on such a run**, not a measurement — the
  unpriced turn was billed and is not in the total. The refusal string says so.

Red at `4d4a45e` / green here: `TestDogfoodUnpricedTurnIsAHardStop` (three of
its four arms — the fourth, a string `cost`, crashes the base runner with a
`TypeError` instead, which is a different defect) and
`TestDogfoodUnpricedTurnStopsBeforeTheNextBilledCall` (base issues 3 requests,
this issues 1). `TestDogfoodAPricedZeroCostTurnStillRuns` is the discriminating
control: without it, "always stop" would satisfy every assertion above.
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
