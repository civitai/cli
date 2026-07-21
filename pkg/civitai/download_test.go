package civitai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadFileSendsBearerAndStreams(t *testing.T) {
	const body = "MODEL-WEIGHTS-BYTES"
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-xyz", "")
	c.AllowPrivateDownloadHosts = true // httptest binds plain-http loopback
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/file")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Errorf("streamed body = %q, want %q", got, body)
	}
	if gotAuth != "Bearer tok-xyz" {
		t.Errorf("Authorization = %q, want Bearer tok-xyz", gotAuth)
	}
}

func TestDownloadFileAnonymousSendsNoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "x")
	}))
	defer srv.Close()

	c := New(srv.URL, "", "") // empty token => anonymous
	c.AllowPrivateDownloadHosts = true
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/file")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "" {
		t.Errorf("anonymous download sent Authorization %q, want none", gotAuth)
	}
}

func TestDownloadFileFollowsRedirect(t *testing.T) {
	const body = "REDIRECTED-STORAGE-BYTES"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/download" {
			http.Redirect(w, r, "/storage/blob", http.StatusFound) // 302
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	c.AllowPrivateDownloadHosts = true
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/api/download")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after following redirect", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Errorf("body after redirect = %q, want %q", got, body)
	}
}

// refreshOnceSource is a TokenSource that returns badTok first and, once
// Refresh is called, returns goodTok thereafter.
type refreshOnceSource struct {
	badTok, goodTok string
	refreshed       bool
}

func (s *refreshOnceSource) Token(context.Context) (string, error) {
	if s.refreshed {
		return s.goodTok, nil
	}
	return s.badTok, nil
}
func (s *refreshOnceSource) Refresh(context.Context) (string, error) {
	s.refreshed = true
	return s.goodTok, nil
}

func TestDownloadFileRefreshesOn401(t *testing.T) {
	const body = "AFTER-REFRESH"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := NewWithSource(srv.URL, &refreshOnceSource{badTok: "bad", goodTok: "good"}, "")
	c.AllowPrivateDownloadHosts = true
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/file")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after refresh+retry", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestDownloadFileKeeps401WhenNotRefreshable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()

	// StaticToken can't refresh → the 401 is returned to the caller.
	c := New(srv.URL, "personal-key", "")
	c.AllowPrivateDownloadHosts = true
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/file")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 preserved for a non-refreshable source", resp.StatusCode)
	}
}

func TestPrimaryFile(t *testing.T) {
	files := []ModelVersionFile{
		{Name: "a.safetensors", Type: "Model", Primary: false},
		{Name: "b.safetensors", Type: "Model", Primary: true},
	}
	if pf := PrimaryFile(files); pf == nil || pf.Name != "b.safetensors" {
		t.Errorf("PrimaryFile picked %+v, want the primary-flagged file", pf)
	}
	// No primary flag => first file.
	nofl := []ModelVersionFile{{Name: "first"}, {Name: "second"}}
	if pf := PrimaryFile(nofl); pf == nil || pf.Name != "first" {
		t.Errorf("PrimaryFile fallback = %+v, want first", pf)
	}
	if PrimaryFile(nil) != nil {
		t.Error("PrimaryFile(nil) should be nil")
	}
}

func TestIsModelWeights(t *testing.T) {
	if !(&ModelVersionFile{Type: "Model"}).IsModelWeights() {
		t.Error(`type "Model" should be weights`)
	}
	for _, ty := range []string{"Training Data", "Config", "VAE", ""} {
		if (&ModelVersionFile{Type: ty}).IsModelWeights() {
			t.Errorf("type %q should NOT be weights", ty)
		}
	}
}

// Guard: the file struct still decodes the live-verified field names.
func TestModelVersionFileDecodesLiveFields(t *testing.T) {
	const raw = `{"id":93211,"name":"dreamshaper_8.safetensors","type":"Model","sizeKB":2082642.47,"primary":true,"downloadUrl":"https://civitai.com/api/download/models/128713","hashes":{"SHA256":"879DB523","AutoV2":"879DB523C3"}}`
	var f ModelVersionFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if f.ID != 93211 || f.Name != "dreamshaper_8.safetensors" || f.Type != "Model" || !f.Primary {
		t.Errorf("decoded file wrong: %+v", f)
	}
	if !strings.Contains(f.DownloadURL, "/api/download/models/128713") {
		t.Errorf("downloadUrl = %q", f.DownloadURL)
	}
	if f.Hashes.SHA256 != "879DB523" {
		t.Errorf("hashes.SHA256 = %q", f.Hashes.SHA256)
	}
}
