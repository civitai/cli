#!/usr/bin/env bash
# The RENDER ORACLE. Serves a trial's built App Block inside the trial
# container, drives it with a headless browser on the host, and reports what the
# browser observed.
#
#   oracle.sh <trial-id> [container-user] [brief-name]
#
# Exit 0 = something was measured (pass or fail). Exit 2 = NOTHING was measured.
# That split is the whole point: a harness that cannot reach the block must not
# emit the same verdict as a block that does not work.
#
# 🔴 WHY THIS EXISTS AT ALL, AND WHY `civitai app validate` IS NOT IT.
# The app-build arc's closing condition is frozen on a browser observing a
# running block. `civitai app validate` PASSES on an untouched `civitai app init`
# scaffold — measured, `✓ <dir> is valid` — so a validate-keyed grade cannot tell
# `scaffolded` from `built` and would score every cell green while measuring
# nothing. It is also shipped by the tool under test, so a defect in it would
# grade itself. It runs here because it is cheap and offline, its result is
# REPORTED, and it decides nothing.
#
# ⚠ It deliberately does NOT short-circuit on a failing gate. "Fail-fast" is
# about ordering and cost, not about authority: aborting the render when the
# validator is unhappy would substitute the validator for the verdict in exactly
# the case where the verdict matters, and would throw away the `observed` string
# — the one field that separates a near-miss (`212 °F`) from nothing at all.
#
# 🔴 IT NEVER MUTATES THE TRIAL'S WORK. It writes one file to the container's
# /tmp, starts one node process, and kills that process by its REPORTED PID on
# the way out. /work is read only. No `docker rm`, `docker stop` or `docker
# commit` appears in this file, deliberately: a graded container is evidence, and
# re-creating one costs an OpenRouter trial.
set -uo pipefail

HERE="$(dirname "$0")"
# Guard the VALUE, not just the cd — `cd ""` is a silent no-op on bash <= 5.2, so
# `cd "$X" || exit` sails past an empty $X and runs against the inherited cwd.
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 2; }

TRIAL="${1:?trial id}"
U="${2:-root}"
BRIEF="${3:-${DOGFOOD_ASSERT:-celsius}}"
C="dogfood-$TRIAL"
ASSERT="$HERE/briefs/$BRIEF.assert.mjs"
SERVER="$HERE/serve-block.mjs"

fatal() { printf 'oracle: %s — nothing was measured (this is NOT a failing trial)\n' "$1" >&2; exit 2; }

[ -f "$ASSERT" ] || fatal "no assertion for brief '$BRIEF' at $ASSERT"
[ -f "$SERVER" ] || fatal "missing $SERVER"

# ── the instrument, before any verdict ───────────────────────────────────────
command -v docker >/dev/null || fatal "no docker on PATH"
command -v node   >/dev/null || fatal "no node on PATH (the assertion runs on the HOST)"

# 🔴 RESOLVE THE BROWSER HERE, NOT BY LETTING THE ASSERTION FAIL LATER. Without
# a browser the assertion exits 2 with its own message, but by then a server is
# running inside someone's container and the reason is three layers down. A
# trial image ships no Chromium — that is why the browser is the host's.
BROWSER="${CIVITAI_CHROME:-}"
if [ -z "$BROWSER" ]; then
  for b in chromium chromium-browser google-chrome google-chrome-stable chrome; do
    if command -v "$b" >/dev/null 2>&1; then BROWSER="$(command -v "$b")"; break; fi
  done
fi
[ -n "$BROWSER" ] || fatal "no Chromium on PATH; set CIVITAI_CHROME to a browser binary"

# Same guard grade.sh carries, and for the same reason: `docker inspect` succeeds
# for a container in ANY state, and against a stopped one every exec fails with
# empty stdout — which reads exactly like a block that rendered nothing.
STATE=$(docker inspect -f '{{.State.Status}}' "$C" 2>/dev/null)
[ -n "$STATE" ] || fatal "no such container: $C"
[ "$STATE" = "running" ] || fatal "container $C is $STATE, not running"

x() { docker exec -u "$U" -w /work "$C" bash -lc "$1" 2>/dev/null; }

docker exec "$C" sh -c 'command -v node >/dev/null' \
  || fatal "no node inside $C — the block cannot be served from where it was built"

printf '=== render oracle: %s (brief=%s) ===\n' "$TRIAL" "$BRIEF"

