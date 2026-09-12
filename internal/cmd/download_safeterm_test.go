package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// civitai/cli#566 — THE DOWNLOAD PATH HAD FIVE SURFACES THAT PRINTED
// UPLOADER-CONTROLLED TEXT WITHOUT safeTerm, AND #564's LEDGER STRUCTURALLY
// COULD NOT DEMAND A ROW FOR ANY OF THEM.
//
// safeTermCoveredBy keys on the ENCLOSING FUNCTION and only fires its GREW
// check for a function that ALREADY calls safeTerm at least once. Every surface
// below called it ZERO times, so no row was ever demanded and their absence was
// not merely unrecorded — it was unrecordable. Fixing the calls is what makes
// them ledgerable; this file is what makes the rows honest.
//
// 🔴 WHAT EACH TEST HERE HAD TO EARN, BECAUSE A LEDGER ROW IS A MEASUREMENT AND
// NOT A LABEL: every assertion below was watched to go RED with the safeTerm
// wrapper at its site deleted — the narrowest expression that can be wrong,
// spliced out on its own, never together with its enclosing statement — and
// GREEN with it restored. The matrix is in the PR body for #566.

// --- the fixture --------------------------------------------------------------
//
// 🔴 THE THREE PRIMITIVES ARE DIFFERENT ON PURPOSE, AND SO ARE THE FIELD VALUES.
//
// A fixture carrying one hazard rune cannot tell a full strip from a partial
// one, and fields that share a value cannot tell a mutant that hardcodes one
// field from a mutant that renders the right one. So each fixture string
// carries a DIFFERENT half of the class, and no two rendered fields below are
// equal to each other or to any constant an assertion names:
//
//   - \x1b is the forgery primitive itself: ESC is what makes "[1A[2K" a cursor
//     move and a line clear rather than five printable characters. safeTerm
//     removes ONLY the ESC (it is Cc); the bracket text is ordinary ASCII and
//     SURVIVES, which is exactly why the expected strings below still contain
//     it. A `want` that dropped the brackets too would be asserting a strip the
//     CLI does not perform.
//   - U+202E REORDERS: everything after it displays right-to-left, so what the
//     user reads is not what the bytes say. No control byte involved.
//   - U+200B is INVISIBLE: it is not unicode.IsSpace, so it pads a name without
//     occupying a cell.
//
// The expected values are derived from the class as PINNED IN
// safeterm_invisible_test.go's TestSafeTerm_StripsTheInvisibleAndBidiClass
// ("a\x1b[2Kb" -> "a[2Kb", "a‮b" -> "ab", "a​b" -> "ab"), not by
// running safeTerm over the fixture and writing down what came back. Deriving
// them that way would make every assertion here satisfied by a safeTerm that
// strips nothing.
const (
	dlHostileName = "alfa\x1b[1A\x1b[2Kbravo‮charlie​delta.safetensors"
	dlSafeName    = "alfa[1A[2Kbravocharliedelta.safetensors"

	dlHostileType = "Arch​ive\x1b]0;pwned\a"
	dlSafeType    = "Archive]0;pwned"
)

// dlHazardRunes reports the hazard runes still present in s, as an INDEPENDENT
// check that does not consult safeTerm or saferune about what they are. Two of
// this repo's own rules meet here: a guard that asks the implementation what it
// strips is satisfied by an implementation that strips nothing, and a guard on
// a WORD is walkable by rewording — so the assertions below pair this
// structural check with a whole-string equality.
func dlHazardRunes(s string) []string {
	var out []string
	for _, r := range s {
		switch r {
		case 0x1b:
			out = append(out, "U+001B ESC")
		case 0x07:
			out = append(out, "U+0007 BEL")
		case 0x200b:
			out = append(out, "U+200B ZWSP")
		case 0x202e:
			out = append(out, "U+202E RLO")
		}
	}
	return out
}

