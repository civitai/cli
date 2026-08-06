package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// --- degenerate dimensions ----------------------------------------------------

// 🔴 The `cfg.Width <= 0 || cfg.Height <= 0` guard, at its BOUNDARY.
//
// Surviving mutation: `<= 0` → `< 0`. No fixture in the suite decoded to a zero
// dimension, so nothing could tell the two apart — and `< 0` is unreachable in
// practice, which makes the mutated guard permanently inert.
//
// A zero-dimension image is not hypothetical and is not malformed enough to be
// caught upstream. MEASURED with the Go 1.25 stdlib:
//
//	jpeg.Encode of a 16x0 image  -> 591 bytes, DecodeConfig returns
//	                                format="jpeg", w=16, h=0, err=nil
//	png.Encode of a 16x0 image   -> the ENCODER refuses ("invalid image size")
//
// so JPEG is the format that can express it and PNG is not — which is why every
// fixture here is a JPEG. `image.DecodeConfig` hands those dimensions back
// cleanly, so without this guard a `width: 0` reaches genapi.GraphImage and then
// the server, where item 19(d) records that the images node validates the
// transformed array and a bad entry is a hard 400 AFTER the graph is priced.
func TestDecodeImageHeader_RefusesDegenerateDimensions(t *testing.T) {
	degenerate := []struct {
		name string
		w, h int
	}{
		{"zero width", 0, 16},
		{"zero height", 16, 0},
		{"both zero", 0, 0},
	}
	for _, tc := range degenerate {
		t.Run(tc.name, func(t *testing.T) {
			data := jpegBytes(t, tc.w, tc.h)

			// Precondition, stated as an assertion rather than assumed: the
			// decoder really does accept this fixture and report the zero. If a
			// future stdlib starts rejecting it, this test must fail LOUDLY here
			// rather than pass for the wrong reason.
			cfg, format, derr := image.DecodeConfig(bytes.NewReader(data))
			if derr != nil {
				t.Fatalf("fixture precondition broken: DecodeConfig refused the %dx%d jpeg itself (%v) — this case no longer reaches the dimension guard",
					tc.w, tc.h, derr)
			}
			if format != "jpeg" {
				t.Fatalf("fixture precondition broken: format = %q, want jpeg", format)
			}
			if cfg.Width != tc.w || cfg.Height != tc.h {
				t.Fatalf("fixture precondition broken: decoded %dx%d, want %dx%d", cfg.Width, cfg.Height, tc.w, tc.h)
			}

			// 🔴 The guard itself.
			w, h, _, err := decodeImageHeader(bytes.NewReader(data), "--image x")
			if err == nil {
				t.Fatalf("🔴 a %dx%d image was ACCEPTED (returned %dx%d) — the server requires positive width and height",
					tc.w, tc.h, w, h)
			}
			// This must be the DIMENSION refusal, not the unknown-format or
			// corrupt-file arm. A test that is green because a *different* guard
			// fired is green for the wrong reason.
			if !strings.Contains(err.Error(), "degenerate size") {
				t.Errorf("wrong guard fired for %dx%d: %v", tc.w, tc.h, err)
			}
		})
	}

	// POSITIVE CONTROL: the same helper, the same format, positive dimensions —
	// accepted. Without it, the refusals above could equally come from a
	// decodeImageHeader that rejects every JPEG.
	t.Run("positive control: a positive-dimension jpeg is accepted", func(t *testing.T) {
		w, h, format, err := decodeImageHeader(bytes.NewReader(jpegBytes(t, 24, 40)), "--image x")
		if err != nil {
			t.Fatalf("POSITIVE CONTROL FAILED: a valid 24x40 jpeg was refused: %v", err)
		}
		if w != 24 || h != 40 || format != "jpeg" {
			t.Fatalf("POSITIVE CONTROL FAILED: got %dx%d %q, want 24x40 jpeg", w, h, format)
		}
	})
}

