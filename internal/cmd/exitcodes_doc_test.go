package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// flattenWS collapses the help section's line wrapping so a sentence written as
// one string in exitCodeDocs can be matched against the rendered, wrapped
// terminal output. Without it every multi-line summary is a false negative.
func flattenWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// readREADME returns the repo's README.md. The path is relative to this
// package's source dir, which `go test` makes the working directory.
func readREADME(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	return string(b)
}

// extractREADMEExitCodeTable pulls the markdown table out of the README's
// "## Exit codes" section: the block from the `| Code | Meaning |` header to
// the first blank line after it.
//
// It Fatals rather than returning empty on a miss — a silent "" would make
// every assertion below compare two empty strings and pass, which is the shape
// of a guard wired to nothing.
func extractREADMEExitCodeTable(t *testing.T, readme string) string {
	t.Helper()
	const heading = "\n## Exit codes\n"
	i := strings.Index(readme, heading)
	if i < 0 {
		t.Fatalf("README.md has no %q heading — the exit-code contract must be published there", strings.TrimSpace(heading))
	}
	rest := readme[i+len(heading):]
	j := strings.Index(rest, "| Code | Meaning |")
	if j < 0 {
		t.Fatal("README.md's Exit codes section has no `| Code | Meaning |` table header")
	}
	rest = rest[j:]
	var lines []string
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// extractREADMEExitCodeSections pulls the per-code `### Exit code N`
// subsections out of the README's "## Exit codes" section: from the first such
// heading to the next `##`-level heading.
//
// Like the table extractor it Fatals rather than returning empty on a miss.
// The long half of the contract living in a block nobody can find is the same
// failure as the table drifting — a guard that quietly compared "" to "" would
// report a serene pass for a README that had lost every subsection.
func extractREADMEExitCodeSections(t *testing.T, readme string) string {
	t.Helper()
	const heading = "\n## Exit codes\n"
	i := strings.Index(readme, heading)
	if i < 0 {
		t.Fatalf("README.md has no %q heading — the exit-code contract must be published there", strings.TrimSpace(heading))
	}
	rest := readme[i+len(heading):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	// Anchored at a LINE START, not anywhere: the section's own intro prose names
	// `### Exit code N` inline to tell the reader where the detail is, and an
	// unanchored search matched that mention — pulling the table and the shell
	// example into the "subsections" and reporting 6 headings where there are 5.
	j := strings.Index(rest, "\n### Exit code ")
	if j < 0 {
		t.Fatal("README.md's Exit codes section has no `### Exit code N` subsections — the per-code detail is gone")
	}
	return strings.TrimRight(rest[j+1:], "\n")
}

// TestREADMEExitCodeTableIsGenerated is the README half of the anti-drift
// guard: the table in the file must be byte-identical to what exitCodeDocs
// renders. Hand-editing the README fails here; editing exitCodeDocs moves the
// README AND `--help` together, because both are rendered from it.
//
// This is deliberately EQUALITY, not a Contains/keyword check: a word like
// "not found" can be spelled by any number of unrelated rows, so a substring
// assertion would pass while the two texts said different things.
func TestREADMEExitCodeTableIsGenerated(t *testing.T) {
	got := extractREADMEExitCodeTable(t, readREADME(t))

	// Positive control: the extractor must have found real rows, so a pass
	// cannot mean "we compared two empty strings".
	if n := strings.Count(got, "\n| `"); n != len(exitCodeDocs) {
		t.Fatalf("extracted %d README table rows, want %d — the extractor is looking at the wrong block", n, len(exitCodeDocs))
	}

	want := readmeExitCodeTable()
	if got != want {
		t.Errorf("README.md's exit-code table has drifted from exitCodeDocs (internal/cmd/exitcodes_doc.go).\n"+
			"Do not edit the README table by hand — edit exitCodeDocs and paste this:\n\n%s\n\ngot:\n\n%s", want, got)
	}
}

// TestREADMEExitCodeSectionsAreGenerated is the same guard for the LONG half.
//
// The summary/detail split moved most of the contract out of the table and into
// `### Exit code N` subsections. Those are the text a reader who follows a row
// actually reads, so leaving them hand-maintained would have re-created the
// exact drift the table guard exists to prevent — one surface generated, one
// not, and nothing tying them together (see the file header in
// exitcodes_doc.go). They are generated, and asserted byte-identical.
func TestREADMEExitCodeSectionsAreGenerated(t *testing.T) {
	got := extractREADMEExitCodeSections(t, readREADME(t))

	// Positive control: the extractor must have found one heading per code that
	// carries Detail, so a pass cannot mean "we compared two empty strings" or
	// "the block held one section out of five".
	wantSections := 0
	for _, d := range exitCodeDocs {
		if len(d.Detail) > 0 {
			wantSections++
		}
	}
	if wantSections < 4 {
		t.Fatalf("only %d codes carry Detail — this guard would be asserting almost nothing", wantSections)
	}
	if n := strings.Count(got, "### Exit code "); n != wantSections {
		t.Fatalf("extracted %d `### Exit code N` subsections, want %d — the extractor is looking at the wrong block", n, wantSections)
	}

	want := readmeExitCodeSections()
	if got != want {
		t.Errorf("README.md's per-code exit-code subsections have drifted from exitCodeDocs (internal/cmd/exitcodes_doc.go).\n"+
			"Do not edit them by hand — edit exitCodeDocs and paste this:\n\n%s\n\ngot:\n\n%s", want, got)
	}
}

// TestRootHelpExitCodesAreGenerated is the `--help` half. It proves the section
// is DERIVED, not merely equal today: it swaps a sentinel into exitCodeDocs and
// requires the freshly built command to carry it. A hand-written copy pasted
// back into root.go passes an equality check on the current text and fails
// this one.
//
// It reads the RENDERED `civitai --help` output rather than `cmd.Long`. The
// section moved OUT of Long and into the help TEMPLATE so it renders after
// Usage/Available Commands (see rootHelpTemplate) — and the rendered text is the
// stronger claim anyway: it is what the user actually sees.
func TestRootHelpExitCodesAreGenerated(t *testing.T) {
	const sentinel = "ZZ-EXIT-DOC-SENTINEL-ZZ"

	before := renderRootHelp(t)
	if !strings.Contains(before, rootExitCodeHelp()) {
		t.Fatalf("root --help does not carry the rendered exit-code section:\n%s", before)
	}
	original := exitCodeDocs[4].Summary
	if !strings.Contains(flattenWS(before), flattenWS(plainify(original))) {
		t.Fatalf("root --help does not carry exit code 4's summary %q", original)
	}

	restore := exitCodeDocs
	defer func() { exitCodeDocs = restore }()
	swapped := make([]ExitCodeDoc, len(restore))
	copy(swapped, restore)
	swapped[4].Summary = sentinel
	exitCodeDocs = swapped

	after := renderRootHelp(t)
	if !strings.Contains(after, sentinel) {
		t.Errorf("root --help is not rendered from exitCodeDocs: changing code 4's summary did not change the help text.\n%s", after)
	}
	if strings.Contains(flattenWS(after), flattenWS(plainify(original))) {
		t.Errorf("root --help still carries the OLD summary for code 4 after the source changed — it holds a hand-written copy")
	}
}

// TestBothFieldsReachTheirSurfaces is the README twin of the sentinel check
// above, extended to the summary/detail split.
//
// Both fields need their own sentinel, and they need DIFFERENT expectations,
// because the split gave them different jobs. A Summary sentinel must reach
// BOTH surfaces — that is the "they cannot drift apart" claim, unchanged. A
// Detail sentinel must reach the README subsections, which is what stops the
// long half from becoming hand-maintained prose the slice no longer controls.
//
// 🔴 The Detail sentinel must ALSO be absent from `--help`, and that half is not
// pedantry: without it, "rootExitCodeHelp renders Summary only" is an
// observation about today's text rather than a property, and a well-meaning
// "let's just print everything again" would restore the 2,525-character wall
// this split exists to remove while every other guard here stayed green.
func TestBothFieldsReachTheirSurfaces(t *testing.T) {
	const (
		summarySentinel = "ZZ-EXIT-SUMMARY-SENTINEL-ZZ"
		detailSentinel  = "ZZ-EXIT-DETAIL-SENTINEL-ZZ"
	)

	// Premise: code 2 is the one whose Detail this whole change is about, so a
	// swap there is the swap that matters. A code with no Detail would make the
	// detail half vacuous.
	if len(exitCodeDocs[2].Detail) == 0 {
		t.Fatal("code 2 carries no Detail — the detail half of this guard would assert nothing")
	}

	restore := exitCodeDocs
	defer func() { exitCodeDocs = restore }()
	swapped := make([]ExitCodeDoc, len(restore))
	copy(swapped, restore)
	swapped[2].Summary = summarySentinel
	detail := make([]string, len(restore[2].Detail))
	copy(detail, restore[2].Detail)
	detail[0] = detailSentinel
	swapped[2].Detail = detail
	exitCodeDocs = swapped

	if !strings.Contains(readmeExitCodeTable(), summarySentinel) {
		t.Error("readmeExitCodeTable() is not rendered from exitCodeDocs' Summary")
	}
	if !strings.Contains(rootExitCodeHelp(), summarySentinel) {
		t.Error("rootExitCodeHelp() is not rendered from exitCodeDocs' Summary")
	}
	if !strings.Contains(renderRootHelp(t), summarySentinel) {
		t.Error("the RENDERED `civitai --help` does not carry a Summary change — it holds a hand-written copy")
	}
	if !strings.Contains(readmeExitCodeSections(), detailSentinel) {
		t.Error("readmeExitCodeSections() is not rendered from exitCodeDocs' Detail")
	}
	if strings.Contains(renderRootHelp(t), detailSentinel) {
		t.Error("`civitai --help` carries a DETAIL entry — the summary/detail split is gone and the terminal is back to printing the full ledger")
	}
}

// TestSummaryIsTheREADMECellAndTheHelpLine pins the containment relationship the
// two surfaces are allowed to have after the split: the Summary is the README
// table cell's opening text AND the `--help` line, de-emphasised. README may say
// MORE (its `### Exit code N` subsection); it may never say something DIFFERENT.
func TestSummaryIsTheREADMECellAndTheHelpLine(t *testing.T) {
	table := readmeExitCodeTable()
	help := flattenWS(rootExitCodeHelp())
	for _, d := range exitCodeDocs {
		if !strings.Contains(table, "| `"+itoa(d.Code)+"` | "+d.Summary) {
			t.Errorf("code %d: the README table row does not open with the Summary %q\n%s", d.Code, d.Summary, table)
		}
		if !strings.Contains(help, flattenWS(plainify(d.Summary))) {
			t.Errorf("code %d: --help does not carry the Summary %q", d.Code, d.Summary)
		}
	}
}

// TestEveryCodeReachesTheTerminal is the "no code silently disappears from
// `--help`" guard. The split shortened what the terminal says about each code;
// it must not have shortened WHICH codes it says anything about, which is the
// one thing a script author reads `--help` for.
func TestEveryCodeReachesTheTerminal(t *testing.T) {
	help := renderRootHelp(t)
	section := help[strings.Index(help, "Exit codes:"):]
	for _, d := range exitCodeDocs {
		if !strings.Contains(section, "\n    "+itoa(d.Code)+"  ") {
			t.Errorf("exit code %d has no line in the rendered `--help` exit-code section:\n%s", d.Code, section)
		}
	}
}

// TestHelpExitCodeSectionStaysSkimmable is the PRODUCT property this change
// exists to deliver, asserted on the rendered terminal output rather than
// inferred from the generator.
//
// Measured on the binary: the pre-split section was 62 lines (widest 81 runes)
// out of a 143-line `civitai --help`; it is now 19 out of 100. The caps below
// are deliberately loose — they constrain the failure mode (the wall coming
// back) and not ordinary editing.
//
// It is here because the sentinel guard's "a Detail entry must not reach
// `--help`" assertion was the ONLY thing observing that property, and a battery
// resting on one row is a battery anyone can delete by accident. This one is
// built differently: it never looks at Detail at all, it reads the rendered
// bytes, and it dies on the same mutation.
func TestHelpExitCodeSectionStaysSkimmable(t *testing.T) {
	const (
		maxLines = 24
		wasLines = 62 // the pre-split section, measured on the binary
	)
	help := renderRootHelp(t)
	i := strings.Index(help, "Exit codes:")
	if i < 0 {
		t.Fatalf("the rendered help has no exit-code section — this guard is reading the wrong text:\n%s", help)
	}
	lines := strings.Split(strings.TrimRight(help[i:], "\n"), "\n")

	// Positive control: a section shorter than one line per code would satisfy
	// every cap below while publishing nothing.
	if len(lines) < len(exitCodeDocs) {
		t.Fatalf("the exit-code section is %d lines for %d codes — it cannot be publishing them all:\n%s",
			len(lines), len(exitCodeDocs), help[i:])
	}
	if len(lines) > maxLines {
		t.Errorf("the `--help` exit-code section is %d lines (cap %d; it was %d before the summary/detail split). "+
			"Depth belongs in Detail, which README publishes in full.", len(lines), maxLines, wasLines)
	}
	for n, line := range lines {
		if w := len([]rune(line)); w > exitCodeHelpWidth {
			t.Errorf("line %d of the `--help` exit-code section is %d runes (cap %d): %q", n+1, w, exitCodeHelpWidth, line)
		}
	}
}

// TestHelpPointsAtTheREADMESection pins the other half of the trade: `--help`
// no longer carries the ledger, so it has to say where the ledger IS, by a name
// that works offline. A pointer naming a heading that does not exist is worse
// than no pointer — it sends the reader looking for something that was renamed.
func TestHelpPointsAtTheREADMESection(t *testing.T) {
	help := flattenWS(renderRootHelp(t))
	if !strings.Contains(help, flattenWS(plainify(exitCodeHelpPointer))) {
		t.Errorf("`civitai --help` does not carry the pointer to the full ledger:\n%s", help)
	}
	readme := readREADME(t)
	if !strings.Contains(readme, "\n## "+exitCodeREADMESection+"\n") {
		t.Errorf("`--help` points readers at a %q section of README.md, which has no such heading", exitCodeREADMESection)
	}
}

// TestSummaryTextIsTerminalSafe constrains the Summary to the emphasis plainify
// can remove. A markdown link or an image in a Summary would reach the terminal
// as raw syntax, so it belongs in Detail — which is README-only and may carry
// links (code 3's does).
func TestSummaryTextIsTerminalSafe(t *testing.T) {
	for _, d := range exitCodeDocs {
		s := d.Summary
		if strings.Contains(s, "](") {
			t.Errorf("code %d: Summary carries a markdown link (README-only — move it to Detail): %q", d.Code, s)
		}
		if strings.Contains(s, "|") {
			t.Errorf("code %d: Summary carries a `|`, which would break the README table row: %q", d.Code, s)
		}
		if plain := plainify(s); strings.ContainsAny(plain, "`*") {
			t.Errorf("code %d: plainify left markdown emphasis in %q", d.Code, plain)
		}
		if strings.Contains(s, "\n") {
			t.Errorf("code %d: Summary carries a newline, which would break the README table row: %q", d.Code, s)
		}
	}
}

// TestSummariesStaySkimmable is the reason this change exists, asserted rather
// than hoped for. The pre-split code-2 entry was 2,525 characters in one
// terminal blob; a summary that grows back toward that is the defect returning.
// The cap is generous — roughly three wrapped terminal lines — so it constrains
// only the failure mode, not ordinary editing.
func TestSummariesStaySkimmable(t *testing.T) {
	const maxSummaryRunes = 220
	for _, d := range exitCodeDocs {
		if n := len([]rune(plainify(d.Summary))); n > maxSummaryRunes {
			t.Errorf("code %d's Summary is %d runes (cap %d) — it is growing back into the wall the split removed. "+
				"Put the depth in Detail, which README publishes in full.", d.Code, n, maxSummaryRunes)
		}
	}
}

// itoa keeps the assertions above free of a strconv import in a file that
// otherwise deals only in strings.
func itoa(n int) string { return strconv.Itoa(n) }

// TestExitCodeDocsAreDenseAndOrdered pins the shape cmd/civitai's constant
// ledger depends on: one entry per code, 0..N with no gaps.
func TestExitCodeDocsAreDenseAndOrdered(t *testing.T) {
	if len(exitCodeDocs) == 0 {
		t.Fatal("exitCodeDocs is empty")
	}
	for i, d := range exitCodeDocs {
		if d.Code != i {
			t.Errorf("exitCodeDocs[%d] documents code %d — the slice must be dense and ordered from 0", i, d.Code)
		}
		if strings.TrimSpace(d.Summary) == "" {
			t.Errorf("code %d has an empty Summary — an empty string makes every Contains assertion vacuously true", d.Code)
		}
	}
}

// TestImageUsageRefusalLedger is the behavioural half of the corrected README
// claim. imageUsageRefusals is rendered into the published contract; this test
// requires each phrase in it to name a refusal loadAndValidateImage really
// classifies as a usage error (exit 2), and requires the two sets to be EQUAL —
// so the ledger fails when it grows a phrase the code does not honour AND when
// the code grows a case the docs do not mention.
//
// Mutation-measured, per case: untagging any ONE of the five refusals reddens
// exactly that subtest, with this test's own message — so a green sweep here is
// not one case carrying five. One KNOWN EQUIVALENT MUTANT, recorded so nobody
// re-derives it as a hole: DELETING the `len(data) == 0` branch outright leaves
// every test green, because DecodeImageInfo then rejects the empty bytes with a
// usage-tagged error of its own. The documented claim (an empty file exits 2)
// stays TRUE under that mutation — only the message degrades — so the guard is
// right not to fail. What it must catch is a change to the CLASSIFICATION, and
// it does.
func TestImageUsageRefusalLedger(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// The fixture for each documented phrase. Keys must match imageUsageRefusals
	// exactly — the ledger assertion below is what makes that a requirement
	// rather than a convention.
	fixtures := map[string]string{
		"missing":             filepath.Join(dir, "nope.png"),
		"empty":               write("empty.png", nil),
		"a directory":         dir,
		"over the size cap":   write("huge.png", make([]byte, kindByteCap(kindIcon)+1)),
		"not a PNG/JPEG/WebP": write("notanimage.png", []byte("this is not an image at all")),
	}

	if len(fixtures) != len(imageUsageRefusals) {
		t.Fatalf("ledger size mismatch: %d documented phrases, %d fixtures", len(imageUsageRefusals), len(fixtures))
	}
	for _, phrase := range imageUsageRefusals {
		if _, ok := fixtures[phrase]; !ok {
			t.Fatalf("imageUsageRefusals documents %q with no fixture proving the code honours it", phrase)
		}
	}

	for _, phrase := range imageUsageRefusals {
		t.Run(phrase, func(t *testing.T) {
			_, _, err := loadAndValidateImage(kindIcon, fixtures[phrase])
			if err == nil {
				t.Fatalf("the published contract lists %q as an exit-2 refusal, but loadAndValidateImage ACCEPTED %s", phrase, fixtures[phrase])
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("the published contract lists %q as an exit-2 refusal, but the error is not ErrUsage-tagged (%T): %v\n"+
					"Either tag it, or drop the phrase from imageUsageRefusals — the docs and the code must agree.", phrase, err, err)
			}
		})
	}

	// The published sentence must actually name every ledger entry.
	sentence := joinPhrases(imageUsageRefusals)
	for _, phrase := range imageUsageRefusals {
		if !strings.Contains(sentence, phrase) {
			t.Errorf("rendered phrase list omits %q: %s", phrase, sentence)
		}
	}
	if !strings.Contains(exitCodeDocs[2].published(), sentence) {
		t.Errorf("exit code 2's documented text does not carry the refusal list %q", sentence)
	}
}

// TestUnreadableImageIsNotAUsageError pins the NEGATIVE half of the corrected
// README claim: "a file that exists but cannot be read … does not exit 2".
//
// 🔴 INVARIANT GUARD, not regression coverage — it passes at its own PR's base
// too. Its job is to stop a future "tidy-up" from tagging the os.ReadFile
// failure in loadAndValidateImage, which would silently make the published
// sentence false. It asserts only what the docs claim (not ErrUsage); which
// code the error DOES produce is cmd/civitai's business, and is pinned there —
// it was exit 5 until issue #241 fixed isNetworkErr, and is exit 1 now (see
// TestFilesystemErrorsAreNotNetworkErrors in cmd/civitai/fs_not_network_test.go).
func TestUnreadableImageIsNotAUsageError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a mode-000 file is still readable, so the probe cannot observe the case")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(p, pngBytes(t, 64, 64), 0o000); err != nil {
		t.Fatal(err)
	}

	// Positive control: the same bytes at 0600 must be ACCEPTED, so a failure
	// below is about readability and not about the fixture being a bad image.
	readable := filepath.Join(dir, "readable.png")
	if err := os.WriteFile(readable, pngBytes(t, 64, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadAndValidateImage(kindIcon, readable); err != nil {
		t.Fatalf("the fixture image must be valid, got: %v", err)
	}

	_, _, err := loadAndValidateImage(kindIcon, p)
	if err == nil {
		t.Skip("this filesystem ignores mode 000 — the unreadable case is not observable here")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("README/--help state that an unreadable file does NOT exit 2, but the error is ErrUsage-tagged: %v\n"+
			"Either untag it, or change exitCodeDocs — the published sentence and the code must agree.", err)
	}
}
