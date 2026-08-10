#!/usr/bin/env bash
#
# dogfood-sandbox.sh — build and operate the sandbox for a CREDENTIALED blind
# dogfood run of the civitai CLI.
#
# Runs 1 and 2 (issues #223-#227, #255-#260) were deliberately un-credentialed,
# which made `civitai generate` — the only irreversibly money-spending surface —
# structurally unreachable. This harness is what makes a credentialed run
# survivable and auditable.
#
# Design doc, including WHAT THIS DOES NOT PROTECT AGAINST:
#   claudedocs/dogfood-3-sandbox.md
#
# Subcommands:
#   init         build the sandbox, seed the credential, lock it down, sample balance
#   guard        policy gate — invoked by the generated `civitai` shim, not by hand
#   status       spend so far, invocation counts, remaining budget
#   finish       sample the closing balance and print the audit report
#   selftest     assertions over the gate; --offline needs no binary or credential
#   enter        spawn a bubblewrap jail in which the repo does not exist
#   allow-pubreq record a publish-request id as withdrawable
#   teardown     unlock the read-only dirs and (optionally) delete the run root
#
# Every path is absolute; every `cd` is gated; every variable is braced.
#
# 🔴 READ THIS BEFORE CHANGING THE ARGV HANDLING. The gate used to classify on
# positional $1/$2/$3 and to look for flags with a plain "is this token
# present?" test. Both were wrong, and both were bypassed by ORDINARY
# invocations rather than by attacks:
#   * Cobra strips the root persistent bools (--color/--no-color/
#     --no-update-check) before resolving the command, so `--no-color generate
#     … --yes` put `--no-color` in $1 and every gate branch was skipped —
#     measured: a real charge with the meter untouched, and `--no-update-check
#     upgrade` allowed outright.
#   * pflag takes the token after a value-taking flag as its VALUE even when it
#     starts with `--`, so `--negative-prompt --help` made a "present?" test
#     see help and allow a real submit, and `--negative-prompt --dry-run` made
#     it see a free preview while the CLI submitted and charged.
# So: commands are resolved against a CLOSED VOCABULARY, and flags are resolved
# by a tokenizer that knows which flags consume the next token. Do not replace
# either with a `case "$1"` or a `has_flag` scan.

set -uo pipefail

# ---------------------------------------------------------------------------
# Defaults. Override on `init` via flags, or afterwards by editing
# ${ROOT}/ledger/policy.env (which `init` writes and every later call reads,
# VALIDATING it — a missing or malformed key is a refusal, not a default).
# ---------------------------------------------------------------------------
DEFAULT_ROOT="/tmp/claude-1000/dogfood3-scratch/run"
DEFAULT_PER_CALL_MAX_COST=100
DEFAULT_TOTAL_SPEND_CAP=2000
DEFAULT_MAX_GENERATE_SUBMITS=20
DEFAULT_MAX_APP_SUBMITS=3
# A listing change on a LIVE listing goes back to moderator review, so it costs
# human attention exactly as a submit does and is capped for the same reason.
DEFAULT_MAX_LISTING_MUTATIONS=10
# Seconds to wait before re-sampling when a submit reports a zero delta.
DEFAULT_METER_SETTLE_SECONDS=5
# Calibration prices a MINIMAL job, not the per-call ceiling: it exists to prove
# the meter observes a charge, not to spend the maximum allowed.
CALIBRATE_MAX_COST="${CALIBRATE_MAX_COST:-25}"
# Require `calibrate` (one deliberate, operator-watched generation) before the
# agent may spend, so the meter's core assumption is measured, not assumed.
DEFAULT_REQUIRE_CALIBRATION=1
# --no-wait returns before the charge settles; off unless calibration showed the
# balance moves by the time the command returns.
DEFAULT_ALLOW_NO_WAIT=0
DEFAULT_SLUG_PREFIX=""
DEFAULT_BASE_URL="https://civitai.com"

# 🔴 THE COMMAND AND FLAG VOCABULARIES THAT USED TO LIVE HERE ARE GONE ON
# PURPOSE. This file previously carried its own tables of top-level commands,
# app/listing/workflows subcommands, root persistent bools, per-command
# boolean flags and per-command value-taking flags — i.e. a second, hand-written
# model of pflag. That model disagreed with the real parser three times, each
# disagreement letting a real spend or mutation through. internal/dogfoodguard
# now answers every one of those questions from the actual cobra tree. Do not
# reintroduce a table here: if the gate needs to know something about an argv,
# teach the classifier to report it.

err()  { printf 'dogfood-sandbox: %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }
note() { printf '%s\n' "$*"; }

SANDBOX_TAG="SANDBOX POLICY"
now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# ---------------------------------------------------------------------------
# Balance parsing
#
# 🔴 The pattern is ANCHORED AT BOTH ENDS on purpose. An unanchored
# `"total":-\?[0-9]\+` matches a PREFIX of the number and returns it as the
# whole value: measured, `{"total":3.5}` parsed as 3 and `{"total":1e9}` parsed
# as 1 — and because both samples of a bracketed spend parse to 1, every delta
# is 0 and the meter reads a constant forever. Same class as the `"total": N`
# with-a-space bug that made an earlier ad-hoc grep return nothing at all.
# A negative or absurdly large total is REJECTED rather than truncated, and
# more than one `total` key is rejected too (Go decodes the last, this reads
# the first, and a disagreement is not something to guess about).
# ---------------------------------------------------------------------------
parse_buzz_total() {
	local json="$1" compact hits v
	compact=$(printf '%s' "${json}" | tr -d ' \n\t\r')
	hits=$(printf '%s' "${compact}" | grep -o '"total":-\?[0-9][0-9]*[,}]' | wc -l)
	[ "${hits}" = 1 ] || return 1
	v=$(printf '%s' "${compact}" | grep -o '"total":-\?[0-9][0-9]*[,}]' | head -1 \
		| sed 's/^"total"://; s/[,}]$//')
	case "${v}" in
		''|*[!0-9]*) return 1 ;;   # rejects "" and any leading '-'
	esac
	[ "${#v}" -le 15 ] || return 1  # beyond this, bash 64-bit arithmetic is not trustworthy
	printf '%s' "${v}"
}

# TSV fields must not carry the record or field separator. A newline is a
# LEGITIMATE character in a prompt, and an unescaped one let a single
# invocation append a second, forged ledger row.
tsv_escape() {
	local s="$1"
	s="${s//\\/\\\\}"
	s="${s//$'\n'/\\n}"
	s="${s//$'\r'/\\r}"
	s="${s//$'\t'/\\t}"
	printf '%s' "${s}"
}

scrub_argv() {
	local out="" a skip=0
	for a in "$@"; do
		if [ "${skip}" = 1 ]; then out="${out} <redacted>"; skip=0; continue; fi
		case "${a}" in
			--token=*)  a='--token=<redacted>' ;;
			--token)    skip=1 ;;
		esac
		out="${out} ${a}"
	done
	tsv_escape "${out# }"
}

# ---------------------------------------------------------------------------
# Policy + ledger scalars, both VALIDATED on read.
#
# 🔴 These used to be sourced and trusted. Under `set -u` an unbound variable
# inside the verdict subshell killed it, the verdict came back EMPTY, and
# `"" != "DENY"` executed the command — measured: with one line deleted from
# the file the header tells the operator to edit, `--max-cost 99999` submitted
# and charged. A malformed ledger scalar disabled the cap the same way.
# ---------------------------------------------------------------------------
POLICY_ERROR=""

load_policy() {
	POLICY_ERROR=""
	local f="${ROOT}/ledger/policy.env" k v
	if [ ! -f "${f}" ]; then
		POLICY_ERROR="no policy at ${f} — run \`init\` first"; return 1
	fi
	# 🔴 UNSET FIRST. Sourcing does not clear a key the file no longer defines,
	# so a value left over from an earlier load masked a deleted key and the
	# validation below passed on a policy that was missing one.
	unset PER_CALL_MAX_COST TOTAL_SPEND_CAP MAX_GENERATE_SUBMITS MAX_APP_SUBMITS \
	      MAX_LISTING_MUTATIONS METER_SETTLE_SECONDS REQUIRE_CALIBRATION ALLOW_NO_WAIT \
	      SLUG_PREFIX BASE_URL
	# shellcheck disable=SC1090
	. "${f}" || { POLICY_ERROR="policy file ${f} could not be read"; return 1; }
	for k in PER_CALL_MAX_COST TOTAL_SPEND_CAP MAX_GENERATE_SUBMITS MAX_APP_SUBMITS \
	         MAX_LISTING_MUTATIONS METER_SETTLE_SECONDS REQUIRE_CALIBRATION ALLOW_NO_WAIT; do
		eval "v=\${${k}:-}"
		case "${v}" in
			''|*[!0-9]*)
				POLICY_ERROR="policy key ${k} is missing or is not a whole number"; return 1 ;;
		esac
	done
	if [ -z "${SLUG_PREFIX:-}" ]; then
		POLICY_ERROR="policy key SLUG_PREFIX is missing or empty"; return 1
	fi
	[ -n "${BASE_URL:-}" ] || BASE_URL="${DEFAULT_BASE_URL}"
	return 0
}

# A ledger counter.
#
# 🔴 MISSING AND EMPTY ARE BOTH MALFORMED, NEVER ZERO. `init` creates every
# counter, so an absent or emptied file means the ledger has been truncated —
# and reading that as 0 silently restarts every cap. Measured on the previous
# version: with the counters emptied, a submit was allowed with the meter and
# the submit count both back at 0.
read_scalar() {
	local f="${ROOT}/ledger/$1" v
	[ -f "${f}" ] || return 1
	v=$(tr -d ' \n\r\t' < "${f}" 2>/dev/null) || return 1
	case "${v}" in
		''|*[!0-9]*) return 1 ;;
	esac
	printf '%s' "${v}"
}

# 🔴 A WRITE THAT FAILS MUST NOT LOOK LIKE A WRITE THAT WORKED. With the
# counter files unwritable the old version bumped nothing and allowed submit
# after submit. Every caller treats a non-zero return as fatal.
write_scalar() {
	local f="${ROOT}/ledger/$1" v="$2" back
	printf '%s\n' "${v}" > "${f}" 2>/dev/null || return 1
	back=$(tr -d ' \n\r\t' < "${f}" 2>/dev/null) || return 1
	[ "${back}" = "${v}" ] || return 1
	return 0
}

# ---------------------------------------------------------------------------
# Running the real binary
#
# The environment is PINNED, not inherited: the agent controls the shim's
# environment, and CIVITAI_BASE_URL would otherwise redirect the very balance
# read the spend meter uses as its oracle. CIVITAI_TOKEN is cleared so the
# seeded config file is the only credential.
# ---------------------------------------------------------------------------

# 🔴 THE CREDENTIAL IS INJECTED, NEVER LEFT ON DISK IN $HOME. `README.md` — one
# of the three things the blind agent is given — says `npm install -g
# @civitai/cli`, and node/npm are in the closure, so a SECOND, UNGATED CLI is
# one documented command away. While a `config.yaml` sat in the sandbox $HOME
# that second binary authenticated with it: measured, `npx -y @civitai/cli
# whoami` returned 401 against a deliberately fake seeded value, which is
# positive proof it read and sent the file. It would then have spent with no
# --max-cost, no cap, no counter, no bracketing, no ledger row and no latch —
# and no defeat-intent is required, it is the documented install command.
#
# So the token lives in ledger/credential (0600) and reaches only the child this
# gate execs. A second binary starts unauthenticated and every ungated path is
# inert. Residual, stated in the doc: the file is readable by the same UID, so
# an agent that goes looking can still find it — but that is intent, not the
# README.
sandbox_credential() {
	local f="${ROOT}/ledger/credential"
	[ -r "${f}" ] || return 0
	head -c 4096 "${f}" | tr -d '\r\n'
}

real_cli() {
	env -u CIVITAI_SUBMIT_PATH -u CIVITAI_ANTIPATTERN_SCAN_DIR \
		-u CIVITAI_CHECK_SUBMISSIONS_CAP -u CIVITAI_CHECK_PUBLISHED_PINS \
		"HOME=${ROOT}/home" \
		"XDG_CONFIG_HOME=${ROOT}/home/.config" \
		"XDG_CACHE_HOME=${ROOT}/home/.cache" \
		"CIVITAI_NO_UPDATE_CHECK=1" \
		"CIVITAI_BASE_URL=${BASE_URL:-${DEFAULT_BASE_URL}}" \
		"CIVITAI_TOKEN=$(sandbox_credential)" \
		"${ROOT}/real/civitai" "$@"
}

# Sample the balance, bounded. A hung `buzz` must not hang the gate.
sample_balance() {
	local phase="$1" json total
	if command -v timeout >/dev/null 2>&1; then
		json=$(timeout 60 "$0" __buzz --root "${ROOT}" 2>/dev/null)
	else
		json=$(real_cli buzz --json 2>/dev/null)
	fi
	total=$(parse_buzz_total "${json}") || return 1
	printf '%s\t%s\t%s\n' "$(now_iso)" "${phase}" "$(tsv_escape "${json}")" \
		>> "${ROOT}/ledger/buzz.tsv"
	printf '%s' "${total}"
}

# Internal: print the verdict for an argv WITHOUT executing anything. Used by
# the selftest to prove that a GUARD_BIN inherited from the environment cannot
# reach the gate, which needs a fresh process to be a real test.
cmd_verdict() {
	local root=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--)     shift; break ;;
			*)      break ;;
		esac
	done
	ROOT="${root:-${DEFAULT_ROOT}}"
	load_policy || { printf 'DENY'; return 0; }
	policy_verdict "$@"
	printf '%s' "${VERDICT:-DENY}"
}

# Internal: a `buzz --json` that `timeout` can kill as its own process.
cmd_buzz_probe() {
	local root=""
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root:-${DEFAULT_ROOT}}"
	load_policy || exit 1
	real_cli buzz --json
}

# ---------------------------------------------------------------------------
# Ledger lock. The cap check, the counter bump and the reservation must be one
# atomic section, or eight concurrent shim invocations each read the same
# pre-spend total and all pass a cap that should have allowed one — measured:
# 8 ALLOW_SPEND rows under a cap with 10 Buzz of headroom, the submit counter
# losing 3 increments, and the meter left holding a number that matched
# nothing.
# ---------------------------------------------------------------------------
LOCK_HELD=0
LOCK_WAIT="${LOCK_WAIT:-60}"
lock_ledger() {
	command -v flock >/dev/null 2>&1 || return 0   # degraded; reported by selftest
	exec 9>"${ROOT}/ledger/.lock" || return 1
	flock -x -w "${LOCK_WAIT}" 9 || return 1
	LOCK_HELD=1
	return 0
}

# Internal: exercise the lock PRIMITIVE directly.
#
# 🔴 THE END-TO-END RACE IS NOT A RELIABLE GUARD. Measured with `flock` deleted,
# the 8-concurrent row reported 1, 2 and 5 allowed submits on three consecutive
# runs — it passed a third of the time, because process startup (bash, policy
# load, a sha256 over a 21 MB binary) staggers the racers by more than the
# critical section takes. A guard that is right two times in three is worse than
# no guard, so the mutual exclusion is pinned here instead: hold the lock in one
# process and prove a second cannot take it.
cmd_locktest() {
	local root="" mode="" secs="2"
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			hold|try) mode="$1"; shift ;;
			*) secs="$1"; shift ;;
		esac
	done
	ROOT="${root:-${DEFAULT_ROOT}}"
	case "${mode}" in
		hold)
			lock_ledger || { printf 'no-lock'; return 1; }
			: > "${ROOT}/ledger/.locktest-held"
			sleep "${secs}"
			unlock_ledger ;;
		try)
			LOCK_WAIT=1
			if lock_ledger; then printf 'acquired'; unlock_ledger
			else printf 'blocked'; fi ;;
		*) printf 'usage'; return 2 ;;
	esac
}
unlock_ledger() {
	[ "${LOCK_HELD}" = 1 ] || return 0
	flock -u 9 2>/dev/null
	exec 9>&- 2>/dev/null
	LOCK_HELD=0
}

