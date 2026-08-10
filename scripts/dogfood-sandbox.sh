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
#   selftest     positive controls on the FREE surface (spends nothing)
#   enter        spawn a bubblewrap jail in which the repo does not exist
#   allow-pubreq record a publish-request id as withdrawable
#   teardown     unlock the read-only dirs and (optionally) delete the run root
#
# Every path is absolute; every `cd` is gated; every variable is braced.

set -uo pipefail

# ---------------------------------------------------------------------------
# Defaults. Override on `init` via flags, or afterwards by editing
# ${ROOT}/ledger/policy.env (which `init` writes and every later call reads).
# ---------------------------------------------------------------------------
DEFAULT_ROOT="/tmp/claude-1000/dogfood3-scratch/run"

# Buzz an ESTIMATE may reach for a single `generate` submit. Enforced by
# REFUSING a submit that does not carry `--max-cost <= this`. The CLI's own
# help is explicit that --max-cost is not a spending cap; see the design doc.
DEFAULT_PER_CALL_MAX_COST=100

# Cumulative OBSERVED spend (start balance minus current) at which further
# `generate` submits are refused. Checked BEFORE each submit.
DEFAULT_TOTAL_SPEND_CAP=2000

# Hard ceilings on the number of mutating invocations, independent of cost.
DEFAULT_MAX_GENERATE_SUBMITS=20
DEFAULT_MAX_APP_SUBMITS=3

# Only apps whose blockId starts with this may be submitted or have their
# listing media mutated. `init` derives a dated default.
DEFAULT_SLUG_PREFIX=""

# ---------------------------------------------------------------------------
# Small helpers
# ---------------------------------------------------------------------------
err()  { printf 'dogfood-sandbox: %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }
note() { printf '%s\n' "$*"; }

# Machine-readable marker so a refusal from THIS harness can never be mistaken
# for a civitai CLI error and filed as a dogfood issue against the CLI.
SANDBOX_TAG="SANDBOX POLICY"

now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# Extract "total" from `civitai buzz --json`. Fails LOUDLY on no match: a
# balance read that silently yields empty must never be read as "spent 0".
parse_buzz_total() {
	local json="$1" v
	v=$(printf '%s' "${json}" | tr -d ' \n\t' | grep -o '"total":-\?[0-9]\+' | head -1 | cut -d: -f2)
	case "${v}" in
		''|*[!0-9-]*) return 1 ;;
	esac
	printf '%s' "${v}"
}

# Scrub credentials out of an argv before it reaches the audit log.
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
	printf '%s' "${out# }"
}

load_policy() {
	[ -f "${ROOT}/ledger/policy.env" ] || die "no policy at ${ROOT}/ledger/policy.env — run \`init\` first"
	# shellcheck disable=SC1091
	. "${ROOT}/ledger/policy.env"
}

# Run the REAL binary with the sandbox environment, bypassing the guard.
# Used by init/finish/selftest and by the guard's own balance sampling.
real_cli() {
	HOME="${ROOT}/home" \
	XDG_CONFIG_HOME="${ROOT}/home/.config" \
	XDG_CACHE_HOME="${ROOT}/home/.cache" \
	CIVITAI_NO_UPDATE_CHECK=1 \
	"${ROOT}/real/civitai" "$@"
}

