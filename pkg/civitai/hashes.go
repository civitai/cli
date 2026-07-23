package civitai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// batchByHashPath is the batch hash-lookup endpoint. It accepts a JSON array of
// uppercase 64-char SHA256 hashes and returns the [{modelVersionId, modelId,
// hash}] entries that matched (misses are simply absent from the response).
const batchByHashPath = "/api/v1/model-versions/by-hash/ids"

// maxHashesPerBatch is the server-enforced cap on how many hashes may be sent in
// a single POST to batchByHashPath. GetModelVersionsByHashes chunks any larger
// input into consecutive requests, so one call handles a slice of any size.
const maxHashesPerBatch = 10000

// sha256HexLen is the length of a SHA256 digest rendered as hex (32 bytes).
const sha256HexLen = 64

// HashMatch is one entry of the batch by-hash response: the model version (and
// its parent model) a given file hash resolves to. Field names track the live
// endpoint EXACTLY (modelVersionId, modelId, hash). Hash is echoed back
// UPPER-cased, matching what the request sent.
type HashMatch struct {
	ModelVersionID int    `json:"modelVersionId"`
	ModelID        int    `json:"modelId"`
	Hash           string `json:"hash"`
}

// GetModelVersionsByHashes resolves a batch of file hashes to their model
// versions via POST /api/v1/model-versions/by-hash/ids — the batch replacement
// for N sequential GET /by-hash/{hash} calls.
//
// Each hash is normalized to upper-case and validated as a 64-char hex SHA256;
// entries that are not a valid SHA256 are SKIPPED (not sent) so one malformed
// entry never fails the whole batch — the caller simply gets no match for it,
// which is indistinguishable from a genuine miss. The surviving hashes are
// chunked to <= maxHashesPerBatch per request (the endpoint cap), each chunk is
// POSTed as a bare JSON array body, and the results are merged in request order.
//
// Empty input — or input with no valid hashes — makes NO request and returns an
// empty result. Misses (a hash with no matching version) are absent from the
// result, exactly as the endpoint reports them; a hash shared across versions
// may appear in MORE than one HashMatch. Transient failures (429 with
// Retry-After, 502/503/504, transient network errors) are retried with bounded
// backoff and classified identically to the GET read path (ErrRateLimited /
// ErrNetwork / …); ctx cancellation aborts promptly.
func (c *Client) GetModelVersionsByHashes(ctx context.Context, hashes []string) ([]HashMatch, error) {
	norm := make([]string, 0, len(hashes))
	for _, h := range hashes {
		h = strings.ToUpper(strings.TrimSpace(h))
		if isSHA256Hex(h) {
			norm = append(norm, h)
		}
	}
	if len(norm) == 0 {
		return nil, nil
	}
	var out []HashMatch
	for start := 0; start < len(norm); start += maxHashesPerBatch {
		end := start + maxHashesPerBatch
		if end > len(norm) {
			end = len(norm)
		}
		var matches []HashMatch
		if _, err := c.postInto(ctx, batchByHashPath, norm[start:end], &matches); err != nil {
			return nil, err
		}
		out = append(out, matches...)
	}
	return out, nil
}

// isSHA256Hex reports whether s is a 64-char hex string (a SHA256 digest). The
// input is already upper-cased by the caller, so only 0-9/A-F are accepted.
func isSHA256Hex(s string) bool {
	if len(s) != sha256HexLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// postInto POSTs body (JSON-encoded) to path and, on a 2xx, unmarshals the
// response into out (when non-nil), returning the raw body. It mirrors getInto:
// the same readError status classification, the same maxBody cap, and the same
// bounded transient-failure retry/backoff via getWithRetry. Retry is safe here
// because the by-hash lookup is idempotent and the build closure re-creates the
// body reader on every attempt.
func (c *Client) postInto(ctx context.Context, path string, body, out any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encoding request body for %s: %w", path, err)
	}
	full := c.BaseURL + path
	build := func() (*http.Request, error) {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(payload))
		if rerr != nil {
			return nil, rerr
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}
	status, raw, err := c.getWithRetry(ctx, path, build)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, readError(status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, fmt.Errorf("unexpected response from %s (status %d): %s", path, status, snippet(raw))
		}
	}
	return raw, nil
}
