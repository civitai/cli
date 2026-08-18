package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newRecoverServer returns an httptest server that hangs on the submit POST
// (forcing the client's short SubmitTimeout to fire as a net.Error timeout)
// and answers GET /api/v1/blocks/submissions with whatever submissions is set
// to at request time. release is closed on cleanup so the hung handler returns.
func newRecoverServer(t *testing.T, submissions func() []Submission) (*httptest.Server, *int32) {
	t.Helper()
	release := make(chan struct{})
	var listCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "submit-version"):
			// Hang past the submit client's timeout, then unblock on teardown.
			<-release
		case r.Method == http.MethodGet && r.URL.Path == SubmissionsPath:
			atomic.AddInt32(&listCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": submissions()})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})
	return srv, &listCalls
}

func recoverClient(url string) *Client {
	c := New(url, "tok", "")
	c.SubmitTimeout = 50 * time.Millisecond // force a fast timeout on the hung POST
	zero := time.Duration(0)
	c.SubmitPollDelay = &zero // don't sleep between poll attempts in tests
	return c
}

// TestSubmitVersionRecoversWhenSubmissionLanded pins the F6 root-cause fix: the
// submit POST times out (response lost) but the submission actually landed, so
// SubmitVersion polls /submissions, finds the pending row, and reports SUCCESS
// surfacing the pubreq id rather than a false failure.
func TestSubmitVersionRecoversWhenSubmissionLanded(t *testing.T) {
	landed := Submission{
		ID:      "pubreq_01KVY45JGYSAXK7DKMQ19T2HGE",
		BlockID: "my-block",
		Version: "0.2.0",
		Status:  "pending",
	}
	srv, listCalls := newRecoverServer(t, func() []Submission {
		return []Submission{landed}
	})

	c := recoverClient(srv.URL)
	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0", Provenance{})
	if err != nil {
		t.Fatalf("SubmitVersion should recover on timeout, got error: %v", err)
	}
	if res.PublishRequestID != landed.ID {
		t.Errorf("PublishRequestID = %q, want %q", res.PublishRequestID, landed.ID)
	}
	if res.Slug != "my-block" || res.Version != "0.2.0" || res.Status != "pending" {
		t.Errorf("unexpected recovered result: %+v", res)
	}
	if atomic.LoadInt32(listCalls) < 1 {
		t.Errorf("expected the recovery to poll /submissions at least once, got %d calls", *listCalls)
	}
}

// TestSubmitVersionRecoveryErrorsWhenAbsent pins the other half: the submit
// POST times out and NO matching submission shows up, so SubmitVersion returns
// a clear, actionable error (upload may not have completed; check app status).
func TestSubmitVersionRecoveryErrorsWhenAbsent(t *testing.T) {
	srv, _ := newRecoverServer(t, func() []Submission {
		// A different app/version is present, but not the one we submitted.
		return []Submission{{ID: "pubreq_other", BlockID: "someone-else", Version: "9.9.9", Status: "pending"}}
	})

	c := recoverClient(srv.URL)
	_, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0", Provenance{})
	if err == nil {
		t.Fatal("expected an error when no matching submission landed")
	}
	for _, want := range []string{"timed out", "may not have completed", "civitai app status my-block"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

// TestSubmitVersionRecoveryMatchesOnlyExactSlugVersion guards the matcher: a
// pending submission for the same slug but a DIFFERENT version must not be
// mistaken for this submit's success.
func TestSubmitVersionRecoveryMatchesOnlyExactSlugVersion(t *testing.T) {
	srv, _ := newRecoverServer(t, func() []Submission {
		return []Submission{{ID: "pubreq_old", BlockID: "my-block", Version: "0.1.0", Status: "pending"}}
	})

	c := recoverClient(srv.URL)
	_, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "my-block", "0.2.0", Provenance{})
	if err == nil {
		t.Fatal("expected an error: only a different version is pending, not 0.2.0")
	}
}

// TestIsTimeoutErr unit-checks the classifier: a deadline-exceeded counts as a
// timeout; a plain error does not.
func TestIsTimeoutErr(t *testing.T) {
	if !isTimeoutErr(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be classified as a timeout")
	}
	if isTimeoutErr(nil) {
		t.Error("nil is not a timeout")
	}
	if isTimeoutErr(errPlain("boom")) {
		t.Error("a plain error is not a timeout")
	}
}

type errPlain string

func (e errPlain) Error() string { return string(e) }