// TestDownloadFixtureIsHostile is the POSITIVE CONTROL on every assertion in
// this file. dlHazardRunes returning nothing is indistinguishable from a clean
// render, so a suite that never watches it return something is a suite whose
// zeros mean nothing.
func TestDownloadFixtureIsHostile(t *testing.T) {
	for _, tc := range []struct {
		what string
		in   string
		want int
	}{
		{"name", dlHostileName, 4},
		{"type", dlHostileType, 3},
	} {
		if got := dlHazardRunes(tc.in); len(got) != tc.want {
			t.Errorf("CONTROL failure, not a finding: the %s fixture carries %d hazard rune(s) %v, want %d. "+
				"Every 'no hazard rune reached the terminal' assertion in this file is vacuous unless this "+
				"fixture actually carries them.", tc.what, len(got), got, tc.want)
		}
		if dlHazardRunes(safeTerm(tc.in)) != nil {
			t.Errorf("CONTROL failure, not a finding: safeTerm itself left a hazard rune in the %s fixture; "+
				"no surface test below can be read as evidence about that surface.", tc.what)
		}
	}
	// The expected strings must not accidentally equal each other or the raw
	// input: an assertion comparing a value to itself passes under any mutant.
	for _, pair := range [][2]string{
		{dlHostileName, dlSafeName}, {dlHostileType, dlSafeType},
		{dlSafeName, dlSafeType}, {dlHostileName, dlHostileType},
	} {
		if pair[0] == pair[1] {
			t.Fatalf("CONTROL failure, not a finding: fixture values %q and %q are equal", pair[0], pair[1])
		}
	}
}

// --- surface 1: checkTargetCollisions ------------------------------------------

// TestCheckTargetCollisionsSanitizesServerFields is civitai/cli#566 surface 2.
//
// The refusal lists every file that would be silently overwritten. Its per-file
// line is the LITERAL TWIN of formatFileList's — same three fields, same shape,
// 44 lines above in the same file — and that one was safeTerm'd while this one
// was not. formatFileList over the IDENTICAL fixture is therefore the clean
// positive control: it proves the expected strings are renderable, so a failure
// here is this renderer's and not the fixture's.
//
// 🔴 THE GROUP LINE'S `target` IS SANITISED TOO, AND IT IS NOT AN EXTRA. It is
// the mixed-origin path targetPath's own doc comment says its print sites must
// sanitise, and without --out its bytes are filepath.Base(SERVER file name).
func TestCheckTargetCollisionsSanitizesServerFields(t *testing.T) {
	dir := t.TempDir()
	// Both files share a name, so both route to one target — that is what makes
	// this a collision. Everything else is pairwise distinct so no mutant can
	// render the wrong field and still match.
	files := []civitai.ModelVersionFile{
		{ID: 4472, Name: dlHostileName, Type: dlHostileType, SizeKB: 3000},
		{ID: 9931, Name: dlHostileName, Type: "Model", SizeKB: 1500},
	}
	o := &downloadOpts{outDir: dir}

	err := checkTargetCollisions(files, o)
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: two same-named files produced no collision refusal, " +
			"so nothing below rendered and every assertion is vacuous")
	}
	msg := err.Error()

	// The whole normalised line, not a keyword: a guard on a word is walkable by
	// rewording, and a `!strings.Contains(raw)` check passes for a renderer that
	// prints nothing at all.
	for _, want := range []string{
		"  " + filepath.Join(dir, dlSafeName) + "  ← 2 files:\n",
		"      - [id 4472] " + dlSafeName + " (" + dlSafeType + ", 2.9 MiB)\n",
		"      - [id 9931] " + dlSafeName + " (Model, 1.5 MiB)\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("SAFETERM REGRESSION in checkTargetCollisions: the refusal does not contain\n  %q\ngot:\n%s", want, msg)
		}
	}
	if got := dlHazardRunes(msg); got != nil {
		t.Errorf("SAFETERM REGRESSION in checkTargetCollisions: %v reached the terminal through the "+
			"same-target refusal — the ONE thing standing between the user and a silent overwrite. "+
			"formatFileList sanitises these exact fields 44 lines above (civitai/cli#566).\n%s", got, msg)
	}

	// The clean positive control on the identical fixture.
	ctl := formatFileList(files)
	if !strings.Contains(ctl, "  - [id 4472] "+dlSafeName+" ("+dlSafeType+", 2.9 MiB)") {
		t.Errorf("CONTROL failure, not a finding: formatFileList — the already-gated twin — does not render "+
			"the expected strings for this fixture, so the assertions above are testing the fixture, not the "+
			"renderer:\n%s", ctl)
	}
	if got := dlHazardRunes(ctl); got != nil {
		t.Errorf("CONTROL failure, not a finding: formatFileList leaked %v, so it is no longer a clean "+
			"control for checkTargetCollisions", got)
	}
}

