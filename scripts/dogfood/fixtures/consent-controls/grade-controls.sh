#!/usr/bin/env bash
# Grade the three consent controls on BOTH arms and print the matrix.
#
#   bash grade-controls.sh
#
# Writes a synthetic trial transcript per fixture (the oracle derives the brief
# from one — it refuses to guess) and then runs `oracle.sh` twice per fixture.
# Run `build.sh` first; this grades containers, it does not create them.
#
# 🔴 READ BOTH ARMS, ALWAYS. A single arm cannot express what these fixtures are
# for. `ctl-genpost-blind` is `yes` consented and `no` unconsented — quoting
# either alone describes a different app than the one that exists.
#
# 🔴 AND READ THE REASON, NOT THE VERDICT. Every cell here can print `RENDER=no`;
# only the reason says whether the arm did its job. The whole point of these
# fixtures is that `ctl-scaffold-untouched` and `ctl-genpost-blind` both fail the
# unconsented arm and the failures mean opposite things — one is the assertion
# throwing before the consent step is reached, the other is the consent verdict
# itself. A matrix of bare yes/no would score them the same.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 2; }
DOGFOOD="$(cd "$HERE/../.." && pwd)"

BRIEF_NAME=genpost
BRIEF_FILE="$DOGFOOD/briefs/$BRIEF_NAME.brief.txt"
RUNS="${DOGFOOD_RUNS:-$HOME/.cache/dogfood-consent-controls}"
FIXTURES="ctl-scaffold-untouched ctl-genpost-blind ctl-genpost-asks"

fatal() { printf 'grade-controls.sh: %s\n' "$1" >&2; exit 2; }

[ -f "$BRIEF_FILE" ] || fatal "no brief at $BRIEF_FILE"
# python3 has TWO jobs here: it writes the transcripts below AND reads the arm
# out of the assertion's json. Swapping either one to another tool does not
# retire this guard.
command -v python3 >/dev/null || fatal "no python3 on PATH (it writes the transcripts and reads the graded arm)"
command -v docker  >/dev/null || fatal "no docker on PATH"

# The browser is the HOST's — a trial image ships none. Resolved here only to
# fail early with a sentence naming the cause; oracle.sh resolves it again.
BROWSER="${CIVITAI_CHROME:-}"
if [ -z "$BROWSER" ]; then
  for b in google-chrome google-chrome-stable chromium chromium-browser chrome; do
    if command -v "$b" >/dev/null 2>&1; then BROWSER="$(command -v "$b")"; break; fi
  done
fi
[ -n "$BROWSER" ] || fatal "no Chromium on PATH; set CIVITAI_CHROME, or run this under \`nix-shell -p chromium\`"

# 🔴 THE TRANSCRIPT IS SYNTHETIC AND SAYS SO. `model: operator-control` is not
# decoration: `oracle.sh` reads ONLY the `start` record and nothing branches on
# this field, but a reader who finds these runs later must not mistake a
# hand-built control for a trial some model drove. The brief text is read from
# the committed file rather than pasted, because the oracle refuses a transcript
# whose `brief_name` and brief prose disagree.
for t in $FIXTURES; do
  mkdir -p "$RUNS/$t"
  # BRIEF_NAME crosses into python as an env var rather than being retyped as a
  # literal: the oracle REFUSES a transcript whose `brief_name` and brief prose
  # disagree, so two copies of this constant would make changing the shell
  # variable alone produce a transcript that is rejected as self-inconsistent.
  BRIEF_FILE="$BRIEF_FILE" BRIEF_NAME="$BRIEF_NAME" TRIAL="$t" \
  OUT="$RUNS/$t/transcript.jsonl" python3 -c '
import json, os, pathlib
brief = pathlib.Path(os.environ["BRIEF_FILE"]).read_text()
trial = os.environ["TRIAL"]
start = {"t": 1790400000.0, "kind": "start", "trial": trial, "model": "operator-control",
         "image": "df-node-root", "user": "root", "agent_env": "",
         "brief_name": os.environ["BRIEF_NAME"], "brief": brief,
         "container": "dogfood-" + trial, "credentialed": False}
end = {"t": 1790400600.0, "kind": "end", "stop": "finished", "finish_reason": "stop",
       "steps": 0, "usage": {"prompt_tokens": 0, "completion_tokens": 0, "cost": 0.0}}
pathlib.Path(os.environ["OUT"]).write_text(json.dumps(start) + "\n" + json.dumps(end) + "\n")
' || fatal "could not write the transcript for $t"
done

printf '=== consent controls, both arms (runs under %s)\n\n' "$RUNS"

