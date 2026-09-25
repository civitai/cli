#!/usr/bin/env bash
# Grade one finished trial against the arc's frozen closing condition, by
# measuring the container — never by reading what the agent said it did.
#
#   grade.sh <trial-id> <container-user> [brief-name]
#
# Both halves are required:
#   A. `civitai agent-setup --check --json` reports ok: true
#   B. `zsh -lic 'civitai --version'` prints a version in the USER's LOGIN shell
#      (the half that was green-by-accident once: ok:true inside the agent's own
#      shell while the login shell still served the old binary).
#
# ── the app-build arc ────────────────────────────────────────────────────────
# A THIRD arm exists for app-build trials and is OPT-IN: pass a brief name (or
# set DOGFOOD_ASSERT) and oracle.sh serves the built block inside this same
# container, drives it with a headless browser, and its `RENDER=` + `observed=`
# are appended to the verdict line below.
#
# ⚠ The brief name you pass is a REQUEST, not the brief. oracle.sh derives the
# brief from the trial's own transcript and REFUSES (exit 2 -> RENDER=unmeasured)
# when the two disagree, so the `render_brief=` on the verdict line below is the
# RESOLVED name read back off the oracle, never the string handed in here.
#
# 🔴 OPT-IN, AND SILENT OTHERWISE, ON PURPOSE. The setup grid of 2026-09-18 was
# graded by the two arms above and nothing else. Running a render oracle by
# default would append fields to every already-measured cell's line and make an
# app-build `no` — which a setup trial MUST produce, it never built an app —
# look like a regression in a grid that never asked the question.
set -uo pipefail
TRIAL="${1:?trial id}"
U="${2:-root}"
BRIEF="${3:-${DOGFOOD_ASSERT:-}}"
C="dogfood-$TRIAL"
HERE="$(dirname "$0")"
# Guard the VALUE, not just the cd (`cd ""` is a silent no-op on bash <= 5.2).
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 2; }

# 🔴 POSITIVE CONTROL ON THE INSTRUMENT ITSELF. Without this, a grade against a
# container that does not exist returns empty stdout, which is byte-identical to
# "the CLI is not installed" — so a typo in the trial id prints a confident
# CLOSING_CONDITION=no about nothing at all.
# 🔴 RUNNABLE, not merely PRESENT. `docker inspect` succeeds for a container in ANY
# state, `exited` included — and against a stopped container every exec fails, stdout
# is empty, and the verdict lands on `check_ok=parse-error`, which is the exact
# reading this guard exists to separate. Reachable: a daemon restart between a long
# matrix and grading, a `docker stop`, or a cgroup OOM kill (newly plausible now that
# --memory is in force).
STATE=$(docker inspect -f '{{.State.Status}}' "$C" 2>/dev/null)
if [ -z "$STATE" ]; then
  printf 'no such container: %s — nothing was measured (this is NOT a failing trial)\n' "$C" >&2
  exit 2
fi
if [ "$STATE" != "running" ]; then
  printf 'container %s is %s, not running — nothing was measured (this is NOT a failing trial)\n' \
    "$C" "$STATE" >&2
  exit 2
fi

x() { docker exec -u "$U" -w /work "$C" bash -lc "$1" 2>&1; }
# stdout ONLY. `--check` prints its error line to STDERR and the payload to
# STDOUT, so a merged capture is unparseable and jq's failure reads as "no JSON
# at all" — which is indistinguishable from "the CLI is not installed".
#
# ⚠ DO NOT RE-DERIVE THIS FROM THE ORDERING. This comment used to say the error
# line comes AHEAD of the JSON; measured on v0.1.106 it comes AFTER. The ordering
# is not the mechanism and is not stable — interleaving of two streams never is —
# so checking it and finding the old claim false would invite deleting a guard
# that is still load-bearing. The mechanism is that they are DIFFERENT STREAMS.
xo() { docker exec -u "$U" -w /work "$C" bash -lc "$1" 2>/dev/null; }
xrc() { docker exec -u "$U" -w /work "$C" bash -lc "$1" >/dev/null 2>&1; echo $?; }

printf '=== %s ===\n' "$TRIAL"

