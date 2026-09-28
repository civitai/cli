#!/usr/bin/env python3
"""Mutation battery for the re-vendor bot's GLOSS-DEBT SIGNAL (#749).

Target: `.github/workflows/revendor-canonical-schema.yml` — the `owed` step that
produces the debt count, the `classify` arm that judges it, and the `signal` job
input that decides whether the resulting issue is FILED or CLOSED. Guarded by
`revendor_owed_signal_test.go`, which extracts each step's `run:` script verbatim
and executes it.

Same harness discipline as the sibling batteries, with two deliberate departures,
both because the target is YAML and shell rather than Go:

 1. 🔴 `scripts/mutate.sh` (gremlins) CANNOT REACH THIS CODE AT ALL. It mutates Go
    OPERATORS — not string literals, not constants, and certainly not a shell
    script embedded in a workflow. A green `mutate.sh` run says nothing whatever
    about these guards, which is why this hand-written battery exists.

 2. 🔴 THE BASELINE IS NOT GREEN IN THIS ENVIRONMENT, AND THE HARNESS SAYS SO
    RATHER THAN FILTERING. Three root-package oracle tests fail without a
    Chromium on PATH. The sibling batteries exit 3 on any baseline failure; a
    `-run` filter is the tempting fix and is the one the README forbids, because
    it can exclude the killing test. Instead the baseline's failure SET is
    recorded, any failure outside `BASELINE_ALLOWED_FAILURES` is still a hard
    exit 3, and a mutant counts as KILLED only on a failure the baseline did not
    already have. The full package runs every time.

⚠️ ONE MUTATION IS DELIBERATELY NOT IN THE TABLE, and it is recorded here rather
than quietly dropped, because it SURVIVES:

    deleting `*[!0-9]*` from the `owed` step's `case "$count" in ''|*[!0-9]*)`

`jq '.owed | length'` returns a number or fails, so no ledger file can make that
step emit something non-empty and non-numeric — the alternative is unreachable AT
THAT SITE and the suite stays green without it. It is kept in the workflow because
it costs nothing and because the same predicate in `classify` IS reachable (the
value crosses a step-output boundary jq does not control); that one is R5 below
and it dies. Reproduce the survivor by hand:

    python3 - <<'EOF'
    p=".github/workflows/revendor-canonical-schema.yml"
    s=open(p).read(); open(p,"w").write(s.replace("            ''|*[!0-9]*)\\n",
                                                  "            ''(\\n",1))
    EOF

Do not add it to MUTANTS to "document" it: exit 1 means a real coverage hole, and
a battery with a permanent expected survivor is one people learn to read past.
"""
import atexit
import hashlib
import os
import re
import subprocess
import sys

# 🔴 A FULL RUN ASSERTS ITS OWN POPULATION — a table that quietly emptied must
# not report a serene killed=0.
EXPECTED_MUTANTS = 8

WT = os.environ.get("MUTATE_TREE", os.getcwd())

# The whole root package, never a `-run` filter. All four guards live here, and a
# filter is how a killing test gets excluded from its own battery.
PKGS = ["."]

YML = ".github/workflows/revendor-canonical-schema.yml"

# 🔴 Failures the baseline is ALLOWED to have, and nothing else. These are an
# environment gap, not a defect: `dogfood_oracle_test.go` needs a Chromium on
# PATH (`CIVITAI_CHROME`). Identical at `origin/main`. Anything failing outside
# this set means the tree is broken and every verdict below would be noise.
BASELINE_ALLOWED_FAILURES = {
    "TestOracleRefusesWhenNothingCanBeMeasured",
    "TestOracleRefusesWhenNothingCanBeMeasured/container_not_running",
    "TestOracleRefusesWhenNothingCanBeMeasured/no_node_in_the_container",
    "TestOracleRefusesWhenNothingCanBeMeasured/no_such_container",
    "TestOracleReportsNoAppAsAVerdictNotAHarnessError",
    "TestOracleSaysTheAppWasNeverBuilt",
}