# 🔴 CLEAR EVERY ARM KNOB ON BOTH BRANCHES — SELECTING AN ARM IS NOT ENOUGH.
# The arms are chosen by AMBIENT environment variables, so the `consented` branch
# is not "the default arm": it is "whatever the operator's shell exports". A stale
# `export CIVITAI_ASSERT_UNCONSENTED=1` makes the row this script prints under the
# literal word `consented` an UNCONSENTED measurement, and the reader has no way
# to see it except by noticing `arm=` inside the pasted summary line. This script
# is the first tool in this tree that prints its own arm COLUMN, so it is the one
# that has to defend it. The rule is not new — `dogfood_oracle_test.go` clears the
# same five by name, and says why in a comment; `oracle.sh` names the same hazard.
#
# ⚠ THE TIMING KNOBS RIDE ALONG, and they are a quieter version of the same
# class: `../../briefs/_cdp.mjs` also reads CIVITAI_ASSERT_WAIT_MS / _LAUNCH_MS /
# _LAUNCH_ATTEMPTS, all three documented as operator-settable in
# `../../README.md`. A stale `CIVITAI_ASSERT_WAIT_MS=1` yields a uniformly
# `RENDER=no` matrix. Less dangerous than the arm knobs — `render_reason=` shows
# a timeout, so it is visible rather than silent — but there is no reason to
# inherit it either.
ARM_ENV="env -u CIVITAI_ASSERT_UNCONSENTED -u CIVITAI_ASSERT_ANON_VIEWER \
         -u CIVITAI_ASSERT_NO_HOST -u CIVITAI_ASSERT_NO_HOST_PICKS -u DOGFOOD_ASSERT \
         -u CIVITAI_ASSERT_WAIT_MS -u CIVITAI_ASSERT_LAUNCH_MS \
         -u CIVITAI_ASSERT_LAUNCH_ATTEMPTS"

UNMEASURED=0
MISLABELLED=0

