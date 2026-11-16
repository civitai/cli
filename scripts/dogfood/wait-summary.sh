#!/usr/bin/env bash
# Summarise a CIVITAI_ASSERT_WAIT_LOG (one JSON line per in-page wait, written
# by briefs/_cdp.mjs) and say whether the short budget the expected-timeout
# negative controls run on still has its margin.
#
# Usage: wait-summary.sh <log>
#
# 🔴 THE MARGIN IS THE CLAIM, SO THE MARGIN IS WHAT THIS CHECKS. Those controls
# (dogfood_control_wait_test.go) earn `no` by waiting a SHORT budget for
# something their fixture never renders. That proves absence only while the
# short budget is comfortably longer than a real mount takes on the machine
# running it. So: the slowest wait that SUCCEEDED, on any leaf, against half
# the shortest budget any leaf ran on. Above it → a ::warning naming both
# numbers. A warning, not a failure: one slow runner is a fact about that
# runner, and the margin was chosen from a distribution, not a single sample.
#
# Exits 0 always on a readable log. An absent or empty log is reported, never
# read as "no slow waits": `go test` replays a cached package without running
# it, and a replay writes nothing.
set -uo pipefail

log="${1:?usage: wait-summary.sh <log>}"
if [ ! -s "$log" ]; then
  echo "::notice title=dogfood waits::no in-page waits were logged ($log is absent or empty) — the root package was replayed from the test cache or ran no browser leaf, so this run says nothing about wait margins"
  exit 0
fi

jq -rs '
  def pct(p): (sort | .[((length - 1) * p | floor)]);
  (map(select(.ok)) | map(.ms)) as $ok
  | (map(select(.ok | not))) as $to
  | (map(.budget) | min) as $short
  | "in-page waits: \(length) logged, \($ok | length) succeeded, \($to | length) timed out",
    (if ($ok | length) > 0 then
       "succeeded (ms): p50=\($ok | pct(0.5)) p95=\($ok | pct(0.95)) max=\($ok | max)"
     else "succeeded: none" end),
    "timed out, by budget (ms): \($to | group_by(.budget) | map("\(.[0].budget)×\(length)") | join(" "))",
    "shortest budget any wait ran on: \($short) ms",
    (if ($ok | length) > 0 and ($ok | max) * 2 > $short then
       "::warning title=dogfood wait margin::the slowest SUCCESSFUL in-page wait took \($ok | max) ms, more than half the shortest budget (\($short) ms). The expected-timeout negative controls report absence after that budget; at this margin a slow mount can make one pass for the wrong reason. Re-measure before trusting them on this runner (dogfood_control_wait_test.go)."
     else empty end)
' "$log"
