package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/url"
	"os"
	"strings"

	// Registers the DecodeConfig handlers for the two formats this CLI reads.
	// 🔴 The blank imports ARE the feature list: image.DecodeConfig can only
	// recognise a format whose decoder is registered, so adding a format here is
	// what makes supportedImageFormats true, and removing one silently turns a
	// working `--image` into "unsupported image format". Keep the two in step.
	_ "image/jpeg"
	_ "image/png"

	"github.com/civitai/cli/internal/genapi"
)

// maxReferenceImages is the largest number of reference images ANY image
// ecosystem accepts, and the only image-count bound this CLI asserts.
//
// 🔴 It is deliberately NOT a per-ecosystem table. The real limits span 1..7
// (Boogu / Flux.1 Kontext / MAI / the SD family 1; Qwen / MageFlow 3;
// Reve / HiDream-O1 4; WanImage 5; Flux.2 / Flux.2 Klein / OpenAI / NanoBanana /
// Seedream / Grok 7) and they live ONLY inside the per-ecosystem graph files.
// The server's own helper for this (civitai/civitai ->
// src/shared/data-graph/generation/images-limit.ts) says in its header that a
// flat constant "would simultaneously over-allow the 1-image ecosystems and
// under-allow the 7-image ones", and that copying the spread produces "a
// parallel table that rots the moment a graph file changes" — so it instantiates
// the real graph instead. The CLI cannot do that, and a vendored copy is exactly
// what AGENTS.md item 13 forbids ("the tier limits").
//
// What this constant CAN honestly assert is the ceiling: above 7 the array is
// truncated for EVERY ecosystem, so refusing cannot block a currently-valid
// request. Below it the CLI genuinely does not know, and says so rather than
// guessing — see imageCountNote.
//
// 🔴 The truncation it guards against is SILENT and is charged. The images node
// applies `arr.slice(0, max)` on INPUT, before the output schema's `.max()`
// check, so an over-limit array can never trip the server's own "Maximum N
// images allowed" message — the extras are simply gone. Measured against
// civitai.com on Qwen (max 3): 4, 5, 6 and 12 images all priced 60 with an
// identical `images: 1.2` cost factor, byte-identical to 3 images.
const maxReferenceImages = 7

// maxImageFileBytes bounds a local --image file. It mirrors the platform's own
// 64 MiB consumer-blob limit (civitai/civitai -> src/utils/consumer-blob-upload.ts).
//
// It is enforced by os.Stat BEFORE any read, so an oversized (or accidentally
// enormous) file fails in milliseconds with an actionable message instead of
// being pulled into memory — UploadPresigned needs the whole body resident to
// set a real Content-Length, so "read it and see" would mean allocating first
// and complaining afterwards.
const maxImageFileBytes = 64 << 20

// imageHeaderProbeBytes bounds how much of a REMOTE image is fetched to read its
// dimensions. image.DecodeConfig reads only the header, and 64 KiB comfortably
// covers a PNG IHDR (33 bytes in) and a JPEG SOFn frame even behind a large EXIF
// block. The transfer is stopped at that point rather than streaming a whole
// file the CLI does not need.
const imageHeaderProbeBytes = 64 << 10

// supportedImageFormats lists the formats whose decoders are registered above,
// mapped to the Content-Type the orchestrator expects on the upload POST.
//
// The content type comes from the DECODED format, never from the filename
// extension: a `.png` that is really a JPEG would otherwise be uploaded
// mislabelled and rejected (or worse, stored wrong) with an error that blames
// the wrong thing.
//
// webp is absent on purpose even though the platform accepts it — Go's standard
// library has no webp decoder and golang.org/x/image is a new third-party
// dependency, which AGENTS.md puts behind maintainer approval.
var supportedImageFormats = map[string]string{
	"png":  "image/png",
	"jpeg": "image/jpeg",
}

// supportedImageFormatList renders the accepted formats for an error message, in
// a stable order (a map range is randomised, and an error whose text reorders
// between runs is a bad error).
func supportedImageFormatList() string { return "png, jpeg" }

// imageSource is one parsed --image value: either a local file or a remote URL.
type imageSource struct {
	raw string
	// url is set when the value parsed as an https URL; path is set otherwise.
	url  string
	path string
}

func (s imageSource) isRemote() bool { return s.url != "" }

