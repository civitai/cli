package validate

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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
//   - TestPatternRulesCoverTheVendoredSchema — the LEDGER, TOTAL against the
//     vendored schema: every pattern the schema can surface is either GLOSSED in
//     `patternRules` or listed as an acknowledged DEBT in
//     `pattern_gloss_owed.json`. `patternRules` is a mirror of a mirror, and an
//     unmaintained mirror is the failure mode AGENTS.md item 1 is about. The
//     reconciliation itself is a pure function (`glossLedgerProblems`) so each of
//     its four classes can be broken and watched to fail on its own message —
//     see TestGlossLedgerReconciliation.
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
// 🔴 AND THAT FIX MOVED THE SELF-BLOCK ONE FIELD OVER RATHER THAN REMOVING IT,
// which is why this paragraph must not be read as "resolved". The rewrite freed
// `scopes` and then fed the SCHEMA's `contentRating` enum into the shape anchor
// below, so a canonical growing the RATINGS reddened this test exactly as an
// added scope used to — and `ci.yml` runs `schema-drift` on every PR in the repo
// with no path filter, so that reds an unrelated human's check until someone
// hand-edits a literal. The anchor now compares two hand-written operands and
// reads no schema at all.
//
// ⚠ IT ALSO HAD A SECOND, INDEPENDENT SPELLING in
// `internal/cmd/message_quality_test.go`, which is the part that makes a one-site
// fix read as complete when it is not. Measured there, and the asymmetry is the
// reusable half: that site compared with `strings.Contains`, so APPENDING a
// rating left it GREEN — the literal is a prefix of the longer message — while
// this file went red. Only an INSERT or a reorder reddened both. A fix verified
// with an append-only mutant therefore looks finished and is not.
// `content_rating_literal_ledger_test.go` is what now fails if either literal
// comes back.
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
	//
	// 🔴 BOTH OPERANDS ARE HAND-WRITTEN AND NEITHER READS THE SCHEMA. That is
	// the whole point of this anchor and it is why it cannot self-block: the
	// list below is a FROZEN SPECIMEN of an enum, not a mirror of the live
	// `contentRating` set, so a canonical that grows the ratings cannot reach
	// this comparison. The previous version fed `schemaEnum(…"contentRating"…)`
	// into the template and compared it against this literal, which made the
	// anchor red on exactly the additive canonical change the re-vendor bot
	// exists to land — the same self-block this guard was rewritten to remove,
	// moved one field over rather than removed. Do NOT re-derive either side
	// here; the schema-sensitive claim is the `want` map below.
	//
	// What this still catches, which is the only thing it claims: a change to
	// `enumMessage` itself. A reword of the library's lead-in shows up in the
	// `want` comparison below (template vs REAL output), and the obvious way to
	// paper that over is to edit the template to match — which reds this.
	//
	// ⚠ What it STOPPED catching, and where that went: as a schema-vs-frozen-string
	// comparison it also detected a rating being RETIRED or REORDERED upstream.
	// Making both operands hand-written gave that up, so it is restored explicitly
	// as the subset assertion further down — do not assume this anchor still covers
	// it.
	// frozenRatings is that specimen, named because it has TWO jobs: anchoring the
	// template here, and the subset assertion below. It is deliberately the only
	// hand-typed rating list in this file besides the constant — a third spelling
	// would trip `content_rating_literal_ledger_test.go`'s per-file count, which is
	// how that ledger notices a new mirror.
	frozenRatings := []string{"g", "pg", "pg13", "r", "x"}

	const wantContentRating = "contentRating: value must be one of 'g', 'pg', 'pg13', 'r', 'x'"
	if tmpl := enumMessage("contentRating", frozenRatings); tmpl != wantContentRating {
		t.Fatalf("the enum message TEMPLATE no longer reproduces a hand-written message, so every "+
			"expectation derived from it below is meaningless — this is about `enumMessage` in this "+
			"file, NOT about the schema's contentRating enum, which this comparison does not read\n"+
			"  want: %s\n  got:  %s", wantContentRating, tmpl)
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
	// enum leaves this whole package GREEN.
	//
	// 🔴 AND `schema-drift` DOES NOT COVER THAT GAP — an earlier version of this
	// comment called it "a strictly stronger instrument than a hand-typed list
	// here", and that was FALSE. `scripts/check-canonical-schema.sh` runs
	// `diff <(jq -S . "$VENDORED") <(jq -S . "$tmp")`: it compares the mirror
	// against the LIVE canonical, so it only ever sees the two DISAGREEING. It
	// reds on a value dropped from the mirror alone — and is blind to a value the
	// CANONICAL itself retires, because once the mirror resyncs both sides agree
	// and the diff is empty. A hand-typed list here is the only thing that could
	// see an upstream retirement, and this list does not attempt it: it guards
	// two members, not the set.
	//
	// So the shrink direction is covered for mirror-only drops and UNCOVERED for
	// upstream retirements. Closing it properly means a growth-tolerant SHRINK
	// ledger (assert a known set is a SUBSET of the schema's, so growth passes
	// and a retirement reds) — deliberately not built here, because it is a new
	// guard rather than the removal of a self-block, and a ledger written wrong
	// re-creates exactly the block this file was rewritten to remove.
	for _, must := range []string{"models:read:self", "posts:write:self"} {
		if !slices.Contains(scopes, must) {
			t.Fatalf("the schema's scope enum does not contain %q — %d values read, so this "+
				"is reading the wrong node or the canonical lost a scope: %v", must, len(scopes), scopes)
		}
	}

	// Both rows DERIVE their values from the schema and spell the lead-in by
	// hand, so an enum that grows moves `want` and `got` together (green) while a
	// reword of the library's rendering moves only `got` (red). `contentRating`
	// used to be pinned to the literal above; that is what blocked an additive
	// canonical, so it derives here like `scopes` always has.
	ratings := schemaEnum(t, "properties", "contentRating", "enum")

	// 🔴 SHRINK GUARD ON `contentRating`, AND IT EXISTS BECAUSE THIS CHANGE TOOK
	// THE OLD ONE AWAY. Before, the anchor compared the SCHEMA's ratings against a
	// frozen string, so it doubled as a content guard: a rating retired or
	// reordered upstream reddened it. Now both anchor operands are hand-written and
	// the `want` row derives, so schema and expectation move together in EVERY
	// direction — measured, dropping `"x"` or reordering the list left this whole
	// package green where it used to fail. `schema-drift` cannot cover that: it
	// diffs the mirror against the live canonical, so an upstream retirement is
	// invisible to it once the mirror resyncs (see the `scopes` note above).
	//
	// A SUBSET assertion restores the direction WITHOUT self-blocking, which is the
	// distinction that matters: growth adds values and a superset still contains
	// the specimen, so an additive canonical stays green — while a retirement or a
	// rename reds here. It reuses `frozenRatings` rather than typing the list a
	// third time.
	//
	// ⚠ IT COVERS RETIREMENT AND RENAME, NOT REORDER — measured, both ways, so this
	// is a scope statement and not an aspiration: dropping `"x"` reds this loop,
	// reordering to `["pg","g","pg13","r","x"]` leaves the whole package green.
	// That is deliberate. A JSON Schema `enum` is a SET, so a reorder changes no
	// manifest's validity — only the order values are rendered in — and the one
	// guard that would catch it (assert the specimen is an ordered PREFIX) would
	// red on a legitimate mid-list insert, which is the self-block this whole
	// change exists to remove. Cosmetic order is the acceptable blind spot; a
	// value disappearing is not.
	for _, must := range frozenRatings {
		if !slices.Contains(ratings, must) {
			t.Errorf("the schema's contentRating enum no longer offers %q — %d values read: %v.\n"+
				"  A rating was RETIRED or RENAMED upstream. That is a real contract change and nothing "+
				"else in this repo can see it: `schema-drift` compares the mirror against the live "+
				"canonical, so once the mirror resyncs both sides agree. If the removal is intended, "+
				"drop it from `frozenRatings` and from the expected-message constant in the same commit.",
				must, len(ratings), ratings)
		}
	}

	want := map[string]string{
		"contentRating": enumMessage("contentRating", ratings),
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

// glossOwedPath is the committed ledger of schema patterns that have NO gloss
// and whose absence has been ACKNOWLEDGED. Relative, because `go test` runs with
// the package directory as its working directory.
const glossOwedPath = "pattern_gloss_owed.json"

// glossOwedWhy is written into the file so a human who opens it cold — most
// likely a reviewer of the re-vendor bot's PR, seeing a row appear — learns what
// the row means without leaving the diff.
const glossOwedWhy = "Schema `pattern`s that have NO author-facing gloss in " +
	"internal/validate/pattern.go, and whose absence has been ACKNOWLEDGED. A row here is a " +
	"DEBT, not a decision: an author who trips this pattern gets the bare regex back. Clear a " +
	"row by writing its gloss in patternRules. See the header of internal/validate/pattern.go " +
	"and TestPatternRulesCoverTheVendoredSchema."

// updateGlossLedger rewrites glossOwedPath instead of asserting against it.
//
// 🔴 THIS FLAG IS THE WHOLE POINT OF THE REDESIGN, so read why before deleting
// it. `schema/` is re-vendored from the live canonical by
// `.github/workflows/revendor-canonical-schema.yml`, which runs `go test ./...`
// on its own output BEFORE opening a PR. The growth half of this ledger used to
// be an unconditional failure, so a canonical that added a `pattern` made the
// automation whose entire job is to land that change unable to land it — twice,
// issues #486 and #743.
//
// 🔴 THE COST WAS NEVER "THE BOT STALLED", which is why two instances are worth
// a redesign. `ci.yml`'s `schema-drift` job runs on `pull_request:` with no path
// filter — every PR in this repo — and it compares the vendored mirror against
// the live canonical, so it stays RED for as long as the mirror is stale. A
// blockage here therefore reddens a check on every unrelated human PR, and the
// only thing that clears it is a human writing a gloss. Both instances were TRUE
// POSITIVES and both were cleared fast — measured from the issues, #486 was open
// 31 minutes (2026-08-24 18:45→19:15Z) and #743 eleven hours (2026-09-28
// 04:51→15:51Z). Zero false positives.
//
// ⚠️ RETRACTED, and recorded rather than quietly dropped: an earlier revision of
// this comment said "five times, issues #323 #486 #607 #695 #743, roughly
// monthly", implying one recurring class under one title. Measured from the
// issues and their run logs, that is wrong in the direction that inflates the
// case. #323 was a HAND-DISPATCHED FIRE DRILL whose own body says "NOT a real
// failure of the bot", filed under the different title "could not re-vendor";
// #607 was `TestEnumFindingsKeepTheirExactWording` alone (the enum half, fixed
// by #745); #695 was `TestBuildExcludesGitFileInSubmodule` in `internal/pkgzip`,
// unrelated to the schema, which files under the same headline only because the
// bot's validate gate is `go test ./...`. So: three schema-shape blockages
// (#486, #607, #743), of which two were this guard.
//
// A gloss is English prose about what a regex MEANS and cannot be derived, so
// the bot cannot WRITE one. What it can do is RECORD that one is owed, which is
// what this flag does: the debt lands in the PR diff where a reviewer is already
// looking, and the workflow keeps a `gloss-owed` issue open until the row is
// cleared. The author-facing behaviour is unchanged either way — patternAdvice
// already fails soft and emits exactly the base library message for an unglossed
// pattern (TestPatternAdviceFailsSoft), so this was never a correctness gate.
// It is a NOTIFICATION gate, and the fix moves the notification off the path
// that blocks the bot rather than deleting it.
var updateGlossLedger = flag.Bool("update-gloss-ledger", false,
	"rewrite pattern_gloss_owed.json so every vendored-schema pattern with no gloss is "+
		"recorded as an acknowledged debt, instead of failing on it")

// glossOwedFile is the on-disk shape of the ledger.
type glossOwedFile struct {
	Why  string   `json:"_why"`
	Owed []string `json:"owed"`
}

// loadGlossOwed loads the acknowledged-debt ledger, returning an error rather
// than failing a test, so the ways it must REFUSE a file can themselves be
// asserted (TestGlossOwedFileRoundTrips) instead of only being reached by
// accident.
//
// Every way of not getting a usable list is an error, because an expectation
// built on a silently-empty list is vacuous rather than wrong: the totality
// assertion compares "unglossed" against "acknowledged", and an acknowledgement
// set that quietly read as empty would demand a gloss for everything (loud,
// fine) — but one that quietly read as FULL would demand nothing, which is the
// exact failure this ledger exists to prevent. Unknown fields are rejected so a
// typo'd key (`owned`) cannot decode to an empty list and read as "nothing is
// owed".
func loadGlossOwed(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unreadable (it must exist even when its list is empty — a "+
			"missing file would leave the coverage assertion with nothing to compare against): %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var f glossOwedFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("does not decode (unknown keys are rejected on purpose: a typo'd "+
			"key would decode to an EMPTY owed list and read as \"no gloss is owed\"): %w", err)
	}
	if !slices.IsSorted(f.Owed) {
		return nil, fmt.Errorf("`owed` is not sorted — the file is machine-written (see "+
			"-update-gloss-ledger) and an unsorted list produces a churning diff: %v", f.Owed)
	}
	for i := 1; i < len(f.Owed); i++ {
		if f.Owed[i] == f.Owed[i-1] {
			return nil, fmt.Errorf("`owed` lists %q twice", f.Owed[i])
		}
	}
	return f.Owed, nil
}

// readGlossOwed is loadGlossOwed with every refusal promoted to a FATAL: a
// reconciliation run against a ledger we could not read is not a weaker check,
// it is no check.
func readGlossOwed(t *testing.T, path string) []string {
	t.Helper()
	owed, err := loadGlossOwed(path)
	if err != nil {
		t.Fatalf("the acknowledged-debt ledger %s: %v", path, err)
	}
	return owed
}

// writeGlossOwed rewrites the ledger. owed must be non-nil so an empty ledger
// marshals as `[]` rather than `null` — `null` would decode back to an empty
// slice and be indistinguishable, but it reads in the diff like a broken file.
func writeGlossOwed(path string, owed []string) error {
	if owed == nil {
		owed = []string{}
	}
	raw, err := json.MarshalIndent(glossOwedFile{Why: glossOwedWhy, Owed: owed}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// unglossedPatterns returns, sorted, every pattern the vendored schema can
// surface as a kind.Pattern finding that patternRules has no gloss for.
//
// Sorted because the result is written to a committed file: an unordered walk
// over a Go map would rewrite that file's line order on every run.
func unglossedPatterns(inSchema map[string]bool, glossed map[string]patternRule) []string {
	out := []string{}
	for p := range inSchema {
		if _, ok := glossed[p]; !ok {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// glossLedgerProblems reconciles the three sets that must agree — the patterns
// the vendored schema can surface, the patterns `patternRules` glosses, and the
// patterns whose missing gloss has been acknowledged — and returns one message
// per disagreement, sorted so the output is stable.
//
// It is a PURE FUNCTION over its three arguments rather than a run of `t.Error`
// calls inside the test, so each of its four classes can be driven with a
// synthetic input and watched to fail on ITS OWN message
// (TestGlossLedgerReconciliation). Breaking a class by mutating the real schema
// instead would take two or three of the other tests in this file down with it,
// and a mutant killed by a different guard's error is green for the wrong
// reason.
//
// The four classes, and what each one is for:
//
//   - UNACKNOWLEDGED — the schema grew a pattern, nobody glossed it and nobody
//     recorded the debt. This is the growth direction, and it is the ONLY class
//     the re-vendor bot can clear on its own (with -update-gloss-ledger).
//   - STALE DEBT — an acknowledgement for a pattern the schema no longer has.
//   - CONTRADICTION — a pattern both glossed and listed as owed, so the ledger
//     no longer says which patterns are explained.
//   - STALE GLOSS — a gloss for a pattern the schema no longer has. This is the
//     SHRINK direction, it is deliberately NOT mechanically clearable, and it
//     still blocks the bot: deleting English prose is a human call, and a table
//     row claiming to explain a rule that no longer exists reads as coverage.
func glossLedgerProblems(inSchema map[string]bool, glossed map[string]patternRule, owed []string) []string {
	owedSet := make(map[string]bool, len(owed))
	for _, p := range owed {
		owedSet[p] = true
	}
	var problems []string
	for _, p := range unglossedPatterns(inSchema, glossed) {
		if !owedSet[p] {
			problems = append(problems, fmt.Sprintf("UNACKNOWLEDGED: schema pattern %q has no gloss "+
				"in patternRules — an author hitting it gets a bare regex back and nobody has been "+
				"told. Write the gloss (plus its ledger row in TestPatternGlossesAreTheRightWayRound "+
				"and its fixture in patternFixtures), or record the debt with: "+
				"go test ./internal/validate -run TestPatternRulesCoverTheVendoredSchema "+
				"-update-gloss-ledger -count=1", p))
		}
	}
	for _, p := range owed {
		if !inSchema[p] {
			problems = append(problems, fmt.Sprintf("STALE DEBT: %s acknowledges %q, which is not a "+
				"reachable pattern in the vendored schema — an acknowledgement for a rule that no "+
				"longer exists reads like a pending task that can never be done", glossOwedPath, p))
		}
		if _, ok := glossed[p]; ok {
			problems = append(problems, fmt.Sprintf("CONTRADICTION: %q is BOTH glossed in patternRules "+
				"and acknowledged as unglossed in %s — delete the ledger row", p, glossOwedPath))
		}
	}
	for p := range glossed {
		if !inSchema[p] {
			problems = append(problems, fmt.Sprintf("STALE GLOSS: patternRules glosses %q, which is not "+
				"a reachable pattern in the vendored schema — a stale row looks like coverage", p))
		}
	}
	slices.Sort(problems)
	return problems
}

// TestGlossLedgerUpdateFlagDefaultsOff is the guard for the one mutation that
// would leave every other guard in this file GREEN while disarming the ledger
// completely: `-update-gloss-ledger` defaulting to true.
//
// With it on, TestPatternRulesCoverTheVendoredSchema regenerates the ledger and
// returns before asserting anything, on every ordinary run — so the coverage
// claim silently becomes "whatever the schema says", and nothing anywhere fails.
// A one-character edit to the flag declaration does it. DefValue is read rather
// than the pointer so passing the flag deliberately (as the bot does) cannot
// satisfy this test by accident.
func TestGlossLedgerUpdateFlagDefaultsOff(t *testing.T) {
	f := flag.Lookup("update-gloss-ledger")
	if f == nil {
		t.Fatal("the -update-gloss-ledger flag is not registered — the bot's reconcile step " +
			"passes it and `go test` rejects an unknown flag, so this would break the re-vendor " +
			"workflow rather than this suite")
	}
	if f.DefValue != "false" {
		t.Fatalf("-update-gloss-ledger defaults to %q. On by default, "+
			"TestPatternRulesCoverTheVendoredSchema rewrites the ledger and returns without "+
			"asserting on EVERY run, and the coverage guard is inert with nothing to say so.", f.DefValue)
	}
}

// TestUnglossedPatternsIsSortedAndTotal pins the two properties the WRITER
// depends on, which the round-trip test cannot see because it feeds an
// already-sorted list.
//
// Sortedness is not cosmetic here: the result is written to a committed file, so
// an unsorted walk over a Go map rewrites that file's line order on every run and
// the bot opens a PR churning the ledger whether or not anything changed.
func TestUnglossedPatternsIsSortedAndTotal(t *testing.T) {
	// Deliberately in non-alphabetical order, and enough of them that an
	// unsorted map walk coming out sorted by chance is negligible (1 in 8!).
	unglossed := []string{"^h$", "^b$", "^f$", "^a$", "^g$", "^c$", "^e$", "^d$"}
	inSchema := map[string]bool{"^glossed$": true}
	for _, p := range unglossed {
		inSchema[p] = true
	}
	got := unglossedPatterns(inSchema, map[string]patternRule{"^glossed$": {rule: "r", example: "e"}})
	if !slices.IsSorted(got) {
		t.Errorf("unglossedPatterns must return a sorted list — the ledger it writes is "+
			"committed, and an unstable order churns the file on every run: %v", got)
	}
	want := slices.Clone(unglossed)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("unglossedPatterns\n  want: %v\n  got:  %v", want, got)
	}
}

// TestGlossLedgerReconciliation drives each class of glossLedgerProblems with a
// synthetic input, so a deleted branch fails HERE, by name, on its own message.
//
// The fixture patterns are pairwise non-substring so a message naming the wrong
// one is visible, and none of them is a real schema pattern — a fixture that
// happened to equal a live regex would let the all-clear case pass for the wrong
// reason.
func TestGlossLedgerReconciliation(t *testing.T) {
	const (
		glossedPat  = "^fixture-alpha$"
		owedPat     = "^fixture-bravo$"
		strayPat    = "^fixture-charlie$"
		vanishedPat = "^fixture-delta$"
	)
	for _, p := range []string{glossedPat, owedPat, strayPat, vanishedPat} {
		if _, live := patternRules[p]; live {
			t.Fatalf("fixture %q is a REAL glossed pattern — the cases below would be testing "+
				"the production table instead of the reconciler", p)
		}
	}
	gloss := func(pats ...string) map[string]patternRule {
		m := map[string]patternRule{}
		for _, p := range pats {
			m[p] = patternRule{rule: "r " + p, example: "e " + p}
		}
		return m
	}
	schema := func(pats ...string) map[string]bool {
		m := map[string]bool{}
		for _, p := range pats {
			m[p] = true
		}
		return m
	}

	cases := []struct {
		name string
		// wantClass is the prefix the single problem must carry. Empty means the
		// input must produce NO problems at all.
		wantClass string
		// wantPattern is the regex the problem must name. Asserted separately
		// from the class so a branch that fires on the wrong pattern is visible.
		wantPattern string
		inSchema    map[string]bool
		glossed     map[string]patternRule
		owed        []string
	}{
		{
			// NEGATIVE CONTROL for the whole function: a fully reconciled ledger
			// must be silent, or every "exactly one problem" below could be
			// satisfied by a reconciler that complains about everything.
			name:     "reconciled",
			inSchema: schema(glossedPat, owedPat),
			glossed:  gloss(glossedPat),
			owed:     []string{owedPat},
		},
		{
			name:        "a new schema pattern with no gloss and no acknowledgement",
			wantClass:   "UNACKNOWLEDGED",
			wantPattern: strayPat,
			inSchema:    schema(glossedPat, owedPat, strayPat),
			glossed:     gloss(glossedPat),
			owed:        []string{owedPat},
		},
		{
			name:        "an acknowledgement the schema no longer has a pattern for",
			wantClass:   "STALE DEBT",
			wantPattern: vanishedPat,
			inSchema:    schema(glossedPat),
			glossed:     gloss(glossedPat),
			owed:        []string{vanishedPat},
		},
		{
			name:        "a pattern both glossed and acknowledged",
			wantClass:   "CONTRADICTION",
			wantPattern: glossedPat,
			inSchema:    schema(glossedPat),
			glossed:     gloss(glossedPat),
			owed:        []string{glossedPat},
		},
		{
			name:        "a gloss the schema no longer has a pattern for",
			wantClass:   "STALE GLOSS",
			wantPattern: vanishedPat,
			inSchema:    schema(glossedPat),
			glossed:     gloss(glossedPat, vanishedPat),
			owed:        nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := glossLedgerProblems(tc.inSchema, tc.glossed, tc.owed)
			if tc.wantClass == "" {
				if len(got) != 0 {
					t.Fatalf("a reconciled ledger must produce no problems, got %d:\n%s",
						len(got), strings.Join(got, "\n"))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want exactly 1 %s problem, got %d:\n%s",
					tc.wantClass, len(got), strings.Join(got, "\n"))
			}
			if !strings.HasPrefix(got[0], tc.wantClass+":") {
				t.Errorf("problem is not a %s:\n%s", tc.wantClass, got[0])
			}
			if !strings.Contains(got[0], tc.wantPattern) {
				t.Errorf("problem does not name %q:\n%s", tc.wantPattern, got[0])
			}
		})
	}
}

// TestGlossOwedFileRoundTrips pins the file format against the pair that writes
// and reads it, in a temp dir so the committed ledger is untouched.
//
// It exists because -update-gloss-ledger makes the WRITER the thing the bot
// relies on: a writer that emitted an unsorted list, dropped an entry, or wrote
// a key the reader rejects would leave the bot committing a file that reds the
// very suite it just ran.
func TestGlossOwedFileRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), glossOwedPath)
	want := []string{"^aaa$", "^bbb$", "^ccc$"}
	if err := writeGlossOwed(path, want); err != nil {
		t.Fatalf("writeGlossOwed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Error("the ledger must end in a newline — a file without one shows as a no-newline " +
			"marker in every later diff")
	}
	if got := readGlossOwed(t, path); !slices.Equal(got, want) {
		t.Errorf("round trip\n  want: %v\n  got:  %v", want, got)
	}

	// An empty ledger must marshal as `[]`, not `null` — asserted on the BYTES,
	// because both decode to a zero-length slice and the round trip above cannot
	// tell them apart.
	if err := writeGlossOwed(path, nil); err != nil {
		t.Fatalf("writeGlossOwed(nil): %v", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"owed": []`)) {
		t.Errorf("an empty ledger must be written as `\"owed\": []`:\n%s", raw)
	}

	// NEGATIVE CONTROLS on the loader: each of these must be REFUSED, because
	// each one decodes to a plausible-looking list that the reconciliation would
	// then trust. Graded on the error message so a refusal for the wrong reason is
	// not counted as a kill.
	for _, bad := range []struct {
		name, body, wantErr string
	}{
		{
			// The expensive one: `owned` decodes to an EMPTY list under a
			// permissive decoder, which reads as "no gloss is owed" — a
			// reassuring zero from a file that is not wired to anything.
			name: "the list key is misspelled", body: `{"_why":"x","owned":["^zzz$"]}`,
			wantErr: "does not decode",
		},
		{
			name: "the list is unsorted", body: `{"_why":"x","owed":["^bbb$","^aaa$"]}`,
			wantErr: "not sorted",
		},
		{
			name: "the list repeats an entry", body: `{"_why":"x","owed":["^aaa$","^aaa$"]}`,
			wantErr: "twice",
		},
	} {
		t.Run(bad.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), glossOwedPath)
			if err := os.WriteFile(p, []byte(bad.body+"\n"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			owed, err := loadGlossOwed(p)
			if err == nil {
				t.Fatalf("accepted, returning %v — the reconciliation would have trusted it", owed)
			}
			if !strings.Contains(err.Error(), bad.wantErr) {
				t.Errorf("refused for the wrong reason\n  want an error mentioning: %s\n  got: %v",
					bad.wantErr, err)
			}
		})
	}
	// POSITIVE CONTROL for the three rows above: a well-formed ledger must be
	// ACCEPTED, or a loader that refused everything would satisfy all of them.
	good := filepath.Join(t.TempDir(), glossOwedPath)
	if err := os.WriteFile(good, []byte(`{"_why":"x","owed":["^aaa$","^bbb$"]}`+"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if owed, err := loadGlossOwed(good); err != nil {
		t.Errorf("a well-formed ledger was refused: %v", err)
	} else if !slices.Equal(owed, []string{"^aaa$", "^bbb$"}) {
		t.Errorf("a well-formed ledger loaded as %v", owed)
	}
}

// TestPatternRulesCoverTheVendoredSchema is a TOTAL ledger over the vendored
// schema's patterns: each one is either glossed in `patternRules` or recorded as
// an acknowledged debt in pattern_gloss_owed.json.
//
// It is a GATE rather than a nicety because `schema/` is a vendored mirror
// (AGENTS.md item 1): the next sync with the server is exactly when this drifts,
// and the failure it produces is invisible in the output — a message that is
// merely terse, not wrong.
//
// 🔴 WHAT CHANGED AND WHY, because the obvious reading of this test is that it
// got weaker. The growth direction used to fail outright, which made it the
// second of the two guards that stopped the re-vendor bot from landing a
// canonical change (the first was the enum wording; see
// TestEnumFindingsKeepTheirExactWording's header). Growth is now clearable
// MECHANICALLY — by recording the debt, not by inventing prose — so the bot can
// land a pattern-adding canonical unaided while a human is still told. The
// notification moved, it was not deleted: the debt row lands in the bot's PR
// diff, and the workflow files a `gloss-owed` issue that stays open until the
// row is cleared. The SHRINK direction (STALE GLOSS) is unchanged and still
// blocks, deliberately.
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

	// 🔴 THE CONTROLS ABOVE ARE WHAT MAKES -update-gloss-ledger SAFE, so it is
	// placed AFTER them and not at the top of the test. A walker wired to nothing
	// returns an empty set; regenerating the ledger from THAT would write an empty
	// `owed` list, acknowledge nothing, and leave the reconciliation below
	// vacuously silent — a green suite over a ledger that describes no schema at
	// all. Rewriting only once the walker has been shown to observe the real
	// schema is the difference.
	if *updateGlossLedger {
		owed := unglossedPatterns(inSchema, patternRules)
		if err := writeGlossOwed(glossOwedPath, owed); err != nil {
			t.Fatalf("rewriting %s: %v", glossOwedPath, err)
		}
		t.Logf("-update-gloss-ledger: %s now records %d unglossed pattern(s) of %d in the schema: %v",
			glossOwedPath, len(owed), len(inSchema), owed)
		return
	}

	for _, problem := range glossLedgerProblems(inSchema, patternRules, readGlossOwed(t, glossOwedPath)) {
		t.Error(problem)
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
