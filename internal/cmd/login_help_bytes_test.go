package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// TestLoginHelpEmitsNoControlBytes is the regression guard for issue #253:
// `civitai login --help` wrote a raw NUL to stdout, because --token's
// NoOptDefVal sentinel started with "\x00" and pflag interpolates NoOptDefVal
// into the help row it then SPLITS on the first "\x00" to align its two
// columns. The failure is invisible in a terminal and only shows up once the
// output is captured — `file(1)` calls it `data`, `grep` says "binary file
// matches", and `git diff` refuses to render it.
//
// 🔴 POSITIVE CONTROLS ARE MANDATORY HERE. "contains no NUL" is satisfied by
// empty output, and it is also satisfied by a build that dropped NoOptDefVal
// altogether (which would break bare `login --token` — the very feature the
// sentinel exists for). So this test additionally requires that the help it
// examined is real AND that it rendered the sentinel-carrying `--token` row
// CONTIGUOUSLY, which is the exact text that used to carry the stray byte.
func TestLoginHelpEmitsNoControlBytes(t *testing.T) {
	out, errOut, err := run(t, "login", "--help")
	if err != nil {
		t.Fatalf("login --help: %v", err)
	}
	help := out + errOut

	// --- Positive control 1: the help is real, not an empty buffer. -----------
	if len(help) < 200 {
		t.Fatalf("login --help produced %d bytes — too short to be real help; "+
			"the no-NUL assertion below would pass vacuously:\n%s", len(help), help)
	}
	if !strings.Contains(help, "Flags:") {
		t.Fatalf("login --help has no Flags: section — nothing to check:\n%s", help)
	}

	// --- Positive control 2: the row that carried the byte is present. --------
	// pflag renders an optional-value flag as `--token string[="<NoOptDefVal>"]`.
	// This half is deliberately independent of the sentinel's BYTES — it fires
	// only if NoOptDefVal was dropped altogether, which would break bare
	// `civitai login --token` ("flag needs an argument"). It must stay ahead of
	// the NUL scan and must NOT depend on the sentinel's value, or a build that
	// reintroduces the NUL fails here instead of at the assertion that names the
	// actual defect.
	const optionalValueMarker = `--token string[="`
	if !strings.Contains(help, optionalValueMarker) {
		t.Fatalf("login --help has no %q row — --token's NoOptDefVal is gone, so bare "+
			"`civitai login --token` would error \"flag needs an argument\". "+
			"Nothing renders the sentinel, so the NUL scan below would pass vacuously.\n%s",
			optionalValueMarker, help)
	}

	// --- The assertion itself. ------------------------------------------------
	if i := strings.IndexByte(help, 0x00); i >= 0 {
		t.Errorf("login --help emitted a raw NUL at byte offset %d (issue #253): "+
			"any captured copy of this output is BINARY to file(1), grep and git.\n"+
			"context: %q", i, help[max(0, i-40):min(len(help), i+40)])
	}

	// The row must also render CONTIGUOUSLY. With the NUL present pflag aligned
	// its two help columns on OUR byte rather than its own, padding the row
	// apart into `--token string[="` + spaces + the rest — so the contiguous
	// form did not exist in the broken output. This catches column corruption
	// from any future separator byte, not just 0x00.
	wantRow := fmt.Sprintf(`--token string[=%q]`, tokenFlagNoValue)
	if !strings.Contains(help, wantRow) {
		t.Errorf("login --help does not render the --token row contiguously as %q — "+
			"pflag split the row, which means the sentinel carries a byte pflag "+
			"treats as a column separator.\n%s", wantRow, help)
	}

	// A NUL is the byte that broke a real consumer, but every other C0 control
	// byte poisons a captured snapshot the same way. Newline and tab are the
	// only ones help output legitimately contains.
	for i := 0; i < len(help); i++ {
		b := help[i]
		if b < 0x20 && b != '\n' && b != '\t' {
			t.Errorf("login --help emitted control byte %#02x at offset %d — "+
				"help output must stay plain text.\ncontext: %q",
				b, i, help[max(0, i-40):min(len(help), i+40)])
			break
		}
	}
}

// TestTokenFlagNoValueSentinelIsPlainText pins the constant itself, so the
// defect cannot come back through a code path this package's help rendering
// does not happen to exercise. The sentinel is USER-VISIBLE (pflag prints it in
// the help row), and pflag's own column alignment uses "\x00" as its separator,
// so a control byte here is never merely cosmetic.
//
// It must also stay non-empty: pflag only permits the no-argument form
// (`civitai login --token`) when NoOptDefVal is non-empty.
func TestTokenFlagNoValueSentinelIsPlainText(t *testing.T) {
	if tokenFlagNoValue == "" {
		t.Fatal("tokenFlagNoValue must be non-empty — pflag rejects bare `--token` without a NoOptDefVal")
	}
	for i := 0; i < len(tokenFlagNoValue); i++ {
		if b := tokenFlagNoValue[i]; b < 0x20 || b == 0x7f {
			t.Errorf("tokenFlagNoValue contains control byte %#02x at index %d (%q); "+
				"it is rendered verbatim into `civitai login --help` — see issue #253",
				b, i, tokenFlagNoValue)
		}
	}
}
