#!/usr/bin/env bash
# The SHIP VERDICT. Grades T0 = SUBMITTED for a credentialed app-build trial run
# with the `ship` brief: did THIS trial put THIS trial's own app into the
# moderation queue, inside the window THIS trial ran in?
#
#   ship.verdict.sh <trial-id> [container-user]
#
# 🔴 IT ALSO GRADES THE T1 PUBLISH FLOOR, AND THAT IS A SECOND VERDICT ON THE SAME
# LINE RATHER THAN A SECOND SCRIPT. The `t1` brief is the `ship` brief plus "give
# the listing an icon and a cover", so a T1 cell is a SHIP cell plus one more
# account read — and the hard part of the ship verdict, the identity check that
# attributes server state to THIS trial's own blockIds read out of the container's
# own manifests, is exactly what the floor check needs too. A forked grader would
# be that predicate open-coded at two sites, which is the shape `_esc.sh` and
# `MANIFEST_FIND` already exist to prevent here. So: `SHIP=` is T0, `FLOOR=` is the
# publish floor, `T1=` is the conjunction, and the EXIT CODE still belongs to the
# ship half alone (see the floor section for why). A `ship` cell reads its own
# fields unchanged and gets the floor state for free.
#
# Exit 0 = something was measured (SHIP=yes or SHIP=no).
# Exit 2 = NOTHING was measured. Same split, and for the same reason, as
# oracle.sh: a harness that could not reach the account must not emit the verdict
# a trial that submitted nothing earns. `unmeasured` is a THIRD STATE and is not
# `no`.
#
# 🔴 WHY THIS IS NOT THE RENDER ORACLE, AND WHY IT COULD NOT BE.
# `oracle.sh` is browser-shaped: it serves the built block and grades the DOM.
# A submission leaves no trace in the DOM at all — it is server state, reachable
# only through the account the trial authenticated as. So this reads
# `civitai app status --json` from inside the trial's own container, with the
# trial's own credential, and grades the rows it comes back with.
#
# 🔴 THE IDENTITY CHECK IS THE WHOLE POINT, AND WITHOUT IT THIS SCRIPT IS A LIE.
# The account a credentialed trial runs as owns roughly a dozen REAL published
# apps, each of which has a submission row. A verdict that asked only "is there a
# pending submission on this account?" would grade the operator's own back
# catalogue as the trial's success — a negative control passing for the wrong
# reason, which this arc has already been bitten by twice (the brief that
# defaulted to `celsius`; the negative control that passed off a shell
# metacharacter). So a row counts only when ALL THREE hold:
#
#   1. status == "pending"           — it is in the moderation queue NOW
#   2. blockId ∈ the trial's own     — read out of the block.manifest.json files
#      manifests                       the trial itself created under /work
#   3. submittedAt ∈ the run window  — [start.t, end.t] of THIS trial's own
#                                       transcript, ± a clock-skew grace
#
# Any one of them alone is satisfied by a pre-existing app. (2) alone would also
# pass a re-grade of a trial whose submission happened on a different day.
#
# 🔴 IT MUTATES NOTHING AND SPENDS NOTHING. `civitai app status` is a genuine
# read — unlike `civitai app listing status`, which opens a shadow revision
# server-side (see runner.py's APP_MUTATING comment). The LISTING form is used
# deliberately, with no slug argument: `civitai app status <slug>` ERRORS when
# the app has no submissions, and that error is indistinguishable from a network
# failure or an expired credential. The listing succeeds whenever auth and the
# network work, so "the call worked and no row names our app" (a real `no`) is
# separable from "the call did not work" (unmeasured). That separation is the
# reason for the shape.
set -uo pipefail

HERE="$(dirname "$0")"
# Guard the VALUE, not just the cd — `cd ""` is a silent no-op on bash <= 5.2.
[ -n "$HERE" ] && [ -d "$HERE" ] || { echo "ship.verdict: cannot resolve script dir" >&2; exit 2; }

