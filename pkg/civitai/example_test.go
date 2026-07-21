package civitai_test

// This file is the external-consumer compile-and-behavior guard for the SDK.
//
// It lives in package `civitai_test` (NOT `civitai`), so it can only touch the
// EXPORTED surface of github.com/civitai/cli/pkg/civitai — exactly what an
// outside module (e.g. civitai-manager) sees. It imports NO internal/* package.
// If the public API ever stops being usable from outside the module, this stops
// compiling.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestExternalConsumer_ReaderWithNew exercises the personal-API-key constructor
// path (civitai.New) and a Reader call end-to-end against an httptest server,
// asserting the bearer token is sent and a typed model is decoded.
func TestExternalConsumer_ReaderWithNew(t *testing.T) {
	const apiKey = "personal-api-key-123"

	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": 42,
			"name": "DreamShaper",
			"type": "Checkpoint",
			"modelVersions": [{
				"id": 128713,
				"name": "v8",
				"files": [{
					"id": 93211,
					"name": "dreamshaper_8.safetensors",
					"type": "Model",
					"primary": true,
					"downloadUrl": "`+"https://civitai.com/api/download/models/128713"+`",
					"hashes": {"SHA256": "879DB523"}
				}]
			}]
		}`)
	}))
	defer srv.Close()

	// Personal-API-key constructor: an outside caller needs only the base URL and
	// the key. The empty submitPath keeps the default (read-only consumers never
	// hit it).
	client := civitai.New(srv.URL, apiKey)

	var _ civitai.Reader = client // the concrete client satisfies the public interface

	md, raw, err := client.GetModel(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if gotAuth != "Bearer "+apiKey {
		t.Errorf("Authorization = %q, want bearer with the personal key", gotAuth)
	}
	if gotPath != "/api/v1/models/42" {
		t.Errorf("path = %q, want /api/v1/models/42", gotPath)
	}
	if md.ID != 42 || md.Name != "DreamShaper" {
		t.Errorf("decoded ModelDetail = %+v, want id 42 / DreamShaper", md)
	}
	if len(raw) == 0 {
		t.Error("expected raw JSON bytes alongside the typed model")
	}
}

// TestExternalConsumer_NewWithSource proves the TokenSource seam is usable from
// outside: StaticToken is a public TokenSource, and a downloader consumer can
// pick the primary file and read the download-relevant typed fields.
func TestExternalConsumer_NewWithSource(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": 128713,
			"modelId": 42,
			"name": "v8",
			"baseModel": "SD 1.5",
			"files": [
				{"id": 1, "name": "pruned.safetensors", "type": "Model", "primary": false,
				 "downloadUrl": "`+"https://civitai.com/api/download/models/1"+`", "hashes": {"SHA256": "AAA"}},
				{"id": 2, "name": "full.safetensors", "type": "Model", "primary": true,
				 "downloadUrl": "`+"https://civitai.com/api/download/models/2"+`", "hashes": {"SHA256": "BBB"}}
			]
		}`)
	}))
	defer srv.Close()

	// NewWithSource with the public StaticToken TokenSource — the same seam an
	// OAuth-backed consumer would use with its own TokenSource implementation.
	var src civitai.TokenSource = civitai.StaticToken("key-from-source")
	client := civitai.NewWithSource(srv.URL, src)

	mv, _, err := client.GetModelVersion(context.Background(), "128713")
	if err != nil {
		t.Fatalf("GetModelVersion: %v", err)
	}
	if gotAuth != "Bearer key-from-source" {
		t.Errorf("Authorization = %q, want bearer from the TokenSource", gotAuth)
	}

	// PrimaryFile + the download-relevant typed fields a downloader consumer needs.
	pf := civitai.PrimaryFile(mv.Files)
	if pf == nil {
		t.Fatal("PrimaryFile returned nil")
	}
	var _ civitai.ModelVersionFile = *pf
	var _ civitai.FileHashes = pf.Hashes
	if pf.ID != 2 || !pf.Primary || pf.Hashes.SHA256 != "BBB" {
		t.Errorf("primary file = %+v, want id 2 / primary / SHA256 BBB", pf)
	}
	if !civitai.DownloadNeedsAuth(pf.DownloadURL, srv.URL) {
		t.Error("DownloadNeedsAuth should report the civitai download route needs auth")
	}
}

// TestExternalConsumer_ErrNotFound proves the classification sentinels are
// reachable and matchable via errors.Is from outside the package.
func TestExternalConsumer_ErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}))
	defer srv.Close()

	client := civitai.New(srv.URL, "") // anonymous is fine for a public read
	_, _, err := client.GetModel(context.Background(), "999999")
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
}

// TestExternalConsumer_Search exercises a list/search Reader call with url.Values
// and reads the pagination envelope — the shape a subscription poller consumes.
func TestExternalConsumer_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "anime" {
			t.Errorf("query = %q, want anime", r.URL.Query().Get("query"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[{"id":1,"name":"m1"}],"metadata":{"nextCursor":"abc"}}`)
	}))
	defer srv.Close()

	client := civitai.New(srv.URL, "")
	res, err := client.SearchModels(context.Background(), url.Values{"query": {"anime"}})
	if err != nil {
		t.Fatalf("SearchModels: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "m1" {
		t.Errorf("items = %+v, want one item named m1", res.Items)
	}
	// The metadata envelope round-trips as an exported type.
	var _ civitai.Metadata = res.Metadata
	if cur := res.Metadata.CursorString(); cur != "abc" {
		t.Errorf("CursorString = %q, want abc", cur)
	}
}

// Compile-time assertions that the documented public interfaces are satisfied by
// the concrete *Client, so a consumer can program against the interfaces.
var (
	_ civitai.Reader     = (*civitai.Client)(nil)
	_ civitai.Downloader = (*civitai.Client)(nil)
)
