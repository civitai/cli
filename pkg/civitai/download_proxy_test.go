package civitai

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Issue #780. Under a proxy the dial guard only ever sees the PROXY's address,
// so before the fix a loopback/private proxy (an agent sandbox's) was always
// refused, while a public proxy disabled the private-address guard for every
// target. These tests drive a real CONNECT proxy on loopback that tunnels every
// CONNECT, whatever host it names, to one local TLS origin — so the target URL
// can carry a public-looking IP literal while the bytes stay on this machine.
// IP literals keep these tests free of DNS (the resolver path is covered in
// download_proxy_resolve_test.go).

// proxyFixture is a loopback CONNECT proxy in front of a loopback TLS origin.
type proxyFixture struct {
	proxyURL *url.URL
	origin   *httptest.Server

	mu       sync.Mutex
	connects []string // CONNECT targets the proxy was asked for, in order
	hits     []string // Host header of each request the origin served
}

func (f *proxyFixture) connectTargets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.connects...)
}

func (f *proxyFixture) originHits() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

// newProxyFixture starts the origin (serving originHandler) and the proxy.
func newProxyFixture(t *testing.T, originHandler http.HandlerFunc) *proxyFixture {
	t.Helper()
	f := &proxyFixture{}
	f.origin = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits = append(f.hits, r.Host)
		f.mu.Unlock()
		originHandler(w, r)
	}))
	t.Cleanup(f.origin.Close)
	originAddr := f.origin.Listener.Addr().String()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		f.mu.Lock()
		f.connects = append(f.connects, r.Host)
		f.mu.Unlock()
		upstream, err := net.Dial("tcp", originAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			_ = upstream.Close()
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		client, rw, err := hj.Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(upstream, rw); _ = upstream.Close() }()
		go func() { _, _ = io.Copy(client, upstream); _ = client.Close() }()
	}))
	t.Cleanup(proxy.Close)
	pu, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	f.proxyURL = pu
	return f
}

// proxiedClient is a guard-ON client whose transport sends requests through
// proxy as chosen by proxyFor (nil: no proxy). It trusts the origin's
// self-signed certificate whatever name the URL uses. The proxy is injected on
// the transport rather than through HTTPS_PROXY because net/http reads the
// environment once per process; downloadHTTPClient consults the transport's own
// Proxy func either way, which is ProxyFromEnvironment by default.
func proxiedClient(proxyFor func(*http.Request) (*url.URL, error)) *Client {
	c := New("https://civitai.com", "")
	c.HTTP = &http.Client{Transport: &http.Transport{
		Proxy:           proxyFor,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test origin is self-signed
	}}
	return c
}

// publicTarget is an IP literal outside every blocked range (TEST-NET-3).
const publicTarget = "203.0.113.10"

// TestDownloadViaLoopbackProxyToPublicTargetSucceeds: a loopback proxy must be
// usable. Before the fix the dial guard refused the proxy's 127.0.0.1.
func TestDownloadViaLoopbackProxyToPublicTargetSucceeds(t *testing.T) {
	f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "weights")
	})
	c := proxiedClient(http.ProxyURL(f.proxyURL))

	resp, err := c.DownloadPresigned(context.Background(), "https://"+publicTarget+"/file.safetensors")
	if err != nil {
		t.Fatalf("download through a loopback proxy to a public target was refused: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "weights" {
		t.Fatalf("got %d %q, want 200 \"weights\"", resp.StatusCode, body)
	}
	if got := f.connectTargets(); len(got) != 1 || got[0] != publicTarget+":443" {
		t.Errorf("proxy CONNECT targets = %v, want [%s:443] (the request must really go through the proxy)", got, publicTarget)
	}
}

