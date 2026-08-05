package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/blockproto"
)

// ---------------------------------------------------------------------------
// GUARD A — the offline half of the ready-ack contract. Runs in `make ci`.
//
// A scaffolded page app that never posts `BLOCK_READY` is invisible: the host
// runs out its bounded init retries and replaces the app with a failure card
// (~37s), and NOTHING local reproduces that — the app renders perfectly when
// you open it yourself. That is how issue #206 shipped in the first place.
//
// WHAT THIS GUARD PROVES
//   - every template that declares a `page` surface and does NOT carry the
//     `@civitai/*` SDK ships a copy of the canonical emitter that is
//     BYTE-IDENTICAL to blockproto.ReadyAckSource();
//   - that copy is REACHED from the template's real entry point — the rendered
//     index.html loads it directly, or loads a module that imports it — checked
//     after stripping comments, so a comment mentioning the filename does not
//     satisfy it;
//   - the canonical emitter still has the shape the host contract requires
//     (window message listener, gated on BLOCK_INIT, `{type,payload}` envelope,
//     addressed at `event.origin` and never `'*'`).
//
// WHAT IT DOES NOT PROVE
// That the ack ever FIRES. No Go static check can prove JavaScript executes.
// An emitter whose source guard is inverted, or whose listener is registered
// too late, passes every assertion here. That is the entire reason Guard B
// (ready_ack_runtime_test.go) exists — it loads the emitter in node, drives it
// with a fake host, and watches for the outbound message.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// The shape predicate + its negative corpus.
// ---------------------------------------------------------------------------

// readyAckRule is one structural requirement on the emitter source. `want`
// false inverts it: the pattern must NOT appear.
type readyAckRule struct {
	name string
	re   *regexp.Regexp
	want bool
	why  string
}

var readyAckRules = []readyAckRule{
	{
		name: "window-message-listener",
		re:   regexp.MustCompile(`window\.addEventListener\(\s*['"]message['"]`),
		want: true,
		why: "the emitter must listen for host messages on `window` — a listener on `document` " +
			"never sees a postMessage, and one attached inside an async chain can miss every retry",
	},
	{
		name: "gated-on-block-init",
		re:   regexp.MustCompile(`['"]BLOCK_INIT['"]`),
		want: true,
		why: "the ack must be triggered by the host's BLOCK_INIT, not by load: the host registers " +
			"its BLOCK_READY subscriber inside an effect and silently drops a message that arrives first",
	},
	{
		// The object literal must carry BOTH `type: 'BLOCK_READY'` and a
		// `payload:` key. `[^{}]*` keeps the match inside the outer object, so
		// a `payload` belonging to some other literal cannot satisfy it.
		name: "type-payload-envelope",
		re:   regexp.MustCompile(`postMessage\(\s*\{[^{}]*type\s*:\s*['"]BLOCK_READY['"][^{}]*payload\s*:`),
		want: true,
		why: "every block -> host message is `{ type, payload }` — the host reads `event.data.payload`, " +
			"so fields placed at the top level arrive as `payload: undefined`",
	},
	{
		name: "targets-event-origin",
		re:   regexp.MustCompile(`event\.origin`),
		want: true,
		why:  "the ack must be addressed at the origin the validated BLOCK_INIT came from",
	},
	{
		name: "no-wildcard-origin",
		re:   regexp.MustCompile(`['"]\*['"]`),
		want: false,
		why:  "the ack must never be broadcast to `'*'` — BLOCK_INIT hands us the real host origin",
	},
}

// checkReadyAckShape returns the names of every violated rule, in declaration
// order. The input must already have comments stripped.
func checkReadyAckShape(code string) []string {
	var bad []string
	for _, r := range readyAckRules {
		if r.re.MatchString(code) != r.want {
			bad = append(bad, r.name)
		}
	}
	return bad
}

func ruleWhy(name string) string {
	for _, r := range readyAckRules {
		if r.name == name {
			return r.why
		}
	}
	return "(unknown rule)"
}

