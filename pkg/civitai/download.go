package civitai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
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
// silent server can't hang forever, and (unless AllowPrivateDownloadHosts) an
// SSRF dial guard that refuses any connection whose RESOLVED IP is internal
// (loopback / link-local / private / ULA). Redirects follow up to 10 hops
// (stripping the Authorization header on a cross-host redirect) and each hop's
// scheme is re-checked to be https by checkDownloadRedirect.
//
// The SSRF guard lives in the DIALER (net.Dialer.Control), so it inspects the
// post-DNS address of EVERY connection — the initial downloadUrl and every
// redirect target alike — which is DNS-rebinding-resistant (a hostname that
// resolves to a public IP on the guard check but an internal one at dial time is
// still caught, because the check IS the dial).
func (c *Client) downloadHTTPClient() *http.Client {
	var base http.RoundTripper
	if c.HTTP != nil {
		base = c.HTTP.Transport
	}
	tr := downloadTransport(base, downloadResponseHeaderTimeout)
	if !c.AllowPrivateDownloadHosts {
		if t, ok := tr.(*http.Transport); ok {
			t.DialContext = guardedDownloadDialContext()
		}
	}
	return &http.Client{
		Transport:     tr,
		CheckRedirect: c.checkDownloadRedirect,
	}
}

// maxDownloadRedirects mirrors Go's default redirect cap; we set our own
// CheckRedirect (to re-assert https per hop), which replaces the default, so we
// re-impose the same bound.
const maxDownloadRedirects = 10

// checkDownloadRedirect is the download client's redirect policy. It re-asserts
// the https-only requirement on each redirect TARGET (a hostile downloadUrl
// could 3xx to a plain-http internal endpoint) and re-imposes the 10-hop cap.
// The internal-IP block is handled separately by the dialer, so it need not be
// repeated here. Skipped entirely when AllowPrivateDownloadHosts (test http
// loopback servers redirect http→http).
func (c *Client) checkDownloadRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxDownloadRedirects {
		return fmt.Errorf("stopped after %d redirects", maxDownloadRedirects)
	}
	if c.AllowPrivateDownloadHosts {
		return nil
	}
	return requireHTTPSDownload(req.URL.String())
}

// requireHTTPSDownload refuses a non-https download URL. A server-supplied
// downloadUrl (or redirect target) of http:// could point at an internal
// plain-http endpoint (e.g. a cloud metadata service); legitimate signed
// object-storage URLs are always https, so this never breaks the normal path.
func requireHTTPSDownload(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download URL %q: %w", rawURL, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		scheme := u.Scheme
		if scheme == "" {
			scheme = "(none)"
		}
		return fmt.Errorf("refusing to download over %s — downloads must use https (got %q)", scheme, rawURL)
	}
	return nil
}

// guardedDownloadDialContext returns a DialContext that refuses to connect to an
// internal IP. net.Dialer.Control runs AFTER DNS resolution with the concrete
// ip:port about to be dialed, so it catches both the initial URL and any
// redirect target and is not fooled by DNS rebinding.
func guardedDownloadDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("refusing to download: unparseable dial address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("refusing to download: unresolved dial address %q", address)
			}
			if isBlockedDownloadIP(ip) {
				return &blockedDownloadAddrError{ip: ip}
			}
			return nil
		},
	}
	return d.DialContext
}

// cgnatBlock is RFC 6598 shared address space (100.64.0.0/10) — reachable
// internal-ish space that stdlib's IsPrivate does NOT cover, so block it as an
// SSRF target explicitly.
var cgnatBlock = func() *net.IPNet { _, n, _ := net.ParseCIDR("100.64.0.0/10"); return n }()

// isBlockedDownloadIP reports whether ip is an internal address the download
// client must never connect to: loopback (127.0.0.0/8, ::1), the unspecified
// address (0.0.0.0, :: — 0.0.0.0 routes to loopback on Linux, a classic
// SSRF-filter bypass), link-local (169.254.0.0/16, fe80::/10 — incl. the cloud
// metadata address 169.254.169.254), private (10/8, 172.16/12, 192.168/16),
// IPv6 ULA (fc00::/7), and CGNAT (100.64.0.0/10). Public IPs (a normal
// signed-storage host) pass.
func isBlockedDownloadIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsUnspecified() || // 0.0.0.0 / :: — routes to loopback
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || // RFC1918 + RFC4193 (fc00::/7)
		cgnatBlock.Contains(ip) // RFC6598 100.64.0.0/10
}

// blockedDownloadAddrError is returned by the dial guard when a download would
// connect to an internal IP (SSRF). It carries the offending IP for a clear,
// actionable message.
type blockedDownloadAddrError struct{ ip net.IP }

func (e *blockedDownloadAddrError) Error() string {
	return fmt.Sprintf("refusing to download from a private/loopback address (%s) — the download URL resolved to a non-public IP", e.ip)
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
	if !c.AllowPrivateDownloadHosts {
		if err := requireHTTPSDownload(fileURL); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" && isTrustedDownloadHost(fileURL, c.BaseURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return hc.Do(req)
}

// DownloadNeedsAuth reports whether a bearer token would be attached when
// downloading fileURL (i.e. it targets a trusted Civitai/base host, so the CLI
// authenticates the request). It backs the `download --dry-run` plan's
// "authentication" line. An off-domain signed-storage redirect target reports
// false — it needs no token — but the initial Civitai download route reports
// true, which is what the user sees before the transfer.
func DownloadNeedsAuth(fileURL, baseURL string) bool {
	return isTrustedDownloadHost(fileURL, baseURL)
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
