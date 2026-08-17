package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// civitai/cli#430 — THE TWO ID SPACES.
//
// `app listing status` renders getMyListingForEdit, which for an APPROVED parent
// resolves the media through an idempotently-opened SHADOW revision (see
// appapi.GetMyListingForEdit). So the `apls_…` screenshot ids a user can SEE
// belong to the shadow. `reorder` addressed its mutation to the PARENT listing
// id, and the server's set-equality check compares the ids it was handed against
// the ids of the listing it was ADDRESSED TO — so every reorder of a live listing
// was refused with `orderedIds must be exactly the listing's current screenshot
// ids`, naming ids the CLI itself had just printed. Measured on the live platform
// in #430: same ids, same order, seconds apart — parent id → HTTP 400, the
// shadow → HTTP 200.
//
// 🔴 THE ASSERTION HAS TO BE *WHICH LISTING ID THE REORDER WAS ADDRESSED TO*, and
// that is the whole reason this bug shipped. A fake server that accepts any
// `listingId` — which is what a "does it exit 0" test builds — is green with the
// defect fully intact, because the request the CLI sent is perfectly well-formed.
// Only the real server has both id spaces to disagree about, so the disagreement
// has to be modelled here: the stub hands back a shadow id that is NOT the parent
// id, and the tests assert the reorder carried the former and never the latter.
//
// The fixture ids are pairwise distinct AND distinct from every constant these
// assertions name, so a mutant that hardcodes one of them cannot survive by
// landing on a value the fixture could only ever produce.
const (
	reorderParentID = "apl_parent_001"
	reorderShadowID = "apl_shadow_777"
	reorderPubReqID = "alpr_pub_042"
)

// reorderNewOrder is the order the user asks for: all three current ids, rotated.
var reorderNewOrder = []string{"apls_c", "apls_a", "apls_b"}

// reorderStub is a listing server that records WHICH listing id every listing
// mutation was addressed to.
type reorderStub struct {
	t *testing.T
	// status is the parent listing's lifecycle state (draft|pending|approved).
	status string
	// submitStatus is the HTTP status submitListingRevision answers with (0 = 200).
	submitStatus int
	// submitMessage is the tRPC error message that accompanies a non-200 submit.
	submitMessage string
	// viewIcon / viewCover are the image ids the post-submit re-read reports in
	// each floor slot; 0 leaves the slot empty.
	viewIcon, viewCover int
	// viewShots is the screenshot order the post-submit re-read reports.
	viewShots []string

	mu               sync.Mutex
	calls            []string
	reorderListingID string
	reorderIDs       []string
	beginListingID   string
	submitShadowID   string
	submitChangelog  string
}

func (s *reorderStub) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	listingEnv(t, srv.URL)
}

// input decodes a tRPC mutation body's `json` payload.
func (s *reorderStub) input(r *http.Request) map[string]any {
	raw, err := readBodyJSON(r)
	if err != nil {
		s.t.Errorf("decoding the body of %s: %v", r.URL.Path, err)
		return map[string]any{}
	}
	in, _ := raw["json"].(map[string]any)
	if in == nil {
		s.t.Errorf("%s carried no `json` payload: %v", r.URL.Path, raw)
		return map[string]any{}
	}
	return in
}

func (s *reorderStub) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/v1/blocks/submissions"):
		s.calls = append(s.calls, "submissions")
		submissionRow(w, "my-app", "block_1")
	case strings.Contains(p, "getMyListingForApp"):
		s.calls = append(s.calls, "getMyListingForApp")
		trpcData(w, map[string]any{"appListingId": reorderParentID, "status": s.status, "contentRating": "g"})
	case strings.Contains(p, "beginListingRevision"):
		s.calls = append(s.calls, "beginListingRevision")
		s.beginListingID, _ = s.input(r)["listingId"].(string)
		trpcData(w, map[string]any{"shadowId": reorderShadowID, "created": true})
	case strings.Contains(p, "reorderScreenshots"):
		s.calls = append(s.calls, "reorderScreenshots")
		in := s.input(r)
		s.reorderListingID, _ = in["listingId"].(string)
		s.reorderIDs = nil
		if list, ok := in["orderedIds"].([]any); ok {
			for _, v := range list {
				id, _ := v.(string)
				s.reorderIDs = append(s.reorderIDs, id)
			}
		}
		trpcData(w, map[string]any{})
	case strings.Contains(p, "submitListingRevision"):
		s.calls = append(s.calls, "submitListingRevision")
		in := s.input(r)
		s.submitShadowID, _ = in["shadowId"].(string)
		s.submitChangelog, _ = in["changelog"].(string)
		if s.submitStatus != 0 {
			w.WriteHeader(s.submitStatus)
			_, _ = w.Write([]byte(`{"error":{"json":{"message":"` + s.submitMessage + `"}}}`))
			return
		}
		trpcData(w, map[string]any{"publishRequestId": reorderPubReqID, "shadowId": reorderShadowID, "slug": "my-app"})
	case strings.Contains(p, "getMyListingForEdit"):
		s.calls = append(s.calls, "getMyListingForEdit")
		shots := []any{}
		for i, id := range s.viewShots {
			shots = append(shots, map[string]any{"id": id, "imageId": 900 + i, "order": i})
		}
		trpcData(w, map[string]any{
			"parentId": reorderParentID, "slug": "my-app", "status": s.status,
			"assets": map[string]any{
				"icon":        reorderSlot(s.viewIcon),
				"cover":       reorderSlot(s.viewCover),
				"screenshots": shots,
			},
		})
	default:
		s.t.Errorf("unexpected path %s", p)
	}
}