TRIAL="${1:?trial id}"
U="${2:-root}"
C="dogfood-$TRIAL"
RUNS="${DOGFOOD_RUNS:-$HERE/runs}"
TRANSCRIPT="$RUNS/$TRIAL/transcript.jsonl"
# Clock skew between this host and the platform's `submittedAt`. The window
# exists to exclude submissions from OTHER DAYS, so a few minutes of slack costs
# the check nothing and removes a whole class of false `no`.
GRACE="${DOGFOOD_SHIP_GRACE_S:-300}"

fatal() { printf 'ship.verdict: %s — nothing was measured (this is NOT a trial that failed to submit)\n' "$1" >&2; exit 2; }

# ── app-controlled text must not be able to write the verdict ────────────────
# 🔴 THE THING BEING GRADED WRITES THE TEXT THIS SCRIPT PRINTS, AND THIS SCRIPT'S
# OUTPUT IS PARSED. `blockId` comes out of the trial's own `block.manifest.json`;
# the manifest PATHS out of directories the trial itself created, which `find`
# prints verbatim — so a directory name carrying a NEWLINE is a whole forged line,
# and one carrying a SPACE is an extra field. A submission row's
# `id`/`blockId`/`status`/`submittedAt` come back across a process boundary from
# the platform. The rule — percent-hex, not `printf '%q'` — lives in `_esc.sh`,
# the same file `oracle.sh` sources.
#
# 🔴 IT IS ONE FILE BECAUSE THIS SCRIPT IS THE EVIDENCE THAT TWO COPIES IS ONE
# COPY TOO MANY. #728 closed exactly this class in `oracle.sh` and this file kept
# the identical forgeable `printf 'manifests=%s\n%s\n'` for a month, one directory
# away, because the rule lived in the other script's body.
ESC="$HERE/_esc.sh"
[ -f "$ESC" ] || fatal "missing $ESC — every app-controlled value on this script's output is escaped by it"
# shellcheck source=_esc.sh
. "$ESC"
# 🔴 SOURCING MUST NOT BE ALLOWED TO FAIL QUIETLY, AND WITHOUT `set -e` IT WOULD.
# An unreadable or half-written `_esc.sh` leaves `tok` undefined; `$(tok "$v")` then
# writes `command not found` to stderr and substitutes the EMPTY STRING, so every
# app-controlled field would silently render BLANK on a stream a consumer parses.
# A blank field is a worse reading than the raw value, not a safer one.
for _f in esc tok prose pathdec; do
  declare -F "$_f" >/dev/null \
    || fatal "$ESC did not define \`$_f\` — app-controlled values would render unescaped or blank"
done
unset _f
# The manifest discovery command lives there too — see the INPUT rule in `_esc.sh`.
# An empty one would find no manifest, and this script exits 2 on that, so the
# failure would read as "the trial created no app" rather than as a broken grader.
[ -n "${MANIFEST_FIND:-}" ] \
  || fatal "$ESC did not define \`MANIFEST_FIND\` — every trial would grade as having created no app"

# ── the instrument, before any verdict ───────────────────────────────────────
command -v docker >/dev/null || fatal "no docker on PATH"
command -v jq     >/dev/null || fatal "no jq on PATH (the submission listing is JSON)"
command -v date   >/dev/null || fatal "no date on PATH (submittedAt has to be ordered)"

# ── the run window, read off the trial's own transcript ──────────────────────
# 🔴 ONLY THE `start` AND `end` RECORDS. Everything between them is the MODEL'S
# OWN OUTPUT, and a grader that branched on any of it would be scoring the trial
# by what it SAID rather than by what the account holds — the mistake grade.sh's
# AGENT_ID read and oracle.sh's brief derivation both exist to avoid.
[ -f "$TRANSCRIPT" ] || fatal "no transcript for '$TRIAL' at $TRANSCRIPT (set DOGFOOD_RUNS to the directory the trial was driven from). Without it there is no run window, and without a window a pre-existing submission grades as this trial's"
WIN_START=$(jq -r 'select(.kind=="start") | .t' "$TRANSCRIPT" 2>/dev/null | head -1)
[ -n "$WIN_START" ] && [ "$WIN_START" != "null" ] \
  || fatal "$TRANSCRIPT has no \`start\` record carrying a timestamp — the run window cannot be established"
