package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#577 — A NEWLINE IN A SERVER-SUPPLIED FILE NAME FORGES WHOLE LINES.
//
// `internal/saferune` deliberately RETAINS `\n` and `\t` ("Cc, minus \n and \t")
// so multi-line fields can format legitimately, so `safeTerm` alone does not
// stop a server-supplied `files[].name` from forging lines. Every surface on the
// download path is a SINGLE-LINE context — a `\r`-rewritten progress line, a row
// in a group listing, a one-line error — so every one of them takes
// `safeTermSingle`, which collapses `\n` and `\t` to a space.
//
// 🔴 THE FORGERY THIS CLOSES, MEASURED BEFORE THE FIX. With
// `files[].name = "weights.safetensors\nSaved …(SHA256 verified)"`, the progress
// line rendered:
//
//	  weights.safetensors
//	Saved /home/u/legit.safetensors (4.0 GiB)  (SHA256 verified)  1 B / 1000 B (0%)
//
// — a forged integrity claim, on its own line, BEFORE ANY BYTES HAD BEEN
// VERIFIED, and `done()` leaves it standing. The CLI asserting an integrity
// result that did not happen is the worst thing on this path: every other
// forgery misleads about a NAME, this one misleads about whether the bytes are
// the bytes.
//
// 🔴 NOT A REGRESSION, AND THAT IS WHY IT NEEDED ITS OWN ISSUE. All four
// surfaces were fully RAW before civitai/cli#572, which gated them with
// `safeTerm` and STATED this residual rather than hiding it. #552's closure by
// #573 covered ~13 tabwriter renderers and `download.go` was never on that
// table, so as of #552 closing, the residual had no tracking object at all.
//
// # WHY THE WHOLE PATH MOVED, NOT THE FOUR SURFACES THE ISSUE NAMED
//
// #566 exists because two spellings of one rule drifted apart, and #572 gated
// `downloadStatusError` once at the top for the same reason. Fixing the four
// reported sites and leaving their structural siblings raw is that mechanism
// again. Every `safeTerm(` in download.go is a single-line context, so all 32
// became `safeTermSingle(` — verified by the origin ledger, which scans BOTH
// spellings (its own comment records why) and reports the same 199 sites before
// and after, so no call site left its view.

// dlNewlineName is the fixture. It is deliberately NOT dlHostileName: that one
// carries invisible and bidi runes, which `safeTerm` already strips, so it
// cannot distinguish the newline transform from the strip that preceded it. The
// forged tail is a complete, plausible success line so a survivor is legible as
// the lie it is rather than as mangled text.
const dlNewlineName = "weights.safetensors\nSaved /home/u/legit.safetensors (4.0 GiB)  (SHA256 verified)"

// dlTabName pins the OTHER retained rune. A tab is never legitimate inside a
// single-line field, and on a tabwriter surface it inserts columns rather than
// merely misaligning them (civitai/cli#552).
const dlTabName = "weights.safetensors\tSaved\t(SHA256 verified)"

// assertOneLine is the shared assertion: the rendered text must occupy exactly
// one line, and must not still contain the forged claim as a separate line.
func assertOneLine(t *testing.T, surface, got string) {
	t.Helper()
	if n := strings.Count(got, "\n"); n != 0 {
		t.Errorf("#577 FORGERY in %s: rendered text holds %d newline(s), want 0 — a server-supplied "+
			"name forged %d extra terminal line(s):\n%s", surface, n, n, got)
	}
	if strings.Contains(got, "\t") {
		t.Errorf("#577 FORGERY in %s: rendered text holds a TAB, which on a tabwriter surface inserts "+
			"a column rather than misaligning one:\n%s", surface, got)
	}
}

// TestProgressLineCannotForgeALine drives surface 1 of civitai/cli#577.
//
// The progress line is rewritten with `\r` at 10 Hz on a TTY, so a newline in it
// does not merely add a line — it strands the forged text ABOVE the rewrite
// point, where nothing ever overwrites it.
//
// 🔴 THE NAME CLAIMS MORE THAN THE TEST, AND THE GAP IS STATED RATHER THAN
// RENAMED AWAY. What is pinned is that no LINE-BREAK RUNE survives — #577's
// whole scope. It is NOT true that this line cannot be forged: `p.name` is
// length-unbounded (nothing in pkg/civitai caps it), and a name padded to the
// terminal width SOFT-WRAPS, which produces the identical stranded
// `(SHA256 verified)` at column zero with no `\n` and no `\t` anywhere.
// Measured by the audit of this PR at widths 80/100/120/132: one logical line
// occupying 14 display rows, 12 of them beginning with the forged text.
// safeTermSingle is a no-op on it and assertOneLine is green.
//
// That is a DIFFERENT primitive (display width, not control runes) and out of
// #577's scope, but the machinery already exists — safeterm.go's
// hardSplitOverlong treats soft-wrap as the same forgery elsewhere. Bounding
// p.name is its own change; until then, do not read this test's name as the
// outcome.
func TestProgressLineCannotForgeALine(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"newline", dlNewlineName},
		{"tab", dlTabName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &progressWriter{name: tc.payload, total: 1000, written: 1}
			got := p.line()
			assertOneLine(t, "(*progressWriter).line", got)
			// POSITIVE CONTROL on the fixture: if the name never reached the
			// line at all, the assertion above would pass vacuously.
			if !strings.Contains(got, "weights.safetensors") {
				t.Fatalf("CONTROL failure, not a finding: the fixture name never reached the rendered "+
					"line, so this subtest proves nothing:\n%s", got)
			}
		})
	}
}

