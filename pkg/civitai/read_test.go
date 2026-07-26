package civitai

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestServer records the last request path + query and returns body for any
// GET.
func newTestServer(t *testing.T, body string) (*httptest.Server, *string, *url.Values, *string) {
	t.Helper()
	var gotPath string
	var gotQuery url.Values
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath, &gotQuery, &gotAuth
}

func TestSearchModelsBuildsRequestAndParses(t *testing.T) {
	const body = `{
	  "items": [
	    {"id": 1, "name": "Pony", "type": "Checkpoint", "nsfw": false,
	     "creator": {"username": "bob"},
	     "stats": {"downloadCount": 10, "thumbsUpCount": 3, "commentCount": 2}}
	  ],
	  "metadata": {"nextCursor": "abc123", "nextPage": "https://x/y?cursor=abc123"}
	}`
	srv, gotPath, gotQuery, gotAuth := newTestServer(t, body)

	c := New(srv.URL, "tok-1")
	q := url.Values{}
	q.Set("query", "pony")
	q.Set("limit", "5")
	q.Set("types", "Checkpoint")
	res, err := c.SearchModels(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchModels: %v", err)
	}
	if *gotPath != "/api/v1/models" {
		t.Errorf("path = %q, want /api/v1/models", *gotPath)
	}
	if gotQuery.Get("query") != "pony" || gotQuery.Get("limit") != "5" || gotQuery.Get("types") != "Checkpoint" {
		t.Errorf("query params not passed through: %v", *gotQuery)
	}
	if *gotAuth != "Bearer tok-1" {
		t.Errorf("auth = %q, want Bearer tok-1", *gotAuth)
	}
	if len(res.Items) != 1 || res.Items[0].ID != 1 || res.Items[0].Creator.Username != "bob" {
		t.Errorf("parsed items wrong: %+v", res.Items)
	}
	if res.Items[0].Stats.DownloadCount != 10 {
		t.Errorf("stats not parsed: %+v", res.Items[0].Stats)
	}
	if got := res.Metadata.CursorString(); got != "abc123" {
		t.Errorf("CursorString = %q, want abc123", got)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw body should be preserved for --json")
	}
}

func TestSearchModelsAnonymousSendsNoAuthHeader(t *testing.T) {
	srv, _, _, gotAuth := newTestServer(t, `{"items":[],"metadata":{}}`)
	// Empty token => anonymous.
	c := New(srv.URL, "")
	if _, err := c.SearchModels(context.Background(), url.Values{}); err != nil {
		t.Fatalf("SearchModels: %v", err)
	}
	if *gotAuth != "" {
		t.Errorf("anonymous request should send no Authorization header, got %q", *gotAuth)
	}
}

func TestGetModelParsesDetail(t *testing.T) {
	const body = `{"id": 42, "name": "M", "type": "LORA", "nsfw": true,
	  "creator": {"username": "alice"}, "tags": ["anime","style"],
	  "stats": {"downloadCount": 5, "thumbsUpCount": 1, "commentCount": 0},
	  "modelVersions": [{"id": 100, "name": "v1", "baseModel": "SDXL 1.0"}]}`
	srv, gotPath, _, _ := newTestServer(t, body)

	c := New(srv.URL, "")
	m, raw, err := c.GetModel(context.Background(), "42")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if *gotPath != "/api/v1/models/42" {
		t.Errorf("path = %q, want /api/v1/models/42", *gotPath)
	}
	if m.ID != 42 || len(m.ModelVersions) != 1 || m.ModelVersions[0].BaseModel != "SDXL 1.0" {
		t.Errorf("parsed detail wrong: %+v", m)
	}
	if len(m.Tags) != 2 {
		t.Errorf("tags not parsed: %v", m.Tags)
	}
	if len(raw) == 0 {
		t.Error("raw body should be returned")
	}
}

func TestGetModelVersionByHashUppercasePath(t *testing.T) {
	srv, gotPath, _, _ := newTestServer(t, `{"id": 7, "modelId": 3, "name": "v", "baseModel": "SD 1.5"}`)
	c := New(srv.URL, "")
	v, _, err := c.GetModelVersionByHash(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetModelVersionByHash: %v", err)
	}
	if *gotPath != "/api/v1/model-versions/by-hash/abc123" {
		t.Errorf("path = %q", *gotPath)
	}
	if v.ID != 7 || v.ModelID != 3 {
		t.Errorf("parsed version wrong: %+v", v)
	}
}

