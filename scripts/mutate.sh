#!/usr/bin/env bash
#
# mutate.sh — run mutation testing over ONE package and print a CORRECTED report.
#
# This is a local investigation tool, not a gate. It is deliberately not wired
# into `make ci` and there is no CI job for it; see
# claudedocs/mutation-testing-experiment.md for the measurements behind that.
#
#   ./scripts/mutate.sh                      # defaults to internal/validate
#   ./scripts/mutate.sh internal/blockproto
#
# ---------------------------------------------------------------------------
# WHAT THIS DOES NOT COVER — read before believing a green run
# ---------------------------------------------------------------------------
#
# 1. IT CANNOT SEE A DEFECT THAT LIVES IN TEST CODE. Mutation testing mutates
#    PRODUCTION code and asks whether any test notices. A test that skips
#    itself, a "positive control" whose condition is `s == s`, an assertion
#    whose expected string can never match, or a mutant killed by a BYSTANDER
#    assertion inside an otherwise-vacuous test all leave this report green.
#    It answers "is this line pinned by something?", never "is it pinned by the
#    assertion that claims to pin it?".
#
# 2. IT ONLY MUTATES OPERATORS. gremlins rewrites arithmetic, comparison
#    boundaries, comparison negation, && / ||, ++ / --, break / continue,
#    bitwise and assignment operators. It does NOT touch string literals,
#    numeric CONSTANTS, function arguments, argument ORDER, struct fields or
#    return values. In a package that is mostly message construction and
#    constant tables, most of the code is simply out of reach.
#
# 3. GREMLINS SCORES A NON-COMPILING MUTANT AS "KILLED". It reads `go test`'s
#    exit code, and exit 1 means both "a test failed" and "the build failed".
#    That is why every run here is post-processed by scripts/mutate-verify.go,
#    which re-applies each claimed kill and asks the compiler. On
#    internal/validate that reclassified 36 of 146 "kills". NEVER quote
#    gremlins' own Killed / efficacy numbers — quote the CORRECTED block.
#
# 4. THE DEFAULT TIMEOUT IS WRONG FOR A FAST SUITE. gremlins derives a per-
#    mutant timeout from how long coverage collection took. internal/validate
#    tests in 0.17s, so at stock settings 162 of 236 mutants reported TIMED OUT
#    — a number that looks like a result and is an artifact. Hence the explicit
#    --timeout-coefficient / --workers below. If you see TIMED OUT > 0, the run
#    is invalid; raise the coefficient rather than reading it.
#
# 5. "NOT COVERED" IS A COVERAGE FACT, NOT A MUTATION RESULT. Those mutants are
#    never executed. They are neither killed nor survived.
#
# 6. SURVIVORS ARE A QUEUE, NOT A BUG LIST. Some cannot change behaviour at all
#    (equivalent mutants). "Equivalent" is a CLAIM: discharge it with an input
#    that would discriminate the mutant from the original, not with reasoning.
#
# ---------------------------------------------------------------------------
# DEPENDENCY POLICY
# ---------------------------------------------------------------------------
# gremlins is NOT a module dependency and must never become one (AGENTS.md
# "Permission boundaries" makes a new third-party dependency ask-first). It is
# installed as a standalone binary into a cache dir with `go install pkg@ver`,
# which runs outside this module and does not touch go.mod / go.sum.
#
set -euo pipefail

PKG="${1:-internal/validate}"
GREMLINS_VERSION="v0.6.0"
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/civitai-cli-mutate"
BIN="$CACHE/gremlins"
REPORT="${MUTATE_REPORT:-$CACHE/$(echo "$PKG" | tr '/' '-').json}"

cd "$(dirname "$0")/.."

if [ ! -d "$PKG" ]; then
  echo "no such package directory: $PKG" >&2
  exit 2
fi

if [ ! -x "$BIN" ]; then
  echo "==> installing gremlins $GREMLINS_VERSION into $CACHE (not a module dependency)"
  mkdir -p "$CACHE"
  # `go install pkg@version` is module-less: it cannot modify go.mod/go.sum.
  GOBIN="$CACHE" GOFLAGS= go install "github.com/go-gremlins/gremlins/cmd/gremlins@$GREMLINS_VERSION"
fi

# Positive control on the instrument: the suite must be GREEN before mutating,
# or every mutant is "killed" by a failure that was already there.
echo "==> baseline: ./$PKG must be green before mutating"
if ! go test "./$PKG/" -count=1; then
  echo "baseline suite is RED — a mutation run against it measures nothing" >&2
  exit 1
fi

echo "==> gremlins over ./$PKG (all mutators enabled)"
"$BIN" unleash "./$PKG" \
  --invert-assignments --invert-bitwise --invert-bwassign \
  --invert-logical --invert-loopctrl --remove-self-assignments \
  --workers "${MUTATE_WORKERS:-8}" \
  --timeout-coefficient "${MUTATE_TIMEOUT_COEFFICIENT:-60}" \
  -o "$REPORT" | tail -5

echo
echo "==> re-classifying: asking the COMPILER which 'kills' actually compile"
go run scripts/mutate-verify.go -pkg "$PKG" -json "$REPORT"

echo
echo "report JSON: $REPORT"
echo "NOTE: this script exits 0 regardless of findings. It is advisory."
