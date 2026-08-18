package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/credscan"
)

// The RENDERER's own edge cases, kept in a separate file from the end-to-end
// tests on purpose: this one imports internal/credscan, so at a base commit
// where that package does not exist it cannot COMPILE — and a compile failure is
// not evidence of a behaviour. app_submit_credential_warning_test.go imports
// nothing new, so it compiles at the base and goes red for the right reason.

// TestRenderCredentialWarningTruncatesKeepingTheFileCount covers the
// pathological-input guard, which a real project cannot cheaply produce: past
// the cap the LIST is elided and the FILE COUNT is not.
//
// 🔴 EVERY EXPECTATION IS A LITERAL, NOT credWarnListCap. The sibling
// skipped-line test records the measurement behind that rule: an expectation
// derived from the constant moved with a mutant that changed it, and the
// mutation SURVIVED a fully green run. 14 findings across 7 files, 12 shown,
// 2 elided, written out.
//
// 🔴 IT IS ALSO THE ONLY TEST THAT CAN SEE A HEADER COUNTING FINDINGS INSTEAD OF
// FILES — measured. The end-to-end fixture plants one credential per file, so
// there len(files) == len(findings) and the mutant `len(files) → len(findings)`
// SURVIVES it. That is why this fixture is 7 files × 2 lines rather than a flat
// list: the two numbers must differ, or the assertion cannot discriminate.
func TestRenderCredentialWarningTruncatesKeepingTheFileCount(t *testing.T) {
	var findings []credscan.Finding
	for i := 0; i < 7; i++ {
		for line := 1; line <= 2; line++ {
			findings = append(findings, credscan.Finding{
				Path:  fmt.Sprintf("src/mod%02d.ts", i),
				Line:  line,
				Label: "apiSecret",
			})
		}
	}

	var buf bytes.Buffer
	got := renderCredentialWarning(&buf, credscan.Report{Findings: findings, FilesScanned: 7, FilesTotal: 7})
	if got == "" {
		t.Fatal("rendered nothing for 14 findings")
	}
	lines := strings.Split(got, "\n")
	if want := "⚠ 7 packaged file(s) look like they hold credentials:"; lines[0] != want {
		t.Errorf("header = %q, want %q — the count is of FILES, and truncation must not change it", lines[0], want)
	}
	var entries, tail int
	for _, l := range lines[1:] {
		switch {
		case strings.Contains(l, "… and "):
			tail++
			if want := "    … and 2 more line(s)"; l != want {
				t.Errorf("tail = %q, want %q", l, want)
			}
		case strings.HasPrefix(l, "    "):
			entries++
		}
	}
	if entries != 12 {
		t.Errorf("listed %d entr(ies), want 12 (the cap)", entries)
	}
	if tail != 1 {
		t.Errorf("found %d truncation tail(s), want 1", tail)
	}

	// 🔴 AND THEY ARE THE FIRST TWELVE, WRITTEN OUT. A count-and-tail assertion
	// cannot see a truncation that keeps the WRONG END — `shown[len-12:]` passes
	// every check above while silently dropping the first six files, which are
	// the ones an author reads. Measured: that mutant survived until this block
	// existed.
	var locs []string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "    ") && !strings.Contains(l, "… and ") {
			locs = append(locs, strings.Fields(l)[0])
		}
	}
	want := []string{
		"src/mod00.ts:1", "src/mod00.ts:2", "src/mod01.ts:1", "src/mod01.ts:2",
		"src/mod02.ts:1", "src/mod02.ts:2", "src/mod03.ts:1", "src/mod03.ts:2",
		"src/mod04.ts:1", "src/mod04.ts:2", "src/mod05.ts:1", "src/mod05.ts:2",
	}
	if strings.Join(locs, ",") != strings.Join(want, ",") {
		t.Errorf("listed entries =\n  %v\nwant\n  %v\nThe cap must keep the FIRST findings, in order.", locs, want)
	}

	// The empty case renders nothing at all — asserted here rather than only
	// end-to-end, because this is the one call site and "" is what suppresses the
	// line.
	if s := renderCredentialWarning(&buf, credscan.Report{FilesScanned: 3, FilesTotal: 3}); s != "" {
		t.Errorf("renderCredentialWarning(empty, not truncated) = %q, want \"\"", s)
	}
}

// TestRenderCredentialWarningAnnouncesATruncatedScan is the reassuring-zero
// guard. Past its byte budget the scan stops, and a stop that printed nothing
// would be indistinguishable from a clean bundle — the one conclusion this
// feature must never invite.
//
// Both arms matter and they render differently: with findings the stop is a note
// under the list; with NONE it is the only line printed, and it has to be a
// warning in its own right.
func TestRenderCredentialWarningAnnouncesATruncatedScan(t *testing.T) {
	var buf bytes.Buffer

	// Arm 1: truncated, nothing found. This is the dangerous one.
	got := renderCredentialWarning(&buf, credscan.Report{Truncated: true, FilesScanned: 40, FilesTotal: 900})
	if got == "" {
		t.Fatal("a truncated scan with no findings rendered NOTHING — that is exactly the reassuring zero " +
			"this feature exists to prevent")
	}
	for _, want := range []string{"stopped after 40 of 900", "NOT checked"} {
		if !strings.Contains(got, want) {
			t.Errorf("truncation line = %q, want it to contain %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "⚠") {
		t.Errorf("truncation-only output = %q, want it to lead with the warning glyph — it is the only line "+
			"printed, so it carries the whole message", got)
	}

	// Arm 2: truncated WITH findings — the list is still the headline.
	got = renderCredentialWarning(&buf, credscan.Report{
		Findings:     []credscan.Finding{{Path: "a.json", Line: 1, Label: "API_SECRET"}},
		Truncated:    true,
		FilesScanned: 40,
		FilesTotal:   900,
	})
	if !strings.Contains(got, "1 packaged file(s) look like they hold credentials:") {
		t.Errorf("output = %q, want the findings header", got)
	}
	if !strings.Contains(got, "stopped after 40 of 900") {
		t.Errorf("output = %q, want it to ALSO say the scan stopped early", got)
	}
}
