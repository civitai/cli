package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/civitai/cli/pkg/civitai"
)

// writePNG writes a valid PNG of the given size to a temp file and returns its path.
func writePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{9, 9, 9, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	p := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return p
}

func trpcData(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{"data": map[string]any{"json": v}},
	})
}

// submissionRow answers GET /api/v1/blocks/submissions?blockId=<slug>.
func submissionRow(w http.ResponseWriter, slug, appBlockID string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"submissions": []map[string]any{{"blockId": slug, "appBlockId": appBlockID}},
	})
}

func fastScanPoll(t *testing.T) {
	t.Helper()
	oldI, oldT := scanPollInterval, scanPollTimeout
	scanPollInterval = time.Millisecond
	scanPollTimeout = 5 * time.Second
	t.Cleanup(func() { scanPollInterval, scanPollTimeout = oldI, oldT })
}

func listingEnv(t *testing.T, srvURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "tok-1")
	t.Setenv("CIVITAI_BASE_URL", srvURL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
}

func TestAppListingStatusRendersFloor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft", "contentRating": "g", "hasPendingRevision": false})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "listing_1", "slug": "my-app", "status": "draft", "hasPendingRevision": false, "shadowId": nil,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 11, "url": "http://x/i.png"},
					"cover":       map[string]any{"imageId": nil, "url": nil},
					"screenshots": []any{map[string]any{"id": "alsc_1", "imageId": 5, "url": "http://x/s.png", "caption": "Grid", "order": 0}},
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "status", "--slug", "my-app")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "Icon:") || !strings.Contains(out, "set ✓") {
		t.Errorf("status should show icon set: %q", out)
	}
	if !strings.Contains(out, "missing cover") {
		t.Errorf("status should flag the missing cover: %q", out)
	}
	if !strings.Contains(out, "alsc_1") {
		t.Errorf("status should list the screenshot id: %q", out)
	}
}

