package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	res, err := c.SubmitVersion(context.Background(), []byte("ZIPDATA"), "my-block", "0.1.0")
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
	_, err := c.SubmitVersion(context.Background(), []byte("x"), "my-block", "0.1.0")
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

func TestWhoAmIParsesTokenScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A full-scope personal key: UserRead | AIServicesWrite | BuzzRead.
		_, _ = w.Write([]byte(`{"username":"zach","id":42,"tokenScope":98305}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if id.TokenScope == nil || *id.TokenScope != (ScopeUserRead|ScopeAIServicesWrite|ScopeBuzzRead) {
		t.Errorf("tokenScope = %v", id.TokenScope)
	}
	if !id.ScopeKnown() {
		t.Error("ScopeKnown should be true when tokenScope is present")
	}
	if !id.CanSpendBuzz() {
		t.Error("CanSpendBuzz should be true for a full-scope key")
	}
	if !id.CanReadBuzz() {
		t.Error("CanReadBuzz should be true for a full-scope key")
	}
}

// scopePtr is a test helper for building an Identity with a known scope mask.
func scopePtr(v int) *int { return &v }

func TestIdentityCapabilityBits(t *testing.T) {
	// An OAuth login token: UserRead | AppBlocksSubmit (bit 25) — no spend, no
	// balance-read.
	oauth := &Identity{TokenScope: scopePtr(ScopeUserRead | ScopeAppBlocksSubmit)}
	if oauth.CanSpendBuzz() {
		t.Error("OAuth token must not be able to spend Buzz")
	}
	if oauth.CanReadBuzz() {
		t.Error("OAuth token must not be able to read Buzz balance")
	}

	spendOnly := &Identity{TokenScope: scopePtr(ScopeAIServicesWrite)}
	if !spendOnly.CanSpendBuzz() || spendOnly.CanReadBuzz() {
		t.Errorf("AIServicesWrite-only: spend=%v read=%v", spendOnly.CanSpendBuzz(), spendOnly.CanReadBuzz())
	}

	readOnly := &Identity{TokenScope: scopePtr(ScopeBuzzRead)}
	if readOnly.CanSpendBuzz() || !readOnly.CanReadBuzz() {
		t.Errorf("BuzzRead-only: spend=%v read=%v", readOnly.CanSpendBuzz(), readOnly.CanReadBuzz())
	}
}

// TestIdentityScopeUnknown: an absent tokenScope must degrade to "unknown", not
// decode as "no capabilities".
func TestIdentityScopeUnknown(t *testing.T) {
	id := &Identity{Username: "zach", ID: 1} // no TokenScope
	if id.ScopeKnown() {
		t.Error("ScopeKnown should be false when tokenScope is absent")
	}
	if id.CanSpendBuzz() || id.CanReadBuzz() {
		t.Error("unknown scope must not report capabilities as true")
	}
	if id.DecodeScopes() != nil {
		t.Errorf("unknown scope should decode to nil, got %v", id.DecodeScopes())
	}
}

// TestDecodeScopes exercises the bit-decode helper against representative masks.
func TestDecodeScopes(t *testing.T) {
	full := &Identity{TokenScope: scopePtr(ScopeFull)}
	names := full.DecodeScopes()
	// Full is bits 0..24 (25 bits) and EXCLUDES AppBlocksSubmit.
	if len(names) != 25 {
		t.Errorf("Full should decode to 25 scopes, got %d: %v", len(names), names)
	}
	if containsStr(names, "AppBlocksSubmit") {
		t.Errorf("Full must NOT include AppBlocksSubmit: %v", names)
	}
	for _, want := range []string{"UserRead", "AIServicesWrite", "BuzzRead", "VaultWrite"} {
		if !containsStr(names, want) {
			t.Errorf("Full should include %s: %v", want, names)
		}
	}

	fullSubmit := &Identity{TokenScope: scopePtr(ScopeFull | ScopeAppBlocksSubmit)}
	if !containsStr(fullSubmit.DecodeScopes(), "AppBlocksSubmit") {
		t.Errorf("Full|AppBlocksSubmit should include AppBlocksSubmit: %v", fullSubmit.DecodeScopes())
	}

	// A typical civitai-cli OAuth mask: UserRead + AppBlocksSubmit, no spend/read.
	oauth := &Identity{TokenScope: scopePtr(ScopeUserRead | ScopeAppBlocksSubmit)}
	got := oauth.DecodeScopes()
	if len(got) != 2 || got[0] != "UserRead" || got[1] != "AppBlocksSubmit" {
		t.Errorf("OAuth mask decode = %v, want [UserRead AppBlocksSubmit]", got)
	}
	if containsStr(got, "AIServicesWrite") || containsStr(got, "BuzzRead") {
		t.Errorf("OAuth mask must lack spend/read scopes: %v", got)
	}
}

// TestCredentialType maps subject.type to a human label.
func TestCredentialType(t *testing.T) {
	oauth := &Identity{Subject: &Subject{Type: "oauth", ID: "1"}}
	if oauth.CredentialType() != "OAuth login" || !oauth.IsOAuth() {
		t.Errorf("oauth subject: type=%q isOAuth=%v", oauth.CredentialType(), oauth.IsOAuth())
	}
	key := &Identity{Subject: &Subject{Type: "apiKey", ID: "1"}}
	if key.CredentialType() != "personal API key" || key.IsOAuth() {
		t.Errorf("apiKey subject: type=%q isOAuth=%v", key.CredentialType(), key.IsOAuth())
	}
	absent := &Identity{}
	if absent.CredentialType() != "unknown" || absent.IsOAuth() {
		t.Errorf("absent subject: type=%q isOAuth=%v", absent.CredentialType(), absent.IsOAuth())
	}
}

// TestWhoAmIParsesSubjectAndBuzzLimit confirms the extended /api/v1/me fields
// (subject + buzzLimit) parse.
func TestWhoAmIParsesSubjectAndBuzzLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"zach","id":42,"tokenScope":98305,"buzzLimit":5000,"subject":{"type":"oauth","id":"abc"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("WhoAmI: %v", err)
	}
	if !id.IsOAuth() {
		t.Errorf("subject.type oauth should set IsOAuth: %+v", id.Subject)
	}
	if id.BuzzLimit == nil || *id.BuzzLimit != 5000 {
		t.Errorf("buzzLimit = %v, want 5000", id.BuzzLimit)
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestBuzzAccountTotal checks the Total helper.
func TestBuzzAccountTotal(t *testing.T) {
	a := &BuzzAccount{Blue: 5, Green: 7, Yellow: 4242}
	if a.Total() != 4254 {
		t.Errorf("Total = %d, want 4254", a.Total())
	}
}

func TestGetBuzzAccountParsesBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != BuzzAccountPath {
			t.Errorf("path = %q, want %q", r.URL.Path, BuzzAccountPath)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"result":{"data":{"json":{"blue":10,"green":20,"yellow":1234}}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	acct, err := c.GetBuzzAccount(context.Background())
	if err != nil {
		t.Fatalf("GetBuzzAccount: %v", err)
	}
	if acct.Yellow != 1234 || acct.Blue != 10 || acct.Green != 20 {
		t.Errorf("balance = %+v", acct)
	}
}

func TestGetBuzzAccount403ReturnsScopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"json": map[string]any{
				"message": "Your API key does not have the required scope for this action",
			}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	_, err := c.GetBuzzAccount(context.Background())
	if !errors.Is(err, ErrBuzzScope) {
		t.Errorf("expected ErrBuzzScope, got %v", err)
	}
}

func TestGetBuzzAccountRequiresToken(t *testing.T) {
	c := New("https://example.com", "", "")
	if _, err := c.GetBuzzAccount(context.Background()); err == nil {
		t.Fatal("expected error with no token")
	}
}
