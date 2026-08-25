#!/usr/bin/env python3
"""Mutation battery for `civitai app listing set-text`.

Same harness discipline as the doctor battery, with the one lesson it taught
baked in: a mutant whose search text no longer matches is reported as
BAD-PATTERN with the SAME verdict vocabulary as every other outcome, and the
process exits non-zero, so a run containing one can never be read as clean.
"""
import hashlib
import os
import re
import subprocess
import sys

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
]


def sha(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def run_tests():
    p = subprocess.run(["go", "test", "-count=1", "-v"] + PKGS,
                       cwd=WT, capture_output=True, text=True)
    return p.stdout + p.stderr


def parse(out):
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
    }
    built = "[build failed]" not in out
    return fails, detail, counts, built


def main():
    only = sys.argv[1:] or None
    base = run_tests()
    _, _, bc, built = parse(base)
    print(f"BASELINE: {bc} built={built}")
    if bc["FAIL"] != 0 or not built:
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
            print(f"{mid}: BAD-PATTERN — matched {n} times (want exactly 1), NOT RUN.")
            results.append((mid, "BAD-PATTERN", "", why))
            continue
        open(path, "w").write(src.replace(old, new, 1))
        out = run_tests()
        fails, detail, counts, built = parse(out)
        open(path, "w").write(src)
        assert sha(path) == before, f"{mid}: tree not restored!"
        if not built and not fails:
            verdict = "BUILD-FAIL"
        elif fails:
            verdict = "KILLED"
        else:
            verdict = "SURVIVED"
        results.append((mid, verdict, "; ".join(
            f"{f} :: {detail.get(f, '(no line)')}" for f in fails[:3]), why))
        print(f"{mid}: {verdict} ({counts})")
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
    print(f"\n==== VERDICT ==== defined={len(results)} killed="
          f"{len([r for r in results if r[1]=='KILLED'])} survived={len(surv)} "
          f"build_fail={len(bf)} not_run={len(bad)}")
    if bad:
        print("!! NOT A CLEAN SWEEP — these NEVER RAN and are evidence of nothing:")
        for mid, _, _, why in bad:
            print(f"     {mid}: {why}")
        return 2
    return 1 if surv else 0


if __name__ == "__main__":
    sys.exit(main())
