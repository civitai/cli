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
DEFAULT_SLUG_PREFIX=""
DEFAULT_BASE_URL="https://civitai.com"

# ---------------------------------------------------------------------------
# Closed vocabularies. An unrecognised command is DENIED, not passed through:
# a new subcommand may spend or mutate, and the gate cannot know that it does
# not.
# ---------------------------------------------------------------------------
TOPLEVEL_CMDS="app articles buzz collections completion creators download generate help images login model-versions models tags upgrade users version whoami workflows"
APP_SUBCMDS="create init validate submit withdraw status list view pull metrics listing dev-token dev-tunnel"
LISTING_SUBCMDS="status set-icon set-cover add-screenshot rm-screenshot reorder"
WORKFLOWS_SUBCMDS="list get cancel"

# Root persistent flags. All BOOLEAN — Cobra strips them anywhere they appear
# and they never consume the following token.
ROOT_BOOL_FLAGS="--color --no-color --no-update-check --help -h --version -v"

# Boolean flags of the gated commands. Used to decide whether a `--help` is a
# real help request or the VALUE of a preceding value-taking flag.
CMD_BOOL_FLAGS="--yes -y --dry-run --print-input --json --force --no-wait --no-download --fail-on-substitution --package-only --skip-validate --strict --no-browser --package-only"

# Flags that CONSUME the next token, per command path.
VALUE_FLAGS_GENERATE="--checkpoint --lora --image --input --negative-prompt --aspect-ratio --quantity --out-dir --timeout --max-cost --external-id --ecosystem"
VALUE_FLAGS_APP_SUBMIT="-o --out"
VALUE_FLAGS_APP_LISTING="--slug --dir --caption --changelog"
VALUE_FLAGS_APP_WITHDRAW="--id"

err()  { printf 'dogfood-sandbox: %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }
note() { printf '%s\n' "$*"; }

SANDBOX_TAG="SANDBOX POLICY"
now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }

in_list() {
	local needle="$1" hay="$2" x
	for x in ${hay}; do [ "${x}" = "${needle}" ] && return 0; done
	return 1
}

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
	unset PER_CALL_MAX_COST TOTAL_SPEND_CAP MAX_GENERATE_SUBMITS MAX_APP_SUBMITS SLUG_PREFIX BASE_URL
	# shellcheck disable=SC1090
	. "${f}" || { POLICY_ERROR="policy file ${f} could not be read"; return 1; }
	for k in PER_CALL_MAX_COST TOTAL_SPEND_CAP MAX_GENERATE_SUBMITS MAX_APP_SUBMITS; do
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

# A ledger counter. A missing file is 0; a MALFORMED one is an error, never 0.
read_scalar() {
	local f="${ROOT}/ledger/$1" v
	[ -f "${f}" ] || { printf '0'; return 0; }
	v=$(tr -d ' \n\r\t' < "${f}")
	[ -n "${v}" ] || { printf '0'; return 0; }
	case "${v}" in
		''|*[!0-9]*) return 1 ;;
	esac
	printf '%s' "${v}"
}

write_scalar() { printf '%s\n' "$2" > "${ROOT}/ledger/$1"; }

# ---------------------------------------------------------------------------
# Running the real binary
#
# The environment is PINNED, not inherited: the agent controls the shim's
# environment, and CIVITAI_BASE_URL would otherwise redirect the very balance
# read the spend meter uses as its oracle. CIVITAI_TOKEN is cleared so the
# seeded config file is the only credential.
# ---------------------------------------------------------------------------
sandbox_env() {
	printf '%s\0' \
		"HOME=${ROOT}/home" \
		"XDG_CONFIG_HOME=${ROOT}/home/.config" \
		"XDG_CACHE_HOME=${ROOT}/home/.cache" \
		"CIVITAI_NO_UPDATE_CHECK=1" \
		"CIVITAI_BASE_URL=${BASE_URL:-${DEFAULT_BASE_URL}}"
}

real_cli() {
	env -u CIVITAI_TOKEN -u CIVITAI_SUBMIT_PATH -u CIVITAI_ANTIPATTERN_SCAN_DIR \
		-u CIVITAI_CHECK_SUBMISSIONS_CAP -u CIVITAI_CHECK_PUBLISHED_PINS \
		"HOME=${ROOT}/home" \
		"XDG_CONFIG_HOME=${ROOT}/home/.config" \
		"XDG_CACHE_HOME=${ROOT}/home/.cache" \
		"CIVITAI_NO_UPDATE_CHECK=1" \
		"CIVITAI_BASE_URL=${BASE_URL:-${DEFAULT_BASE_URL}}" \
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
lock_ledger() {
	command -v flock >/dev/null 2>&1 || return 0   # degraded; reported by selftest
	exec 9>"${ROOT}/ledger/.lock" || return 1
	flock -x -w 60 9 || return 1
	LOCK_HELD=1
	return 0
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

	while [ $# -gt 0 ]; do
		case "$1" in
			--root)            root="$2"; shift 2 ;;
			--binary)          binary="$2"; shift 2 ;;
			--token-file)      token_file="$2"; shift 2 ;;
			--per-call-max)    per_call="$2"; shift 2 ;;
			--total-cap)       total_cap="$2"; shift 2 ;;
			--max-generates)   max_gen="$2"; shift 2 ;;
			--max-app-submits) max_app="$2"; shift 2 ;;
			--slug-prefix)     slug_prefix="$2"; shift 2 ;;
			--base-url)        base_url="$2"; shift 2 ;;
			--force)           force=1; shift ;;
			*) die "init: unknown flag $1" ;;
		esac
	done

	ROOT="${root}"
	[ -n "${binary}" ] || die "init: --binary <path to built civitai> is required"
	[ -x "${binary}" ] || die "init: ${binary} is not an executable file"

	if [ -e "${ROOT}" ] && [ "${force}" != 1 ]; then
		die "init: ${ROOT} already exists (use --force to supersede it, or \`teardown\` first)"
	fi
	if [ -e "${ROOT}" ]; then
		# 🔴 SUPERSEDE, NEVER DELETE. The ledger is the audit artefact of the
		# previous run; a mistyped `init --force` used to destroy it before the
		# new credential had even been validated.
		local superseded
		superseded="${ROOT}.superseded-$(date -u +%Y%m%dT%H%M%SZ)"
		chmod -R u+w "${ROOT}" 2>/dev/null
		mv "${ROOT}" "${superseded}" || die "init: could not set the previous run aside"
		note "init: previous run preserved at ${superseded}"
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

	[ -n "${slug_prefix}" ] || slug_prefix="dogfood3-$(date -u +%Y%m%d)-"

	mkdir -p "${ROOT}/real" "${ROOT}/bin" "${ROOT}/harness" \
	         "${ROOT}/home/.config" "${ROOT}/home/.cache" \
	         "${ROOT}/workspace" "${ROOT}/ledger" || die "init: mkdir failed"
	chmod 0700 "${ROOT}" "${ROOT}/home" "${ROOT}/ledger"

	cp "${binary}" "${ROOT}/real/civitai" || die "init: could not copy the binary"

	local self
	self=$(readlink -f "$0")
	cp "${self}" "${ROOT}/harness/dogfood-sandbox.sh" || die "init: could not copy the harness"
	chmod 0555 "${ROOT}/harness/dogfood-sandbox.sh"

	cat > "${ROOT}/bin/civitai" <<SHIM
