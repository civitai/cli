#!/usr/bin/env bash
# Run the trial matrix, 4 trials in flight at a time.
#
# 🔴 AGENT IDENTITY IS ITS OWN AXIS, CROSSED WITH THE MODEL — NEVER BOUND TO IT.
# `civitai agent-setup` branches hard on which agent it detects, so identity and
# model are two variables. An earlier version of this file gave `claude`/`gpt` a
# known identity and `gemini`/`grok` none, which CONFOUNDS them: the resulting
# grid partitioned perfectly by model and was equally well explained by identity,
# and reading it the wrong way would have shipped "Gemini and Grok fail the
# onboarding", which is false. The cross below is what makes the two separable,
# and the harness must be able to reproduce the correction — not just the error.
set -u
# Guard the VALUE, not just the cd: `cd ""` is a silent no-op on bash <= 5.2, so
# `cd "$X" || exit` sails past an empty $X and runs against the inherited cwd.
HERE="$(dirname "$0")"
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "cannot resolve script dir" >&2; exit 1; }
cd "$HERE" || exit 1
mkdir -p runs logs

# model|short   — the driving model.
MODELS=(
  "anthropic/claude-sonnet-5|claude"
  "openai/gpt-5.6-terra|gpt"
  "google/gemini-3.8-flash|gemini"
  "x-ai/grok-4.6|grok"
)
# Override for a cheap smoke run: DOGFOOD_MODELS='google/gemini-3.8-flash|gemini'
# (space-separated rows). Same for DOGFOOD_ENVS. Whole-matrix runs cost real
# money, so there has to be a way to exercise this script without paying for one.
[ -n "${DOGFOOD_MODELS:-}" ] && read -r -a MODELS <<<"$DOGFOOD_MODELS"
# image|short|container-user
ENVS=(
  "df-node-root|noderoot|root"
  "df-node-user|nodeuser|dev"
  "df-ubuntu-apt|ubuntu|dev"
  "df-stale-cli|stale|root"
)
[ -n "${DOGFOOD_ENVS:-}" ] && read -r -a ENVS <<<"$DOGFOOD_ENVS"
# short|agent-env  — the identity the CLI will DETECT. Empty env => `other`,
# which is what every agent with no entry in the CLI's table gets. All three rows
# matter: `other` is where the verdict differs, not an edge case.
# 🔴 `codex` is here because the CLI SERIALISES TOML for it and JSON for claude —
# a different write path, not a different label. ⚠ It exercises the WRITE, not the
# merge-into-existing branch: every trial starts from an empty project and no
# image ships a ~/.codex/config.toml, so the trial CREATES that file. The merge
# paths remain unexercised, as the README and the evidence doc both state.
IDENTITIES=(
  "claudeid|CLAUDECODE=1"
  "codexid|CODEX_SANDBOX=1"
  "other|"
)
[ -n "${DOGFOOD_IDENTITIES:-}" ] && read -r -a IDENTITIES <<<"$DOGFOOD_IDENTITIES"

# The default restricts which identities each env is crossed with, so the full
# cross does not cost 3x for no information: the cheapest env runs ALL THREE
# identities (that is the de-confounding control) and every other env runs one.
# Set FULL_CROSS=1 for every combination. (An earlier draft of this comment named
# an `IDENT_FOR` variable that has never existed.)
FULL_CROSS="${FULL_CROSS:-0}"