func TestAppListingSetIconDraft(t *testing.T) {
	fastScanPoll(t)
	var setIconCalled, ingestCalled bool
	var scanCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft", "contentRating": "g"})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			ingestCalled = true
			trpcData(w, map[string]any{"imageId": 4242})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			// First poll pending, then scanned.
			n := atomic.AddInt32(&scanCalls, 1)
			status := "scanned"
			if n == 1 {
				status = "pending"
			}
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 4242, "status": status}}})
		case strings.Contains(r.URL.Path, "setIcon"):
			setIconCalled = true
			raw, _ := readBodyJSON(r)
			jsonField, _ := raw["json"].(map[string]any)
			if jsonField["imageId"].(float64) != 4242 {
				t.Errorf("setIcon imageId = %v, want 4242", jsonField["imageId"])
			}
			if jsonField["listingId"] != "listing_1" {
				t.Errorf("setIcon listingId = %v, want listing_1", jsonField["listingId"])
			}
			trpcData(w, map[string]any{"status": "attached", "iconId": 4242})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{"parentId": "listing_1", "status": "draft",
				"assets": map[string]any{"icon": map[string]any{"imageId": 4242}, "cover": map[string]any{"imageId": nil}, "screenshots": []any{}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	iconPath := writePNG(t, 256, 256)
	out, _, err := run(t, "app", "listing", "set-icon", iconPath, "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	if !ingestCalled || !setIconCalled {
		t.Fatalf("expected ingest+setIcon; ingest=%v setIcon=%v", ingestCalled, setIconCalled)
	}
	if atomic.LoadInt32(&scanCalls) < 2 {
		t.Errorf("expected the scan to be polled at least twice, got %d", scanCalls)
	}
	if !strings.Contains(out, "scanning image") {
		t.Errorf("expected scan progress in output: %q", out)
	}
	if !strings.Contains(out, "Icon set") {
		t.Errorf("expected success output: %q", out)
	}
	if !strings.Contains(out, "Still required before publishing: cover") {
		t.Errorf("expected the trailing floor line: %q", out)
	}
}

func TestAppListingSetIconLiveOpensRevision(t *testing.T) {
	fastScanPoll(t)
	var began, submitted, attachedToShadow bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "approved", "contentRating": "g"})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 5})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 5, "status": "scanned"}}})
		case strings.Contains(r.URL.Path, "beginListingRevision"):
			began = true
			trpcData(w, map[string]any{"shadowId": "shadow_9", "created": true})
		case strings.Contains(r.URL.Path, "setIcon"):
			raw, _ := readBodyJSON(r)
			jsonField, _ := raw["json"].(map[string]any)
			if jsonField["listingId"] == "shadow_9" {
				attachedToShadow = true
			}
			trpcData(w, map[string]any{"status": "attached", "iconId": 5})
		case strings.Contains(r.URL.Path, "submitListingRevision"):
			submitted = true
			raw, _ := readBodyJSON(r)
			jsonField, _ := raw["json"].(map[string]any)
			if jsonField["changelog"] != "brand refresh" {
				t.Errorf("changelog = %v, want brand refresh", jsonField["changelog"])
			}
			trpcData(w, map[string]any{"publishRequestId": "pubreq_9", "shadowId": "shadow_9", "slug": "my-app"})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{"parentId": "listing_1", "status": "approved",
				"assets": map[string]any{"icon": map[string]any{"imageId": 5}, "cover": map[string]any{"imageId": 7}, "screenshots": []any{}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	iconPath := writePNG(t, 256, 256)
	out, _, err := run(t, "app", "listing", "set-icon", iconPath, "--slug", "my-app", "--changelog", "brand refresh")
	if err != nil {
		t.Fatalf("set-icon live: %v", err)
	}
	if !began || !attachedToShadow || !submitted {
		t.Fatalf("live path incomplete: began=%v attachedToShadow=%v submitted=%v", began, attachedToShadow, submitted)
	}
	if !strings.Contains(out, "moderator review") {
		t.Errorf("expected the mod-review notice: %q", out)
	}
	if !strings.Contains(out, "pubreq_9") {
		t.Errorf("expected the revision id in output: %q", out)
	}
}

func TestAppListingSetIconBlockedNoAttach(t *testing.T) {
	fastScanPoll(t)
	var setIconCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft"})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 66})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 66, "status": "blocked"}}})
		case strings.Contains(r.URL.Path, "setIcon"):
			setIconCalled = true
			trpcData(w, map[string]any{"status": "attached"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	iconPath := writePNG(t, 256, 256)
	_, _, err := run(t, "app", "listing", "set-icon", iconPath, "--slug", "my-app")
	if err == nil {
		t.Fatal("expected an error for a blocked image")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention the block: %v", err)
	}
	if setIconCalled {
		t.Error("setIcon must NOT be called after a blocked scan")
	}
}

func TestAppListingLocalValidation(t *testing.T) {
	// A server that fails the test if ANY request reaches it — validation must
	// short-circuit before the network.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no network call expected, got %s", r.URL.Path)
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	t.Run("missing file", func(t *testing.T) {
		_, _, err := run(t, "app", "listing", "set-icon", filepath.Join(t.TempDir(), "nope.png"), "--slug", "my-app")
		if err == nil || !strings.Contains(err.Error(), "no such file") {
			t.Errorf("want no-such-file, got %v", err)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "note.txt")
		_ = os.WriteFile(bad, []byte("i am not an image"), 0o644)
		_, _, err := run(t, "app", "listing", "set-icon", bad, "--slug", "my-app")
		if err == nil || !strings.Contains(err.Error(), "unrecognized image") {
			t.Errorf("want unrecognized-image, got %v", err)
		}
	})

	t.Run("too big", func(t *testing.T) {
		big := filepath.Join(t.TempDir(), "big.png")
		// A valid PNG header + padding beyond the icon cap so the SIZE check trips.
		blob := make([]byte, maxIconBytes+1)
		_ = os.WriteFile(big, blob, 0o644)
		_, _, err := run(t, "app", "listing", "set-icon", big, "--slug", "my-app")
		if err == nil || !strings.Contains(err.Error(), "larger than") {
			t.Errorf("want too-big, got %v", err)
		}
	})
}

func TestAppListingScope403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"json": map[string]any{"message": "Your API key does not have the required scope for this action"}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "status", "--slug", "my-app")
	if err == nil || !strings.Contains(err.Error(), "Apps submit scope") {
		t.Errorf("want a legible scope-403 error, got %v", err)
	}
}

func TestSubmitFloorHeadsUp(t *testing.T) {
	var buf bytes.Buffer
	printListingFloorHeadsUp(&buf)
	got := buf.String()
	// The heads-up must convey the listing is created on APPROVAL, then media is
	// added — not that the listing commands work at submit time.
	for _, want := range []string{"APPROVED", "After approval", "set-icon", "set-cover", "listing status"} {
		if !strings.Contains(got, want) {
			t.Errorf("heads-up missing %q: %q", want, got)
		}
	}
	// Regression guard: it must NOT imply the media commands work while pending.
	for _, gone := range []string{"won't publish until", "when you submit"} {
		if strings.Contains(got, gone) {
			t.Errorf("heads-up should not contain stale phrasing %q: %q", gone, got)
		}
	}
}

// TestAppListingPendingNoListingYet drives the pending-app path: the submission
// exists but has no backing appBlockId yet (the store listing is created only on
// moderator approval). The CLI must (a) say the app is still pending review and
// the listing is created on approval — NOT "run `civitai app submit` first" — and
// (b) tag the error ErrNotFound so the entrypoint maps it to exit 4.
func TestAppListingPendingNoListingYet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			// A pending submission with no appBlockId yet.
			submissionRow(w, "my-app", "")
		default:
			t.Errorf("unexpected path %s — should short-circuit before the listing lookup", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "status", "--slug", "my-app")
	if err == nil {
		t.Fatal("expected a not-found error for a pending app with no listing yet")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("missing-listing error must be tagged ErrNotFound (→ exit 4), got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "pending review") || !strings.Contains(err.Error(), "approves") {
		t.Errorf("error should explain the listing is created on approval: %v", err)
	}
	// Regression guard: it must NOT tell the author to submit again — they did.
	if strings.Contains(err.Error(), "civitai app submit") {
		t.Errorf("error should not tell the author to submit again: %v", err)
	}
}

// TestAppListingHelpApprovalWording pins the corrected help text on the listing
// command group and its status subcommand: it must convey the listing is created
// on APPROVAL, and the old submit-time phrasing must be gone.
func TestAppListingHelpApprovalWording(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"group help", []string{"app", "listing", "--help"}},
		{"status help", []string{"app", "listing", "status", "--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := run(t, tc.args...)
			if err != nil {
				t.Fatalf("%v: %v", tc.args, err)
			}
			if !strings.Contains(out, "APPROVES") {
				t.Errorf("help should say the listing is created when a moderator APPROVES the app:\n%s", out)
			}
			for _, gone := range []string{"exists only after you have submitted", "created when you submit"} {
				if strings.Contains(out, gone) {
					t.Errorf("help still contains stale submit-time phrasing %q:\n%s", gone, out)
				}
			}
		})
	}
}

// readBodyJSON decodes a request body into a generic map.
func readBodyJSON(r *http.Request) (map[string]any, error) {
	var m map[string]any
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&m)
	return m, err
}
