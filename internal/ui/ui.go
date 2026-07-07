// Package ui is the single, cohesive presentation layer for the civitai CLI.
//
// Everything user-facing that wants color, glyphs, or a spinner goes through
// here so the rules live in ONE place:
//
//   - Color is configured exactly once, from the root command's
//     PersistentPreRunE, via Configure. Precedence (highest first):
//     --no-color / NO_COLOR (force OFF) > --color / CLICOLOR_FORCE (force ON) >
//     auto (ON only when the target writer is a real TTY and TERM != "dumb").
//   - The force overrides are ABSOLUTE and stream-independent. The auto case is
//     resolved PER-WRITER: color is enabled for exactly the streams that are a
//     real terminal. So a piped stdout with a still-TTY stderr yields plain
//     stdout AND colored stderr — the bare helpers render against stdout (the
//     configured default writer), a Printer/Styler renders against the writer it
//     is bound to. See EnabledFor.
//   - The styled-string helpers (Success, Warn, ErrorMsg, Info, Bold, Dim, URL,
//     Code) RETURN strings so call sites keep using fmt.Fprintf and output stays
//     composable + testable. When color is disabled every helper returns PLAIN
//     text — the glyph prefixes stay (they are meaningful ASCII/Unicode) but NO
//     ANSI escape ever leaks (guaranteed by an Ascii lipgloss color profile).
//   - Machine-readable output (--json / --quiet / any structured path) must NOT
//     go through these helpers. See CONVENTION.md.
//
// Configure is safe to call once at startup; the helpers are safe to call from
// any goroutine afterwards (guarded by an RWMutex). If Configure is never called
// the helpers behave as if disabled (plain text) — the safe default.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// Options configures color enablement. The flag bools come from the root
// command's persistent --no-color / --color flags; Writer is the destination
// the BARE helpers resolve their auto color against (typically stdout). The
// NO_COLOR / CLICOLOR_FORCE / TERM environment variables are consulted by
// Configure itself.
type Options struct {
	// NoColor is the resolved --no-color flag (force color OFF).
	NoColor bool
	// ForceColor is the resolved --color flag (force color ON even off a TTY).
	ForceColor bool
	// Writer is where the bare helpers' output goes; the auto path enables their
	// color only when this is a real terminal. Typically os.Stdout. A Printer or
	// Styler bound to a different stream resolves auto against THAT stream.
	Writer io.Writer
}

// colorMode is the resolved, stream-independent part of the color decision. The
// force cases are absolute; modeAuto defers to the specific writer's TTY-ness.
type colorMode int

const (
	// modeOff forces color off (default until Configure runs — the safe plain
	// default so any helper called pre-configuration never leaks ANSI).
	modeOff colorMode = iota
	// modeOn forces color on regardless of the writer.
	modeOn
	// modeAuto enables color per-writer: on for a real TTY, off otherwise.
	modeAuto
)

var (
	mu sync.RWMutex
	// mode is the resolved force/auto decision (stream-independent).
	mode colorMode
	// defaultWriter is what the bare package-level helpers resolve auto against
	// (the configured Writer — stdout). nil means os.Stdout.
	defaultWriter io.Writer
)

// isTerminal reports whether w is a real terminal we may color/animate on. It is
// a package var so tests can simulate a TTY stream without a real terminal.
var isTerminal = func(w io.Writer) bool {
	if f, ok := w.(*os.File); ok && f != nil {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// styleSet is one fully-built palette (either the plain Ascii set or the colored
// ANSI256 set). Both are built once at init and are immutable thereafter, so the
// per-writer decision is a cheap pick between the two.
type styleSet struct {
	success lipgloss.Style
	warn    lipgloss.Style
	err     lipgloss.Style
	info    lipgloss.Style
	bold    lipgloss.Style
	dim     lipgloss.Style
	url     lipgloss.Style
	code    lipgloss.Style
	spinner lipgloss.Style
}

var (
	plainStyles styleSet
	colorStyles styleSet
)

func init() {
	// Build BOTH palettes once. Enablement is now decided per-writer at render
	// time (stylesFor), so both must always be available.
	plainStyles = buildStyles(false)
	colorStyles = buildStyles(true)
}

// buildStyles constructs a palette. When off, a renderer pinned to the Ascii
// profile guarantees Render() emits zero ANSI; when on, ANSI256 gives a
// deterministic, widely-supported palette (so golden tests of enabled output are
// stable). The two contrast-sensitive styles (URL, spinner) use AdaptiveColor so
// they stay legible on both light and dark terminal backgrounds; the Dark
// variant matches the historical fixed color, and lipgloss resolves adaptive
// colors deterministically here (the io.Discard renderer reports a dark
// background) so golden tests remain stable.
func buildStyles(on bool) styleSet {
	r := lipgloss.NewRenderer(io.Discard)
	if on {
		r.SetColorProfile(termenv.ANSI256)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	return styleSet{
		success: r.NewStyle().Foreground(lipgloss.Color("42")),  // green
		warn:    r.NewStyle().Foreground(lipgloss.Color("214")), // amber
		err:     r.NewStyle().Foreground(lipgloss.Color("203")), // red
		info:    r.NewStyle().Foreground(lipgloss.Color("39")),  // blue
		bold:    r.NewStyle().Bold(true),
		dim:     r.NewStyle().Faint(true),
		// URL: underlined cyan on dark, a deeper blue on light (256-color 25) so
		// it doesn't wash out on a white background.
		url: r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "45"}).Underline(true),
		// Code: magenta-ish — fixed is fine (reads on both).
		code: r.NewStyle().Foreground(lipgloss.Color("170")),
		// Spinner accent: pink on dark, a stronger magenta (162) on light.
		spinner: r.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "162", Dark: "205"}),
	}
}

