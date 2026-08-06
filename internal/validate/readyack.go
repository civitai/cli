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
// authors that this warning is noise. So package.json is read first and an
// `@civitai/*` dependency ENDS the check.
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
// SCOPE — IT ONLY FIRES WHERE IT CAN OBSERVE. Mirroring lockfile.go's early-out
// shape: no `page` surface, no check. Unreadable package.json, no check.
// Unwalkable tree, no check. And it reads SOURCE only — never `outputDir`,
// which does not exist before the first build and is gitignored, so its absence
// says nothing. A check that cannot observe returns NO findings rather than
// manufacturing advice.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/civitai/cli/internal/blockproto"
	"github.com/civitai/cli/internal/manifest"
)

// readyAckType is the message the page host waits for. It is the same literal
// blockproto's emitter posts; the emitter is the authority for the SHAPE, this
// is only the token we look for.
const readyAckType = "BLOCK_READY"

// civitaiDepPrefix marks a first-party package. Any of them is taken as
// evidence that a real transport is in play — `@civitai/blocks-react` is the
// one that acks today, but a project pulling `@civitai/app-sdk` is on the SDK
// path too, and a false negative is the cheap direction.
//
// This deliberately mirrors `civitaiSDKDeps` in
// internal/scaffold/ready_ack_contract_test.go, which decides the same question
// for the templates. Keep them agreeing: they answer "does this project's ack
// come from the SDK", and disagreeing would mean the CLI warns at a shape its
// own scaffold guard considers correct.
const civitaiDepPrefix = "@civitai/"

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

// readyAckSkipDirs are never descended into: dependencies, VCS metadata, and
// build output. The manifest's own outputDir is skipped on top of these.
var readyAckSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"out": true, "coverage": true, ".vite": true, ".next": true,
	".svelte-kit": true, ".output": true,
}

// readyAckChecks returns the ready-ack advisory for the project in dir, or nil.
//
// It runs in the projectState branch of validateDir (beside lockfileChecks) and
// NOT in warningChecks: warningChecks is reached under ManifestOnly, which
// `civitai app init` uses to self-check the template it just wrote, and a check
// that reads `src/` has no business running there.
func readyAckChecks(dir string, generic any, m *manifest.Manifest) []string {
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
	if emitsReadyAck(dir, m.OutputDir) {
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
			if strings.HasPrefix(name, civitaiDepPrefix) {
				return sdkPresent
			}
		}
	}
	return sdkAbsent
}

// emitsReadyAck reports whether any source file under dir mentions the ready-ack
// message OUTSIDE a comment.
//
// 🔴 READING NOTHING IS NOT THE SAME AS FINDING NOTHING, and this returns TRUE
// (no finding) when it opened ZERO source files. That is this check's own
// positive control, inlined: a zero-hit scan over a zero-file tree is
// indistinguishable from a scanner wired to nothing — a broken extension table,
// an over-eager skip list, or `validate` pointed at a directory that holds a
// manifest and no checkout (which is exactly the shape of every manifest-only
// fixture in this package). Concluding "this app never acks" from a scan that
// never read a line is manufacturing advice from an absence.
//
// 🔴 The comment strip is load-bearing, not tidiness. Both shipped SDK-free
// templates carry a source COMMENT naming `BLOCK_READY`
// (`static/app.js`, `page-vite/src/App.jsx`: "The ONE message a page app must
// send is `BLOCK_READY`"), and those comments SURVIVE deleting `civitai-host.js`.
// Without stripping, the exact repair this check exists to demand — restore the
// emitter — would already look satisfied, and the check would be inert on the
// one population it was written for. Measured: with the strip, deleting
// civitai-host.js from a freshly scaffolded static app makes the warning fire;
// without it, it does not.
//
// An unwalkable/unreadable tree reports TRUE (i.e. "no finding"): we could not
// observe, so we say nothing.
func emitsReadyAck(dir, outputDir string) bool {
	skipOut := ""
	if out := strings.TrimSpace(outputDir); out != "" {
		skipOut = filepath.Clean(filepath.Join(dir, filepath.FromSlash(out)))
	}

	found := false
	scanned := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if readyAckSkipDirs[d.Name()] || (skipOut != "" && filepath.Clean(path) == skipOut) {
				return fs.SkipDir
			}
			return nil
		}
		if !readyAckSourceExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		if strings.Contains(stripComments(string(data)), readyAckType) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		// Could not observe the tree — do not manufacture advice.
		return true
	}
	if scanned == 0 {
		return true
	}
	return found
}

// stripComments removes HTML comments and then JS line/block comments, leaving
// string and template literals intact (so `'https://x'` is not read as the start
// of a comment, and an emitter that posts from inside a string still counts).
func stripComments(src string) string {
	return stripJSComments(stripHTMLComments(src))
}

func stripHTMLComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], "<!--") {
			end := strings.Index(src[i+4:], "-->")
			if end < 0 {
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
	"`@civitai/blocks-react`, whose transport acks for you (never both: whichever answers the first " +
	"BLOCK_INIT cancels the host's retry). This is a source-text check, so it is wrong about a project " +
	"whose ack arrives from a bundled dependency or a file type it does not read; it never inspects " +
	"outputDir, which is not built yet"
