package validate

import (
	"encoding/json"
	"strings"
	"testing"
)

// storage_collection_schema_test.go pins the CLI's offline verdict on a storage
// collection's `schema` (civitai/civitai#7491): the JSON Schema subset every
// document's `data` in the collection must match.
//
// AUTHORITY: civitai `src/shared/constants/block-storage.constants.ts`
// (`parseCollectionSchema`) and its tests
// (`block-manifest-validator.storage.test.ts`). Every refusal the server's
// parser makes and the vendored schema cannot is pinned here WHOLE, with the
// server's text; the expected values are literals, never derived from
// storageschema.go. The byte counts were measured with Node's
// `Buffer.byteLength(JSON.stringify(schema))`, the server's own computation.
//
// 🔴 EVERY REJECTION ROW CARRIES AN ACCEPT TWIN at the boundary. Before the
// re-vendor, `schema` was an unknown collection key refused for EVERY input, so
// a reject-only table would already have been green.

const schemaAt = "storage.collections.cards.schema"

// cardsManifest is a manifest that is clean apart from the one private
// collection `cards`, which declares `schema` (raw JSON).
func cardsManifest(schema string) string {
	return docManifest(oneCollection("cards", `{"visibility":"private","schema":`+schema+`}`), "apps:storage")
}

// mustJSON marshals v for splicing into a fixture.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertCleanResult requires no errors AND no warnings.
func assertCleanResult(t *testing.T, label, manifest string) {
	t.Helper()
	res := manifestOnlyResult(t, manifest)
	if len(res.Errors)+len(res.Warnings) != 0 {
		t.Errorf("%s: want a clean result, got errors:%s\n warnings:%s",
			label, describe(res.Errors), describe(res.Warnings))
	}
}

// cardSchema uses every keyword of the subset once, in a shape an app would
// really declare (the server test's CARD_SCHEMA).
const cardSchema = `{"type":"object","description":"A saved card.","required":["title","rating"],` +
	`"additionalProperties":false,"properties":{` +
	`"title":{"type":"string","minLength":1,"maxLength":80,"pattern":"^[^<>]*$"},` +
	`"rating":{"type":"integer","minimum":1,"maximum":5},` +
	`"tags":{"type":"array","minItems":0,"maxItems":6,"items":{"type":["string","null"]}},` +
	`"tone":{"enum":["warm","cool",7,null]},` +
	`"kind":{"const":"card"},` +
	`"scores":{"type":"object","additionalProperties":{"type":"number"}}}}`

// nestedSchema is a schema nested `levels` deep (the collection's own schema is
// level 1), stepping through properties.n, items and additionalProperties in
// turn so each counts as a level — the server test's `nested()`. It returns the
// schema and the dotted location of its innermost schema.
func nestedSchema(levels int) (map[string]any, string) {
	steps := []string{"properties.n", "items", "additionalProperties"}
	schema := map[string]any{"type": "string"}
	var path []string
	for level := levels - 1; level >= 1; level-- {
		step := steps[level%len(steps)]
		path = append([]string{step}, path...)
		switch step {
		case "properties.n":
			schema = map[string]any{"type": "object", "properties": map[string]any{"n": schema}}
		case "items":
			schema = map[string]any{"type": "array", "items": schema}
		default:
			schema = map[string]any{"type": "object", "additionalProperties": schema}
		}
	}
	return schema, strings.Join(append([]string{schemaAt}, path...), ".")
}

// withPatterns is an object schema with n properties, each carrying a pattern.
func withPatterns(n int) map[string]any {
	props := map[string]any{}
	for i := 0; i < n; i++ {
		props["p"+itoaTest(i)] = map[string]any{"pattern": "^x", "maxLength": 5}
	}
	return map[string]any{"type": "object", "properties": props}
}

