package civitai

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
)

// This file is the follow-up to civitai/cli#526's audit. #526 added the
// raw-control-byte repair; these tests cover the three things it left unpinned
// and the one hole its shape opened.
//
// Every test here is RED at 517fc76 (the #526 squash, i.e. origin/main before
// this branch) and green at HEAD. The matrix is in the PR body.
//
// 🔴 The fixture strings below are pairwise distinct and distinct from every
// constant asserted on, so a mutant that hardcodes one literal cannot satisfy
// another assertion.

// escapedCR is the TWO-CHARACTER sequence a repaired body carries where the wire
// carried one 0x0D byte. It is what the snippet must NOT contain: seeing it
// means the error message is quoting the CLI's repair rather than the server.
const escapedCR = `\r`

// serveOnce returns a client pointed at a server that answers every request with
// status + body.
func serveOnce(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "")
}

// TestGetIntoErrorSnippetQuotesTheWireBytesNotTheRepairedOnes is finding 2(b).
//
// README's Troubleshooting row promises "the text after the colon is the
// server's own body, truncated". #526's pre-pass reassigned the body to the
// REPAIRED bytes before the snippet was taken, so a server that sent one CR was
// reported as having sent a backslash and an `r` — the CLI quoting itself. The
// fixture decodes to neither shape: `id` is a string where the SDK wants an int,
// so the repair succeeds at the JSON level and the typed decode still fails,
// which is the only way to reach this message with a repairable body.
func TestGetIntoErrorSnippetQuotesTheWireBytesNotTheRepairedOnes(t *testing.T) {
	c := serveOnce(t, http.StatusOK,
		"{\"items\":[{\"id\":\"kestrel-97\",\"name\":\"Marbled\rHeron\"}],\"metadata\":{}}")

	_, err := c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("a body whose id is a string must still fail the typed decode; " +
			"without the failure this test asserts on nothing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected response from /api/v1/models (status 200)") {
		t.Fatalf("wrong error reached the assertion below: %q", msg)
	}
	if strings.Contains(msg, escapedCR) {
		t.Errorf("the snippet carries the CLI's REPAIR (%s), not the server's bytes — "+
			"README's \"the server's own body\" is false again:\n%q", escapedCR, msg)
	}
	// Positive control: the body really did reach the snippet. Without this a
	// snippet that dropped the body entirely would satisfy the check above.
	if !strings.Contains(msg, "MarbledHeron") {
		t.Errorf("the server's own text is missing from the snippet — "+
			"expected the CR-joined name with the CR removed by the strip:\n%q", msg)
	}
}

// TestGetIntoUnrepairableBodyReportsTheOriginalBytes is finding 4: the
// retry-fails path. Nothing exercised a body the sanitizer cannot repair
// through getInto, and a mutant dropping the guard on the repaired bytes
// survived a full green pkg/civitai + internal/cmd run.
//
// 🔴 IT IS AN INVARIANT GUARD, NOT REGRESSION COVERAGE, AND IT IS LABELLED AS
// ONE. It PASSES at 517fc76 — the pre-pass shape returned the same error for
// the same body — so it pins no bug that was ever live. Its whole value is
// mutation: it goes red, with the message above, on a decodeBody that assigns
// the sanitized bytes without checking they decoded.
//
// The fixture is a trailing comma: valid-looking, unrepairable by an in-string
// control-byte rewriter, and not the shape any other test here uses.
func TestGetIntoUnrepairableBodyReportsTheOriginalBytes(t *testing.T) {
	const body = `{"items":[{"id":41,"name":"Sorrel Kite",}],"metadata":{}}`
	c := serveOnce(t, http.StatusOK, body)

	res, err := c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatalf("an unrepairable body must be an error, got %+v", res)
	}
	msg := err.Error()
	if !strings.Contains(msg, "unexpected response from /api/v1/models (status 200)") {
		t.Errorf("the unrepairable body must reach the documented message:\n%q", msg)
	}
	if !strings.Contains(msg, "Sorrel Kite") {
		t.Errorf("the snippet must quote the body that failed:\n%q", msg)
	}
	if res != nil {
		t.Errorf("no result may be returned alongside the error: %+v", res)
	}
}

