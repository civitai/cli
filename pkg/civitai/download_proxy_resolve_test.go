package civitai

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// The hostname half of the #780 proxy target check: a proxied target named by
// a HOSTNAME is resolved here (lookupDownloadHost, the dial guard's resolver)
// and refused when any answer is private. The fake resolver below answers for
// *.example.test only, so no real DNS is touched.

func fakeResolver(t *testing.T, answers map[string][]string) {
	t.Helper()
	prev := lookupDownloadHost
	lookupDownloadHost = func(_ context.Context, host string) ([]net.IPAddr, error) {
		ips, ok := answers[host]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		var out []net.IPAddr
		for _, s := range ips {
			out = append(out, net.IPAddr{IP: net.ParseIP(s)})
		}
		return out, nil
	}
	t.Cleanup(func() { lookupDownloadHost = prev })
}

func TestDownloadViaProxyResolvesTargetHostname(t *testing.T) {
	fakeResolver(t, map[string][]string{
		"files.example.test":    {"203.0.113.20"},
		"internal.example.test": {"10.1.2.3"},
		"mixed.example.test":    {"203.0.113.21", "192.168.4.4"},
		"self.example.test":     {"::1"},
	})

	t.Run("public hostname is downloaded through the proxy", func(t *testing.T) {
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		resp, err := c.DownloadPresigned(context.Background(), "https://files.example.test/f")
		if err != nil {
			t.Fatalf("public hostname via proxy refused: %v", err)
		}
		_ = resp.Body.Close()
		if got := f.connectTargets(); len(got) != 1 || got[0] != "files.example.test:443" {
			t.Errorf("proxy CONNECT targets = %v, want [files.example.test:443]", got)
		}
	})

	refused := []struct {
		name, host, ip, reason string
	}{
		{"private answer", "internal.example.test", "10.1.2.3", "private"},
		{"one private answer among public ones", "mixed.example.test", "192.168.4.4", "private"},
		{"loopback answer", "self.example.test", "::1", "loopback"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("origin reached for %s", r.Host)
			})
			c := proxiedClient(http.ProxyURL(f.proxyURL))
			_, err := c.DownloadPresigned(context.Background(), "https://"+tc.host+"/f")
			if err == nil {
				t.Fatalf("%s via proxy was downloaded", tc.host)
			}
			want := "refusing to download from " + tc.host + " (resolved to " + tc.ip + ") via proxy " + f.proxyURL.Host +
				": " + tc.ip + " is a " + tc.reason + " address"
			if !strings.Contains(err.Error(), want) {
				t.Errorf("refusal should contain %q, got: %v", want, err)
			}
			if got := f.connectTargets(); len(got) != 0 {
				t.Errorf("proxy was asked to CONNECT %v", got)
			}
		})
	}

	t.Run("unresolvable hostname proceeds through the proxy", func(t *testing.T) {
		// Operator decision on #780: a name that does not resolve HERE is
		// handed to the proxy, which resolves it itself. Sandboxes whose only
		// route out is the proxy often have no DNS of their own.
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		resp, err := c.DownloadPresigned(context.Background(), "https://nowhere.example.test/f")
		if err != nil {
			t.Fatalf("an unresolvable target via proxy was refused; it must be passed to the proxy: %v", err)
		}
		_ = resp.Body.Close()
		if got := f.connectTargets(); len(got) != 1 || got[0] != "nowhere.example.test:443" {
			t.Errorf("proxy CONNECT targets = %v, want [nowhere.example.test:443]", got)
		}
		if got := f.originHits(); len(got) != 1 {
			t.Errorf("origin hits = %v, want 1", got)
		}
	})
}

