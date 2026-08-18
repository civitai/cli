package cmd

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

// neverSettlingScanPoll shrinks the poll budget so a scan that NEVER reaches a
// terminal state costs a few hundred milliseconds instead of scanPollTimeout.
// Used by the fast-fail tests: they assert the CLI answered from the ATTACH, so
// the poll must be cheap enough to fail loudly rather than hang if it ever runs.
func neverSettlingScanPoll(t *testing.T) {
	t.Helper()
	oldI, oldT := scanPollInterval, scanPollTimeout
	scanPollInterval = time.Millisecond
	scanPollTimeout = 300 * time.Millisecond
	t.Cleanup(func() { scanPollInterval, scanPollTimeout = oldI, oldT })
}

// callLog records the ORDER in which the fake server was hit. The load-bearing
// assertion for issue #270 is a sequence, not a pair of booleans: "both happened"
// is equally true of the ordering that shipped the two-minute wait.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (c *callLog) add(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, name)
}

func (c *callLog) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

// indexOf returns the position of the first call named `name`, or -1.
func (c *callLog) indexOf(name string) int {
	for i, got := range c.all() {
		if got == name {
			return i
		}
	}
	return -1
}

func (c *callLog) count(name string) int {
	n := 0
	for _, got := range c.all() {
		if got == name {
			n++
		}
	}
	return n
}

// requireBefore fails unless BOTH calls were observed and `first` came first.
// Requiring both is what stops the assertion passing vacuously on a run where
// neither happened.
func (c *callLog) requireBefore(t *testing.T, first, second string) {
	t.Helper()
	i, j := c.indexOf(first), c.indexOf(second)
	if i < 0 {
		t.Fatalf("%s was never called; sequence was %v", first, c.all())
	}
	if j < 0 {
		t.Fatalf("%s was never called; sequence was %v", second, c.all())
	}
	if i > j {
		t.Fatalf("%s must be called BEFORE %s (issue #270 — the server validates "+
			"geometry/aspect/MIME at ATTACH, so polling first makes the author wait "+
			"out the whole scan for a verdict available immediately); sequence was %v",
			first, second, c.all())
	}
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
			// The scan is still in flight at attach time, which is the normal case
			// now that the attach comes first — the server writes the id and flags
			// `scanPending`, and the CLI polls afterwards.
			trpcData(w, map[string]any{"status": "attached", "iconId": 4242, "scanPending": true})
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

// ---------------------------------------------------------------------------
// Issue #270 — ATTACH BEFORE SCAN POLL.
//
// The server validates geometry, aspect, MIME and byte size at ATTACH (inside
// loadValidatedImage, ahead of the ingestion-status gate) and passes
// allowPending:true, so a still-scanning image is written and flagged rather than
// refused. The CLI used to poll the scan first — up to scanPollTimeout — and only
// then ask, so an author with a wrongly-shaped icon waited two minutes for a
// verdict that was available immediately. These tests pin the ORDER, the
// fast-fail, and that no scan signal was lost by moving the poll after the attach.
// ---------------------------------------------------------------------------

// listingHandler is the shared fake for the #270 tests: one draft listing, an
// inline-icon ingest, and pluggable setIcon / scan responses. Every request is
// recorded in the call log so a test can assert the SEQUENCE.
type listingHandler struct {
	log      *callLog
	imageID  int
	setIcon  func(w http.ResponseWriter)
	scanStat func(w http.ResponseWriter, n int)
}

func (h listingHandler) serve(t *testing.T) http.HandlerFunc {
	var scanCalls int32
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			h.log.add("submissions")
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			h.log.add("getMyListingForApp")
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft"})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			h.log.add("ingest")
			trpcData(w, map[string]any{"imageId": h.imageID})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			h.log.add("scan")
			h.scanStat(w, int(atomic.AddInt32(&scanCalls, 1)))
		case strings.Contains(r.URL.Path, "setIcon"):
			h.log.add("setIcon")
			h.setIcon(w)
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			h.log.add("getMyListingForEdit")
			trpcData(w, map[string]any{"parentId": "listing_1", "status": "draft",
				"assets": map[string]any{"icon": map[string]any{"imageId": h.imageID},
					"cover": map[string]any{"imageId": nil}, "screenshots": []any{}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}
}

// alwaysPendingScan never reaches a terminal state — the only way to answer is
// from the attach.
func alwaysPendingScan(imageID int) func(http.ResponseWriter, int) {
	return func(w http.ResponseWriter, _ int) {
		trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": imageID, "status": "pending"}}})
	}
}

// trpcBadRequest writes the tRPC error envelope a rejected attach really returns.
func trpcBadRequest(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": message, "code": -32600}},
	})
}

// TestAppListingAttachIsObservedBeforeScanPoll is the ordering guard. Both calls
// happening is not the contract — the SEQUENCE is, and the pre-#270 code made
// both calls too.
func TestAppListingAttachIsObservedBeforeScanPoll(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	h := listingHandler{
		log:     log,
		imageID: 4242,
		setIcon: func(w http.ResponseWriter) {
			trpcData(w, map[string]any{"status": "attached", "iconId": 4242, "scanPending": true})
		},
		scanStat: func(w http.ResponseWriter, n int) {
			st := "scanned"
			if n == 1 {
				st = "pending"
			}
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 4242, "status": st}}})
		},
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	log.requireBefore(t, "setIcon", "scan")
	// …and the scan is still waited on afterwards: the signal moved, it did not go away.
	if got := log.count("scan"); got < 2 {
		t.Errorf("the scan must still be polled to completion after the attach, got %d polls (sequence %v)", got, log.all())
	}
	if !strings.Contains(out, "Icon set") {
		t.Errorf("expected success output: %q", out)
	}
}