# ---------------------------------------------------------------------------
# init
# ---------------------------------------------------------------------------
cmd_init() {
	local binary="" token_file="" root="${DEFAULT_ROOT}" force=0
	local per_call="${DEFAULT_PER_CALL_MAX_COST}"
	local total_cap="${DEFAULT_TOTAL_SPEND_CAP}"
	local max_gen="${DEFAULT_MAX_GENERATE_SUBMITS}"
	local max_app="${DEFAULT_MAX_APP_SUBMITS}"
	local slug_prefix="${DEFAULT_SLUG_PREFIX}"
	local base_url="${DEFAULT_BASE_URL}"
	local max_listing="${DEFAULT_MAX_LISTING_MUTATIONS}"
	local settle="${DEFAULT_METER_SETTLE_SECONDS}"
	local require_cal="${DEFAULT_REQUIRE_CALIBRATION}"
	local allow_no_wait="${DEFAULT_ALLOW_NO_WAIT}"
	local guard=""

	while [ $# -gt 0 ]; do
		case "$1" in
			--root)                root="$2"; shift 2 ;;
			--binary)              binary="$2"; shift 2 ;;
			--guard)               guard="$2"; shift 2 ;;
			--token-file)          token_file="$2"; shift 2 ;;
			--per-call-max)        per_call="$2"; shift 2 ;;
			--total-cap)           total_cap="$2"; shift 2 ;;
			--max-generates)       max_gen="$2"; shift 2 ;;
			--max-app-submits)     max_app="$2"; shift 2 ;;
			--max-listing-changes) max_listing="$2"; shift 2 ;;
			--settle-seconds)      settle="$2"; shift 2 ;;
			--skip-calibration)    require_cal=0; shift ;;
			--allow-no-wait)       allow_no_wait=1; shift ;;
			--slug-prefix)         slug_prefix="$2"; shift 2 ;;
			--base-url)            base_url="$2"; shift 2 ;;
			--force)               force=1; shift ;;
			*) die "init: unknown flag $1" ;;
		esac
	done

	ROOT="${root}"
	[ -n "${binary}" ] || die "init: --binary <path to built civitai> is required"
	[ -x "${binary}" ] || die "init: ${binary} is not an executable file"

	if [ -e "${ROOT}" ] && [ "${force}" != 1 ]; then
		die "init: ${ROOT} already exists (use --force to supersede it, or \`teardown\` first)"
	fi
	local token=""
	if [ -n "${token_file}" ]; then
		[ -r "${token_file}" ] || die "init: cannot read --token-file ${token_file}"
		token=$(head -c 4096 "${token_file}" | tr -d '\r\n')
	elif [ -n "${CIVITAI_DOGFOOD_TOKEN:-}" ]; then
		token="${CIVITAI_DOGFOOD_TOKEN}"
	else
		die "init: supply the credential via --token-file <path> or the CIVITAI_DOGFOOD_TOKEN environment variable"
	fi
	[ -n "${token}" ] || die "init: the supplied credential is empty"

	if [ -e "${ROOT}" ]; then
		# 🔴 SUPERSEDE, NEVER DELETE — AND ONLY AFTER THE CREDENTIAL IS IN
		# HAND. The ledger is the previous run's audit artefact. This block used
		# to run BEFORE the token was read, so `init --force` with an
		# unreadable or empty token moved the sandbox aside and then died,
		# leaving no run root and a superseded one still holding both
		# credentials — measured.
		local superseded
		superseded="${ROOT}.superseded-$(date -u +%Y%m%dT%H%M%SZ)"
		chmod -R u+w "${ROOT}" 2>/dev/null
		mv "${ROOT}" "${superseded}" || die "init: could not set the previous run aside"
		note "init: previous run preserved at ${superseded}"
	fi

	[ -n "${slug_prefix}" ] || slug_prefix="dogfood3-$(date -u +%Y%m%d)-"

	mkdir -p "${ROOT}/real" "${ROOT}/bin" "${ROOT}/harness" \
	         "${ROOT}/home/.config" "${ROOT}/home/.cache" \
	         "${ROOT}/workspace" "${ROOT}/ledger" || die "init: mkdir failed"
	chmod 0700 "${ROOT}" "${ROOT}/home" "${ROOT}/ledger"

	cp "${binary}" "${ROOT}/real/civitai" || die "init: could not copy the binary"

	# 🔴 THE CLASSIFIER MUST COME FROM THE SAME TREE AS THE BINARY UNDER TEST.
	# It embeds the command tree, so a classifier built from a different commit
	# can disagree with the binary about what an argv means — which is the exact
	# class of defect it exists to remove. Build both from one checkout.
	if [ -z "${guard}" ]; then
		local repo
		repo=$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd) || die "init: cannot locate the repo"
		command -v go >/dev/null 2>&1 \
			|| die "init: go is not on PATH; build internal/dogfoodguard yourself and pass --guard <path>"
		guard="${ROOT}/real/dogfoodguard"
		( cd "${repo}" && go build -o "${guard}" ./internal/dogfoodguard ) \
			|| die "init: could not build the classifier from ${repo}"
	else
		[ -x "${guard}" ] || die "init: --guard ${guard} is not executable"
		cp "${guard}" "${ROOT}/real/dogfoodguard" || die "init: could not copy the classifier"
	fi
	[ -x "${ROOT}/real/dogfoodguard" ] || die "init: no classifier at ${ROOT}/real/dogfoodguard"

	local self
	self=$(readlink -f "$0")
	cp "${self}" "${ROOT}/harness/dogfood-sandbox.sh" || die "init: could not copy the harness"
	chmod 0555 "${ROOT}/harness/dogfood-sandbox.sh"

	cat > "${ROOT}/bin/civitai" <<SHIM
#!/usr/bin/env bash
exec "${ROOT}/harness/dogfood-sandbox.sh" guard --root "${ROOT}" -- "\$@"
SHIM
	chmod 0555 "${ROOT}/bin/civitai"

	# 🔴 npm's LAUNCHER IS A SYMLINK AND `readlink -f` COLLAPSES IT. Resolving
	# `npm` to its store target puts `lib/node_modules/npm/bin/npm` on PATH, and
	# node then looks for `npm-prefix.js` beside the NODE binary — measured
	# inside the jail: `Cannot find module
	# '…/nodejs-slim/bin/node_modules/npm/bin/npm-prefix.js'`, `npm install`
	# exited 1, and the agent still could not produce a lockfile. So node/npm in
	# the closure was necessary but NOT sufficient. These shims invoke npm the
	# way its own launcher does. Written here because ${ROOT}/bin is locked 0500
	# at the end of init.
	local npm_bin npm_dir
	npm_bin=$(command -v npm 2>/dev/null || true)
	if [ -n "${npm_bin}" ]; then
		npm_dir=$(dirname "$(readlink -f "${npm_bin}")")
		if [ -f "${npm_dir}/npm-cli.js" ]; then
			printf '#!/usr/bin/env bash\nexec node %q "$@"\n' "${npm_dir}/npm-cli.js" > "${ROOT}/bin/npm"
			chmod 0555 "${ROOT}/bin/npm"
		fi
		if [ -f "${npm_dir}/npx-cli.js" ]; then
			printf '#!/usr/bin/env bash\nexec node %q "$@"\n' "${npm_dir}/npx-cli.js" > "${ROOT}/bin/npx"
			chmod 0555 "${ROOT}/bin/npx"
		fi
	fi

	: > "${ROOT}/ledger/invocations.tsv"
	: > "${ROOT}/ledger/buzz.tsv"
	: > "${ROOT}/ledger/spend.tsv"
	: > "${ROOT}/ledger/pubreq.allow"
	: > "${ROOT}/ledger/workflow.allow"
	write_scalar generate_submits 0   || die "init: ledger not writable"
	write_scalar app_submits 0        || die "init: ledger not writable"
	write_scalar listing_mutations 0  || die "init: ledger not writable"
	write_scalar observed_spend 0     || die "init: ledger not writable"
	write_scalar reserved 0           || die "init: ledger not writable"
	rm -f "${ROOT}/ledger/meter_broken" "${ROOT}/ledger/calibration.ok"

	cat > "${ROOT}/ledger/policy.env" <<POLICY
# Written by dogfood-sandbox.sh init on $(now_iso). Read and VALIDATED by every
# later call — a missing or malformed key is a refusal, not a default.
ROOT="${ROOT}"
PER_CALL_MAX_COST=${per_call}
TOTAL_SPEND_CAP=${total_cap}
MAX_GENERATE_SUBMITS=${max_gen}
MAX_APP_SUBMITS=${max_app}
MAX_LISTING_MUTATIONS=${max_listing}
METER_SETTLE_SECONDS=${settle}
REQUIRE_CALIBRATION=${require_cal}
ALLOW_NO_WAIT=${allow_no_wait}
SLUG_PREFIX="${slug_prefix}"
BASE_URL="${base_url}"
BINARY_SOURCE="${binary}"
BINARY_SHA256="$(sha256sum "${binary}" | cut -d' ' -f1)"
GUARD_SHA256="$(sha256sum "${ROOT}/real/dogfoodguard" | cut -d' ' -f1)"
HARNESS_SHA256="$(sha256sum "${self}" | cut -d' ' -f1)"
POLICY
	chmod 0600 "${ROOT}/ledger/policy.env"
	load_policy || die "init: the policy just written does not validate: ${POLICY_ERROR}"

	(umask 077; printf '%s' "${token}" > "${ROOT}/ledger/credential") \
		|| die "init: could not store the credential"
	chmod 0600 "${ROOT}/ledger/credential"
	token=""
	# Nothing may write a config into the sandbox HOME — see sandbox_credential.
	rm -f "${ROOT}/home/.config/civitai/config.yaml"

	real_cli whoami >/dev/null 2>&1 || die "init: whoami failed with the seeded credential"

	local start
	start=$(sample_balance "start") || die "init: could not read the opening Buzz balance — refusing to start a run whose meter does not work"
	write_scalar start_balance "${start}"

	chmod 0555 "${ROOT}/real/civitai" "${ROOT}/real/dogfoodguard"
	chmod 0500 "${ROOT}/real" "${ROOT}/bin" "${ROOT}/harness"

	cmd_write_brief

	note "sandbox ready at ${ROOT}"
	note "  PATH entry ............ ${ROOT}/bin        (the agent's \`civitai\`)"
	note "  agent working dir ..... ${ROOT}/workspace"
	note "  opening balance ....... ${start} Buzz"
	note "  per-call estimate cap . ${per_call} Buzz (--max-cost required at or below this)"
	note "  total spend cap ....... ${total_cap} Buzz"
	note "  generate submits ...... ${max_gen} max"
	note "  app submits ........... ${max_app} max"
	note "  sanctioned blockId .... ${slug_prefix}*"
	note ""
	note "Brief for the blind agent: ${ROOT}/AGENT-BRIEF.md"
}

