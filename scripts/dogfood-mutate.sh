#!/usr/bin/env bash
#
# dogfood-mutate.sh — the mutation battery for scripts/dogfood-sandbox.sh.
#
# 🔴 WHY THIS IS A COMMITTED SCRIPT AND NOT A TRANSCRIPT. Three review rounds
# reported mutation results the next round had to take on trust and could not
# re-derive; one of those published tables was later falsified (rows that
# asserted a REASON and never the VERDICT, so 8 deny->allow flips survived).
# `scripts/mutate.sh` covers Go packages only, so the shell gate — where the
# money decisions live — had no re-runnable evidence. This is that artifact:
# every mutant is CHECKSUM-GATED (the target must occur, the file hash must
# change, the mutant must still parse), so a pattern that stops matching is
# reported INVALID rather than silently scored as a survivor.
#
# 🔴 KNOWN LIMITATION: the gate cannot see a mutation whose REPLACEMENT text is
# malformed. Measured once — perl interpolated `${ROOT}` and `$(...)` in a
# replacement, so the mutant wrote to `/home/.config/...` instead of the run
# root, changed the file, parsed cleanly, and read as a SURVIVOR. Escape every
# `$` on the replacement side, and when a mutant survives, read the mutated
# region before believing it.
#
#   ./scripts/dogfood-mutate.sh          # full battery
#
# Expected on a clean tree: killed=62 survived=2 invalid=0, the two survivors
# being the NULL control and write_scalar's readback (declared redundant in
# claudedocs/dogfood-3-sandbox.md).
# Every mutation is a perl program in SINGLE quotes: the `$`-expressions inside
# are perl/sed syntax and must NOT be expanded by this shell. The directive must
# precede the first COMMAND to be file-wide.
# shellcheck disable=SC2016
set -u
HERE=$(cd "$(dirname "$(readlink -f "$0")")" && pwd)
S="${HERE}/dogfood-sandbox.sh"
B=$(mktemp -t dogfood-mutate-orig.XXXXXX)
cp "${S}" "${B}"
trap 'cp "${B}" "${S}"; rm -f "${B}"' EXIT INT TERM

echo "BASELINE: $(bash "${S}" selftest --offline 2>&1 | tail -1)"
echo

KILLED=0; SURVIVED=0; INVALID=0
run() {
  local name="$1"; shift
  local h0 h1 out fails
  h0=$(sha256sum "${S}" | cut -d' ' -f1)
  "$@" >/dev/null 2>&1
  h1=$(sha256sum "${S}" | cut -d' ' -f1)
  if [ "${h0}" = "${h1}" ]; then printf '  INVALID  %-52s (unchanged)\n' "${name}"; INVALID=$((INVALID+1)); cp "${B}" "${S}"; return; fi
  if ! bash -n "${S}" 2>/dev/null; then printf '  INVALID  %-52s (parse)\n' "${name}"; INVALID=$((INVALID+1)); cp "${B}" "${S}"; return; fi
  out=$(bash "${S}" selftest --offline 2>&1)
  fails=$(printf '%s' "${out}" | grep -c '^  FAIL')
  if [ "${fails}" = 0 ]; then printf '  SURVIVED %-52s\n' "${name}"; SURVIVED=$((SURVIVED+1))
  else printf '  KILLED   %-52s %-3s | %s\n' "${name}" "${fails}" \
    "$(printf '%s' "${out}" | grep -m1 '^  FAIL' | sed 's/^  FAIL  //; s/  */ /g' | cut -c1-46)"; KILLED=$((KILLED+1)); fi
  cp "${B}" "${S}"
}
P() { perl -0pi -e "$1" "${S}"; }

echo "--- 🔴 deny->allow with a BYTE-IDENTICAL reason (8 survived last round) ---"
flip() { run "deny->allow: $1" P "s/deny (\"[^\"]*$2)/allow \$1/"; }
flip "upgrade"                "would replace the binary under evaluation"
flip "login"                  "this sandbox is already authenticated"
flip "dev-tunnel/dev-token"   "is out of scope for this run"
flip "unknown subcommand"     "is not a subcommand of"
flip "args the CLI rejects"   "the CLI would reject these arguments"
flip "TOTAL_SPEND_CAP"        "the run's total spend cap"
flip "MAX_GENERATE_SUBMITS"   "the generation cap for this run"
flip "MAX_APP_SUBMITS"        "the app-submission cap"
flip "MAX_LISTING_MUTATIONS"  "the listing-mutation cap"
flip "remaining budget"       "--max-cost \\\$\{mc\} exceeds the"
flip "per-call ceiling"       "--max-cost \\\$\{mc\} exceeds this run's"
flip "no --max-cost"          "a generation that actually submits"
flip "--no-wait"              "--no-wait returns before the charge"
flip "--timeout"              "--timeout stops the CLI waiting"
flip "meter_broken"           "the spend meter stopped agreeing"
flip "calibration"            "the spend meter has not been calibrated"
flip "submit prefix"          "this run may only submit its own throwaway app"
flip "listing prefix"         "this run may not touch the account"
flip "submit unreadable id"   "cannot read a blockId"
flip "listing unidentifiable" "cannot identify which app"
flip "withdraw allowlist"     "so it may be one of the account"
flip "cancel allowlist"       "cancelling DOES NOT REFUND"
flip "classifier drift"       "the invocation could not be classified"
flip "flag parse error"       "the CLI would reject these flags"
flip "unknown command"        "the CLI does not recognise this command"

