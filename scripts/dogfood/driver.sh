#!/usr/bin/env bash
# Run the trial matrix, 4 trials in flight at a time.
#
# 🔴 AGENT IDENTITY IS ITS OWN AXIS, CROSSED WITH THE MODEL — NEVER BOUND TO IT.
# `civitai agent-setup` branches hard on which agent it detects, so identity and
# model are two variables. An earlier version of this file gave `claude`/`gpt` a
# known identity and `gemini`/`grok` none, which CONFOUNDS them: the resulting
# grid partitioned perfectly by model and was equally well explained by identity,
# and reading it the wrong way would have shipped "Gemini and Grok fail the
# onboarding", which is false. The cross below is what makes the two separable,
# and the harness must be able to reproduce the correction — not just the error.
set -u
# Guard the VALUE, not just the cd: `cd ""` is a silent no-op on bash <= 5.2, so
# `cd "$X" || exit` sails past an empty $X and runs against the inherited cwd.
HERE="$(dirname "$0")"
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 1; }
cd "$HERE" || exit 1
mkdir -p runs logs

# model|short   — the driving model.
MODELS=(
  "anthropic/claude-sonnet-5|claude"
  "openai/gpt-5.6-terra|gpt"
  "google/gemini-3.8-flash|gemini"
  "x-ai/grok-4.6|grok"
)
# Override for a cheap smoke run: DOGFOOD_MODELS='google/gemini-3.8-flash|gemini'
# (space-separated rows). Same for DOGFOOD_ENVS. Whole-matrix runs cost real
# money, so there has to be a way to exercise this script without paying for one.
[ -n "${DOGFOOD_MODELS:-}" ] && read -r -a MODELS <<<"$DOGFOOD_MODELS"
# image|short|container-user
ENVS=(
  "df-node-root|noderoot|root"
  "df-node-user|nodeuser|dev"
  "df-ubuntu-apt|ubuntu|dev"
  "df-stale-cli|stale|root"
)
[ -n "${DOGFOOD_ENVS:-}" ] && read -r -a ENVS <<<"$DOGFOOD_ENVS"
# short|agent-env  — the identity the CLI will DETECT. Empty env => `other`,
# which is what every agent with no entry in the CLI's table gets. All three rows
# matter: `other` is where the verdict differs, not an edge case.
# 🔴 `codex` is here because the CLI SERIALISES TOML for it and JSON for claude —
# a different write path, not a different label. ⚠ It exercises the WRITE, not the
# merge-into-existing branch: every trial starts from an empty project and no
# image ships a ~/.codex/config.toml, so the trial CREATES that file. The merge
# paths remain unexercised, as the README and the evidence doc both state.
IDENTITIES=(
  "claudeid|CLAUDECODE=1"
  "codexid|CODEX_SANDBOX=1"
  "other|"
)
[ -n "${DOGFOOD_IDENTITIES:-}" ] && read -r -a IDENTITIES <<<"$DOGFOOD_IDENTITIES"

# The default restricts which identities each env is crossed with, so the full
# cross does not cost 3x for no information: the cheapest env runs ALL THREE
# identities (that is the de-confounding control) and every other env runs one.
# Set FULL_CROSS=1 for every combination. (An earlier draft of this comment named
# an `IDENT_FOR` variable that has never existed.)
FULL_CROSS="${FULL_CROSS:-0}"

