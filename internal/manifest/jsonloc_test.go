package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsonloc_test.go pins the location a malformed manifest reports.
//
// 🔴 THE ROW THAT MATTERS IS THE BYTE-VS-RUNE PAIR. `encoding/json` reports a
// BYTE offset; a column is a CHARACTER position. The two agree on every ASCII
// manifest, so an implementation that confuses them passes every ordinary
// fixture and mis-reports the moment an author writes an accented display name,
// an em dash in a tagline, or an emoji.
//
// The discriminator is a PAIR of fixtures identical in characters and different
// in bytes: `asciiPrefix` and `widePrefix` below have the same rune count and
// differ by 6 bytes. They must report the SAME line and column.
//
//   - counting the column in BYTES makes the columns differ;
//   - treating the offset as a RUNE index makes the sliced position differ,
//     which moves the LINE, not just the column, on the multi-line rows.
//
// A single fixture cannot see either bug. Two identical-looking fixtures can.

// asciiPrefix and widePrefix are the same NUMBER OF CHARACTERS and a different
// NUMBER OF BYTES. Both are 8 runes.
//
//	asciiPrefix: 8 runes,  8 bytes
//	widePrefix:  8 runes, 14 bytes  (é=2, —=3, 🚀=4)
const (
	asciiPrefix = "aaaaaaaa"
	widePrefix  = "Café — 🚀"
)

type locFixture struct {
	name string
	body string
	// wantLine / wantCol are LITERAL expected values, counted by hand from the
	// fixture, never derived from the implementation.
	wantLine, wantCol int
	// why records what the row is for.
	why string
}

func locFixtures(t *testing.T) []locFixture {
	t.Helper()
	// Premise for the pair: same runes, different bytes. Asserted rather than
	// assumed — a fixture that lost its multi-byte characters would silently
	// turn the discriminating row into a second ASCII row.
	if got := len([]rune(asciiPrefix)); got != len([]rune(widePrefix)) {
		t.Fatalf("PREMISE BROKEN: prefixes differ in RUNES (%d vs %d) — the pair "+
			"discriminates nothing", got, len([]rune(widePrefix)))
	}
	if len(asciiPrefix) == len(widePrefix) {
		t.Fatalf("PREMISE BROKEN: prefixes are the same length in BYTES (%d) — a "+
			"byte-counted column would agree with a rune-counted one", len(asciiPrefix))
	}

	return []locFixture{
		{
			name: "trailing comma, line 4",
			// {\n  "blockId": "ok-app",\n  "name": "x",\n}\n
			body:     "{\n  \"blockId\": \"ok-app\",\n  \"name\": \"x\",\n}\n",
			wantLine: 4, wantCol: 1,
			why: "the offending `}` is the first character of the fourth line",
		},
		{
			name: "ASCII prefix on the failing line (CONTROL)",
			// `{"name":"aaaaaaaa","blockId":}` — the `}` is the 30th character.
			body:     `{"name":"` + asciiPrefix + `","blockId":}`,
			wantLine: 1, wantCol: 30,
			why: "the baseline the multi-byte row is compared against; all-ASCII, " +
				"so bytes and characters agree and this row cannot see the bug",
		},
		{
			name: "multi-byte prefix on the failing line",
			// Character-for-character the row above; 6 bytes longer. Column 30
			// again — that identity IS the assertion.
			body:     `{"name":"` + widePrefix + `","blockId":}`,
			wantLine: 1, wantCol: 30,
			why: "🔴 THE DISCRIMINATOR: identical to the row above in CHARACTERS and " +
				"6 bytes longer, so it must report the SAME column",
		},
		{
			name: "multi-byte on an EARLIER line",
			body: "{\n  \"name\": \"" + widePrefix + "\",\n  \"blockId\":\n}\n",
			// line 4 is "}"; the value is missing so the scanner chokes on it.
			wantLine: 4, wantCol: 1,
			why: "a rune-indexed slice would land on the wrong LINE here, not merely " +
				"the wrong column",
		},
		{
			name:     "unexpected end of input",
			body:     `{"blockId":`,
			wantLine: 1, wantCol: 11,
			why: "offset == len(raw) is a real shape and must not index out of range",
		},
		{
			name: "wrong TYPE, not a syntax error",
			// Parses fine; blockId is a number where Manifest wants a string.
			// json.UnmarshalTypeError carries an offset too.
			// Line 2 is `  "blockId": 123`; the offset lands on the last digit
			// of the offending value, character 16.
			body:     "{\n  \"blockId\": 123\n}\n",
			wantLine: 2, wantCol: 16,
			why: "LoadRaw decodes twice, so a type error is reachable and also has a " +
				"position worth printing",
		},
	}
}

