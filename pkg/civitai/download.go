package civitai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
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

// PresignedDownloader streams a file from a URL that is ALREADY AUTHORIZED by
// its own signature, with NO credential attached.
//
// 🔴 It exists because reuse of DownloadFile would have leaked a credential.
// isTrustedDownloadHost (below) attaches the bearer token to civitai.com and to
// ANY *.civitai.com subdomain — which is correct for the model-download route it
// was written for, and wrong for an orchestrator blob: those are served from a
// *.civitai.com host and are presigned, so DownloadFile would send a full-scope
// personal API key (25 scopes, including ModelsDelete and VaultWrite) to a
// request that needs no token at all. Weakening isTrustedDownloadHost was not an
// option — `civitai download` depends on it attaching the token — so the fix is
// a SEAM that never has a credential to attach, not a hole in the predicate.
//
// Everything else is shared with DownloadFile: the same SSRF dial guard, the
// same https-per-redirect-hop policy and 10-hop cap, the same
// ResponseHeaderTimeout. The CALLER owns resp.Body and MUST close it.
type PresignedDownloader interface {
	DownloadPresigned(ctx context.Context, fileURL string) (*http.Response, error)
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
// Without a proxy, the SSRF guard lives in the DIALER (net.Dialer.Control), so
// it inspects the post-DNS address of EVERY connection — the initial downloadUrl
// and every redirect target alike — which is DNS-rebinding-resistant (a hostname
// that resolves to a public IP on the guard check but an internal one at dial
// time is still caught, because the check IS the dial).
//
// With a proxy (the transport's Proxy func — HTTPS_PROXY & co. by default —
// names one for the request), the dialer only ever sees the PROXY's address, so
// the dial check alone would refuse every private proxy and wave through every
// target behind a public one (#780). proxyTargetGuard covers that case: see its
// doc for what it checks and the limit it cannot get past.
func (c *Client) downloadHTTPClient() *http.Client {
	var base http.RoundTripper
	if c.HTTP != nil {
		base = c.HTTP.Transport
	}
	tr := downloadTransport(base, downloadResponseHeaderTimeout)
	if !c.AllowPrivateDownloadHosts {
		if t, ok := tr.(*http.Transport); ok {
			t.DialContext = guardedDownloadDialContext()
			if t.Proxy != nil {
				tr = &proxyTargetGuard{next: t, proxy: t.Proxy}
			}
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
	return requireHTTPSTransfer(rawURL, "download", "downloads")
}

// requireHTTPSTransfer is the ONE https-scheme predicate for every transfer this
// package performs — the download path and the presigned UPLOAD path
// (upload.go) alike. The verb/plural are message-only: a second copy of this
// check worded for uploads is exactly the duplicated security logic item 17 of
// AGENTS.md exists to prevent, so callers parameterise the wording rather than
// re-implement the decision.
func requireHTTPSTransfer(rawURL, verb, plural string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid %s URL %q: %w", verb, rawURL, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		scheme := u.Scheme
		if scheme == "" {
			scheme = "(none)"
		}
		return fmt.Errorf("refusing to %s over %s — %s must use https (got %q)", verb, scheme, plural, rawURL)
	}
	return nil
}

// guardedDownloadDialContext returns a DialContext that refuses to connect to an
// internal IP. net.Dialer.Control runs AFTER DNS resolution with the concrete
// ip:port about to be dialed, so it catches both the initial URL and any
// redirect target and is not fooled by DNS rebinding.
//
// The one exception is the proxy address proxyTargetGuard recorded for THIS
// request: that exact host:port (as configured, before DNS) is dialed without
// the private-address check, so a loopback or private proxy works. Nothing else
// is exempt — a different private address on a proxied request is still refused.
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
			if reason := blockedDownloadReason(ip); reason != "" {
				return &blockedDownloadAddrError{ip: ip, reason: reason}
			}
			return nil
		},
	}
	proxyDialer := &net.Dialer{Timeout: d.Timeout, KeepAlive: d.KeepAlive}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if allowed, ok := ctx.Value(allowedProxyAddrKey{}).(string); ok && allowed == addr {
			return proxyDialer.DialContext(ctx, network, addr)
		}
		conn, err := d.DialContext(ctx, network, addr)
		var blocked *blockedDownloadAddrError
		if err != nil && errors.As(err, &blocked) && blocked.host == "" {
			// Name the host that was dialed, not just the IP it resolved to.
			if h, _, serr := net.SplitHostPort(addr); serr == nil {
				blocked.host = h
			}
		}
		return conn, err
	}
}