# ── the app brief ────────────────────────────────────────────────────────────
# DOGFOOD_BRIEF turns every cell from a SETUP trial into an APP-BUILD trial by
# appending one operator-typed line to the task (runner.py's --brief). Empty =>
# the matrix this file has always run, with a byte-identical task.
#
#   DOGFOOD_BRIEF="$(cat briefs/celsius.brief.txt)" DOGFOOD_TRIAL_PREFIX=ta bash driver.sh
#
# 🔴 AND IT REFUSES TO RUN UNDER THE DEFAULT PREFIX, BECAUSE THE RESUME GUARD
# WOULD OTHERWISE SKIP THE WHOLE MATRIX. Trial ids are `<prefix>-<model>-<env>-
# <identity>` and the guard in run_one() skips any id whose transcript already
# carries an `end` record. Run the setup matrix, then run an app matrix under
# the same prefix, and EVERY cell is skipped as "complete" while this script
# prints MATRIX COMPLETE — the third instance of the failure the two comments
# in run_one() and in the identity loop below already exist to prevent, and the
# only one where the skipped cells hold a DIFFERENT task. Make the operator say
# which namespace the run lands in; there is no safe default to guess.
#
# 🔴 AND IT CARRIES THE BRIEF'S NAME, NOT JUST ITS TEXT. The grader has to map a
# graded cell back to `briefs/<name>.assert.mjs`, and from prose alone the only
# route is an exact match against `briefs/*.brief.txt` — which stops resolving
# every already-run trial the moment a brief file is reworded. Two ways in, and
# they cannot disagree:
#
#   DOGFOOD_BRIEF_NAME=genpost                      # reads briefs/genpost.brief.txt
#   DOGFOOD_BRIEF="$(cat briefs/genpost.brief.txt)" # name derived by exact match
#
# An ad-hoc DOGFOOD_BRIEF matching no brief file is still allowed and still
# runs — it simply records no name, and oracle.sh will then require the brief to
# be named explicitly (and say that it was not verified).
BRIEF="${DOGFOOD_BRIEF:-}"
BRIEF_NAME="${DOGFOOD_BRIEF_NAME:-}"
PREFIX="${DOGFOOD_TRIAL_PREFIX:-t}"
if [ -n "$BRIEF_NAME" ] && [ -z "$BRIEF" ]; then
  [ -f "briefs/$BRIEF_NAME.brief.txt" ] || {
    echo "DOGFOOD_BRIEF_NAME=$BRIEF_NAME has no briefs/$BRIEF_NAME.brief.txt" >&2
    exit 1
  }
  BRIEF="$(cat "briefs/$BRIEF_NAME.brief.txt")"