func reorderSlot(imageID int) map[string]any {
	if imageID == 0 {
		return map[string]any{}
	}
	return map[string]any{"imageId": imageID}
}

func (s *reorderStub) called(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == name {
			n++
		}
	}
	return n
}

func (s *reorderStub) addressedTo() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reorderListingID
}

func (s *reorderStub) orderSent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.reorderIDs...)
}

// runReorderCmd drives the real command tree.
func runReorderCmd(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	args := append([]string{"app", "listing", "reorder"}, reorderNewOrder...)
	args = append(args, "--slug", "my-app")
	out, _, err := run(t, append(args, extra...)...)
	return out, err
}

// TestAppListingReorderOnLiveListingTargetsTheShadowRevision is the #430
// regression guard. It is RED at b1213a3 on the id assertion below, with the
// parent id in place of the shadow.
func TestAppListingReorderOnLiveListingTargetsTheShadowRevision(t *testing.T) {
	st := &reorderStub{t: t, status: "approved", viewIcon: 11, viewCover: 22, viewShots: reorderNewOrder}
	st.start(t)

	out, err := runReorderCmd(t)
	if err != nil {
		t.Fatalf("reorder on a live listing: %v", err)
	}

	// 🔴 THE LOAD-BEARING ASSERTION. Not "it exited 0" — the buggy CLI does too,
	// against any stub that does not model the second id space.
	if got := st.addressedTo(); got != reorderShadowID {
		t.Fatalf("reorderScreenshots was addressed to listingId %q, want the SHADOW revision %q.\n"+
			"civitai/cli#430: `app listing status` prints the SHADOW's screenshot ids, so a reorder "+
			"addressed to the parent is validated against a different id set and the server refuses it "+
			"with `orderedIds must be exactly the listing's current screenshot ids` — naming ids the CLI "+
			"itself just printed.", got, reorderShadowID)
	}
	if got := st.addressedTo(); got == reorderParentID {
		t.Fatalf("reorderScreenshots was addressed to the PARENT listing id %q — that is the bug", got)
	}
	if got := st.called("beginListingRevision"); got != 1 {
		t.Errorf("beginListingRevision ran %d time(s), want 1 — the shadow id has to come from somewhere", got)
	}
	if got := st.beginListingID; got != reorderParentID {
		t.Errorf("beginListingRevision was addressed to %q, want the PARENT %q", got, reorderParentID)
	}
	// The ids the user typed reach the server unchanged and in order — a fix that
	// retargets the mutation but reorders or drops the payload is a different bug.
	if got := st.orderSent(); !equalStrings(got, reorderNewOrder) {
		t.Errorf("orderedIds = %v, want %v (as typed, in order)", got, reorderNewOrder)
	}

	// The revision is SUBMITTED. Without this, the reorder lands in a shadow no
	// CLI command can publish and the ✓ is false of the public listing.
	if got := st.called("submitListingRevision"); got != 1 {
		t.Fatalf("submitListingRevision ran %d time(s), want 1 — a reorder staged on a revision the CLI "+
			"cannot submit is a change the public gallery never sees", got)
	}
	if got := st.submitShadowID; got != reorderShadowID {
		t.Errorf("submitListingRevision was addressed to %q, want the shadow %q", got, reorderShadowID)
	}
	if st.submitChangelog == "" {
		t.Errorf("submitListingRevision carried no changelog — the moderator gets no description")
	}

	for _, want := range []string{reorderPubReqID, "moderator review", "Your live listing is unchanged"} {
		if !strings.Contains(out, want) {
			t.Errorf("live reorder output does not say %q:\n%s", want, out)
		}
	}
	// 🔴 And it must NOT print the draft arm's bare success. On a live listing the
	// public gallery is unchanged until a moderator approves, so `✓ Reordered 3
	// screenshots` would be true of the revision and false of the listing.
	if strings.Contains(out, "Reordered") {
		t.Errorf("live reorder printed the draft arm's bare success line — the public gallery is "+
			"unchanged until the revision is approved:\n%s", out)
	}
}

