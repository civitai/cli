package appapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestImageUploadMintClassifiesOnTheServerWords pins that the image-upload
// mint's 403 arms classify on the server's UNFILTERED message
// (redact.ErrorMessage), while showing only the filtered text. Each message
// carries a URL, which the display filter withholds — so a classifier fed the
// filtered text sees only a byte count and every row falls to the generic
// "needs Apps-author access" arm.
func TestImageUploadMintClassifiesOnTheServerWords(t *testing.T) {
	cases := []struct {
		name, message, want string
	}{
		{"scope", "Your API key does not have the required scope for this action — see https://civitai.com/docs/scopes",
			"listing media needs the Apps submit scope (403)"},
		{"takedown", "This listing was removed by a moderator — see https://civitai.com/content/rules",
			"this listing is under a moderator takedown (403)"},
		{"not owned", "You can only change your own listings — see https://civitai.com/user/account",
			"this listing belongs to another account (403)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"json":{"message":"` + tc.message + `","code":-32603}}}`))
			}))
			defer srv.Close()

			_, _, err := New(srv.URL, "tok", "").MintImageUpload(context.Background())
			if err == nil {
				t.Fatal("a 403 returned no error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Errorf("the 403 was not classified on the server's words (want %q):\n  %s", tc.want, msg)
			}
			if strings.Contains(msg, "://") {
				t.Errorf("the filtered display text carries the URL:\n  %s", msg)
			}
		})
	}
}