fi
if [ -z "$BRIEF_NAME" ] && [ -n "$BRIEF" ]; then
  for f in briefs/*.brief.txt; do
    [ -f "$f" ] || continue
    [ "$(cat "$f")" = "$BRIEF" ] && BRIEF_NAME="$(basename "$f" .brief.txt)"
  done
fi
case "$BRIEF" in
  *$'\n'*|*$'\r'*)
    echo "DOGFOOD_BRIEF must be a single line — runner.py refuses a multi-line brief" >&2
    exit 1 ;;
esac
if [ -n "$BRIEF" ] && [ "$PREFIX" = "t" ]; then
  cat >&2 <<'MSG'
refusing to run: DOGFOOD_BRIEF is set but DOGFOOD_TRIAL_PREFIX is still the
default `t`, which is the SETUP matrix's namespace. An app-build cell would
collide with the setup cell of the same name, and the resume guard would skip
it as already complete — reporting MATRIX COMPLETE having run nothing.
Set a distinct namespace, e.g. DOGFOOD_TRIAL_PREFIX=ta
MSG
  exit 1
fi

# ── the credential and the run caps ──────────────────────────────────────────
# A credentialed matrix reaches a REAL account. Everything here is pass-through
# to runner.py, which owns the enforcement; the driver's only job is to refuse
# the two mistakes that are invisible afterwards.
CREDENTIAL="${DOGFOOD_CREDENTIAL_FILE:-}"
APP_PREFIX="${DOGFOOD_APP_PREFIX:-}"
MAX_GENERATIONS="${DOGFOOD_MAX_GENERATIONS:-}"
MAX_SUBMISSIONS="${DOGFOOD_MAX_SUBMISSIONS:-}"
# Pass-through so a matrix can be run under a different output ceiling without
# editing runner.py. Empty => runner.py's own default. It is a ceiling, not a
# spend cap; money is still bounded by runner.py's --max-cost.
MAX_TOKENS="${DOGFOOD_MAX_TOKENS:-}"
# Pass-through for runner.py's --prompt-url: fetch the agent instructions from
# somewhere other than the hosted prompt.md, so a PROPOSED edit to them can be
# measured before it ships. Empty => the hosted default, and the whole matrix is
# then byte-identical to every grid already measured.
#
# 🔴 A MATRIX RUN WITH THIS SET IS NOT EVIDENCE ABOUT THE SHIPPED ENTRYPOINT.
# `grade.sh` reads the CONTAINER, so its verdict line cannot tell you the
# instructions were patched; the trial's `start` record carries `prompt_url` for
# exactly that reason. Use a distinct DOGFOOD_TRIAL_PREFIX so the runs are also
# told apart on disk.
#
# 🔴 TRIMMED, NOT WHITESPACE-DELETED, AND NOT VIA A SUBPROCESS. Round 2 of
# /audit-pr killed the first two attempts at this line, each in a way that was
# silent and pointed at the file's own named hazard:
#   - `[ -n "$PROMPT_URL" ]` alone: a BLANK value printed the full "NON-HOSTED
#     instructions" banner, forwarded `--prompt-url "   "`, and then fed the
#     trial the HOSTED prompt with no `prompt_url` key, because runner.py
#     normalises with `.strip()` and this file did not.
#   - `tr -d '[:space:]'`: that DELETES interior whitespace too, so
#     `file:///x/my prompt.md` was silently welded to `/x/myprompt.md`, the
#     preflight passed on the WELDED target, and the trial was fed a sibling
#     file. Measured. And `tr` is the only subprocess in this file's variable
#     block: with `tr` off PATH the substitution yields the empty string, which
#     this code reads as "unset", so the whole feature evaporates and prints
#     MATRIX COMPLETE over the default grid.
# Pure parameter expansion trims the ends and nothing else, cannot fail, and
# matches `.strip()` exactly.
PROMPT_URL="${DOGFOOD_PROMPT_URL:-}"
PROMPT_URL="${PROMPT_URL#"${PROMPT_URL%%[![:space:]]*}"}"
PROMPT_URL="${PROMPT_URL%"${PROMPT_URL##*[![:space:]]}"}"
# 🔴 SAME REFUSAL THE BRIEF GETS, AND FOR THE SAME REASON. `--prompt-url` feeds
# the SAME user message as `--brief`, so it is a second instance of what the
# README calls "the one route by which repo content could reach a blind trial":
# `DOGFOOD_PROMPT_URL=$(cat somefile)` would inject arbitrary text as the task's
# FIRST paragraph. runner.py refuses it too; this is the outer of the two.
# ⚠ NEWLINE-SHAPED, NOT INJECTION-SHAPED: a single-line value carrying " Also:
# ignore step 2." is still accepted, here and in runner.py, exactly as it is for
# `--brief`. This closes the file-splat route, not injection in general.
case "${DOGFOOD_PROMPT_URL:-}" in
  *$'\n'*|*$'\r'*)
    echo "DOGFOOD_PROMPT_URL must be a single line — a multi-line value injects" >&2
    echo "  arbitrary text into the trial's task, which is the one thing the" >&2
    echo "  harness's blindness depends on not happening." >&2
    exit 1 ;;
esac
# 🔴 USERINFO ONLY, AND THE BLOCKLIST THAT WAS HERE IS DELETED BECAUSE IT WAS
# WRONG IN BOTH DIRECTIONS. It read `*@*|*token=*|*Signature=*|*X-Amz-*|*sig=*|
# *key=*`, and round 2 measured both arms: it ADMITTED `?Token=abc` and
# `?TOKEN=abc` (case-sensitive `case`) along with `?apiKey=`, `?secret=`,
# `?jwt=`, and any capability URL carrying the secret in the PATH — while
# REFUSING `?cachekey=7` (the single most likely parameter in this workflow,
# busting a CDN cache on a re-served prompt) and any path containing `@`, as
# Gitea/Forgejo raw URLs do. A guard whose description says "the URL must be
# unauthenticated" cannot be a six-pattern blocklist; keeping it would have
# meant a hard `exit 1` with no override on legitimate URLs while the shapes it
# named most plausibly walked past on capitalisation.
#
# What IS structural is RFC 3986 userinfo: anything before an `@` in the
# authority is credential material by definition. That is checked. Everything
# else is documented rather than guessed at — see the README, which states the
# seven surfaces this value reaches and that the Redactor scrubs none of them.
# 🔴 THE EXPLICIT DEFAULT IS "NOT MOVED", AND THE DEFAULT IS READ FROM
# runner.py RATHER THAN SPELLED TWICE. Round 2: aligning this file with
# `.strip()` closed the BLANK value and left the other value the two predicates
# disagree on — the hosted URL typed out in full. The driver's test was
# non-emptiness; the runner's is `stripped != USER`. So
# `DOGFOOD_PROMPT_URL="${PATCHED_URL:-https://developer.civitai.com/agent-setup/prompt.md}"`
# — an ordinary wrapper shape — printed the full "NON-HOSTED instructions"
# banner over a grid that ran against the hosted prompt, with no `prompt_url` in
# the transcript. Same symptom, same direction, as the blank-value defect.
#
# `runner.py --print-task` with no other argument IS the default task, which is
# the default URL and nothing else (TestDogfoodTaskDefaultIsByteIdentical pins
# that). Deriving it here means the two layers cannot drift; a second literal
# copy in this file would be a new rot surface for the exact claim it fixes.
if [ -n "$PROMPT_URL" ]; then
  PROMPT_URL_DEFAULT="$(python3 runner.py --print-task 2>/dev/null)" || PROMPT_URL_DEFAULT=""
  if [ -z "$PROMPT_URL_DEFAULT" ]; then
    # Fail LOUD and OPEN: the banner may over-claim, and that is better said than
    # guessed. runner.py still decides the task and the mark correctly either way.
    echo "note: could not derive the default prompt URL from runner.py, so the" >&2
    echo "  banner below cannot tell you whether this URL is actually the default." >&2
    echo "  The transcript's prompt_url key remains authoritative." >&2
  elif [ "$PROMPT_URL" = "$PROMPT_URL_DEFAULT" ]; then
    # Identical to the hosted default => this run IS a default run. Clearing it
    # makes the banner, the forwarding and the start record agree by
    # construction, which is what the previous round claimed and did not deliver.
    PROMPT_URL=""
  fi
fi
case "${PROMPT_URL#*://}" in
  *@*) case "${PROMPT_URL#*://}" in
         */*@*) : ;;                       # the `@` is in the path, not userinfo
         *) echo "DOGFOOD_PROMPT_URL carries URL userinfo (user[:password]@host)." >&2
            echo "  Refused: this value is recorded in the transcript, printed to stderr," >&2
            echo "  passed to the container's own curl and sent to OpenRouter inside the" >&2
            echo "  task, and the Redactor only knows strings read from the credential" >&2
            echo "  FILE — it scrubs none of them. Serve the file unauthenticated." >&2
            echo "  ⚠ This checks userinfo ONLY. A token in a query parameter or in the" >&2
            echo "  path is NOT detected and leaks just as widely." >&2
            exit 1 ;;
       esac ;;
