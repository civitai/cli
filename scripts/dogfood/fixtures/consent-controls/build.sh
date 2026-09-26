#!/usr/bin/env bash
# Build the UNCONSENTED ARM'S OWN CONTROLS — two scaffold-derived App Blocks that
# differ by exactly one branch, plus the untouched scaffold they are derived from.
#
#   bash build.sh              # build all three fixture containers
#   bash build.sh --keep       # ...reusing containers that already exist
#
# 🔴 WHAT THESE ADD — AND READ THE RETRACTION BELOW BEFORE WRITING A BIGGER REASON.
# `CIVITAI_ASSERT_UNCONSENTED=1` grades "did the block ASK the host for consent".
# These fixtures add exactly two things to it: ATTRIBUTION — a pair differing in
# ONE controlled variable rather than a version bump — and REPRODUCIBILITY, since
# the existing real-bundle controls are containers in the "DO NOT DESTROY" set
# while these rebuild from git in ~5 min. Nothing more.
#
# 🔴 RETRACTED, AND DO NOT RE-DERIVE IT. The first version of this header, of this
# directory's README, of the block in `../../README.md` and of the PR body all led
# with: "an arm whose `no` has never been watched arrive for the arm's OWN reason
# is a claim about the instrument, not a measurement." THAT SENTENCE IS FALSE, and
# was false when written. It had been watched, on a real bundle, before any of
# this existed: `ab-ship-mimo-02` (the live `ab-img-poster v0.1.0`) graded
# `RENDER=no observed=ready>generating>ready` — "spent without asking" — and its
# `v0.1.1` fix graded `RENDER=yes observed=ready`. Two more of the seven
# fixtures grade `no` on the arm FOR THE ARM'S OWN REASON, and the handoff's own "How to verify" section
# runs exactly that cell. The claim reached four sites because nobody re-read the
# table it contradicts. You are at least the second person to write a reason here:
# if the one above stops holding, WRITE THAT IT HAS NONE rather than reaching for
# a better one.
#
# ⚠ `ctl-scaffold-untouched` is NOT the arm's negative control and never could be.
# It is ONE measurement, kept as a live row rather than as prose only because the
# template can change under it. A `page-money` scaffold INSTALLED AND BUILT still
# fails BOTH arms with the identical render reason — it ships `pm-*` testids, so
# the genpost assertion throws before the consent step is reached. Its row is
# EXPECTED to stay `no`/`no`; a row that MOVES means the template gained the
# genpost shape, which is the thing worth being told about.
#
# The pair that does the work is `ctl-genpost-blind` / `ctl-genpost-asks`. Both
# reach the Generate click; they differ by the consent branch alone, so the arm's
# verdict moving between them is attributable to that branch and nothing else.
# `asks` is the positive control for `blind`: it proves an ask IS observable on
# this exact vite-built bundle of the PUBLISHED SDK — the one property the Go
# fixtures in `dogfood_oracle_consent_test.go` structurally cannot reach, since
# their app is a hand-written inline-transport stub.
#
# ⚠ npm 10.x cannot install the page-money scaffold — arborist dies with
# `Cannot read properties of null (reading 'edgesOut')`. THIS IS A KNOWN,
# DIAGNOSED, ALREADY-REMEDIED DEFECT IN THIS REPO: see the comment above
# `actions/setup-node` in `.github/workflows/ci.yml`, which carries the same
# crash, the same `vitest -> jsdom -> canvas` chain, and the house remedy
# ("node 24 (npm 11), NOT 22 (npm 10) … Do not drop back to 22") at four sites.
# Pinning npm here is therefore the repo's CONVENTION, not a deviation from it,
# and the default below is the version that comment measured good. 🔴 The genuine
# residual: `../../envs/node-root.Dockerfile`, `../../envs/node-user.Dockerfile`
# and `../../envs/stale-cli.Dockerfile` are all still `FROM node:22-bookworm-slim`,
# so the TRIAL images never got that fix — an open question about what a trial
# should measure, not an undiagnosed crash.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 2; }

IMAGE="${DOGFOOD_IMAGE:-df-node-root}"
# The version `.github/workflows/ci.yml` records as measured-good against this
# crash. 12.1.0 also works (measured), but matching the house number keeps one
# claim in one place.
NPM_PIN="${DOGFOOD_NPM_PIN:-11.19.0}"
KEEP=no
# Reject an unrecognised argument rather than ignoring it: the full path
# `docker rm -f`s all three containers and spends ~5 min, so `--kep` silently
# doing the expensive thing is the worst available behaviour. Checked in EVERY
# position, not just $1 — `build.sh --keep --kep` would otherwise set KEEP and
# ignore the typo, which is the same class one argument over.
while [ $# -gt 0 ]; do
  case "$1" in
    --keep) KEEP=yes ;;
    *)      printf 'build.sh: unknown argument %s (expected --keep or nothing)\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

