package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// buildModelsPayload builds a valid `{"items":[…],"metadata":{}}` JSON body of
// at least minBytes, returning the payload and the exact item count so a test
// can assert the full body was read + parsed (no truncation). Each item carries
// a fixed filler so the body grows deterministically.
func buildModelsPayload(minBytes int) (string, int) {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	filler := strings.Repeat("a", 512)
	n := 0
	for b.Len() < minBytes {
		if n > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"name":"%s","type":"LORA"}`, n, filler)
		n++
	}
	b.WriteString(`],"metadata":{}}`)
	return b.String(), n
}

// serveBody returns an httptest server that writes body (as JSON) for any GET.
func serveBody(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSearchModelsReadsBodyOver1MiB is the regression test for the client-side
// 1 MiB read cap. Before the fix the read path used
// io.ReadAll(io.LimitReader(resp.Body, 1<<20)), which truncated any response
// over 1 MiB mid-JSON, so json.Unmarshal failed with a misleading
// "unexpected response from /api/v1/models (status 200)". This exercises the
// real read seam (SearchModels → getInto) against a ~1.5 MiB valid body and
// asserts the FULL body is read + parsed (every item present).
func TestSearchModelsReadsBodyOver1MiB(t *testing.T) {
	body, want := buildModelsPayload(1500000) // ~1.5 MiB, > 1<<20
	if len(body) <= 1<<20 {
		t.Fatalf("test payload not larger than 1 MiB: %d bytes", len(body))
	}
	srv := serveBody(t, body)

	c := New(srv.URL, "tok", "")
	res, err := c.SearchModels(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchModels on a >1MiB body: %v", err)
	}
	if len(res.Items) != want {
		t.Errorf("parsed %d items, want %d — body was truncated", len(res.Items), want)
	}
	if len(res.Raw) != len(body) {
		t.Errorf("Raw body = %d bytes, want the full %d bytes", len(res.Raw), len(body))
	}
	// Last item must be present + intact (truncation drops the tail first).
	if res.Items[want-1].ID != want-1 {
		t.Errorf("last item id = %d, want %d", res.Items[want-1].ID, want-1)
	}
}

// TestSearchModelsBoundaryAroundOneMiB checks a body just under and just over
// the old 1 MiB cap both parse fully.
func TestSearchModelsBoundaryAroundOneMiB(t *testing.T) {
	cases := []struct {
		name    string
		minSize int
	}{
		{"justUnder1MiB", 1<<20 - 20000},
		{"justOver1MiB", 1<<20 + 20000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, want := buildModelsPayload(tc.minSize)
			srv := serveBody(t, body)
			c := New(srv.URL, "", "")
			res, err := c.SearchModels(context.Background(), url.Values{})
			if err != nil {
				t.Fatalf("SearchModels (%d bytes): %v", len(body), err)
			}
			if len(res.Items) != want {
				t.Errorf("parsed %d items, want %d (body %d bytes)", len(res.Items), want, len(body))
			}
		})
	}
}

// TestReadBodyOverCapReturnsError asserts a body at/over the effective cap
// yields a clear error rather than silently handing truncated bytes to the
// parser. A small Client.MaxResponseBody override stands in for the 64 MiB
// default so the test needn't allocate 64 MiB.
func TestReadBodyOverCapReturnsError(t *testing.T) {
	const capBytes = 4096
	body, _ := buildModelsPayload(capBytes * 4) // comfortably over the cap
	srv := serveBody(t, body)

	c := New(srv.URL, "", "")
	c.MaxResponseBody = capBytes
	_, err := c.SearchModels(context.Background(), url.Values{})
	if err == nil {
		t.Fatal("expected an over-cap error, got nil (silent truncation)")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("error = %q, want it to mention the body exceeded the limit", err)
	}
}

// TestReadBodyUnderCapReturnsFullBody asserts a body at/under the cap is read in
// full (no off-by-one truncation at the boundary).
func TestReadBodyUnderCapReturnsFullBody(t *testing.T) {
	body := strings.Repeat("x", 1000)
	cases := []struct {
		name  string
		limit int64
	}{
		{"underCap", 2000},
		{"exactlyCap", 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := readResponseBody(strings.NewReader(body), tc.limit)
			if err != nil {
				t.Fatalf("readResponseBody: %v", err)
			}
			if len(raw) != len(body) {
				t.Errorf("read %d bytes, want %d", len(raw), len(body))
			}
		})
	}
}

// TestReadResponseBodyOverLimitError unit-tests the helper directly: a body one
// byte over the limit errors and does not silently truncate.
func TestReadResponseBodyOverLimitError(t *testing.T) {
	body := strings.Repeat("x", 1025)
	_, err := readResponseBody(strings.NewReader(body), 1024)
	if err == nil {
		t.Fatal("expected an over-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("error = %q, want 'exceeded'", err)
	}
}
