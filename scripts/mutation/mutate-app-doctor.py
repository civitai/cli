#!/usr/bin/env python3
"""Mutation battery for `civitai app doctor`.

Each mutant is the NARROWEST expression that can be wrong. For each:
  apply -> run the FULL relevant packages -> record which tests failed and the
  first assertion line of each -> restore and verify the tree is byte-identical.

Reports SURVIVED loudly. Runs the whole package (never a -run filter that could
exclude the killing test).
"""
import atexit
import hashlib
import os
import re
import subprocess
import sys

# 🔴 A FULL RUN ASSERTS ITS OWN POPULATION, so a table that quietly emptied
# cannot report a serene `killed=0` and exit 0. Lower this in the same commit
# that retires a mutant.
EXPECTED_MUTANTS = 45

WT = os.environ.get("MUTATE_TREE", os.getcwd())
PKGS = ["./internal/cmd/", "./internal/appapi/", "./cmd/civitai/"]

DOCTOR = "internal/cmd/app_doctor.go"
LISTING = "internal/appapi/listing.go"
METRICS = "internal/cmd/app_metrics.go"

# (id, file, old, new, why)
MUTANTS = [
    ("M1-exit-gate", DOCTOR,
     "if payload.Summary.Gating > 0 {",
     "if payload.Summary.Gating > 1 {",
     "off-by-one on the exit gate: one blocking problem stops failing the build"),

    ("M2-severity-compare", DOCTOR,
     "return strings.EqualFold(strings.TrimSpace(p.Severity), appapi.SeverityBlocking)",
     "return strings.EqualFold(strings.TrimSpace(p.Severity), appapi.SeverityAdvisory)",
     "the severity split is inverted"),

    ("M3-ok-field", DOCTOR,
     "out.OK = out.Summary.Gating == 0",
     "out.OK = true",
     "--json `ok` always says the listing is fine"),

    ("M4-icon-remedy", DOCTOR,
     'return "civitai app listing set-icon <file> --slug " + slug',
     'return "civitai app listing set-cover <file> --slug " + slug',
     "missing-icon advises the cover command"),

    ("M5-slug-predicate", DOCTOR,
     "if appapi.SameSlug(slug, r.Slug) {",
     "if slug == r.Slug {",
     "the typed slug is compared byte-for-byte instead of through the shared predicate"),

    ("M6-unknown-slug", DOCTOR,
     "return doctorNoSuchApp(slug, rows)",
     "return nil",
     "an unknown slug reads as a clean run"),

    ("M7-edit-url-id-space", DOCTOR,
     "editURL := editBase + fmt.Sprintf(listingEditPath, r.AppListingID)",
     "editURL := editBase + fmt.Sprintf(listingEditPath, r.Slug)",
     "the listing editor URL is built from the slug, so it 404s"),

    ("M8-empty-sentence", DOCTOR,
     'fmt.Fprintf(w, "No App listings to check — you own none and hold no collaborator seats. %s creates one.\\n",\n\t\t\tst.Code("civitai app submit"))',
     '_ = st',
     "an empty run prints nothing at all"),

    ("M9-clean-sentence", DOCTOR,
     'fmt.Fprintln(w, "  "+st.Success("No problems — this listing is complete."))',
     "_ = st",
     "a clean app renders as a gap instead of a verdict"),

    ("M10-trpc-input", LISTING,
     "\tif input != nil {",
     "\tif true {",
     "an input-less tRPC query is sent with ?input={\"json\":null}"),

    ("M11-extra-request", DOCTOR,
     "rows, err := client.ListMyListings(cmdCtx(cmd))",
     "_, _ = client.GetAssetScanStatuses(cmdCtx(cmd), []int{1})\n\t\t\trows, err := client.ListMyListings(cmdCtx(cmd))",
     "doctor starts asking getAssetScanStatuses (the owner-filtered proc)"),

    ("M12-scanning-remedy", DOCTOR,
     'return "nothing to do — the scan finishes on its own; re-run `civitai app doctor` in a minute"',
     'return "civitai app listing set-icon <file> --slug " + slug',
     "scanning-media advises re-attaching, which restarts the scan"),

    ("M13-unknown-severity", DOCTOR,
     "return strings.EqualFold(strings.TrimSpace(p.Severity), appapi.SeverityBlocking)",
     "return !strings.EqualFold(strings.TrimSpace(p.Severity), appapi.SeverityAdvisory)",
     "an unrecognised severity is promoted to blocking"),

    ("M14-summary-count", DOCTOR,
     "out.Summary.Blocking += len(app.Blocking)",
     "out.Summary.Blocking += 1",
     "the summary counts blocked APPS instead of blocking PROBLEMS"),

    ("M15-exit-tag", DOCTOR,
     'return fmt.Errorf("%w: %d blocking problem(s) on publishable listing(s) — the report above lists them; "+\n\t\t\t"this exit code is the verdict, not a failure of the command",\n\t\t\tErrListingBlocked, payload.Summary.Gating)',
     'return civitai.Tag(civitai.ErrBadRequest, fmt.Errorf("%w: %d blocking problem(s) on publishable listing(s) — the report above lists them; "+\n\t\t\t"this exit code is the verdict, not a failure of the command",\n\t\t\tErrListingBlocked, payload.Summary.Gating))',
     "the verdict is tagged ErrBadRequest, moving its exit code from 1 to 2"),

    ("M16-metrics-403-claim", METRICS,
     '"no token configured — app analytics need %s. Or set CIVITAI_TOKEN to either one"',
     '"no token configured — app analytics need %s; a browser login (`civitai login`) is full-scope-refused with 403 here"',
     "the corrected D2 claim drifts back to the false one"),

    # --- delisted-gating battery (coordinator follow-up 1) ---
    ("M17-gates-inverted", DOCTOR,
     'return !strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)',
     'return strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)',
     "the predicate is inverted: ONLY delisted listings gate"),

    ("M18-gates-always-true", DOCTOR,
     'return !strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)',
     'return true',
     "delisted listings gate again — the defect this rule exists to remove"),

    ("M19-gates-always-false", DOCTOR,
     'return !strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)',
     'return false',
     "NOTHING gates — the exit code can never be 1"),

    ("M20-exit-reads-blocking", DOCTOR,
     "if payload.Summary.Gating > 0 {",
     "if payload.Summary.Blocking > 0 {",
     "the exit gate reads the total instead of the gating subset"),

    ("M21-ok-reads-blocking", DOCTOR,
     "out.OK = out.Summary.Gating == 0",
     "out.OK = out.Summary.Blocking == 0",
     "--json `ok` disagrees with the exit code on a delisted-only run"),

    ("M22-gating-tally-unconditional", DOCTOR,
     "\t\tif gates {\n\t\t\t// The ONLY place the exit code is accumulated.\n\t\t\tout.Summary.Gating += len(app.Blocking)",
     "\t\tif true {\n\t\t\t// The ONLY place the exit code is accumulated.\n\t\t\tout.Summary.Gating += len(app.Blocking)",
     "every app's blocking problems are tallied as gating"),

    ("M23-delisted-rows-hidden", DOCTOR,
     "\tfor _, r := range ordered {",
     "\tfor _, r := range ordered {\n\t\tif !doctorGates(r.Status) {\n\t\t\tcontinue\n\t\t}",
     "delisted apps are DROPPED from the report — the population-becomes-unreachable failure"),

    ("M24-delisted-status-not-normalised", DOCTOR,
     'return !strings.EqualFold(strings.TrimSpace(status), listingStatusRemoved)',
     'return status != listingStatusRemoved',
     "a padded or mis-cased `removed` silently gates again"),

    # 🔴 THE NARROWEST EXPRESSION THAT CAN BE WRONG. Deleting the two grouping
    # loops would remove the guard TOGETHER WITH its enclosing construction and
    # leave `ordered` empty, which dies to a dozen unrelated assertions for the
    # wrong reason. This keeps the ordering machinery and only defeats the SORT,
    # by making the second pass select the same rows as the first.
    ("M25-ordering-dropped", DOCTOR,
     "\tfor _, r := range rows {\n\t\tif !doctorGates(r.Status) {",
     "\tfor _, r := range rows {\n\t\tif false && !doctorGates(r.Status) {",
     "delisted apps are dropped from the ordering pass, so they no longer sort last"),

    ("M26-delisted-heading-dropped", DOCTOR,
     'fmt.Fprintln(w, st.Info("Delisted (status \'removed\') — reported for completeness; these do NOT set the exit code."))',
     "_ = st",
     "the delisted section loses its heading, so a 0 exit beside BLOCKING findings is unexplained"),

    ("M27-summary-explanation-dropped", DOCTOR,
     "\tif payload.Summary.Delisted > 0 {\n\t\tfmt.Fprintf(w, \"%d of them are delisted; %d blocking problem(s) on publishable listings set the exit code.\\n\",\n\t\t\tpayload.Summary.Delisted, payload.Summary.Gating)\n\t}",
     "",
     "the summary stops explaining why blocking and the exit code disagree"),

    # --- kind-aware TEXT remedy (audit finding 2 on #489, same defect class here) ---
    ("M28-onsite-arm-dropped", DOCTOR,
     "\t\tif listingIsOnsite(kind) {",
     "\t\tif false {",
     "an onsite app is told to edit the browser, whose edit (3b-sync) reverts"),

    ("M29-onsite-arm-always", DOCTOR,
     "\t\tif listingIsOnsite(kind) {",
     "\t\tif true {",
     "an offsite app is told to edit a block.manifest.json it does not have"),

    ("M30-kind-not-normalised", DOCTOR,
     'return strings.EqualFold(strings.TrimSpace(kind), "onsite")',
     'return kind == "onsite"',
     "a padded or mis-cased kind silently re-routes the text advice"),

    ("M31-kind-unknown-takes-manifest", DOCTOR,
     'return strings.EqualFold(strings.TrimSpace(kind), "onsite")',
     'return !strings.EqualFold(strings.TrimSpace(kind), "offsite")',
     "an absent/unknown kind takes the manifest arm instead of the recoverable one"),

    ("M32-kind-not-passed-through", DOCTOR,
     "Fix:      doctorRemedy(p, r.Slug, editURL, r.Kind),",
     'Fix:      doctorRemedy(p, r.Slug, editURL, ""),',
     "the row's kind never reaches the remedy, so every app gets the offsite arm"),

    # --- audit finding 1: blocked-media sufficiency per slot ---
    ("M33-screenshot-advice-appends", DOCTOR,
     '\t\t\treturn "REMOVE the blocked screenshot — adding another does not clear it: " +',
     '\t\t\treturn "civitai app listing add-screenshot <file> --slug " + slug + " " + "" +',
     "a blocked SCREENSHOT is answered with add-screenshot, which appends and leaves it blocked"),

    ("M34-blocked-kind-always-empty", DOCTOR,
     '\t\tif blockedKindWord[kind].MatchString(label) {',
     '\t\tif false {',
     "the slot is never extracted, so every blocked asset gets the generic arm"),

    ("M35-fallback-drops-removal", DOCTOR,
     '\t\t\t"civitai app listing rm-screenshot (find the alsc_ id with " +',
     '\t\t\t"civitai app listing add-screenshot (find the alsc_ id with " +',
     "the unknown-label fallback loses the REMOVAL, which is the only sufficient screenshot fix"),

    # --- audit finding 3: the page cap ---
    ("M36-truncation-never-reported", DOCTOR,
     "func doctorPageTruncated(n int) bool { return n >= appapi.ListMineCap }",
     "func doctorPageTruncated(n int) bool { return false }",
     "a truncated page reports ok:true indistinguishably from a complete one"),

    ("M37-truncation-off-by-one", DOCTOR,
     "func doctorPageTruncated(n int) bool { return n >= appapi.ListMineCap }",
     "func doctorPageTruncated(n int) bool { return n > appapi.ListMineCap }",
     "an exactly-at-cap page is called complete, though it is the commonest truncated case"),

    ("M38-truncation-caveat-silent", DOCTOR,
     "\tif !payload.Summary.Truncated {\n\t\treturn\n\t}",
     "\tif true {\n\t\treturn\n\t}",
     "the stderr caveat never prints, so only a JSON reader can learn the page was capped"),

    # --- audit finding 5: --json carries kind ---
    ("M39-json-drops-kind", DOCTOR,
     "\t\t\tKind:         r.Kind,",
     '\t\t\tKind:         "",',
     "--json omits the field that decides `fix`, so a consumer cannot reproduce the branch"),

    # --- delta-audit fixes ---
    ("M40-error-prefix-restored", "cmd/civitai/main.go",
     "\tif errors.Is(err, cmd.ErrListingBlocked) {\n\t\treturn err.Error()\n\t}",
     "\tif false {\n\t\treturn err.Error()\n\t}",
     "the verdict line leads with `Error:` again, which a CI scraper flags as a failure"),

    ("M41-error-prefix-dropped-everywhere", "cmd/civitai/main.go",
     '\treturn "Error: " + err.Error()',
     "\treturn err.Error()",
     "the prefix is suppressed for REAL failures too, hiding them from the same scraper"),

    ("M42-truncation-from-filtered-slice", DOCTOR,
     "\tpayload.Summary.Truncated = doctorPageTruncated(len(rows))",
     "\tpayload.Summary.Truncated = doctorPageTruncated(len(selected))",
     "truncation is computed from the filtered slice again, so a by-slug run is dead-false"),

    ("M43-notfound-hides-the-cap", DOCTOR,
     "\tif doctorPageTruncated(len(rows)) {",
     "\tif false {",
     "a not-found off a capped page is reported as conclusive about an app the caller may own"),

    ("M44-json-kind-hardcoded", DOCTOR,
     "\t\t\tKind:         r.Kind,",
     '\t\t\tKind:         "offsite",',
     "the kind is HARDCODED rather than nulled — the mutant M39 could not see"),

    # 🔴 THE MUTANT MUST SPAN BOTH LINES OR IT CREATES NO DEFECT. The first
    # version replaced only the PROSE and left `"civitai app listing
    # submit-revision --slug " + slug` on the next line — so the string still
    # contained "submit-revision", the assertion still passed, and the sweep
    # reported SURVIVED for a mutation that had not removed anything. A SURVIVED
    # verdict from a mutant that does not produce its own defect is a FALSE
    # ALARM, and is exactly as misleading as a KILLED verdict from a mutant that
    # never ran. Both clauses are deleted here.
    ("M45-screenshot-omits-submit", DOCTOR,
     '\t\t\t\t". On an approved listing that stages a revision, so finish with " +\n'
     '\t\t\t\t"civitai app listing submit-revision --slug " + slug',
     '\t\t\t\t""',
     "the screenshot remedy omits submit-revision, so doctor keeps reporting blocked-media"),
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
    """Return (failed_tests, first assertion line per test).

    🔴 THE ATTRIBUTION IS APPROXIMATE AND THE VERDICTS ARE NOT. This scans
    BACKWARD up to 60 lines from a `--- FAIL` header for any `_test.go:NN:`,
    which cross-attributes across subtests — in one real sweep all four of a
    mutant's reported killers echoed a single subtest's message. Which tests
    FAILED is exact; the REASON printed beside each is a nearby line, not a
    proven cause. Do not quote these as "each killed by an assertion naming the
    specific defect" without reading the run.
    """
    fails = re.findall(r"^\s*--- FAIL: (\S+)", out, re.M)
    detail = {}
    lines = out.split("\n")
    for i, line in enumerate(lines):
        m = re.match(r"^=== RUN\s+(\S+)$", line)
        if not m:
            continue
    # simpler: attribute the first "_test.go:NN:" line following a FAIL header
    for i, line in enumerate(lines):
        m = re.match(r"^\s*--- FAIL: (\S+)", line)
        if not m:
            continue
        name = m.group(1)
        # the assertion lines are printed BEFORE the FAIL header in go test -v
        for j in range(i - 1, max(0, i - 60), -1):
            mm = re.search(r"(\w+_test\.go:\d+:\s*.*)", lines[j])
            if mm:
                detail[name] = mm.group(1)[:300]
                break
    counts = {
        "PASS": len(re.findall(r"^\s*--- PASS:", out, re.M)),
        "FAIL": len(fails),
        "SKIP": len(re.findall(r"^\s*--- SKIP:", out, re.M)),
        # 🔴 A RUN CAN DIE PART-WAY AND STILL LOOK KILLED. One mutant reported
        # PASS=766 against a 3049 baseline — ~2280 tests produced no verdict at
        # all, almost certainly a panic. It was legitimately KILLED (its killers
        # fire before the death), but nothing here could tell "31 tests failed"
        # from "the suite stopped a third of the way in", and the second is a
        # result about the harness rather than about the code.
        "PANIC": len(re.findall(r"^panic:", out, re.M)),
    }
    build_err = "build failed" if "[build failed]" in out or "cannot use" in out else ""
    return fails, detail, counts, build_err


def main():
    # 🔴 THE SELECTOR USED TO MATCH FULL IDS ONLY, so the invocation the README
    # itself taught (`… .py M1 M7`) selected NOTHING, ran nothing, and fell
    # through to `return 0` — which the README defines as "every mutant ran,
    # compiled, and was killed". An instrument that reports success having run
    # nothing is the whole failure class this harness exists to avoid, and it
    # was in the harness. It now accepts a PREFIX, and refuses a name that
    # matches no mutant instead of quietly doing nothing.
    only = sys.argv[1:] if len(sys.argv) > 1 else None
    if only:
        known = [m[0] for m in MUTANTS]
        unmatched = [a for a in only
                     if not any(k == a or k.startswith(a + "-") for k in known)]
        if unmatched:
            print(f"!! these selectors match no mutant: {unmatched}")
            print(f"   known: {known}")
            return 2
    base_out = run_tests()
    _, _, base_counts, _ = parse(base_out)
    base_total = base_counts["PASS"] + base_counts["FAIL"] + base_counts["SKIP"]
    print(f"BASELINE: {base_counts} total={base_total}")
    if base_counts["FAIL"] != 0 or "[build failed]" in base_out:
        # A baseline that does not COMPILE is a bad baseline, not a "mutant that
        # never ran" — it takes 3 like any other, which is what the README says.
        print("!! baseline is not green (or does not build) — every result below is meaningless")
        return 3

    # 🔴 A FULL RUN ASSERTS ITS OWN POPULATION. Without this a table that
    # quietly emptied — or a selector bug like the one above — reports a serene
    # `killed=0 survived=0` and exits 0.
    if not only and len(MUTANTS) < EXPECTED_MUTANTS:
        print(f"!! this battery defines {len(MUTANTS)} mutants, expected at least {EXPECTED_MUTANTS}. "
              f"If mutants were RETIRED, lower EXPECTED_MUTANTS in the same commit that deletes them.")
        return 2

    results = []
    for mid, rel, old, new, why in MUTANTS:
        if only and not any(mid == a or mid.startswith(a + "-") for a in only):
            continue
        path = os.path.join(WT, rel)
        before = sha(path)
        src = open(path).read()
        n = src.count(old)
        if n != 1:
            print(f"{mid}: BAD-PATTERN — matched {n} times (want exactly 1), NOT RUN. "
                  f"The mutant's search text is stale against the current source.")
            results.append((mid, "BAD-PATTERN", "", why))
            continue
        open(path, "w").write(src.replace(old, new, 1))
        out = run_tests()
        fails, detail, counts, build_err = parse(out)
        open(path, "w").write(src)
        assert sha(path) == before, f"{mid}: tree not restored!"
        # 🔴 BUILD ERROR WINS OVER `fails`. This was `build_err and not fails`,
        # so a mutant that failed to COMPILE while some unrelated test also
        # reported a failure scored KILLED — a build error is not evidence about
        # the code under test, whatever else the run printed.
        if build_err:
            verdict = "BUILD-FAIL"
        elif fails:
            verdict = "KILLED"
        else:
            verdict = "SURVIVED"
        killers = "; ".join(f"{f} :: {detail.get(f,'(no line)')}" for f in fails[:4])
        # Compare this run's TOTAL against the baseline's. A large shortfall
        # means tests never ran, which no verdict word conveys on its own.
        total = counts["PASS"] + counts["FAIL"] + counts["SKIP"]
        short = base_total - total
        note = ""
        if counts["PANIC"] or short > base_total * 0.05:
            note = (f"  ⚠ TRUNCATED RUN: {total} verdicts vs baseline {base_total}"
                    f" ({short} missing, panics={counts['PANIC']}) — the verdict below is about a run"
                    f" that did not finish; treat the KILL as unattributed.")
        # 🔴 `note` GOES IN THE TUPLE. It used to be a local printed to the
        # stream only, so the `==== TABLE ====` at the end listed a truncated
        # run as a plain unmarked KILLED and `ran`/`killed` counted it like any
        # other. A reader recounting from the committed artifact saw no trace.
        if note:
            verdict += " (TRUNCATED)"
        results.append((mid, verdict, killers, why))
        print(f"{mid}: {verdict} ({counts})")
        if note:
            print(note)
        if verdict == "KILLED":
            for f in fails[:4]:
                print(f"    <- {f}\n       {detail.get(f,'(no assertion line captured)')}")
        elif verdict == "SURVIVED":
            print(f"    !! SURVIVED: {why}")

    bad = [r for r in results if r[1] == "BAD-PATTERN"]
    surv = [r for r in results if r[1] == "SURVIVED"]
    bf = [r for r in results if r[1] == "BUILD-FAIL"]
    print("\n==== TABLE ====")
    for mid, verdict, killers, why in results:
        print(f"{mid}\t{verdict}\t{why}\n\t{killers}")
    # 🔴 `ran` EXCLUDES EVERY TERM THAT IS EVIDENCE OF NOTHING. It used to
    # subtract only BAD-PATTERN while still counting BUILD-FAILs, so the summary
    # could print `ran=39 killed=38 build_fail=1` — arithmetic disagreeing with
    # itself. A mutant that never compiled did not run.
    killed = len([r for r in results if r[1].startswith("KILLED")])
    trunc = len([r for r in results if "TRUNCATED" in r[1]])
    print(f"\n==== VERDICT ==== defined={len(results)} ran={len(results)-len(bad)-len(bf)} "
          f"killed={killed} survived={len(surv)} build_fail={len(bf)} not_run={len(bad)} "
          f"truncated={trunc}")
    if bad:
        print("!! NOT A CLEAN SWEEP — these mutants NEVER RAN, so they are evidence of nothing:")
        for mid, _, _, why in bad:
            print(f"     {mid}: {why}")
        return 2
    if bf:
        # A mutant that does not COMPILE tested nothing either. It used to be
        # counted outside `killed` while the process still exited 0, so the
        # summary's own arithmetic disagreed with itself.
        print("!! these mutants did not COMPILE, so they are evidence of nothing:")
        for mid, _, _, why in bf:
            print(f"     {mid}: {why}")
        return 2
    if surv:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