WIN_END=$(jq -r 'select(.kind=="end") | .t' "$TRANSCRIPT" 2>/dev/null | tail -1)
WIN_END_SRC=end-record
if [ -z "$WIN_END" ] || [ "$WIN_END" = "null" ]; then
  # A trial killed by the driver's timeout leaves no `end` record. Its container
  # and its account effects are still real, so it is gradeable — the upper bound
  # is simply "now", and the cell says which bound it used.
  WIN_END=$(date -u +%s)
  WIN_END_SRC=now-no-end-record
fi
# Integer seconds. `t` is a float (time.time()); `date`, and the comparison
# below, want whole seconds.
WIN_START=${WIN_START%%.*}
WIN_END=${WIN_END%%.*}
LO=$((WIN_START - GRACE))
HI=$((WIN_END + GRACE))

# The brief this trial was run with. REPORTED, never a gate: the negative
# controls this script has to fail include trials run with OTHER briefs (an
# untouched scaffold from a `genpost` cell), and refusing to grade them would
# take away exactly the controls that prove the verdict can go `no`.
T_BRIEF_NAME=$(jq -r 'select(.kind=="start") | .brief_name // ""' "$TRANSCRIPT" 2>/dev/null | head -1)

# ── the container ────────────────────────────────────────────────────────────
# `docker inspect` succeeds for a container in ANY state, and against a stopped
# one every exec fails with empty stdout — which reads exactly like an account
# with no matching submission.
STATE=$(docker inspect -f '{{.State.Status}}' "$C" 2>/dev/null)
[ -n "$STATE" ] || fatal "no such container: $C"
[ "$STATE" = "running" ] || fatal "container $C is $STATE, not running"

# `x` runs a command text this script AUTHORED. Nothing app-controlled may be
# spliced into it.
x() { docker exec -u "$U" -w /work "$C" bash -lc "$1" 2>/dev/null; }

# 🔴 AN APP-CONTROLLED VALUE CROSSES INTO THE CONTAINER THROUGH THE ENVIRONMENT,
# NEVER THROUGH THE COMMAND TEXT. This script's one such site was `x "cat '$m'"`,
# where `$m` is a path `find` read off the trial's own filesystem — so a directory
# named with a single quote ran the rest of its own name as a command inside the
# grader's exec, and the grader is the thing whose output decides the verdict.
# Deliberately not hand-rolled quote escaping: `docker exec -e` passes the value as
# one argv element, which has no edge cases to get wrong.
xe() {
  local -a ev=()
  while [ "$#" -gt 1 ]; do ev+=(-e "$1"); shift; done
  docker exec ${ev[@]+"${ev[@]}"} -u "$U" -w /work "$C" bash -lc "$1" 2>/dev/null
}

# ── WHOSE app: the slugs this trial created, read out of the container ───────
# Not the trial id, not a directory name, not the `--app-prefix`: the manifests
# the trial itself wrote. The prefix would be a claim by whoever started the run;
# the manifests are a measurement of the box. (The prefix cap is what keeps them
# honest during the run — that is a different mechanism, enforced elsewhere.)
#
# 🔴 ONE LINE PER MANIFEST, WHATEVER THE TRIAL NAMED ITS DIRECTORIES. Both the
# discovery command and the decode are `MANIFEST_FIND`/`pathdec` from `_esc.sh` —
# one rule, one place, because the `grep -c .` line count below was ALSO wrong in
# `oracle.sh`, in the same direction, for the same reason. A directory name
# carrying a newline used to count as two manifests AND split into two halves in
# the loop that reads each `blockId`, so neither half named a file, `TRIAL_SLUGS`
# came back empty, and a trial that really did submit graded as exit-2 unmeasured.
MANIFESTS=$(x "$MANIFEST_FIND")
APP_COUNT=$(printf '%s' "$MANIFESTS" | grep -c . || true)
if [ "$APP_COUNT" -eq 0 ]; then
  # 🔴 UNMEASURED, NOT `no`, AND THE REASON IS THE IDENTITY CHECK ITSELF. With no
  # manifest there is no blockId, so conjunct (2) has no left-hand side: this
  # script cannot tell "submitted nothing" from "submitted something it cannot
  # attribute". Emitting `SHIP=no` here would be a verdict computed without the
  # only field that makes the verdict mean anything.
  fatal "no block.manifest.json under /work in $C — the trial created no app, so there is no blockId to check a submission against"
