package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The submit-version response grew an OPTIONAL `listing` field
// (civitai/civitai#7311): the store listing's readiness for the submission just
// queued. It is `null` when the server could not read it, and ABSENT on a server
// that predates it, so all three shapes must decode — the old two to a nil
// pointer, never to a zero-valued struct that would read as "not blocked".

// submitWithBody runs SubmitVersion against a server answering 200 with body.
func submitWithBody(t *testing.T, body string) *SubmitResult {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "tok", "")
	res, err := c.SubmitVersion(context.Background(), []byte("ZIP"), "demo", "0.2.0", Provenance{})
	if err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if res == nil {
		t.Fatal("SubmitVersion returned a nil result with a nil error")
	}
	return res
}

func TestSubmitVersionDecodesListingReadiness(t *testing.T) {
	// The shape the server sends (publish-request.service.ts SubmitListingReadiness,
	// problems from computeListingProblems: code / label / severity). Field values
	// are pairwise distinct so a decode that crossed two fields shows up.
	res := submitWithBody(t, `{
		"publishRequestId": "pr_7", "slug": "demo", "version": "0.2.0", "status": "pending",
		"listing": {
			"approvalBlocked": true,
			"missing": ["icon", "cover"],
			"nextStep": "Add an icon and a cover on your listing's Media tab before a moderator can approve it.",
			"problems": [
				{"code": "missing-icon", "label": "Missing icon (required before publishing)", "severity": "blocking"},
				{"code": "no-screenshots", "label": "No screenshots (recommended)", "severity": "advisory"}
			]
		}
	}`)
	l := res.Listing
	if l == nil {
		t.Fatal("Listing is nil; the server sent a listing object")
	}
	if !l.ApprovalBlocked {
		t.Error("ApprovalBlocked = false, want true")
	}
	if len(l.Missing) != 2 || l.Missing[0] != "icon" || l.Missing[1] != "cover" {
		t.Errorf("Missing = %q, want [icon cover]", l.Missing)
	}
	if want := "Add an icon and a cover on your listing's Media tab before a moderator can approve it."; l.NextStep != want {
		t.Errorf("NextStep = %q, want %q", l.NextStep, want)
	}
	if len(l.Problems) != 2 {
		t.Fatalf("Problems = %+v, want 2", l.Problems)
	}
	p0, p1 := l.Problems[0], l.Problems[1]
	if p0.Code != "missing-icon" || p0.Label != "Missing icon (required before publishing)" || p0.Severity != "blocking" {
		t.Errorf("Problems[0] = %+v", p0)
	}
	if p1.Code != "no-screenshots" || p1.Label != "No screenshots (recommended)" || p1.Severity != "advisory" {
		t.Errorf("Problems[1] = %+v", p1)
	}
	// The rest of the result still decodes beside the new field.
	if res.PublishRequestID != "pr_7" || res.Slug != "demo" || res.Status != "pending" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestSubmitVersionListingNullAndAbsentDecodeToNil(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"absent (older server)", `{"publishRequestId":"pr_1","slug":"demo","version":"0.2.0","status":"pending"}`},
		{"null (could not be read)", `{"publishRequestId":"pr_1","slug":"demo","version":"0.2.0","status":"pending","listing":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := submitWithBody(t, tc.body)
			if res.Listing != nil {
				t.Errorf("Listing = %+v, want nil", res.Listing)
			}
			if res.PublishRequestID != "pr_1" {
				t.Errorf("PublishRequestID = %q, want pr_1", res.PublishRequestID)
			}
		})
	}
}

func TestSubmitVersionListingNullNextStepDecodesEmpty(t *testing.T) {
	// nextStep is `string | null`; null must decode to "" rather than fail the
	// whole submit result.
	res := submitWithBody(t, `{"publishRequestId":"pr_2","slug":"demo","version":"0.2.0","status":"pending",
		"listing":{"approvalBlocked":false,"missing":[],"nextStep":null,"problems":[]}}`)
	if res.Listing == nil {
		t.Fatal("Listing is nil; the server sent a listing object")
	}
	if res.Listing.ApprovalBlocked || res.Listing.NextStep != "" || len(res.Listing.Problems) != 0 {
		t.Errorf("Listing = %+v, want not blocked, empty next step, no problems", res.Listing)
	}
}
