package validate

import (
	"strings"
	"testing"
)

// finding_fields_test.go pins the VALUE of the field every check carries, not
// merely that it carried one.
//
// 🔴 WHY THIS FILE EXISTS, stated because the gap it closes was invisible to a
// full green suite AND to a mutation sweep. The three guards in
// finding_contract_test.go and `TestValidateJSONEveryFindingCarriesAField` all
// answer "is the field non-empty?"; only eight sites had their SPECIFIC field
// asserted anywhere. The original sweep mutated a field to `""` or to JSON
// Pointer — both of which those guards see — and never to a WRONG BUT PLAUSIBLE
// value, which is the shape a real refactor produces. Three such mutants
// survived the whole suite:
//
//   - buildCoherence's two pair rules INVERTED, so "buildCommand is set but
//     outputDir is missing" reported `buildCommand`;
//   - the budgeted-scope-no-page warning moved from `page` to `scopes`;
//   - BOTH lockfile findings moved from `(project)` to `(root)` — the exact
//     sentinel collapse AGENTS.md item 23 marks 🔴, because it sends a CI job
//     looking in block.manifest.json for a missing lockfile.
//
// The ledger below is BIDIRECTIONAL, which is what makes it a gate rather than a
// sample: every finding the corpus produces must match exactly ONE entry (a new
// check with no entry fails), and every entry must match at least one finding (a
// stale entry cannot sit there looking like coverage).
//
// 🔴 EACH `field` IS DERIVED FROM WHAT THE CHECK MEANS, NOT FROM WHAT THE CODE
// DOES — that is what `why` records. Copying the value out of the implementation
// would make this test a restatement of the thing it is supposed to constrain.

// fieldExpectation is one finding the corpus produces, the field it MUST carry,
// and the reason that field is the right one.
type fieldExpectation struct {
	// fragment identifies exactly one finding. It is matched with Contains
	// against the message, and the test fails if it matches more than one
	// distinct finding or none.
	fragment string
	field    string
	why      string
}

