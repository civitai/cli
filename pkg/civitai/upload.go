package civitai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// PresignedUploader POSTs bytes to a URL that is ALREADY AUTHORIZED by its own
// signature, with NO credential attached.
//
// 🔴 It is the write-side twin of PresignedDownloader, and it exists for the
// SAME reason (see the PresignedDownloader doc and AGENTS.md item 17): the
// orchestrator's consumer-blob upload URL is served from a *.civitai.com host,
// which isTrustedDownloadHost matches — so any code path that carries a token
// would hand a full-scope personal API key to a request whose own signature
// already authorizes it.
//
// This interface has NO token parameter and no TokenSource is consulted on the
// way through, so there is nothing to attach and nothing a later change to the
// host predicate could re-attach. That is the point: the safety is structural,
// not conditional.
//
// The CALLER owns resp.Body and MUST close it.
type PresignedUploader interface {
	UploadPresigned(ctx context.Context, uploadURL, contentType string, body []byte) (*http.Response, error)
}

// UploadPresigned implements PresignedUploader.
//
// It reuses downloadHTTPClient() wholesale, so the upload inherits — unchanged,
// with no second copy to drift — the SSRF dial guard (every connection's
// POST-DNS ip:port is checked, so a hostile or misconfigured upload host that
// resolves to loopback/RFC1918/link-local/CGNAT is refused at dial time), the
// https-per-redirect-hop policy, the 10-hop cap and the ResponseHeaderTimeout.
// The scheme of the initial URL is checked by the same requireHTTPSTransfer
// predicate the download path uses.
//
// The METHOD is POST, not PUT: the orchestrator's presigned consumer-blob URL
// points at `POST /v2/consumer/blobs` (civitai/civitai ->
// src/utils/consumer-blob-upload.ts). A PUT is not an equivalent spelling.
//
// The body is taken as a []byte rather than an io.Reader on purpose. The
// request must carry a real Content-Length (the orchestrator's signed endpoint
// rejects a chunked body), and it must be replayable byte-for-byte if the
// transport retries a hop — neither is guaranteed for an arbitrary reader. The
// caller is responsible for bounding the size before it gets here; see
// internal/cmd/generate_image.go, which refuses an over-limit file by stat
// BEFORE reading a single byte.
func (c *Client) UploadPresigned(ctx context.Context, uploadURL, contentType string, body []byte) (*http.Response, error) {
	if !c.AllowPrivateDownloadHosts {
		if err := requireHTTPSTransfer(uploadURL, "upload", "uploads"); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType == "" {
		return nil, fmt.Errorf("refusing to upload without a Content-Type — the orchestrator rejects an unlabelled blob")
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))
	return c.downloadHTTPClient().Do(req)
}
