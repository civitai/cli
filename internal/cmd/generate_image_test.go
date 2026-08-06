package cmd

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// --- fixtures ----------------------------------------------------------------
//
// Every image fixture is ENCODED IN-TEST rather than committed: a binary in the
// repo is unreviewable, and a hand-made one cannot be varied per case. The
// dimensions are deliberately non-square and pairwise distinct so a transposed
// width/height (or a value copied from the wrong fixture) fails loudly instead
// of matching by coincidence.

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// gifBytes is the UNSUPPORTED-format fixture. It is written as raw header bytes
// on purpose: importing image/gif would REGISTER the gif decoder for the whole
// test binary, so image.DecodeConfig would start recognising it and the
// "unsupported format" path would become unreachable — a test that silently
// stops testing what it names.
func gifBytes() []byte {
	return []byte("GIF89a\x0a\x00\x0a\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff")
}

func writeFixture(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// --- decodeImageHeader -------------------------------------------------------

func TestDecodeImageHeader_ValidFormats(t *testing.T) {
	cases := []struct {
		name       string
		data       []byte
		wantW      int
		wantH      int
		wantFormat string
	}{
		{"png", pngBytes(t, 640, 480), 640, 480, "png"},
		{"png portrait", pngBytes(t, 111, 222), 111, 222, "png"},
		{"jpeg", jpegBytes(t, 832, 1216), 832, 1216, "jpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h, format, err := decodeImageHeader(bytes.NewReader(tc.data), "fixture")
			if err != nil {
				t.Fatalf("decodeImageHeader: %v", err)
			}
			if w != tc.wantW || h != tc.wantH {
				t.Errorf("dimensions = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
			if format != tc.wantFormat {
				t.Errorf("format = %q, want %q", format, tc.wantFormat)
			}
		})
	}
}

func TestDecodeImageHeader_Rejections(t *testing.T) {
	full := pngBytes(t, 320, 240)
	cases := []struct {
		name    string
		data    []byte
		wantSub string
	}{
		{"not an image at all", []byte("this is a text file, not a picture"), "not an image this CLI recognises"},
		{"empty", nil, "not an image this CLI recognises"},
		{"unsupported format (gif)", gifBytes(), "not an image this CLI recognises"},
		// A valid PNG signature followed by nothing: the format IS recognised,
		// so this exercises the truncated/corrupt arm rather than the unknown
		// format arm — a different message, deliberately.
		{"truncated png header", full[:12], "truncated or corrupt"},
		{"truncated jpeg header", jpegBytes(t, 64, 64)[:8], "truncated or corrupt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := decodeImageHeader(bytes.NewReader(tc.data), "--image x")
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message %q lacks %q", err, tc.wantSub)
			}
			// Every rejection must name what IS accepted.
			if !strings.Contains(err.Error(), "png, jpeg") {
				t.Errorf("message %q does not name the supported formats", err)
			}
		})
	}
}

// --- parseImageFlag ----------------------------------------------------------

func TestParseImageFlag(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantURL  string
		wantPath string
		wantErr  string
	}{
		{name: "https url", in: "https://example.com/a.png", wantURL: "https://example.com/a.png"},
		{name: "relative path", in: "./pics/a.png", wantPath: "./pics/a.png"},
		{name: "absolute path", in: "/tmp/a.png", wantPath: "/tmp/a.png"},
		{name: "bare filename", in: "a.png", wantPath: "a.png"},
		{name: "windows path", in: `C:\pics\a.png`, wantPath: `C:\pics\a.png`},
		{name: "trims whitespace", in: "  ./a.png  ", wantPath: "./a.png"},
		{name: "http refused", in: "http://example.com/a.png", wantErr: "must use https"},
		{name: "file scheme refused", in: "file:///etc/passwd", wantErr: "not a supported URL scheme"},
		{name: "data scheme refused", in: "data:image/png;base64,AAAA", wantErr: "not a supported URL scheme"},
		{name: "ftp refused", in: "ftp://example.com/a.png", wantErr: "not a supported URL scheme"},
		{name: "empty refused", in: "   ", wantErr: "the value is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseImageFlag(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want an error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("message %q lacks %q", err, tc.wantErr)
				}
				// Every bad --image value is a LOCAL mistake -> exit 2.
				if !errors.Is(err, ErrUsage) {
					t.Errorf("errors.Is(err, ErrUsage) = false — the exit code is pinned by kind, not text")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseImageFlag(%q): %v", tc.in, err)
			}
			if got.url != tc.wantURL {
				t.Errorf("url = %q, want %q", got.url, tc.wantURL)
			}
			if got.path != tc.wantPath {
				t.Errorf("path = %q, want %q", got.path, tc.wantPath)
			}
			if got.isRemote() != (tc.wantURL != "") {
				t.Errorf("isRemote() = %t", got.isRemote())
			}
		})
	}
}

