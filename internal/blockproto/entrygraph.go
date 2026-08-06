package blockproto

// entrygraph.go resolves THE SET OF FILES A BROWSER ACTUALLY LOADS for a block
// project: index.html, the scripts it references, and the modules those import.
//
// WHY IT LIVES HERE. It started as test-only code in
// `internal/scaffold/ready_ack_contract_test.go`, where Guard A uses it to prove
// a scaffolded template's vendored emitter is REACHED and not merely present.
// `internal/validate`'s page-without-ack advisory needed exactly the same
// question answered for an AUTHOR's project — and while it was answered by a
// whole-tree grep instead, an author who copied `civitai-host.js` in without
// referencing it from index.html got a green `validate` for an app that was
// still completely broken. That is the same "presence is not reachability" hole
// Guard A had before it was rewritten, regenerated at a second call site, which
// is precisely the shape `PackageAcksReady` was consolidated here to avoid.
//
// 🔴 AN HTML `src` IS A URL. A JS SPECIFIER IS A MODULE SPECIFIER. THEY ARE
// RESOLVED BY DIFFERENT RULES, AND CONFLATING THEM PRODUCED THREE SEPARATE BUGS.
// The first version of this file ran every reference through one resolver
// written for module specifiers. Measured consequences, each on a project one
// character away from a shipped template:
//
//	<script src="app.js">         read as a BARE specifier -> "not a path in
//	                              this project" -> gap -> presence tier -> the
//	                              headline defect SILENT again, at ordinary HTML
//	<script src="./civitai-host"> extension-guessed to civitai-host.js and
//	                              ACCEPTED, on a no-build template where the
//	                              browser fetches the literal path and 404s —
//	                              the exact "ships a 404" shape wiring.go claims
//	                              this resolver rejects
//
// So there are now two syntaxes (`refSyntax`). Under URL syntax a specifier that
// is not scheme-qualified or protocol-relative is DOCUMENT-RELATIVE — `app.js`,
// `./app.js` and `/app.js` all name a file in this project — and there is no
// extension or directory-index guessing, because a browser fetches the literal
// path. Under module syntax a bare specifier is a package, and extension/index
// resolution applies, because a bundler does that.
//
// 🔴 IT IS A MODEL OF A BROWSER, NOT A BUNDLER, AND IT SAYS SO.
// The graph carries a `Complete` flag, and every caller must branch on it. A
// reference this resolver cannot account for — a bundler alias, a bare
// specifier that is not a declared dependency, a CDN URL, a path that resolves
// to a file that is not there, a file it could not read, a budget it exhausted,
// AN IMPORT BELOW ITS DEPTH BOUND — sets `Complete = false` and records why in
// `Gaps`. An INCOMPLETE graph is evidence about NOTHING: the emitter may well be
// reached through the part we could not follow. Callers that would report a
// finding must go quiet (or fall back to a weaker check that discloses itself),
// never treat "we did not see it" as "it is not there".
//
// 🔴 THE DEPTH BOUND IS A BUDGET LIKE ANY OTHER, AND TRUNCATING IT IS A GAP.
// It did not used to be: `continue`-ing at MaxDepth left `Complete` true, so a
// CORRECT project whose ack sat below the bound got the STRONG tier's warning
// and `--strict` rc=1 — a false warning built on a graph we had stopped walking,
// while three separate comments claimed an exhausted budget made the graph
// incomplete. Measured at depth 10 before the fix. Only imports that are NOT
// declared packages count as truncated: a leaf importing `react` and nothing
// else has nothing left to see.
//
// WHAT IT DELIBERATELY DOES NOT MODEL. `resolve.alias` / `tsconfig` path
// mapping, glob imports, dynamic `import(expr)` with a computed specifier,
// framework-owned entry points (Next.js, SvelteKit) that have no root
// index.html, and anything a plugin injects at transform time. Each of those
// makes the graph INCOMPLETE rather than wrong, which is the safe direction:
// the cost is a check that goes quiet, never a warning at a correct project.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EntryGraph defaults. `validate` walks an author's project, so the budgets are
// cost limits rather than correctness ones — exhausting one makes the graph
// INCOMPLETE, which is the direction that stays quiet.
const (
	DefaultEntryMaxDepth     = 8
	DefaultEntryMaxFiles     = 200
	DefaultEntryMaxFileBytes = 2 << 20 // 2 MiB
)

