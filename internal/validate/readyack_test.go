package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

// ---------------------------------------------------------------------------
// Fixtures.
// ---------------------------------------------------------------------------

// ackManifest is an otherwise-valid PAGE manifest. `build` splices in the
// buildCommand + outputDir pair the schema couples, so the outputDir-is-never-
// scanned case has a real outputDir to be ignored.
func ackManifest(build bool) string {
	extra := ""
	if build {
		extra = `, "buildCommand": "npm run build", "outputDir": "dist"`
	}
	return `{
		"blockId": "ack-block", "version": "0.1.0", "name": "Ack Block",
		"contentRating": "g", "scopes": [],
		"page": {"path": "/", "title": "Ack Block"},
		"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"}` + extra + `
	}`
}

// nonPageManifest declares no `page` surface at all.
const nonPageManifest = `{
	"blockId": "slot-block", "version": "0.1.0", "name": "Slot Block",
	"contentRating": "g", "scopes": [],
	"iframe": {"minHeight": 400, "resizable": true, "sandbox": "allow-scripts"}
}`

// realEmitter is what a correctly scaffolded SDK-free page app ships. Taken
// from blockproto rather than retyped, so this fixture cannot drift away from
// the thing the CLI actually writes.
func realEmitter() string { return string(blockproto.ReadyAckSource()) }