// --- surface 2: (*progressWriter).line -----------------------------------------

// TestProgressWriterLineSanitizesServerName is civitai/cli#566 surface 1.
//
// p.name is the server's files[].name: downloadOne passes f.Name to writePart,
// which passes it to newProgressWriter. Both of line's callers REWRITE the line
// in place — Write's TTY branch prints "\r"+line() ten times a second and
// done() prints "\r"+line()+"\n" — so the CLI is already telling the terminal to
// move its cursor, and an ESC in the file name extends that reach upward into
// the pickle/archive EXECUTION WARNING emitPreDownloadNotes printed just above.
//
// Both branches are driven: a known total (the percentage line) and an unknown
// one (the byte-count line). They were two separate interpolations of p.name
// before #566, which is the drift shape the fix collapses into one strip.
func TestProgressWriterLineSanitizesServerName(t *testing.T) {
	for _, tc := range []struct {
		branch  string
		total   int64
		written int
		want    string
	}{
		{"known total", 2048, 512, "  " + dlSafeName + "  512 B / 2.0 KiB (25%)"},
		{"unknown total", 0, 300, "  " + dlSafeName + "  300 B"},
	} {
		t.Run(tc.branch, func(t *testing.T) {
			var sink bytes.Buffer
			pw := newProgressWriter(&sink, dlHostileName, tc.total)
			n, err := pw.Write(make([]byte, tc.written))
			if err != nil || n != tc.written {
				t.Fatalf("CONTROL failure, not a finding: Write(%d) = (%d, %v)", tc.written, n, err)
			}

			if got := pw.line(); got != tc.want {
				t.Errorf("SAFETERM REGRESSION in (*progressWriter).line: rendered\n  %q\nwant\n  %q", got, tc.want)
			}
			if got := dlHazardRunes(pw.line()); got != nil {
				t.Errorf("SAFETERM REGRESSION in (*progressWriter).line: %v reached the \\r-rewritten "+
					"progress line, where a cursor escape is worth most (civitai/cli#566)", got)
			}

			// …and it actually reaches the writer. sink is not a TTY, so Write
			// took the plain-line branch; asserting on line() alone would not
			// prove the string is ever emitted.
			if got := sink.String(); got != tc.want+"\n" {
				t.Errorf("SAFETERM REGRESSION in (*progressWriter).line: the writer received\n  %q\nwant\n  %q", got, tc.want+"\n")
			}
		})
	}
}

// --- surface 3: downloadOne and writePart --------------------------------------

// dlFakeDownloader is the civitai.Downloader seam, so downloadOne's error paths
// can be DRIVEN rather than read. #566 flagged this surface's evidence as "read,
// not driven"; this is what closes that gap.
type dlFakeDownloader struct {
	err  error  // returned instead of a response
	body string // streamed when err == nil
	code int
}

func (d dlFakeDownloader) DownloadFile(_ context.Context, _ string) (*http.Response, error) {
	if d.err != nil {
		return nil, d.err
	}
	return &http.Response{
		StatusCode:    d.code,
		Body:          io.NopCloser(strings.NewReader(d.body)),
		ContentLength: int64(len(d.body)),
		Header:        http.Header{},
	}, nil
}

// dlErrReader fails mid-stream, which is the only way into writePart's
// "streaming %s" return.
type dlErrReader struct{}

func (dlErrReader) Read([]byte) (int, error) { return 0, errors.New("unexpected EOF mid-stream") }

