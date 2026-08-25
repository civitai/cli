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
// listingWordingRow is one route's hand-written sentence.
type listingWordingRow struct {
	name    string
	route   listingRoute
	want    []string
	notWant []string
}

// A lookup: it read, so it changed nothing.
const readSubject = "store-listing lookup"

// An image ingest: it created an Image row attached to no listing.
const ingestSubject = "image-upload request"

// A listing write: it may have PARTIALLY APPLIED, so `app listing status` is
// the authority on what is attached now.
const changeSubject = "store-listing change"

// 🔴 The change arm's REMEDY, spelled here so that a read or an ingest cannot
// borrow it. It used to be "fix the value and retry", which civitai/cli#391
// measured FALSE for `beginListingRevision`: that request carries only a listing
// id the CLI minted from a lookup that had already returned 200, so its author
// has no value to fix. This phrase presumes nothing about what was sent, and
// states the one thing every change route has in common.
const changeRemedy = "may have partially applied"

// 🔴 The READ arm's retired clause, kept as a PROHIBITION rather than deleted
// with it — the same treatment valueBlame gets above, and for the same reason.
// "check the app you named" was true of the three reads that carry a slug or an
// id, and became false the moment a fourth read (appListings.listMine, the
// `app doctor` enumeration) reached the same arm carrying no input at all. One
// arm answering for N routes may only claim what is true of all N. Deleting the
// phrase without banning it would retire the lesson silently, and the next
// author to want a friendlier read message would write it straight back.
const namedValueBlame = "check the app you named"

// listingWordingRows is the third guard's table, lifted out of its test so the
// set of routes it covers can itself be checked. See
// TestEveryListingRouteHasAWordingRow: without that, the table was enforced only
// in the DELETE direction — a route ADDED with no row here was green, which
// quietly made the "three independent guards" claim two for any new route.
func listingWordingRows() []listingWordingRow {
	return []listingWordingRow{
		// ---- reads: the caller asked to look ----
		{
			name:    "getMyListingForApp is a lookup",
			route:   trpcGetMyListingForApp,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy, valueBlame},
		},
		{
			name:    "getMyListingForEdit is a lookup",
			route:   trpcGetMyListingForEdit,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy, valueBlame},
		},
		{
			name:    "getAssetScanStatuses is a lookup",
			route:   trpcGetAssetScanStatuses,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy, valueBlame},
		},
		{
			// 🔴 Written from what listMine DOES, not copied from the row above
			// it — and the difference is the whole reason this row was worth
			// writing by hand. listMine sends NO input: no slug, no listing id,
			// no image id. So beyond the shared "nothing was changed" it may not
			// blame a value, and it may not tell the caller to check something
			// they named, because they named nothing. `namedValueBlame` below is
			// in notWant for exactly that: it is the clause this route's arrival
			// made false, and it stays banned so it cannot drift back.
			name:    "listMine enumerates and blames no value the caller sent",
			route:   trpcListMine,
			want:    []string{readSubject, "nothing was changed"},
			notWant: []string{changeSubject, ingestSubject, changeRemedy, valueBlame, namedValueBlame},
		},

		// ---- ingests: an Image row, attached to nothing ----
		{
			name:    "the presigned mint changes no listing",
			route:   imageUploadRoute,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy, valueBlame},
		},
		{
			// A tRPC POST that creates an Image row and attaches it to nothing.
			// Same story as the mint above (#374).
			name:    "the data-uri ingest changes no listing",
			route:   trpcIngestAssetFromDataURI,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy, valueBlame},
		},
		{
			// Step 3 of the same user action the mint starts.
			name:    "persistAssetImage changes no listing",
			route:   trpcPersistAssetImage,
			want:    []string{ingestSubject, "no listing was changed"},
			notWant: []string{changeSubject, readSubject, changeRemedy, valueBlame},
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
	}
}

func TestListingBadRequestSubjectFollowsTheOperation(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": "Invalid input"}},
	})
	for _, tc := range listingWordingRows() {
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

// TestEveryListingRouteHasAWordingRow closes the ADD direction of this table.
//
// 🔴 Measured gap (civitai/cli#391 audit): the cry-wolf control that added a
// legitimate 14th route with every other edit the guards demand, but NO row
// here, exited 0. So this table was enforced only when a row was DELETED — a
// route added without a sentence was silently uncovered, which reduced "three
// independent guards that must agree" to two for exactly the routes most likely
// to be misclassified: the new ones.
//
// It compares against the SOURCE-TEXT ledger rather than against
// listingRouteCases(), so this stays a genuinely separate opinion: the case
// table and this table can now only agree by both being right about the set of
// routes that exists.
func TestEveryListingRouteHasAWordingRow(t *testing.T) {
	declared := declaredListingRoutes(t)
	if len(declared) == 0 {
		t.Fatalf("the sweep found no routes at all — this comparison would be vacuous")
	}

	covered := map[string]string{}
	for _, row := range listingWordingRows() {
		if prev, dup := covered[row.route.path]; dup {
			t.Errorf("route %q has two rows in this table (%q and %q) — one sentence per route, "+
				"or a re-label only has to contradict whichever row is read first", row.route.path, prev, row.name)
		}
		covered[row.route.path] = row.name
	}
	for path, d := range declared {
		if _, ok := covered[path]; !ok {
			t.Errorf("route %q (declared at %s) has no wording row in listingWordingRows().\n"+
				"Write the sentence its 400 must produce, by hand, from what the route DOES — not by copying "+
				"a neighbour and not from route.op. This table is the third of the three guards that must "+
				"agree, and it is worth nothing for a route it does not mention.", path, d.file)
		}
	}
	for path, name := range covered {
		if _, ok := declared[path]; !ok {
			t.Errorf("this table has a row %q for route %q, which is not declared anywhere — "+
				"delete the row if the route was retired, or fix the path it names", name, path)
		}
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
