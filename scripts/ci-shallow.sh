#!/usr/bin/env bash
#
# ci-shallow.sh — run the test suite in a DEPTH-1 clone of this repo.
#
# WHY THIS EXISTS
#
# GitHub Actions checks out with `actions/checkout@v4`, which clones at
# **depth 1**. Several tests in this repo read git history — the AGENTS.md
# split guards resolve `git show <base-commit>:AGENTS.md` — and a base blob
# that resolves fine in a developer's full clone is simply NOT IN THE OBJECT
# STORE at depth 1. PR #305 shipped a green local `make ci` (18 packages ok)
# and a red `build-test` in CI for exactly that reason:
#
#     --- FAIL: TestEveryBaseBodyLineSurvivedTheMove
#         agents_split_preserved_test.go:512: CONTROL failure: the differ
#         compared 0 lines, so its clean result says nothing
#
# The author's local gate and their merged-tree gate were the SAME
# environment, so neither could observe a defect that only exists in the
# other. This script is the missing environment, as one command.
#
# WHAT IT DOES AND DOES NOT COVER
#
# It clones the CURRENT BRANCH AT ITS COMMITTED TIP over `file://` (which
# forces the real transport, so `--depth` is honoured — a plain local path
# clone hardlinks the whole object store and is never shallow), then runs
# `go test ./...` inside that clone. So:
#
#   * UNCOMMITTED WORKING-TREE CHANGES ARE INVISIBLE TO IT. That is the
#     correct semantics for a pre-push check — CI will only ever see what you
#     pushed — but it is a sharp edge, so the script prints the ref and sha it
#     cloned and warns loudly when your tree is dirty.
#   * It runs the TEST SUITE only, not `go vet` / `go build` / `gofmt`. Those
#     do not read git history, so `make ci` already covers them and running
#     them twice buys nothing.
#
# POSITIVE CONTROLS
#
# A shallow gate that silently does nothing — wrong directory, failed clone,
# zero packages tested — is precisely the failure class it exists to catch,
# and it would be WORSE than not having it, because it would read as
# reassurance. So this script:
#
#   1. asserts the clone really is shallow (`--is-shallow-repository` = true);
#   2. asserts the clone's HEAD is the sha we asked for;
#   3. derives the expected package count from `go list` IN THE CLONE and
#      requires that many `ok` lines — with an absolute floor underneath it so
#      a `go list` returning nothing cannot set the bar to zero;
#   4. reads the test OUTPUT rather than the exit code alone: it counts `ok` /
#      `--- FAIL` / `FAIL` lines, greps for `panic: test timed out` (which
#      truncates the run while still reporting no failures) and for
#      `no tests to run` (which a package count cannot see, because a filtered
#      run still prints one `ok` per package);
#   5. FAILS on a broken clone instead of falling back to the working tree — a
#      silent fallback would make this an ordinary `make test` in a disguise.
#
# Set CI_SHALLOW_KEEP=1 to keep the clone directory for debugging.

set -euo pipefail

# The smallest number of test-bearing packages this repo can plausibly have.
# Measured at 932c5e9: `go list -f '{{if or .TestGoFiles .XTestGoFiles}}…'`
# reports 19 of 19 packages. The floor is deliberately well below that so it
# does not need bumping every time a package is added, and well above zero so
# a clone whose `go list` came back empty cannot report a serene pass.
readonly MIN_TEST_PACKAGES=15

progname="ci-shallow"

die() {
	printf '%s: %s\n' "$progname" "$*" >&2
	exit 1
}

note() {
	printf '%s: %s\n' "$progname" "$*"
}

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) ||
	die "not inside a git repository — nothing to clone"

head_sha=$(git rev-parse HEAD 2>/dev/null) ||
	die "HEAD does not resolve (an empty repository?)"

branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$branch" = "HEAD" ]; then
	die "HEAD is detached at ${head_sha:0:12}. A depth-1 clone needs a named ref to ask for; check out a branch first."
fi

short_sha=${head_sha:0:12}

# Committed state only. Say so HERE, in the output, not just in the docs —
# someone running this with dirty WIP will otherwise believe it covered their
# changes.
dirty_count=$(git status --porcelain | grep -c . || true)
note "cloning ${branch} at ${short_sha} (committed state only) — depth 1"
if [ "$dirty_count" -gt 0 ]; then
	note ""
	note "  WARNING: your working tree has ${dirty_count} uncommitted change(s)."
	note "  They are NOT in this run. A depth-1 clone carries committed state"
	note "  only, so this checks what you would PUSH, not what you have."
	note ""
fi

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/civitai-ci-shallow.XXXXXXXXXX") ||
	die "could not create a temporary directory"

