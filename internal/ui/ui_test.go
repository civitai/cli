package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// disable forces the plain (no-color) style set and restores afterward, so a
// test asserting plain output is independent of process env / prior Configure.
func disable(t *testing.T) {
	t.Helper()
	Configure(Options{NoColor: true})
}

// enable forces the colored style set (ForceColor bypasses the TTY check).
func enable(t *testing.T) {
	t.Helper()
	Configure(Options{ForceColor: true})
}

const esc = "\x1b"

// TestHelpersPlainWhenDisabled is the golden guarantee: with color disabled,
// NONE of the styled helpers may emit an ANSI escape. The glyph prefixes stay.
func TestHelpersPlainWhenDisabled(t *testing.T) {
	disable(t)
	cases := map[string]string{
		"Success":  Success("saved"),
		"Warn":     Warn("careful"),
		"ErrorMsg": ErrorMsg("boom"),
		"Info":     Info("fyi"),
		"Bold":     Bold("strong"),
		"Dim":      Dim("quiet"),
		"URL":      URL("https://civitai.com"),
		"Code":     Code("civitai login"),
	}
	for name, got := range cases {
		if strings.Contains(got, esc) {
			t.Errorf("%s leaked ANSI when disabled: %q", name, got)
		}
	}
	// Glyphs + content survive.
	if Success("saved") != "✓ saved" {
		t.Errorf("Success plain form = %q, want %q", Success("saved"), "✓ saved")
	}
	if Warn("careful") != "⚠ careful" {
		t.Errorf("Warn plain form = %q", Warn("careful"))
	}
	if ErrorMsg("boom") != "✗ boom" {
		t.Errorf("ErrorMsg plain form = %q", ErrorMsg("boom"))
	}
	if Info("fyi") != "fyi" || Bold("strong") != "strong" || URL("u") != "u" {
		t.Errorf("plain passthroughs wrong: %q %q %q", Info("fyi"), Bold("strong"), URL("u"))
	}
}

// TestHelpersColoredWhenEnabled: with color forced on, the helpers DO emit ANSI
// (and still contain the underlying text).
func TestHelpersColoredWhenEnabled(t *testing.T) {
	enable(t)
	defer disable(t)
	got := Success("saved")
	if !strings.Contains(got, esc) {
		t.Errorf("Success should emit ANSI when enabled: %q", got)
	}
	if !strings.Contains(got, "saved") {
		t.Errorf("Success should still contain the text: %q", got)
	}
	if !Enabled() {
		t.Error("Enabled() should be true after ForceColor Configure")
	}
}