# Sample the balance. Echoes the total on stdout; non-zero on any failure.
sample_balance() {
	local phase="$1" json total
	json=$(real_cli buzz --json 2>/dev/null)
	total=$(parse_buzz_total "${json}") || return 1
	printf '%s\t%s\t%s\n' "$(now_iso)" "${phase}" "${json}" >> "${ROOT}/ledger/buzz.tsv"
	printf '%s' "${total}"
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

	while [ $# -gt 0 ]; do
		case "$1" in
			--root)          root="$2"; shift 2 ;;
			--binary)        binary="$2"; shift 2 ;;
			--token-file)    token_file="$2"; shift 2 ;;
			--per-call-max)  per_call="$2"; shift 2 ;;
			--total-cap)     total_cap="$2"; shift 2 ;;
			--max-generates) max_gen="$2"; shift 2 ;;
			--max-app-submits) max_app="$2"; shift 2 ;;
			--slug-prefix)   slug_prefix="$2"; shift 2 ;;
			--force)         force=1; shift ;;
			*) die "init: unknown flag $1" ;;
		esac
	done

	ROOT="${root}"
	[ -n "${binary}" ] || die "init: --binary <path to built civitai> is required"
	[ -x "${binary}" ] || die "init: ${binary} is not an executable file"

	if [ -e "${ROOT}" ] && [ "${force}" != 1 ]; then
		die "init: ${ROOT} already exists (use --force to rebuild, or \`teardown\` first)"
	fi
	if [ -e "${ROOT}" ]; then
		cmd_teardown --root "${ROOT}" --delete --quiet
	fi

	# The token must never arrive as a harness argument (shell history) and must
	# never be written into the repo.
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

	if [ -z "${slug_prefix}" ]; then
		slug_prefix="dogfood3-$(date -u +%Y%m%d)-"
	fi

	mkdir -p "${ROOT}/real" "${ROOT}/bin" "${ROOT}/harness" \
	         "${ROOT}/home/.config" "${ROOT}/home/.cache" \
	         "${ROOT}/workspace" "${ROOT}/ledger" || die "init: mkdir failed"
	chmod 0700 "${ROOT}" "${ROOT}/home" "${ROOT}/ledger"

	cp "${binary}" "${ROOT}/real/civitai" || die "init: could not copy the binary"

	# The harness runs from a COPY so the run is self-contained and so a jail
	# that does not mount the repo still works.
	local self
	self=$(readlink -f "$0")
	cp "${self}" "${ROOT}/harness/dogfood-sandbox.sh" || die "init: could not copy the harness"
	chmod 0555 "${ROOT}/harness/dogfood-sandbox.sh"

	# The `civitai` the agent gets is a shim into the policy gate.
	cat > "${ROOT}/bin/civitai" <<SHIM
#!/usr/bin/env bash
exec "${ROOT}/harness/dogfood-sandbox.sh" guard --root "${ROOT}" -- "\$@"
SHIM
	chmod 0555 "${ROOT}/bin/civitai"

	: > "${ROOT}/ledger/invocations.tsv"
	: > "${ROOT}/ledger/buzz.tsv"
	: > "${ROOT}/ledger/pubreq.allow"
	printf '0\n' > "${ROOT}/ledger/generate_submits"
	printf '0\n' > "${ROOT}/ledger/app_submits"

	cat > "${ROOT}/ledger/policy.env" <<POLICY
# Written by dogfood-sandbox.sh init on $(now_iso). Read by every later call.
ROOT="${ROOT}"
PER_CALL_MAX_COST=${per_call}
TOTAL_SPEND_CAP=${total_cap}
MAX_GENERATE_SUBMITS=${max_gen}
MAX_APP_SUBMITS=${max_app}
SLUG_PREFIX="${slug_prefix}"
BINARY_SOURCE="${binary}"
BINARY_SHA256="$(sha256sum "${binary}" | cut -d' ' -f1)"
HARNESS_SHA256="$(sha256sum "${self}" | cut -d' ' -f1)"
POLICY
	chmod 0600 "${ROOT}/ledger/policy.env"

	# Seed the credential THROUGH the CLI (it is the authority on the config
	# format) into the sandbox config only.
	real_cli login --token "${token}" >/dev/null 2>&1 \
		|| die "init: seeding the credential failed — check the token"
	token=""
	[ -f "${ROOT}/home/.config/civitai/config.yaml" ] \
		|| die "init: no config written at ${ROOT}/home/.config/civitai/config.yaml"
	chmod 0600 "${ROOT}/home/.config/civitai/config.yaml"

	# Prove the credential works before locking anything down.
	real_cli whoami >/dev/null 2>&1 || die "init: whoami failed with the seeded credential"

	local start
	start=$(sample_balance "start") || die "init: could not read the opening Buzz balance — refusing to start a run whose meter does not work"
	printf '%s\n' "${start}" > "${ROOT}/ledger/start_balance"

	# Structural: `upgrade` writes .civitai.new into the binary's directory via
	# O_CREATE (minio/selfupdate apply.go). A 0500 directory makes that EACCES.
	# The 0555 on the file itself closes the in-place-write variant.
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
	# The caps live in policy.env, not in init's locals, so source them —
	# without this the brief renders literal placeholder text and a blind agent
	# is told the prefix is "the sanctioned prefix".
	load_policy
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