fi
# EVERY manifest's blockId, not the first one's. A trial that scaffolded twice
# has two identities and either is legitimately its own; picking one by sort
# order would be an arbitrary choice that changes the verdict.
#
# Normalised the same way `appapi.SameSlug` normalises: trimmed and folded to
# lower case. `manifest.Load` is a bare json.Unmarshal with no schema
# validation, so a hand-edited `"blockId": " Ab-Ship "` reaches this line, and
# an exact byte compare against the server's spelling would silently miss it.
TRIAL_SLUGS=$(
  printf '%s\n' "$MANIFESTS" | while IFS= read -r m; do
    [ -n "$m" ] || continue
    pathdec m "$m"
    xe "DF_M=$m" 'cat "$DF_M"' | jq -r '.blockId // empty' 2>/dev/null
  done | tr 'A-Z' 'a-z' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' \
       | grep -v '^$' | sort -u
)
[ -n "$TRIAL_SLUGS" ] || fatal "the trial's $APP_COUNT manifest(s) declare no blockId — nothing to check a submission against"

# ── the account, read with the trial's own credential ────────────────────────
x 'command -v civitai >/dev/null' || fatal "no civitai on PATH in $C — the account cannot be read from where the trial ran"

# 🔴 STDOUT ONLY. `app status` writes its API-cap note to STDERR precisely so
# that --json stdout stays a pure parseable payload; merging the streams would
# feed that sentence to jq.
STATUS_JSON=$(docker exec -u "$U" -w /work "$C" bash -lc 'civitai app status --json' 2>/dev/null)
SRC=$?
if [ "$SRC" != "0" ]; then
  fatal "\`civitai app status --json\` exited $SRC inside $C (no network, no credential, an expired token, or the API refused). A failed read is not evidence of an absent submission"
fi
ROWS=$(printf '%s' "$STATUS_JSON" | jq -c '.submissions // empty' 2>/dev/null)
[ -n "$ROWS" ] || fatal "\`civitai app status --json\` returned no \`submissions\` array this script can read: $(printf '%s' "$STATUS_JSON" | head -c 300)"
ACCOUNT_N=$(printf '%s' "$ROWS" | jq -r 'length' 2>/dev/null)

# `$TRIAL` is argv and is already constrained by the container check above — an id
# carrying whitespace names no container, so it has gone through `fatal` — but it
# goes through `tok` anyway rather than resting on that reasoning, which a later
# edit to that check would silently invalidate.
printf '=== ship verdict: %s ===\n' "$(tok "$TRIAL")"
printf -- '--- run window\n'
# `window_end_source` is this script's own vocabulary (two fixed strings) and is
# the only unwrapped field here. The bounds are read off the transcript, the grace
# out of the environment, and the brief name off the transcript again.
printf 'window_start=%s window_end=%s window_end_source=%s grace_s=%s brief=%s\n' \
  "$(tok "$WIN_START")" "$(tok "$WIN_END")" "$WIN_END_SRC" "$(tok "$GRACE")" \
  "$(tok "${T_BRIEF_NAME:-none}")"
printf -- '--- the trial'"'"'s own apps\n'
# The count, then one path per line. 🔴 THESE PATHS ARE DIRECTORY NAMES THE TRIAL
# CREATED and `find` prints them verbatim, so this was the strongest forgery on
# this script: a directory named `a<LF>ship_trial=… SHIP=yes …` makes `find` emit a
# complete summary-shaped line, and `shipField` returns off the FIRST
# `ship_trial=`-prefixed line it sees — which is this one, printed long before the
# real verdict. A space in a path is the milder version, an extra field.
printf 'manifests=%s\n' "$APP_COUNT"
while IFS= read -r m; do
  [ -n "$m" ] || continue
  # Decode BEFORE `tok`, so a path carrying a newline renders as the `%0A` it
  # actually holds rather than as the 0x01 the transport used to carry it.
  pathdec m "$m"
  printf '%s\n' "$(tok "$m")"
