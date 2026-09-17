package appapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// This file pins ErrNothingSent at the seam that produces it (issue #637).
// internal/cmd holds the behavioural half — that the past-tense block does not
// print — and it would stay green if this tag were attached for the wrong
// reason, or attached always. What is asserted here is the RELATIONSHIP: the tag
// is present exactly when THE REQUEST NEVER REACHED THE CONNECTION, and absent
// when it did.
//
// ⚠ NOT "when no bytes reached the network" — that is the retracted wording, and
// this header carried it for a round after the code stopped meaning it. A
// request the transport began writing and could not finish is NOT tagged; see
// submitRequestSentTrace and TestSubmitVersionDoesNotTagWhenTheWriteWasCutShort.
// A maintainer deriving the invariant from a file header is how the narrowing
// that test exists to forbid would come back.

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

// TestSubmitVersionTagsNothingSentWhenTheRequestCannotBeBuilt covers the tag
// site issue #637 names second: "doOnceWith → http.NewRequestWithContext fails
// before contact on a malformed CIVITAI_BASE_URL".
//
// 🔴 IT IS REACHABLE, AND THAT WAS CHECKED RATHER THAN ASSUMED. internal/config
// binds CIVITAI_BASE_URL straight onto base_url with no url.Parse and no
// validation anywhere between there and this call, so a control character in the
// environment variable arrives here intact.
//
// ⚠ IT DOES NOT COVER A TAG AT THE BUILD SITE, AND AN EARLIER VERSION OF THIS
// COMMENT SAID IT DID — contradicting appblocks.go's own note four lines below
// that call, which records the mutant showing that tag was redundant. There is
// no tag there: the build error travels back through authedDoWith and the
// `!sent` arm tags it. What this test covers is that arm, reached by a failure
// that never gets near the connection.
func TestSubmitVersionTagsNothingSentWhenTheRequestCannotBeBuilt(t *testing.T) {
	// A DEL byte is rejected by net/url and is the shape an unvalidated env var
	// can carry. The URL is otherwise well-formed, so nothing earlier rejects it.
	c := New("http://exa\x7fmple.com", "tok", "/api/blocks/submit-version")

	_, err := c.SubmitVersion(context.Background(), []byte("zip"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a base URL that cannot form a request must fail the submit")
	}
	if !strings.Contains(err.Error(), "invalid control character") {
		t.Fatalf("CONTROL failure: the failure did not come from request construction, so this "+
			"test is measuring a different site: %v", err)
	}
	if !errors.Is(err, ErrNothingSent) {
		t.Errorf("want ErrNothingSent when the request could not even be built, got %v", err)
	}
}

// TestSubmitVersionDoesNotTagWhenTheServerAnswered is the POSITIVE CONTROL, and
// the negative cases here are worthless without it: a SubmitVersion that
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

// TestSubmitVersionDoesNotTagWhenTheWriteWasCutShort is round 1's F1, and it is
// the case that inverted this mechanism's first design.
//
// 🔴 A SUBMIT THAT TIMES OUT MID-BODY IS NOT "NOTHING SENT". The first version
// keyed the tag on WroteRequest reporting a CLEAN write, reasoning that a failed
// write delivers an unknown number of bytes and might deliver none. Measured
// against this exact server on three runs: the handler had already read 204,594
// / 233,266 / 204,594 body bytes when the CLI said "nothing was uploaded". The
// PR about the CLI making false claims about bytes made one.
//
// 🔴 THE CONTROL COUNTS BYTES ON THE CLIENT SIDE, AND IT HAD TO. Round 2 caught
// the first version asserting only that the SERVER received something — which a
// run where the whole body wrote cleanly into socket buffers also satisfies,
// since the handler stops reading either way. On such a host the test passes
// while the mutant it is credited with killing survives: the ASSERTION holding
// either way does not make the KILL hold either way. A counting net.Conn makes
// the difference observable, and the control was watched to fire — draining the
// body in the handler produces "the client wrote 8389316 of 8388627 body bytes".
func TestSubmitVersionDoesNotTagWhenTheWriteWasCutShort(t *testing.T) {
	var received, written atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "submissions") {
			// The recovery poll's route, answered immediately: it shares this
			// handler, and letting it reach the stall below cost ~6s per run.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"submissions":[]}`))
			return
		}
		buf := make([]byte, 32<<10)
		for i := 0; i < 8; i++ {
			n, err := r.Body.Read(buf)
			received.Add(int64(n))
			if err != nil {
				break
			}
		}
		// Stall with the body part-read so the client's deadline fires mid-upload.
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	base := &net.Dialer{}
	c := New(srv.URL, "tok", "/api/blocks/submit-version")
	c.HTTP = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := base.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return &countingConn{Conn: conn, written: &written}, nil
		},
	}}
	c.SubmitTimeout = 900 * time.Millisecond
	zero := time.Duration(0)
	c.SubmitPollDelay = &zero

	// 6 MiB: big enough that the write cannot complete into a socket buffer,
	// small enough that base64 keeps it under MaxSubmitBodyBytes — over that, the
	// ceiling refusal fires first and this test measures nothing.
	zip := make([]byte, 6<<20)
	_, err := c.SubmitVersion(context.Background(), zip, "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("a submit that times out mid-upload must fail")
	}
	if errors.Is(err, ErrBundleTooLarge) {
		t.Fatalf("CONTROL failure: the ceiling refusal fired, so no upload was attempted: %v", err)
	}
	if got := received.Load(); got == 0 {
		t.Fatalf("CONTROL failure: the server received 0 body bytes, so this run is not the "+
			"cut-short case it claims to measure: %v", err)
	}
	if got, full := written.Load(), int64(SubmitBodySize(len(zip), Provenance{})); got >= full {
		t.Fatalf("CONTROL failure: the client wrote %d of %d body bytes — the transfer COMPLETED, "+
			"so this run does not exercise a cut-short write and the mutant it is credited with "+
			"killing would survive here. The stall is not holding.", got, full)
	}
	if errors.Is(err, ErrNothingSent) {
		t.Errorf("ErrNothingSent was attached to a submit whose body the server had already read "+
			"%d bytes of. The tag suppresses the entry-list diagnosis and, on the timeout arm, "+
			"selects a message asserting nothing was uploaded — both false here, and the second is "+
			"the exact defect issue #637 exists to remove.\ngot: %v", received.Load(), err)
	}
}

