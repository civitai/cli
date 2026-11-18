package validate

import (
	"strings"
	"testing"
)

// storage_collection_migrations_test.go pins the CLI's offline verdict on a
// storage collection's `migrations` (civitai/civitai#7661): the declarative ops
// — "default", "rename", "remove", "acknowledge" — civitai.com runs over a
// collection's stored documents when a version that narrows its `schema` is
// approved.
//
// AUTHORITY: the vendored schema (`storage.collections.<name>.migrations` and
// `$defs.storageMigrationOp` / `$defs.storageMigrationPath`). The CLI adds no
// semantic check of its own here: whether an op COVERS a breaking change needs
// the previous approved manifest, which only the server has, so that verdict is
// civitai.com's at submit. What the CLI can and does check offline is the SHAPE.
//
// 🔴 THE ACCEPT ARM IS THE REGRESSION TEST. Before the re-vendor, a collection's
// `additionalProperties: false` refused the `migrations` key for EVERY input, so
// `civitai app validate` rejected every manifest that used the feature — and a
// reject-only table would already have been green. Every rejection row below
// therefore has an accept twin that differs from it in the one rejected detail.

const migrationsAt = "storage.collections.cards.migrations"

// migrationsSchema is a collection schema a real migration would sit beside: it
// is the shape the civitai PR's own worked example declares.
const migrationsSchema = `{"type":"object","additionalProperties":false,"required":["status"],` +
	`"properties":{"title":{"type":"string","maxLength":80},"stars":{"type":"integer","maximum":5},` +
	`"status":{"enum":["draft","live"]},"tags":{"type":"array","maxItems":6,"items":{"type":"string"}}}}`

// migrationsManifest is a manifest that is clean apart from the one private
// collection `cards`, which declares migrationsSchema and `migrations` (raw JSON).
func migrationsManifest(migrations string) string {
	return docManifest(oneCollection("cards",
		`{"visibility":"private","schema":`+migrationsSchema+`,"migrations":`+migrations+`}`), "apps:storage")
}

// nOps is a JSON array of n distinct "remove" ops.
func nOps(n int) string {
	ops := make([]string, n)
	for i := range ops {
		ops[i] = `{"op":"remove","path":"/f` + itoaTest(i) + `"}`
	}
	return "[" + strings.Join(ops, ",") + "]"
}

// segments is a JSON Pointer of n segments.
func segments(n int) string {
	return strings.Repeat("/s", n)
}

// assertRefusedUnderMigrations requires at least one ERROR, and every error to
// sit at or under `at` — so a refusal for some unrelated reason (a broken
// fixture) is not counted as the schema refusing the op.
func assertRefusedUnderMigrations(t *testing.T, manifest, at string) {
	t.Helper()
	fs := manifestOnlyFindings(t, manifest)
	if len(fs) == 0 {
		t.Fatalf("want an error at or under %s, got a clean result", at)
	}
	for _, f := range fs {
		if f.Field != at && !strings.HasPrefix(f.Field, at+".") && !strings.HasPrefix(f.Field, at+"[") {
			t.Errorf("error outside %s — the fixture is broken, not the op refused:%s", at, describe(fs))
			return
		}
	}
}