Submitting an app puts it in front of a real human moderator. That is expected
and approved for this run — but do it deliberately, not to see what happens.

Report what you find as you would to the tool's authors: what worked, what was
confusing, what was wrong, and what you could not do.
BRIEF
}

# ---------------------------------------------------------------------------
# guard — the policy gate. Invoked as: guard --root <ROOT> -- <civitai argv...>
# ---------------------------------------------------------------------------
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
	load_policy

	local argv=("$@")
	local scrubbed
	scrubbed=$(scrub_argv "${argv[@]+"${argv[@]}"}")

	local verdict reason
	verdict=$(policy_verdict "${argv[@]+"${argv[@]}"}")
	reason="${verdict#*:}"
	verdict="${verdict%%:*}"

	if [ "${verdict}" = "DENY" ]; then
		printf '%s: %s\n' "${SANDBOX_TAG}" "${reason}" >&2
		log_invocation "DENY" "126" "0" "${scrubbed}" "${reason}"
		return 126
	fi

	# A real spend: bracket the call with balance samples.
	local before="" after="" delta=""
	if [ "${verdict}" = "ALLOW_SPEND" ]; then
		before=$(sample_balance "pre-generate") || {
			printf '%s: %s\n' "${SANDBOX_TAG}" \
				"could not read the Buzz balance, so the spend meter cannot be kept — refusing to submit" >&2
			log_invocation "DENY" "126" "0" "${scrubbed}" "balance read failed (fail closed)"
			return 126
		}
		bump_counter "generate_submits"
	fi
	if [ "${verdict}" = "ALLOW_APP_SUBMIT" ]; then
		bump_counter "app_submits"
	fi

	local t0 t1 rc
	t0=$(date +%s%N)
	HOME="${ROOT}/home" \
	XDG_CONFIG_HOME="${ROOT}/home/.config" \
	XDG_CACHE_HOME="${ROOT}/home/.cache" \
	CIVITAI_NO_UPDATE_CHECK=1 \
	"${ROOT}/real/civitai" "${argv[@]+"${argv[@]}"}"
	rc=$?
	t1=$(date +%s%N)

	if [ "${verdict}" = "ALLOW_SPEND" ] && [ -n "${before}" ]; then
		after=$(sample_balance "post-generate") || after=""
		if [ -n "${after}" ]; then
			delta=$(( before - after ))
			local cum
			cum=$(cat "${ROOT}/ledger/observed_spend" 2>/dev/null || printf '0')
			printf '%s\n' "$(( cum + delta ))" > "${ROOT}/ledger/observed_spend"
			printf '%s\tgenerate\t%s\t%s\t%s\n' "$(now_iso)" "${before}" "${after}" "${delta}" \
				>> "${ROOT}/ledger/spend.tsv"
		fi
	fi

	log_invocation "${verdict}" "${rc}" "$(( (t1 - t0) / 1000000 ))" "${scrubbed}" "${delta:-}"
	return "${rc}"
}

log_invocation() {
	printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$(now_iso)" "$1" "$2" "$3" "$4" "${5:-}" >> "${ROOT}/ledger/invocations.tsv"
}

bump_counter() {
	local f="${ROOT}/ledger/$1" n
	n=$(cat "${f}" 2>/dev/null || printf '0')
	printf '%s\n' "$(( n + 1 ))" > "${f}"
}

read_counter() {
	cat "${ROOT}/ledger/$1" 2>/dev/null || printf '0'
}

