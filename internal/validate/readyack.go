package validate

// readyack.go implements the PAGE-APP READY-ACK advisory.
//
// WHY THIS EXISTS. A `page` app is not revealed by the host until it posts
// `BLOCK_READY` — that handler in `PageBlockHost.tsx` is the only transition
// into the host's `ready` state. An app that never posts it renders perfectly
// everywhere you can look locally and is replaced by a visible failure card in
// the real host once the bounded init retries run out (~37s). That is issue
// #206, and `4018e2c` fixed the `static` and `page-vite` TEMPLATES — but every
// app scaffolded before that commit is still broken, and nothing tells its
// author. This check is the thing that tells them.
//
// 🔴 IT IS A WARNING, NOT AN ERROR, AND THAT IS DELIBERATE.
// The lockfile check next door (lockfile.go) earns hard-error status because the
// platform PROVABLY fails: the build recipe runs `npm ci` and dies. Nothing here
// is provable. This check infers RUNTIME behaviour from STATIC TEXT, and there
// are real, correct projects it cannot read: an ack that arrives through a
// bundled dependency, a framework wrapper, a code-split chunk, a generated file,
// or simply a file extension this scan does not open. Hard-failing on a
// heuristic is the exact false-warning-at-a-correct-project failure the
// dev-tunnel preflight (AGENTS.md item 10) spent four measured corrections
// avoiding — and unlike that one, `--strict` here would turn it into a build
// break in somebody's CI. `Result.Warnings` costs an author nothing to ignore
// and still puts the sentence in front of them.
//
// EVIDENCE ORDER — THE DEPENDENCY IS CHECKED FIRST, AND THAT IS LOAD-BEARING.
// For a project depending on `@civitai/blocks-react`, the ack comes from the
// SDK's `IframeTransport` and the literal `BLOCK_READY` NEVER APPEARS in `src/`.
// A source scan alone therefore warns at every correct page-money app on the
// platform, which is the worst possible outcome for advisory output: it teaches
// authors that this warning is noise. So package.json is read first and a
// dependency that ACKS ends the check.
//
// 🔴 "A dependency that acks" is an EXACT SET (blockproto.PackageAcksReady), not
// the `@civitai/` scope. Four of the six published first-party packages do not
// ack at all — `@civitai/theme` and `@civitai/components` are plain CSS, and are
// exactly what a hand-written no-build page app installs — so a scope test goes
// SILENT on a genuinely broken app. Measured per package in blockproto, and
// reproduced live on a `static` scaffold with the emitter deleted. Do not
// re-widen this to a prefix.
//
// WHAT IT DOES NOT PROVE. That the ack FIRES. No static check can — an emitter
// with an inverted `event.source` guard satisfies every assertion here (that is
// what the scaffold's Guard B exists for, see AGENTS.md item 11). This check
// answers one narrower question: does anything in this project's source so much
// as MENTION the message, outside a comment. A "no" is strong evidence of the
// #206 shape; a "yes" is weak evidence of correctness, and is treated as
// conclusive anyway, because being generous here costs a missed warning while
// being strict costs a false one.
//
// SCOPE — IT ONLY FIRES WHERE IT CAN OBSERVE, AND "OBSERVE" MEANS THE WHOLE
// TREE. Mirroring lockfile.go's early-out shape: no `page` surface, no check;
// unreadable package.json, no check. Beyond that, the scan concludes "this app
// never acks" ONLY if it read every source file it could reach and found
// nothing. Anything that leaves a gap — an unresolvable symlink, a read error, a
// file over the size cap, the file-count budget — reports UNOBSERVABLE and stays
// silent. A partial scan that says "absent" is manufacturing advice from a gap.
//
// 🔴 SKIPPING IS A COST DECISION, NEVER A CORRECTNESS ONE, because this is a
// PRESENCE check: scanning an extra directory can only ever ADD ack evidence,
// while skipping one can only ever CREATE a false warning. That asymmetry is why
// the manifest's `outputDir` is NOT skipped (it was, and it was wrong — a
// perfectly valid `"outputDir": "src"` on a page-vite app skipped the directory
// holding the emitter and warned at a correct project; `"outputDir": "."`
// skipped the entire tree). What remains is a fixed list of names that are
// never source — dependencies, VCS metadata, and the conventional build
// directories — chosen for cost. The residual trade is accepted and stated: a
// stale committed build under a NON-conventional output directory can retain an
// ack the source has lost, silencing a genuinely broken app. That is a false
// negative, and false negatives are the cheap direction here.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
)

