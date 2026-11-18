// Package antipattern is the scaffold-currency "born-broken app" gate.
//
// The pins-vs-published guard (internal/scaffold, PR #118) catches a template
// whose `@civitai/*` PIN has fallen behind npm. It does NOT catch a scaffold
// whose CODE still uses a dead platform surface — a template that compiles and
// installs cleanly yet ships an app that does not work. That is the gap this
// package closes: a small CURATED denylist, matched line by line.
//
// WHAT THE DENYLIST HOLDS — two families:
//   - a dead TOOL: `@civitai/blocks-cli`, an npm package deprecated in favour
//     of this CLI.
//   - a dead HOST MESSAGE: `RESIZE_IFRAME` on a page surface, where the host
//     ignores it — the code compiles, ships, and does nothing. Message-name
//     rules need tighter scoping than package rules; see `Rule.Exts`.
//
// WHAT IT DOES NOT HOLD — any `/api/v1/blocks/*` REST route. The platform's
// direction for App Blocks is REST for data reads and writes, and the
// postMessage bridge only for capabilities that need host chrome or the host
// session UI (pickers, Buzz purchase, image upload, publish/post,
// sign-in/consent, token refresh). So a direct call to a live route is the
// supported path, not an anti-pattern, and no rule here flags one.
//
// The inverse holds too: the bridge hooks that still exist
// (`useBuzzBalance()`, `useSharedStorage()`, …) keep working and shipped apps
// use them, so no rule flags the bridge either. Both transports are valid.
//
// Two route rules used to live here and were removed, because a REQUIRED CI
// gate steering authors off the supported path is worse than no gate:
//   - `shared-storage-rest` — removed on the 2026-10-04 decision to
//     consolidate shared storage on REST. Its motivating bug (an app POSTing a
//     hand-rolled, wrong payload) was real, but it was a property of the
//     payload, not of the transport.
//   - `buzz-rest` — it called `/api/v1/blocks/buzz` a REMOVED route. The route
//     was deleted and then restored (civitai #5051, 2026-09-23) and is live.
//
// A route rule is only right for a route that is GONE, checked against the
// server — never against a list of routes copied into this repo, which is how
// `buzz-rest` went stale. Remove it if the route comes back.
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
// scaffolded app's source, means the scaffold is using a dead platform
// surface. Each rule carries the concrete replacement so the failure
// message tells the author exactly what to use instead.
type Rule struct {
	// ID is a short stable identifier used in messages and tests.
	ID string
	// What is a human phrase naming the offending pattern.
	What string
	// Pattern matches the offending token on a single line.
	Pattern *regexp.Regexp
	// Replacement is the concrete thing to do instead.
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
// a surface that is dead for the app using it, and must NOT match a live REST
// route or a working bridge hook (see the package doc). Each entry carries its
// replacement.
func Rules() []Rule {
	return []Rule{
		// NO REST-ROUTE RULE HERE, DELIBERATELY. Two lived at this spot —
		// `buzz-rest` and `shared-storage-rest` — and both were removed: the
		// routes are live and REST is the supported path for data (package doc).
		// TestLegitRoutesNeverMatchAnyRule, TestSharedStorageRestIsAccepted and
		// the scan-clean `buzz-rest-ok` fixture are the guards. The shared-storage pattern was also ineffective: against a
		// real migrated app it matched 1 comment line and 0 call sites, which
		// read `'blocks/shared-storage/list'` because `@civitai/sdk`'s
		// `app.site.get` prepends `/api/v1`.
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
// The FILE rule is applied too, ahead of the scannedExts test: today scannedExts
// hides most of that divergence by accident (`.envrc`, `db.env` and `old.zip`
// all have extensions it does not scan), but `.env.json` does not, and "a
// finding is in a file that ships" should hold structurally rather than by
// coincidence of the extension table.
//
// 🔴 THAT FILE RULE IS ASKED OF THE WALK'S PROJECT-RELATIVE PATH, NOT OF THE BASE
// NAME (#466). #461 scoped the packager's dotenv allow-list to the project root,
// which made "is this file dropped?" a question a base name can no longer answer:
// `.env.production` ships and `app/.env.production` does not. This walk called
// pkgzip.IsExcludedFile — the ROOT answer — at every depth, so for the three
// allow-listed names it asked the wrong question everywhere below the root. It
// was inert (none of `.example`, `.sample`, `.production` is in scannedExts, so
// the extension gate skipped those files either way) and gated by this comment
// alone. pkgzip.IsExcludedPath takes the `rel` the walk already computes and is
// the form Build agrees with, so the divergence is removed rather than pinned.
// The correction can only ever skip MORE — and only files the packager drops.
// TestScanDirJudgesKeptDotenvNamesByDepth is the behavioural guard; it widens
// scannedExts for its own duration to make the predicate reachable.
//
// scannedExts stays exactly as it is, and is now free to move: it answers a
// different question — which file types are worth grepping — and with the
// depth-aware call above, growing it can no longer make this walk read a file
// the packager drops.
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
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		// The packager drops these regardless of extension, so a finding in one
		// names code that will not be in the bundle. Judged on the PATH, not the
		// base name — see the doc comment (#466).
		judged := filepath.ToSlash(rel)
		if relErr != nil {
			// Unreachable through WalkDir, which only ever hands us paths under
			// dir. If it happened, `rel` is the ABSOLUTE path and its leading
			// components are not project directories — a `/home/…/build/…`
			// segment would exclude the file for a reason that has nothing to do
			// with the project. Fall back to the base name, i.e. the root answer
			// this call gave before #466.
			judged = d.Name()
		}
		if pkgzip.IsExcludedPath(judged) {
			return nil
		}
		if !scannedExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
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