fatal() { printf 'build.sh: %s\n' "$1" >&2; exit 2; }

# The twins' lockfile, pulled from whichever twin resolves it first. See
# prepare(): both twins must install from ONE lockfile or ordinary registry
# drift makes them differ in a second file and aborts the run.
LOCK=
cleanup() { [ -n "$LOCK" ] && rm -f "$LOCK"; }
trap cleanup EXIT

command -v docker >/dev/null || fatal "no docker on PATH"
docker image inspect "$IMAGE" >/dev/null 2>&1 \
  || fatal "no image '$IMAGE' — build it first: docker build -t $IMAGE -f $HERE/../../envs/node-root.Dockerfile $HERE/../.."

# One container per fixture. The oracle grades a CONTAINER and resolves the app
# by finding a manifest under /work, taking the first of a sorted list — so two
# app dirs in one container would silently grade only one of them.
start() {
  local name="$1"
  if [ "$KEEP" = "yes" ] && [ "$(docker inspect -f '{{.State.Status}}' "dogfood-$name" 2>/dev/null)" = "running" ]; then
    printf '  reusing dogfood-%s\n' "$name"
    return 0
  fi
  docker rm -f "dogfood-$name" >/dev/null 2>&1
  docker run -d --name "dogfood-$name" --pids-limit 512 --memory 2g --cpus 2 \
    -w /work "$IMAGE" sleep infinity >/dev/null \
    || fatal "could not start dogfood-$name"
}

# `civitai` comes from the PUBLIC npm registry, exactly as a trial's step 5 does:
# a fixture built from this checkout would be testing an artefact no trial loads.
#
# ⚠ `--keep` SKIPS THIS ENTIRELY when the scaffold is already installed, and that
# is what the flag is for — the npm install is the minutes-long half. It does NOT
# skip the App.tsx copy or the build, which is the half you iterate on. A `--keep`
# that re-ran `civitai app init` over a populated directory would do nothing
# useful and take just as long, so the flag would be lying about what it saves.
#
# 🔴 THE TWINS INSTALL FROM ONE LOCKFILE, AND THAT IS LOAD-BEARING, NOT TIDINESS.
# The scaffold ships no lockfile and its deps are caret ranges (`vite ^8`,
# `vitest ^4.1`, `@civitai/app-sdk ^0.51`, …), so two independent `npm install`s
# minutes apart resolve differently the moment anything upstream publishes — and
# `package-lock.json` is one of the files the twin-drift guard compares. The run
# would then abort blaming drift, for a reason that has nothing to do with the
# fixtures. This repo already knows those publishes are frequent: `pins-vs-published`
# in `.github/workflows/ci.yml` exists for exactly that. So the FIRST twin resolves
# the tree and the SECOND installs from its lockfile with `npm ci`.
#
# 🔴 `--keep` CANNOT RE-RESOLVE A REUSED TWIN, SO IT CHECKS ONE INSTEAD.
# An earlier draft of this comment claimed `--keep` "still PULLS the lockfile, so
# a run where one twin is reused and the other is rebuilt cannot silently
# diverge". That was FALSE in one of the two directions: the reuse fast path
# returns before any lockfile handling, so `blind rebuilt / asks reused` left the
# reused twin on a lockfile days old, and the run then died at the drift guard
# with "differ in more than src/App.tsx" — the misleading message this file works
# hard to avoid, with no hint that the cure is dropping `--keep`. The SECOND twin
# is now checked against the lockfile in play and told exactly that. (The first
# has nothing to compare against: it DEFINES the reference, reused or not.)
prepare() {
  local name="$1" dir="$2" lock="${3:-}"
  if [ "$KEEP" = "yes" ] \
     && docker exec "dogfood-$name" test -d "/work/$dir/node_modules" >/dev/null 2>&1; then
    if [ -n "$lock" ] && [ -s "$lock" ]; then
      # Hash INSIDE the container and compare hex to hex, so nothing depends on
      # how a shell carries the bytes.
      local have want why
      have=$(docker exec "dogfood-$name" sh -c \
        "sha256sum '/work/$dir/package-lock.json' 2>/dev/null" | cut -d' ' -f1)
      want=$(sha256sum "$lock" | cut -d' ' -f1)
      if [ -z "$have" ] || [ "$have" != "$want" ]; then
        # An explicit variable, not `${have:+a}${have:-b}` — `:-` yields the
        # VALUE when the variable is set, so that idiom spliced the raw digest
        # into the sentence on the branch it is most often read on.
        if [ -z "$have" ]; then why="is missing while the twin has one"; else why="differs from the twin's"; fi
        fatal "dogfood-$name is being REUSED (--keep) but its lockfile $why — the pair would differ in package-lock.json and the drift guard would report that as attribution drift. Re-run WITHOUT --keep."
      fi
    fi
    printf '  reusing the scaffold in dogfood-%s:/work/%s\n' "$name" "$dir"
    return 0
  fi
  docker exec "dogfood-$name" bash -lc "
    npm install -g @civitai/cli >/dev/null 2>&1 || exit 1
    npm install -g 'npm@$NPM_PIN' >/dev/null 2>&1 || exit 1
    cd /work && rm -rf '$dir' && civitai app init '$dir' --template page-money >/dev/null 2>&1
  " || fatal "scaffolding failed for dogfood-$name"
  if [ -n "$lock" ] && [ -s "$lock" ]; then
    docker cp "$lock" "dogfood-$name:/work/$dir/package-lock.json" >/dev/null \
      || fatal "could not seed the lockfile into dogfood-$name"
    docker exec "dogfood-$name" bash -lc \
      "cd '/work/$dir' && npm ci --no-audit --no-fund >/tmp/install.log 2>&1" \
      || fatal "npm ci failed for dogfood-$name — read /tmp/install.log in the container"
  else
    docker exec "dogfood-$name" bash -lc \
      "cd '/work/$dir' && npm install --no-audit --no-fund >/tmp/install.log 2>&1" \
      || fatal "npm install failed for dogfood-$name — read /tmp/install.log in the container"
  fi
}

