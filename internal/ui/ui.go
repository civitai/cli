// Package ui is the single, cohesive presentation layer for the civitai CLI.
//
// Everything user-facing that wants color, glyphs, or a spinner goes through
// here so the rules live in ONE place:
//
//   - Color is configured exactly once, from the root command's
//     PersistentPreRunE, via Configure. Precedence (highest first):
//     --no-color / NO_COLOR (force OFF) > --color / CLICOLOR_FORCE (force ON) >
//     auto (ON only when the target writer is a real TTY and TERM != "dumb").
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
// status output is written to (used for the auto TTY check). The NO_COLOR /
// CLICOLOR_FORCE / TERM environment variables are consulted by Configure itself.
type Options struct {
	// NoColor is the resolved --no-color flag (force color OFF).
	NoColor bool
	// ForceColor is the resolved --color flag (force color ON even off a TTY).
	ForceColor bool
	// Writer is where status output goes; the auto path enables color only when
	// this is a real terminal. Typically os.Stderr.
	Writer io.Writer
}

var (
	mu      sync.RWMutex
	enabled bool

	styleSuccess lipgloss.Style
	styleWarn    lipgloss.Style
	styleError   lipgloss.Style
	styleInfo    lipgloss.Style
	styleBold    lipgloss.Style
	styleDim     lipgloss.Style
	styleURL     lipgloss.Style
	styleCode    lipgloss.Style
	spinnerStyle lipgloss.Style
)

func init() {
	// Default to DISABLED (plain) until Configure runs, so any helper called
	// before configuration never leaks ANSI. Build the plain style set.
	rebuildStyles(false)
}

// Configure resolves color enablement from opts + environment and (re)builds the
// style set. Call it once, early (the root PersistentPreRunE). Precedence:
//
//	--no-color / NO_COLOR      → OFF  (highest)
//	--color   / CLICOLOR_FORCE → ON
//	auto: TTY writer && TERM != "dumb"
func Configure(o Options) {
	on := resolveEnabled(o)
	mu.Lock()
	defer mu.Unlock()
	enabled = on
	rebuildStyles(on)
}

// resolveEnabled applies the documented precedence. Split out (pure-ish, reads
// env) so it is unit-testable via t.Setenv + a non-TTY Writer.
func resolveEnabled(o Options) bool {
	// Force OFF wins over everything.
	if o.NoColor || envSet("NO_COLOR") {
		return false
	}
	// Force ON next.
	if o.ForceColor || envTrue("CLICOLOR_FORCE") {
		return true
	}
	// Auto: only on a real terminal with a capable TERM.
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(o.Writer)
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

// isTerminal reports whether w is a real terminal we may color/animate on.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok && f != nil {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

// rebuildStyles constructs the style set for the given enablement. When off, a
// renderer pinned to the Ascii profile guarantees Render() emits zero ANSI; when
// on, ANSI256 gives a deterministic, widely-supported palette (so golden tests
// of the enabled output are stable).
func rebuildStyles(on bool) {
	r := lipgloss.NewRenderer(io.Discard)
	if on {
		r.SetColorProfile(termenv.ANSI256)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	styleSuccess = r.NewStyle().Foreground(lipgloss.Color("42")) // green
	styleWarn = r.NewStyle().Foreground(lipgloss.Color("214"))   // amber
	styleError = r.NewStyle().Foreground(lipgloss.Color("203"))  // red
	styleInfo = r.NewStyle().Foreground(lipgloss.Color("39"))    // blue
	styleBold = r.NewStyle().Bold(true)
	styleDim = r.NewStyle().Faint(true)
	styleURL = r.NewStyle().Foreground(lipgloss.Color("45")).Underline(true) // cyan
	styleCode = r.NewStyle().Foreground(lipgloss.Color("170"))               // magenta-ish
	spinnerStyle = r.NewStyle().Foreground(lipgloss.Color("205"))            // pink
}

// Enabled reports whether styled (colored) output is on.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// ── styled-string helpers (return strings; plain when disabled) ──────────────

// Success renders a "✓ <s>" success line in green (glyph kept when disabled).
func Success(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleSuccess.Render("✓ " + s)
}

// Warn renders a "⚠ <s>" warning line in amber.
func Warn(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleWarn.Render("⚠ " + s)
}

// ErrorMsg renders a "✗ <s>" error line in red.
func ErrorMsg(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleError.Render("✗ " + s)
}

// Info renders an informational line in blue (no glyph).
func Info(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleInfo.Render(s)
}

// Bold renders s bold.
func Bold(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleBold.Render(s)
}

// Dim renders s faint/dim.
func Dim(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleDim.Render(s)
}

// URL renders a URL in underlined cyan (the one thing a user must click).
func URL(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleURL.Render(s)
}

// Code renders inline code / a literal command.
func Code(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	return styleCode.Render(s)
}

// ── Printer: an ergonomic writer-bound wrapper ───────────────────────────────

// Printer binds an io.Writer so call sites can emit styled lines without
// threading the writer through every helper. Each method writes ONE line
// (newline-terminated). Nil-safe: a zero Printer writes to os.Stdout.
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

// Successf writes a styled success line.
func (p *Printer) Successf(format string, a ...any) {
	fmt.Fprintln(p.writer(), Success(fmt.Sprintf(format, a...)))
}

// Warnf writes a styled warning line.
func (p *Printer) Warnf(format string, a ...any) {
	fmt.Fprintln(p.writer(), Warn(fmt.Sprintf(format, a...)))
}

// Errorf writes a styled error line.
func (p *Printer) Errorf(format string, a ...any) {
	fmt.Fprintln(p.writer(), ErrorMsg(fmt.Sprintf(format, a...)))
}

// Infof writes a styled info line.
func (p *Printer) Infof(format string, a ...any) {
	fmt.Fprintln(p.writer(), Info(fmt.Sprintf(format, a...)))
}

// ── spinner ──────────────────────────────────────────────────────────────────

// Spinner returns a bubbles spinner model pre-styled to match the CLI's accent.
// Callers embed it in their own bubbletea models (e.g. the dev-tunnel wait);
// simple "spin while doing X" sites should use WithSpinner instead.
func Spinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	mu.RLock()
	s.Style = spinnerStyle
	mu.RUnlock()
	return s
}

// IsTTY reports whether w is a terminal we can animate on. Exposed so callers
// (and the dev-tunnel wait) share one definition of "animatable".
func IsTTY(w io.Writer) bool { return isTerminal(w) }