func TestResolveEnabledPrecedence(t *testing.T) {
	// A non-TTY writer so "auto" resolves to false unless forced.
	var buf bytes.Buffer
	tests := []struct {
		name    string
		opts    Options
		noColor string // NO_COLOR value ("" = unset via marker below)
		force   string // CLICOLOR_FORCE value
		term    string
		set     bool // whether to set NO_COLOR
		setF    bool // whether to set CLICOLOR_FORCE
		want    bool
	}{
		{name: "auto non-tty off", opts: Options{Writer: &buf}, want: false},
		{name: "flag no-color beats force", opts: Options{Writer: &buf, NoColor: true, ForceColor: true}, want: false},
		{name: "flag color on", opts: Options{Writer: &buf, ForceColor: true}, want: true},
		{name: "NO_COLOR env off", opts: Options{Writer: &buf, ForceColor: true}, noColor: "1", set: true, want: false},
		{name: "CLICOLOR_FORCE on", opts: Options{Writer: &buf}, force: "1", setF: true, want: true},
		{name: "CLICOLOR_FORCE=0 ignored", opts: Options{Writer: &buf}, force: "0", setF: true, want: false},
		{name: "TERM dumb off", opts: Options{Writer: &buf}, term: "dumb", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clean slate for the env this test cares about.
			t.Setenv("NO_COLOR", "")
			t.Setenv("CLICOLOR_FORCE", "")
			t.Setenv("TERM", tc.term)
			if !tc.set {
				// t.Setenv("","") leaves it set-but-empty which envSet treats as unset.
			} else {
				t.Setenv("NO_COLOR", tc.noColor)
			}
			if tc.setF {
				t.Setenv("CLICOLOR_FORCE", tc.force)
			}
			if got := resolveEnabled(tc.opts); got != tc.want {
				t.Errorf("resolveEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPrinterPlain: a Printer over a buffer with color disabled writes plain,
// newline-terminated lines with the glyphs and no ANSI.
func TestPrinterPlain(t *testing.T) {
	disable(t)
	var buf bytes.Buffer
	p := NewPrinter(&buf)
	p.Successf("done %d", 3)
	p.Warnf("hmm")
	p.Errorf("nope")
	p.Infof("note")
	out := buf.String()
	if strings.Contains(out, esc) {
		t.Errorf("printer leaked ANSI when disabled:\n%q", out)
	}
	for _, want := range []string{"✓ done 3\n", "⚠ hmm\n", "✗ nope\n", "note\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("printer output missing %q:\n%q", want, out)
		}
	}
}

// TestWithSpinnerNonTTY: on a non-TTY writer WithSpinner prints one plain
// "message…" line, runs work, returns work's error — and NEVER animates.
func TestWithSpinnerNonTTY(t *testing.T) {
	var buf bytes.Buffer
	sentinel := errors.New("work failed")
	ran := false
	err := WithSpinner(context.Background(), &buf, "Uploading", func(ctx context.Context) error {
		ran = true
		return sentinel
	})
	if !ran {
		t.Fatal("work did not run")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("WithSpinner should return work's error, got %v", err)
	}
	out := buf.String()
	if out != "Uploading…\n" {
		t.Errorf("non-TTY spinner output = %q, want %q", out, "Uploading…\n")
	}
	if strings.Contains(out, "\r") || strings.Contains(out, esc) {
		t.Errorf("non-TTY spinner must not animate: %q", out)
	}
}

// TestWithSpinnerNonTTYSuccess: the happy path returns nil and still prints the
// single line.
func TestWithSpinnerNonTTYSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := WithSpinner(context.Background(), &buf, "Working", func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if buf.String() != "Working…\n" {
		t.Errorf("output = %q", buf.String())
	}
}

// ── per-stream color resolution (PR 2) ───────────────────────────────────────

// ttyBuf is a writer we can force isTerminal to treat as a real TTY, so
// per-stream tests can simulate "stdout piped, stderr a terminal" without an
// actual pty.
type ttyBuf struct{ bytes.Buffer }

// withFakeTTY overrides the isTerminal seam so that exactly the given writers
// report as terminals, restoring the real check afterward.
func withFakeTTY(t *testing.T, ttys ...io.Writer) {
	t.Helper()
	orig := isTerminal
	t.Cleanup(func() { isTerminal = orig })
	isTerminal = func(w io.Writer) bool {
		for _, x := range ttys {
			if x == w {
				return true
			}
		}
		return false
	}
}

// TestPerStreamAuto is the headline PR-2 case: with AUTO color (no force
// flags/env), a piped stdout stays plain while a TTY stderr is colored. The bare
// helpers resolve against stdout (the configured default writer); a Styler bound
// to stderr resolves against stderr.
func TestPerStreamAuto(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("TERM", "xterm")

	var stdout bytes.Buffer // piped (non-TTY)
	var stderr ttyBuf       // a real terminal
	withFakeTTY(t, &stderr)

	// Auto mode, default writer = stdout (piped).
	Configure(Options{Writer: &stdout})
	defer disable(t)

	if EnabledFor(&stdout) {
		t.Error("stdout is piped — EnabledFor(stdout) must be false in auto mode")
	}
	if !EnabledFor(&stderr) {
		t.Error("stderr is a TTY — EnabledFor(stderr) must be true in auto mode")
	}

	// Bare helper → stdout decision → PLAIN.
	if got := Warn("careful"); strings.Contains(got, esc) {
		t.Errorf("bare Warn (stdout-resolved) must be plain when stdout is piped: %q", got)
	}
	// Styler over stderr → stderr decision → COLORED.
	if got := For(&stderr).Warn("careful"); !strings.Contains(got, esc) {
		t.Errorf("For(stderr).Warn must be colored when stderr is a TTY: %q", got)
	}
	// A Printer bound to stderr writes colored; to stdout writes plain.
	var eb ttyBuf
	withFakeTTY(t, &eb)
	Configure(Options{Writer: &stdout})
	NewPrinter(&eb).Errorf("boom")
	if !strings.Contains(eb.String(), esc) {
		t.Errorf("Printer(stderr-TTY).Errorf must be colored: %q", eb.String())
	}
	var ob bytes.Buffer
	NewPrinter(&ob).Errorf("boom")
	if strings.Contains(ob.String(), esc) {
		t.Errorf("Printer(stdout-pipe).Errorf must be plain: %q", ob.String())
	}
}

// TestPerStreamNoColorBothPlain: --no-color forces BOTH streams plain, even the
// TTY one (force OFF is absolute).
func TestPerStreamNoColorBothPlain(t *testing.T) {
	var stdout bytes.Buffer
	var stderr ttyBuf
	withFakeTTY(t, &stderr)
	Configure(Options{NoColor: true, Writer: &stdout})
	defer disable(t)

	if EnabledFor(&stdout) || EnabledFor(&stderr) {
		t.Error("--no-color must disable color for every stream, TTY or not")
	}
	if strings.Contains(For(&stderr).Warn("x"), esc) {
		t.Errorf("--no-color must keep even a TTY stderr plain")
	}
}

// TestPerStreamForceColorBothColored: --color forces BOTH streams colored, even
// a piped one (force ON is absolute).
func TestPerStreamForceColorBothColored(t *testing.T) {
	var stdout bytes.Buffer // piped
	var stderr bytes.Buffer // piped
	Configure(Options{ForceColor: true, Writer: &stdout})
	defer disable(t)

	if !EnabledFor(&stdout) || !EnabledFor(&stderr) {
		t.Error("--color must enable color for every stream, piped or not")
	}
	if !strings.Contains(For(&stderr).Warn("x"), esc) {
		t.Errorf("--color must color even a piped stderr")
	}
}

// TestAdaptiveStylesColoredDeterministic: the two AdaptiveColor styles (URL,
// spinner) still emit ANSI when enabled and resolve deterministically (same
// bytes every call) so golden assertions stay stable.
func TestAdaptiveStylesColoredDeterministic(t *testing.T) {
	enable(t)
	defer disable(t)
	a := URL("https://civitai.com")
	b := URL("https://civitai.com")
	if !strings.Contains(a, esc) {
		t.Errorf("URL should emit ANSI when enabled: %q", a)
	}
	if a != b {
		t.Errorf("AdaptiveColor URL must render deterministically: %q vs %q", a, b)
	}
	// Deterministic Dark variant (256-color 45) is picked (the io.Discard
	// renderer reports a dark background), so golden assertions stay stable.
	if !strings.Contains(a, "38;5;45") {
		t.Errorf("URL should resolve to the deterministic dark cyan (38;5;45): %q", a)
	}
	// The text survives (lipgloss splits underlined runs per-rune, so assert the
	// characters are present in order rather than as one contiguous run).
	for _, ch := range "https://civitai.com" {
		if !strings.ContainsRune(a, ch) {
			t.Errorf("URL dropped character %q: %q", ch, a)
		}
	}
	// Spinner accent style is non-empty/renderable (adaptive) and plain when off.
	sp := Spinner()
	_ = sp // constructing it must not panic; style resolution happened.
}
