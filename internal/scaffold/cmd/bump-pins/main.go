// Command bump-pins keeps the scaffold's @civitai/* pins in lockstep with npm.
//
// The page-money template pins `@civitai/app-sdk` and `@civitai/blocks-react`
// with pre-1.0 carets that LOCK THE MINOR, so every time those packages publish
// a new minor the pins-vs-published guard (TestScaffoldPinsSatisfyPublished)
// goes red until a human hand-bumps the pins in THREE places. This command
// automates that hand-fix: for each distinct @civitai/* package pinned in the
// template it fetches npm's `latest`, computes the desired caret pin
// (scaffold.DesiredPin), and — if the current literal differs — rewrites it in
// all three literal sites:
//
//  1. templates/page-money/package.json.tmpl   ("@civitai/<pkg>": "^X.Y.Z")
//  2. templates/page-money/README.md.tmpl       (@civitai/<pkg>@^X.Y.Z prose)
//  3. scaffold_test.go                          (the mustContain assertion)
//
// 🔴 It rewrites the LITERAL pins (it does NOT template them): the guard reads
// the raw `^X.Y.Z` bytes out of the .tmpl, so templating would blind it.
//
// It is idempotent (a no-op exit 0 when every pin is already current) and
// reuses the guard's shared logic (scaffold.CivitaiPinRe / FetchNpmLatest /
// DesiredPin) so the bumper and the guard agree by construction.
//
// Usage:
//
//	go run ./internal/scaffold/cmd/bump-pins            # rewrite stale pins in place
//	go run ./internal/scaffold/cmd/bump-pins --check    # exit 1 if a bump is NEEDED (write nothing)
//	go run ./internal/scaffold/cmd/bump-pins --dir DIR  # scaffold package dir (default internal/scaffold)
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/civitai/cli/internal/scaffold"
)

// The three literal pin sites, relative to the scaffold package dir.
const (
	pkgJSONFile = "templates/page-money/package.json.tmpl"
	readmeFile  = "templates/page-money/README.md.tmpl"
	testFile    = "scaffold_test.go"
)

// fileKind selects the pin's literal shape in a given file.
type fileKind int

const (
	// jsonPin: `"@civitai/<pkg>": "^X.Y.Z"` (package.json.tmpl + the
	// scaffold_test.go mustContain assertion — byte-identical form).
	jsonPin fileKind = iota
	// prosePin: `@civitai/<pkg>@^X.Y.Z` (README.md.tmpl prose).
	prosePin
)

type site struct {
	file string
	kind fileKind
}

var sites = []site{
	{pkgJSONFile, jsonPin},
	{readmeFile, prosePin},
	{testFile, jsonPin},
}

// change records one rewritten pin, for reporting.
type change struct {
	pkg    string
	oldPin string // e.g. ^0.24.0
	newPin string // e.g. ^0.25.0
	file   string
}

func main() {
	dir := flag.String("dir", "internal/scaffold", "scaffold package directory (holds templates/ + scaffold_test.go)")
	check := flag.Bool("check", false, "exit non-zero if a bump is NEEDED; write nothing (for CI)")
	flag.Parse()

	changes, err := run(*dir, scaffold.FetchNpmLatest, *check, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bump-pins:", err)
		os.Exit(2)
	}
	if *check && len(changes) > 0 {
		fmt.Fprintf(os.Stderr, "bump-pins: %d pin(s) out of date — run `go run ./internal/scaffold/cmd/bump-pins`\n", len(changes))
		os.Exit(1)
	}
}

