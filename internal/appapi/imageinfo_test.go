package appapi

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

// webpVP8X builds a minimal VP8X (extended) WebP header carrying canvas dims.
func webpVP8X(w, h int) []byte {
	b := make([]byte, 30)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8X")
	// b[16:20] chunk size, b[20] flags, b[21:24] reserved
	wm1 := uint32(w - 1)
	hm1 := uint32(h - 1)
	b[24], b[25], b[26] = byte(wm1), byte(wm1>>8), byte(wm1>>16)
	b[27], b[28], b[29] = byte(hm1), byte(hm1>>8), byte(hm1>>16)
	return b
}

// webpVP8L builds a minimal VP8L (lossless) WebP header.
func webpVP8L(w, h int) []byte {
	b := make([]byte, 25)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8L")
	b[20] = 0x2f
	packed := uint32(w-1) | uint32(h-1)<<14
	binary.LittleEndian.PutUint32(b[21:25], packed)
	return b
}

// webpVP8 builds a minimal VP8 (lossy) WebP header.
func webpVP8(w, h int) []byte {
	b := make([]byte, 30)
	copy(b[0:4], "RIFF")
	copy(b[8:12], "WEBP")
	copy(b[12:16], "VP8 ")
	b[23], b[24], b[25] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(b[26:28], uint16(w))
	binary.LittleEndian.PutUint16(b[28:30], uint16(h))
	return b
}

func TestDecodeImageInfo(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		wantW    int
		wantH    int
		wantMime string
	}{
		{"png", pngBytes(t, 256, 256), 256, 256, "image/png"},
		{"jpeg", jpegBytes(t, 640, 480), 640, 480, "image/jpeg"},
		{"webp-vp8x", webpVP8X(1920, 1080), 1920, 1080, "image/webp"},
		{"webp-vp8l", webpVP8L(300, 200), 300, 200, "image/webp"},
		{"webp-vp8", webpVP8(128, 128), 128, 128, "image/webp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := DecodeImageInfo(tc.data)
			if err != nil {
				t.Fatalf("DecodeImageInfo: %v", err)
			}
			if info.Width != tc.wantW || info.Height != tc.wantH || info.MimeType != tc.wantMime {
				t.Errorf("got %+v, want %dx%d %s", info, tc.wantW, tc.wantH, tc.wantMime)
			}
		})
	}
}

func TestDecodeImageInfoRejectsNonImage(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("not an image at all"),
		[]byte("GIF89a\x01\x00\x01\x00"), // gif is not in the allowed set
		{},
	} {
		if _, err := DecodeImageInfo(data); err == nil {
			t.Errorf("expected error for %q", data)
		}
	}
}
