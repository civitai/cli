#!/usr/bin/env bash
# Decide whether the `dogfood-browser` job must run for this pull request.
#
#   browser-suite-relevant.sh <event-name> <changed-paths-file> <diff-exit-code>
#
# Prints `run=true|false` and `reason=<…>` on stdout, and appends both to
# $GITHUB_OUTPUT when set. Always exits 0 with a verdict; a usage error is the
# one non-zero exit, and the workflow treats ANY failure of this step as
# run=true (fail open, see ci.yml).
#
# 🔴 FAIL OPEN, BY CONSTRUCTION RATHER THAN BY CARE. The browser suite is skipped
# ONLY when every changed path is POSITIVELY classified irrelevant below. A
# path matching nothing — a new top-level directory, a renamed file, anything
# nobody thought about — runs it. So does every event that is not a pull
# request, a diff that failed, and an EMPTY diff (an empty diff on a PR is far
# more likely a broken base than a no-op change, and nothing was proven).
#
# 🔴 WHAT "IRRELEVANT" RESTS ON, AND ITS SCOPE. The browser tier (the files
# listed in browser-tier.sh) drives scripts/dogfood with a STUB
# docker and a STUB `civitai`; it never builds or execs this repo's CLI. That
# is pinned structurally (no dogfood_*_test.go may import this module — see
# TestNoDogfoodTestImportsTheModule) and was MEASURED for the PR that added
# this file, two ways: an strace of the whole
# tier (every openat/exec/stat under the repo) and behavioural breaks planted in
# internal/cmd, internal/scaffold, internal/validate, cmd/civitai and the
# scaffold templates, none of which any browser test noticed. Both are in that
# PR's body. What it does NOT cover: a dependency neither the trace nor the
# sampled breaks exercised. Such a break is caught on the push to main and by
# the nightly run, not on the PR — that is the trade this file makes.
#
# Changing a pattern here changes what a PR is allowed to skip. The table test
# (dogfood_browser_relevance_test.go) pins one row per class.
set -uo pipefail

event="${1:-}"
list="${2:-}"
diff_rc="${3:-}"
if [ -z "$event" ] || [ -z "$list" ] || [ -z "$diff_rc" ]; then
  echo "usage: browser-suite-relevant.sh <event-name> <changed-paths-file> <diff-exit-code>" >&2
  exit 2
fi

verdict() {
  echo "run=$1"
  echo "reason=$2"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    { echo "run=$1"; echo "reason=$2"; } >> "$GITHUB_OUTPUT"
  fi
  exit 0
}

[ "$event" = "pull_request" ] || verdict true "event '$event' always runs the browser suite"
[ "$diff_rc" = "0" ] || verdict true "the diff failed (exit $diff_rc), so nothing was classified"
[ -r "$list" ] || verdict true "the changed-path list '$list' is unreadable"

n=0
while IFS= read -r path || [ -n "$path" ]; do
  [ -n "$path" ] || continue
  n=$((n + 1))
  case "$path" in
    # ── RELEVANT: the suite's own inputs ─────────────────────────────────────
    scripts/dogfood/*) verdict true "$path is the browser harness itself" ;;
    .github/workflows/ci.yml) verdict true "$path defines the browser jobs" ;;
    go.mod | go.sum) verdict true "$path changes the toolchain or the module graph the test binary is built from" ;;
    *_test.go)
      # EVERY test file at the repo root, not only dogfood_*_test.go: they are
      # one package, so any of them can declare a helper a browser test uses —
      # directly or through another helper. Treating the rest as irrelevant
      # needed a guard on that reference graph and still left its transitive
      # edges open; measured over 200 replayed commits the wider rule costs 7
      # extra runs.
      case "$path" in
        */*) ;; # not at the root: falls through to the class rules below
        *) verdict true "$path is a root test file — the same package as the browser tier" ;;
      esac
      ;;
  esac
  case "$path" in
    # ── IRRELEVANT, positively: measured not to reach the browser tier ───────
    internal/* | pkg/* | cmd/* | schema/* | examples/* | npm/* | claudedocs/*) continue ;;
    tools/*) continue ;;
    .github/ISSUE_TEMPLATE/* | .github/workflows/*) continue ;; # ci.yml matched above
    *.md) continue ;; # root docs; scripts/dogfood/*.md matched above
    main.go | schema.go | Makefile | LICENSE | flake.nix | flake.lock | .gitignore | .golangci.yml | .goreleaser.yaml) continue ;;
    *) verdict true "$path is not classified — failing open" ;;
  esac
done < "$list"

[ "$n" -gt 0 ] || verdict true "the diff is empty — nothing was proven irrelevant"
verdict false "all $n changed path(s) are classified irrelevant to the browser suite"