// --- resolveLocalImage -------------------------------------------------------

func TestResolveLocalImage_UploadsAndCarriesDimensions(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "cat.png", pngBytes(t, 321, 123))

	s := &genSeams{uploadReplyURL: "https://orchestration.civitai.com/v2/consumer/blobs/X.png"}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{p})
	if err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if len(imgs) != 1 {
		t.Fatalf("got %d images, want 1", len(imgs))
	}
	if imgs[0].Width != 321 || imgs[0].Height != 123 {
		t.Errorf("dimensions = %dx%d, want 321x123", imgs[0].Width, imgs[0].Height)
	}
	if imgs[0].URL != "https://orchestration.civitai.com/v2/consumer/blobs/X.png" {
		t.Errorf("URL = %q — a local file must be referenced by its UPLOADED blob, never by its path", imgs[0].URL)
	}
	if s.uploadCalls != 1 {
		t.Fatalf("uploadCalls = %d, want 1", s.uploadCalls)
	}
	if s.lastUploadContentType != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", s.lastUploadContentType)
	}
}

// The Content-Type comes from the DECODED format, not the extension: a JPEG
// named .png must upload as image/jpeg.
func TestResolveLocalImage_ContentTypeFollowsBytesNotExtension(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "lying.png", jpegBytes(t, 64, 48))

	s := &genSeams{}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{p})
	if err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if s.lastUploadContentType != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg — it must follow the bytes, not the .png name", s.lastUploadContentType)
	}
	if imgs[0].Width != 64 || imgs[0].Height != 48 {
		t.Errorf("dimensions = %dx%d, want 64x48", imgs[0].Width, imgs[0].Height)
	}
}

// 🔴 The size bound must fail FAST — by stat, with nothing read and nothing
// uploaded. The fixture is a sparse file, so the test costs no real bytes.
func TestResolveLocalImage_OverSizeLimitFailsWithoutReadingOrUploading(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Sparse: one byte written past the limit sets the size without allocating.
	if _, err := f.WriteAt([]byte{0}, maxImageFileBytes+1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	_ = f.Close()

	s := &genSeams{}
	_, err = resolveImages(context.Background(), s.deps(t), []string{p})
	if err == nil {
		t.Fatal("an over-limit file must be refused")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("errors.Is(err, ErrUsage) = false")
	}
	for _, want := range []string{"64 MiB", "nothing was read or uploaded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err, want)
		}
	}
	if s.uploadCalls != 0 {
		t.Errorf("an over-limit file was uploaded %d time(s)", s.uploadCalls)
	}
}

// POSITIVE CONTROL for the uploadCalls==0 assertions in this file: a file one
// byte UNDER the same boundary is accepted and does upload. This also pins the
// comparison as `>` rather than `>=` — an off-by-one that rejected exactly the
// limit would fail here.
func TestResolveLocalImage_PositiveControl_AtLimitIsAccepted(t *testing.T) {
	dir := t.TempDir()
	// A real (small) PNG padded out to EXACTLY the limit. Trailing bytes after
	// IEND are ignored by DecodeConfig, which only reads the header.
	data := pngBytes(t, 33, 44)
	padded := append(data, make([]byte, maxImageFileBytes-len(data))...)
	p := writeFixture(t, dir, "atlimit.png", padded)
	if fi, err := os.Stat(p); err != nil || fi.Size() != maxImageFileBytes {
		t.Fatalf("fixture is %v bytes, want exactly %d", fi.Size(), int64(maxImageFileBytes))
	}

	s := &genSeams{}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{p})
	if err != nil {
		t.Fatalf("positive control FAILED: a file at exactly the limit was refused: %v", err)
	}
	if s.uploadCalls != 1 {
		t.Fatalf("positive control FAILED: uploadCalls = %d, want 1 — the zeros asserted elsewhere would be meaningless", s.uploadCalls)
	}
	if imgs[0].Width != 33 || imgs[0].Height != 44 {
		t.Errorf("dimensions = %dx%d, want 33x44", imgs[0].Width, imgs[0].Height)
	}
}

