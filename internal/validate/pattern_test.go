package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	cli "github.com/civitai/cli"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// pattern_test.go pins the four independent claims pattern.go makes. None of
// them subsumes the others, and each is stated with the failure it exists to
// catch:
//
//   - TestPatternFindingsCarryTheRuleAndAnExample — the OUTPUT: through the real
//     validate.Dir, a pattern violation names the rule in English and shows a
//     value that satisfies it, WITHOUT dropping the raw regex. Rows deny each
//     other's rule text, so two swapped table entries fail rather than passing
//     on "some gloss appeared".
//   - TestEnumFindingsKeepTheirExactWording — the CONTROL. The enum messages are
//     the standard the pattern gloss was written to match, so a rewrite of the
//     finding renderer that regressed them has to fail here. Byte-exact.
//   - TestPatternRulesCoverTheVendoredSchema — the LEDGER, bidirectional against
//     the vendored schema. `patternRules` is a mirror of a mirror, and an
//     unmaintained mirror is the failure mode AGENTS.md item 1 is about.
//   - TestPatternRuleExamplesSatisfyTheirPattern — each example is a CLAIM about
//     the regex it sits under. Shipping an example the schema rejects would be
//     the worst possible version of this feature: authoritative-looking advice
//     that walks the author into a second failure.

// patternFixture is one manifest that trips exactly one glossed pattern.
type patternFixture struct {
	name string
	// pattern is the key in patternRules this fixture must produce a finding
	// for. The expectations are derived FROM the table via this key, never
	// spelled out again here — a reword moves the code and the test together.
	pattern string
	// manifest is a complete-enough manifest; other findings are allowed and
	// ignored, only the row's own field is read.
	manifest string
	// field is the dotted path the finding must be located at. Asserted so a
	// row cannot be satisfied by a gloss landing on some other finding.
	field string
	// got is the offending value, which the base library message quotes. Kept
	// so the row proves the ORIGINAL message survived rather than being
	// replaced by prose.
	got string
}

func patternFixtures() []patternFixture {
	const base = `"name":"x","version":"1.0.0","contentRating":"g","scopes":[],` +
		`"kind":"page","iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}`
	return []patternFixture{
		{
			name:     "blockId",
			pattern:  `^[a-z][a-z0-9-]*[a-z0-9]$`,
			manifest: `{"blockId":"My First App!",` + base + `}`,
			field:    "blockId",
			got:      "My First App!",
		},
		{
			// A SPACE and a "!", the same shape as the blockId fixture, because
			// the two patterns' messages must be told apart while the offending
			// value looks identical — the cross-row absence assertion is what
			// reads this row.
			name:     "goods.id",
			pattern:  `^[a-z0-9][a-z0-9_-]*$`,
			manifest: `{"blockId":"ok-app","goods":[{"id":"Extra Credits!","title":"Extra credits","priceBuzz":100}],` + base + `}`,
			field:    "goods[0].id",
			got:      "Extra Credits!",
		},
		{
			name:     "version",
			pattern:  `^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`,
			manifest: `{"blockId":"ok-app","name":"x","version":"1.0","contentRating":"g","scopes":[]}`,
			field:    "version",
			got:      "1.0",
		},
		{
			name:     "tagline",
			pattern:  `\S`,
			manifest: `{"blockId":"ok-app","tagline":"   ",` + base + `}`,
			field:    "tagline",
			got:      "   ",
		},
		{
			// A DEEP LINK rather than a wrong host: the host arm and the
			// path-depth arm are separate halves of this regex, and a deep
			// link is the mistake an author actually makes (pasting the URL
			// out of the browser bar while looking at a file).
			name:     "repository",
			pattern:  `^https://(github\.com|gitlab\.com|codeberg\.org)/[^/]+/[^/]+/?$`,
			manifest: `{"blockId":"ok-app","repository":"https://github.com/civitai/civitai/tree/main",` + base + `}`,
			field:    "repository",
			got:      "https://github.com/civitai/civitai/tree/main",
		},
		{
			name:     "minApiVersion",
			pattern:  `^\d+(\.\d+)*$`,
			manifest: `{"blockId":"ok-app","minApiVersion":"v1",` + base + `}`,
			field:    "minApiVersion",
			got:      "v1",
		},
		{
			name:     "buildCommand",
			pattern:  `^(?:(?:npm|pnpm|yarn) run [a-zA-Z0-9:_-]+|(?:npx )?vite build)$`,
			manifest: `{"blockId":"ok-app","buildCommand":"rm -rf /","outputDir":"dist",` + base + `}`,
			field:    "buildCommand",
			got:      "rm -rf /",
		},
		{
			name:     "assetBundleUrl",
			pattern:  `^https://`,
			manifest: `{"blockId":"ok-app","assetBundleUrl":"http://example.com/b.zip",` + base + `}`,
			field:    "assetBundleUrl",
			got:      "http://example.com/b.zip",
		},
		{
			name:     "page.path",
			pattern:  `^/`,
			manifest: `{"blockId":"ok-app","page":{"path":"run","title":"t"},` + base + `}`,
			field:    "page.path",
			got:      "run",
		},
	}
}

