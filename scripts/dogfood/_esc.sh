# shellcheck shell=bash
# ── app-controlled text must not be able to write the verdict ────────────────
# NOT A SCRIPT. This file is SOURCED by every grader under this directory that
# READS OR PRINTS a value it did not itself author — `oracle.sh` and
# `ship.verdict.sh` today. It runs nothing on its own.
#
# It holds two rules, both of which had already been open-coded wrong in both
# graders at once:
#
#   * the OUTPUT rule (`esc`/`tok`/`prose`, below) — app-controlled text must not
#     be able to write a field or a line of a grader's own output stream;
#   * the INPUT rule (`MANIFEST_FIND`/`pathdec`, at the foot of this file) — the
#     app-controlled PATHS the graders discover must survive being carried out of
#     the container, counted, and handed back in.
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

# ── the INPUT rule: discovering the trial's manifests ────────────────────────
# 🔴 THE PATHS ARE APP-CONTROLLED TOO, AND READING THEM WRONG MOVES A VERDICT.
# Both graders locate the trial's app by `find`ing its manifests inside the
# container and carrying the result back through a command substitution. Three
# things were wrong with that, all three chosen by the app, and all three
# duplicated in both files — the N−1 shape this file exists to make impossible:
#
#   1. `… | head -1 | xargs dirname` WORD-SPLITS. Measured:
#      `printf '%s\n' '/work/my app/block.manifest.json' | xargs dirname` prints
#      TWO lines, `/work` and `app`, so `$APP_DIR` became `/work<LF>app` — and
#      `$APP_DIR` is what gets validated, what `outputDir` resolves against, and
#      what is SERVED to the browser. A space in a directory name therefore moved
#      a RENDER verdict. A path containing a single quote is worse: `xargs` exits
#      non-zero with `unmatched single quote` and `$APP_DIR` is EMPTY, which reads
#      as "the trial created no app".
#   2. `grep -c .` over newline-separated `find` output COUNTS LINES, so a
#      directory name containing a newline counts as two manifests — and the
#      `while IFS= read -r` loops fed from the same string split one path into two
#      halves, neither of which names a file. On `ship.verdict.sh` that emptied
#      `TRIAL_SLUGS` and turned a real `SHIP=yes` into an exit-2 `unmeasured`.
#   3. The discovered path was then spliced into a `bash -lc "… '$path' …"`
#      COMMAND STRING, so a quote in it ran the rest of the name as a command in
#      the grader's own exec. Closed at the call sites by passing every such value
#      through `docker exec -e`; nothing here hand-rolls quote escaping.
#
# 🔴 WHY NOT `-print0`, WHICH IS THE OBVIOUS FIX FOR (2). Bash COMMAND
# SUBSTITUTION DISCARDS NUL BYTES — measured: `X=$(printf 'a\0b\0')` warns
# `ignored null byte in input` and leaves `${#X}` = 2. So NUL-delimited data
# cannot be carried out of `$(docker exec …)` at all, and a grader built on
# `-print0` would silently concatenate every path into one.
#
# So the container SWAPS THE TWO DELIMITERS instead: `-print0` separates records
# with NUL, then a real newline inside a path becomes 0x01 and the NUL separators
# become newlines. The result is one line per path, carried through a command
# substitution intact, and `pathdec` puts the newline back before the path is used
# or printed.
#
# 🔴 AN ORDINARY PATH IS LEFT BYTE-IDENTICAL by both halves, which is what keeps
# `grade.sh`, the Go suites and a human reader seeing the strings they already
# see. `TestOracleLeavesLegitimateManifestValuesByteIdentical` and
# `TestShipVerdictLeavesLegitimateValuesByteIdentical` are the arms that pin it.
#
# ⚠ ONE KNOWN LIMIT, STATED RATHER THAN HIDDEN: a path that already contains a
# literal 0x01 byte decodes back as a newline, i.e. this transport cannot tell
# 0x01 from LF. Both are control bytes, so the COUNT and the escaped output stay
# right either way; only a `cat` of that exact path would miss. `tr` is the only
# tool involved, so this needs nothing the trial images do not already ship
# (coreutils in the Debian/Ubuntu images, busybox elsewhere) — unlike `base64`,
# which would also defeat the test harness's container-path rewriting.
MANIFEST_FIND='find /work -maxdepth 4 -name block.manifest.json -not -path "*/node_modules/*" -print0 2>/dev/null | LC_ALL=C tr "\n" "\001" | LC_ALL=C tr "\000" "\n" | LC_ALL=C sort'

# pathdec <varname> <transported-path> — assigns the real path to <varname>.
#
# `printf -v`, not a `$(…)` substitution, and that is load-bearing: command
# substitution strips TRAILING newlines, so a decoded path ending in one would
# come back short. Assigning directly cannot lose a byte.
pathdec() { local _v="${2-}"; printf -v "$1" '%s' "${_v//$'\001'/$'\n'}"; }
