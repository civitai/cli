package blockproto

import (
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
			g := ResolveEntryGraph(writeTree(t, c.files), EntryGraphOptions{Dependencies: c.deps})
			if g.Complete != c.wantComplete {
				t.Fatalf("Complete = %v, want %v (gaps: %v)\ntrace:\n  %s",
					g.Complete, c.wantComplete, g.Gaps, strings.Join(g.Trace, "\n  "))
			}
			if !c.wantComplete && !strings.Contains(strings.Join(g.Gaps, "\n"), c.wantGap) {
				t.Fatalf("gap reason does not mention %q — got %v\n"+
					"a gap recorded by a different branch leaves the one this case pins untested", c.wantGap, g.Gaps)
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
		// gap is the ENTRY being unreadable, not the root.
		g := ResolveEntryGraph(dir, EntryGraphOptions{MaxFileBytes: 64})
		if g.Complete {
			t.Fatal("a file the resolver refused to read was reported as a complete graph")
		}
		if !strings.Contains(strings.Join(g.Gaps, "\n"), "could not read") {
			t.Fatalf("gaps do not name the unread file: %v", g.Gaps)
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

// TestEntryGraphCommentsAreStripped pins that the graph's Code carries no
// comments — the check built on it asks whether a file MENTIONS a token, and a
// comment naming it is not an implementation of it.
func TestEntryGraphCommentsAreStripped(t *testing.T) {
	dir := writeTree(t, []gfile{
		{"index.html", "<!-- BLOCK_READY lives in app.js -->\n<script src=\"./app.js\"></script>"},
		{"app.js", "// TODO: post BLOCK_READY\nvar marker = 'KEPT_IN_A_STRING';\n"},
	})
	g := ResolveEntryGraph(dir, EntryGraphOptions{})
	for _, f := range g.Files {
		if strings.Contains(f.Code, "BLOCK_READY") {
			t.Fatalf("%s kept a commented-out mention: %q", f.Rel, f.Code)
		}
	}
	i := g.FileAt(filepath.Join(dir, "app.js"))
	if i < 0 || !strings.Contains(g.Files[i].Code, "KEPT_IN_A_STRING") {
		t.Fatal("the strip ate code outside a comment — string literals must survive")
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