// EntryGraphOptions tunes ResolveEntryGraph. The zero value is usable and takes
// the Default* constants above.
type EntryGraphOptions struct {
	// MaxDepth bounds how far below index.html (depth 0) the walk follows
	// imports. Guard A passes 2 — index.html's scripts and their direct
	// imports — because a template's emitter is meant to be loaded by the entry
	// itself and a deeper chain is a template smell. `validate` passes the
	// default, because an author's module layout is none of our business.
	MaxDepth int
	// MaxFiles bounds files read. Exhausting it is a Gap.
	MaxFiles int
	// MaxFileBytes bounds a single file. Exceeding it is a Gap: we did not read
	// the file, so we do not know what is in it.
	MaxFileBytes int64
	// Dependencies are the npm package names the project declares. A BARE
	// MODULE specifier naming one is ACCOUNTED FOR (it is a package, so it is
	// not a file in this project) and does not make the graph incomplete. A
	// bare module specifier that is NOT a declared dependency is the alias
	// case, and DOES — `import '@/civitai-host.js'` is a real file behind a
	// bundler alias we cannot follow. Leave nil to treat every bare module
	// specifier as a gap.
	//
	// It does NOT apply to an HTML `src`, which has no bare specifiers at all.
	Dependencies map[string]bool
	// Inspect, when non-nil, is called for every file the walk reads, with that
	// file's comment-stripped contents WHILE THEY ARE IN HAND.
	//
	// 🔴 This is why EntryFile carries no `Code` field. Retaining every graph
	// file's contents peaked 410 MB RSS on a 200-module graph entirely INSIDE
	// the file-count and file-size budgets (measured; ~17 MB before the graph
	// existed) — a check whose own caps exist because `validate` must not become
	// a memory event, exceeding the very figure those caps were sized against.
	// The tree scan next door deliberately holds one file at a time; so does
	// this.
	Inspect func(f EntryFile, code string)
}

func (o EntryGraphOptions) withDefaults() EntryGraphOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultEntryMaxDepth
	}
	if o.MaxFiles <= 0 {
		o.MaxFiles = DefaultEntryMaxFiles
	}
	if o.MaxFileBytes <= 0 {
		o.MaxFileBytes = DefaultEntryMaxFileBytes
	}
	return o
}

// EntryFile is one file the browser loads. Its CONTENTS are not retained — see
// EntryGraphOptions.Inspect.
type EntryFile struct {
	// Path is absolute and cleaned; Rel is slash-separated, relative to Root.
	Path string
	Rel  string
	// Depth is 0 for index.html, 1 for a file index.html references, and so on.
	Depth int
	// Spec is the specifier that pulled this file in ("" for index.html);
	// Via indexes the referencing file in Files (-1 for index.html).
	Spec string
	Via  int
}

// EntryGraph is the result of ResolveEntryGraph.
type EntryGraph struct {
	Root  string
	Files []EntryFile
	// Complete reports that every reference encountered was accounted for. A
	// false here means the graph is a LOWER BOUND on what the browser loads and
	// must never be read as evidence that something is absent.
	Complete bool
	// Gaps explains, in author-readable terms, why Complete is false.
	Gaps []string
	// Trace lists every reference considered and what it resolved to, for
	// error messages that name the near-miss instead of saying "nothing".
	Trace []string
	// ScriptRefs counts `<script src=…>` references found in index.html,
	// whether or not they resolved. Zero distinguishes "index.html loads
	// nothing" from "index.html loads the wrong thing".
	ScriptRefs int
}

// FileAt returns the index in Files of the file at the given absolute path, or
// -1.
func (g *EntryGraph) FileAt(abs string) int {
	want := filepath.Clean(abs)
	for i, f := range g.Files {
		if f.Path == want {
			return i
		}
	}
	return -1
}

// Chain renders how Files[i] is reached, for a human.
func (g *EntryGraph) Chain(i int) string {
	if i < 0 || i >= len(g.Files) {
		return ""
	}
	f := g.Files[i]
	if f.Via < 0 {
		return f.Rel
	}
	parent := g.Files[f.Via]
	verb := "imports"
	if parent.Depth == 0 {
		verb = "loads"
	}
	return g.Chain(f.Via) + " " + verb + " " + f.Spec + " -> " + f.Rel
}

func (g *EntryGraph) gap(format string, a ...any) {
	g.Complete = false
	g.Gaps = append(g.Gaps, fmt.Sprintf(format, a...))
}

// refSyntax picks the resolution rules. The distinction is not cosmetic — see
// the file header for the three bugs conflating them produced.
type refSyntax int

const (
	// syntaxURL is an HTML attribute value: a URL resolved against the
	// document. There are NO bare specifiers, and no extension guessing.
	syntaxURL refSyntax = iota
	// syntaxModule is a JS/TS import specifier: bare means a package, and a
	// bundler applies extension and directory-index resolution.
	syntaxModule
)