// run discovers the @civitai/* packages pinned in the template, resolves each to
// its desired pin via fetch, and rewrites every stale literal across the three
// sites. With check=true it computes the needed changes but writes nothing.
// It returns the list of applied (or, under --check, needed) changes.
func run(dir string, fetch func(pkg string) (string, error), check bool, out io.Writer) ([]change, error) {
	// 1. Discover the distinct @civitai/* packages from the canonical source.
	pkgs, err := discoverPackages(filepath.Join(dir, pkgJSONFile))
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no @civitai/* pins found in %s — the template or the extractor is broken", pkgJSONFile)
	}

	// 2. Resolve each package to its desired caret pin. A transient npm error
	//    (or a genuinely-missing package) skips that package without failing —
	//    the pins-vs-published guard is the loud channel for real drift.
	desired := map[string]string{} // pkg -> desired bare "X.Y.Z"
	for _, pkg := range pkgs {
		latest, err := fetch(pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bump-pins: skipping %s (npm lookup failed: %v)\n", pkg, err)
			continue
		}
		pin, err := scaffold.DesiredPin(latest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bump-pins: skipping %s (bad published version %q: %v)\n", pkg, latest, err)
			continue
		}
		desired[pkg] = pin[1:] // drop the leading caret; the site regexes own the "^"
	}

	// 3. Rewrite every stale literal across the three sites.
	var changes []change
	for _, s := range sites {
		path := filepath.Join(dir, s.file)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", s.file, err)
		}
		updated := string(content)
		fileChanged := false
		for _, pkg := range pkgs {
			bare, ok := desired[pkg]
			if !ok {
				continue // skipped above
			}
			next, oldBare, did := rewritePin(updated, pkg, bare, s.kind)
			if did {
				updated = next
				fileChanged = true
				changes = append(changes, change{pkg: pkg, oldPin: "^" + oldBare, newPin: "^" + bare, file: s.file})
			}
		}
		if fileChanged && !check {
			// Preserve the file mode.
			info, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("stat %s: %w", s.file, err)
			}
			if err := os.WriteFile(path, []byte(updated), info.Mode()); err != nil {
				return nil, fmt.Errorf("writing %s: %w", s.file, err)
			}
		}
	}

	// 4. Report.
	for _, c := range changes {
		verb := "would bump"
		if !check {
			verb = "bumped"
		}
		fmt.Fprintf(out, "%s %s: %s -> %s (%s)\n", verb, c.pkg, c.oldPin, c.newPin, c.file)
	}
	if len(changes) == 0 {
		fmt.Fprintln(out, "all @civitai/* pins are current — nothing to do")
	}
	return changes, nil
}

// discoverPackages returns the distinct @civitai/* package names pinned in the
// package.json.tmpl, in a stable order.
func discoverPackages(pkgJSONPath string) ([]string, error) {
	content, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", pkgJSONPath, err)
	}
	seen := map[string]bool{}
	var pkgs []string
	for _, m := range scaffold.CivitaiPinRe.FindAllStringSubmatch(string(content), -1) {
		pkg := m[1]
		if !seen[pkg] {
			seen[pkg] = true
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

// caretVer matches a bare semver `X.Y.Z` right after a caret. Kept literal so we
// never blanket-replace an unrelated version elsewhere in the file.
const caretVer = `\d+\.\d+\.\d+`

// rewritePin replaces the pkg's caret version literal with newBare (a bare
// "X.Y.Z") in content, matching only the exact @civitai/<pkg> pin in the given
// literal shape. It returns the new content, the first old bare version it found
// (for reporting), and whether anything changed (false when the pin is absent or
// already equals newBare).
func rewritePin(content, pkg, newBare string, kind fileKind) (string, string, bool) {
	var re *regexp.Regexp
	switch kind {
	case jsonPin:
		// group1 = `"@civitai/<pkg>": "^`  group2 = version  group3 = `"`
		re = regexp.MustCompile(`("` + regexp.QuoteMeta(pkg) + `"\s*:\s*"\^)(` + caretVer + `)(")`)
	case prosePin:
		// group1 = `@civitai/<pkg>@^`  group2 = version  group3 = "" (empty tail)
		re = regexp.MustCompile(`(` + regexp.QuoteMeta(pkg) + `@\^)(` + caretVer + `)()`)
	default:
		return content, "", false
	}

	m := re.FindStringSubmatch(content)
	if m == nil {
		return content, "", false // pin not present in this file
	}
	oldBare := m[2]
	if oldBare == newBare {
		return content, oldBare, false // already current
	}
	updated := re.ReplaceAllString(content, `${1}`+newBare+`${3}`)
	return updated, oldBare, true
}
