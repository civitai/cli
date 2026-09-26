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
# `v0.1.1` fix graded `RENDER=yes observed=ready`. Three more of the seven
# fixtures grade `no` on the arm, and the handoff's own "How to verify" section
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
[ "${1:-}" = "--keep" ] && KEEP=yes

fatal() { printf 'build.sh: %s\n' "$1" >&2; exit 2; }

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
prepare() {
  local name="$1" dir="$2"
  if [ "$KEEP" = "yes" ] \
     && docker exec "dogfood-$name" test -d "/work/$dir/node_modules" >/dev/null 2>&1; then
    printf '  reusing the scaffold in dogfood-%s:/work/%s\n' "$name" "$dir"
    return 0
  fi
  docker exec "dogfood-$name" bash -lc "
    npm install -g @civitai/cli >/dev/null 2>&1 || exit 1
    npm install -g npm@$NPM_PIN >/dev/null 2>&1 || exit 1
    cd /work && rm -rf '$dir' && civitai app init '$dir' --template page-money >/dev/null 2>&1
    cd '/work/$dir' && npm install --no-audit --no-fund >/tmp/install.log 2>&1
  " || fatal "prepare failed for dogfood-$name — read /tmp/install.log in the container"
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
  prepare "$name" ab-ctl-genpost
  docker cp "$HERE/$src" "dogfood-$name:/work/ab-ctl-genpost/src/App.tsx" >/dev/null \
    || fatal "could not install $src into dogfood-$name"
  build_app "$name" ab-ctl-genpost
done

# 🔴 THE TWINS MUST DIFFER IN EXACTLY ONE FILE, AND THAT IS CHECKED, NOT TRUSTED.
# The whole attribution rests on it: if a second file drifted — a lockfile, a
# manifest, a scaffold revision — the arm's verdict moving between them would no
# longer be evidence about the consent branch.
sums() { docker exec "dogfood-$1" bash -lc \
  'cd /work/ab-ctl-genpost && find . -type f -not -path "./node_modules/*" -not -path "./dist/*" | sort | xargs sha256sum'; }
DIFFER=$(diff <(sums ctl-genpost-blind) <(sums ctl-genpost-asks) \
  | grep -E '^[<>]' | awk '{print $3}' | sort -u)
if [ "$DIFFER" != "./src/App.tsx" ]; then
  fatal "the twins differ in more than src/App.tsx — attribution is void. Differing: ${DIFFER:-<none>}"
fi
printf '=== twins verified: the only differing file is ./src/App.tsx\n'

printf '\nNow write a synthetic trial transcript per fixture (the oracle derives the\n'
printf 'brief from one) and grade each on both arms. The recipe is in this\n'
printf "directory's README.md.\n"