// allowedProxyAddrKey is the context key under which proxyTargetGuard records
// the canonical host:port of the proxy chosen for a request.
type allowedProxyAddrKey struct{}

// lookupDownloadHost resolves a download target's host for proxyTargetGuard. It
// is the resolver the dial guard's net.Dialer uses (net.DefaultResolver); a
// variable only so tests can map hostnames without real DNS.
var lookupDownloadHost = net.DefaultResolver.LookupIPAddr

// proxyTargetLookupTimeout bounds the proxied pre-check's own DNS lookup. A
// lookup that times out is treated like any other failed lookup: the name is
// passed to the proxy (see proxyTargetGuard). A variable only so tests can
// shorten it.
var proxyTargetLookupTimeout = 3 * time.Second

// proxyTargetGuard applies the private-address rules to the TARGET of a request
// that goes through a proxy (#780). For each request it hands to the transport
// — the first URL and every redirect hop, since http.Client calls RoundTrip
// once per hop — it asks the transport's own Proxy func whether this request is
// proxied. If not, it does nothing and the dial guard checks the target as
// before. If so, it:
//
//  1. classifies the target host (see checkProxiedDownloadTarget) and refuses
//     the request if it is, or resolves here to, ANY private/loopback address,
//     by the same blockedDownloadReason the dial guard uses — before anything
//     is sent; a mixed answer with one private address among public ones is
//     refused; and
//  2. records the proxy's host:port in the request context, so the dial guard
//     lets exactly that address through even when it is private (an agent
//     sandbox's loopback proxy is the case that motivated this).
//
// A real hostname that does not resolve HERE (a lookup error, no addresses, or
// no answer within proxyTargetLookupTimeout) is passed to the proxy unchecked,
// and the proxy resolves it. That is a deliberate
// operator decision: a sandbox whose only route out is the proxy often has no
// DNS of its own, and refusing every such name would block exactly the setup
// #780 is about. It widens the gap below: such a name is not checked at all.
//
// 🔴 THIS IS A BEST-EFFORT PRE-CHECK, NOT THE DIAL-TIME GUARANTEE THE DIRECT PATH
// HAS. Through a proxy the CLI never learns which address the proxy connects to:
// the proxy resolves the target itself, later and possibly against different
// DNS. A name that resolves public here and private at the proxy — DNS
// rebinding with a short TTL, split-horizon DNS where the proxy sees internal
// records, a hosts-file entry on the proxy host — passes this check and is
// reached, and so is a name this machine cannot resolve at all. Closing that
// needs the proxy to enforce the rule. What this does guarantee is narrower: a
// target that resolves to a private address from THIS machine is never
// requested through any proxy, public or private.
type proxyTargetGuard struct {
	next  http.RoundTripper
	proxy func(*http.Request) (*url.URL, error)
}

func (g *proxyTargetGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyURL, err := g.proxy(req)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return g.next.RoundTrip(req)
	}
	if err := checkProxiedDownloadTarget(req.Context(), req.URL.Hostname(), proxyURL); err != nil {
		return nil, err
	}
	ctx := context.WithValue(req.Context(), allowedProxyAddrKey{}, proxyDialAddr(proxyURL))
	return g.next.RoundTrip(req.WithContext(ctx))
}