cmd_write_brief() {
	load_policy || die "brief: ${POLICY_ERROR}"
	cat > "${ROOT}/AGENT-BRIEF.md" <<BRIEF
# civitai CLI — evaluation sandbox

You have a \`civitai\` binary on your PATH, its \`README.md\`, and \`--help\`.
Work only inside \`${ROOT}/workspace\`.

🔴 **The \`civitai\` on your PATH is the only one in scope.** The README suggests
\`npm install -g @civitai/cli\`; do NOT install or run a second copy. It would not
be the build under evaluation, it is not covered by the safety gate, and any
finding from it tells the authors nothing about this binary.

This is a real, credentialed account. \`civitai generate\` **spends real money**.
A sandbox policy gate sits in front of the CLI and refuses some invocations; a
refusal from the gate is prefixed \`${SANDBOX_TAG}\` and is **not** a CLI bug —
do not file it as one. Everything else you see IS the CLI.

The gate:

- \`civitai generate\` that actually submits must carry \`--max-cost N\` with
  N at most **${PER_CALL_MAX_COST}**. \`--dry-run\` and \`--print-input\` are
  unrestricted; they spend nothing. (This requirement is the sandbox's, not the
  CLI's — do not report it as a CLI bug.)
- The run may spend at most **${TOTAL_SPEND_CAP} Buzz** in total, across at
  most **${MAX_GENERATE_SUBMITS}** generations.
- \`app submit\` and \`app listing set-*\`/\`add-screenshot\`/\`rm-screenshot\`/
  \`reorder\` only work on an app whose \`blockId\` starts with
  \`${SLUG_PREFIX}\`, and at most **${MAX_APP_SUBMITS}** submissions are
  allowed. Scaffold your app with a name under that prefix, e.g.
  \`civitai app create ${SLUG_PREFIX}myapp\`.
- \`upgrade\`, \`login\`, \`app dev-tunnel\` and \`app dev-token\` are refused.
- A command the gate does not recognise is refused rather than guessed at, so a
  typo may produce a \`${SANDBOX_TAG}\` line rather than the CLI's own error.

Submitting an app puts it in front of a real human moderator. That is expected
and approved for this run — but do it deliberately, not to see what happens.

Report what you find as you would to the tool's authors: what worked, what was
confusing, what was wrong, and what you could not do.
BRIEF
}

# ---------------------------------------------------------------------------
# CLASSIFICATION — delegated to the REAL Cobra command tree.
#
# 🔴 THE SHIM NO LONGER PARSES ARGV. It used to keep its own model of how the
# CLI reads a command line, and that model disagreed with pflag three times,
# each disagreement letting a real spend or a real mutation through:
#   * `--no-color generate … --yes`      (root persistent flags precede the command)
#   * `--negative-prompt --help`         (a flag VALUE that looks like a flag)
#   * `--print-input=false`              (present, but FALSE)
# Each was fixed as a spelling; the class survived. internal/dogfoodguard now
# answers "what is this invocation?" using cobra's own Find + ParseFlags, so
# the gate and the binary cannot disagree by construction. It never executes
# anything.
#
# 🔴 FAIL CLOSED: a non-zero exit, empty output, a missing `ok`, or an
# unrecognised field is a DENY.
# ---------------------------------------------------------------------------
declare -A GBOOL GINT GSTR GCHANGED
declare -a GARG
GPATH=""; GARGC=0; GERR=""; GKIND=""
GHASSUB=""; GARGSERR=""

# 🔴 ASSIGNED UNCONDITIONALLY AT STARTUP, WHICH IS THE POINT. The selftest
# swaps in fake classifiers to prove the shim refuses drifted output, and it
# cannot do that by overwriting the real binary — a running executable gives
# ETXTBSY and the write SILENTLY fails, which made four "drift is refused" rows
# pass against the real classifier instead. So the path is injectable. Setting
# it here to the empty string discards any value inherited from the
# environment, so an agent exporting GUARD_BIN cannot point the gate at a
# classifier of its own.
GUARD_BIN=""

# escape() writes \\ first, then \n/\r/\t; undoing it must therefore take the
# single-character escapes FIRST and the backslash LAST, or a literal "\\n" in
# a prompt round-trips to a newline. Every current consumer fails closed on the
# difference, but the asymmetry was real.
unescape() {
	local s="$1"
	s="${s//\\t/$'\t'}"
	s="${s//\\r/$'\r'}"
	s="${s//\\n/$'\n'}"
	s="${s//\\\\/\\}"
	printf '%s' "${s}"
}

classify() {
	GBOOL=(); GINT=(); GSTR=(); GCHANGED=(); GARG=()
	GPATH=""; GARGC=0; GERR=""; GKIND=""
	GHASSUB=""; GARGSERR=""
	local bin="${GUARD_BIN:-${ROOT}/real/dogfoodguard}" out rc ok="" k v
	if [ ! -x "${bin}" ]; then
		GERR="the classifier ${bin} is missing or not executable"; return 1
	fi
	# Bounded like the balance read: a classifier that hangs must not hang the
	# gate. It does no I/O beyond writing its verdict, so 30s is generous.
	if command -v timeout >/dev/null 2>&1; then
		out=$(timeout 30 "${bin}" -- "$@" 2>/dev/null); rc=$?
	else
		out=$("${bin}" -- "$@" 2>/dev/null); rc=$?
	fi
	if [ -z "${out}" ]; then
		GERR="the classifier produced no output (exit ${rc})"; return 1
	fi
	while IFS=$'\t' read -r k v; do
		[ -n "${k}" ] || continue
		case "${k}" in
			ok)          ok="${v}" ;;
			err)         GERR=$(unescape "${v}") ;;
			kind)        GKIND="${v}" ;;
			path)        GPATH=$(unescape "${v}") ;;
			argc)        GARGC="${v}" ;;
			arg.*)       GARG[${k#arg.}]=$(unescape "${v}") ;;
			bool.*)      GBOOL[${k#bool.}]="${v}" ;;
			int.*)       GINT[${k#int.}]="${v}" ;;
			str.*)       GSTR[${k#str.}]=$(unescape "${v}") ;;
			raw.*)       GSTR[${k#raw.}]=$(unescape "${v}") ;;
			changed.*)   GCHANGED[${k#changed.}]="${v}" ;;
			# `runnable` is informational: it does NOT discriminate an unknown
			# subcommand (every group parent is runnable — it prints its help),
			# so the gate uses hassubcommands + argc. Accepted and ignored.
			runnable)    : ;;
			hassubcommands) GHASSUB="${v}" ;;
			argserr)     GARGSERR=$(unescape "${v}") ;;
			# `blockid` is only emitted in the classifier's manifest mode, which
			# resolve_block_id parses directly; accept and ignore it here so a
			# stray one is not mistaken for classifier drift.
			blockid)     : ;;
			*)           GERR="the classifier emitted an unrecognised field \`${k}\`"; return 1 ;;
		esac
	done <<< "${out}"
	case "${GARGC}" in
		''|*[!0-9]*) GERR="the classifier reported a non-numeric argument count"; return 1 ;;
	esac
	[ "${ok}" = "1" ] || return 1
	return 0
}

gbool()    { printf '%s' "${GBOOL[$1]:-false}"; }
gstr()     { printf '%s' "${GSTR[$1]:-}"; }
gchanged() { printf '%s' "${GCHANGED[$1]:-false}"; }

# Which app would the CLI act on? Answered by encoding/json in the classifier.
#
# 🔴 THIS USED TO BE A GREP FOR THE FIRST `"blockId"`, AND Go TAKES THE LAST.
# Measured on a manifest carrying the key twice — `sanctioned-ok` first,
# `someones-real-app` second — the gate resolved the sanctioned one and returned
# ALLOW_APP_SUBMIT while the CLI would have submitted the foreign app. Same
# gate-vs-binary disagreement class the classifier exists to remove.
resolve_block_id() {
	local dir="$1" slug="$2" bin out
	if [ -n "${slug}" ]; then printf '%s' "${slug}"; return 0; fi
	bin="${GUARD_BIN:-${ROOT}/real/dogfoodguard}"
	[ -x "${bin}" ] || return 1
	out=$("${bin}" blockid "${dir}" 2>/dev/null) || return 1
	case "${out}" in
		*"ok"$'\t'"1"*) ;;
		*) return 1 ;;
	esac
	printf '%s' "$(printf '%s' "${out}" | grep -m1 '^blockid' | cut -f2)"
}

# ---------------------------------------------------------------------------
# policy_verdict — sets VERDICT / VERDICT_REASON / VERDICT_MC.
#
# It deliberately does NOT run in a subshell: an internal failure under `set -u`
# must kill the whole gate (so nothing runs) rather than return an empty string
# the caller reads as "not DENY".
# ---------------------------------------------------------------------------
VERDICT=""; VERDICT_REASON=""; VERDICT_MC=0; VERDICT_SLUG=""

deny()  { VERDICT="DENY"; VERDICT_REASON="$1"; return 0; }
allow() { VERDICT="ALLOW"; VERDICT_REASON="${1:-}"; return 0; }

policy_verdict() {
	VERDICT=""; VERDICT_REASON=""; VERDICT_MC=0; VERDICT_SLUG=""

	if ! classify "$@"; then
		case "${GKIND}" in
			unknown-command)
				deny "the CLI does not recognise this command (${GERR})" ;;
			flag-parse-error)
				deny "the CLI would reject these flags (${GERR}); the gate refuses rather than guess what was meant" ;;
			*)
				deny "the invocation could not be classified (${GERR:-no detail}) — refusing" ;;
		esac
		return 0
	fi

	# Help is free on every command, including the forbidden ones. This is the
	# REAL value of the help flag, so `--help=false` is not a help request.
	if [ "$(gbool help)" = "true" ]; then allow "help"; return 0; fi

	local path="${GPATH}"

	# 🔴 UNKNOWN SUBCOMMANDS, ASKED OF COBRA RATHER THAN OF A TABLE. The old
	# `is_group_path` listed the parents by hand and missed `models`, `images`,
	# `users` and `tags` — measured, the binary exits 2 for an unknown
	# subcommand under every one of them while the gate said ALLOW. A command
	# that HAS subcommands and was handed a positional was handed a subcommand
	# that does not exist, whatever its parent. (`Runnable()` does NOT
	# discriminate: all of those parents are runnable, they print their help.)
	if [ -n "${GARGSERR}" ]; then
		deny "the CLI would reject these arguments (${GARGSERR})"
		return 0
	fi
	if [ "${GHASSUB}" = "true" ] && [ "${GARGC}" -gt 0 ]; then
		deny "\`${GARG[0]:-?}\` is not a subcommand of \`${path:-civitai}\`; the gate fails closed rather than guess whether it spends or mutates"
		return 0
	fi
	if [ "${GHASSUB}" = "true" ]; then allow "usage"; return 0; fi

	case "${path}" in
		upgrade)
			deny "\`upgrade\` would replace the binary under evaluation mid-run"; return 0 ;;
		login)
			deny "this sandbox is already authenticated; run \`civitai whoami\` to see as whom"; return 0 ;;
		"app dev-tunnel"|"app dev-token")
			deny "\`${path}\` is out of scope for this run"; return 0 ;;

		"app withdraw")
			local id
			id=$(gstr id)
			[ -n "${id}" ] || id="${GARG[0]:-}"
			if [ -z "${id}" ]; then
				allow "withdraw with no id (a usage error the CLI reports)"; return 0
			fi
			if grep -qxF "${id}" "${ROOT}/ledger/pubreq.allow" 2>/dev/null; then
				allow "withdraw of a submission this sandbox created"; return 0
			fi
			deny "\`${id}\` was not created by this sandbox, so it may be one of the account's real pending submissions"
			return 0 ;;

		"app submit")
			if [ "$(gbool package-only)" = "true" ]; then
				allow "--package-only never submits"; return 0
			fi
			local dir id n
			dir="${GARG[0]:-}"
			[ -n "${dir}" ] || dir="."
			if ! id=$(resolve_block_id "${dir}" ""); then
				deny "cannot read a blockId from ${dir}/block.manifest.json, so the app being submitted cannot be identified"
				return 0
			fi
			case "${id}" in
				"${SLUG_PREFIX}"*) ;;
				*) deny "blockId \`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may only submit its own throwaway app"; return 0 ;;
			esac
			n=$(read_scalar app_submits) || { deny "the app-submission counter is unreadable or malformed"; return 0; }
			if [ "${n}" -ge "${MAX_APP_SUBMITS}" ]; then
				deny "the app-submission cap for this run (${MAX_APP_SUBMITS}) is already used"; return 0
			fi
			VERDICT="ALLOW_APP_SUBMIT"; VERDICT_REASON="submitting ${id}"; VERDICT_SLUG="${id}"; return 0 ;;

		"app pull")
			# 🔴 `app pull` REACHES THE ACCOUNT'S REAL APPS AND PROVISIONS A
			# THIRD CREDENTIAL. It hits an owner-only endpoint, lazily creates a
			# Forgejo identity, and git writes an access token into
			# `.git/config` — and `git` is in the closure. SLUG_PREFIX exists to
			# stop this run touching real apps; it covered submit and listing
			# and not this.
			local pull_app
			pull_app=$(gstr app)
			if [ -z "${pull_app}" ]; then
				allow "pull with no --app (a usage error the CLI reports)"; return 0
			fi
			case "${pull_app}" in
				"${SLUG_PREFIX}"*) allow "pull of this run's own app"; return 0 ;;
				*) deny "\`${pull_app}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — \`app pull\` reaches a real app and writes a git credential into .git/config"
				   return 0 ;;
			esac ;;

		"app listing status")
			allow "listing read"; return 0 ;;

		"app listing set-icon"|"app listing set-cover"|"app listing add-screenshot"|"app listing rm-screenshot"|"app listing reorder")
			local slug dir id m
			slug=$(gstr slug); dir=$(gstr dir)
			[ -n "${dir}" ] || dir="."
			if ! id=$(resolve_block_id "${dir}" "${slug}"); then
				deny "cannot identify which app \`${path}\` would mutate (no --slug, and no readable blockId in ${dir}/block.manifest.json)"
				return 0
			fi
			case "${id}" in
				"${SLUG_PREFIX}"*) ;;
				*) deny "\`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may not touch the account's real listings"; return 0 ;;
			esac
			# A listing change on a LIVE listing goes back to moderator review,
			# so it carries the same human cost as a submit and is capped too.
			m=$(read_scalar listing_mutations) || { deny "the listing-mutation counter is unreadable or malformed"; return 0; }
			if [ "${m}" -ge "${MAX_LISTING_MUTATIONS}" ]; then
				deny "the listing-mutation cap for this run (${MAX_LISTING_MUTATIONS}) is already used"; return 0
			fi
			VERDICT="ALLOW_LISTING"; VERDICT_REASON="listing mutation on ${id}"; return 0 ;;

		"workflows cancel")
			local wid
			wid="${GARG[0]:-}"
			if [ -z "${wid}" ]; then
				allow "cancel with no id (a usage error the CLI reports)"; return 0
			fi
			if grep -qxF "${wid}" "${ROOT}/ledger/workflow.allow" 2>/dev/null; then
				allow "cancel of a workflow this sandbox created"; return 0
			fi
			deny "workflow \`${wid}\` was not created by this sandbox — cancelling DOES NOT REFUND, so the gate will not cancel a job it did not start"
			return 0 ;;

		generate)
			if [ "$(gbool print-input)" = "true" ] || [ "$(gbool dry-run)" = "true" ]; then
				allow "generate preview (spends nothing)"; return 0
			fi
			if [ -f "${ROOT}/ledger/meter_broken" ]; then
				deny "the spend meter stopped agreeing with the account earlier in this run, so spend is no longer being counted — refusing to submit (see ledger/meter_broken)"
				return 0
			fi
			# 🔴 ONE SPEND AT A TIME. Two concurrent generates cannot both be
			# metered: the balance delta of either is the sum of both, and the
			# crash record is per-run. The cap arithmetic already reserves, but
			# refusing outright is the honest bound — and the threat model says
			# the agent "runs things in parallel because that seems efficient".
			local inflight_pid
			if [ -f "${ROOT}/ledger/inflight" ]; then
				inflight_pid=$(cut -f4 < "${ROOT}/ledger/inflight" 2>/dev/null)
				if [ -n "${inflight_pid}" ] && kill -0 "${inflight_pid}" 2>/dev/null; then
					deny "another generation is still running in this sandbox (pid ${inflight_pid}); two concurrent spends cannot both be metered, so this one is refused — wait for it to finish"
					return 0
				fi
			fi
			if [ "${REQUIRE_CALIBRATION}" = "1" ] && [ ! -f "${ROOT}/ledger/calibration.ok" ]; then
				deny "the spend meter has not been calibrated against a real generation; run \`dogfood-sandbox.sh calibrate\` (it spends once, deliberately, while you watch) or re-init with --skip-calibration"
				return 0
			fi
			if [ "$(gbool no-wait)" = "true" ] && [ "${ALLOW_NO_WAIT}" != "1" ]; then
				deny "--no-wait returns before the charge has settled, and the meter reads the balance when the command returns — refused so the ledger cannot silently under-count"
				return 0
			fi
			# 🔴 `--timeout` IS AN UNBLOCKED `--no-wait`. The CLI's own help says
			# it "stops the CLI waiting; it does NOT stop the generation and does
			# NOT stop the charge". A short timeout therefore returns before the
			# charge settles, which is the same silent under-count `--no-wait` is
			# refused for — measured, `--timeout 1s` was ALLOW_SPEND.
			if [ "$(gchanged timeout)" = "true" ] && [ "${ALLOW_NO_WAIT}" != "1" ]; then
				deny "--timeout stops the CLI waiting but not the charge, so a short timeout returns before the balance settles — refused for the same reason as --no-wait"
				return 0
			fi
			local cum res committed remaining mc n
			cum=$(read_scalar observed_spend) || { deny "the spend ledger is unreadable or malformed — refusing to submit"; return 0; }
			res=$(read_scalar reserved)       || { deny "the reservation ledger is unreadable or malformed — refusing to submit"; return 0; }
			committed=$(( cum + res ))
			if [ "${committed}" -ge "${TOTAL_SPEND_CAP}" ]; then
				deny "the run's total spend cap (${TOTAL_SPEND_CAP} Buzz) is reached — ${cum} observed, ${res} committed in flight"
				return 0
			fi
			n=$(read_scalar generate_submits) || { deny "the submission counter is unreadable or malformed"; return 0; }
			if [ "${n}" -ge "${MAX_GENERATE_SUBMITS}" ]; then
				deny "the generation cap for this run (${MAX_GENERATE_SUBMITS} submits) is already used"; return 0
			fi
			if [ "$(gchanged max-cost)" != "true" ]; then
				deny "a generation that actually submits must carry --max-cost (at most ${PER_CALL_MAX_COST}); use --dry-run to price it first"
				return 0
			fi
			mc="${GINT[max-cost]:-}"
			case "${mc}" in
				''|*[!0-9]*) deny "--max-cost must be a non-negative whole number of Buzz, got \`${mc}\`"; return 0 ;;
			esac
			if [ "${mc}" -gt "${PER_CALL_MAX_COST}" ]; then
				deny "--max-cost ${mc} exceeds this run's per-invocation ceiling of ${PER_CALL_MAX_COST} Buzz"; return 0
			fi
			remaining=$(( TOTAL_SPEND_CAP - committed ))
			if [ "${mc}" -gt "${remaining}" ]; then
				deny "--max-cost ${mc} exceeds the ${remaining} Buzz left under this run's total cap"; return 0
			fi
			VERDICT="ALLOW_SPEND"; VERDICT_REASON="submit with --max-cost ${mc}"; VERDICT_MC="${mc}"
			return 0 ;;
	esac

	allow "read-only or local"
	return 0
}

# ---------------------------------------------------------------------------
# guard
# ---------------------------------------------------------------------------
refuse() {
	printf '%s: %s\n' "${SANDBOX_TAG}" "$1" >&2
	log_invocation "DENY" "126" "0" "$2" "$1"
	unlock_ledger
	exit 126
}

cmd_guard() {
	local root=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--)     shift; break ;;
			*)      break ;;
		esac
	done
	ROOT="${root:-${DEFAULT_ROOT}}"

	local argv=("$@")
	local scrubbed
	scrubbed=$(scrub_argv "${argv[@]+"${argv[@]}"}")

	if ! load_policy; then
		printf '%s: %s\n' "${SANDBOX_TAG}" "the sandbox policy is unusable (${POLICY_ERROR}) — refusing to run anything" >&2
		printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$(now_iso)" "DENY" "126" "0" "${scrubbed}" \
			"policy invalid: $(tsv_escape "${POLICY_ERROR}")" >> "${ROOT}/ledger/invocations.tsv" 2>/dev/null
		exit 126
	fi

	# 🔴 VERIFY THE PROVENANCE HASHES, DO NOT MERELY RECORD THEM. `init` writes
	# BINARY_SHA256 and GUARD_SHA256 into policy.env and nothing ever read them
	# back. Measured: replacing real/dogfoodguard with a two-line "everything is
	# a read" script made the gate run an UNMETERED `generate --yes` and log it
	# as read-only. The classifier is the gate's only source of truth about what
	# an argv means, so an unverified classifier is no gate at all.
	verify_provenance || {
		printf '%s: %s\n' "${SANDBOX_TAG}" "${PROVENANCE_ERROR}" >&2
		printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$(now_iso)" "DENY" "126" "0" "${scrubbed}" \
			"provenance: $(tsv_escape "${PROVENANCE_ERROR}")" >> "${ROOT}/ledger/invocations.tsv" 2>/dev/null
		exit 126
	}

	lock_ledger || refuse "could not take the ledger lock within 60s" "${scrubbed}"

	reconcile_inflight

	policy_verdict "${argv[@]+"${argv[@]}"}"

	# 🔴 An empty or unrecognised verdict is a DENY. This is the backstop for
	# any internal failure that leaves VERDICT unset — measured as load-bearing:
	# with a branch returning without setting VERDICT, deleting this case lets
	# the real binary run.
	case "${VERDICT}" in
		ALLOW|ALLOW_SPEND|ALLOW_APP_SUBMIT|ALLOW_LISTING|DENY) ;;
		*) refuse "the gate could not classify this invocation (internal error) — refusing" "${scrubbed}" ;;
	esac

	if [ "${VERDICT}" = "DENY" ]; then
		refuse "${VERDICT_REASON}" "${scrubbed}"
	fi

	# 🔴 EVERY counter bump is fatal if the write fails. With the ledger
	# unwritable the previous version bumped nothing and allowed submit after
	# submit, meter and counter both reading 0 forever.
	local before="" mc="${VERDICT_MC}"
	# 🔴 COUNTERS FOR THE MODERATOR-FACING ACTIONS ARE BUMPED AFTER THE CALL, ON
	# SUCCESS. Bumping before exec and ignoring rc meant three LOCAL validation
	# failures — the CLI refusing to package a project — burned all three app
	# submissions and the fourth was refused. Measured: rc=1,1,1 with the counter
	# at 1,2,3, then rc=126. Only the SPEND reservation stays pre-exec, because
	# that is money at risk rather than a count of things that happened.
	# 🔴 PROVE THE COUNTER IS WRITABLE BEFORE THE CALL, EVEN THOUGH IT IS BUMPED
	# AFTER IT. Moving the bump to after-success (so a local validation failure
	# no longer burns a submission) would otherwise mean an unwritable ledger
	# stops blocking: the spend happens, the write fails, the count is lost and
	# the cap is evadable. This is a no-op rewrite of the current value purely
	# to establish writability while refusing still costs nothing.
	case "${VERDICT}" in
		# 🔴 THE METER FILE ITSELF, NOT JUST THE COUNTER. Asserting only
		# `generate_submits` proved the wrong file: with just `observed_spend`
		# at 0444, 111 Buzz moved, the meter read 0 and nothing latched, while
		# every invocation row recorded `delta=37` — the spend was observed and
		# never accumulated. `observed_spend` and `reserved` are what the cap is
		# computed from, so they are what must be writable before a spend.
		ALLOW_SPEND)      assert_writable generate_submits "${scrubbed}"
		                  assert_writable observed_spend "${scrubbed}"
		                  assert_writable reserved "${scrubbed}"
		                  # The audit trail is the deliverable: with
		                  # invocations.tsv at 0444 a generate charged 37 and
		                  # wrote ZERO rows. Disk-full is the realistic trigger,
		                  # and ${ROOT} also holds the npm cache.
		                  assert_appendable invocations.tsv "${scrubbed}"
		                  assert_appendable spend.tsv "${scrubbed}" ;;
		ALLOW_APP_SUBMIT) assert_writable app_submits "${scrubbed}" ;;
		ALLOW_LISTING)    assert_writable listing_mutations "${scrubbed}" ;;
	esac

	if [ "${VERDICT}" = "ALLOW_SPEND" ]; then
		before=$(sample_balance "pre-generate") || \
			refuse "could not read the Buzz balance, so the spend meter cannot be kept — refusing to submit" "${scrubbed}"
		local res
		res=$(read_scalar reserved) || refuse "the reservation ledger is unreadable or malformed" "${scrubbed}"
		write_scalar reserved "$(( res + mc ))" || refuse "could not record the spend reservation — refusing to submit" "${scrubbed}"
		# A crash between here and the reconciliation must not lose the charge.
		printf '%s\t%s\t%s\t%s\t%s\n' "$(now_iso)" "${mc}" "${before}" "$$" "${scrubbed}" \
			> "${ROOT}/ledger/inflight"
		# Held for the lifetime of THIS process, so a later invocation can tell
		# a running spend from a crashed one without consulting a pid.
		if command -v flock >/dev/null 2>&1; then
			exec 8>"${ROOT}/ledger/inflight.lock" && flock -x -n 8
		fi
		workflow_snapshot
	fi

	unlock_ledger

	local t0 t1 rc
	t0=$(date +%s%N)
	env -u CIVITAI_SUBMIT_PATH -u CIVITAI_ANTIPATTERN_SCAN_DIR \
		-u CIVITAI_CHECK_SUBMISSIONS_CAP -u CIVITAI_CHECK_PUBLISHED_PINS \
		"HOME=${ROOT}/home" \
		"XDG_CONFIG_HOME=${ROOT}/home/.config" \
		"XDG_CACHE_HOME=${ROOT}/home/.cache" \
		"CIVITAI_NO_UPDATE_CHECK=1" \
		"CIVITAI_BASE_URL=${BASE_URL}" \
		"CIVITAI_TOKEN=$(sandbox_credential)" \
		"${ROOT}/real/civitai" "${argv[@]+"${argv[@]}"}"
	rc=$?
	t1=$(date +%s%N)

	local delta=""
	# 🔴 The ledger append happens INSIDE the lock, along with the settlement
	# accounting: it used to run after unlock_ledger, so concurrent invocations
	# appended unserialised.
	lock_ledger || err "could not re-take the ledger lock; the ledger may be inconsistent"

	if [ "${VERDICT}" = "ALLOW_SPEND" ]; then
		delta=$(settle_spend "${before}" "${mc}" "${rc}")
		rm -f "${ROOT}/ledger/inflight"
		# A submission counts when it happened: rc 0, or any observed charge.
		local n
		if [ "${rc}" = 0 ] || { [ "${delta}" != "0" ] && [ "${delta}" != "" ]; }; then
			n=$(read_scalar generate_submits) && write_scalar generate_submits "$(( n + 1 ))"
		fi
	fi

	# Record the ids this run created, so the agent can withdraw its OWN
	# submission and cancel its OWN workflow without the operator doing it by
	# hand. Reads only; costs one extra API call after a successful mutation.
	case "${VERDICT}" in
		ALLOW_APP_SUBMIT)
			if [ "${rc}" = 0 ]; then
				local a
				a=$(read_scalar app_submits) && write_scalar app_submits "$(( a + 1 ))"
				record_pubreq_ids "${VERDICT_SLUG}"
			fi ;;
		ALLOW_LISTING)
			if [ "${rc}" = 0 ]; then
				local m
				m=$(read_scalar listing_mutations) && write_scalar listing_mutations "$(( m + 1 ))"
			fi ;;
		ALLOW_SPEND) record_workflow_ids ;;
	esac

	log_invocation "${VERDICT}" "${rc}" "$(( (t1 - t0) / 1000000 ))" "${scrubbed}" \
		"${VERDICT_REASON}${delta:+ | delta=${delta}}"
	unlock_ledger
	return "${rc}"
}

# Reconcile the balance after a submit. Echoes the delta (or `unknown`).
#
# 🔴 THREE WAYS THIS CAN LIE, AND ALL THREE NOW LATCH `meter_broken`:
#   * the read FAILS            — charge the worst case the estimate allowed;
#   * the read SUCCEEDS but is STALE — a job that certainly cost something
#     reports a delta of 0. Measured against a stub settling 2s later (the
#     shape of --no-wait): 3 submits, meter 0, balance really moved 111, and
#     nothing latched. A zero delta after a submit is now re-sampled once after
#     a settle delay and, if still zero, treated as a broken meter;
#   * the delta is NEGATIVE — a Buzz credit landing in the same window HIDES a
#     real charge. Measured: a +100 grant alongside a 37 charge recorded 0.
#     Clamping to zero is what made that silent, so it no longer clamps.
settle_spend() {
	local before="$1" mc="$2" rc="${3:-0}" after res delta
	res=$(read_scalar reserved) || res="${mc}"

	after=$(sample_balance "post-generate") || after=""

	if [ -n "${after}" ] && [ "${after}" = "${before}" ] && [ "${METER_SETTLE_SECONDS}" -gt 0 ]; then
		sleep "${METER_SETTLE_SECONDS}"
		after=$(sample_balance "post-generate-resample") || after=""
	fi

	# 🔴 THE rc-AWARE SUPPRESSION APPLIES TO EXACTLY ONE CASE: THE READ WORKED
	# AND THE DELTA REALLY IS 0. Suppressing on rc alone was wrong in both
	# directions and both were measured. A zero delta on a non-zero exit IS
	# expected — the CLI's own `--max-cost` refusal, a bad `--input`, an
	# `--ecosystem` typo — and latching on it killed the run at the agent's
	# first likely mistake. But a generate can charge and THEN fail (AGENTS item
	# 28(b); #279), and with rc≠0 a failed read or a credit-masked charge left
	# 37 Buzz unmetered and unlatched. So this is the ONLY case rc suppresses;
	# everything else goes through apply_settlement, which both this path and
	# the crash path share.
	if [ -n "${after}" ] && [ "${before}" = "${after}" ] && [ "${rc}" != 0 ]; then
		printf '%s\tgenerate\t%s\t%s\t0 (rc=%s, no charge)\n' "$(now_iso)" "${before}" "${after}" "${rc}" \
			>> "${ROOT}/ledger/spend.tsv"
		delta=0
	else
		delta=$(apply_settlement "${before}" "${after}" "${mc}" "generate")
	fi

	write_scalar reserved "$(( res - mc < 0 ? 0 : res - mc ))" || true
	printf '%s' "${delta}"
}

# 🔴 THE METER WRITE IS THE ONE WRITE THAT CANNOT BE REFUSED — it happens after
# the money has moved. Ignoring its failure with `|| true` meant that with only
# `observed_spend` at 0444, 111 Buzz moved, the meter read 0 and nothing
# latched, while every invocation row recorded `delta=37`: the spend was
# OBSERVED and never accumulated. It cannot be undone, so it latches.
meter_write_failed() {
	break_meter "the spend ledger could not be written after a charge; the observed figure is now incomplete"
	return 0
}

# Read a counter and write the same value back, so an unreadable or unwritable
# ledger is a refusal BEFORE anything is spent rather than a lost count after.
assert_writable() {
	local name="$1" scrubbed="$2" v
	v=$(read_scalar "${name}") || refuse "the ${name} counter is unreadable or malformed" "${scrubbed}"
	write_scalar "${name}" "${v}" || refuse "the ${name} counter is not writable, so this run could not be counted — refusing" "${scrubbed}"
}

# Prove an append-only ledger file can actually be appended to.
assert_appendable() {
	local name="$1" scrubbed="$2"
	printf '' >> "${ROOT}/ledger/${name}" 2>/dev/null \
		|| refuse "the ${name} audit log is not writable, so this run could not be recorded — refusing" "${scrubbed}"
}

PROVENANCE_ERROR=""
verify_provenance() {
	PROVENANCE_ERROR=""
	local want got
	for pair in "civitai:${BINARY_SHA256:-}" "dogfoodguard:${GUARD_SHA256:-}"; do
		local name="${pair%%:*}" want="${pair#*:}"
		if [ -z "${want}" ]; then
			PROVENANCE_ERROR="policy.env records no hash for ${name} — refusing to run against an unverifiable binary"
			return 1
		fi
		got=$(sha256sum "${ROOT}/real/${name}" 2>/dev/null | cut -d' ' -f1)
		if [ -z "${got}" ]; then
			PROVENANCE_ERROR="${ROOT}/real/${name} is missing — refusing"
			return 1
		fi
		if [ "${got}" != "${want}" ]; then
			PROVENANCE_ERROR="${name} does not match the hash recorded at init (recorded ${want:0:12}…, found ${got:0:12}…) — refusing to run a substituted binary"
			return 1
		fi
	done
	return 0
}

# 🔴 A SPEND KILLED MID-FLIGHT MUST NOT VANISH FROM THE LEDGER. SIGKILL cannot
# be trapped, so the guard writes an `inflight` record BEFORE the call and
# clears it after; the next invocation reconciles whatever it finds. Measured
# before this existed: SIGKILL during a submit moved 37 Buzz, left
# observed_spend at 0, wrote zero ledger rows, and leaked the reservation.
reconcile_inflight() {
	local f="${ROOT}/ledger/inflight"
	[ -f "${f}" ] || return 0

	# 🔴 LIVENESS BY LOCK, NOT BY PID. The pid check was namespace-unsafe:
	# `enter` runs `--unshare-pid`, so the first command in the jail has $$ = 1
	# or 2, and a stale record carrying either was NEVER reconciled — measured,
	# pid 1 and pid 2 both left the record in place, leaked the reservation, and
	# then made every later generate refuse with "another generation is still
	# running". One interrupted generate bricked the spend path for the whole
	# run, and `--die-with-parent` makes that routine. The writer instead holds
	# an exclusive flock for the duration of its child; if we can take it, the
	# writer is gone.
	if command -v flock >/dev/null 2>&1; then
		exec 7>"${ROOT}/ledger/inflight.lock" || return 0
		if ! flock -x -n 7; then
			exec 7>&- 2>/dev/null
			return 0
		fi
		flock -u 7 2>/dev/null; exec 7>&- 2>/dev/null
	fi

	local ts mc before _pid
	IFS=$'\t' read -r ts mc before _pid _ < "${f}"
	rm -f "${f}"
	case "${mc}" in ''|*[!0-9]*) mc=0 ;; esac
	case "${before}" in ''|*[!0-9]*) before="" ;; esac

	local now delta
	now=$(sample_balance "reconcile") || now=""

	# 🔴 ONE RULE, ONE PLACE. This used to carry its own arithmetic and
	# regenerated all three defects settle_spend had just had removed:
	# measured, a reconcile with delta 0 recorded 0 and did not latch, a
	# NEGATIVE delta was clamped to 0 (with settle_spend's own comment saying
	# clamping "is what made that silent" twelve lines away), and an unwritable
	# meter was ignored — after which the run simply continued. Ctrl-C on a slow
	# generate is the likeliest trigger and lands on the delta-0 row. Both paths
	# now call apply_settlement.
	delta=$(apply_settlement "${before}" "${now}" "${mc}" "reconcile")

	write_scalar reserved "$(( $(read_scalar reserved || printf 0) - mc < 0 ? 0 : $(read_scalar reserved || printf 0) - mc ))" || true
	printf '%s\tRECONCILED\t0\t0\t%s\t%s\n' "$(now_iso)" \
		"$(tsv_escape "an earlier generation did not finish cleanly (started ${ts})")" \
		"$(tsv_escape "delta=${delta}")" >> "${ROOT}/ledger/invocations.tsv"
	printf '%s: %s\n' "${SANDBOX_TAG}" \
		"an earlier generation was interrupted; ${delta} reconciled into the ledger" >&2
}

# The settlement arithmetic, shared by the live path and the crash path.
#
# before/after are balances ("" when unreadable), mc is the worst case the
# estimate allowed, phase names the caller for the spend log. Echoes the delta
# it recorded. Latches meter_broken on every shape it cannot trust: an
# unreadable balance, a negative delta, or a zero delta the caller says should
# have moved money.
apply_settlement() {
	local before="$1" after="$2" mc="$3" phase="$4" cum delta
	cum=$(read_scalar observed_spend) || cum=0

	if [ -z "${before}" ] || [ -z "${after}" ]; then
		write_scalar observed_spend "$(( cum + mc ))" || meter_write_failed
		printf '%s\t%s\t%s\tUNREADABLE\tassumed-%s\n' "$(now_iso)" "${phase}" "${before:-UNKNOWN}" "${mc}" \
			>> "${ROOT}/ledger/spend.tsv"
		break_meter "a balance read failed during ${phase}; ${mc} Buzz charged to the ledger as a worst case"
		printf '%s' "unknown"
		return 0
	fi

	delta=$(( before - after ))
	if [ "${delta}" -lt 0 ]; then
		write_scalar observed_spend "$(( cum + mc ))" || meter_write_failed
		printf '%s\t%s\t%s\t%s\tNEGATIVE(%s)-assumed-%s\n' "$(now_iso)" "${phase}" "${before}" "${after}" "${delta}" "${mc}" \
			>> "${ROOT}/ledger/spend.tsv"
		break_meter "the balance went UP across ${phase} (delta ${delta}); a credit can hide a real charge, so ${mc} Buzz was charged as a worst case"
		printf '%s' "negative:${delta}"
		return 0
	fi

	if [ "${delta}" = 0 ]; then
		write_scalar observed_spend "$(( cum + mc ))" || meter_write_failed
		printf '%s\t%s\t%s\t%s\tZERO-assumed-%s\n' "$(now_iso)" "${phase}" "${before}" "${after}" "${mc}" \
			>> "${ROOT}/ledger/spend.tsv"
		break_meter "${phase} saw the balance move by 0 for a generation that was submitted, so the meter is not seeing charges; ${mc} Buzz charged as a worst case"
		printf '%s' "zero"
		return 0
	fi

	write_scalar observed_spend "$(( cum + delta ))" || meter_write_failed
	printf '%s\t%s\t%s\t%s\t%s\n' "$(now_iso)" "${phase}" "${before}" "${after}" "${delta}" \
		>> "${ROOT}/ledger/spend.tsv"
	printf '%s' "${delta}"
}

break_meter() {
	printf '%s\n' "$1" > "${ROOT}/ledger/meter_broken"
	printf '%s: %s — no further generation will be allowed this run\n' "${SANDBOX_TAG}" "$1" >&2
}

# After a SUCCESSFUL app submit, record the publish-request ids for the app this
# run submitted, so the agent can withdraw its own before a moderator sees it.
#
# 🔴 THIS GREPPED THE HUMAN TABLE, WHICH HAS NO ID COLUMN. `app status` prints
# BLOCK_ID/VERSION/STATUS/DEPLOY/SUBMITTED/URL (app_status.go:128) — measured
# with a positive control, 0 `pubreq_` matches in the human output and 2 in
# `--json`. So pubreq.allow was never populated and `app withdraw` was ALWAYS
# denied: the documented cleanup path did not exist. Scoped to the submitted
# blockId, so it records only what this run created.
record_pubreq_ids() {
	local slug="${1:-}" out
	[ -n "${slug}" ] || return 0
	out=$(real_cli app status "${slug}" --json 2>/dev/null) || return 0
	printf '%s' "${out}" | grep -o 'pubreq_[A-Za-z0-9_-]*' | sort -u \
		>> "${ROOT}/ledger/pubreq.allow" 2>/dev/null || true
	sort -u -o "${ROOT}/ledger/pubreq.allow" "${ROOT}/ledger/pubreq.allow" 2>/dev/null || true
}

# Workflow ids are recorded by PROVENANCE, not by shape.
#
# 🔴 THE OLD VERSION FILTERED THE ACCOUNT-WIDE `workflows list` BY UUID SHAPE.
# Measured, that captured a workflow this run did not create and missed a
# ULID-shaped one — and `workflows cancel` DOES NOT REFUND, so allowlisting
# someone else's job is exactly the wrong direction. This snapshots the ids
# that existed BEFORE the submit and records only what appeared after.
workflow_snapshot() {
	real_cli workflows list --json 2>/dev/null \
		| grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' | sed 's/.*"\([^"]*\)"$/\1/' \
		| sort -u > "${ROOT}/ledger/workflow.before" 2>/dev/null || : > "${ROOT}/ledger/workflow.before"
}

record_workflow_ids() {
	local after="${ROOT}/ledger/workflow.after"
	real_cli workflows list --json 2>/dev/null \
		| grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' | sed 's/.*"\([^"]*\)"$/\1/' \
		| sort -u > "${after}" 2>/dev/null || return 0
	if [ -f "${ROOT}/ledger/workflow.before" ]; then
		comm -13 "${ROOT}/ledger/workflow.before" "${after}" \
			>> "${ROOT}/ledger/workflow.allow" 2>/dev/null || true
	fi
	sort -u -o "${ROOT}/ledger/workflow.allow" "${ROOT}/ledger/workflow.allow" 2>/dev/null || true
	rm -f "${after}" "${ROOT}/ledger/workflow.before"
}

log_invocation() {
	printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$(now_iso)" "$1" "$2" "$3" "$(tsv_escape "$4")" "$(tsv_escape "${5:-}")" \
		>> "${ROOT}/ledger/invocations.tsv"
}

# ---------------------------------------------------------------------------
# allow-pubreq / status / finish / enter / teardown
# ---------------------------------------------------------------------------
cmd_allow_pubreq() {
	local root="${DEFAULT_ROOT}" id=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			*) id="$1"; shift ;;
		esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"
	[ -n "${id}" ] || die "allow-pubreq: pass the publish-request id"
	printf '%s\n' "${id}" >> "${ROOT}/ledger/pubreq.allow"
	note "recorded ${id} as withdrawable"
}

# ---------------------------------------------------------------------------
# calibrate — the ONE deliberate, operator-watched generation that measures the
# meter's core assumption: does `buzz` reflect the charge by the time `generate`
# returns?
#
# 🔴 THIS SPENDS REAL BUZZ, ONCE, ON PURPOSE. Everything else in this harness
# assumes the balance moves before the command returns, and that assumption was
# never tested — measured against a stub settling 2 s later, three submits left
# the meter reading 0 while the balance really moved, with nothing latched. A
# harness whose meter is assumed rather than measured is not auditable, so the
# gate refuses to spend until this has been run (or --skip-calibration was
# recorded at init).
# ---------------------------------------------------------------------------
cmd_calibrate() {
	local root="${DEFAULT_ROOT}" yes=0
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--yes)  yes=1; shift ;;
			*) die "calibrate: unknown flag $1" ;;
		esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"

	if [ "${yes}" != 1 ]; then
		note "calibrate will submit ONE real generation and SPEND REAL BUZZ."
		note "It exists so the spend meter is measured rather than assumed."
		note "Re-run with --yes to proceed."
		return 2
	fi

	local before after delta
	before=$(sample_balance "calibrate-before") || die "calibrate: could not read the opening balance"
	note "balance before: ${before}"

	# 🔴 CALIBRATE GOES THROUGH THE GATE. It used to call real_cli directly, so
	# the one spend the go/no-go MAKES MANDATORY bypassed the lock, the caps,
	# the meter, the counter and the ledger entirely — measured: 37 Buzz moved,
	# observed_spend 0, submits 0, ledger rows 0, and `finish` then reported
	# "observed 37 / MEASURED TOTAL SPEND 74". The doc lists a disagreement
	# between those two figures as an ABORT CONDITION, so the prescribed
	# procedure guaranteed the alarm fired every run — a permanently-red gate
	# that trains the operator to explain away the only signal that catches an
	# escaped charge.
	#
	# The gate refuses to spend until calibration.ok exists, so it is written
	# provisionally and removed again if the run does not prove the meter.
	printf 'provisional %s\n' "$(now_iso)" > "${ROOT}/ledger/calibration.ok"
	note "submitting one generation through the gate (--max-cost ${CALIBRATE_MAX_COST}) …"
	"${ROOT}/bin/civitai" generate "a plain grey square, calibration" --yes \
		--max-cost "${CALIBRATE_MAX_COST}" --no-download >/dev/null 2>&1
	local rc=$?

	after=$(sample_balance "calibrate-after") || die "calibrate: could not read the closing balance"
	delta=$(( before - after ))
	note "balance after:  ${after}   delta=${delta}   (generate rc=${rc})"

	if [ "${rc}" != 0 ]; then
		rm -f "${ROOT}/ledger/calibration.ok"
		die "calibrate: the generation itself failed (rc=${rc}); fix that before calibrating"
	fi
	if [ "${delta}" -le 0 ]; then
		rm -f "${ROOT}/ledger/calibration.ok"
		printf '%s\n' "calibration FAILED: the balance did not move by the time generate returned (delta ${delta})" \
			> "${ROOT}/ledger/meter_broken"
		die "calibrate: the balance did NOT move by the time the command returned (delta ${delta}). The meter cannot count spend on this account/endpoint — do not run the dogfood until that is understood."
	fi

	printf 'calibrated %s delta=%s before=%s after=%s\n' "$(now_iso)" "${delta}" "${before}" "${after}" \
		> "${ROOT}/ledger/calibration.ok"
	note ""
	note "CALIBRATED — one generation moved the balance by ${delta} Buzz before the"
	note "command returned, so the meter observes charges synchronously."
	note "Recorded in ${ROOT}/ledger/calibration.ok"
}

cmd_status() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"
	local start cum res
	start=$(read_scalar start_balance) || start="?"
	cum=$(read_scalar observed_spend)  || cum="MALFORMED"
	res=$(read_scalar reserved)        || res="MALFORMED"
	note "run root .............. ${ROOT}"
	note "opening balance ....... ${start} Buzz"
	note "observed spend ........ ${cum} / ${TOTAL_SPEND_CAP} Buzz"
	note "committed in flight ... ${res} Buzz"
	note "generate submits ...... $(read_scalar generate_submits) / ${MAX_GENERATE_SUBMITS}"
	note "app submits ........... $(read_scalar app_submits) / ${MAX_APP_SUBMITS}"
	note "invocations logged .... $(wc -l < "${ROOT}/ledger/invocations.tsv" 2>/dev/null || printf 0)"
	note "refusals .............. $(grep -c "$(printf '\tDENY\t')" "${ROOT}/ledger/invocations.tsv" 2>/dev/null || printf 0)"
	if [ -f "${ROOT}/ledger/meter_broken" ]; then
		note ""
		note "🔴 THE SPEND METER IS BROKEN — a balance read failed mid-run. The"
		note "   observed figure above includes a worst-case assumption, not a"
		note "   measurement. Reconcile against the account before trusting it."
	fi
}

cmd_finish() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"
	# An outstanding crash record would otherwise leave the two figures
	# disagreeing for a reason the report never explains.
	lock_ledger && { reconcile_inflight; unlock_ledger; }
	local start end
	start=$(read_scalar start_balance) || die "finish: no usable opening balance recorded"
	if ! end=$(sample_balance "end"); then
		err "finish: could not read the closing balance — the figure below is the per-call ledger only"
		end=""
	fi
	[ -n "${end}" ] && write_scalar end_balance "${end}"
	note "=== dogfood run audit ==="
	cmd_status --root "${ROOT}"
	if [ -n "${end}" ]; then
		note "closing balance ....... ${end} Buzz"
		note "MEASURED TOTAL SPEND .. $(( start - end )) Buzz"
	fi
	note ""
	note "full invocation log ... ${ROOT}/ledger/invocations.tsv"
	note "balance samples ....... ${ROOT}/ledger/buzz.tsv"
}

cmd_enter() {
	local root="${DEFAULT_ROOT}" with_claude=0
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--with-claude) with_claude=1; shift ;;
			*) break ;;
		esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"
	command -v bwrap >/dev/null 2>&1 || die "enter: bwrap (bubblewrap) is not on PATH"

	# --with-claude seeds a FRESH $HOME with ONLY the credential file.
	#
	# 🔴 THE HOST ~/.claude IS NEVER BOUND, AND THAT IS THE WHOLE POINT. It
	# holds `projects/` — 34 project directories on this host, including this
	# repo's own transcripts and both prior dogfood runs — so binding it would
	# hand the agent exactly what the method exists to hide. Measured: a fresh
	# HOME containing only `.claude/.credentials.json` runs a real session
	# (`claude -p` → rc 0), the session CREATES `projects/` itself, and every
	# blindness probe reads 0 inside the jail while the same rig reads
	# 120/1507/331/3 with the mounts deliberately wrong.
	if [ "${with_claude}" = 1 ]; then
		local hostcred="${HOME}/.claude/.credentials.json"
		[ -r "${hostcred}" ] || die "enter --with-claude: no credential at ${hostcred}"
		mkdir -p "${ROOT}/home/.claude"
		(umask 077; cp "${hostcred}" "${ROOT}/home/.claude/.credentials.json") \
			|| die "enter --with-claude: could not seed the credential"
		chmod 0600 "${ROOT}/home/.claude/.credentials.json"
	fi

	local shell_bin
	shell_bin=$(command -v bash) || die "enter: no bash on PATH"
	[ $# -gt 0 ] || set -- "${shell_bin}" -i

	# 🔴 DO NOT BIND /nix WHOLESALE. `nix build` copies the WHOLE working tree
	# to a world-readable /nix/store/<hash>-source (flake.nix: `src =
	# lib.cleanSource ./.`), so `--ro-bind /nix /nix` put this repo — AGENTS.md,
	# the handoffs, the decisions — back inside a jail whose entire purpose is
	# that the agent cannot read them. Measured: 4 readable copies of this
	# repo's AGENTS.md under /nix/store, and one `grep -rl` ends the blindness.
	# The earlier §5 transcript checked /home and the repo path and never
	# checked /nix, which is exactly how the claim survived.
	#
	# So: bind only the CLOSURE of the handful of tools the jail needs. Anything
	# not reachable from those roots — including every -source copy — is absent.
	local -a roots=() binds=()
	local b p
	# 🔴 `command -v` returns the NAME for a shell builtin, not a path —
	# `command -v printf` is literally `printf`. Feeding that to readlink -f
	# yields a cwd-relative path that does not exist, and nix-store then fails
	# on the whole list, which read as "nix-store is missing". Only absolute
	# paths are roots.
	# 🔴 node/npm ARE IN THE CLOSURE, AND THAT IS A DELIBERATE WIDENING OF THE
	# EXECUTION SURFACE. Without them the run cannot reach its own purpose:
	# measured, `app create <prefix>myapp` succeeds but writes no lockfile, and
	# `app validate` then exits 1 (`npm ci … hard-fails`), so the agent can never
	# produce a submittable app. With them, `npm install` reaches
	# registry.npmjs.org AND RUNS PACKAGE LIFECYCLE SCRIPTS — arbitrary code
	# execution inside the jail that did not exist before. The jail still has no
	# access to the repo, the operator's HOME or the real credential store, so
	# the blast radius is the sandbox plus the network; it is not nothing, and
	# the doc says so.
	local tools="bash env cat cp rm mkdir chmod chown date grep sed tr head cut wc
	             sha256sum flock timeout mv ls readlink stat sort tail touch find id
	             dirname basename mktemp sleep awk diff comm coreutils
	             node npm npx git tar gzip xz which uname getent"
	for b in ${tools}; do
		p=$(command -v "${b}" 2>/dev/null) || continue
		case "${p}" in /*) ;; *) continue ;; esac
		p=$(readlink -f "${p}") || continue
		[ -e "${p}" ] || continue
		roots+=("${p}")
	done
	if [ "${with_claude}" = 1 ]; then
		p=$(command -v claude 2>/dev/null) || p=""
		[ -n "${p}" ] && roots+=("$(readlink -f "${p}")")
	fi
	[ "${#roots[@]}" -gt 0 ] || die "enter: could not resolve any tool paths"

	local ca=""
	for p in /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-bundle.crt; do
		[ -e "${p}" ] && { ca=$(readlink -f "${p}"); break; }
	done
	[ -n "${ca}" ] && roots+=("${ca}")

	# nix-store is not always on PATH even where it exists (a restricted shell,
	# a non-login environment), so look in the usual places rather than
	# concluding it is absent and refusing.
	local nixstore=""
	for p in "$(command -v nix-store 2>/dev/null)" \
	         /run/current-system/sw/bin/nix-store \
	         /nix/var/nix/profiles/default/bin/nix-store; do
		[ -n "${p}" ] && [ -x "${p}" ] && { nixstore="${p}"; break; }
	done

	local closure=""
	[ -n "${nixstore}" ] && closure=$("${nixstore}" -qR "${roots[@]}" 2>/dev/null | sort -u)
	if [ -z "${closure}" ]; then
		die "enter: could not compute the store closure (nix-store missing or failed). Refusing to fall back to binding /nix wholesale, which would put the repo back inside the jail."
	fi
	while IFS= read -r p; do
		[ -n "${p}" ] || continue
		binds+=(--ro-bind "${p}" "${p}")
	done <<< "${closure}"

	local envbin path_dirs
	envbin=$(readlink -f "$(command -v env)")
	path_dirs="${ROOT}/bin"
	for p in "${roots[@]}"; do
		case "${path_dirs}:" in *":$(dirname "${p}"):"*) continue ;; esac
		path_dirs="${path_dirs}:$(dirname "${p}")"
	done

	# 🔴 ORDER IS LOAD-BEARING, TWICE OVER. bwrap applies these in sequence, so
	# `--tmpfs /tmp` must come BEFORE the run-root bind (the default run root
	# lives under /tmp and a later tmpfs SHADOWS it — measured, bwrap died with
	# "Can't chdir to <root>/workspace"), and the read-only re-binds of `real/`,
	# `harness/` and `bin/` must come AFTER the read-write parent bind or the
	# parent overrides them. Those three re-binds move the `upgrade` refusal and
	# the harness's own integrity from convention to KERNEL: measured before
	# them, `chmod u+w $ROOT/real` and `touch $ROOT/real/.civitai.new` both
	# SUCCEEDED inside the jail.
	#
	# 🔴 `--clearenv` IS NOT COSMETIC. Blindness is a mount property; the
	# environment is not a mount. Measured inside without it: 118 inherited
	# variables, including a real `CIVITAI_TOKEN` (a second credential sitting
	# outside every gate), an unrelated `sk-ant-…` key, and a `CDPATH` naming
	# the very repo the jail exists to hide.
	exec bwrap \
		"${binds[@]}" \
		--ro-bind "${envbin}" /usr/bin/env \
		--ro-bind /etc/resolv.conf /etc/resolv.conf \
		${ca:+--ro-bind "${ca}" /etc/ssl/certs/ca-certificates.crt} \
		--proc /proc --dev /dev --tmpfs /tmp \
		--bind "${ROOT}" "${ROOT}" \
		--ro-bind "${ROOT}/real" "${ROOT}/real" \
		--ro-bind "${ROOT}/harness" "${ROOT}/harness" \
		--ro-bind "${ROOT}/bin" "${ROOT}/bin" \
		--unshare-pid --die-with-parent \
		--clearenv \
		--setenv HOME "${ROOT}/home" \
		--setenv XDG_CONFIG_HOME "${ROOT}/home/.config" \
		--setenv XDG_CACHE_HOME "${ROOT}/home/.cache" \
		--setenv CIVITAI_NO_UPDATE_CHECK 1 \
		${ca:+--setenv SSL_CERT_FILE /etc/ssl/certs/ca-certificates.crt} \
		--setenv PATH "${path_dirs}" \
		--chdir "${ROOT}/workspace" \
		"$@"
}

cmd_teardown() {
	local root="${DEFAULT_ROOT}" delete=0 quiet=0
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--delete) delete=1; shift ;;
			--quiet) quiet=1; shift ;;
			*) die "teardown: unknown flag $1" ;;
		esac
	done
	# 🔴 THE SIBLING SCAN MUST NOT BE BEHIND THE EXISTENCE CHECK. It was, and a
	# failed `init --force` is exactly the case that leaves NO run root and a
	# superseded one still holding both credentials — so teardown printed "does
	# not exist" and returned, leaving the thing it exists to remove.
	if [ -e "${root}" ]; then
		chmod -R u+w "${root}" 2>/dev/null
		if [ "${delete}" = 1 ]; then
			shred_secrets "${root}"
			rm -rf "${root}"
			[ "${quiet}" = 1 ] || note "teardown: deleted ${root}"
		else
			[ "${quiet}" = 1 ] || note "teardown: unlocked ${root} (not deleted; pass --delete to remove)"
		fi
	else
		[ "${quiet}" = 1 ] || note "teardown: ${root} does not exist"
	fi

	# 🔴 `init --force` LEAVES A SUPERSEDED ROOT, AND IT HOLDS BOTH CREDENTIALS.
	# The go/no-go is scrupulous about `shred -u` on the operator's own token
	# file and was silent about the copies the harness itself makes: the Civitai
	# key in `home/.config/civitai/config.yaml` and, after `--with-claude`, the
	# Anthropic OAuth token in `home/.claude/.credentials.json`. `teardown
	# --delete` removed only ${ROOT} and never the siblings.
	local sib found=0
	for sib in "${root}".superseded-*; do
		[ -e "${sib}" ] || continue
		found=1
		if [ "${delete}" = 1 ]; then
			chmod -R u+w "${sib}" 2>/dev/null
			shred_secrets "${sib}"
			rm -rf "${sib}"
			[ "${quiet}" = 1 ] || note "teardown: deleted superseded run ${sib}"
		else
			note "teardown: 🔴 ${sib} still holds a credential — re-run with --delete"
		fi
	done
	[ "${found}" = 1 ] || true
}

# Overwrite credential material before unlinking it. `shred` is best-effort on a
# journalling or copy-on-write filesystem — it is not a guarantee, it is the
# difference between "deleted" and "deleted and overwritten once".
shred_secrets() {
	local root="$1" f
	# Three credentials can exist in a run root: the Civitai token, the Anthropic
	# OAuth token under --with-claude, and any git access token `app pull` wrote
	# into a workspace .git/config.
	local gitcfg
	while IFS= read -r gitcfg; do
		[ -n "${gitcfg}" ] && rm -f "${gitcfg}" 2>/dev/null
	done < <(find "${root}/workspace" -name config -path '*/.git/*' 2>/dev/null)
	for f in "${root}/ledger/credential" \
	         "${root}/home/.config/civitai/config.yaml" \
	         "${root}/home/.claude/.credentials.json"; do
		[ -f "${f}" ] || continue
		if command -v shred >/dev/null 2>&1; then
			shred -u "${f}" 2>/dev/null || rm -f "${f}"
		else
			rm -f "${f}"
		fi
	done
}

# ---------------------------------------------------------------------------
# selftest
# ---------------------------------------------------------------------------
SELFTEST_PASS=0
SELFTEST_FAIL=0

st_check() {
	local label="$1" want="$2" got="$3"
	if [ "${want}" = "${got}" ]; then
		printf '  PASS  %-62s (%s)\n' "${label}" "${got}"; SELFTEST_PASS=$(( SELFTEST_PASS + 1 ))
	else
		printf '  FAIL  %-62s want=%s got=%s\n' "${label}" "${want}" "${got}"; SELFTEST_FAIL=$(( SELFTEST_FAIL + 1 ))
	fi
}

vd() { policy_verdict "$@"; printf '%s' "${VERDICT}"; }

# 🔴 A DENY is not evidence that the branch you meant was reached. Measured
# twice now: with the command resolved positionally, `--no-color generate …
# --yes` still DENIED — as an unrecognised command called `--no-color` — and a
# blind auditor found a dev-tunnel row carried by a bystander the same way.
# These rows pin the REASON.
# 🔴 A REASON IS NOT EVIDENCE OF A DENY — the exact inverse of the note above,
# and it was missed for two rounds. `allow "…"` and `deny "…"` set the same
# VERDICT_REASON, so a `deny`→`allow` flip with a byte-identical message was
# INVISIBLE: measured, 5 of 5 such flips (login, TOTAL_SPEND_CAP,
# MAX_GENERATE_SUBMITS, the withdraw allowlist, the --no-wait gate) survived a
# 177/177 suite. This asserts BOTH, so the verdict cannot be carried by the
# message and the message cannot be carried by the verdict.
st_reason() {
	local label="$1" want_verdict="$2" needle="$3"; shift 3
	policy_verdict "$@"
	local got="${VERDICT}"
	case "${VERDICT_REASON}" in
		*"${needle}"*) ;;
		*) got="${VERDICT}/reason:${VERDICT_REASON}" ;;
	esac
	st_check "${label}" "${want_verdict}/reason" "${got}/reason"
}

st_reset_ledger() {
	write_scalar observed_spend "${1:-0}"
	write_scalar reserved "${2:-0}"
	write_scalar generate_submits "${3:-0}"
	write_scalar app_submits "${4:-0}"
	write_scalar listing_mutations "${5:-0}"
}

cmd_selftest() {
	local root="${DEFAULT_ROOT}" offline=0
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			--offline) offline=1; shift ;;
			*) shift ;;
		esac
	done

	if [ "${offline}" = 1 ]; then
		root="${TMPDIR:-/tmp}/dogfood-selftest-$$"
		rm -rf "${root}"; mkdir -p "${root}/ledger" "${root}/workspace" "${root}/real"
		cat > "${root}/ledger/policy.env" <<OFFLINE
ROOT="${root}"
PER_CALL_MAX_COST=100
TOTAL_SPEND_CAP=2000
MAX_GENERATE_SUBMITS=20
MAX_APP_SUBMITS=3
MAX_LISTING_MUTATIONS=10
METER_SETTLE_SECONDS=0
REQUIRE_CALIBRATION=0
ALLOW_NO_WAIT=0
SLUG_PREFIX="sanctioned-"
BASE_URL="https://civitai.com"
OFFLINE
		: > "${root}/ledger/pubreq.allow"
		: > "${root}/ledger/workflow.allow"
		local repo
		repo=$(cd "$(dirname "$(readlink -f "$0")")/.." && pwd) || die "selftest: cannot locate the repo"
		command -v go >/dev/null 2>&1 || die "selftest --offline needs go to build internal/dogfoodguard"
		( cd "${repo}" && go build -o "${root}/real/dogfoodguard" ./internal/dogfoodguard ) \
			|| die "selftest: could not build the classifier"
	fi

	ROOT="${root}"
	load_policy || die "selftest: ${POLICY_ERROR}"
	st_reset_ledger

	local td="${ROOT}/ledger/_selftest"
	rm -rf "${td}"; mkdir -p "${td}/good" "${td}/bad"
	printf '{"blockId":"%sdemo","name":"d","version":"1.0.0","kind":"page"}\n' "${SLUG_PREFIX}" > "${td}/good/block.manifest.json"
	printf '{"blockId":"someones-real-app","name":"d","version":"1.0.0","kind":"page"}\n'        > "${td}/bad/block.manifest.json"

	note "--- the classifier is the REAL cobra tree ---"
	st_check "classifier present" "yes" "$([ -x "${ROOT}/real/dogfoodguard" ] && printf yes || printf no)"
	st_check "an unclassifiable invocation denies" "DENY" "$(vd --definitely-not-a-flag)"

	# 🔴 The shim and the classifier are versioned together. Output the shim
	# does not fully understand means they have DRIFTED, and drift is exactly
	# the failure this whole design removes — so it denies rather than acting on
	# the fields it did recognise. Only runnable where the classifier is
	# writable (the offline root); the credentialed sandbox locks it 0500.
	local fake="${ROOT}/ledger/_fakeguard"
	mkfake() { printf '#!/usr/bin/env bash\n%s\n' "$1" > "${fake}"; chmod 0755 "${fake}"; GUARD_BIN="${fake}"; }
	mkfake 'printf "ok\t1\npath\tgenerate\nbogus\tx\n"'
	st_reason "an UNRECOGNISED classifier field denies" "DENY" "could not be classified" generate cat --yes --max-cost 5
	mkfake 'printf "path\tgenerate\n"'
	st_reason "classifier output with no ok field denies" "DENY" "could not be classified" generate cat --yes --max-cost 5
	mkfake 'exit 9'
	st_reason "a silent non-zero classifier denies" "DENY" "could not be classified" generate cat --dry-run
	mkfake 'printf "ok\t1\npath\tgenerate\nargc\tnotanumber\n"'
	st_reason "a non-numeric argument count denies" "DENY" "could not be classified" generate cat --dry-run
	# 🔴 THE CLAIM THAT WAS HERE WAS WRONG. It said cobra's ValidateArgs never
	# errors for these commands. Measured, it does: `workflows cancel` with no
	# id gives "accepts 1 arg(s), received 0", and `generate a b` gives "accepts
	# at most 1 arg(s), received 2" — both are now real rows below. Only the
	# `unknown-command` KIND is unreachable through the corpus (cobra reports
	# those spellings as flag-parse errors), so it alone is exercised through an
	# injected classifier.
	mkfake 'printf "ok\t0\nkind\tunknown-command\nerr\tunknown command \"zzz\"\n"; exit 2'
	st_reason "an unknown-command kind denies" "DENY" "does not recognise this command" zzz
	GUARD_BIN=""; rm -f "${fake}"
	# Real argserr cases, measured against the shipped tree rather than faked.
	st_reason "workflows cancel with no id denies" "DENY" "would reject these arguments" workflows cancel
	st_reason "generate with two positionals denies" "DENY" "would reject these arguments" generate a b
	st_check  "civitai help generate is help"     "ALLOW" "$(vd help generate)"
	st_check  "civitai help is help"              "ALLOW" "$(vd help)"
	# A classifier that claims everything is a harmless read must still not be
	# able to say so from OUTSIDE the sandbox: GUARD_BIN is reset at startup, so
	# an exported value cannot reach the gate.
	# 🔴 THE FIXTURE MUST BE ALLOW-SHAPED, OR THE ROW IS A BYSTANDER. With
	# `GUARD_BIN=/bin/true` the injected classifier produces NO output, so the
	# gate denies for that reason whether or not the reset exists — the row
	# passed with the protection deleted. An allow-shaped fake distinguishes
	# them: honoured it says ALLOW, ignored it says DENY.
	mkfake 'printf "ok\t1\npath\twhoami\nargc\t0\nhassubcommands\tfalse\nargserr\t\n"'
	st_check "an injected classifier IS honoured inside the selftest" "ALLOW" "$(vd generate cat --yes)"
	cp "${fake}" "${ROOT}/ledger/_allowfake"
	GUARD_BIN=""; rm -f "${fake}"
	st_check "the real classifier is back" "DENY" "$(vd generate cat --yes)"
	st_check "an INHERITED GUARD_BIN is ignored" "DENY" \
		"$(GUARD_BIN="${ROOT}/ledger/_allowfake" bash "$0" __verdict --root "${ROOT}" -- generate cat --yes 2>/dev/null)"
	rm -f "${ROOT}/ledger/_allowfake"

	note "--- a DELETED counter is malformed, not zero ---"
	local cf
	for cf in observed_spend generate_submits; do
		mv "${ROOT}/ledger/${cf}" "${ROOT}/ledger/${cf}.away"
		st_reason "a DELETED ${cf} refuses a spend" "DENY" "unreadable or malformed" generate cat --yes --max-cost 5
		mv "${ROOT}/ledger/${cf}.away" "${ROOT}/ledger/${cf}"
	done

	note "--- forbidden commands (reason-pinned, so no bystander can carry them) ---"
	st_reason "upgrade denied AS upgrade"        "DENY" "replace the binary"        upgrade
	st_reason "login denied AS login"            "DENY" "already authenticated"     login --token x
	st_reason "dev-tunnel denied AS dev-tunnel"  "DENY" "out of scope"              app dev-tunnel
	st_reason "dev-token denied AS dev-token"    "DENY" "out of scope"              app dev-token
	st_reason "unknown top-level denied"         "DENY" "not a subcommand"          frobnicate
	st_reason "unknown app subcommand denied"    "DENY" "not a subcommand"          app frobnicate
	st_reason "unknown listing verb denied"      "DENY" "not a subcommand"          app listing frobnicate
	st_reason "unknown workflows verb denied"    "DENY" "not a subcommand"          workflows frobnicate

	note "--- root persistent flags before the subcommand ---"
	st_reason "--no-color generate hits the MONEY branch" "DENY" "--max-cost" --no-color generate cat --yes
	st_reason "--no-color upgrade denied AS upgrade"      "DENY" "replace the binary" --no-color upgrade
	st_reason "--no-update-check upgrade denied"          "DENY" "replace the binary" --no-update-check upgrade
	st_reason "--color login denied AS login"             "DENY" "already authenticated" --color login --token x
	st_reason "app --no-color submit foreign on PREFIX"   "DENY" "sanctioned prefix" app --no-color submit "${td}/bad" --yes
	st_check  "--no-color generate --max-cost is a spend" "ALLOW_SPEND" "$(vd --no-color generate cat --yes --max-cost 5)"

	note "--- a flag VALUE that looks like a flag ---"
	st_check "--negative-prompt --help is NOT help"     "DENY" "$(vd generate cat --yes --negative-prompt --help)"
	st_check "--negative-prompt --dry-run is NOT free"  "DENY" "$(vd generate cat --yes --negative-prompt --dry-run)"
	st_check "--ecosystem --print-input is NOT free"    "DENY" "$(vd generate cat --yes --ecosystem --print-input)"
	st_check "submit -o --help is NOT help"             "DENY" "$(vd app submit "${td}/bad" --yes -o --help)"
	st_check "genuine generate --help"                  "ALLOW" "$(vd generate --help)"
	st_check "genuine upgrade --help"                   "ALLOW" "$(vd upgrade --help)"
	st_check "genuine listing set-icon --help"          "ALLOW" "$(vd app listing set-icon --help)"
	st_check "bare --help"                              "ALLOW" "$(vd --help)"

	note "--- 🔴 ALL TWELVE pflag boolean spellings, on every gated bool ---"
	local v
	for v in 1 t T TRUE true True; do
		st_check "--dry-run=${v} is a free preview"      "ALLOW" "$(vd generate cat --yes --dry-run="${v}")"
		st_check "--print-input=${v} is a free preview"  "ALLOW" "$(vd generate cat --yes --print-input="${v}")"
	done
	for v in 0 f F FALSE false False; do
		st_check "--dry-run=${v} still SPENDS"           "DENY"  "$(vd generate cat --yes --dry-run="${v}")"
		st_check "--print-input=${v} still SPENDS"       "DENY"  "$(vd generate cat --yes --print-input="${v}")"
		st_check "--help=${v} is NOT a help request"     "DENY"  "$(vd generate cat --yes --help="${v}")"
		st_reason "--package-only=${v} still gated"      "DENY" "sanctioned prefix" app submit "${td}/bad" --yes --package-only="${v}"
	done
	for v in 1 t T TRUE true True; do
		st_check "--help=${v} IS a help request"         "ALLOW" "$(vd generate cat --yes --help="${v}")"
		st_check "--package-only=${v} never submits"     "ALLOW" "$(vd app submit "${td}/bad" --yes --package-only="${v}")"
	done
	st_reason "an unparseable bool value denies"  "DENY" "would reject these flags" generate cat --yes --dry-run=maybe
	st_reason "an unparseable int value denies"   "DENY" "would reject these flags" generate cat --yes --max-cost abc
	st_reason "--max-cost 08 denies (not a crash)" "DENY" "would reject these flags" generate cat --yes --max-cost 08

	note "--- generate money path ---"
	st_check "real dry-run is free"            "ALLOW"       "$(vd generate cat --dry-run)"
	st_check "no --max-cost denied"            "DENY"        "$(vd generate cat --yes)"
	st_check "--max-cost at ceiling allowed"   "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 100)"
	st_check "--max-cost=N form"               "ALLOW_SPEND" "$(vd generate cat --yes --max-cost=1)"
	st_reason "--max-cost over ceiling denied" "DENY" "per-invocation ceiling" generate cat --yes --max-cost 101
	st_check "LAST --max-cost wins (5 then 99999)"  "DENY"        "$(vd generate cat --yes --max-cost 5 --max-cost 99999)"
	st_check "LAST --max-cost wins (99999 then 5)"  "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 99999 --max-cost 5)"
	st_reason "--no-wait refused by default"   "DENY" "settled" generate cat --yes --max-cost 5 --no-wait

	note "--- 🔴 EACH CAP, ISOLATED, REASON-PINNED ---"
	st_reset_ledger 2000 0 0 0 0
	st_reason "TOTAL_SPEND_CAP branch"        "DENY" "total spend cap" generate cat --yes --max-cost 1
	st_reset_ledger 1995 0 0 0 0
	st_reason "remaining-budget branch"       "DENY" "left under this run's total cap" generate cat --yes --max-cost 50
	st_reset_ledger 0 0 20 0 0
	st_reason "MAX_GENERATE_SUBMITS branch"   "DENY" "generation cap" generate cat --yes --max-cost 1
	# A reservation alone must reach the cap: 0 observed + 2000 in flight is a
	# full budget even though nothing has settled yet.
	st_reset_ledger 0 2000 0 0 0
	st_reason "the RESERVATION alone reaches the cap" "DENY" "committed in flight" generate cat --yes --max-cost 1
	st_reset_ledger 0 1999 0 0 0
	st_reason "a reservation shrinks the remaining budget" "DENY" "left under this run's total cap" generate cat --yes --max-cost 5
	st_reset_ledger 0 0 0 3 0
	st_reason "MAX_APP_SUBMITS branch"        "DENY" "app-submission cap" app submit "${td}/good" --yes
	st_reset_ledger 0 0 0 0 10
	st_reason "MAX_LISTING_MUTATIONS branch"  "DENY" "listing-mutation cap" app listing set-icon i.png --dir "${td}/good"
	st_reset_ledger

	note "--- 🔴 a malformed or EMPTY ledger scalar is not zero ---"
	local f
	for f in observed_spend reserved generate_submits; do
		printf 'garbage\n' > "${ROOT}/ledger/${f}"
		st_reason "malformed ${f} refuses a spend" "DENY" "unreadable or malformed" generate cat --yes --max-cost 5
		: > "${ROOT}/ledger/${f}"
		st_reason "EMPTY ${f} refuses a spend"     "DENY" "unreadable or malformed" generate cat --yes --max-cost 5
		write_scalar "${f}" 0
	done
	printf 'garbage\n' > "${ROOT}/ledger/app_submits"
	st_reason "malformed app_submits refuses a submit" "DENY" "unreadable or malformed" app submit "${td}/good" --yes
	write_scalar app_submits 0
	printf 'garbage\n' > "${ROOT}/ledger/listing_mutations"
	st_reason "malformed listing_mutations refuses" "DENY" "unreadable or malformed" app listing set-icon i.png --dir "${td}/good"
	write_scalar listing_mutations 0

	note "--- app identity gating ---"
	st_check "submit sanctioned"               "ALLOW_APP_SUBMIT" "$(vd app submit "${td}/good" --yes)"
	st_reason "submit foreign denied"          "DENY" "sanctioned prefix" app submit "${td}/bad" --yes
	st_check "submit -o value not read as dir" "ALLOW_APP_SUBMIT" "$(vd app submit -o "${td}/bad" "${td}/good")"
	st_check "listing sanctioned --dir"        "ALLOW_LISTING"    "$(vd app listing set-icon i.png --dir "${td}/good")"
	st_reason "listing foreign --slug denied"  "DENY" "may not touch the account's real listings" app listing set-icon i.png --slug someones-real-app
	st_reason "listing unidentifiable denied"  "DENY" "cannot identify" app listing set-icon i.png --dir "${ROOT}/ledger"
	st_check "listing status is a read"        "ALLOW" "$(vd app listing status)"

	note "--- unknown subcommands, asked of cobra rather than a table ---"
	local parent
	for parent in models images users tags articles collections creators model-versions app workflows; do
		st_reason "unknown subcommand under \`${parent}\` denied" "DENY" "is not a subcommand" ${parent} frobnicate
	done
	st_check "a known subcommand still runs"      "ALLOW" "$(vd models search --query x)"
	st_check "a parent with no args is usage"     "ALLOW" "$(vd models)"
	st_check "a positional the CLI accepts"       "ALLOW" "$(vd app view someslug)"
	st_check "download takes a positional"        "ALLOW" "$(vd download 12345)"

	note "--- 🔴 the manifest is parsed by encoding/json, not by grep ---"
	printf '{"blockId":"%sok","name":"n","version":"1.0.0","kind":"page","blockId":"someones-real-app"}\n' "${SLUG_PREFIX}" \
		> "${td}/good/dup.json"
	mkdir -p "${td}/dup"; cp "${td}/good/dup.json" "${td}/dup/block.manifest.json"
	st_reason "a DUPLICATE blockId resolves to the LAST value, as Go does" "DENY" \
		"sanctioned prefix" app submit "${td}/dup" --yes
	printf 'not json at all\n' > "${td}/dup/block.manifest.json"
	st_reason "an unparseable manifest denies" "DENY" "cannot read a blockId" app submit "${td}/dup" --yes
	printf '{"name":"n"}\n' > "${td}/dup/block.manifest.json"
	st_reason "a manifest with no blockId denies" "DENY" "cannot read a blockId" app submit "${td}/dup" --yes
	# A classifier that reports failure while EXITING ZERO. Today's classifier
	# never does this — `|| return 1` already catches its non-zero exit — so
	# without this row the ok-check is an unpinned redundancy.
	# The fake must SUCCEED for the argv classification and FAIL only for the
	# blockid lookup, or the run is denied earlier and the row proves nothing.
	# shellcheck disable=SC2016  # the body is a script; $1/$4 must NOT expand here
	mkfake 'if [ "$1" = "blockid" ]; then printf "ok\t0\nerr\tno manifest\n"; exit 0; fi
printf "ok\t1\npath\tapp submit\nhassubcommands\tfalse\nargserr\t\nbool.package-only\tfalse\nargc\t1\narg.0\t%s\n" "$4"'
	st_reason "a lookup that fails while exiting 0 denies" "DENY" "cannot read a blockId" app submit "${td}/dup" --yes
	GUARD_BIN=""; rm -f "${fake}"
	rm -rf "${td}/dup" "${td}/good/dup.json"

	note "--- --timeout is an unblocked --no-wait ---"
	st_reason "--timeout refused by default" "DENY" "stops the CLI waiting" generate cat --yes --max-cost 5 --timeout 1s
	st_check  "no --timeout is fine"         "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 5)"

	note "--- 🔴 app pull reaches REAL apps and provisions a git credential ---"
	st_reason "pull of a foreign app denied" "DENY" "outside the sanctioned prefix" \
		app pull --app someones-real-app
	st_check  "pull of our own app allowed" "ALLOW" "$(vd app pull --app "${SLUG_PREFIX}mine")"
	# --app is a REQUIRED flag, so the classifier reports it and the gate denies
	# — which is what the binary does too (exit 2). Not a distortion.
	st_reason "pull with no --app denied as the CLI would" "DENY" "would reject these arguments" app pull

	note "--- withdraw + workflows cancel allowlists ---"
	st_reason "unknown pubreq denied"          "DENY" "not created by this sandbox" app withdraw pubreq_UNKNOWN
	st_reason "non-pubreq-shaped id denied"    "DENY" "not created by this sandbox" app withdraw NOT-A-PUBREQ-ID
	st_reason "--id form denied"               "DENY" "not created by this sandbox" app withdraw --id NOT-A-PUBREQ
	st_check  "no id is a CLI usage error"     "ALLOW" "$(vd app withdraw)"
	printf 'pubreq_MINE\n' >> "${ROOT}/ledger/pubreq.allow"
	st_check  "allowlisted pubreq permitted"   "ALLOW" "$(vd app withdraw pubreq_MINE)"
	st_reason "cancel of a foreign workflow denied" "DENY" "DOES NOT REFUND" workflows cancel someone-elses
	printf 'wf-mine\n' >> "${ROOT}/ledger/workflow.allow"
	st_check  "cancel of our own workflow allowed" "ALLOW" "$(vd workflows cancel wf-mine)"
	st_check  "workflows list is a read"       "ALLOW" "$(vd workflows list)"

	note "--- calibration + meter_broken gates ---"
	printf 'REQUIRE_CALIBRATION=1\n' >> "${ROOT}/ledger/policy.env"
	load_policy || die "selftest: policy reload failed"
	st_reason "uncalibrated meter refuses to spend" "DENY" "calibrated" generate cat --yes --max-cost 5
	printf 'calibrated\n' > "${ROOT}/ledger/calibration.ok"
	st_check  "calibrated meter may spend" "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 5)"
	: > "${ROOT}/ledger/meter_broken"
	st_reason "a broken meter refuses to spend" "DENY" "no longer being counted" generate cat --yes --max-cost 5
	st_check  "dry-run still allowed with a broken meter" "ALLOW" "$(vd generate cat --dry-run)"
	rm -f "${ROOT}/ledger/meter_broken" "${ROOT}/ledger/calibration.ok"
	grep -v '^REQUIRE_CALIBRATION=1$' "${ROOT}/ledger/policy.env" > "${ROOT}/ledger/p.tmp"
	mv "${ROOT}/ledger/p.tmp" "${ROOT}/ledger/policy.env"
	load_policy || die "selftest: policy restore failed"

	note "--- policy validation ---"
	local pol="${ROOT}/ledger/policy.env" bak="${ROOT}/ledger/policy.bak"
	cp "${pol}" "${bak}"
	local key
	for key in PER_CALL_MAX_COST TOTAL_SPEND_CAP MAX_GENERATE_SUBMITS MAX_APP_SUBMITS \
	           MAX_LISTING_MUTATIONS REQUIRE_CALIBRATION ALLOW_NO_WAIT METER_SETTLE_SECONDS; do
		grep -v "^${key}=" "${bak}" > "${pol}"
		if load_policy; then st_check "policy without ${key} rejected" "rejected" "accepted"
		else st_check "policy without ${key} rejected" "rejected" "rejected"; fi
	done
	# 🔴 An EMPTY SLUG_PREFIX matches every blockId, so it is not merely a
	# missing key — it is a gate that silently passes the account's real apps.
	sed 's/^SLUG_PREFIX=.*/SLUG_PREFIX=""/' "${bak}" > "${pol}"
	if load_policy; then st_check "policy with an EMPTY SLUG_PREFIX rejected" "rejected" "accepted"
	else st_check "policy with an EMPTY SLUG_PREFIX rejected" "rejected" "rejected"; fi
	printf 'PER_CALL_MAX_COST=notanumber\n' >> "${pol}"
	if load_policy; then st_check "non-numeric policy key rejected" "rejected" "accepted"
	else st_check "non-numeric policy key rejected" "rejected" "rejected"; fi
	cp "${bak}" "${pol}"; rm -f "${bak}"
	load_policy || die "selftest: could not restore the policy"

	note "--- the balance parser ---"
	local j
	for j in '{"total":3.5}' '{"total":1e9}' '{"total":-500}' '{"total":99999999999999999999}' \
	         '{"blue":1}' '' '{"total":}' '{"total":1,"total":2}'; do
		if parse_buzz_total "${j}" >/dev/null 2>&1; then
			st_check "parser rejects ${j:-<empty>}" "reject" "accept($(parse_buzz_total "${j}"))"
		else
			st_check "parser rejects ${j:-<empty>}" "reject" "reject"
		fi
	done
	st_check "parser accepts compact"       "4187454" "$(parse_buzz_total '{"total":4187454}' || printf FAILED)"
	st_check "parser accepts pretty"        "4187454" "$(parse_buzz_total '{
  "blue": 1338425,
  "total": 4187454
}' || printf FAILED)"
	st_check "parser accepts a real zero"   "0" "$(parse_buzz_total '{"total":0}' || printf FAILED)"

	note "--- TSV escaping + lock ---"
	st_check "newline escaped" 'a\nb' "$(tsv_escape "$(printf 'a\nb')")"
	st_check "tab escaped"     'a\tb' "$(tsv_escape "$(printf 'a\tb')")"
	st_check "argv scrub escapes a newline" 'gen a\nDENY\tx' "$(scrub_argv gen "$(printf 'a\nDENY\tx')")"
	st_check "token redacted"  'login --token <redacted>' "$(scrub_argv login --token SECRETVALUE)"
	st_check "flock available" "yes" "$(command -v flock >/dev/null 2>&1 && printf yes || printf no)"

	rm -rf "${td}"

	if [ "${offline}" = 1 ]; then
		selftest_e2e "${ROOT}/real/dogfoodguard"
		selftest_init_contract "${ROOT}/real/dogfoodguard"
		note ""
		note "--- offline mode: the credentialed checks below were skipped ---"
		rm -rf "${ROOT}"
	else
		note ""
		note "--- balance meter (a real read; spends nothing) ---"
		local b
		if b=$(sample_balance "selftest"); then
			st_check "buzz --json parses to an integer" "yes" "$(case "${b}" in ''|*[!0-9]*) printf no ;; *) printf yes ;; esac)"
			note "  observed balance: ${b} Buzz"
		else
			st_check "buzz --json parses to an integer" "yes" "no"
		fi
		note ""
		note "--- lockdown ---"
		st_check "binary dir not writable"  "no" "$([ -w "${ROOT}/real" ] && printf yes || printf no)"
		st_check "binary file not writable" "no" "$([ -w "${ROOT}/real/civitai" ] && printf yes || printf no)"
		st_check "classifier present"       "yes" "$([ -x "${ROOT}/real/dogfoodguard" ] && printf yes || printf no)"
		st_check "credential is NOT on disk in HOME" "no" \
			"$([ -f "${ROOT}/home/.config/civitai/config.yaml" ] && printf yes || printf no)"
		st_check "credential is held for injection"  "yes" \
			"$([ -s "${ROOT}/ledger/credential" ] && printf yes || printf no)"
		st_reset_ledger
	fi

	note ""
	note "selftest: ${SELFTEST_PASS} passed, ${SELFTEST_FAIL} failed"
	[ "${SELFTEST_FAIL}" = 0 ]
}

