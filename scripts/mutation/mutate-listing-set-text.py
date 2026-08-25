#!/usr/bin/env python3
"""Mutation battery for `civitai app listing set-text`.

Same harness discipline as the doctor battery, with the one lesson it taught
baked in: a mutant whose search text no longer matches is reported as
BAD-PATTERN with the SAME verdict vocabulary as every other outcome, and the
process exits non-zero, so a run containing one can never be read as clean.
"""
import atexit
import hashlib
import os
import re
import subprocess
import sys

# 🔴 A FULL RUN ASSERTS ITS OWN POPULATION — see the sibling battery.
EXPECTED_MUTANTS = 38

WT = os.environ.get("MUTATE_TREE", os.getcwd())
PKGS = ["./internal/cmd/", "./internal/appapi/"]
API2 = "internal/appapi/listing.go"

CMD = "internal/cmd/app_listing_set_text.go"
API = "internal/appapi/listing.go"

MUTANTS = [
    # --- the tri-state on the wire: the contract the command exists for ---
    ("S1-omitempty-collapse", API,
     '\tif p.Tagline != nil {\n\t\tout["tagline"] = *p.Tagline\n\t}',
     '\tif p.Tagline != nil && *p.Tagline != "" {\n\t\tout["tagline"] = *p.Tagline\n\t}',
     "an explicit empty tagline is dropped — `--tagline \"\"` becomes a silent no-op"),

    ("S2-clear-sends-empty-not-null", API,
     '\tif p.ClearTagline {\n\t\tout["tagline"] = nil\n\t}',
     '\tif p.ClearTagline {\n\t\tout["tagline"] = ""\n\t}',
     "`--clear` empties the column instead of nulling it — the two states collapse"),

    ("S3-changed-becomes-nonempty", CMD,
     'if f.Changed("tagline") {',
     'if tagline != "" {',
     "flag presence is inferred from the VALUE, so `--tagline \"\"` is dropped"),

    ("S4-patch-always-sends-all", API,
     '\tif p.Description != nil {\n\t\tout["description"] = *p.Description\n\t}',
     '\tout["description"] = ""\n\tif p.Description != nil {\n\t\tout["description"] = *p.Description\n\t}',
     "an omitted field is sent anyway, overwriting a column the user never named"),

    # --- category validation ---
    ("S5-category-any-value", CMD,
     "\tif contains(appapi.MarketplaceCategories, v) {\n\t\treturn nil\n\t}",
     "\tif true {\n\t\treturn nil\n\t}",
     "any category string is accepted, so the local mirror validates nothing"),

    # 🔴 The NARROWEST expression that can be wrong: lowercase the input at the
    # membership test only. No new helper is introduced (an unused one would be a
    # lint failure and would not be a mutation of the real code path), and the
    # value still SENT is the caller's original spelling — so the mutant makes the
    # CLI wave through a string the server's z.enum will reject.
    ("S6-category-case-insensitive", CMD,
     "\tif contains(appapi.MarketplaceCategories, v) {",
     "\tif contains(appapi.MarketplaceCategories, strings.ToLower(v)) {",
     "category is matched case-insensitively, but the server compares exactly"),

    ("S7-empty-category-allowed", CMD,
     '\tif v == "" {\n\t\treturn asUsageError(fmt.Errorf(\n\t\t\t"--category cannot be empty',
     '\tif false {\n\t\treturn asUsageError(fmt.Errorf(\n\t\t\t"--category cannot be empty',
     "an empty category is sent to the server instead of refused locally"),

    ("S8-category-list-dropped-value", API,
     '\t"discovery",\n',
     '',
     "the category mirror loses a value the server accepts"),

    # --- local refusals ---
    ("S9-empty-patch-allowed", CMD,
     "\tif p.Empty() {",
     "\tif false {",
     "a run with no field at all is sent to the server as an empty patch"),

    ("S10-tagline-cap-off-by-one", CMD,
     "if n := len([]rune(tagline)); n > appapi.MaxTaglineRunes {",
     "if n := len([]rune(tagline)); n > appapi.MaxTaglineRunes+1 {",
     "the tagline cap is off by one"),

    ("S11-cap-counts-bytes", CMD,
     "if n := len([]rune(tagline)); n > appapi.MaxTaglineRunes {",
     "if n := len(tagline); n > appapi.MaxTaglineRunes {",
     "the cap counts bytes, so a multi-byte tagline within the limit is refused"),

    ("S12-contradiction-allowed", CMD,
     "\t\tif seen[name] && f.Changed(name) {",
     "\t\tif false && seen[name] && f.Changed(name) {",
     "`--tagline X --clear tagline` is silently resolved instead of refused"),

    ("S13-unknown-clear-field-ignored", CMD,
     "\t\tif !contains(setTextFieldNames, name) {",
     "\t\tif false && !contains(setTextFieldNames, name) {",
     "a typo in --clear is silently ignored, so the field is never cleared"),

    # --- reporting ---
    ("S14-requires-review-ignored", CMD,
     "\tif res != nil && res.RequiresReview {",
     "\tif false {",
     "the server staging a revision is not reported — the success line becomes false"),

    # S15/S16 RETIRED: they mutated `if ref != nil && ref.HasPendingRevision`,
    # which the audit showed was the DEFECT — that gate could not see an open,
    # unsubmitted shadow. The gate is now `ShadowID != nil || HasPendingRevision`
    # and its two directions are covered by S19 (dropped/narrowed) and S21
    # (always fires). Deleted rather than left stale: a mutant whose pattern no
    # longer matches is reported NOT-RUN, and a battery carrying permanent
    # not-runs is one people learn to read past.

    ("S17-empty-vs-cleared-line-same", CMD,
     '\tif v == "" {\n\t\treturn field + " set to an empty string',
     '\tif false {\n\t\treturn field + " set to an empty string',
     "the success line stops distinguishing an empty string from a cleared field"),

    # --- targeting ---
    ("S18-writes-wrong-listing-key", API,
     'in := map[string]any{"listingId": listingID, "patch": patch.wire()}',
     'in := map[string]any{"shadowId": listingID, "patch": patch.wire()}',
     "the write is addressed with the shadow proc's key, which updateListing refuses"),

    # --- audit finding 1: the shadow-overwrite warning ---
    ("S19-shadow-gate-dropped", CMD,
     "\tif ref != nil && (ref.ShadowID != nil || ref.HasPendingRevision) {",
     "\tif ref != nil && ref.HasPendingRevision {",
     "the warning reverts to the SUBMITTED-only gate and cannot see an open, unsubmitted shadow"),

    ("S20-shadowid-not-decoded", API,
     '\tShadowID *string `json:"shadowId"`',
     '\tShadowID *string `json:"-"`',
     "shadowId stops being decoded, so the open-shadow warning goes silent"),

    ("S21-shadow-gate-always", CMD,
     "\tif ref != nil && (ref.ShadowID != nil || ref.HasPendingRevision) {",
     "\tif ref != nil {",
     "the warning fires on every run, training the reader to ignore it"),

    # --- audit finding 2: the kind gate ---
    ("S22-kind-gate-removed", CMD,
     "\t\t\tif err := refuseOnsiteTextEdit(ctx, client, slug); err != nil {",
     "\t\t\tif err := error(nil); err != nil {",
     "an onsite listing is written, and the platform reverts it at the next approve"),

    ("S23-kind-gate-inverted", CMD,
     '\t\tif kind == appapi.ListingKindOnsite {',
     '\t\tif kind != appapi.ListingKindOnsite {',
     "offsite is refused and onsite is written — the gate exactly backwards"),

    ("S24-kind-fail-open", CMD,
     '\t\tif kind == "" {',
     '\t\tif false {',
     "an unestablished kind proceeds instead of refusing"),

    ("S25-kind-not-normalised", CMD,
     "\t\tkind := strings.ToLower(strings.TrimSpace(r.Kind))",
     "\t\tkind := r.Kind",
     "a padded or mis-cased kind slips past the onsite refusal"),

    ("S26-unlisted-slug-fails-open", CMD,
     '\treturn civitai.Tag(civitai.ErrNotFound, fmt.Errorf(\n\t\t"no listing of yours is called %q',
     '\treturn nil\n}\n\nfunc unusedRefuse(slug string) error {\n\treturn civitai.Tag(civitai.ErrNotFound, fmt.Errorf(\n\t\t"no listing of yours is called %q',
     "a slug listMine does not list proceeds to the write instead of refusing"),

    # --- audit finding 6: the blank-set guard ---
    ("S27-blank-guard-removed", CMD,
     "\tif !assumeYes {",
     "\tif false {",
     "a blank set empties a public field with no confirmation"),

    ("S28-blank-guard-ignores-whitespace", CMD,
     '\t\t\tif f.val != nil && strings.TrimSpace(*f.val) == "" {',
     '\t\t\tif f.val != nil && *f.val == "" {',
     "a whitespace-only value is treated as real, though the server trims it to empty"),

    ("S29-blank-guard-blocks-yes", CMD,
     "\tif !assumeYes {",
     "\tif true {",
     "--yes stops working, making `set to an empty string` unreachable"),

    # --- audit finding 6: --json ---
    ("S30-json-reports-blank-as-set", CMD,
     '\t\tcase strings.TrimSpace(*v) == "":\n\t\t\tout.Fields[k] = "empty"',
     '\t\tcase false:\n\t\t\tout.Fields[k] = "empty"',
     "--json reports a whitespace-only write as `set`, which doctor will contradict"),

    ("S31-json-openrevision-dropped", CMD,
     "\t\tout.OpenRevision = ref.ShadowID != nil || ref.HasPendingRevision",
     "\t\tout.OpenRevision = false",
     "--json never reports that an open revision would overwrite the edit"),

    # --- audit finding 3: the category mirror's CONTENTS ---
    ("S32-category-misspelled", API,
     '\t"discovery",',
     '\t"discovry",',
     "a hand-copied category is MISSPELLED — the likelier drift than deletion, and it INVERTS: "
     "`--category discovery` is then refused locally and the refusal names the typo as the allowed value"),

    ("S33-category-reordered", API,
     '\t"generation",\n\t"games",',
     '\t"games",\n\t"generation",',
     "the mirror's ORDER drifts, changing what every refusal prints"),

    # 🔴 `%w` -> `%v`, NOT "delete the verb". The first draft removed `%w:` and
    # left `ErrOnsiteTextNotEditable` in the argument list, so `go vet`'s printf
    # check rejected it and the mutant BUILD-FAILED — evidence of nothing, and a
    # mutant that cannot compile tests nothing at all. `%v` keeps the arity, so
    # it compiles and produces exactly the defect under test: the sentinel is
    # FORMATTED rather than WRAPPED, the message is byte-identical, and
    # `errors.Is` stops matching — which is precisely how an exit code drifts
    # while every prose assertion in the suite still passes.
    ("S34-onsite-refusal-reclassified", CMD,
     '\t\t\t\t"%w: %q is an ON-SITE app',
     '\t\t\t\t"%v: %q is an ON-SITE app',
     "the sentinel is formatted, not wrapped, so errors.Is fails and the exit code is unpinned"),

    # --- #489 delta-audit fixes ---
    ("S35-read-arm-blames-a-named-value", API,
     '\t\t\treturn fmt.Errorf("the server rejected this store-listing lookup (400): %s — nothing was changed; `civitai app doctor` lists every app you can work on", msg)',
     '\t\t\treturn fmt.Errorf("the server rejected this store-listing lookup (400): %s — nothing was changed; check the app you named (list your apps with `civitai app status`)", msg)',
     "the shared READ arm blames a value listMine never sends, and points at a route that cannot list a seat-held listing"),

    ("S36-capped-notfound-stated-as-fact", CMD,
     "\tif len(rows) >= appapi.ListMineCap {",
     "\tif false {",
     "a miss off a CAPPED page is reported as conclusive, hard-blocking a write on an app the caller owns"),

    ("S37-cap-off-by-one", CMD,
     "\tif len(rows) >= appapi.ListMineCap {",
     "\tif len(rows) > appapi.ListMineCap {",
     "an exactly-at-cap page is called complete, though a clamp produces exactly that"),

    ("S38-advisory-skips-the-human-path", CMD,
     '\t\tfmt.Fprintf(out, "  Send it for review: %s\\n", st.Code("civitai app listing submit-revision"))',
     '\t\tfmt.Fprintf(out, "  Send it for review: %s\\n", st.Code("civitai app listing submit-revision"))\n\t\treturn',
     "the human rendering returns early and loses the overwrite advisory that --json still reports"),

    ("S39-unknown-kind-refusal-untagged", CMD,
     '\t\t\t\t"%w: could not establish whether %q is an on-site or off-site app',
     '\t\t\t\t"%v: could not establish whether %q is an on-site or off-site app',
     "the unknown-kind refusal formats its sentinel instead of wrapping it, unpinning the exit code"),

    ("S40-fake-fidelity-guard-blind", "internal/cmd/listing_ref_fake_fidelity_test.go",
     '\t\tif !strings.Contains(m[1], `"shadowId"`) {',
     "\t\tif false {",
     "the fake-fidelity guard stops seeing offenders, so the next consumer can be blinded again"),
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
        # 🔴 A run can DIE part-way and still look KILLED. See the sibling
        # battery's note: one mutant reported a third of the baseline's verdicts,
        # which is a fact about the harness, not about the code.
        "PANIC": len(re.findall(r"^panic:", out, re.M)),
    }
    built = "[build failed]" not in out
    return fails, detail, counts, built


