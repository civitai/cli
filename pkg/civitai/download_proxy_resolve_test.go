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

	t.Run("unresolvable hostname is refused, not passed", func(t *testing.T) {
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("origin reached for %s", r.Host)
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		_, err := c.DownloadPresigned(context.Background(), "https://nowhere.example.test/f")
		if err == nil {
			t.Fatal("an unresolvable target via proxy was downloaded unchecked")
		}
		var dnsErr *net.DNSError
		if !errors.As(err, &dnsErr) || !strings.Contains(err.Error(), "could not resolve it to check") {
			t.Errorf("want a resolution refusal wrapping the DNS error, got: %v", err)
		}
		if got := f.connectTargets(); len(got) != 0 {
			t.Errorf("proxy was asked to CONNECT %v", got)
		}
	})
}
