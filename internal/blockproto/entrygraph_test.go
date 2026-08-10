package blockproto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type gfile struct{ path, body string }

func writeTree(t *testing.T, files []gfile) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// ---------------------------------------------------------------------------
// The wiring predicate's control pair. MOVED here from
// internal/scaffold/ready_ack_contract_test.go along with the predicate itself:
// `internal/validate` needs the same resolution for an author's project, and a
// predicate open-coded at two sites is wrong at one of them.
//
// It exists because the first version of this check matched a BASENAME, which
// accepted every "wrong directory" shape. Each reject case below is a shape that
// check accepted while shipping a broken app; the accept cases are the two real
// templates' shapes. Ask of any future edit: can it pass while the emitter is
// somewhere the browser will not find it?
// ---------------------------------------------------------------------------

func TestReadyAckWiringPredicate(t *testing.T) {
	const ackRel = "src/civitai-host.js"

	cases := []struct {
		name       string
		ack        string // where the emitter really is
		files      []gfile
		wantAccept bool
		// wantErr, when set, must appear in the rejection message. It pins WHICH
		// branch rejected — without it a case can be green because some other
		// check happened to reject first.
		wantErr string
	}{
		{
			name: "accept: index.html loads it directly (the static shape)",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="./civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			name: "accept: the entry module imports it (the page-vite shape)",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host.js';\nimport React from 'react';"},
			},
			wantAccept: true,
		},
		{
			// NEW: an inline module in index.html executes exactly like an
			// external one. Without reading inline blocks the graph would report
			// itself COMPLETE while missing the emitter, which is a false finding
			// at a correct project.
			name: "accept: an inline module in index.html imports it",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", "<script type=\"module\">\nimport './civitai-host.js';\n</script>"},
			},
			wantAccept: true,
		},
		{
			// NEW: extensionless specifiers are what a bundler resolves. Pins the
			// extension-candidate branch.
			name: "accept: an extensionless import resolves to the emitter",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host';"},
			},
			wantAccept: true,
		},
		{
			name: "reject: right basename, wrong directory in the script tag",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="./vendor/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
		},
		{
			name: "reject: right basename, unresolvable relative import",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import '../nonexistent/civitai-host.js';"},
			},
		},
		{
			name: "reject: a BARE specifier that would resolve to a package",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import 'civitai-host.js';"},
			},
		},
		{
			name: "reject: referenced only from an HTML comment",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", "<!-- <script src=\"./civitai-host.js\"></script> -->\n<script src=\"./app.js\"></script>"},
				{"app.js", "// app"},
			},
		},
		{
			name: "reject: the import is commented out but still greppable",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "// import './civitai-host.js';\nimport React from 'react';"},
			},
		},
		{
			name: "reject: imported by a file nothing loads",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import React from 'react';"},
				{"src/orphan.js", "import './civitai-host.js';"},
			},
		},
		{
			// Pins the ErrNoScriptTags EARLY RETURN specifically. Without the
			// wantErr the case passes either way — the fallthrough at the end
			// also returns an error — so deleting that early return would go
			// unnoticed.
			name:    "reject: no script tags at all",
			ack:     "civitai-host.js",
			files:   []gfile{{"index.html", "<h1>hi</h1>"}},
			wantErr: "no <script src> at all",
		},
		{
			// Rejected by the FALLTHROUGH (an absolute URL is not a relative or
			// root-relative path and names no declared dependency), not by a
			// URL-specific guard — stated so the case doesn't look like it
			// exercises one.
			name: "reject: loaded from a CDN URL that merely ends in the name",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="https://cdn.example.com/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "not a path in this project",
		},
		{
			// 🔴 Pins the `//` half of the URL guard, which IS load-bearing:
			// without it the leading slash is stripped, filepath.Join cleans the
			// `..`, and this resolves to EXACTLY the emitter's path — an
			// off-project URL would be accepted as the emitter.
			name: "reject: protocol-relative URL that cleans back to the emitter path",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="//evil.example.com/../civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "not a path in this project",
		},
		{
			// Pins the `?`/`#` suffix stripping: Vite's `?url` / `?raw` suffixes
			// are not part of the path, and the emitter is still the emitter.
			name: "accept: entry module imports it with a Vite ?url suffix",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host.js?url';"},
			},
			wantAccept: true,
		},
		{
			// 🔴 THE HEADLINE DEFECT'S SECOND LIFE. `<script src="app.js">` — no
			// `./` — is entirely ordinary HTML and names the same file. Running
			// it through MODULE rules made it a "bare specifier", so it resolved
			// to nothing, the graph went incomplete, and validate fell to the
			// presence tier: the shipped false pass, intact, one character away.
			// Measured before the fix.
			name: "accept: a document-relative src with no leading ./",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="civitai-host.js"></script><script src="app.js"></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			// 🔴 UNQUOTED ATTRIBUTE VALUES ARE LEGAL HTML. The src regex required
			// quotes, so the tag was dropped, then re-classified as an INLINE
			// script with an empty body — the reference vanished with `Complete`
			// still true, and a correct `static` scaffold got the strong tier's
			// warning plus --strict rc=1. A false warning at a correct project.
			name: "accept: an unquoted src attribute",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src=./civitai-host.js></script><script src=app.js></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			name: "accept: a single-quoted src attribute",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src='./civitai-host.js'></script>`},
			},
			wantAccept: true,
		},
		{
			// 🔴 A URL IS FETCHED LITERALLY. Extension guessing belongs to a
			// bundler resolving a MODULE specifier, never to an HTML src: on the
			// no-build `static` template the browser asks for `/civitai-host`
			// and gets a 404. This was REJECTED at e800129 and accepted by the
			// first version of this resolver — the exact "ships a 404" shape
			// wiring.go claims to reject.
			name: "reject: an extensionless src is a 404, not an extension to guess",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="./civitai-host"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// The counterpart: extension resolution IS correct for a module
			// specifier, because a bundler does exactly that. Without this pair
			// "no guessing" and "guess everywhere" are indistinguishable.
			name: "accept: an extensionless MODULE import still resolves",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host';"},
			},
			wantAccept: true,
		},
		{
			// A scheme-qualified src is a URL even without `//`. Under URL rules
			// the default branch is document-relative, so without the scheme
			// check `foo:bar` would resolve to a path inside the project.
			name: "reject: a scheme-qualified src is a URL, not a relative path",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="data:text/javascript,0"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "not a path in this project",
		},
		{
			// 🔴 `\b` IS NOT AN HTML ATTRIBUTE-NAME BOUNDARY. It matches between
			// `-` and `s`, so `data-src=` was read as `src=` and the WRONG
			// reference resolved — with `Complete` still true, so the STRONG
			// tier fired. Measured on a correct `static` scaffold: `--strict`
			// rc=1. `data-src` on a `<script>` is a real consent-manager /
			// lazy-load pattern, not a contrivance.
			name: "accept: data-src before the real src does not hijack it",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script data-src="./app.js" src="./civitai-host.js"></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			name: "accept: x-src before the real src does not hijack it",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script x-src="./app.js" src="./civitai-host.js"></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			// The inverse control: a tag carrying ONLY `data-src` has no src at
			// all, so it loads nothing and cannot satisfy the wiring check.
			// Without this, "do not match data-src" is indistinguishable from
			// "match any attribute whose name ends in src".
			name: "reject: data-src alone is not a load",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script data-src="./civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// 🔴 The same `\b` on the TAG name: `<script-loader>` is a custom
			// element, not a script, and a browser executes nothing in it.
			name: "reject: a <script-loader> custom element is not a script tag",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script-loader src="./civitai-host.js"></script-loader><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// 🟡 F2: `</script >` — trailing whitespace before the `>` — is a
			// valid close tag and a browser accepts it. This commit changed the
			// close pattern to `</script\s*>` and shipped it unpinned; without
			// the `\s*` the block never matches, so the inline module's imports
			// are never read and a correctly wired project is rejected.
			name: "accept: an inline module closed by `</script >`",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", "<script type=\"module\">import './civitai-host.js';</script >"},
			},
			wantAccept: true,
		},
		{
			// 🟡 F2, the other direction: `</scriptx>` closes NOTHING — a browser
			// keeps parsing the script until a real `</script>`, so code AFTER
			// the bogus tag still executes. Widening the close pattern to
			// `</script[^>]*>` ends the block early and loses exactly that code.
			// (My first draft of this case asserted the opposite and was simply
			// wrong about HTML: it is an ACCEPT, and the accept is what
			// discriminates the widening.)
			name: "accept: an inline module continues past a bogus `</scriptx>`",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", "<script type=\"module\">var x = 1;</scriptx>\nimport './civitai-host.js';</script>"},
			},
			wantAccept: true,
		},
		{
			// 🟡 F2: the TAG boundary again, this time for the inline path. A
			// `<script-loader>` custom element's body is never executed, so an
			// import inside it is not a load. Reverting the tag boundary makes
			// this element an inline script and accepts it.
			name: "reject: an import inside a <script-loader> body is not a load",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", "<script-loader>import './civitai-host.js';</script-loader>\n<script src=\"./app.js\"></script>"},
				{"app.js", "// app"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// 🟢 `srcAttrValue` reports an EMPTY src as PRESENT, which makes the
			// tag EXTERNAL rather than inline. A browser ignores the body of an
			// element that has a `src` attribute, so the import below must NOT
			// count. Flipping that `return "", true` reproduces verbatim the
			// hazard its own comment documents.
			name: "reject: a body import inside a tag that has an (empty) src",
			ack:  ackRel,
			files: []gfile{
				{"index.html", "<script src=\"\">import './src/civitai-host.js';</script>\n<script src=\"./app.js\"></script>"},
				{"app.js", "// app"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// 🟢 The scheme class must accept a SINGLE-letter and an UPPERCASE
			// scheme, or an off-project URL resolves as a project file.
			name: "reject: a single-letter scheme is still a URL",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="c:/elsewhere/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "not a path in this project",
		},
		{
			name: "reject: an uppercase scheme is still a URL",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="HTTPS://cdn.example.com/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantErr: "not a path in this project",
		},
		{
			// 🔴 The scheme regex is ANCHORED, and THE ANCHOR is what is
			// load-bearing here: an unanchored pattern matches a `https:`
			// sitting in a QUERY STRING and throws away a perfectly ordinary
			// local reference. Dropping the `^` survived the previous sweep.
			// (An earlier comment credited the ORDER of the scheme check and
			// the `?`/`#` strip. It does not: two independent sweeps swapped
			// them and nothing failed, because a scheme-qualified specifier
			// stays scheme-qualified once its query is removed.)
			name: "accept: a local src whose query string contains a URL",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script src="./civitai-host.js?from=https://cdn.example.com"></script>`},
			},
			wantAccept: true,
		},
		{
			// The `../` arm of the module resolver, which nothing pinned: an
			// emitter one directory ABOVE the importing module is still inside
			// the project and must resolve.
			name: "accept: the entry module imports the emitter from a parent directory",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import '../civitai-host.js';"},
			},
			wantAccept: true,
		},
		{
			// Dynamic import with a literal specifier is a real load.
			name: "accept: a dynamic import() with a literal specifier",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "if (x) { import('./civitai-host.js'); }"},
			},
			wantAccept: true,
		},
		{
			// NEW: pins readyAckWiringDepth. Found by a SURVIVING mutant —
			// widening Guard A's bound from 2 to 99 passed the entire suite, so
			// nothing was holding the "one level of imports" contract at all. A
			// TEMPLATE's emitter is meant to be loaded by the entry itself;
			// burying it behind a helper module is a template smell, and this is
			// the only case that can tell the two bounds apart. (`validate` uses
			// the deeper default for an author's project — see
			// TestEntryGraphBudgetsAreGaps for that bound.)
			name: "reject: the emitter is two import levels below the entry",
			ack:  ackRel,
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './helper.js';"},
				{"src/helper.js", "import './civitai-host.js';"},
			},
			wantErr: "no <script src> in index.html resolves to it",
		},
		{
			// NEW: pins the ROOT CONTAINMENT guard. Without it the `..` climbs out
			// of the project and — if a same-named file happens to sit beside it —
			// a sibling checkout's emitter would satisfy this project's wiring.
			name: "reject: an import that escapes the project root",
			ack:  "civitai-host.js",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import '../../civitai-host.js';"},
			},
			wantErr: "not a path in this project",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The emitter itself always exists where `ack` says — so a rejection
			// can only come from the REFERENCE, never from a missing file.
			dir := writeTree(t, append([]gfile{{c.ack, "// emitter"}}, c.files...))
			chain, err := ReadyAckWiring(dir, c.ack)
			if c.wantAccept {
				if err != nil {
					t.Fatalf("wiring check REJECTED a correctly wired project: %v", err)
				}
				t.Logf("accepted: %s", chain)
				return
			}
			if err == nil {
				t.Fatalf("wiring check ACCEPTED a project the browser cannot load the emitter in: %q\n"+
					"this is the basename-matching class — the reference must RESOLVE to the emitter's real path", chain)
			}
			if c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("rejected for the wrong reason — want an error mentioning %q, got:\n%v\n"+
					"a rejection from a different branch would leave the one this case exists to pin untested",
					c.wantErr, err)
			}
			t.Logf("rejected: %v", err)
		})
	}
}

// ---------------------------------------------------------------------------
// Completeness — the half `internal/validate` branches on.
//
// 🔴 These are the cases where the graph must admit it did not see everything.
// A caller reads `Complete == false` as "this says nothing", so a bug that
// reports a partial graph as complete converts every one of these into a
// confident false warning at a correct project.
// ---------------------------------------------------------------------------

func TestEntryGraphCompleteness(t *testing.T) {
	deps := map[string]bool{"react": true, "react-dom": true, "@civitai/blocks-react": true}

	cases := []struct {
		name         string
		files        []gfile
		deps         map[string]bool
		opts         *EntryGraphOptions
		wantComplete bool
		wantFiles    []string // Rel paths that must be in the graph
		wantGap      string   // substring of the reason, when incomplete
	}{
		{
			name: "the page-vite shape resolves completely",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host.js';\nimport React from 'react';\n" +
					"import { createRoot } from 'react-dom/client';\nimport App from './App.jsx';\nimport './index.css';"},
				{"src/civitai-host.js", "// emitter"},
				{"src/App.jsx", "import React from 'react';"},
				{"src/index.css", "body{}"},
			},
			deps:         deps,
			wantComplete: true,
			wantFiles:    []string{"index.html", "src/main.jsx", "src/civitai-host.js", "src/App.jsx", "src/index.css"},
		},
		{
			// 🔴 THE ALIAS CASE. A bare specifier that is not a declared
			// dependency is a real file behind a bundler alias we cannot follow.
			// Reporting this graph complete would warn at a correct project.
			name: "a bundler alias makes the graph incomplete",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import '@/civitai-host.js';"},
				{"src/civitai-host.js", "// emitter"},
			},
			deps:         deps,
			wantComplete: false,
			wantGap:      "bundler alias",
		},
		{
			// The positive control for the case above: the SAME shape with the
			// specifier declared as a dependency is accounted for, so "a bare
			// specifier is a gap" cannot be satisfied by a resolver that treats
			// every bare specifier as one.
			name: "a declared dependency is accounted for, not a gap",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import 'react';\nimport 'react-dom/client';\nimport '@civitai/blocks-react';"},
			},
			deps:         deps,
			wantComplete: true,
			wantFiles:    []string{"index.html", "src/main.jsx"},
		},
		{
			name: "no index.html at the project root is a gap, not an empty answer",
			files: []gfile{
				{"src/main.jsx", "import './civitai-host.js';"},
			},
			deps:         deps,
			wantComplete: false,
			wantGap:      "index.html",
		},
		{
			// A reference that points at nothing means the resolver's model
			// disagrees with a project that presumably builds. That is a gap, not
			// evidence the browser 404s it.
			name: "a reference to a file that is not there is a gap",
			files: []gfile{
				{"index.html", `<script src="./civitai-host.js"></script>`},
			},
			deps:         deps,
			wantComplete: false,
			wantGap:      "does not exist",
		},
		{
			name: "an off-project URL is a gap",
			files: []gfile{
				{"index.html", `<script src="https://cdn.example.com/host.js"></script>`},
			},
			deps:         deps,
			wantComplete: false,
			wantGap:      "could not resolve",
		},
		{
			// 🔴 A dependency name is matched at a PATH SEPARATOR, never at a
			// character offset. `reactive-ui` starts with `react` and is a
			// different package; accepting it would classify a real project file
			// as a dependency, turning a gap into a confident wrong finding. The
			// audit's finer-grained prefix mutant survived the old corpus.
			name: "a bare specifier that merely starts like a dependency is still a gap",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import 'reactive-ui';"},
			},
			deps:         deps,
			wantComplete: false,
			wantGap:      "bundler alias",
		},
		{
			// The positive control for the boundary: a real SUBPATH of a
			// declared dependency is accounted for.
			name: "a subpath of a declared dependency is accounted for",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import 'react-dom/client';\nimport '@civitai/blocks-react/dist/x.js';"},
			},
			deps:         deps,
			wantComplete: true,
			wantFiles:    []string{"index.html", "src/main.jsx"},
		},
		{
			// 🔴 TRUNCATION IS A GAP. Before this, `continue`-ing at MaxDepth
			// left Complete TRUE, so a CORRECT project whose ack sat below the
			// bound got the strong tier's warning and --strict rc=1 — a finding
			// built on a graph we had stopped walking. Measured at depth 10.
			name: "an import below MaxDepth is a gap, not an absence",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';"},
				{"b.js", "import './c.js';"},
				{"c.js", "// end"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: false,
			wantGap:      "stopped at import depth 2",
		},
		{
			// 🔴 A DIAMOND AT THE BOUND IS NOT TRUNCATION, AND CALLING IT ONE
			// SILENCED THE DEFECT THIS CHECK EXISTS FOR. The deepest module
			// re-imports a file the walk ALREADY READ, so nothing is unseen —
			// but the first version gapped anyway, with a message that was
			// literally false about a file sitting in g.Files, and dropped the
			// project to the presence tier. Measured end-to-end: an orphaned
			// emitter went from `unwired`/rc=1 to SILENT/rc=0. A diamond is the
			// normal shape of any real module graph, so this is not an edge
			// case. A gap that over-fires is the false pass wearing a hat.
			name: "a diamond at the depth bound is not truncation",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './shared.js';\nimport './b.js';"},
				{"b.js", "import './shared.js';"},
				{"shared.js", "export const s = 1;"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "a.js", "shared.js", "b.js"},
		},
		{
			// The same rule for a self-cycle: `b.js` importing itself at the
			// bound leaves nothing unseen either.
			name: "a self-cycle at the depth bound is not truncation",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';"},
				{"b.js", "import './b.js';"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "a.js", "b.js"},
		},
		{
			// 🔴 F1: THE ANSWER MUST NOT DEPEND ON THE TEXTUAL ORDER OF TWO
			// IMPORT STATEMENTS. `g.Files` records files that have been POPPED,
			// so a target already ENQUEUED is absent from it — and evaluating
			// truncation mid-walk therefore gapped, or did not, according to
			// which of two same-depth imports was written first. Measured
			// end-to-end on real scaffolds, flat and nested: in the losing order
			// an orphaned emitter shipped SILENTLY. The check is now deferred to
			// the end of the walk. These two cases are the SAME GRAPH with the
			// two imports swapped, and they must agree.
			name: "the bound is order-independent (import A then B)",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';\nimport './sib.js';"},
				{"b.js", "import './sib.js';"},
				{"sib.js", "export const s = 1;"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "a.js", "b.js", "sib.js"},
		},
		{
			name: "the bound is order-independent (import B then A)",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './sib.js';\nimport './b.js';"},
				{"b.js", "import './sib.js';"},
				{"sib.js", "export const s = 1;"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "a.js", "sib.js", "b.js"},
		},
		{
			// 🟡 F3: truncatedBelow resolves against the IMPORTING MODULE's
			// directory, not the project root. Every other fixture here is
			// root-flat, where the two coincide — so only a NESTED one can tell
			// them apart. Resolving against the root would look for
			// `<root>/sib.js`, miss the file, and gap at a complete graph.
			name: "the bound resolves against the importing module's directory",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/deep/a.js"></script>`},
				{"src/deep/a.js", "import './b.js';\nimport './sib.js';"},
				{"src/deep/b.js", "import './sib.js';"},
				{"src/deep/sib.js", "export const s = 1;"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "src/deep/a.js", "src/deep/b.js", "src/deep/sib.js"},
		},
		{
			// 🟡 F3: the refUnresolved arm. A bundler alias at the bound is
			// something we genuinely cannot follow, so it MUST gap — otherwise
			// the strong tier fires on a graph with a hole in it.
			name: "a bundler alias at the depth bound is a gap",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';"},
				{"b.js", "import '@/civitai-host.js';"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: false,
			wantGap:      "stopped at import depth 2",
		},
		{
			// 🟡 F3: the resolveRefPath-error arm. A dangling import at the
			// bound is the same model disagreement the normal path gaps on.
			name: "a dangling import at the depth bound is a gap",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';"},
				{"b.js", "import './nope.js';"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: false,
			wantGap:      "stopped at import depth 2",
		},
		{
			// The counterpart: a leaf at the bound that imports only a declared
			// PACKAGE has nothing left to see, so it is NOT a gap. Without this
			// the rule above degenerates into "any project as deep as the bound
			// is unobservable".
			name: "a package import at the depth bound is not truncated content",
			files: []gfile{
				{"index.html", `<script type="module" src="/a.js"></script>`},
				{"a.js", "import './b.js';"},
				{"b.js", "import 'react';"},
			},
			deps:         deps,
			opts:         &EntryGraphOptions{MaxDepth: 2},
			wantComplete: true,
			wantFiles:    []string{"index.html", "a.js", "b.js"},
		},
		{
			// 🔴 `entryModuleExts` decides what can PULL files into the graph. A
			// browser never follows imports out of a stylesheet, so widening this
			// set would let a `.css` add reachable files — and silence the check
			// on the strength of something that is never executed.
			name: "a stylesheet's imports are not followed",
			files: []gfile{
				{"index.html", `<script type="module" src="/src/main.js"></script>`},
				{"src/main.js", "import './theme.css';"},
				{"src/theme.css", "/* x */ import './civitai-host.js';"},
				{"src/civitai-host.js", "// emitter"},
			},
			deps:         deps,
			wantComplete: true,
			wantFiles:    []string{"index.html", "src/main.js", "src/theme.css"},
		},
		{
			// An index.html with nothing to load is a COMPLETE graph of one file.
			// It is the shape a `validate` finding is allowed to rest on, so it
			// must not be confused with "we could not look".
			name:         "an index.html that loads nothing is complete",
			files:        []gfile{{"index.html", "<h1>hi</h1>"}},
			deps:         deps,
			wantComplete: true,
			wantFiles:    []string{"index.html"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := EntryGraphOptions{}
			if c.opts != nil {
				o = *c.opts
			}
			o.Dependencies = c.deps
			g := ResolveEntryGraph(writeTree(t, c.files), o)
			if g.Complete != c.wantComplete {
				t.Fatalf("Complete = %v, want %v (gaps: %v)\ntrace:\n  %s",
					g.Complete, c.wantComplete, g.Gaps, strings.Join(g.Trace, "\n  "))
			}
			if !c.wantComplete && !strings.Contains(strings.Join(g.Gaps, "\n"), c.wantGap) {
				t.Fatalf("gap reason does not mention %q — got %v\n"+
					"a gap recorded by a different branch leaves the one this case pins untested", c.wantGap, g.Gaps)
			}
			// 🔴 `Complete` AND `Gaps` MUST AGREE, over this whole corpus.
			// `internal/validate` relies on it in two places that CANNOT fail
			// on their own: the strong tiers append `Gaps` and are quiet only
			// because a complete graph has none, and the presence tier's
			// "reasons" would be empty for an incomplete graph that recorded
			// none. Both would go silently wrong here rather than there, so
			// this is where it is asserted. `gap` is the only writer of either
			// field; a second writer, or a `gap` that stopped clearing
			// Complete, breaks this row.
			if g.Complete != (len(g.Gaps) == 0) {
				t.Fatalf("Complete = %v but len(Gaps) = %d — the flag and the reasons disagree, so a caller "+
					"branching on Complete and a caller rendering Gaps see different graphs", g.Complete, len(g.Gaps))
			}
			if len(g.Gaps) != len(g.gapKinds) {
				t.Fatalf("Gaps (%d) and gapKinds (%d) are out of step — the parallel slices rankGaps sorts "+
					"together have drifted, so gaps would be reordered under the wrong ranks",
					len(g.Gaps), len(g.gapKinds))
			}
			for _, want := range c.wantFiles {
				if !graphHasRel(g, want) {
					t.Fatalf("graph is missing %q; it has %v", want, graphRels(g))
				}
			}
			if c.wantFiles != nil && len(g.Files) != len(c.wantFiles) {
				t.Fatalf("graph has %v, want exactly %v — an EXTRA file is as wrong as a missing one: "+
					"it means the walk reached something the browser does not load", graphRels(g), c.wantFiles)
			}
		})
	}
}

// TestEntryGraphBudgetsAreGaps pins that exhausting a budget is UNOBSERVABLE
// rather than "nothing more is there". Both are silent-by-default outcomes for
// callers, and reporting either as a complete graph manufactures a finding.
func TestEntryGraphBudgetsAreGaps(t *testing.T) {
	t.Run("a file over the size cap is a gap", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script src="./app.js"></script>`},
			{"app.js", strings.Repeat("x", 4096)},
		})
		if g := ResolveEntryGraph(dir, EntryGraphOptions{}); !g.Complete {
			t.Fatalf("positive control failed: a 4 KiB entry must resolve completely, gaps %v", g.Gaps)
		}
		// 64 bytes: index.html (32) still reads, app.js (4096) does not — so the
		// gap is the ENTRY being skipped, not the root.
		g := ResolveEntryGraph(dir, EntryGraphOptions{MaxFileBytes: 64})
		if g.Complete {
			t.Fatal("a file the resolver refused to read was reported as a complete graph")
		}
		joined := strings.Join(g.Gaps, "\n")
		if !strings.Contains(joined, "did not read") || !strings.Contains(joined, "app.js") {
			t.Fatalf("gaps do not name the skipped file: %v", g.Gaps)
		}
		// 🔴 THE SIZE CAP IS OUR OWN LIMIT, NOT A PROBLEM WITH THE FILE, and the
		// wording and the RANK both have to say so. It used to report
		// "could not read app.js … make it readable" — false advice about a
		// perfectly readable file — and it was ranked gapUnreadable, so a limit
		// WE impose outranked a genuine dangling reference in the capped
		// advisory. That is the ranking hazard gapKind exists to close, in the
		// one branch that had not been separated from it.
		if strings.Contains(joined, "make it readable") {
			t.Errorf("the size-cap gap gives advice about a file that is perfectly readable: %v", g.Gaps)
		}
		for i, k := range g.gapKinds {
			if strings.Contains(g.Gaps[i], "app.js") && k != gapBudget {
				t.Errorf("the size-cap gap is ranked %v, want gapBudget — a limit this check imposes must "+
					"never outrank a real defect in the author's project: %s", k, g.Gaps[i])
			}
		}
	})

	t.Run("the file budget is a gap", func(t *testing.T) {
		files := []gfile{{"index.html", `<script type="module" src="/a0.js"></script>`}}
		for i := 0; i < 5; i++ {
			files = append(files, gfile{
				path: "a" + string(rune('0'+i)) + ".js",
				body: "import './a" + string(rune('0'+i+1)) + ".js';",
			})
		}
		files = append(files, gfile{"a5.js", "// end"})
		dir := writeTree(t, files)
		if g := ResolveEntryGraph(dir, EntryGraphOptions{}); !g.Complete {
			t.Fatalf("positive control failed: a 7-file chain must resolve completely, gaps %v", g.Gaps)
		}
		g := ResolveEntryGraph(dir, EntryGraphOptions{MaxFiles: 3})
		if g.Complete {
			t.Fatal("a walk that stopped at its file budget was reported as a complete graph")
		}
		if !strings.Contains(strings.Join(g.Gaps, "\n"), "stopped after") {
			t.Fatalf("gaps do not name the budget: %v", g.Gaps)
		}
	})

	t.Run("the depth bound is a gap only when there is more to see", func(t *testing.T) {
		dir := writeTree(t, []gfile{
			{"index.html", `<script type="module" src="/a.js"></script>`},
			{"a.js", "import './b.js';"},
			{"b.js", "import './c.js';"},
			{"c.js", "// end"},
		})
		g := ResolveEntryGraph(dir, EntryGraphOptions{MaxDepth: 2})
		if graphHasRel(g, "c.js") {
			t.Fatal("MaxDepth=2 reached a depth-3 file — the bound does nothing")
		}
		if !graphHasRel(g, "b.js") {
			t.Fatalf("MaxDepth=2 did not reach the depth-2 file; graph is %v", graphRels(g))
		}
	})
}

// inspectAll collects every file the walk read, keyed by Rel, using the same
// Inspect callback production uses. There is no retained `Code` field to read —
// see TestEntryGraphRetainsNoContents.
func inspectAll(t *testing.T, dir string, opts EntryGraphOptions) (*EntryGraph, map[string]string) {
	t.Helper()
	seen := map[string]string{}
	opts.Inspect = func(f EntryFile, code string) { seen[f.Rel] = code }
	return ResolveEntryGraph(dir, opts), seen
}

// TestEntryGraphCommentsAreStripped pins that the code handed to Inspect carries
// no comments — the check built on it asks whether a file MENTIONS a token, and
// a comment naming it is not an implementation of it.
func TestEntryGraphCommentsAreStripped(t *testing.T) {
	dir := writeTree(t, []gfile{
		{"index.html", "<!-- BLOCK_READY lives in app.js -->\n<script src=\"./app.js\"></script>"},
		{"app.js", "// TODO: post BLOCK_READY\nvar marker = 'KEPT_IN_A_STRING';\n"},
	})
	_, code := inspectAll(t, dir, EntryGraphOptions{})
	for rel, c := range code {
		if strings.Contains(c, "BLOCK_READY") {
			t.Fatalf("%s kept a commented-out mention: %q", rel, c)
		}
	}
	if !strings.Contains(code["app.js"], "KEPT_IN_A_STRING") {
		t.Fatal("the strip ate code outside a comment — string literals must survive")
	}
}

// TestEntryGraphInspectSeesEveryFile is Inspect's POSITIVE CONTROL. Every
// assertion built on Inspect reports a NEGATIVE ("no file mentions the token"),
// and a zero from a callback wired to nothing is indistinguishable from a zero
// that means something. This asserts the callback fires once per graph file, in
// a shape where the count must be non-zero.
func TestEntryGraphInspectSeesEveryFile(t *testing.T) {
	dir := writeTree(t, []gfile{
		{"index.html", `<script type="module" src="/src/main.js"></script>`},
		{"src/main.js", "import './a.js';\nimport './style.css';"},
		{"src/a.js", "var a = 1;"},
		{"src/style.css", "body{}"},
	})
	g, code := inspectAll(t, dir, EntryGraphOptions{})
	if !g.Complete {
		t.Fatalf("fixture should resolve completely: %v", g.Gaps)
	}
	want := []string{"index.html", "src/main.js", "src/a.js", "src/style.css"}
	if len(code) != len(want) {
		t.Fatalf("Inspect fired for %v, want exactly %v", graphRels(g), want)
	}
	for _, rel := range want {
		if _, ok := code[rel]; !ok {
			t.Fatalf("Inspect never saw %s — a callback that misses files reports a silent false negative", rel)
		}
	}
}

// TestEntryGraphRetainsNoContents pins ONE HALF of the memory contract.
//
// It asserts no graph field holds a file's contents — the property whose absence
// let a 200-module graph inside both budgets peak 421-439 MB.
//
// 🔴 WHAT IT CANNOT SEE, STATED BECAUSE IT WAS ONCE CITED AS IF IT COULD: the
// ALIASING half. A Go substring shares its parent's backing array, so an
// un-cloned import specifier pins its whole file (measured: 558-628 MB with
// `Code` already gone). `strings.Contains` below reads the LOGICAL string, so it
// passes identically at 558 MB and at 33 MB. Only `strings.Clone` in the walk
// prevents that, and only a live RSS measurement on a fixture whose modules are
// large AND carry imports can observe it — see the table in
// EntryGraphOptions.Inspect. Do not read a green here as "memory is fine".
func TestEntryGraphRetainsNoContents(t *testing.T) {
	dir := writeTree(t, []gfile{
		{"index.html", `<script src="./app.js"></script>`},
		{"app.js", "var UNIQUE_TOKEN_NOT_IN_ANY_FIELD = 1;"},
	})
	g, code := inspectAll(t, dir, EntryGraphOptions{})
	if !strings.Contains(code["app.js"], "UNIQUE_TOKEN_NOT_IN_ANY_FIELD") {
		t.Fatal("positive control failed: Inspect did not receive the file's contents")
	}
	for _, f := range g.Files {
		for _, field := range []string{f.Path, f.Rel, f.Spec} {
			if strings.Contains(field, "UNIQUE_TOKEN_NOT_IN_ANY_FIELD") {
				t.Fatalf("%s retains file CONTENTS in a graph field (%q) — the graph must hold one file's "+
					"bytes at a time, like the tree scan", f.Rel, field)
			}
		}
	}
}

// TestEntryGraphRootContainment pins BOTH arms of the containment check.
//
// 🟢 Dropping only the `rel == ".."` arm admits a file OUTSIDE the project with
// `Complete` still true — the project's own emitter would then be "satisfied" by
// a same-named file in a sibling checkout. It is reachable through directory-
// index resolution: `import '..'` resolves to the parent DIRECTORY, and the
// module resolver will happily take `<parent>/index.js`.
func TestEntryGraphRootContainment(t *testing.T) {
	base := t.TempDir()
	// A file that exists OUTSIDE the project root, reachable only by escaping it.
	if err := os.WriteFile(filepath.Join(base, "index.js"), []byte("// outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proj")
	for _, f := range []gfile{
		{"index.html", `<script type="module" src="/src/main.js"></script>`},
		{"src/main.js", "import '../..';"},
	} {
		p := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Positive control: the escape target really is resolvable, so a green below
	// cannot mean "there was nothing to admit".
	if _, err := os.Stat(filepath.Join(base, "index.js")); err != nil {
		t.Fatalf("fixture missing its off-project target: %v", err)
	}
	g := ResolveEntryGraph(root, EntryGraphOptions{})
	for _, f := range g.Files {
		if !strings.HasPrefix(f.Path, root) {
			t.Fatalf("the graph admitted %s, which is OUTSIDE the project root %s — a same-named file in a "+
				"sibling checkout would satisfy this project's wiring check", f.Path, root)
		}
	}
	if g.Complete {
		t.Fatal("a reference escaping the project root must be a GAP: we stopped modelling there")
	}
}

func graphRels(g *EntryGraph) []string {
	out := make([]string, 0, len(g.Files))
	for _, f := range g.Files {
		out = append(out, f.Rel)
	}
	return out
}

func graphHasRel(g *EntryGraph, rel string) bool {
	for _, f := range g.Files {
		if f.Rel == rel {
			return true
		}
	}
	return false
}

// TestGapsAreRankedMostLikelyCauseFirst pins the ORDER of Gaps.
//
// 🔴 A CONSUMER THAT RENDERS ONLY THE FIRST FEW BURIES THE ACTUAL BUG WITHOUT
// IT. `internal/validate`'s advisory caps at three. Measured before ranking, on
// an index.html carrying three CDN `<script src>` tags above a dangling
// `./civitai-host.js`: the report showed the three off-project URLs and withheld
// the dangling reference, under a lead-in claiming the cause was among them.
//
// The ranking is a claim about LIKELIHOOD, not correctness — every gap is
// equally real and each still clears Complete. A CDN reference is expected in a
// working project; a local reference to a file that is not there is the #206
// population.
func TestGapsAreRankedMostLikelyCauseFirst(t *testing.T) {
	dir := writeTree(t, []gfile{
		// Document order is the LOSING order: the off-project references come
		// first and the dangling local one last.
		{"index.html", `<!doctype html>` +
			`<script src="https://cdn.example.com/a.js"></script>` +
			`<script src="//proto.example.com/b.js"></script>` +
			`<script type="module" src="./entry.js"></script>` +
			`<script src="./gone.js"></script>`},
		{"entry.js", "import '@alias/x.js';\nexport const e = 1;\n"},
	})
	g := ResolveEntryGraph(dir, EntryGraphOptions{})
	if g.Complete {
		t.Fatalf("fixture must be incomplete; gaps=%v", g.Gaps)
	}
	if len(g.Gaps) < 4 {
		t.Fatalf("fixture produced %d gaps, want >= 4 — it does not exercise ordering: %v", len(g.Gaps), g.Gaps)
	}
	// The dangling LOCAL reference must sort ahead of every unresolved one.
	dangling, firstUnresolved := -1, -1
	for i, s := range g.Gaps {
		if strings.Contains(s, `"./gone.js"`) && dangling < 0 {
			dangling = i
		}
		if strings.Contains(s, "could not resolve") && firstUnresolved < 0 {
			firstUnresolved = i
		}
	}
	if dangling < 0 || firstUnresolved < 0 {
		t.Fatalf("fixture did not produce both gap kinds (dangling=%d unresolved=%d): %v",
			dangling, firstUnresolved, g.Gaps)
	}
	if dangling > firstUnresolved {
		t.Errorf("a dangling local reference (index %d) sorts BELOW an off-project one (index %d); a "+
			"consumer that shows the first three would bury the actual bug:\n%v", dangling, firstUnresolved, g.Gaps)
	}
	// The kinds themselves must be non-decreasing — the sort really ran, rather
	// than the fixture happening to be in order.
	for i := 1; i < len(g.gapKinds); i++ {
		if g.gapKinds[i] < g.gapKinds[i-1] {
			t.Fatalf("gapKinds are not sorted at %d: %v (%v)", i, g.gapKinds, g.Gaps)
		}
	}
}

// TestGapRankingIsStableWithinAKind keeps the ranking from scrambling the order
// an author reads their own file in. Gaps of one kind must stay in document
// order — a sort that is merely "grouped" would make the first thing they see
// depend on nothing they can predict.
//
// 🔴 STABILITY IS LOAD-BEARING, NOT TIDINESS: it decides WHICH THREE gaps the
// capped advisory shows, so `TestTheActualCauseSurvivesTheCap`'s whole argument —
// that the withheld gaps are the least likely — rests on it.
//
// 🔴 AND THE FIRST VERSION OF THIS GUARD COULD NOT FAIL. It used a 2-gap
// fixture, and Go's `sort.Slice` switches to INSERTION SORT below n=13, which is
// stable by accident. Measured: `sort.SliceStable` → `sort.Slice` reddened **0
// subtests across the entire suite**. The fixture is now deliberately larger
// than that threshold — that is the only reason the mutant dies — and the
// constant is named so nobody "simplifies" the fixture back under it.
func TestGapRankingIsStableWithinAKind(t *testing.T) {
	// 🔴 TWO PROPERTIES THE FIXTURE MUST HAVE, AND MISSING EITHER MAKES THE
	// MUTANT SURVIVE. Both were measured, not reasoned about.
	//
	//  1. MORE THAN ~12 ELEMENTS. Go's `sort.Slice` runs insertion sort below
	//     that, which is stable by accident. The first version used 2, and the
	//     `sort.SliceStable` -> `sort.Slice` mutant reddened 0 subtests.
	//  2. MIXED KINDS. With one kind the comparator is all-equal, and pdqsort
	//     detects an all-equal partition and leaves it alone — so widening to 40
	//     of a single kind ALSO reddened 0. The sort has to actually move
	//     something before instability can show.
	//
	// So: 30 gaps alternating between two kinds. After ranking, the 15 dangling
	// ones come first and the 15 unresolved ones follow, and within each run the
	// author's document order must survive.
	const pairs = 15

	var html strings.Builder
	html.WriteString(`<!doctype html>`)
	for i := 0; i < pairs; i++ {
		// Zero-padded so lexical order matches document order — otherwise a
		// failure is ambiguous between "unstable" and "sorted by name".
		fmt.Fprintf(&html, `<script src="./gone-%03d.js"></script>`, 2*i)
		fmt.Fprintf(&html, `<script src="https://cdn.example.com/off-%03d.js"></script>`, 2*i+1)
	}
	dir := writeTree(t, []gfile{{"index.html", html.String()}})
	g := ResolveEntryGraph(dir, EntryGraphOptions{})

	if len(g.Gaps) < 2*pairs {
		t.Fatalf("fixture produced %d gaps, want >= %d — below Go's insertion-sort threshold an unstable "+
			"sort passes by accident and this test proves nothing", len(g.Gaps), 2*pairs)
	}
	// Positive control on the SHAPE: both kinds must really be present, or the
	// comparator is all-equal and pdqsort never moves anything — the second way
	// this guard measured green while testing nothing.
	kinds := map[gapKind]int{}
	for _, k := range g.gapKinds {
		kinds[k]++
	}
	if len(kinds) < 2 {
		t.Fatalf("the fixture produced only one gap kind (%v); with an all-equal comparator pdqsort leaves "+
			"the slice alone, so an unstable sort is indistinguishable from a stable one", kinds)
	}

	// Within each KIND, the numbers must ascend — i.e. document order survived.
	last := map[gapKind]int{}
	for i, s := range g.Gaps {
		n := gapNumberIn(t, s)
		k := g.gapKinds[i]
		if prev, seen := last[k]; seen && n <= prev {
			t.Fatalf("gaps of kind %v are out of document order (%d after %d) — the sort is not STABLE, so "+
				"WHICH THREE the capped advisory shows depends on nothing the author can predict:\n%v",
				k, n, prev, g.Gaps)
		}
		last[k] = n
	}
}

// gapNumberIn pulls the NNN out of a `gone-NNN.js` / `off-NNN.js` token, so the
// assertion does not depend on the gap's wording.
func gapNumberIn(t *testing.T, gap string) int {
	t.Helper()
	for _, tok := range strings.FieldsFunc(gap, func(r rune) bool {
		return r == ' ' || r == '"' || r == ',' || r == '/'
	}) {
		var n int
		if _, err := fmt.Sscanf(tok, "gone-%03d.js", &n); err == nil {
			return n
		}
		if _, err := fmt.Sscanf(tok, "off-%03d.js", &n); err == nil {
			return n
		}
	}
	t.Fatalf("could not read a file number out of gap %q", gap)
	return -1
}

// TestGapsCarryNoAbsolutePaths is the general form of the leak that shipped.
//
// 🔴 ONE OF SEVEN GAP SITES INTERPOLATED A RAW ERROR instead of going through
// relTo, so it emitted `stat /abs/.../index.html: no such file or directory`.
// Now that gaps are printed to authors that is machine-specific noise AND a
// single unbreakable token — measured at 120 runes on a deep fixture path,
// producing a 136-rune line under a 79-rune budget, which no greedy wrap can
// split. The check is over the ROOT rather than one message, so a new site
// cannot reintroduce it.
func TestGapsCarryNoAbsolutePaths(t *testing.T) {
	trees := map[string][]gfile{
		"no index.html": {{"package.json", `{}`}},
		"dangling ref":  {{"index.html", `<!doctype html><script src="./gone.js"></script>`}},
		"over the size cap (a BUDGET, not a defect)": {
			{"index.html", `<!doctype html><script type="module" src="./e.js"></script>`},
			{"e.js", "import './big.css';\n"},
			{"big.css", strings.Repeat("/*pad*/\n", 300)},
		},
		// 🔴 THE ONLY ROW THAT REACHES readableErr. The size-cap row above
		// returns a plain fmt.Errorf, NOT a *fs.PathError, so it never enters
		// the branch this test exists to guard — it was labelled
		// "unreadable/oversize" and tested the wrong half. A chmod-000 file is
		// the shape that produces a *fs.PathError carrying an absolute path.
		"genuinely unreadable": {
			{"index.html", `<!doctype html><script type="module" src="./locked.js"></script>`},
			{"locked.js", "export const x = 1;\n"},
		},
		"off-project": {{"index.html", `<!doctype html><script src="../../outside.js"></script>`}},
	}
	checked := 0
	for name, files := range trees {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, files)
			opts := EntryGraphOptions{}
			switch name {
			case "over the size cap (a BUDGET, not a defect)":
				opts.MaxFileBytes = 64 // force the read to be DECLINED on big.css
			case "genuinely unreadable":
				if os.Geteuid() == 0 {
					t.Skip("running as root: mode 0000 does not deny, so this row cannot fail a read")
				}
				locked := filepath.Join(dir, "locked.js")
				if err := os.Chmod(locked, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
				// Positive control on the FIXTURE: an unreadable row that is
				// still readable proves nothing, which is how the size-cap row
				// passed while exercising a different branch entirely.
				if _, err := os.ReadFile(locked); err == nil {
					t.Skip("this filesystem ignores mode 0000")
				}
			}
			g := ResolveEntryGraph(dir, opts)
			if g.Complete {
				t.Fatalf("fixture is complete, so it records no gap to inspect")
			}
			for _, gap := range g.Gaps {
				if strings.Contains(gap, dir) {
					t.Errorf("gap leaks the absolute project root:\n%s", gap)
				}
				for _, tok := range strings.Fields(gap) {
					if n := len([]rune(tok)); n > 60 {
						t.Errorf("gap carries a %d-rune unbreakable token, which sets the printed line width "+
							"on its own: %q", n, tok)
					}
				}
			}
			checked += len(g.Gaps)
		})
	}
	// POSITIVE CONTROL: a zero here is indistinguishable from four fixtures that
	// produced no gaps at all, in which case nothing above was inspected.
	if checked < 4 {
		t.Fatalf("only %d gap(s) were inspected across four fixtures — the corpus is not exercising the sites", checked)
	}
}