cleanup() {
	if [ -n "${CI_SHALLOW_KEEP:-}" ]; then
		printf '%s: kept the clone at %s (CI_SHALLOW_KEEP is set)\n' "$progname" "$work_dir" >&2
		return
	fi
	rm -rf "$work_dir"
}
trap cleanup EXIT

clone_dir="$work_dir/clone"
log_file="$work_dir/go-test.out"

# `file://` is load-bearing: given a plain path, git treats the clone as local,
# hardlinks the entire object store and IGNORES --depth, which would hand us a
# full clone that passes every test the shallow one fails.
if ! git clone --quiet --depth 1 --branch "$branch" "file://$repo_root" "$clone_dir" 2>"$work_dir/clone.err"; then
	sed 's/^/  /' "$work_dir/clone.err" >&2 || true
	die "the depth-1 clone failed (see above). Refusing to fall back to the working tree — that would test the wrong thing."
fi

# CONTROL 1: it has to actually be shallow, or this whole exercise is a slow
# `make test`.
is_shallow=$(git -C "$clone_dir" rev-parse --is-shallow-repository 2>/dev/null || echo unknown)
if [ "$is_shallow" != "true" ]; then
	die "the clone reports --is-shallow-repository=${is_shallow}, want true. It is not a shallow clone, so it proves nothing about CI."
fi

# CONTROL 2: it has to be the commit we asked for.
clone_sha=$(git -C "$clone_dir" rev-parse HEAD 2>/dev/null || echo none)
if [ "$clone_sha" != "$head_sha" ]; then
	die "the clone is at ${clone_sha}, but ${branch} is at ${head_sha}. Refusing to report on a tree that is not yours."
fi

depth=$(git -C "$clone_dir" rev-list --count HEAD)
note "clone ok: shallow=true, HEAD=${clone_sha:0:12}, ${depth} commit(s) of history"

# CONTROL 3: how many packages MUST report ok. Derived from the clone itself,
# with a floor underneath so an empty answer cannot become the bar.
expected_ok=$(cd "$clone_dir" && go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | grep -c . || true)
if [ "$expected_ok" -lt "$MIN_TEST_PACKAGES" ]; then
	die "the clone reports only ${expected_ok} package(s) with tests, below the floor of ${MIN_TEST_PACKAGES}. Something is wrong with the clone, not with your change."
fi
note "expecting ${expected_ok} package(s) to report ok"
note ""

set +e
(cd "$clone_dir" && go test ./...) 2>&1 | tee "$log_file"
go_test_rc=${PIPESTATUS[0]}
set -e

# CONTROL 4: read the OUTPUT, never the exit code alone. A truncating
# `panic: test timed out` still reports no `--- FAIL` lines, and a wrapper's
# trailing command can swallow a non-zero status entirely.
ok_count=$(grep -c '^ok[[:space:]]' "$log_file" || true)
fail_case_count=$(grep -c -- '--- FAIL' "$log_file" || true)
fail_pkg_count=$(grep -c '^FAIL' "$log_file" || true)
timeout_count=$(grep -c 'panic: test timed out' "$log_file" || true)
# A package count alone cannot see `-run <no match>`: every package still
# reports `ok`, just with this marker appended. It is the one cheap signal that
# distinguishes "all green" from "nothing ran".
empty_run_count=$(grep -c 'no tests to run' "$log_file" || true)

note ""
note "results: ok=${ok_count}/${expected_ok}  failing-tests=${fail_case_count}  failing-packages=${fail_pkg_count}  timeouts=${timeout_count}  filtered-empty=${empty_run_count}  go-test-rc=${go_test_rc}"

problems=0
if [ "$timeout_count" -gt 0 ]; then
	note "  a test timed out — the run was TRUNCATED, so everything after it is unmeasured"
	problems=1
fi
if [ "$empty_run_count" -gt 0 ]; then
	note "  ${empty_run_count} package(s) reported 'no tests to run' — an ok line that measured nothing"
	problems=1
fi
if [ "$fail_case_count" -gt 0 ] || [ "$fail_pkg_count" -gt 0 ]; then
	problems=1
fi
if [ "$ok_count" -ne "$expected_ok" ]; then
	note "  ${expected_ok} package(s) have tests but only ${ok_count} reported ok — some package did not run to a verdict"
	problems=1
fi
if [ "$go_test_rc" -ne 0 ]; then
	problems=1
fi

if [ "$problems" -ne 0 ]; then
	note ""
	die "FAILED in a depth-1 clone. This is what CI sees. If it passes under \`make ci\`, the difference is git history: a full clone resolves base blobs that CI cannot."
fi

note ""
note "PASSED in a depth-1 clone: ${ok_count} package(s) ok, 0 failures, 0 timeouts."
