package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
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

// 🔴 THE ASSERTION IS assertOneLine, NOT A SECOND COPY OF IT. A first draft of
// this file defined gbfAssertOneLine — byte-identical to
// download_newline_test.go's assertOneLine but for the issue number in its
// message, in the SAME package. Two spellings of one rule is the mechanism
// civitai/cli#566 exists about, and writing a second one inside a PR applying
// that lesson is how it recurs. Found by this PR's round-0 audit.

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
		assertOneLine(t, "blobStatusError (HTTP "+http.StatusText(status)+")", err.Error())
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

// TestDownloadBlobToErrorsCannotForgeALine drives all four surfaces — the
// overwrite refusal, both `%s: %w` pairs and the `Saved` line — each as its own
// subtest, each mutation-verified alone.
//
// 🔴 IT CLAIMED ALL FOUR AND DROVE TWO. An audit measured it: ungating `install
// %s`'s operand alone SURVIVED the whole suite, and ungating the `Saved` line
// entirely SURVIVED it too, while this doc and the safeTermCoveredBy row both
// listed them. `Saved` had been honestly ledgered notCovered BEFORE this PR, so
// the row made its coverage look better than the tree's. Both are driven now.
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
		assertOneLine(t, "downloadBlobTo (refusing to overwrite)", err.Error())
	})

	// 🔴 THESE TWO SUBTESTS EXIST BECAUSE THE DOC ABOVE CLAIMED THEM AND THE
	// TEST DID NOT DRIVE THEM. Measured by this PR's nine-axis audit: ungating
	// `install %s`'s operand alone SURVIVED the whole suite, and ungating the
	// `Saved` line entirely SURVIVED it too — while this file's header and the
	// safeTermCoveredBy row both listed them as covered. A row that reads as
	// coverage while providing none is the class this repo treats as worse than
	// no row, because it stops the next reader looking.
	t.Run("install %s: %w — both halves", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		// os.Rename fails when the destination is a DIRECTORY, and the error is
		// an *os.LinkError carrying both paths — so the cause repeats the
		// hostile bytes the operand already holds.
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		fetch := func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader("payload")),
				ContentLength: 7,
			}, nil
		}
		err := downloadBlobTo(context.Background(), fetch, io.Discard, io.Discard,
			"https://example.invalid/blob", target, true)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: renaming onto a directory succeeded")
		}
		if !strings.Contains(err.Error(), "install ") {
			t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
		}
		assertOneLine(t, "downloadBlobTo (install %s: %w)", err.Error())
	})

	t.Run("the Saved line", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, gbfHostileName)
		fetch := func(context.Context, string) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader("payload")),
				ContentLength: 7,
			}, nil
		}
		var out bytes.Buffer
		if err := downloadBlobTo(context.Background(), fetch, &out, io.Discard,
			"https://example.invalid/blob", target, true); err != nil {
			t.Fatalf("CONTROL failure, not a finding: the transfer failed: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "Saved ") {
			t.Fatalf("CONTROL failure, not a finding: no Saved line was written:\n%s", got)
		}
		// The success line is ONE line. A forged newline here is the worst of
		// the set: it is the line the user reads as "this finished".
		if n := strings.Count(strings.TrimRight(got, "\n"), "\n"); n != 0 {
			t.Errorf("#574 FORGERY in downloadBlobTo (Saved): %d forged line(s) on the success "+
				"line:\n%s", n, got)
		}
		if strings.Contains(got, "\t") {
			t.Errorf("#574 FORGERY in downloadBlobTo (Saved): a TAB survived:\n%s", got)
		}
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
		assertOneLine(t, "downloadBlobTo (download %s: %w)", err.Error())
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

// TestDownloadOutputsRefusalCannotForgeALine drives the refusal users ACTUALLY
// reach — the one #574's six-site table missed.
//
// 🔴 IT WAS FULLY RAW, NOT MERELY UNGATED FOR `\n`. `j.target` is
// planOutputTarget's output, whose `{workflowId}` comes off the wire, and it was
// joined into the message with no sanitiser at all — so this is the #566 class
// (raw ANSI) rather than the #577 one. Measured before the fix: `esc=true`, two
// forged newlines and a tab, with `\x1b[1A\x1b[2K` — cursor-up plus erase-line —
// intact.
//
// 🔴 AND IT IS THE REACHABLE BRANCH. downloadOutputs pre-checks every job here
// before any bytes move, so downloadBlobTo's own `!force` refusal (which #574
// DID name) fires only in a TOCTOU window. The requirement's table was built by
// reading two functions; it hardened the branch users almost never hit and left
// this one raw. Found by this PR's round-0 audit, which is the round that asks
// whether the requirement itself is right.
func TestDownloadOutputsRefusalCannotForgeALine(t *testing.T) {
	// 🔴 DRIVE THE REAL FUNCTION. A first draft of this test rebuilt the message
	// inline with safeTermSingle(target) hardcoded — so it asserted on the
	// HELPER, not on the production line, and ungating that line left it GREEN.
	// Caught by its own mutation check. A guard that reconstructs the thing it
	// is guarding is testing its own arithmetic.
	dir := t.TempDir()
	// A cursor-control payload, not merely a newline: this surface had NO gate
	// at all, so a `\n`-only fixture could not distinguish "gated for lines"
	// from "gated".
	hostileID := "wf1\x1b[1A\x1b[2K\nSaved out.png (2.0 MiB)\tDONE"
	url := "https://example.invalid/blob.png"
	outs := []genapi.Output{{Blob: genapi.Blob{URL: &url}}}

	// Pre-create what planOutputTarget will choose, so the refusal fires.
	target, err := planOutputTarget("", dir, hostileID, 1, url)
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: planOutputTarget refused the fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %v", err)
	}
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("CONTROL failure, not a finding: %v", err)
	}
	// POSITIVE CONTROL on the fixture: the hostile id must actually reach the
	// target, or the refusal below cannot carry it.
	if !strings.Contains(target, "wf1") {
		t.Fatalf("CONTROL failure, not a finding: planOutputTarget dropped the id; target=%q", target)
	}

	_, err = downloadOutputs(context.Background(), nil, io.Discard, io.Discard,
		hostileID, outs, dir, "", false)
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: an existing target produced no refusal")
	}
	got := err.Error()
	if !strings.Contains(got, "refusing to overwrite") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error: %v", err)
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("#566 FORGERY in downloadOutputs (refusing to overwrite): a raw ESC survived into "+
			"the one error standing between the user and an overwrite:\n%q", got)
	}
	// Header + one line per clash. A forged newline breaks that relationship.
	if n := strings.Count(strings.TrimRight(got, "\n"), "\n"); n != 2 {
		t.Errorf("#574 FORGERY in downloadOutputs (refusing to overwrite): %d newline(s) for ONE "+
			"clash, want 2 (header, the row, the trailer) — the server chose the geometry:\n%s", n, got)
	}
	if strings.Contains(got, "\t") {
		t.Errorf("#574 FORGERY in downloadOutputs (refusing to overwrite): a TAB survived:\n%s", got)
	}
}