// findingFieldLedger is the whole contract, one row per finding the corpus
// produces. Ordered by the file that emits it.
func findingFieldLedger() []fieldExpectation {
	return []fieldExpectation{
		// --- validate.go: the document itself ---------------------------
		{"not found at project root", FieldDocument,
			"there is no manifest, so no key inside one can be named"},
		{"is not valid JSON", FieldDocument,
			"the document did not decode; nothing inside it is addressable"},

		// --- validate.go: schemaErrors ----------------------------------
		{"(root): missing property 'contentRating'", FieldDocument,
			"a missing top-level required property is located at the object that lacks it"},
		{"(root): missing property 'scopes'", FieldDocument,
			"same: the object is what is incomplete, not the absent key"},
		{"(root): missing property 'outputDir'", FieldDocument,
			"same, via the schema's buildCommand=>outputDir allOf"},
		{"blockId: 'Bad_Id' does not match pattern", "blockId",
			"the offending value is blockId's"},
		{"scopes[1]: value must be one of", "scopes[1]",
			"an array violation names the ELEMENT; `scopes` alone cannot point at a line"},
		{"buildCommand: 'rm -rf /' does not match pattern", "buildCommand",
			"the schema's pattern is on buildCommand's own value"},
		{"buildCommand: maxLength", "buildCommand",
			"the length constraint is on buildCommand's own value"},
		{"outputDir: not failed", "outputDir",
			"the schema locates this `not` violation at outputDir"},
		{"iframe.minHeight: maximum", "iframe.minHeight",
			"the range constraint is on iframe.minHeight's own value"},

		// --- validate.go: serverOwnedFieldChecks ------------------------
		{"trustTier is server-owned", "trustTier",
			"the remedy is to delete this key; it is the key that must not be there"},
		{"iframe.src is server-owned", "iframe.src",
			"same, nested — the sub-key is what must be deleted, not the iframe block"},

		// --- validate.go: buildCoherence --------------------------------
		{"buildCommand is too long", "buildCommand",
			"the rule reports on buildCommand's own length"},
		{"is not an allowed build invocation", "buildCommand",
			"the rule reports on buildCommand's own value"},
		{"buildCommand is set but outputDir is missing", "outputDir",
			"PAIR RULE: the finding names the MISSING side (AGENTS.md item 23) — " +
				"buildCommand is correct as written; outputDir is the edit"},
		{"outputDir is set but buildCommand is missing", "buildCommand",
			"PAIR RULE, the mirror: outputDir is correct as written; buildCommand is the edit"},
		{"must be a relative path under the project root", "outputDir",
			"the rule reports on outputDir's own value"},
		{"must not escape the project root", "outputDir",
			"the rule reports on outputDir's own value"},

		// --- semantic.go ------------------------------------------------
		{"requires a verified/internal trust tier", "renderMode",
			"the gate is on the renderMode value the author chose; trustTier is server-owned " +
				"and not an edit the author can make"},
		{"must also declare an iframe block", "iframe",
			"PAIR RULE: `page` is set and correct; the missing side is the iframe block"},
		{"iframe block is required for renderMode=iframe", "iframe",
			"the missing block is the finding's subject"},
		{"iframe.minHeight is required and must be a number", "iframe.minHeight",
			"the required sub-field that is absent"},
		{"iframe.minHeight must be a number in", "iframe.minHeight",
			"the sub-field whose value is out of range"},
		{"iframe.resizable is required and must be a boolean", "iframe.resizable",
			"the required sub-field that is absent"},
		{"iframe.sandbox must contain at least one token", "iframe.sandbox",
			"the sandbox string's own content"},
		{`sandbox token "allow-popups" is not allowed`, "iframe.sandbox",
			"a disallowed token lives INSIDE the sandbox string; there is no finer location"},
		{`sandbox token "allow-same-origin" is not allowed`, "iframe.sandbox",
			"same — one finding per token, all on the one string that holds them"},
		{"MUST NOT combine allow-same-origin with allow-scripts", "iframe.sandbox",
			"the escape combo is an interaction of two TOKENS in one string value, " +
				"not of two manifest fields"},
		{"is not one of the manifest's declared scopes", "scopeJustifications.models:read:self",
			"PER-KEY rule: the offending KEY, because `scopeJustifications` alone is " +
				"useless on the only manifests where this fires more than once"},
		{"sensitive scopes require a justification", "scopeJustifications",
			"the map the author must ADD entries to. Not `scopes` — the declared scopes " +
				"are correct — and not per-scope, because the server emits ONE message " +
				"naming them all and this mirror is byte-identical on purpose"},

		// --- targets.go -------------------------------------------------
		{"is not a known slot", "targets[0].slotId",
			"PER-ELEMENT rule: the index plus the sub-key, so a consumer can point at a line"},
		{"is the page slot", "targets[1].slotId",
			"same — and the two findings in this fixture must land on DIFFERENT fields"},

		// --- warnings.go ------------------------------------------------
		{"page.buzzBudgetPerGen is not set", "page.buzzBudgetPerGen",
			"PAIR RULE: the budgeted scope is deliberate; the missing budget is the edit"},
		{`but no "page" block is declared`, "page",
			"PAIR RULE: the scope is deliberate; the missing `page` block is the edit. " +
				"NOT `scopes` — that would tell the author to drop the scope they meant to declare"},
		{"the budget is inert", "scopes",
			"PAIR RULE, the mirror: the budget is deliberate; the missing scope is the edit"},

		// --- lockfile.go ------------------------------------------------
		{"package.json is present but no lockfile is committed", FieldProject,
			"🔴 repository state, not a manifest key: the manifest can be byte-perfect " +
				"and still trip this. `(root)` would send a CI job looking inside " +
				"block.manifest.json for a file that is missing from the repo"},
		{"more than one lockfile is committed", FieldProject,
			"same: the finding is about which files are committed"},
		{"not a lockfile the platform build can install from", FieldProject,
			"same again, and the one most likely to be mis-filed: the remedy is `rm` plus " +
				"`npm install`, so it touches no manifest key at all. `(root)` would send a " +
				"CI job into block.manifest.json looking for a file on disk"},

		// --- readyack.go ------------------------------------------------
		{"nothing in this project's source posts BLOCK_READY", FieldProject,
			"the finding is about what the browser loads. `page` is what makes the check " +
				"APPLY and is the one field that is CORRECT — reporting it there would " +
				"send a consumer to the wrong file entirely"},
	}
}

// corpusFindings runs every fixture and returns the DISTINCT findings produced,
// each with the fixture that produced it (for the failure message).
func corpusFindings(t *testing.T) map[Finding]string {
	t.Helper()
	findingSiteHook = nil
	out := map[Finding]string{}
	for _, fx := range fieldCoverageCorpus() {
		dir := writeFixture(t, fx.files)
		res, err := Dir(dir)
		if err != nil {
			t.Fatalf("%s: Dir: %v", fx.name, err)
		}
		for _, f := range append(append([]Finding{}, res.Errors...), res.Warnings...) {
			// The "no manifest" fixture interpolates a temp path; normalise so
			// the distinct set is stable.
			if _, ok := out[f]; !ok {
				out[f] = fx.name
			}
		}
	}
	return out
}