// readyAckType is the message the page host waits for. It is the same literal
// blockproto's emitter posts; the emitter is the authority for the SHAPE, this
// is only the token we look for.
const readyAckType = "BLOCK_READY"

// readyAckSourceExts are the extensions a hand-written emitter can live in.
// Deliberately CODE ONLY — `.md` is excluded because a README that explains the
// handshake is not an implementation of it, and both shipped scaffold READMEs
// describe `BLOCK_READY` at length. Including docs would silence the check for
// exactly the apps it exists to find.
var readyAckSourceExts = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
	".html": true, ".htm": true,
	".vue": true, ".svelte": true, ".astro": true,
}

// readyAckMarkupExts are the subset whose comments are `<!-- -->`. HTML comment
// stripping is applied ONLY to these.
//
// 🔴 Applying it to `.js`/`.ts` was a false-warning source: `stripHTMLComments`
// has no string awareness, so a perfectly ordinary `var OPEN = '<!--';` in a
// sanitiser started a "comment" that ran to end of file and deleted the emitter
// below it. Reproduced live on a `static` scaffold. JS has no HTML comments
// worth stripping here — `//` and `/* */` cover it, and Annex B's HTML-like
// comment form is vanishingly rare next to the literal appearing in a string.
var readyAckMarkupExts = map[string]bool{
	".html": true, ".htm": true, ".vue": true, ".svelte": true, ".astro": true,
}

// readyAckSkipDirs are never descended into. These names are never source:
// dependencies, VCS metadata, and the conventional build directories. The list
// is a COST decision (see the header) — every entry can only cost a false
// warning, so it is kept short and conventional rather than expanded to every
// directory that might be large. `vendor` and `public` are deliberately ABSENT:
// both routinely hold hand-written source (a vendored emitter, Vite's static
// `public/`), and cost is bounded by the caps below instead.
var readyAckSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"out": true, "coverage": true, ".vite": true, ".next": true,
	".svelte-kit": true, ".output": true,
}

// Scan caps. `validate` used to be a manifest-only read; walking a project tree
// is new cost, and an app that commits a large bundle or a huge vendored tree
// must not turn it into a memory event (measured before these existed: 20,008
// files with one 88 MB .js peaked at 316 MB RSS).
//
// Hitting either cap makes the scan UNOBSERVABLE, not "absent" — we stopped
// reading, so we do not know. That is deliberately the direction that stays
// silent.
const (
	maxAckScanFiles = 5000
	maxAckFileBytes = 2 << 20 // 2 MiB
)

// readyAckChecks returns the ready-ack advisory for the project in dir, or nil.
//
// It runs in the projectState branch of validateDir (beside lockfileChecks) and
// NOT in warningChecks: warningChecks is reached under ManifestOnly, which
// `civitai app init` uses to self-check the template it just wrote, and a check
// that reads `src/` has no business running there.
func readyAckChecks(dir string, generic any) []string {
	if !declaresPage(generic) {
		return nil
	}
	// The dependency is checked FIRST — see the file header. `unknown` means the
	// package.json is there but unreadable, i.e. we cannot answer the question
	// that decides whether a source scan is even meaningful.
	switch sdkDependency(dir) {
	case sdkPresent, sdkUnknown:
		return nil
	}
	// Only a scan that read the WHOLE reachable tree and found nothing is
	// evidence of absence.
	if scanForReadyAck(dir) != ackAbsent {
		return nil
	}
	return []string{readyAckAdvice}
}

