package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// Issues #295 and #344 — the icon's binding cap is on an image the CLI never
// sees, and the numbers on screen did not add up.
//
// The measured symptom (credentialed dogfood run, 2026-08-10): a 1024x1024 JPEG
// of 38,201 bytes uploaded as `Uploading icon (37.3 KiB)…` and came back with a
// 400 naming 1,202,233 bytes against a 1,048,576 cap. FOUR numbers, none of them
// each other's: the file, the printed size, the server's count, the documented
// 2 MiB cap. The server counts the PNG it re-encodes from the DECODED PIXELS, so
// the missing link is the pixel dimensions — which the CLI decodes and threw
// away (#295) — and the units of the server's number (#344).
//
// These guards pin the PAIR, because either half alone leaves the author
// stranded: dimensions with no explanation are a silent number, and an
// explanation with no dimensions has nothing to compare against.
//
// 🔴 What they deliberately do NOT pin: any threshold. AGENTS.md item 25 keeps
// the platform's dimension/aspect/decoded bounds out of this CLI, and nothing
// here vendors one. Every number asserted below is derived from the test's own
// source file.
// ---------------------------------------------------------------------------

// iconAttachStub is a listingHandler whose setIcon reply is supplied by the
// caller and whose scan, if it is ever reached, settles immediately.
func iconAttachStub(log *callLog, imageID int, setIcon func(http.ResponseWriter)) listingHandler {
	return listingHandler{
		log:     log,
		imageID: imageID,
		setIcon: setIcon,
		scanStat: func(w http.ResponseWriter, _ int) {
			trpcData(w, map[string]any{"statuses": []map[string]any{{"imageId": imageID, "status": "scanned"}}})
		},
	}
}

// uploadLine returns the single "Uploading …" line out of a command's stdout,
// failing when there is not exactly one. Extracting the line rather than
// searching the whole buffer is what stops a match somewhere else in the output
// from standing in for the line under test.
func uploadLine(t *testing.T, out string) string {
	t.Helper()
	var hits []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Uploading ") {
			hits = append(hits, strings.TrimSpace(l))
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want exactly one `Uploading …` line, got %d, in:\n%s", len(hits), out)
	}
	return hits[0]
}

// TestUploadLineNamesTheDecodedDimensions is the #295 regression guard.
//
// The dimensions are asserted as a PAIR of DIFFERENT numbers (700x300, neither a
// factor of the other, neither equal to the byte count) so a transposed
// width/height reddens. A square fixture would pass a swapped format string.
//
// The byte count is asserted too: the contract is "dimensions ALONGSIDE the
// size", and replacing one quantity with the other is the other way to make a
// naive "does it mention 700" check pass.
func TestUploadLineNamesTheDecodedDimensions(t *testing.T) {
	fastScanPoll(t)
	log := &callLog{}
	h := iconAttachStub(log, 4242, func(w http.ResponseWriter) {
		trpcData(w, map[string]any{"status": "attached", "iconId": 4242})
	})
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	const wantW, wantH = 700, 300
	icon := writePNG(t, wantW, wantH)
	fi, err := os.Stat(icon)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	out, _, err := run(t, "app", "listing", "set-icon", icon, "--slug", "my-app")
	if err != nil {
		t.Fatalf("set-icon: %v", err)
	}
	line := uploadLine(t, out)
	if want := fmt.Sprintf("%d×%d", wantW, wantH); !strings.Contains(line, want) {
		t.Errorf("the upload line must name the dimensions the CLI already decoded (issue #295).\n"+
			"want %q in: %q\n"+
			"They are free — loadAndValidateImage decodes them from the header to pick the MIME "+
			"type — and they are the quantity every server-side listing bound is a function of.",
			want, line)
	}
	if want := humanBytes(fi.Size()); !strings.Contains(line, want) {
		t.Errorf("the upload line must still name the byte count (%q), not swap it for the "+
			"dimensions — the author needs both to read a size rejection: %q", want, line)
	}
}