// TestReadErrorSnippetStripsTerminalControlRunes is the (a) half of the
// stderr-hole decision: snippet() now routes the server's bytes through
// internal/saferune.
//
// 🔴 THE PATH THIS COVERS IS THE ERROR PATH, WHICH HAS NO OTHER GATE.
// internal/cmd's safeTerm sits on the HUMAN RENDERERS; cmd/civitai/main.go
// prints `Error: <err>` to stderr with no filter at all. Before this, a 404
// body could carry a cursor-up + line-erase and overwrite the CLI's own
// preceding output from inside an error message.
//
// The escape arrives JSON-ESCAPED on the wire — which is what a real server
// emits — so this is not a fixture that only a malformed body could produce.
func TestReadErrorSnippetStripsTerminalControlRunes(t *testing.T) {
	// ESC[2K is ERASE LINE; U+200E is an invisible bidi mark. Both are written
	// as escapes in this source: a literal control byte in a Go string literal
	// is exactly what staticcheck's ST1018 exists to catch.
	c := serveOnce(t, http.StatusNotFound,
		"{\"error\":\"no such widget \\u001b[2K\\u200eSHA256 verified\"}")

	_, _, err := c.GetModel(context.Background(), "9")
	if err == nil {
		t.Fatal("a 404 must be an error; without it this test asserts on nothing")
	}
	msg := err.Error()
	// Positive control FIRST: the server's text must be present, or the two
	// absence checks below are satisfied by an empty snippet.
	if !strings.Contains(msg, "no such widget") || !strings.Contains(msg, "SHA256 verified") {
		t.Fatalf("the server's own words are missing — the checks below would be vacuous:\n%q", msg)
	}
	for _, bad := range []struct {
		r    rune
		name string
	}{
		{'\x1b', "ESC (0x1B) — an ANSI/OSC introducer"},
		{'\u200e', "U+200E LEFT-TO-RIGHT MARK"},
	} {
		if strings.ContainsRune(msg, bad.r) {
			t.Errorf("%s reached the error string, which main.go prints to stderr unfiltered:\n%q",
				bad.name, msg)
		}
	}
}

// TestPostIntoRepairsControlBytesLikeGetInto is finding 6, behaviourally: the
// batch by-hash POST claims in its doc comment to mirror getInto, and the
// control-byte repair was a fourth axis on which it did not.
//
// It is unobservable in production today (HashMatch is two ints and a hex
// string), so this is the guard that makes the CLAIM checkable rather than a
// fix for a reported bug.
func TestPostIntoRepairsControlBytesLikeGetInto(t *testing.T) {
	const hash = "6B1E0F2A9C7D45538E0A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A raw 0x0B (vertical tab) inside the echoed hash string: invalid JSON
		// on the wire, exactly the #525 class.
		_, _ = w.Write([]byte("[{\"modelVersionId\":8123,\"modelId\":557,\"hash\":\"\v" + hash + "\"}]"))
	}))
	t.Cleanup(srv.Close)

	got, err := New(srv.URL, "").GetModelVersionsByHashes(context.Background(), []string{hash})
	if err != nil {
		t.Fatalf("postInto did not repair a raw control byte the way getInto does: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 match, got %d (%+v)", len(got), got)
	}
	if got[0].ModelVersionID != 8123 || got[0].ModelID != 557 {
		t.Errorf("decoded the wrong entry: %+v", got[0])
	}
	if got[0].Hash != "\v"+hash {
		t.Errorf("the repair must not change the DOCUMENT, only its encoding: hash = %q", got[0].Hash)
	}
}

// decodeBodyCallers is the LEDGER of functions allowed to reach the shared 2xx
// decode. It fails in both directions: a third caller (the set grew — decide
// whether it should mirror) and a named one that no longer calls it (the set
// shrank — postInto's "it mirrors getInto" comment has gone stale again).
var decodeBodyCallers = []string{"getInto", "postInto"}

// TestDecodeBodyIsSharedByGetIntoAndPostInto is the structural half of finding
// 6. The behavioural test above can only see the repair; this sees the SEAM —
// that the two transports answer the question with one piece of code rather
// than two that agree today.
func TestDecodeBodyIsSharedByGetIntoAndPostInto(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("CONTROL failure, not a finding: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files, calls := 0, 0
	seen := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("CONTROL failure, not a finding: cannot parse %s: %v", name, perr)
		}
		files++
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "decodeBody" {
					calls++
					seen[fd.Name.Name] = true
				}
				return true
			})
		}
	}
	// Positive controls: a parser that found nothing must fail, not report a
	// reassuring zero.
	if files < 5 {
		t.Fatalf("CONTROL failure, not a finding: parsed only %d source files in pkg/civitai", files)
	}
	if calls < len(decodeBodyCallers) {
		t.Fatalf("found %d decodeBody call sites, expected at least %d — "+
			"either the scan is broken or a transport stopped sharing the decode",
			calls, len(decodeBodyCallers))
	}
	var got []string
	for name := range seen {
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), decodeBodyCallers...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("decodeBody's callers are %v, ledgered as %v.\n"+
			"GREW: a new transport now shares the 2xx decode — say so in postInto's\n"+
			"      mirror comment and add it here.\n"+
			"SHRANK: a transport went back to its own json.Unmarshal, so the repair,\n"+
			"      the failure message and the snippet can now disagree between them.",
			got, want)
	}
}