// TestCheckTargetCollisionsCannotForgeARow drives surface 2.
//
// The refusal is a multi-line block BY DESIGN — a group header plus one `- [id N]`
// row per file — so "the whole message is one line" is the wrong assertion here.
// What must hold is that the number of lines is a function of the FILE COUNT and
// nothing else, which is what a forged row breaks.
func TestCheckTargetCollisionsCannotForgeARow(t *testing.T) {
	// 🔴 THE BOUND IS DIFFERENTIAL, NOT ARITHMETIC, AND THE FIRST DRAFT PROVED
	// WHY. That draft hand-counted "header + group line + one row per file" = 4
	// and reported a FORGERY at 5 — the real message also carries a trailer, so
	// the test was wrong and the code was right. A hand-counted bound is a second
	// claim that can be wrong independently of the thing it checks. Rendering the
	// SAME shape twice and comparing the line counts cannot be: whatever the
	// message's real geometry is, a hostile name must not change it.
	render := func(t *testing.T, name string) []string {
		t.Helper()
		files := []civitai.ModelVersionFile{
			{ID: 1, Name: name, Type: "Model", SizeKB: 10},
			{ID: 2, Name: name, Type: "Model", SizeKB: 10},
		}
		err := checkTargetCollisions(files, &downloadOpts{out: filepath.Join(t.TempDir(), "same.bin")})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: two files at one target produced no collision error")
		}
		return strings.Split(strings.TrimRight(err.Error(), "\n"), "\n")
	}

	benign := render(t, "weights.safetensors")
	// POSITIVE CONTROL on the harness: the benign refusal must be multi-line, or
	// "the counts match" is a comparison between two trivial values.
	if len(benign) < 4 {
		t.Fatalf("CONTROL failure, not a finding: the benign refusal rendered %d line(s); this test "+
			"compares geometries and needs a real one:\n%s", len(benign), strings.Join(benign, "\n"))
	}

	for _, tc := range []struct{ name, payload string }{
		{"newline", dlNewlineName},
		{"tab", dlTabName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := render(t, tc.payload)
			// 🔴 THE LINE-COUNT BOUND CANNOT SEE A TAB, AND AN AUDIT MEASURED
			// THIS SUBTEST PASSING ON FULLY UNFIXED CODE. A tab adds no line, so
			// len(got) != len(benign) never fires for it, and the forged-claim
			// loop skips a row containing "[id ". The tab needs its own
			// assertion or `dlTabName` here is decoration.
			for _, l := range got {
				if strings.Contains(l, "\t") {
					t.Errorf("#577 FORGERY in checkTargetCollisions: a TAB survived into a row — on a "+
						"tabwriter surface it inserts a column rather than misaligning one:\n%s",
						strings.Join(got, "\n"))
				}
			}
			if len(got) != len(benign) {
				t.Errorf("#577 FORGERY in checkTargetCollisions: a hostile name rendered %d line(s) "+
					"where a benign one renders %d — the server chose the geometry:\n%s",
					len(got), len(benign), strings.Join(got, "\n"))
			}
			for _, l := range got {
				if strings.Contains(l, "(SHA256 verified)") && !strings.Contains(l, "[id ") {
					t.Errorf("#577 FORGERY in checkTargetCollisions: a forged claim escaped its "+
						"row:\n%s", strings.Join(got, "\n"))
				}
			}
		})
	}
}

// TestDownloadStatusErrorCannotForgeALine drives surface 4 — all four arms,
// because #572 gated this function ONCE at the top precisely so the arms cannot
// drift, and a test that drove one arm would not notice if that changed.
func TestDownloadStatusErrorCannotForgeALine(t *testing.T) {
	// 🔴 FOUR ARMS, NOT THREE. An earlier draft of this test and its comment both
	// said "all three arms", and the function has four — the default arm
	// (`download of %s failed (HTTP %d)`) was untested. If someone later pushes
	// the gate down into the arms, which is exactly the #566 drift this guard
	// names as its reason to exist, that arm would break with the suite green.
	for _, status := range []int{
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusInternalServerError, // the default arm
	} {
		for _, payload := range []string{dlNewlineName, dlTabName} {
			err := downloadStatusError(status, payload)
			if err == nil {
				t.Fatalf("CONTROL failure, not a finding: status %d produced no error", status)
			}
			assertOneLine(t, "downloadStatusError (status "+http.StatusText(status)+")", err.Error())
		}
	}
	// The gate must not have broken the classification it sits in front of.
	if err := downloadStatusError(http.StatusOK, dlNewlineName); err != nil {
		t.Errorf("CONTROL failure, not a finding: a 2xx produced an error: %v", err)
	}
}