done <<<"$MANIFESTS"
# `blockId` is the app's own choice of name. It is folded to lower case upstream,
# which happens to deny it the literal token `SHIP=` — but every other field on
# the summary line below is spelled in lower case (`matched=`, `sub_status=`,
# `submitted_at=`, `window=`, `grace_s=`), `trial_slugs=` is printed before all of
# them, and those are the fields a reader checks the verdict AGAINST. Do not read
# the case fold as the guard: it is an accident of a normalisation that exists for
# slug matching, and `sub_block=` carries the same name back from the server with
# its case intact.
printf 'trial_slugs=%s\n' "$(tok "$(printf '%s' "$TRIAL_SLUGS" | tr '\n' ',' | sed 's/,$//')")"
printf -- '--- the account\n'
# 🔴 THE COUNT IS ON THE CELL SO A ZERO CANNOT PASS AS EVIDENCE. An empty
# listing is what a probe wired to nothing returns, and it is also what a fresh
# account legitimately returns. On the operator's account this number is ~14 and
# is the POSITIVE CONTROL that the read reached a real account at all; a 0 here
# makes every `no` below suspect rather than conclusive. It is reported, not
# refused on, because a genuinely empty account is a legal state.
printf 'account_submissions=%s\n' "$(tok "$ACCOUNT_N")"

# ── the PUBLISH FLOOR: an icon AND a cover, on the trial's own listing ───────
# 🔴 THIS IS THE T1 HALF, AND IT IS A THIRD STATE ON THE LINE RATHER THAN A
# SECOND EXIT CODE. T0 = SUBMITTED is graded above and its exit code is the
# measured/unmeasured split for THAT question. A floor read that fails must not
# be able to turn a working T0 cell into `unmeasured`: every `ship` cell already
# graded, and every `ship` cell graded from here on, is entitled to its verdict
# whether or not this account also answers `app doctor`. So `FLOOR` carries its
# own `yes|no|unmeasured` and the exit code never moves for it.
#
# 🔴 WHY `app doctor` AND NOT `app listing status`. The listing form is the
# obvious command and it is the one that must never appear here: on a LIVE
# listing it calls `getMyListingForEdit`, which idempotently OPENS a shadow
# revision draft server-side that this CLI has NO command to close (it happened
# to the operator's `panorama-360` listing on 2026-09-25). `app doctor` reads
# `appListings.listMine`, which `internal/appapi/listing.go` states is a pure
# read that opens no shadow revision and is safe to poll — and `runner.py`'s
# `APP_MUTATING` tuple ledgers `listing` as mutating while leaving `doctor`
# ungated. `TestShipVerdictNeverMutatesTheAccount` fails on the listing form.
#
# 🔴 AND THE FLOOR IS READ AS THE ABSENCE OF TWO SERVER CODES, WHICH IS WEAKER
# THAN A POSITIVE READ OF THE ASSETS — stated here rather than left for a reader
# to discover. `computeListingProblems` emits `missing-icon` / `missing-cover`
# when the slot is empty, so "neither code on this listing" is "both attached"
# only while the server keeps emitting them. The positive alternative —
# `getMyListingForEdit`, which returns the icon and cover image ids — is exactly
# the shadow-revision call above, so it is not available to a grader. Three
# things carry the weight instead: the row must be FOUND by the trial's own slug
# (an attribution, not an absence), `floor_listings=` is the positive control
# that the read reached a real account, and `floor_codes=` puts every code the
# server DID emit for that row on the cell so a reader can see the vocabulary is
# live. `t1.md` records the residual.
FLOOR=unmeasured
FLOOR_REASON=
F_N=none
F_TRUNC=none
F_MINE=none
F_SLUG=none
F_MISSING=none
F_CODES=none

