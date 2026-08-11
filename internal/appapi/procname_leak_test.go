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
// 🔴 This table covers EVERY route, and its phrases are spelled out per case
// rather than looked up from the op. That is the whole point of it. The
// per-route reachability table in listing_op_test.go checks its expectation
// against a `wantOp` sitting three lines from `route.op`, so editing BOTH
// re-labels a route under a green suite — measured: six of the seven `change`
// routes survived exactly that two-line edit, and only `setIcon` died, because
// `setIcon` was the only one this table used to name. Every route now has a
// hand-written sentence here, in a different file, that a re-label must
// contradict.
//
// What this table does NOT prove is that any route REACHES its arm through the
// real client — that is listing_op_test.go's job, and civitai/cli#374 is what
// happens when only one of the two halves exists.
func TestListingBadRequestSubjectFollowsTheOperation(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": "Invalid input"}},
	})
	// A lookup: it read, so it changed nothing.
	const readSubject = "store-listing lookup"
	// An image ingest: it created an Image row attached to no listing.
	const ingestSubject = "image-upload request"
	// A listing write: it may have PARTIALLY APPLIED, so `app listing status` is
	// the authority on what is attached now.
	const changeSubject = "store-listing change"
	// 🔴 The change arm's REMEDY, spelled here so that a read or an ingest
	// cannot borrow it. It used to be "fix the value and retry", which
	// civitai/cli#391 measured FALSE for `beginListingRevision`: that request
	// carries only a listing id the CLI minted from a lookup that had already
	// returned 200, so its author has no value to fix. This phrase presumes
	// nothing about what was sent, and states the one thing every change route
	// has in common.
	const changeRemedy = "may have partially applied"

	for _, tc := range []struct {
		name    string
		route   listingRoute
		want    []string
		notWant []string
	}{
		// ---- reads: the caller asked to look ----
		{
			name:    "getMyListingForApp is a lookup",
			route:   trpcGetMyListingForApp,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy},
		},
		{
			name:    "getMyListingForEdit is a lookup",
			route:   trpcGetMyListingForEdit,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy},
		},
		{
			name:    "getAssetScanStatuses is a lookup",
			route:   trpcGetAssetScanStatuses,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy},
		},

		// ---- ingests: an Image row, attached to nothing ----
		{
			name:    "the presigned mint changes no listing",
			route:   imageUploadRoute,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy},
		},
		{
			// A tRPC POST that creates an Image row and attaches it to nothing.
			// Same story as the mint above (#374).
			name:    "the data-uri ingest changes no listing",
			route:   trpcIngestAssetFromDataURI,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy},
		},
		{
			// Step 3 of the same user action the mint starts.
			name:    "persistAssetImage changes no listing",
			route:   trpcPersistAssetImage,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy},
		},

		// ---- changes: each writes the listing, so none of them may say
		// "nothing was changed" — a 400 here may have partially applied ----
		{
			name:    "setIcon attaches an icon to the listing",
			route:   trpcSetIcon,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "setCover attaches a cover to the listing",
			route:   trpcSetCover,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "addScreenshot appends to the listing",
			route:   trpcAddScreenshot,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "removeScreenshot deletes from the listing",
			route:   trpcRemoveScreenshot,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "reorderScreenshots rewrites the listing's order",
			route:   trpcReorderScreenshots,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "beginListingRevision opens a shadow revision",
			route:   trpcBeginListingRevision,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
		},
		{
			name:    "submitListingRevision sends the revision to review",
			route:   trpcSubmitListingRevision,
			want:    []string{changeSubject, changeRemedy, "civitai app listing status"},
			notWant: []string{"nothing was changed", readSubject, ingestSubject},
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