// TestDownloadViaProxyRefusesPrivateTarget: under a proxy, a target that is a
// private or loopback address is refused before anything reaches the proxy,
// and the message names the address, the reason and the proxy. Before the fix
// the proxy (here loopback, so the request failed on the PROXY's address) — or,
// with a public proxy, nothing at all — stood in the way.
func TestDownloadViaProxyRefusesPrivateTarget(t *testing.T) {
	cases := []struct {
		target, reason string
	}{
		{"10.0.0.5", "private"},
		{"127.0.0.1", "loopback"},
		{"169.254.169.254", "link-local"},
		{"100.64.0.9", "CGNAT"},
		{"[fd00::5]", "private"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("origin reached for a private target (Host %q)", r.Host)
			})
			c := proxiedClient(http.ProxyURL(f.proxyURL))

			_, err := c.DownloadPresigned(context.Background(), "https://"+tc.target+"/x")
			if err == nil {
				t.Fatalf("a %s target through a proxy was downloaded", tc.reason)
			}
			ip := strings.Trim(tc.target, "[]")
			for _, want := range []string{
				"refusing to download from " + ip,
				"via proxy " + f.proxyURL.Host,
				ip + " is a " + tc.reason + " address",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal should contain %q, got: %v", want, err)
				}
			}
			if got := f.connectTargets(); len(got) != 0 {
				t.Errorf("proxy was asked to CONNECT %v; the target must be refused before the proxy is used", got)
			}
		})
	}
}

// TestDownloadViaProxyRefusesRedirectToPrivate: the target check runs on every
// redirect hop, not only the first URL.
func TestDownloadViaProxyRefusesRedirectToPrivate(t *testing.T) {
	const private = "10.0.0.7"
	f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, private) {
			t.Errorf("origin reached for the private redirect target (Host %q)", r.Host)
			return
		}
		http.Redirect(w, r, "https://"+private+"/internal", http.StatusFound)
	})
	c := proxiedClient(http.ProxyURL(f.proxyURL))

	_, err := c.DownloadPresigned(context.Background(), "https://"+publicTarget+"/file")
	if err == nil {
		t.Fatal("a redirect to a private target through a proxy was followed")
	}
	if !strings.Contains(err.Error(), "refusing to download from "+private) ||
		!strings.Contains(err.Error(), private+" is a private address") {
		t.Errorf("refusal should name the redirect target %s and why: %v", private, err)
	}
	if got := f.connectTargets(); len(got) != 1 || got[0] != publicTarget+":443" {
		t.Errorf("proxy CONNECT targets = %v, want only the public first hop", got)
	}
	if got := f.originHits(); len(got) != 1 {
		t.Errorf("origin hits = %v, want exactly the first hop", got)
	}
}

// TestDownloadProxyExemptionIsOnlyTheProxyAddress: the private-address
// exemption covers exactly the proxy chosen for the request. Here the proxy
// handles the first hop only (as NO_PROXY would arrange), and that hop
// redirects to a DIRECT loopback address, which the dial guard must still
// refuse.
func TestDownloadProxyExemptionIsOnlyTheProxyAddress(t *testing.T) {
	direct := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("direct loopback origin reached")
	}))
	defer direct.Close()
	directURL, _ := url.Parse(direct.URL)

	f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, direct.URL+"/x", http.StatusFound)
	})
	c := proxiedClient(func(r *http.Request) (*url.URL, error) {
		if r.URL.Hostname() == publicTarget {
			return f.proxyURL, nil
		}
		return nil, nil // not proxied: a direct dial
	})

	_, err := c.DownloadPresigned(context.Background(), "https://"+publicTarget+"/file")
	if err == nil {
		t.Fatal("a direct redirect to loopback was followed on a request whose first hop used a proxy")
	}
	if !strings.Contains(err.Error(), "refusing to download from "+directURL.Hostname()) ||
		!strings.Contains(err.Error(), "is a loopback address") {
		t.Errorf("refusal should name %s as loopback: %v", directURL.Hostname(), err)
	}
	if strings.Contains(err.Error(), "via proxy") {
		t.Errorf("the direct hop was not proxied; the message must not say it was: %v", err)
	}
}