def main():
    # 🔴 PREFIX SELECTOR, AND A BOGUS NAME REFUSES. Matching full ids only meant
    # the README's own `… .py S1 S7` selected NOTHING and exited 0 — "every
    # mutant ran and was killed" — having run none.
    only = sys.argv[1:] or None
    if only:
        known = [m[0] for m in MUTANTS]
        unmatched = [x for x in only
                     if not any(k == x or k.startswith(x + "-") for k in known)]
        if unmatched:
            print(f"!! these selectors match no mutant: {unmatched}")
            print(f"   known: {known}")
            return 2
    base = run_tests()
    _, _, bc, built = parse(base)
    base_total = bc["PASS"] + bc["FAIL"]
    print(f"BASELINE: {bc} total={base_total} built={built}")
    if bc["FAIL"] != 0 or not built:
        print("!! baseline is not green — every result below is meaningless")
        return 3

    if not only and len(MUTANTS) < EXPECTED_MUTANTS:
        print(f"!! this battery defines {len(MUTANTS)} mutants, expected at least {EXPECTED_MUTANTS}.")
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
        # 🔴 Restore is GUARANTEED, not hoped for — an interrupt otherwise left
        # the tree mutated, and `open(path,"w")` truncates, so a signal during
        # restore could leave the Go file EMPTY.
        _restore_queue[path] = src
        try:
            open(path, "w").write(src.replace(old, new, 1))
            out = run_tests()
            fails, detail, counts, built = parse(out)
        finally:
            open(path, "w").write(src)
            _restore_queue.pop(path, None)
        # A real check, not `assert` — bare asserts vanish under `python3 -O`.
        if sha(path) != before:
            raise SystemExit(f"{mid}: tree NOT restored — {path} differs from its pre-mutation bytes")
        # A build error wins over `fails`: it is evidence about the mutant, not
        # about the code under test, whatever else the run printed.
        if not built:
            verdict = "BUILD-FAIL"
        elif fails:
            verdict = "KILLED"
        else:
            verdict = "SURVIVED"
        total = counts["PASS"] + counts["FAIL"]
        short = base_total - total
        results.append((mid, verdict, "; ".join(
            f"{f} :: {detail.get(f, '(no line)')}" for f in fails[:3]), why))
        print(f"{mid}: {verdict} ({counts})")
        if counts["PANIC"] or short > base_total * 0.05:
            # 🔴 The mark goes in the VERDICT so it reaches the TABLE and the
            # committed artifact, not only the stream.
            results[-1] = (results[-1][0], results[-1][1] + " (TRUNCATED)", results[-1][2], results[-1][3])
            print(f"  ⚠ TRUNCATED RUN: {total} verdicts vs baseline {base_total}"
                  f" ({short} missing, panics={counts['PANIC']}) — treat the KILL as unattributed.")
        for f in fails[:3]:
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
    print(f"\n==== VERDICT ==== defined={len(results)} ran={len(results)-len(bad)-len(bf)} "
          f"killed={killed} survived={len(surv)} build_fail={len(bf)} not_run={len(bad)} "
          f"truncated={trunc}")
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