MUTANTS = [
    # ── the PRODUCER: `owed` must not go green on a count it did not produce ──
    # ⚠ R1 pins SELF-CONTAINMENT, not the live defect. GitHub already runs `run:`
    # blocks as `bash -e {0}`, so on a real runner removing `set -e` from the
    # script changes nothing by itself — the reason the explicit flag is here is
    # that a `shell:` override on the step, or a `defaults.run.shell`, would
    # silently take errexit away. This mutant asserts the script does not depend
    # on the platform's default flags. The mutant IS productive: the harness runs
    # the script itself, not GitHub.
    ("R1-owed-drop-set-e", YML,
     "          set -euo pipefail\n          ledger=",
     "          set -uo pipefail\n          ledger=",
     "the script stops carrying its own errexit and depends on the runner's "
     "default shell flags — an absent or unparseable ledger then publishes an "
     "empty count instead of failing"),

    ("R2-owed-case-drop-empty-alt", YML,
     "            ''|*[!0-9]*)\n",
     "            *[!0-9]*)\n",
     "🔴 THE LIVE HOLE: a ZERO-BYTE or whitespace-only ledger publishes an empty "
     "count, because jq exits 0 printing nothing and errexit has nothing to trip "
     "on — this alternative is the only guard in front of it"),

    ("R3-owed-publish-before-assert", YML,
     "          count=\"$(jq '.owed | length' \"$ledger\")\"\n          case \"$count\" in",
     "          count=\"$(jq '.owed | length' \"$ledger\")\"\n"
     "          echo \"count=$count\" >> \"$GITHUB_OUTPUT\"\n          case \"$count\" in",
     "the count reaches GITHUB_OUTPUT before it is validated, so a bad value is "
     "readable downstream whatever this step's exit code"),

    # ── the CONSUMER: `classify` must not default an unproduced count to 0 ──
    ("R4-classify-drop-empty-alt", YML,
     "            ''|*[!0-9]*) owed_count_ok=no ;;",
     "            *[!0-9]*) owed_count_ok=no ;;",
     "an EMPTY count is accepted, restoring the `${R_OWED_COUNT:-0}` behaviour that "
     "made an unproduced count read as `0 owed`"),

    ("R5-classify-drop-numeric-alt", YML,
     "            ''|*[!0-9]*) owed_count_ok=no ;;",
     "            '') owed_count_ok=no ;;",
     "a non-empty, non-numeric count is accepted; it is `!= \"0\"`, so the bot files a "
     "gloss-owed issue whose body renders the garbage as the number of patterns owed"),

    ("R6-classify-drop-R_OWED-half", YML,
     'elif [ "$R_OWED" != "success" ] || [ "$owed_count_ok" != "yes" ]; then',
     'elif [ "$owed_count_ok" != "yes" ]; then',
     "a FAILED owed step with a stale-but-well-formed count is judged on that count, "
     "so a step that died is reported as `gloss-owed`"),

    # ── the SEAM: classify must be able to SEE the step at all ──
    ("R7-classify-env-wrong-step", YML,
     "R_OWED: ${{ steps.owed.outcome }}",
     "R_OWED: ${{ steps.probe.outcome }}",
     "the arm branches on ANOTHER step's outcome — an env var that reads as coverage "
     "and provides none, which is the shape of the original missing-env defect"),

    # ── the LAST LINK: a correct verdict still closes the issue without this ──
    ("R8-signal-drop-owed-failed", YML,
     " || needs.revendor.outputs.state == 'owed-failed' }}",
     " }}",
     "`owed-failed` is reachable on a GREEN job, so dropping it from `failing:` makes "
     "failure-issue.yml CLOSE the debt's own issue over a correctly-computed verdict"),
]


# Files currently mutated, so an abnormal exit can put them back.
_restore_queue = {}


def _restore_all():
    for path, src in list(_restore_queue.items()):
        try:
            open(path, "w").write(src)
        except OSError:
            pass


atexit.register(_restore_all)


