package cmd

import (
	"errors"
	"os"
	"path/filepath"
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

// TestREADMETableIsGeneratedFromTheSameSlice is the README twin of the sentinel
// check above. Together they prove BOTH published surfaces read the same
// variable, which is what makes "they cannot drift apart" a structural claim
// rather than an observation about today's text.
func TestREADMETableIsGeneratedFromTheSameSlice(t *testing.T) {
	const sentinel = "ZZ-EXIT-DOC-SENTINEL-ZZ"

	restore := exitCodeDocs
	defer func() { exitCodeDocs = restore }()
	swapped := make([]ExitCodeDoc, len(restore))
	copy(swapped, restore)
	swapped[2].Summary = sentinel
	exitCodeDocs = swapped

	if !strings.Contains(readmeExitCodeTable(), sentinel) {
		t.Error("readmeExitCodeTable() is not rendered from exitCodeDocs")
	}
	if !strings.Contains(rootExitCodeHelp(), sentinel) {
		t.Error("rootExitCodeHelp() is not rendered from exitCodeDocs")
	}
}

// TestHelpTextIsAPrefixOfTheREADMECell pins the containment relationship the
// two surfaces are allowed to have: everything `--help` says about a code is
// the README cell's opening text, de-emphasised. README may say MORE (Extra);
// it may never say something DIFFERENT.
func TestHelpTextIsAPrefixOfTheREADMECell(t *testing.T) {
	for _, d := range exitCodeDocs {
		cell := d.readmeCell()
		if !strings.HasPrefix(cell, d.shared()) {
			t.Errorf("code %d: README cell does not open with the shared text\n cell:   %s\n shared: %s", d.Code, cell, d.shared())
		}
		if !strings.Contains(flattenWS(rootExitCodeHelp()), flattenWS(plainify(d.shared()))) {
			t.Errorf("code %d: --help does not carry the shared text %q", d.Code, d.shared())
		}
	}
}

// TestSharedExitCodeTextIsTerminalSafe constrains the shared text to the
// emphasis plainify can remove. A markdown link or an image in Summary/Notes
// would reach the terminal as raw syntax, so it belongs in Extra.
func TestSharedExitCodeTextIsTerminalSafe(t *testing.T) {
	for _, d := range exitCodeDocs {
		for _, s := range append([]string{d.Summary}, d.Notes...) {
			if strings.Contains(s, "](") {
				t.Errorf("code %d: shared text carries a markdown link (README-only — move it to Extra): %q", d.Code, s)
			}
			if strings.Contains(s, "|") {
				t.Errorf("code %d: shared text carries a `|`, which would break the README table row: %q", d.Code, s)
			}
			if plain := plainify(s); strings.ContainsAny(plain, "`*") {
				t.Errorf("code %d: plainify left markdown emphasis in %q", d.Code, plain)
			}
		}
	}
}

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
	if !strings.Contains(exitCodeDocs[2].shared(), sentence) {
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