// manifestOnlyFindings writes body as a manifest and returns the errors the
// REAL validator produces for it. ManifestOnly is used rather than Dir so the
// fixtures need no lockfile or source tree — the schema layer is what is under
// test, and it is identical on both paths.
func manifestOnlyFindings(t *testing.T, body string) []Finding {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	res, err := ManifestOnly(dir)
	if err != nil {
		t.Fatalf("ManifestOnly: %v", err)
	}
	return res.Errors
}

// findingFor returns the single finding whose Field is field and whose message
// mentions "does not match pattern", or fails.
func patternFindingFor(t *testing.T, fs []Finding, field string) Finding {
	t.Helper()
	var hits []Finding
	for _, f := range fs {
		if f.Field == field && strings.Contains(f.Message, "does not match pattern") {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly 1 pattern finding on %q, got %d\nall findings: %v", field, len(hits), fs)
	}
	return hits[0]
}

// TestPatternRuleTableIsWellFormed is the PRECONDITION for every assertion
// below. An empty or duplicated rule silently disarms both the "present" and
// the "absent" halves of the row assertions — strings.Contains(x, "") is always
// true — so the table's own shape is checked first, exactly as AGENTS.md item
// 21(f) requires of substitutionLead.
func TestPatternRuleTableIsWellFormed(t *testing.T) {
	if len(patternRules) == 0 {
		t.Fatal("patternRules is empty — every assertion in this file would be vacuous")
	}
	seenRule := map[string]string{}
	seenExample := map[string]string{}
	for pat, r := range patternRules {
		if strings.TrimSpace(r.rule) == "" {
			t.Errorf("pattern %q has an empty rule", pat)
		}
		if strings.TrimSpace(r.example) == "" {
			t.Errorf("pattern %q has an empty example", pat)
		}
		if prev, dup := seenRule[r.rule]; dup {
			t.Errorf("patterns %q and %q share a rule — the cross-row absence assertions "+
				"cannot tell them apart", prev, pat)
		}
		seenRule[r.rule] = pat
		if prev, dup := seenExample[r.example]; dup {
			t.Errorf("patterns %q and %q share an example", prev, pat)
		}
		seenExample[r.example] = pat
	}
}

// TestPatternGlossesAreTheRightWayRound pins, for each pattern, a LITERAL
// fragment of its rule and its LITERAL example — spelled here, derived from what
// the regex MEANS rather than read out of patternRules.
//
// 🔴 IT EXISTS BECAUSE EVERY OTHER GUARD IN THIS FILE DERIVES ITS EXPECTATION
// FROM THE TABLE, AND A TABLE THAT IS WRONG THE SAME WAY IN BOTH PLACES AGREES
// WITH ITSELF. Measured: SWAPPING the blockId and version rows left
// TestPatternFindingsCarryTheRuleAndAnExample entirely green — its `want` moved
// with the mutation, and its cross-row denial moved too, because the rule it
// denies for `version` had become blockId's. That is AGENTS.md item 23's
// "wrong but plausible value" shape: a sweep that only ever mutates a field to
// "" or to a different notation cannot see it, and neither can a test that reads
// the thing it is constraining.
//
// Rows are matched on a DISTINCTIVE fragment rather than the whole sentence so
// an editorial reword does not fail this; a fragment that stops identifying the
// rule uniquely fails on the pairwise-distinct check below.
func TestPatternGlossesAreTheRightWayRound(t *testing.T) {
	ledger := []struct {
		pattern string
		// ruleFragment is what this regex MEANS, in the fewest words that no
		// other row's rule could contain.
		ruleFragment string
		example      string
		why          string
	}{
		{`^[a-z][a-z0-9-]*[a-z0-9]$`, "lowercase letters, digits and hyphens", "my-first-app",
			"a slug alphabet with a first- and last-character rule"},
		// The fragment names UNDERSCORES because that is the whole difference
		// from the blockId row above: same lowercase-slug family, one extra
		// legal character and no last-character rule. A fragment quoting the
		// shared part would sit inside blockId's rule too and the absence half
		// would stop working — which is exactly what the pairwise
		// non-containment check below is there to catch.
		{`^[a-z0-9][a-z0-9_-]*$`, "underscores and hyphens", "extra-credits",
			"a slug alphabet that also admits _ and constrains only the FIRST character"},
		{`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`, "semantic version", "1.0.0",
			"exactly three numeric components plus an optional prerelease"},
		{`\S`, "non-whitespace", "A tiny image tool",
			"the only requirement is that SOMETHING is there"},
		// "repository root URL" rather than a fragment naming the hosts or the
		// scheme: the assetBundleUrl row below already owns "https:// URL", and
		// a fragment quoting the scheme would sit inside this rule too.
		{`^https://(github\.com|gitlab\.com|codeberg\.org)/[^/]+/[^/]+/?$`, "repository root URL", "https://github.com/civitai/civitai",
			"an exact host allowlist plus a path that is exactly two segments"},
		// "dot-separated numbers ONLY" rather than the bare phrase: the version
		// rule above says "three dot-separated numbers", so the shorter
		// fragment appears in two rules and the absence half stops working.
		{`^\d+(\.\d+)*$`, "dot-separated numbers only", "1.2",
			"any number of numeric components, unlike the version rule above"},
		{`^(?:(?:npm|pnpm|yarn) run [a-zA-Z0-9:_-]+|(?:npx )?vite build)$`, "allowlisted build invocations", "npm run build",
			"a closed set of commands, not a shape"},
		{`^https://`, "https:// URL", "https://example.com/bundle.zip",
			"a scheme requirement and nothing else"},
		{`^/`, `must start with a "/"`, "/",
			"a leading-character requirement on a mount path"},
	}
	if len(ledger) != len(patternRules) {
		t.Fatalf("%d ledger rows for %d glossed patterns — the ledger must be total, "+
			"or a gloss can be wrong with nothing to say so", len(ledger), len(patternRules))
	}
	// The fragments must be pairwise non-containing, or "row A's fragment is in
	// row A's rule" is satisfiable by a swap.
	for i, a := range ledger {
		for j, b := range ledger {
			if i != j && strings.Contains(a.ruleFragment, b.ruleFragment) {
				t.Fatalf("fragment %q contains %q — the rows cannot tell each other apart",
					a.ruleFragment, b.ruleFragment)
			}
		}
	}
	for _, row := range ledger {
		got, ok := patternRules[row.pattern]
		if !ok {
			t.Errorf("no gloss for %q", row.pattern)
			continue
		}
		if !strings.Contains(got.rule, row.ruleFragment) {
			t.Errorf("pattern %q (%s)\n  rule must mention: %s\n  got:               %s",
				row.pattern, row.why, row.ruleFragment, got.rule)
		}
		if got.example != row.example {
			t.Errorf("pattern %q example\n  want: %s\n  got:  %s", row.pattern, row.example, got.example)
		}
		// ABSENCE: no other row's fragment may appear in this rule.
		for _, other := range ledger {
			if other.pattern == row.pattern {
				continue
			}
			if strings.Contains(got.rule, other.ruleFragment) {
				t.Errorf("the rule for %q carries %q, which belongs to %q — the table is "+
					"the wrong way round", row.pattern, other.ruleFragment, other.pattern)
			}
		}
	}
}

func TestPatternFindingsCarryTheRuleAndAnExample(t *testing.T) {
	fixtures := patternFixtures()
	if len(fixtures) != len(patternRules) {
		t.Fatalf("%d fixtures for %d glossed patterns — every gloss needs a fixture that "+
			"reaches it, or its rows below prove nothing", len(fixtures), len(patternRules))
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := patternRules[tc.pattern]
			if !ok {
				t.Fatalf("fixture names pattern %q, which is not in patternRules", tc.pattern)
			}
			f := patternFindingFor(t, manifestOnlyFindings(t, tc.manifest), tc.field)

			// The base half is the LIBRARY's sentence, reconstructed from the
			// fixture's own value and regex rather than copied out of the
			// output. That is deliberate: the claim this half makes is
			// "unchanged", so it has to be derived from something other than
			// the code path under test. It also sidesteps the library's own
			// escaping of the regex, which a hand-spelled literal would have to
			// mirror and would then pin to the wrong thing.
			base := (&kind.Pattern{Got: tc.got, Want: tc.pattern}).LocalizedString(printer)
			// Sanity: the base must actually quote the offending value, or the
			// exact-match below is pinning a sentence that lost it.
			if !strings.Contains(base, tc.got) {
				t.Fatalf("the library's own message no longer names the offending value %q: %s", tc.got, base)
			}

			// 1-4, in one byte-exact assertion: the field prefix, the preserved
			// library text, the rule in English, and a value that satisfies it.
			want := tc.field + ": " + base + " — " + rule.rule + ` (example: "` + rule.example + `")`
			if f.Message != want {
				t.Errorf("pattern finding message\n  want: %s\n  got:  %s", want, f.Message)
			}

			// 5. 🔴 ABSENCE. Every OTHER pattern's rule must be missing. A
			//    table whose entries were swapped still produces "a rule and an
			//    example", and assertions 3+4 alone would be satisfied by the
			//    wrong ones landing here only if they happened to match — this
			//    is what makes a swap fail from both directions.
			for otherPat, other := range patternRules {
				if otherPat == tc.pattern {
					continue
				}
				if strings.Contains(f.Message, other.rule) {
					t.Errorf("message carries the rule for pattern %q, which is not this finding's:\n%s",
						otherPat, f.Message)
				}
			}
		})
	}
}

