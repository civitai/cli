package scaffold

// Structural guard: every design-system custom property the scaffold templates
// reference must be one the pinned UI pack actually DEFINES.
//
// Why this shape, and not a grep. The defect that produced this guard was a
// namespace rename in `@civitai/blocks-react` (`--ci-color-*` →
// `--civitai-color-*`, with two names also changing suffix:
// `text-muted` → `text-dimmed`, `primary-contrast` → `primary-fg`). Every one
// of the 27 references in page-money went dead at once, and NOTHING went red:
// an undefined CSS custom property resolves to nothing, so the declaration is
// dropped, the build is green, and the `template-page-money` CI job (install →
// typecheck → build) is green — the app just renders with an unset shell
// background, unset text and an invalid focus outline.
//
// A guard that greps for the literal `--ci-color-` would pin a WORD. It passes
// the moment the dead spelling is gone and says nothing about whether the new
// spelling is right, so the NEXT rename — the actual recurring hazard — walks
// straight past it. This guard pins the RELATIONSHIP instead: membership of the
// referenced set in the defined set. A token dropped or re-spelled upstream
// stops being a member, so the same class is caught however it is spelled, and
// even a wholesale namespace rename is caught (the template keeps spelling the
// old namespace, which the regenerated ledger no longer contains).
//
// The defined set is a checked-in ledger (testdata/design-tokens.txt) because CI
// cannot reach npm reliably and this guard gates every PR. Its provenance header
// records the package, the caret pin and the concrete version the tokens were
// read from; `TestDesignTokenLedgerMatchesTemplatePin` fails when that pin no
// longer matches the template, and `bump-pins` regenerates the ledger in the
// same run in which it bumps the pin so the two cannot drift.
//
// RESIDUAL, stated rather than papered over: the ledger is only as fresh as the
// last regeneration. Within one caret range (`^0.43.0` admits every `0.43.x`) a
// patch release could add a token — a template using it would red, which is a
// false failure a regeneration fixes — or REMOVE one, which the ledger would
// still list, and that direction is a false PASS. Closing it would need a
// network read at test time, which is the thing this guard deliberately does not
// do. Measured across the whole range this pin admits and the next minor
// (0.43.0, 0.43.1, 0.44.0, 0.44.1, 0.44.2) the defined set is byte-identical, so
// the residual is small today; it is not zero.

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// loadLedger reads and parses the checked-in ledger, failing closed.
func loadLedger(t *testing.T) *DesignTokenLedger {
	t.Helper()
	raw, err := os.ReadFile(DesignTokenLedgerFile)
	if err != nil {
		t.Fatalf(
			"cannot read the design-token ledger %s: %v\n"+
				"  The ledger is the DEFINED set this guard checks references against. Without it\n"+
				"  the membership test would be vacuous — every dead token would pass — so a missing\n"+
				"  ledger is a hard failure, not a skip.\n"+
				"  fix: go run ./internal/scaffold/cmd/bump-pins",
			DesignTokenLedgerFile, err)
	}
	l, err := ParseDesignTokenLedger(raw)
	if err != nil {
		t.Fatalf("design-token ledger %s is unusable: %v\n  fix: go run ./internal/scaffold/cmd/bump-pins", DesignTokenLedgerFile, err)
	}
	return l
}

