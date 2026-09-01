// Shared design-token logic for the scaffold templates.
//
// The page-money template styles its own chrome with the CSS custom properties
// the `@civitai/blocks-react` UI pack injects (`--civitai-color-surface`,
// `--civitai-color-text`, …). A reference to a token the pack does NOT define is
// silently inert: CSS resolves an undefined custom property to nothing, so the
// declaration is dropped and the element falls back to the UA default. Nothing
// errors, `npm run build` is green, and the template-page-money CI job (install →
// typecheck → build) is green too — a scaffolded app just ships an unset shell
// background, unset text colour and an invalid focus outline.
//
// That is not hypothetical. The pack renamed the whole token namespace
// `--ci-color-*` → `--civitai-color-*` and, for two names, changed the SUFFIX as
// well (`text-muted` → `text-dimmed`, `primary-contrast` → `primary-fg`); the
// templates kept the old spellings and every app created from them was born
// unthemed. The pack's own `dist/ui/Badge.js` still carries a comment recording
// the rename.
//
// So the invariant this file exists to make checkable is a RELATIONSHIP, not a
// word: every design-system custom property referenced anywhere under
// `templates/` must be a MEMBER of the set the pinned pack DEFINES. Grepping for
// the literal string `--ci-color-` would only ever catch the rename that already
// happened; membership catches the next one too, because a token that is dropped
// or re-spelled upstream stops being in the set.
//
// Two consumers keep that honest and MUST agree by construction, so — exactly as
// with the `@civitai/*` pins in pins.go — the logic lives here in production
// code rather than in a `_test.go`:
//
//   - the offline structural guard (design_tokens_guard_test.go), which runs in
//     the default `go test ./...` and checks every template reference against the
//     checked-in ledger at testdata/design-tokens.txt.
//   - the bump-pins command (internal/scaffold/cmd/bump-pins), which REGENERATES
//     that ledger from the newly published version whenever it bumps the pack's
//     pin — so the pin and the ledger cannot drift apart.
//
// 🔴 The ledger is checked in on purpose. CI cannot reach npm reliably and the
// offline guard is the one that gates every PR, so the guard never fetches; it
// reads bytes a human (or the daily bumper) committed, and records which package
// version they came from.
package scaffold

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DesignTokenPkg is the npm package that DEFINES the design tokens the scaffold
// templates reference. It is one of the `@civitai/*` packages page-money pins,
// so its version is whatever that pin currently resolves to.
const DesignTokenPkg = "@civitai/blocks-react"

// DesignTokenLedgerFile is the checked-in ledger, relative to this package dir.
const DesignTokenLedgerFile = "testdata/design-tokens.txt"

// DesignTokenRefRe matches a design-system custom-property REFERENCE as it
// appears in a template — inside `var(--x)`, inside a JS string, or as an object
// key. Two namespaces are matched:
//
//   - `--civitai-*`  the live namespace. Matching the whole namespace (not just
//     `--civitai-color-*`) is what makes a WHOLESALE upstream rename detectable:
//     the template keeps spelling the old namespace, the regenerated ledger no
//     longer contains it, and membership fails.
//   - `--ci-color-*` the dead namespace this guard was written for. Kept
//     explicitly so a reintroduction is caught by membership rather than by a
//     string match, and so it names the token in the failure.
//
// Deliberately NOT `--ci-*`: that would match an ordinary CLI long flag
// (`--ci-mode`) appearing in template prose.
//
// The trailing `[a-z0-9]` anchor means an interpolated reference
// (`var(--civitai-color-${intent})`) yields the truncated stem `--civitai-color`,
// which is not in the ledger and therefore reds. No template builds a token name
// dynamically today; if one ever needs to, that is a deliberate decision to make
// here, not something to discover from a confusing failure.
//
// A template DEFINING its own `--civitai-…` property is caught the same way and
// that is intended: the namespace belongs to the pack, so a template minting a
// name in it either shadows a real token or invents one the pack will never
// theme. Overriding an EXISTING token is fine — that name is in the ledger.
var DesignTokenRefRe = regexp.MustCompile(`--(?:civitai|ci-color)-[a-z0-9-]*[a-z0-9]`)

