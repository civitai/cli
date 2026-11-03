package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// storage_imports_test.go pins `civitai app validate` against CROSS-APP STORAGE
// IMPORTS (civitai web v5.1.425): the manifest's `storage.imports` array, the
// collection-level `exportable` flag and the `apps:storage:import` scope.
//
// THE MOTIVATING FAILURE (2026-11-03). The vendored schema predated the
// feature, so a real fork manifest — the seed the platform writes when a viewer
// forks an app — failed offline validation with two findings the server does
// not raise:
//
//	scopes[6]: value must be one of …            (apps:storage:import unknown)
//	storage: additional properties 'imports' not allowed
//
// so an author could only submit with --skip-validate.
//
// The fixture is that real fork seed manifest, MINUS its `iframe.src` key:
// iframe.src is server-owned, the CLI refusing it is intended
// (TestValidateRejectsDevSetIframeSrc), and the seed carrying it is a separate
// platform defect. Every expected message below is written out literally, not
// read from the schema, so a schema that stops describing imports goes red here.

// forkManifest returns the fork fixture decoded, for a test to mutate.
func forkManifest(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "fork-manifest.json"))
	if err != nil {
		t.Fatalf("read fork fixture: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode fork fixture: %v", err)
	}
	// Positive control on the fixture itself: if it stopped carrying an import
	// or the scope, the clean-validate assertion below would be vacuous.
	imports, _ := m["storage"].(map[string]any)["imports"].([]any)
	if len(imports) != 1 {
		t.Fatalf("fork fixture must carry exactly one storage.imports entry, has %d", len(imports))
	}
	if _, ok := stringSet(m["scopes"])["apps:storage:import"]; !ok {
		t.Fatal("fork fixture must declare apps:storage:import")
	}
	if iframe, _ := m["iframe"].(map[string]any); iframe["src"] != nil {
		t.Fatal("fork fixture must not carry the server-owned iframe.src")
	}
	return m
}

// firstImport returns storage.imports[0] of m, for in-place mutation.
func firstImport(m map[string]any) map[string]any {
	return m["storage"].(map[string]any)["imports"].([]any)[0].(map[string]any)
}

func encodeManifest(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	return string(b)
}

// TestForkManifestWithStorageImportsValidates is assertion (a): the real fork
// seed manifest validates with no errors at all.
func TestForkManifestWithStorageImportsValidates(t *testing.T) {
	assertNoErrors(t, "fork seed manifest", encodeManifest(t, forkManifest(t)))
}

// TestStorageImportRejectsUnknownAccess is assertion (b), first half: `access`
// is the closed set read|readwrite, so "write" — the plausible wrong spelling —
// is refused at exactly that element, with the library's enum message.
func TestStorageImportRejectsUnknownAccess(t *testing.T) {
	m := forkManifest(t)
	firstImport(m)["access"] = "write"
	assertOnlyError(t, encodeManifest(t, m), "storage.imports[0].access",
		"storage.imports[0].access: value must be one of 'read', 'readwrite'")
}

// TestStorageImportRejectsMissingFrom is assertion (b), second half: an import
// that does not name its source app is refused, located at the import object
// (the object is what is incomplete, not the absent key).
func TestStorageImportRejectsMissingFrom(t *testing.T) {
	m := forkManifest(t)
	delete(firstImport(m), "from")
	assertOnlyError(t, encodeManifest(t, m), "storage.imports[0]",
		"storage.imports[0]: missing property 'from'")
}

// TestStorageImportScopeValidates is assertion (c): apps:storage:import is a
// known scope. The manifest is otherwise minimal but COHERENT — a collection of
// its own for apps:storage, an import for apps:storage:import, and a
// justification for the import scope — so a clean result is not a CLI gap
// standing in for "valid".
func TestStorageImportScopeValidates(t *testing.T) {
	assertNoErrors(t, "apps:storage:import scope", `{
		"blockId": "imports-fixture", "name": "Imports Fixture", "version": "1.0.0", "contentRating": "g",
		"scopes": ["apps:storage", "apps:storage:import"],
		"scopeJustifications": {"apps:storage:import": "reads and updates the entries you keep in the app this one was forked from"},
		"storage": {
			"collections": {"notes": {"visibility": "private", "exportable": false}},
			"imports": [{"from": "parent-app", "collections": ["notes"], "access": "read"}]
		},
		"iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}
	}`)
}