// TestScaffoldTemplatesOnlyUseDefinedDesignTokens is the guard.
func TestScaffoldTemplatesOnlyUseDefinedDesignTokens(t *testing.T) {
	ledger := loadLedger(t)

	scan, err := ScanDesignTokenRefs(templatesFS, "templates")
	if err != nil {
		t.Fatalf("scanning templates for design-token references: %v", err)
	}

	// ── POSITIVE CONTROLS ───────────────────────────────────────────────────
	// This test's reassuring answer is "no dead tokens", which is exactly the
	// answer a scanner wired to an empty tree also gives. Assert it read this
	// repo's templates and found real references before believing the zero.
	t.Logf("scanned %d template file(s); %d file(s) reference design tokens; %d distinct (file,token) pair(s), %d total occurrence(s); ledger holds %d token(s) from %s@%s",
		scan.FilesRead, scan.FilesWith, len(scan.Refs), scan.Occurrences, len(ledger.Tokens), ledger.Pkg, ledger.Version)

	// Measured on this tree: 3 templates, 53 files. The floor is loose — it
	// exists to catch "walked nothing / walked the wrong tree", not to track a
	// count.
	if scan.FilesRead < 20 {
		t.Fatalf("read only %d template file(s) — the scanner is not looking at this repo's templates, so a clean verdict here would be a statement about nothing", scan.FilesRead)
	}
	// Measured on this tree: 27 occurrences across 3 files of page-money. If a
	// future change legitimately removes ALL design-token usage, lower these
	// floors DELIBERATELY — do not let the guard silently become a guard over
	// nothing.
	if scan.FilesWith < 3 || scan.Occurrences < 20 {
		t.Fatalf("found design tokens in only %d file(s) / %d occurrence(s) — expected the page-money chrome to reference the pack's tokens; either the templates stopped using them (lower this floor on purpose) or DesignTokenRefRe stopped matching", scan.FilesWith, scan.Occurrences)
	}

	// ── THE INVARIANT ───────────────────────────────────────────────────────
	for _, r := range scan.Refs {
		if ledger.Has(r.Token) {
			continue
		}
		t.Errorf(
			"DEAD DESIGN TOKEN — this reference resolves to NOTHING at runtime, silently.\n"+
				"  file:  %s\n"+
				"  token: %s  ×%d   (not defined by %s@%s)\n"+
				"  Undefined CSS custom properties are dropped, so the build stays green and the\n"+
				"  scaffolded app just renders unstyled at this site. Nothing else will catch it.\n"+
				"  fix: replace it with the token the pack defines for the same intent — see\n"+
				"       %s for the full defined set. If the pack renamed it, regenerate the\n"+
				"       ledger first: go run ./internal/scaffold/cmd/bump-pins",
			r.File, r.Token, r.Occurrences, ledger.Pkg, ledger.Version, DesignTokenLedgerFile)
	}
}

// TestDesignTokenLedgerMatchesTemplatePin closes the drift between the pin and
// the ledger. bump-pins rewrites the pack's caret pin in the template; if the
// ledger is not regenerated in the same run, it describes a version the scaffold
// no longer installs and the membership check above is checking against the
// wrong set. This is the loud channel for that.
func TestDesignTokenLedgerMatchesTemplatePin(t *testing.T) {
	ledger := loadLedger(t)

	pins := collectCivitaiPins(t)
	if len(pins) == 0 {
		t.Fatal("no @civitai/* pins found in any template package.json.tmpl — the extractor is broken")
	}
	var pin string
	for _, p := range pins {
		if p.pkg == ledger.Pkg {
			pin = p.pin
			break
		}
	}
	if pin == "" {
		t.Fatalf("the ledger's package %q is not pinned by any template package.json.tmpl — the ledger describes a package the scaffold does not install", ledger.Pkg)
	}

	if pin != ledger.Pin {
		t.Fatalf(
			"DESIGN-TOKEN LEDGER IS STALE — it describes a version the scaffold no longer installs.\n"+
				"  package:       %s\n"+
				"  template pins: %s\n"+
				"  ledger says:   %s   (tokens read from %s)\n"+
				"  The membership guard is now checking template references against the WRONG token\n"+
				"  set, so it can pass over a token the newly-pinned pack dropped.\n"+
				"  fix: go run ./internal/scaffold/cmd/bump-pins   (rewrites the pins AND this ledger)",
			ledger.Pkg, pin, ledger.Pin, ledger.Version)
	}

	// The recorded version must be one the recorded pin actually admits —
	// otherwise the provenance is internally incoherent even when it matches.
	ok, err := CaretAdmits(ledger.Pin, ledger.Version)
	if err != nil {
		t.Fatalf("ledger provenance is unparseable: pin %q vs version %q: %v", ledger.Pin, ledger.Version, err)
	}
	if !ok {
		t.Fatalf("ledger provenance is incoherent: pin %q does NOT admit the version %q the tokens were read from", ledger.Pin, ledger.Version)
	}
}