#!/usr/bin/env bash
exec "${ROOT}/harness/dogfood-sandbox.sh" guard --root "${ROOT}" -- "\$@"
SHIM
	chmod 0555 "${ROOT}/bin/civitai"

	: > "${ROOT}/ledger/invocations.tsv"
	: > "${ROOT}/ledger/buzz.tsv"
	: > "${ROOT}/ledger/spend.tsv"
	: > "${ROOT}/ledger/pubreq.allow"
	write_scalar generate_submits 0
	write_scalar app_submits 0
	write_scalar observed_spend 0
	write_scalar reserved 0
	rm -f "${ROOT}/ledger/meter_broken"

	cat > "${ROOT}/ledger/policy.env" <<POLICY
# Written by dogfood-sandbox.sh init on $(now_iso). Read and VALIDATED by every
# later call — a missing or malformed key is a refusal, not a default.
ROOT="${ROOT}"
PER_CALL_MAX_COST=${per_call}
TOTAL_SPEND_CAP=${total_cap}
MAX_GENERATE_SUBMITS=${max_gen}
MAX_APP_SUBMITS=${max_app}
SLUG_PREFIX="${slug_prefix}"
BASE_URL="${base_url}"
BINARY_SOURCE="${binary}"
BINARY_SHA256="$(sha256sum "${binary}" | cut -d' ' -f1)"
HARNESS_SHA256="$(sha256sum "${self}" | cut -d' ' -f1)"
POLICY
	chmod 0600 "${ROOT}/ledger/policy.env"
	load_policy || die "init: the policy just written does not validate: ${POLICY_ERROR}"

	real_cli login --token "${token}" >/dev/null 2>&1 \
		|| die "init: seeding the credential failed — check the token"
	token=""
	[ -f "${ROOT}/home/.config/civitai/config.yaml" ] \
		|| die "init: no config written at ${ROOT}/home/.config/civitai/config.yaml"
	chmod 0600 "${ROOT}/home/.config/civitai/config.yaml"

	real_cli whoami >/dev/null 2>&1 || die "init: whoami failed with the seeded credential"

	local start
	start=$(sample_balance "start") || die "init: could not read the opening Buzz balance — refusing to start a run whose meter does not work"
	write_scalar start_balance "${start}"

	chmod 0555 "${ROOT}/real/civitai"
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
# ARGV TOKENIZER
#
# Fills TOK_KIND[i] for each argv token: "flag", "value", or "positional".
# Value-taking flags are resolved per command path. An UNRECOGNISED flag on a
# spending command is treated as value-taking, which biases the classification
# toward DENY rather than toward an unmetered submit.
# ---------------------------------------------------------------------------
declare -a TOK TOK_KIND
CMD1=""; CMD2=""; CMD3=""
# The token that SHOULD have been a subcommand but is not in the vocabulary.
# Without this an unknown verb resolved to the empty string and was
# indistinguishable from a bare `civitai app`, which the gate allows as a usage
# request — so `app frobnicate` and `app listing frobnicate` were ALLOWED.
CMD2_UNKNOWN=""; CMD3_UNKNOWN=""
IDX1=-1; IDX2=-1; IDX3=-1

value_flags_for_path() {
	case "${CMD1}:${CMD2}" in
		generate:*)     printf '%s' "${VALUE_FLAGS_GENERATE}" ;;
		app:submit)     printf '%s' "${VALUE_FLAGS_APP_SUBMIT}" ;;
		app:listing)    printf '%s' "${VALUE_FLAGS_APP_LISTING}" ;;
		app:withdraw)   printf '%s' "${VALUE_FLAGS_APP_WITHDRAW}" ;;
		*)              printf '%s' "" ;;
	esac
}

# Resolve the command path against the closed vocabulary. The FIRST token that
# is not a flag is the command; root persistent bools never consume a value, so
# skipping every leading `-*` is correct here and is what fixes the
# `--no-color generate` bypass.
# The first token after index `from` that does not start with '-'. Between a
# parent command and its subcommand Cobra permits only the parent's own flags,
# and none of `civitai`'s parent commands has a value-taking flag — so "first
# token not starting with -" is the correct rule here and needs no knowledge of
# which flags consume a value.
next_word_after() {
	local from="$1" i n
	n=${#TOK[@]}
	for (( i=from+1; i<n; i++ )); do
		case "${TOK[i]}" in -*) continue ;; esac
		printf '%s\t%s' "${TOK[i]}" "${i}"; return 0
	done
	return 1
}

resolve_command() {
	CMD1=""; CMD2=""; CMD3=""; CMD2_UNKNOWN=""; CMD3_UNKNOWN=""
	IDX1=-1; IDX2=-1; IDX3=-1
	local i n t pair word idx
	n=${#TOK[@]}
	for (( i=0; i<n; i++ )); do
		t="${TOK[i]}"
		case "${t}" in -*) continue ;; esac
		CMD1="${t}"; IDX1=${i}; break
	done
	[ -n "${CMD1}" ] || return 0

	if [ "${CMD1}" = "app" ]; then
		if pair=$(next_word_after "${IDX1}"); then
			word="${pair%%$'\t'*}"; idx="${pair##*$'\t'}"
			if in_list "${word}" "${APP_SUBCMDS}"; then
				CMD2="${word}"; IDX2="${idx}"
			else
				CMD2_UNKNOWN="${word}"; return 0
			fi
		fi
		if [ "${CMD2}" = "listing" ]; then
			if pair=$(next_word_after "${IDX2}"); then
				word="${pair%%$'\t'*}"; idx="${pair##*$'\t'}"
				if in_list "${word}" "${LISTING_SUBCMDS}"; then
					CMD3="${word}"; IDX3="${idx}"
				else
					CMD3_UNKNOWN="${word}"
				fi
			fi
		fi
	elif [ "${CMD1}" = "workflows" ]; then
		if pair=$(next_word_after "${IDX1}"); then
			word="${pair%%$'\t'*}"; idx="${pair##*$'\t'}"
			if in_list "${word}" "${WORKFLOWS_SUBCMDS}"; then
				CMD2="${word}"; IDX2="${idx}"
			else
				CMD2_UNKNOWN="${word}"
			fi
		fi
	fi
	return 0
}

