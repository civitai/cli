package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// storagescope_test.go pins the LEGACY-HOOK scope advisory and the tree walk every
// source-reading storage check shares. The document-store findings themselves
// (undeclared collection names, document store without `apps:storage`) are
// pinned in docstore_test.go.

func storageProject(t *testing.T, manifestJSON string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"block.manifest.json": manifestJSON}
	for k, v := range files {
		all[k] = v
	}
	for name, body := range all {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const usesApp = `import { useAppStorage } from '@civitai/blocks-react';
export function Save() { const s = useAppStorage(); return s; }`

const usesShared = `import { useSharedStorage } from '@civitai/blocks-react';
export function Board() { const s = useSharedStorage<Entry>(); return s; }`

const usesSharedNested = `import { useSharedStorage } from '@civitai/blocks-react';
export function Board() { const s = useSharedStorage<Entry<number>>(); return s; }`

// legacyWarnings returns the legacy-hook advisories, keyed on the hook each
// message names, with their fields.
func legacyWarnings(t *testing.T, dir string) (app, shared bool, fields []string) {
	t.Helper()
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("fixture manifest must be VALID or the test measures nothing:%s", describe(res.Errors))
	}
	for _, w := range res.Warnings {
		switch {
		case strings.Contains(w.Message, "calls `useAppStorage()`"):
			app = true
			fields = append(fields, w.Field)
		case strings.Contains(w.Message, "calls `useSharedStorage()`"):
			shared = true
			fields = append(fields, w.Field)
		case strings.Contains(w.Message, "useAppStorage") || strings.Contains(w.Message, "useSharedStorage"):
			t.Errorf("a warning names a legacy hook but is not the legacy-scope advisory: %s", w.Message)
		}
	}
	return app, shared, fields
}

// privateKV is a valid declaration for the rows that need `apps:storage` (which
// is refused without a collection to reach).
var privateKV = oneCollection("kv", `{"visibility":"private"}`)