# ── locate the app ───────────────────────────────────────────────────────────
# The app is wherever the model put a manifest. Reading the trial id or guessing
# a directory name would be an assertion by whoever named the trial rather than a
# measurement of the box — the same mistake grade.sh's AGENT_ID read exists to
# avoid. node_modules is excluded: a dependency shipping a manifest of its own
# would otherwise out-sort the real app on a deep path.
MANIFESTS=$(x 'find /work -maxdepth 4 -name block.manifest.json -not -path "*/node_modules/*" 2>/dev/null | sort')
APP_COUNT=$(printf '%s' "$MANIFESTS" | grep -c . || true)
APP_DIR=""
[ "$APP_COUNT" -gt 0 ] && APP_DIR=$(printf '%s\n' "$MANIFESTS" | head -1 | xargs dirname)

printf -- '--- app discovery\n'
printf 'manifests=%s\n%s\n' "$APP_COUNT" "${MANIFESTS:-(none)}"

# ── A: the offline gate. Reported. Never the verdict. ────────────────────────
printf -- '--- gate: civitai app validate (reported, NOT the verdict)\n'
GATE=unavailable
GATE_RC=
if [ -n "$APP_DIR" ] && x 'command -v civitai >/dev/null'; then
  GATE_OUT=$(docker exec -u "$U" -w /work "$C" bash -lc "civitai app validate '$APP_DIR'" 2>&1)
  docker exec -u "$U" -w /work "$C" bash -lc "civitai app validate '$APP_DIR'" >/dev/null 2>&1
  GATE_RC=$?
  printf '%s\nrc=%s\n' "$GATE_OUT" "$GATE_RC"
  [ "$GATE_RC" = "0" ] && GATE=pass || GATE=fail
else
  printf 'skipped: %s\n' \
    "$([ -n "$APP_DIR" ] && echo 'no civitai on PATH in the container' || echo 'no app directory')"
fi

# ── B: what to serve ─────────────────────────────────────────────────────────
# A manifest with a buildCommand declares its output in `outputDir`; a no-build
# app (the `static` template) IS its own directory. Resolve to whichever holds an
# index.html, and say which — "the app was never built" and "the app renders
# nothing" are different findings and must not collapse into one `no`.
SERVED=""
RENDER_PASS=no
OBSERVED=
REASON=
OUTDIR=
BUILDCMD=
# 🔴 REPORTED, NEVER THE VERDICT — the same standing as the validate gate. A
# manifest is a DECLARATION, not a behaviour: a block can declare
# `posts:write:self` and post nothing, and the platform grants scopes at review
# rather than at manifest time. It is on the cell because a generate-then-post
# app declaring only `ai:write:budgeted` is worth seeing next to the render
# verdict (see briefs/genpost.md), not because it decides anything.
SCOPES=

if [ -z "$APP_DIR" ]; then
  REASON="no block.manifest.json under /work — no app was created"
else
  OUTDIR=$(x "cat '$APP_DIR/block.manifest.json'" | jq -r '.outputDir // empty' 2>/dev/null)
  BUILDCMD=$(x "cat '$APP_DIR/block.manifest.json'" | jq -r '.buildCommand // empty' 2>/dev/null)
  # `none` (not an empty field) when the key is absent or empty, so a reader can
  # tell "declared no scopes" from "this oracle predates the field".
  SCOPES=$(x "cat '$APP_DIR/block.manifest.json'" | jq -r 'if (.scopes // []) | length > 0 then (.scopes | join(",")) else "none" end' 2>/dev/null)
  CAND="$APP_DIR"
  [ -n "$OUTDIR" ] && CAND="$APP_DIR/$OUTDIR"
  if x "test -f '$CAND/index.html'"; then
    SERVED="$CAND"
  elif [ "$CAND" != "$APP_DIR" ] && x "test -f '$APP_DIR/index.html'"; then
    # Declared an outputDir, did not produce one, but the source tree has an
    # entry document. Serve it and SAY SO — this is the shape a model that wrote
    # the app and never ran the build leaves behind.
    SERVED="$APP_DIR"
    REASON="outputDir '$OUTDIR' has no index.html; served the source tree instead"
  else
    REASON="no index.html in $CAND${BUILDCMD:+ (buildCommand '$BUILDCMD' declared — the app was never built)}"
  fi
fi

printf -- '--- serving\n'
printf 'app_dir=%s outputDir=%s served=%s\n' "${APP_DIR:-none}" "${OUTDIR:-none}" "${SERVED:-none}"

# ── C: serve it, inside the container, and drive it from the host ────────────
SRV_PID=
cleanup() {
  # Kill by the PID the server REPORTED, never by a `-f` pattern: a pattern
  # matches any sibling node process, including this same server started by a
  # concurrent oracle run against another trial.
  if [ -n "$SRV_PID" ] && printf '%s' "$SRV_PID" | grep -qE '^[0-9]+$'; then
    docker exec "$C" sh -c "kill $SRV_PID" >/dev/null 2>&1
  fi
}
trap cleanup EXIT

