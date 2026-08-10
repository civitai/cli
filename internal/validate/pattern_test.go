package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
// accident. They are asserted BYTE-EXACT, not by fragment: the point is that
// they did not move at all.
func TestEnumFindingsKeepTheirExactWording(t *testing.T) {
	const body = `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"zz",` +
		`"scopes":["models:read:self","bogus:scope"],"kind":"page",` +
		`"iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false}}`
	got := map[string]string{}
	for _, f := range manifestOnlyFindings(t, body) {
		got[f.Field] = f.Message
	}
	want := map[string]string{
		"contentRating": "contentRating: value must be one of 'g', 'pg', 'pg13', 'r', 'x'",
		"scopes[1]": "scopes[1]: value must be one of 'models:read:self', 'user:read:self', " +
			"'ai:write:budgeted', 'buzz:read:self', 'social:tip:self', 'apps:storage:read', " +
			"'apps:storage:write', 'apps:storage:shared:read', 'apps:storage:shared:write', " +
			"'collections:read:self', 'collections:write:self', 'collections:read:private'",
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