# ---------------------------------------------------------------------------
# init + calibrate, driven by a stub binary. No credential, no network.
# ---------------------------------------------------------------------------
selftest_init_contract() {
	local base="${TMPDIR:-/tmp}/dogfood-init-$$"
	local guard_src="$1"
	rm -rf "${base}"; mkdir -p "${base}"
	local stub="${base}/civitai-stub"
	cat > "${stub}" <<'STUB'
#!/usr/bin/env bash
set -u
: "${XDG_CACHE_HOME:=${HOME}/.cache}"
mkdir -p "${XDG_CACHE_HOME}"
BAL="${XDG_CACHE_HOME}/bal"; [ -f "${BAL}" ] || echo 10000 > "${BAL}"
a=(); for x in "$@"; do case "${x}" in --no-color|--color|--no-update-check) ;; *) a+=("${x}") ;; esac; done
set -- "${a[@]+"${a[@]}"}"
case "${1:-}" in
  buzz)   [ -n "${CIVITAI_TOKEN:-}" ] || { echo "unauthorized (401)" >&2; exit 3; }
          printf '{\n  "total": %s\n}\n' "$(cat "${BAL}")" ;;
  whoami) [ -n "${CIVITAI_TOKEN:-}" ] || { echo "unauthorized (401)" >&2; exit 3; }
          echo authed ;;
  generate) [ -n "${CIVITAI_TOKEN:-}" ] || { echo "unauthorized (401)" >&2; exit 3; }
          echo $(( $(cat "${BAL}") - 37 )) > "${BAL}"; echo generated ;;
  *) : ;;
