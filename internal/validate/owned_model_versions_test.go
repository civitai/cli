package validate

import (
	"encoding/json"
	"strings"
	"testing"
)

// owned_model_versions_test.go pins the CLI's offline verdict on a storage
// collection's `ownedModelVersions` (civitai/civitai#6132): 1 to 4 distinct
// JSON-pointer-ish paths into a document's `data`, legal ONLY on a shared
// collection whose `write` is "own".
//
// THERE IS NO GO MIRROR OF THIS RULE, deliberately. The canonical schema
// expresses every constraint the server parser enforces on the declaration
// (`parseCollection` in civitai `src/shared/constants/block-storage.constants.ts`):
// the shared-only `false` in the private branch, the `if write ∈ {anyone, owner,
// none} then false` branch, the item pattern, minItems/maxItems/uniqueItems.
// This package mirrors no other storage-collection rule imperatively either, so
// the vendored schema is the whole check and these tests drive it through the
// REAL validator (ManifestOnly), never through a regexp compiled here.
//
// 🔴 EVERY REJECTION ROW CARRIES AN ACCEPT TWIN. A manifest that is rejected
// proves nothing on its own: before this schema change the key was refused by
// `additionalProperties:false` for EVERY input, so a reject-only table was
// already green. Each row therefore asserts (a) the same collection WITHOUT the
// defect validates clean, and (b) the defect produces a finding located at the
// `ownedModelVersions` field itself — not one level up at the collection, which
// is where the old unknown-key refusal landed.

// ownedVersionsManifest is a manifest that is clean apart from the one storage
// collection `c`, whose declaration is spliced in raw. The scopes are the ones
// the server requires for a viewer-written shared collection (`apps:storage`,
// plus the sensitive `apps:storage:publish` with its justification), so a clean
// collection really does produce zero findings.
func ownedVersionsManifest(collection string) string {
	return `{"blockId":"ok-app","name":"x","version":"1.0.0","contentRating":"pg",` +
		`"scopes":["apps:storage","apps:storage:publish"],` +
		`"scopeJustifications":{"apps:storage:publish":"viewers publish their own workflows"},` +
		`"kind":"page","iframe":{"sandbox":"allow-scripts","minHeight":100,"resizable":false},` +
		`"storage":{"collections":{"c":` + collection + `}}}`
}

// sharedOwnWith returns a shared/own collection declaring the given pointers.
func sharedOwnWith(t *testing.T, pointers ...string) string {
	t.Helper()
	b, err := json.Marshal(pointers)
	if err != nil {
		t.Fatal(err)
	}
	return `{"visibility":"shared","write":"own","ownedModelVersions":` + string(b) + `}`
}

func assertClean(t *testing.T, label, manifest string) {
	t.Helper()
	for _, f := range manifestOnlyFindings(t, manifest) {
		t.Errorf("%s: unexpected finding on %q: %s", label, f.Field, f.Message)
	}
}

const ownedField = "storage.collections.c.ownedModelVersions"

// TestOwnedModelVersionsAccepted is the ACCEPT arm: the manifest shape an app
// author actually writes, plus the pattern's boundaries (4 segments, a 64-char
// segment, a leading underscore, 4 pointers). Red before the re-vendor — the
// key was an unknown property then.
func TestOwnedModelVersionsAccepted(t *testing.T) {
	seg64 := "/a" + strings.Repeat("b", 63)
	for _, tc := range []struct {
		name     string
		pointers []string
	}{
		{"documented example", []string{"/workflow/modelVersionId"}},
		{"one segment", []string{"/modelVersionId"}},
		{"four segments (max)", []string{"/a/b/c/d"}},
		{"64-char segment (max)", []string{seg64}},
		{"leading underscore, upper case", []string{"/_Id/X9"}},
		{"four distinct pointers (max)", []string{"/a", "/b", "/c/d", "/e/f/g/h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertClean(t, tc.name, ownedVersionsManifest(sharedOwnWith(t, tc.pointers...)))
		})
	}

	// NEGATIVE CONTROL on the fixture's scopes: the collection this feature
	// lives on needs the SENSITIVE `apps:storage:publish`, so dropping its
	// justification must produce the server's scopeJustifications refusal
	// locally. Without the mirror entry this manifest validated clean offline
	// and was refused at submit.
	unjustified := strings.Replace(ownedVersionsManifest(sharedOwnWith(t, "/workflow/modelVersionId")),
		`"scopeJustifications":{"apps:storage:publish":"viewers publish their own workflows"},`, "", 1)
	var hit bool
	for _, f := range manifestOnlyFindings(t, unjustified) {
		if f.Field == "scopeJustifications" && strings.Contains(f.Message, "apps:storage:publish") {
			hit = true
		}
	}
	if !hit {
		t.Fatal("removing the justification produced no scopeJustifications finding naming apps:storage:publish")
	}
}