// TestReadyAckShapePredicate is the predicate's own control pair: the canonical
// emitter must satisfy every rule, and a corpus of near-misses must each be
// rejected BY THE EXPECTED RULE. Asserting only "rejected" would pass for a
// predicate that rejects everything, including the real emitter.
func TestReadyAckShapePredicate(t *testing.T) {
	t.Run("positive: the canonical emitter satisfies every rule", func(t *testing.T) {
		bad := checkReadyAckShape(stripJSComments(string(blockproto.ReadyAckSource())))
		for _, name := range bad {
			t.Errorf("blockproto's ready-ack emitter violates %q: %s", name, ruleWhy(name))
		}
	})

	// Each of these is something that LOOKS like it implements the handshake to
	// a grep, and does not.
	corpus := []struct {
		name     string
		src      string
		wantRule string
	}{
		{
			name:     "a comment that merely mentions the message",
			src:      "// TODO: post BLOCK_READY to the parent once we know the origin.\n(function () {})();\n",
			wantRule: "window-message-listener",
		},
		{
			name: "prose in a README",
			src: "# Handshake\n\nThe host waits for a `BLOCK_READY` message before it will show your app.\n" +
				"Send it in response to `BLOCK_INIT`.\n",
			wantRule: "window-message-listener",
		},
		{
			name: "the wrong envelope — fields at the top level",
			src: "window.addEventListener('message', function (event) {\n" +
				"  if (event.data && event.data.type === 'BLOCK_INIT') {\n" +
				"    window.parent.postMessage({ type: 'BLOCK_READY', height: 0 }, event.origin);\n" +
				"  }\n});\n",
			wantRule: "type-payload-envelope",
		},
		{
			name: "a post with no BLOCK_INIT trigger",
			src: "window.addEventListener('load', function () {\n" +
				"  window.parent.postMessage({ type: 'BLOCK_READY', payload: { height: 0 } }, '*');\n" +
				"});\n",
			wantRule: "gated-on-block-init",
		},
		{
			name: "a correct ack broadcast to the wildcard origin",
			src: "window.addEventListener('message', function (event) {\n" +
				"  if (event.data && event.data.type === 'BLOCK_INIT') {\n" +
				"    // event.origin is right there, and we ignore it.\n" +
				"    window.parent.postMessage({ type: 'BLOCK_READY', payload: { height: 0 } }, '*');\n" +
				"  }\n});\n",
			wantRule: "no-wildcard-origin",
		},
	}

	for _, c := range corpus {
		t.Run("negative: "+c.name, func(t *testing.T) {
			bad := checkReadyAckShape(stripJSComments(c.src))
			if len(bad) == 0 {
				t.Fatalf("the shape predicate ACCEPTED a snippet that does not implement the handshake:\n%s", c.src)
			}
			if !containsStr(bad, c.wantRule) {
				t.Fatalf("rejected for the wrong reason: got violations %v, expected %q among them — "+
					"a green here would say nothing about %s", bad, c.wantRule, c.wantRule)
			}
		})
	}
}

