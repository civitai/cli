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
	got := renderCredentialWarning(&buf, findings)
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

	// The empty case renders nothing at all — asserted here rather than only
	// end-to-end, because this is the one call site and "" is what suppresses the
	// line.
	if s := renderCredentialWarning(&buf, nil); s != "" {
		t.Errorf("renderCredentialWarning(nil) = %q, want \"\"", s)
	}
}