// checkProxiedDownloadTarget refuses host when it is, or resolves to, any
// private/loopback address. See proxyTargetGuard for why this is a best-effort
// pre-check. ONE trailing dot is stripped first, then the host is classified:
//
//   - an IP literal net.ParseIP accepts, or one containing ':' (IPv6, including
//     a zone, which net.ParseIP rejects), is checked as that address; an
//     unparseable ':' host is refused;
//   - a host made only of numeric or 0x labels is an IP literal in a form the
//     PROXY may accept even though net.ParseIP does not ("2130706433",
//     "0x7f000001", "127.1"). It is parsed the way inet_aton does and checked,
//     and refused when it cannot be parsed. Without this, the pure-Go resolver
//     (CGO_ENABLED=0, how releases are built) fails to look such a host up, and
//     the unresolvable branch below would pass it to the proxy unchecked;
//   - "localhost" and any "*.localhost" name are loopback (RFC 6761);
//   - anything else is a real hostname: it is resolved here, and if it does
//     not resolve, it passes (the proxy resolves it).
func checkProxiedDownloadTarget(ctx context.Context, host string, proxyURL *url.URL) error {
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return fmt.Errorf("refusing to download via proxy %s: the URL has no host", proxyURL.Host)
	}
	refuse := func(ip net.IP, literal bool) error {
		if reason := blockedDownloadReason(ip); reason != "" {
			return &blockedDownloadAddrError{host: host, ip: ip, reason: reason, proxy: proxyURL.Host, literal: literal}
		}
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return refuse(ip, true)
	}
	if strings.Contains(host, ":") {
		a, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("refusing to download from %s via proxy %s: it looks like an IPv6 address but is not a valid one, so it cannot be checked", host, proxyURL.Host)
		}
		return refuse(net.IP(a.WithZone("").AsSlice()), true)
	}
	if isNumericHost(host) {
		ip, ok := parseInetAton(host)
		if !ok {
			return fmt.Errorf("refusing to download from %s via proxy %s: it looks like a numeric IP address but is not a valid one, so it cannot be checked", host, proxyURL.Host)
		}
		return refuse(ip, true)
	}
	if lower := strings.ToLower(host); lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return &blockedDownloadAddrError{host: host, reason: "loopback", proxy: proxyURL.Host}
	}
	lctx, cancel := context.WithTimeout(ctx, proxyTargetLookupTimeout)
	defer cancel()
	addrs, err := lookupDownloadHost(lctx, host)
	if err != nil {
		return nil // unresolvable here (or timed out): the proxy resolves it (see proxyTargetGuard)
	}
	for _, a := range addrs {
		if err := refuse(a.IP, false); err != nil {
			return err
		}
	}
	return nil
}

// isNumericHost reports whether every dot-separated label of host is a decimal
// number or a 0x/0X hex number (possibly with no digits after the prefix,
// which parseInetAton then rejects). Such a host can only be meant as an IPv4
// literal; no registrable name is all-numeric.
func isNumericHost(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		digits, isDigit := label, func(r rune) bool { return r >= '0' && r <= '9' }
		if len(label) >= 2 && label[0] == '0' && (label[1] == 'x' || label[1] == 'X') {
			digits = label[2:]
			isDigit = func(r rune) bool {
				return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			}
		}
		for _, r := range digits {
			if !isDigit(r) {
				return false
			}
		}
	}
	return true
}

// parseInetAton parses an IPv4 address the way inet_aton(3) does: one to four
// parts, each decimal, 0x-prefixed hex, or 0-prefixed octal; every part but the
// last is one byte and the last fills the remaining bytes ("127.1" is
// 127.0.0.1, "2130706433" is 127.0.0.1). ok is false for anything inet_aton
// would reject, including a part that overflows its bytes.
func parseInetAton(host string) (ip net.IP, ok bool) {
	parts := strings.Split(host, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return nil, false
	}
	vals := make([]uint64, len(parts))
	for i, p := range parts {
		var v uint64
		var err error
		switch {
		case len(p) >= 2 && p[0] == '0' && (p[1] == 'x' || p[1] == 'X'):
			if len(p) == 2 {
				return nil, false
			}
			v, err = strconv.ParseUint(p[2:], 16, 64)
		case len(p) > 1 && p[0] == '0':
			v, err = strconv.ParseUint(p[1:], 8, 64)
		default:
			v, err = strconv.ParseUint(p, 10, 64)
		}
		if err != nil {
			return nil, false
		}
		vals[i] = v
	}
	n := len(vals)
	for _, v := range vals[:n-1] {
		if v > 0xff {
			return nil, false
		}
	}
	if vals[n-1] > (uint64(1)<<(8*(5-n)))-1 {
		return nil, false
	}
	var addr uint64
	for i, v := range vals[:n-1] {
		addr |= v << (24 - 8*i)
	}
	addr |= vals[n-1]
	return net.IPv4(byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr)), true
}

