package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestReadErrorReachesStderrWithoutTerminalControlRunes is the SEAM guard for
// the civitai/cli#526 follow-up's (a) decision.
//
// 🔴 EACH SIDE OF THIS SEAM IS TESTED AND NEITHER SIDE OWNS IT.
// pkg/civitai proves snippet() strips; internal/cmd proves safeTerm strips on
// the HUMAN renderers. Nothing joined them, and the join is where the hole was:
// errorLine below hands err.Error() to fmt.Fprintln(os.Stderr, …) with no
// filter of its own, so before this a 404 body carrying ESC[1A ESC[2K could
// erase the CLI's own preceding output from inside an "Error: " line.
//
// This test builds the state neither package's fixtures build: a real HTTP
// reply, decoded by the real SDK, rendered by the real stderr renderer.
//
// It also pins the published EXIT CODE across the change (AGENTS.md item 7): a
// 404 must still classify as exit 4. The strip touches the message, and the
// message is not what carries the classification — this is the assertion that
// says so rather than assuming it.
func TestReadErrorReachesStderrWithoutTerminalControlRunes(t *testing.T) {
	// ESC[1A is CURSOR UP, ESC[2K is ERASE LINE, U+2066 is an isolate that can
	// reorder what follows. All three are written as escapes: a literal control
	// byte in a Go string literal is what staticcheck's ST1018 catches.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(
			"{\"message\":\"no version 44190\\u001b[1A\\u001b[2K\\u2066 checksum ok\"}"))
	}))
	t.Cleanup(srv.Close)

	_, _, err := civitai.New(srv.URL, "").GetModelVersion(context.Background(), "44190")
	if err == nil {
		t.Fatal("a 404 must be an error; without it this test asserts on nothing")
	}
	line := errorLine(err)

	// Positive control FIRST: the server's own words must be in the line, or the
	// absence checks below pass on an empty string.
	if !strings.Contains(line, "no version 44190") || !strings.Contains(line, "checksum ok") {
		t.Fatalf("the server's words are missing — the checks below would be vacuous:\n%q", line)
	}
	for _, bad := range []struct {
		r    rune
		name string
	}{
		{'\u001b', "ESC (0x1B), the ANSI/OSC introducer"},
		{'\u2066', "U+2066 LEFT-TO-RIGHT ISOLATE"},
	} {
		if strings.ContainsRune(line, bad.r) {
			t.Errorf("%s reached the line main() writes to stderr:\n%q", bad.name, line)
		}
	}
	// The published exit code for a 404 read is 4 and must not have moved.
	if got := exitCode(err); got != exitNotFound {
		t.Errorf("exit code = %d, want %d (exitNotFound) — a 404 read's published "+
			"classification must survive the message filter", got, exitNotFound)
	}
}

// TestThrottle429KeepsExitSixThroughTheMessageFilter is the EXIT-CODE half of
// round 3's finding 4, at the seam where the code is actually published.
//
// 🔴 A 404 IS THE WRONG WITNESS FOR THIS CLAIM, AND IT WAS THE ONLY ONE.
// The test above pins a 404 at exit 4, and item 38's evidence file cited it for
// "the exit-code contract is unchanged". But 404 is classified from the STATUS
// alone — the message filter cannot reach it, so that assertion is vacuous about
// filtering. The 429 branch is the one place in pkg/civitai where the TEXT of
// an error picks the exit code, and it is where the filter did move one:
// snippet() strips Default_Ignorable runes, so a proxy throttle whose message
// carried a U+00AD inside "many" was reclassified from 6 to 2.
//
// pkg/civitai's TestDeepPagingCapClassifiesOnTheWireMessageNotTheStrippedOne
// owns the sentinel; this owns the number a script reads from `$?`.
func TestThrottle429KeepsExitSixThroughTheMessageFilter(t *testing.T) {
	// U+00AD SOFT HYPHEN inside "many", written as an escape (ST1018).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(
			"{\"message\":\"Edge throttle: too ma\\u00adny pages in flight, slow down\"}"))
	}))
	t.Cleanup(srv.Close)

	_, _, err := civitai.New(srv.URL, "").GetModelVersion(context.Background(), "31708")
	if err == nil {
		t.Fatal("a 429 must be an error; without it this test asserts on nothing")
	}
	// Positive control on the INSTRUMENT: exitCode must be able to return 2, or
	// "it returned 6" says nothing about whether it discriminates at all.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("CONTROL failure, not a finding: exitCode(ErrBadRequest) = %d, want %d",
			got, exitUsage)
	}
	if got := exitCode(err); got != exitRateLimited {
		t.Errorf("a throttle 429 exits %d, want %d (exitRateLimited).\n"+
			"The deep-paging-cap reclassification is reading the FILTERED message, so "+
			"the CLI's own strip synthesised the cap phrase and turned a retryable "+
			"throttle into a usage error:\n%q", got, exitRateLimited, err)
	}
}