// ackProject writes a project (paths may contain subdirectories) and returns it.
func ackProject(t *testing.T, manifestJSON string, files map[string]string) string {
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

// hasReadyAckWarning compares against the advisory VALUE, not a substring of its
// prose. A substring match would keep passing if the message were rewritten into
// something that no longer says what to do, and would also match a different
// warning that happened to share a phrase.
func hasReadyAckWarning(res Result) bool {
	for _, w := range res.Warnings {
		if w == readyAckAdvice {
			return true
		}
	}
	return false
}

func wantAckWarning(t *testing.T, dir string, want bool) Result {
	t.Helper()
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got := hasReadyAckWarning(res); got != want {
		t.Fatalf("ready-ack warning = %v, want %v\nwarnings: %v\nerrors: %v", got, want, res.Warnings, res.Errors)
	}
	return res
}

// ---------------------------------------------------------------------------
// The table.
// ---------------------------------------------------------------------------

func TestReadyAckWarning(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		files    map[string]string
		wantWarn bool
		why      string
	}{
		{
			// THE TARGET POPULATION: a page app scaffolded before 4018e2c. Its
			// index.html loads app.js, and neither says hello.
			name:     "page app with no ack at all warns",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     `document.title = 'hi';`,
			},
			wantWarn: true,
			why:      "this is exactly the #206 shape the check exists to find",
		},
		{
			name:     "page app shipping the real emitter does not warn",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html":       `<!doctype html><script src="./civitai-host.js"></script>`,
				"civitai-host.js":  realEmitter(),
				"unrelated.txt":    "BLOCK_READY",
				"src/component.js": `export const x = 1;`,
			},
			wantWarn: false,
		},
		{
			// 🔴 THE REGRESSION THAT WOULD HURT MOST. The SDK's IframeTransport
			// acks internally and the literal never appears in src/, so a
			// source-only check warns at EVERY correct page-money app.
			name:     "page app depending on @civitai/blocks-react does not warn",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"react": "^19.0.0", "@civitai/blocks-react": "^0.39.0"}}`,
				"index.html":   `<script type="module" src="/src/main.tsx"></script>`,
				"src/main.tsx": `import { BlockProvider } from '@civitai/blocks-react';`,
			},
			wantWarn: false,
		},
		{
			name:     "the SDK dep counts from devDependencies too",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"devDependencies": {"@civitai/app-sdk": "^0.31.0"}}`,
				"src/main.ts":  `export const x = 1;`,
			},
			wantWarn: false,
		},
		{
			// The dep gate must be about @civitai/*, not about "has a
			// package.json". Without this, adding any dependency would silence
			// the check.
			name:     "a package.json with no @civitai dep still warns",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"react": "^19.0.0", "react-dom": "^19.0.0"}}`,
				"index.html":   `<script type="module" src="/src/main.jsx"></script>`,
				"src/main.jsx": `import App from './App';`,
			},
			wantWarn: true,
		},
		{
			name:     "a non-page app never warns",
			manifest: nonPageManifest,
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     `document.title = 'hi';`,
			},
			wantWarn: false,
			why:      "the ready contract pinned here is the PAGE host's",
		},
		{
			// 🔴 The comment strip. Both shipped SDK-free templates carry this
			// exact sentence in a source comment, and it SURVIVES deleting the
			// emitter — so without stripping, the check is inert on the very
			// apps it was written for.
			name:     "an ack named only in a comment warns",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js": "// The ONE message a page app must send is `BLOCK_READY`; without it the host\n" +
					"// shows a failure card.\ndocument.title = 'hi';\n",
			},
			wantWarn: true,
		},
		{
			name:     "an ack named only in an HTML comment warns",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": "<!doctype html>\n<!-- we post BLOCK_READY from app.js -->\n<script src=\"./app.js\"></script>",
				"app.js":     `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			name:     "an ack described only in the README warns",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     `document.title = 'hi';`,
				"README.md":  "The host waits for a `BLOCK_READY` message before it will show your app.",
			},
			wantWarn: true,
			why:      "docs are not an implementation; both scaffold READMEs describe the handshake",
		},
		{
			// outputDir is never read: it does not exist before the first build
			// and is gitignored, so a hit there says nothing about what is
			// submitted... and its ABSENCE must not be read as evidence either.
			name:     "an ack present only in outputDir warns",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json":     `{"dependencies": {"react": "^19.0.0"}}`,
				"index.html":       `<script type="module" src="/src/main.jsx"></script>`,
				"src/main.jsx":     `import App from './App';`,
				"dist/assets/x.js": `window.parent.postMessage({type:'BLOCK_READY',payload:{}},o)`,
			},
			wantWarn: true,
		},
		{
			name:     "an ack in node_modules does not count",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json":            `{"dependencies": {"react": "^19.0.0"}}`,
				"src/main.jsx":            `import App from './App';`,
				"node_modules/dep/idx.js": `postMessage({type:'BLOCK_READY'})`,
			},
			wantWarn: true,
		},
		{
			name:     "an inline ack in index.html does not warn",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": "<!doctype html><script>\n" +
					"window.addEventListener('message', function (e) {\n" +
					"  if (e.data && e.data.type === 'BLOCK_INIT') {\n" +
					"    window.parent.postMessage({ type: 'BLOCK_READY', payload: {} }, e.origin);\n" +
					"  }\n});\n</script>",
			},
			wantWarn: false,
		},
		{
			name:     "an ack in a .vue single-file component does not warn",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json":  `{"dependencies": {"vue": "^3.5.0"}}`,
				"src/App.vue":   "<script setup>\nwindow.parent.postMessage({ type: 'BLOCK_READY', payload: {} }, origin);\n</script>",
				"package-x.txt": "ignored",
			},
			wantWarn: false,
		},
		{
			// A URL inside a string must not be read as the start of a comment —
			// otherwise everything after it is stripped and a real ack below it
			// disappears.
			name:     "a URL literal does not swallow the rest of the file",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js": "var docs = 'https://civitai.com/docs';\n" +
					"window.parent.postMessage({ type: 'BLOCK_READY', payload: {} }, o);\n",
			},
			wantWarn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantAckWarning(t, ackProject(t, tt.manifest, tt.files), tt.wantWarn)
			if tt.why != "" {
				t.Logf("%s — %s", tt.name, tt.why)
			}
		})
	}
}