// TestIconRejectionSaysWhoseBytesTheServerCounted is the #344 regression guard.
//
// It drives the measured symptom: a size-shaped 400 out of `appListings.setIcon`
// whose byte count is ~32x the file. The assertions are the four things an author
// needed and did not get, plus the one thing they DID get and must keep.
func TestIconRejectionSaysWhoseBytesTheServerCounted(t *testing.T) {
	neverSettlingScanPoll(t)
	// Shaped like the real rejection but not vendored as a bound: the CLI never
	// reads these numbers, it only has to stop them reading as the author's.
	const serverMsg = "icon is 1202233 bytes (max 1048576)"
	log := &callLog{}
	h := iconAttachStub(log, 77, func(w http.ResponseWriter) { trpcBadRequest(w, serverMsg) })
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	const wantW, wantH = 700, 300
	icon := writePNG(t, wantW, wantH)
	fi, err := os.Stat(icon)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	_, _, err = run(t, "app", "listing", "set-icon", icon, "--slug", "my-app")
	if err == nil {
		t.Fatal("expected the server's size rejection to surface")
	}
	got := err.Error()

	// 1. The server still speaks for itself, verbatim and first. The annotation is
	//    an addition, never a replacement — the CLI vendors none of those bounds.
	if !strings.Contains(got, serverMsg) {
		t.Fatalf("the server's own message must still be relayed verbatim; got:\n%s", got)
	}
	// 2. The file the author passed, by name and by both of its measurements.
	for _, want := range []string{icon, humanBytes(fi.Size()), fmt.Sprintf("%d×%d", wantW, wantH)} {
		if !strings.Contains(got, want) {
			t.Errorf("the rejection must name %q — the whole defect is a byte count that matches "+
				"nothing the author can see (file, printed size, or documented cap); got:\n%s", want, got)
		}
	}
	// 3. The UNITS of the server's number. Without this the two byte counts read as
	//    the same quantity and the author re-compresses, which cannot help.
	for _, want := range []string{"re-encoded server-side", "1024px"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rejection must say the server measured an image IT made (want %q), or the "+
				"1202233 stays unattributable; got:\n%s", want, got)
		}
	}
	// 4. The lever. "Your image is too big" with no next action is what #344 called
	//    unactionable.
	if !strings.Contains(got, "PIXEL dimensions") {
		t.Errorf("the rejection must name the lever (smaller pixel dimensions, not heavier "+
			"compression); got:\n%s", got)
	}
	// 5. Classification is pinned by errors.Is, never by message text (AGENTS item 7).
	//    Wrapping an error is exactly how a tag gets dropped.
	if !errors.Is(err, civitai.ErrBadRequest) {
		t.Errorf("annotating must not drop the BAD_REQUEST tag the exit-code classifier reads; got: %v", err)
	}
}

// TestNonIconRejectionOmitsTheReEncodeExplanation is the other half of the pair,
// and it is a CORRECTNESS guard rather than a tidiness one.
//
// Cover and screenshot ride the mint+persist full-res path, where the CLI sends
// `sizeBytes: len(data)` — the server measures the SAME bytes the CLI does. The
// re-encode paragraph would therefore be FALSE there, and a false explanation of
// a byte count is worse than none. The "what was sent" line still applies,
// because it is measured from the source file whatever the server checked.
func TestNonIconRejectionOmitsTheReEncodeExplanation(t *testing.T) {
	neverSettlingScanPoll(t)
	const serverMsg = "screenshot must be at least 320px on the shorter side (got 300px)"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/blocks/submissions"):
			submissionRow(w, "my-app", "block_1")
		case strings.Contains(r.URL.Path, "getMyListingForApp"):
			trpcData(w, map[string]any{"appListingId": "listing_1", "status": "draft"})
		case strings.Contains(r.URL.Path, "image-upload"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "uuid-1", "uploadURL": "http://" + r.Host + "/upload-sink",
			})
		case strings.Contains(r.URL.Path, "/upload-sink"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "persistAssetImage"):
			trpcData(w, map[string]any{"imageId": 91})
		case strings.Contains(r.URL.Path, "addScreenshot"):
			trpcBadRequest(w, serverMsg)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	listingEnv(t, srv.URL)

	const wantW, wantH = 700, 300
	shot := writePNG(t, wantW, wantH)
	_, _, err := run(t, "app", "listing", "add-screenshot", shot, "--slug", "my-app")
	if err == nil {
		t.Fatal("expected the server's screenshot rejection to surface")
	}
	got := err.Error()

	if !strings.Contains(got, serverMsg) {
		t.Fatalf("the server's own message must still be relayed verbatim; got:\n%s", got)
	}
	// Positive control for this test's own instrument: the annotation IS reached on
	// this path, so the absence asserted below is a measurement and not a case that
	// never ran.
	if want := fmt.Sprintf("%d×%d", wantW, wantH); !strings.Contains(got, want) {
		t.Fatalf("a non-icon rejection must still name what was sent (%q) — if it does not, the "+
			"absence assertion below proves nothing; got:\n%s", want, got)
	}
	for _, unwanted := range []string{"re-encoded server-side", "1024px", "PIXEL dimensions"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a %s is NOT re-encoded (the CLI sends sizeBytes: len(data) on the full-res "+
				"path, so the server counts the same bytes) — saying %q here is a false explanation "+
				"of the server's number; got:\n%s", kindScreenshot, unwanted, got)
		}
	}
}

