package genapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// signedURLSentinel stands in for the signature of a presigned URL the server
// put in a response body.
const signedURLSentinel = "SENTINEL-SIGNED-URL-9c1e"

// TestSignedUploadURLIsNeverEchoed pins that a presigned URL in a response body
// never reaches the error text. The presign reply's whole payload is a signed
// PUT URL that authorizes an upload by itself, and the upload reply's `url` can
// be a signed read URL — so the decode-failure, missing-field and non-200 paths
// used to print a working credential on stderr.
//
// Each row asserts the body it served carries the sentinel (or "not echoed"
// would hold vacuously), the error does not, and the error still names the
// route or status (or "not echoed" would also be satisfied by swallowing it).
func TestSignedUploadURLIsNeverEchoed(t *testing.T) {
	signed := `https://uploads.example.com/v2/consumer/blobs?sig=` + signedURLSentinel + `&se=2030`

	cases := []struct {
		name     string
		presign  int
		pBody    string
		upload   int
		uBody    string
		wantText string
	}{
		{name: "presign: 200 that does not decode beside the signed URL", presign: 200,
			pBody: `{"uploadUrl":"` + signed + `","expiresAt":2030}`, wantText: "unexpected getConsumerBlobUploadUrl response"},
		{name: "presign: 200 with the signed URL under another key", presign: 200,
			pBody: `{"url":"` + signed + `","expiresAt":"2030-01-01T00:00:00Z"}`, wantText: "unexpected getConsumerBlobUploadUrl response"},
		{name: "presign: 201 carrying the signed URL", presign: 201,
			pBody: `{"uploadUrl":"` + signed + `","expiresAt":"2030-01-01T00:00:00Z"}`, wantText: "refused to issue an image-upload URL (201)"},
		{name: "presign: 500 carrying the signed URL", presign: 500,
			pBody: `{"uploadUrl":"` + signed + `","expiresAt":"2030-01-01T00:00:00Z"}`, wantText: "refused to issue an image-upload URL (500)"},
		{name: "presign: 403 plain text holding the signed URL", presign: 403,
			pBody: "denied for " + signed, wantText: "refused to issue an image-upload URL (403)"},
		{name: "upload reply: 200 that does not decode beside a signed blob URL", upload: 200,
			uBody: `{"id":7,"available":true,"url":"` + signed + `"}`, wantText: "unexpected image-upload response"},
		{name: "upload reply: 403 carrying a signed blob URL", upload: 403,
			uBody: `{"id":"BLOB1","available":true,"url":"` + signed + `"}`, wantText: "the image upload was rejected (403)"},
		{name: "upload reply: 500 plain text holding a signed blob URL", upload: 500,
			uBody: "stored at " + signed, wantText: "the image upload was rejected (500)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.pBody+tc.uBody, signedURLSentinel) {
				t.Fatalf("PREMISE BROKEN: the fixture bodies carry no signed URL")
			}
			h := newBlobHarness(t)
			if tc.presign != 0 {
				h.presignStatus, h.presignBody = tc.presign, tc.pBody
			}
			if tc.upload != 0 {
				h.uploadStatus, h.uploadBody2 = tc.upload, tc.uBody
			}

			_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
			if err == nil {
				t.Fatal("a signed-URL route failure returned no error — the failure was swallowed")
			}
			msg := err.Error()
			if strings.Contains(msg, signedURLSentinel) {
				t.Errorf("the error text carries the signed URL from the body:\n  %s", msg)
			}
			if !strings.Contains(msg, tc.wantText) {
				t.Errorf("the error no longer names the route or status (want %q):\n  %s", tc.wantText, msg)
			}
		})
	}
}

// TestCredentialRouteErrorMessageIsShown pins the other half of the rule: what
// the server says in an error MESSAGE field still reaches the user. Without it,
// "never echo the body" is also satisfied by withholding everything — which is
// what applying the shape check to the WHOLE presign body did to the route's
// real JSON 403, leaving only a byte count.
func TestCredentialRouteErrorMessageIsShown(t *testing.T) {
	const scopeMsg = "Your API key does not have the required scope for this action"
	cases := []struct {
		name    string
		presign int
		pBody   string
		upload  int
		uBody   string
		want    string
		unauth  bool
	}{
		{name: "presign: JSON 403 with an error field", presign: 403,
			pBody: `{"error":"` + scopeMsg + `"}`, want: scopeMsg, unauth: true},
		{name: "presign: bare-text 403", presign: 403,
			pBody: "Orchestrator access denied for this account", want: "Orchestrator access denied for this account", unauth: true},
		{name: "upload reply: JSON 403 with a message field", upload: 403,
			uBody: `{"message":"Request has expired"}`, want: "Request has expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newBlobHarness(t)
			if tc.presign != 0 {
				h.presignStatus, h.presignBody = tc.presign, tc.pBody
			}
			if tc.upload != 0 {
				h.uploadStatus, h.uploadBody2 = tc.upload, tc.uBody
			}
			_, err := h.client.UploadImageBlob(context.Background(), h.upl, "image/png", []byte("PNG"))
			if err == nil {
				t.Fatal("a refused request returned no error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the server's error message did not reach the user (want %q):\n  %s", tc.want, err)
			}
			if tc.unauth && !errors.Is(err, civitai.ErrUnauthorized) {
				t.Errorf("a presign 403 must stay classified as ErrUnauthorized: %v", err)
			}
		})
	}
}

// TestWorkflowPayloadIsNeverEchoed pins that a getWorkflow or
// queryGeneratedImages payload that fails to decode is reported by its size, not
// printed: a workflow's outputs carry blob URLs, which can be signed read URLs.
// The decode error is still shown, so the failure stays diagnosable.
func TestWorkflowPayloadIsNeverEchoed(t *testing.T) {
	signed := `https://orchestration.example.com/v2/consumer/blobs/B1.png?sig=` + signedURLSentinel + `&se=2030`
	cases := []struct {
		name     string
		payload  string
		call     func(c *Client) error
		wantText string
	}{
		{"getWorkflow: off-shape id beside a signed output URL",
			`{"id":7,"status":"succeeded","steps":[{"name":"s0","output":{"images":[{"id":"B1","url":"` + signed + `"}]}}]}`,
			func(c *Client) error { _, _, err := c.GetWorkflow(context.Background(), "wf_1"); return err },
			"unexpected orchestrator.getWorkflow payload"},
		{"queryGeneratedImages: off-shape id beside a signed output URL",
			`{"items":[{"id":7,"status":"succeeded","url":"` + signed + `"}],"nextCursor":null}`,
			func(c *Client) error { _, _, err := c.QueryWorkflows(context.Background(), ListOptions{Take: 5}); return err },
			"unexpected orchestrator.queryGeneratedImages payload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.payload, signedURLSentinel) {
				t.Fatalf("PREMISE BROKEN: the payload carries no signed URL")
			}
			c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"result":{"data":{"json":` + tc.payload + `}}}`))
			}))
			err := tc.call(c)
			if err == nil {
				t.Fatal("an undecodable payload returned no error — the failure was swallowed")
			}
			msg := err.Error()
			if strings.Contains(msg, signedURLSentinel) {
				t.Errorf("the error text carries the signed URL from the payload:\n  %s", msg)
			}
			if !strings.Contains(msg, tc.wantText) {
				t.Errorf("the error no longer names the procedure (want %q):\n  %s", tc.wantText, msg)
			}
		})
	}
}
