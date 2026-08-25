#!/usr/bin/env python3
"""Mutation battery for `civitai app doctor`.

Each mutant is the NARROWEST expression that can be wrong. For each:
  apply -> run the FULL relevant packages -> record which tests failed and the
  first assertion line of each -> restore and verify the tree is byte-identical.

Reports SURVIVED loudly. Runs the whole package (never a -run filter that could
exclude the killing test).
"""
import hashlib
import os
import re
import subprocess
import sys

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
     '\t\t\treturn "civitai app listing add-screenshot <file> --slug " + slug + " " +',
     "a blocked SCREENSHOT is answered with add-screenshot, which appends and leaves it blocked"),

    ("M34-blocked-kind-always-empty", DOCTOR,
     '\tl := strings.ToLower(label)',
     '\tl := ""',
     "the slot is never extracted, so every blocked asset gets the generic arm"),

    ("M35-fallback-drops-removal", DOCTOR,
     '\t\t"civitai app listing set-icon, civitai app listing set-cover, civitai app listing rm-screenshot"',
     '\t\t"civitai app listing set-icon, civitai app listing set-cover, civitai app listing add-screenshot"',
     "the unknown-label fallback loses the REMOVAL, which is the only sufficient screenshot fix"),

    # --- audit finding 3: the page cap ---
    ("M36-truncation-never-reported", DOCTOR,
     "\tout.Summary.Truncated = len(rows) >= appapi.ListMineCap",
     "\tout.Summary.Truncated = false",
     "a truncated page reports ok:true indistinguishably from a complete one"),

    ("M37-truncation-off-by-one", DOCTOR,
     "\tout.Summary.Truncated = len(rows) >= appapi.ListMineCap",
     "\tout.Summary.Truncated = len(rows) > appapi.ListMineCap",
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
]


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
    only = sys.argv[1:] if len(sys.argv) > 1 else None
    base_out = run_tests()
    _, _, base_counts, _ = parse(base_out)
    base_total = base_counts["PASS"] + base_counts["FAIL"] + base_counts["SKIP"]
    print(f"BASELINE: {base_counts} total={base_total}")
    if base_counts["FAIL"] != 0:
        print("!! baseline is not green — every result below is meaningless")
        return 3

    results = []
    for mid, rel, old, new, why in MUTANTS:
        if only and mid not in only:
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
        if build_err and not fails:
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
    print("\n==== TABLE ====")
    for mid, verdict, killers, why in results:
        print(f"{mid}\t{verdict}\t{why}\n\t{killers}")
    print(f"\n==== VERDICT ==== ran={len(results)-len(bad)} killed="
          f"{len([r for r in results if r[1]=='KILLED'])} survived={len(surv)} not_run={len(bad)}")
    bf = [r for r in results if r[1] == "BUILD-FAIL"]
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