// TestDownloadUnresolvableWithoutProxyUnchanged is an INVARIANT GUARD: with no
// proxy, an unresolvable target fails at the dial exactly as on main — the
// proxy pre-check (and its resolver) is never consulted, and nothing is
// downloaded. ".invalid" is reserved (RFC 6761) and never resolves.
func TestDownloadUnresolvableWithoutProxyUnchanged(t *testing.T) {
	prev := lookupDownloadHost
	lookupDownloadHost = func(_ context.Context, host string) ([]net.IPAddr, error) {
		t.Errorf("proxy pre-check resolver consulted for %s on a request with no proxy", host)
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	t.Cleanup(func() { lookupDownloadHost = prev })

	c := proxiedClient(nil)
	_, err := c.DownloadPresigned(context.Background(), "https://nowhere.invalid/f")
	if err == nil {
		t.Fatal("an unresolvable target with no proxy was downloaded")
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		t.Errorf("want the dial's DNS error, got: %v", err)
	}
	if strings.Contains(err.Error(), "via proxy") {
		t.Errorf("no proxy was used; the error must not say one was: %v", err)
	}
}

// TestDownloadViaProxyRefusesNonStandardIPForms: hosts that are IP literals in
// a form net.ParseIP rejects, or loopback by name. The pure-Go resolver
// (CGO_ENABLED=0, how releases are built) answers "no such host" for every one
// of them — measured with CGO_ENABLED=0 on go1.26 — and the fake resolver here
// answers exactly that, so the test does not depend on how it was built.
// Before the fix the "unresolvable, so the proxy resolves it" branch let them
// all through; a proxy that parses them like inet_aton then reaches loopback or
// a private address.
func TestDownloadViaProxyRefusesNonStandardIPForms(t *testing.T) {
	fakeResolver(t, nil) // every lookup: no such host

	refused := []struct {
		host string // as written in the URL
		want string // fragment of the refusal
	}{
		{"2130706433", "2130706433 (read as 127.0.0.1) via proxy "},
		{"0x7f000001", "0x7f000001 (read as 127.0.0.1) via proxy "},
		{"127.1", "127.1 (read as 127.0.0.1) via proxy "},
		{"0177.0.0.1", "0177.0.0.1 (read as 127.0.0.1) via proxy "},
		{"10.0.0.5.", "refusing to download from 10.0.0.5 via proxy "},
		{"167772165", "167772165 (read as 10.0.0.5) via proxy "},
		{"[fe80::1%25eth0]", "fe80::1%eth0 (read as fe80::1) via proxy "},
		{"foo.localhost", "foo.localhost is a loopback name (RFC 6761)"},
		{"localhost.", "localhost is a loopback name (RFC 6761)"},
		{"0x", "looks like a numeric IP address but is not a valid one"},
		{"1.2.3.4.5", "looks like a numeric IP address but is not a valid one"},
		{"08.0.0.1", "looks like a numeric IP address but is not a valid one"},
		{"256.0.0.1", "looks like a numeric IP address but is not a valid one"},
		{"4294967296", "looks like a numeric IP address but is not a valid one"},
	}
	for _, tc := range refused {
		t.Run(tc.host, func(t *testing.T) {
			f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("origin reached for %s", r.Host)
			})
			c := proxiedClient(http.ProxyURL(f.proxyURL))
			_, err := c.DownloadPresigned(context.Background(), "https://"+tc.host+"/f")
			if err == nil {
				t.Fatalf("%s via proxy was downloaded", tc.host)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal should contain %q, got: %v", tc.want, err)
			}
			if got := f.connectTargets(); len(got) != 0 {
				t.Errorf("proxy was asked to CONNECT %v", got)
			}
		})
	}

	// Control: a public address written as one decimal number (203.0.113.10)
	// is still downloaded through the proxy.
	t.Run("public decimal form passes", func(t *testing.T) {
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "ok")
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		resp, err := c.DownloadPresigned(context.Background(), "https://3405803786/f")
		if err != nil {
			t.Fatalf("public decimal form refused: %v", err)
		}
		_ = resp.Body.Close()
		if got := f.connectTargets(); len(got) != 1 || got[0] != "3405803786:443" {
			t.Errorf("proxy CONNECT targets = %v, want [3405803786:443]", got)
		}
	})
}
