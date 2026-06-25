package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSubmitTimeoutIsLongAndScoped pins the F6 fix: the submit upload gets a
// substantially longer timeout than the fast, interactive calls, scoped to the
// submit client only. A short shared timeout previously produced false
// "context deadline exceeded" failures on submits that had already succeeded
// server-side, so the user's retry hit "you already have a pending submission".
func TestSubmitTimeoutIsLongAndScoped(t *testing.T) {
	if submitTimeout != 120*time.Second {
		t.Errorf("submitTimeout = %v, want 120s", submitTimeout)
	}
	if defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout = %v, want 30s", defaultTimeout)
	}
	if submitTimeout <= defaultTimeout {
		t.Errorf("submit timeout (%v) must exceed the fast-call timeout (%v)", submitTimeout, defaultTimeout)
	}

	c := New("https://example.com", "tok", "")
	// The shared client stays on the short fast-call timeout...
	if c.HTTP.Timeout != defaultTimeout {
		t.Errorf("shared client timeout = %v, want %v (fast calls must not be lengthened)", c.HTTP.Timeout, defaultTimeout)
	}
	// ...while the submit client uses the long timeout.
	if got := c.submitClient().Timeout; got != submitTimeout {
		t.Errorf("submit client timeout = %v, want %v", got, submitTimeout)
	}
}

func TestSubmitVersionSendsBearerAndBase64(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		var body submitBody
		_ = json.Unmarshal(b, &body)
		gotBody = body.BundleBase64
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SubmitResult{
			PublishRequestID: "pr_1", Slug: "my-block", Version: "0.1.0", Status: "pending",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "/api/blocks/submit-version")
	res, err := c.SubmitVersion(context.Background(), []byte("ZIPDATA"))
	if err != nil {
		t.Fatalf("SubmitVersion: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth header = %q, want Bearer tok123", gotAuth)
	}
	decoded, _ := base64.StdEncoding.DecodeString(gotBody)
	if string(decoded) != "ZIPDATA" {
		t.Errorf("decoded body = %q, want ZIPDATA", decoded)
	}
	if res.PublishRequestID != "pr_1" || res.Status != "pending" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestSubmitVersionSurfacesServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "bundle is missing required file: block.manifest.json"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.SubmitVersion(context.Background(), []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing required file") {
		t.Errorf("error %q should surface the server message", err)
	}
}

func TestWhoAmIRequiresToken(t *testing.T) {
	c := New("https://example.com", "", "")
	if _, err := c.WhoAmI(context.Background()); err == nil {
		t.Fatal("expected error with no token")
	}
}

func TestWhoAmIParsesIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "nope"})
			return
		}
		_ = json.NewEncoder(w).Encode(Identity{Username: "zach", ID: 42})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.Username != "zach" || id.ID != 42 {
		t.Errorf("identity = %+v", id)
	}
}