// proxyDialAddr is the host:port the transport dials for proxyURL, with the
// scheme's default port filled in the way net/http does.
func proxyDialAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
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
	return blockedDownloadReason(ip) != ""
}

// blockedDownloadReason names the internal range ip falls in, or "" when it is a
// public address. It is the one predicate behind isBlockedDownloadIP; the name
// only feeds the refusal message.
func blockedDownloadReason(ip net.IP) string {
	switch {
	case ip.IsLoopback():
		return "loopback"
	case ip.IsUnspecified(): // 0.0.0.0 / :: — routes to loopback
		return "unspecified"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return "link-local"
	case ip.IsPrivate(): // RFC1918 + RFC4193 (fc00::/7)
		return "private"
	case cgnatBlock.Contains(ip): // RFC6598 100.64.0.0/10
		return "CGNAT"
	default:
		return ""
	}
}

// blockedDownloadAddrError is returned when a download would reach an internal
// IP (SSRF): by the dial guard on a direct connection, or by proxyTargetGuard
// for a proxied request's target. It names the host, the address it resolved
// to, which internal range that is, and the proxy when there was one.
type blockedDownloadAddrError struct {
	host    string // the hostname dialed or requested; "" when unknown
	ip      net.IP // nil for a name that is loopback by definition (localhost)
	reason  string // blockedDownloadReason(ip), or "loopback" for localhost
	proxy   string // proxy host:port when the target was checked for a proxied request
	literal bool   // host is an IP literal (ip was parsed from it, not resolved)
}

func (e *blockedDownloadAddrError) Error() string {
	var target, what string
	if e.ip == nil {
		target = e.host
		what = fmt.Sprintf("%s is %s %s name (RFC 6761)", e.host, article(e.reason), e.reason)
	} else {
		target = e.ip.String()
		if e.host != "" && e.host != target {
			verb := "resolved to"
			if e.literal {
				verb = "read as"
			}
			target = fmt.Sprintf("%s (%s %s)", e.host, verb, e.ip)
		}
		what = fmt.Sprintf("%s is %s %s address", e.ip, article(e.reason), e.reason)
	}
	if e.proxy != "" {
		return fmt.Sprintf("refusing to download from %s via proxy %s: %s, and downloads may not reach private/loopback addresses (a private proxy is allowed; a private target is not)", target, e.proxy, what)
	}
	return fmt.Sprintf("refusing to download from %s: %s, and downloads may not reach private/loopback addresses", target, what)
}

// article is "an" before a reason that starts with a vowel ("unspecified"),
// else "a".
func article(word string) string {
	if word != "" && strings.ContainsRune("aeiouAEIOU", rune(word[0])) {
		return "an"
	}
	return "a"
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

// DownloadPresigned implements PresignedDownloader. See the interface doc for
// why this is a separate entry point rather than a flag on DownloadFile.
//
// It passes an EMPTY token to doDownload, so no Authorization header is ever
// built — the check there is `token != "" && isTrustedDownloadHost(...)`, and
// the empty token short-circuits it before the host predicate is even consulted.
// It also does NOT do the 401-refresh replay DownloadFile does: there is no
// credential to refresh, and a 401 from a presigned URL means the signature is
// wrong or expired, which a token would not fix.
func (c *Client) DownloadPresigned(ctx context.Context, fileURL string) (*http.Response, error) {
	return c.doDownload(ctx, c.downloadHTTPClient(), fileURL, "")
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