// parseImageFlag classifies one --image value as a URL or a local path.
//
// The discriminator is an explicit scheme, not os.Stat: a path that does not
// exist must report "no such file", not be reinterpreted as a URL, and a URL
// typo must not become a filename. Only https is accepted as a remote scheme —
// http:// would send the user's reference image (and the request) over the wire
// in clear, and the SSRF posture of the download client this reuses is
// https-only by design. Anything else with a scheme (file://, data:, ftp://) is
// refused by name rather than silently treated as a relative path.
func parseImageFlag(raw string) (imageSource, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return imageSource{}, asUsageError(errors.New(
			"invalid --image: the value is empty — pass a file path or an https:// URL"))
	}
	u, err := url.Parse(v)
	if err == nil && u.Scheme != "" && u.Host != "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			return imageSource{raw: v, url: v}, nil
		case "http":
			return imageSource{}, asUsageError(fmt.Errorf(
				"invalid --image %q — a reference-image URL must use https, not http", raw))
		default:
			return imageSource{}, asUsageError(fmt.Errorf(
				"invalid --image %q — %q is not a supported URL scheme; pass a local file path or an https:// URL", raw, u.Scheme))
		}
	}
	// A scheme with no host (data:, file:/x) never reaches the branch above.
	if u != nil && u.Scheme != "" && u.Host == "" && !looksLikeWindowsPath(v) {
		return imageSource{}, asUsageError(fmt.Errorf(
			"invalid --image %q — %q is not a supported URL scheme; pass a local file path or an https:// URL", raw, u.Scheme))
	}
	return imageSource{raw: v, path: v}, nil
}

// looksLikeWindowsPath reports whether v is a drive-letter path (C:\pics\a.png),
// which url.Parse reads as scheme "c". Without this a Windows user's perfectly
// ordinary path is rejected as an unsupported URL scheme.
func looksLikeWindowsPath(v string) bool {
	if len(v) < 3 || v[1] != ':' {
		return false
	}
	c := v[0]
	if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') {
		return false
	}
	return v[2] == '\\' || v[2] == '/'
}

// decodeImageHeader reads width/height and the format name from an image's
// leading bytes using image.DecodeConfig.
//
// 🔴 DecodeConfig, never Decode: it parses the header only. Decode would
// materialise every pixel — a 64 MiB JPEG expands to hundreds of MiB of RGBA —
// to learn two integers the header already states.
func decodeImageHeader(r io.Reader, what string) (int, int, string, error) {
	cfg, format, err := image.DecodeConfig(r)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return 0, 0, "", fmt.Errorf(
				"%s is not an image this CLI recognises — supported formats: %s", what, supportedImageFormatList())
		}
		return 0, 0, "", fmt.Errorf(
			"%s could not be read as an image (%w) — the file may be truncated or corrupt; supported formats: %s",
			what, err, supportedImageFormatList())
	}
	// 🔴 DEFENSIVE ONLY — this branch is UNREACHABLE as the package is currently
	// built, and no test can kill a mutation inside it. image.DecodeConfig can
	// only name a format whose decoder is registered, this file registers exactly
	// png and jpeg (the blank imports above), and both are keys of
	// supportedImageFormats — so every input either yields one of those two names
	// or fails earlier with image.ErrFormat. Neutering this `if` therefore leaves
	// the whole suite green, and that is a fact about reachability, not a missing
	// test: forcing it reachable would mean registering a third decoder, which is
	// exactly what gifBytes() in generate_image_test.go documents as the thing
	// that would silently destroy the real unsupported-format test.
	//
	// It is kept because the invariant it depends on is one line away from being
	// broken: adding a blank import WITHOUT adding the map entry makes this the
	// live guard that stops an unlabelled format reaching the upload. That
	// lockstep is what TestSupportedImageFormats_DecoderRegistrationIsInLockstep
	// pins — the reachable half of this rule.
	if _, ok := supportedImageFormats[format]; !ok {
		return 0, 0, "", fmt.Errorf(
			"%s is a %s image, which this CLI cannot attach — supported formats: %s",
			what, format, supportedImageFormatList())
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, "", fmt.Errorf(
			"%s reports a degenerate size (%dx%d) — the server requires positive width and height", what, cfg.Width, cfg.Height)
	}
	return cfg.Width, cfg.Height, format, nil
}

