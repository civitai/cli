package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
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

// downloadResponseHeaderTimeout bounds how long the download client waits for a
// server to send response HEADERS after the connection is accepted. There is
// deliberately NO overall client Timeout (a large file legitimately streams for
// a long time), so without this a server that accepts the connection then never
// responds would hang the download forever. Body streaming remains governed by
// the caller's (SIGINT-bound) context.
const downloadResponseHeaderTimeout = 30 * time.Second

// downloadHTTPClient returns an *http.Client tuned for large streamed downloads:
// NO overall timeout (a 10+ GB file legitimately runs for a long time; the
// caller's context governs cancellation instead), a ResponseHeaderTimeout so a
// silent server can't hang forever, and the DEFAULT redirect policy (follow up
// to 10 hops, stripping the Authorization header on a cross-host redirect).
func (c *Client) downloadHTTPClient() *http.Client {
	var base http.RoundTripper
	if c.HTTP != nil {
		base = c.HTTP.Transport
	}
	return &http.Client{Transport: downloadTransport(base, downloadResponseHeaderTimeout)}
}

// downloadTransport returns a RoundTripper carrying a ResponseHeaderTimeout. When
// base is an *http.Transport (or nil, meaning the shared default) it is CLONED —
// never mutated — before the timeout is set, so shared transport state is left
// untouched. A non-*http.Transport base (e.g. a test round-tripper) is returned
// as-is (it governs its own timeouts).
func downloadTransport(base http.RoundTripper, headerTimeout time.Duration) http.RoundTripper {
	var src *http.Transport
	switch t := base.(type) {
	case *http.Transport:
		src = t
	case nil:
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			src = dt
		}
	default:
		return base
	}
	if src == nil {
		return base
	}
	clone := src.Clone()
	clone.ResponseHeaderTimeout = headerTimeout
	return clone
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

// doDownload builds + issues a single GET. The bearer token is attached ONLY
// when non-empty AND the destination is a trusted host+scheme (see
// isTrustedDownloadHost): f.DownloadURL is SERVER-SUPPLIED, so a hostile value
// must never be able to exfiltrate the token to an arbitrary host. An off-domain
// or non-https URL (e.g. a signed object-storage redirect target) is fetched
// WITHOUT the Authorization header — it needs none.
func (c *Client) doDownload(ctx context.Context, hc *http.Client, fileURL, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" && isTrustedDownloadHost(fileURL, c.BaseURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return hc.Do(req)
}

// isTrustedDownloadHost reports whether the bearer token may be attached to a GET
// for fileURL. The token is attached only when EITHER:
//   - fileURL is https AND its host is civitai.com or a *.civitai.com subdomain
//     (an exact dotted-suffix match, so "evilcivitai.com" and
//     "civitai.com.evil.com" are both rejected — never a naive substring test),
//     OR
//   - fileURL is same-scheme + same-host as the configured API base URL (the
//     endpoint the user explicitly pointed the CLI at — trusted by definition;
//     this also lets a self-hosted / test API authenticate its own downloads).
//
// An off-domain signed-storage redirect target matches neither and is fetched
// without the token, matching Go's own cross-host Authorization stripping.
func isTrustedDownloadHost(fileURL, baseURL string) bool {
	u, err := url.Parse(fileURL)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Hostname() // strips any :port
	if u.Scheme == "https" && (host == "civitai.com" || strings.HasSuffix(host, ".civitai.com")) {
		return true
	}
	if b, berr := url.Parse(baseURL); berr == nil && b.Host != "" {
		if u.Scheme == b.Scheme && u.Host == b.Host {
			return true
		}
	}
	return false
}