func TestSearchImagesPassesFiltersAndParses(t *testing.T) {
	const body = `{"items":[{"id":9,"url":"https://img/1.png","width":512,"height":768,
	  "nsfwLevel":"None","type":"image","postId":55,"username":"u",
	  "stats":{"likeCount":2,"heartCount":4,"commentCount":1}}],
	  "metadata":{"nextCursor":123}}`
	srv, gotPath, gotQuery, _ := newTestServer(t, body)
	c := New(srv.URL, "")
	q := url.Values{}
	q.Set("modelId", "4384")
	q.Set("limit", "1")
	res, err := c.SearchImages(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchImages: %v", err)
	}
	if *gotPath != "/api/v1/images" {
		t.Errorf("path = %q", *gotPath)
	}
	if gotQuery.Get("modelId") != "4384" {
		t.Errorf("modelId not passed: %v", *gotQuery)
	}
	if len(res.Items) != 1 || res.Items[0].Username != "u" || res.Items[0].Stats.HeartCount != 4 {
		t.Errorf("parsed image wrong: %+v", res.Items)
	}
	// nextCursor came in as a NUMBER — CursorString must still render it.
	if got := res.Metadata.CursorString(); got != "123" {
		t.Errorf("numeric cursor CursorString = %q, want 123", got)
	}
}

func TestSearchTagsAndCreators(t *testing.T) {
	srvT, pathT, _, _ := newTestServer(t, `{"items":[{"name":"anime","link":"https://x/models?tag=anime"}],"metadata":{"totalItems":1}}`)
	c := New(srvT.URL, "")
	tags, err := c.SearchTags(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchTags: %v", err)
	}
	if *pathT != "/api/v1/tags" || len(tags.Items) != 1 || tags.Items[0].Name != "anime" {
		t.Errorf("tags wrong: path=%q items=%+v", *pathT, tags.Items)
	}
	if tags.Metadata.TotalItems == nil || *tags.Metadata.TotalItems != 1 {
		t.Errorf("tags metadata.totalItems not parsed: %+v", tags.Metadata)
	}

	srvC, pathC, _, _ := newTestServer(t, `{"items":[{"username":"artist","modelCount":12,"link":"l"}],"metadata":{}}`)
	c2 := New(srvC.URL, "")
	cr, err := c2.SearchCreators(context.Background(), url.Values{})
	if err != nil {
		t.Fatalf("SearchCreators: %v", err)
	}
	if *pathC != "/api/v1/creators" || len(cr.Items) != 1 || cr.Items[0].ModelCount != 12 {
		t.Errorf("creators wrong: path=%q items=%+v", *pathC, cr.Items)
	}
}

func TestSearchUsersPath(t *testing.T) {
	srv, gotPath, gotQuery, _ := newTestServer(t, `{"items":[{"id":5,"username":"bob","image":"i"}]}`)
	c := New(srv.URL, "")
	q := url.Values{}
	q.Set("query", "bob")
	res, err := c.SearchUsers(context.Background(), q)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if *gotPath != "/api/v1/users" || gotQuery.Get("query") != "bob" {
		t.Errorf("users request wrong: path=%q q=%v", *gotPath, *gotQuery)
	}
	if len(res.Items) != 1 || res.Items[0].ID != 5 {
		t.Errorf("users parse wrong: %+v", res.Items)
	}
}

