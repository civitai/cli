package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// storageManifest is a minimal VALID page manifest declaring `scopes`.
//
// It must pass the schema and the semantic rules, or a test asserting "no
// warning" could pass because validation bailed earlier — and a test asserting
// "warning" could be reading somebody else's finding.
func storageManifest(t *testing.T, scopes ...string) string {
	t.Helper()
	quoted := make([]string, 0, len(scopes))
	just := make([]string, 0, len(scopes))
	for _, s := range scopes {
		quoted = append(quoted, `"`+s+`"`)
		// Every SENSITIVE scope needs a justification or the manifest is a hard
		// error; supplying one for all of them is simpler than tracking which.
		just = append(just, `"`+s+`": "exercised by a validate test"`)
	}
	return `{
  "blockId": "storage-scope-fixture",
  "version": "0.1.0",
  "name": "Storage Scope Fixture",
  "tagline": "a fixture",
  "type": "block",
  "scopes": [` + strings.Join(quoted, ", ") + `],
  "scopeJustifications": {` + strings.Join(just, ", ") + `},
  "page": { "path": "/", "title": "Storage Scope Fixture", "icon": "bolt" },
  "iframe": { "minHeight": 600, "maxHeight": 4000, "resizable": true, "sandbox": "allow-scripts" },
  "contentRating": "g",
  "bootSkeleton": true,
  "minApiVersion": "1.0"
}`
}