// Configure resolves the color mode from opts + environment and records the
// bare-helper default writer. Call it once, early (the root PersistentPreRunE).
// Precedence:
//
//	--no-color / NO_COLOR      → OFF  (highest)
//	--color   / CLICOLOR_FORCE → ON
//	auto: per-writer TTY (TERM != "dumb")
func Configure(o Options) {
	m := resolveMode(o)
	w := o.Writer
	mu.Lock()
	defer mu.Unlock()
	mode = m
	defaultWriter = w
}

// resolveMode applies the documented precedence for the stream-independent part
// of the decision. Split out (pure-ish, reads env) so it is unit-testable via
// t.Setenv.
func resolveMode(o Options) colorMode {
	// Force OFF wins over everything.
	if o.NoColor || envSet("NO_COLOR") {
		return modeOff
	}
	// Force ON next.
	if o.ForceColor || envTrue("CLICOLOR_FORCE") {
		return modeOn
	}
	// A dumb terminal can't render our styling — treat as forced off.
	if os.Getenv("TERM") == "dumb" {
		return modeOff
	}
	return modeAuto
}

// resolveEnabled resolves the full decision (mode + the writer's TTY-ness) for
// o.Writer. Retained as the single-writer convenience used by the precedence
// tests; the runtime path uses EnabledFor.
func resolveEnabled(o Options) bool {
	switch resolveMode(o) {
	case modeOff:
		return false
	case modeOn:
		return true
	default:
		return isTerminal(o.Writer)
	}
}

// envSet reports whether name is present and non-empty (the NO_COLOR contract:
// any non-empty value disables color).
func envSet(name string) bool {
	v, ok := os.LookupEnv(name)
	return ok && v != ""
}

// envTrue reports whether name is set to a non-empty, non-"0" value.
func envTrue(name string) bool {
	v, ok := os.LookupEnv(name)
	return ok && v != "" && v != "0"
}

// EnabledFor reports whether styled (colored) output is enabled for the specific
// writer w. The force modes are absolute; in auto mode the answer is w's
// TTY-ness — so color follows each stream independently (piped stdout off, TTY
// stderr on).
func EnabledFor(w io.Writer) bool {
	mu.RLock()
	m := mode
	mu.RUnlock()
	switch m {
	case modeOff:
		return false
	case modeOn:
		return true
	default:
		return isTerminal(w)
	}
}

// Enabled reports whether styled output is on for the configured default writer
// (stdout). Prefer EnabledFor(w) when the destination stream matters.
func Enabled() bool { return EnabledFor(curDefaultWriter()) }

// curDefaultWriter returns the writer the bare helpers resolve against.
func curDefaultWriter() io.Writer {
	mu.RLock()
	w := defaultWriter
	mu.RUnlock()
	if w == nil {
		return os.Stdout
	}
	return w
}

// stylesFor picks the palette for w's resolved enablement.
func stylesFor(w io.Writer) styleSet {
	if EnabledFor(w) {
		return colorStyles
	}
	return plainStyles
}

// ── Styler: writer-bound styled-string rendering ─────────────────────────────

// Styler renders styled strings resolved against a SPECIFIC writer's color
// enablement. Use it (via For) at stderr call sites where the bare helpers —
// which resolve against stdout — would mis-decide (e.g. colored stderr while
// stdout is piped). The methods mirror the package-level helpers.
type Styler struct{ w io.Writer }

// For returns a Styler bound to w.
func For(w io.Writer) Styler { return Styler{w: w} }

