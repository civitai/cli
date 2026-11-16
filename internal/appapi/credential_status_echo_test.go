package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// statusSentinel stands in for a live credential, or the signature of a signed
// URL, that the server put in a response body. It is distinct from credSentinel
// (credential_body_error_test.go) so a failure names which table it came from.
const statusSentinel = "SENTINEL-STATUS-CRED-4b9d"

// TestCredentialRouteResponseIsNeverEchoed pins that a response body from a
// route whose success payload IS a credential (OAuth tokens, a device_code, a
// dev token, a Forgejo push token, a presigned upload URL) never reaches the
// error text — on the paths #800 left alone because they are outside its diff:
//
//   - a NON-200 status. A 201 is still a successful mint for most servers, and
//     a 500 from a proxy or a half-failed handler can carry the payload the
//     handler had already built. Every one of these used to fall back to the
//     raw body when it had no recognised error field, so
//     `token refresh failed (status 201): {"access_token":"…"}` printed the
//     token.
//   - a 200 whose body decodes but lacks a field the CLI needs, or does not
//     decode at all, on the image-upload mint (which printed the signed PUT URL).
//
// Every row asserts three things: the body it served carries the sentinel (or
// "not echoed" would hold vacuously), the error does not, and the error still
// names the route or the status (or "not echoed" would also be satisfied by
// swallowing the failure).
func TestCredentialRouteResponseIsNeverEchoed(t *testing.T) {
	tokenBody := `{"access_token":"` + statusSentinel + `","token_type":"Bearer","expires_in":3600,` +
		`"refresh_token":"` + statusSentinel + `","scope":"33554433"}`
	deviceBody := `{"device_code":"` + statusSentinel + `","user_code":"ABCD-EFGH",` +
		`"verification_uri":"https://civitai.com/device","expires_in":600,"interval":5}`
	cloneBody := `{"result":{"data":{"json":{"notYetAvailable":false,"slug":"my-block",` +
		`"token":"` + statusSentinel + `","cloneUrl":"https://dev-7:` + statusSentinel + `@forgejo.example.com/apps/my-block.git"}}}}`
	devTokenBody := `{"token":"` + statusSentinel + `"}`
	signedURL := `https://uploads.example.com/put/abc123?X-Amz-Signature=` + statusSentinel + `&X-Amz-Expires=600`

	refresh := func(base string) error {
		_, err := NewOAuthClient(base).Refresh(context.Background(), "old-rt")
		return err
	}
	deviceToken := func(base string) error {
		_, _, err := NewOAuthClient(base).pollOnce(context.Background(), "dc")
		return err
	}
	deviceInit := func(base string) error {
		_, err := NewOAuthClient(base).StartDevice(context.Background())
		return err
	}
	cloneInfo := func(base string) error {
		_, err := New(base, "tok", "").GetForgejoCloneInfo(context.Background(), "my-block")
		return err
	}
	devToken := func(base string) error {
		_, err := New(base, "tok", "").MintDevToken(context.Background(), "my-block", nil, nil, false)
		return err
	}
	imageUpload := func(base string) error {
		_, _, err := New(base, "tok", "").MintImageUpload(context.Background())
		return err
	}

	cases := []struct {
		name     string
		status   int
		body     string
		wantText string
		call     func(base string) error
	}{
		{"refresh: 201 carrying the rotated pair", 201, tokenBody, "token refresh failed (status 201)", refresh},
		{"refresh: 500 carrying the rotated pair", 500, tokenBody, "token refresh failed (status 500)", refresh},
		{"device-token: 201 carrying tokens", 201, tokenBody, "device-token poll failed (status 201)", deviceToken},
		{"device-token: 500 carrying tokens", 500, tokenBody, "device-token poll failed (status 500)", deviceToken},
		{"device-init: 201 carrying the device_code", 201, deviceBody, "device init failed (status 201)", deviceInit},
		{"device-init: 500 carrying the device_code", 500, deviceBody, "device init failed (status 500)", deviceInit},
		{"forgejo clone info: 201 carrying the push token", 201, cloneBody, "getMyForgejoCloneInfo failed (HTTP 201)", cloneInfo},
		{"forgejo clone info: 500 carrying the push token", 500, cloneBody, "getMyForgejoCloneInfo failed (HTTP 500)", cloneInfo},
		{"forgejo clone info: 403 plain-text body", 403, "denied " + statusSentinel, "not permitted", cloneInfo},
		{"dev-token: 201 carrying the token", 201, devTokenBody, "server returned 201", devToken},
		{"dev-token: 500 carrying the token", 500, devTokenBody, "server returned 500", devToken},
		{"image-upload mint: 200 with the signed URL but no id", 200,
			`{"id":"","uploadURL":"` + signedURL + `"}`, "unexpected image-upload response", imageUpload},
		{"image-upload mint: 200 that does not decode beside the signed URL", 200,
			`{"id":42,"uploadURL":"` + signedURL + `"}`, "unexpected image-upload response", imageUpload},
		{"image-upload mint: 201 carrying the signed URL", 201,
			`{"id":"img-1","uploadURL":"` + signedURL + `"}`, "HTTP 201", imageUpload},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.body, statusSentinel) {
				t.Fatalf("PREMISE BROKEN: the fixture body carries no credential")
			}
			var base string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/openid-configuration" {
					writeDiscovery(w, base)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			base = srv.URL

			err := tc.call(srv.URL)
			if err == nil {
				t.Fatal("a credential-route failure returned no error — the failure was swallowed")
			}
			msg := err.Error()
			if strings.Contains(msg, statusSentinel) {
				t.Errorf("the error text carries the credential from the body:\n  %s", msg)
			}
			if !strings.Contains(msg, tc.wantText) {
				t.Errorf("the error no longer names the route or status (want %q):\n  %s", tc.wantText, msg)
			}
		})
	}
}