printf -- '--- A: agent-setup --check --json\n'
CHECK_RC=$(xrc 'civitai agent-setup --check --json')
CHECK_OUT=$(xo 'civitai agent-setup --check --json')
printf '%s\nrc=%s\n' "$CHECK_OUT" "$CHECK_RC"
# NOT `.ok // "absent"` — jq's `//` treats `false` as empty, so a real `ok:false`
# came back as "absent", i.e. the instrument could not distinguish a failing
# setup from no JSON at all.
OK=$(printf '%s' "$CHECK_OUT" | jq -r 'if has("ok") then (.ok|tostring) else "absent" end' 2>/dev/null || echo parse-error)
[ -z "$OK" ] && OK=parse-error
# 🔴 READ THE IDENTITY THE CONTAINER ACTUALLY REPORTS, NOT THE ONE THE TRIAL ID
# CLAIMS. The matrix's whole finding is that the verdict is a function of (agent
# identity, npm prefix writable) — and identity reached a reader only through the
# filename `t-<model>-<env>-<identity>`, which is an assertion by whoever named
# the trial, not a measurement of the box. A mislabelled or mis-exported identity
# would have been graded under the wrong cell with nothing to notice it.
AGENT_ID=$(printf '%s' "$CHECK_OUT" | jq -r '.agent // "unknown"' 2>/dev/null || echo unknown)
[ -z "$AGENT_ID" ] && AGENT_ID=unknown
FAILED=$(printf '%s' "$CHECK_OUT" | jq -r '[.checks[]|select(.ok==false)|.name]|join(",")' 2>/dev/null)

printf -- '--- B: login shell\n'
# 🔴 ARM B RUNS zsh DIRECTLY — NOT THROUGH `x`, WHICH WRAPS EVERYTHING IN
# `bash -lc`. That wrapper made this measure the wrong shell: the outer bash login
# sources `~/.profile`, whose stock `if [ -d "$HOME/.local/bin" ]` block prepends a
# directory that **zsh never sees**, because zsh reads .zshenv/.zprofile/.zshrc and
# not .profile. MEASURED in df-node-user and df-ubuntu-apt with the CLI installed
# under $HOME/.local: `bash -lc "zsh -lic 'civitai --version'"` printed 0.1.105,
# while `zsh -lic 'civitai --version'` printed `command not found`. Opposite
# verdicts for the same container — and the frozen closing condition names the
# zsh form, so the wrapper was grading a condition nobody agreed to.
zexec() { docker exec -u "$U" -w /work "$C" zsh -lic "$1" 2>&1; }
LOGIN_OUT=$(zexec 'civitai --version')
docker exec -u "$U" -w /work "$C" zsh -lic 'civitai --version' >/dev/null 2>&1
LOGIN_RC=$?
printf '%s\nrc=%s\n' "$LOGIN_OUT" "$LOGIN_RC"

printf -- '--- agent-shell version (for the A/B disagreement)\n'
x 'civitai --version'

printf -- '--- footprint\n'
x 'ls -la /work; echo "-- AGENTS.md:"; wc -c AGENTS.md 2>/dev/null; echo "-- CLAUDE.md:"; wc -c CLAUDE.md 2>/dev/null'
x 'for f in "$HOME"/.claude.json "$HOME"/.mcp.json /work/.mcp.json "$HOME"/.codex/config.toml "$HOME"/.config/opencode/opencode.json /work/opencode.json "$HOME"/.cursor/mcp.json /work/.cursor/mcp.json; do [ -e "$f" ] && { echo "== $f"; head -c 1500 "$f"; echo; }; done; true'

printf -- '--- verdict\n'
LOGIN_VER=$(printf '%s' "$LOGIN_OUT" | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | tail -1)
AGENT_VER=$(x 'civitai --version' | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | tail -1)

# 🔴 ROW PRESENCE IS PART OF THE VERDICT, NOT DECORATION. Any proposal to stop
# the MCP rows failing `ok` must keep REPORTING them — the user still has to
# paste. Without this assertion the obvious WRONG fix, deleting the rows
# outright, grades exactly like the right one.
ROWS=$(printf '%s' "$CHECK_OUT" | jq -r '[.checks[].name]|join(",")' 2>/dev/null)
MCP_ROWS=$(printf '%s' "$CHECK_OUT" | jq -r '[.checks[]|select(.name|startswith("mcp-"))]|length' 2>/dev/null)
[ -z "$MCP_ROWS" ] && MCP_ROWS=unreadable

