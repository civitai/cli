package civitai

import (
	"context"
	"errors"
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
// raw-control-byte repair; these tests cover the things it left unpinned, the
// hole its shape opened, and — added in round 3 — the exit code the fix for
// that hole moved.
//
// 🔴 THE MATRIX IS HERE, AND IT IS NOT UNIFORM. An earlier revision of this
// header said "every test here is RED at 517fc76 and green at HEAD" and
// deferred the matrix to the PR body. Both halves were wrong: one of the five
// tests below is labelled in its own doc comment as an INVARIANT GUARD that
// PASSES at 517fc76 — so the header contradicted an accurate label 100 lines
// under it — and a PR body is not in the tree, so a reader of the file could
// not check either claim. A test you have not watched fail proves nothing, and
// a header that says you watched them all fail when you did not is the same
// error one level up.
//
// Measured by running this file against a `git archive 517fc76` tree, not
// reasoned about:
//
//	TestGetIntoErrorSnippetQuotesTheWireBytesNotTheRepairedOnes   RED at 517fc76
//	TestReadErrorSnippetStripsTerminalControlRunes                RED at 517fc76
//	TestPostIntoRepairsControlBytesLikeGetInto                    RED at 517fc76
//	TestDecodeBodyIsSharedByGetIntoAndPostInto                    RED at 517fc76
//	TestGetIntoUnrepairableBodyReportsTheOriginalBytes            INVARIANT GUARD —
//	                                                              PASSES at 517fc76;
//	                                                              its value is the mutant
//	TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne  see below
//
// The last one is a round-3 regression against THIS BRANCH, not against
// 517fc76, and its row does not compress into the column above. Its subject is
// the exit code that f936cfc — this branch's round-2 head — moved by classifying
// a 429 on snippet()'s STRIPPED output: RED there, on the two classification
// assertions. At 517fc76 it is ALSO red, but for the opposite reason and with
// different assertions: that tree has no strip at all, so the classification
// half passes and the two assertions that the DISPLAY is still filtered fail.
// "Red at both ends, for opposite reasons" is the honest row; "red at 517fc76"
// alone would read as regression coverage for a bug 517fc76 did not have.
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

// softHyphenIn429 is a 429 body from a THROTTLE, carrying one U+00AD SOFT
// HYPHEN inside the word "many". U+00AD is Cf, therefore
// Default_Ignorable_Code_Point, therefore in saferune's class — so the strip
// joins "ma" and "ny" and produces the cap's own phrase out of bytes the server
// never sent. Written as a Go escape rather than a literal (ST1018).
//
// The surrounding words are distinct from every other fixture in this file and
// from the phrases isDeepPagingCap matches, so an assertion below cannot be
// satisfied by a mutant that hardcodes another fixture's literal.
const softHyphenIn429 = "{\"message\":\"Upstream throttle: too ma\u00adny pages " +
	"queued on this key, retry in 41s\"}"

// softHyphen is the same rune, for the assertion that the DISPLAY still strips
// it. Named rather than written inline so the two cannot drift apart.
const softHyphen = '\u00ad'

// realDeepPagingCap is the server's actual cap message. It carries no ignorable
// rune, so the strip cannot be what makes it match.
const realDeepPagingCap = `{"message":"You've requested too many pages; please use cursors instead"}`

// TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne is round 3's
// finding 4, and it is the only BEHAVIOURAL one: a published exit code moved.
//
// 🔴 A DISPLAY FILTER BECAME A CLASSIFIER. readError extracts the server's
// message, hands it to snippet() — which since this PR routes it through
// internal/saferune — and then asked isDeepPagingCap() about the STRIPPED
// string. isDeepPagingCap's own comment says the match is "deliberately narrow
// so a real rate-limit 429 is never misclassified as a usage error", and
// removing invisible runes can only WIDEN it, in exactly that direction: a 429
// whose message carries one Default_Ignorable rune inside "many" was reclassified
// from ErrRateLimited (exit 6) to ErrBadRequest (exit 2). Contrived for
// Civitai's own backend; not contrived for the proxy, CDN and captive-portal
// 429s readError also handles. AGENTS.md items 7 and 24 make that a published
// contract.
//
// Both halves are needed and neither is redundant:
//
//   - (a) is the regression. It is RED at f936cfc (this branch before round 3)
//     and green at HEAD.
//   - (b) is the POSITIVE CONTROL on the classifier. Without it, `func
//     isDeepPagingCap(string) bool { return false }` — and any fix that simply
//     stopped calling it — passes (a) and this test asserts nothing.
func TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne(t *testing.T) {
	// (a) A throttle 429 the strip would turn into a cap.
	c := serveOnce(t, http.StatusTooManyRequests, softHyphenIn429)
	_, err := c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("a 429 must be an error; without it this test asserts on nothing")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("a throttle 429 whose message merely CONTAINS an invisible rune lost "+
			"ErrRateLimited (exit 6). The classifier is reading snippet()'s output, not "+
			"the wire message — the CLI's own strip is inventing the cap phrase:\n%q", err)
	}
	if errors.Is(err, ErrBadRequest) {
		t.Errorf("a throttle 429 was reclassified as a usage error (exit 2):\n%q", err)
	}
	// The DISPLAY must still be stripped. A "fix" that stopped stripping the
	// message would satisfy both checks above and reopen the stderr hole §2 of
	// claudedocs/decisions/38-read-body-repair-and-snippet.md closes.
	if strings.ContainsRune(err.Error(), softHyphen) {
		t.Errorf("U+00AD survived into the error string — snippet no longer strips "+
			"what it prints:\n%q", err)
	}
	if !strings.Contains(err.Error(), "too many pages queued") {
		t.Errorf("the server's own words are missing from the message, so the checks "+
			"above are not about the fixture they name:\n%q", err)
	}

	// (b) The real cap must still be reclassified.
	c = serveOnce(t, http.StatusTooManyRequests, realDeepPagingCap)
	_, err = c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("a 429 must be an error; without it half (b) asserts on nothing")
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("the deep-paging cap must still be reclassified as a usage error "+
			"(exit 2) — otherwise a scripter's 429 backoff loop spins on a request "+
			"that is structurally doomed:\n%q", err)
	}
	if errors.Is(err, ErrRateLimited) {
		t.Errorf("the deep-paging cap must NOT stay ErrRateLimited:\n%q", err)
	}

	// (c) The SECOND, deliberate consequence of classifying pre-snippet, pinned
	// rather than left in a comment: snippet also truncates at 500 BYTES, so
	// before this change a cap message whose phrase sat past the display budget
	// was invisible to the classifier and exited 6 — a scripter's backoff loop
	// spinning forever on a structurally doomed request. The bound is a display
	// budget, not a statement about what the server said.
	long := `{"message":"` + strings.Repeat("context, ", 70) +
		"you have requested too many pages here" + `"}`
	if len(long) <= 520 {
		t.Fatalf("CONTROL failure, not a finding: the fixture is %d bytes, which does "+
			"not exceed snippet's 500-byte bound — half (c) would assert nothing", len(long))
	}
	c = serveOnce(t, http.StatusTooManyRequests, long)
	_, err = c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("a 429 must be an error; without it half (c) asserts on nothing")
	}
	// Positive control on the FIXTURE: the phrase really is past the bound, so
	// the assertion below is about truncation and not about the phrase.
	if strings.Contains(snippet([]byte(long)), "too many pages") {
		t.Fatal("CONTROL failure, not a finding: the cap phrase survives snippet's " +
			"truncation, so half (c) is not testing what it says")
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("a deep-paging cap whose phrase falls past snippet's 500-byte "+
			"display bound must still be reclassified (exit 2). The classifier is "+
			"reading the truncated string:\n%q", err)
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