// TestOwnedModelVersionsRejectedOutsideSharedOwn — the key is refused on a
// private collection and on every shared write policy except "own". The server
// reason: the check compares the version's owner against the viewer WRITING the
// document, which is that document's author only under "own".
func TestOwnedModelVersionsRejectedOutsideSharedOwn(t *testing.T) {
	for _, tc := range []struct {
		name string
		// collection is the declaration WITHOUT ownedModelVersions; it must be
		// valid on its own so the finding is attributable to the key.
		collection string
	}{
		{"private", `{"visibility":"private"}`},
		{"shared write:anyone", `{"visibility":"shared","write":"anyone"}`},
		{"shared write:owner", `{"visibility":"shared","write":"owner"}`},
		{"shared write:none", `{"visibility":"shared","write":"none","counters":["plays"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// (a) accept twin: the collection is fine without the key.
			assertClean(t, "twin without ownedModelVersions", ownedVersionsManifest(tc.collection))

			// (b) the key, with a pointer that is valid on shared/own.
			with := strings.TrimSuffix(tc.collection, "}") + `,"ownedModelVersions":["/workflow/modelVersionId"]}`
			fs := manifestOnlyFindings(t, ownedVersionsManifest(with))
			if len(fs) != 1 || fs[0].Field != ownedField {
				t.Fatalf("want exactly one finding located at %q, got %d: %v", ownedField, len(fs), fs)
			}
		})
	}
	// CONTROL: the same valid pointer on shared/own — the one policy where it
	// is legal — must be clean, or the rows above are rejecting the POINTER
	// rather than the policy.
	assertClean(t, "shared/own control", ownedVersionsManifest(sharedOwnWith(t, "/workflow/modelVersionId")))
}

// TestOwnedModelVersionsPointerShape drives the item pattern
// `^(/[A-Za-z_][A-Za-z0-9_]{0,63}){1,4}$` through the real validator.
//
// The CLI's schema library compiles patterns with Go's RE2; the server's is
// ECMAScript. This regex uses no construct where they differ (ASCII classes,
// bounded repeats, no lookaround, no flags), and the expected verdicts below
// were cross-checked against node's RegExp on the same strings — including the
// `$`-before-trailing-newline case, which RE2 and ECMAScript (no `m` flag) both
// refuse. The accept half lives in TestOwnedModelVersionsAccepted, which is the
// twin for every row here.
func TestOwnedModelVersionsPointerShape(t *testing.T) {
	for _, bad := range []string{
		"/0",                           // segment starts with a digit (an array index)
		"/a~0b",                        // RFC 6901 escape — not supported
		"/a-b",                         // hyphen
		"/a/b/c/d/e",                   // 5 segments
		"/a/0",                         // array index in a later segment
		"",                             // empty
		"/",                            // empty segment
		"a",                            // no leading slash
		"/a/",                          // trailing empty segment
		"//a",                          // empty first segment
		"/a.b",                         // dot
		"/a b",                         // space
		"/é",                           // non-ASCII letter
		"/a\n",                         // trailing newline must not satisfy `$`
		"/a" + strings.Repeat("b", 64), // 65-char segment
	} {
		t.Run(strings.ReplaceAll(bad, "\n", `\n`), func(t *testing.T) {
			fs := manifestOnlyFindings(t, ownedVersionsManifest(sharedOwnWith(t, bad)))
			f := patternFindingFor(t, fs, ownedField+"[0]")
			if !strings.Contains(f.Message, `^(/[A-Za-z_][A-Za-z0-9_]{0,63}){1,4}$`) {
				t.Errorf("finding does not name the ownedModelVersions pattern: %s", f.Message)
			}
			if len(fs) != 1 {
				t.Errorf("want only the pattern finding, got %d: %v", len(fs), fs)
			}
		})
	}
}

// TestOwnedModelVersionsArrayBounds — 1 to 4 DISTINCT strings.
func TestOwnedModelVersionsArrayBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
	}{
		{"empty", `[]`},
		{"five pointers", `["/a","/b","/c","/d","/e"]`},
		{"duplicate", `["/a","/a"]`},
		{"not an array", `"/a"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := manifestOnlyFindings(t, ownedVersionsManifest(
				`{"visibility":"shared","write":"own","ownedModelVersions":`+tc.value+`}`))
			if len(fs) != 1 || fs[0].Field != ownedField {
				t.Fatalf("want exactly one finding located at %q, got %d: %v", ownedField, len(fs), fs)
			}
		})
	}
	// Non-string item: located on the item.
	fs := manifestOnlyFindings(t, ownedVersionsManifest(
		`{"visibility":"shared","write":"own","ownedModelVersions":[7]}`))
	if len(fs) != 1 || fs[0].Field != ownedField+"[0]" {
		t.Fatalf("non-string item: want one finding at %q, got %d: %v", ownedField+"[0]", len(fs), fs)
	}
}