tokenize() {
	TOK=("$@")
	TOK_KIND=()
	resolve_command
	local vflags spending=0
	vflags=$(value_flags_for_path)
	[ "${CMD1}" = "generate" ] && spending=1
	local i n t expect_value=0
	n=${#TOK[@]}
	for (( i=0; i<n; i++ )); do
		t="${TOK[i]}"
		if [ "${expect_value}" = 1 ]; then
			TOK_KIND[i]="value"; expect_value=0; continue
		fi
		case "${t}" in
			--*=*)  TOK_KIND[i]="flag" ;;
			-*)
				TOK_KIND[i]="flag"
				if in_list "${t}" "${vflags}"; then
					expect_value=1
				elif in_list "${t}" "${ROOT_BOOL_FLAGS}" || in_list "${t}" "${CMD_BOOL_FLAGS}"; then
					: # boolean, consumes nothing
				elif [ "${spending}" = 1 ]; then
					# Unknown flag on the money path: assume it eats the next
					# token, so a following --dry-run/--help is NOT mistaken for
					# a free preview or a help request.
					expect_value=1
				fi
				;;
			*)      TOK_KIND[i]="positional" ;;
		esac
	done
}

# Is FLAG present as a FLAG (never as another flag's value)?
tok_has_flag() {
	local want="$1" i n
	n=${#TOK[@]}
	for (( i=0; i<n; i++ )); do
		[ "${TOK_KIND[i]}" = "flag" ] || continue
		case "${TOK[i]}" in
			"${want}"|"${want}"=*) return 0 ;;
		esac
	done
	return 1
}

# Value of FLAG. Returns the LAST occurrence, because pflag does — measured on
# a free read, `--limit 1 --limit 3` returns three results. Reading the first
# let `--max-cost 5 --max-cost 99999` show the gate a 5 while the CLI enforced
# 99999.
tok_flag_value() {
	local want="$1" i n found=1 out=""
	n=${#TOK[@]}
	for (( i=0; i<n; i++ )); do
		[ "${TOK_KIND[i]}" = "flag" ] || continue
		case "${TOK[i]}" in
			"${want}"=*) out="${TOK[i]#*=}"; found=0 ;;
			"${want}")
				if [ $(( i + 1 )) -lt "${n}" ] && [ "${TOK_KIND[i+1]}" = "value" ]; then
					out="${TOK[i+1]}"; found=0
				fi ;;
		esac
	done
	[ "${found}" = 0 ] || return 1
	printf '%s' "${out}"
}

# Positional arguments AFTER the resolved command path — i.e. every positional
# token whose index is past the last command word. Indices come from
# resolve_command, so this needs no re-matching by string value (which used to
# mis-skip an argument that happened to equal its own command's name).
tok_positionals_after_command() {
	local i n last
	n=${#TOK[@]}
	last="${IDX1}"
	[ "${IDX2}" -gt "${last}" ] 2>/dev/null && last="${IDX2}"
	[ "${IDX3}" -gt "${last}" ] 2>/dev/null && last="${IDX3}"
	for (( i=last+1; i<n; i++ )); do
		[ "${TOK_KIND[i]}" = "positional" ] || continue
		printf '%s\n' "${TOK[i]}"
	done
}

# Is there a genuine --help / -h?
#
# 🔴 ALLOWLIST, NOT DENYLIST. A `--help` counts only when the token before it
# is nothing, a command word, or a KNOWN boolean flag. `--negative-prompt
# --help` is pflag taking `--help` as the prompt VALUE, and the old blanket
# "is --help anywhere in argv" test turned that into a full bypass of every
# DENY branch — measured: a real submit, charged, meter untouched.
tok_is_help_request() {
	local i n prev
	n=${#TOK[@]}
	for (( i=0; i<n; i++ )); do
		case "${TOK[i]}" in
			--help|-h|--help=*) ;;
			*) continue ;;
		esac
		[ "${TOK_KIND[i]}" = "flag" ] || continue
		if [ "${i}" = 0 ]; then return 0; fi
		prev="${TOK[i-1]}"
		if in_list "${prev}" "${ROOT_BOOL_FLAGS}" \
			|| in_list "${prev}" "${CMD_BOOL_FLAGS}" \
			|| in_list "${prev}" "${TOPLEVEL_CMDS}" \
			|| in_list "${prev}" "${APP_SUBCMDS}" \
			|| in_list "${prev}" "${LISTING_SUBCMDS}"; then
			return 0
		fi
	done
	return 1
}