// TestAppListingGeometryRejectionDoesNotWaitOutTheScan is the user-visible point
// of #270: a wrongly-shaped icon must surface the SERVER'S OWN message without
// waiting for a scan that, here, never settles.
func TestAppListingGeometryRejectionDoesNotWaitOutTheScan(t *testing.T) {
	neverSettlingScanPoll(t)
	const serverMsg = "icon must be square-ish (aspect 2.00 outside 0.9–1.1)"
	log := &callLog{}
	h := listingHandler{
		log:      log,
		imageID:  77,
		setIcon:  func(w http.ResponseWriter) { trpcBadRequest(w, serverMsg) },
		scanStat: alwaysPendingScan(77),
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 512, 256), "--slug", "my-app")
	if err == nil {
		t.Fatal("expected the server's geometry rejection to surface")
	}
	if !strings.Contains(err.Error(), serverMsg) {
		t.Errorf("the CLI must relay the server's own message verbatim (it vendors no bounds); got: %v", err)
	}
	if n := log.count("scan"); n != 0 {
		t.Errorf("a rejected attach must not be preceded or followed by a scan poll, got %d (sequence %v)", n, log.all())
	}
	if log.indexOf("setIcon") < 0 {
		t.Fatalf("setIcon was never called; sequence %v", log.all())
	}
}

// TestAppListingBlockedAtAttachIsReported covers the TERMINAL-blocked path: the
// scan already finished Blocked, so `loadValidatedImage` throws BAD_REQUEST at
// attach and nothing is written. Exactly one message reaches the user.
func TestAppListingBlockedAtAttachIsReported(t *testing.T) {
	neverSettlingScanPoll(t)
	const serverMsg = "that image was rejected during scanning — choose a different image"
	log := &callLog{}
	h := listingHandler{
		log:      log,
		imageID:  66,
		setIcon:  func(w http.ResponseWriter) { trpcBadRequest(w, serverMsg) },
		scanStat: alwaysPendingScan(66),
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err == nil {
		t.Fatal("expected an error for a blocked image")
	}
	if !strings.Contains(err.Error(), serverMsg) {
		t.Errorf("error should carry the server's blocked message: %v", err)
	}
	// One verdict, not two: the CLI's own blocked-scan wording must not also fire.
	if strings.Contains(out, "blocked by the content scan") {
		t.Errorf("the CLI printed a SECOND blocked diagnosis alongside the server's: %q", out)
	}
	if strings.Contains(out, "Icon set") {
		t.Errorf("a rejected attach must not report success: %q", out)
	}
}

// TestAppListingBlockedDuringPostAttachPoll is the INVERSION of the pre-#270
// TestAppListingSetIconBlockedNoAttach, which asserted "setIcon must NOT be called
// after a blocked scan". With the attach first, a scan that flips to Blocked
// afterwards is caught by the poll instead — the signal must survive the move, and
// the command must still not report success.
func TestAppListingBlockedDuringPostAttachPoll(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	h := listingHandler{
		log:     log,
		imageID: 66,
		setIcon: func(w http.ResponseWriter) {
			trpcData(w, map[string]any{"status": "attached", "iconId": 66, "scanPending": true})
		},
		scanStat: func(w http.ResponseWriter, _ int) {
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 66, "status": "blocked"}}})
		},
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err == nil {
		t.Fatal("expected an error for a blocked image")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention the block: %v", err)
	}
	if strings.Contains(out, "Icon set") {
		t.Errorf("a blocked scan must not report success: %q", out)
	}
	log.requireBefore(t, "setIcon", "scan")
}

