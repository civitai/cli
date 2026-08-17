// Package antipattern is the scaffold-currency "born-broken app" gate.
//
// The pins-vs-published guard (internal/scaffold, PR #118) catches a template
// whose `@civitai/*` PIN has fallen behind npm. It does NOT catch a scaffold
// whose CODE still calls a dead/superseded platform endpoint — a template that
// compiles and installs cleanly yet ships an app that is broken the moment a
// user runs it. That is the gap this package closes.
//
// THE MOTIVATING FAILURE (playable-collections, 2026-07): an app fell 9 SDK
// minors behind with THREE broken features because its scaffold used REST
// endpoints the platform had SUPERSEDED with host-mediated SDK hooks:
//   - it fetched `/api/v1/blocks/buzz` — a route that was REMOVED (buzz is now
//     read through the host bridge behind `useBuzzBalance()`), so every buzz
//     read 404'd.
//   - it wrote shared state by POSTing `/api/v1/blocks/shared-storage/*` with a
//     hand-rolled (and wrong) payload shape, instead of the host-mediated
//     shared-storage bridge — so shared writes silently mismatched the contract.
//
// The doctrine (App Blocks "bridge-first"): a browser App block does NOT talk to
// civitai's REST API directly for any surface that has a PAGE-HOST BRIDGE. The
// host (which holds the real user session) mediates those via postMessage, and
// the SDK exposes them as hooks. The authoritative inventory of bridged surfaces
// is civitai's `src/components/AppBlocks/hostHandlerParity.ts` INVENTORY: buzz
// reads, viewer, workflows (submit/estimate/poll/cancel), pickers, app storage,
// shared storage, image upload, wildcard packs. A direct `fetch()` to the REST
// route BEHIND one of those bridges is the anti-pattern.
//
// PRECISION — this is a small CURATED denylist, NOT a blanket "no /api/v1/blocks".
// Several `/api/v1/blocks/*` routes are legitimately REST and have NO bridge:
//   - `/api/v1/blocks/models`, `/api/v1/blocks/images` — public high-volume
//     catalog reads (there is no host bridge; the scaffolds fetch these).
//   - `/api/v1/blocks/dev-token` — the headless dev-token mint (a CLI/dev path,
//     not a runtime block surface).
//   - `/api/v1/blocks/me` — the viewer self-read; its `useViewer()` bridge
//     successor exists but the route STAYS LIVE until the hook publishes and
//     consumers migrate, and the page-money scaffold uses it in its dev harness.
//
// Flagging any of those would false-fail a correct scaffold, so they are
// deliberately absent from the denylist. We only flag routes that are REMOVED or
// whose bridge has fully superseded the REST shape (buzz, shared-storage).
//
// THE DENYLIST IS NOT ONLY ABOUT REST. It already covered a dead TOOL
// (`@civitai/blocks-cli`, a retired npm package). `RESIZE_IFRAME` adds a third
// family — a dead HOST MESSAGE: a block -> host message type the page host does
// not handle, so the code compiles, installs, ships, and does nothing. The raw
// page templates demoed it until #206, and on a `page` surface the host marks it
// N/A. Message-name rules need tighter scoping than route or package rules; see
// `Rule.Exts`.
package antipattern

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/civitai/cli/internal/pkgzip"
)

// Rule is one curated anti-pattern: a regexp that, when it matches a line of a
// scaffolded app's source, means the scaffold is using a dead/discouraged
// platform surface. Each rule carries the concrete replacement so the failure
// message tells the author exactly what to use instead.
type Rule struct {
	// ID is a short stable identifier used in messages and tests.
	ID string
	// What is a human phrase naming the offending pattern.
	What string
	// Pattern matches the offending token on a single line.
	Pattern *regexp.Regexp
	// Replacement is the concrete thing to use instead (a hook / the Go CLI).
	Replacement string
	// Exts, when non-nil, restricts this rule to those file extensions
	// (lowercase, dotted). Nil means every extension in scannedExts.
	//
	// It exists because the default corpus deliberately includes prose and data
	// (.md, .txt, .json) so a documented dead workflow or a dead pin is caught —
	// right for a rule matching a PACKAGE NAME or a URL, wrong for one matching
	// a MESSAGE NAME, where quoting the token in a sentence, or using it as a
	// JSON key in a handler table, is ordinary writing rather than a call.
	Exts map[string]bool
}

// codeExts is the Exts value for rules that must only fire on executable code.
var codeExts = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".html": true, ".htm": true,
}

// appliesTo reports whether the rule is evaluated against a file with the given
// lowercased, dotted extension.
func (r Rule) appliesTo(ext string) bool { return r.Exts == nil || r.Exts[ext] }

