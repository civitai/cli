#!/usr/bin/env bash
# The RENDER ORACLE. Serves a trial's built App Block inside the trial
# container, drives it with a headless browser on the host, and reports what the
# browser observed.
#
#   oracle.sh <trial-id> [container-user] [brief-name]
#
# The brief is DERIVED from the trial's own transcript. The third argument is an
# optional cross-check, not an input: when it disagrees with what the trial was
# run with, this refuses rather than picking one. See "WHICH BRIEF" below.
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
# NOT the brief — a REQUEST for one, reconciled below against what the trial was
# actually run with. There is deliberately no default here; the old
# `${3:-${DOGFOOD_ASSERT:-celsius}}` is the defect this section exists to close.
REQUESTED="${3:-${DOGFOOD_ASSERT:-}}"
C="dogfood-$TRIAL"
SERVER="$HERE/serve-block.mjs"
# Where runner.py wrote this trial's transcript. driver.sh runs it as
# `--out runs` from this directory, so that is the default; point DOGFOOD_RUNS
# elsewhere when the trial was driven from another checkout.
RUNS="${DOGFOOD_RUNS:-$HERE/runs}"
TRANSCRIPT="$RUNS/$TRIAL/transcript.jsonl"
# The brief a trial that demonstrably had NONE is graded against. Only ever
# reached when the transcript positively records an empty brief — i.e. a setup
# cell — and the run says so on its own summary line.
FALLBACK_BRIEF=celsius

fatal() { printf 'oracle: %s — nothing was measured (this is NOT a failing trial)\n' "$1" >&2; exit 2; }

[ -f "$SERVER" ] || fatal "missing $SERVER"

# ── the instrument, before any verdict ───────────────────────────────────────
command -v docker >/dev/null || fatal "no docker on PATH"
command -v node   >/dev/null || fatal "no node on PATH (the assertion runs on the HOST)"
command -v jq     >/dev/null || fatal "no jq on PATH (the brief is read out of the trial's transcript)"

