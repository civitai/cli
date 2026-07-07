// Package dnsprobe resolves a hostname via DNS-over-HTTPS (DoH) to Cloudflare,
// bypassing the OS/local resolver, and builds an *http.Client that dials the
// DoH-resolved IP for that host.
//
// WHY this exists: the dev-tunnel readiness probe GETs the freshly-minted
// ephemeral host `dev-<16hex>.civit.ai` the instant the reverse tunnel binds —
// BEFORE external-dns + Cloudflare have published the record. The OS resolver's
// first lookup returns NXDOMAIN and NEGATIVE-CACHES it for up to the civit.ai
// zone's SOA minimum TTL (1800s / 30 min, which Cloudflare does not let us
// lower). That poisons the OS cache with two consequences:
//  1. the CLI keeps seeing the cached NXDOMAIN and hangs "still waiting…" for up
//     to 30 min even after the record goes live, and
//  2. worse, the SAME poisoned OS-resolver cache breaks the user's BROWSER — when
//     they open civitai.com/apps/dev/<blockId>, the iframe host `dev-*.civit.ai`
//     won't resolve, so the app "doesn't render".
//
// Resolving the probe over DoH (a) lets the CLI see the record as soon as
// Cloudflare has it (no local negative cache) and (b) CRUCIALLY never asks the OS
// resolver about the not-yet-published host, so the OS negative cache is never
// poisoned and the browser's later first query resolves cleanly.
//
// If DoH itself fails (e.g. 443 to Cloudflare is blocked), the client falls back
// to the OS resolver so behavior is never worse than today.
package dnsprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ErrNotPublished is returned by a Resolver (via errors.Is) when the DoH server
// AUTHORITATIVELY reports the name does not exist yet (NXDOMAIN) or resolved with
// no A/AAAA answer — the "DNS hasn't propagated yet" signal. It is DISTINCT from
// a transient DoH failure (network error, non-200, unparseable body), which is
// returned as an ordinary error and triggers the OS-resolver fallback.
var ErrNotPublished = errors.New("dns record not published yet")

// DefaultDoHEndpoint is Cloudflare's DoH JSON endpoint (RFC 8484 JSON form).
const DefaultDoHEndpoint = "https://cloudflare-dns.com/dns-query"

// defaultDoHTimeout bounds a single DoH lookup so a hung Cloudflare request can't
// stall a readiness poll.
const defaultDoHTimeout = 5 * time.Second

// Resolver resolves a hostname to IP addresses. Implementations MUST return
// ErrNotPublished for an authoritative not-found (NXDOMAIN / no address answer)
// and any OTHER error for a transient failure (so the caller can fall back to the
// OS resolver only on transient failures, never masking a real NXDOMAIN).
type Resolver interface {
	Resolve(ctx context.Context, host string) ([]netip.Addr, error)
}

// DoHResolver resolves via a DNS-over-HTTPS JSON endpoint (Cloudflare by default).
type DoHResolver struct {
	// Endpoint is the DoH JSON URL. Empty → DefaultDoHEndpoint.
	Endpoint string
	// Client is the HTTP client used for the DoH request. Nil → a client with
	// defaultDoHTimeout. NOTE: this client resolves the DoH ENDPOINT host
	// (cloudflare-dns.com — a stable, long-published record) via the OS resolver;
	// only the EPHEMERAL tunnel host is resolved over DoH. That is intentional and
	// safe: the OS negative cache is only ever poisoned by a query for the
	// not-yet-published host, which we never send to it.
	Client *http.Client
}

// DefaultResolver is the production resolver: Cloudflare DoH JSON.
var DefaultResolver Resolver = &DoHResolver{}

// dohJSONResponse is the subset of the Cloudflare DoH JSON response we read.
// Status is the DNS RCODE (0 = NOERROR, 3 = NXDOMAIN). Answer holds the records.
type dohJSONResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"` // RR type: 1 = A, 28 = AAAA, 5 = CNAME
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

const (
	rcodeNXDOMAIN = 3
	rrTypeA       = 1
	rrTypeAAAA    = 28
)

// Resolve queries the DoH endpoint for A then (if none) AAAA records of host and
// returns the resolved addresses. An authoritative not-found (NXDOMAIN, or
// NOERROR with no address answer) is reported as ErrNotPublished; any transport
// or protocol failure is returned as an ordinary (transient) error.
func (r *DoHResolver) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return nil, errors.New("dnsprobe: empty host")
	}

	// Query A first (CF-proxied dev hosts always have an A record); if the name
	// exists but has no A, try AAAA before concluding "not published".
	addrs, nxA, err := r.query(ctx, host, "A")
	if err != nil {
		return nil, err
	}
	if len(addrs) > 0 {
		return addrs, nil
	}
	// No A answer. Try AAAA — but if the A query already said NXDOMAIN the name
	// definitively doesn't exist, so skip the extra round-trip.
	if !nxA {
		aaaa, _, aerr := r.query(ctx, host, "AAAA")
		if aerr != nil {
			// A returned clean NODATA but the AAAA follow-up failed TRANSIENTLY — we
			// can't authoritatively conclude "not published" (there might be an AAAA we
			// couldn't fetch). Propagate the transient error so the caller takes the
			// fallback path rather than mis-classifying a blip as authoritative NXDOMAIN.
			return nil, aerr
		}
		if len(aaaa) > 0 {
			return aaaa, nil
		}
	}
	// Clean NXDOMAIN, or clean NOERROR/NODATA on BOTH A and AAAA with no address
	// record yet → authoritatively not published (from the readiness probe's POV).
	return nil, fmt.Errorf("%s: %w", host, ErrNotPublished)
}