# ── the app brief ────────────────────────────────────────────────────────────
# DOGFOOD_BRIEF turns every cell from a SETUP trial into an APP-BUILD trial by
# appending one operator-typed line to the task (runner.py's --brief). Empty =>
# the matrix this file has always run, with a byte-identical task.
#
#   DOGFOOD_BRIEF="$(cat briefs/celsius.brief.txt)" DOGFOOD_TRIAL_PREFIX=ta bash driver.sh
#
# 🔴 AND IT REFUSES TO RUN UNDER THE DEFAULT PREFIX, BECAUSE THE RESUME GUARD
# WOULD OTHERWISE SKIP THE WHOLE MATRIX. Trial ids are `<prefix>-<model>-<env>-
# <identity>` and the guard in run_one() skips any id whose transcript already
# carries an `end` record. Run the setup matrix, then run an app matrix under
# the same prefix, and EVERY cell is skipped as "complete" while this script
# prints MATRIX COMPLETE — the third instance of the failure the two comments
# in run_one() and in the identity loop below already exist to prevent, and the
# only one where the skipped cells hold a DIFFERENT task. Make the operator say
# which namespace the run lands in; there is no safe default to guess.
BRIEF="${DOGFOOD_BRIEF:-}"
PREFIX="${DOGFOOD_TRIAL_PREFIX:-t}"
case "$BRIEF" in
  *$'\n'*|*$'\r'*)
    echo "DOGFOOD_BRIEF must be a single line — runner.py refuses a multi-line brief" >&2
    exit 1 ;;
esac
if [ -n "$BRIEF" ] && [ "$PREFIX" = "t" ]; then
  cat >&2 <<'MSG'
refusing to run: DOGFOOD_BRIEF is set but DOGFOOD_TRIAL_PREFIX is still the
default `t`, which is the SETUP matrix's namespace. An app-build cell would
collide with the setup cell of the same name, and the resume guard would skip
it as already complete — reporting MATRIX COMPLETE having run nothing.
Set a distinct namespace, e.g. DOGFOOD_TRIAL_PREFIX=ta
MSG
  exit 1
fi

# ── the credential and the run caps ──────────────────────────────────────────
# A credentialed matrix reaches a REAL account. Everything here is pass-through
# to runner.py, which owns the enforcement; the driver's only job is to refuse
# the two mistakes that are invisible afterwards.
CREDENTIAL="${DOGFOOD_CREDENTIAL_FILE:-}"
APP_PREFIX="${DOGFOOD_APP_PREFIX:-}"
MAX_GENERATIONS="${DOGFOOD_MAX_GENERATIONS:-}"
MAX_SUBMISSIONS="${DOGFOOD_MAX_SUBMISSIONS:-}"
# Pass-through so a matrix can be run under a different output ceiling without
# editing runner.py. Empty => runner.py's own default. It is a ceiling, not a
# spend cap; money is still bounded by runner.py's --max-cost.
MAX_TOKENS="${DOGFOOD_MAX_TOKENS:-}"
if [ -n "$CREDENTIAL" ]; then
  # 🔴 SAME RESUME-GUARD TRAP AS THE BRIEF, AND WORSE. A credentialed cell run
  # under the setup matrix's namespace is skipped as "complete" by an
  # UNCREDENTIALED transcript of the same id — so the run that was supposed to
  # reach the account reaches nothing, and MATRIX COMPLETE is printed over it.
  if [ "$PREFIX" = "t" ]; then
    cat >&2 <<'MSG'
refusing to run: DOGFOOD_CREDENTIAL_FILE is set but DOGFOOD_TRIAL_PREFIX is
still the default `t`, the SETUP matrix's namespace. Set a distinct namespace,
e.g. DOGFOOD_TRIAL_PREFIX=tc
MSG
    exit 1
  fi
  [ -s "$CREDENTIAL" ] || {
    echo "refusing to run: DOGFOOD_CREDENTIAL_FILE=$CREDENTIAL is missing or empty" >&2
    exit 1
  }
  # 🔴 A CREDENTIALED MATRIX WITH NO APP PREFIX CANNOT BE TOLD FROM THE
  # ACCOUNT'S REAL APPS AFTERWARDS, and the prefix is also what the runner's
  # refusal keys on — without it, every mutating `app` command is ungated.
  [ -n "$APP_PREFIX" ] || {
    echo "refusing to run: a credentialed matrix needs DOGFOOD_APP_PREFIX, e.g." >&2
    echo "  DOGFOOD_APP_PREFIX=dogfood4-  — it is what keeps the trial off the" >&2
    echo "  account's existing apps and what makes its own apps identifiable." >&2
    exit 1
  }
