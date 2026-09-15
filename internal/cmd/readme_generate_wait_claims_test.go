package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/genapi"
)

// readme_generate_wait_claims_test.go pins the two README sentences
// civitai/cli#604 added, because both of them shipped FALSE and nothing could
// say so.
//
// 🔴 THE FAILURE WAS NOT A TYPO IN EITHER CASE — IT WAS PROSE CLAIMING MORE
// THAN THE CODE DOES, IN THE REASSURING DIRECTION.
//
//   - "One row per output" described printOutputURLs' loop as if it printed
//     every kept output. It numbers by index into `kept` and SKIPS an output
//     with no URL, and genapi.Deliverable does not require one — so a server
//     returning `url: null` legitimately produces a listing with a gap, and the
//     README promised a row that is not there.
//   - "every line `generate` writes while it waits … flattened the same way" was
//     false at the commit that wrote it: the quiet poll reporter rendered the
//     failed status check's error with a plain `%v`, and that error's text is
//     the server's own `message` verbatim.
//
// 🔴 TWO HALVES PER CLAIM, AND NEITHER ALONE IS THE GUARD. The STRING half pins
// the whole normalised bullet, because a guard on individual WORDS is walkable
// by rewording — this repo's rule for a prose artifact is to pin the normalised
// string and pay the cosmetic-reword cost for a machine-readable claim. The
// BEHAVIOURAL half drives the renderer the sentence is about, so the pair fails
// in both directions: reword the README and the string half goes red, change
// the renderer and the behavioural half does. A string pin alone would freeze a
// sentence without ever asking whether it is TRUE, which is the state both of
// these were in.

// readmeText returns README.md as one string.
func readmeText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: read README.md: %v", err)
	}
	return string(b)
}

// normaliseProse collapses every run of whitespace — the line wrapping included
// — to a single space, so the pinned literals below are about the WORDS and a
// re-wrap is not a false failure.
func normaliseProse(s string) string { return strings.Join(strings.Fields(s), " ") }

// readmeBullet extracts the top-level `- ` list item that starts with marker,
// up to the next top-level `- ` or a blank line followed by a non-indented line.
//
// It fails rather than returning "" on a miss: an extractor that silently found
// nothing satisfies every "the text does not contain X" check and reports a
// serene pass, which is the reassuring zero this repo keeps hitting.
func readmeBullet(t *testing.T, readme, marker string) string {
	t.Helper()
	lines := strings.Split(readme, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, marker) {
			if start >= 0 {
				t.Fatalf("CONTROL failure, not a finding: README.md holds more than one bullet starting %q "+
					"(lines %d and %d), so this test cannot say which one it pinned", marker, start+1, i+1)
			}
			start = i
		}
	}
	if start < 0 {
		t.Fatalf("CONTROL failure, not a finding: README.md holds no bullet starting %q. The claim this "+
			"test pins was moved, reworded past its opening words, or deleted — and an extractor that "+
			"finds nothing would otherwise pass every assertion below.", marker)
	}
	end := start + 1
	for end < len(lines) {
		l := lines[end]
		if strings.HasPrefix(l, "- ") || strings.TrimSpace(l) == "" || !strings.HasPrefix(l, "  ") {
			break
		}
		end++
	}
	return strings.Join(lines[start:end], "\n")
}

// wantNoDownloadBullet is README.md's `--no-download` bullet, normalised.
//
// Pinned WHOLE rather than by keyword: the defect it guards against is a
// sentence that over-claims, and any keyword subset of it can be over-claimed
// around.
const wantNoDownloadBullet = "- `--no-download` waits and prints the output URLs instead of writing files. " +
	"One row on **stdout** per output that came back **with a URL**, the CLI's own number and the URL " +
	"separated by a tab, so `cut -f2` is the intended read. The number is that output's position among " +
	"the run's kept outputs, so it skips an output the server returned without one — a gap in the " +
	"numbering is the CLI's own, not a dropped row. The URL is the **server's**, so it is flattened to " +
	"one line and one tab-separated field first: a presigned URL cannot add a numbered row of its own, " +
	"or an extra field, to that listing."

// TestREADMENoDownloadListingClaimMatchesTheRenderer pins the `--no-download`
// bullet against what printOutputURLs really writes.
func TestREADMENoDownloadListingClaimMatchesTheRenderer(t *testing.T) {
	got := normaliseProse(readmeBullet(t, readmeText(t), "- `--no-download` waits"))
	if got != wantNoDownloadBullet {
		t.Errorf("README.md's `--no-download` bullet has changed.\n--- README.md says ---\n%s\n"+
			"--- this test pins ---\n%s\n\nIf the RENDERER changed, change both. If only the wording "+
			"changed, update the literal — but re-read the behavioural half below first: the sentence this "+
			"replaced said \"one row per output\", which is false whenever an output arrives with no URL.",
			got, wantNoDownloadBullet)
	}

	// The behavioural half: a kept set whose middle output has no URL.
	urlOf := func(s string) *string { return &s }
	kept := []genapi.Output{
		{Blob: genapi.Blob{ID: "a", URL: urlOf("https://example.invalid/o/a.jpeg")}},
		{Blob: genapi.Blob{ID: "b", URL: nil}},
		{Blob: genapi.Blob{ID: "c", URL: urlOf("https://example.invalid/o/c.jpeg")}},
	}
	var out, errw bytes.Buffer
	printOutputURLs(&out, &errw, kept)
	rows := gfsLines(out.String())

	// (a) The README's narrowed claim: one row per output THAT HAS A URL.
	if len(rows) != 2 {
		t.Errorf("README.md promises one stdout row per output that came back WITH A URL. Three kept "+
			"outputs, two of them with a URL, rendered %d row(s):\n%s", len(rows), out.String())
	}
	// (b) The claim the README used to make, shown FALSE — so the old wording
	// cannot be restored as if it were merely shorter. This is the assertion that
	// makes the pin above a statement about the code rather than about the text.
	if len(rows) == len(kept) {
		t.Errorf("printOutputURLs wrote one row per KEPT output (%d of %d), which is the claim #604 round 0 "+
			"found false and this bullet was narrowed away from. If the renderer genuinely now emits a row "+
			"for a URL-less output, widen the bullet back — but say what that row CONTAINS:\n%s",
			len(rows), len(kept), out.String())
	}
	// (c) The gap the README describes: the numbers are positions in `kept`, so
	// the second row is numbered 3, not 2.
	if len(rows) == 2 {
		if n, _, _ := strings.Cut(rows[1], "\t"); n != "3" {
			t.Errorf("README.md says the number is the output's position among the kept outputs, so the row "+
				"after a URL-less output is numbered 3. It is numbered %q:\n%s", n, out.String())
		}
	}
}

