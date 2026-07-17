package api

import (
	"context"
	"net/http"
)

// Downloader streams a file's bytes from a (possibly redirecting) URL. Behind an
// interface so the command layer stays testable without a live server.
type Downloader interface {
	// DownloadFile issues an authenticated GET to fileURL and returns the open
	// HTTP response for streaming. The CALLER owns resp.Body and MUST close it.
	// Redirects are followed automatically; the Civitai `/api/download` route
	// 302s to signed object storage on a DIFFERENT host, and Go's default client
	// strips the Authorization header on a cross-host redirect (exactly what we
	// want — the signed URL needs no bearer). A 401 with a refreshable token
	// source is retried once after a transparent refresh.
	DownloadFile(ctx context.Context, fileURL string) (*http.Response, error)
}

// downloadHTTPClient returns an *http.Client tuned for large streamed downloads:
// NO overall timeout (a 10+ GB file legitimately runs for a long time; the
// caller's context governs cancellation instead), reusing the shared client's
// Transport, and the DEFAULT redirect policy (follow up to 10 hops, stripping
// the Authorization header on a cross-host redirect).
func (c *Client) downloadHTTPClient() *http.Client {
	var transport http.RoundTripper
	if c.HTTP != nil {
		transport = c.HTTP.Transport
	}
	return &http.Client{Transport: transport}
}

// DownloadFile implements Downloader. See the interface doc for the contract.
func (c *Client) DownloadFile(ctx context.Context, fileURL string) (*http.Response, error) {
	hc := c.downloadHTTPClient()
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.doDownload(ctx, hc, fileURL, token)
	if err != nil {
		return nil, err
	}
	// One transparent refresh + retry on a 401 (OAuth access token expired).
	// A non-refreshable (personal-key) source returns ErrNoRefresh and we keep
	// the 401 response for the caller to map to actionable guidance.
	if resp.StatusCode == http.StatusUnauthorized {
		if newTok, rerr := c.Tokens.Refresh(ctx); rerr == nil && newTok != "" {
			_ = resp.Body.Close()
			return c.doDownload(ctx, hc, fileURL, newTok)
		}
	}
	return resp, nil
}

// doDownload builds + issues a single GET with the given bearer token (attached
// only when non-empty, so anonymous downloads send no Authorization header).
func (c *Client) doDownload(ctx context.Context, hc *http.Client, fileURL, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return hc.Do(req)
}