for t in $FIXTURES; do
  for arm in consented unconsented; do
    if [ "$arm" = unconsented ]; then
      OUT=$( cd "$DOGFOOD" && $ARM_ENV CIVITAI_CHROME="$BROWSER" DOGFOOD_RUNS="$RUNS" \
               CIVITAI_ASSERT_UNCONSENTED=1 bash oracle.sh "$t" root 2>&1 )
    else
      OUT=$( cd "$DOGFOOD" && $ARM_ENV CIVITAI_CHROME="$BROWSER" DOGFOOD_RUNS="$RUNS" \
               bash oracle.sh "$t" root 2>&1 )
    fi
    RC=$?
    mkdir -p "$RUNS/$t"
    printf '%s\n' "$OUT" > "$RUNS/$t/oracle.$arm.txt"
    VERDICT=$(printf '%s\n' "$OUT" | grep -a '^brief=' | tail -1)
    REASON=$(printf '%s\n' "$OUT" | grep -a '^render_reason=' | tail -1)
    if [ -z "$VERDICT" ]; then
      UNMEASURED=$((UNMEASURED + 1))
      # Report the oracle's ACTUAL exit code and its own last stderr line rather
      # than asserting `exited 2` — which was never captured, and would be a
      # claim about a number this script had not read.
      printf '%-24s %-12s %s\n' "$t" "$arm" \
        "<NOTHING MEASURED — oracle.sh exited $RC with no verdict line>"
      printf '%-24s %-12s   %s\n' '' '' \
        "$(printf '%s\n' "$OUT" | grep -a '^oracle: ' | tail -1)"
      continue
    fi
    # 🔴 ASSERT THE ARM THE ORACLE REPORTS EQUALS THE ONE THIS COLUMN CLAIMS.
    # The `env -u` above removes the known route to a mislabelled row; this
    # catches every other one, including a future arm knob nobody added here.
    # 🔴 READ IT OUT OF THE ASSERTION'S JSON, NOT OUT OF THE SUMMARY LINE.
    # Two regexes over that line were tried and BOTH could be defeated, because
    # the line interleaves oracle-owned fields with app-controlled ones
    # (`observed`, and `scopes`/`app_dir`, which come from the block's own
    # manifest and directory name) and every `.*` is greedy. The last attempt
    # excluded `\` on the theory that `printf %q` backslashes every literal
    # space — but `%q` switches to ANSI-C `$'…'` form as soon as the string holds
    # a control character, and in THAT form spaces are not escaped at all:
    #
    #   printf '%q' 'a b'            -> a\ b
    #   printf '%q' "$(printf 'x\ty b')" -> $'x\ty b'      <- space survives
    #
    # Status text is `textContent.trim()`, so an interior newline or tab is
    # ordinary markup, and a block could then spell ` arm=<label> served=x ` into
    # its own status and MASK a real mismatch — the one outcome this guard exists
    # to prevent.
    #
    # 🔴 REQUIRE EXACTLY ONE ASSERTION-BEARING OBJECT. DO NOT REPLACE THIS WITH A
    # FOURTH ATTEMPT AT PICKING THE RIGHT LINE — three have now been defeated,
    # each by a different app-controlled field, and each fix un-fixed the one
    # before it. You would be the fourth person here.
    #
    #   1. regex over the summary line        -> defeated via `observed`
    #   2. last `^{` line, whole output       -> defeated via `render_reason`
    #      (that one INVERTED it: attempt 1 caught the case attempt 2 masked)
    #   3. last `^{` line, cut at the oracle's `--- render verdict` marker
    #                                         -> defeated via `outputDir`, which
    #      `oracle.sh` prints RAW *before* the assertion runs, so the app can
    #      forge the marker itself. Attempt 2 caught what attempt 3 masked.
    #
    # The common cause is upstream and OUT OF THIS PR'S RANGE: `oracle.sh` renders
    # `outputDir`, `scopes`, `app_dir` and `served` with a raw `%s` from the
    # block's own manifest, and `jq -r` turns a `\n` in any of them into a real
    # newline. While that is true, NO position-based pick over that stream is
    # safe, because the app can write above the first marker and below the last.
    #
    # Counting escapes that: a healthy run emits EXACTLY ONE `^{` line carrying
    # the assertion's `assertion` key (measured, 6/6 cells), and a forgery can
    # only ADD one. Suppressing the real one instead means no verdict line, which
    # the branch above already catches as UNMEASURED. So `!= 1` is unreadable ->
    # mislabelled -> exit 2, wherever the forgery sits.
    #
    # ⚠ THIS IS NOT A CLAIM THAT THE CLASS IS CLOSED. It is fail-closed against
    # the three known vectors and needs no marker; hardening `oracle.sh` to quote
    # its manifest-derived fields is the actual fix, and it is filed, not done.
    GOT_ARM=$(printf '%s\n' "$OUT" | grep -a '^{' | python3 -c '
import json, sys
found = []
for line in sys.stdin:
    try:
        d = json.loads(line)
    except Exception:
        continue
    if isinstance(d, dict) and "assertion" in d:
        found.append(d.get("hostArm", ""))
print(found[0] if len(found) == 1 else "")
' 2>/dev/null)
    # An arm that could not be READ is mislabelled, not skipped. ⚠ It no longer
    # means "the summary format moved" — that was true of the sed and is not true
    # of this read. It now means any of: the assertion never ran (oracle.sh skips
    # it entirely when nothing was served, so there is NO json line while the
    # summary line still says `arm=unmeasured`), no `^{` line before the verdict
    # marker, unparseable json, no `assertion` key, `hostArm` absent, or python3
    # failing. All of those fail closed, and none is distinguishable here — read
    # the saved `oracle.<arm>.txt` to tell them apart.
    if [ -z "$GOT_ARM" ] || [ "$GOT_ARM" != "$arm" ]; then
      GOT_ARM="${GOT_ARM:-<unreadable>}"
      MISLABELLED=$((MISLABELLED + 1))
      printf '%-24s %-12s 🔴 ARM MISMATCH — this row is an "%s" measurement\n' "$t" "$arm" "$GOT_ARM"
    fi
    printf '%-24s %-12s %s\n' "$t" "$arm" "$VERDICT"
    printf '%-24s %-12s   %s\n' '' '' "${REASON:-render_reason=<none>}"
  done
  printf '\n'
done

printf 'Full oracle output per cell: %s/<fixture>/oracle.<arm>.txt\n' "$RUNS"

# 🔴 A RUN IN WHICH NOTHING WAS MEASURED MUST NOT EXIT 0. Two of the three
# fixtures legitimately grade `no`, so the VERDICTS are not this script's
# business — but "the oracle could not run" is an instrument failure, and the
# handoff invokes this script as the reproduction step, so anything wrapping it
# would read six unmeasured cells as success.
if [ "$UNMEASURED" -gt 0 ] || [ "$MISLABELLED" -gt 0 ]; then
  printf '\nINSTRUMENT FAILURE: %s cell(s) measured nothing, %s row(s) mislabelled.\n' \
    "$UNMEASURED" "$MISLABELLED" >&2
  exit 2
fi