// TestCollectionSchemaAccepted is the ACCEPT arm. Red before the re-vendor: the
// collection's `additionalProperties:false` refused the `schema` key outright,
// so `civitai app validate` rejected every manifest that used the feature.
func TestCollectionSchemaAccepted(t *testing.T) {
	deep16, _ := nestedSchema(16)
	for _, tc := range []struct{ name, schema string }{
		{"every keyword of the subset", cardSchema},
		{"an empty schema (matches anything)", `{}`},
		{"additionalProperties true", `{"additionalProperties":true}`},
		{"nested exactly 16 deep", mustJSON(t, deep16)},
		{"exactly 16 patterns", mustJSON(t, withPatterns(16))},
		{"a pattern with maxLength 1000", `{"pattern":"x","maxLength":1000}`},
		{"a 256-character pattern", `{"pattern":"` + strings.Repeat("p", 256) + `","maxLength":1}`},
		{"each min equal to its max", `{"minLength":3,"maxLength":3,"minItems":2,"maxItems":2,"minimum":1.5,"maximum":1.5}`},
		{"a count of 2^53-1", `{"maxItems":9007199254740991}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertCleanResult(t, tc.name, cardsManifest(tc.schema))
		})
	}
	t.Run("a shared own collection", func(t *testing.T) {
		m := docManifest(oneCollection("cards", `{"visibility":"shared","write":"own","schema":`+cardSchema+`}`),
			"apps:storage", "apps:storage:publish")
		assertCleanResult(t, "shared own", m)
	})
}

// TestCollectionSchemaSchemaRulesStillReport — the keyword subset and each
// keyword's own shape are the VENDORED SCHEMA's to report (storageschema.go adds
// nothing there). These pin that the re-vendor carried them, at the right field,
// so the accept arm above is not passing because the schema stopped checking.
func TestCollectionSchemaSchemaRulesStillReport(t *testing.T) {
	for _, tc := range []struct{ name, schema, field, fragment string }{
		{"a keyword outside the subset", `{"properties":{"when":{"type":"string","format":"date-time"}}}`,
			schemaAt + ".properties.when", "additional properties 'format' not allowed"},
		{"a pattern with no maxLength", `{"pattern":"^a"}`, schemaAt, "missing property 'maxLength'"},
		{"a pattern with maxLength 1001", `{"pattern":"^a","maxLength":1001}`, schemaAt + ".maxLength", "maximum"},
		{"a 257-character pattern", `{"pattern":"` + strings.Repeat("p", 257) + `","maxLength":1}`,
			schemaAt + ".pattern", "maxLength"},
		{"a boolean items schema", `{"items":true}`, schemaAt + ".items", "want object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hit bool
			fs := manifestOnlyFindings(t, cardsManifest(tc.schema))
			for _, f := range fs {
				if f.Field == tc.field && strings.Contains(f.Message, tc.fragment) {
					hit = true
				}
			}
			if !hit {
				t.Errorf("want an error at %s mentioning %q, got:%s", tc.field, tc.fragment, describe(fs))
			}
		})
	}
}

// TestCollectionSchemaSizeIsJSONStringifyBytes — the server bounds the schema at
// 16384 bytes of `JSON.stringify` output, in UTF-8. Each row sits EXACTLY on the
// bound and then one character over it, and each exercises one way Go's own
// json.Marshal disagrees with JSON.stringify (it escapes < > & and U+2028 to six
// bytes, and writes -0 as "-0"), so a mirror that marshalled instead of counting
// fails here.
func TestCollectionSchemaSizeIsJSONStringifyBytes(t *testing.T) {
	sized := func(description string) string {
		return mustJSON(t, map[string]any{"type": "object", "description": description})
	}
	refusal := func(n string) string {
		return schemaAt + " is " + n + " bytes of JSON — a collection schema may be at most 16384 bytes"
	}
	// {"type":"object","description":""} is 34 bytes before the description.
	for _, tc := range []struct {
		name      string
		at, over  string
		overBytes string
	}{
		{"ASCII", strings.Repeat("d", 16350), strings.Repeat("d", 16351), "16385"},
		{"< written as itself", strings.Repeat("<", 16350), strings.Repeat("<", 16351), "16385"},
		{"U+2028 written as itself (3 bytes)", strings.Repeat(" ", 5450), strings.Repeat(" ", 5451), "16387"},
		{"a control character as \\u0001 (6 bytes)", strings.Repeat("\x01", 2725), strings.Repeat("\x01", 2726), "16390"},
		{"a quote as \\\" (2 bytes)", strings.Repeat(`"`, 8175), strings.Repeat(`"`, 8176), "16386"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertCleanResult(t, "at the bound", cardsManifest(sized(tc.at)))
			assertOnlyError(t, cardsManifest(sized(tc.over)), schemaAt, refusal(tc.overBytes))
		})
	}
	t.Run("counted in UTF-8 bytes, not characters", func(t *testing.T) {
		// 9,000 characters, 18,000 bytes: a bound in characters would admit it.
		assertOnlyError(t, cardsManifest(sized(strings.Repeat("é", 9000))), schemaAt, refusal("18034"))
	})
	t.Run("numbers as JavaScript prints them", func(t *testing.T) {
		// JSON.stringify writes this enum as [1e+21,1.5e-7,0,100,100000000000000000000,0.1]:
		// 72 bytes with an empty description.
		numbers := func(pad int) string {
			return `{"enum":[1e21,1.5e-7,-0,100.0,1e20,0.1],"description":"` + strings.Repeat("d", pad) + `"}`
		}
		assertCleanResult(t, "at the bound", cardsManifest(numbers(16312)))
		assertOnlyError(t, cardsManifest(numbers(16313)), schemaAt, refusal("16385"))
	})
}

// TestCollectionSchemaDepth — at most 16 schemas nested in one another, the
// collection's own counting as 1. The refusal names the innermost location.
func TestCollectionSchemaDepth(t *testing.T) {
	deep16, _ := nestedSchema(16)
	assertCleanResult(t, "16 deep", cardsManifest(mustJSON(t, deep16)))
	deep17, innermost := nestedSchema(17)
	assertOnlyError(t, cardsManifest(mustJSON(t, deep17)), innermost,
		innermost+" is nested too deeply — a collection schema may nest at most 16 schemas")
}

// TestCollectionSchemaPatternCount — at most 16 patterns in one collection schema.
func TestCollectionSchemaPatternCount(t *testing.T) {
	assertCleanResult(t, "16 patterns", cardsManifest(mustJSON(t, withPatterns(16))))
	assertOnlyError(t, cardsManifest(mustJSON(t, withPatterns(19))), schemaAt,
		schemaAt+" declares 19 patterns — a collection schema may declare at most 16")
}

// TestCollectionSchemaMinAboveMax — each lower bound above its upper bound, one
// pair at a time. The twins (each min equal to its max) are in the accept arm.
func TestCollectionSchemaMinAboveMax(t *testing.T) {
	for _, tc := range []struct{ schema, field, message string }{
		{`{"minLength":9,"maxLength":4}`, schemaAt + ".minLength",
			schemaAt + ".minLength (9) is greater than maxLength (4), so nothing could match"},
		{`{"minItems":3,"maxItems":1}`, schemaAt + ".minItems",
			schemaAt + ".minItems (3) is greater than maxItems (1), so nothing could match"},
		{`{"minimum":2.5,"maximum":-1}`, schemaAt + ".minimum",
			schemaAt + ".minimum (2.5) is greater than maximum (-1), so nothing could match"},
		{`{"minimum":1e21,"maximum":1e20}`, schemaAt + ".minimum",
			schemaAt + ".minimum (1e+21) is greater than maximum (100000000000000000000), so nothing could match"},
	} {
		t.Run(tc.schema, func(t *testing.T) {
			assertOnlyError(t, cardsManifest(tc.schema), tc.field, tc.message)
		})
	}
}

// TestCollectionSchemaCountAboveSafeInteger — the server takes a count keyword
// only up to Number.MAX_SAFE_INTEGER; the schema's `integer` admits more. The
// twin (2^53-1) is in the accept arm.
func TestCollectionSchemaCountAboveSafeInteger(t *testing.T) {
	assertOnlyError(t, cardsManifest(`{"maxItems":9007199254740992}`), schemaAt+".maxItems",
		schemaAt+".maxItems must be a whole number of 0 or more")
}

// TestCollectionSchemaLongPropertyNameIsBoundedInTheMessage — the server's
// message cuts a property name to its first 80 UTF-16 code units (boundKey);
// the field keeps the whole name. 41 emoji are 82 code units but 41 code
// points, so a mirror counting runes would not cut it.
func TestCollectionSchemaLongPropertyNameIsBoundedInTheMessage(t *testing.T) {
	name := strings.Repeat("😀", 41)
	schema := `{"properties":{"` + name + `":{"minLength":3,"maxLength":2}}}`
	assertOnlyError(t, cardsManifest(schema), schemaAt+".properties."+name+".minLength",
		schemaAt+".properties."+strings.Repeat("😀", 40)+"….minLength (3) is greater than maxLength (2), so nothing could match")
}

// TestCollectionSchemaNotOnWriteNone — no document body is ever written to a
// write "none" collection. The vendored schema refuses the key there too, but
// its `false` schema reads "false schema", so the server's text stands beside it.
func TestCollectionSchemaNotOnWriteNone(t *testing.T) {
	const plays = `{"visibility":"shared","write":"none","counters":["plays"]`
	assertCleanResult(t, "twin without a schema", docManifest(oneCollection("plays", plays+`}`), "apps:storage"))

	fs := manifestOnlyFindings(t, docManifest(oneCollection("plays", plays+`,"schema":{}}`), "apps:storage"))
	const field = "storage.collections.plays.schema"
	want := field + ` does not apply to write "none" — no document body is ever written to that ` +
		"collection, so a schema would check nothing"
	var hit bool
	for _, f := range fs {
		if f.Field != field {
			t.Errorf("unexpected finding at %s: %s", f.Field, f.Message)
		}
		if f.Message == want {
			hit = true
		}
	}
	if !hit {
		t.Errorf("want the server's write \"none\" refusal at %s, got:%s", field, describe(fs))
	}
}

// patternWarning is the advisory for a pattern JavaScript's "u" mode refuses.
func patternWarning(at, reason string) string {
	return at + ` is likely refused at submit: civitai.com compiles it as a JavaScript regular expression ` +
		`with the "u" flag, which does not accept ` + reason + `. The server has the final say (it also ` +
		`refuses patterns open to catastrophic backtracking, which this CLI does not check).`
}

// patternManifest is cardsManifest with one schema whose pattern is p (the regex
// itself, JSON-encoded here so the table reads as the pattern, not its escaping).
func patternManifest(t *testing.T, p string) string {
	t.Helper()
	return cardsManifest(mustJSON(t, map[string]any{"pattern": p, "maxLength": 10}))
}

// Reason tails the table below pins. Literals, not storageschema.go's.
const (
	identityTail = ` (with "u", only ^ $ \ . * + ? ( ) [ ] { } | / may be escaped as themselves, and - only inside [...])`
	classTail    = ` inside [...] (with "u", only ^ $ \ . * + ? ( ) [ ] { } | / - may be escaped as themselves there)`
)

// TestCollectionSchemaPatternJSUModeSyntax — the server compiles each pattern
// with `new RegExp(p, "u")`. The CLI's check is a conservative syntax checker
// for that language (not Go's RE2, which disagrees with it in both directions).
// The REFUSED rows each throw a SyntaxError in Node 24 with "u" and draw exactly
// one WARNING (never an error: the server stays authoritative, ReDoS included);
// the ACCEPTED rows each compile in Node 24 with "u" and draw nothing. Several
// accepted rows are ones RE2 refuses (lookaround, backreferences, \u{…}, \cA,
// \k<n>), and several refused rows are ones RE2 accepts (\-, \_, lone braces,
// (?i), \z, \pL, [[:alpha:]], \Q…\E, (?P<n>…)) — so an RE2-based check fails
// both arms.
func TestCollectionSchemaPatternJSUModeSyntax(t *testing.T) {
	const at = schemaAt + ".pattern"
	for _, tc := range []struct{ pattern, reason string }{
		{`a\-b`, `the escape \-` + identityTail},
		{`\_`, `the escape \_` + identityTail},
		{`\:`, `the escape \:` + identityTail},
		{`\#`, `the escape \#` + identityTail},
		{`\@`, `the escape \@` + identityTail},
		{`\z`, `the escape \z` + identityTail},
		{`\Qa.b\E`, `the escape \Q` + identityTail},
		{`{`, `a lone { (write \{ for the character itself)`},
		{`}`, `a lone } (write \} for the character itself)`},
		{`]`, `a lone ] (write \] for the character itself)`},
		{`a{`, `a lone { (write \{ for the character itself)`},
		{`a{,3}`, `a lone { (write \{ for the character itself)`},
		{`[[:alpha:]]`, `a lone ] (write \] for the character itself)`},
		{`(?i)x`, `the inline flag group (?i)`},
		{`(?s)x`, `the inline flag group (?s)`},
		{`\pL`, `\p not followed by {…} (a Unicode property is written \p{L})`},
		{`\PL`, `\P not followed by {…} (a Unicode property is written \P{L})`},
		{`(?P<n>x)`, `the Python-style named group (?P<…>) (JavaScript writes (?<name>…))`},
		{`([a-z]`, `a ( with no matching )`},
		{`a)`, `a ) with no matching (`},
		{`[a`, `a [ with no matching ]`},
		{`a\`, `a trailing \`},
		{`\x4`, `\x not followed by two hex digits`},
		{`\u12`, `\u not followed by four hex digits or a {…} code point`},
		{`\u{110000}`, `\u not followed by four hex digits or a {…} code point`},
		{`\c1`, `\c not followed by a letter`},
		{`\k`, `\k not followed by <name>`},
		{`\00`, `\0 followed by a digit`},
		{`[\B]`, `the escape \B` + classTail},
		{`[\z]`, `the escape \z` + classTail},
	} {
		t.Run("refused "+tc.pattern, func(t *testing.T) {
			res := manifestOnlyResult(t, patternManifest(t, tc.pattern))
			if len(res.Errors) != 0 {
				t.Errorf("want no errors, got:%s", describe(res.Errors))
			}
			want := patternWarning(at, tc.reason)
			if len(res.Warnings) != 1 || res.Warnings[0].Field != at || res.Warnings[0].Message != want {
				t.Errorf("want exactly the advisory at %s:\n %s\ngot:%s", at, want, describe(res.Warnings))
			}
		})
	}
	for _, p := range []string{
		`(?=a)b`, `(?!a)b`, `(?<=a)b`, `(?<!a)b`,
		`(a)\1`, `(?<n>a)\k<n>`,
		`\u{1F600}`, `\u00e9`, `\x41`, `\cA`, `\0`,
		`\p{L}`, `\P{Lu}`,
		`[\-]`, `[a\-z]`, `[\b]`, `[{}()|]`, `[^]`,
		`\d\w\s\b\B\D\W\S`, `\/`, `\.\*\+\?\(\)\|\^\$\\\{\}\[\]`,
		`a{2,3}`, `a{2}`, `a{2,}?`,
		`^[^<>]*$`, `(?:a|b)+?`,
	} {
		t.Run("accepted "+p, func(t *testing.T) {
			assertCleanResult(t, p, patternManifest(t, p))
		})
	}
}

// TestCollectionSchemaPatternWarningsFollowTheServersWalk — a pattern the
// server never compiles draws no advisory: none in a schema it refuses for its
// size, none on write "none" (the size or write refusal is the whole story),
// and every one it does reach is reported at its own location.
func TestCollectionSchemaPatternWarningsFollowTheServersWalk(t *testing.T) {
	t.Run("nested patterns are each reported where they are", func(t *testing.T) {
		res := manifestOnlyResult(t, cardsManifest(
			`{"properties":{"a":{"pattern":"(?i)a","maxLength":3}},"items":{"pattern":"\\z","maxLength":3}}`))
		got := map[string]bool{}
		for _, w := range res.Warnings {
			got[w.Field] = true
		}
		for _, f := range []string{schemaAt + ".properties.a.pattern", schemaAt + ".items.pattern"} {
			if !got[f] {
				t.Errorf("no pattern advisory at %s:%s", f, describe(res.Warnings))
			}
		}
		if len(res.Warnings) != 2 {
			t.Errorf("want 2 advisories, got:%s", describe(res.Warnings))
		}
	})
	t.Run("none in a schema refused for its size", func(t *testing.T) {
		big := `{"pattern":"(?i)a","maxLength":3,"description":"` + strings.Repeat("d", 16400) + `"}`
		res := manifestOnlyResult(t, cardsManifest(big))
		if len(res.Warnings) != 0 {
			t.Errorf("want no advisory, got:%s", describe(res.Warnings))
		}
		if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].Message, "bytes of JSON") {
			t.Errorf("control: want the size refusal alone, got:%s", describe(res.Errors))
		}
	})
	t.Run("none on write none", func(t *testing.T) {
		res := manifestOnlyResult(t, docManifest(oneCollection("plays",
			`{"visibility":"shared","write":"none","counters":["plays"],"schema":{"pattern":"(?i)a","maxLength":3}}`),
			"apps:storage"))
		if len(res.Warnings) != 0 {
			t.Errorf("want no advisory, got:%s", describe(res.Warnings))
		}
	})
}

// schemaCollections is a manifest whose private collections c0, c1, … each declare a schema
// with counts[i] patterns, every pattern source distinct unless same is set.
func schemaCollections(t *testing.T, same bool, counts ...int) string {
	t.Helper()
	cols := map[string]any{}
	for i, n := range counts {
		props := map[string]any{}
		for j := 0; j < n; j++ {
			src := "^c" + itoaTest(i) + "x" + itoaTest(j)
			if same {
				src = "^same"
			}
			props["p"+itoaTest(j)] = map[string]any{"pattern": src, "maxLength": 9}
		}
		cols["c"+itoaTest(i)] = map[string]any{"visibility": "private",
			"schema": map[string]any{"type": "object", "properties": props}}
	}
	return docManifest(mustJSON(t, map[string]any{"collections": cols}), "apps:storage")
}

// TestCollectionSchemaManifestPatternCap — at most 32 patterns across ALL of a manifest's
// collection schemas (APP_STORAGE_MANIFEST_MAX_PATTERNS). Every pattern SITE counts: a source
// repeated in several places is counted each time. Only collections that parsed are counted,
// so one already over its own cap of 16 is not added.
func TestCollectionSchemaManifestPatternCap(t *testing.T) {
	const field = "storage.collections"
	assertCleanResult(t, "16 + 16", schemaCollections(t, false, 16, 16))
	assertOnlyError(t, schemaCollections(t, false, 16, 16, 5), field,
		"storage.collections declare 37 patterns across their schemas — a manifest may declare at most 32 in all")
	// The same source 33 times is 33 patterns: the cap counts sites, not distinct sources.
	assertOnlyError(t, schemaCollections(t, true, 11, 11, 11), field,
		"storage.collections declare 33 patterns across their schemas — a manifest may declare at most 32 in all")
	// c0 is refused on its own (17 > 16), so only c1 and c2 count: 32, under the cap.
	assertOnlyError(t, schemaCollections(t, false, 17, 16, 16), "storage.collections.c0.schema",
		"storage.collections.c0.schema declares 17 patterns — a collection schema may declare at most 16")
}
