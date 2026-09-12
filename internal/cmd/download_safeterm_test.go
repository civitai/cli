package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
// COULD NOT DEMAND A ROW FOR ANY OF THEM. civitai/cli#572 FOUND A SIXTH,
// targetPath, WHICH LOOKED GATED BECAUSE IT USED `%q`.
//
// safeTermCoveredBy keys on the ENCLOSING FUNCTION and only fires its GREW
// check for a function that ALREADY calls safeTerm at least once. Every surface
// below called it ZERO times, so no row was ever demanded and their absence was
// not merely unrecorded — it was unrecordable. Fixing the calls is what makes
// them ledgerable; this file is what makes the rows honest.
//
// 🔴 WHAT EACH TEST HERE HAD TO EARN, BECAUSE A LEDGER ROW IS A MEASUREMENT AND
// NOT A LABEL: every gate below was watched to go RED with its wrapper deleted —
// the narrowest expression that can be wrong, spliced out on its own, never
// together with its enclosing statement — and GREEN with it restored.
//
// MEASURED AT THIS COMMIT, by an AST walk attributing every safeTerm /
// safeTermSingle / safeTermErr call to its enclosing function, cross-checked
// against a per-line grep that agreed exactly:
//
//	19 gated call sites in the SEVEN functions this file pins —
//	  targetPath 1, checkTargetCollisions 3, downloadOne 6, writePart 6,
//	  downloadStatusError 1, (*progressWriter).line 1, safeTermErr 1.
//	21 mutants: one deletion per call site, plus 2 nesting-order mutants for
//	  the dashIfEmpty/safeTerm swap. 18 KILLED, 3 SURVIVED, 0 build-broken,
//	  0 timeout panics; every run reported 1599 top-level results, so nothing
//	  was silently skipped.
//
// Every kill was required to name a FAILING ASSERTION OF ITS OWN — a mutant
// that merely fails to compile proves nothing — and the run was checked for
// build markers and a timeout panic rather than read off an exit code.
//
// The negative control (safeTerm neutered to the identity, spelled so it still
// COMPILES) reddened 37 top-level tests, 7 of this file's 8 — against 0 failures
// on the unmutated tree in the same harness, so the control was watched to move
// in BOTH directions. A SURVIVED verdict here is therefore a fact about the
// fixture and not a harness wired to nothing. The 8th,
// TestDownloadErrorsReachStderrUnfiltered, stays GREEN under it and is meant to:
// it reads main.go's SOURCE and asserts nothing about what safeTerm does.
//
// ⚠ THE NUMBERS ABOVE REPLACE "20 mutants over the 21 gated call sites",
// "reddened 34 tests including all four of this file's", "17 KILLED, 4 SURVIVED"
// and "reddened 36 top-level tests", and each correction is recorded rather than
// quietly applied. "21 sites" was never right: the same walk measures 20 at the
// commit where that sentence was written, and 18 after #572 removed the
// unreachable mkdir gate. 36 went to 37 when civitai/cli#573 landed
// TestGatedRenderersDoNotForgeOutsideTheirTable — a count in prose is a claim,
// and this one has now drifted stale, drifted wrong, AND been moved by someone
// else's merge.
//
// 🔴 THE THREE SURVIVORS ARE NAMED RATHER THAN ROUNDED AWAY, because a ledger row
// that reads as coverage while providing none is worse than no row. They are
// three of the four #566 named; the fourth is now KILLED:
//
//   - writePart's `finalize %s` and both `finalize`/`streaming` causes. Reaching
//     them needs a failing Close (a full disk or a pulled descriptor) or a
//     stream error whose own message carries hostile bytes. Neither is driven
//     here, and neither is claimed.
//
// ⚠ THE FOURTH SURVIVOR IS GONE, AND ITS RECORDED REASON WAS FALSE. safeTermErr
// on `download %s: %w` was written up as surviving "because the cause is a
// *url.Error and Go's own %q already escapes the class — defence in depth, not
// coverage". Measured in round 2 of #572, all three clauses are wrong: `%q` is
// strconv.Quote, which escapes what is not unicode.IsPrint and so passes U+2800
// (So) and U+034F (Mn) THROUGH; url.URL.String() writes RawQuery back verbatim,
// so a hostile downloadUrl query reaches *url.Error raw; and pkg/civitai's
// https/parse refusals on this path are not *url.Error at all. It survived for
// want of a driving test, not for redundancy — a maintainer following that
// sentence would have deleted a live gate against a green suite. The two
// subtests named for it below now kill it.
//
// Everything else — targetPath's refusal on both streams, all three collision
// fields, the progress line, and every error string on the downloadOne path —
// dies to a named assertion in this file.
//
// ⚠ WHAT NONE OF THIS CLOSES, stated because the gates would otherwise read
// wider than they are: saferune deliberately KEEPS `\n` and `\t`, so a newline
// in a server-supplied name still forges whole lines on these surfaces. Every
// one of these surfaces was fully RAW before #566, so what this file pins is a
// strict improvement on that, not a claim to have closed line forgery.
//
// That residual belongs to civitai/cli#577, NOT to #552. #552 was CLOSED by
// #573, whose measured scope was ~13 tabwriter renderers and which deliberately
// excluded free-text surfaces; download.go was never on its table. #577 is the
// split-out issue that owns the four download surfaces, and it leaves the
// transform open rather than prescribing safeTermSingle — these are free-text
// lines, not cells, so #573's cell rule does not apply to them unmodified.

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
// safeterm_invisible_test.go's TestSafeTerm_StripsTheInvisibleAndBidiClass —
// `a\x1b[2Kb` -> `a[2Kb`, `a<U+202E>b` -> `ab`, `a<U+200B>b` -> `ab` — not by
// running safeTerm over the fixture and writing down what came back. Deriving
// them that way would make every assertion here satisfied by a safeTerm that
// strips nothing.
//
// Written as ESCAPES, not as raw bytes: staticcheck's ST1018 rejects a literal
// carrying a Unicode format character, and AGENTS.md records that `make ci`
// cannot see that (lint is a separate job) — four fixtures once reached a push
// on exactly this rule.
const (
	dlHostileName = "alfa\x1b[1A\x1b[2Kbravo\u202echarlie\u200bdelta.safetensors"
	dlSafeName    = "alfa[1A[2Kbravocharliedelta.safetensors"

	dlHostileType = "Arch\u200bive\x1b]0;pwned\a"
	dlSafeType    = "Archive]0;pwned"

	// 🔴 A THIRD FIXTURE, FOR THE ONE SURFACE WHOSE ONLY GATE WAS `%q` —
	// civitai/cli#572. The two above carry ESC, BEL, ZWSP and RLO, and `%q`
	// escapes EVERY one of them; a test built from them cannot tell `safeTerm(x)`
	// inside a `%q` from a bare `%q`, because both render safely. So this fixture
	// carries the half `%q` passes THROUGH, raw: U+2800 BRAILLE PATTERN BLANK
	// (So, a blankButGraphic member) and U+034F COMBINING GRAPHEME JOINER (Mn,
	// the rune saferune's package doc names as the one the `Cf` first cut of #393
	// missed). It keeps one ESC as well, so the fixture still spans both halves.
	//
	// Derived from the class as pinned in safeterm_invisible_test.go, not by
	// running safeTerm over it: U+2800 and U+034F are removed outright, the ESC
	// goes and its `[2K` survives as ordinary ASCII.
	dlHostileQuoted = "pad\u2800\u034fcore\x1b[2K"
	dlSafeQuoted    = "padcore[2K"

	// \ud83d\udd34 A FOURTH FIXTURE, FOR THE HALF OF dlHostileQuoted THAT CAN RIDE IN A
	// URL \u2014 civitai/cli#572 round 2. net/url refuses ANY ASCII control byte
	// outright (`net/url: invalid control character in URL`), so the ESC above
	// cannot reach a parsed URL's query; U+2800 and U+034F can, and
	// url.URL.String() writes RawQuery back VERBATIM \u2014 it percent-encodes the
	// path and punycodes the host, but never re-encodes the query. So this is
	// the payload a server's files[].downloadUrl carries all the way into the
	// error text on the download path.
	dlHostileQuery = "pad\u2800\u034fcore"
	dlSafeQuery    = "padcore"
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
		// 🔴 THE TWO `%q` DOES NOT ESCAPE (civitai/cli#572). strconv quotes what
		// is not unicode.IsPrint, and IsPrint admits both of these, so a `%q`-only
		// surface emits them verbatim. Without these arms this scanner would
		// report a clean render for exactly the payload #572 is about.
		case 0x2800:
			out = append(out, "U+2800 BRAILLE PATTERN BLANK")
		case 0x034f:
			out = append(out, "U+034F COMBINING GRAPHEME JOINER")
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
		{"quoted", dlHostileQuoted, 3},
		{"query", dlHostileQuery, 2},
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
		{dlHostileQuoted, dlSafeQuoted}, {dlSafeQuoted, dlSafeName}, {dlSafeQuoted, dlSafeType},
	} {
		if pair[0] == pair[1] {
			t.Fatalf("CONTROL failure, not a finding: fixture values %q and %q are equal", pair[0], pair[1])
		}
	}

	// 🔴 THE CONTROL THAT MAKES dlHostileQuoted WORTH HAVING, and it is a claim
	// about the STANDARD LIBRARY, so it is measured here rather than assumed:
	// `%q` alone leaves this fixture's hazard runes on the terminal. Without this,
	// TestTargetPathRefusalSanitizesTheServerName could not distinguish the gate
	// it pins from the `%q` that was already there (civitai/cli#572).
	if got := dlHazardRunes(fmt.Sprintf("%q", dlHostileQuoted)); len(got) != 2 {
		t.Errorf("CONTROL failure, not a finding: %%q of the quoted fixture left %d hazard rune(s) %v, want 2 "+
			"(U+2800 and U+034F). If Go's strconv started escaping them, the #572 finding is closed upstream "+
			"and this fixture no longer discriminates — re-derive it before trusting the test that uses it.",
			len(got), got)
	}
	if got := dlHazardRunes(fmt.Sprintf("%q", safeTerm(dlHostileQuoted))); got != nil {
		t.Errorf("CONTROL failure, not a finding: safeTerm+%%q still left %v", got)
	}
}