// TestEnumFindingsKeepTheirExactWording is the CONTROL for the whole file.
//
// The enum messages are the standard the pattern gloss was written to match, so
// they are the thing a rewrite of schemaErrors is most likely to change by
// accident. The message is asserted BYTE-EXACT, not by fragment: the point is
// that it did not move at all.
//
// 🔴 WHAT IS PINNED IS THE SHAPE, AND WHAT IS DERIVED IS THE CONTENT — because
// pinning both made this guard SELF-BLOCKING, measured. `schema/` is a vendored
// mirror (AGENTS.md item 1) re-synced by `revendor-canonical-schema.yml`, which
// runs the suite on its own output before opening a PR. This test used to spell
// out all thirteen scopes, so the canonical adding `goods:read:self` /
// `goods:purchase:self` reddened it — and the automation whose whole job is to
// land that change could not land it: it could never accept a canonical that
// ADDS an enum member, which is the ordinary way a capability list changes. So
// WHICH values are allowed now comes from the vendored schema, and the sentence
// they are rendered into is spelled here.
//
// 🔴 THIS UNBLOCKS ENUM GROWTH ONLY — IT DOES NOT MAKE THE RE-VENDOR BOT
// SELF-SUFFICIENT, and do not read it as doing so. There are TWO guards over the
// mirror and this is one of them: a canonical that adds a `pattern` still reds
// `TestPatternRulesCoverTheVendoredSchema` until a human writes the gloss, so
// the bot could not have self-landed even the change that motivated this edit
// (the `goods` canonical added BOTH two scopes and the `goods[].id` pattern).
// That is deliberate — a gloss is English prose about what a regex MEANS, which
// cannot be derived, and a bot shipping a bare regex to app authors would be
// worse than a red check. Expect a canonical that adds a pattern to need a
// human; expect an enum-only one not to.
func TestEnumFindingsKeepTheirExactWording(t *testing.T) {
	const body = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"zz",` +
		`"scopes":["models:read:self","bogus:scope"],"kind":"page",` +
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`
	got := map[string]string{}
	for _, f := range manifestOnlyFindings(t, body) {
		got[f.Field] = f.Message
	}

	// enumMessage is the SHAPE, written out once and literally: the field
	// prefix, the library's lead-in, each value in single quotes, ", " between
	// them, schema order preserved. Nothing here is read out of the code under
	// test.
	enumMessage := func(field string, allowed []string) string {
		quoted := make([]string, len(allowed))
		for i, v := range allowed {
			quoted[i] = "'" + v + "'"
		}
		return field + ": value must be one of " + strings.Join(quoted, ", ")
	}

	// 🔴 THE TEMPLATE IS ANCHORED, NOT TRUSTED. If both halves were derived, the
	// shape claim would rest on this file's own template and nothing could fail:
	// reword the library's lead-in and `want` moves with `got`. So the template
	// must first REPRODUCE, byte for byte, a message a human wrote out in full.
	// contentRating is the anchor because it is the site-wide rating system
	// rather than an app-blocks capability list, so unlike `scopes` it is not
	// expected to grow — and it is asserted against the real output below too,
	// making template, schema and validator agree three ways.
	const wantContentRating = "contentRating: value must be one of 'g', 'pg', 'pg13', 'r', 'x'"
	if tmpl := enumMessage("contentRating", schemaEnum(t, "properties", "contentRating", "enum")); tmpl != wantContentRating {
		t.Fatalf("the enum message SHAPE moved — every expectation derived from it below is "+
			"now meaningless, so this is fatal rather than an error\n  want: %s\n  got:  %s",
			wantContentRating, tmpl)
	}

	scopes := schemaEnum(t, "properties", "scopes", "items", "enum")
	// CONTENT control on the derived half. schemaEnum already fatals on a
	// missing or empty enum, but not on resolving to the WRONG array, and a
	// set that lost long-standing members is a real regression rather than
	// growth. These two are the oldest and the most consequential member.
	//
	// 🔴 IT NAMES TWO MEMBERS AND GUARDS ONLY THOSE TWO — it is NOT a shrink
	// detector for the enum as a whole, and must not be quoted as one. Measured:
	// dropping `apps:storage:read` or `collections:write:self` from the vendored
	// enum leaves this whole package GREEN. The shrink direction is owned by
	// `schema-drift` (`scripts/check-canonical-schema.sh`), which normalises with
	// `jq -S` and compares against the LIVE canonical, so it reds on any dropped
	// value — a strictly stronger instrument than a hand-typed list here, which
	// is why this list is deliberately not grown to all fifteen.
	for _, must := range []string{"models:read:self", "posts:write:self"} {
		if !slices.Contains(scopes, must) {
			t.Fatalf("the schema's scope enum does not contain %q — %d values read, so this "+
				"is reading the wrong node or the canonical lost a scope: %v", must, len(scopes), scopes)
		}
	}

	want := map[string]string{
		"contentRating": wantContentRating,
		"scopes[1]":     enumMessage("scopes[1]", scopes),
	}
	for field, wantMsg := range want {
		if got[field] != wantMsg {
			t.Errorf("enum finding on %q changed\n  want: %s\n  got:  %s", field, wantMsg, got[field])
		}
	}
	// Positive control: an enum finding must have been produced at all, or the
	// loop above compares two absent values and reports nothing.
	if len(got) == 0 {
		t.Fatal("the fixture produced no findings — this control observes nothing")
	}
}