// countingConn counts the bytes the CLIENT actually wrote to the socket. The
// server's read count cannot answer that: a handler that stops reading leaves
// the rest in kernel buffers, so "the server got some" is true both when the
// write was cut short and when it completed.
type countingConn struct {
	net.Conn
	written *atomic.Int64
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.written.Add(int64(n))
	return n, err
}

// TestSubmitVersionKeepsTheSentFactAcrossA401Retry pins the STICKINESS of the
// flag across authedDoWith's one refresh-and-retry.
//
// 🔴 AN EARLIER COMMENT HERE SAID THIS COULD NOT BE PINNED WITHOUT A RACE. That
// was wrong, and round 1 of the audit supplied the seam: the separating point is
// the DIALER, not the server. A transport whose DialContext succeeds once and
// then fails gives attempt 1 a full written body and a 401, and attempt 2 a
// failure with nothing written — deterministically, and httptrace still fires
// because it is still an *http.Transport. A comment claiming nobody can cover
// something is what stops the next person looking.
func TestSubmitVersionKeepsTheSentFactAcrossA401Retry(t *testing.T) {
	var hits, dials atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.Copy(io.Discard, r.Body) // read the whole body, then refuse it
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	base := &net.Dialer{}
	tr := &http.Transport{
		DisableKeepAlives: true, // force a second dial for the retry
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if dials.Add(1) > 1 {
				return nil, errors.New("dial refused: the retry never reached the server")
			}
			return base.DialContext(ctx, network, addr)
		},
	}

	c := NewWithSource(srv.URL, refreshingSource{}, "/api/blocks/submit-version")
	c.HTTP = &http.Client{Transport: tr}

	_, err := c.SubmitVersion(context.Background(), []byte("zip-bytes"), "demo", "0.1.0", Provenance{})
	if err == nil {
		t.Fatal("the retry could not dial, so the submit must fail")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("CONTROL failure: the server was hit %d time(s), want 1 — the first attempt must "+
			"have gone out in full for this test to mean anything", got)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("CONTROL failure: %d dial(s), want 2 — the refresh-and-retry did not happen, so "+
			"the stickiness this test exists for was never exercised", got)
	}
	if errors.Is(err, ErrNothingSent) {
		t.Errorf("ErrNothingSent was attached although attempt 1 wrote its whole body to the "+
			"server. A per-attempt reset produces exactly this, and it contradicts README.md's "+
			"promise that the block prints for a request that really went out.\ngot: %v", err)
	}
}

// refreshingSource is a TokenSource that can refresh, so a 401 triggers
// authedDoWith's retry rather than being returned as-is.
type refreshingSource struct{}

func (refreshingSource) Token(context.Context) (string, error)   { return "tok-1", nil }
func (refreshingSource) Refresh(context.Context) (string, error) { return "tok-2", nil }

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

	// 🔴 SUPPRESSING THE BYTE BLOCK IS ONLY HALF THE FIX. The recovery error says
	// "the upload may not have completed … check whether it landed before
	// resubmitting", which produces exactly the hesitation issue #637 is about.
	// When nothing was written the outcome is not unknown, and the message says so.
	if !strings.Contains(err.Error(), "nothing was uploaded and no submission was created") {
		t.Errorf("the timeout error still leaves the outcome open when the CLI knows it: %v", err)
	}
	if strings.Contains(err.Error(), "may not have completed") {
		t.Errorf("the recovery wording reached a run that sent nothing: %v", err)
	}
	// The recovery poll DOES still run — skipping it was tried and reverted,
	// because TestSubmitVersionRecoversFromADeadlineExceededTokenError next door
	// pins the token seam as the positive control for its whole family. So the
	// transport is called once per attempt plus once per poll, and what changed
	// is only what the CLI says afterwards.
	if got := rt.calls.Load(); got != int64(1+submitPollAttempts) {
		t.Errorf("transport calls = %d, want %d (one submit + %d recovery polls) — if the poll "+
			"stopped running, the sibling family's positive control went with it", got,
			1+submitPollAttempts, submitPollAttempts)
	}
}

// TestNothingSentPreservesClassification is an INVARIANT GUARD, not regression
// coverage, and is labelled as one because nothing in this PR could have broken
// it: civitai.Tag's taggedError.Unwrap returns BOTH errors, so no mutation of
// this file's code makes it red, and the mutation battery names no mutant it
// kills. internal/cmd's TestSubmitDiagnosisAbsentWhenTheCredentialIsLocallyRefused
// asserts the same errors.Is through the same chain plus the behavioural half.
//
// Kept anyway, at one layer down: exit 3 for a locally-refused credential is
// published contract, and this is the package that attaches the new tag. What it
// pins is what the tag must NOT do — strip the failure KIND an already-classified
// error carries. internal/auth tags
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