func (s Styler) set() styleSet { return stylesFor(s.w) }

// Success renders a "✓ <str>" success line in green (glyph kept when disabled).
func (s Styler) Success(str string) string { return s.set().success.Render("✓ " + str) }

// Warn renders a "⚠ <str>" warning line in amber.
func (s Styler) Warn(str string) string { return s.set().warn.Render("⚠ " + str) }

// ErrorMsg renders a "✗ <str>" error line in red.
func (s Styler) ErrorMsg(str string) string { return s.set().err.Render("✗ " + str) }

// Info renders an informational line in blue (no glyph).
func (s Styler) Info(str string) string { return s.set().info.Render(str) }

// Bold renders str bold.
func (s Styler) Bold(str string) string { return s.set().bold.Render(str) }

// Dim renders str faint/dim.
func (s Styler) Dim(str string) string { return s.set().dim.Render(str) }

// URL renders a URL in underlined cyan/blue.
func (s Styler) URL(str string) string { return s.set().url.Render(str) }

// Code renders inline code / a literal command.
func (s Styler) Code(str string) string { return s.set().code.Render(str) }

// ── styled-string helpers (return strings; plain when disabled) ──────────────
//
// These resolve against the configured default writer (stdout). For output
// written to a different stream (stderr), bind a Styler with For(w) so color
// follows that stream.

// Success renders a "✓ <s>" success line in green (glyph kept when disabled).
func Success(s string) string { return For(curDefaultWriter()).Success(s) }

// Warn renders a "⚠ <s>" warning line in amber.
func Warn(s string) string { return For(curDefaultWriter()).Warn(s) }

// ErrorMsg renders a "✗ <s>" error line in red.
func ErrorMsg(s string) string { return For(curDefaultWriter()).ErrorMsg(s) }

// Info renders an informational line in blue (no glyph).
func Info(s string) string { return For(curDefaultWriter()).Info(s) }

// Bold renders s bold.
func Bold(s string) string { return For(curDefaultWriter()).Bold(s) }

// Dim renders s faint/dim.
func Dim(s string) string { return For(curDefaultWriter()).Dim(s) }

// URL renders a URL in underlined cyan (the one thing a user must click).
func URL(s string) string { return For(curDefaultWriter()).URL(s) }

// Code renders inline code / a literal command.
func Code(s string) string { return For(curDefaultWriter()).Code(s) }

// ── Printer: an ergonomic writer-bound wrapper ───────────────────────────────

// Printer binds an io.Writer so call sites can emit styled lines without
// threading the writer through every helper. Each method writes ONE line
// (newline-terminated) and resolves color against the BOUND writer, so a Printer
// over stderr colors independently of stdout. Nil-safe: a zero Printer writes to
// os.Stdout.
type Printer struct {
	w io.Writer
}

// NewPrinter binds p to w.
func NewPrinter(w io.Writer) *Printer { return &Printer{w: w} }

func (p *Printer) writer() io.Writer {
	if p == nil || p.w == nil {
		return os.Stdout
	}
	return p.w
}

// styler returns a Styler bound to the Printer's writer (per-stream color).
func (p *Printer) styler() Styler { return For(p.writer()) }

// Successf writes a styled success line.
func (p *Printer) Successf(format string, a ...any) {
	fmt.Fprintln(p.writer(), p.styler().Success(fmt.Sprintf(format, a...)))
}

// Warnf writes a styled warning line.
func (p *Printer) Warnf(format string, a ...any) {
	fmt.Fprintln(p.writer(), p.styler().Warn(fmt.Sprintf(format, a...)))
}

// Errorf writes a styled error line.
func (p *Printer) Errorf(format string, a ...any) {
	fmt.Fprintln(p.writer(), p.styler().ErrorMsg(fmt.Sprintf(format, a...)))
}

// Infof writes a styled info line.
func (p *Printer) Infof(format string, a ...any) {
	fmt.Fprintln(p.writer(), p.styler().Info(fmt.Sprintf(format, a...)))
}

// ── spinner ──────────────────────────────────────────────────────────────────

// Spinner returns a bubbles spinner model pre-styled to match the CLI's accent.
// Callers embed it in their own bubbletea models (e.g. the dev-tunnel wait);
// simple "spin while doing X" sites should use WithSpinner instead. The accent
// is resolved against the default writer (stdout) — the spinner only animates on
// a TTY, and --no-color forces it plain.
func Spinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = stylesFor(curDefaultWriter()).spinner
	return s
}

// IsTTY reports whether w is a terminal we can animate on. Exposed so callers
// (and the dev-tunnel wait) share one definition of "animatable".
func IsTTY(w io.Writer) bool { return isTerminal(w) }