resolve_block_id() {
	local dir="$1" slug="$2" manifest id
	if [ -n "${slug}" ]; then printf '%s' "${slug}"; return 0; fi
	manifest="${dir%/}/block.manifest.json"
	[ -r "${manifest}" ] || return 1
	id=$(tr -d '\n' < "${manifest}" \
		| grep -o '"blockId"[[:space:]]*:[[:space:]]*"[^"]*"' \
		| head -1 | sed 's/.*"blockId"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
	[ -n "${id}" ] || return 1
	printf '%s' "${id}"
}

# ---------------------------------------------------------------------------
# policy_verdict — sets VERDICT / VERDICT_REASON / VERDICT_MC.
#
# It deliberately does NOT run in a subshell: an internal failure under `set -u`
# must kill the whole gate (so nothing runs) rather than return an empty string
# the caller reads as "not DENY".
# ---------------------------------------------------------------------------
VERDICT=""; VERDICT_REASON=""; VERDICT_MC=0

deny() { VERDICT="DENY"; VERDICT_REASON="$1"; return 0; }
allow() { VERDICT="ALLOW"; VERDICT_REASON="${1:-}"; return 0; }

policy_verdict() {
	VERDICT=""; VERDICT_REASON=""; VERDICT_MC=0
	tokenize "$@"

	if tok_is_help_request; then allow "help"; return 0; fi

	if [ -z "${CMD1}" ]; then allow "root usage"; return 0; fi

	if ! in_list "${CMD1}" "${TOPLEVEL_CMDS}"; then
		deny "\`${CMD1}\` is not a command this gate recognises; it fails closed rather than guess whether a new command spends or mutates"
		return 0
	fi

	case "${CMD1}" in
		upgrade) deny "\`upgrade\` would replace the binary under evaluation mid-run"; return 0 ;;
		login)   deny "this sandbox is already authenticated; run \`civitai whoami\` to see as whom"; return 0 ;;
	esac

	if [ "${CMD1}" = "app" ]; then
		if [ -n "${CMD2_UNKNOWN}" ]; then
			deny "unrecognised \`app ${CMD2_UNKNOWN}\`; the gate fails closed rather than guess whether it mutates"
			return 0
		fi
		if [ -z "${CMD2}" ]; then allow "app usage"; return 0; fi
		case "${CMD2}" in
			dev-tunnel|dev-token)
				deny "\`app ${CMD2}\` is out of scope for this run"; return 0 ;;
			withdraw)
				local id=""
				id=$(tok_flag_value "--id") || id=""
				if [ -z "${id}" ]; then
					id=$(tok_positionals_after_command | head -1)
				fi
				if [ -z "${id}" ]; then
					allow "withdraw with no id (a usage error the CLI reports)"; return 0
				fi
				# 🔴 ANY id, not just a pubreq_-shaped one. app_withdraw.go takes
				# args[0] with no format validation, so the old "the CLI will
				# refuse it" fallback was a false statement about a branch that
				# reaches the network.
				if grep -qxF "${id}" "${ROOT}/ledger/pubreq.allow" 2>/dev/null; then
					allow "withdraw of a submission this sandbox created"; return 0
				fi
				deny "\`${id}\` was not created by this sandbox, so it may be one of the account's real pending submissions"
				return 0 ;;
			submit)
				if tok_has_flag "--package-only"; then
					allow "--package-only never submits"; return 0
				fi
				local dir id n
				dir=$(tok_positionals_after_command | head -1)
				[ -n "${dir}" ] || dir="."
				if ! id=$(resolve_block_id "${dir}" ""); then
					deny "cannot read a blockId from ${dir}/block.manifest.json, so the app being submitted cannot be identified"
					return 0
				fi
				case "${id}" in
					"${SLUG_PREFIX}"*) ;;
					*) deny "blockId \`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may only submit its own throwaway app"; return 0 ;;
				esac
				n=$(read_scalar app_submits) || { deny "the app-submission counter is unreadable"; return 0; }
				if [ "${n}" -ge "${MAX_APP_SUBMITS}" ]; then
					deny "the app-submission cap for this run (${MAX_APP_SUBMITS}) is already used"; return 0
				fi
				VERDICT="ALLOW_APP_SUBMIT"; VERDICT_REASON="submitting ${id}"; return 0 ;;
			listing)
				if [ -n "${CMD3_UNKNOWN}" ]; then
					deny "unrecognised \`app listing ${CMD3_UNKNOWN}\`; the gate fails closed on anything that might mutate a listing"
					return 0
				fi
				case "${CMD3}" in
					"") allow "listing usage"; return 0 ;;
					status) allow "listing read"; return 0 ;;
					set-icon|set-cover|add-screenshot|rm-screenshot|reorder)
						local slug dir id
						slug=$(tok_flag_value "--slug") || slug=""
						dir=$(tok_flag_value "--dir") || dir=""
						[ -n "${dir}" ] || dir="."
						if ! id=$(resolve_block_id "${dir}" "${slug}"); then
							deny "cannot identify which app \`app listing ${CMD3}\` would mutate (no --slug, and no readable blockId in ${dir}/block.manifest.json)"
							return 0
						fi
						case "${id}" in
							"${SLUG_PREFIX}"*) allow "listing mutation on ${id}"; return 0 ;;
							*) deny "\`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may not touch the account's real listings"; return 0 ;;
						esac ;;
					*) deny "unrecognised \`app listing ${CMD3}\`; the gate fails closed on anything that might mutate a listing"; return 0 ;;
				esac ;;
			create|init|validate|status|list|view|pull|metrics)
				allow "app read/local"; return 0 ;;
			*)
				deny "unrecognised \`app ${CMD2}\`; the gate fails closed"; return 0 ;;
		esac
	fi

	if [ "${CMD1}" = "generate" ]; then
		if tok_has_flag "--print-input" || tok_has_flag "--dry-run"; then
			allow "generate preview (spends nothing)"; return 0
		fi
		if [ -f "${ROOT}/ledger/meter_broken" ]; then
			deny "the spend meter could not read a balance earlier in this run, so spend is no longer being counted — refusing to submit (see ledger/meter_broken)"
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
		n=$(read_scalar generate_submits) || { deny "the submission counter is unreadable"; return 0; }
		if [ "${n}" -ge "${MAX_GENERATE_SUBMITS}" ]; then
			deny "the generation cap for this run (${MAX_GENERATE_SUBMITS} submits) is already used"; return 0
		fi
		if ! mc=$(tok_flag_value "--max-cost"); then
			deny "a generation that actually submits must carry --max-cost (at most ${PER_CALL_MAX_COST}); use --dry-run to price it first"
			return 0
		fi
		case "${mc}" in
			''|*[!0-9]*) deny "--max-cost must be a whole number of Buzz, got \`${mc}\`"; return 0 ;;
		esac
		if [ "${mc}" -gt "${PER_CALL_MAX_COST}" ]; then
			deny "--max-cost ${mc} exceeds this run's per-invocation ceiling of ${PER_CALL_MAX_COST} Buzz"; return 0
		fi
		remaining=$(( TOTAL_SPEND_CAP - committed ))
		if [ "${mc}" -gt "${remaining}" ]; then
			deny "--max-cost ${mc} exceeds the ${remaining} Buzz left under this run's total cap"; return 0
		fi
		VERDICT="ALLOW_SPEND"; VERDICT_REASON="submit with --max-cost ${mc}"; VERDICT_MC="${mc}"
		return 0
	fi

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

	lock_ledger || refuse "could not take the ledger lock within 60s" "${scrubbed}"

	policy_verdict "${argv[@]+"${argv[@]}"}"

	# 🔴 An empty or unrecognised verdict is a DENY. This is the backstop for
	# any internal failure that leaves VERDICT unset.
	case "${VERDICT}" in
		ALLOW|ALLOW_SPEND|ALLOW_APP_SUBMIT|DENY) ;;
		*) refuse "the gate could not classify this invocation (internal error) — refusing" "${scrubbed}" ;;
	esac

	if [ "${VERDICT}" = "DENY" ]; then
		refuse "${VERDICT_REASON}" "${scrubbed}"
	fi

	local before="" mc="${VERDICT_MC}"
	if [ "${VERDICT}" = "ALLOW_SPEND" ]; then
		before=$(sample_balance "pre-generate") || \
			refuse "could not read the Buzz balance, so the spend meter cannot be kept — refusing to submit" "${scrubbed}"
		local res n
		res=$(read_scalar reserved) || refuse "the reservation ledger is unreadable" "${scrubbed}"
		n=$(read_scalar generate_submits) || refuse "the submission counter is unreadable" "${scrubbed}"
		write_scalar reserved "$(( res + mc ))"
		write_scalar generate_submits "$(( n + 1 ))"
	elif [ "${VERDICT}" = "ALLOW_APP_SUBMIT" ]; then
		local a
		a=$(read_scalar app_submits) || refuse "the app-submission counter is unreadable" "${scrubbed}"
		write_scalar app_submits "$(( a + 1 ))"
	fi

	unlock_ledger

	local t0 t1 rc
	t0=$(date +%s%N)
	env -u CIVITAI_TOKEN -u CIVITAI_SUBMIT_PATH -u CIVITAI_ANTIPATTERN_SCAN_DIR \
		-u CIVITAI_CHECK_SUBMISSIONS_CAP -u CIVITAI_CHECK_PUBLISHED_PINS \
		"HOME=${ROOT}/home" \
		"XDG_CONFIG_HOME=${ROOT}/home/.config" \
		"XDG_CACHE_HOME=${ROOT}/home/.cache" \
		"CIVITAI_NO_UPDATE_CHECK=1" \
		"CIVITAI_BASE_URL=${BASE_URL}" \
		"${ROOT}/real/civitai" "${argv[@]+"${argv[@]}"}"
	rc=$?
	t1=$(date +%s%N)

	local delta=""
	if [ "${VERDICT}" = "ALLOW_SPEND" ]; then
		local after res cum
		lock_ledger || err "could not re-take the ledger lock; the spend ledger may be short one entry"
		after=$(sample_balance "post-generate") || after=""
		res=$(read_scalar reserved) || res="${mc}"
		cum=$(read_scalar observed_spend) || cum=0
		if [ -n "${after}" ]; then
			delta=$(( before - after ))
			[ "${delta}" -ge 0 ] || delta=0
			write_scalar observed_spend "$(( cum + delta ))"
			printf '%s\tgenerate\t%s\t%s\t%s\n' "$(now_iso)" "${before}" "${after}" "${delta}" \
				>> "${ROOT}/ledger/spend.tsv"
		else
			# 🔴 FAIL CLOSED ON A LOST SAMPLE. Dropping the delta silently let a
			# flaky balance read under-count forever — measured: 5 submits,
			# 185 truly spent, meter 0. Charge the worst case the estimate
			# allowed and stop the money path for the rest of the run.
			delta="unknown"
			write_scalar observed_spend "$(( cum + mc ))"
			printf '%s\tgenerate\t%s\tUNREADABLE\tassumed-%s\n' "$(now_iso)" "${before}" "${mc}" \
				>> "${ROOT}/ledger/spend.tsv"
			: > "${ROOT}/ledger/meter_broken"
			printf '%s: %s\n' "${SANDBOX_TAG}" \
				"the post-submit balance read failed; ${mc} Buzz has been charged to the ledger as a worst case and no further generation will be allowed this run" >&2
		fi
		write_scalar reserved "$(( res - mc < 0 ? 0 : res - mc ))"
		unlock_ledger
	fi

	log_invocation "${VERDICT}" "${rc}" "$(( (t1 - t0) / 1000000 ))" "${scrubbed}" "${delta:-}"
	return "${rc}"
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
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) break ;; esac
	done
	ROOT="${root}"; load_policy || die "${POLICY_ERROR}"
	command -v bwrap >/dev/null 2>&1 || die "enter: bwrap (bubblewrap) is not on PATH"

	local shell_bin="${SHELL:-/bin/sh}"
	[ $# -gt 0 ] || set -- "${shell_bin}" -i

	# 🔴 ORDER IS LOAD-BEARING. bwrap applies these in sequence, so `--tmpfs
	# /tmp` must come BEFORE the run-root bind: the default run root lives under
	# /tmp, and a tmpfs mounted afterwards SHADOWS it. Measured — with the bind
	# first, bwrap died with "Can't chdir to <root>/workspace: No such file or
	# directory". PATH is set explicitly rather than inherited because the
	# operator's ~/.nix-profile/bin does not exist inside the jail.
	exec bwrap \
		--ro-bind /nix /nix \
		--ro-bind /run/current-system /run/current-system \
		--ro-bind /etc/resolv.conf /etc/resolv.conf \
		--ro-bind /etc/ssl /etc/ssl \
		--ro-bind /etc/static /etc/static \
		--ro-bind /usr /usr \
		--proc /proc --dev /dev --tmpfs /tmp \
		--bind "${ROOT}" "${ROOT}" \
		--unshare-pid --die-with-parent \
		--setenv HOME "${ROOT}/home" \
		--setenv XDG_CONFIG_HOME "${ROOT}/home/.config" \
		--setenv XDG_CACHE_HOME "${ROOT}/home/.cache" \
		--setenv CIVITAI_NO_UPDATE_CHECK 1 \
		--setenv PATH "${ROOT}/bin:/run/current-system/sw/bin:/usr/bin:/bin" \
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
	[ -e "${root}" ] || { [ "${quiet}" = 1 ] || note "teardown: ${root} does not exist"; return 0; }
	chmod -R u+w "${root}" 2>/dev/null
	if [ "${delete}" = 1 ]; then
		rm -rf "${root}"
		[ "${quiet}" = 1 ] || note "teardown: deleted ${root}"
	else
		[ "${quiet}" = 1 ] || note "teardown: unlocked ${root} (not deleted; pass --delete to remove)"
	fi
}