// wantFlattenedFieldsSentence is the `label: value` bullet's FIRST sentence —
// the one that enumerates what is flattened — normalised.
//
// The bullet continues for another eight lines of caveats that this test has no
// claim on; pinning the whole bullet would make every edit to those caveats a
// failure of a guard about something else.
var flattenedFieldsSentenceRe = regexp.MustCompile(
	`(?s)- \*\*` + "`" + `label: value` + "`" + ` lines are a narrower promise\.\*\*(.*?) — are flattened the same way\.`)

const wantFlattenedFieldsSentence = "The single-line metadata fields — `images … --meta`'s model / " +
	"sampler / seed / resources, `app status --id`'s live URL and block id, `app listing status`'s " +
	"screenshot ids and captions, the `--no-wait` re-attach hint, and `generate`'s wait-path lines (the " +
	"submit receipt, the status line the poll prints or redraws, the server's own message when a status " +
	"check fails and is retried, and the re-attach block printed when a wait ends without a result)"

// TestREADMEFlattenedWaitPathClaimIsNotWiderThanTheCode pins the enumeration of
// what `generate` flattens while it waits, and the one surface that was raw.
func TestREADMEFlattenedWaitPathClaimIsNotWiderThanTheCode(t *testing.T) {
	readme := readmeText(t)
	m := flattenedFieldsSentenceRe.FindStringSubmatch(readme)
	if m == nil {
		t.Fatalf("CONTROL failure, not a finding: README.md's `label: value` bullet no longer matches the "+
			"extractor (pattern: %s), so every assertion below would be about nothing.", flattenedFieldsSentenceRe)
	}
	if got := normaliseProse(m[1]); got != wantFlattenedFieldsSentence {
		t.Errorf("README.md's flattened-fields enumeration has changed.\n--- README.md says ---\n%s\n"+
			"--- this test pins ---\n%s\n\nThe sentence this replaced said \"every line `generate` writes "+
			"while it waits\", which was false at the commit that wrote it AND is wider than the code even "+
			"now: the failure reason this command appends to its dead-end errors is deliberately "+
			"MULTI-LINE (serverReasonSuffix, indentContinuation), and the bullet three items down says so. "+
			"A universal quantifier here contradicts it.", got, wantFlattenedFieldsSentence)
	}

	// The contradiction guard. These two bullets make OPPOSITE claims about
	// generate's error path, and the flattened one is the one that drifted wider.
	// If the multi-line bullet ever stops naming it, the enumeration above may be
	// widened — but that is a decision, not a silent consequence.
	const multiLineClaim = "the same reason on `generate`'s **error** path"
	if !strings.Contains(readme, multiLineClaim) {
		t.Errorf("README.md's multi-line-surfaces bullet no longer names %q. The flattened-fields "+
			"enumeration above is worded to stay OFF generate's error path precisely because that reason "+
			"keeps its line breaks; if that stopped being true, both bullets move together.", multiLineClaim)
	}

	// The behavioural half, on the surface the sentence's new clause is about:
	// the poll's retry line, which renders the SERVER's own message.
	var b bytes.Buffer
	r := &quietPollReporter{w: &b, now: newFakeClock().Now, heartbeat: 0}
	r.tick(pollEvent{
		attempt: 2,
		status:  gfsHostileStatus,
		wait:    5 * time.Second,
		err:     apiErrorWithMessage(t, 500, gfsHostileServerMessage),
	})
	line := b.String()
	// POSITIVE CONTROL: both server operands must be in the rendered line, or the
	// flattening assertion is measured on a line that carried neither.
	if !strings.Contains(line, "processing") || !strings.Contains(line, "boom") {
		t.Fatalf("CONTROL failure, not a finding: the status and the server's message did not both reach "+
			"the poll's retry line:\n%q", line)
	}
	if n := strings.Count(strings.TrimSuffix(line, "\n"), "\n"); n != 0 {
		t.Errorf("README.md says the status line the poll prints — the server's own message included — is "+
			"flattened. It emitted %d extra line(s):\n%q", n, line)
	}
	if strings.Contains(line, "\t") {
		t.Errorf("README.md says that line is flattened to one line AND one tab-separated field. A TAB "+
			"survived:\n%q", line)
	}
}