// TestDesignTokenLedgerNamespace pins the ledger to the namespace
// DesignTokenRefRe knows how to see. If the pack renames its namespace wholesale
// the regenerated ledger would hold tokens the reference regex cannot match, and
// the membership guard would go quietly blind rather than red.
func TestDesignTokenLedgerNamespace(t *testing.T) {
	ledger := loadLedger(t)
	for _, tok := range ledger.Tokens {
		if !strings.HasPrefix(tok, "--civitai-") {
			t.Errorf("ledger token %q is outside the --civitai-* namespace DesignTokenRefRe matches — update the regex in designtokens.go, or this guard is blind to references to it", tok)
		}
		if DesignTokenRefRe.FindString(tok) != tok {
			t.Errorf("ledger token %q is not matched WHOLE by DesignTokenRefRe — a reference to it could never be recognised", tok)
		}
	}
}

// ── INSTRUMENT VALIDATION ───────────────────────────────────────────────────
// Everything above reads a verdict out of ExtractDesignTokenRefs /
// ScanDesignTokenRefs. Until these two controls have been watched to work, that
// verdict is a fact about the extractor, not about the templates.

// TestExtractDesignTokenRefs is the POSITIVE control: fed input that MUST
// produce matches, the extractor produces exactly them.
func TestExtractDesignTokenRefs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "css var reference",
			in:   "outline: 2px solid var(--civitai-color-primary);",
			want: []string{"--civitai-color-primary"},
		},
		{
			name: "nested var with fallback, both extracted",
			in:   `color: 'var(--civitai-color-text-dimmed, var(--civitai-color-text))',`,
			want: []string{"--civitai-color-text", "--civitai-color-text-dimmed"},
		},
		{
			name: "literal fallback is not a token",
			in:   `color: 'var(--civitai-color-success, #2f9e44)',`,
			want: []string{"--civitai-color-success"},
		},
		{
			name: "dead legacy namespace is still seen",
			in:   `background: 'var(--ci-color-surface-2)',`,
			want: []string{"--ci-color-surface-2"},
		},
		{
			name: "js object key form",
			in:   `style={{ ['--civitai-color-primary']: accent }}`,
			want: []string{"--civitai-color-primary"},
		},
		{
			name: "non-colour tokens are in scope too",
			in:   "border-radius: var(--civitai-radius); font-family: var(--civitai-font-mono);",
			want: []string{"--civitai-font-mono", "--civitai-radius"},
		},
		{
			name: "an ordinary CLI long flag is NOT a token",
			in:   "run `civitai app build --ci-mode --civitai=no`",
			want: nil,
		},
		{
			// Prose naming the namespace is documentation, not a reference. The
			// guard reported its OWN comment as a dead token on the first run
			// before this case existed.
			name: "a globbed namespace in prose is not a reference",
			in:   "every --civitai-color-* token, and the legacy --ci-color-* ones",
			want: nil,
		},
		{
			name: "a real token adjacent to a globbed one is still seen",
			in:   "--civitai-color-* generally; here we use var(--civitai-color-text).",
			want: []string{"--civitai-color-text"},
		},
		{
			name: "unrelated custom properties are ignored",
			in:   "--my-app-color: red; --mantine-color-body: blue;",
			want: nil,
		},
		{
			name: "no tokens at all",
			in:   "const x = 1;",
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractDesignTokenRefs(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("ExtractDesignTokenRefs(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("ExtractDesignTokenRefs(%q) = %v, want %v", c.in, got, c.want)
				}
			}
		})
	}
}