esac
exit 0
STUB
	chmod 0755 "${stub}"

	local run="${base}/run"
	CIVITAI_DOGFOOD_TOKEN=stub-token bash "$(readlink -f "$0")" init \
		--root "${run}" --binary "${stub}" --guard "${guard_src}" \
		--slug-prefix "sanctioned-" --settle-seconds 0 >/dev/null 2>&1

	note "--- 🔴 init leaves NO credential in the sandbox HOME ---"
	st_check "init wrote no civitai config" "no" \
		"$([ -f "${run}/home/.config/civitai/config.yaml" ] && printf yes || printf no)"
	st_check "init stored the credential for injection" "yes" \
		"$([ -s "${run}/ledger/credential" ] && printf yes || printf no)"
	st_check "the credential file is owner-only" "600" \
		"$(stat -c '%a' "${run}/ledger/credential" 2>/dev/null)"
	# POSITIVE CONTROL: an UNGATED binary in the same HOME is unauthenticated,
	# while the gated one works — otherwise "no config" proves nothing.
	HOME="${run}/home" XDG_CONFIG_HOME="${run}/home/.config" \
		XDG_CACHE_HOME="${run}/home/.cache" "${stub}" whoami >/dev/null 2>&1
	st_check "an UNGATED binary is unauthenticated" "3" "$?"
	"${run}/bin/civitai" whoami >/dev/null 2>&1
	st_check "the GATED binary is authenticated"    "0" "$?"

	note "--- 🔴 calibrate spends THROUGH the gate ---"
	bash "$(readlink -f "$0")" calibrate --root "${run}" --yes >/dev/null 2>&1
	st_check "calibrate moved the meter"          "37" "$(cat "${run}/ledger/observed_spend" 2>/dev/null)"
	st_check "calibrate counted a submission"     "1"  "$(cat "${run}/ledger/generate_submits" 2>/dev/null)"
	st_check "calibrate wrote a ledger row"       "1"  "$(grep -c ALLOW_SPEND "${run}/ledger/invocations.tsv" 2>/dev/null || true)"
	st_check "calibrate recorded its verdict"     "yes" \
		"$([ -f "${run}/ledger/calibration.ok" ] && printf yes || printf no)"

	chmod -R u+w "${base}" 2>/dev/null
	rm -rf "${base}"
}

