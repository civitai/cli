package cmd

// validate_print_test.go covers the PRINTER: the fourth print site, and the
// quoted-span rule that keeps remedy text pasteable.
//
// Split from app_validate_wrap_test.go because these are about
// validate_print.go's own behaviour rather than about `app validate`'s output
// contract, and because `app submit` is a different command.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/scaffold"
)

// lockfileMismatch renders page-vite and swaps its lockfile, producing the
// longest ERROR the CLI emits (~400 chars) — the one that reaches `app submit`.
func lockfileMismatch(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "block")
	if _, err := scaffold.Render(scaffold.PageVite, dir, scaffold.Data{Slug: "wrap-block", Name: "Wrap Block"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	// A rendered page-vite ships NO lockfile, so writing a pnpm one alone
	// produces the MISMATCH error (pnpm lockfile vs an npm buildCommand) rather
	// than the missing-lockfile one. The mismatch is the message that carries
	// the quoted remedy this file is about; the two-lockfile case would be a
	// third message again.
	if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err == nil {
		t.Fatal("page-vite now ships a package-lock.json — this fixture would produce the " +
			"two-lockfile error instead of the mismatch, and the assertions below are about the mismatch")
	}
	if err := os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("lockfileVersion: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// findingLines returns the wrapped FINDING lines from a command's stderr — the
// bulleted item plus its hanging-indent continuations.
//
// 🔴 NON-FINDING LINES ARE OUT OF SCOPE, AND THE EARLIER JUSTIFICATION FOR THAT
// WAS MISATTRIBUTED — corrected here from measurement rather than from memory.
// THREE distinct lines can exceed the budget, and only ONE is about a path:
//
//	130 runes  `app validate`: ✗ N validation error(s) in <dir>:
//	           DOES interpolate the directory the user typed, so its width is
//	           theirs to control and wrapping would break a path they may need
//	           to copy. Measured at a deep fixture path.
//	 82 runes  `app submit`: ✗ validation failed (N error(s)) — fix before
//	           submitting, or pass --skip-validate:
//	           a CONSTANT with NO path in it — measured byte-identical at a
//	           short and a deep path. It is over budget simply because it is a
//	           long sentence.
//	184 runes  `Error: refusing to submit without --yes …`
//	           not a header at all: `cmd/civitai/main.go` prints `Error: %v` for
//	           EVERY error returned by EVERY command.
//
// Wrapping that last one is a deliberate NON-change here. It is a CLI-wide error
// path rather than a validation printer, so it would alter every command's
// stderr at once, and AGENTS.md item 24 pins error-message preservation
// byte-for-byte across it. If it is worth wrapping, it is worth doing as its own
// change measured against that contract.
//
// So this helper scopes the width assertion to FINDINGS, which is what
// validate_print.go actually owns.
func findingLines(stderr string) []string {
	var out []string
	inItem := false
	for _, line := range strings.Split(stderr, "\n") {
		switch {
		case strings.HasPrefix(line, findingBullet):
			inItem = true
		case inItem && strings.HasPrefix(line, findingIndent):
		default:
			inItem = false
			continue
		}
		out = append(out, line)
	}
	return out
}

// TestAppSubmitWrapsItsErrorsToo is the FOURTH print site.
//
// 🔴 `app validate` and `printWarnings` wrapped; `app submit`'s ERROR loop did
// not, so a single run printed a 412-rune unwrapped error AND a wrapped warning
// — two layouts in one run, on the highest-traffic path and the last point
// before an app reaches review. Measured on the PR as first audited. It also
// made this change's own stated rationale ("one place fixes every long message")
// false, which is the part worth failing on.
func TestAppSubmitWrapsItsErrorsToo(t *testing.T) {
	dir := lockfileMismatch(t)
	stdout, stderr, err := run(t, "app", "submit", dir)
	if err == nil {
		t.Fatalf("a mismatched lockfile must fail submit before any upload\nstdout:%s\nstderr:%s", stdout, stderr)
	}
	lines := findingLines(stderr)
	// POSITIVE CONTROL: with no finding on screen every width assertion below is
	// vacuous, and `app submit` refusing early for some unrelated reason looks
	// identical from here.
	if len(lines) < 2 {
		t.Fatalf("app submit printed %d finding line(s); the lockfile error is ~400 chars and must wrap to "+
			"several:\n%s", len(lines), stderr)
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > findingWrapWidth {
			t.Errorf("app submit printed a %d-rune finding line, over the %d-rune budget — this is the "+
				"unwrapped print site, and it puts two layouts in one run:\n%s", n, findingWrapWidth, line)
		}
	}
	if !strings.Contains(unwrapFinding(stderr), "pnpm-lock.yaml is committed") {
		t.Errorf("wrapping lost the lockfile error's content:\n%s", stderr)
	}
}

// TestQuotedRemedySpansAreNotSplit pins the copy-pasteable unit.
//
// 🔴 THE WRAPPING FIX INTRODUCED A USER-VISIBLE REGRESSION OF ITS OWN.
// `"pnpm run build"` — the value the message tells the author to put in their
// manifest — came out as `"pnpm` / `run build"` across a line break, where the
// unwrapped base printed it whole. Remedy text a message says to paste has to
// survive the layout.
func TestQuotedRemedySpansAreNotSplit(t *testing.T) {
	dir := lockfileMismatch(t)
	_, stderr, err := run(t, "app", "validate", dir)
	if err == nil {
		t.Fatalf("fixture must fail validate:\n%s", stderr)
	}
	for _, want := range []string{`"pnpm run build"`, `"npm run build"`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("%s was split across a line break; it is a value the author has to paste:\n%s", want, stderr)
		}
	}
}

// TestFindingTokensQuotedSpans drives the tokenizer directly, where inputs the
// real messages do not happen to contain can still be exercised.
//
// 🔴 THE TWO FALLBACKS ARE THE POINT. Keeping a quoted span whole is only safe
// because an OVERLONG span is split back into words (otherwise the atomic span
// blows the width contract wrapping exists to hold) and because an UNBALANCED
// quote cannot swallow the tail (worst case must be exactly today's greedy
// behaviour, never worse).
func TestFindingTokensQuotedSpans(t *testing.T) {
	const width = 40
	cases := []struct {
		name string
		in   string
		want []string // nil = assert only the invariants below
	}{
		{"a quoted span is one token", `set "pnpm run build" now`,
			[]string{"set", `"pnpm run build"`, "now"}},
		{"balanced tokens are left alone", `the "page" surface`,
			[]string{"the", `"page"`, "surface"}},
		{"two spans on one line", `"a b" and "c d"`,
			[]string{`"a b"`, "and", `"c d"`}},
		{"an OVERLONG span is split back into words", `x "` + strings.Repeat("word ", 20) + `y" z`, nil},
		{"an UNBALANCED quote does not swallow the tail", `a 6" pipe and more words`,
			[]string{"a", `6"`, "pipe", "and", "more", "words"}},
		{"apostrophes are not quotes", `this project's own terms`,
			[]string{"this", "project's", "own", "terms"}},
		{"empty", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := findingTokens(c.in, width)
			if c.want != nil && strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("findingTokens(%q) = %q, want %q", c.in, got, c.want)
			}
			// Invariant for EVERY case: no MULTI-WORD token may exceed the
			// width, or an atomic span blows the wrap budget.
			for _, tok := range got {
				if len([]rune(tok)) > width && strings.Contains(tok, " ") {
					t.Errorf("a %d-rune multi-word token survived at width %d, which blows the wrap budget: %q",
						len([]rune(tok)), width, tok)
				}
			}
			// And nothing may be lost or reordered.
			flatIn := strings.Join(strings.Fields(c.in), " ")
			flatOut := strings.Join(strings.Fields(strings.Join(got, " ")), " ")
			if flatIn != flatOut {
				t.Errorf("tokenizing changed the text:\n got: %q\nwant: %q", flatOut, flatIn)
			}
		})
	}
}

// TestPrintFindingHoldsTheWidthWithQuotedSpans is the end-to-end pairing: the
// quoted-span rule must not be able to push a printed line over the budget.
func TestPrintFindingHoldsTheWidthWithQuotedSpans(t *testing.T) {
	msgs := []string{
		`set "` + strings.Repeat("a very long quoted phrase ", 8) + `" to fix it`,
		`an unbalanced 6" quote ` + strings.Repeat("and more words ", 20),
		strings.Repeat(`"a b" `, 40),
	}
	for i, msg := range msgs {
		var b strings.Builder
		printFinding(&b, msg)
		lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		if len(lines) < 2 {
			t.Errorf("msg %d did not wrap at all (%d line)", i, len(lines))
		}
		for _, line := range lines {
			if n := len([]rune(line)); n > findingWrapWidth {
				t.Errorf("msg %d printed a %d-rune line over the %d budget: %q", i, n, findingWrapWidth, line)
			}
		}
	}
}