type pending struct {
	spec  string
	from  string // directory the specifier resolves against
	via   int
	depth int
	what  string
	syn   refSyntax
}

// ResolveEntryGraph walks the project in dir the way a browser does, starting
// at index.html. It never returns nil.
//
// 🔴 Read `Complete` before reading `Files`. See the file header.
func ResolveEntryGraph(dir string, opts EntryGraphOptions) *EntryGraph {
	opts = opts.withDefaults()
	g := &EntryGraph{Root: dir, Complete: true}

	indexPath := filepath.Clean(filepath.Join(dir, "index.html"))
	raw, err := readCapped(indexPath, opts.MaxFileBytes)
	if err != nil {
		// No root index.html is not a broken project — it is a project whose
		// entry point this resolver does not model (a framework-owned entry, a
		// nested html root). There is nothing to walk, so the graph is empty
		// AND incomplete: callers must not read "no files load the emitter"
		// out of it.
		g.gap("no readable index.html at the project root (%v)", err)
		return g
	}
	html := StripHTMLComments(string(raw))
	index := EntryFile{Path: indexPath, Rel: "index.html", Depth: 0, Via: -1}
	g.Files = append(g.Files, index)
	if opts.Inspect != nil {
		opts.Inspect(index, StripCommentsForExt(string(raw), ".html"))
	}

	var queue []pending

	// `<script src=…>` is a URL resolved against the document, which sits at
	// the project root.
	for _, attrs := range reScriptOpen.FindAllStringSubmatch(html, -1) {
		src, ok := srcAttrValue(attrs[1])
		if !ok {
			continue
		}
		g.ScriptRefs++
		queue = append(queue, pending{spec: src, from: dir, via: 0, depth: 1,
			what: "index.html <script src>", syn: syntaxURL})
	}
	// Inline `<script>` blocks execute too, and a module one can import the
	// emitter. Without this a project whose index.html carries
	// `<script type="module">import './civitai-host.js'</script>` would resolve
	// to a graph that "completely" misses the emitter — a false finding at a
	// correct project. Their specifiers are MODULE syntax, resolved against the
	// document's directory (the project root).
	for _, m := range reScriptBlock.FindAllStringSubmatch(html, -1) {
		if _, ok := srcAttrValue(m[1]); ok {
			continue // external: already queued above
		}
		for _, im := range reJSImport.FindAllStringSubmatch(StripJSComments(m[2]), -1) {
			queue = append(queue, pending{spec: im[1], from: dir, via: 0, depth: 1,
				what: "inline <script> in index.html", syn: syntaxModule})
		}
	}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		resolved, kind := resolveProjectRef(dir, p.from, p.spec, opts.Dependencies, p.syn)
		g.note(p.what, p.spec, resolved, kind)
		switch kind {
		case refPackage:
			// A declared dependency. It is not a file in this project, and
			// whether IT acks is a separate question the caller answers from
			// package.json — so this is accounted for, not a gap.
			continue
		case refUnresolved:
			if p.syn == syntaxURL {
				g.gap("could not resolve %s %q — an off-project URL, or a path outside this project", p.what, p.spec)
			} else {
				g.gap("could not resolve %s %q — a bundler alias, a bare specifier that is not a declared dependency, or an off-project URL", p.what, p.spec)
			}
			continue
		}
		file, err := resolveRefPath(resolved, p.syn)
		if err != nil {
			// A reference that points at nothing means this resolver disagrees
			// with a project that presumably builds — our model is wrong, not
			// the project. That is a GAP, not "the browser 404s it": treating
			// it as the latter would let a mis-modelled project produce a
			// confident finding.
			g.gap("%s %q resolves to %s, which does not exist — this resolver's model of the project is incomplete",
				p.what, p.spec, relTo(dir, resolved))
			continue
		}
		if g.FileAt(file) >= 0 {
			continue
		}
		if len(g.Files) >= opts.MaxFiles {
			g.gap("stopped after %d files — the entry graph is larger than this check reads", opts.MaxFiles)
			return g
		}
		body, err := readCapped(file, opts.MaxFileBytes)
		if err != nil {
			g.gap("could not read %s (%v)", relTo(dir, file), err)
			continue
		}
		ext := strings.ToLower(filepath.Ext(file))
		code := StripCommentsForExt(string(body), ext)
		idx := len(g.Files)
		f := EntryFile{Path: file, Rel: relTo(dir, file), Depth: p.depth, Spec: p.spec, Via: p.via}
		g.Files = append(g.Files, f)
		if opts.Inspect != nil {
			opts.Inspect(f, code)
		}
		if !entryModuleExts[ext] {
			continue
		}
		for _, im := range reJSImport.FindAllStringSubmatch(code, -1) {
			spec := im[1]
			if p.depth+1 > opts.MaxDepth {
				// 🔴 TRUNCATION IS A GAP. A package specifier is not truncated
				// content — it was never going to be followed — so only a
				// reference naming something in this project counts.
				if _, k := resolveProjectRef(dir, filepath.Dir(file), spec, opts.Dependencies, syntaxModule); k != refPackage {
					g.gap("stopped at import depth %d: %s imports %q, which this check did not follow",
						opts.MaxDepth, f.Rel, spec)
				}
				continue
			}
			queue = append(queue, pending{
				spec: spec, from: filepath.Dir(file), via: idx, depth: p.depth + 1,
				what: "import in " + f.Rel, syn: syntaxModule,
			})
		}
	}
	return g
}

