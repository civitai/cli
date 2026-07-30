package appapi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG DecodeConfig
	_ "image/png"  // register PNG DecodeConfig
)

// ImageInfo is the minimal media metadata the full-resolution persist path
// (`persistAssetImage`) needs: pixel dimensions + the wire MIME type.
type ImageInfo struct {
	Width    int
	Height   int
	MimeType string // "image/png" | "image/jpeg" | "image/webp"
}

// DecodeImageInfo reads width/height/MIME from an image HEADER without a full
// decode. PNG/JPEG go through the stdlib `image.DecodeConfig`; WebP (which the
// stdlib does not register) is parsed from its RIFF/VP8 container header. It
// returns an error for an unrecognized, unsupported, or truncated image so the
// caller can fail cleanly BEFORE any upload.
//
// Only png/jpeg/webp are accepted — the exact set the listing-asset attach
// validation allows (`LISTING_ASSET_ALLOWED_MIME`).
func DecodeImageInfo(data []byte) (ImageInfo, error) {
	// WebP first: the stdlib doesn't register it, and its RIFF/WEBP container is
	// unambiguous, so detect + parse it before falling through to DecodeConfig.
	if len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return decodeWebPInfo(data)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ImageInfo{}, fmt.Errorf("unrecognized image (want png, jpeg, or webp): %w", err)
	}
	switch format {
	case "png":
		return ImageInfo{Width: cfg.Width, Height: cfg.Height, MimeType: "image/png"}, nil
	case "jpeg":
		return ImageInfo{Width: cfg.Width, Height: cfg.Height, MimeType: "image/jpeg"}, nil
	default:
		return ImageInfo{}, fmt.Errorf("unsupported image format %q (want png, jpeg, or webp)", format)
	}
}

// decodeWebPInfo parses canvas dimensions from a WebP RIFF header. It supports
// the three sub-formats: VP8 (lossy simple), VP8L (lossless), and VP8X
// (extended, which carries the canvas size directly). Bytes 0..11 are the RIFF
// + WEBP magic (already checked by the caller); byte 12 begins the first chunk
// FourCC, byte 16 its 4-byte size, byte 20 its payload.
func decodeWebPInfo(data []byte) (ImageInfo, error) {
	if len(data) < 16 {
		return ImageInfo{}, fmt.Errorf("truncated webp header")
	}
	switch string(data[12:16]) {
	case "VP8 ": // lossy simple
		// payload@20: 3-byte frame tag, then key-frame start code 0x9d 0x01 0x2a,
		// then 14-bit width + 14-bit height (each a 16-bit little-endian field).
		if len(data) < 30 {
			return ImageInfo{}, fmt.Errorf("truncated VP8 header")
		}
		if data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return ImageInfo{}, fmt.Errorf("bad VP8 key-frame start code")
		}
		w := int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff)
		return ImageInfo{Width: w, Height: h, MimeType: "image/webp"}, nil
	case "VP8L": // lossless
		// payload@20: 1 signature byte 0x2f, then 4 bytes packing
		// (width-1):14, (height-1):14, ... little-endian.
		if len(data) < 25 {
			return ImageInfo{}, fmt.Errorf("truncated VP8L header")
		}
		if data[20] != 0x2f {
			return ImageInfo{}, fmt.Errorf("bad VP8L signature")
		}
		b := binary.LittleEndian.Uint32(data[21:25])
		w := int(b&0x3fff) + 1
		h := int((b>>14)&0x3fff) + 1
		return ImageInfo{Width: w, Height: h, MimeType: "image/webp"}, nil
	case "VP8X": // extended
		// payload@20: 1 byte flags, 3 bytes reserved, 3 bytes (width-1) LE,
		// 3 bytes (height-1) LE.
		if len(data) < 30 {
			return ImageInfo{}, fmt.Errorf("truncated VP8X header")
		}
		w := int(uint32(data[24])|uint32(data[25])<<8|uint32(data[26])<<16) + 1
		h := int(uint32(data[27])|uint32(data[28])<<8|uint32(data[29])<<16) + 1
		return ImageInfo{Width: w, Height: h, MimeType: "image/webp"}, nil
	default:
		return ImageInfo{}, fmt.Errorf("unrecognized webp chunk %q", string(data[12:16]))
	}
}
