package genapi

import (
	"context"
	"strings"
	"testing"
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