// Rules is the curated denylist. Keep it SMALL and PRECISE — every entry must be
// a surface the platform has REMOVED or fully superseded with a host bridge, and
// must NOT match a legitimately-REST endpoint (see the package doc). Each entry
// links to its replacement hook.
func Rules() []Rule {
	return []Rule{
		{
			// REMOVED route. Buzz balance is read through the page-host bridge
			// (GET_BUZZ_BALANCE in hostHandlerParity.ts) exposed as a hook; the
			// REST route no longer exists, so a scaffold that fetches it 404s.
			// The terminating char class keeps this to the whole `/buzz` path
			// segment (won't match a hypothetical `/buzz-*` sibling), and we scan
			// line-by-line so `$` anchors end-of-line.
			ID:          "buzz-rest",
			What:        "fetch of the REMOVED /api/v1/blocks/buzz REST route",
			Pattern:     regexp.MustCompile("/api/v1/blocks/buzz(?:[/?'\"\\x60\\s),;]|$)"),
			Replacement: "read buzz through the host bridge — useBuzzBalance() / useBuzzWorkflow() from @civitai/blocks-react (the /api/v1/blocks/buzz route was REMOVED)",
		},
		{
			// SUPERSEDED route. Cross-user/app-global storage is host-mediated
			// via the SHARED_* bridges (SHARED_APPEND / SHARED_GET_COUNT /
			// SHARED_VOTE / … in hostHandlerParity.ts). The REST route is the
			// exact "wrong payload shape" trap the playable-collections app hit.
			ID:          "shared-storage-rest",
			What:        "direct fetch of the superseded /api/v1/blocks/shared-storage REST route",
			Pattern:     regexp.MustCompile("/api/v1/blocks/shared-storage"),
			Replacement: "use the host-mediated shared-storage hooks from @civitai/blocks-react (the SHARED_APPEND / SHARED_GET_COUNT / SHARED_VOTE bridges), not the REST route — the REST payload shape is superseded",
		},
		{
			// DEPRECATED tool. The npm `@civitai/blocks-cli` package is retired in
			// favour of this Go CLI; a scaffold that references it teaches authors
			// a dead workflow. The trailing boundary keeps this to the EXACT
			// package name — a version suffix (`@1.2.3`), a subpath (`/bin`), a
			// quote, or end-of-line terminates it — so a hypothetical sibling like
			// `@civitai/blocks-client` is NOT false-flagged.
			ID:          "deprecated-blocks-cli",
			What:        "reference to the deprecated @civitai/blocks-cli npm package",
			Pattern:     regexp.MustCompile("@civitai/blocks-cli(?:[@/'\"\\x60\\s),;]|$)"),
			Replacement: "use the Go civitai CLI (`civitai app <init|submit>`); the @civitai/blocks-cli npm package is deprecated",
		},
		{
			// DEAD MESSAGE (not a route). `hostHandlerParity.ts` marks
			// RESIZE_IFRAME **N/A for PageBlockHost** — "a page block fills the
			// surface and does NOT size to content" — so a page app that posts it
			// is posting into a handler that does not exist. It is inert rather
			// than broken, which is exactly why it survives every local check and
			// teaches the next message the author writes to look like this one.
			// Both raw page templates demoed it until #206 removed them; anything
			// scaffolded before that still carries it.
			//
			// SCOPE (read before widening): this package scans SCAFFOLDED APP
			// TREES, and every template this CLI ships declares a `page` surface
			// (scaffold.Template.ReadyAckPath's doc, and the manifest evidence in
			// ready_ack_contract_test.go). The rule therefore assumes a page
			// surface rather than reading the manifest — Rule is a per-line regex
			// with no manifest context by construction. If a non-page (model-slot)
			// template ever ships, this rule needs manifest awareness, because
			// RESIZE_IFRAME is honoured on the surfaces that DO size to content.
			//
			// TWO precision devices, because a message NAME is a far more
			// quotable token than a route or a package name:
			//   1. `Exts: codeExts` — prose and data are not calls. Double-quoting
			//      a message name in a sentence is ordinary English, and a JSON
			//      handler table `{"RESIZE_IFRAME": …}` is a data structure, not
			//      a post. Without this the rule fails a README that says "do not
			//      post RESIZE_IFRAME".
			//   2. A MATCHING ' or " pair, deliberately excluding the backtick
			//      form — markdown backticks are how our own docs name it, and a
			//      mismatched pair (`'RESIZE_IFRAME"`) is not a JS string literal
			//      at all. A JS template literal holding a constant message type
			//      is the (vanishingly rare) cost of excluding backticks.
			// `What` says "reference to", not "postMessage of": the pattern
			// matches the quoted token wherever it appears in code — a dispatch
			// table key, a comparison, a constant — and all of those are dead on
			// a page surface. Claiming it matched a postMessage call would be a
			// claim the regex does not make.
			ID:          "resize-iframe-page",
			What:        "reference to RESIZE_IFRAME, a message the page host does not handle",
			Exts:        codeExts,
			Pattern:     regexp.MustCompile(`'RESIZE_IFRAME'|"RESIZE_IFRAME"`),
			Replacement: "delete it — hostHandlerParity.ts marks RESIZE_IFRAME N/A for PageBlockHost (a page block fills the host surface and does NOT size to content), so the message is dead code. The one message a page app must send is BLOCK_READY: keep the vendored civitai-host.js emitter `civitai app init` ships, or adopt @civitai/blocks-react (whose transport acks internally) — never both",
		},
	}
}

