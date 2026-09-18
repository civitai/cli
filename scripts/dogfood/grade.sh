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
LOGIN_OUT=$(x "zsh -lic 'civitai --version'")
LOGIN_RC=$(xrc "zsh -lic 'civitai --version'")
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

PASS=no
if [ "$OK" = "true" ] && [ -n "$LOGIN_VER" ] && [ "$LOGIN_VER" = "$AGENT_VER" ] \
   && [ "$MCP_ROWS" = "2" ]; then PASS=yes; fi
printf 'check_ok=%s failed_checks=[%s] mcp_rows=%s rows=[%s] login_version=%s agent_shell_version=%s CLOSING_CONDITION=%s\n' \
  "$OK" "${FAILED:-}" "$MCP_ROWS" "${ROWS:-}" "${LOGIN_VER:-none}" "${AGENT_VER:-none}" "$PASS"
