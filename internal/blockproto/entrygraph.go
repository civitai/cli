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
// 🔴 IT IS A MODEL OF A BROWSER, NOT A BUNDLER, AND IT SAYS SO.
// The graph carries a `Complete` flag, and every caller must branch on it. A
// reference this resolver cannot account for — a bundler alias, a bare
// specifier that is not a declared dependency, a CDN URL, a path that resolves
// to a file that is not there, a file it could not read, a budget it exhausted —
// sets `Complete = false` and records why in `Gaps`. An INCOMPLETE graph is
// evidence about NOTHING: the emitter may well be reached through the part we
// could not follow. Callers that would report a finding must go quiet (or fall
// back to a weaker check that discloses itself), never treat "we did not see it"
// as "it is not there".
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
	// default, because an author's own module layout is none of our business.
	MaxDepth int
	// MaxFiles bounds files read. Exhausting it is a Gap.
	MaxFiles int
	// MaxFileBytes bounds a single file. Exceeding it is a Gap: we did not read
	// the file, so we do not know what is in it.
	MaxFileBytes int64
	// Dependencies are the npm package names the project declares. A BARE
	// specifier naming one is ACCOUNTED FOR (it is a package, so it is not a
	// file in this project) and does not make the graph incomplete. A bare
	// specifier that is NOT a declared dependency is the alias case, and DOES —
	// `import '@/civitai-host.js'` is a real file behind a bundler alias we
	// cannot follow. Leave nil to treat every bare specifier as a gap.
	Dependencies map[string]bool
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

// EntryFile is one file the browser loads, with its comments already stripped.
type EntryFile struct {
	// Path is absolute and cleaned; Rel is slash-separated, relative to Root.
	Path string
	Rel  string
	// Code is the file's contents with comments stripped per its extension
	// (see StripCommentsForExt). A mention inside a comment is not a load and
	// not an implementation.
	Code string
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
	g.add(EntryFile{
		Path: indexPath, Rel: "index.html", Depth: 0, Via: -1,
		Code: StripCommentsForExt(string(raw), ".html"),
	})

	type pending struct {
		spec  string
		from  string // directory the specifier resolves against
		via   int
		depth int
		what  string
	}
	var queue []pending

	// `<script src=…>` resolves against the document, which sits at the root.
	for _, m := range reScriptSrc.FindAllStringSubmatch(html, -1) {
		g.ScriptRefs++
		queue = append(queue, pending{spec: m[1], from: dir, via: 0, depth: 1, what: "index.html <script src>"})
	}
	// Inline `<script>` blocks execute too, and a module one can import the
	// emitter. Without this a project whose index.html carries
	// `<script type="module">import './civitai-host.js'</script>` would resolve
	// to a graph that "completely" misses the emitter — a false finding at a
	// correct project.
	for _, m := range reScriptBlock.FindAllStringSubmatch(html, -1) {
		if reSrcAttr.MatchString(m[1]) {
			continue // external: already queued above
		}
		for _, im := range reJSImport.FindAllStringSubmatch(StripJSComments(m[2]), -1) {
			queue = append(queue, pending{spec: im[1], from: dir, via: 0, depth: 1, what: "inline <script> in index.html"})
		}
	}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		resolved, kind := resolveProjectRef(dir, p.from, p.spec, opts.Dependencies)
		g.note(p.what, p.spec, resolved, kind)
		switch kind {
		case refPackage:
			// A declared dependency. It is not a file in this project, and
			// whether IT acks is a separate question the caller answers from
			// package.json — so this is accounted for, not a gap.
			continue
		case refUnresolved:
			g.gap("could not resolve %s %q — a bundler alias, a bare specifier that is not a declared dependency, or an off-project URL", p.what, p.spec)
			continue
		}
		file, err := resolveModulePath(resolved)
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
		idx := len(g.Files)
		g.add(EntryFile{
			Path: file, Rel: relTo(dir, file), Depth: p.depth, Spec: p.spec, Via: p.via,
			Code: StripCommentsForExt(string(body), ext),
		})
		if p.depth >= opts.MaxDepth || !entryModuleExts[ext] {
			continue
		}
		for _, im := range reJSImport.FindAllStringSubmatch(g.Files[idx].Code, -1) {
			queue = append(queue, pending{
				spec: im[1], from: filepath.Dir(file), via: idx, depth: p.depth + 1,
				what: "import in " + g.Files[idx].Rel,
			})
		}
	}
	return g
}

func (g *EntryGraph) add(f EntryFile) { g.Files = append(g.Files, f) }

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
// imports worth chasing.
var entryModuleExts = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".vue": true, ".svelte": true, ".astro": true,
}

// entryResolveExts are appended to an extensionless specifier, in the order a
// bundler tries them.
var entryResolveExts = []string{".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".mts", ".cts", ".vue", ".svelte", ".astro"}

// resolveModulePath turns a resolved reference into a file that exists,
// applying the extension and directory-index resolution a bundler does. It
// returns an error when nothing exists there.
func resolveModulePath(p string) (string, error) {
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return filepath.Clean(p), nil
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
	// refPackage: a bare specifier naming a declared npm dependency.
	refPackage
)

// resolveProjectRef resolves a `<script src>` or an import specifier.
//
// A root-relative ref resolves against the project root, a relative one against
// fromDir. A bare specifier is a PACKAGE when the project declares it as a
// dependency and unresolvable otherwise — which is what makes
// `import 'civitai-host.js'` (a bare specifier that resolves to a package, not
// to this file) a non-match rather than a hit.
func resolveProjectRef(projectDir, fromDir, spec string, deps map[string]bool) (string, refKind) {
	// A protocol-relative URL only LOOKS root-relative. Without this, the `/`
	// case below strips one slash and `filepath.Join` cleans the rest, so
	// `//evil.example.com/../civitai-host.js` resolves to EXACTLY the emitter's
	// path and is accepted — measured. Pinned by the "protocol-relative URL
	// that cleans back" corpus case.
	//
	// There is deliberately no `strings.Contains(spec, "://")` companion: no URI
	// scheme can begin with `/`, `./` or `../`, so an `https://…` specifier
	// already falls through to the bare-specifier branch, where it is not a
	// declared dependency and reports refUnresolved — the identical answer.
	if strings.HasPrefix(spec, "//") {
		return "", refUnresolved
	}
	// Vite allows query/hash suffixes (`?raw`, `?url`); they are not part of the
	// path, and an emitter imported as `./civitai-host.js?url` is still the
	// emitter. Pinned by the "?url suffix" accept case.
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

var (
	reScriptSrc = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	// Captures a whole `<script …>…</script>` so an INLINE module's imports can
	// be read. Group 1 is the attribute list, group 2 the body.
	reScriptBlock = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)
	reSrcAttr     = regexp.MustCompile(`(?is)\bsrc\s*=\s*["']`)
	// Captures the SPECIFIER of an import / re-export / require, so it can be
	// resolved. Matching a basename inside the specifier is what let
	// `import '../nonexistent/civitai-host.js'` through.
	reJSImport = regexp.MustCompile(`(?:\bimport\s*\(?\s*|\bfrom\s+|\brequire\(\s*)['"]([^'"]+)['"]`)
)