// The same boundary through the REAL --image path, so the guard is pinned where
// a user meets it and not only at the helper. A degenerate file must be refused
// as a usage error and must never be uploaded.
func TestResolveLocalImage_DegenerateDimensionsAreRefusedBeforeUpload(t *testing.T) {
	dir := t.TempDir()
	bad := writeFixture(t, dir, "flat.jpg", jpegBytes(t, 16, 0))

	s := &genSeams{}
	_, err := resolveImages(context.Background(), s.deps(t), []string{bad})
	if err == nil {
		t.Fatal("🔴 a 16x0 image was accepted by --image")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want ErrUsage (exit 2), got %v", err)
	}
	// 🔴 Nothing was uploaded: the refusal happens before the network write.
	if s.uploadCalls != 0 {
		t.Errorf("a degenerate image was uploaded anyway (%d upload(s))", s.uploadCalls)
	}

	// POSITIVE CONTROL for that zero: the SAME seam, driven by a valid file.
	good := writeFixture(t, dir, "ok.jpg", jpegBytes(t, 24, 40))
	ok := &genSeams{}
	imgs, err := resolveImages(context.Background(), ok.deps(t), []string{good})
	if err != nil {
		t.Fatalf("POSITIVE CONTROL FAILED: a valid image was refused: %v", err)
	}
	if ok.uploadCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: upload seam called %d times, want 1 — the 0 above proves nothing", ok.uploadCalls)
	}
	if imgs[0].Width != 24 || imgs[0].Height != 40 {
		t.Errorf("dimensions = %dx%d, want 24x40", imgs[0].Width, imgs[0].Height)
	}
}

// --- the supported-format allowlist -------------------------------------------

// The REACHABLE half of the unsupported-format rule.
//
// The `if _, ok := supportedImageFormats[format]; !ok` branch in
// decodeImageHeader is UNREACHABLE as the package is built (see the comment at
// that branch): only png and jpeg have registered decoders, both are keys of the
// map, and everything else fails earlier with image.ErrFormat. Neutering it
// therefore survives, and no test can change that without registering a third
// decoder — which gifBytes() documents as the thing that would silently destroy
// the real unsupported-format test.
//
// So instead of pretending to cover the branch, this pins the INVARIANT that
// makes it unreachable, which is the thing that can actually regress: the map
// and the blank imports must stay in lockstep. Add a decoder without a map
// entry and the "unsupported format" arm goes live for a format users can
// legitimately hand us; add a map entry without a decoder and the CLI advertises
// a format it cannot read.
func TestSupportedImageFormats_DecoderRegistrationIsInLockstep(t *testing.T) {
	// Every advertised format must actually DECODE, and must decode under the
	// name the map is keyed by (the Content-Type is looked up with that key).
	fixtures := map[string]func(t *testing.T, w, h int) []byte{
		"png":  pngBytes,
		"jpeg": jpegBytes,
	}
	for format := range supportedImageFormats {
		fixture, ok := fixtures[format]
		if !ok {
			t.Fatalf("supportedImageFormats advertises %q but this test has no fixture for it — "+
				"a format was added to the map without proving a decoder is registered for it", format)
		}
		_, _, got, err := decodeImageHeader(bytes.NewReader(fixture(t, 12, 20)), "--image x")
		if err != nil {
			t.Errorf("advertised format %q does not decode: %v — the map and the blank imports have drifted", format, err)
		}
		if got != format {
			t.Errorf("fixture for %q decoded as %q — the map key must be the name image.DecodeConfig reports", format, got)
		}
	}

	// The content types are what the upload POST is labelled with; assert the
	// VALUES, not just that the keys exist.
	want := map[string]string{"png": "image/png", "jpeg": "image/jpeg"}
	if len(supportedImageFormats) != len(want) {
		t.Errorf("supportedImageFormats has %d entries (%v), want %d — "+
			"if a format was added on purpose, add it to this test AND check the unreachability comment in decodeImageHeader is still true",
			len(supportedImageFormats), supportedImageFormats, len(want))
	}
	for k, v := range want {
		if supportedImageFormats[k] != v {
			t.Errorf("supportedImageFormats[%q] = %q, want %q", k, supportedImageFormats[k], v)
		}
	}

	// The user-facing list must name exactly the advertised formats. It is a
	// hand-written string (a map range would reorder between runs), so it is the
	// half most likely to go stale.
	list := supportedImageFormatList()
	for format := range supportedImageFormats {
		if !strings.Contains(list, format) {
			t.Errorf("supportedImageFormatList() = %q does not mention the advertised format %q", list, format)
		}
	}

	// NEGATIVE CONTROL: an unregistered format fails EARLY, with ErrFormat —
	// which is exactly why the allowlist branch cannot be reached. If this ever
	// stops being ErrFormat, the unreachability comment in decodeImageHeader is
	// wrong and must be revisited.
	_, _, gifFormat, err := decodeImageHeader(bytes.NewReader(gifBytes()), "--image x")
	if err == nil {
		t.Fatal("gif was accepted — a decoder for it has been registered somewhere in this test binary")
	}
	if gifFormat != "" {
		t.Errorf("gif decoded as %q; the allowlist branch may now be reachable", gifFormat)
	}
	if !strings.Contains(err.Error(), "not an image this CLI recognises") {
		t.Errorf("gif failed via the wrong arm (%v) — the unreachability argument for the allowlist branch depends on this being the ErrFormat arm", err)
	}
}

