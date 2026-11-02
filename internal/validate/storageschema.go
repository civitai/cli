package validate

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// storageschema.go mirrors the server's refusals of a storage collection's
// `schema` (the per-collection document schema, civitai/civitai#7491) that the
// vendored JSON Schema CANNOT express.
//
// AUTHORITY: civitai `src/shared/constants/block-storage.constants.ts` —
// `parseCollectionSchema` / `parseSchemaNode`, run by parseManifestStorage at
// submit, and the constants beside them (APP_STORAGE_SCHEMA_*). The vendored
// schema's `$defs.storageCollectionSchema` already states the keyword subset,
// the type names, each keyword's own shape, the 256-character `pattern` bound
// and "a pattern needs a maxLength of at most 1000"; those stay the schema's
// (same split as storagemanifest.go). What is left, and lives here:
//
//   - the size: at most 16384 bytes of the schema as UTF-8 JSON
//     (`JSON.stringify`, so no whitespace — see jsonStringifyBytes);
//   - the depth: at most 16 schemas nested in one another;
//   - the pattern count: at most 16 `pattern`s per collection schema;
//   - minLength/minItems/minimum not above its maxLength/maxItems/maximum;
//   - a count keyword above 2^53-1 (the schema's `integer` admits it);
//   - no schema on a `write: "none"` collection. The schema states this one too
//     (`then: {properties: {schema: false}}`), but a failing `false` schema
//     reads "false schema" with no reason, so this carries the server's text
//     beside it — the same posture as buildCommandRe in validate.go.
//
// HARD ERRORS with the server's text, like storagemanifest.go.
//
// 🔴 `pattern` IS NOT CHECKED FOR PARITY, AND MUST NOT CLAIM IT. The server
// compiles a pattern as an ECMAScript regular expression with the `u` flag;
// this CLI has Go's RE2, which is a different language. Neither direction
// implies the other: RE2 refuses valid ECMAScript (lookaround, backreferences,
// `\u{…}`), and RE2 accepts what ECMAScript-`u` refuses (`(?i)`, `\z`, `\pL`,
// `\Q…\E`). So a pattern RE2 cannot compile draws a WARNING that says the CLI
// cannot check it and the server decides (storageCollectionSchemaWarnings),
// and a pattern RE2 compiles draws nothing — which is NOT a statement that the
// server will accept it. The server's submit-time ReDoS gate (recheck) has no
// local counterpart either.
//
// KNOWN DIVERGENCES, each an edge no real schema reaches: the byte count of a
// string holding a lone UTF-16 surrogate (Go's decoder has already replaced it
// with U+FFFD, 3 bytes, where JSON.stringify writes the 6-byte escape); and a
// property name that is all digits renders as `[n]` in the schema finding's
// field (fieldPath), while the findings here keep it dotted.

// Mirrors of APP_STORAGE_SCHEMA_MAX_DEPTH / _MAX_BYTES / _MAX_PATTERNS /
// _PATTERN_MAX_LENGTH.
const (
	appStorageSchemaMaxDepth         = 16
	appStorageSchemaMaxBytes         = 16 * 1024
	appStorageSchemaMaxPatterns      = 16
	appStorageSchemaPatternMaxLength = 256
)

// maxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER, the server's upper
// bound on a count keyword (isIntegerIn(value, 0, Number.MAX_SAFE_INTEGER)).
const maxSafeInteger = 1<<53 - 1

// schemaCountKeywords / schemaBoundPairs mirror SCHEMA_COUNT_KEYWORDS and
// SCHEMA_BOUND_PAIRS.
var (
	schemaCountKeywords = []string{"minLength", "maxLength", "minItems", "maxItems"}
	schemaBoundPairs    = [][2]string{
		{"minLength", "maxLength"},
		{"minItems", "maxItems"},
		{"minimum", "maximum"},
	}
)

// collectionSchema is one declared `schema` the server goes on to parse: a
// collection under a valid name, with a valid visibility, that declares one.
type collectionSchema struct {
	at        string // "storage.collections.<name>.schema", the server's `at`
	raw       any
	writeNone bool // the collection is shared with write "none"
}