// TestAppListingScreenshotBlockedNamesItsRemoval — attaching first means a blocked
// SCREENSHOT leaves a row behind that the old order could never create, so the
// failure has to hand over the id needed to undo it.
func TestAppListingScreenshotBlockedNamesItsRemoval(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft"})
		case strings.Contains(r.URL.Path, "image-upload"):
			// A REST route, not tRPC: {id, uploadURL} at the top level.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "uuid-1", "uploadURL": "http://" + r.Host + "/upload-sink",
			})
		case strings.Contains(r.URL.Path, "/upload-sink"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "persistAssetImage"):
			log.add("persist")
			trpcData(w, map[string]any{"imageId": 91})
		case strings.Contains(r.URL.Path, "addScreenshot"):
			log.add("addScreenshot")
			trpcData(w, map[string]any{"status": "attached", "id": "alsc_new", "order": 0, "scanPending": true})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			log.add("scan")
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageID": 91, "imageId": 91, "status": "blocked"}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "add-screenshot", writePNG(t, 640, 480), "--slug", "my-app")
	if err == nil {
		t.Fatal("expected an error for a blocked screenshot")
	}
	log.requireBefore(t, "addScreenshot", "scan")
	if !strings.Contains(out, "rm-screenshot alsc_new") {
		t.Errorf("a blocked screenshot is already written, so the failure must name how to remove it: %q", out)
	}
}

// TestAppListingSkipsScanPollWhenAttachReportsScanned pins the fast path AND the
// reading of the server's own flag: `scanPending` is set only while the scan is in
// flight and the key is OMITTED once the image is Scanned, so an attach that comes
// back without it owes no poll at all.
//
// It is the negative half of a pair — TestAppListingAttachIsObservedBeforeScanPoll
// is the positive control that this fake CAN record a non-zero scan count, so a
// zero here is a measurement rather than a harness wired to nothing.
func TestAppListingSkipsScanPollWhenAttachReportsScanned(t *testing.T) {
	neverSettlingScanPoll(t)
	log := &callLog{}
	h := listingHandler{
		log:     log,
		imageID: 12,
		setIcon: func(w http.ResponseWriter) {
			// No `scanPending` key — the server saw ingestion == Scanned.
			trpcData(w, map[string]any{"status": "attached", "iconId": 12})
		},
		scanStat: alwaysPendingScan(12),
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	if n := log.count("scan"); n != 0 {
		t.Errorf("an already-scanned attach must not poll, got %d polls (sequence %v)", n, log.all())
	}
	if !strings.Contains(out, "Icon set") {
		t.Errorf("expected success output: %q", out)
	}
}

// TestAppListingLegacyPendingAttachFallsBackToScanFirst covers the one shape that
// would make attaching first SILENTLY do nothing: a server without
// `allowPending` answers `{status:"pending"}` and writes no id. Today's procs
// never return it, but printing "Icon set ✓" over a no-op is the worst possible
// regression, so the CLI falls back to the pre-#270 order and re-attaches.
func TestAppListingLegacyPendingAttachFallsBackToScanFirst(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	var attachN int32
	h := listingHandler{
		log:     log,
		imageID: 31,
		setIcon: func(w http.ResponseWriter) {
			if atomic.AddInt32(&attachN, 1) == 1 {
				trpcData(w, map[string]any{"status": "pending"})
				return
			}
			trpcData(w, map[string]any{"status": "attached", "iconId": 31})
		},
		scanStat: func(w http.ResponseWriter, n int) {
			st := "scanned"
			if n == 1 {
				st = "pending"
			}
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 31, "status": st}}})
		},
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	if got := log.count("setIcon"); got != 2 {
		t.Errorf("a `pending` attach wrote nothing, so it must be retried after the scan: setIcon calls = %d (sequence %v)", got, log.all())
	}
	if got := log.count("scan"); got < 2 {
		t.Errorf("the fallback must wait the scan out before re-attaching, got %d polls (sequence %v)", got, log.all())
	}
	if !strings.Contains(out, "Icon set") {
		t.Errorf("expected success once the retry attached: %q", out)
	}
}