// --- response-body handle leaks -----------------------------------------------

// closeRecorder is a response body that records whether it was closed. The
// recording happens on the side THIS TEST owns — not on the server — so the
// observation is synchronous with the call under test and cannot race.
type closeRecorder struct {
	mu     sync.Mutex
	reader io.Reader
	closed int
}

func newCloseRecorder(data []byte) *closeRecorder {
	return &closeRecorder{reader: bytes.NewReader(data)}
}

func (c *closeRecorder) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (c *closeRecorder) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return nil
}

func (c *closeRecorder) closeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// 🔴 resolveRemoteImage must close the response body on EVERY path.
//
// Surviving mutation: delete `defer resp.Body.Close()`. A leaked body holds the
// connection out of the pool; with `--image` passed several times that is one
// leak per reference image, and the fetch happens before a spend the user is
// still deciding on.
//
// The fetcher seam returns a response this test constructed, so "was it closed?"
// is answered by a counter in this process at the moment resolveRemoteImage
// returns. There is no server, no goroutine and no timing dependency, which is
// what keeps it deterministic.
func TestResolveRemoteImage_ClosesTheResponseBody(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   []byte
		wantOK bool
	}{
		{"success", http.StatusOK, pngBytes(t, 64, 48), true},
		{"non-2xx", http.StatusForbidden, []byte("nope"), false},
		{"2xx but undecodable", http.StatusOK, []byte("not an image at all"), false},
		{"2xx but degenerate dimensions", http.StatusOK, jpegBytes(t, 16, 0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newCloseRecorder(tc.body)
			s := &genSeams{downloadBlob: func(context.Context, string) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: rec, Header: make(http.Header)}, nil
			}}

			_, err := resolveImages(context.Background(), s.deps(t), []string{"https://example.com/pic.png"})
			if tc.wantOK && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("want an error for this case")
			}

			// 🔴 THE assertion, on every path.
			if got := rec.closeCount(); got != 1 {
				t.Errorf("🔴 HANDLE LEAK: the response body was closed %d time(s), want exactly 1", got)
			}
		})
	}

	// POSITIVE CONTROL for the recorder: it reports 0 when nothing closes it, so
	// a "1" above is a real observation and not a counter stuck at one.
	t.Run("control: the recorder can report zero", func(t *testing.T) {
		rec := newCloseRecorder([]byte("x"))
		if got := rec.closeCount(); got != 0 {
			t.Fatalf("CONTROL FAILED: a never-closed recorder reports %d, want 0", got)
		}
		_ = rec.Close()
		if got := rec.closeCount(); got != 1 {
			t.Fatalf("CONTROL FAILED: recorder counted %d closes after one Close(), want 1", got)
		}
	})
}

// Every reference image gets its body closed, not just the first — the leak this
// guards against is per-image, so a loop is where it bites.
func TestResolveRemoteImage_ClosesEveryBodyAcrossMultipleImages(t *testing.T) {
	const n = 4
	var recs []*closeRecorder
	s := &genSeams{downloadBlob: func(context.Context, string) (*http.Response, error) {
		rec := newCloseRecorder(pngBytes(t, 64, 48))
		recs = append(recs, rec)
		return &http.Response{StatusCode: http.StatusOK, Body: rec, Header: make(http.Header)}, nil
	}}

	urls := make([]string, 0, n)
	for i := 0; i < n; i++ {
		urls = append(urls, fmt.Sprintf("https://example.com/pic%d.png", i))
	}
	if _, err := resolveImages(context.Background(), s.deps(t), urls); err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if len(recs) != n {
		t.Fatalf("the fetch seam was reached %d time(s), want %d", len(recs), n)
	}
	for i, rec := range recs {
		if got := rec.closeCount(); got != 1 {
			t.Errorf("🔴 HANDLE LEAK: image %d's body was closed %d time(s), want 1", i, got)
		}
	}
}
