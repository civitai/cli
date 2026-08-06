package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/scaffold"
)

// ---------------------------------------------------------------------------
// Fixtures.
// ---------------------------------------------------------------------------

// ackManifest is an otherwise-valid PAGE manifest. `build` splices in the
// buildCommand + outputDir pair the schema couples, so the outputDir-is-never-
// scanned case has a real outputDir to be ignored.
func ackManifest(build bool) string { return ackManifestOut(build, "dist") }

// ackManifestOut is ackManifest with an explicit outputDir. `dist` is ALSO in
// readyAckSkipDirs, so a fixture using it can never reach the outputDir branch —
// the name-based skip always wins first. A non-standard outputDir is the only
// way to exercise that branch, which is why this exists.
func ackManifestOut(build bool, outputDir string) string {
	extra := ""
	if build {
		extra = `, "buildCommand": "npm run build", "outputDir": "` + outputDir + `"`
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

// nullPageManifest sets the key EXPLICITLY to null. `encoding/json` decodes that
// to a present key holding a nil value, so a presence-only test ("is the key
// there") reads it as a page app and a `v != nil` test does not.
const nullPageManifest = `{
	"blockId": "null-page", "version": "0.1.0", "name": "Null Page",
	"contentRating": "g", "scopes": [], "page": null,
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

// hasReadyAckWarning compares against the advisory VALUES, not a substring of
// their prose. A substring match would keep passing if a message were rewritten
// into something that no longer says what to do, and would also match a
// different warning that happened to share a phrase.
//
// It matches the SET, so a caller asking only "did the ready-ack check fire"
// keeps working across the tiers. A caller that cares WHICH tier fired must use
// readyAckKind — "it warned" and "it warned for the right reason" are different
// claims, and the whole defect this file now covers was a check that answered
// the weaker question while the message promised the stronger one.
func hasReadyAckWarning(res Result) bool { return readyAckKind(res) != "" }

// readyAckKind names the tier that fired, or "".
func readyAckKind(res Result) string {
	for _, w := range res.Warnings {
		switch w {
		case readyAckAdviceUnwired:
			return "unwired"
		case readyAckAdviceMissing:
			return "missing"
		case readyAckAdvicePresenceOnly:
			return "presence-only"
		}
	}
	return ""
}

// wantAckKind asserts the exact tier. `want` of "" means no ready-ack warning.
func wantAckKind(t *testing.T, dir, want string) Result {
	t.Helper()
	res, err := Dir(dir)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got := readyAckKind(res); got != want {
		t.Fatalf("ready-ack advisory tier = %q, want %q\nwarnings: %v\nerrors: %v", got, want, res.Warnings, res.Errors)
	}
	return res
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
			name:     "the acking dep counts from devDependencies too",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"devDependencies": {"@civitai/blocks-react": "^0.39.0"}}`,
				"src/main.ts":  `export const x = 1;`,
			},
			wantWarn: false,
		},
		{
			name:     "the acking dep counts from peerDependencies too",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"peerDependencies": {"@civitai/blocks-react": "^0.39.0"}}`,
				"src/main.ts":  `export const x = 1;`,
			},
			wantWarn: false,
		},
		{
			// 🔴 THE FALSIFIED CLAIM. `@civitai/theme` is framework-agnostic CSS
			// tokens and contains no BLOCK_READY at all (measured on 0.2.0), so a
			// scope test silenced a genuinely broken app. Reproduced live on a
			// `static` scaffold with the emitter deleted before this case existed.
			name:     "a non-acking @civitai package does NOT silence the check",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"@civitai/theme": "^0.2.0", "@civitai/components": "^0.3.0"}}`,
				"index.html":   `<!doctype html><script src="./app.js"></script>`,
				"app.js":       `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			// `@civitai/app-sdk@0.31.0` ships 17 runtime .js files and NONE posts
			// BLOCK_READY — the only occurrences are a .d.ts type literal and the
			// README, and it declares no dependencies, so it cannot be acking
			// transitively. An app-sdk-only project genuinely does not ack.
			name:     "@civitai/app-sdk alone does NOT silence the check",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"@civitai/app-sdk": "^0.31.0"}}`,
				"index.html":   `<!doctype html><script type="module" src="/src/main.ts"></script>`,
				"src/main.ts":  `export const x = 1;`,
			},
			wantWarn: true,
		},
		{
			// Prefix widening: a sibling package name that merely starts the
			// same way makes no ack promise.
			name:     "a sibling of the acking package does NOT silence the check",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"@civitai/blocks-react-native": "^1.0.0"}}`,
				"index.html":   `<!doctype html><script src="./app.js"></script>`,
				"app.js":       `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			// Substring widening: a fork that merely CONTAINS the acking name.
			name:     "a re-scoped fork does NOT silence the check",
			manifest: ackManifest(true),
			files: map[string]string{
				"package.json": `{"dependencies": {"@myorg/@civitai/blocks-react": "^1.0.0"}}`,
				"index.html":   `<!doctype html><script src="./app.js"></script>`,
				"app.js":       `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			// The dep gate must be about a package that ACKS, not about "has a
			// package.json". Without this, adding any dependency would silence
			// the check.
			name:     "a package.json with no acking dep still warns",
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
			// An explicit null is not a page surface. Pins the `v != nil` half of
			// declaresPage: a presence-only test passes every other case in this
			// table, so without this the two halves are indistinguishable.
			name:     `an explicit "page": null does not warn`,
			manifest: nullPageManifest,
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     `document.title = 'hi';`,
			},
			wantWarn: false,
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
			// The conventional build directories are skipped BY NAME, for cost.
			// A stale `dist` therefore cannot supply ack evidence the source
			// lacks — the one correctness benefit of skipping, and the reason
			// the name list keeps these entries.
			name:     "an ack present only in dist/ warns",
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
			// 🔴 THE FALSIFIED CLAIM, HALF ONE. `outputDir` used to be skipped
			// from the MANIFEST, which meant a perfectly valid `"outputDir":
			// "src"` skipped the directory holding the emitter and warned at a
			// correct project — reproduced live on a page-vite scaffold whose
			// manifest had ZERO validation errors. Skipping is a cost decision,
			// never a correctness one: for a presence check, scanning extra can
			// only ADD evidence while skipping can only CREATE a false warning.
			name:     `"outputDir": "src" must not hide the emitter that lives there`,
			manifest: ackManifestOut(true, "src"),
			files: map[string]string{
				"package.json":        `{"dependencies": {"react": "^19.0.0"}}`,
				"index.html":          `<script type="module" src="/src/main.jsx"></script>`,
				"src/main.jsx":        `import './civitai-host.js';`,
				"src/civitai-host.js": realEmitter(),
			},
			wantWarn: false,
		},
		{
			// 🔴 HALF TWO. `"outputDir": "."` resolved to the project root, so the
			// manifest-driven skip removed the ENTIRE tree, `scanned == 0`, and a
			// genuinely broken app was silenced. No skip rule may ever be able to
			// remove the root: the list is applied to ENTRIES, never to the root
			// the scanner was handed.
			name:     `"outputDir": "." must not skip the whole project`,
			manifest: ackManifestOut(true, "."),
			files: map[string]string{
				"package.json": `{"dependencies": {"react": "^19.0.0"}}`,
				"index.html":   `<!doctype html><script src="./app.js"></script>`,
				"app.js":       `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			// A non-conventional output directory is now SCANNED. That is the
			// accepted residual trade: a stale build under `public/` can retain
			// an ack the source lost and silence the check. A false negative,
			// deliberately preferred over false-warning at every project whose
			// outputDir happens to hold source (Vite's `public/` is a SOURCE
			// directory, which is what made this the realistic shape).
			name:     "a non-conventional outputDir is scanned rather than skipped",
			manifest: ackManifestOut(true, "public"),
			files: map[string]string{
				"package.json":  `{"dependencies": {"react": "^19.0.0"}}`,
				"index.html":    `<script type="module" src="/src/main.jsx"></script>`,
				"src/main.jsx":  `import App from './App';`,
				"public/app.js": `window.parent.postMessage({type:'BLOCK_READY',payload:{}},o)`,
			},
			wantWarn: false,
		},
		{
			// 🔴 An unterminated `<!--` in a .js used to truncate the scan of that
			// file, deleting the emitter below it and warning at a correct
			// project. Reproduced live by prefixing a scaffold's own
			// civitai-host.js with this line. Two defences now: HTML stripping is
			// not applied to .js at all, and an unterminated marker keeps the
			// remainder verbatim.
			name:     "an unterminated <!-- in a .js does not truncate the scan",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     "var OPEN_COMMENT = '<!--';\n" + realEmitter(),
			},
			wantWarn: false,
		},
		{
			// The same hazard in a MARKUP file, where HTML stripping does apply:
			// the unterminated marker must keep the tail rather than discard it.
			name:     "an unterminated <!-- in .html keeps the rest of the file",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": "<!doctype html>\n<!-- unterminated\n<script>\n" +
					"window.parent.postMessage({ type: 'BLOCK_READY', payload: {} }, o);\n</script>",
			},
			wantWarn: false,
		},
		{
			// A .js pair of `<!--` … `-->` in strings must not swallow what is
			// between them either — the reason HTML stripping is gated to markup.
			name:     "a matched <!-- --> pair in .js strings does not eat the emitter",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js": "var OPEN = '<!--';\n" +
					"window.parent.postMessage({ type: 'BLOCK_READY', payload: {} }, o);\n" +
					"var CLOSE = '-->';\n",
			},
			wantWarn: false,
		},
		{
			// A real HTML comment in a markup file must STILL be stripped — the
			// positive control for the gate above. Without it, "don't strip HTML
			// in .js" is indistinguishable from "don't strip HTML at all".
			name:     "an HTML comment in .html is still stripped",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": "<!doctype html>\n<!-- TODO: post BLOCK_READY here -->\n<script src=\"./app.js\"></script>",
				"app.js":     `document.title = 'hi';`,
			},
			wantWarn: true,
		},
		{
			// A block comment naming the message is still a comment. There was no
			// /* */ fixture at all before.
			name:     "an ack named only in a /* */ block comment warns",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js":     "/*\n * We should post BLOCK_READY at some point.\n */\ndocument.title = 'hi';\n",
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
			// A URL inside a string must not be read as the start of a comment.
			// The ack is on the SAME LINE as the URL on purpose: with quote
			// handling defeated, the `//` in `https://` starts a line comment and
			// swallows everything after it — including the ack. Split across two
			// lines this case passes with the string handling deleted, which is
			// how the first version of it left that branch unproven.
			name:     "a URL literal does not swallow the ack after it",
			manifest: ackManifest(false),
			files: map[string]string{
				"index.html": `<!doctype html><script src="./app.js"></script>`,
				"app.js": "var docs = 'https://civitai.com/docs'; " +
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

	t.Run("an unreadable source file silences the check", func(t *testing.T) {
		// A DANGLING SYMLINK with a scanned extension: os.Stat cannot resolve it,
		// so the walk stops there. We then know nothing about the rest of the
		// tree — including whatever we had not reached yet — so the only honest
		// answer is silence.
		//
		// 🔴 This fixture reaches the os.Stat guard, NOT the os.ReadFile one
		// below it. Measured: mutating that ReadFile branch to `continue` fails
		// NOTHING in the suite, and the same is true of the os.ReadDir branch.
		// Both are reachable only via permissions (a mode-0000 file, a mode-0111
		// dir), and chmod is a no-op for root, so neither can be pinned on a root
		// CI runner. Shipped behaviour for both was verified by hand — they stay
		// silent — but do not read this case as covering them.
		dir := ackProject(t, ackManifest(false), map[string]string{
			"index.html": `<!doctype html><script src="./app.js"></script>`,
			"app.js":     `document.title = 'hi';`,
		})
		if err := os.Symlink(filepath.Join(dir, "gone.js"), filepath.Join(dir, "broken.js")); err != nil {
			t.Skipf("symlinks unavailable here: %v", err)
		}
		wantAckWarning(t, dir, false)
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

// TestReadyAckSkipListNeverRemovesTheRoot pins the "applied to ENTRIES only"
// half of the skip rule.
//
// Found by a surviving mutant: adding `if readyAckSkipDirs[filepath.Base(dir)]
// { return }` to the top of the walk passed the ENTIRE suite, because every
// fixture's project root is a random t.TempDir() name. A project whose own
// directory is called `build`, `out` or `dist` is perfectly ordinary, and under
// that mutant it scanned nothing, reported "no files read", and went silent —
// the same whole-tree removal that `"outputDir": "."` used to cause.
func TestReadyAckSkipListNeverRemovesTheRoot(t *testing.T) {
	for _, name := range []string{"dist", "build", "out", "node_modules"} {
		t.Run("project directory named "+name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(p, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, p), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write("block.manifest.json", ackManifest(false))
			write("index.html", `<!doctype html><script src="./app.js"></script>`)
			write("app.js", `document.title = 'hi';`)
			// The #206 shape: it must still be REPORTED even though the project
			// happens to live in a directory whose name is on the skip list.
			wantAckWarning(t, root, true)

			// Positive control on the same tree: with an emitter it goes silent,
			// so the assertion above cannot be satisfied by a scan that warns
			// unconditionally.
			//
			// 🔴 index.html has to LOAD it. Writing the file alone used to be
			// enough here, and that is precisely the false pass this check was
			// rewritten to close — an unreferenced emitter is now (correctly) the
			// `unwired` finding, which this control would otherwise trip.
			write("civitai-host.js", realEmitter())
			write("index.html", `<!doctype html><script src="./civitai-host.js"></script><script src="./app.js"></script>`)
			wantAckWarning(t, root, false)
		})
	}
}

// TestReadyAckFollowsSymlinkedSource pins the monorepo shape. filepath.WalkDir
// does NOT follow symlinks and does not even report them as directories, so a
// project whose `src` is a symlink into a shared package had its entire source
// tree silently skipped and warned at a correct project — reproduced live on a
// page-vite scaffold before this existed.
func TestReadyAckFollowsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	proj := filepath.Join(root, "app")

	for _, d := range []string{shared, proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(proj, "block.manifest.json"), ackManifest(true))
	write(filepath.Join(proj, "package.json"), `{"dependencies": {"react": "^19.0.0"}}`)
	write(filepath.Join(proj, "package-lock.json"), `{"lockfileVersion":3}`)
	write(filepath.Join(proj, "index.html"), `<script type="module" src="/src/main.jsx"></script>`)
	write(filepath.Join(shared, "main.jsx"), `import './civitai-host.js';`)
	write(filepath.Join(shared, "civitai-host.js"), realEmitter())

	// Positive control FIRST: with the emitter reachable only through the link,
	// a green below could otherwise mean the scanner found it somewhere else.
	if err := os.Symlink(filepath.Join("..", "shared"), filepath.Join(proj, "src")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	wantAckWarning(t, proj, false)

	// Negative control: break the symlinked tree the same way #206 breaks an
	// app, and the warning must come back — otherwise "does not warn" above is
	// satisfied by a scanner that silently gave up on the symlink.
	if err := os.Remove(filepath.Join(shared, "civitai-host.js")); err != nil {
		t.Fatal(err)
	}
	wantAckWarning(t, proj, true)
}

// TestReadyAckScannerVisitsEachDirectoryOnce pins the visited set DIRECTLY, by
// counting files read.
//
// Found by a surviving mutant: deleting the visited set entirely passed the
// suite, including the cycle test below. That test was proving the wrong thing —
// a self-referential symlink terminates anyway, because Linux gives up with
// ELOOP after ~40 resolutions and the resulting error makes the scan partial. So
// termination is guaranteed by the OS, and the visited set's real job is
// avoiding DUPLICATE WORK: without it, two links to the same directory read
// every file in it twice, inflating the file count toward a budget whose only
// effect is to silence the check.
//
// Asserting the count is the only thing that can see that. This walks the
// scanner directly rather than through Dir(), because the number of files read
// is the observable and the verdict is not.
func TestReadyAckScannerVisitsEachDirectoryOnce(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	const nFiles = 3
	for i := 0; i < nFiles; i++ {
		p := filepath.Join(shared, fmt.Sprintf("m%d.js", i))
		if err := os.WriteFile(p, []byte("var x=1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Two more routes to the same directory. A walk with no visited set reads
	// each file three times.
	for _, link := range []string{"link1", "link2"} {
		if err := os.Symlink("shared", filepath.Join(dir, link)); err != nil {
			t.Skipf("symlinks unavailable here: %v", err)
		}
	}

	s := &ackScanner{visited: map[string]bool{}}
	s.walk(dir)
	if s.partial {
		t.Fatalf("the scan reported partial on a readable tree — it should have read all %d files", nFiles)
	}
	if s.files != nFiles {
		t.Fatalf("scanner read %d files, want %d — three paths reach the same directory, so a walk that "+
			"does not record RESOLVED paths reads each file once per route", s.files, nFiles)
	}
}

// TestReadyAckSymlinkCycleTerminates pins that a self-referential link does not
// hang. Note what it does NOT prove: the OS's own ELOOP limit terminates this
// even with the visited set deleted, so this is a backstop assertion and
// TestReadyAckScannerVisitsEachDirectoryOnce is what actually covers the guard.
func TestReadyAckSymlinkCycleTerminates(t *testing.T) {
	dir := ackProject(t, ackManifest(false), map[string]string{
		"index.html":      `<!doctype html><script src="./app.js"></script>`,
		"src/app.js":      `document.title = 'hi';`,
		"src/nested/x.js": `document.title = 'hi';`,
	})
	if err := os.Symlink("..", filepath.Join(dir, "src", "loop")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	// The assertion is that this RETURNS at all; the verdict is secondary.
	done := make(chan bool, 1)
	go func() {
		res, err := Dir(dir)
		if err != nil {
			t.Error(err)
		}
		done <- hasReadyAckWarning(res)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the scan did not terminate on a symlink cycle — the visited set is not doing its job")
	}
}

// TestReadyAckCapValues pins the caps INDEPENDENTLY of the code under test.
//
// 🔴 TestReadyAckCapsAreUnobservable below sizes its fixtures FROM these
// constants, so a mutant that raises one does not fail — it just allocates. An
// audit mutant setting maxAckFileBytes to 1<<40 hung the suite twice instead of
// going red. A literal here is the only thing that can see that, and it is also
// the only record that these numbers are a deliberate cost decision rather than
// whatever the code happens to say.
func TestReadyAckCapValues(t *testing.T) {
	if maxAckFileBytes != 2<<20 {
		t.Errorf("maxAckFileBytes = %d, want %d (2 MiB) — these caps exist because `validate` must not "+
			"become a memory event; raising one silently makes every cap fixture allocate instead of fail",
			maxAckFileBytes, 2<<20)
	}
	if maxAckScanFiles != 5000 {
		t.Errorf("maxAckScanFiles = %d, want 5000", maxAckScanFiles)
	}
	if maxAckGraphFiles != 200 {
		t.Errorf("maxAckGraphFiles = %d, want 200", maxAckGraphFiles)
	}
}

// TestReadyAckCapsAreUnobservable pins that hitting a cap stays SILENT. A scan
// that stopped early has a gap in it, and a gap is not evidence of absence.
//
// It sizes its fixtures from the constants on purpose (a hardcoded 2 MiB would
// silently stop exercising the branch if the cap moved); TestReadyAckCapValues
// is what makes that safe.
func TestReadyAckCapsAreUnobservable(t *testing.T) {
	// 🔴 Sizing a fixture from the constant under test turns a raised cap into an
	// ALLOCATION rather than a failure: a 1<<40 mutant hung this suite twice
	// instead of going red. TestReadyAckCapValues is the real guard; this bound
	// makes the hang a fast, readable failure instead.
	if maxAckFileBytes > 8<<20 || maxAckScanFiles > 20000 {
		t.Fatalf("the caps are too large to exercise (maxAckFileBytes=%d, maxAckScanFiles=%d) — this test "+
			"builds fixtures from them, so it would allocate instead of testing. See TestReadyAckCapValues.",
			maxAckFileBytes, maxAckScanFiles)
	}
	t.Run("a file over the size cap silences the check", func(t *testing.T) {
		dir := ackProject(t, ackManifest(false), map[string]string{
			"index.html": `<!doctype html><script src="./app.js"></script>`,
			"app.js":     `document.title = 'hi';`,
		})
		// Positive control: without the oversized file this project warns.
		wantAckWarning(t, dir, true)
		big := strings.Repeat("x", maxAckFileBytes+1)
		if err := os.WriteFile(filepath.Join(dir, "bundle.js"), []byte(big), 0o600); err != nil {
			t.Fatal(err)
		}
		wantAckWarning(t, dir, false)
	})

	t.Run("exceeding the file budget silences the check", func(t *testing.T) {
		dir := ackProject(t, ackManifest(false), map[string]string{
			"index.html": `<!doctype html><script src="./app.js"></script>`,
			"app.js":     `document.title = 'hi';`,
		})
		wantAckWarning(t, dir, true)
		for i := 0; i <= maxAckScanFiles; i++ {
			p := filepath.Join(dir, "gen", fmt.Sprintf("f%05d.js", i))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("var x=1;\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		wantAckWarning(t, dir, false)
	})
}

// TestReadyAckAdviceIsActionable pins the message CONTENT. hasReadyAckWarning
// compares against the readyAckAdvice variable, which is self-referential —
// replacing the whole advisory with "something may be wrong" passes every other
// test in this file. AGENTS.md requires a message to name what to do, so assert
// the parts an author acts on.
func TestReadyAckAdviceIsActionable(t *testing.T) {
	required := []struct{ substr, why string }{
		{"page", "the author has to know WHICH manifest field put them here"},
		{readyAckType, "the message that is missing"},
		{"civitai app init", "the concrete command that produces a correct emitter"},
		{blockproto.ReadyAckFilename, "the file to copy in"},
		{"index.html", "where the emitter has to be referenced from"},
		{"not enough on its own", "copying the file was the ONE step the old wording let an author stop at"},
		{"@civitai/blocks-react", "the alternative to hand-writing it"},
		{"never both", "adopting the SDK without deleting the emitter is strictly worse"},
		{"#206", "the issue an author can read for the full story"},
		{"--strict", "so nobody thinks this blocks them today"},
	}
	if len(readyAckAdvisories) != 3 {
		t.Fatalf("readyAckAdvisories has %d entries, want 3 — a tier added without a case here ships "+
			"unasserted prose", len(readyAckAdvisories))
	}
	for _, advice := range readyAckAdvisories {
		for _, r := range required {
			if !strings.Contains(advice, r.substr) {
				t.Errorf("an advisory does not mention %q — %s\ngot: %s", r.substr, r.why, advice)
			}
		}
		// A message that says nothing useful is short. This is a crude floor, but
		// it is the one thing a "something may be wrong" rewrite cannot satisfy.
		if len(advice) < 400 {
			t.Errorf("an advisory is %d chars — too short to carry a diagnosis and a remedy: %s", len(advice), advice)
		}
	}
}

// TestReadyAckAdvisoriesStateTheirOwnStrength pins the disclosure that the
// dogfood defect turned on.
//
// 🔴 The presence-only tier CANNOT see wiring, and it is the tier that fires at
// the project shapes this CLI models worst. A message that reads identically to
// the reachability tier tells an author their app is checked when it is not —
// which is how "copy the emitter in" became a green check for a broken app. So
// each tier must name what it did, and the weak one must name what it did not.
// 🔴 AN EARLIER VERSION OF THIS TEST WAS HALF VACUOUS, and an audit mutant
// proved it. It asserted the two strong advisories contain "index.html loads" —
// but that phrase comes from the SHARED `readyAckRemedy` fragment, which EVERY
// advisory carries, including the weak one. So replacing the unwired advisory's
// entire headline with "but something is off" PASSED. That is the spelled-guard
// class: a check satisfied by a different feature's text.
//
// Every literal below appears in exactly ONE tier's own diagnosis, and each is
// paired with an INVERSE assertion that no other tier carries it — so a rewrite
// that blurs two tiers together fails in both directions.
func TestReadyAckAdvisoriesStateTheirOwnStrength(t *testing.T) {
	const disclosure = "did NOT check that the file is loaded"
	// The operative half of the weak tier's disclosure: without this sentence an
	// author reads "add the emitter", stops, and stays broken — which is the
	// defect. Deleting it was a surviving mutant.
	const operative = "will silence this warning"

	perTier := []struct {
		name   string
		advice string
		// own are literals this tier's DIAGNOSIS must carry, and which no other
		// tier may carry.
		own []string
	}{
		{"unwired", readyAckAdviceUnwired, []string{
			"DOES contain",                        // it found the message…
			"nothing index.html loads reaches it", // …and nothing loads it
			"orphan",                              // named for what it is
		}},
		{"missing", readyAckAdviceMissing, []string{
			"nothing index.html loads posts it either", // resolved, and absent
		}},
		{"presence-only", readyAckAdvicePresenceOnly, []string{
			disclosure,
			operative,
			"could not resolve the files your",
		}},
	}

	// 🔴 THE SHARED OPENING DIAGNOSIS, and the reason this test needed a second
	// round. `own` literals can only pin a tier that has something UNIQUE to
	// say — and the presence tier's three unique literals ALL live in its
	// trailing disclosure, so replacing its OPENING ("…but nothing in this
	// project's source posts BLOCK_READY") with vague prose survived everything
	// below. That is the identical half-vacuous shape this test's own header
	// records catching last round, one tier over.
	//
	// The sentence is shared with the `missing` tier by design — both report an
	// absence — so it is asserted as SET MEMBERSHIP: present in exactly those
	// two, absent from the tier that fires because the message IS present.
	const foundNothing = "nothing in this project's source posts "
	for _, tier := range perTier {
		wantShared := tier.name != "unwired"
		if got := strings.Contains(tier.advice, foundNothing); got != wantShared {
			if wantShared {
				t.Errorf("the %s advisory no longer opens by saying WHAT WAS FOUND (%q) — every other literal "+
					"this test pins for it lives in the trailing disclosure, so the diagnosis itself was "+
					"unguarded:\n%s", tier.name, foundNothing, tier.advice)
			} else {
				t.Errorf("the unwired advisory says nothing in the source posts the message, but that tier "+
					"fires precisely because something DOES:\n%s", tier.advice)
			}
		}
	}

	for _, tier := range perTier {
		for _, lit := range tier.own {
			if !strings.Contains(tier.advice, lit) {
				t.Errorf("the %s advisory no longer says %q — that literal is this tier's whole diagnosis, "+
					"and without it the message does not tell an author WHAT WAS FOUND:\n%s",
					tier.name, lit, tier.advice)
			}
			// The inverse control: no OTHER tier may claim it. This is what
			// makes the assertion above a statement about THIS tier rather than
			// about the shared remedy fragment every advisory carries.
			for _, other := range perTier {
				if other.name == tier.name {
					continue
				}
				if strings.Contains(other.advice, lit) {
					t.Errorf("the %s advisory carries %q, which belongs to the %s tier — the tiers are no "+
						"longer distinguishable by their text, so a reader cannot tell which check ran:\n%s",
						other.name, lit, tier.name, other.advice)
				}
			}
		}
	}
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
		for _, advice := range readyAckAdvisories {
			if e == advice {
				t.Fatalf("a ready-ack advisory reached Errors — it must stay a warning")
			}
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
	if got := scanForReadyAck(dest); got != ackAbsent {
		t.Fatalf("scanForReadyAck(page-money) = %v, want ackAbsent — the dep gate is no longer the reason "+
			"it is accepted, so nothing in this suite covers that branch any more", got)
	}
	if got := sdkDependency(dest); got != sdkPresent {
		t.Fatalf("sdkDependency(page-money) = %v, want sdkPresent — the rendered package.json must depend "+
			"on one of %v", got, blockproto.AckingPackages())
	}
	wantAckWarning(t, dest, false)
}

// TestSDKTemplateDepsAreNotJustAnyCivitaiPackage pins that page-money is
// accepted because of the package that ACKS, not merely because it has
// `@civitai/*` dependencies. It depends on BOTH `@civitai/app-sdk` (which does
// not ack) and `@civitai/blocks-react` (which does), so a scope test and an
// exact test agree on this template — which is exactly why the template alone
// could never have caught the widening.
func TestSDKTemplateDepsAreNotJustAnyCivitaiPackage(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "page-money")
	if _, err := scaffold.Render(scaffold.PageMoney, dest, scaffold.Data{Slug: "ack-block", Name: "Ack Block"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dest, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	acking := 0
	for _, name := range blockproto.AckingPackages() {
		if strings.Contains(string(raw), `"`+name+`"`) {
			acking++
		}
	}
	if acking == 0 {
		t.Fatalf("page-money depends on no acking package (%v) — it must, or the SDK branch of the check "+
			"has no real-template coverage at all", blockproto.AckingPackages())
	}
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
			for _, advice := range readyAckAdvisories {
				if !strings.Contains(advice, blockproto.ReadyAckFilename) {
					t.Errorf("an advisory does not name %s — an author cannot act on it", blockproto.ReadyAckFilename)
				}
			}
		})
	}
}