esac
# Same shape, for runner.py's --max-steps. Empty => runner.py's own default of 40.
#
# 🔴 40 CANNOT CARRY A BUILD-AND-SHIP BRIEF, AND UNTIL THIS KNOB EXISTED NO
# DRIVER-LAUNCHED CELL COULD RUN UNDER ANY OTHER NUMBER. Measured on
# `at1-mimo-noderoot-claudeid` (brief `t1`, xiaomi/mimo-v2.5): the trial ended
# `stop: max-steps` at step 40 with `generations: 0`, `submissions: 0`, and its
# last three tool calls were `npx tsc --noEmit`, `npm run build`,
# `civitai app validate` — it had BUILT the app and was cut off immediately
# before the submit-and-media phase the brief is graded on. A plain build-only
# `celsius` cell took 65 steps, so 40 is below the floor for any brief that
# builds AND ships, and that cell was ungradeable for a reason that had nothing
# to do with the model or the product. Every rank-3-style "caps 80/$0.50" run on
# record must therefore have been a direct runner.py invocation; the driver could
# not have produced one.
#
# 🔴 AND THERE IS DELIBERATELY NO WARNING ON A HIGH VALUE. A step cap is not a
# spend cap (runner.py's own comment at the flag says so: every turn resends the
# whole history, so cumulative prompt tokens grow O(n^2) in steps). The bound
# that IS about money is --max-cost, which is denominated in dollars and so is
# model-priced; a step number is not. The only (steps, cost) pair we have is that
# same cell — 40 steps = 856,834 prompt tokens = $0.0113, heavily cache-
# discounted at ~$0.013/M effective — one point on one model, which fixes no
# quadratic. Reading cost ~ steps^2 off it FOR SCALE ONLY (an extrapolation, not
# a measurement), the $1.00 --max-cost default binds that model near ~375 steps
# and a 100x-pricier one near ~38: any single threshold typed here would be wrong
# for most of the matrix while --max-cost is right for all of it. Enforcement
# stays in runner.py.
MAX_STEPS="${DOGFOOD_MAX_STEPS:-}"