// collectionSchemas returns the declared schemas, in collection-name order.
// The server parses a collection's schema only after its visibility parsed
// (parseCollection returns before that otherwise), so neither does this.
func collectionSchemas(generic any) []collectionSchema {
	cols, ok := storageCollections(generic)
	if !ok {
		return nil
	}
	var out []collectionSchema
	for _, c := range cols {
		raw, has := c.decl["schema"]
		if !has {
			continue
		}
		vis, _ := c.decl["visibility"].(string)
		if vis != "private" && vis != "shared" {
			continue
		}
		write, _ := c.decl["write"].(string)
		out = append(out, collectionSchema{
			at:        childField(childField("storage.collections", c.name), "schema"),
			raw:       raw,
			writeNone: vis == "shared" && write == "none",
		})
	}
	return out
}

// schemaWalk is what one walk of a collection schema found: the imperative
// errors, and every pattern it reached (with where), in the server's walk order.
type schemaWalk struct {
	errors   []Finding
	patterns []schemaPattern
}

type schemaPattern struct {
	at      string // the `pattern` key's own location
	pattern any    // as declared; the schema reports a non-string
}

// walkCollectionSchema runs the server's parseCollectionSchema over one
// declared schema, reporting only the rules the vendored schema cannot. It
// returns nil when the schema is not an object (the schema's finding) or is
// over the size bound — the server stops at either, so nothing below them is
// said.
func walkCollectionSchema(cs collectionSchema) *schemaWalk {
	if _, ok := cs.raw.(map[string]any); !ok {
		return nil
	}
	w := &schemaWalk{}
	if n := jsonStringifyBytes(cs.raw); n > appStorageSchemaMaxBytes {
		w.errors = append(w.errors, newFinding(cs.at, fmt.Sprintf(
			"%s is %d bytes of JSON — a collection schema may be at most %d bytes",
			cs.at, n, appStorageSchemaMaxBytes)))
		return w
	}
	walkSchemaNode(w, cs.at, cs.at, cs.raw, 1)
	if len(w.patterns) > appStorageSchemaMaxPatterns {
		w.errors = append(w.errors, newFinding(cs.at, fmt.Sprintf(
			"%s declares %d patterns — a collection schema may declare at most %d",
			cs.at, len(w.patterns), appStorageSchemaMaxPatterns)))
	}
	return w
}

// walkSchemaNode mirrors parseSchemaNode. `field` is the dotted location; `at` is the
// server's own text for it, which bounds a long property name (boundKey).
func walkSchemaNode(w *schemaWalk, field, at string, node any, depth int) {
	if depth > appStorageSchemaMaxDepth {
		w.errors = append(w.errors, newFinding(field, fmt.Sprintf(
			"%s is nested too deeply — a collection schema may nest at most %d schemas",
			at, appStorageSchemaMaxDepth)))
		return
	}
	n, ok := node.(map[string]any)
	if !ok {
		return // "must be a schema object" — the schema's finding
	}
	child := func(key, label string, sub any) {
		walkSchemaNode(w, childField(field, key), at+"."+label, sub, depth+1)
	}
	if props, ok := n["properties"].(map[string]any); ok {
		for _, name := range sortedSchemaKeys(props) {
			walkSchemaNode(w, childField(childField(field, "properties"), name),
				at+".properties."+boundKey(name), props[name], depth+1)
		}
	}
	if ap, has := n["additionalProperties"]; has {
		if _, isBool := ap.(bool); !isBool {
			child("additionalProperties", "additionalProperties", ap)
		}
	}
	if items, has := n["items"]; has {
		child("items", "items", items)
	}
	for _, kw := range schemaCountKeywords {
		if v, ok := n[kw].(float64); ok && v == math.Trunc(v) && v > maxSafeInteger {
			w.errors = append(w.errors, newFinding(childField(field, kw), fmt.Sprintf(
				"%s.%s must be a whole number of 0 or more", at, kw)))
		}
	}
	for _, pair := range schemaBoundPairs {
		lo, okLo := n[pair[0]].(float64)
		hi, okHi := n[pair[1]].(float64)
		if okLo && okHi && lo > hi {
			w.errors = append(w.errors, newFinding(childField(field, pair[0]), fmt.Sprintf(
				"%s.%s (%s) is greater than %s (%s), so nothing could match",
				at, pair[0], jsNumber(lo), pair[1], jsNumber(hi))))
		}
	}
	if p, has := n["pattern"]; has {
		w.patterns = append(w.patterns, schemaPattern{at: childField(field, "pattern"), pattern: p})
	}
}

