package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// credSentinel stands in for a live credential the server put in a 2xx body.
const credSentinel = "SENTINEL-CREDENTIAL-7f3a"

// TestUndecodableCredentialBodyIsNotEchoed pins that a 2xx body from a route
// whose success payload IS a credential never reaches the error text when it
// fails to decode.
//
// 🔴 A body that does not decode is not a body without a credential. One
// off-shape field (a numeric `scope`, a string `expires_in`, a raw control byte
// anywhere) fails the whole json.Unmarshal while the tokens beside it are
// intact — and the refresh route has already ROTATED them, so the only copy of
// the new refresh token used to be the `Error: unexpected refresh response: …`
// line on stderr, in a terminal scrollback or a CI log. Measured on the real
// binary at df56b13 with `civitai whoami` against a refresh reply carrying
// `"scope":33554433`: both tokens printed, the config kept the revoked pair.
//
// Every row asserts the failure is still REPORTED (an error, naming the route)
// — "never echo" is also satisfied by swallowing the failure, which would be
// worse.
func TestUndecodableCredentialBodyIsNotEchoed(t *testing.T) {
	tokenBody := `{"access_token":"` + credSentinel + `","token_type":"Bearer","expires_in":3600,` +
		`"refresh_token":"` + credSentinel + `","scope":33554433}`

	cases := []struct {
		name     string
		body     string
		wantText string
		call     func(base string) error
	}{
		{
			name:     "refresh: numeric scope beside live tokens",
			body:     tokenBody,
			wantText: "unexpected refresh response",
			call: func(base string) error {
				_, err := NewOAuthClient(base).Refresh(context.Background(), "old-rt")
				return err
			},
		},
		{
			name: "refresh: string expires_in beside live tokens",
			body: `{"access_token":"` + credSentinel + `","token_type":"Bearer","expires_in":"3600",` +
				`"refresh_token":"` + credSentinel + `","scope":"33554433"}`,
			wantText: "unexpected refresh response",
			call: func(base string) error {
				_, err := NewOAuthClient(base).Refresh(context.Background(), "old-rt")
				return err
			},
		},
		{
			name:     "device-token poll: numeric scope beside live tokens",
			body:     tokenBody,
			wantText: "unexpected device-token response",
			call: func(base string) error {
				_, _, err := NewOAuthClient(base).pollOnce(context.Background(), "dc")
				return err
			},
		},
		{
			name: "device-init: string expires_in beside the device_code",
			body: `{"device_code":"` + credSentinel + `","user_code":"ABCD-EFGH",` +
				`"verification_uri":"https://civitai.com/device","expires_in":"600","interval":5}`,
			wantText: "unexpected device-init response",
			call: func(base string) error {
				_, err := NewOAuthClient(base).StartDevice(context.Background())
				return err
			},
		},
		{
			name:     "dev-token: a raw control byte elsewhere in the body",
			body:     "{\"token\":\"" + credSentinel + "\",\"note\":\"a\x01b\"}",
			wantText: "unexpected /api/v1/blocks/dev-token response",
			call: func(base string) error {
				_, err := New(base, "tok", "").MintDevToken(context.Background(), "my-block", nil, nil, false)
				return err
			},
		},
		{
			name:     "dev-token: decodes, but the token is under another key",
			body:     `{"devToken":"` + credSentinel + `"}`,
			wantText: "dev-token response had no token",
			call: func(base string) error {
				_, err := New(base, "tok", "").MintDevToken(context.Background(), "my-block", nil, nil, false)
				return err
			},
		},
		{
			name: "forgejo clone info: string notYetAvailable beside the push token",
			body: `{"result":{"data":{"json":{"notYetAvailable":"false","slug":"my-block",` +
				`"token":"` + credSentinel + `","cloneUrl":"https://dev-7:` + credSentinel + `@forgejo.example.com/apps/my-block.git"}}}}`,
			wantText: "unexpected getMyForgejoCloneInfo response",
			call: func(base string) error {
				_, err := New(base, "tok", "").GetForgejoCloneInfo(context.Background(), "my-block")
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// PREMISE: the body really carries the credential, or "not echoed"
			// would hold vacuously.
			if !strings.Contains(tc.body, credSentinel) {
				t.Fatalf("PREMISE BROKEN: the fixture body carries no credential")
			}
			var base string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/openid-configuration" {
					writeDiscovery(w, base)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			base = srv.URL

			err := tc.call(srv.URL)
			if err == nil {
				t.Fatal("an undecodable credential body returned no error — the failure was swallowed")
			}
			msg := err.Error()
			if strings.Contains(msg, credSentinel) {
				t.Errorf("the error text carries the credential from the body:\n  %s", msg)
			}
			if !strings.Contains(msg, tc.wantText) {
				t.Errorf("the error no longer names the route that failed (want %q):\n  %s", tc.wantText, msg)
			}
		})
	}
}
