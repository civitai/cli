package validate

import (
	"strings"
	"testing"
)

// storage_removed_primitives_test.go pins storageRemovedPrimitiveChecks: counters,
// marks and `write: "none"` were removed from App Storage (civitai #10391), and
// civitai.com refuses them at submit (deprecatedPrimitiveErrors in
// storage-schema-migrations.ts) unless the collection's previous approved
// version already declared them. The CLI cannot see the approved manifest, so it
// reports every declaration as an ERROR with the server's first-version message,
// plus a sentence naming the grandfathered case. Messages are written out
// literally, never built from the constants under test.

const removedPrimitiveMarker = "were removed and cannot be declared on a new app"

// withoutRemovedPrimitives drops the removed-primitive errors, for tests of an
// older rule whose fixture still declares counters or marks.
func withoutRemovedPrimitives(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if strings.Contains(f.Message, removedPrimitiveMarker) ||
			strings.Contains(f.Message, `"none" was removed and cannot be declared on a new app`) {
			continue
		}
		out = append(out, f)
	}
	return out
}

const removedPrimitiveTail = ` (civitai #10391). civitai.com accepts one only on a collection whose ` +
	`previous approved version already declares it, where it does nothing; the CLI cannot see the ` +
	`approved manifest, so it reports every declaration; declare a viewer-keyed collection instead ` +
	`(keyedBy "viewer" or "parent+viewer": each viewer's own document is their vote or entry), and ` +
	`report a document through the moderation report API — see https://developer.civitai.com/apps/guide/storage`

func TestStorageRemovedPrimitivesAreErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		decl   string
		scopes []string
		want   []Finding
	}{
		{
			name:   "counters and marks on a write-own collection",
			decl:   `{"visibility":"shared","write":"own","counters":["plays","likes"],"marks":["starred"]}`,
			scopes: []string{"apps:storage", "apps:storage:publish"},
			want: []Finding{
				{Field: "storage.collections.votes.marks", Message: `storage.collections.votes.marks: marks ` +
					`were removed and cannot be declared on a new app (declared: "starred")` + removedPrimitiveTail},
				{Field: "storage.collections.votes.counters", Message: `storage.collections.votes.counters: ` +
					`counters were removed and cannot be declared on a new app (declared: "plays", "likes")` +
					removedPrimitiveTail},
			},
		},
		{
			name:   "write none with a counter",
			decl:   `{"visibility":"shared","write":"none","counters":["anchor"]}`,
			scopes: []string{"apps:storage"},
			want: []Finding{
				{Field: "storage.collections.votes.counters", Message: `storage.collections.votes.counters: ` +
					`counters were removed and cannot be declared on a new app (declared: "anchor")` +
					removedPrimitiveTail},
				{Field: "storage.collections.votes.write", Message: `storage.collections.votes.write: "none" ` +
					`was removed and cannot be declared on a new app` + removedPrimitiveTail},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := manifestOnlyFindings(t, docManifest(oneCollection("votes", tc.decl), tc.scopes...))
			if len(got) != len(tc.want) {
				t.Fatalf("want %d errors, got %d:%s", len(tc.want), len(got), describe(got))
			}
			// Order-insensitive: the result's order is not this check's contract.
			for _, w := range tc.want {
				hit := false
				for _, g := range got {
					hit = hit || (g.Field == w.Field && g.Message == w.Message)
				}
				if !hit {
					t.Errorf("missing:\n  [%s] %s\n got:%s", w.Field, w.Message, describe(got))
				}
			}
		})
	}
}

// TestStorageGenericReplacementIsClean is the twin: the pattern the message
// points at (a viewer-keyed shared collection) draws no error at all, so the
// errors above are the removed primitives' and nothing else's.
func TestStorageGenericReplacementIsClean(t *testing.T) {
	assertNoErrors(t, "viewer-keyed collection", docManifest(oneCollection("votes",
		`{"visibility":"shared","write":"own","keyedBy":"parent+viewer"}`), "apps:storage", "apps:storage:publish"))
}

// TestStorageRemovedPrimitivesDoNotHoldBackScopeRules — the server refuses them
// in its submit-time migration check, not its parser, so the storage⇄scopes
// rules still judge the declaration. A write-own collection with a counter and
// no apps:storage:publish gets BOTH stories.
func TestStorageRemovedPrimitivesDoNotHoldBackScopeRules(t *testing.T) {
	fs := manifestOnlyFindings(t, docManifest(oneCollection("votes",
		`{"visibility":"shared","write":"own","counters":["plays"]}`), "apps:storage"))
	var scope, removed int
	for _, f := range fs {
		switch {
		case strings.Contains(f.Message, removedPrimitiveMarker):
			removed++
		case f.Field == "scopes" && strings.Contains(f.Message, "requires the apps:storage:publish scope"):
			scope++
		}
	}
	if removed != 1 || scope != 1 || len(fs) != 2 {
		t.Errorf("want 1 removed-primitive and 1 scope error, got %d and %d of %d:%s", removed, scope, len(fs), describe(fs))
	}
}