# Does argv contain this exact token?
has_flag() {
	local want="$1"; shift
	local a
	for a in "$@"; do
		[ "${a}" = "${want}" ] && return 0
	done
	return 1
}

# Value of --flag V or --flag=V; empty if absent.
flag_value() {
	local want="$1"; shift
	local a next=0
	for a in "$@"; do
		if [ "${next}" = 1 ]; then printf '%s' "${a}"; return 0; fi
		case "${a}" in
			"${want}")   next=1 ;;
			"${want}"=*) printf '%s' "${a#*=}"; return 0 ;;
		esac
	done
	return 1
}

# The blockId a mutating app command would act on: --slug wins, else the
# manifest in --dir (listing) or the positional dir (submit), default ".".
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

# Echoes "<VERDICT>:<reason>". VERDICT is ALLOW, ALLOW_SPEND, ALLOW_APP_SUBMIT
# or DENY. Deliberately fails CLOSED: anything it cannot classify but that can
# mutate or spend is denied.
policy_verdict() {
	local c1="${1:-}" c2="${2:-}" c3="${3:-}"

	# Help is always free, on every command, including the forbidden ones —
	# reading --help must never be blocked, or the agent cannot learn the tool.
	if has_flag "--help" "$@" || has_flag "-h" "$@"; then
		printf 'ALLOW:help'; return 0
	fi

	case "${c1}" in
		upgrade)
			printf 'DENY:%s' "\`upgrade\` would replace the binary under evaluation mid-run"
			return 0 ;;
		login)
			printf 'DENY:%s' "this sandbox is already authenticated; run \`civitai whoami\` to see as whom"
			return 0 ;;
	esac

	if [ "${c1}" = "app" ]; then
		case "${c2}" in
			dev-tunnel|dev-token)
				printf 'DENY:%s' "\`app ${c2}\` is out of scope for this run"
				return 0 ;;
			withdraw)
				local id
				id=$(printf '%s\n' "$@" | grep -m1 '^pubreq_' || true)
				[ -n "${id}" ] || id=$(flag_value "--id" "$@" || true)
				if [ -z "${id}" ]; then
					printf 'ALLOW:withdraw with no id (the CLI will refuse it)'; return 0
				fi
				if grep -qxF "${id}" "${ROOT}/ledger/pubreq.allow" 2>/dev/null; then
					printf 'ALLOW:withdraw of a submission this sandbox created'; return 0
				fi
				printf 'DENY:%s' "${id} was not created by this sandbox, so it may be one of the account's real pending submissions"
				return 0 ;;
			submit)
				if has_flag "--package-only" "$@"; then
					printf 'ALLOW:--package-only never submits'; return 0
				fi
				local n dir="" a skipnext=0
				# positional dir: first non-flag arg after `submit`, skipping -o/--out values
				for a in "${@:3}"; do
					if [ "${skipnext}" = 1 ]; then skipnext=0; continue; fi
					case "${a}" in
						-o|--out) skipnext=1 ;;
						-*)       ;;
						*)        dir="${a}"; break ;;
					esac
				done
				[ -n "${dir}" ] || dir="."
				local id
				if ! id=$(resolve_block_id "${dir}" ""); then
					printf 'DENY:%s' "cannot read a blockId from ${dir}/block.manifest.json, so the app being submitted cannot be identified"
					return 0
				fi
				case "${id}" in
					"${SLUG_PREFIX}"*) ;;
					*) printf 'DENY:%s' "blockId \`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may only submit its own throwaway app"
					   return 0 ;;
				esac
				n=$(read_counter "app_submits")
				if [ "${n}" -ge "${MAX_APP_SUBMITS}" ]; then
					printf 'DENY:%s' "the app-submission cap for this run (${MAX_APP_SUBMITS}) is already used"
					return 0
				fi
				printf 'ALLOW_APP_SUBMIT:submitting %s' "${id}"
				return 0 ;;
			listing)
				case "${c3}" in
					status|"")
						printf 'ALLOW:listing read'; return 0 ;;
					set-icon|set-cover|add-screenshot|rm-screenshot|reorder)
						local slug dir id
						slug=$(flag_value "--slug" "$@" || true)
						dir=$(flag_value "--dir" "$@" || true)
						[ -n "${dir}" ] || dir="."
						if ! id=$(resolve_block_id "${dir}" "${slug}"); then
							printf 'DENY:%s' "cannot identify which app \`app listing ${c3}\` would mutate (no --slug, and no readable blockId in ${dir}/block.manifest.json)"
							return 0
						fi
						case "${id}" in
							"${SLUG_PREFIX}"*)
								printf 'ALLOW:listing mutation on %s' "${id}"; return 0 ;;
							*)
								printf 'DENY:%s' "\`${id}\` is outside the sanctioned prefix \`${SLUG_PREFIX}\` — this run may not touch the account's real listings"
								return 0 ;;
						esac ;;
					*)
						printf 'DENY:%s' "unrecognised \`app listing ${c3}\`; the gate fails closed on anything that might mutate a listing"
						return 0 ;;
				esac ;;
		esac
		printf 'ALLOW:app read/local'; return 0
	fi

	if [ "${c1}" = "generate" ]; then
		if has_flag "--print-input" "$@" || has_flag "--dry-run" "$@"; then
			printf 'ALLOW:generate preview (spends nothing)'; return 0
		fi
		local n cum remaining mc
		# The MONEY cap is checked before the invocation cap: when both are
		# tripped the spend figure is the more actionable reason to print.
		cum=$(cat "${ROOT}/ledger/observed_spend" 2>/dev/null || printf '0')
		if [ "${cum}" -ge "${TOTAL_SPEND_CAP}" ]; then
			printf 'DENY:%s' "the run's total spend cap (${TOTAL_SPEND_CAP} Buzz) is reached — ${cum} observed"
			return 0
		fi
		n=$(read_counter "generate_submits")
		if [ "${n}" -ge "${MAX_GENERATE_SUBMITS}" ]; then
			printf 'DENY:%s' "the generation cap for this run (${MAX_GENERATE_SUBMITS} submits) is already used"
			return 0
		fi
		if ! mc=$(flag_value "--max-cost" "$@"); then
			printf 'DENY:%s' "a generation that actually submits must carry --max-cost (at most ${PER_CALL_MAX_COST}); use --dry-run to price it first"
			return 0
		fi
		case "${mc}" in
			''|*[!0-9]*)
				printf 'DENY:%s' "--max-cost must be a whole number of Buzz, got \`${mc}\`"
				return 0 ;;
		esac
		if [ "${mc}" -gt "${PER_CALL_MAX_COST}" ]; then
			printf 'DENY:%s' "--max-cost ${mc} exceeds this run's per-invocation ceiling of ${PER_CALL_MAX_COST} Buzz"
			return 0
		fi
		remaining=$(( TOTAL_SPEND_CAP - cum ))
		if [ "${mc}" -gt "${remaining}" ]; then
			printf 'DENY:%s' "--max-cost ${mc} exceeds the ${remaining} Buzz left under this run's total cap"
			return 0
		fi
		printf 'ALLOW_SPEND:submit with --max-cost %s' "${mc}"
		return 0
	fi

	printf 'ALLOW:read-only or local'
	return 0
}