// --- surface 1: checkTargetCollisions ------------------------------------------

// TestCheckTargetCollisionsSanitizesServerFields is civitai/cli#566 surface 2.
//
// The refusal lists every file that would be silently overwritten. Its per-file
// line is the LITERAL TWIN of formatFileList's — same three fields, same shape,
// in the same file — and that one was safeTerm'd while this one was not.
// formatFileList over the IDENTICAL fixture is therefore the clean positive
// control: it proves the expected strings are renderable, so a failure here is
// this renderer's and not the fixture's.
//
// (The twin is named, not located. Earlier revisions of this comment and of
// checkTargetCollisions' said "44 lines above"; it was ~55 by the time #572
// read it. A distance between two functions is a fact with no owner — a
// function name is one the compiler and every grep can still resolve.)
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
			"formatFileList sanitises these exact fields (civitai/cli#566).\n%s", got, msg)
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

// --- surface 1b: targetPath's unusable-filename refusal -------------------------

// TestTargetPathRefusalSanitizesTheServerName is civitai/cli#572.
//
// 🔴 THE ONE SURFACE ON THIS PATH WHOSE ONLY GATE WAS `%q`, AND `%q` IS NOT A
// SANITISER. targetPath called safeTerm ZERO times, so — exactly like #566's
// five — the ledger's GREW check could never demand a row for it. The refusal
// echoes the server's files[].name, and it is reachable with an ARBITRARY
// payload because `filepath.Base("<anything>/..")` is "..": the degenerate
// basename the guard tests for says nothing about the bytes in front of it.
//
// It reaches a terminal on BOTH streams, which is why both are driven below:
// stderr through downloadSelected (main.go prints err.Error() unfiltered), and
// STDOUT through printDownloadPlan's "target: (unresolved) — %v", one line under
// a file name that IS correctly sanitised.
func TestTargetPathRefusalSanitizesTheServerName(t *testing.T) {
	// Base(x) == ".." for this, so the guard fires carrying the whole prefix.
	hostile := dlHostileQuoted + "/.."
	// Derived from the class, not from running safeTerm: the invisible runes go,
	// the ESC goes and its "[2K" stays as ordinary ASCII. Everything left is
	// printable ASCII, so %q adds delimiters and no escapes.
	want := `server returned an unusable filename "` + dlSafeQuoted + `/.."; pass --out to set the output path`
	f := civitai.ModelVersionFile{ID: 7781, Name: hostile, Type: "Model", SizeKB: 12}

	t.Run("the refusal itself", func(t *testing.T) {
		_, _, err := targetPath(f, &downloadOpts{})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: a name whose basename is \"..\" was accepted, so the " +
				"refusal never rendered and every assertion here is vacuous")
		}
		if err.Error() != want {
			t.Errorf("SAFETERM REGRESSION in targetPath (unusable filename %%q): error was\n  %q\nwant\n  %q",
				err.Error(), want)
		}
		if got := dlHazardRunes(err.Error()); got != nil {
			t.Errorf("SAFETERM REGRESSION in targetPath: %v reached the terminal. `%%q` escapes the Cc/Cf half "+
				"of the class and passes the rest through RAW — that is the whole finding (civitai/cli#572)", got)
		}
	})

	// --out is the USER's own path and targetPath returns it verbatim, so the
	// refusal is unreachable there. Asserted, not assumed: it is what makes the
	// strip above unambiguously server-text only (saferune's package doc).
	t.Run("--out never reaches the refusal", func(t *testing.T) {
		got, _, err := targetPath(f, &downloadOpts{out: hostile})
		if err != nil {
			t.Fatalf("--out must be returned verbatim, got error %v", err)
		}
		if got != hostile {
			t.Errorf("--out was rewritten: got %q, want the user's own bytes %q", got, hostile)
		}
	})

	t.Run("on stderr via downloadSelected", func(t *testing.T) {
		var out, errb bytes.Buffer
		_, err := downloadSelected(context.Background(), dlFakeDownloader{}, &out, &errb,
			[]civitai.ModelVersionFile{f}, &downloadOpts{})
		if err == nil {
			t.Fatal("CONTROL failure, not a finding: downloadSelected did not surface the refusal")
		}
		if err.Error() != want {
			t.Errorf("SAFETERM REGRESSION via downloadSelected: got\n  %q\nwant\n  %q", err.Error(), want)
		}
	})

	t.Run("on stdout via printDownloadPlan", func(t *testing.T) {
		var out bytes.Buffer
		if err := printDownloadPlan(&out, []civitai.ModelVersionFile{f}, &downloadOpts{}, "https://civitai.com"); err != nil {
			t.Fatalf("CONTROL failure, not a finding: printDownloadPlan: %v", err)
		}
		plan := out.String()
		if !strings.Contains(plan, "  target: (unresolved) — "+want+"\n") {
			t.Errorf("SAFETERM REGRESSION in printDownloadPlan: the unresolved-target line does not contain\n"+
				"  %q\ngot:\n%s", want, plan)
		}
		// The point of driving this surface: the sanitised name is printed one
		// line ABOVE, so a raw refusal here is the sibling-renderer split #566 is
		// about, inside a single function.
		if !strings.Contains(plan, "\n"+dlSafeQuoted+"/..\n") {
			t.Errorf("CONTROL failure, not a finding: the plan's own file-name line did not render as expected, "+
				"so the comparison this subtest makes is not available:\n%s", plan)
		}
		if got := dlHazardRunes(plan); got != nil {
			t.Errorf("SAFETERM REGRESSION in printDownloadPlan: %v reached STDOUT through the unresolved-target "+
				"line (civitai/cli#572)", got)
		}
	})
}