// TestReadyAckCannotObserve pins the early-outs. Each of these is a project the
// check cannot answer for, and "cannot answer" must produce NO finding rather
// than a guess — the false-warning-at-a-correct-project failure AGENTS.md item
// 10 exists to prevent.
func TestReadyAckCannotObserve(t *testing.T) {
	t.Run("an unparseable package.json silences the check", func(t *testing.T) {
		dir := ackProject(t, ackManifest(true), map[string]string{
			"package.json": `{"dependencies": {`, // truncated
			"src/main.jsx": `import App from './App';`,
		})
		wantAckWarning(t, dir, false)
	})

	t.Run("a directory holding only a manifest silences the check", func(t *testing.T) {
		// Zero source files READ. A zero-hit scan over a zero-file tree says
		// nothing about the app — it is the same observation a scanner wired to
		// nothing would produce.
		dir := ackProject(t, ackManifest(false), nil)
		wantAckWarning(t, dir, false)
	})

	t.Run("a tree of only unscanned file types silences the check", func(t *testing.T) {
		dir := ackProject(t, ackManifest(false), map[string]string{
			"README.md":  "no code here",
			"notes.txt":  "nor here",
			"logo.svg":   "<svg/>",
			"styles.css": "body{}",
		})
		wantAckWarning(t, dir, false)
	})

	t.Run("one scannable file is enough to conclude", func(t *testing.T) {
		// The positive control for the two cases above: as soon as the scanner
		// really reads a line of code, a missing ack IS a finding. Without this,
		// "scanned == 0 means silence" is indistinguishable from silence always.
		dir := ackProject(t, ackManifest(false), map[string]string{
			"README.md":  "no code here",
			"index.html": `<!doctype html><h1>hi</h1>`,
		})
		wantAckWarning(t, dir, true)
	})

	t.Run("an unreadable package.json silences the check", func(t *testing.T) {
		dir := ackProject(t, ackManifest(true), map[string]string{
			"src/main.jsx": `import App from './App';`,
		})
		// A DIRECTORY where package.json belongs: os.ReadFile fails with
		// something that is NOT os.IsNotExist, on every platform and as any
		// user (a chmod-based fixture is a no-op for root).
		if err := os.Mkdir(filepath.Join(dir, "package.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		wantAckWarning(t, dir, false)
	})
}

// TestReadyAckIsAdvisoryOnly pins the tier. The check must never reach Errors —
// it infers runtime behaviour from static text, and a hard failure on a
// heuristic breaks correct projects.
func TestReadyAckIsAdvisoryOnly(t *testing.T) {
	dir := ackProject(t, ackManifest(false), map[string]string{
		"index.html": `<!doctype html><script src="./app.js"></script>`,
		"app.js":     `document.title = 'hi';`,
	})
	res := wantAckWarning(t, dir, true)
	for _, e := range res.Errors {
		if e == readyAckAdvice {
			t.Fatalf("the ready-ack advisory reached Errors — it must stay a warning")
		}
	}
	if !res.OK() {
		t.Fatalf("a page app missing the ack must still VALIDATE (exit 0 without --strict); errors: %v", res.Errors)
	}
}

// TestReadyAckSkippedByManifestOnly pins the PLACEMENT. warningChecks runs
// unconditionally, including under ManifestOnly — which `civitai app init` uses
// to self-check the template it just wrote. A check that reads src/ placed there
// would make init's self-validation depend on files that are not its business.
func TestReadyAckSkippedByManifestOnly(t *testing.T) {
	dir := ackProject(t, ackManifest(false), map[string]string{
		"index.html": `<!doctype html><script src="./app.js"></script>`,
		"app.js":     `document.title = 'hi';`,
	})

	// Positive control: the same directory DOES warn through Dir. Without it, a
	// green below is indistinguishable from a check that never fires at all.
	wantAckWarning(t, dir, true)

	res, err := ManifestOnly(dir)
	if err != nil {
		t.Fatalf("ManifestOnly: %v", err)
	}
	if hasReadyAckWarning(res) {
		t.Fatalf("ManifestOnly emitted the ready-ack advisory; it must stay in the projectState branch\nwarnings: %v", res.Warnings)
	}
}

// ---------------------------------------------------------------------------
// The seam guard: the CHECK against the real TEMPLATES.
// ---------------------------------------------------------------------------

// TestShippedTemplatesDoNotTripTheReadyAckWarning is the guard that matters
// most. Every unit case above builds a synthetic project; this one renders the
// REAL templates and requires the CLI's own check to accept them, so drift on
// EITHER side (a template that stops shipping the emitter, or a check that
// stops recognising it) fails loudly. It mirrors the seam guards in
// internal/scaffold/dev_embed_contract_test.go.
//
// It deliberately ignores validation ERRORS: a freshly rendered page-vite has
// no lockfile yet, which is a legitimate hard error and not this test's subject.
func TestShippedTemplatesDoNotTripTheReadyAckWarning(t *testing.T) {
	examined := 0
	for _, tmpl := range scaffold.AllTemplates() {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), string(tmpl))
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "ack-block", Name: "Ack Block"}); err != nil {
				t.Fatalf("render %s: %v", tmpl, err)
			}
			wantAckWarning(t, dest, false)
		})
		examined++
	}
	// COUNT POSITIVE CONTROL: a zero here is otherwise indistinguishable from an
	// enumeration that returned nothing.
	if examined < 3 {
		t.Fatalf("examined only %d template(s), expected at least 3 — the enumeration stopped OBSERVING", examined)
	}
}

