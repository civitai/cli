package civitai

import (
	"errors"
	"testing"
)

// TestDecodeBodyTagsAnUnreadableBodyOnly pins ErrUnreadableResponse to the ONE
// failure it names — a 2xx body that does not decode even after the control-
// character repair — with the message unchanged, and never to a body that
// decodes (directly, or after the repair).
func TestDecodeBodyTagsAnUnreadableBodyOnly(t *testing.T) {
	var out struct {
		Name string `json:"name"`
	}
	_, err := decodeBody("/api/v1/x", 200, []byte("<html>edge page</html>"), &out)
	if err == nil {
		t.Fatal("CONTROL failure: an HTML body decoded")
	}
	if !errors.Is(err, ErrUnreadableResponse) {
		t.Errorf("an undecodable 2xx body is not tagged ErrUnreadableResponse: %v", err)
	}
	if want := "unexpected response from /api/v1/x (status 200): <html>edge page</html>"; err.Error() != want {
		t.Errorf("the tag changed the message:\n got %q\nwant %q", err.Error(), want)
	}
	for _, kind := range []error{ErrUnauthorized, ErrNotFound, ErrBadRequest, ErrRateLimited, ErrNetwork} {
		if errors.Is(err, kind) {
			t.Errorf("an unreadable body also carries %v — it would move the exit code", kind)
		}
	}

	for name, body := range map[string]string{
		"valid":                 `{"name":"ok"}`,
		"repaired control char": "{\"name\":\"a\tb\"}",
	} {
		if _, err := decodeBody("/api/v1/x", 200, []byte(body), &out); err != nil {
			t.Errorf("%s body: %v", name, err)
		}
	}
}
