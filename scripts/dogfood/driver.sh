#!/usr/bin/env bash
# Run the trial matrix: 4 models x 4 environments, 4 trials in flight at a time.
# Each model carries a plausible agent identity, because `civitai agent-setup`
# branches hard on which agent it detects — and `other` (no entry in the CLI's
# table) is the branch worth covering, not an edge case.
set -u
# Guard the VALUE, not just the cd: `cd ""` is a silent no-op on bash <= 5.2, so
# `cd "$X" || exit` sails past an empty $X and runs against the inherited cwd.
HERE="$(dirname "$0")"
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 1; }
cd "$HERE" || exit 1
mkdir -p runs logs

# model|short|agent-env
MODELS=(
  "anthropic/claude-sonnet-5|claude|CLAUDECODE=1"
  "openai/gpt-5.6-terra|gpt|CODEX_SANDBOX=1"
  "google/gemini-3.8-flash|gemini|"
  "x-ai/grok-4.6|grok|"
)
# image|short|container-user
ENVS=(
  "df-node-root|noderoot|root"
  "df-node-user|nodeuser|dev"
  "df-ubuntu-apt|ubuntu|dev"
  "df-stale-cli|stale|root"
)

N=0
for m in "${MODELS[@]}"; do
  IFS='|' read -r model ms menv <<<"$m"
  for e in "${ENVS[@]}"; do
    IFS='|' read -r image es euser <<<"$e"
    TRIAL="t-${ms}-${es}"
    [ -f "runs/$TRIAL/transcript.jsonl" ] && { echo "skip $TRIAL (done)"; continue; }
    args=(--model "$model" --image "$image" --trial "$TRIAL" --user "$euser" --out runs)
    [ -n "$menv" ] && args+=(--agent-env "$menv")
    ( timeout 1500 python3 runner.py "${args[@]}" >"logs/$TRIAL.out" 2>"logs/$TRIAL.err"
      echo "done $TRIAL rc=$?" ) &
    N=$((N+1))
    if [ $((N % 4)) -eq 0 ]; then wait; fi
  done
done
wait
echo "MATRIX COMPLETE"