// TestExtractDesignTokenDefs covers the other half of the instrument: the
// extraction bump-pins uses to BUILD the ledger. Both declaration forms the pack
// ships, and the shapes that must NOT count as a definition.
func TestExtractDesignTokenDefs(t *testing.T) {
	// Both forms appear in the real pack: `@property` blocks in the generated
	// token sheet, `--x: value` pairs in the :root rule.
	src := "@property --civitai-color-text {\n  syntax: '<color>';\n}\n" +
		":root{--civitai-color-text:#222;--civitai-radius:8px;}\n" +
		"a{color:var(--civitai-color-primary)}\n" + // a REFERENCE, not a definition
		"--mantine-color-body: #fff;\n" // another design system's token
	got := ExtractDesignTokenDefs(src)
	want := []string{"--civitai-color-text", "--civitai-radius"}
	if len(got) != len(want) {
		t.Fatalf("ExtractDesignTokenDefs = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("ExtractDesignTokenDefs = %v, want %v", got, want)
		}
	}
}

// TestScanDesignTokenRefsNegativeControl is the NEGATIVE control: the scanner
// must be able to REPORT a dead token. A scanner that returns nothing whatever
// you feed it is indistinguishable from a clean tree, so the clean verdict the
// real guard reports would mean nothing without this.
//
// It drives the same ScanDesignTokenRefs the guard uses, over a synthetic FS
// holding one live token and one dead one, and asserts the membership check
// separates them.
func TestScanDesignTokenRefsNegativeControl(t *testing.T) {
	ledger := loadLedger(t)

	fsys := fstest.MapFS{
		"templates/fake/src/live.css.tmpl": {Data: []byte("a{color:var(--civitai-color-text)}")},
		"templates/fake/src/dead.css.tmpl": {Data: []byte("a{color:var(--civitai-color-text-muted)}")},
		"templates/fake/src/legacy.css.tmpl": {
			Data: []byte("a{color:var(--ci-color-primary)}"),
		},
	}
	scan, err := ScanDesignTokenRefs(fsys, "templates")
	if err != nil {
		t.Fatalf("scanning synthetic FS: %v", err)
	}
	if scan.FilesRead != 3 || scan.FilesWith != 3 {
		t.Fatalf("read %d file(s) / %d with tokens from the synthetic FS, want 3/3", scan.FilesRead, scan.FilesWith)
	}
	if len(scan.Refs) != 3 || scan.Occurrences != 3 {
		t.Fatalf("found %d reference(s) / %d occurrence(s) in the synthetic FS, want 3/3: %v", len(scan.Refs), scan.Occurrences, scan.Refs)
	}

	var dead []string
	for _, r := range scan.Refs {
		if !ledger.Has(r.Token) {
			dead = append(dead, r.Token)
		}
	}
	// `--civitai-color-text-muted` is the suffix the pack RENAMED away
	// (→ text-dimmed) and `--ci-color-primary` is the dead namespace. Both must
	// be reported; the live one must not be.
	if len(dead) != 2 {
		t.Fatalf("membership check reported %d dead token(s) %v, want exactly 2 (--civitai-color-text-muted, --ci-color-primary) — the control did not separate live from dead", len(dead), dead)
	}
	for _, want := range []string{"--civitai-color-text-muted", "--ci-color-primary"} {
		found := false
		for _, d := range dead {
			if d == want {
				found = true
			}
		}
		if !found {
			t.Errorf("negative control: %q was NOT reported dead (reported: %v)", want, dead)
		}
	}
	// And the sanity half: the token that IS defined must not be flagged.
	if ledger.Has("--civitai-color-text") == false {
		t.Error("--civitai-color-text is missing from the ledger — the live arm of the control is broken")
	}
}

