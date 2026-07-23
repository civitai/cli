package civitai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// hashOf returns a deterministic valid 64-char uppercase SHA256-shaped hex
// string for the byte b, so tests can mint distinct valid hashes cheaply.
func hashOf(b byte) string {
	return strings.ToUpper(strings.Repeat(fmt.Sprintf("%02x", b), 32))
}

// batchHandler is a fake /by-hash/ids endpoint. It records every POST's decoded
// body and, for each request, returns a HashMatch for any requested hash present
// in matches (keyed by uppercase hash).
func batchHandler(t *testing.T, matches map[string]HashMatch) (http.HandlerFunc, *[][]string, *string) {
	t.Helper()
	var mu sync.Mutex
	var bodies [][]string
	var gotAuth string
	h := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != batchByHashPath {
			t.Errorf("path = %q, want %q", r.URL.Path, batchByHashPath)
		}
		raw, _ := io.ReadAll(r.Body)
		var req []string
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("request body is not a JSON array: %q", raw)
		}
		mu.Lock()
		bodies = append(bodies, req)
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		var resp []HashMatch
		for _, h := range req {
			if m, ok := matches[strings.ToUpper(h)]; ok {
				resp = append(resp, m)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
	return h, &bodies, &gotAuth
}

func TestGetModelVersionsByHashesMapsAndAuth(t *testing.T) {
	h0, h1, h2 := hashOf(0xa0), hashOf(0xb1), hashOf(0xc2)
	matches := map[string]HashMatch{
		h0: {ModelVersionID: 100, ModelID: 10, Hash: h0},
		h2: {ModelVersionID: 300, ModelID: 30, Hash: h2},
	}
	handler, bodies, gotAuth := batchHandler(t, matches)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := New(srv.URL, "tok-9")
	// h1 has no match (a genuine miss); it must simply be absent from the result.
	got, err := c.GetModelVersionsByHashes(context.Background(), []string{h0, h1, h2})
	if err != nil {
		t.Fatalf("GetModelVersionsByHashes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d: %+v", len(got), got)
	}
	byHash := map[string]HashMatch{}
	for _, m := range got {
		byHash[m.Hash] = m
	}
	if m := byHash[h0]; m.ModelVersionID != 100 || m.ModelID != 10 {
		t.Errorf("h0 match wrong: %+v", m)
	}
	if m := byHash[h2]; m.ModelVersionID != 300 || m.ModelID != 30 {
		t.Errorf("h2 match wrong: %+v", m)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly 1 POST, got %d", len(*bodies))
	}
	if *gotAuth != "Bearer tok-9" {
		t.Errorf("auth = %q, want Bearer tok-9", *gotAuth)
	}
}

func TestGetModelVersionsByHashesUppercasesAndValidates(t *testing.T) {
	valid := hashOf(0x4d)
	handler, bodies, _ := batchHandler(t, map[string]HashMatch{
		valid: {ModelVersionID: 1, ModelID: 2, Hash: valid},
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := New(srv.URL, "")
	// A lowercase valid hash (must be upper-cased on the wire), a too-short one,
	// a non-hex one, and blank — the invalid three are skipped, not sent.
	got, err := c.GetModelVersionsByHashes(context.Background(), []string{
		strings.ToLower(valid), "abc123", strings.Repeat("z", 64), "  ",
	})
	if err != nil {
		t.Fatalf("GetModelVersionsByHashes: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want 1 POST, got %d", len(*bodies))
	}
	sent := (*bodies)[0]
	if len(sent) != 1 || sent[0] != valid {
		t.Errorf("only the valid hash should be sent, upper-cased: got %v", sent)
	}
	if len(got) != 1 || got[0].ModelVersionID != 1 {
		t.Errorf("match wrong: %+v", got)
	}
}

func TestGetModelVersionsByHashesChunksOver10k(t *testing.T) {
	handler, bodies, _ := batchHandler(t, map[string]HashMatch{})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// 10001 distinct valid hashes -> ceil(10001/10000) = 2 POSTs (10000 + 1).
	const n = maxHashesPerBatch + 1
	in := make([]string, 0, n)
	for i := 0; i < n; i++ {
		in = append(in, fmt.Sprintf("%064X", i))
	}
	c := New(srv.URL, "")
	if _, err := c.GetModelVersionsByHashes(context.Background(), in); err != nil {
		t.Fatalf("GetModelVersionsByHashes: %v", err)
	}
	if len(*bodies) != 2 {
		t.Fatalf("want 2 chunked POSTs, got %d", len(*bodies))
	}
	if len((*bodies)[0]) != maxHashesPerBatch || len((*bodies)[1]) != 1 {
		t.Errorf("chunk sizes = %d,%d, want %d,1", len((*bodies)[0]), len((*bodies)[1]), maxHashesPerBatch)
	}
}

func TestGetModelVersionsByHashesChunksMergeResults(t *testing.T) {
	// Two chunks each returning one match -> merged into a 2-element result.
	first := fmt.Sprintf("%064X", 0)
	last := fmt.Sprintf("%064X", maxHashesPerBatch) // the sole hash in chunk 2
	handler, bodies, _ := batchHandler(t, map[string]HashMatch{
		first: {ModelVersionID: 11, ModelID: 1, Hash: first},
		last:  {ModelVersionID: 22, ModelID: 2, Hash: last},
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	const n = maxHashesPerBatch + 1
	in := make([]string, 0, n)
	for i := 0; i < n; i++ {
		in = append(in, fmt.Sprintf("%064X", i))
	}
	c := New(srv.URL, "")
	got, err := c.GetModelVersionsByHashes(context.Background(), in)
	if err != nil {
		t.Fatalf("GetModelVersionsByHashes: %v", err)
	}
	if len(*bodies) != 2 {
		t.Fatalf("want 2 POSTs, got %d", len(*bodies))
	}
	if len(got) != 2 {
		t.Fatalf("want 2 merged matches, got %d: %+v", len(got), got)
	}
}

func TestGetModelVersionsByHashesEmptyMakesNoRequest(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	for _, in := range [][]string{nil, {}, {"", "  ", "not-a-hash"}} {
		got, err := c.GetModelVersionsByHashes(context.Background(), in)
		if err != nil {
			t.Fatalf("input %v: %v", in, err)
		}
		if len(got) != 0 {
			t.Errorf("input %v: want empty result, got %+v", in, got)
		}
	}
	if called {
		t.Error("no HTTP request should be made for empty/all-invalid input")
	}
}

func TestGetModelVersionsByHashesRateLimited(t *testing.T) {
	// A 429 WITHOUT Retry-After is terminal (no retry) and classified ErrRateLimited.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = io.Discard
	_, err := c.GetModelVersionsByHashes(context.Background(), []string{hashOf(0x01)})
	if err == nil {
		t.Fatal("expected an error for a 429")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("429 should classify as ErrRateLimited, got: %v", err)
	}
}

func TestGetModelVersionsByHashesMalformedBody(t *testing.T) {
	// A 200 whose body is not a JSON array of HashMatch -> a clear parse error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totally":"not an array"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.GetModelVersionsByHashes(context.Background(), []string{hashOf(0x02)})
	if err == nil {
		t.Fatal("expected a parse error for a malformed body")
	}
	if !strings.Contains(err.Error(), "unexpected response") {
		t.Errorf("want an 'unexpected response' parse error, got: %v", err)
	}
}

func TestGetModelVersionsByHashesServerError(t *testing.T) {
	// A persistent 503 is retried then surfaces as ErrNetwork (service unavailable).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"overloaded"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	c.RetryBackoffBase = zeroBackoff()
	c.Stderr = io.Discard
	_, err := c.GetModelVersionsByHashes(context.Background(), []string{hashOf(0x03)})
	if err == nil {
		t.Fatal("expected an error for a persistent 503")
	}
	if !errors.Is(err, ErrNetwork) {
		t.Errorf("503 should classify as ErrNetwork, got: %v", err)
	}
}

// TestReaderInterfaceHasBatchByHash is a compile-time assertion that *Client
// satisfies the Reader interface (which now includes GetModelVersionsByHashes).
func TestReaderInterfaceHasBatchByHash(t *testing.T) {
	var _ Reader = (*Client)(nil)
}