// resolveLocalImage reads a local file, extracts its dimensions and uploads it,
// returning the graph entry that references the stored blob.
func resolveLocalImage(ctx context.Context, deps generateDeps, src imageSource) (genapi.GraphImage, error) {
	what := fmt.Sprintf("--image %s", src.raw)

	info, err := os.Stat(src.path)
	if err != nil {
		if os.IsNotExist(err) {
			return genapi.GraphImage{}, asUsageError(fmt.Errorf(
				"%s: no such file — pass a path that exists, or an https:// URL", what))
		}
		return genapi.GraphImage{}, fmt.Errorf("%s: %w", what, err)
	}
	if info.IsDir() {
		return genapi.GraphImage{}, asUsageError(fmt.Errorf("%s is a directory, not an image file", what))
	}
	// 🔴 Bound BEFORE reading. See maxImageFileBytes.
	if info.Size() > maxImageFileBytes {
		return genapi.GraphImage{}, asUsageError(fmt.Errorf(
			"%s is %d bytes, over the %d-byte (64 MiB) upload limit — nothing was read or uploaded; shrink or re-encode the image first",
			what, info.Size(), int64(maxImageFileBytes)))
	}
	if info.Size() == 0 {
		return genapi.GraphImage{}, asUsageError(fmt.Errorf("%s is empty (0 bytes)", what))
	}

	body, err := os.ReadFile(src.path)
	if err != nil {
		return genapi.GraphImage{}, fmt.Errorf("%s: %w", what, err)
	}
	width, height, format, err := decodeImageHeader(bytes.NewReader(body), what)
	if err != nil {
		return genapi.GraphImage{}, asUsageError(err)
	}
	if deps.uploadImage == nil {
		return genapi.GraphImage{}, fmt.Errorf("%s: no image-upload seam is configured", what)
	}
	blobURL, err := deps.uploadImage(ctx, supportedImageFormats[format], body)
	if err != nil {
		return genapi.GraphImage{}, fmt.Errorf("%s: %w", what, err)
	}
	return genapi.GraphImage{URL: blobURL, Width: width, Height: height}, nil
}

// resolveRemoteImage reads a remote image's dimensions and passes the URL
// through unchanged.
//
// 🔴 Why fetch at all, rather than pass the URL through blind: the server
// REQUIRES width and height (a URL with neither is a hard 400, measured), and it
// derives them from nothing — there is no server-side dimension probe on this
// path. So the choice is "read the header" or "refuse every URL". Reading a
// bounded prefix is strictly better: it is a few KiB, it uses the SAME
// credential-free, SSRF-guarded, https-only transport as blob downloads (no new
// HTTP path), and it independently proves the URL is publicly fetchable — which
// the orchestrator itself demands (measured: an unfetchable URL returns
// `400 Input image failed to download from URL`, and the CLI turning that into a
// local error is the difference between a clear message and a mid-submit
// failure).
func resolveRemoteImage(ctx context.Context, deps generateDeps, src imageSource) (genapi.GraphImage, error) {
	what := fmt.Sprintf("--image %s", src.raw)
	if deps.downloadBlob == nil {
		return genapi.GraphImage{}, fmt.Errorf("%s: no image-fetch seam is configured", what)
	}
	resp, err := deps.downloadBlob(ctx, src.url)
	if err != nil {
		return genapi.GraphImage{}, fmt.Errorf("%s could not be fetched: %w", what, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return genapi.GraphImage{}, asUsageError(fmt.Errorf(
			"%s returned HTTP %d — the URL must be publicly fetchable, because the generator downloads it server-side too",
			what, resp.StatusCode))
	}
	head, err := io.ReadAll(io.LimitReader(resp.Body, imageHeaderProbeBytes))
	if err != nil {
		return genapi.GraphImage{}, fmt.Errorf("%s could not be read: %w", what, err)
	}
	width, height, _, err := decodeImageHeader(bytes.NewReader(head), what)
	if err != nil {
		return genapi.GraphImage{}, asUsageError(err)
	}
	return genapi.GraphImage{URL: src.url, Width: width, Height: height}, nil
}

// resolveImages turns the --image values into graph entries.
//
// A local path is uploaded; an https URL is passed through. Both paths carry
// real width/height, because the server rejects an entry without them.
func resolveImages(ctx context.Context, deps generateDeps, raws []string) ([]genapi.GraphImage, error) {
	if len(raws) == 0 {
		return nil, nil
	}
	out := make([]genapi.GraphImage, 0, len(raws))
	for _, raw := range raws {
		src, err := parseImageFlag(raw)
		if err != nil {
			return nil, err
		}
		var img genapi.GraphImage
		if src.isRemote() {
			img, err = resolveRemoteImage(ctx, deps, src)
		} else {
			img, err = resolveLocalImage(ctx, deps, src)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, img)
	}
	return out, nil
}

// imageCountNote is the honest half of the cap story: a warning, printed once,
// that says what the CLI does NOT know.
//
// It fires for more than one image because that is the whole range in which the
// server may silently truncate and the CLI cannot tell. It never blocks — see
// maxReferenceImages for why a per-ecosystem refusal would be a vendored mirror,
// and AGENTS.md item 13 for why this CLI does not build those.
func imageCountNote(n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf(
		"%d reference images passed. Per-ecosystem limits range from 1 to %d and are NOT knowable from here — "+
			"if this ecosystem accepts fewer, the server DROPS the extras silently, prices the truncated job and charges for it. "+
			"Check the estimate below reflects the images you expect.",
		n, maxReferenceImages)
}
