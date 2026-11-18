package appapi

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestDevTokenErrorClassifiesBeforeTheDisplayFilter pins that the dev-token 404
// is told apart on the server's OWN wording, not on what redact lets the error
// print. The "no live deployment" row carries a newline, which ServerText
// refuses whatever the message's length, so the shown text is a size-only
// placeholder: if classification read the shown text it would miss the phrase
// and wrap ErrSlugRegisteredToOtherAccount — the sentinel that renames the app.
// The bare "App not found" row is the control: it must still wrap the sentinel.
func TestDevTokenErrorClassifiesBeforeTheDisplayFilter(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantRename bool
	}{
		{"no live deployment, withheld from display", `{"message":"block 'b1' has no live deployment\nsecond line"}`, false},
		{"bare collision 404", `{"message":"App not found"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := devTokenError(http.StatusNotFound, []byte(tc.body))
			if got := errors.Is(err, ErrSlugRegisteredToOtherAccount); got != tc.wantRename {
				t.Errorf("errors.Is(err, ErrSlugRegisteredToOtherAccount) = %v, want %v: %v", got, tc.wantRename, err)
			}
			if strings.Contains(err.Error(), "second line") {
				t.Errorf("a message ServerText refuses was shown: %v", err)
			}
		})
	}
}