// TestReadyAckWiringPredicate is the wiring check's own control pair, built on
// synthetic trees rather than the real templates so the REJECT cases can exist
// at all.
//
// It exists because the first version of this check matched a BASENAME, which
// accepted every "wrong directory" shape. Each reject case below is a shape
// that check accepted while shipping a broken app; the accept cases are the two
// real templates' shapes. Ask of any future edit: can it pass while the emitter
// is somewhere the browser will not find it?
func TestReadyAckWiringPredicate(t *testing.T) {
	const ackRel = "src/civitai-host.js"

	type file struct{ path, body string }
	cases := []struct {
		name       string
		ack        string // where the emitter really is
		files      []file
		wantAccept bool
	}{
		{
			name: "accept: index.html loads it directly (the static shape)",
			ack:  "civitai-host.js",
			files: []file{
				{"index.html", `<script src="./civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
			wantAccept: true,
		},
		{
			name: "accept: the entry module imports it (the page-vite shape)",
			ack:  ackRel,
			files: []file{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import './civitai-host.js';\nimport React from 'react';"},
			},
			wantAccept: true,
		},
		{
			name: "reject: right basename, wrong directory in the script tag",
			ack:  "civitai-host.js",
			files: []file{
				{"index.html", `<script src="./vendor/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
		},
		{
			name: "reject: right basename, unresolvable relative import",
			ack:  ackRel,
			files: []file{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import '../nonexistent/civitai-host.js';"},
			},
		},
		{
			name: "reject: a BARE specifier that would resolve to a package",
			ack:  ackRel,
			files: []file{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import 'civitai-host.js';"},
			},
		},
		{
			name: "reject: referenced only from an HTML comment",
			ack:  "civitai-host.js",
			files: []file{
				{"index.html", "<!-- <script src=\"./civitai-host.js\"></script> -->\n<script src=\"./app.js\"></script>"},
				{"app.js", "// app"},
			},
		},
		{
			name: "reject: the import is commented out but still greppable",
			ack:  ackRel,
			files: []file{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "// import './civitai-host.js';\nimport React from 'react';"},
			},
		},
		{
			name: "reject: imported by a file nothing loads",
			ack:  ackRel,
			files: []file{
				{"index.html", `<script type="module" src="/src/main.jsx"></script>`},
				{"src/main.jsx", "import React from 'react';"},
				{"src/orphan.js", "import './civitai-host.js';"},
			},
		},
		{
			name:  "reject: no script tags at all",
			ack:   "civitai-host.js",
			files: []file{{"index.html", "<h1>hi</h1>"}},
		},
		{
			name: "reject: loaded from a CDN URL that merely ends in the name",
			ack:  "civitai-host.js",
			files: []file{
				{"index.html", `<script src="https://cdn.example.com/civitai-host.js"></script><script src="./app.js"></script>`},
				{"app.js", "// app"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			// The emitter itself always exists where `ack` says — so a
			// rejection can only come from the REFERENCE, never from a missing
			// file.
			all := append([]file{{c.ack, "// emitter"}}, c.files...)
			for _, f := range all {
				p := filepath.Join(dir, filepath.FromSlash(f.path))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			chain, err := readyAckWiring(dir, c.ack)
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
			t.Logf("rejected: %v", err)
		})
	}
}

// ---------------------------------------------------------------------------
// The per-template contract.
// ---------------------------------------------------------------------------

// minPageTemplatesNeedingAck is the COUNT POSITIVE CONTROL. Without it a guard
// wired to nothing — a manifest decoder that stopped finding `page`, a render
// that stopped emitting, an enumeration that returned an empty slice — reports
// a serene pass. `static` and `page-vite` are both SDK-free page templates, so
// this floor is 2 today; raise it when a third lands, never lower it to make a
// run green.
//
// This complements the `AllTemplates() != 3` tripwire in slug_test.go rather
// than duplicating it: that one pins the total, this one pins how many the
// ready-ack guard actually EXAMINED.
const minPageTemplatesNeedingAck = 2

// TestScaffoldedPageTemplatesShipTheReadyAck is the contract guard.
func TestScaffoldedPageTemplatesShipTheReadyAck(t *testing.T) {
	canonical := blockproto.ReadyAckSource()

	examined := 0
	var (
		pageTemplates []string
		sdkTemplates  []string
	)

	for _, tmpl := range AllTemplates() {
		dest := filepath.Join(t.TempDir(), string(tmpl))
		if _, err := Render(tmpl, dest, Data{Slug: "ready-block", Name: "Ready Block"}); err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}

		if !manifestDeclaresPage(t, tmpl, dest) {
			// Not a full-page surface: the readiness contract this guard pins
			// is the page host's. Skipped on EVIDENCE from the rendered
			// manifest, not from a list in this file.
			t.Logf("%s: manifest declares no `page` surface — not subject to the page-host ready contract", tmpl)
			continue
		}
		pageTemplates = append(pageTemplates, string(tmpl))

		if pkgs := civitaiSDKDeps(t, dest); len(pkgs) > 0 {
			// `@civitai/blocks-react`'s IframeTransport acks internally (it
			// dispatches BLOCK_READY from its validated-BLOCK_INIT branch), so
			// a second emitter here would double-post into a rate-limited
			// inbound channel. Again: skipped on evidence — the rendered
			// package.json really depends on the SDK.
			sdkTemplates = append(sdkTemplates, string(tmpl))
			t.Logf("%s: depends on %s — the SDK transport performs the ack; no vendored emitter expected",
				tmpl, strings.Join(pkgs, ", "))
			if rel := tmpl.ReadyAckPath(); rel != "" {
				t.Errorf("%s carries the SDK AND declares ReadyAckPath()=%q — that double-posts BLOCK_READY; "+
					"pick one", tmpl, rel)
			}
			continue
		}

		rel := tmpl.ReadyAckPath()
		if rel == "" {
			t.Errorf("template %q declares a `page` surface and carries no @civitai/* SDK, but "+
				"ReadyAckPath() is empty — a page app that never posts BLOCK_READY is replaced by a "+
				"failure card in the real host (issue #206). Give it a path in scaffold.go so "+
				"Render ships blockproto's emitter.", tmpl)
			continue
		}
		examined++

		t.Run(string(tmpl), func(t *testing.T) {
			// ASSERTION 1 — byte-equality with the single authority. Kept
			// independent of the wiring assertion below: a template can ship a
			// perfect copy nobody loads, or load a copy that has drifted, and
			// the two failures need different fixes.
			t.Run("byte-identical to blockproto.ReadyAckSource", func(t *testing.T) {
				got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
				if err != nil {
					t.Fatalf("%s does not ship %s: %v — Render must copy blockproto's emitter in", tmpl, rel, err)
				}
				if string(got) != string(canonical) {
					t.Fatalf("%s ships a ready-ack emitter that is NOT byte-identical to "+
						"blockproto.ReadyAckSource() (%d bytes rendered vs %d canonical).\n"+
						"There must be exactly ONE authority for the handshake — edit "+
						"internal/blockproto/ready-ack.js, never a per-template copy.",
						tmpl, len(got), len(canonical))
				}
			})

			// ASSERTION 2 — the copy is actually LOADED. Byte-equality of an
			// orphan file proves nothing about the running app.
			t.Run("reached from the rendered entry point", func(t *testing.T) {
				chain, err := readyAckWiring(dest, rel)
				if err != nil {
					t.Fatalf("%s ships %s but nothing loads it: %v\n"+
						"Reference it from index.html (a <script src>) or from the entry module "+
						"index.html loads (an `import`). A mention in a comment does not count.",
						tmpl, rel, err)
				}
				t.Logf("%s: %s", tmpl, chain)
			})
		})
	}

	// COUNT POSITIVE CONTROL. A zero here is otherwise indistinguishable from a
	// guard that examined nothing at all.
	if examined < minPageTemplatesNeedingAck {
		t.Fatalf("the ready-ack guard examined only %d SDK-free page template(s), expected at least %d.\n"+
			"  page-declaring templates seen: %v\n"+
			"  of those, SDK-backed (skipped): %v\n"+
			"This is the count control: a low number means the guard stopped OBSERVING (manifest "+
			"decoding, SDK detection, or AllTemplates() changed), not that the templates got safer.",
			examined, minPageTemplatesNeedingAck, pageTemplates, sdkTemplates)
	}
	t.Logf("examined %d SDK-free page template(s) out of %d page-declaring: %v (SDK-backed: %v)",
		examined, len(pageTemplates), pageTemplates, sdkTemplates)
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// manifestDeclaresPage decodes the RENDERED manifest and reports whether it
// declares a non-null `page` surface.
func manifestDeclaresPage(t *testing.T, tmpl Template, dir string) bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "block.manifest.json"))
	if err != nil {
		t.Fatalf("%s: read rendered manifest: %v", tmpl, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: rendered manifest is not valid JSON: %v", tmpl, err)
	}
	v, ok := m["page"]
	return ok && strings.TrimSpace(string(v)) != "null"
}

// civitaiSDKDeps returns the `@civitai/*` packages the rendered package.json
// depends on (sorted). Empty for a template with no package.json at all.
func civitaiSDKDeps(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read package.json in %s: %v", dir, err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("rendered package.json in %s is not valid JSON: %v", dir, err)
	}
	var out []string
	for _, set := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name := range set {
			if strings.HasPrefix(name, "@civitai/") {
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

var (
	reHTMLComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reScriptSrc   = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	// Captures the SPECIFIER of an import / re-export / require, so it can be
	// resolved. Matching a basename inside the specifier is what let
	// `import '../nonexistent/civitai-host.js'` through.
	reJSImport = regexp.MustCompile(`(?:\bimport\s*\(?\s*|\bfrom\s+|\brequire\(\s*)['"]([^'"]+)['"]`)
)

// readyAckWiring walks the rendered entry chain and returns a description of
// where the emitter is loaded from, or an error if nothing reaches it.
//
// It follows the same path a browser does — index.html's <script> tags, then
// (for a module entry) that module's imports — rather than grepping the whole
// tree, so an emitter referenced only from a README, a comment, or a file
// nothing loads does not satisfy it.
//
// 🔴 Every reference is RESOLVED to a path and compared with where the emitter
// actually IS. It is deliberately NOT a basename match. A basename match is a
// SPELLED check rather than a structural one, and it passes while the hazard
// exists in a different shape — both of these were MEASURED surviving the
// entire suite before this was rewritten:
//
//	static/index.html    src="./vendor/civitai-host.js"          -> ships a 404
//	page-vite/main.jsx   import '../nonexistent/civitai-host.js' -> build fails
//
// Both reproduce #206 with every check green, which is why the first version of
// this guard did not deserve the claim that it proved the entry point REACHES
// the emitter.
func readyAckWiring(dir, ackRel string) (string, error) {
	want := filepath.Clean(filepath.Join(dir, filepath.FromSlash(ackRel)))

	htmlRaw, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return "", err
	}
	html := reHTMLComment.ReplaceAllString(string(htmlRaw), "")

	// Every reference considered, with what it resolved to and whether that
	// file exists — reported verbatim on failure so a near-miss (right
	// basename, wrong directory) names itself instead of reading as "nothing
	// references it at all".
	var tried []string
	note := func(what, spec, resolved string) {
		if resolved == "" {
			tried = append(tried, fmt.Sprintf("%s %q -> not a path in this project (bare specifier or URL)", what, spec))
			return
		}
		state := "exists"
		if _, err := os.Stat(resolved); err != nil {
			state = "DOES NOT EXIST"
		}
		rel, relErr := filepath.Rel(dir, resolved)
		if relErr != nil {
			rel = resolved
		}
		tried = append(tried, fmt.Sprintf("%s %q -> %s (%s)", what, spec, filepath.ToSlash(rel), state))
	}

	var entries []string
	for _, m := range reScriptSrc.FindAllStringSubmatch(html, -1) {
		src := m[1]
		// A <script src> resolves against the document, which sits at the
		// project root.
		resolved, ok := resolveProjectRef(dir, dir, src)
		if ok && resolved == want {
			// Existence is pinned by the byte-equality sub-test, which reads
			// this exact path — re-asserting it here would add a branch no
			// mutation can reach.
			return "index.html loads " + src + " -> " + ackRel, nil
		}
		note("index.html <script src>", src, resolved)
		entries = append(entries, src)
	}
	if len(entries) == 0 {
		return "", errNoScriptTags
	}

	// Not loaded directly — follow index.html's script entries and resolve
	// every import in them. One level deep on purpose: the emitter is meant to
	// be imported by the entry module itself, not buried behind a chain.
	for _, src := range entries {
		entryPath, ok := resolveProjectRef(dir, dir, src)
		if !ok {
			continue
		}
		raw, err := os.ReadFile(entryPath)
		if err != nil {
			continue // a <script src> that is not a file in the tree
		}
		code := stripJSComments(string(raw))
		for _, m := range reJSImport.FindAllStringSubmatch(code, -1) {
			spec := m[1]
			// An import resolves against the IMPORTING MODULE's directory.
			resolved, ok := resolveProjectRef(dir, filepath.Dir(entryPath), spec)
			if ok && resolved == want {
				return "index.html loads " + src + ", which imports " + spec + " -> " + ackRel, nil
			}
			note("import in "+src, spec, resolved)
		}
	}
	return "", readyAckWiringError(errNotReached.Error() + "\n  references resolved:\n    " +
		strings.Join(tried, "\n    "))
}

// resolveProjectRef resolves a <script src> or an import specifier to a path
// inside the project. A root-relative ref resolves against the project root, a
// relative one against fromDir. A bare specifier (a package name) or an
// absolute URL is not a file in this project and reports false — which is what
// makes `import 'civitai-host.js'` (a bare specifier that would resolve to a
// PACKAGE, not this file) a rejection rather than a match.
func resolveProjectRef(projectDir, fromDir, spec string) (string, bool) {
	if strings.Contains(spec, "://") || strings.HasPrefix(spec, "//") {
		return "", false
	}
	// Vite allows query/hash suffixes (`?raw`, `?url`); they are not part of
	// the path.
	if i := strings.IndexAny(spec, "?#"); i >= 0 {
		spec = spec[:i]
	}
	if spec == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(spec, "/"):
		return filepath.Clean(filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(spec, "/")))), true
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"):
		return filepath.Clean(filepath.Join(fromDir, filepath.FromSlash(spec))), true
	}
	return "", false
}

type readyAckWiringError string

func (e readyAckWiringError) Error() string { return string(e) }

const (
	errNoScriptTags = readyAckWiringError("the rendered index.html has no <script src> at all")
	errNotReached   = readyAckWiringError("no <script src> in index.html resolves to it, and no import in an entry module resolves to it")
)

// stripJSComments removes // and /* */ comments while respecting string and
// template literals, so a URL like 'https://x' is not mistaken for a comment
// and a comment mentioning a filename cannot satisfy a wiring check.
func stripJSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i = min(i+2, len(src))
		case c == '\'' || c == '"' || c == '`':
			quote := c
			b.WriteByte(c)
			i++
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					b.WriteString(src[i : i+2])
					i += 2
					continue
				}
				b.WriteByte(src[i])
				if src[i] == quote {
					i++
					break
				}
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