def sha(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def run_tests():
    p = subprocess.run(["go", "test", "-count=1", "-v"] + PKGS,
                       cwd=WT, capture_output=True, text=True)
    return p.stdout + p.stderr


def parse(out):
    """🔴 KILLER ATTRIBUTION IS APPROXIMATE; VERDICTS ARE NOT. The backward scan
    for a nearby `_test.go:NN:` cross-attributes across subtests. Which tests
    FAILED is exact; the reason printed beside each is a nearby line."""
    fails = re.findall(r"^\s*--- FAIL: (\S+)", out, re.M)
    lines = out.split("\n")
    detail = {}
    for i, line in enumerate(lines):
        m = re.match(r"^\s*--- FAIL: (\S+)", line)
        if not m:
            continue
        for j in range(i - 1, max(0, i - 60), -1):
            mm = re.search(r"(\w+_test\.go:\d+:\s*.*)", lines[j])
            if mm:
                detail[m.group(1)] = mm.group(1)[:260]
                break
    counts = {
        "PASS": len(re.findall(r"^\s*--- PASS:", out, re.M)),
        "FAIL": len(fails),
        # 🔴 A run can DIE part-way and still look KILLED.
        "PANIC": len(re.findall(r"^panic:", out, re.M)),
    }
    built = "[build failed]" not in out
    return fails, detail, counts, built


def main():
    # 🔴 PREFIX SELECTOR AT THE `-` BOUNDARY, AND A BOGUS NAME REFUSES. A selector
    # matching nothing must never exit 0, which this file would otherwise read as
    # "every mutant ran, compiled, and was killed".
    only = sys.argv[1:] or None
    known = [m[0] for m in MUTANTS]
    if only:
        unmatched = [x for x in only
                     if not any(k == x or k.startswith(x + "-") for k in known)]
        if unmatched:
            print(f"!! these selectors match no mutant: {unmatched}")
            print(f"   known: {known}")
            return 2

    base = run_tests()
    base_fails, _, bc, built = parse(base)
    base_total = bc["PASS"] + bc["FAIL"]
    base_set = set(base_fails)
    print(f"BASELINE: {bc} total={base_total} built={built}")
    if not built:
        print("!! baseline does not BUILD — every result below is meaningless")
        return 3
    unexpected = sorted(base_set - BASELINE_ALLOWED_FAILURES)
    if unexpected:
        print("!! baseline fails tests outside BASELINE_ALLOWED_FAILURES, so the tree is "
              f"broken and every result below is noise: {unexpected}")
        return 3
    if base_set:
        print(f"   baseline has {len(base_set)} ALLOWED failure(s) (env gap: no Chromium on "
              "PATH). A mutant counts as KILLED only on a failure NOT in this set.")
    # Positive control on the allow-list itself: if it has silently grown to
    # swallow the guards' own tests, every mutant would read as SURVIVED.
    swallowed = sorted(t for t in BASELINE_ALLOWED_FAILURES if t.startswith("TestRevendor"))
    if swallowed:
        print(f"!! BASELINE_ALLOWED_FAILURES swallows this battery's own tests: {swallowed}")
        return 2

    if not only and len(MUTANTS) < EXPECTED_MUTANTS:
        print(f"!! this battery defines {len(MUTANTS)} mutants, expected at least "
              f"{EXPECTED_MUTANTS}.")
        return 2

    results = []
    for mid, rel, old, new, why in MUTANTS:
        if only and not any(mid == x or mid.startswith(x + "-") for x in only):
            continue
        path = os.path.join(WT, rel)
        before = sha(path)
        src = open(path).read()
        n = src.count(old)
        if n != 1:
            print(f"{mid}: BAD-PATTERN — matched {n} times (want exactly 1), NOT RUN.")
            results.append((mid, "BAD-PATTERN", "", why))
            continue
        # 🔴 Restore is GUARANTEED, not hoped for.
        _restore_queue[path] = src
        try:
            open(path, "w").write(src.replace(old, new, 1))
            if sha(path) == before:
                raise SystemExit(f"{mid}: the mutation changed no bytes — it is a no-op")
            out = run_tests()
            fails, detail, counts, built = parse(out)
        finally:
            open(path, "w").write(src)
            _restore_queue.pop(path, None)
        # A real check, not `assert` — bare asserts vanish under `python3 -O`.
        if sha(path) != before:
            raise SystemExit(f"{mid}: tree NOT restored — {path} differs from its "
                             "pre-mutation bytes")
        new_fails = [f for f in fails if f not in base_set]
        if not built:
            verdict = "BUILD-FAIL"
        elif new_fails:
            verdict = "KILLED"
        else:
            verdict = "SURVIVED"
        total = counts["PASS"] + counts["FAIL"]
        short = base_total - total
        results.append((mid, verdict, "; ".join(
            f"{f} :: {detail.get(f, '(no line)')}" for f in new_fails[:3]), why))
        print(f"{mid}: {verdict} ({counts}) new_failures={len(new_fails)}")
        if counts["PANIC"] or short > base_total * 0.05:
            # 🔴 The mark goes in the VERDICT so it reaches the committed table.
            results[-1] = (results[-1][0], results[-1][1] + " (TRUNCATED)",
                           results[-1][2], results[-1][3])
            print(f"  ⚠ TRUNCATED RUN: {total} verdicts vs baseline {base_total}"
                  f" ({short} missing, panics={counts['PANIC']}) — treat the KILL as "
                  "unattributed.")
        for f in new_fails[:3]:
            print(f"    <- {f}\n       {detail.get(f, '(no assertion line)')}")
        if verdict == "SURVIVED":
            print(f"    !! SURVIVED: {why}")

    bad = [r for r in results if r[1] == "BAD-PATTERN"]
    surv = [r for r in results if r[1] == "SURVIVED"]
    bf = [r for r in results if r[1] == "BUILD-FAIL"]
    print("\n==== TABLE ====")
    for mid, verdict, killers, why in results:
        print(f"{mid}\t{verdict}\t{why}\n\t{killers}")
    killed = len([r for r in results if r[1].startswith("KILLED")])
    trunc = len([r for r in results if "TRUNCATED" in r[1]])
    print(f"\n==== VERDICT ==== defined={len(results)} "
          f"ran={len(results)-len(bad)-len(bf)} killed={killed} survived={len(surv)} "
          f"build_fail={len(bf)} not_run={len(bad)} truncated={trunc}")
    if bad:
        print("!! NOT A CLEAN SWEEP — these NEVER RAN and are evidence of nothing:")
        for mid, _, _, why in bad:
            print(f"     {mid}: {why}")
        return 2
    if bf:
        print("!! these mutants did not COMPILE, so they are evidence of nothing:")
        for mid, _, _, why in bf:
            print(f"     {mid}: {why}")
        return 2
    return 1 if surv else 0


if __name__ == "__main__":
    sys.exit(main())