func TestResolveLocalImage_Rejections(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		setup   func(t *testing.T) string
		wantSub string
	}{
		{
			name:    "missing file",
			setup:   func(t *testing.T) string { return filepath.Join(dir, "nope.png") },
			wantSub: "no such file",
		},
		{
			name:    "a directory",
			setup:   func(t *testing.T) string { return dir },
			wantSub: "is a directory",
		},
		{
			name:    "empty file",
			setup:   func(t *testing.T) string { return writeFixture(t, dir, "empty.png", nil) },
			wantSub: "is empty",
		},
		{
			name:    "not an image",
			setup:   func(t *testing.T) string { return writeFixture(t, dir, "notes.txt", []byte("hello world")) },
			wantSub: "not an image this CLI recognises",
		},
		{
			name:    "unsupported format",
			setup:   func(t *testing.T) string { return writeFixture(t, dir, "anim.gif", gifBytes()) },
			wantSub: "not an image this CLI recognises",
		},
		{
			name:    "corrupt header",
			setup:   func(t *testing.T) string { return writeFixture(t, dir, "cut.png", pngBytes(t, 100, 100)[:14]) },
			wantSub: "truncated or corrupt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &genSeams{}
			_, err := resolveImages(context.Background(), s.deps(t), []string{tc.setup(t)})
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message %q lacks %q", err, tc.wantSub)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("errors.Is(err, ErrUsage) = false — a bad --image is a local mistake (exit 2)")
			}
			if s.uploadCalls != 0 {
				t.Errorf("a rejected image was uploaded %d time(s)", s.uploadCalls)
			}
		})
	}
}

func TestResolveLocalImage_UploadFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "cat.png", pngBytes(t, 10, 20))

	s := &genSeams{uploadErr: errors.New("the image upload was rejected (500)")}
	_, err := resolveImages(context.Background(), s.deps(t), []string{p})
	if err == nil {
		t.Fatal("an upload failure must surface")
	}
	if !strings.Contains(err.Error(), "the image upload was rejected") {
		t.Errorf("message %q lost the cause", err)
	}
	// It names WHICH --image failed.
	if !strings.Contains(err.Error(), "cat.png") {
		t.Errorf("message %q does not name the offending --image", err)
	}
}

// --- resolveRemoteImage ------------------------------------------------------

// tlsFetcherFor wires the real credential-free downloader at a loopback TLS
// httptest server.
//
// 🔴 It must be a TLS server, not a plain one: parseImageFlag refuses http://,
// so a plain httptest server cannot reach the remote path at all — the test
// would fail at flag parsing and never exercise the fetch. The server's own
// client supplies the self-signed cert's root; AllowPrivateDownloadHosts lets
// the SSRF dial guard through to loopback.
func tlsFetcherFor(t *testing.T, srv *httptest.Server) blobFetcher {
	t.Helper()
	c := civitai.New(srv.URL, "secret-personal-key")
	c.HTTP = srv.Client()
	c.AllowPrivateDownloadHosts = true
	return c.DownloadPresigned
}

// remoteImageFetcher is a blobFetcher backed by a TLS httptest server, so the
// URL path exercises real HTTPS without a live network.
func remoteImageFetcher(t *testing.T, status int, body []byte, calls *int, gotAuth *bool) (blobFetcher, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		_, ok := r.Header["Authorization"]
		*gotAuth = *gotAuth || ok
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return tlsFetcherFor(t, srv), srv.URL + "/pic.png"
}