// TestNonBadRequestRejectionIsPassedThroughUnannotated proves the annotation is a
// BRANCH on the classification, not something applied to every failure.
//
// A 500 says nothing about the image, so appending the author's file size and a
// paragraph about re-encoding would invent a diagnosis. This is the negative
// control for the two tests above: without it, "the annotation appeared" is
// equally true of code that annotates unconditionally.
//
// 🔴 IT IS AN INVARIANT GUARD, NOT A REGRESSION GUARD, and it is labelled as
// such because it is GREEN on origin/main — where nothing is annotated at all,
// so every absence it asserts holds vacuously. It earns its place by being red
// under mutation (drop the errors.Is branch in attachRejectionAdvice and it
// fails with this test's own message), not by having failed before the fix.
func TestNonBadRequestRejectionIsPassedThroughUnannotated(t *testing.T) {
	neverSettlingScanPoll(t)
	log := &callLog{}
	h := iconAttachStub(log, 5, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"json":{"message":"internal","code":-32603}}}`))
	})
	srv := httptest.NewServer(h.serve(t))
	defer srv.Close()
	listingEnv(t, srv.URL)

	_, _, err := run(t, "app", "listing", "set-icon", writePNG(t, 700, 300), "--slug", "my-app")
	if err == nil {
		t.Fatal("expected the 500 to surface")
	}
	got := err.Error()
	for _, unwanted := range []string{"what was sent", "re-encoded server-side", "700×300"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("a non-BAD_REQUEST failure carries no verdict about the image, so %q must not "+
				"be appended to it; got:\n%s", unwanted, got)
		}
	}
}

// TestAttachRejectionAdviceIsATaggedBranch exercises the predicate directly, at
// the two edges the end-to-end tests cannot cheaply reach: a nil error, and a
// tagged error that is not BAD_REQUEST.
func TestAttachRejectionAdviceIsATaggedBranch(t *testing.T) {
	info := appapi.ImageInfo{Width: 700, Height: 300, MimeType: "image/png"}
	if got := attachRejectionAdvice(nil, kindIcon, "x.png", 10, info); got != nil {
		t.Errorf("nil must pass through, got %v", got)
	}
	notFound := civitai.Tag(civitai.ErrNotFound, errors.New("no listing"))
	if got := attachRejectionAdvice(notFound, kindIcon, "x.png", 10, info); got.Error() != notFound.Error() {
		t.Errorf("a non-BAD_REQUEST error must pass through unchanged, got: %v", got)
	}
	bad := civitai.Tag(civitai.ErrBadRequest, errors.New("icon is 1202233 bytes (max 1048576)"))
	annotated := attachRejectionAdvice(bad, kindIcon, "x.png", 10, info)
	if annotated.Error() == bad.Error() {
		t.Fatal("a BAD_REQUEST must be annotated — this test's premise is that the branch fires")
	}
	if !errors.Is(annotated, civitai.ErrBadRequest) {
		t.Error("the classification tag must survive the annotation")
	}
}
