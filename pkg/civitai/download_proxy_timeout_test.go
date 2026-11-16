package civitai

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestDownloadViaProxyLookupTimeoutGoesToProxy: the proxied pre-check's DNS
// lookup has its own timeout, and a lookup that times out is treated as
// unresolvable, so the name goes to the proxy (the operator's rule for a name
// that does not resolve here). The fake resolver answers a PRIVATE address
// after 2s unless its context ends first, so without the timeout this request
// is refused rather than hanging the test.
func TestDownloadViaProxyLookupTimeoutGoesToProxy(t *testing.T) {
	prevTimeout := proxyTargetLookupTimeout
	proxyTargetLookupTimeout = 50 * time.Millisecond
	t.Cleanup(func() { proxyTargetLookupTimeout = prevTimeout })

	prev := lookupDownloadHost
	lookupDownloadHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return []net.IPAddr{{IP: net.ParseIP("10.9.9.9")}}, nil
		}
	}
	t.Cleanup(func() { lookupDownloadHost = prev })

	f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	c := proxiedClient(http.ProxyURL(f.proxyURL))
	start := time.Now()
	resp, err := c.DownloadPresigned(context.Background(), "https://slow.example.test/f")
	if err != nil {
		t.Fatalf("a target whose lookup timed out should go to the proxy, got: %v", err)
	}
	_ = resp.Body.Close()
	if d := time.Since(start); d > time.Second {
		t.Errorf("request took %v; the lookup should have timed out after ~50ms", d)
	}
	if got := f.connectTargets(); len(got) != 1 || got[0] != "slow.example.test:443" {
		t.Errorf("proxy CONNECT targets = %v, want [slow.example.test:443]", got)
	}
}