// TestDownloadOneErrorsSanitizeTheServerName is civitai/cli#566 surface 3.
//
// 🔴 AN ERROR STRING IS A TERMINAL SURFACE. cmd/civitai/main.go prints
// err.Error() to stderr with no filter of its own, so every %s in these returns
// reaches a terminal exactly like a rendered table cell does — with a worse
// payload than a table cell carries: the SHA256-mismatch string is the CLI
// asserting an INTEGRITY FAILURE, and the 401 string is the CLI instructing the
// user to run `civitai login`.
func TestDownloadOneErrorsSanitizeTheServerName(t *testing.T) {
	const body = "SEVENTEEN-BYTES!!"
	// A wrong hash that is not a run of one character, so a mutant that prints a
	// zeroed or truncated hash cannot match by accident.
	const wrongSHA = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"

	newFile := func(sha string) civitai.ModelVersionFile {
		return civitai.ModelVersionFile{
			ID: 5517, Name: dlHostileName, Type: dlHostileType, SizeKB: 900,
			DownloadURL: "https://example.invalid/blob",
			Hashes:      civitai.FileHashes{SHA256: sha},
		}
	}

	t.Run("transport failure", func(t *testing.T) {
		var out, errb bytes.Buffer
		dl := dlFakeDownloader{err: errors.New("dial tcp 203.0.113.7:443: connect: connection refused")}
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(""),
			filepath.Join(t.TempDir(), dlHostileName), &downloadOpts{})
		assertDownloadErr(t, err, "downloadOne (download %s)",
			"download "+dlSafeName+": dial tcp 203.0.113.7:443: connect: connection refused")
	})

	// 🔴 THE WRAPPED CAUSE IS A SECOND COPY OF THE HOSTILE BYTES, AND #566's
	// FIRST FIX — safeTerm ON THE %s ALONE — PUT THEM STRAIGHT BACK. The real
	// cause on this path is a *url.Error whose URL is the server's own
	// files[].downloadUrl, so the message read "download <name-clean>: Get
	// <url-raw>". This subtest pins the OUTCOME (nothing hostile on stderr, and
	// the chain still classifiable), not one mechanism.
	//
	// ⚠ STATED BECAUSE IT CHANGES WHAT THIS SUBTEST IS EVIDENCE FOR: on a
	// *url.Error the neutralising is done by the STANDARD LIBRARY, which renders
	// the URL with %q and so escapes every rune in the class as ASCII text.
	// safeTermErr is defence in depth here and this subtest does NOT go red if
	// it is deleted from this one call. The subtests that DO pin safeTermErr are
	// "output directory" below and writePart's "create" — *fs.PathError and
	// *os.LinkError render their paths RAW, with no quoting anywhere.
	t.Run("transport failure whose cause quotes the server URL", func(t *testing.T) {
		var out, errb bytes.Buffer
		hostileURL := "https://cdn.example.invalid/" + dlHostileName
		dl := dlFakeDownloader{err: &url.Error{Op: "Get", URL: hostileURL, Err: errors.New("EOF")}}
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(""),
			filepath.Join(t.TempDir(), dlHostileName), &downloadOpts{})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: a failing Downloader produced no error")
		}
		if !strings.HasPrefix(err.Error(), "download "+dlSafeName+": Get ") {
			t.Errorf("SAFETERM REGRESSION in downloadOne (download %%s): got\n  %q", err.Error())
		}
		if got := dlHazardRunes(err.Error()); got != nil {
			t.Errorf("SAFETERM REGRESSION in downloadOne (download %%s: %%w): %v reached stderr through "+
				"the WRAPPED CAUSE — the server's own downloadUrl (civitai/cli#566)", got)
		}
		// The classification must survive the strip: a sanitised cause that broke
		// errors.As would silently repoint the failure at the generic exit code
		// (AGENTS.md items 7 and 24).
		var ue *url.Error
		if !errors.As(err, &ue) {
			t.Errorf("safeTermErr broke the error chain: errors.As(*url.Error) no longer matches, so the "+
				"exit-code classifier can no longer see what kind of failure this is:\n  %#v", err)
		}
	})

	t.Run("install", func(t *testing.T) {
		// A non-empty DIRECTORY at target, so the final os.Rename fails and the
		// cause is an *os.LinkError — which renders BOTH paths raw, with no
		// quoting. presentTargetSatisfies already returns false for a directory,
		// so the download runs to completion and the failure is the install.
		root := t.TempDir()
		target := filepath.Join(root, dlHostileName)
		if err := os.MkdirAll(filepath.Join(target, "occupied"), 0o755); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		var out, errb bytes.Buffer
		dl := dlFakeDownloader{body: body, code: http.StatusOK}
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(sha256hex(body)), target, &downloadOpts{})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: rename onto a non-empty directory succeeded")
		}
		if !strings.HasPrefix(err.Error(), "install "+filepath.Join(root, dlSafeName)+": rename ") {
			t.Errorf("SAFETERM REGRESSION in downloadOne (install %%s): got\n  %q", err.Error())
		}
		if got := dlHazardRunes(err.Error()); got != nil {
			t.Errorf("SAFETERM REGRESSION in downloadOne (install %%s: %%w): %v reached stderr. "+
				"*os.LinkError renders BOTH of its paths raw, so sanitising the %%s alone is decorative "+
				"(civitai/cli#566)", got)
		}
	})

	t.Run("SHA256 mismatch", func(t *testing.T) {
		var out, errb bytes.Buffer
		dl := dlFakeDownloader{body: body, code: http.StatusOK}
		target := filepath.Join(t.TempDir(), dlHostileName)
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(wrongSHA), target, &downloadOpts{})
		assertDownloadErr(t, err, "downloadOne (SHA256 mismatch for %s)",
			"SHA256 mismatch for "+dlSafeName+" — expected "+wrongSHA+", got "+sha256hex(body)+
				" (deleted the partial download)")
		if _, statErr := os.Stat(target + ".part"); !os.IsNotExist(statErr) {
			t.Errorf("the corrupt partial was not deleted: stat %q -> %v", target+".part", statErr)
		}
	})

	t.Run("HTTP status", func(t *testing.T) {
		var out, errb bytes.Buffer
		dl := dlFakeDownloader{body: "", code: http.StatusUnauthorized}
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(""),
			filepath.Join(t.TempDir(), dlHostileName), &downloadOpts{})
		assertDownloadErr(t, err, "downloadStatusError (401)",
			"downloading "+dlSafeName+" requires authentication (401) — run `civitai login` "+
				"(or set CIVITAI_TOKEN); this file needs a token (most model files do; some public files don't)")
	})

	t.Run("output directory", func(t *testing.T) {
		// A regular FILE where downloadOne wants a directory, so MkdirAll fails
		// with ENOTDIR and the hostile bytes land in "create output directory %s".
		root := t.TempDir()
		blocker := filepath.Join(root, dlHostileName)
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		var out, errb bytes.Buffer
		_, err := downloadOne(context.Background(), dlFakeDownloader{}, &out, &errb, newFile(""),
			filepath.Join(blocker, "weights.bin"), &downloadOpts{})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: MkdirAll over a regular file succeeded")
		}
		if !strings.HasPrefix(err.Error(), "create output directory "+filepath.Join(root, dlSafeName)+":") {
			t.Errorf("SAFETERM REGRESSION in downloadOne (create output directory %%s): got\n  %q", err.Error())
		}
		if got := dlHazardRunes(err.Error()); got != nil {
			t.Errorf("SAFETERM REGRESSION in downloadOne (create output directory %%s): %v reached stderr", got)
		}
	})

	t.Run("saved line", func(t *testing.T) {
		// The success path, so the already-gated `Saved <target>` line is covered
		// by this test too — the ledger row for downloadOne claims the function,
		// not one of its returns.
		var out, errb bytes.Buffer
		dl := dlFakeDownloader{body: body, code: http.StatusOK}
		root := t.TempDir()
		skipped, err := downloadOne(context.Background(), dl, &out, &errb, newFile(sha256hex(body)),
			filepath.Join(root, dlHostileName), &downloadOpts{})
		if err != nil || skipped {
			t.Fatalf("CONTROL failure, not a finding: downloadOne = (%v, %v)", skipped, err)
		}
		want := "Saved " + filepath.Join(root, dlSafeName) + " (17 B)  (SHA256 verified)\n"
		if got := out.String(); got != want {
			t.Errorf("SAFETERM REGRESSION in downloadOne (Saved %%s): stdout was\n  %q\nwant\n  %q", got, want)
		}
		if got := dlHazardRunes(out.String() + errb.String()); got != nil {
			t.Errorf("SAFETERM REGRESSION on the download success path: %v reached the terminal beside "+
				"the `SHA256 verified` claim the line exists to make", got)
		}
	})
}