// storageCollectionSchemaChecks reports the imperative refusals of every
// declared collection schema (see the file comment). ERRORS.
func storageCollectionSchemaChecks(generic any) []Finding {
	var out []Finding
	for _, cs := range collectionSchemas(generic) {
		if cs.writeNone {
			// The server refuses the key here and parses nothing inside it.
			out = append(out, newFinding(cs.at, cs.at+` does not apply to write "none" — no document body `+
				"is ever written to that collection, so a schema would check nothing"))
			continue
		}
		if w := walkCollectionSchema(cs); w != nil {
			out = append(out, w.errors...)
		}
	}
	return out
}

// storageCollectionSchemaWarnings reports each `pattern` that Go's RE2 cannot
// compile. A WARNING, never an error: the server's language is ECMAScript with
// the `u` flag, and RE2 refusing a pattern says nothing certain about it (see
// the file comment). Only the patterns the server would go on to compile are
// considered — none in a schema it refuses for its size or on write "none".
func storageCollectionSchemaWarnings(generic any) []Finding {
	var out []Finding
	for _, cs := range collectionSchemas(generic) {
		if cs.writeNone {
			continue
		}
		w := walkCollectionSchema(cs)
		if w == nil {
			continue
		}
		for _, p := range w.patterns {
			src, ok := p.pattern.(string)
			if !ok || utf8.RuneCountInString(src) > appStorageSchemaPatternMaxLength {
				continue // the schema's finding
			}
			if _, err := regexp.Compile(src); err != nil {
				out = append(out, newFinding(p.at, fmt.Sprintf(
					"%s can't be checked offline: civitai.com compiles it as a JavaScript regular "+
						"expression with the \"u\" flag, and the Go (RE2) engine this CLI uses does not "+
						"accept it (%s). It may still be valid — the server decides at submit.",
					p.at, err)))
			}
		}
	}
	return out
}

// boundKey mirrors the server's boundKey: a key longer than 80 UTF-16 code
// units is cut to its first 80 and marked with an ellipsis (JS `slice`).
func boundKey(key string) string {
	units := utf16.Encode([]rune(key))
	if len(units) <= 80 {
		return key
	}
	return string(utf16.Decode(units[:80])) + "…"
}

// jsNumber renders a float64 as JavaScript's Number#toString does (the
// server interpolates the numbers into its message): shortest round-trip
// digits, plain notation from 1e-6 up to 1e21, exponent notation outside it.
// The same rules encoding/json uses for floats.
func jsNumber(f float64) string {
	if f == 0 {
		return "0" // and -0, which JS also prints "0"
	}
	format := byte('f')
	if abs := math.Abs(f); abs < 1e-6 || abs >= 1e21 {
		format = 'e'
	}
	s := strconv.FormatFloat(f, format, -1, 64)
	if format == 'e' {
		// Go writes "1e-07"; JS writes "1e-7".
		if n := len(s); n >= 4 && s[n-4] == 'e' && s[n-3] == '-' && s[n-2] == '0' {
			s = s[:n-2] + s[n-1:]
		}
	}
	return s
}

// jsonStringifyBytes is len(UTF-8(JSON.stringify(v))) for a value decoded by
// encoding/json — counted, not marshalled, because json.Marshal is not
// JSON.stringify: it escapes <, >, & and U+2028/U+2029 (6 bytes where
// JSON.stringify writes the character), and writes -0 as "-0".
func jsonStringifyBytes(v any) int {
	switch x := v.(type) {
	case nil:
		return 4
	case bool:
		if x {
			return 4
		}
		return 5
	case float64:
		return len(jsNumber(x))
	case string:
		return jsonStringBytes(x)
	case []any:
		n := 2
		for i, e := range x {
			if i > 0 {
				n++
			}
			n += jsonStringifyBytes(e)
		}
		return n
	case map[string]any:
		n := 2
		i := 0
		for k, e := range x {
			if i > 0 {
				n++
			}
			n += jsonStringBytes(k) + 1 + jsonStringifyBytes(e)
			i++
		}
		return n
	default:
		// Unreachable for a value encoding/json decoded into `any`.
		return 0
	}
}

// jsonStringBytes is the UTF-8 length of JSON.stringify(s): the quotes, a
// two-byte escape for " \ \b \f \n \r \t, a six-byte \u00XX for any other
// control character, and every other character as itself.
func jsonStringBytes(s string) int {
	n := 2
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '\b' || r == '\f' || r == '\n' || r == '\r' || r == '\t':
			n += 2
		case r < 0x20:
			n += 6
		default:
			n += utf8.RuneLen(r)
		}
	}
	return n
}

// sortedSchemaKeys returns m's keys in ascending order, so the walk (and the order of
// its findings before sortFindings) is deterministic.
func sortedSchemaKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