# ---------------------------------------------------------------------------
# End-to-end exercise of the GUARD (not just the classifier), driven by a stub
# binary that charges a known amount. The classifier being right says nothing
# about whether the guard acts on it.
# ---------------------------------------------------------------------------
# grep -c prints "0" AND exits 1 when there are no matches, so `|| printf 0`
# appends a SECOND zero. Count via a helper that always succeeds.
reconciled_rows() {
	grep -c RECONCILED "$1/ledger/invocations.tsv" 2>/dev/null || true
}

selftest_e2e() {
	local base="${TMPDIR:-/tmp}/dogfood-e2e-$$"
	local guard_src="$1"
	rm -rf "${base}"
	mkdir -p "${base}/real" "${base}/bin" "${base}/ledger" \
	         "${base}/home/.config" "${base}/home/.cache" "${base}/workspace"
	cp "${guard_src}" "${base}/real/dogfoodguard" || { st_check "e2e classifier copied" yes no; return; }
	chmod 0755 "${base}/real/dogfoodguard"

	cat > "${base}/real/civitai" <<'STUB'
#!/usr/bin/env bash
set -u
BAL="${XDG_CACHE_HOME}/bal"; NB="${XDG_CACHE_HOME}/nbuzz"
[ -f "${BAL}" ] || echo 10000 > "${BAL}"
[ -f "${NB}" ]  || echo 0 > "${NB}"
args=(); for a in "$@"; do case "${a}" in --no-color|--color|--no-update-check) ;; *) args+=("${a}") ;; esac; done
set -- "${args[@]+"${args[@]}"}"
case "${1:-}" in
  buzz)
    [ -n "${STUB_BUZZ_ALWAYS_FAILS:-}" ] && { echo boom >&2; exit 5; }
    # Fail only the POST read (any buzz after the first of this invocation).
    if [ -n "${STUB_POSTREAD_FAIL:-}" ] && [ "$(cat "${NB}")" -ge 1 ]; then echo boom >&2; exit 5; fi
    # Widen the locked section so the concurrency row is deterministic: the
    # pre-spend balance read happens INSIDE the lock, so without flock the
    # eight racers overlap reliably rather than by luck.
    [ -n "${STUB_SLOW_BUZZ:-}" ] && sleep 0.3
    n=$(cat "${NB}"); echo $(( n + 1 )) > "${NB}"
    if [ -n "${STUB_FLAKY_BUZZ:-}" ] && [ $(( n % 2 )) = 1 ]; then echo boom >&2; exit 5; fi
    printf '{\n  "total": %s\n}\n' "$(cat "${BAL}")" ;;
  workflows)
    if [ -n "${STUB_WORKFLOWS:-}" ]; then
      if [ -f "${XDG_CACHE_HOME}/wf-submitted" ]; then
        printf '[{"id":"wf-old"},{"id":"wf-new"}]\n'
      else printf '[{"id":"wf-old"}]\n'; fi
    fi ;;
  generate)
    # A failure that spends NOTHING — the CLI's own --max-cost refusal shape.
    [ -n "${STUB_FAIL_RC:-}" ] && { echo "estimate exceeds --max-cost" >&2; exit "${STUB_FAIL_RC}"; }
    if [ -n "${STUB_REPORT_INFLIGHT:-}" ]; then
      [ -f "${XDG_CACHE_HOME}/../../ledger/inflight" ] && echo yes > "${XDG_CACHE_HOME}/saw-inflight" || echo no > "${XDG_CACHE_HOME}/saw-inflight"
    fi
    [ -n "${STUB_WORKFLOWS:-}" ] && : > "${XDG_CACHE_HOME}/wf-submitted"
    [ -n "${STUB_SLOW_GEN:-}" ] && sleep "${STUB_SLOW_GEN}"
    # Make the meter unwritable DURING the call, after the pre-flight has
    # already proved it writable — the only way the post-charge write can fail,
    # and the reason meter_write_failed exists.
    [ -n "${STUB_LOCK_METER:-}" ] && chmod 0444 "${XDG_CACHE_HOME}/../../ledger/observed_spend" 2>/dev/null
    if [ -n "${STUB_LAGGY:-}" ]; then :                       # charge never lands
    elif [ -n "${STUB_CREDIT:-}" ]; then echo $(( $(cat "${BAL}") - 37 + 100 )) > "${BAL}"
    else echo $(( $(cat "${BAL}") - 37 )) > "${BAL}"; fi
    # A charge that lands and THEN fails — the shape AGENTS item 28(b) refuses
    # to make claims about, and the one the rc-aware suppression must not hide.
    [ -n "${STUB_FAIL_AFTER_CHARGE:-}" ] && { echo "submitted, then failed" >&2; exit 1; }
    echo generated ;;
  app)
    [ -n "${STUB_APP_RC:-}" ] && { echo "validation failed" >&2; exit "${STUB_APP_RC}"; }
    if [ "${2:-}" = "status" ] && [ -n "${STUB_PUBREQ:-}" ]; then
      printf '{"submissions":[{"publishRequestId":"pubreq_MINE"}]}\n'
    fi
    : ;;
  *) : ;;
