package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// TestCapWorded429WithRetryAfterExitsFiveNotTwo is the THIRD 429 exit code, and
// the one the published contract calls out in 🔴: a cap-worded 429 that carries
// Retry-After exits 5, NOT 2, because the HEADER is consulted before the
// MESSAGE. See `exitcodes_doc.go`'s code-6 bullet and README's exit-code rows.
//
// 🔴 THE SENTINEL AND THE NUMBER ARE TWO CLAIMS, AND THIS FILE OWNS THE NUMBER.
// pkg/civitai's TestCapWorded429WithRetryAfterIsRetriedNotReclassified pins the
// sentinel (ErrNetwork, not ErrBadRequest) and kills the ordering mutant. It
// stops one hop short of `$?`, which is what a scripter actually branches on and
// what the bullet publishes — the same division of labour the test above states
// for the 6-vs-2 pair. Nothing composed cap-body + Retry-After -> exitCode()
// until this test, so the seam between the classification and the number was
// unowned for the one 429 case the contract marks 🔴.
func TestCapWorded429WithRetryAfterExitsFiveNotTwo(t *testing.T) {
	// Cap wording AND Retry-After on every response, so the request exhausts.
	// Retry-After: 0 makes backoffFor return 0 (it clamps the override), and
	// RetryBackoffBase is pinned to 0 as well so the test cannot sleep even if
	// the override path changes.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(
			`{"message":"You've requested too many pages, please use cursors instead"}`))
	}))
	t.Cleanup(srv.Close)

	c := civitai.New(srv.URL, "")
	zero := time.Duration(0)
	c.RetryBackoffBase = &zero

	_, _, err := c.GetModelVersion(context.Background(), "31708")
	if err == nil {
		t.Fatal("a persistent 429 must be an error; without it this test asserts on nothing")
	}
	// Positive control on the INSTRUMENT: exitCode must be able to return 2 —
	// the very code this test claims was NOT reached — or "it returned 5" says
	// nothing about whether exitCode discriminates at all.
	if got := exitCode(civitai.Tag(civitai.ErrBadRequest, errors.New("bad enum"))); got != exitUsage {
		t.Fatalf("CONTROL failure, not a finding: exitCode(ErrBadRequest) = %d, want %d",
			got, exitUsage)
	}
	// The header decided, so the request was RETRIED rather than reclassified.
	// Asserted here too because it is what makes the 5 meaningful: a 5 reached
	// after ONE request would be a different bug with the same exit code.
	if hits < 2 {
		t.Errorf("the header must win, which means retrying: got %d request(s) — "+
			"1 means the cap wording was consulted first", hits)
	}
	if got := exitCode(err); got != exitNetwork {
		t.Errorf("a cap-worded 429 carrying Retry-After exits %d, want %d (exitNetwork).\n"+
			"Getting %d (exitUsage) means the MESSAGE was consulted before the HEADER, "+
			"inverting the 🔴 bullet in exitcodes_doc.go and README:\n%q",
			got, exitNetwork, exitUsage, err)
	}
}