func (g *EntryGraph) note(what, spec, resolved string, kind refKind) {
	switch kind {
	case refPackage:
		g.Trace = append(g.Trace, fmt.Sprintf("%s %q -> a declared npm dependency, not a file in this project", what, spec))
	case refUnresolved:
		g.Trace = append(g.Trace, fmt.Sprintf("%s %q -> not a path in this project (bare specifier or URL)", what, spec))
	default:
		state := "exists"
		if _, err := os.Stat(resolved); err != nil {
			state = "DOES NOT EXIST"
		}
		g.Trace = append(g.Trace, fmt.Sprintf("%s %q -> %s (%s)", what, spec, relTo(g.Root, resolved), state))
	}
}

func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(rel)
}

func readCapped(path string, max int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", filepath.Base(path))
	}
	if info.Size() > max {
		return nil, fmt.Errorf("file is larger than %d bytes", max)
	}
	return os.ReadFile(path)
}

// entryModuleExts are the extensions whose imports are followed. A `.css` a
// module imports is still part of the graph (it is added to Files) but has no
// imports worth chasing — and widening this set would let a stylesheet pull
// files into the graph, which no browser does.
var entryModuleExts = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".vue": true, ".svelte": true, ".astro": true,
}

// entryResolveExts are appended to an extensionless MODULE specifier, in the
// order a bundler tries them. They are never applied to a URL.
var entryResolveExts = []string{".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".mts", ".cts", ".vue", ".svelte", ".astro"}

// resolveRefPath turns a resolved reference into a file that exists.
//
// 🔴 URL syntax gets NO extension or directory-index guessing. A browser fetches
// the literal path: `<script src="./civitai-host">` on a no-build template is a
// 404, and guessing `.js` for it accepted exactly the "ships a 404" shape this
// resolver exists to reject.
func resolveRefPath(p string, syn refSyntax) (string, error) {
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return filepath.Clean(p), nil
	}
	if syn == syntaxURL {
		return "", os.ErrNotExist
	}
	for _, ext := range entryResolveExts {
		if info, err := os.Stat(p + ext); err == nil && !info.IsDir() {
			return filepath.Clean(p + ext), nil
		}
	}
	for _, ext := range entryResolveExts {
		cand := filepath.Join(p, "index"+ext)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return filepath.Clean(cand), nil
		}
	}
	return "", os.ErrNotExist
}

type refKind int

const (
	// refUnresolved: we cannot say what file this is. Always a gap.
	refUnresolved refKind = iota
	// refProject: a path inside the project.
	refProject
	// refPackage: a bare MODULE specifier naming a declared npm dependency.
	refPackage
)

// reURLScheme matches an RFC 3986 scheme prefix. A browser treats
// `<script src="foo:bar">` as an absolute URL rather than a relative path, so
// this is how an HTML `src` is told apart from a document-relative one.
var reURLScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