# ---------------------------------------------------------------------------
# selftest
# ---------------------------------------------------------------------------
SELFTEST_PASS=0
SELFTEST_FAIL=0

st_check() {
	local label="$1" want="$2" got="$3"
	if [ "${want}" = "${got}" ]; then
		printf '  PASS  %-58s (%s)\n' "${label}" "${got}"; SELFTEST_PASS=$(( SELFTEST_PASS + 1 ))
	else
		printf '  FAIL  %-58s want=%s got=%s\n' "${label}" "${want}" "${got}"; SELFTEST_FAIL=$(( SELFTEST_FAIL + 1 ))
	fi
}

vd() { policy_verdict "$@"; printf '%s' "${VERDICT}"; }

# 🔴 A DENY is not evidence that the branch you meant was reached. Measured:
# with the command resolved positionally again, `--no-color generate … --yes`
# still DENIED — as an unrecognised command called `--no-color` — so every
# verdict-only assertion for that bypass stayed green while the gate was blind.
# These rows pin the REASON, so a refusal from a bystander branch fails.
st_reason() {
	local label="$1" needle="$2"; shift 2
	policy_verdict "$@"
	case "${VERDICT_REASON}" in
		*"${needle}"*) st_check "${label}" "match" "match" ;;
		*)             st_check "${label}" "match" "no-match: ${VERDICT_REASON}" ;;
	esac
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
		rm -rf "${root}"; mkdir -p "${root}/ledger" "${root}/workspace"
		cat > "${root}/ledger/policy.env" <<OFFLINE
