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
# which is what every agent with no entry in the CLI's table gets. Both rows
# matter: `other` is where the verdict differs, not an edge case.
IDENTITIES=(
  "claudeid|CLAUDECODE=1"
  "other|"
)

# IDENT_FOR restricts which identities a given env is crossed with, so the full
# cross does not cost 4x for no information. Default: cross the cheapest env
# with BOTH identities (that is the de-confounding control), and run the rest
# under one identity. Set FULL_CROSS=1 for every combination.
FULL_CROSS="${FULL_CROSS:-0}"

run_one() {  # model short image ienv trial user
  local model=$1 image=$3 ienv=$4 trial=$5 euser=$6
  [ -f "runs/$trial/transcript.jsonl" ] && { echo "skip $trial (done)"; return; }
  local args=(--model "$model" --image "$image" --trial "$trial" --user "$euser" --out runs)
  [ -n "$ienv" ] && args+=(--agent-env "$ienv")
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
      # The de-confounding cell: every model against BOTH identities on one env.
      # Without it, model and identity are two names for the same column.
      if [ "$FULL_CROSS" != "1" ] && [ "$es" != "noderoot" ] && [ "$is" != "claudeid" ]; then
        continue
      fi
      run_one "$model" "$ms" "$image" "$ienv" "t-${ms}-${es}-${is}" "$euser"
      N=$((N+1))
      if [ $((N % 4)) -eq 0 ]; then wait; fi
    done
  done
done
wait
echo "MATRIX COMPLETE — $N trial(s). Grade each: bash grade.sh <trial-id> <container-user>"
echo "🔴 A per-model verdict is only readable against the SAME identity. Compare"
echo "   t-<model>-noderoot-claudeid against t-<model>-noderoot-other before"
echo "   attributing any difference to the model."