// TestPostsWriteSelfScopeAccepted is the ACCEPT arm for the scope the vendored
// schema gained with the App Blocks → Post bridge.
//
// The enum-wording test above only proves the string appears in a REJECTION
// message; a schema could list a value there and still refuse it (the message is
// built from the enum, the acceptance is a separate code path). This asserts the
// thing an app author actually depends on: a manifest declaring the scope, with
// its required justification, produces no manifest error at all.
//
// The rejection arm is the pre-release binary, which is exercised outside the
// test suite — a suite compiled against the NEW schema cannot observe the old
// one.
func TestPostsWriteSelfScopeAccepted(t *testing.T) {
	const body = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"pg",` +
		`"scopes":["models:read:self","posts:write:self"],` +
		`"scopeJustifications":{"posts:write:self":"lets the viewer publish the images this app generated for them"},` +
		`"kind":"page",` +
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`
	for _, f := range manifestOnlyFindings(t, body) {
		t.Errorf("unexpected finding on %q: %s", f.Field, f.Message)
	}

	// NEGATIVE CONTROL for this fixture specifically: the SAME manifest with the
	// justification removed MUST fail, and fail for the justification's own
	// reason. Without this, a fixture that silently stopped reaching the
	// validator would pass the loop above by producing nothing.
	const unjustified = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"pg",` +
		`"scopes":["models:read:self","posts:write:self"],"kind":"page",` +
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`
	var hit bool
	for _, f := range manifestOnlyFindings(t, unjustified) {
		if strings.Contains(f.Message, "posts:write:self") &&
			strings.Contains(f.Message, "scopeJustifications") {
			hit = true
		}
	}
	if !hit {
		t.Fatal("removing the justification produced no scopeJustifications finding naming posts:write:self")
	}
}