// --- the dashIfEmpty/safeTerm nesting ------------------------------------------

// TestFileListPlaceholderSurvivesAnInvisibleType is civitai/cli#572.
//
// `safeTerm(dashIfEmpty(f.Type))` asks "is the RAW type empty" — a type of one
// zero-width rune answers no, so the placeholder is skipped and safeTerm then
// empties the field, rendering "(, 2.9 MiB)". Nesting the other way round asks
// the question of the string that will actually be PRINTED. reportBaseModel
// already had that order; formatFileList and checkTargetCollisions did not.
//
// Cosmetic and hostile-input-only, which is why it is pinned here rather than
// argued about: the assertion costs one line and the drift cannot recur silently.
func TestFileListPlaceholderSurvivesAnInvisibleType(t *testing.T) {
	// A type made ENTIRELY of runes in the class, so safeTerm empties it while
	// dashIfEmpty on the raw value would not.
	// Escapes, not raw bytes: staticcheck's ST1018 rejects a literal carrying a
	// Unicode format character, and `make ci` cannot see it (lint is a separate
	// job). U+2800 and U+034F are not Cf so ST1018 is blind to them; they are
	// written as escapes anyway, because a reviewer cannot see them either.
	const invisibleType = "\u2800\u034f\u200b"
	if safeTerm(invisibleType) != "" {
		t.Fatalf("CONTROL failure, not a finding: the fixture type does not strip to empty (%q), so neither "+
			"nesting order can differ and this test proves nothing", safeTerm(invisibleType))
	}
	if dashIfEmpty(invisibleType) == "-" {
		t.Fatal("CONTROL failure, not a finding: dashIfEmpty already replaces the RAW fixture, so the two " +
			"nesting orders agree and this test proves nothing")
	}

	files := []civitai.ModelVersionFile{
		{ID: 4472, Name: dlHostileName, Type: invisibleType, SizeKB: 3000},
		{ID: 9931, Name: dlHostileName, Type: "Model", SizeKB: 1500},
	}
	const wantRow = "[id 4472] " + dlSafeName + " (-, 2.9 MiB)"

	if got := formatFileList(files); !strings.Contains(got, wantRow) {
		t.Errorf("formatFileList rendered an EMPTY type field instead of the %q placeholder.\nwant a row "+
			"containing: %q\ngot:\n%s\n\ndashIfEmpty must wrap safeTerm, not the other way round "+
			"(civitai/cli#572).", "-", wantRow, got)
	}

	err := checkTargetCollisions(files, &downloadOpts{outDir: t.TempDir()})
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: two same-named files produced no collision refusal")
	}
	if !strings.Contains(err.Error(), wantRow) {
		t.Errorf("checkTargetCollisions rendered an EMPTY type field instead of the %q placeholder.\nwant a "+
			"row containing: %q\ngot:\n%s", "-", wantRow, err.Error())
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
	// ⚠ WHAT THIS ONE SUBTEST IS AND IS NOT EVIDENCE FOR. Its payload is
	// dlHostileName, every rune of which `%q` DOES escape, so it stays green with
	// the safeTermErr on this call deleted. It pins the message shape; the two
	// subtests after it are what pin the call. Read them together — this one
	// alone was once written up as proof that the call was redundant, and that
	// write-up was wrong (civitai/cli#572 round 2).
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

	// 🔴 THE `download %s: %w` GATE IS LIVE, AND THIS IS WHAT KILLS IT —
	// civitai/cli#572 round 2. Until this subtest existed the ledger claimed the
	// call SURVIVED "because the cause is a *url.Error and Go's own %q already
	// escapes the class". Measured here, that sentence is false on its own
	// terms: `%q` is strconv.Quote, which escapes what is not unicode.IsPrint,
	// and IsPrint ADMITS U+2800 (So) and U+034F (Mn) — both in saferune's class.
	//
	// The rune gets there because url.URL.String() percent-encodes the PATH and
	// punycodes the HOST but writes RawQuery back untouched, so a server-supplied
	// files[].downloadUrl with the payload in its query survives round-tripping
	// through net/url and lands raw inside *url.Error's quoted URL.
	t.Run("transport failure whose *url.Error carries the runes %q does not escape", func(t *testing.T) {
		// The premise, asserted rather than assumed: net/url really does hand the
		// query back verbatim. Without this the subtest could be green because the
		// fixture never reached the error, which is indistinguishable from a strip.
		parsed, perr := url.Parse("https://cdn.example.invalid/blob?sig=" + dlHostileQuery)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: url.Parse rejected the fixture: %v", perr)
		}
		hostileURL := parsed.String()
		if got := dlHazardRunes(hostileURL); len(got) != 2 {
			t.Fatalf("CONTROL failure, not a finding: url.URL.String() left %d hazard rune(s) %v in the "+
				"query, want 2. If net/url started encoding RawQuery this subtest proves nothing and the "+
				"ledger row for safeTermErr must be re-measured.", len(got), got)
		}
		// And that Go's own rendering of that URL is NOT safe, which is the exact
		// claim the old ledger row made in the opposite direction.
		raw := (&url.Error{Op: "Get", URL: hostileURL, Err: errors.New("EOF")}).Error()
		if dlHazardRunes(raw) == nil {
			t.Fatalf("CONTROL failure, not a finding: *url.Error rendered %q clean, so this subtest cannot "+
				"distinguish safeTermErr from the standard library", hostileURL)
		}

		var out, errb bytes.Buffer
		dl := dlFakeDownloader{err: &url.Error{Op: "Get", URL: hostileURL, Err: errors.New("EOF")}}
		_, err := downloadOne(context.Background(), dl, &out, &errb, newFile(""),
			filepath.Join(t.TempDir(), dlHostileName), &downloadOpts{})
		assertDownloadErr(t, err, "downloadOne (download %s: %w, *url.Error query)",
			`download `+dlSafeName+`: Get "https://cdn.example.invalid/blob?sig=`+dlSafeQuery+`": EOF`)
		var ue *url.Error
		if !errors.As(err, &ue) {
			t.Errorf("safeTermErr broke the error chain: errors.As(*url.Error) no longer matches, so the "+
				"exit-code classifier can no longer see what kind of failure this is:\n  %#v", err)
		}
	})

	// 🔴 AND THE CAUSE ON THIS PATH IS OFTEN NOT A *url.Error AT ALL. The real
	// pkg/civitai client refuses a non-https downloadUrl BEFORE any request is
	// built, and returns a plain fmt.Errorf that prints the whole raw URL through
	// `%q` — the same `%q` that does not escape the two runes above. This drives
	// the REAL Downloader (no fake, no network: the refusal happens before the
	// dial), so the wrapped cause is the one production produces.
	t.Run("real client's https refusal, whose cause is not a *url.Error", func(t *testing.T) {
		var out, errb bytes.Buffer
		f := newFile("")
		f.DownloadURL = "http://cdn.example.invalid/blob?sig=" + dlHostileQuery
		// The default client, i.e. AllowPrivateDownloadHosts=false — the shipped
		// configuration, not a test-only bypass.
		dl := civitai.New("https://civitai.com", "tok")
		_, err := downloadOne(context.Background(), dl, &out, &errb, f,
			filepath.Join(t.TempDir(), dlHostileName), &downloadOpts{})
		assertDownloadErr(t, err, "downloadOne (download %s: %w, https refusal)",
			`download `+dlSafeName+`: refusing to download over http — downloads must use https `+
				`(got "http://cdn.example.invalid/blob?sig=`+dlSafeQuery+`")`)
		// The cause is deliberately asserted NOT to be a *url.Error: that is the
		// half of the old ledger row this subtest refutes directly.
		var ue *url.Error
		if errors.As(err, &ue) {
			t.Errorf("the https refusal is now a *url.Error (%#v). That does not make the gate redundant — "+
				"see the sibling subtest — but the comment above is then describing the wrong mechanism.", ue)
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

	// There is deliberately NO `create output directory` subtest. The one that
	// stood here handed downloadOne a target of <tmp>/<hostile name>/weights.bin —
	// a shape targetPath cannot produce, since the server's name is always the
	// LEAF. It went red on a mutation and so read as coverage, while the gate it
	// pinned could never have run on a real path. See download.go's comment at the
	// MkdirAll (civitai/cli#572).

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
	// A NOTE, NOT A FAILURE. main.go growing a sanitiser would be a CORRECT
	// change, and a gate that goes red on a correct change is one people learn to
	// click through. The load-bearing claim is the loop above — that main.go
	// filters nothing TODAY, which is what makes the per-error gates in
	// download.go load-bearing rather than redundant. If this line ever prints,
	// reconcile the two claims; do not treat it as a regression (civitai/cli#572).
	if strings.Contains(string(src), "safeTerm") || strings.Contains(string(src), "saferune") {
		t.Log("main.go now references a sanitiser on the way out. Not a failure: re-read download.go's " +
			"per-error gates and decide whether they are still the load-bearing ones (civitai/cli#566).")
	}
}