# ---------------------------------------------------------------------------
# allow-pubreq / status / finish / selftest / enter / teardown
# ---------------------------------------------------------------------------
cmd_allow_pubreq() {
	local root="${DEFAULT_ROOT}" id=""
	while [ $# -gt 0 ]; do
		case "$1" in
			--root) root="$2"; shift 2 ;;
			*) id="$1"; shift ;;
		esac
	done
	ROOT="${root}"; load_policy
	[ -n "${id}" ] || die "allow-pubreq: pass the publish-request id"
	printf '%s\n' "${id}" >> "${ROOT}/ledger/pubreq.allow"
	note "recorded ${id} as withdrawable"
}

cmd_status() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root}"; load_policy
	local start cum
	start=$(cat "${ROOT}/ledger/start_balance" 2>/dev/null || printf '?')
	cum=$(cat "${ROOT}/ledger/observed_spend" 2>/dev/null || printf '0')
	note "run root .............. ${ROOT}"
	note "opening balance ....... ${start} Buzz"
	note "observed spend ........ ${cum} / ${TOTAL_SPEND_CAP} Buzz"
	note "generate submits ...... $(read_counter generate_submits) / ${MAX_GENERATE_SUBMITS}"
	note "app submits ........... $(read_counter app_submits) / ${MAX_APP_SUBMITS}"
	note "invocations logged .... $(wc -l < "${ROOT}/ledger/invocations.tsv" 2>/dev/null || printf 0)"
	note "refusals .............. $(grep -c '	DENY	' "${ROOT}/ledger/invocations.tsv" 2>/dev/null || printf 0)"
}

