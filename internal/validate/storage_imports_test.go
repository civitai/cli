package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// --- the server's import rules (civitai block-storage.constants.ts:
// appStorageScopeErrors and parseImports), mirrored in storagemanifest.go.
// Every message is the server's text, pinned whole. ---

const importJustification = `"apps:storage:import": "reads and updates the entries you keep in the app this one was forked from"`

// importsManifest is a manifest whose storage is `storage` and whose scopes are
// `scopes`; apps:storage:import gets its justification when it is declared, so
// a sensitive-scope or unknown-justification finding never stands in for the
// one under test.
func importsManifest(storage string, scopes ...string) string {
	quoted := make([]string, 0, len(scopes))
	just := ""
	for _, s := range scopes {
		quoted = append(quoted, `"`+s+`"`)
		if s == "apps:storage:import" {
			just = `"scopeJustifications": {` + importJustification + `},`
		}
	}
	return `{"blockId": "imports-fixture", "name": "Imports Fixture", "version": "1.0.0", "contentRating": "g",
		"scopes": [` + strings.Join(quoted, ", ") + `], ` + just + `
		"storage": ` + storage + `,
		"iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}}`
}

const oneImport = `{"imports": [{"from": "parent-app", "collections": ["notes"], "access": "read"}]}`

// TestStorageImportsOnlyValidates — an app that only reaches another app's data
// declares NO collections of its own. The server requires apps:storage for it
// all the same (the storage route checks it on every call), so "apps:storage
// with nothing to reach" must wait for no imports either. Before the fix the
// CLI refused this manifest with "scopes includes apps:storage but storage
// declares no collections", which the server does not.
func TestStorageImportsOnlyValidates(t *testing.T) {
	assertNoErrors(t, "imports only, both scopes", importsManifest(oneImport, "apps:storage", "apps:storage:import"))
}

func TestStorageImportScopeRules(t *testing.T) {
	t.Run("imports only, without apps:storage", func(t *testing.T) {
		assertOnlyError(t, importsManifest(oneImport, "apps:storage:import"), "scopes",
			"storage imports from parent-app but scopes does not include apps:storage — add it, or remove storage")
	})
	t.Run("imports without apps:storage:import", func(t *testing.T) {
		assertOnlyError(t, importsManifest(oneImport, "apps:storage"), "scopes",
			"storage imports from parent-app, which requires the apps:storage:import scope — add it to "+
				"scopes with a scopeJustifications entry")
	})
	t.Run("apps:storage:import with no imports", func(t *testing.T) {
		assertOnlyError(t, importsManifest(`{"collections": {"notes": {"visibility": "private"}}}`,
			"apps:storage", "apps:storage:import"), "storage",
			"scopes includes apps:storage:import but storage declares no imports — declare them under "+
				"storage.imports, or remove the scope")
	})
	t.Run("collections AND imports, both scopes (accept twin)", func(t *testing.T) {
		assertNoErrors(t, "collections and imports", importsManifest(
			`{"collections": {"notes": {"visibility": "private"}}, "imports": [{"from": "parent-app", "collections": ["notes"], "access": "read"}]}`,
			"apps:storage", "apps:storage:import"))
	})
}

func TestStorageImportParseRefusals(t *testing.T) {
	t.Run("an import from this app itself", func(t *testing.T) {
		assertOnlyError(t, importsManifest(
			`{"imports": [{"from": "imports-fixture", "collections": ["notes"], "access": "read"}]}`,
			"apps:storage", "apps:storage:import"), "storage.imports[0].from",
			"storage.imports[0].from names this app itself — declare its collections instead")
	})
	t.Run("a second import from one source", func(t *testing.T) {
		assertOnlyError(t, importsManifest(
			`{"imports": [{"from": "parent-app", "collections": ["notes"], "access": "read"},`+
				`{"from": "other-app", "collections": ["notes"], "access": "read"},`+
				`{"from": "parent-app", "collections": ["drafts"], "access": "readwrite"}]}`,
			"apps:storage", "apps:storage:import"), "storage.imports[2]",
			`storage.imports[2] imports from "parent-app" again — declare one import per app`)
	})
	// The server compares against the imports it KEPT, and it keeps no entry
	// that drew an error: the first entry here is refused for its access, so
	// the second one is not a duplicate of it. Only the schema's finding remains.
	t.Run("no duplicate of an entry that was itself refused", func(t *testing.T) {
		assertOnlyError(t, importsManifest(
			`{"imports": [{"from": "parent-app", "collections": ["notes"], "access": "write"},`+
				`{"from": "parent-app", "collections": ["drafts"], "access": "read"}]}`,
			"apps:storage", "apps:storage:import"), "storage.imports[0].access",
			"storage.imports[0].access: value must be one of 'read', 'readwrite'")
	})
}