# ── `app listing set-text`: OFF by default, and the opt-in is loud ───────────
# runner.py refuses `civitai app listing set-text` under a credential unless
# --allow-listing-text is passed, and until now that flag was reachable only by
# invoking runner.py directly. So the one instrument that drives real agents end
# to end could not reach the command that sets a listing DESCRIPTION, which made
# it blind to exactly the defect `civitai app submit`'s listing-completeness gate
# was added for: a matrix trial physically could not have fixed an
# `empty-description`, so its leaving one behind measured the harness, not the
# product.
#
# 🔴 THE DEFAULT STAYS OFF, AND THAT IS NOT CAUTION FOR ITS OWN SAKE. `set-text`
# rewrites a listing's public tagline/description/category IN PLACE on every
# listing status — not a "material" change, so no revision and no moderator
# review — it is public the moment it returns, and this CLI ships NO command that
# restores the previous value. The account a credentialed matrix runs against owns
# real published listings. runner.py's own comment at APP_LISTING_DESTRUCTIVE
# carries the rest.
#
# 🔴 ONLY THE LITERAL `1` ARMS IT, AND ANYTHING ELSE IS A HARD REFUSAL RATHER THAN
# A SILENT OFF. The obvious `[ -n "$X" ]` test arms on `0`, on `false` and on `no`
# — three spellings an operator reaching for OFF would plausibly type — and for a
# write with no undo, guessing in either direction is wrong. An unset variable is
# the only quiet default.
ALLOW_LISTING_TEXT="${DOGFOOD_ALLOW_LISTING_TEXT:-}"
case "$ALLOW_LISTING_TEXT" in
  ""|1) ;;
  *)
    cat >&2 <<MSG