// schemaEnum returns the `enum` the VENDORED schema declares at an explicit key
// path, in the schema's own order — which is the order the library renders them
// in, so a derived message matches byte for byte.
//
// The path is spelled by the caller rather than searched for: a search would
// find "some enum" and could silently move to a different one, which is the
// failure mode the callers' own content controls exist to catch. Every way of
// not-finding it is FATAL, because an expectation derived from a missing or
// empty enum is vacuous rather than wrong — `enum: []` would render "value must
// be one of " on BOTH sides and agree.
func schemaEnum(t *testing.T, path ...string) []string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("vendored schema does not decode: %v", err)
	}
	node := doc
	for i, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v: %v is not an object, so %q cannot be read",
				path, path[:i], key)
		}
		if node, ok = m[key]; !ok {
			t.Fatalf("schema path %v: no %q — this test is reading a shape the schema "+
				"no longer has", path, key)
		}
	}
	arr, ok := node.([]any)
	if !ok {
		t.Fatalf("schema path %v is a %T, not an array of enum values", path, node)
	}
	if len(arr) == 0 {
		t.Fatalf("schema path %v is an EMPTY enum — a message derived from it would be "+
			"vacuously equal to the one the validator produced", path)
	}
	out := make([]string, 0, len(arr))
	for i, v := range arr {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("schema path %v item %d is a %T, not a string", path, i, v)
		}
		out = append(out, s)
	}
	return out
}