// TestStorageImportScopeIsSensitive — apps:storage:import is in the server's
// SENSITIVE_BLOCK_SCOPES (civitai packages/civitai-shared/src/sensitive-block-scopes.ts),
// so without a justification the manifest is refused at submit.
func TestStorageImportScopeIsSensitive(t *testing.T) {
	m := `{"blockId": "imports-fixture", "name": "Imports Fixture", "version": "1.0.0", "contentRating": "g",
		"scopes": ["apps:storage", "apps:storage:import"],
		"storage": ` + oneImport + `,
		"iframe": {"sandbox": "allow-scripts", "minHeight": 400, "resizable": false}}`
	assertOnlyError(t, m, "scopeJustifications",
		"sensitive scopes require a justification — add a non-empty scopeJustifications entry for: apps:storage:import")
}

// --- the SOURCE advisory for a `from` call (storagescope.go rule 1b) ---

// forkSourceManifest declares one own collection (`kv`) and one import (of
// `notes` from parent-app), so an own-collection reading and an import reading
// of the same call give DIFFERENT answers.
func forkSourceManifest() string {
	return importsManifest(
		`{"collections": {"kv": {"visibility": "private"}}, "imports": [{"from": "parent-app", "collections": ["notes"], "access": "readwrite"}]}`,
		"apps:storage", "apps:storage:import")
}

func TestStorageImportedCollectionAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name, src, field, message string
	}{
		{"listed by the import: silent",
			`export const n = () => app.storage.collection('notes', { from: 'parent-app' }).list();`, "", ""},
		{"double quotes, more options after from: silent",
			`export const n = () => app.storage.collection("notes", { from: "parent-app", limit: 5 });`, "", ""},
		{"not listed by the import",
			`export const d = () => app.storage.collection('drafts_q7', { from: 'parent-app' }).list();`,
			"storage.imports[0].collections",
			"src/fork.ts calls `storage.collection(\"drafts_q7\", { from: \"parent-app\" })` but the import from " +
				"\"parent-app\" does not list \"drafts_q7\" — a collection an import does not list is not reachable, " +
				"so every call on it is refused with `import_not_declared`. Add it to that import's collections, or " +
				"correct the name. ADVISORY: this reads source statically, so only string-literal names are checked."},
		{"no import from that source",
			`export const s = () => app.storage.collection('notes', { from: 'stranger-app' }).list();`,
			"storage.imports",
			"src/fork.ts calls `storage.collection(\"notes\", { from: \"stranger-app\" })` but storage.imports " +
				"declares no import from \"stranger-app\" — an app reaches another app's collection only through an " +
				"import it declares, so every call on it is refused with `import_not_declared`. Declare the import " +
				"under storage.imports, or correct the source. ADVISORY: this reads source statically, so only " +
				"string-literal names are checked."},
		// The own-collection reading is unchanged for a call with NO from: the
		// same name is an own collection here, and kv (declared) stays silent.
		{"no from: still an own collection",
			"export const n = () => app.storage.collection('notes').list();\n" +
				"export const k = () => app.storage.collection('kv').list();",
			"storage.collections.notes", undeclaredMessage("src/fork.ts", "notes")},
		{"computed from: unknowable, silent",
			`export const n = () => app.storage.collection('drafts_q7', { from: PARENT }).list();`, "", ""},
		{"shorthand from: unknowable, silent",
			`export const n = (from: string) => app.storage.collection('drafts_q7', { from }).list();`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storageSourceWarnings(t, forkSourceManifest(), map[string]string{"src/fork.ts": tc.src})
			if tc.message == "" {
				if len(got) != 0 {
					t.Fatalf("want no storage advisory, got %d:%s", len(got), describe(got))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want exactly one storage advisory, got %d:%s", len(got), describe(got))
			}
			if got[0].Message != tc.message {
				t.Errorf("message\n  got:  %s\n  want: %s", got[0].Message, tc.message)
			}
			if got[0].Field != tc.field {
				t.Errorf("field = %q, want %q", got[0].Field, tc.field)
			}
		})
	}
}
