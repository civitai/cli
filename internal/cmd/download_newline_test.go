package cmd

import (
	"bytes"
	"context"
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

// TestDownloadStatusErrorCannotForgeALine drives surface 4 — all three arms,
// because #572 gated this function ONCE at the top precisely so the arms cannot
// drift, and a test that drove one arm would not notice if that changed.
func TestDownloadStatusErrorCannotForgeALine(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		err := downloadStatusError(status, dlNewlineName)
		if err == nil {
			t.Fatalf("CONTROL failure, not a finding: status %d produced no error", status)
		}
		assertOneLine(t, "downloadStatusError (status "+http.StatusText(status)+")", err.Error())
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