// designTokenDefColonRe matches a token DEFINITION in the published pack's
// stylesheet: `--civitai-color-text: #222`, including inside the JS string
// literal the pack ships the stylesheet as.
var designTokenDefColonRe = regexp.MustCompile(`(--civitai-[a-z0-9-]*[a-z0-9])\s*:`)

// designTokenDefPropertyRe matches the `@property --civitai-color-text {` form
// the pack's token block is generated as. Measured on 0.43.1 the `@property`
// set (24) is a strict SUBSET of the colon set (27, the three non-colour
// `--civitai-font` / `--civitai-font-mono` / `--civitai-radius` tokens being
// colon-only), but relying on that containment is a guess about a generator we
// do not own, so both forms are extracted and unioned.
var designTokenDefPropertyRe = regexp.MustCompile(`@property\s+(--civitai-[a-z0-9-]*[a-z0-9])`)

// ExtractDesignTokenRefs returns the distinct design-system custom properties
// referenced in src, sorted. It never returns nil for a hit-free input — an
// empty slice is a real answer ("this file references none").
//
// A GLOB in prose is not a reference. Template comments and READMEs legitimately
// name the namespace as `--civitai-color-*`, which the regex would otherwise
// see as the stem `--civitai-color` — a token no pack defines, so the guard
// would red on its own documentation. (Found by the guard, on the first run,
// against a comment written in the same change.) Matches trailed by `*` or `-*`
// are therefore dropped: RE2 has no lookahead, so it is done here.
func ExtractDesignTokenRefs(src string) []string {
	counts := countDesignTokenRefs(src)
	out := make([]string, 0, len(counts))
	for tok := range counts {
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}

// countDesignTokenRefs returns each referenced token and how many times it
// occurs. Counting from the MATCH LIST is load-bearing: a naive
// strings.Count(src, tok) overcounts, because `--civitai-color-text` is a
// substring of `--civitai-color-text-dimmed`.
func countDesignTokenRefs(src string) map[string]int {
	counts := map[string]int{}
	for _, loc := range DesignTokenRefRe.FindAllStringIndex(src, -1) {
		if isGlobbedNamespace(src[loc[1]:]) {
			continue
		}
		counts[src[loc[0]:loc[1]]]++
	}
	return counts
}

// isGlobbedNamespace reports whether the text immediately following a match
// makes it a prose wildcard (`--civitai-color-*`, `--civitai-color*`) rather
// than a real token reference.
func isGlobbedNamespace(tail string) bool {
	return strings.HasPrefix(tail, "*") || strings.HasPrefix(tail, "-*")
}

// ExtractDesignTokenDefs returns the distinct design-system custom properties
// DEFINED in src, sorted. src is the raw text of a file shipped in the pack.
func ExtractDesignTokenDefs(src string) []string {
	var all []string
	for _, m := range designTokenDefColonRe.FindAllStringSubmatch(src, -1) {
		all = append(all, m[1])
	}
	for _, m := range designTokenDefPropertyRe.FindAllStringSubmatch(src, -1) {
		all = append(all, m[1])
	}
	return dedupeSorted(all)
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// DesignTokenLedger is the parsed testdata/design-tokens.txt: the token set plus
// the provenance that says which published package version it was derived from.
type DesignTokenLedger struct {
	Pkg     string   // e.g. @civitai/blocks-react
	Pin     string   // the template's caret pin at generation time, e.g. ^0.43.0
	Version string   // the concrete published version the tokens came from, e.g. 0.43.1
	Tokens  []string // sorted, deduped
}

// Has reports whether tok is a member of the ledger.
func (l *DesignTokenLedger) Has(tok string) bool {
	for _, t := range l.Tokens {
		if t == tok {
			return true
		}
	}
	return false
}

// ledgerHeaderRe matches a `# key: value` provenance line.
var ledgerHeaderRe = regexp.MustCompile(`^#\s*(package|pin|version)\s*:\s*(\S+)\s*$`)

// ParseDesignTokenLedger parses the ledger file's bytes.
//
// It FAILS CLOSED. A ledger that is missing, empty, all-comments, or missing any
// provenance field is an error, never an empty-but-usable set — an empty set
// would make the membership check vacuous and every dead token would pass.
func ParseDesignTokenLedger(raw []byte) (*DesignTokenLedger, error) {
	l := &DesignTokenLedger{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	var toks []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if m := ledgerHeaderRe.FindStringSubmatch(line); m != nil {
				switch m[1] {
				case "package":
					l.Pkg = m[2]
				case "pin":
					l.Pin = m[2]
				case "version":
					l.Version = m[2]
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "--") {
			return nil, fmt.Errorf("ledger line %q is neither a `# key: value` header nor a `--token`", line)
		}
		toks = append(toks, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading ledger: %w", err)
	}
	if l.Pkg == "" || l.Pin == "" || l.Version == "" {
		return nil, fmt.Errorf(
			"ledger is missing provenance (package=%q pin=%q version=%q) — it must record which published package the tokens came from, or nobody can tell whether it is stale",
			l.Pkg, l.Pin, l.Version)
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("ledger lists ZERO tokens — an empty set makes the membership check vacuous, so this is a hard error, not an empty pass")
	}
	l.Tokens = dedupeSorted(toks)
	return l, nil
}

// RenderDesignTokenLedger produces the ledger file's bytes for a token set.
// bump-pins writes this; the header it emits is what ParseDesignTokenLedger
// reads back, so the two round-trip by construction.
func RenderDesignTokenLedger(pkg, pin, version string, tokens []string) []byte {
	var b strings.Builder
	b.WriteString("# Design tokens DEFINED by the pinned UI pack — the membership ledger for\n")
	b.WriteString("# internal/scaffold/design_tokens_guard_test.go.\n")
	b.WriteString("#\n")
	b.WriteString("# Every `--civitai-*` (and any legacy `--ci-color-*`) custom property referenced\n")
	b.WriteString("# anywhere under internal/scaffold/templates/ must appear below. A reference to a\n")
	b.WriteString("# token the pack does not define resolves to nothing at runtime: no error, a\n")
	b.WriteString("# green build, and an app scaffolded with unset colours.\n")
	b.WriteString("#\n")
	b.WriteString("# 🔴 DO NOT HAND-EDIT. Regenerate, which also re-reads the pin:\n")
	b.WriteString("#     go run ./internal/scaffold/cmd/bump-pins\n")
	b.WriteString("# The bumper rewrites this file in the SAME run in which it bumps the pack's pin,\n")
	b.WriteString("# so the pin and this ledger cannot drift apart. `--check` reports without writing.\n")
	b.WriteString("#\n")
	b.WriteString("# `pin` is the caret literal in templates/page-money/package.json.tmpl at\n")
	b.WriteString("# generation time; `version` is the concrete version those tokens were read from.\n")
	b.WriteString("# The guard fails when `pin` no longer matches the template.\n")
	b.WriteString("#\n")
	fmt.Fprintf(&b, "# package: %s\n", pkg)
	fmt.Fprintf(&b, "# pin: %s\n", pin)
	fmt.Fprintf(&b, "# version: %s\n", version)
	b.WriteString("\n")
	for _, t := range dedupeSorted(tokens) {
		b.WriteString(t)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// npmTarballURL builds the registry tarball URL for a scoped or unscoped package.
// npm serves `@scope/name` at `/@scope/name/-/name-<version>.tgz` — the basename
// drops the scope, which is the easy thing to get wrong.
func npmTarballURL(pkg, version string) string {
	base := pkg
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		base = pkg[i+1:]
	}
	return npmRegistryBase + "/" + pkg + "/-/" + base + "-" + version + ".tgz"
}

// maxTarballBytes caps what FetchNpmTokens will read from the registry. The
// 0.43.1 tarball is ~313 KiB unpacked-per-file; 64 MiB is far above any
// plausible UI pack and far below anything that could exhaust a CI runner.
const maxTarballBytes = 64 << 20

// FetchNpmTokens downloads the published tarball for pkg@version and returns the
// design tokens it DEFINES, sorted.
//
// It is used by bump-pins to regenerate the ledger. The offline guard never
// calls it — see this file's header for why the ledger is checked in.
//
// An empty result is an ERROR, not an empty set: the pack has defined tokens in
// every published version, so zero means the extractor stopped matching (most
// likely because the namespace was renamed again) and the caller must not write
// an empty ledger on the strength of it.
func FetchNpmTokens(pkg, version string) ([]string, error) {
	url := npmTarballURL(pkg, version)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, fmt.Errorf("GET %s: %s: %w", url, resp.Status, ErrPkgNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxTarballBytes))
	if err != nil {
		return nil, fmt.Errorf("GET %s: gunzip: %w", url, err)
	}
	defer gz.Close()

	var all []string
	tr := tar.NewReader(gz)
	files := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("GET %s: tar: %w", url, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxTarballBytes))
		if err != nil {
			return nil, fmt.Errorf("GET %s: reading %s: %w", url, hdr.Name, err)
		}
		files++
		all = append(all, ExtractDesignTokenDefs(string(body))...)
	}
	if files == 0 {
		return nil, fmt.Errorf("GET %s: tarball contained no regular files", url)
	}
	tokens := dedupeSorted(all)
	if len(tokens) == 0 {
		return nil, fmt.Errorf(
			"GET %s: read %d file(s) but found NO --civitai-* token definitions — the pack's token namespace was probably renamed again; update designTokenDefColonRe/designTokenDefPropertyRe and DesignTokenRefRe in internal/scaffold/designtokens.go before regenerating",
			url, files)
	}
	return tokens, nil
}

// TemplateDesignTokenRef is one design-token reference found in a template file:
// a distinct (file, token) pair, with how many times it occurs in that file.
type TemplateDesignTokenRef struct {
	File        string // template-relative path, e.g. templates/page-money/src/App.tsx.tmpl
	Token       string
	Occurrences int
}

// DesignTokenScan is the result of walking a template tree, carrying the counts
// a caller needs to assert a POSITIVE CONTROL. This scanner's reassuring answer
// is an empty finding list — exactly what a scanner pointed at an empty tree
// also produces — so the counts are part of the result, not a debug aside.
type DesignTokenScan struct {
	Refs        []TemplateDesignTokenRef // distinct (file, token) pairs
	FilesRead   int                      // every file walked
	FilesWith   int                      // files holding at least one reference
	Occurrences int                      // total textual occurrences
}

// ScanDesignTokenRefs walks fsys from root and returns every distinct
// (file, token) design-token reference plus the scan's coverage counts.
func ScanDesignTokenRefs(fsys fs.FS, root string) (DesignTokenScan, error) {
	var s DesignTokenScan
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s.FilesRead++
		counts := countDesignTokenRefs(string(raw))
		if len(counts) > 0 {
			s.FilesWith++
		}
		toks := make([]string, 0, len(counts))
		for tok := range counts {
			toks = append(toks, tok)
		}
		sort.Strings(toks)
		for _, tok := range toks {
			s.Occurrences += counts[tok]
			s.Refs = append(s.Refs, TemplateDesignTokenRef{File: p, Token: tok, Occurrences: counts[tok]})
		}
		return nil
	})
	if err != nil {
		return DesignTokenScan{FilesRead: s.FilesRead}, err
	}
	return s, nil
}