echo
echo "--- structural: this round's fixes ---"
run "rc-aware suppression removed entirely" \
  P 's/\tif \[ -n "\$\{after\}" \] \&\& \[ "\$\{before\}" = "\$\{after\}" \] \&\& \[ "\$\{rc\}" != 0 \]; then/\tif false; then/'
run "app counter bumps regardless of rc" \
  P 's/\t\t\tif \[ "\$\{rc\}" = 0 \]; then\n\t\t\t\tlocal a/\t\t\tif true; then\n\t\t\t\tlocal a/' 
run "provenance verification removed" \
  P 's/\tPROVENANCE_ERROR=""\n\tlocal want got/\tPROVENANCE_ERROR=""; return 0\n\tlocal want got/'
run "provenance compares nothing" \
  P 's/\t\tif \[ "\$\{got\}" != "\$\{want\}" \]; then/\t\tif false; then/'
run "assert_writable removed" \
  P 's/\t\tALLOW_SPEND\)      assert_writable generate_submits "\$\{scrubbed\}"/\t\tALLOW_SPEND)      :/'
run "reconcile_inflight removed" \
  P 's/\treconcile_inflight\n//'
run "inflight record never written" \
  P 's/\t\t\t> "\$\{ROOT\}\/ledger\/inflight"/\t\t\t> \/dev\/null/'
run "hassubcommands check removed" \
  P 's/\tif \[ "\$\{GHASSUB\}" = "true" \] \&\& \[ "\$\{GARGC\}" -gt 0 \]; then/\tif false; then/'
run "argserr ignored" \
  P 's/\tif \[ -n "\$\{GARGSERR\}" \]; then/\tif false; then/'
run "resolve_block_id ignores a failed lookup" \
  P 's/\t\t\*\) return 1 ;;\n\tesac\n\tprintf/\t\t*) : ;;\n\tesac\n\tprintf/'
run "GUARD_BIN reset deleted" \
  P 's/^GUARD_BIN=""$//m'
run "record_pubreq_ids is a no-op" \
  P 's/\tout=\$\(real_cli app status "\$\{slug\}" --json 2>\/dev\/null\) \|\| return 0/\treturn 0/'
run "workflow provenance ignored (shape filter)" \
  P 's/\t\tcomm -13 "\$\{ROOT\}\/ledger\/workflow.before" "\$\{after\}" \\\n\t\t\t>> "\$\{ROOT\}\/ledger\/workflow.allow" 2>\/dev\/null \|\| true/\t\tcat "${after}" >> "${ROOT}\/ledger\/workflow.allow" 2>\/dev\/null || true/'

echo
echo "--- structural: previous rounds, re-verified ---"
run "read_scalar EMPTY as zero" \
  P 's/(\tv=\$\(tr -d . \\n\\r\\t. < "\$\{f\}" 2>\/dev\/null\) \|\| return 1\n)/$1\t[ -n "\${v}" ] || { printf 0; return 0; }\n/' 
run "read_scalar MISSING as zero" \
  P 's/\t\[ -f "\$\{f\}" \] \|\| return 1/\t[ -f "${f}" ] || { printf 0; return 0; }/'
run "write_scalar readback removed" \
  P 's/\t\[ "\$\{back\}" = "\$\{v\}" \] \|\| return 1\n//'
run "ledger lock removed" \
  P 's/\tflock -x -w "\$\{LOCK_WAIT\}" 9 \|\| return 1\n//'
run "tsv_escape drops newlines" \
  P 's/\ts="\$\{s\/\/\$.\\n.\/\\\\n\}"\n//'
run "unanchored balance parser" \
  P "s/'\"total\":-\\\\\?\[0-9\]\[0-9\]\*\[,\}\]'/'\"total\":-\\\\?[0-9][0-9]*'/g"