cmd_finish() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root}"; load_policy
	local start end
	start=$(cat "${ROOT}/ledger/start_balance" 2>/dev/null) || die "finish: no opening balance recorded"
	if ! end=$(sample_balance "end"); then
		err "finish: could not read the closing balance — the spend total below is the per-call ledger only"
		end=""
	fi
	printf '%s\n' "${end}" > "${ROOT}/ledger/end_balance"
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

cmd_selftest() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) shift ;; esac
	done
	ROOT="${root}"; load_policy

	local pass=0 fail=0
	check() {
		local label="$1" want="$2" got="$3"
		if [ "${want}" = "${got}" ]; then
			printf '  PASS  %-56s (%s)\n' "${label}" "${got}"; pass=$(( pass + 1 ))
		else
			printf '  FAIL  %-56s want=%s got=%s\n' "${label}" "${want}" "${got}"; fail=$(( fail + 1 ))
		fi
	}

	note "--- policy verdicts (no CLI is executed) ---"
	check "upgrade is denied"                 "DENY"             "$(policy_verdict upgrade | cut -d: -f1)"
	check "upgrade --help is allowed"         "ALLOW"            "$(policy_verdict upgrade --help | cut -d: -f1)"
	check "login is denied"                   "DENY"             "$(policy_verdict login --token x | cut -d: -f1)"
	check "dev-tunnel is denied"              "DENY"             "$(policy_verdict app dev-tunnel | cut -d: -f1)"
	check "dev-token is denied"               "DENY"             "$(policy_verdict app dev-token | cut -d: -f1)"
	check "generate --dry-run is free"        "ALLOW"            "$(policy_verdict generate x --dry-run | cut -d: -f1)"
	check "generate --print-input is free"    "ALLOW"            "$(policy_verdict generate x --print-input | cut -d: -f1)"
	check "generate without --max-cost denied" "DENY"            "$(policy_verdict generate x --yes | cut -d: -f1)"
	check "generate over per-call cap denied"  "DENY"            "$(policy_verdict generate x --yes --max-cost $(( PER_CALL_MAX_COST + 1 )) | cut -d: -f1)"
	check "generate at the cap is a spend"     "ALLOW_SPEND"     "$(policy_verdict generate x --yes --max-cost "${PER_CALL_MAX_COST}" | cut -d: -f1)"
	check "generate --max-cost=N form parsed"  "ALLOW_SPEND"     "$(policy_verdict generate x --yes --max-cost=1 | cut -d: -f1)"
	check "non-numeric --max-cost denied"      "DENY"            "$(policy_verdict generate x --yes --max-cost abc | cut -d: -f1)"
	check "withdraw of an unknown id denied"   "DENY"            "$(policy_verdict app withdraw pubreq_UNKNOWN | cut -d: -f1)"
	check "app list is allowed"                "ALLOW"           "$(policy_verdict app list | cut -d: -f1)"
	check "models search is allowed"           "ALLOW"           "$(policy_verdict models search --query x | cut -d: -f1)"
	check "listing status is allowed"          "ALLOW"           "$(policy_verdict app listing status | cut -d: -f1)"
	check "unknown listing verb denied"        "DENY"            "$(policy_verdict app listing frobnicate | cut -d: -f1)"

	# Identity gating needs real manifests on disk.
	local td="${ROOT}/ledger/_selftest"
	rm -rf "${td}"; mkdir -p "${td}/good" "${td}/bad"
	printf '{"blockId":"%sdemo","name":"d","version":"1.0.0","kind":"page"}\n' "${SLUG_PREFIX}" > "${td}/good/block.manifest.json"
	printf '{"blockId":"someones-real-app","name":"d","version":"1.0.0","kind":"page"}\n'        > "${td}/bad/block.manifest.json"
	check "submit of a sanctioned blockId"     "ALLOW_APP_SUBMIT" "$(policy_verdict app submit "${td}/good" --yes | cut -d: -f1)"
	check "submit of a foreign blockId denied" "DENY"             "$(policy_verdict app submit "${td}/bad" --yes | cut -d: -f1)"
	check "submit -o value not read as dir"    "ALLOW_APP_SUBMIT" "$(policy_verdict app submit -o "${td}/bad" "${td}/good" | cut -d: -f1)"
	check "listing on a foreign --slug denied" "DENY"             "$(policy_verdict app listing set-icon i.png --slug someones-real-app | cut -d: -f1)"
	check "listing on sanctioned --dir"        "ALLOW"            "$(policy_verdict app listing set-icon i.png --dir "${td}/good" | cut -d: -f1)"
	check "listing with no identity denied"    "DENY"             "$(policy_verdict app listing set-icon i.png --dir "${ROOT}/ledger" | cut -d: -f1)"
	rm -rf "${td}"

	note ""
	note "--- balance meter (a real read; spends nothing) ---"
	local b
	if b=$(sample_balance "selftest"); then
		check "buzz --json parses to an integer total" "yes" "$(case "${b}" in ''|*[!0-9]*) printf no ;; *) printf yes ;; esac)"
		note "  observed balance: ${b} Buzz"
	else
		check "buzz --json parses to an integer total" "yes" "no"
	fi
	# Negative control: the parser must FAIL on rubbish, not return 0.
	if parse_buzz_total '{"blue":1}' >/dev/null 2>&1; then
		check "balance parser rejects a payload with no total" "reject" "accept"
	else
		check "balance parser rejects a payload with no total" "reject" "reject"
	fi

	note ""
	note "--- isolation ---"
	local real_cfg="${HOME}/.config/civitai/config.yaml"
	if [ -f "${real_cfg}" ]; then
		note "  operator config present at ${real_cfg}"
		note "  sha256 before: $(sha256sum "${real_cfg}" | cut -d' ' -f1)"
	else
		note "  operator has no ~/.config/civitai/config.yaml (nothing to clobber)"
	fi
	check "sandbox config exists" "yes" "$([ -f "${ROOT}/home/.config/civitai/config.yaml" ] && printf yes || printf no)"
	check "binary dir is not writable" "no" "$([ -w "${ROOT}/real" ] && printf yes || printf no)"
	check "binary file is not writable" "no" "$([ -w "${ROOT}/real/civitai" ] && printf yes || printf no)"

	note ""
	note "selftest: ${pass} passed, ${fail} failed"
	[ "${fail}" = 0 ]
}

cmd_enter() {
	local root="${DEFAULT_ROOT}"
	while [ $# -gt 0 ]; do
		case "$1" in --root) root="$2"; shift 2 ;; *) break ;; esac
	done
	ROOT="${root}"; load_policy
	command -v bwrap >/dev/null 2>&1 || die "enter: bwrap (bubblewrap) is not on PATH"

	local shell_bin="${SHELL:-/bin/sh}"
	[ $# -gt 0 ] || set -- "${shell_bin}" -i

	# Only what the CLI needs: the Nix store (toolchain + certs), the system
	# profile, DNS + TLS trust, and the run root. NOTHING under /home is bound,
	# so the repo checkout does not exist inside.
	#
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

usage() {
	sed -n '3,30p' "$0"
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
		*)             usage ;;
	esac
}

main "$@"