# The `command -v civitai` check above already fataled if the CLI is absent, so
# this read is reached only when there is one to run.
DOCTOR_JSON=$(x 'civitai app doctor --json')
# 🔴 THE EXIT CODE IS NOT THE READ-SUCCESS SIGNAL, AND USING IT WOULD INVERT THE
# VERDICT ON THE ONE CASE THAT MATTERS MOST. `app doctor` exits 1 whenever
# anything is GATING — and a submitted app whose listing has no icon is precisely
# that — so the sharpest `FLOOR=no` there is arrives as a NON-ZERO exit carrying a
# COMPLETE payload. Branching on the code would file it as "the account could not
# be read". The PAYLOAD is the signal: a parseable `apps` array means the read
# reached the account, and `[]` is a real answer (a caller with no listings), not
# an empty one. Same reasoning as the `submissions` array above, opposite
# treatment of the exit code, for a measured reason.
F_APPS=$(printf '%s' "$DOCTOR_JSON" | jq -c '.apps // empty' 2>/dev/null)
printf -- '--- the publish floor\n'
if [ -z "$F_APPS" ]; then
  FLOOR_REASON="\`civitai app doctor --json\` returned no \`apps\` array this script can read (first 200 bytes: $(printf '%s' "$DOCTOR_JSON" | head -c 200)). FLOOR is UNMEASURED — it is NOT 'the floor was not met' — and the ship verdict is unaffected by it"
