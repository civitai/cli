package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The civitai.red-only icon / cover override: `set-icon|set-cover --red` and
// `clear-red`. Every case drives the real command tree against a fake server
// and asserts what reached the WIRE (proc, slot, listing, image) — the output
// line alone could be printed by a command that wrote the wrong slot.
//
// Ids are pairwise distinct and distinct from every constant the code names:
// listing `listing_7`, shadow `shadow_8`, ingested images 7101 (cover) / 7202
// (icon), a pre-existing listing icon 7303.

// redCall is one setRedMedia request as the fake server saw it.
type redCall struct {
	ListingID string
	Slot      string
	ImageID   any // float64 from JSON, or nil for an explicit null
	HasImage  bool
}

type redFake struct {
	mu        sync.Mutex
	red       []redCall
	ownAttach []string // setIcon / setCover — must stay empty under --red
	submitted bool
	// status of the listing getMyListingForApp reports.
	status string
	// submit400 makes submitListingRevision refuse (the below-floor path).
	submit400 bool
	// editRedIcon is the image id getMyListingForEdit reports in redMedia.icon.
	editRedIcon any
}

func (f *redFake) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_7", "status": f.status, "contentRating": "g"})
		// Ingest: the inline data-URI path (icon) and the mint/put/persist path
		// (cover), each minting its own image id.
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 7202})
		case strings.Contains(r.URL.Path, "persistAssetImage"):
			trpcData(w, map[string]any{"imageId": 7101})
		case strings.Contains(r.URL.Path, "image-upload"):
			// REST, not tRPC — not wrapped in result.data.json.
			fmt.Fprintf(w, `{"id":"key","uploadURL":"http://%s/put/key"}`, r.Host)
		case strings.HasPrefix(r.URL.Path, "/put/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			trpcData(w, map[string]any{"statuses": []map[string]any{
				{"imageId": 7101, "status": "scanned"}, {"imageId": 7202, "status": "scanned"}}})
		case strings.Contains(r.URL.Path, "beginListingRevision"):
			trpcData(w, map[string]any{"shadowId": "shadow_8", "created": true})
		case strings.Contains(r.URL.Path, "setRedMedia"):
			raw, _ := readBodyJSON(r)
			j, _ := raw["json"].(map[string]any)
			img, has := j["imageId"]
			call := redCall{ListingID: str(j["listingId"]), Slot: str(j["slot"]), ImageID: img, HasImage: has}
			f.red = append(f.red, call)
			if img == nil {
				trpcData(w, map[string]any{"status": "cleared", "slot": call.Slot})
			} else {
				trpcData(w, map[string]any{"status": "attached", "slot": call.Slot, "imageId": img})
			}
		case strings.Contains(r.URL.Path, "setIcon"), strings.Contains(r.URL.Path, "setCover"):
			f.ownAttach = append(f.ownAttach, r.URL.Path)
			trpcData(w, map[string]any{"status": "attached"})
		case strings.Contains(r.URL.Path, "submitListingRevision"):
			f.submitted = true
			if f.submit400 {
				trpcBadRequest(w, "Listing is missing a cover")
				return
			}
			trpcData(w, map[string]any{"publishRequestId": "pubreq_8", "shadowId": "shadow_8", "slug": "my-app"})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{"parentId": "listing_7", "status": f.status,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 7303},
					"cover":       map[string]any{"imageId": nil},
					"screenshots": []any{},
				},
				"redMedia": map[string]any{
					"icon":  map[string]any{"imageId": f.editRedIcon},
					"cover": map[string]any{"imageId": nil},
				}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func TestSetIconRedWritesTheRedSlotNotTheListingIcon(t *testing.T) {
	fastScanPoll(t)
	f := &redFake{status: "draft"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app", "--red")
	if err != nil {
		t.Fatalf("set-icon --red: %v", err)
	}
	if len(f.ownAttach) != 0 {
		t.Fatalf("--red must not touch the listing's own icon; saw %v", f.ownAttach)
	}
	if len(f.red) != 1 {
		t.Fatalf("expected exactly one setRedMedia call, got %d: %+v", len(f.red), f.red)
	}
	got := f.red[0]
	if got.ListingID != "listing_7" || got.Slot != "icon" || got.ImageID != float64(7202) {
		t.Errorf("setRedMedia = %+v, want listing_7 / icon / 7202", got)
	}
	if !strings.Contains(out, "civitai.red icon set") {
		t.Errorf("expected the civitai.red success line: %q", out)
	}
}

func TestSetCoverRedOnALiveListingAttachesToTheRevisionAndSubmits(t *testing.T) {
	fastScanPoll(t)
	f := &redFake{status: "approved"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-cover", writePNG(t, 1600, 900), "--slug", "my-app", "--red", "--changelog", "red hero")
	if err != nil {
		t.Fatalf("set-cover --red live: %v", err)
	}
	if len(f.red) != 1 || f.red[0].ListingID != "shadow_8" || f.red[0].Slot != "cover" || f.red[0].ImageID != float64(7101) {
		t.Fatalf("setRedMedia = %+v, want one call: shadow_8 / cover / 7101", f.red)
	}
	if !f.submitted {
		t.Fatal("a live --red attach must submit the revision")
	}
	if !strings.Contains(out, "civitai.red cover staged on a revision") || !strings.Contains(out, "pubreq_8") {
		t.Errorf("expected the staged-revision line naming the request: %q", out)
	}
}

// The below-floor path confirms the attach LANDED by identity before treating
// the submit's 400 as progress. For --red that identity must be read from the
// override slot: the listing's own icon (7303 here) is a different image.
func TestSetIconRedBelowFloorIsProgressOnlyWhenTheRedSlotHoldsIt(t *testing.T) {
	fastScanPoll(t)
	f := &redFake{status: "approved", submit400: true, editRedIcon: 7202}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app", "--red", "-y")
	if err != nil {
		t.Fatalf("the floor refusal of a landed --red attach is progress, not a failure: %v", err)
	}
	if !strings.Contains(out, "civitai.red icon staged on an open revision") {
		t.Errorf("expected the below-floor staged line: %q", out)
	}

	// Negative control: the override slot does NOT hold the image (only the
	// listing's own icon is set), so nothing proves the attach landed and the
	// refusal must surface.
	f2 := &redFake{status: "approved", submit400: true, editRedIcon: nil}
	srv2 := httptest.NewServer(f2.handler(t))
	defer srv2.Close()
	listingEnv(t, srv2.URL)
	if _, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app", "--red", "-y"); err == nil {
		t.Fatal("an attach the override slot does not show must not be reported as staged")
	}
}

func TestClearRedSendsAnExplicitNullForTheNamedSlot(t *testing.T) {
	f := &redFake{status: "draft"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "clear-red", "cover", "--slug", "my-app")
	if err != nil {
		t.Fatalf("clear-red: %v", err)
	}
	if len(f.red) != 1 {
		t.Fatalf("expected one setRedMedia call, got %+v", f.red)
	}
	got := f.red[0]
	// HasImage: the key must be PRESENT and null — an absent key is a different
	// request, and the server tells them apart.
	if got.ListingID != "listing_7" || got.Slot != "cover" || !got.HasImage || got.ImageID != nil {
		t.Errorf("setRedMedia = %+v, want listing_7 / cover / imageId: null", got)
	}
	if !strings.Contains(out, "civitai.red cover override removed") {
		t.Errorf("expected the removed line: %q", out)
	}
}

func TestClearRedOnALiveListingSaysItIsStagedNotDone(t *testing.T) {
	f := &redFake{status: "approved"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "clear-red", "icon", "--slug", "my-app")
	if err != nil {
		t.Fatalf("clear-red live: %v", err)
	}
	if strings.Contains(out, "override removed\n") {
		t.Errorf("a live clear is staged, not done: %q", out)
	}
	if !strings.Contains(out, "staged on an open revision") || !strings.Contains(out, "civitai app listing submit-revision") {
		t.Errorf("expected the staged line and the publish command: %q", out)
	}
	if f.submitted {
		t.Error("clear-red must not submit the revision on its own")
	}
}

func TestClearRedRefusesAnythingButIconOrCover(t *testing.T) {
	f := &redFake{status: "draft"}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "clear-red", "screenshot", "--slug", "my-app")
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Fatalf("clear-red screenshot must be a usage error (exit 2), got %v", err)
	}
	if len(f.red) != 0 {
		t.Errorf("nothing may be written for a bad slot: %+v", f.red)
	}
}

func TestAddScreenshotHasNoRedFlag(t *testing.T) {
	_, _, err := run(t, "app", "listing", "add-screenshot", "x.png", "--red")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("a screenshot has no civitai.red override; --red must be refused, got %v", err)
	}
}