# Pull a twin's resolved lockfile to the host so the other twin can `npm ci` from
# it. Runs even under --keep, which is the point.
pull_lock() {
  local name="$1"
  [ -n "$LOCK" ] && return 0
  LOCK="$(mktemp)" || fatal "could not make a temp file for the lockfile"
  docker cp "dogfood-$name:/work/ab-ctl-genpost/package-lock.json" "$LOCK" >/dev/null 2>&1 \
    || fatal "could not read the lockfile out of dogfood-$name"
  [ -s "$LOCK" ] || fatal "the lockfile pulled from dogfood-$name is empty"
}

build_app() {
  local name="$1" dir="$2"
  # The scaffold's own vitest specs assert the scaffold's `pm-*` testids and its
  # props, so they cannot typecheck against a replacement App. They are removed
  # rather than skipped: `npm run build` is `tsc --noEmit && vite build`, and a
  # fixture whose build was weakened is not evidence about a real build.
  docker exec "dogfood-$name" bash -lc "
    cd '/work/$dir' && rm -f src/*.test.tsx src/*.test.ts
    npm run build >/tmp/build.log 2>&1
  " || fatal "build failed for dogfood-$name — read /tmp/build.log in the container"
}

printf '=== ctl-scaffold-untouched — the scaffold, installed and BUILT, nothing edited\n'
start ctl-scaffold-untouched
prepare ctl-scaffold-untouched ab-ctl-scaffold
build_app ctl-scaffold-untouched ab-ctl-scaffold

for pair in "ctl-genpost-blind:App.blind.tsx" "ctl-genpost-asks:App.asks.tsx"; do
  name="${pair%%:*}"
  src="${pair#*:}"
  printf '=== %s — %s\n' "$name" "$src"
  start "$name"
  prepare "$name" ab-ctl-genpost "$LOCK"
  pull_lock "$name"
  docker cp "$HERE/$src" "dogfood-$name:/work/ab-ctl-genpost/src/App.tsx" >/dev/null \
    || fatal "could not install $src into dogfood-$name"
  build_app "$name" ab-ctl-genpost
done

# 🔴 THE TWINS MUST DIFFER IN EXACTLY ONE FILE, AND THAT IS CHECKED, NOT TRUSTED.
# The whole attribution rests on it: if a second file drifted — a lockfile, a
# manifest, a scaffold revision — the arm's verdict moving between them would no
# longer be evidence about the consent branch.
#
# 🔴 AN EMPTY DIFFERENCE IS THREE DIFFERENT FINDINGS AND ONLY ONE OF THEM IS
# "they differ in more than App.tsx" — so it gets its own branch. Measured: the
# twins being BYTE-IDENTICAL (both containers handed the same App source), both
# `docker exec`s failing, and `find` matching nothing (`xargs sha256sum` then
# hashes stdin and exits 0) all produce an EMPTY diff. Reported through the
# more-than-one-file message, each would send the operator hunting a drifting
# file that does not exist, while the real state is the opposite — there is no
# control at all. Same split `oracle.sh` keeps between a verdict and "nothing was
# measured".
# 🔴 `-print0`/`-0` IS THE LOAD-BEARING PART, AND A `sed` ON THE OTHER SIDE DOES
# NOT SUBSTITUTE FOR IT. Bare `xargs` word-splits on whitespace, so a file whose
# name contains a space never reaches the comparison at all: `xargs` re-hashes the
# leading token (collapsed by `sort -u`) and errors on the tail to STDERR, which
# the command substitution below does not capture. MEASURED — with an
# uncontrolled second difference named `./src/App.tsx old.bak` present in one twin
# only, the bare form yielded `DIFFER = ./src/App.tsx` and the guard PASSED.
# An earlier round fixed only the PARSING half and left a comment claiming the
# case was closed; it was not. Both halves are needed and both are here.
sums() { docker exec "dogfood-$1" bash -lc \
  'cd /work/ab-ctl-genpost && find . -type f -not -path "./node_modules/*" -not -path "./dist/*" -print0 | sort -z | xargs -0 sha256sum'; }
BLIND_SUMS=$(sums ctl-genpost-blind)
ASKS_SUMS=$(sums ctl-genpost-asks)

# POSITIVE CONTROL, before any comparison: a read that returned nothing compares
# equal to another read that returned nothing, and that is not agreement.
#
# 🔴 THE FLOOR IS 2 BECAUSE 1 IS A REACHABLE FAILURE VALUE, NOT BECAUSE THE TREE
# IS LARGE. When `find` matches nothing, `xargs sha256sum` falls back to hashing
# STDIN and emits exactly ONE conforming line — `e3b0c442…  -` — at rc 0. A floor
# of 1 passes that; a floor of 2 catches it. (A real scaffold yields ~27, so the
# floor is nowhere near the legitimate range either way — but that is a comfort
# margin, not the reason, and an earlier draft gave it as the reason. Do not
# "simplify" this to -ge 1.)
BLIND_N=$(printf '%s\n' "$BLIND_SUMS" | grep -c '^[0-9a-f]\{64\}  ' || true)
ASKS_N=$(printf '%s\n' "$ASKS_SUMS"  | grep -c '^[0-9a-f]\{64\}  ' || true)
[ "$BLIND_N" -ge 2 ] && [ "$ASKS_N" -ge 2 ] \
  || fatal "could not read the twins' file lists (blind=$BLIND_N asks=$ASKS_N hashed line(s)) — NOTHING was compared. This is an instrument failure, not a verdict about the fixtures."

# Strip the `< ` / `> ` marker and the 64-hex digest + two spaces, leaving the
# path VERBATIM — `awk '{print $3}'` would read only the first whitespace token
# of the name. This is the PARSING half; `sums()`'s `-print0` above is the half
# that decides whether such a path is in the stream at all.
DIFFER=$(diff <(printf '%s\n' "$BLIND_SUMS") <(printf '%s\n' "$ASKS_SUMS") \
  | grep -E '^[<>] ' | sed -E 's/^[<>] [0-9a-f]{64}  //' | sort -u)

# 🔴 A SECOND, INDEPENDENT CHECK ON THE SAME HAZARD: the twins must hold the SAME
# NUMBER of files. Its unique value is the case where the path comparison reads
# CLEAN while the trees genuinely differ — what the word-splitting producer did
# (blind=2 asks=3 with `DIFFER` naming only `./src/App.tsx`). Runs AFTER `DIFFER`
# so it can quote it: placed before, it preempted the check below and replaced a
# message naming the file with one that did not.
[ "$BLIND_N" -eq "$ASKS_N" ] \
  || fatal "the twins hold DIFFERENT NUMBERS of files (blind=$BLIND_N asks=$ASKS_N) — one has a file the other does not, so attribution is void. The path comparison reported: ${DIFFER:-<no difference, which means it is not seeing the extra file>}"

if [ -z "$DIFFER" ]; then
  fatal "the twins are IDENTICAL — they differ in NO file, so there is no controlled delta and the pair measures nothing. Expected exactly ./src/App.tsx to differ; check that each container got its OWN App source."
fi
if [ "$DIFFER" != "./src/App.tsx" ]; then
  fatal "the twins differ in more than src/App.tsx — attribution is void. Differing: $(printf '%s' "$DIFFER" | tr '\n' ' ')"
fi
printf '=== twins verified: %s/%s files compared, the only differing file is ./src/App.tsx\n' \
  "$BLIND_N" "$ASKS_N"

printf '\nNow write a synthetic trial transcript per fixture (the oracle derives the\n'
printf 'brief from one) and grade each on both arms. The recipe is in this\n'
printf "directory's README.md.\n"