func TestReadErrorSurfacesBody(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		// Terminal statuses map through readError.
		{http.StatusNotFound, `{"error":"No model with id 999"}`, "not found (404): No model with id 999"},
		{http.StatusUnauthorized, `{"error":"Unauthorized"}`, "unauthorized (401)"},
		// A 400 is surfaced as a concise "invalid request parameter" message — the
		// raw body is never dumped. A plain {"message":...} body carries through.
		{http.StatusBadRequest, `{"message":"bad param"}`, "invalid request parameter (400): bad param"},
		// A zod-validation 400 (the shape the API actually returns for a bad enum)
		// is parsed down to the offending field + its message, NOT the raw blob.
		{http.StatusBadRequest, `{"error":{"name":"ZodError","message":"[{\"code\":\"invalid_value\",\"path\":[\"period\"],\"message\":\"Invalid option: expected one of \\\"Day\\\"|\\\"Week\\\"\"}]"}}`, `invalid request parameter (400): period — Invalid option: expected one of "Day"|"Week"`},
		// A 429 without Retry-After is terminal (deep-paging limit): readError's
		// 429 branch surfaces the --cursor guidance, no retry-exhaustion message.
		{http.StatusTooManyRequests, `{"error":"too many pages"}`, "rate limited (429): too many pages — for deep paging use --cursor instead of --page"},
		// A persistently-503 status is retried and surfaces the exhaustion error
		// naming the status + attempt count.
		{http.StatusServiceUnavailable, `{"error":"overloaded"}`, "HTTP 503 after 4 attempts"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		c := New(srv.URL, "")
		c.RetryBackoffBase = zeroBackoff() // instant retries for the retriable rows
		c.Stderr = &bytes.Buffer{}         // swallow the retry notices
		_, _, err := c.GetModel(context.Background(), "999")
		srv.Close()
		if err == nil {
			t.Errorf("status %d: expected error", tc.status)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: error %q should contain %q", tc.status, err, tc.want)
		}
	}
}

// TestReadError400Classification asserts a 400 response is tagged with
// ErrBadRequest (the usage-class sentinel) and that a ZodError body is rendered
// concisely — naming the offending field — with no raw blob leaking through.
func TestReadError400Classification(t *testing.T) {
	const zodBody = `{"error":{"name":"ZodError","message":"[{\"code\":\"invalid_value\",\"path\":[\"period\"],\"message\":\"Invalid option: expected one of \\\"Day\\\"|\\\"Week\\\"\"}]"}}`
	err := readError(http.StatusBadRequest, []byte(zodBody))
	if err == nil {
		t.Fatal("expected an error for a 400")
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("400 should be tagged ErrBadRequest, got: %v", err)
	}
	if strings.Contains(err.Error(), "ZodError") {
		t.Errorf("raw ZodError blob must not leak: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid request parameter (400): period — ") {
		t.Errorf("concise 400 message should name the field, got: %q", err.Error())
	}

	// A 400 with no parseable detail falls back to the clean generic message.
	if got := readError(http.StatusBadRequest, []byte(`not json`)).Error(); got != "invalid request parameter (400)" {
		t.Errorf("unparseable 400 = %q, want %q", got, "invalid request parameter (400)")
	}
}

// TestReadError400FlattenedZod asserts the FLATTENED zod 400 shape (formErrors/
// fieldErrors, what the apps-store endpoint returns for a bad enum) is surfaced
// as `field — message`, not the bare generic string — so the user learns WHICH
// field was wrong and WHY, staying in sync with the backend enum.
func TestReadError400FlattenedZod(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"fieldErrors",
			`{"error":{"formErrors":[],"fieldErrors":{"sort":["Invalid enum value. Expected 'top-rated' | 'popular', received 'best'"]}}}`,
			`invalid request parameter (400): sort — Invalid enum value. Expected 'top-rated' | 'popular', received 'best'`,
		},
		{
			// formErrors-only (no field path) falls back to the top-level message.
			"formErrors",
			`{"error":{"formErrors":["Unrecognized key(s) in object: 'foo'"],"fieldErrors":{}}}`,
			`invalid request parameter (400): Unrecognized key(s) in object: 'foo'`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := readError(http.StatusBadRequest, []byte(tc.body))
			if err == nil {
				t.Fatal("expected an error for a 400")
			}
			if !errors.Is(err, ErrBadRequest) {
				t.Errorf("400 should be tagged ErrBadRequest, got: %v", err)
			}
			if err.Error() != tc.want {
				t.Errorf("flattened-zod 400 = %q, want %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "fieldErrors") || strings.Contains(err.Error(), "formErrors") {
				t.Errorf("raw flatten keys must not leak: %q", err.Error())
			}
		})
	}
}

func TestCursorStringEmpty(t *testing.T) {
	var m Metadata
	if got := m.CursorString(); got != "" {
		t.Errorf("empty metadata CursorString = %q, want empty", got)
	}
}