// TestWritePartErrorsSanitizeTheServerName covers the frame BETWEEN downloadOne
// and the progress writer. Leaving these raw while its caller's are gated would
// rebuild #566's own defect shape — one renderer sanitising a field while its
// sibling does not — one call frame down.
func TestWritePartErrorsSanitizeTheServerName(t *testing.T) {
	t.Run("streaming", func(t *testing.T) {
		var errb bytes.Buffer
		_, _, err := writePart(dlErrReader{}, filepath.Join(t.TempDir(), "plain.part"), &errb,
			dlHostileName, 0, false)
		assertDownloadErr(t, err, "writePart (streaming %s)",
			"streaming "+dlSafeName+": unexpected EOF mid-stream")
	})

	t.Run("create", func(t *testing.T) {
		root := t.TempDir()
		blocker := filepath.Join(root, "blocked")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("CONTROL failure, not a finding: %v", err)
		}
		partPath := filepath.Join(blocker, dlHostileName+".part")
		var errb bytes.Buffer
		_, _, err := writePart(strings.NewReader("unused"), partPath, &errb, "n/a", 0, false)
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: os.Create under a regular file succeeded")
		}
		if !strings.HasPrefix(err.Error(), "create "+filepath.Join(blocker, dlSafeName+".part")+":") {
			t.Errorf("SAFETERM REGRESSION in writePart (create %%s): got\n  %q", err.Error())
		}
		if got := dlHazardRunes(err.Error()); got != nil {
			t.Errorf("SAFETERM REGRESSION in writePart (create %%s): %v reached stderr", got)
		}
	})
}