// TestCollectionMigrationsAccepted is the ACCEPT arm, and the regression test:
// red on the pre-re-vendor schema, which refused the `migrations` key outright.
func TestCollectionMigrationsAccepted(t *testing.T) {
	for _, tc := range []struct{ name, migrations string }{
		{"the civitai PR's worked example, one op of each kind",
			`[{"op":"default","path":"/status","value":"draft"},` +
				`{"op":"rename","from":"/name","to":"/title"},` +
				`{"op":"remove","path":"/legacy"},` +
				`{"op":"acknowledge","path":"/stars","keyword":"maximum"}]`},
		{"a default whose value is an object", `[{"op":"default","path":"/meta","value":{"a":1}}]`},
		{"a default whose value is null", `[{"op":"default","path":"/meta","value":null}]`},
		{"acknowledge at the document root", `[{"op":"acknowledge","path":"","keyword":"additionalProperties"}]`},
		{"acknowledge inside items, with the * segment", `[{"op":"acknowledge","path":"/tags/*","keyword":"type"}]`},
		{"a path with ~0 and ~1 escapes", `[{"op":"remove","path":"/a~0b/c~1d"}]`},
		{"a 15-segment path", `[{"op":"remove","path":"` + segments(15) + `"}]`},
		{"a 15-segment acknowledge path", `[{"op":"acknowledge","path":"` + segments(15) + `","keyword":"enum"}]`},
		{"exactly 32 ops", nOps(32)},
		{"exactly 1 op", nOps(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertCleanResult(t, tc.name, migrationsManifest(tc.migrations))
		})
	}
	t.Run("every acknowledge keyword the schema lists", func(t *testing.T) {
		for _, kw := range []string{"type", "const", "enum", "minimum", "maximum", "minLength", "maxLength",
			"minItems", "maxItems", "pattern", "additionalProperties", "properties"} {
			assertCleanResult(t, kw, migrationsManifest(`[{"op":"acknowledge","path":"/stars","keyword":"`+kw+`"}]`))
		}
	})
}

// TestCollectionMigrationsRefused is the REJECT arm. Each row's accept twin is
// named beside it and lives in TestCollectionMigrationsAccepted.
func TestCollectionMigrationsRefused(t *testing.T) {
	for _, tc := range []struct{ name, migrations string }{
		// twin: one op of each kind
		{"an op that does not exist", `[{"op":"drop","path":"/legacy"}]`},
		{"an op with no op", `[{"path":"/legacy"}]`},
		{"a default with no value", `[{"op":"default","path":"/status"}]`},
		{"a rename with no to", `[{"op":"rename","from":"/name"}]`},
		{"a remove with an extra key", `[{"op":"remove","path":"/legacy","value":1}]`},
		{"an acknowledge with no keyword", `[{"op":"acknowledge","path":"/stars"}]`},
		// twin: every acknowledge keyword the schema lists
		{"acknowledge of a newly required field", `[{"op":"acknowledge","path":"/status","keyword":"required"}]`},
		{"acknowledge of a keyword outside the list", `[{"op":"acknowledge","path":"/stars","keyword":"format"}]`},
		// twin: acknowledge at the document root — "" is acknowledge-only
		{"a remove at the document root", `[{"op":"remove","path":""}]`},
		{"a default at the document root", `[{"op":"default","path":"","value":{}}]`},
		// twin: acknowledge inside items — * is acknowledge-only
		{"a remove with a * segment", `[{"op":"remove","path":"/tags/*"}]`},
		{"a rename to a * segment", `[{"op":"rename","from":"/name","to":"/*"}]`},
		// twin: a path with ~0 and ~1 escapes
		{"a path with a bare ~", `[{"op":"remove","path":"/a~b"}]`},
		{"a path with no leading /", `[{"op":"remove","path":"legacy"}]`},
		{"a path with an empty segment", `[{"op":"remove","path":"/a//b"}]`},
		// twin: a 15-segment path
		{"a 16-segment path", `[{"op":"remove","path":"` + segments(16) + `"}]`},
		{"a 16-segment acknowledge path", `[{"op":"acknowledge","path":"` + segments(16) + `","keyword":"enum"}]`},
		// twin: exactly 32 ops / exactly 1 op
		{"33 ops", nOps(33)},
		{"no ops", `[]`},
		{"not an array", `{"op":"remove","path":"/legacy"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusedUnderMigrations(t, migrationsManifest(tc.migrations), migrationsAt)
		})
	}
}

// TestCollectionMigrationsNotOnWriteNone — no document body is ever written to
// a write "none" collection, so a migration there would migrate nothing; the
// vendored schema refuses the key there. The twin is the same collection
// without it.
func TestCollectionMigrationsNotOnWriteNone(t *testing.T) {
	const plays = `{"visibility":"shared","write":"none","counters":["plays"]`
	assertCleanResult(t, "twin without migrations", docManifest(oneCollection("plays", plays+`}`), "apps:storage"))
	assertRefusedUnderMigrations(t,
		docManifest(oneCollection("plays", plays+`,"migrations":[{"op":"remove","path":"/x"}]}`), "apps:storage"),
		"storage.collections.plays.migrations")
}