ROOT="${root}"
PER_CALL_MAX_COST=100
TOTAL_SPEND_CAP=2000
MAX_GENERATE_SUBMITS=20
MAX_APP_SUBMITS=3
SLUG_PREFIX="sanctioned-"
BASE_URL="https://civitai.com"
OFFLINE
		: > "${root}/ledger/pubreq.allow"
		printf '0\n' > "${root}/ledger/observed_spend"
		printf '0\n' > "${root}/ledger/reserved"
		printf '0\n' > "${root}/ledger/generate_submits"
		printf '0\n' > "${root}/ledger/app_submits"
	fi

	ROOT="${root}"
	load_policy || die "selftest: ${POLICY_ERROR}"

	local td="${ROOT}/ledger/_selftest"
	rm -rf "${td}"; mkdir -p "${td}/good" "${td}/bad"
	printf '{"blockId":"%sdemo","name":"d","version":"1.0.0","kind":"page"}\n' "${SLUG_PREFIX}" > "${td}/good/block.manifest.json"
	printf '{"blockId":"someones-real-app","name":"d","version":"1.0.0","kind":"page"}\n'        > "${td}/bad/block.manifest.json"

	note "--- forbidden commands ---"
	st_check "upgrade denied"                      "DENY"  "$(vd upgrade)"
	st_check "login denied"                        "DENY"  "$(vd login --token x)"
	st_check "app dev-tunnel denied"               "DENY"  "$(vd app dev-tunnel)"
	st_check "app dev-token denied"                "DENY"  "$(vd app dev-token)"
	st_check "unknown top-level command denied"    "DENY"  "$(vd frobnicate --yes)"
	st_check "unknown app subcommand denied"       "DENY"  "$(vd app frobnicate)"
	st_check "unknown listing verb denied"         "DENY"  "$(vd app listing frobnicate)"

	note "--- FINDING 1: root persistent flags must not hide the command ---"
	st_check "--no-color generate is still a spend"    "DENY" "$(vd --no-color generate cat --yes)"
	st_check "--no-color upgrade still denied"         "DENY" "$(vd --no-color upgrade)"
	st_check "--no-update-check upgrade still denied"  "DENY" "$(vd --no-update-check upgrade)"
	st_check "--color login still denied"              "DENY" "$(vd --color login --token x)"
	st_check "app --no-color submit foreign denied"    "DENY" "$(vd app --no-color submit "${td}/bad" --yes)"
	st_check "--no-color app withdraw denied"          "DENY" "$(vd --no-color app withdraw pubreq_OTHER)"
	st_check "--no-color generate --max-cost is spend" "ALLOW_SPEND" "$(vd --no-color generate cat --yes --max-cost 5)"
	note "    the REASON must be the real branch, not the unknown-command fallback"
	st_reason "--no-color generate refused for the MONEY reason" "--max-cost" \
		--no-color generate cat --yes
	st_reason "--no-color upgrade refused AS upgrade" "replace the binary" \
		--no-color upgrade
	st_reason "app --no-color submit foreign refused on the PREFIX" "sanctioned prefix" \
		app --no-color submit "${td}/bad" --yes
	st_reason "--no-color app withdraw refused on the ALLOWLIST" "not created by this sandbox" \
		--no-color app withdraw pubreq_OTHER

	note "--- FINDING 2: --help as a flag VALUE is not a help request ---"
	st_check "genuine generate --help"                 "ALLOW" "$(vd generate --help)"
	st_check "genuine upgrade --help"                  "ALLOW" "$(vd upgrade --help)"
	st_check "genuine app listing set-icon --help"     "ALLOW" "$(vd app listing set-icon --help)"
	st_check "bare --help"                             "ALLOW" "$(vd --help)"
	st_check "--negative-prompt --help is NOT help"    "DENY"  "$(vd generate cat --yes --negative-prompt --help)"
	st_check "--ecosystem --help is NOT help"          "DENY"  "$(vd generate cat --yes --ecosystem --help)"
	st_check "submit -o --help is NOT help"            "DENY"  "$(vd app submit "${td}/bad" --yes -o --help)"
	st_check "--negative-prompt -h is NOT help"        "DENY"  "$(vd generate cat --yes --negative-prompt -h)"
	note "    (and the same trick aimed at --dry-run, which the audit did not list)"
	st_check "--negative-prompt --dry-run is NOT free" "DENY"  "$(vd generate cat --yes --negative-prompt --dry-run)"
	st_check "--input --print-input is NOT free"       "DENY"  "$(vd generate --yes --input --print-input)"
	st_check "an unknown flag before --dry-run is not free" "DENY" "$(vd generate cat --yes --mystery --dry-run)"

	note "--- generate money path ---"
	st_check "real dry-run is free"                "ALLOW"       "$(vd generate cat --dry-run)"
	st_check "real print-input is free"            "ALLOW"       "$(vd generate cat --print-input)"
	st_check "no --max-cost denied"                "DENY"        "$(vd generate cat --yes)"
	st_check "--max-cost over ceiling denied"      "DENY"        "$(vd generate cat --yes --max-cost 101)"
	st_check "--max-cost at ceiling allowed"       "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 100)"
	st_check "--max-cost=N form"                   "ALLOW_SPEND" "$(vd generate cat --yes --max-cost=1)"
	st_check "non-numeric --max-cost denied"       "DENY"        "$(vd generate cat --yes --max-cost abc)"
	note "    LAST --max-cost wins, as pflag does"
	st_check "--max-cost 5 then 99999 denied"      "DENY"        "$(vd generate cat --yes --max-cost 5 --max-cost 99999)"
	st_check "--max-cost 99999 then 5 allowed"     "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 99999 --max-cost 5)"

	note "--- app identity gating ---"
	st_check "submit sanctioned"                   "ALLOW_APP_SUBMIT" "$(vd app submit "${td}/good" --yes)"
	st_check "submit foreign denied"               "DENY"             "$(vd app submit "${td}/bad" --yes)"
	st_check "submit -o value not read as dir"     "ALLOW_APP_SUBMIT" "$(vd app submit -o "${td}/bad" "${td}/good")"
	st_check "package-only allowed"                "ALLOW"            "$(vd app submit "${td}/bad" --package-only)"
	st_check "listing foreign --slug denied"       "DENY"             "$(vd app listing set-icon i.png --slug someones-real-app)"
	st_check "listing sanctioned --dir"            "ALLOW"            "$(vd app listing set-icon i.png --dir "${td}/good")"
	st_check "listing unidentifiable denied"       "DENY"             "$(vd app listing set-icon i.png --dir "${ROOT}/ledger")"
	st_check "listing status is a read"            "ALLOW"            "$(vd app listing status)"

	note "--- FINDING 9: withdraw ---"
	st_check "unknown pubreq id denied"            "DENY"  "$(vd app withdraw pubreq_UNKNOWN)"
	st_check "non-pubreq-shaped id denied"         "DENY"  "$(vd app withdraw NOT-A-PUBREQ-ID)"
	st_check "--id form denied"                    "DENY"  "$(vd app withdraw --id NOT-A-PUBREQ-ID)"
	st_check "no id at all is a CLI usage error"   "ALLOW" "$(vd app withdraw)"
	printf 'pubreq_MINE\n' >> "${ROOT}/ledger/pubreq.allow"
	st_check "allowlisted id permitted"            "ALLOW" "$(vd app withdraw pubreq_MINE)"

	note "--- FINDING 3: malformed policy / ledger fails CLOSED ---"
	printf 'garbage\n' > "${ROOT}/ledger/observed_spend"
	st_check "malformed observed_spend denies spend" "DENY" "$(vd generate cat --yes --max-cost 5)"
	printf '0\n' > "${ROOT}/ledger/observed_spend"
	printf '\n' > "${ROOT}/ledger/reserved"
	st_check "empty reserved reads as 0"             "ALLOW_SPEND" "$(vd generate cat --yes --max-cost 5)"
	printf 'x\n' > "${ROOT}/ledger/reserved"
	st_check "malformed reserved denies spend"       "DENY" "$(vd generate cat --yes --max-cost 5)"
	printf '0\n' > "${ROOT}/ledger/reserved"
	local saved_pc="${PER_CALL_MAX_COST}"
	local pol="${ROOT}/ledger/policy.env" bak="${ROOT}/ledger/policy.bak"
	cp "${pol}" "${bak}"
	grep -v '^PER_CALL_MAX_COST=' "${bak}" > "${pol}"
	if load_policy; then
		st_check "policy missing a key is rejected" "rejected" "accepted"
	else
		st_check "policy missing a key is rejected" "rejected" "rejected"
	fi
	printf 'PER_CALL_MAX_COST=notanumber\n' >> "${pol}"
	if load_policy; then
		st_check "policy with a non-numeric key is rejected" "rejected" "accepted"
	else
		st_check "policy with a non-numeric key is rejected" "rejected" "rejected"
	fi
	cp "${bak}" "${pol}"; rm -f "${bak}"
	load_policy || die "selftest: could not restore the policy"
	st_check "policy restored" "${saved_pc}" "${PER_CALL_MAX_COST}"

	note "--- FINDING 4: the meter_broken latch stops the money path ---"
	: > "${ROOT}/ledger/meter_broken"
	st_check "spend refused once the meter is broken" "DENY"  "$(vd generate cat --yes --max-cost 5)"
	st_check "dry-run still allowed"                  "ALLOW" "$(vd generate cat --dry-run)"
	rm -f "${ROOT}/ledger/meter_broken"

	note "--- FINDING 5: the balance parser ---"
	local j
	for j in '{"total":3.5}' '{"total":1e9}' '{"total":-500}' '{"total":99999999999999999999}' \
	         '{"blue":1}' '' '{"total":}' '{"total":1,"total":2}'; do
		if parse_buzz_total "${j}" >/dev/null 2>&1; then
			st_check "parser rejects ${j:-<empty>}" "reject" "accept($(parse_buzz_total "${j}"))"
		else
			st_check "parser rejects ${j:-<empty>}" "reject" "reject"
		fi
	done
	st_check "parser accepts compact"        "4187454" "$(parse_buzz_total '{"total":4187454}' || printf FAILED)"
	st_check "parser accepts spaced/pretty"  "4187454" "$(parse_buzz_total '{
  "blue": 1338425,
  "green": 999980,
  "total": 4187454,
  "yellow": 1849049
}' || printf FAILED)"
	st_check "parser accepts a real zero"    "0"       "$(parse_buzz_total '{"total":0}' || printf FAILED)"

	note "--- FINDING 6: the ledger lock exists ---"
	if command -v flock >/dev/null 2>&1; then
		st_check "flock is available" "yes" "yes"
	else
		st_check "flock is available" "yes" "no"
	fi

	note "--- FINDING 7: TSV escaping ---"
	st_check "newline escaped"  'a\nb'  "$(tsv_escape "$(printf 'a\nb')")"
	st_check "tab escaped"      'a\tb'  "$(tsv_escape "$(printf 'a\tb')")"
	st_check "argv scrub escapes a newline" 'gen a\nDENY\tx' "$(scrub_argv gen "$(printf 'a\nDENY\tx')")"
	st_check "token redacted"   'login --token <redacted>' "$(scrub_argv login --token SECRETVALUE)"

	rm -rf "${td}"

	if [ "${offline}" = 1 ]; then
		selftest_e2e
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
		st_check "sandbox config exists"    "yes" "$([ -f "${ROOT}/home/.config/civitai/config.yaml" ] && printf yes || printf no)"
	fi

	note ""
	note "selftest: ${SELFTEST_PASS} passed, ${SELFTEST_FAIL} failed"
	[ "${SELFTEST_FAIL}" = 0 ]
}