// TestParseDesignTokenLedgerFailsClosed pins the fail-closed contract: a
// missing/empty/provenance-less ledger must be an ERROR, never a usable empty
// set. An empty set would make the membership guard pass over everything.
func TestParseDesignTokenLedgerFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty file", ""},
		{"only whitespace", "\n\n   \n"},
		{"comments only, no tokens", "# package: p\n# pin: ^1.0.0\n# version: 1.0.0\n"},
		{"tokens but no provenance", "--civitai-color-text\n--civitai-color-body\n"},
		{"missing version", "# package: p\n# pin: ^1.0.0\n--civitai-color-text\n"},
		{"missing pin", "# package: p\n# version: 1.0.0\n--civitai-color-text\n"},
		{"missing package", "# pin: ^1.0.0\n# version: 1.0.0\n--civitai-color-text\n"},
		{"garbage body line", "# package: p\n# pin: ^1.0.0\n# version: 1.0.0\nnot-a-token\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, err := ParseDesignTokenLedger([]byte(c.in))
			if err == nil {
				t.Fatalf("expected an error, got a usable ledger with %d token(s) — this parses OPEN and would make the guard vacuous", len(l.Tokens))
			}
		})
	}

	// Positive half: a well-formed ledger parses.
	l, err := ParseDesignTokenLedger([]byte("# package: @civitai/blocks-react\n# pin: ^0.43.0\n# version: 0.43.1\n\n--civitai-color-text\n--civitai-color-body\n"))
	if err != nil {
		t.Fatalf("well-formed ledger failed to parse: %v", err)
	}
	if l.Pkg != "@civitai/blocks-react" || l.Pin != "^0.43.0" || l.Version != "0.43.1" || len(l.Tokens) != 2 {
		t.Fatalf("well-formed ledger parsed wrong: %+v", l)
	}
	if !l.Has("--civitai-color-text") || l.Has("--civitai-color-nope") {
		t.Fatal("Has() is wrong on a well-formed ledger")
	}
}

// TestRenderDesignTokenLedgerRoundTrips pins the render→parse contract bump-pins
// depends on: what the bumper writes is what the guard reads back.
func TestRenderDesignTokenLedgerRoundTrips(t *testing.T) {
	in := []string{"--civitai-color-text", "--civitai-radius", "--civitai-color-body", "--civitai-color-text"}
	raw := RenderDesignTokenLedger("@civitai/blocks-react", "^0.43.0", "0.43.1", in)
	l, err := ParseDesignTokenLedger(raw)
	if err != nil {
		t.Fatalf("rendered ledger does not parse: %v\n%s", err, raw)
	}
	if l.Pkg != "@civitai/blocks-react" || l.Pin != "^0.43.0" || l.Version != "0.43.1" {
		t.Fatalf("provenance did not round-trip: %+v", l)
	}
	// deduped + sorted
	want := []string{"--civitai-color-body", "--civitai-color-text", "--civitai-radius"}
	if len(l.Tokens) != len(want) {
		t.Fatalf("tokens = %v, want %v", l.Tokens, want)
	}
	for i := range want {
		if l.Tokens[i] != want[i] {
			t.Fatalf("tokens = %v, want %v", l.Tokens, want)
		}
	}
}

// TestCheckedInLedgerIsWhatTheRendererWouldWrite proves the checked-in bytes
// were produced by RenderDesignTokenLedger and not hand-edited — the ledger's
// own header says DO NOT HAND-EDIT, and this is what makes that enforceable.
func TestCheckedInLedgerIsWhatTheRendererWouldWrite(t *testing.T) {
	raw, err := os.ReadFile(DesignTokenLedgerFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", DesignTokenLedgerFile, err)
	}
	l, err := ParseDesignTokenLedger(raw)
	if err != nil {
		t.Fatalf("cannot parse %s: %v", DesignTokenLedgerFile, err)
	}
	want := RenderDesignTokenLedger(l.Pkg, l.Pin, l.Version, l.Tokens)
	if string(raw) != string(want) {
		t.Errorf(
			"%s is not byte-identical to what RenderDesignTokenLedger emits for its own contents —\n"+
				"  it has been hand-edited, or the renderer changed without the ledger being regenerated.\n"+
				"  fix: go run ./internal/scaffold/cmd/bump-pins",
			DesignTokenLedgerFile)
	}
}
