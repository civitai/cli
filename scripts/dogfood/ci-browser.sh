#!/usr/bin/env bash
# Resolve a browser for the render-oracle tests and prove it launches.
#
# ONE COPY, CALLED BY EVERY CI JOB THAT DRIVES A BROWSER (`build-test` and
# `dogfood-browser`). These were two inline steps of `build-test`; they moved
# here verbatim when the browser tests got their own job, because
# a second inline copy is a resolver that can drift from the first — and
# `TestEveryBrowserResolverAgreesOnTheSameOrder` reads THIS file for the order.
#
# Writes CIVITAI_CHROME to $GITHUB_ENV when run under Actions; prints it
# otherwise. Exits non-zero, with an ::error::, when no browser resolves or the
# resolved one cannot launch.
set -uo pipefail

: "${RUNNER_TEMP:=$(mktemp -d)}"
mkdir -p "$RUNNER_TEMP"

# ── resolve ──────────────────────────────────────────────────────────────────
# 🔴 THE RENDER-ORACLE TESTS REFUSE TO SKIP UNDER CI, so a job running them has
# to be able to give them a browser. They drive a real headless Chromium against
# a real served block (scripts/dogfood/oracle.sh); a skip there and a pass there
# are indistinguishable in the log the merge gate is read from, which is why the
# helper fails instead — see dogfood_oracle_test.go.
#
# Resolved rather than installed: the ubuntu runner image ships browsers, and an
# unconditional apt/snap install would cost every PR minutes for something
# already present. If this is what goes red, the fix is one line — add an install
# — NOT loosening the test into a skip.
#
# 🔴 THE ORDER PUTS RELEASE BUILDS FIRST, AND IT CHANGED FOR A REASON.
# `ubuntu-latest` ships BOTH: `/usr/bin/google-chrome` is a
# `google-chrome-stable` .deb, and `/usr/bin/chromium` is a symlink into
# `/usr/local/share/chromium/`, which `install-google-chrome.sh` unzips straight
# out of the `chromium-browser-snapshots` bucket at whatever per-commit revision
# sits nearest Chrome's. The versions say it plainly — Chrome 152.0.7977.82
# against Chromium 152.0.7977.0. The old order named `chromium` first, so CI
# drove an un-release-qualified trunk snapshot with a shipped release on the same
# disk. ⚠ That is a determinism argument, not a measurement: nothing here has
# shown the snapshot launching less reliably than the release.
#
# The same list is in scripts/dogfood/briefs/_cdp.mjs, scripts/dogfood/oracle.sh
# and dogfood_oracle_test.go, and `TestEveryBrowserResolverAgreesOnTheSameOrder`
# fails when they drift.
CIVITAI_CHROME=
for b in google-chrome google-chrome-stable chromium chromium-browser chrome; do
  if command -v "$b" >/dev/null 2>&1; then
    CIVITAI_CHROME=$(command -v "$b")
    "$b" --version || true
    break
  fi
done
if [ -z "$CIVITAI_CHROME" ]; then
  echo "::error::no Chromium/Chrome on this runner. The render-oracle tests refuse to skip under CI, so \`go test\` will fail. Add a browser install step (the tests themselves must NOT be loosened into a skip)."
  exit 1
fi
export CIVITAI_CHROME
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "CIVITAI_CHROME=$CIVITAI_CHROME" >> "$GITHUB_ENV"
else
  echo "CIVITAI_CHROME=$CIVITAI_CHROME"
fi

# ── smoke ────────────────────────────────────────────────────────────────────
# 🔴 A RESOLVED BROWSER IS NOT A WORKING BROWSER, AND THAT GAP IS WHAT MADE
# `build-test` FLAKY. Measured over four runs — `main` and three PRs, one of them
# a single markdown file — the resolver printed a version, and `go test` then
# died with:
#
#   {"assertion":"celsius","pass":false,"reason":"harness error: browser
#    never printed a DevTools endpoint: [ERROR:dbus/bus.cc:405] Failed to
#    connect to the bus: Could not parse server address: …"}
#   oracle: the assertion could not run (exit 2) — nothing was measured
#
# In a 31 KB log that produced ZERO `--- FAIL:` lines and every other package
# `ok`: the oracle behaved correctly (exit 2, "nothing was measured", rather than
# a false `RENDER=no`) and the job went red with no failing test to point at.
# Two of those runs went green on re-run with no change, so the intermittency is
# in the browser's startup, not in the code under test.
#
# This launches the browser through the harness's own `launch()` BEFORE any test
# does. Three things it buys, none of which the version print can: (1) a browser
# that cannot bind a debugging port fails here, by name, in three lines, instead
# of eighty lines into a Go test; (2) it publishes the cold-launch time on every
# run; (3) a launch that needed a retry raises a ::warning EVEN THOUGH IT
# SUCCEEDED, which is the half a retry normally hides.

# NEGATIVE CONTROL — can this still go red? A smoke test that reports success
# whatever it is pointed at is indistinguishable from a working browser. One
# attempt and a short budget so the control costs ~0s: the fake exits 3.
ctl="$RUNNER_TEMP/not-a-browser"
# The fake must EXIST: a missing one also fails to launch, and the control
# would then pass while having tested nothing (measured while factoring this
# out: a RUNNER_TEMP that did not exist made exactly that happen).
if ! { printf '#!/bin/sh\nexit 3\n' > "$ctl" && chmod +x "$ctl" && [ -x "$ctl" ]; }; then
  echo "::error::could not create the smoke test's negative-control binary at $ctl — the control would pass vacuously"
  exit 1
fi
if CIVITAI_CHROME="$ctl" CIVITAI_ASSERT_LAUNCH_ATTEMPTS=1 CIVITAI_ASSERT_LAUNCH_MS=2000 \
   node -e 'import("./scripts/dogfood/briefs/_cdp.mjs").then(async (m) => { const { proc } = await m.launch(); proc.kill("SIGKILL"); })' \
   >/dev/null 2>&1; then
  echo "::error::the browser smoke test reported SUCCESS for a binary that exits 3 without printing anything. It is not testing the browser, so its verdict about the real one means nothing."
  exit 1
fi

# THE REAL LAUNCH. `|| true` then judged by CONTENT: this must distinguish
# "failed" from "succeeded after a retry", and an exit code cannot say the second.
node -e 'import("./scripts/dogfood/briefs/_cdp.mjs").then(async (m) => { const { proc } = await m.launch(); proc.kill("SIGKILL"); console.log("SMOKE OK"); })' \
  > "$RUNNER_TEMP/browser-smoke.log" 2>&1 || true
cat "$RUNNER_TEMP/browser-smoke.log"

if ! grep -aq 'SMOKE OK' "$RUNNER_TEMP/browser-smoke.log"; then
  echo "::error::$CIVITAI_CHROME resolved but never printed a DevTools endpoint. Every render-oracle test will report a harness error and the job will go red with no failing test name. Do NOT loosen the tests into a skip and do NOT make the oracle swallow this — fix or replace the browser (see LAUNCH_FLAGS / LAUNCH_MS in scripts/dogfood/briefs/_cdp.mjs)."
  exit 1
fi

if grep -aq 'BROWSER LAUNCH RETRY' "$RUNNER_TEMP/browser-smoke.log"; then
  echo "::warning title=browser launch retried::the browser did not start on the first attempt. This run is green, but the launch flake is still live — read the attempt diagnostics above before treating this job's colour as a statement about the browser."
fi
