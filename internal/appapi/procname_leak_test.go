package appapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestListingBadRequestDoesNotLeakTheProcedureName is civitai/cli#363 §3: a
// rejected icon surfaced as `appListings.setIcon rejected the request (400)`,
// which reads as the CLI calling something that does not exist rather than the
// server refusing the value the user supplied.
func TestListingBadRequestDoesNotLeakTheProcedureName(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": "This listing is live"}},
	})
	err := listingError(http.StatusBadRequest, body, trpcSetIcon)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()

	// Structural, not spelled: assert the tRPC METHOD NAME the CLI called is
	// absent, whatever it happens to be, rather than one hard-coded spelling.
	if name := trpcName(trpcSetIcon); strings.Contains(msg, name) {
		t.Errorf("internal procedure name %q leaked into a 400: %s", name, msg)
	}
	if !strings.Contains(msg, "This listing is live") {
		t.Errorf("the server's own reason must survive; got: %s", msg)
	}
	if !strings.Contains(msg, "civitai app listing status") {
		t.Errorf("house style: name the next command; got: %s", msg)
	}
}

// TestListingUnexpectedStatusKeepsTheProcedureName is the negative control. An
// unexpected status is exactly where the method name earns its place — a
// blanket strip would have removed the only clue in a bug report.
func TestListingUnexpectedStatusKeepsTheProcedureName(t *testing.T) {
	err := listingError(http.StatusTeapot, []byte(`{}`), trpcSetIcon)
	if err == nil {
		t.Fatal("expected an error")
	}
	if name := trpcName(trpcSetIcon); !strings.Contains(err.Error(), name) {
		t.Errorf("an unexpected status must still report which call failed (%q); got: %s", name, err.Error())
	}
}