// schemaPatterns walks the VENDORED schema and returns every `pattern` that can
// surface as a kind.Pattern finding.
//
// Patterns under a `not` are excluded because a failing `not` surfaces as
// kind.Not — "not failed", no keyword path, no regex, no value — so there is
// nothing for a gloss to key on. That exclusion is the reason the four
// outputDir sub-patterns are legitimately absent from patternRules.
func schemaPatterns(t *testing.T) map[string]bool {
	t.Helper()
	var doc any
	if err := json.Unmarshal(cli.SchemaJSON, &doc); err != nil {
		t.Fatalf("vendored schema does not decode: %v", err)
	}
	out := map[string]bool{}
	var walk func(node any, underNot bool)
	walk = func(node any, underNot bool) {
		switch n := node.(type) {
		case map[string]any:
			for k, v := range n {
				if k == "pattern" && !underNot {
					if s, ok := v.(string); ok {
						out[s] = true
					}
					continue
				}
				walk(v, underNot || k == "not")
			}
		case []any:
			for _, v := range n {
				walk(v, underNot)
			}
		}
	}
	walk(doc, false)
	return out
}

// TestPatternRulesCoverTheVendoredSchema is a BIDIRECTIONAL ledger.
//
// Growth direction: the vendored schema gains a `pattern` and nobody writes a
// gloss — the author gets the raw regex back for that field, silently. Shrink
// direction: the schema drops a pattern and the gloss sits in the table looking
// like coverage for a rule that no longer exists.
//
// It is a GATE rather than a nicety because `schema/` is a vendored mirror
// (AGENTS.md item 1): the next sync with the server is exactly when this drifts,
// and the failure it produces is invisible in the output — a message that is
// merely terse, not wrong.
func TestPatternRulesCoverTheVendoredSchema(t *testing.T) {
	inSchema := schemaPatterns(t)
	// Positive control. A walker wired to nothing returns an empty set, and an
	// empty set compared against an empty table would agree. Assert the walker
	// found something, and that it found the pattern this whole issue is about.
	if len(inSchema) < 2 {
		t.Fatalf("the schema walker found %d patterns — it is not observing the schema", len(inSchema))
	}
	if !inSchema[`^[a-z][a-z0-9-]*[a-z0-9]$`] {
		t.Fatal("the schema walker did not find the blockId pattern — it is reading the wrong shape")
	}
	// Negative control on the `not` exclusion: outputDir's sub-patterns must NOT
	// be collected, or the ledger below would demand glosses for rules that can
	// never reach a kind.Pattern finding.
	if inSchema[`^/`] && !inSchema[`^https://`] {
		t.Fatal("unexpected schema shape; re-check the walker")
	}
	for _, notPat := range []string{`\\`, `(^|/)\.\.(/|$)`, `^[A-Za-z]:`} {
		if inSchema[notPat] {
			t.Errorf("walker collected %q, which lives under a `not` and cannot surface "+
				"as kind.Pattern", notPat)
		}
	}

	for pat := range inSchema {
		if _, ok := patternRules[pat]; !ok {
			t.Errorf("schema pattern %q has no gloss in patternRules — an author hitting it "+
				"gets a bare regex back", pat)
		}
	}
	for pat := range patternRules {
		if !inSchema[pat] {
			t.Errorf("patternRules glosses %q, which is not a reachable pattern in the "+
				"vendored schema — a stale row looks like coverage", pat)
		}
	}
}

