package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// civitai/cli#574 — generate's BLOB DOWNLOAD PATH printed the server-derived
// name raw in six error strings, while its own `Saved` line was gated.
//
// `downloadBlobTo`'s `name` is `filepath.Base(target)`, and `target` comes from
// `planOutputTarget(o.outName, …)` whose `{workflowId}` is read off the wire —
// `renderOutName`'s own doc comment says so. So every one of those strings is a
// terminal surface carrying uploader-influenced text, on the ONE path in this
// CLI that spends the user's money.
//
// 🔴 THE INCONSISTENCY IS WHAT MAKES IT A BUG RATHER THAN A GAP: the same
// function's success line already read `safeTerm(target)`. One value, two
// treatments, decided by whether the download worked.
//
// # blobStatusError IS downloadStatusError'S STRUCTURAL TWIN
//
// Same signature, same `defer civitai.TagStatus`, same arms — and civitai/cli#572
// gated that one ONCE at the top rather than at each return, because #566 exists
// precisely because two spellings of one rule drifted apart. This gates its twin
// the same way. If the two ever diverge, that is the defect, not the style.
//
// # THE TRANSFORM IS safeTermSingle, NOT safeTerm
//
// `internal/saferune` keeps `\n` and `\t` by design, so `safeTerm` alone leaves
// the server able to forge a whole line — civitai/cli#577, measured one command
// over, where a file name forged a `(SHA256 verified)` line on a transfer that
// had not happened. Every string here is one line, so every one collapses.
//
// # AND BOTH HALVES OF EACH `%s: %w` PAIR
//
// #577's first pass gated the `%s` operands and left `safeTermErr` on `safeTerm`,
// so the WRAPPED CAUSE still forged the line: `*fs.PathError` renders its Path
// unquoted and that Path carries the hostile name. Fixed there by moving
// `safeTermErr` onto `safeTermSingle`; this path inherits that, and the two
// `%s: %w` sites here wrap their causes through it.

// gbfHostileName is the fixture: a plausible complete success line, so a
// survivor reads as the lie it is rather than as mangled text. It carries a
// newline AND a tab — the two runes saferune deliberately retains.
const gbfHostileName = "out-1.png\nSaved /home/u/real.png (2.0 MiB)\tDONE"

// gbfAssertOneLine is the shared assertion.
func gbfAssertOneLine(t *testing.T, surface, got string) {
	t.Helper()
	if n := strings.Count(got, "\n"); n != 0 {
		t.Errorf("#574 FORGERY in %s: %d forged line(s) — a server-derived name chose the "+
			"geometry of a message the user reads on the money-spending path:\n%s", surface, n, got)
	}
	if strings.Contains(got, "\t") {
		t.Errorf("#574 FORGERY in %s: a TAB survived; on any tabwriter surface it inserts a "+
			"column rather than misaligning one:\n%s", surface, got)
	}
}

// TestBlobStatusErrorCannotForgeALine drives all FOUR arms.
//
// Four, not three: 401/403, 404 and `default` are the error arms and the 2xx
// arm returns nil. The sibling test for `downloadStatusError` shipped covering
// three and an audit found the omitted one was `default` — precisely the arm
// that would ship raw if someone ever pushed the single top-of-function gate
// down into the arms, which is the drift this shape exists to prevent.
func TestBlobStatusErrorCannotForgeALine(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError, // the default arm
	} {
		err := blobStatusError(status, gbfHostileName)
		if err == nil {
			t.Fatalf("CONTROL failure, not a finding: status %d produced no error", status)
		}
		gbfAssertOneLine(t, "blobStatusError (HTTP "+http.StatusText(status)+")", err.Error())
		// POSITIVE CONTROL: the name must actually reach the message, or the
		// assertion above passes vacuously.
		if !strings.Contains(err.Error(), "out-1.png") {
			t.Fatalf("CONTROL failure, not a finding: the name never reached the HTTP %d message:\n%s",
				status, err.Error())
		}
	}
	// The gate must not have broken the classification it sits in front of.
	if err := blobStatusError(http.StatusOK, gbfHostileName); err != nil {
		t.Errorf("CONTROL failure, not a finding: a 2xx produced an error: %v", err)
	}
}

