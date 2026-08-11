package appapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if name := trpcSetIcon.name(); strings.Contains(msg, name) {
		t.Errorf("internal procedure name %q leaked into a 400: %s", name, msg)
	}
	if !strings.Contains(msg, "This listing is live") {
		t.Errorf("the server's own reason must survive; got: %s", msg)
	}
	if !strings.Contains(msg, "civitai app listing status") {
		t.Errorf("house style: name the next command; got: %s", msg)
	}
}

// TestListingBadRequestSubjectFollowsTheOperation: de-leaking the proc name from
// the 400 arm replaced it with a subject scoped to a STATUS, not to what the
// caller was doing — and listingError serves reads (trpcQuery), changes
// (trpcMutation) and the presigned-upload mint alike. "The server rejected this
// store-listing change" is then false twice over: a read changed nothing, and an
// upload-URL mint touches no listing.
//
// This is the WORDING of each arm, held against a route chosen by hand. It says
// nothing about which routes reach which arm — that is
// TestListingBadRequestSubjectIsReachablePerRoute (listing_op_test.go), and
// civitai/cli#374 is what happens when only this half exists.
func TestListingBadRequestSubjectFollowsTheOperation(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": "Invalid input"}},
	})
	for _, tc := range []struct {
		name    string
		route   listingRoute
		want    []string
		notWant []string
	}{
		{
			name:    "read",
			route:   trpcGetMyListingForApp,
			want:    []string{"lookup", "nothing was changed"},
			notWant: []string{"store-listing change", "fix the value"},
		},
		{
			name:    "upload mint",
			route:   imageUploadRoute,
			want:    []string{"image-upload", "no listing was changed"},
			notWant: []string{"store-listing change"},
		},
		{
			// The other ingest shape: a tRPC POST that creates an Image row and
			// attaches it to nothing. Same story as the mint above (#374).
			name:    "data-uri ingest",
			route:   trpcIngestAssetFromDataURI,
			want:    []string{"image-upload", "no listing was changed"},
			notWant: []string{"store-listing change", "fix the value"},
		},
		{
			// The control: a real mutation IS a store-listing change, and the
			// wording civitai/cli#363 asked for must survive for it.
			name:    "change",
			route:   trpcSetIcon,
			want:    []string{"store-listing change", "civitai app listing status"},
			notWant: []string{"nothing was changed"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := listingError(http.StatusBadRequest, body, tc.route)
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("missing %q; got: %s", w, msg)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(msg, w) {
					t.Errorf("must not claim %q; got: %s", w, msg)
				}
			}
			// Whatever the subject, no tRPC method name reaches a 400 and the
			// server's own reason survives. (Scoped to the tRPC paths: the
			// upload route is a plain REST path whose last segment IS the
			// user-facing name of the operation, not an internal proc.)
			if name := tc.route.name(); strings.HasPrefix(tc.route.path, "/api/trpc/") && strings.Contains(msg, name) {
				t.Errorf("internal procedure name %q leaked: %s", name, msg)
			}
			if !strings.Contains(msg, "Invalid input") {
				t.Errorf("the server's own reason must survive; got: %s", msg)
			}
		})
	}
}

// TestListingReadBadRequestIsReachableAsARead proves the read subject is not
// dead code: a 400 on the ENTRY read (`civitai app listing status` →
// appListings.getMyListingForApp, a trpcQuery) must arrive worded as a lookup.
// Without this, listingOpRead could be passed nowhere and every test above would
// still pass. Kept alongside listing_op_test.go's per-route table, which
// generalises it to every route the client can call.
func TestListingReadBadRequestIsReachableAsARead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"json":{"message":"Invalid input"}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetMyListingForApp(context.Background(), "", "my-block")
	if err == nil {
		t.Fatal("expected a 400 error")
	}
	if strings.Contains(err.Error(), "store-listing change") {
		t.Errorf("a READ reported as a change that never happened: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "lookup") {
		t.Errorf("the read arm must be what answers a query 400; got: %s", err.Error())
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
	if name := trpcSetIcon.name(); !strings.Contains(err.Error(), name) {
		t.Errorf("an unexpected status must still report which call failed (%q); got: %s", name, err.Error())
	}
}