# ---------------------------------------------------------------------------
# End-to-end exercise of the GUARD (not just the classifier), driven by a stub
# binary that charges a known amount. This is what makes the meter, the lock
# and the ledger regression-testable without a credential or a network — the
# classifier being right says nothing about whether the guard acts on it.
# ---------------------------------------------------------------------------
selftest_e2e() {
	local base="${TMPDIR:-/tmp}/dogfood-e2e-$$"
	rm -rf "${base}"
	mkdir -p "${base}/real" "${base}/bin" "${base}/ledger" \
	         "${base}/home/.config" "${base}/home/.cache" "${base}/workspace"

	cat > "${base}/real/civitai" <<'STUB'
#!/usr/bin/env bash
set -u
BAL="${XDG_CACHE_HOME}/bal"; NB="${XDG_CACHE_HOME}/nbuzz"
[ -f "${BAL}" ] || echo 10000 > "${BAL}"
[ -f "${NB}" ]  || echo 0 > "${NB}"
args=()
for a in "$@"; do
  case "${a}" in --no-color|--color|--no-update-check) ;; *) args+=("${a}") ;; esac
done
set -- "${args[@]+"${args[@]}"}"
case "${1:-}" in
  buzz)
    n=$(cat "${NB}"); echo $(( n + 1 )) > "${NB}"
    if [ -n "${STUB_FLAKY_BUZZ:-}" ] && [ $(( n % 2 )) = 1 ]; then echo boom >&2; exit 5; fi
    printf '{\n  "total": %s\n}\n' "$(cat "${BAL}")" ;;
  generate) echo $(( $(cat "${BAL}") - 37 )) > "${BAL}"; echo generated ;;
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

	cat > "${base}/ledger/policy.env" <<POL