// TestSDKTemplateIsQuietOnlyBecauseOfTheDependency is the positive control for
// the dep gate. page-money is accepted above; this proves WHY. If its source
// ever did contain the literal, the case above would pass for a reason that has
// nothing to do with the branch it is meant to cover, and deleting the dep gate
// would go unnoticed — which is precisely the regression that false-warns at
// every correct money-path app.
func TestSDKTemplateIsQuietOnlyBecauseOfTheDependency(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "page-money")
	if _, err := scaffold.Render(scaffold.PageMoney, dest, scaffold.Data{Slug: "ack-block", Name: "Ack Block"}); err != nil {
		t.Fatal(err)
	}
	if emitsReadyAck(dest, "dist") {
		t.Fatalf("page-money's SOURCE mentions %s — the dep gate is no longer the reason it is accepted, "+
			"so nothing in this suite covers that branch any more", readyAckType)
	}
	if got := sdkDependency(dest); got != sdkPresent {
		t.Fatalf("sdkDependency(page-money) = %v, want sdkPresent — the rendered package.json must depend on %s*",
			got, civitaiDepPrefix)
	}
	wantAckWarning(t, dest, false)
}

// TestDeletingTheEmitterMakesTheWarningFire is the NEGATIVE CONTROL for the
// guard above: it takes a correct scaffold, breaks it exactly the way a
// pre-4018e2c app is broken, and requires the warning. Without it,
// "no template warns" is satisfied by a check that never warns about anything.
func TestDeletingTheEmitterMakesTheWarningFire(t *testing.T) {
	for _, tmpl := range []scaffold.Template{scaffold.Static, scaffold.PageVite} {
		t.Run(string(tmpl), func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), string(tmpl))
			if _, err := scaffold.Render(tmpl, dest, scaffold.Data{Slug: "ack-block", Name: "Ack Block"}); err != nil {
				t.Fatal(err)
			}
			wantAckWarning(t, dest, false)

			rel := tmpl.ReadyAckPath()
			if rel == "" {
				t.Fatalf("%s no longer declares a ReadyAckPath — this control has nothing to delete", tmpl)
			}
			if err := os.Remove(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
			wantAckWarning(t, dest, true)

			// The advisory must name the file the author has to restore.
			if !strings.Contains(readyAckAdvice, blockproto.ReadyAckFilename) {
				t.Errorf("the advisory does not name %s — an author cannot act on it", blockproto.ReadyAckFilename)
			}
		})
	}
}