refusing to run: DOGFOOD_ALLOW_LISTING_TEXT=$ALLOW_LISTING_TEXT is not a value
this script will act on. It permits \`civitai app listing set-text\`, which
rewrites a listing's PUBLIC tagline/description/category in place with no undo,
so it is armed by the literal 1 and by nothing else. Unset it to leave the
default refusal in place.
MSG
    exit 1 ;;
esac

if [ -n "$CREDENTIAL" ]; then
  # 🔴 SAME RESUME-GUARD TRAP AS THE BRIEF, AND WORSE. A credentialed cell run
  # under the setup matrix's namespace is skipped as "complete" by an
  # UNCREDENTIALED transcript of the same id — so the run that was supposed to
  # reach the account reaches nothing, and MATRIX COMPLETE is printed over it.
  if [ "$PREFIX" = "t" ]; then
    cat >&2 <<'MSG'
refusing to run: DOGFOOD_CREDENTIAL_FILE is set but DOGFOOD_TRIAL_PREFIX is
still the default `t`, the SETUP matrix's namespace. Set a distinct namespace,
e.g. DOGFOOD_TRIAL_PREFIX=tc
MSG
    exit 1
  fi
  [ -s "$CREDENTIAL" ] || {
    echo "refusing to run: DOGFOOD_CREDENTIAL_FILE=$CREDENTIAL is missing or empty" >&2
    exit 1
  }
  # 🔴 A CREDENTIALED MATRIX WITH NO APP PREFIX CANNOT BE TOLD FROM THE
  # ACCOUNT'S REAL APPS AFTERWARDS, and the prefix is also what the runner's
  # refusal keys on — without it, every mutating `app` command is ungated.
  [ -n "$APP_PREFIX" ] || {
    echo "refusing to run: a credentialed matrix needs DOGFOOD_APP_PREFIX, e.g." >&2
    echo "  DOGFOOD_APP_PREFIX=dogfood4-  — it is what keeps the trial off the" >&2
    echo "  account's existing apps and what makes its own apps identifiable." >&2
    exit 1
  }
  # 🔴 ANNOUNCED ONCE, BEFORE ANY CELL STARTS, AND ONLY WHERE IT CAN BITE. The
  # flag is inert without a credential (runner.py's refusals are conditional on
  # `armed`), so a banner on an uncredentialed run would teach the operator to
  # skip the one they need to read. The prefix is quoted because it is the only
  # thing keeping the writes off the account's own listings.
  if [ "$ALLOW_LISTING_TEXT" = "1" ]; then
    echo "🔴 DOGFOOD_ALLOW_LISTING_TEXT=1 — every cell of this matrix may run" >&2
    echo "   \`civitai app listing set-text\` against the REAL account. It rewrites a" >&2
    echo "   listing's public tagline/description/category IN PLACE, on any listing" >&2
    echo "   status, with no revision, no moderator review and NO undo in this CLI." >&2
    echo "   Only listings under DOGFOOD_APP_PREFIX=$APP_PREFIX should be reachable." >&2
  fi
elif [ "$ALLOW_LISTING_TEXT" = "1" ]; then
  # Not fatal: an uncredentialed matrix reaches no account, so the flag changes
  # nothing. Said out loud anyway, because an operator who set it meant to arm
  # something and is entitled to know it did not.
  echo "note: DOGFOOD_ALLOW_LISTING_TEXT=1 is inert without DOGFOOD_CREDENTIAL_FILE" >&2
  echo "      — an uncredentialed trial reaches no listing to rewrite." >&2
fi

if [ -n "$PROMPT_URL" ]; then
  # Loud, on stderr, before any trial starts. Not fatal — this is a supported
  # mode — but the verdict it produces is about PROPOSED instructions, and
  # nothing downstream says so: grade.sh reads the container.
  echo "🔴 DOGFOOD_PROMPT_URL=$PROMPT_URL — the trials below are being fed" >&2
  echo "   NON-HOSTED instructions. Their verdicts are evidence about that URL," >&2
  echo "   NOT about the shipped entrypoint. Each trial's start record carries" >&2
  echo "   prompt_url; grade.sh's verdict line does not." >&2
  # 🔴 REACHABILITY PREFLIGHT — this file's stated job is "to refuse the two
  # mistakes that are invisible afterwards", and an unreachable URL is exactly
  # one. `grade.sh` reads the container, so a model that could not fetch its
  # instructions produces an ordinary `no`, indistinguishable from a product
  # defect — after paying up to `timeout 1500` per cell plus the OpenRouter
  # spend for the whole grid.
  #
  # ⚠ IT IS A HOST-SIDE CHECK AND THE TRIAL FETCHES FROM INSIDE A CONTAINER, so
  # this can pass where the container still cannot reach it (`127.0.0.1` is the
  # classic case: from the container that is the container). It rules out typos,
  # an unstarted server and a missing file; it does not certify container
  # reachability.
  #
  # 🔴 `-q` IS LOAD-BEARING AND curl's STDERR IS PRINTED, NOT DISCARDED. Round 2
  # measured both: without `-q` this reads `~/.curlrc`, and one plausible
  # hardening line (`--proto =https`) made the preflight hard-refuse the README's
  # OWN recommended `http://$SRV:8099/...` recipe while blaming the URL; and
  # `2>/dev/null` threw away the one thing `-S` exists to produce, so a DNS
  # failure, a 404 and a missing file all emitted the identical three lines. A
  # guard whose purpose is to replace an opaque failure with a stated reason must
  # not manufacture a different opaque failure.
  if [ "${DOGFOOD_SKIP_PROMPT_PREFLIGHT:-}" = "1" ]; then
    echo "   (preflight SKIPPED by DOGFOOD_SKIP_PROMPT_PREFLIGHT=1 — an unreachable" >&2
    echo "    URL will present as every cell failing for no stated reason.)" >&2
  elif command -v curl >/dev/null 2>&1; then
    pf_err="$(curl -qfsS --proto '=http,https,file' --max-time 10 -o /dev/null "$PROMPT_URL" 2>&1)"
    pf_rc=$?
    if [ "$pf_rc" -ne 0 ]; then
      echo "refusing to run: could not fetch DOGFOOD_PROMPT_URL from this host." >&2
      echo "  curl said: ${pf_err:-(no message)}" >&2
      echo "  Every cell would otherwise fail as an ordinary CLOSING_CONDITION=no," >&2
      echo "  which is indistinguishable from a product defect, after paying for the" >&2
      echo "  whole grid. If this host cannot reach a URL the CONTAINER can, set" >&2
      echo "  DOGFOOD_SKIP_PROMPT_PREFLIGHT=1." >&2
      exit 1
    fi
    echo "   (preflight: fetchable from this HOST — not a guarantee the container" >&2
    echo "    can reach it; 127.0.0.1 would pass here and fail in the trial.)" >&2
  else
    echo "   (preflight SKIPPED: no curl on PATH — an unreachable URL will present" >&2
    echo "    as every cell failing for no stated reason.)" >&2
  fi
fi