ROOT="${base}"
PER_CALL_MAX_COST=100
TOTAL_SPEND_CAP=2000
MAX_GENERATE_SUBMITS=50
MAX_APP_SUBMITS=3
SLUG_PREFIX="sanctioned-"
BASE_URL="https://example.invalid"
POL
	: > "${base}/ledger/invocations.tsv"; : > "${base}/ledger/buzz.tsv"
	: > "${base}/ledger/spend.tsv";       : > "${base}/ledger/pubreq.allow"
	printf '0\n' > "${base}/ledger/observed_spend"
	printf '0\n' > "${base}/ledger/reserved"
	printf '0\n' > "${base}/ledger/generate_submits"
	printf '0\n' > "${base}/ledger/app_submits"

	local C="${base}/bin/civitai"
	local rc meter lines allow

	note "--- end-to-end through the real guard (stub charges 37) ---"

	"${C}" generate cat --yes --max-cost 50 >/dev/null 2>&1; rc=$?
	meter=$(cat "${base}/ledger/observed_spend")
	st_check "e2e allowed spend returns the CLI's rc" "0" "${rc}"
	st_check "e2e meter moved by the real charge"     "37" "${meter}"

	"${C}" --no-color generate cat --yes >/dev/null 2>&1; rc=$?
	st_check "e2e --no-color generate refused"        "126" "${rc}"

	"${C}" generate cat --yes --negative-prompt --help >/dev/null 2>&1; rc=$?
	st_check "e2e --negative-prompt --help refused"   "126" "${rc}"

	"${C}" --no-update-check upgrade >/dev/null 2>&1; rc=$?
	st_check "e2e --no-update-check upgrade refused"  "126" "${rc}"

	"${C}" frobnicate --yes >/dev/null 2>&1; rc=$?
	st_check "e2e an unrecognised command refused"    "126" "${rc}"

	# The guard must refuse everything when its own policy will not validate.
	cp "${base}/ledger/policy.env" "${base}/ledger/policy.keep"
	grep -v '^TOTAL_SPEND_CAP=' "${base}/ledger/policy.keep" > "${base}/ledger/policy.env"
	"${C}" whoami >/dev/null 2>&1; rc=$?
	st_check "e2e a malformed policy refuses even a read" "126" "${rc}"
	cp "${base}/ledger/policy.keep" "${base}/ledger/policy.env"
	rm -f "${base}/ledger/policy.keep"

	lines=$(wc -l < "${base}/ledger/invocations.tsv")
	"${C}" generate "$(printf 'a\nDENY\tinjected')" --yes --max-cost 5 >/dev/null 2>&1
	st_check "e2e a newline prompt adds ONE ledger row" "1" \
		"$(( $(wc -l < "${base}/ledger/invocations.tsv") - lines ))"

	# Concurrency: leave exactly 10 Buzz of headroom and fire 8 at once.
	printf '1990\n' > "${base}/ledger/observed_spend"
	printf '0\n' > "${base}/ledger/reserved"
	printf '0\n' > "${base}/ledger/generate_submits"
	: > "${base}/ledger/invocations.tsv"
	local i
	for i in 1 2 3 4 5 6 7 8; do "${C}" generate "p${i}" --yes --max-cost 10 >/dev/null 2>&1 & done
	wait
	allow=$(grep -c 'ALLOW_SPEND' "${base}/ledger/invocations.tsv" 2>/dev/null || printf 0)
	st_check "e2e 8 concurrent submits, 10 Buzz headroom" "1" "${allow}"
	st_check "e2e submit counter matches the allowed rows" "${allow}" "$(cat "${base}/ledger/generate_submits")"

	# A post-call balance read that fails must charge the worst case and latch.
	printf '0\n' > "${base}/ledger/observed_spend"
	printf '0\n' > "${base}/ledger/reserved"
	printf '0\n' > "${base}/ledger/generate_submits"
	printf '0\n' > "${base}/home/.cache/nbuzz"
	rm -f "${base}/ledger/meter_broken"
	STUB_FLAKY_BUZZ=1 "${C}" generate cat --yes --max-cost 60 >/dev/null 2>&1
	st_check "e2e a lost post-sample charges the worst case" "60" "$(cat "${base}/ledger/observed_spend")"
	st_check "e2e a lost post-sample latches meter_broken"   "yes" \
		"$([ -f "${base}/ledger/meter_broken" ] && printf yes || printf no)"
	"${C}" generate cat --yes --max-cost 5 >/dev/null 2>&1; rc=$?
	st_check "e2e spend refused after the latch"            "126" "${rc}"

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
		guard)         cmd_guard "$@" ;;
		status)        cmd_status "$@" ;;
		finish)        cmd_finish "$@" ;;
		selftest)      cmd_selftest "$@" ;;
		enter)         cmd_enter "$@" ;;
		allow-pubreq)  cmd_allow_pubreq "$@" ;;
		teardown)      cmd_teardown "$@" ;;
		__buzz)        cmd_buzz_probe "$@" ;;
		*)             usage ;;
	esac
}

main "$@"