// TestLegacyStorageHookNeedsItsScope — a CALL to `useAppStorage()` while the
// manifest declares neither `apps:storage:read` nor `apps:storage:write`, or to
// `useSharedStorage()` with neither shared scope, is the 2026-10-01 failure:
// every save 403s in production. That is the whole claim. A manifest that
// declares ONE of the store's scopes is someone who knows scopes exist and
// chose, so it is silent; there is no unconditional "deprecated" warning.
//
// 🔴 THE SCOPES ARE AN EXPLICIT SET, NOT A PREFIX. The pre-#856 check took any
// `apps:storage:` scope outside the shared namespace as satisfying the
// per-viewer store. `apps:storage:publish` (the document store's) has that
// prefix and grants the legacy store nothing, so the prefix would now read a
// document-store manifest as having declared the legacy one.
//
// Both directions: a check that warned unconditionally would pass every "want"
// row, so the SILENT rows carry the weight.
func TestLegacyStorageHookNeedsItsScope(t *testing.T) {
	tests := []struct {
		name       string
		storage    string
		scopes     []string
		files      map[string]string
		wantApp    bool
		wantShared bool
		why        string
	}{
		{
			name:    "calls useAppStorage, no storage scope",
			scopes:  []string{"user:read:self"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
		},
		{
			name:   "calls useAppStorage with apps:storage:read — silent",
			scopes: []string{"apps:storage:read"},
			files:  map[string]string{"src/App.tsx": usesApp},
			why:    "one of the store's scopes is a choice this check does not second-guess",
		},
		{
			name:   "calls useAppStorage with apps:storage:write — silent",
			scopes: []string{"apps:storage:write"},
			files:  map[string]string{"src/App.tsx": usesApp},
		},
		{
			name:    "calls useAppStorage with only the SHARED scopes",
			scopes:  []string{"apps:storage:shared:read", "apps:storage:shared:write"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
			why:     "a shared scope does not satisfy the per-viewer store",
		},
		{
			name:    "calls useAppStorage with only the DOCUMENT-STORE scopes",
			storage: oneCollection("posts", `{"visibility":"shared","write":"own"}`),
			scopes:  []string{"apps:storage", "apps:storage:publish"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
			why:     "apps:storage / apps:storage:publish grant the legacy store nothing",
		},
		{
			name:       "calls useSharedStorage with a type argument",
			scopes:     []string{"user:read:self"},
			files:      map[string]string{"src/Board.tsx": usesShared},
			wantShared: true,
		},
		{
			name:       "calls useSharedStorage with a NESTED type argument",
			scopes:     []string{"user:read:self"},
			files:      map[string]string{"src/Board.tsx": usesSharedNested},
			wantShared: true,
		},
		{
			name:   "calls useSharedStorage with apps:storage:shared:read — silent",
			scopes: []string{"apps:storage:shared:read"},
			files:  map[string]string{"src/Board.tsx": usesShared},
		},
		{
			name:       "calls useSharedStorage with only the PER-VIEWER scopes",
			scopes:     []string{"apps:storage:read", "apps:storage:write"},
			files:      map[string]string{"src/Board.tsx": usesShared},
			wantShared: true,
			why:        "the two stores are asked independently",
		},
		{
			name:       "both hooks, no storage scope — two findings",
			scopes:     []string{"user:read:self"},
			files:      map[string]string{"src/App.tsx": usesApp, "src/Board.tsx": usesShared},
			wantApp:    true,
			wantShared: true,
		},
		{
			name:    "the document store declared does not silence a legacy call",
			storage: privateKV,
			scopes:  []string{"apps:storage"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
		},
		{
			name:   "IMPORTED but never called — silent",
			scopes: []string{"user:read:self"},
			files: map[string]string{
				"src/App.tsx": "import { useAppStorage } from '@civitai/blocks-react';\nexport const x = 1;",
			},
			why: "the hook-NAME matcher this replaced fired here; a call is the claim",
		},
		{
			// The playable-collections false positive, in a NON-test file: an
			// object whose string values name hooks is data, not a call.
			name:   "hook names as string VALUES — silent",
			scopes: []string{"user:read:self"},
			files: map[string]string{
				"src/scope-ledger.ts": "export const LEDGER = { app: 'useAppStorage', shared: 'useSharedStorage' };",
			},
			why: "a string naming a hook is not a call to it",
		},
		{
			name:   "no storage use at all — silent",
			scopes: []string{"user:read:self"},
			files:  map[string]string{"src/App.tsx": `export const x = 1;`},
		},
		{
			name:   "the hook named only in a COMMENT — silent",
			scopes: []string{"user:read:self"},
			files: map[string]string{
				"src/App.tsx": "// later: persist with useAppStorage()\n/* or useSharedStorage() */\nexport const x = 1;",
			},
			why: "a comment is not a call",
		},
		{
			name:   "the hook named only in a NON-SOURCE file — silent",
			scopes: []string{"user:read:self"},
			files: map[string]string{
				"README.md":   "we will use useAppStorage() eventually",
				"notes.txt":   "useSharedStorage()",
				"src/App.tsx": `export const x = 1;`,
			},
			why: "the extension filter: prose and data are not code",
		},
		{
			name:   "the hook only inside node_modules — silent",
			scopes: []string{"user:read:self"},
			files: map[string]string{
				"node_modules/@civitai/blocks-react/dist/index.js": usesApp,
				"src/App.tsx": `export const x = 1;`,
			},
			why: "skip-dirs: a dependency's own source is not the app's",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := storageProject(t, docManifest(tc.storage, tc.scopes...), tc.files)
			app, shared, fields := legacyWarnings(t, dir)
			if app != tc.wantApp {
				t.Errorf("useAppStorage advisory = %v, want %v (%s)", app, tc.wantApp, tc.why)
			}
			if shared != tc.wantShared {
				t.Errorf("useSharedStorage advisory = %v, want %v (%s)", shared, tc.wantShared, tc.why)
			}
			// The source is CORRECT (using storage is the point) and the
			// manifest is what lacks the scope, so the finding names `scopes`.
			for _, f := range fields {
				if f != "scopes" {
					t.Errorf("Field = %q, want %q", f, "scopes")
				}
			}
		})
	}
}

// TestLegacyStorageHookMessage pins the whole advisory text, for each store.
func TestLegacyStorageHookMessage(t *testing.T) {
	for _, tc := range []struct {
		file, src, want string
	}{
		{"src/App.tsx", usesApp, legacyMessage("useAppStorage", "apps:storage:read", "apps:storage:write")},
		{"src/Board.tsx", usesShared, legacyMessage("useSharedStorage", "apps:storage:shared:read", "apps:storage:shared:write")},
	} {
		res, err := Dir(storageProject(t, docManifest("", "user:read:self"), map[string]string{tc.file: tc.src}))
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		var hit bool
		for _, w := range res.Warnings {
			hit = hit || w.Message == tc.want
		}
		if !hit {
			t.Errorf("no warning with the legacy-scope text\n  want: %s\n  got:%s", tc.want, describe(res.Warnings))
		}
	}
}

// TestStorageScanFollowsSymlinkedSource pins the hazard readyack recorded after
// reproducing it live: filepath.WalkDir does NOT follow symlinked directories and
// does not report them as directories either, so a monorepo whose `src` is a
// symlink into a shared package had its ENTIRE source tree skipped — the silent
// direction, the defect ships.
func TestStorageScanFollowsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "packages", "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "App.tsx"), []byte(usesApp), 0o600); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(root, "app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "block.manifest.json"), []byte(docManifest("", "user:read:self")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(proj, "src")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	app, _, _ := legacyWarnings(t, proj)
	if !app {
		t.Error("storage use behind a symlinked src/ was not seen — the walk stopped following symlinks")
	}
}

// TestStorageScanUnobservableStaysSilent — reading only PART of a tree is not
// finding nothing. A scan that hits a cap must not report against a project it
// did not finish reading.
func TestStorageScanUnobservableStaysSilent(t *testing.T) {
	big := strings.Repeat("x", maxStorageFileBytes+1)
	// 🔴 THE ORDERING IS THE TEST. os.ReadDir returns entries SORTED, so the
	// call sites sort BEFORE the oversized file: the scan records them and only
	// afterwards goes partial, leaving the partial guard as the single thing that
	// can suppress the findings. Reversed, this passes with the guard deleted.
	dir := storageProject(t, docManifest("", "user:read:self"), map[string]string{
		"src/aaa-App.tsx":   usesApp + "\nexport const q = () => app.storage.collection('partial_scan_q');\n",
		"src/zzz-bundle.js": big,
	})
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	for _, w := range res.Warnings {
		t.Errorf("warned from a scan that hit its byte cap — a partial read must stay silent: %s", w.Message)
	}
	// CONTROL: without the oversized file the same source IS reported.
	res, err = Dir(storageProject(t, docManifest("", "user:read:self"), map[string]string{
		"src/aaa-App.tsx": usesApp + "\nexport const q = () => app.storage.collection('partial_scan_q');\n",
	}))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("control: the same source without the oversized file produced no warning")
	}
}

// TestStorageScanToleratesABadScopesField — a malformed `scopes` produces no
// scope advisory from the source scan: the schema layer reports that defect,
// and a second story about the same field is worse than none.
func TestStorageScanToleratesABadScopesField(t *testing.T) {
	m := `{"blockId":"x","name":"X","version":"0.1.0","contentRating":"g","scopes":"not-an-array",` +
		`"iframe":{"sandbox":"allow-scripts","minHeight":400,"resizable":false}}`
	res, err := Dir(storageProject(t, m, map[string]string{
		"src/App.ts": "export const q = (n: string) => app.storage.collection(n).list();\n",
	}))
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "calls the document store") {
			t.Errorf("advised about scopes on a manifest whose scopes field is not even an array: %s", w.Message)
		}
	}
}