// TestAppListingLiveScanFailureLeavesTheRevisionUnsubmitted — on a LIVE listing the
// attach now lands on the shadow revision before the scan verdict, so a blocked
// image must NOT be submitted for moderator review, and the user must be told the
// live listing is untouched.
func TestAppListingLiveScanFailureLeavesTheRevisionUnsubmitted(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "approved"})
		case strings.Contains(r.URL.Path, "ingestAssetFromDataUri"):
			trpcData(w, map[string]any{"imageId": 5})
		case strings.Contains(r.URL.Path, "beginListingRevision"):
			log.add("begin")
			trpcData(w, map[string]any{"shadowId": "shadow_9", "created": true})
		case strings.Contains(r.URL.Path, "setIcon"):
			log.add("setIcon")
			trpcData(w, map[string]any{"status": "attached", "iconId": 5, "scanPending": true})
		case strings.Contains(r.URL.Path, "getAssetScanStatuses"):
			log.add("scan")
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": 5, "status": "blocked"}}})
		case strings.Contains(r.URL.Path, "submitListingRevision"):
			log.add("submit")
			trpcData(w, map[string]any{"publishRequestId": "pubreq_9", "shadowId": "shadow_9", "slug": "my-app"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app", "--changelog", "brand refresh")
	if err == nil {
		t.Fatal("expected the blocked scan to fail the command")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention the block: %v", err)
	}
	if log.count("submit") != 0 {
		t.Errorf("a blocked image must never be submitted for moderator review (sequence %v)", log.all())
	}
	if !strings.Contains(out, "live listing is unchanged") {
		t.Errorf("the user must be told the revision was not submitted: %q", out)
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
	// 🔴 INVERTED by draft-at-submit: THIS submit minted the store listing as a draft,
	// so the heads-up must tell the author the media is settable NOW, while in review,
	// and that it carries forward on approval.
	for _, want := range []string{"NOW", "in review", "carry forward", "set-icon", "set-cover", "listing status"} {
		if !strings.Contains(got, want) {
			t.Errorf("heads-up missing %q: %q", want, got)
		}
	}
	// Regression guard on the OLD premise: it must no longer make the author wait for
	// approval before touching listing media.
	for _, gone := range []string{"After approval", "once your app is APPROVED", "after approval"} {
		if strings.Contains(got, gone) {
			t.Errorf("heads-up should not contain stale wait-for-approval phrasing %q: %q", gone, got)
		}
	}
}

// readmeCommandRefRow returns the single command-reference TABLE ROW whose
// command cell names `cmd`. A row is `| `civitai …` | … |`; a passing mention in
// prose is not one, which is the distinction this helper exists to enforce.
func readmeCommandRefRow(t *testing.T, readme, cmd string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(readme, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		// The command cell is everything up to the first unescaped ` | `.
		cell := line
		if i := strings.Index(line[1:], "` | "); i >= 0 {
			cell = line[:i+2]
		}
		if strings.Contains(cell, cmd) {
			found = append(found, line)
		}
	}
	switch len(found) {
	case 0:
		t.Fatalf("README.md has no command-reference table row for %q — it is undocumented "+
			"in the table a reader scans to find out what the CLI can do", cmd)
	case 1:
		return found[0]
	default:
		t.Fatalf("README.md has %d command-reference rows for %q; expected exactly one", len(found), cmd)
	}
	return ""
}

// TestReadmeDocumentsAppListing is the regression guard for issue #227 point 1:
// `civitai app listing` had ZERO mentions in an 80 KB README while GATING
// publication (a listing needs an icon and a cover before it can go live), so a
// reader planning a release budgeted no time for artwork.
//
// The guard is structural rather than a `strings.Contains("app listing")`: it
// requires a real command-reference ROW, and requires that row to name EVERY
// subcommand actually registered on the group. Adding `app listing set-banner`
// therefore fails here until the README is updated — which is the drift that
// produced the defect in the first place.
func TestReadmeDocumentsAppListing(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	readme := string(raw)

	row := readmeCommandRefRow(t, readme, "civitai app listing")

	// Every registered subcommand must be named in that row. Reading them off the
	// live command tree (never a hardcoded list) is what makes this catch drift.
	subs := newAppListingCmd().Commands()
	if len(subs) < 6 {
		t.Fatalf("expected the `app listing` group to register >= 6 subcommands, got %d — "+
			"if the group shrank on purpose, re-derive this floor", len(subs))
	}
	for _, sub := range subs {
		if !strings.Contains(row, sub.Name()) {
			t.Errorf("README command-reference row for `app listing` does not name the %q "+
				"subcommand:\n%s", sub.Name(), row)
		}
	}

	// The row must carry the fact that GATES publication — both required assets.
	// Naming the commands without it documents the tool and not the deadline.
	for _, want := range []string{"icon", "cover"} {
		if !strings.Contains(strings.ToLower(row), want) {
			t.Errorf("README command-reference row for `app listing` should name the %q "+
				"the publish floor requires:\n%s", want, row)
		}
	}

	// …and the publication FLOW must show the commands too, not just the table.
	// Scoped to the text outside the row so a single mention can't satisfy both.
	body := strings.Replace(readme, row, "", 1)
	for _, want := range []string{
		"civitai app listing status",
		"civitai app listing set-icon",
		"civitai app listing set-cover",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("README should show %q in the publication flow, not only in the "+
				"command-reference table", want)
		}
	}
}

// 🔴 TestAppListingPendingReachesDraftBySlug — the whole point of draft-at-submit.
// A first-version app pending review has NO backing appBlockId, but `civitai app
// submit` minted its store listing as a pre-approval DRAFT, and the slug is the only
// selector the CLI can send for it.
//
// This INVERTS the old TestAppListingPendingNoListingYet, which asserted the CLI
// short-circuits to a "listing is created on approval" error. That short-circuit is
// now the bug: it would make the pending-media flow unreachable from the CLI. So this
// pins that the CLI (a) does NOT bail before the lookup, (b) sends the SLUG (the only
// selector that can name a null-appBlockId draft) and omits the empty appBlockId, and
// (c) renders the draft's media.
//
// 🔴 THE FAKE ANSWERS THE LOOKUP UNCONDITIONALLY, SO THIS PINS WHAT THE CLI SENDS AND
// NOTHING ABOUT WHAT A SERVER RESOLVES. The handler branches on PATH only; it never
// reads the input it captured, so it would hand back the same draft for a request
// carrying no slug at all. That is fine for the claim being made — but the comment
// this test used to carry ("even though the server supports it") turned it into
// evidence for a server behaviour that was never measured. See civitai/cli#424 and
// the doc comment on `resolveListing`: the genuinely PENDING first-version case is
// asserted from server source and remains unmeasured to this day.
func TestAppListingPendingReachesDraftBySlug(t *testing.T) {
	var listingForAppInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			// A pending submission with NO appBlockId — the app is still in review.
			submissionRow(w, "my-app", "")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			listingForAppInput = r.URL.Query().Get("input")
			trpcData(w, map[string]any{
				"appListingId": "apl_draft", "status": "draft",
				"contentRating": "g", "hasPendingRevision": false,
			})
		case strings.Contains(r.URL.Path, "getMyListingForEdit"):
			trpcData(w, map[string]any{
				"parentId": "apl_draft", "slug": "my-app", "status": "draft",
				"hasPendingRevision": false, "shadowId": nil,
				"assets": map[string]any{
					"icon":        map[string]any{"imageId": 11, "url": "http://x/i.png"},
					"cover":       map[string]any{"imageId": nil, "url": nil},
					"screenshots": []any{},
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
		t.Fatalf("a pending app's DRAFT listing must be reachable by slug, got: %v", err)
	}
	if listingForAppInput == "" {
		t.Fatal("getMyListingForApp was never called — the CLI short-circuited before the lookup")
	}
	if !strings.Contains(listingForAppInput, `"slug":"my-app"`) {
		t.Errorf("the pending lookup must carry the slug: %q", listingForAppInput)
	}
	// An empty appBlockId must be OMITTED, not sent as "" — the server's
	// "either appBlockId or slug" schema would otherwise see a supplied-but-empty id.
	if strings.Contains(listingForAppInput, "appBlockId") {
		t.Errorf("an absent appBlockId must be omitted from the lookup: %q", listingForAppInput)
	}
	// The draft's media renders — the author can see what they've attached while pending.
	if !strings.Contains(out, "icon") && !strings.Contains(out, "Icon") {
		t.Errorf("status should render the pending draft's media: %q", out)
	}
}

// 🔴 TestAppListingHelpPendingWording pins the INVERTED help text on the listing
// command group and its status subcommand. Draft-at-submit means the listing exists
// as a DRAFT from `civitai app submit`, so the help must tell the author the media is
// settable WHILE PENDING — the previous "created when a moderator APPROVES your app"
// wording is now false and actively discourages the flow the backend enables.
func TestAppListingHelpPendingWording(t *testing.T) {
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
			if !strings.Contains(out, "DRAFT") {
				t.Errorf("help should say the listing is created as a DRAFT at submit:\n%s", out)
			}
			if !strings.Contains(out, "civitai app submit") {
				t.Errorf("help should name `civitai app submit` as what creates the listing:\n%s", out)
			}
			if !strings.Contains(out, "pending review") {
				t.Errorf("help should say media is settable while pending review:\n%s", out)
			}
			// Regression guard on the OLD, now-false premise.
			for _, gone := range []string{
				"created when a moderator APPROVES",
				"not at submit time",
				"not when you\nsubmit it",
				"reports nothing until then",
			} {
				if strings.Contains(out, gone) {
					t.Errorf("help still contains the stale created-on-approval phrasing %q:\n%s", gone, out)
				}
			}
		})
	}
}