# ── WHICH BRIEF: read off the trial, never defaulted in silence ──────────────
# 🔴 THE BRIEF USED TO BE AN OPTIONAL POSITIONAL DEFAULTING TO `celsius`, WHICH
# IS A GRADE AGAINST THE WRONG QUESTION WITH NOTHING TO NOTICE IT. Measured
# 2026-09-21: `oracle.sh ab-genpost-mimo-01 root` — a trial built from the
# genpost brief — ran the CELSIUS assertion, timed out waiting for
# `[data-testid="celsius"]`, and printed `RENDER=no`. That is byte-identical to
# the verdict a model that built nothing earns, and it was nearly reported as
# one.
#
# The trial knows the answer: runner.py records the brief in its `start` record.
# So the brief is DERIVED here, and an explicit argument that DISAGREES with the
# transcript is REFUSED rather than resolved — a disagreement means the operator
# and the transcript hold different beliefs about what was asked, and a verdict
# under either of them is worthless. Refusing exits 2 (nothing measured), which
# grade.sh already renders as `RENDER=unmeasured` and no reader can mistake for
# a statement about the block.
#
# 🔴 TWO WAYS TO IDENTIFY A BRIEF, AND THE NAME IS THE GOOD ONE. `start.brief`
# holds the brief's PROSE, not its name, so resolving it means matching that
# prose against `briefs/*.brief.txt` — which silently stops resolving every
# already-run trial the moment anyone rewords a brief file. runner.py therefore
# records `brief_name` too (`--brief-name`, added with this change) and that is
# preferred when present. The prose match remains for trials run before the
# field existed; it is exact, so it either resolves or refuses.
norm() { printf '%s' "${1-}" | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'; }

T_NAME=
T_TEXT=
HAVE_START=no
if [ -f "$TRANSCRIPT" ]; then
  # `head -1` after jq rather than `head -1` before it: a trial killed mid-write
  # leaves a half-written final line, and jq streams every well-formed record
  # ahead of it before complaining.
  #
  # 🔴 ONLY THE `start` RECORD, EVER. Everything else in a transcript is the
  # MODEL'S OWN OUTPUT and its terminal classification; a grader that branched
  # on any of it would be scoring a trial by what it said rather than by what
  # the container holds, which is the mistake grade.sh's AGENT_ID read exists to
  # avoid. Pinned by TestGradersDoNotReadTheStopVocabulary.
  START=$(jq -c 'select(.kind=="start")' "$TRANSCRIPT" 2>/dev/null | head -1)
  if [ -n "$START" ]; then
    HAVE_START=yes
    T_NAME=$(printf '%s' "$START" | jq -r '.brief_name // ""' 2>/dev/null)
    T_TEXT=$(norm "$(printf '%s' "$START" | jq -r '.brief // ""' 2>/dev/null)")
  fi
fi

DERIVED=
BRIEF_SOURCE=
if [ "$HAVE_START" = "yes" ] && [ -n "$T_NAME" ]; then
  DERIVED="$T_NAME"
  BRIEF_SOURCE=transcript-name
  # A transcript that names a brief AND carries prose that is not that brief's
  # is internally inconsistent — one of the two was written by hand. Refuse:
  # picking either would be guessing which half is the lie.
  NAMED_FILE="$HERE/briefs/$T_NAME.brief.txt"
  if [ -f "$NAMED_FILE" ] && [ -n "$T_TEXT" ] \
     && [ "$(norm "$(cat "$NAMED_FILE")")" != "$T_TEXT" ]; then
    fatal "$TRANSCRIPT is self-inconsistent: it records brief_name='$T_NAME' but its brief TEXT is not the contents of $NAMED_FILE"
  fi
elif [ "$HAVE_START" = "yes" ] && [ -n "$T_TEXT" ]; then
  HITS=()
  for f in "$HERE"/briefs/*.brief.txt; do
    [ -f "$f" ] || continue
    n=$(basename "$f" .brief.txt)
    [ "$(norm "$(cat "$f")")" = "$T_TEXT" ] && HITS+=("$n")
  done
  case "${#HITS[@]}" in
    1) DERIVED="${HITS[0]}"; BRIEF_SOURCE=transcript-text ;;
    0) fatal "the brief recorded in $TRANSCRIPT matches none of $HERE/briefs/*.brief.txt — either a brief file was reworded after this trial ran (re-run it, or grade it from the trial's own brief text) or this trial was run with an ad-hoc brief that has no assertion" ;;
    *) fatal "the brief recorded in $TRANSCRIPT matches ${#HITS[@]} brief files (${HITS[*]}) — two briefs carry the same text, so the trial cannot be attributed to either" ;;
  esac
fi

# Reconcile. The disagreement branch is the whole point of this block.
BRIEF_NOTE=
if [ -n "$REQUESTED" ] && [ -n "$DERIVED" ] && [ "$REQUESTED" != "$DERIVED" ]; then
  fatal "brief disagreement: you asked for '$REQUESTED' but $TRANSCRIPT records '$DERIVED' ($BRIEF_SOURCE). Refusing to grade — one of the two is wrong and a verdict under either is worthless"
fi
if [ -n "$DERIVED" ]; then
  BRIEF="$DERIVED"
elif [ -n "$REQUESTED" ]; then
  BRIEF="$REQUESTED"
  if [ "$HAVE_START" = "yes" ]; then
    BRIEF_SOURCE=argument-trial-recorded-no-brief
    BRIEF_NOTE="⚠ $TRANSCRIPT records NO brief for this trial (a setup cell, or a runner that predates --brief), so nothing verified '$REQUESTED'."
  else
    BRIEF_SOURCE=argument-unverified
    BRIEF_NOTE="⚠ no transcript for '$TRIAL' at $TRANSCRIPT (set DOGFOOD_RUNS to the directory the trial was driven from), so nothing verified '$REQUESTED'."
  fi
elif [ "$HAVE_START" = "yes" ]; then
  # The case the arc's setup cells are in: the transcript POSITIVELY records an
  # empty brief, so the trial built no app and every brief grades it the same
  # `no app was created`. Defensible — and said out loud rather than assumed.
  BRIEF="$FALLBACK_BRIEF"
  BRIEF_SOURCE=default-trial-recorded-no-brief
  BRIEF_NOTE="⚠ $TRANSCRIPT records an EMPTY brief — this is a setup trial that built no app. Grading it against the default '$FALLBACK_BRIEF' assertion; the verdict is 'no app was created', not a statement about any brief."
else
  fatal "no brief and no way to derive one: nothing was passed and no transcript for '$TRIAL' exists at $TRANSCRIPT (set DOGFOOD_RUNS to the directory the trial was driven from). Refusing to guess — a grade against the wrong brief is a confident \`no\` that reads as a model which built nothing"
fi

ASSERT="$HERE/briefs/$BRIEF.assert.mjs"
[ -f "$ASSERT" ] || fatal "no assertion for brief '$BRIEF' at $ASSERT"

# 🔴 RESOLVE THE BROWSER HERE, NOT BY LETTING THE ASSERTION FAIL LATER. Without
# a browser the assertion exits 2 with its own message, but by then a server is
# running inside someone's container and the reason is three layers down. A
# trial image ships no Chromium — that is why the browser is the host's.
#
# 🔴 THE ORDER IS RELEASE-BUILDS-FIRST AND IT IS PINNED. `ubuntu-latest` carries
# a release `google-chrome` AND a raw `chromium-browser-snapshots` build at
# `/usr/bin/chromium`; naming chromium first drove every CI run on an
# un-release-qualified trunk snapshot. The same list lives in
# `scripts/dogfood/briefs/_cdp.mjs`, `.github/workflows/ci.yml` and
# `dogfood_oracle_test.go`, and
# `TestEveryBrowserResolverAgreesOnTheSameOrder` fails when they drift.
BROWSER="${CIVITAI_CHROME:-}"
if [ -z "$BROWSER" ]; then
  for b in google-chrome google-chrome-stable chromium chromium-browser chrome; do
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

# Deliberately NOT prefixed `brief=`: that prefix is how grade.sh and the Go
# tests find the SUMMARY line, and a second line starting with it would be read
# as one.
printf -- '--- brief resolution\n'
printf 'resolved_brief=%s source=%s requested=%s transcript=%s\n' \
  "$BRIEF" "$BRIEF_SOURCE" "${REQUESTED:-none}" \
  "$([ "$HAVE_START" = "yes" ] && printf '%s' "$TRANSCRIPT" || printf '(none at %s)' "$TRANSCRIPT")"
[ -n "$BRIEF_NOTE" ] && printf '%s\n' "$BRIEF_NOTE"

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
# verdict (see briefs/genpost.md), not because it decides the verdict.
#
# 🔴 IT IS ALSO AN INPUT TO THE HOST EMULATION, AND THOSE ARE DIFFERENT CLAIMS.
# The list is handed to the assertion, which seeds it as the bootstrap's
# `token.scopes` so that a block gating generation on a granted scope takes the
# granted BRANCH rather than the refused one. That is what a real host does — the
# platform's grant derives from this same declaration at review time. It still
# decides no verdict: `TestOracleReportsScopesWithoutDeciding` pins that a
# manifest declaring nothing can grade `yes` and one declaring both can grade
# `no`, and `token.raw` stays empty so nothing can COMPLETE here either way.
# Measured 2026-09-21 on `ab-genpost-dsv4-01`: with `scopes: []` seeded
# unconditionally, a correct consent-first app never left `ready` and the cell
# read `RENDER=no` about the harness. See briefs/_cdp.mjs `hostBootstrap`.
SCOPES=
# The machine-readable half of the same read: a comma-separated list, EMPTY when
# the manifest declares none (where `$SCOPES` renders the word `none` for a human
# reader). One jq, two renderings — a second parser would let the cell's
# `scopes=` field and the block's `token.scopes` disagree silently.
SCOPES_CSV=
AVIEWER=
# Which arm the assertion graded, and the two scope readings the seam guards use.
# Declared here so the summary line renders `unmeasured` rather than tripping
# `set -u` when nothing was served.
AARM=
ADECL=

if [ -z "$APP_DIR" ]; then
  REASON="no block.manifest.json under /work — no app was created"
else
  # Read the manifest ONCE. It used to be three `docker exec cat`s, and a fourth
  # was about to be added for the scope seed below — at which point the cell's
  # `scopes=` field and the list the block is shown would have been two
  # independent reads of one file.
  MANIFEST=$(x "cat '$APP_DIR/block.manifest.json'")
  OUTDIR=$(printf '%s' "$MANIFEST" | jq -r '.outputDir // empty' 2>/dev/null)
  BUILDCMD=$(printf '%s' "$MANIFEST" | jq -r '.buildCommand // empty' 2>/dev/null)
  SCOPES_CSV=$(printf '%s' "$MANIFEST" | jq -r '(.scopes // []) | join(",")' 2>/dev/null)
  # `none` (not an empty field) when the key is absent or empty, so a reader can
  # tell "declared no scopes" from "this oracle predates the field".
  SCOPES="${SCOPES_CSV:-none}"
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

  printf -- '--- assertion: %s (scopes=%s)\n' "$BRIEF" "$SCOPES"
  # 🔴 SAY IT OUT LOUD WHEN THE ARM IS NOT THE DEFAULT. The arm is selected by an
  # ambient environment variable, so a stale export in an operator's shell would
  # otherwise silently turn every cell of a matrix into a consent verdict wearing
  # an ordinary cell's clothes. `arm=` on the summary line is the machine-readable
  # half; this is the half a human scrolling the run sees.
  [ "${CIVITAI_ASSERT_UNCONSENTED:-}" = "1" ] && printf '⚠ UNCONSENTED ARM: token.scopes is seeded EMPTY and the verdict is "did the block ASK the host for consent", NOT "did it reach generating". This is a different question from the default arm; see briefs/genpost.md.\n'
  # 🔴 THE SCOPE LIST IS AN ARGUMENT, NOT AN ENVIRONMENT VARIABLE. It is data
  # about THIS block, derived from THIS container's manifest, so it belongs on
  # the call that grades that block — an env var would be ambient state a
  # concurrent run, a stale export or an operator's shell could set, and a
  # bootstrap seeded from an operator's shell is not a measurement of the app.
  OUT=$(CIVITAI_CHROME="$BROWSER" node "$ASSERT" "$URL" "$SCOPES_CSV" 2>&1)
  ARC=$?
  printf '%s\n' "$OUT"
  # Exit 2 is the assertion's own "the harness could not run" code, and it must
  # not become a verdict about the block.
  [ "$ARC" = "2" ] && fatal "the assertion could not run (exit 2): $OUT"
  JSON=$(printf '%s\n' "$OUT" | grep -a '^{' | tail -1)
  OBSERVED=$(printf '%s' "$JSON" | jq -r 'if has("observed") then (.observed|tostring) else "" end' 2>/dev/null)
  # Which viewer the block was actually shown, read back from the assertion
  # rather than re-derived here: one rule, one place. Without it a cell cannot
  # tell "the model built nothing" from "the model built a sign-in CTA and the
  # harness was anonymous" — the reading that cost `ab-genpost-mimo-01` a
  # verdict on 2026-09-21.
  AVIEWER=$(printf '%s' "$JSON" | jq -r '.hostViewer // empty' 2>/dev/null)
  # Which ARM the assertion graded, read back rather than re-derived from this
  # script's own environment: the env var is read in `_cdp.mjs`, so a cell that
  # trusted `$CIVITAI_ASSERT_UNCONSENTED` here would be reporting what the
  # OPERATOR asked for rather than what the block was shown.
  AARM=$(printf '%s' "$JSON" | jq -r '.hostArm // empty' 2>/dev/null)
  # 🔴 SEAM GUARD, NOW IN TWO HALVES BECAUSE THE ARM MADE THEM TWO FACTS.
  # `scopes=` on the cell is what the MANIFEST declares. `hostScopesDeclared` is
  # the list that crossed the process boundary as argv[3] — it must always equal
  # the manifest, whatever arm is running, and that is what catches an assertion
  # which quietly ignored its argument (the original hazard: a confident cell
  # whose `scopes=` field describes a list the block never received).
  # `hostScopes` is what the BLOCK WAS SHOWN, which the unconsented arm empties on
  # purpose. Both render an empty list as `none`, so they compare directly. A
  # disagreement is a defect in this harness, so it exits 2 (nothing was measured)
  # rather than emitting a verdict.
  ADECL=$(printf '%s' "$JSON" | jq -r '.hostScopesDeclared // empty' 2>/dev/null)
  ASCOPES=$(printf '%s' "$JSON" | jq -r '.hostScopes // empty' 2>/dev/null)
  if [ -n "$ADECL" ] && [ "$ADECL" != "$SCOPES" ]; then
    fatal "scope seam mismatch: the manifest declares '$SCOPES' but the assertion received '$ADECL'"
  fi
  # 🔴 AND THE ARM'S OWN SEAM, WHICH IS THE ONE THAT WOULD MAKE A NEW ARM GREEN
  # VACUOUSLY. An `unconsented` run whose block was shown the manifest's scopes is
  # the DEFAULT arm wearing the unconsented arm's label: its consent verdict would
  # be earned on an already-consented token, i.e. a measurement of nothing. The
  # mirror case is a `consented` run that was shown less than the manifest
  # declares, which is #690 silently undone.
  case "$AARM" in
    unconsented)
      [ "$ASCOPES" = "none" ] || fatal "arm seam mismatch: the assertion reports arm=unconsented but the block was shown scopes '$ASCOPES' — an unconsented arm that granted the block its scopes measures nothing" ;;
    consented)
      [ "$ASCOPES" = "$SCOPES" ] || fatal "arm seam mismatch: the assertion reports arm=consented but presented '$ASCOPES' where the manifest declares '$SCOPES'" ;;
    '') : ;;
    *) fatal "the assertion reported an unknown arm '$AARM'" ;;
  esac
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
# 🔴 `arm=` IS ON THE SUMMARY LINE, NOT ONLY IN THE ASSERTION'S JSON, BECAUSE THE
# ARM IS SET BY AN ENVIRONMENT VARIABLE AND A CELL IS READ OUT OF CONTEXT. Two
# runs of the same trial now legitimately disagree — `yes` on the consented arm
# and `no` on the unconsented one is the CORRECT reading of `ab-ship-mimo-02` —
# so a verdict without this field does not say which question it answers. It is
# read back from the assertion rather than from this script's own environment,
# so it describes the run that happened.
printf 'brief=%s brief_source=%s app_dirs=%s app_dir=%s gate=%s gate_rc=%s scopes=%s viewer=%s arm=%s served=%s observed=%s RENDER=%s\n' \
  "$BRIEF" "$BRIEF_SOURCE" "$APP_COUNT" "${APP_DIR:-none}" "$GATE" "${GATE_RC:-none}" "${SCOPES:-none}" \
  "${AVIEWER:-unmeasured}" "${AARM:-unmeasured}" "${SERVED:-none}" "$(printf '%q' "${OBSERVED:-}")" "$RENDER_PASS"
exit 0
