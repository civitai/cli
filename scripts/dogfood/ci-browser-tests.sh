#!/usr/bin/env bash
# Run the repo-root package's BROWSER tier and prove it ran — the body of the
# `dogfood-browser` CI job, in a script so every workflow that runs the suite
# runs the same check rather than a copy of it. Run from the repo root, after
# ci-browser.sh has resolved a browser.
#
# 🔴 THIS IS THE LEDGER FOR THE BROWSER HALF OF THE SPLIT. `go test -run
# <filter>` exits 0 when the filter matches nothing, and prints `[no tests to
# run]`. So the verdict here is not the exit code: the top-level PASS lines
# `go test -json` reported must be EXACTLY the tests browser-tier.sh lists —
# none missing (a renamed test, a file under a build tag, a skip, a filter
# that matched nothing), none extra (an unanchored filter). `build-test`
# carries the mirror image for the always tier; together every root test runs
# exactly once.
set -uo pipefail

: "${RUNNER_TEMP:=$(mktemp -d)}"
mkdir -p "$RUNNER_TEMP"
here=$(dirname "$0")

run=$("$here/browser-tier.sh" regex) || exit 1
"$here/browser-tier.sh" names > "$RUNNER_TEMP/browser.txt" || exit 1

# -count=1: these tests' inputs are mostly files their CHILD processes read
# (oracle.sh, _cdp.mjs), which Go's test cache does not track — a cached replay
# would re-print a PASS for code that changed.
rc=0
go test -json -count=1 -run "$run" . > "$RUNNER_TEMP/browser.json" || rc=$?
jq -rj 'select(.Action == "output") | .Output' "$RUNNER_TEMP/browser.json"
if [ "$rc" -ne 0 ]; then
  echo "::error::the browser tier failed (go test exit $rc) — the failing tests are named above."
  exit "$rc"
fi

jq -r 'select(.Action == "pass" and .Test != null and (.Test | contains("/") | not)) | .Test' \
  "$RUNNER_TEMP/browser.json" | LC_ALL=C sort > "$RUNNER_TEMP/passed.txt"
echo "browser tier: $(wc -l < "$RUNNER_TEMP/passed.txt") top-level PASS, $(wc -l < "$RUNNER_TEMP/browser.txt") listed"
if [ ! -s "$RUNNER_TEMP/passed.txt" ] || ! diff "$RUNNER_TEMP/browser.txt" "$RUNNER_TEMP/passed.txt"; then
  echo "::error::the browser tier did not PASS exactly its listed tests (diff above: < listed but no PASS, > PASSed but not listed). An empty PASS list means the -run filter matched nothing, which \`go test\` reports as success."
  exit 1
fi

if [ -n "${CIVITAI_ASSERT_WAIT_LOG:-}" ]; then
  "$here/wait-summary.sh" "$CIVITAI_ASSERT_WAIT_LOG"
fi