// declaresPage reports whether the manifest declares a non-null `page` surface.
// `"page": null` is not a page app; an absent key is not either.
func declaresPage(generic any) bool {
	m, ok := generic.(map[string]any)
	if !ok {
		return false
	}
	v, present := m["page"]
	return present && v != nil
}

// sdkState is the three-valued answer to "does this project depend on a
// first-party package". The third value is the point: "we could not tell" must
// not be collapsed into "no", which is what would warn at a project whose
// package.json this CLI simply failed to parse.
type sdkState int

const (
	sdkAbsent sdkState = iota
	sdkPresent
	sdkUnknown
)

func sdkDependency(dir string) sdkState {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		if os.IsNotExist(err) {
			// No package.json at all: a static, no-build app. It installs
			// nothing, so it cannot be acking through a dependency — the source
			// scan is the whole answer for it.
			return sdkAbsent
		}
		return sdkUnknown
	}
	// json.RawMessage values, not strings: a package.json with an unusual
	// (non-string) version entry must not make the whole decode fail and warn.
	var pkg struct {
		Dependencies     map[string]json.RawMessage `json:"dependencies"`
		DevDependencies  map[string]json.RawMessage `json:"devDependencies"`
		PeerDependencies map[string]json.RawMessage `json:"peerDependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return sdkUnknown
	}
	for _, set := range []map[string]json.RawMessage{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies} {
		for name := range set {
			if blockproto.PackageAcksReady(name) {
				return sdkPresent
			}
		}
	}
	return sdkAbsent
}

// ackScanResult is the THREE-valued outcome of the source scan. Two values
// would collapse "we did not look" into "it is not there", which is the whole
// class of bug this check has to avoid.
type ackScanResult int

const (
	// ackFound: a source file mentions the message outside a comment.
	ackFound ackScanResult = iota
	// ackAbsent: every reachable source file was read, and none mentions it.
	// This is the ONLY result that produces a warning.
	ackAbsent
	// ackUnobservable: the scan left a gap — an unresolvable symlink, a read
	// error, a file over the size cap, the file budget, or simply no source
	// files at all.
	ackUnobservable
)

// scanForReadyAck walks dir looking for the ready-ack message in source.
//
// 🔴 READING NOTHING IS NOT THE SAME AS FINDING NOTHING. A zero-hit scan over a
// zero-file tree is indistinguishable from a scanner wired to nothing — a broken
// extension table, an over-eager skip list, or `validate` pointed at a directory
// that holds a manifest and no checkout (the shape of every manifest-only
// fixture in this package). So "no files read" is ackUnobservable, not ackAbsent.
//
// 🔴 The comment strip is load-bearing, not tidiness. Both shipped SDK-free
// templates carry a source COMMENT naming `BLOCK_READY`
// (`static/app.js`, `page-vite/src/App.jsx`: "The ONE message a page app must
// send is `BLOCK_READY`"), and those comments SURVIVE deleting `civitai-host.js`.
// Without stripping, the exact repair this check exists to demand — restore the
// emitter — would already look satisfied, and the check would be inert on the
// one population it was written for. Measured both ways.
//
// 🔴 SYMLINKED DIRECTORIES ARE FOLLOWED. filepath.WalkDir does not follow them,
// and it does not report them as directories either, so a monorepo whose `src`
// is a symlink into a shared package had its ENTIRE source tree skipped and
// warned at a correct project — reproduced live. Following is also the safe
// direction for a presence check (more files can only add evidence). Cycles are
// bounded by a visited set keyed on the RESOLVED path, and total work by the
// caps above.
func scanForReadyAck(dir string) ackScanResult {
	s := &ackScanner{visited: map[string]bool{}}
	s.walk(dir)
	switch {
	case s.found:
		return ackFound
	case s.partial || s.files == 0:
		return ackUnobservable
	default:
		return ackAbsent
	}
}

type ackScanner struct {
	visited map[string]bool
	files   int
	found   bool
	// partial records that something in the tree could not be read. It is
	// sticky: once set, the answer can never be ackAbsent.
	partial bool
}

func (s *ackScanner) walk(dir string) {
	if s.found || s.partial {
		return
	}
	// Resolve before recording: two paths that reach the same directory (a
	// symlink and its target) must count once, or a self-referential link
	// recurses forever.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		s.partial = true
		return
	}
	if s.visited[real] {
		return
	}
	s.visited[real] = true

	entries, err := os.ReadDir(dir)
	if err != nil {
		s.partial = true
		return
	}
	for _, e := range entries {
		if s.found || s.partial {
			return
		}
		path := filepath.Join(dir, e.Name())
		// os.Stat FOLLOWS symlinks — that is the point: a symlinked source
		// directory must be walked as a directory. A dangling link fails here
		// and correctly makes the scan partial.
		info, err := os.Stat(path)
		if err != nil {
			s.partial = true
			return
		}
		if info.IsDir() {
			// Note the skip list is applied to ENTRIES only, never to the root
			// we were handed — no skip rule can remove the whole tree.
			if readyAckSkipDirs[e.Name()] {
				continue
			}
			s.walk(path)
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !readyAckSourceExts[ext] {
			continue
		}
		if info.Size() > maxAckFileBytes || s.files >= maxAckScanFiles {
			s.partial = true
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			s.partial = true
			return
		}
		s.files++
		if strings.Contains(stripComments(string(data), ext), readyAckType) {
			s.found = true
			return
		}
	}
}

// stripComments removes comments from src, given its file extension. JS
// line/block comments are always stripped, leaving string and template literals
// intact (so `'https://x'` is not read as the start of a comment, and an emitter
// that posts from inside a string still counts). HTML comments are stripped only
// for markup files — see readyAckMarkupExts for why that gate exists.
func stripComments(src, ext string) string {
	if readyAckMarkupExts[strings.ToLower(ext)] {
		src = stripHTMLComments(src)
	}
	return stripJSComments(src)
}

func stripHTMLComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
				// 🔴 UNTERMINATED. Keep the remainder VERBATIM rather than
				// discarding it. Dropping the tail is lossy in the expensive
				// direction — everything below an unclosed `<!--` disappears,
				// including an emitter, and the author gets a warning about a
				// correct project. There is no upside to the lossy branch: text
				// after an unclosed marker is not evidence of a comment, it is
				// evidence the file is not what we assumed.
				b.WriteString(src[i:])
				return b.String()
			}
			i += 4 + end + 3
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

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

// readyAckAdvice names the concrete next command, per the house rule that a
// message must tell the author what to run. It also states its own evidentiary
// limit — an author who knows their ack comes from a bundled dependency needs to
// be able to recognise this as a false alarm without reading the source of the
// CLI.
var readyAckAdvice = "the manifest declares a \"page\" surface but nothing in this project's source posts " +
	readyAckType + " — the host will not reveal a page app until it acks the host's BLOCK_INIT, so an app " +
	"that never sends it renders fine locally and is replaced by a failure card in the real host once the " +
	"bounded init retries run out (issue #206). Apps scaffolded before that fix ship no emitter: run " +
	"`civitai app init` into a scratch directory and copy its `" + blockproto.ReadyAckFilename + "` " +
	"into this project, loading it from index.html (a <script src>) or from the entry module — or adopt " +
	"`@civitai/blocks-react`, whose iframe transport acks for you (never both: whichever answers the first " +
	"BLOCK_INIT cancels the host's retry). Note `@civitai/app-sdk` alone does NOT ack — it is the " +
	"server-side SDK and no runtime code in it posts " + readyAckType + ". This is a source-text check, so " +
	"it is wrong about a project whose ack arrives from a bundled dependency or a file type it does not " +
	"read; it is advisory only and never fails `civitai app validate` unless you pass --strict"