// resolveProjectRef resolves a `<script src>` (syntaxURL) or an import
// specifier (syntaxModule).
//
// Under MODULE syntax a bare specifier is a PACKAGE when the project declares
// it as a dependency and unresolvable otherwise — which is what makes
// `import 'civitai-host.js'` (a bare specifier that resolves to a package, not
// to this file) a non-match rather than a hit.
//
// Under URL syntax there are no bare specifiers at all: `app.js` is
// document-relative and names a file in this project, exactly as `./app.js`
// does. Reading it as a bare specifier is what left the headline defect intact
// for every project whose index.html omits `./`.
func resolveProjectRef(projectDir, fromDir, spec string, deps map[string]bool, syn refSyntax) (string, refKind) {
	// A protocol-relative URL only LOOKS root-relative. Without this, the `/`
	// case below strips one slash and `filepath.Join` cleans the rest, so
	// `//evil.example.com/../civitai-host.js` resolves to EXACTLY the emitter's
	// path and is accepted — measured. Pinned by the "protocol-relative URL
	// that cleans back" corpus case.
	if strings.HasPrefix(spec, "//") {
		return "", refUnresolved
	}
	if syn == syntaxURL && reURLScheme.MatchString(spec) {
		return "", refUnresolved
	}
	// Vite allows query/hash suffixes (`?raw`, `?url`); a URL has a real query
	// and fragment. Neither is part of the path, and an emitter referenced as
	// `./civitai-host.js?url` is still the emitter.
	if i := strings.IndexAny(spec, "?#"); i >= 0 {
		spec = spec[:i]
	}
	if spec == "" {
		return "", refUnresolved
	}
	var out string
	switch {
	case strings.HasPrefix(spec, "/"):
		out = filepath.Clean(filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(spec, "/"))))
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"):
		out = filepath.Clean(filepath.Join(fromDir, filepath.FromSlash(spec)))
	case syn == syntaxURL:
		// Document-relative. `app.js` and `./app.js` are the same URL.
		out = filepath.Clean(filepath.Join(fromDir, filepath.FromSlash(spec)))
	default:
		if declaresPackage(deps, spec) {
			return "", refPackage
		}
		return "", refUnresolved
	}
	// A reference that escapes the project root is not a file in this project.
	// A monorepo really can import one, which is exactly why it is a GAP rather
	// than a miss: we stop modelling there.
	if rel, err := filepath.Rel(projectDir, out); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", refUnresolved
	}
	return out, refProject
}

// declaresPackage reports whether spec names a declared dependency, allowing a
// subpath (`react-dom/client` -> `react-dom`, `@scope/pkg/sub` -> `@scope/pkg`).
//
// 🔴 The boundary is a PATH SEPARATOR, never a character offset. `reactive-ui`
// starts with `react` and is a different package; a prefix test that does not
// split on `/` accepts it and silently classifies a real project file as a
// dependency — which turns a gap into a confident (wrong) finding.
func declaresPackage(deps map[string]bool, spec string) bool {
	if len(deps) == 0 {
		return false
	}
	if deps[spec] {
		return true
	}
	parts := strings.Split(spec, "/")
	if strings.HasPrefix(spec, "@") {
		if len(parts) >= 2 && deps[parts[0]+"/"+parts[1]] {
			return true
		}
		return false
	}
	return deps[parts[0]]
}

// srcAttrValue extracts a `src` attribute value from a tag's attribute list,
// accepting the three HTML forms: double-quoted, single-quoted and UNQUOTED.
//
// 🔴 The unquoted form is legal HTML and used to be dropped entirely. The tag
// then looked like an INLINE script with an empty body, so the reference
// vanished from the graph while `Complete` stayed TRUE — a correct `static`
// scaffold written `src=./civitai-host.js` produced the strong tier's warning
// and `--strict` rc=1. A false warning at a correct project is the outcome
// AGENTS.md item 10 records four measured corrections to avoid.
func srcAttrValue(attrs string) (string, bool) {
	m := reSrcAttr.FindStringSubmatch(attrs)
	if m == nil {
		return "", false
	}
	for _, v := range m[1:] {
		if v != "" {
			return v, true
		}
	}
	// `src=""` / `src=''`: present but empty. It IS a src attribute (so the tag
	// is external rather than inline) and it names nothing.
	return "", true
}

var (
	// reScriptOpen captures a `<script …>` open tag's attribute list.
	reScriptOpen = regexp.MustCompile(`(?is)<script\b([^>]*)>`)
	// reScriptBlock captures a whole `<script …>…</script>` so an INLINE
	// module's imports can be read. Group 1 is the attribute list, group 2 the
	// body.
	reScriptBlock = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	// reSrcAttr captures a src value in any of HTML's three quoting forms.
	reSrcAttr = regexp.MustCompile(`(?is)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'` + "`" + `=<>]+))`)
	// reJSImport captures the SPECIFIER of an import / re-export / require, so
	// it can be resolved. Matching a basename inside the specifier is what let
	// `import '../nonexistent/civitai-host.js'` through.
	reJSImport = regexp.MustCompile(`(?:\bimport\s*\(?\s*|\bfrom\s+|\brequire\(\s*)['"]([^'"]+)['"]`)
)