func TestResolveRemoteImage_PassesURLThroughWithDimensions(t *testing.T) {
	calls, auth := 0, false
	fetch, url := remoteImageFetcher(t, 200, pngBytes(t, 1024, 768), &calls, &auth)

	s := &genSeams{downloadBlob: fetch}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{url})
	if err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if imgs[0].URL != url {
		t.Errorf("URL = %q, want the caller's URL passed through unchanged", imgs[0].URL)
	}
	if imgs[0].Width != 1024 || imgs[0].Height != 768 {
		t.Errorf("dimensions = %dx%d, want 1024x768", imgs[0].Width, imgs[0].Height)
	}
	if calls != 1 {
		t.Fatalf("the URL was fetched %d time(s), want 1", calls)
	}
	// 🔴 A user-supplied URL is a third-party host: no credential may go there.
	if auth {
		t.Error("🔴 CREDENTIAL LEAK: the reference-image fetch carried an Authorization header")
	}
	// 🔴 A URL is passed through, never uploaded.
	if s.uploadCalls != 0 {
		t.Errorf("a remote URL was uploaded %d time(s); it must be referenced as-is", s.uploadCalls)
	}
}

func TestResolveRemoteImage_Rejections(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    []byte
		wantSub string
	}{
		{"404", http.StatusNotFound, nil, "returned HTTP 404"},
		{"403", http.StatusForbidden, nil, "returned HTTP 403"},
		{"500", http.StatusInternalServerError, nil, "returned HTTP 500"},
		{"200 but not an image", http.StatusOK, []byte("<html>oops</html>"), "not an image this CLI recognises"},
		{"200 but truncated", http.StatusOK, pngBytes(t, 50, 50)[:10], "truncated or corrupt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls, auth := 0, false
			fetch, url := remoteImageFetcher(t, tc.status, tc.body, &calls, &auth)
			s := &genSeams{downloadBlob: fetch}

			_, err := resolveImages(context.Background(), s.deps(t), []string{url})
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("message %q lacks %q", err, tc.wantSub)
			}
			if !errors.Is(err, ErrUsage) {
				t.Errorf("errors.Is(err, ErrUsage) = false")
			}
		})
	}
}

// Only a bounded prefix is read from a remote image — the CLI must not stream a
// whole file to learn two integers.
func TestResolveRemoteImage_ReadsOnlyABoundedPrefix(t *testing.T) {
	header := pngBytes(t, 200, 100)
	sent := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(header)
		sent += len(header)
		// Then a long tail. The CLI must stop reading well before the end.
		chunk := make([]byte, 32<<10)
		for i := 0; i < 64; i++ { // 2 MiB of tail
			n, err := w.Write(chunk)
			sent += n
			if err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	s := &genSeams{downloadBlob: tlsFetcherFor(t, srv)}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{srv.URL + "/big.png"})
	if err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if imgs[0].Width != 200 || imgs[0].Height != 100 {
		t.Errorf("dimensions = %dx%d, want 200x100", imgs[0].Width, imgs[0].Height)
	}
}

// --- multiple / mixed --------------------------------------------------------

func TestResolveImages_MixedSourcesPreserveOrder(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "local.png", pngBytes(t, 11, 22))
	calls, auth := 0, false
	fetch, url := remoteImageFetcher(t, 200, jpegBytes(t, 33, 44), &calls, &auth)

	s := &genSeams{downloadBlob: fetch, uploadReplyURL: "https://blob/UP.png"}
	imgs, err := resolveImages(context.Background(), s.deps(t), []string{url, p})
	if err != nil {
		t.Fatalf("resolveImages: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("got %d images, want 2", len(imgs))
	}
	if imgs[0].URL != url || imgs[0].Width != 33 || imgs[0].Height != 44 {
		t.Errorf("images[0] = %+v, want the remote one first", imgs[0])
	}
	if imgs[1].URL != "https://blob/UP.png" || imgs[1].Width != 11 || imgs[1].Height != 22 {
		t.Errorf("images[1] = %+v, want the uploaded local one second", imgs[1])
	}
	if s.uploadCalls != 1 {
		t.Errorf("uploadCalls = %d, want 1 (only the local file uploads)", s.uploadCalls)
	}
}

// --- imageCountNote ----------------------------------------------------------

func TestImageCountNote(t *testing.T) {
	if got := imageCountNote(0); got != "" {
		t.Errorf("imageCountNote(0) = %q, want empty", got)
	}
	if got := imageCountNote(1); got != "" {
		t.Errorf("imageCountNote(1) = %q, want empty — one image can never be truncated", got)
	}
	got := imageCountNote(3)
	if got == "" {
		t.Fatal("imageCountNote(3) must warn")
	}
	for _, want := range []string{"DROPS the extras silently", "charges"} {
		if !strings.Contains(got, want) {
			t.Errorf("note %q lacks %q", got, want)
		}
	}
}
