#!/usr/bin/env bash
# Grade one finished trial against the arc's frozen closing condition, by
# measuring the container — never by reading what the agent said it did.
#
#   grade.sh <trial-id> <container-user>
#
# Both halves are required:
#   A. `civitai agent-setup --check --json` reports ok: true
#   B. `zsh -lic 'civitai --version'` prints a version in the USER's LOGIN shell
#      (the half that was green-by-accident once: ok:true inside the agent's own
#      shell while the login shell still served the old binary).
set -uo pipefail
TRIAL="${1:?trial id}"
U="${2:-root}"
C="dogfood-$TRIAL"

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
# stdout ONLY. `--check` prints its error line to stderr AHEAD of the JSON, so a
# merged capture is unparseable and jq's failure reads as "no JSON at all".
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
printf 'check_ok=%s failed_checks=[%s] mcp_rows=%s rows=[%s] login_version=%s agent_shell_version=%s CLOSING_CONDITION=%s\n' \
  "$OK" "${FAILED:-}" "$MCP_ROWS" "${ROWS:-}" "${LOGIN_VER:-none}" "${AGENT_VER:-none}" "$PASS"