// query performs one DoH JSON lookup and returns the address answers (of the
// matching family), whether the response was NXDOMAIN, and a transient error.
func (r *DoHResolver) query(ctx context.Context, host, qtype string) (addrs []netip.Addr, nxdomain bool, err error) {
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = DefaultDoHEndpoint
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: defaultDoHTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	q := req.URL.Query()
	q.Set("name", host)
	q.Set("type", qtype)
	req.URL.RawQuery = q.Encode()
	// The JSON DoH form is selected by the Accept header.
	req.Header.Set("Accept", "application/dns-json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("dnsprobe: DoH %s returned HTTP %d", endpoint, resp.StatusCode)
	}
	var parsed dohJSONResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("dnsprobe: parse DoH response: %w", err)
	}
	if parsed.Status == rcodeNXDOMAIN {
		return nil, true, nil
	}
	want := rrTypeA
	if qtype == "AAAA" {
		want = rrTypeAAAA
	}
	for _, a := range parsed.Answer {
		if a.Type != want {
			continue // skip CNAME/other RRs in the chain
		}
		if ip, perr := netip.ParseAddr(strings.TrimSpace(a.Data)); perr == nil {
			addrs = append(addrs, ip)
		}
	}
	return addrs, false, nil
}

// DialClient resolves host via the given Resolver and returns an *http.Client to
// probe it with, dialing the DoH-resolved IP for host (TLS SNI + the Host header
// are preserved automatically because the request URL still names host — only the
// dial TARGET is overridden). The returned client's requests therefore NEVER
// touch the OS resolver for host.
//
// Behavior:
//   - resolver reports ErrNotPublished  → returns (nil, ErrNotPublished): the
//     caller treats it as the "DNS not up yet" not-ready state.
//   - resolver returns a transient error → FALLS BACK to a plain client that uses
//     the OS resolver (returns (client, nil)) so we are never worse than today.
//   - resolver succeeds                  → returns a client pinned to the resolved
//     IP for host.
//
// timeout bounds the returned client's own requests.
func DialClient(ctx context.Context, r Resolver, host string, timeout time.Duration) (*http.Client, error) {
	if r == nil {
		r = DefaultResolver
	}
	addrs, err := resolveWithRetry(ctx, r, host)
	if err != nil {
		if errors.Is(err, ErrNotPublished) {
			// Authoritative NXDOMAIN/no-answer — surface it so the probe can report
			// the distinct DNS-pending state (and, importantly, we never asked the OS
			// resolver, so its negative cache stays clean for the browser).
			return nil, err
		}
		// A transient DoH failure PERSISTED across the retry (443 blocked, timeout,
		// bad body): fall back to the OS resolver so the probe still works — never
		// worse than before this change. This is the LAST resort.
		return osResolverClient(timeout), nil
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%s: %w", host, ErrNotPublished)
	}
	return pinnedClient(host, addrs, timeout), nil
}

// dohRetryBackoff is the short pause between the first DoH resolve and its single
// retry (bounded by the caller's ctx). Kept small so a lone hiccup on
// cloudflare-dns.com is smoothed over without materially slowing a readiness poll.
const dohRetryBackoff = 150 * time.Millisecond

// resolveWithRetry resolves host, retrying ONCE on a TRANSIENT failure before
// giving up. A transient blip on cloudflare-dns.com must NOT immediately trigger
// the OS-resolver fallback — that fallback re-poisons the OS negative cache for
// the unpublished host, the exact failure this package prevents. An authoritative
// ErrNotPublished is returned immediately and NEVER retried (a real NXDOMAIN must
// not fall back). A canceled ctx aborts the backoff promptly.
func resolveWithRetry(ctx context.Context, r Resolver, host string) ([]netip.Addr, error) {
	addrs, err := r.Resolve(ctx, host)
	if err == nil || errors.Is(err, ErrNotPublished) {
		return addrs, err
	}
	// First attempt failed transiently — pause briefly (interruptibly) then retry once.
	timer := time.NewTimer(dohRetryBackoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, err // keep the original transient error; caller falls back
	case <-timer.C:
	}
	return r.Resolve(ctx, host)
}

// osResolverClient is a plain client that resolves via the OS resolver (the
// fallback when DoH is unavailable).
func osResolverClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// pinnedClient returns an *http.Client whose transport dials the given resolved
// address(es) whenever the request targets host, and otherwise dials normally.
// Only the dial target is overridden — TLS ServerName and the Host header still
// carry host, so certificate validation and virtual-host routing are unchanged.
func pinnedClient(host string, addrs []netip.Addr, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	// Pre-render the pinned IP once; prefer the first (an A record for a
	// CF-proxied host is a stable anycast VIP).
	pinned := addrs[0].String()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(dctx context.Context, network, addr string) (net.Conn, error) {
			reqHost, port, err := net.SplitHostPort(addr)
			if err != nil {
				// No port? dial as given (shouldn't happen for http/https).
				return dialer.DialContext(dctx, network, addr)
			}
			if strings.EqualFold(reqHost, host) {
				return dialer.DialContext(dctx, network, net.JoinHostPort(pinned, port))
			}
			// A redirect to a different host — resolve it normally.
			return dialer.DialContext(dctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