// TestEveryFindingCarriesItsDocumentedField is the field-VALUE gate.
//
// It is deliberately bidirectional. A one-directional version — "for each entry,
// find the finding and check it" — is what `TestValidateJSONSemanticChecksAreFieldTagged`
// already does for eight sites, and it cannot see a check that has no entry:
// that is precisely how fifteen sites went unpinned while the suite stayed green.
func TestEveryFindingCarriesItsDocumentedField(t *testing.T) {
	ledger := findingFieldLedger()
	found := corpusFindings(t)

	// Positive control on the harness. A ledger that matched nothing would report
	// a serene pass in exactly the same shape as a correct one.
	const minFindings = 30
	if len(found) < minFindings {
		t.Fatalf("the corpus produced only %d distinct findings, expected at least %d — "+
			"the harness is not exercising the checks, and every assertion below is vacuous",
			len(found), minFindings)
	}
	if len(ledger) < minFindings {
		t.Fatalf("the ledger holds only %d entries, expected at least %d", len(ledger), minFindings)
	}

	matched := make([]int, len(ledger))
	for f, fixture := range found {
		var hits []int
		for i, e := range ledger {
			if strings.Contains(f.Message, e.fragment) {
				hits = append(hits, i)
			}
		}
		switch len(hits) {
		case 0:
			t.Errorf("this finding matches NO ledger entry, so its field is pinned by nothing:\n"+
				"  fixture: %s\n  field:   %q\n  message: %s\n"+
				"Add a row to findingFieldLedger() naming the field this check must carry "+
				"and WHY that is the right field — derived from what the check means, not "+
				"from what the code currently does.", fixture, f.Field, f.Message)
			continue
		case 1:
			// The intended case.
		default:
			var frags []string
			for _, i := range hits {
				frags = append(frags, ledger[i].fragment)
			}
			t.Errorf("finding %q matches %d ledger entries (%v) — the fragments are ambiguous, "+
				"so one entry could shadow another and go untested. Make them distinguishing.",
				f.Message, len(hits), frags)
			continue
		}
		e := ledger[hits[0]]
		matched[hits[0]]++
		if f.Field != e.field {
			t.Errorf("WRONG FIELD.\n  finding:  %s\n  fixture:  %s\n  carries:  %q\n  must be: %q\n  because:  %s",
				f.Message, fixture, f.Field, e.field, e.why)
		}
	}

	for i, e := range ledger {
		if matched[i] == 0 {
			t.Errorf("ledger entry %q matched NO finding — it is stale (the check changed its "+
				"message or was deleted), so it has been asserting nothing.", e.fragment)
		}
	}
}

// TestPairRuleNamesTheMissingField pins the CONVENTION AGENTS.md item 23 states
// as a contract and nothing asserted: "for an 'X is set but Y is missing' pair
// rule the field is **Y** (the missing one)".
//
// It is separate from the ledger above on purpose. The ledger says WHAT each
// field is; this says WHY that set of values is not arbitrary, and it is the
// test that fails with the right explanation when someone flips a pair. Four
// pair rules exist across two files, and an inversion of any of them is a
// plausible-looking refactor that changes nothing else observable.
func TestPairRuleNamesTheMissingField(t *testing.T) {
	pairs := []struct {
		fragment string
		present  string // the field whose PRESENCE triggered the rule
		missing  string // the field whose ABSENCE the rule reports — the expected Field
	}{
		{"buildCommand is set but outputDir is missing", "buildCommand", "outputDir"},
		{"outputDir is set but buildCommand is missing", "outputDir", "buildCommand"},
		{`but no "page" block is declared`, "scopes", "page"},
		{"the budget is inert", "page.buzzBudgetPerGen", "scopes"},
		{"page.buzzBudgetPerGen is not set", "scopes", "page.buzzBudgetPerGen"},
		{"must also declare an iframe block", "page", "iframe"},
	}

	found := corpusFindings(t)
	for _, p := range pairs {
		if p.present == p.missing {
			t.Fatalf("pair %q names the same field on both sides — the assertion below would be vacuous", p.fragment)
		}
		var hits int
		for f := range found {
			if !strings.Contains(f.Message, p.fragment) {
				continue
			}
			hits++
			// The rule really is ABOUT both fields — otherwise "name the missing
			// one" is a claim about a pair that does not exist.
			for _, name := range []string{p.present, p.missing} {
				if !strings.Contains(f.Message, lastSegment(name)) {
					t.Errorf("pair rule %q does not mention %q in its message, so it is not the "+
						"pair this test claims it is:\n  %s", p.fragment, name, f.Message)
				}
			}
			if f.Field != p.missing {
				t.Errorf("PAIR RULE INVERTED: %q reports field %q.\n"+
					"  %q is the side that is SET and correct; %q is the side that is MISSING "+
					"and is the edit that resolves the finding. AGENTS.md item 23: the field is "+
					"the missing one.\n  %s", p.fragment, f.Field, p.present, p.missing, f.Message)
			}
		}
		if hits != 1 {
			t.Errorf("the corpus produced %d findings matching %q, want exactly 1 — this test "+
				"is not checking what it claims to", hits, p.fragment)
		}
	}
}

// lastSegment returns the member name of a dotted path ("page.buzzBudgetPerGen"
// -> "buzzBudgetPerGen"), which is how the prose messages spell a nested field
// when they do not spell the whole path.
func lastSegment(field string) string {
	if i := strings.LastIndex(field, "."); i >= 0 {
		return field[i+1:]
	}
	return field
}