// Finding is a single rule match at a location in the scanned tree.
type Finding struct {
	File string // path relative to the scanned root
	Line int    // 1-based line number
	Text string // the trimmed source line
	Rule Rule
}

// scannedExts are the source extensions we grep. Anything else (images, lock
// files, fonts) is skipped. package.json / README are included so a dead pin or
// a documented dead workflow is caught too.
var scannedExts = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".mjs": true, ".cjs": true, ".json": true,
	".html": true, ".htm": true, ".css": true,
	".md": true, ".txt": true,
}

// ScanDir walks dir and returns every anti-pattern Finding in its source files,
// sorted by file then line. A clean tree returns nil, nil.
//
// 🔴 WHAT IT SCANS IS EXACTLY WHAT THE PACKAGER SHIPS — pkgzip owns that rule and
// this package does NOT keep a copy of it (issue #435, part 3). It used to: an
// 8-name `skipDirs` (node_modules, .git, dist, build, coverage, out, .vite,
// .next) against pkgzip's fixed names plus its two directory PATTERN rules
// (dotenv-shaped and archive-shaped, both case-insensitive). So the walk
// descended into `.venv/`, `.pnpm-store/`, `.turbo/`, `.cache/`, `.env.d/`,
// `old.zip/` and the rest, and a finding there is one an author cannot act on:
// the code it names is not in the bundle, and most of it is not theirs.
// TestScanDirSkipsExactlyWhatThePackagerDrops is the ledger — it builds its
// roster FROM pkgzip.ExcludedNames(), so neither side can move alone. No count
// is restated here on purpose; both lists are read, never quoted.
//
// A duplicated predicate is the drift that produced #409 and #420 — one shape of
// a name fixed, the other left behind — so this delegates rather than mirrors.
// The FILE rule (pkgzip.IsExcludedFile) is applied too, ahead of the scannedExts
// test: today scannedExts hides most of that divergence by accident (`.envrc`,
// `db.env` and `old.zip` all have extensions it does not scan), but `.env.json`
// does not, and "a finding is in a file that ships" should hold structurally
// rather than by coincidence of the extension table.
//
// scannedExts stays exactly as it is. It answers a different question — which
// file types are worth grepping — and is not part of this rule.
//
// 🔴 THE ROOT IS NEVER SKIPPED, mirroring pkgzip.Build's own `p == dir` guard.
// Without it a project directory that happens to be named `dist`, `build` or
// `x.zip` scans NOTHING and reports a clean tree — a reassuring zero from a
// walk that never opened a file. Scanning a tree the packager would drop is
// correct here: the caller named that tree, so it is the subject, not content.
func ScanDir(dir string) ([]Finding, error) {
	rules := Rules()
	var findings []Finding

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// The scanned root is the subject of the scan, not content inside
			// it — see the doc comment.
			if path == dir {
				return nil
			}
			if pkgzip.IsExcluded(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		// The packager drops these by NAME regardless of extension, so a finding
		// in one names code that will not be in the bundle.
		if pkgzip.IsExcludedFile(d.Name()) {
			return nil
		}
		if !scannedExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		for i, line := range strings.Split(string(data), "\n") {
			for _, r := range rules {
				if !r.appliesTo(ext) {
					continue
				}
				if r.Pattern.MatchString(line) {
					findings = append(findings, Finding{
						File: rel,
						Line: i + 1,
						Text: strings.TrimSpace(line),
						Rule: r,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Rule.ID < findings[j].Rule.ID
	})
	return findings, nil
}

// Format renders findings as a human-readable, CI-friendly report. Empty
// findings returns "".
func Format(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "scaffold-currency: %d anti-pattern(s) found in the scaffolded app:\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "  %s:%d — %s [%s]\n", f.File, f.Line, f.Rule.What, f.Rule.ID)
		fmt.Fprintf(&b, "      line: %s\n", f.Text)
		fmt.Fprintf(&b, "      fix:  %s\n\n", f.Rule.Replacement)
	}
	return strings.TrimRight(b.String(), "\n")
}