// TestJSONErrorLocation drives the REAL LoadRaw against files on disk — not the
// helper in isolation — so the wiring is part of what is pinned. A helper that
// is correct and unreferenced would pass an isolated test and change nothing an
// author sees.
func TestJSONErrorLocation(t *testing.T) {
	var pairCols []int
	for _, tc := range locFixtures(t) {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, Filename), []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			_, _, err := LoadRaw(dir)
			if err == nil {
				t.Fatal("PREMISE BROKEN: the fixture parsed cleanly, so this row tests nothing")
			}
			msg := err.Error()
			if !strings.Contains(msg, Filename+" is not valid JSON at ") {
				t.Fatalf("message does not carry a location:\n%s", msg)
			}
			want := fmt.Sprintf("line %d, column %d", tc.wantLine, tc.wantCol)
			if !strings.Contains(msg, want) {
				t.Errorf("wrong location (%s)\n  want: %s\n  got:  %s", tc.why, want, msg)
			}
			// The original decoder text must survive — the location is added,
			// not substituted. Dropping "invalid character" would trade one
			// missing half of the diagnosis for the other.
			if !strings.Contains(msg, "invalid character") &&
				!strings.Contains(msg, "unexpected end of JSON input") &&
				!strings.Contains(msg, "cannot unmarshal") {
				t.Errorf("the decoder's own reason was lost:\n%s", msg)
			}
		})
		if strings.HasSuffix(tc.name, "(CONTROL)") || tc.name == "multi-byte prefix on the failing line" {
			pairCols = append(pairCols, tc.wantCol)
		}
	}
	// 🔴 The pair's whole point, asserted as its own claim rather than left
	// implicit in two identical literals: if a future edit changes one row's
	// expected column and not the other, the pair stops discriminating and this
	// fails instead of quietly passing.
	if len(pairCols) != 2 {
		t.Fatalf("expected exactly 2 rows in the byte-vs-rune pair, found %d", len(pairCols))
	}
	if pairCols[0] != pairCols[1] {
		t.Fatalf("the byte-vs-rune pair now EXPECTS different columns (%d vs %d). The pair "+
			"only discriminates a byte-counted column from a character-counted one while "+
			"both rows expect the SAME column; one of the two literals has been edited",
			pairCols[0], pairCols[1])
	}
}

// TestValidManifestStillParses is the CONTROL for the whole file: the location
// machinery must not turn a good manifest into a failure. Without it, "every
// malformed fixture reports a location" is also satisfied by a loader that
// rejects everything.
func TestValidManifestStillParses(t *testing.T) {
	dir := t.TempDir()
	body := `{"blockId":"ok-app","name":"` + widePrefix + `","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	generic, m, err := LoadRaw(dir)
	if err != nil {
		t.Fatalf("a valid manifest must still load: %v", err)
	}
	if generic == nil || m == nil {
		t.Fatal("LoadRaw returned no document")
	}
	if m.Name != widePrefix {
		t.Errorf("multi-byte name round-tripped wrong: %q", m.Name)
	}
	if _, err := Load(dir); err != nil {
		t.Errorf("Load: %v", err)
	}
}

// TestInvalidJSONFailsSoftWithoutAnOffset pins the degradation contract: an
// error carrying no position produces exactly the message the CLI printed
// before, rather than a fabricated `line 1, column 1`.
func TestInvalidJSONFailsSoftWithoutAnOffset(t *testing.T) {
	got := invalidJSON([]byte(`{}`), errors.New("some decoder failure with no position")).Error()
	if strings.Contains(got, "line ") || strings.Contains(got, "column ") {
		t.Errorf("a positionless error must not gain a location: %s", got)
	}
	if !strings.Contains(got, Filename+" is not valid JSON: ") {
		t.Errorf("the pre-existing message shape was not preserved: %s", got)
	}
	// Positive control: an error that DOES carry an offset must gain one, or
	// the assertion above is satisfied by a function that never adds a location.
	var syn *json.SyntaxError
	if err := json.Unmarshal([]byte(`{,}`), new(map[string]any)); !errors.As(err, &syn) {
		t.Fatalf("PREMISE BROKEN: wanted a *json.SyntaxError, got %T", err)
	}
	if withLoc := invalidJSON([]byte(`{,}`), syn).Error(); !strings.Contains(withLoc, "line 1, column 2") {
		t.Errorf("an offset-bearing error gained no location: %s", withLoc)
	}
}

// TestLineColumnEdges covers the clamps directly. These shapes are real
// (`unexpected end of JSON input` reports len(raw)) and an out-of-range slice
// would panic inside a validation path rather than report a finding.
func TestLineColumnEdges(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		offset    int64
		line, col int
	}{
		{"zero offset clamps to the start", "{}", 0, 1, 1},
		{"offset past the end clamps to the end", "{}", 99, 1, 3},
		{"first byte", "{}", 1, 1, 1},
		{"after a newline", "{\n}", 3, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, col := lineColumn([]byte(tc.raw), tc.offset)
			if line != tc.line || col != tc.col {
				t.Errorf("lineColumn(%q, %d) = %d,%d want %d,%d",
					tc.raw, tc.offset, line, col, tc.line, tc.col)
			}
		})
	}
}