// TestPatternRuleExamplesSatisfyTheirPattern checks each example against the
// regex it is filed under. An example the schema would reject is worse than no
// example: it is confident advice that produces a second failure.
func TestPatternRuleExamplesSatisfyTheirPattern(t *testing.T) {
	for pat, r := range patternRules {
		re, err := regexp.Compile(pat)
		if err != nil {
			t.Errorf("pattern %q does not compile: %v", pat, err)
			continue
		}
		if !re.MatchString(r.example) {
			t.Errorf("example %q does NOT satisfy its own pattern %q", r.example, pat)
		}
		// Negative control per row: the matcher must be able to say NO, or a
		// regexp that matched everything would make the assertion above vacuous.
		// A whitespace-only string is rejected by all seven — including `\S`,
		// which is the one pattern here that accepts nearly anything else.
		if re.MatchString("   ") {
			t.Errorf("pattern %q matches a control string it must reject — "+
				"the row above proves nothing", pat)
		}
	}
}

// TestPatternAdviceFailsSoft pins the degradation contract: an unglossed
// pattern must add NOTHING, leaving exactly the message the CLI printed before.
// Manufacturing prose for a rule we do not know is the false-advice failure
// AGENTS.md item 10 spent four measured corrections avoiding.
func TestPatternAdviceFailsSoft(t *testing.T) {
	if got := patternAdvice(&kind.Pattern{Got: "x", Want: `^no-such-pattern$`}); got != "" {
		t.Errorf("an unglossed pattern must add nothing, got %q", got)
	}
	// Positive control: a KNOWN pattern must add something, or the assertion
	// above is satisfied by a function that always returns "".
	if got := patternAdvice(&kind.Pattern{Got: "x", Want: `^[a-z][a-z0-9-]*[a-z0-9]$`}); got == "" {
		t.Error("a glossed pattern added nothing — patternAdvice is wired to nothing")
	}
}