// TestAppListingReorderOnUnapprovedListingTargetsTheParent pins the other arm.
//
// #430 called draft/pending "unaffected" from the code path and said so
// explicitly: INFERRED, never measured — the reporter had no draft listing to
// test against. This is what turns that into a contract. With no approved parent
// there IS no shadow, so the parent id is the edit target and opening a revision
// would be both wrong and a new round-trip.
func TestAppListingReorderOnUnapprovedListingTargetsTheParent(t *testing.T) {
	for _, status := range []string{"draft", "pending"} {
		t.Run(status, func(t *testing.T) {
			st := &reorderStub{t: t, status: status}
			st.start(t)

			out, err := runReorderCmd(t)
			if err != nil {
				t.Fatalf("reorder on a %s listing: %v", status, err)
			}
			if got := st.addressedTo(); got != reorderParentID {
				t.Errorf("reorderScreenshots was addressed to %q, want the parent %q — a %s listing "+
					"has no shadow revision, so the parent IS the edit target", got, reorderParentID, status)
			}
			if got := st.addressedTo(); got == reorderShadowID {
				t.Errorf("reorderScreenshots was addressed to a shadow id on a %s listing", status)
			}
			if got := st.called("beginListingRevision"); got != 0 {
				t.Errorf("beginListingRevision ran %d time(s) on a %s listing, want 0", got, status)
			}
			if got := st.called("submitListingRevision"); got != 0 {
				t.Errorf("submitListingRevision ran %d time(s) on a %s listing, want 0 — there is no "+
					"revision to submit and nothing goes to a moderator", got, status)
			}
			if got := st.orderSent(); !equalStrings(got, reorderNewOrder) {
				t.Errorf("orderedIds = %v, want %v", got, reorderNewOrder)
			}
			if !strings.Contains(out, "Reordered 3 screenshots") {
				t.Errorf("a %s listing is edited directly, so the plain success line is the honest "+
					"one:\n%s", status, out)
			}
			if strings.Contains(out, "moderator") {
				t.Errorf("a %s listing reorder does not reach a moderator:\n%s", status, out)
			}
		})
	}
}

// TestAppListingReorderBelowFloorOnLiveListing extends civitai/cli#400's rule to
// the reorder path, and pins the IDENTITY question it turns on.
//
// A live listing still below the publish floor cannot have a revision SUBMITTED,
// so the submit 400s. That is not a failure of the reorder, which landed in the
// shadow — exactly #400's shape, one command's success reported as a failure for
// the next command not having run. But the discriminator has to be that the
// reorder REALLY LANDED, by id and in the requested order: "the revision exists"
// is satisfied by a shadow whose gallery is untouched.
func TestAppListingReorderBelowFloorOnLiveListing(t *testing.T) {
	for _, tc := range []struct {
		name      string
		viewShots []string
		wantErr   bool
		wantOut   string
	}{
		{
			name:      "the order landed — staged, exit 0",
			viewShots: reorderNewOrder,
			wantOut:   "staged on an open revision",
		},
		{
			name: "the order did NOT land — the submit error is all the user has",
			// The shadow still holds the ORIGINAL order, so nothing this command
			// asked for is in it and there is nothing to report as progress.
			viewShots: []string{"apls_a", "apls_b", "apls_c"},
			wantErr:   true,
		},
		{
			// 🔴 A PREFIX IS NOT A MATCH, and this row is here because the guard
			// that says so SURVIVED without it: with only the per-position
			// comparison, a revision holding the first two ids in the right
			// places iterates over what it has, matches every one of them, and
			// reports the whole reorder as landed. (It is also what turns a
			// SHORTER list into an index panic on a longer one.)
			name:      "the revision holds only part of the order",
			viewShots: []string{"apls_c", "apls_a"},
			wantErr:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &reorderStub{
				t: t, status: "approved",
				submitStatus:  http.StatusBadRequest,
				submitMessage: "listing needs a cover before it can be submitted",
				viewIcon:      11, // cover left empty: the floor is unmet
				viewShots:     tc.viewShots,
			}
			st.start(t)

			out, err := runReorderCmd(t)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected the submit 400 to be the command's error; output was:\n%s", out)
				}
				if strings.Contains(out, "staged on an open revision") {
					t.Errorf("reported progress for an order that is not in the revision:\n%s", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("a below-floor submit refusal is not a failure of the reorder: %v", err)
			}
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("output does not say %q:\n%s", tc.wantOut, out)
			}
			if strings.Contains(out, "pending moderator review") {
				t.Errorf("nothing was submitted, so nothing is pending review:\n%s", out)
			}
		})
	}
}