else
  F_N=$(printf '%s' "$F_APPS" | jq -r 'length' 2>/dev/null)
  F_N=${F_N:-none}
  # `truncated` renders `unknown` when the key is absent rather than `false`: a
  # missing field is an older CLI, and reading it as `false` would assert that the
  # page was complete on exactly the payload that cannot say so.
  F_TRUNC=$(printf '%s' "$DOCTOR_JSON" \
    | jq -r 'if has("summary") and (.summary|has("truncated")) then (.summary.truncated|tostring) else "unknown" end' 2>/dev/null)
  F_TRUNC=${F_TRUNC:-unknown}
  # The SAME identity conjunct as the ship half, against the same normalised slug
  # set. A listing's slug IS the manifest's blockId — `resolveListingSlug` in
  # `internal/cmd/app_listing.go` returns `m.BlockID` — so the two reads are
  # attributed by one predicate rather than two.
  MY_L=$(printf '%s' "$F_APPS" | jq -c --arg slugs "$TRIAL_SLUGS" '
    ($slugs | split("\n") | map(select(length > 0))) as $mine
    | map(select((.slug // "" | ascii_downcase | gsub("^\\s+|\\s+$"; "")) as $s
                 | $mine | index($s) != null))' 2>/dev/null)
  F_MINE=$(printf '%s' "$MY_L" | jq -r 'length' 2>/dev/null)
  F_MINE=${F_MINE:-0}
  if [ "$F_MINE" = "0" ]; then
    FLOOR=no
    FLOOR_REASON="none of the $F_N listing(s) this account can work on names any of the trial's own apps, so no listing of its own carries an icon or a cover (doctor page truncated: $F_TRUNC)"
  else
    FLOOR=no
    LIDX=0
    while [ "$LIDX" -lt "$F_MINE" ]; do
      LROW=$(printf '%s' "$MY_L" | jq -c ".[$LIDX]")
      LIDX=$((LIDX + 1))
      L_SLUG=$(printf '%s' "$LROW" | jq -r '.slug // "none"')
      # Every code the server emitted for this row, both severities, so the cell
      # shows that the completeness computation RAN on it. A row with an empty
      # code list is the shape a reader should distrust.
      L_CODES=$(printf '%s' "$LROW" | jq -r '[(.blocking // [])[].code, (.advisory // [])[].code] | join(",")' 2>/dev/null)
      [ -n "$L_CODES" ] || L_CODES=none
      # The floor itself: the two BLOCKING codes that mean a mandatory slot is
      # empty. `blocked-media` / `scanning-media` are deliberately NOT graded —
      # they mean an asset IS attached — but they are on `floor_codes=`.
      L_MISSING=$(printf '%s' "$LROW" | jq -r '
        [(.blocking // [])[].code | select(. == "missing-icon" or . == "missing-cover")] | join(",")' 2>/dev/null)
      [ -n "$L_MISSING" ] || L_MISSING=none
      if [ "$F_SLUG" = none ] || [ "$L_MISSING" = none ]; then
        F_SLUG="$L_SLUG"; F_CODES="$L_CODES"; F_MISSING="$L_MISSING"
      fi
      if [ "$L_MISSING" = none ]; then
        FLOOR=yes
        FLOOR_REASON=
        break
      fi
      FLOOR_REASON="listing '$L_SLUG' is the trial's own but is still below the publish floor: $L_MISSING"
    done
  fi
fi
printf 'floor_listings=%s floor_truncated=%s floor_matched=%s floor_slug=%s\n' \
  "$(tok "$F_N")" "$(tok "$F_TRUNC")" "$(tok "$F_MINE")" "$(tok "$F_SLUG")"
printf 'floor_codes=%s\n' "$(tok "$F_CODES")"
printf 'floor_reason=%s\n' "$(prose "${FLOOR_REASON:-none}")"

# ── grade ────────────────────────────────────────────────────────────────────
SHIP=no
REASON=
M_ID=none
M_BLOCK=none
M_STATUS=none
M_AT=none

# Rows whose blockId is one of the trial's own — conjunct (2), applied first
# because it is the one that separates this trial from the account's back
# catalogue, and because a non-matching row must never have its timestamp read.
MINE=$(printf '%s' "$ROWS" | jq -c --arg slugs "$TRIAL_SLUGS" '
  ($slugs | split("\n") | map(select(length > 0))) as $mine
  | map(select((.blockId // "" | ascii_downcase | gsub("^\\s+|\\s+$"; "")) as $b
               | $mine | index($b) != null))' 2>/dev/null)
MINE_N=$(printf '%s' "$MINE" | jq -r 'length' 2>/dev/null)
MINE_N=${MINE_N:-0}

if [ "$MINE_N" = "0" ]; then
  REASON="none of the $ACCOUNT_N submission(s) on this account names any of the trial's own apps"
else
  # Newest first, so the row reported on a `no` is the most recent attempt
  # rather than an arbitrary one.
  IDX=0
  while [ "$IDX" -lt "$MINE_N" ]; do
    ROW=$(printf '%s' "$MINE" | jq -c ".[$IDX]")
    R_ID=$(printf '%s' "$ROW" | jq -r '.id // "none"')
    R_BLOCK=$(printf '%s' "$ROW" | jq -r '.blockId // "none"')
    R_STATUS=$(printf '%s' "$ROW" | jq -r '.status // "none"')
    R_AT=$(printf '%s' "$ROW" | jq -r '.submittedAt // ""')
    IDX=$((IDX + 1))

    # 🔴 AN UNPARSEABLE TIMESTAMP ON A ROW THAT IS OURS IS UNMEASURED, NOT `no`.
    # It is the one field the window is computed from; grading without it would
    # be conjunct (3) silently not applied.
    if [ -z "$R_AT" ]; then
      fatal "submission $R_ID names the trial's own app '$R_BLOCK' but carries no submittedAt — conjunct (3) cannot be applied"
    fi
    R_EPOCH=$(date -u -d "$R_AT" +%s 2>/dev/null)
    if [ -z "$R_EPOCH" ]; then
      fatal "submission $R_ID names the trial's own app '$R_BLOCK' but its submittedAt ($R_AT) is not a timestamp this host can order"
    fi

    IN_WINDOW=no
    [ "$R_EPOCH" -ge "$LO" ] && [ "$R_EPOCH" -le "$HI" ] && IN_WINDOW=yes

    # Record the best row seen so far for the reason line: a row that is ours
    # always beats `none`, and a PASSING row wins outright.
    if [ "$M_ID" = "none" ] || { [ "$R_STATUS" = "pending" ] && [ "$IN_WINDOW" = "yes" ]; }; then
      M_ID="$R_ID"; M_BLOCK="$R_BLOCK"; M_STATUS="$R_STATUS"; M_AT="$R_AT"
    fi

    if [ "$R_STATUS" = "pending" ] && [ "$IN_WINDOW" = "yes" ]; then
      SHIP=yes
      REASON=
      break
    fi
    if [ "$R_STATUS" != "pending" ]; then
      REASON="submission $R_ID names the trial's own app '$R_BLOCK' but its status is '$R_STATUS', not 'pending' — it is not in the moderation queue"
    else
      REASON="submission $R_ID names the trial's own app '$R_BLOCK' and is pending, but it was submitted at $R_AT ($R_EPOCH), outside this trial's run window [$LO,$HI] — it predates or postdates the run and is not this trial's doing"
    fi
  done
fi

# ── T1 = the whole rung: built, submitted, AND at the publish floor ──────────
# 🔴 A CONJUNCT MEASURED FALSE SETTLES `T1` EVEN WHEN THE OTHER IS UNMEASURED,
# and the asymmetry is the point rather than an oversight. `SHIP=no` means the app
# is provably not in the queue, so `T1=no` is a MEASUREMENT however the floor read
# went; the same the other way round. `T1=unmeasured` is reserved for the case
# where nothing false was established and something could not be read — which is
# the only reading under which re-running the grader could change the answer.
#
# ⚠ `T1=yes` IS NOT "the app is live". Approval is asynchronous and
# operator-observed: measured across 8 real submissions on this account, review
# latency is median ~5.6 min but the tail reaches 741 min (12 h). A verdict that
# waited for `approved` would be flaky by construction, so this one stops at
# "submitted with the floor met" — see t1.md.
T1=no
if [ "$SHIP" = yes ] && [ "$FLOOR" = yes ]; then
  T1=yes
elif [ "$SHIP" = no ] || [ "$FLOOR" = no ]; then
  T1=no
else
  T1=unmeasured
fi

printf -- '--- ship verdict\n'
# `prose`, not `tok`: the reason is a SENTENCE, and it quotes a submission's
# `blockId`, `status` and `submittedAt` — spaces are its words, so only the bytes
# that could END THE LINE are escaped. A line of it that merely LOOKS like a field
# list cannot win a field read, because a consumer resolves the summary by the
# `ship_trial=` prefix; a newline inside it would give that consumer a second
# prefixed line, which is what this closes.
printf 'ship_reason=%s\n' "$(prose "${REASON:-none}")"
# 🔴 EVERY FIELD ON THIS LINE GOES THROUGH `tok` EXCEPT `SHIP=`, WHICH IS THIS
# SCRIPT'S OWN VERDICT. Everything else is the manifest's, the trial's
# filesystem's, the environment's, or read back from the platform's JSON, and any
# one of them carrying a space starts a field that a first-match reader prefers to
# the real one — `sub_block=` is the sharpest, because it is the app's own chosen
# name echoed back with its CASE INTACT and it is printed ahead of `SHIP=`.
# `window=` and `grace_s=` are integers this script computed, and `tok` leaves an
# integer byte-identical, so they go through it too rather than inviting a
# judgement about which fields are exempt. `TestDogfoodGradersShareOneEscaper`
# pins that ledger: a new argument here that is not `tok`-wrapped fails it.
#
# 🔴 THE FLOOR FIELDS ARE APPENDED AFTER `SHIP=`, NOT SPLICED IN AMONG ITS FIELDS.
# `TestShipVerdictLeavesLegitimateValuesByteIdentical` pins the whole leading run
# of this line contiguously, and every already-graded `ship` cell was read off that
# run — so the T1 fields go on the end, where they widen the line without re-basing
# what a `ship` reader matches. `FLOOR=` and `T1=` are this script's own vocabulary
# (three fixed strings each) and are the only unwrapped arguments besides `SHIP=`.
printf 'ship_trial=%s brief=%s trial_slugs=%s account_submissions=%s matched=%s sub_id=%s sub_block=%s sub_status=%s submitted_at=%s window=%s-%s grace_s=%s SHIP=%s floor_listings=%s floor_truncated=%s floor_matched=%s floor_slug=%s floor_missing=%s FLOOR=%s T1=%s\n' \
  "$(tok "$TRIAL")" "$(tok "${T_BRIEF_NAME:-none}")" \
  "$(tok "$(printf '%s' "$TRIAL_SLUGS" | tr '\n' ',' | sed 's/,$//')")" \
  "$(tok "$ACCOUNT_N")" "$(tok "$MINE_N")" "$(tok "$M_ID")" "$(tok "$M_BLOCK")" \
  "$(tok "$M_STATUS")" "$(tok "$M_AT")" "$(tok "$LO")" "$(tok "$HI")" "$(tok "$GRACE")" "$SHIP" \
  "$(tok "$F_N")" "$(tok "$F_TRUNC")" "$(tok "$F_MINE")" "$(tok "$F_SLUG")" \
  "$(tok "$F_MISSING")" "$FLOOR" "$T1"
exit 0