// TestDownloadBlobToErrorsCannotForgeALine drives the overwrite refusal, the
// two `%s: %w` pairs, and the `Saved` line.
func TestDownloadBlobToErrorsCannotForgeALine(t *testing.T) {
	t.Run("overwrite refusal", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatalf("CONTROL failure, not a finding: could not create the existing file: %v", err)
		}
		err := downloadBlobTo(context.Background(), nil, io.Discard, io.Discard,
			"https://example.invalid/blob", target, false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: an existing target produced no refusal")
		}
		if !strings.Contains(err.Error(), "refusing to overwrite") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		gbfAssertOneLine(t, "downloadBlobTo (refusing to overwrite)", err.Error())
	})

	t.Run("download %s: %w — both halves", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		// A fetcher that fails with a cause carrying the hostile bytes a second
		// time: gating only the %s operand leaves this half forging the line.
		fetch := func(context.Context, string) (*http.Response, error) {
			return nil, &os.PathError{Op: "open", Path: target, Err: errors.New("no such file or directory")}
		}
		err := downloadBlobTo(context.Background(), fetch, io.Discard, io.Discard,
			"https://example.invalid/blob", target, true)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: a failing fetcher produced no error")
		}
		if !strings.Contains(err.Error(), "download ") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		gbfAssertOneLine(t, "downloadBlobTo (download %s: %w)", err.Error())
		// The strip must not break the chain the exit-code classifier reads.
		var pe *os.PathError
		if !errors.As(err, &pe) {
			t.Errorf("safeTermErr broke the error chain: errors.As(*os.PathError) no longer matches, so "+
				"the published exit code can no longer see what kind of failure this is:\n  %#v", err)
		}
	})
}

// TestCreateOutputDirectoryStaysUngated is an INVARIANT guard, labelled as one:
// it passes before this change too. Its value is the mutant.
//
// 🔴 `dir` IS filepath.Dir(target) — THE USER'S OWN `--out-dir`, NEVER THE
// SERVER'S LEAF, because outputTarget refuses any name that is not its own
// filepath.Base. Sanitising it would strip USER-TYPED bytes, which
// internal/saferune's rule forbids (civitai/cli#393), and download.go carries
// the byte-identical line ungated for the same reason — civitai/cli#572's round
// 0 REMOVED a gate someone had added there, after it had already survived a
// 20-mutant sweep, because breakable is not reachable.
//
// So a reader now sees one gated twin and one ungated one and will try to make
// them agree. This is the test that tells them which direction is wrong.
func TestCreateOutputDirectoryStaysUngated(t *testing.T) {
	// A directory component the user typed, carrying a rune saferune strips.
	// The escape is required, not stylistic: staticcheck's ST1018 rejects a raw
	// U+200B in a literal, and it is the ONE linter that sees it — `make ci` does
	// not run lint, which is why AGENTS.md says to run both.
	const userDir = "my\u200bout dir"
	dir := filepath.Join(t.TempDir(), userDir)
	// Make MkdirAll fail by putting a FILE where the directory must go.
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %v", err)
	}
	target := filepath.Join(dir, "out-1.png")
	err := downloadBlobTo(context.Background(), nil, io.Discard, io.Discard,
		"https://example.invalid/blob", target, true)
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: creating a directory over a file succeeded")
	}
	if !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error, so this test does not "+
			"drive the line it names: %v", err)
	}
	// 🔴 COUNT, DO NOT `Contains` — AND THE FIRST DRAFT OF THIS ASSERTION WAS
	// VACUOUS FOR EXACTLY THE REASON THIS WHOLE ARC IS ABOUT. `Contains` was
	// satisfied by the WRAPPED CAUSE: the message is
	// `create output directory <dir>: mkdir <dir>: not a directory`, so the path
	// appears TWICE and `%w` renders its half raw whatever the `%s` half does.
	// Gating the operand left the cause carrying the fixture, `Contains` stayed
	// true, and the mutant SURVIVED — one value, printed twice, asserted on one
	// half. Measured, then fixed.
	//
	// The count is the discriminator: ungated renders it twice, gated once.
	if n := strings.Count(err.Error(), userDir); n != 2 {
		t.Errorf("civitai/cli#393 REGRESSION in downloadBlobTo (create output directory %%s): the "+
			"user's own --out-dir appears %d time(s) in the message, want 2 (the `%%s` operand and "+
			"the `%%w` cause). `dir` is filepath.Dir(target), which outputTarget guarantees holds no "+
			"server bytes — gating it strips what the USER typed, and someone copying that path "+
			"gets one that does not exist.\ngot: %s", n, err.Error())
	}
}
