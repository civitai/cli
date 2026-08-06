package civitai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDownloadPresignedSendsNoAuthorizationHeader is the credential-leak guard,
// WITH its positive control in the same test.
//
// The claim under test is an ABSENCE — "no Authorization header reaches the blob
// host" — and an absence measured by a probe that is wired to nothing looks
// exactly the same as an absence produced by correct code. So case (B) drives
// the SAME recorder, through the SAME server, over the SAME URL, and requires it
// to observe a header. Report the pair, never the zero alone.
//
// The setup is the hazardous one on purpose: baseURL EQUALS the blob host, so
// isTrustedDownloadHost returns true and DownloadFile genuinely does attach the
// token. That is what makes (A) a real result rather than a URL the predicate
// would have rejected anyway.
func TestDownloadPresignedSendsNoAuthorizationHeader(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, "blob-bytes")
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-api-key")
	c.AllowPrivateDownloadHosts = true // httptest binds plain-http loopback
	blobURL := srv.URL + "/blob/1.jpeg"

	// (A) The guard: the presigned path must send NO credential.
	resp, err := c.DownloadPresigned(context.Background(), blobURL)
	if err != nil {
		t.Fatalf("DownloadPresigned: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if len(seen) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(seen))
	}
	if seen[0] != "" {
		t.Errorf("DownloadPresigned leaked a credential to the blob host: Authorization = %q", seen[0])
	}

	// (B) POSITIVE CONTROL: the same recorder, the same host, the same URL —
	// through DownloadFile, which SHOULD authenticate. If this is also empty the
	// probe observes nothing and (A) proves nothing.
	resp2, err := c.DownloadFile(context.Background(), blobURL)
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	_ = resp2.Body.Close()
	if len(seen) != 2 {
		t.Fatalf("expected exactly 2 requests, got %d", len(seen))
	}
	if !strings.HasPrefix(seen[1], "Bearer ") {
		t.Fatalf("POSITIVE CONTROL FAILED: DownloadFile sent Authorization = %q, want a Bearer token — "+
			"the recorder cannot observe the header, so the empty value in case (A) means nothing", seen[1])
	}
	if !strings.Contains(seen[1], "secret-api-key") {
		t.Errorf("positive control: want the configured token in the header, got %q", seen[1])
	}
}

// TestDownloadPresignedKeepsTheTransportGuards proves the credential-free path
// did not become a second, unguarded downloader: it must still refuse a non-https
// URL when private hosts are not allowed (the guard `civitai download` relies on).
func TestDownloadPresignedKeepsTheTransportGuards(t *testing.T) {
	c := New("https://civitai.com", "tok")
	// AllowPrivateDownloadHosts stays FALSE — the production configuration.
	_, err := c.DownloadPresigned(context.Background(), "http://orchestration.civitai.com/blob/1.jpeg")
	if err == nil {
		t.Fatal("plain-http blob URL: want a refusal, got nil")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("refusal should name the https requirement, got %v", err)
	}
}

// TestIsTrustedDownloadHostStillTrustsCivitaiSubdomains pins WHY the seam had to
// exist: the trusted-host predicate does match the orchestrator blob host, so
// reusing DownloadFile really would have attached the token. If this ever goes
// false, the predicate was weakened — which is exactly what
// PresignedDownloader's doc forbids, because `civitai download` depends on it.
func TestIsTrustedDownloadHostStillTrustsCivitaiSubdomains(t *testing.T) {
	for _, u := range []string{
		"https://orchestration.civitai.com/blob/abc",
		"https://civitai.com/api/download/models/1",
	} {
		if !isTrustedDownloadHost(u, "https://civitai.com") {
			t.Errorf("isTrustedDownloadHost(%q) = false, want true — the token-attaching predicate must not be weakened", u)
		}
	}
	// And it must still reject the look-alikes.
	for _, u := range []string{
		"https://evilcivitai.com/blob",
		"https://civitai.com.evil.com/blob",
	} {
		if isTrustedDownloadHost(u, "https://civitai.com") {
			t.Errorf("isTrustedDownloadHost(%q) = true, want false", u)
		}
	}
}
