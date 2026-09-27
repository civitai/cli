# shellcheck shell=bash
# ── app-controlled text must not be able to write the verdict ────────────────
# NOT A SCRIPT. This file is SOURCED by every grader under this directory that
# prints a value it did not itself author — `oracle.sh` and `ship.verdict.sh`
# today. It defines nothing else and runs nothing on its own.
#
# 🔴 THE THING BEING GRADED WRITES THE TEXT THE GRADER PRINTS. `oracle.sh` reads
# `outputDir`, `buildCommand` and `scopes` out of the trial's own
# `block.manifest.json`, and `app_dir`/`served` out of directories the trial
# itself created. `ship.verdict.sh` reads `blockId` out of those same manifests,
# the manifest PATHS out of the trial's own filesystem, and a submission row's
# `id`/`blockId`/`status`/`submittedAt` back across a process boundary from the
# platform. Printed with a raw `%s` any of them can forge the grader's output,
# and the output is PARSED:
#
#   * `grade.sh` finds the render summary as the last `^brief=` line and then
#     takes the FIRST `KEY=` token per field
#     (`tr ' ' '\n' | sed -n 's/^KEY=//p' | head -1`).
#   * the Go suites find a summary by a line PREFIX (`brief=`, `ship_trial=`) and
#     split it with `strings.Fields`, first match per key — and `shipField`
#     returns off the FIRST prefixed line it sees, so an EARLIER forged line wins
#     outright.
#
# So there are two forgeries, and neither needs the container to be escaped:
#
#   * a SPACE ends the field and starts another. Any app-controlled field printed
#     BEFORE the verdict puts its own tokens ahead of the real ones, and a
#     first-match read prefers them. Measured on oracle.sh (#728): a manifest
#     declaring `"scopes": ["x RENDER=yes viewer=signed-in"]` made the graded cell
#     read `RENDER=yes` for an app that built nothing.
#   * a NEWLINE ends the line. A value carrying one, followed by a complete
#     summary-shaped line, emits a SECOND line a consumer can take for the
#     summary — and on `ship.verdict.sh` a directory name is enough, because
#     `find` prints it verbatim.
#
# 🔴 WHY NOT `printf '%q'`, WHICH IS WHAT oracle.sh's `observed=` USED. Two
# reasons, and the first is a live break. bash 5.3 escapes a COMMA where 5.2 does
# not, so `%q` renders a two-scope manifest as
# `ai:write:budgeted\,posts:write:self` on one grader's host and
# `ai:write:budgeted,posts:write:self` on another's — a field whose bytes depend
# on the grader's bash version, in a format both consumers compare exactly.
# Second, `%q` escapes with backslashes that a reader then has to un-read. The
# percent-hex rule below is version-independent and, because it has a fast path
# that returns a value with no whitespace and no control byte untouched, leaves
# every legitimate value BYTE-IDENTICAL — which is what keeps `grade.sh` and the
# Go suites reading the strings they already read.
#
# The escape is percent-hex, and `%` is escaped too — but ONLY when the value
# needed escaping at all, so a path or a reason that merely contains a `%` is
# still emitted verbatim.
#
#   tok   — a value that must stay ONE field: whitespace and control bytes are
#           escaped. A path, a comma-joined scope list, a slug, `none`, `212` and
#           `dist` are untouched.
#   prose — a value that IS a line of its own (`render_reason=`, `ship_reason=`,
#           the brief notes, the validator's own words): control bytes are
#           escaped, SPACES ARE KEPT, because the reason is a sentence that both a
#           reader and the suites' `strings.Contains` need to stay one.
#
# 🔴 ONE RULE, ONE PLACE — AND THAT IS ENFORCED, NOT ASKED FOR. A predicate
# open-coded at N sites is wrong at N−1 of them in the same direction, which is
# exactly how `ship.verdict.sh` came to print the same `printf 'manifests=%s\n%s\n'`
# that #728 had already fixed one file away. `TestDogfoodGradersShareOneEscaper`
# fails if any script here defines its own escaping instead of sourcing this file,
# if a summary line grows an argument that does not go through `tok`, or if
# `printf '%q'` reappears.
esc() {
  local mode="$1" v="${2-}" out= i c h
  case "$mode" in
    # The fast path's predicate is the slow path's escape set, or a value could
    # pass the test and still carry a byte the loop would have escaped.
    tok)   [[ "$v" == *[[:space:]]* || "$v" == *[[:cntrl:]]* ]] || { printf '%s' "$v"; return 0; } ;;
    prose) [[ "$v" == *[[:cntrl:]]* ]]                         || { printf '%s' "$v"; return 0; } ;;
    *) printf 'esc: unknown mode %s\n' "$mode" >&2; exit 2 ;;
  esac
  for (( i = 0; i < ${#v}; i++ )); do
    c=${v:i:1}
    case "$c" in
      '%') out+='%25' ;;
      # 🔴 `=` GOES TOO, ONCE A VALUE IS ON THE SLOW PATH, AND IT IS NOT
      # COSMETIC. Escaping only the whitespace leaves the token
      # `x%20RENDER=yes` — inert to both consumers, which split on whitespace,
      # but it still CONTAINS the string `RENDER=yes`, so a person or a script
      # grepping a matrix for `RENDER=yes` (or `SHIP=yes`) matches a cell whose
      # verdict is `no`. A value that needed escaping at all has forfeited the
      # benefit of the doubt, so nothing in it is left looking like a field
      # assignment.
      '=') out+='%3D' ;;
      # `[[:cntrl:]]` covers LF, CR and TAB, so `prose` neutralises the
      # line-ending vector without touching the spaces between its words.
      [[:cntrl:]]) printf -v h '%%%02X' "'$c"; out+="$h" ;;
      [[:space:]]) if [ "$mode" = tok ]; then printf -v h '%%%02X' "'$c"; out+="$h"; else out+="$c"; fi ;;
      *) out+="$c" ;;
    esac
  done
  printf '%s' "$out"
}
tok()   { esc tok   "${1-}"; }
prose() { esc prose "${1-}"; }