// successLine returns the first line of s containing sub, or "" if none does.
func successLine(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// TestAppListingSuccessLinesCarryOneCheckGlyph — `ui.Success` already prefixes
// "✓ ", and three call sites in app_listing.go appended a second one to their
// format string, so the rendered line read `✓ Icon set ✓`.
//
// The assertion is on the COUNT, not on the absence of a trailing glyph: a fix
// that moved the duplicate somewhere else in the line would still be wrong, and
// requiring exactly one also fails if someone later drops the ui.Success
// prefix, which is the other way this line can go wrong.
func TestAppListingSuccessLinesCarryOneCheckGlyph(t *testing.T) {
	log := &callLog{}
	h := listingHandler{
		log:     log,
		imageID: 12,
		setIcon: func(w http.ResponseWriter) {
			trpcData(w, map[string]any{"status": "attached", "iconId": 12})
		},
		scanStat: alwaysPendingScan(12),
	}
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	out, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 256, 256), "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	line := successLine(out, "Icon set")
	if line == "" {
		// CONTROL: without the success line there is nothing to count, and a
		// zero would read as a pass.
		t.Fatalf("CONTROL failure: no \"Icon set\" line in the output, so there is no glyph to count:\n%s", out)
	}
	if n := strings.Count(line, "✓"); n != 1 {
		t.Errorf("success line %q carries %d ✓ glyphs, want exactly 1 — ui.Success already prefixes one", line, n)
	}
}

// readBodyJSON decodes a request body into a generic map.
func readBodyJSON(r *http.Request) (map[string]any, error) {
	var m map[string]any
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&m)
	return m, err
}