esac
exit 0
STUB
	chmod 0755 "${base}/real/civitai"

	cat > "${base}/bin/civitai" <<SHIM
#!/usr/bin/env bash
exec "$(readlink -f "$0")" guard --root "${base}" -- "\$@"
SHIM
	chmod 0755 "${base}/bin/civitai"

	e2e_policy() {
		# The hashes are real: the guard verifies them before every invocation,
		# so a fixture without them refuses everything (which is how this block
		# first went red — correctly).
		cat > "${base}/ledger/policy.env" <<POL
ROOT="${base}"
PER_CALL_MAX_COST=100
TOTAL_SPEND_CAP=2000
MAX_GENERATE_SUBMITS=50
MAX_APP_SUBMITS=3
MAX_LISTING_MUTATIONS=10
METER_SETTLE_SECONDS=0
REQUIRE_CALIBRATION=${1:-0}
ALLOW_NO_WAIT=0
SLUG_PREFIX="sanctioned-"
BASE_URL="https://example.invalid"
BINARY_SHA256="$(sha256sum "${base}/real/civitai" | cut -d' ' -f1)"
GUARD_SHA256="$(sha256sum "${base}/real/dogfoodguard" | cut -d' ' -f1)"
POL
	}
	e2e_reset() {
		: > "${base}/ledger/invocations.tsv"; : > "${base}/ledger/buzz.tsv"
		: > "${base}/ledger/spend.tsv";       : > "${base}/ledger/pubreq.allow"
		: > "${base}/ledger/workflow.allow"
		local f
		for f in observed_spend reserved generate_submits app_submits listing_mutations; do
			printf '0\n' > "${base}/ledger/${f}"
		done
		printf '10000\n' > "${base}/home/.cache/bal"
		printf '0\n' > "${base}/home/.cache/nbuzz"
		rm -f "${base}/ledger/meter_broken" "${base}/ledger/calibration.ok"
	}
	e2e_policy 0; e2e_reset

	local C="${base}/bin/civitai" rc allow lines i

	note "--- end-to-end through the real guard (stub charges 37) ---"

	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1; rc=$?
	st_check "e2e allowed spend returns the CLI's rc" "0"  "${rc}"
	st_check "e2e meter moved by the real charge"     "37" "$(cat "${base}/ledger/observed_spend")"

	"${C}" --no-color generate cat --yes >/dev/null 2>&1
	st_check "e2e --no-color generate refused"        "126" "$?"
	"${C}" generate cat --yes --print-input=false >/dev/null 2>&1
	st_check "e2e --print-input=false refused"        "126" "$?"
	"${C}" generate cat --yes --dry-run=false >/dev/null 2>&1
	st_check "e2e --dry-run=false refused"            "126" "$?"
	"${C}" generate cat --yes --negative-prompt --help >/dev/null 2>&1
	st_check "e2e --negative-prompt --help refused"   "126" "$?"
	"${C}" --no-update-check upgrade >/dev/null 2>&1
	st_check "e2e --no-update-check upgrade refused"  "126" "$?"
	"${C}" frobnicate >/dev/null 2>&1
	st_check "e2e an unrecognised command refused"    "126" "$?"
	"${C}" workflows cancel someone-elses >/dev/null 2>&1
	st_check "e2e cancel of a foreign workflow refused" "126" "$?"
	"${C}" generate cat --yes --max-cost 08 >/dev/null 2>&1; rc=$?
	st_check "e2e --max-cost 08 refuses cleanly"      "126" "${rc}"

	lines=$(wc -l < "${base}/ledger/invocations.tsv")
	"${C}" generate "$(printf 'a\nDENY\tinjected')" --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e a newline prompt adds ONE ledger row" "1" \
		"$(( $(wc -l < "${base}/ledger/invocations.tsv") - lines ))"
	st_check "e2e ALLOW rows carry a reason" "yes" \
		"$(awk -F'\t' '$2 ~ /^ALLOW/ && $6 != "" {n++} END{print (n>0)?"yes":"no"}' "${base}/ledger/invocations.tsv")"

	# The guard must refuse everything when its own policy will not validate.
	cp "${base}/ledger/policy.env" "${base}/ledger/policy.keep"
	grep -v '^TOTAL_SPEND_CAP=' "${base}/ledger/policy.keep" > "${base}/ledger/policy.env"
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e a malformed policy refuses even a read" "126" "$?"
	cp "${base}/ledger/policy.keep" "${base}/ledger/policy.env"
	rm -f "${base}/ledger/policy.keep"

	# Concurrency: leave exactly 10 Buzz of headroom and fire 8 at once.
	e2e_reset; printf '1990\n' > "${base}/ledger/observed_spend"
	for i in 1 2 3 4 5 6 7 8; do STUB_SLOW_BUZZ=1 "${C}" generate "p${i}" --yes --max-cost 10 >/dev/null 2>&1 & done
	wait
	allow=$(grep -c 'ALLOW_SPEND' "${base}/ledger/invocations.tsv" 2>/dev/null || printf 0)
	# NOT a strict count: see cmd_locktest — with the lock deleted this row
	# still passed on a third of runs, so it is kept only as a smoke test that
	# concurrent invocations do not corrupt the ledger. The mutual exclusion
	# itself is pinned deterministically below.
	st_check "e2e concurrent submits keep the counter consistent" \
		"${allow}" "$(cat "${base}/ledger/generate_submits")"

	note "--- 🔴 the ledger lock, pinned as a PRIMITIVE (deterministic) ---"
	rm -f "${base}/ledger/.locktest-held"
	bash "$(readlink -f "$0")" __locktest --root "${base}" hold 3 >/dev/null 2>&1 &
	local holder=$!
	local waited=0
	while [ ! -f "${base}/ledger/.locktest-held" ] && [ "${waited}" -lt 30 ]; do
		sleep 0.1; waited=$(( waited + 1 ))
	done
	st_check "e2e the holder took the lock" "yes" \
		"$([ -f "${base}/ledger/.locktest-held" ] && printf yes || printf no)"
	st_check "e2e a second process is BLOCKED while it is held" "blocked" \
		"$(bash "$(readlink -f "$0")" __locktest --root "${base}" try 2>/dev/null)"
	wait "${holder}" 2>/dev/null
	st_check "e2e the lock is free once released" "acquired" \
		"$(bash "$(readlink -f "$0")" __locktest --root "${base}" try 2>/dev/null)"
	rm -f "${base}/ledger/.locktest-held"

	note "--- 🔴 a charge that happens and THEN fails must still latch ---"
	e2e_reset
	STUB_POSTREAD_FAIL=1 STUB_FAIL_AFTER_CHARGE=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e rc!=0 + unreadable post-read LATCHES"  "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	st_check "e2e rc!=0 + unreadable charges worst case" "60" "$(cat "${base}/ledger/observed_spend")"
	e2e_reset
	STUB_CREDIT=1 STUB_FAIL_AFTER_CHARGE=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e rc!=0 + NEGATIVE delta LATCHES"        "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	e2e_reset
	STUB_FAIL_AFTER_CHARGE=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e rc!=0 with a REAL charge is metered"   "37" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e rc!=0 with a real charge does not latch" "no" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	# CONTROL: the benign case must still be suppressed.
	e2e_reset
	STUB_FAIL_RC=2 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e a failure that spent NOTHING is suppressed" "0" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e that suppression does not latch"           "no" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	note "--- 🔴 the meter file itself must be writable before a spend ---"
	e2e_reset
	chmod 0444 "${base}/ledger/observed_spend"
	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1; rc=$?
	chmod 0644 "${base}/ledger/observed_spend"
	st_check "e2e an unwritable METER refuses a spend" "126"   "${rc}"
	st_check "e2e that refusal spent nothing"          "10000" "$(cat "${base}/home/.cache/bal")"

	note "--- 🔴 a meter write that fails AFTER the charge must latch ---"
	e2e_reset
	STUB_LOCK_METER=1 "${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	chmod 0644 "${base}/ledger/observed_spend" 2>/dev/null
	st_check "e2e a post-charge meter write failure LATCHES" "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	note "--- 🔴 a live generate's crash record must not be consumed ---"
	e2e_reset
	STUB_SLOW_GEN=3 "${C}" generate slow --yes --max-cost 50 >/dev/null 2>&1 &
	local slow=$!
	sleep 1
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e a read mid-generate writes NO reconcile row" "0" \
		"$(reconciled_rows "${base}")"
	"${C}" generate second --yes --max-cost 10 >/dev/null 2>&1
	st_check "e2e a SECOND concurrent generate is refused" "126" "$?"
	wait "${slow}" 2>/dev/null
	st_check "e2e the live generate metered exactly once" "37" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e no reconcile row was ever written"      "0" \
		"$(reconciled_rows "${base}")"

	note "--- 🔴 the three ways the meter can lie, each must LATCH ---"

	e2e_reset
	STUB_FLAKY_BUZZ=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e a FAILED post-read charges the worst case" "60" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e a FAILED post-read latches"                "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	e2e_reset
	STUB_LAGGY=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e a ZERO delta charges the worst case"       "60" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e a ZERO delta latches"                      "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e spend refused after the latch"             "126" "$?"

	e2e_reset
	STUB_CREDIT=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e a NEGATIVE delta charges the worst case"   "60" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e a NEGATIVE delta latches"                  "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	note "--- 🔴 a truncated or unwritable ledger must not restart the caps ---"
	e2e_reset
	: > "${base}/ledger/generate_submits"
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e an EMPTIED counter refuses a spend" "126" "$?"
	e2e_reset
	chmod 0444 "${base}/ledger/generate_submits"
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1; rc=$?
	chmod 0644 "${base}/ledger/generate_submits"
	st_check "e2e an UNWRITABLE counter refuses a spend" "126" "${rc}"
	st_check "e2e nothing was charged in that attempt"   "10000" "$(cat "${base}/home/.cache/bal")"

	note "--- 🔴 the PRE-read fail-closed path, which had no coverage at all ---"
	e2e_reset
	# A balance read that fails BEFORE the submit must refuse and spend nothing.
	STUB_BUZZ_ALWAYS_FAILS=1 "${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e a failed PRE-read refuses"          "126"   "$?"
	st_check "e2e a failed PRE-read spent nothing"    "10000" "$(cat "${base}/home/.cache/bal")"
	st_check "e2e a failed PRE-read left the meter 0" "0"     "$(cat "${base}/ledger/observed_spend")"

	note "--- 🔴 a non-spending FAILURE must not latch or credit ---"
	e2e_reset
	STUB_FAIL_RC=2 "${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e a failed generate returns the CLI rc" "2" "$?"
	st_check "e2e a failed generate did not latch"      "no" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	st_check "e2e a failed generate credited nothing"   "0" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e a failed generate burned no submit"   "0" "$(cat "${base}/ledger/generate_submits")"
	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e the NEXT generate still works"        "0" "$?"

	note "--- 🔴 an interrupted spend is reconciled on the next invocation ---"
	e2e_reset
	printf '%s\t50\t10000\tgenerate interrupted\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "${base}/ledger/inflight"
	printf '50\n' > "${base}/ledger/reserved"
	printf '9963\n' > "${base}/home/.cache/bal"
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e the interrupted charge was reconciled" "37" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e the leaked reservation was released"   "0"  "$(cat "${base}/ledger/reserved")"
	st_check "e2e a RECONCILED row was written"          "1" \
		"$(grep -c 'RECONCILED' "${base}/ledger/invocations.tsv")"

	note "--- app/listing counters bump on SUCCESS only ---"
	e2e_reset
	mkdir -p "${base}/workspace/app"
	printf '{"blockId":"sanctioned-x","name":"n","version":"1.0.0","kind":"page"}\n' \
		> "${base}/workspace/app/block.manifest.json"
	STUB_APP_RC=1 "${C}" app submit "${base}/workspace/app" --yes >/dev/null 2>&1
	st_check "e2e a REJECTED submit burns no slot" "0" "$(cat "${base}/ledger/app_submits")"
	"${C}" app submit "${base}/workspace/app" --yes >/dev/null 2>&1
	st_check "e2e an ACCEPTED submit counts"       "1" "$(cat "${base}/ledger/app_submits")"

	note "--- 🔴 provenance is VERIFIED, not merely recorded ---"
	e2e_reset
	# 🔴 UNLINK BEFORE WRITING. Overwriting a binary that has just been executed
	# gives ETXTBSY and the write SILENTLY FAILS — measured, the substitution
	# never happened and this row passed against the REAL classifier, which is
	# the second time that trap has produced a green row about a swap that did
	# not occur. `rm` then create makes a new inode.
	rm -f "${base}/real/dogfoodguard"
	printf '#!/usr/bin/env bash\nprintf "ok\\t1\\npath\\twhoami\\nargc\\t0\\nhassubcommands\\tfalse\\nargserr\\t\\n"\n' \
		> "${base}/real/dogfoodguard"
	chmod 0755 "${base}/real/dogfoodguard"
	st_check "e2e the classifier really WAS substituted" "yes" \
		"$([ "$(head -c 2 "${base}/real/dogfoodguard")" = "#!" ] && printf yes || printf no)"
	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e a SUBSTITUTED classifier is refused" "126" "$?"
	st_check "e2e the substitution spent nothing"      "10000" "$(cat "${base}/home/.cache/bal")"
	rm -f "${base}/real/dogfoodguard"
	cp "${guard_src}" "${base}/real/dogfoodguard"; chmod 0755 "${base}/real/dogfoodguard"
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e the restored classifier works again" "0" "$?"

	note "--- 🔴 the in-flight record exists DURING the call ---"
	e2e_reset
	STUB_REPORT_INFLIGHT=1 "${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e the stub saw an inflight record mid-call" "yes" \
		"$(cat "${base}/home/.cache/saw-inflight" 2>/dev/null || printf no)"

	note "--- id recording: pubreq and workflow provenance ---"
	e2e_reset
	mkdir -p "${base}/workspace/app2"
	printf '{"blockId":"sanctioned-y","name":"n","version":"1.0.0","kind":"page"}\n' \
		> "${base}/workspace/app2/block.manifest.json"
	STUB_PUBREQ=1 "${C}" app submit "${base}/workspace/app2" --yes >/dev/null 2>&1
	st_check "e2e a submit records its OWN pubreq id" "1" \
		"$(grep -c 'pubreq_MINE' "${base}/ledger/pubreq.allow" 2>/dev/null || printf 0)"
	"${C}" app withdraw pubreq_MINE >/dev/null 2>&1
	st_check "e2e withdraw of the recorded id is allowed" "0" "$?"
	"${C}" app withdraw pubreq_SOMEONEELSE >/dev/null 2>&1
	st_check "e2e withdraw of a foreign id is refused"   "126" "$?"

	e2e_reset
	STUB_WORKFLOWS=1 "${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1
	st_check "e2e only the NEW workflow id is allowlisted" "wf-new" \
		"$(tr -d ' \n' < "${base}/ledger/workflow.allow" 2>/dev/null)"

	note "--- 🔴 the crash path settles by the SAME rule as the live path ---"
	# Each row drives reconcile_inflight through the shim by leaving a record
	# whose writer is gone, then asserts the meter AND the latch.
	stale_record() {  # $1 = mc, $2 = before-balance
		printf '%s\t%s\t%s\t999999\t stale\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$2" \
			> "${base}/ledger/inflight"
		rm -f "${base}/ledger/inflight.lock"
	}
	e2e_reset; printf '9963\n' > "${base}/home/.cache/bal"
	stale_record 50 10000
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e reconcile: a real delta is metered"   "37" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e reconcile: a real delta does not latch" "no" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	e2e_reset
	stale_record 50 10000   # balance unchanged => delta 0
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e reconcile: a ZERO delta charges the worst case" "50" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e reconcile: a ZERO delta LATCHES"                "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	e2e_reset; printf '10100\n' > "${base}/home/.cache/bal"
	stale_record 50 10000   # balance went UP
	"${C}" whoami >/dev/null 2>&1
	st_check "e2e reconcile: a NEGATIVE delta charges the worst case" "50" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e reconcile: a NEGATIVE delta LATCHES"                "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	e2e_reset; printf '9963\n' > "${base}/home/.cache/bal"
	stale_record 50 10000
	chmod 0444 "${base}/ledger/observed_spend"
	"${C}" whoami >/dev/null 2>&1
	chmod 0644 "${base}/ledger/observed_spend"
	st_check "e2e reconcile: an unwritable meter LATCHES" "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"

	note "--- 🔴 the audit trail must be writable before a spend ---"
	e2e_reset
	chmod 0444 "${base}/ledger/invocations.tsv"
	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1; rc=$?
	chmod 0644 "${base}/ledger/invocations.tsv"
	st_check "e2e an unwritable audit log refuses a spend" "126"   "${rc}"
	st_check "e2e that refusal spent nothing"              "10000" "$(cat "${base}/home/.cache/bal")"

	note "--- 🔴 the sandbox HOME holds NO credential ---"
	st_check "e2e no civitai config on disk" "no" \
		"$([ -f "${base}/home/.config/civitai/config.yaml" ] && printf yes || printf no)"

	note "--- 🔴 concurrency OUTCOME, not just ledger consistency ---"
	e2e_reset; printf '1990\n' > "${base}/ledger/observed_spend"
	for i in 1 2 3 4 5 6 7 8; do STUB_SLOW_BUZZ=1 "${C}" generate "c${i}" --yes --max-cost 10 >/dev/null 2>&1 & done
	wait
	st_check "e2e 8 racers, 10 Buzz headroom => exactly 1 allowed" "1" \
		"$(grep -c ALLOW_SPEND "${base}/ledger/invocations.tsv" 2>/dev/null || true)"
	st_check "e2e 8 racers write no reconcile rows" "0" "$(reconciled_rows "${base}")"

	note "--- calibration gate, end to end ---"
	e2e_policy 1; e2e_reset
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e uncalibrated refuses to spend" "126" "$?"
	printf 'calibrated\n' > "${base}/ledger/calibration.ok"
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e calibrated may spend"          "0" "$?"

	rm -rf "${base}"
}

usage() {
	sed -n '3,25p' "$0"
	exit 2
}

main() {
	local sub="${1:-}"
	[ $# -gt 0 ] && shift
	case "${sub}" in
		init)          cmd_init "$@" ;;
		calibrate)     cmd_calibrate "$@" ;;
		guard)         cmd_guard "$@" ;;
		status)        cmd_status "$@" ;;
		finish)        cmd_finish "$@" ;;
		selftest)      cmd_selftest "$@" ;;
		enter)         cmd_enter "$@" ;;
		allow-pubreq)  cmd_allow_pubreq "$@" ;;
		teardown)      cmd_teardown "$@" ;;
		__buzz)        cmd_buzz_probe "$@" ;;
		__verdict)     cmd_verdict "$@" ;;
		__locktest)    cmd_locktest "$@" ;;
		*)             usage ;;
	esac
}

main "$@"