fi

run_one() {  # model short image ienv trial user
  local model=$1 image=$3 ienv=$4 trial=$5 euser=$6
  # 🔴 GATE ON COMPLETION, NOT ON EXISTENCE. runner.py opens transcript.jsonl in
  # "w" mode BEFORE it creates the container or makes its first API call, so a
  # trial killed by the timeout below — or one that died on a retry storm —
  # leaves a non-empty file with no `end` record. Keyed on `-f`, that trial is
  # skipped forever and the matrix reports COMPLETE while missing it; grade.sh
  # then scores its half-built container as an ordinary failure, which is
  # indistinguishable from a real product defect.
  if [ -f "runs/$trial/transcript.jsonl" ] \
     && grep -q '"kind": "end"' "runs/$trial/transcript.jsonl"; then
    echo "skip $trial (complete)"; return
  fi
  if [ -f "runs/$trial/transcript.jsonl" ]; then
    echo "re-running $trial (previous attempt did not finish)"
  fi
  local args=(--model "$model" --image "$image" --trial "$trial" --user "$euser" --out runs)
  [ -n "$ienv" ] && args+=(--agent-env "$ienv")
  [ -n "$BRIEF" ] && args+=(--brief "$BRIEF")
  # The credential is passed as a PATH, never as a value — see runner.py's
  # credential section for the three surfaces that keeps it off.
  [ -n "$CREDENTIAL" ] && args+=(--credential-file "$CREDENTIAL")
  [ -n "$APP_PREFIX" ] && args+=(--app-prefix "$APP_PREFIX")
  [ -n "$MAX_GENERATIONS" ] && args+=(--max-generations "$MAX_GENERATIONS")
  [ -n "$MAX_SUBMISSIONS" ] && args+=(--max-submissions "$MAX_SUBMISSIONS")
  [ -n "$MAX_TOKENS" ] && args+=(--max-tokens "$MAX_TOKENS")
  ( timeout 1500 python3 runner.py "${args[@]}" >"logs/$trial.out" 2>"logs/$trial.err"
    echo "done $trial rc=$?" ) &
}

N=0
for m in "${MODELS[@]}"; do
  IFS='|' read -r model ms <<<"$m"
  for e in "${ENVS[@]}"; do
    IFS='|' read -r image es euser <<<"$e"
    for i in "${IDENTITIES[@]}"; do
      IFS='|' read -r is ienv <<<"$i"
      # The de-confounding cell: every model against EVERY identity on one env.
      # Without it, model and identity are two names for the same column.
      #
      # 🔴 THE NON-CROSSED ENVS RUN THE FIRST IDENTITY IN THE LIST, NOT A LITERAL.
      # This used to compare against the literal `claudeid`, which silently dropped
      # every non-noderoot env whenever DOGFOOD_IDENTITIES did not happen to contain
      # that exact name — including BOTH envs rank 34's closing condition is graded
      # on — while still printing MATRIX COMPLETE. Measured:
      # `DOGFOOD_IDENTITIES='codexid|…' bash driver.sh` ran 4 trials, all noderoot.
      # That is the same "reports COMPLETE while cells are missing" failure the
      # resume guard above exists to prevent, arriving by a different route.
      if [ "$FULL_CROSS" != "1" ] && [ "$es" != "noderoot" ] \
         && [ "$is" != "${IDENTITIES[0]%%|*}" ]; then
        continue
      fi
      run_one "$model" "$ms" "$image" "$ienv" "${PREFIX}-${ms}-${es}-${is}" "$euser"
      N=$((N+1))
      if [ $((N % 4)) -eq 0 ]; then wait; fi
    done
  done
done
wait
echo "MATRIX COMPLETE — $N trial(s). Grade each: bash grade.sh <trial-id> <container-user>"
echo "🔴 A per-model verdict is only readable against the SAME identity. Compare"
echo "   the t-<model>-noderoot-<id> cells across ids — this run used: ${IDENTITIES[*]%%|*}"
echo "   — before attributing any difference to the model."
