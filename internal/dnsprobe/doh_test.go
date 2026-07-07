package dnsprobe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

// dohHandler returns an httptest handler that answers the Cloudflare DoH JSON API:
// for name==liveName it returns an A record (ip); NXDOMAIN for nxName; and an empty
// NOERROR answer otherwise. It asserts the request carries Accept: application/dns-json.
func dohHandler(t *testing.T, liveName, ip, nxName string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/dns-json" {
			t.Errorf("Accept = %q, want application/dns-json", got)
		}
		name := strings.TrimSuffix(r.URL.Query().Get("name"), ".")
		qtype := r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/dns-json")
		switch {
		case name == liveName && qtype == "A":
			fmt.Fprintf(w, `{"Status":0,"Answer":[{"name":%q,"type":5,"TTL":60,"data":"cname.example."},{"name":%q,"type":1,"TTL":60,"data":%q}]}`, name, name, ip)
		case name == nxName:
			// NXDOMAIN
			fmt.Fprint(w, `{"Status":3,"Answer":[]}`)
		default:
			// NOERROR / NODATA (no address answer)
			fmt.Fprint(w, `{"Status":0,"Answer":[]}`)
		}
	}
}

func TestDoHResolverAAnswer(t *testing.T) {
	srv := httptest.NewServer(dohHandler(t, "dev-0123456789abcdef.civit.ai", "203.0.113.7", "nope.civit.ai"))
	defer srv.Close()

	r := &DoHResolver{Endpoint: srv.URL, Client: srv.Client()}
	addrs, err := r.Resolve(context.Background(), "dev-0123456789abcdef.civit.ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(addrs) != 1 || addrs[0].String() != "203.0.113.7" {
		t.Fatalf("addrs = %v, want [203.0.113.7]", addrs)
	}
}

func TestDoHResolverNXDOMAIN(t *testing.T) {
	srv := httptest.NewServer(dohHandler(t, "live.civit.ai", "203.0.113.7", "dev-deadbeefdeadbeef.civit.ai"))
	defer srv.Close()

	r := &DoHResolver{Endpoint: srv.URL, Client: srv.Client()}
	_, err := r.Resolve(context.Background(), "dev-deadbeefdeadbeef.civit.ai")
	if !errors.Is(err, ErrNotPublished) {
		t.Fatalf("NXDOMAIN should map to ErrNotPublished, got %v", err)
	}
}

func TestDoHResolverEmptyAnswerIsNotPublished(t *testing.T) {
	// A name that returns NOERROR but no A/AAAA answer (NODATA) — treated as
	// not-published from the readiness probe's point of view.
	srv := httptest.NewServer(dohHandler(t, "live.civit.ai", "203.0.113.7", "nx.civit.ai"))
	defer srv.Close()

	r := &DoHResolver{Endpoint: srv.URL, Client: srv.Client()}
	_, err := r.Resolve(context.Background(), "empty.civit.ai")
	if !errors.Is(err, ErrNotPublished) {
		t.Fatalf("empty answer should map to ErrNotPublished, got %v", err)
	}
}

func TestDoHResolverNon200IsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r := &DoHResolver{Endpoint: srv.URL, Client: srv.Client()}
	_, err := r.Resolve(context.Background(), "dev-0123456789abcdef.civit.ai")
	if err == nil {
		t.Fatal("expected a transient error on non-200 DoH response")
	}
	if errors.Is(err, ErrNotPublished) {
		t.Fatalf("a 502 from the DoH endpoint must NOT be classified as not-published (it's transient): %v", err)
	}
}

// stubResolver is an injectable Resolver for DialClient tests.
type stubResolver struct {
	addrs []netip.Addr
	err   error
}

func (s stubResolver) Resolve(context.Context, string) ([]netip.Addr, error) {
	return s.addrs, s.err
}

// TestDialClientDialsResolvedIP: DialClient's client must dial the DoH-resolved IP
// for the host while preserving the Host header (no OS resolution of the host).
func TestDialClientDialsResolvedIP(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL) // http://127.0.0.1:PORT
	port := u.Port()

	// Resolver claims the (never-DNS-registered) host resolves to 127.0.0.1 — the
	// httptest server's address — so if the client actually dials the resolved IP,
	// the request reaches the server even though "tunnel.example" has no DNS record.
	r := stubResolver{addrs: []netip.Addr{netip.MustParseAddr("127.0.0.1")}}
	client, err := DialClient(context.Background(), r, "tunnel.example", 3*time.Second)
	if err != nil {
		t.Fatalf("DialClient error: %v", err)
	}
	target := fmt.Sprintf("http://tunnel.example:%s/", port)
	resp, err := client.Get(target)
	if err != nil {
		t.Fatalf("GET via pinned client failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if want := "tunnel.example:" + port; gotHost != want {
		t.Fatalf("server saw Host %q, want %q (Host header must be preserved)", gotHost, want)
	}
}

// TestDialClientNotPublished: an authoritative not-found is surfaced as
// ErrNotPublished with no client (the caller treats it as the DNS-pending state).
func TestDialClientNotPublished(t *testing.T) {
	r := stubResolver{err: fmt.Errorf("x: %w", ErrNotPublished)}
	client, err := DialClient(context.Background(), r, "tunnel.example", time.Second)
	if !errors.Is(err, ErrNotPublished) {
		t.Fatalf("want ErrNotPublished, got err=%v", err)
	}
	if client != nil {
		t.Fatalf("want nil client on not-published, got %v", client)
	}
}

// TestDialClientEmptyAddrsNotPublished: a success with no addresses is also
// not-published.
func TestDialClientEmptyAddrsNotPublished(t *testing.T) {
	r := stubResolver{addrs: nil, err: nil}
	client, err := DialClient(context.Background(), r, "tunnel.example", time.Second)
	if !errors.Is(err, ErrNotPublished) {
		t.Fatalf("want ErrNotPublished on empty addrs, got err=%v", err)
	}
	if client != nil {
		t.Fatalf("want nil client, got %v", client)
	}
}

// TestDialClientFallsBackOnTransientDoHFailure: a transient DoH error (e.g. 443 to
// Cloudflare blocked) falls back to an OS-resolver client so behavior is never
// worse than before — DialClient returns a usable client and no error.
func TestDialClientFallsBackOnTransientDoHFailure(t *testing.T) {
	r := stubResolver{err: errors.New("doh unreachable: connection refused")}
	client, err := DialClient(context.Background(), r, "tunnel.example", time.Second)
	if err != nil {
		t.Fatalf("transient DoH failure must fall back, not error; got %v", err)
	}
	if client == nil {
		t.Fatal("want a fallback (OS-resolver) client, got nil")
	}
	// The fallback client must reach a normal host via the OS resolver.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	resp, gerr := client.Get(srv.URL + "/")
	if gerr != nil {
		t.Fatalf("fallback client GET failed: %v", gerr)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fallback status = %d, want 200", resp.StatusCode)
	}
}
