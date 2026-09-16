package appapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync/atomic"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// This file pins ErrNothingSent at the seam that produces it (issue #637).
// internal/cmd holds the behavioural half — that the past-tense block does not
// print — and it would stay green if this tag were attached for the wrong
// reason, or attached always. What is asserted here is the RELATIONSHIP: the tag
// is present exactly when no request bytes reached the network, and absent when
// they did.

// errTokenSource fails the way internal/auth does when an OAuth refresh succeeds
// over the network but persisting the new tokens to disk does not.
type errTokenSource struct{ err error }

func (e errTokenSource) Token(context.Context) (string, error)   { return "", e.err }
func (e errTokenSource) Refresh(context.Context) (string, error) { return "", e.err }

// countingHandler answers every request with status, recording how many arrived.
func countingHandler(hits *atomic.Int64, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// TestSubmitVersionTagsNothingSentWhenTheTokenLookupFails is #637's measured
// path: authedDoWith returns on Tokens.Token() before doOnceWith builds
// anything, and internal/auth returns that error UNTAGGED.
func TestSubmitVersionTagsNothingSentWhenTheTokenLookupFails(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(countingHandler(&hits, http.StatusOK, "{}"))
	defer srv.Close()

	c := NewWithSource(srv.URL, errTokenSource{
		err: fmt.Errorf("persist refreshed tokens: open config.yaml: read-only file system"),
	}, "/api/blocks/submit-version")

	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a token-source failure must fail the submit")
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("CONTROL failure: the server saw %d request(s), want 0 — this case is not "+
			"exercising the pre-request failure it claims to", got)
	}
	if !errors.Is(err, ErrNothingSent) {
		t.Errorf("want ErrNothingSent on an error raised before any request was built, got %v.\n"+
			"Untagged, `app submit` prints \"What this CLI sent … N bytes on the wire\" over the "+
			"zero bytes this server just counted — issue #637.", err)
	}
	// The message must survive the tag verbatim: AGENTS.md item 7.
	if got := err.Error(); got != "persist refreshed tokens: open config.yaml: read-only file system" {
		t.Errorf("the tag changed the user-visible message, got %q", got)
	}
}

// TestSubmitVersionTagsNothingSentWhenTheDialFails is the case a "was a request
// BUILT" gate would miss: the request is built and handed to the transport, and
// the connection is never established.
func TestSubmitVersionTagsNothingSentWhenTheDialFails(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close()

	c := New(url, "tok", "/api/blocks/submit-version")
	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a dial failure must fail the submit")
	}
	if !errors.Is(err, ErrNothingSent) {
		t.Errorf("want ErrNothingSent when the connection was never established, got %v", err)
	}
}

// TestSubmitVersionDoesNotTagWhenTheServerAnswered is the POSITIVE CONTROL, and
// the three negative cases here are worthless without it: a SubmitVersion that
// tagged unconditionally would satisfy every one of them.
//
// It also pins the feature #637 must not "fix" by deleting: a 500 that arrives
// AFTER the upload is exactly the case the entry-list diagnosis exists for
// (#423), and the tag must stay off it.
func TestSubmitVersionDoesNotTagWhenTheServerAnswered(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(countingHandler(&hits, http.StatusInternalServerError, `{"error":"boom"}`))
	defer srv.Close()

	c := New(srv.URL, "tok", "/api/blocks/submit-version")
	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a 500 must fail the submit")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("CONTROL failure: the server saw %d request(s), want exactly 1 — the negative "+
			"cases in this file prove nothing if this harness cannot reach the server at all", got)
	}
	if errors.Is(err, ErrNothingSent) {
		t.Errorf("ErrNothingSent was attached to a failure the server ANSWERED (%d request(s) "+
			"received). The tag is a wire fact; tagging here suppresses the #423 diagnosis on the "+
			"one path it was written for. got: %v", hits.Load(), err)
	}
}

// TestSubmitVersionDoesNotTagWhenTheFAILURECAMEAfterTheWrite is the case the
// `!wrote.Load()` condition on the ordinary arm exists for, and the only one in
// this file where that condition is load-bearing.
//
// 🔴 IT WAS FOUND BY A SURVIVING MUTANT, not by design: replacing that arm with
// an unconditional tag passed the whole battery, because every other failure
// here is a transport error where nothing was written anyway, and a non-200 is
// not an error at that point at all (authedDoWith returns the status, and
// serverError is built afterwards on a different path). The over-cap body read
// is the one reachable failure that arrives AFTER the request went out, so it is
// what makes the condition falsifiable.
func TestSubmitVersionDoesNotTagWhenTheFailureCameAfterTheWrite(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, 4096))
		}
	}())
	defer srv.Close()

	c := New(srv.URL, "tok", "/api/blocks/submit-version")
	c.MaxResponseBody = 16 // the reply is 4 KiB, so readBody fails over the cap

	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("an over-cap response body must fail the submit")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("CONTROL failure: the server saw %d request(s), want exactly 1 — the failure this "+
			"test is about has to happen AFTER the request went out", got)
	}
	if errors.Is(err, ErrNothingSent) {
		t.Errorf("ErrNothingSent was attached to a failure that arrived after the request was "+
			"written (%d request(s) received). The request bytes DID leave this machine, so the "+
			"past-tense diagnosis is true here and suppressing it loses the one thing the CLI knows "+
			"and the response does not. got: %v", hits.Load(), err)
	}
}

