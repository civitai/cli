package appapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// trpcOK writes a tRPC success envelope {result:{data:{json:<v>}}}.
func trpcOK(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": map[string]any{"data": map[string]any{"json": v}},
	})
}

// trpcErr writes a tRPC error envelope with the given HTTP status + message.
func trpcErr(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"json": map[string]any{"message": msg, "code": -32600}},
	})
}

func TestGetMyListingForApp(t *testing.T) {
	var gotInput, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotInput = r.URL.Query().Get("input")
		gotAuth = r.Header.Get("Authorization")
		trpcOK(w, map[string]any{
			"appListingId":       "listing_1",
			"status":             "draft",
			"contentRating":      "g",
			"hasPendingRevision": false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok123", "")
	ref, err := c.GetMyListingForApp(context.Background(), "block_9")
	if err != nil {
		t.Fatalf("GetMyListingForApp: %v", err)
	}
	if gotAuth != "Bearer tok123" {
		t.Errorf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotInput, `"appBlockId":"block_9"`) {
		t.Errorf("input should carry appBlockId: %q", gotInput)
	}
	if ref.AppListingID != "listing_1" || ref.Status != "draft" {
		t.Errorf("ref = %+v", ref)
	}
}

func TestIngestAssetFromDataURI(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		trpcOK(w, 4242)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	id, err := c.IngestAssetFromDataURI(context.Background(), []byte("PNGBYTES"), "image/png")
	if err != nil {
		t.Fatalf("IngestAssetFromDataURI: %v", err)
	}
	if id != 4242 {
		t.Errorf("imageId = %d, want 4242", id)
	}
	// The bytes travel under {"json":{"dataUri":"data:image/png;base64,...","kind":"icon"}}.
	jsonField, _ := gotBody["json"].(map[string]any)
	dataURI, _ := jsonField["dataUri"].(string)
	if !strings.HasPrefix(dataURI, "data:image/png;base64,") {
		t.Errorf("dataUri = %q", dataURI)
	}
	if jsonField["kind"] != "icon" {
		t.Errorf("kind = %v, want icon", jsonField["kind"])
	}
}

func TestIngestAssetFullRes(t *testing.T) {
	var minted, put, persisted bool
	var persistBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == ImageUploadPath:
			minted = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "uuid-key", "uploadURL": "http://" + r.Host + "/put/uuid-key"})
		case strings.HasPrefix(r.URL.Path, "/put/"):
			put = true
			if r.Method != http.MethodPut {
				t.Errorf("presigned upload method = %s, want PUT", r.Method)
			}
			// The presigned PUT must NOT carry our bearer.
			if r.Header.Get("Authorization") != "" {
				t.Errorf("presigned PUT should not carry Authorization")
			}
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "persistAssetImage"):
			persisted = true
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &persistBody)
			trpcOK(w, 777)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	info := ImageInfo{Width: 1280, Height: 720, MimeType: "image/jpeg"}
	id, err := c.IngestAssetFullRes(context.Background(), []byte("JPEGBYTES"), info)
	if err != nil {
		t.Fatalf("IngestAssetFullRes: %v", err)
	}
	if !minted || !put || !persisted {
		t.Fatalf("flow incomplete: minted=%v put=%v persisted=%v", minted, put, persisted)
	}
	if id != 777 {
		t.Errorf("imageId = %d, want 777", id)
	}
	jsonField, _ := persistBody["json"].(map[string]any)
	if jsonField["url"] != "uuid-key" {
		t.Errorf("persist url = %v, want uuid-key", jsonField["url"])
	}
	if jsonField["width"].(float64) != 1280 || jsonField["mimeType"] != "image/jpeg" {
		t.Errorf("persist body = %+v", jsonField)
	}
}

func TestGetAssetScanStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trpcOK(w, map[string]any{"statuses": []map[string]any{
			{"imageId": 5, "status": "scanned"},
		}})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	got, err := c.GetAssetScanStatuses(context.Background(), []int{5})
	if err != nil {
		t.Fatalf("GetAssetScanStatuses: %v", err)
	}
	if len(got) != 1 || got[0].ImageID != 5 || got[0].Status != "scanned" {
		t.Errorf("statuses = %+v", got)
	}
}

func TestSetIcon(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		trpcOK(w, map[string]any{"status": "attached", "iconId": 9})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	res, err := c.SetIcon(context.Background(), "listing_1", 9)
	if err != nil {
		t.Fatalf("SetIcon: %v", err)
	}
	if res.Status != "attached" || res.IconID == nil || *res.IconID != 9 {
		t.Errorf("res = %+v", res)
	}
	jsonField, _ := body["json"].(map[string]any)
	if jsonField["listingId"] != "listing_1" || jsonField["imageId"].(float64) != 9 {
		t.Errorf("setIcon body = %+v", jsonField)
	}
}

func TestBeginAndSubmitRevision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "beginListingRevision"):
			trpcOK(w, map[string]any{"shadowId": "shadow_7", "created": true})
		case strings.Contains(r.URL.Path, "submitListingRevision"):
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			jsonField, _ := body["json"].(map[string]any)
			if jsonField["changelog"] != "new icon" {
				t.Errorf("changelog = %v", jsonField["changelog"])
			}
			trpcOK(w, map[string]any{"publishRequestId": "pubreq_3", "shadowId": "shadow_7", "slug": "my-app"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "")
	shadow, created, err := c.BeginListingRevision(context.Background(), "listing_1")
	if err != nil || shadow != "shadow_7" || !created {
		t.Fatalf("begin: shadow=%q created=%v err=%v", shadow, created, err)
	}
	rev, err := c.SubmitListingRevision(context.Background(), "shadow_7", "new icon")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if rev.PublishRequestID != "pubreq_3" || rev.Slug != "my-app" {
		t.Errorf("rev = %+v", rev)
	}
}

func TestListingErrorMapping(t *testing.T) {
	cases := []struct {
		status     int
		msg        string
		wantSubstr string
		echoMsg    bool // whether the server message is surfaced (401 deliberately drops it)
	}{
		{http.StatusUnauthorized, "expired", "not logged in", false},
		{http.StatusForbidden, "Your API key does not have the required scope for this action", "Apps submit scope", true},
		{http.StatusForbidden, "Apps authoring is not enabled", "Apps-author access", true},
		{http.StatusNotFound, "no listing found", "no store listing found", true},
		{http.StatusBadRequest, "This listing is live", "rejected the request", true},
	}
	for _, tc := range cases {
		body, _ := json.Marshal(map[string]any{"error": map[string]any{"json": map[string]any{"message": tc.msg}}})
		err := listingError(tc.status, body, trpcSetIcon)
		if err == nil {
			t.Fatalf("status %d: expected error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.wantSubstr) {
			t.Errorf("status %d error %q should contain %q", tc.status, err, tc.wantSubstr)
		}
		if tc.echoMsg && !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("status %d error %q should echo server msg %q", tc.status, err, tc.msg)
		}
	}
}

// TestSetIconScopeForbiddenEndToEnd confirms a 403 flows through listingError.
func TestSetIconScopeForbiddenEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trpcErr(w, http.StatusForbidden, "Your API key does not have the required scope for this action")
	}))
	defer srv.Close()
	c := New(srv.URL, "tok", "")
	_, err := c.SetIcon(context.Background(), "listing_1", 1)
	if err == nil || !strings.Contains(err.Error(), "Apps submit scope") {
		t.Errorf("want scope-403 error, got %v", err)
	}
}