// storageWarnings returns only this check's findings, keyed on the hook each
// message names.
//
// 🔴 It filters by the SCOPE NAMES the advisory carries, not by a loose phrase:
// this project also emits a ready-ack advisory for the same fixtures (a page app
// whose source never acks), so a test that counted ALL warnings would pass on the
// wrong finding entirely. Asked for and measured — the fixtures below DO carry a
// ready-ack warning, and an unfiltered count read 2 where this check emitted 1.
func storageWarnings(t *testing.T, dir string) (app, shared bool, fields []string) {
	t.Helper()
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("fixture manifest must be VALID or the test measures nothing; errors: %v", res.Errors)
	}
	for _, w := range res.Warnings {
		switch {
		case strings.Contains(w.Message, "`useAppStorage()`"):
			app = true
			fields = append(fields, w.Field)
		case strings.Contains(w.Message, "`useSharedStorage()`"):
			shared = true
			fields = append(fields, w.Field)
		}
	}
	return app, shared, fields
}

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
export function Board() { const s = useSharedStorage(); return s; }`

// TestStorageScopeWarning covers the check in BOTH directions. A gate that
// warned unconditionally would pass every "wantApp: true" row, so the rows that
// expect SILENCE are the ones carrying the weight.
func TestStorageScopeWarning(t *testing.T) {
	tests := []struct {
		name       string
		scopes     []string
		files      map[string]string
		wantApp    bool
		wantShared bool
		why        string
	}{
		{
			// THE TARGET POPULATION — the 2026-10-01 incident, exactly: storage
			// everywhere, the money scope only.
			name:    "uses app storage, declares only the money scope",
			scopes:  []string{"ai:write:budgeted"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
			why:     "this is the shape that shipped and 403'd in production",
		},
		{
			name:    "declares both storage scopes — silent",
			scopes:  []string{"apps:storage:read", "apps:storage:write"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: false,
			why:     "REACHABILITY: proves the check is not a blanket warn",
		},
		{
			// The deliberate narrowness: one storage scope is someone who knows
			// scopes exist and chose. We do not second-guess WHICH.
			name:    "declares only the READ scope — silent, by design",
			scopes:  []string{"apps:storage:read"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: false,
			why:     "a read-only app that declared read must not be nagged about write",
		},
		{
			// 🔴 THE PREFIX TRAP, and the single most important row here.
			// `apps:storage:shared:read` HAS `apps:storage:` as a prefix, so a
			// plain HasPrefix test reads this manifest as having declared the
			// per-viewer store and SUPPRESSES the finding — a false negative in
			// the exact direction this check exists to remove.
			name:    "declares only SHARED scopes but uses per-viewer storage",
			scopes:  []string{"apps:storage:shared:read", "apps:storage:shared:write"},
			files:   map[string]string{"src/App.tsx": usesApp},
			wantApp: true,
			why:     "a shared scope does not satisfy the per-viewer store",
		},
		{
			name:       "declares only per-viewer scopes but uses SHARED storage",
			scopes:     []string{"apps:storage:read", "apps:storage:write"},
			files:      map[string]string{"src/Board.tsx": usesShared},
			wantShared: true,
			why:        "the mirror of the row above; the two stores are independent",
		},
		{
			name:       "uses BOTH stores, declares neither — two findings",
			scopes:     []string{"ai:write:budgeted"},
			files:      map[string]string{"src/App.tsx": usesApp, "src/Board.tsx": usesShared},
			wantApp:    true,
			wantShared: true,
		},
		{
			name:   "no storage use at all — silent",
			scopes: []string{"ai:write:budgeted"},
			files:  map[string]string{"src/App.tsx": `export const x = 1;`},
		},
		{
			// Comment stripping. The scaffolds DOCUMENT these hooks in prose, so
			// without stripping the check fires at a project that only mentions
			// storage in a comment.
			name:   "the hook named only in a COMMENT — silent",
			scopes: []string{"ai:write:budgeted"},
			files: map[string]string{
				"src/App.tsx": "// later: persist with useAppStorage()\n/* or useSharedStorage() */\nexport const x = 1;",
			},
			why: "a comment is not a call",
		},
		{
			name:   "the hook named only in a NON-SOURCE file — silent",
			scopes: []string{"ai:write:budgeted"},
			files: map[string]string{
				"README.md":   "we will use useAppStorage() eventually",
				"notes.txt":   "useSharedStorage()",
				"src/App.tsx": `export const x = 1;`,
			},
			why: "the extension filter: prose and data are not code",
		},
		{
			// A dependency that mentions the hook is not THIS app using it, and a
			// vendored tree would otherwise make the check fire at every app.
			name:   "the hook only inside node_modules — silent",
			scopes: []string{"ai:write:budgeted"},
			files: map[string]string{
				"node_modules/@civitai/blocks-react/dist/index.js": usesApp,
				"src/App.tsx": `export const x = 1;`,
			},
			why: "skip-dirs: a dependency's own source is not the app's",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := storageProject(t, storageManifest(t, tc.scopes...), tc.files)
			app, shared, fields := storageWarnings(t, dir)
			if app != tc.wantApp {
				t.Errorf("app-storage warning = %v, want %v (%s)", app, tc.wantApp, tc.why)
			}
			if shared != tc.wantShared {
				t.Errorf("shared-storage warning = %v, want %v (%s)", shared, tc.wantShared, tc.why)
			}
			// 🔴 The FIELD is part of the contract, not decoration. readyack
			// reports `(project)` because its remedy is in source; this one must
			// report `scopes`, because the source is correct and the MANIFEST is
			// what needs the edit. Sending a consumer to `(project)` here would
			// point them at the one place that is already right.
			for _, f := range fields {
				if f != "scopes" {
					t.Errorf("Field = %q, want \"scopes\"", f)
				}
			}
		})
	}
}

// TestStorageScopeFollowsSymlinkedSource pins the hazard readyack recorded after
// reproducing it live: filepath.WalkDir does NOT follow symlinked directories and
// does not report them as directories either, so a monorepo whose `src` is a
// symlink into a shared package had its ENTIRE source tree skipped. For THIS
// check the consequence is the silent direction — storage use invisible, no
// warning, the defect ships.
func TestStorageScopeFollowsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	// The real source lives outside the project, as in a monorepo.
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
	if err := os.WriteFile(
		filepath.Join(proj, "block.manifest.json"),
		[]byte(storageManifest(t, "ai:write:budgeted")), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(proj, "src")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	app, _, _ := storageWarnings(t, proj)
	if !app {
		t.Error("storage use behind a symlinked src/ was not seen — the walk stopped following symlinks")
	}
}

// TestStorageScopeUnobservableScanStaysSilent pins the posture readyack states:
// reading only PART of a tree is not finding nothing. A scan that hits a cap must
// not report against a project it did not finish reading.
func TestStorageScopeUnobservableScanStaysSilent(t *testing.T) {
	big := strings.Repeat("x", maxStorageFileBytes+1)
	dir := storageProject(t, storageManifest(t, "ai:write:budgeted"), map[string]string{
		// 🔴 THE ORDERING IS THE TEST. os.ReadDir returns entries SORTED, so the
		// hook file must sort BEFORE the oversized one: the scan then sets
		// `usesAppStorage` and only afterwards goes partial, leaving the partial
		// guard as the single thing that can suppress the finding.
		//
		// Reversed — the big file first — this test passes whether the guard
		// exists or not, because the hook is never read and `usesAppStorage`
		// stays false. It was written that way, the mutation sweep scored the
		// guard SURVIVED, and that is what exposed it. A fixture whose values
		// make the guarded branch unreachable is the classic vacuous pass.
		"src/aaa-App.tsx":   usesApp,
		"src/zzz-bundle.js": big,
	})
	app, _, _ := storageWarnings(t, dir)
	if app {
		t.Error("warned from a scan that hit its byte cap — a partial read must stay silent")
	}
}

// TestStorageScopeDeclaresStore is the unit-level pin on the prefix logic, so a
// regression names the predicate rather than surfacing as a confusing fixture
// failure three layers up.
func TestStorageScopeDeclaresStore(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		prefix   string
		want     bool
	}{
		{"per-viewer read satisfies per-viewer", []string{"apps:storage:read"}, appStorageScopePrefix, true},
		{"per-viewer write satisfies per-viewer", []string{"apps:storage:write"}, appStorageScopePrefix, true},
		{"shared does NOT satisfy per-viewer", []string{"apps:storage:shared:write"}, appStorageScopePrefix, false},
		{"shared satisfies shared", []string{"apps:storage:shared:read"}, sharedStorageScopePrefix, true},
		{"per-viewer does NOT satisfy shared", []string{"apps:storage:write"}, sharedStorageScopePrefix, false},
		{"money scope satisfies neither", []string{"ai:write:budgeted"}, appStorageScopePrefix, false},
		{"empty satisfies neither", nil, sharedStorageScopePrefix, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaresStore(tc.declared, tc.prefix); got != tc.want {
				t.Errorf("declaresStore(%v, %q) = %v, want %v", tc.declared, tc.prefix, got, tc.want)
			}
		})
	}
}

// TestStorageScopeTolerABadScopesField pins that a malformed `scopes` produces no
// finding from THIS check — the schema layer reports that defect, and a second
// confusing advisory about the same thing is worse than none.
func TestStorageScopeToleratesABadScopesField(t *testing.T) {
	for _, raw := range []string{`"scopes": "not-an-array"`, `"scopes": [1, 2]`, `"scopes": []`} {
		t.Run(raw, func(t *testing.T) {
			// Built by hand: storageManifest always emits a well-formed array.
			m := `{"blockId":"x","version":"0.1.0","name":"X","tagline":"t","type":"block",` + raw +
				`,"page":{"path":"/","title":"X","icon":"bolt"},` +
				`"iframe":{"minHeight":600,"maxHeight":4000,"resizable":true,"sandbox":"allow-scripts"},` +
				`"contentRating":"g","bootSkeleton":true,"minApiVersion":"1.0"}`
			dir := storageProject(t, m, map[string]string{"src/App.tsx": usesApp})
			// Dir may legitimately report schema errors here; we only assert this
			// check does not PANIC and does not invent a second story.
			res, err := Dir(dir)
			if err != nil {
				t.Fatalf("Dir: %v", err)
			}
			for _, w := range res.Warnings {
				if strings.Contains(w.Message, "`useAppStorage()`") && raw == `"scopes": "not-an-array"` {
					t.Error("warned about scopes on a manifest whose scopes field is not even an array")
				}
			}
		})
	}
}