if [ -n "$SERVED" ]; then
  docker cp "$SERVER" "$C:/tmp/dogfood-serve-block.mjs" >/dev/null 2>&1 \
    || fatal "could not copy the server into $C"
  docker exec "$C" sh -c 'rm -f /tmp/dogfood-serve.log' >/dev/null 2>&1
  # `-d`, not a backgrounded `&` inside a foreground exec: the detached form is
  # what docker provides for this, and it returns immediately without leaving a
  # client attached to a pipe nobody reads.
  docker exec -d "$C" sh -c \
    "node /tmp/dogfood-serve-block.mjs '$SERVED' >/tmp/dogfood-serve.log 2>&1" \
    >/dev/null 2>&1

  PORT=
  for _ in $(seq 1 50); do
    LOG=$(docker exec "$C" sh -c 'cat /tmp/dogfood-serve.log 2>/dev/null')
    PORT=$(printf '%s\n' "$LOG" | sed -n 's/^LISTENING \([0-9]\{1,\}\)$/\1/p' | head -1)
    SRV_PID=$(printf '%s\n' "$LOG" | sed -n 's/^PID \([0-9]\{1,\}\)$/\1/p' | head -1)
    [ -n "$PORT" ] && break
    sleep 0.2
  done
  [ -n "$PORT" ] || fatal "the in-container server never reported a port: $(docker exec "$C" sh -c 'cat /tmp/dogfood-serve.log 2>/dev/null')"

  IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{if .IPAddress}}{{.IPAddress}} {{end}}{{end}}' "$C" 2>/dev/null | awk '{print $1}')
  [ -n "$IP" ] || fatal "container $C has no reachable IP address"
  URL="http://$IP:$PORT/"
  printf 'url=%s pid=%s\n' "$URL" "${SRV_PID:-unknown}"

  # 🔴 PREFLIGHT, SO AN UNREACHABLE SERVER IS NOT GRADED AS A BROKEN BLOCK.
  # Without it the assertion navigates to a dead address, times out waiting for
  # the input, and emits the same `pass:false` a real failure does.
  REACHED=no
  for _ in $(seq 1 25); do
    if node -e 'fetch(process.argv[1]).then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))' "$URL" 2>/dev/null; then
      REACHED=yes; break
    fi
    sleep 0.2
  done
  [ "$REACHED" = "yes" ] || fatal "served $SERVED in $C but could not reach $URL from the host"

  printf -- '--- assertion: %s\n' "$BRIEF"
  OUT=$(CIVITAI_CHROME="$BROWSER" node "$ASSERT" "$URL" 2>&1)
  ARC=$?
  printf '%s\n' "$OUT"
  # Exit 2 is the assertion's own "the harness could not run" code, and it must
  # not become a verdict about the block.
  [ "$ARC" = "2" ] && fatal "the assertion could not run (exit 2): $OUT"
  JSON=$(printf '%s\n' "$OUT" | grep -a '^{' | tail -1)
  OBSERVED=$(printf '%s' "$JSON" | jq -r 'if has("observed") then (.observed|tostring) else "" end' 2>/dev/null)
  AREASON=$(printf '%s' "$JSON" | jq -r '.reason // empty' 2>/dev/null)
  APASS=$(printf '%s' "$JSON" | jq -r 'if has("pass") then (.pass|tostring) else "absent" end' 2>/dev/null)
  [ "$APASS" = "true" ] && RENDER_PASS=yes
  [ -n "$AREASON" ] && REASON="$AREASON"
  # A `pass` key that never arrived means the assertion printed something this
  # script cannot read — which is a statement about the instrument, not the block.
  [ "$APASS" = "absent" ] && fatal "the assertion printed no readable verdict: $OUT"
fi

printf -- '--- render verdict\n'
# The reason gets its own line because it is free-form prose from the assertion:
# on the summary line it would need shell-quoting and would bury the fields a
# reader greps for. `observed` stays ON the summary line — 🔴 it is the field the
# brief's own doc requires an oracle to carry through, because the assertion is
# strict (`212 °F` fails where `212` passes) and a bare `no` cannot tell a
# near-miss from a block that rendered nothing.
printf 'render_reason=%s\n' "${REASON:-none}"
printf 'brief=%s app_dirs=%s app_dir=%s gate=%s gate_rc=%s scopes=%s served=%s observed=%s RENDER=%s\n' \
  "$BRIEF" "$APP_COUNT" "${APP_DIR:-none}" "$GATE" "${GATE_RC:-none}" "${SCOPES:-none}" \
  "${SERVED:-none}" "$(printf '%q' "${OBSERVED:-}")" "$RENDER_PASS"
exit 0
