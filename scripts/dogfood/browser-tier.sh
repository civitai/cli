#!/usr/bin/env bash
# The CI split of the repo-root test package, in ONE place.
#
#   browser-tier.sh files   the root test files whose tests are the BROWSER tier
#   browser-tier.sh names   those files' top-level tests, sorted, one per line
#   browser-tier.sh regex   ^(A|B|…)$ over those names
#
# `dogfood-browser` runs the root package with `-run "$(… regex)"`; `build-test`
# runs it with `-skip "$(… regex)"`. Everything not named here is the always
# tier, so a NEW root test — in any file not listed below — runs in
# `build-test` without anyone doing anything. Locally `go test ./...` ignores
# all of this and runs everything.
#
# 🔴 THE TIER IS DECIDED BY FILE, NEVER BY NAME. A name rule puts a new test in
# whichever tier its name happens to match. The browser tier is the files whose
# tests drive a real headless browser through scripts/dogfood (they call
# oracleBrowser). Two files that DO touch the harness stay OUT on purpose:
#   - dogfood_t1_floor_test.go reads internal/cmd/app_doctor.go (its codes are
#     the CLI's), so a product change can break it;
#   - dogfood_submission_cap_seam_test.go builds ./cmd/civitai.
#
# The name list is grepped out of Go source, which is a claim about `gofmt`'s
# layout. Nothing checks that statically; the two CI jobs check it against what
# ACTUALLY RAN: `dogfood-browser` requires its PASS lines to be exactly this
# list (ci-browser-tests.sh), and `build-test` requires the tests it reported
# to be exactly the real binary's `go test -list` minus it. A name this list
# gets wrong in either direction reds one of the two. Run from the repo root.
set -euo pipefail

FILES=(
  dogfood_cdp_launch_test.go
  dogfood_oracle_consent_test.go
  dogfood_oracle_injection_test.go
  dogfood_oracle_picker_test.go
  dogfood_oracle_post_test.go
  dogfood_oracle_test.go
  dogfood_ship_verdict_test.go
)

names() {
  local f
  for f in "${FILES[@]}"; do
    [ -f "$f" ] || { echo "browser-tier.sh: $f does not exist — remove it from FILES (run from the repo root)" >&2; exit 1; }
  done
  # `|| true`: grep exits 1 on no match, and the emptiness check below is the
  # verdict, not grep's status.
  { grep -hoE '^func Test[A-Za-z0-9_]*\(' "${FILES[@]}" || true; } | sed -e 's/^func //' -e 's/($//' | LC_ALL=C sort
}

case "${1:-}" in
  files) printf '%s\n' "${FILES[@]}" ;;
  names)
    n=$(names)
    [ -n "$n" ] || { echo "browser-tier.sh: the browser tier holds no tests — the filter would match nothing" >&2; exit 1; }
    printf '%s\n' "$n"
    ;;
  regex)
    n=$(names)
    [ -n "$n" ] || { echo "browser-tier.sh: the browser tier holds no tests — the filter would match nothing" >&2; exit 1; }
    printf '^(%s)$\n' "$(printf '%s\n' "$n" | paste -sd'|' -)"
    ;;
  *) echo "usage: browser-tier.sh files|names|regex" >&2; exit 2 ;;
esac