// TestDownloadOneMismatchCannotForgeALine drives surface 3 — the SHA256-mismatch
// message, i.e. the CLI asserting an integrity FAILURE. A forged line here is
// the inverse of the progress-line forgery: the user is told the bytes are
// wrong, on one line, while another line tells them a different file was saved
// and verified.
func TestDownloadOneMismatchCannotForgeALine(t *testing.T) {
	const body = "SEVENTEEN-BYTES!!"
	const wrongSHA = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
	f := civitai.ModelVersionFile{
		ID: 5517, Name: dlNewlineName, Type: "Model", SizeKB: 1,
		DownloadURL: "https://example.invalid/blob",
		Hashes:      civitai.FileHashes{SHA256: wrongSHA},
	}
	var out, errb bytes.Buffer
	dl := dlFakeDownloader{body: body, code: http.StatusOK}
	_, err := downloadOne(context.Background(), dl, &out, &errb, f,
		filepath.Join(t.TempDir(), "out.bin"), &downloadOpts{})
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a body whose hash does not match produced no error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error than the mismatch path, so "+
			"this test drives the wrong surface: %v", err)
	}
	assertOneLine(t, "downloadOne (SHA256 mismatch)", err.Error())
}

// TestWrappedCauseCannotForgeALine is the guard my own F1 fix did not have.
//
// 🔴 THE `%s` HALF AND THE `%w` HALF ARE TWO SURFACES, AND A FIRST PASS AT #577
// CLOSED ONLY ONE. Every `%s: %w` pair on this path sanitises the operand and
// wraps the cause with safeTermErr — which used safeTerm, so it kept `\n`.
// *fs.PathError and *os.LinkError render their path UNQUOTED, and that path
// carries filepath.Base(f.Name), so the CAUSE forged the line the operand could
// not. Measured before the fix:
//
//	create /…/weights.safetensors Saved … (SHA256 verified).part: open /…/weights.safetensors
//	Saved /home/u/legit.safetensors (4.0 GiB)  (SHA256 verified).part: no such file or directory
//
// That is #566's own shape — one value, printed twice, sanitised on one half —
// and three comments claimed the residual was closed while it was open. Reverting
// safeTermErr to safeTerm reddened NOTHING until this test existed.
func TestWrappedCauseCannotForgeALine(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{
		{"newline", dlNewlineName},
		{"tab", dlTabName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A parent directory that does not exist makes os.Create fail with
			// an *fs.PathError whose Path carries the hostile name.
			_, _, err := writePart(strings.NewReader(""),
				filepath.Join("/nonexistent-dir-for-577", tc.payload+".part"),
				io.Discard, tc.payload, 0, false)
			if err == nil {
				t.Fatal("CONTROL failure, not a finding: creating under a missing directory succeeded")
			}
			// POSITIVE CONTROL: the hostile bytes must be in the CAUSE, not merely
			// somewhere in the rendered message.
			//
			// 🔴 THIS USED TO READ err.Error(), WHICH COULD NOT FAIL. The message is
			// "create <sanitised partPath>: <cause>", and the %s operand supplies
			// "weights.safetensors" on its own — so the control stayed green even
			// when the cause carried no name at all, which is the one condition its
			// own sentence claims to rule out. Measured: replacing the wrapped cause
			// with a name-free fs.ErrNotExist left it green, and compounding that
			// with the safeTermErr revert this test exists to kill left it green too.
			// Unwrap, so the assertion reads the half that matters.
			cause := errors.Unwrap(err)
			if cause == nil {
				// 🔴 THIS IS A FINDING, NOT A CONTROL FAILURE, and it is worth the
				// extra sentence: "CONTROL failure, not a finding" is this repo's
				// idiom for "the harness broke, disregard it", and this branch is
				// the ONLY signal in the suite that catches a dropped %w. Labelling
				// it as harness noise would tell the one reader who sees it to look
				// away. safeterm.go states that the wrap is what keeps the
				// exit-code classifier (AGENTS.md items 7 and 24) able to see the
				// sentinel, so losing it is a real regression.
				t.Fatalf("REGRESSION: writePart's error wraps nothing, so the exit-code classifier can no "+
					"longer see the cause's sentinel (AGENTS.md items 7 and 24). The likeliest cause is a `%%w` "+
					"that became `%%s`. Error was:\n%s", err.Error())
			}
			if !strings.Contains(cause.Error(), "weights.safetensors") {
				t.Fatalf("CONTROL failure, not a finding: the name never reached the CAUSE:\n%s", cause.Error())
			}
			assertOneLine(t, "writePart (create %s: %w — the WRAPPED CAUSE)", err.Error())
		})
	}
}