# 🔴 `LOGIN_RC` IS PART OF HALF B, NOT DECORATION. `LOGIN_OUT` is a MERGED stream,
# so `grep -oE '<semver>'` matches a version appearing in ANY line — an npm
# deprecation banner, a .zshrc greeting, an error. Without the rc, half B reads
# "some semver appeared in both shells and they matched", which a banner printing
# the same node/npm version in both satisfies with `civitai` absent from both.
# The rc was already being captured and was going unused.
PASS=no
if [ "$OK" = "true" ] && [ "$LOGIN_RC" = "0" ] && [ -n "$LOGIN_VER" ] \
   && [ "$LOGIN_VER" = "$AGENT_VER" ] && [ "$MCP_ROWS" = "2" ]; then PASS=yes; fi

# ── the render arm, when a brief was named ───────────────────────────────────
# 🔴 THE ORACLE'S EXIT CODE IS PART OF THE READING, NOT NOISE. It exits 2 when
# it measured NOTHING (no browser, unreachable server, a container that went
# away). Folding that into `RENDER=no` would report "the model did not build the
# app" about a run in which no block was ever loaded — the capability confound,
# arriving through the grader. It becomes `RENDER=unmeasured` instead, which no
# reader can mistake for a verdict.
RENDER_FIELDS=
if [ -n "$BRIEF" ]; then
  printf -- '--- C: render oracle (%s)\n' "$BRIEF"
  ORACLE_OUT=$(bash "$HERE/oracle.sh" "$TRIAL" "$U" "$BRIEF" 2>&1)
  ORACLE_RC=$?
  printf '%s\n' "$ORACLE_OUT"
  if [ "$ORACLE_RC" != "0" ]; then
    RENDER_FIELDS=" render_brief=$BRIEF RENDER=unmeasured"
  else
    SUMMARY=$(printf '%s\n' "$ORACLE_OUT" | grep -a '^brief=' | tail -1)
    R_GATE=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^gate=//p' | head -1)
    R_OBS=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^observed=//p' | head -1)
    R_PASS=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^RENDER=//p' | head -1)
    # REPORTED, not part of the verdict — see the note in oracle.sh.
    R_SCOPES=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^scopes=//p' | head -1)
    # 🔴 THE BRIEF ON THE CELL IS THE ONE THE ORACLE RESOLVED, NOT THE ONE THIS
    # SCRIPT WAS HANDED. The oracle derives the brief from the trial's own
    # transcript and refuses on a disagreement, so labelling the cell with
    # `$BRIEF` would be re-asserting the unverified request next to a verdict
    # measured under the verified one.
    R_BRIEF=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^brief=//p' | head -1)
    R_SRC=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^brief_source=//p' | head -1)
    # Which viewer the block was shown. A `no` graded against an anonymous
    # viewer is a different finding from a `no` graded against a signed-in one.
    R_VIEWER=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^viewer=//p' | head -1)
    # 🔴 WHICH ARM, AND IT IS CARRIED FOR A DIFFERENT REASON FROM THE FIELDS
    # ABOVE. The arm is chosen by an AMBIENT environment variable, so without this
    # a stale `CIVITAI_ASSERT_UNCONSENTED=1` in an operator's shell turns a whole
    # matrix into consent verdicts that read as ordinary render verdicts. A cell
    # saying `arm=unconsented` cannot be mistaken for one; a cell with no arm field
    # at all can.
    R_ARM=$(printf '%s\n' "$SUMMARY" | tr ' ' '\n' | sed -n 's/^arm=//p' | head -1)
    RENDER_FIELDS=" render_brief=${R_BRIEF:-$BRIEF} brief_source=${R_SRC:-unknown} validate_gate=${R_GATE:-unknown} scopes=${R_SCOPES:-unknown} viewer=${R_VIEWER:-unknown} arm=${R_ARM:-unknown} observed=${R_OBS:-} RENDER=${R_PASS:-unreadable}"
  fi
fi

printf 'agent=%s check_ok=%s failed_checks=[%s] mcp_rows=%s rows=[%s] login_version=%s agent_shell_version=%s CLOSING_CONDITION=%s%s\n' \
  "$AGENT_ID" "$OK" "${FAILED:-}" "$MCP_ROWS" "${ROWS:-}" "${LOGIN_VER:-none}" "${AGENT_VER:-none}" "$PASS" "$RENDER_FIELDS"
