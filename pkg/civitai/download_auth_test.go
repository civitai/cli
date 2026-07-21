package civitai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestIsTrustedDownloadHost is the security gate for attaching the bearer token:
// only https civitai.com (+ subdomains) or the exact configured API origin are
// trusted; a hostile/off-domain URL is not.
func TestIsTrustedDownloadHost(t *testing.T) {
	const base = "https://civitai.com"
	cases := []struct {
		fileURL string
		baseURL string
		want    bool
	}{
		// civitai domain over https.
		{"https://civitai.com/api/download/models/1", base, true},
		{"https://models.civitai.com/blob", base, true},
		{"https://a.b.civitai.com/blob", base, true},
		// Look-alike / substring attacks must be rejected.
		{"https://evilcivitai.com/steal", base, false},
		{"https://civitai.com.evil.com/steal", base, false},
		{"https://notcivitai.com/steal", base, false},
		// Right host, wrong scheme (would leak the token over cleartext).
		{"http://civitai.com/api/download/models/1", base, false},
		// Off-domain signed-storage target (the redirect destination).
		{"https://signed-storage.example.com/blob?sig=abc", base, false},
		// Same-origin as the configured API endpoint is trusted (self-host / test).
		{"http://127.0.0.1:8080/dl", "http://127.0.0.1:8080", true},
		{"http://127.0.0.1:8080/dl", "http://127.0.0.1:9999", false},  // different port
		{"http://127.0.0.1:8080/dl", "https://127.0.0.1:8080", false}, // different scheme
		// Garbage.
		{"://nope", base, false},
		{"", base, false},
	}
	for _, tc := range cases {
		if got := isTrustedDownloadHost(tc.fileURL, tc.baseURL); got != tc.want {
			t.Errorf("isTrustedDownloadHost(%q, %q) = %v, want %v", tc.fileURL, tc.baseURL, got, tc.want)
		}
	}
}

// TestDownloadFileSkipsAuthForUntrustedHost proves the token is NOT sent to an
// off-domain download host even when one is configured — the exfiltration
// defense. The BaseURL is the trusted civitai.com API, but the download URL is a
// different (local, off-domain) host, so no Authorization must reach it.
func TestDownloadFileSkipsAuthForUntrustedHost(t *testing.T) {
	const body = "SIGNED-STORAGE-BYTES"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New("https://civitai.com", "secret-token") // trusted API base, but...
	c.AllowPrivateDownloadHosts = true              // httptest binds plain-http loopback
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/signed-blob")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Errorf("body = %q, want %q (download must still succeed without auth)", got, body)
	}
	if gotAuth != "" {
		t.Errorf("token leaked to untrusted host: Authorization = %q, want none", gotAuth)
	}
}

// TestDownloadTransportSetsResponseHeaderTimeout proves the download client
// carries a ResponseHeaderTimeout and does so on a CLONE (the shared default
// transport is never mutated).
func TestDownloadTransportSetsResponseHeaderTimeout(t *testing.T) {
	tr := downloadTransport(nil, downloadResponseHeaderTimeout)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatalf("downloadTransport(nil) = %T, want *http.Transport", tr)
	}
	if ht.ResponseHeaderTimeout != downloadResponseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", ht.ResponseHeaderTimeout, downloadResponseHeaderTimeout)
	}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok && dt.ResponseHeaderTimeout != 0 {
		t.Errorf("DefaultTransport was mutated: ResponseHeaderTimeout = %v (must clone, not mutate)", dt.ResponseHeaderTimeout)
	}
}

// TestDownloadTransportHeaderTimeoutFires proves a server that accepts the
// connection but never sends headers is aborted by the ResponseHeaderTimeout
// rather than hanging. Uses a short timeout for a fast, non-flaky test.
func TestDownloadTransportHeaderTimeoutFires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second): // well beyond the client's header timeout
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "too late")
	}))
	defer srv.Close()

	hc := &http.Client{Transport: downloadTransport(nil, 50*time.Millisecond)}
	resp, err := hc.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected a ResponseHeaderTimeout error, got a response")
	}
}