// TestSubmitWroteTraceIgnoresAFailedWrite pins the narrowing inside the trace
// hook: `if info.Err == nil`.
//
// 🔴 IT DRIVES THE HOOK DIRECTLY, AND AN EARLIER VERSION OF THIS TEST DROVE A
// REAL DROPPED CONNECTION AND WAS FLAKY BY CONSTRUCTION. Measured against a
// server that closes every connection the instant it accepts it: WroteRequest
// fired on all 4 runs, with info.Err SET on two and NIL on the other two, where
// the write had completed into the socket and the failure surfaced on the read.
// The same physical event resolves either way, so a test asserting either branch
// of it is asserting on a race. Calling the hook is the deterministic version of
// the same question, and it is why submitWroteTrace is a named function.
//
// Both directions, because the negative alone is satisfied by a hook that never
// records anything at all.
func TestSubmitWroteTraceIgnoresAFailedWrite(t *testing.T) {
	var wrote atomic.Bool
	tr := submitWroteTrace(&wrote)

	tr.WroteRequest(httptrace.WroteRequestInfo{Err: errors.New("write tcp: broken pipe")})
	if wrote.Load() {
		t.Error("a write that FAILED was recorded as sent. How much of the body reached the wire " +
			"is unknown and can be zero, so `app submit` would print \"N bytes on the wire\" for a " +
			"number nothing supports — issue #637's defect, one step later.")
	}

	tr.WroteRequest(httptrace.WroteRequestInfo{})
	if !wrote.Load() {
		t.Error("CONTROL failure: a write that SUCCEEDED was not recorded, so the check above " +
			"passes against a hook that records nothing and every submit failure would be tagged " +
			"ErrNothingSent — which silently deletes the diagnosis instead of narrowing it.")
	}
}

// deadlineRoundTripper fails every request with a timeout WITHOUT ever writing
// it, which is what a dial that never completes looks like from here. A real
// unroutable address would do the same thing over several seconds and depend on
// the host's network; this is the same shape, deterministic.
type deadlineRoundTripper struct{ calls atomic.Int64 }

func (d *deadlineRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return nil, context.DeadlineExceeded
}

// TestSubmitVersionTagsNothingSentWhenATimeoutPrecededTheWrite covers the arm
// that needs its own tagging: timedOutSubmitError formats its cause with %v, so
// a tag applied before recoverTimedOutSubmit is dropped rather than carried.
//
// A timeout is the one failure where the answer genuinely depends on WHEN it
// happened — after the write, the bytes did leave and the diagnosis is the most
// useful thing the CLI can print; before it, nothing left and the same block
// would be false. Only the second case is asserted here, because it is the one
// that was wrong.
func TestSubmitVersionTagsNothingSentWhenATimeoutPrecededTheWrite(t *testing.T) {
	rt := &deadlineRoundTripper{}
	zero := time.Duration(0)
	c := New("http://127.0.0.1:1", "tok", "/api/blocks/submit-version")
	c.HTTP = &http.Client{Transport: rt}
	c.SubmitPollDelay = &zero

	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a timed-out submit with no recoverable submission must fail")
	}
	if rt.calls.Load() == 0 {
		t.Fatal("CONTROL failure: the stubbed transport was never called, so this test is not " +
			"exercising the timeout arm")
	}
	if !errors.Is(err, ErrNothingSent) {
		t.Errorf("want ErrNothingSent when the timeout preceded the write, got %v.\n"+
			"timedOutSubmitError uses %%v, not %%w, so this arm has to tag its own result — a tag "+
			"applied to the cause is dropped on the floor.", err)
	}
}

// TestNothingSentPreservesClassification pins what the tag must NOT do: strip
// the failure KIND an already-classified error carries. internal/auth tags
// civitai.ErrUnauthorized onto "no refresh token stored" — a local error — and
// exit code 3 for that is published contract (claudedocs/decisions/07, and the
// exit-code map in cmd/civitai branches on errors.Is).
func TestNothingSentPreservesClassification(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(countingHandler(&hits, http.StatusOK, "{}"))
	defer srv.Close()

	c := NewWithSource(srv.URL, errTokenSource{
		err: civitai.Tag(civitai.ErrUnauthorized, errors.New("no refresh token stored — run `civitai login` again")),
	}, "/api/blocks/submit-version")

	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a locally-refused credential must fail the submit")
	}
	if !errors.Is(err, ErrNothingSent) {
		t.Errorf("want ErrNothingSent, got %v", err)
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("the tag must ADD a wire fact, never replace the classification: "+
			"errors.Is(err, civitai.ErrUnauthorized) is false, so this failure no longer exits 3. got %v", err)
	}
}
