package civitai

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestUploadPresignedViaProxyChecksTarget: UploadPresigned is built on
// downloadHTTPClient, so a proxied upload gets the same target check: a private
// target is refused before the proxy is used, and a public one goes through a
// loopback proxy.
func TestUploadPresignedViaProxyChecksTarget(t *testing.T) {
	t.Run("private target refused", func(t *testing.T) {
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("origin reached for %s", r.Host)
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		_, err := c.UploadPresigned(context.Background(), "https://10.0.0.8/v2/consumer/blobs", "image/png", []byte("x"))
		if err == nil {
			t.Fatal("a proxied upload to a private target was sent")
		}
		if !strings.Contains(err.Error(), "refusing to download from 10.0.0.8 via proxy "+f.proxyURL.Host) ||
			!strings.Contains(err.Error(), "10.0.0.8 is a private address") {
			t.Errorf("refusal should name the target and why: %v", err)
		}
		if got := f.connectTargets(); len(got) != 0 {
			t.Errorf("proxy was asked to CONNECT %v", got)
		}
	})
	t.Run("public target through a loopback proxy", func(t *testing.T) {
		f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if r.Method != http.MethodPost || string(body) != "x" {
				t.Errorf("origin got %s %q, want POST \"x\"", r.Method, body)
			}
		})
		c := proxiedClient(http.ProxyURL(f.proxyURL))
		resp, err := c.UploadPresigned(context.Background(), "https://"+publicTarget+"/v2/consumer/blobs", "image/png", []byte("x"))
		if err != nil {
			t.Fatalf("proxied upload to a public target refused: %v", err)
		}
		_ = resp.Body.Close()
		if got := f.connectTargets(); len(got) != 1 {
			t.Errorf("proxy CONNECT targets = %v, want one", got)
		}
	})
}

// socks5Fixture is a minimal SOCKS5 (RFC 1928, no auth, CONNECT only) proxy on
// loopback that tunnels every CONNECT to origin, recording the requested target.
type socks5Fixture struct {
	url     *url.URL
	mu      sync.Mutex
	targets []string
}

func (s *socks5Fixture) requested() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.targets...)
}

func newSocks5Fixture(t *testing.T, originAddr string) *socks5Fixture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	s := &socks5Fixture{url: &url.URL{Scheme: "socks5", Host: ln.Addr().String()}}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(t, conn, originAddr)
		}
	}()
	return s
}

func (s *socks5Fixture) serve(t *testing.T, conn net.Conn, originAddr string) {
	defer conn.Close()
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil || hdr[0] != 5 {
		return
	}
	if _, err := io.ReadFull(conn, make([]byte, hdr[1])); err != nil {
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil { // no authentication
		return
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil || req[1] != 1 {
		return
	}
	var host string
	switch req[3] {
	case 1: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = string(b)
	case 4: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(conn, pb); err != nil {
		return
	}
	s.mu.Lock()
	s.targets = append(s.targets, net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb)))))
	s.mu.Unlock()
	upstream, err := net.Dial("tcp", originAddr)
	if err != nil {
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, conn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(conn, upstream); done <- struct{}{} }()
	<-done
}

// TestDownloadViaSocks5ProxyExemptsOnlyTheProxy: a socks5 proxy's own loopback
// address is exempt from the dial guard (proxyDialAddr's socks5 case), while a
// private target named through it is refused before the proxy is asked.
func TestDownloadViaSocks5ProxyExemptsOnlyTheProxy(t *testing.T) {
	f := newProxyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host != publicTarget {
			t.Errorf("origin reached for %s", r.Host)
			return
		}
		_, _ = io.WriteString(w, "weights")
	})
	socks := newSocks5Fixture(t, f.origin.Listener.Addr().String())
	c := proxiedClient(http.ProxyURL(socks.url))

	resp, err := c.DownloadPresigned(context.Background(), "https://"+publicTarget+"/file")
	if err != nil {
		t.Fatalf("download through a loopback socks5 proxy to a public target was refused: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "weights" {
		t.Errorf("body = %q, want \"weights\"", body)
	}
	if got := socks.requested(); len(got) != 1 || got[0] != publicTarget+":443" {
		t.Errorf("socks5 CONNECT targets = %v, want [%s:443]", got, publicTarget)
	}

	_, err = c.DownloadPresigned(context.Background(), "https://10.0.0.6/file")
	if err == nil {
		t.Fatal("a private target through a socks5 proxy was downloaded")
	}
	if !strings.Contains(err.Error(), "refusing to download from 10.0.0.6 via proxy "+socks.url.Host) ||
		!strings.Contains(err.Error(), "10.0.0.6 is a private address") {
		t.Errorf("refusal should name the target, the proxy and why: %v", err)
	}
	if got := socks.requested(); len(got) != 1 {
		t.Errorf("socks5 proxy was asked for %v; the private target must be refused before it", got)
	}
}

// TestBlockedAddrMessageArticle pins the article in the refusal: "an
// unspecified address", "a loopback address".
func TestBlockedAddrMessageArticle(t *testing.T) {
	for _, tc := range []struct{ ip, want string }{
		{"0.0.0.0", "0.0.0.0 is an unspecified address"},
		{"127.0.0.1", "127.0.0.1 is a loopback address"},
	} {
		ip := net.ParseIP(tc.ip)
		err := &blockedDownloadAddrError{ip: ip, reason: blockedDownloadReason(ip)}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("message %q should contain %q", err.Error(), tc.want)
		}
	}
}