run_one() {  # model short image ienv trial user
  local model=$1 image=$3 ienv=$4 trial=$5 euser=$6
  # 🔴 GATE ON COMPLETION, NOT ON EXISTENCE. runner.py opens transcript.jsonl in
  # "w" mode BEFORE it creates the container or makes its first API call, so a
  # trial killed by the timeout below — or one that died on a retry storm —
  # leaves a non-empty file with no `end` record. Keyed on `-f`, that trial is
  # skipped forever and the matrix reports COMPLETE while missing it; grade.sh
  # then scores its half-built container as an ordinary failure, which is
  # indistinguishable from a real product defect.
  if [ -f "runs/$trial/transcript.jsonl" ] \
     && grep -q '"kind": "end"' "runs/$trial/transcript.jsonl"; then
    echo "skip $trial (complete)"; return
  fi
  if [ -f "runs/$trial/transcript.jsonl" ]; then
    echo "re-running $trial (previous attempt did not finish)"
  fi
  local args=(--model "$model" --image "$image" --trial "$trial" --user "$euser" --out runs)
  [ -n "$ienv" ] && args+=(--agent-env "$ienv")
  [ -n "$BRIEF" ] && args+=(--brief "$BRIEF")
  [ -n "$BRIEF_NAME" ] && args+=(--brief-name "$BRIEF_NAME")
  [ -n "$PROMPT_URL" ] && args+=(--prompt-url "$PROMPT_URL")
  # The credential is passed as a PATH, never as a value — see runner.py's
  # credential section for the three surfaces that keeps it off.
  [ -n "$CREDENTIAL" ] && args+=(--credential-file "$CREDENTIAL")
  [ -n "$APP_PREFIX" ] && args+=(--app-prefix "$APP_PREFIX")
  [ -n "$MAX_GENERATIONS" ] && args+=(--max-generations "$MAX_GENERATIONS")
  [ -n "$MAX_SUBMISSIONS" ] && args+=(--max-submissions "$MAX_SUBMISSIONS")
  [ -n "$MAX_TOKENS" ] && args+=(--max-tokens "$MAX_TOKENS")
  [ -n "$MAX_STEPS" ] && args+=(--max-steps "$MAX_STEPS")
  # Passed only on the literal 1 — see the ALLOW_LISTING_TEXT block above. Omitted
  # otherwise, so runner.py's own default refusal is what applies.
  [ "$ALLOW_LISTING_TEXT" = "1" ] && args+=(--allow-listing-text)
  ( timeout 1500 python3 runner.py "${args[@]}" >"logs/$trial.out" 2>"logs/$trial.err"
    echo "done $trial rc=$?" ) &
}

N=0
for m in "${MODELS[@]}"; do
  IFS='|' read -r model ms <<<"$m"
  for e in "${ENVS[@]}"; do
    IFS='|' read -r image es euser <<<"$e"
    for i in "${IDENTITIES[@]}"; do
      IFS='|' read -r is ienv <<<"$i"
      # The de-confounding cell: every model against EVERY identity on one env.
      # Without it, model and identity are two names for the same column.
      #
      # 🔴 THE NON-CROSSED ENVS RUN THE FIRST IDENTITY IN THE LIST, NOT A LITERAL.
      # This used to compare against the literal `claudeid`, which silently dropped
      # every non-noderoot env whenever DOGFOOD_IDENTITIES did not happen to contain
      # that exact name — including BOTH envs rank 34's closing condition is graded
      # on — while still printing MATRIX COMPLETE. Measured:
      # `DOGFOOD_IDENTITIES='codexid|…' bash driver.sh` ran 4 trials, all noderoot.
      # That is the same "reports COMPLETE while cells are missing" failure the
      # resume guard above exists to prevent, arriving by a different route.
      if [ "$FULL_CROSS" != "1" ] && [ "$es" != "noderoot" ] \
         && [ "$is" != "${IDENTITIES[0]%%|*}" ]; then
        continue
      fi
      run_one "$model" "$ms" "$image" "$ienv" "${PREFIX}-${ms}-${es}-${is}" "$euser"
      N=$((N+1))
      if [ $((N % 4)) -eq 0 ]; then wait; fi
    done
  done
done
wait
echo "MATRIX COMPLETE — $N trial(s). Grade each: bash grade.sh <trial-id> <container-user>"
echo "🔴 A per-model verdict is only readable against the SAME identity. Compare"
echo "   the t-<model>-noderoot-<id> cells across ids — this run used: ${IDENTITIES[*]%%|*}"
echo "   — before attributing any difference to the model."