// assertDownloadErr pins the WHOLE error string and, separately, that no hazard
// rune survived. Two assertions rather than one because they fail for different
// reasons: the equality catches a wrong field or a reworded message, the rune
// scan catches a strip that only handles part of the class.
func assertDownloadErr(t *testing.T, err error, site, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("CONTROL failure, not a finding: %s returned no error, so nothing was rendered", site)
	}
	if err.Error() != want {
		t.Errorf("SAFETERM REGRESSION in %s: error was\n  %q\nwant\n  %q", site, err.Error(), want)
	}
	if got := dlHazardRunes(err.Error()); got != nil {
		t.Errorf("SAFETERM REGRESSION in %s: %v reached stderr. cmd/civitai/main.go prints err.Error() "+
			"unfiltered (civitai/cli#566)", site, got)
	}
}

// TestDownloadErrorsReachStderrUnfiltered is the premise the four tests above
// rest on, asserted rather than assumed: nothing between a RunE return and the
// terminal strips anything, so the gate has to be at the fmt.Errorf.
//
// It reads main.go rather than exec'ing the binary because the claim is about
// the SOURCE — a live probe would be evidence about whatever binary happened to
// be on disk.
func TestDownloadErrorsReachStderrUnfiltered(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "cmd", "civitai", "main.go"))
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read main.go: %v", err)
	}
	// Both halves of the claim: the error's own bytes are printed, and the only
	// thing added is a constant prefix.
	for _, want := range []string{
		`fmt.Fprintln(os.Stderr, errorLine(err))`,
		`return "Error: " + err.Error()`,
	} {
		if !strings.Contains(string(src), want) {
			t.Fatalf("main.go no longer renders a returned error as %s. If the print site moved, re-check "+
				"whether it now filters: download.go's error strings are gated at the fmt.Errorf precisely "+
				"because nothing downstream was (civitai/cli#566).", want)
		}
	}
	if strings.Contains(string(src), "safeTerm") || strings.Contains(string(src), "saferune") {
		t.Errorf("main.go now sanitises on the way out. That is not wrong, but the per-error gates in " +
			"download.go were justified by its absence — reconcile the two rather than leaving both " +
			"claims standing (civitai/cli#566).")
	}
}