run "zero-delta latch removed" \
  P 's/\tif \[ "\$\{delta\}" = 0 \]; then/\tif false; then/'
run "negative-delta latch removed" \
  P 's/\tif \[ "\$\{delta\}" -lt 0 \]; then/\tif false; then/'
run "failed-read latch removed" \
  P 's/\tif \[ -z "\$\{before\}" \] \|\| \[ -z "\$\{after\}" \]; then/\tif false; then/'
run "SLUG_PREFIX empty-check removed" \
  P 's/\tif \[ -z "\$\{SLUG_PREFIX:-\}" \]; then\n\t\tPOLICY_ERROR="policy key SLUG_PREFIX is missing or empty"; return 1\n\tfi\n//'
run "classifier failure fails OPEN" \
  P 's/\tif ! classify "\$@"; then/\tif false; then/'
run "help keyed on presence not value" \
  P 's/\tif \[ "\$\(gbool help\)" = "true" \]; then allow "help"; return 0; fi/\tif [ "$(gchanged help)" = "true" ]; then allow "help"; return 0; fi/'
run "free-preview keyed on presence not value" \
  P 's/if \[ "\$\(gbool print-input\)" = "true" \] \|\| \[ "\$\(gbool dry-run\)" = "true" \]; then/if [ "$(gchanged print-input)" = "true" ] || [ "$(gchanged dry-run)" = "true" ]; then/'

echo
echo "--- structural: round 5 ---"
run "rc suppression widened to any non-zero exit" \
  P 's/\tif \[ -n "\$\{after\}" \] \&\& \[ "\$\{before\}" = "\$\{after\}" \] \&\& \[ "\$\{rc\}" != 0 \]; then/\tif [ "${rc}" != 0 ]; then/'
run "rc suppression drops the readable-balance guard" \
  P 's/\tif \[ -n "\$\{after\}" \] \&\& \[ "\$\{before\}" = "\$\{after\}" \] \&\& \[ "\$\{rc\}" != 0 \]; then/\tif [ "${before}" = "${after}" ] \&\& [ "${rc}" != 0 ]; then/'
run "inflight liveness check removed" \
  P 's/\t\tif ! flock -x -n 7; then/\t\tif false; then/'
run "concurrent-generate refusal removed" \
  P 's/\t\t\t\tif \[ -n "\$\{inflight_pid\}" \] \&\& kill -0 "\$\{inflight_pid\}" 2>\/dev\/null; then/\t\t\t\tif false; then/'
run "inflight record omits the pid" \
  P 's/"\$\(now_iso\)" "\$\{mc\}" "\$\{before\}" "\$\$" "\$\{scrubbed\}"/"$(now_iso)" "${mc}" "${before}" "" "${scrubbed}"/'
run "meter file writability not asserted" \
  P 's/\t\t                  assert_writable observed_spend "\$\{scrubbed\}"\n//'
run "meter_write_failed does not latch" \
  P 's/meter_write_failed\(\) \{\n\tbreak_meter/meter_write_failed() {\n\treturn 0\n\tbreak_meter/'
run "reconcile does not share apply_settlement" \
  P 's/\tdelta=\$\(apply_settlement "\$\{before\}" "\$\{now\}" "\$\{mc\}" "reconcile"\)/\tdelta=0/'
run "credential written into HOME again" \
  P 's/\trm -f "\$\{ROOT\}\/home\/.config\/civitai\/config.yaml"/\tmkdir -p "\$\{ROOT\}\/home\/.config\/civitai" \&\& cp "\$\{ROOT\}\/ledger\/credential" "\$\{ROOT\}\/home\/.config\/civitai\/config.yaml"/'
run "app pull prefix gate removed" \
  P 's/\t\t\t\t\*\) deny "\\`\$\{pull_app\}\\` is outside/\t\t\t\t*) allow "${pull_app} is outside/'
run "audit-log appendability not asserted" \
  P 's/\t\t                  assert_appendable invocations.tsv "\$\{scrubbed\}"\n//'
run "calibrate bypasses the gate again" \
  P 's/\t"\$\{ROOT\}\/bin\/civitai" generate "a plain grey square, calibration" --yes/\treal_cli generate "a plain grey square, calibration" --yes/'

echo
echo "--- null control (MUST survive) ---"
run "NULL comment-only" \
  P 's/^# Every path is absolute.*$/# Every path is absolute; every cd gated; every var braced./m'

echo
echo "TOTALS: killed=${KILLED} survived=${SURVIVED} invalid=${INVALID}"
