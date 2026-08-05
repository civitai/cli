package genapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

const versionBody = `{
  "id": 250712,
  "modelId": 4384,
  "name": "v8",
  "baseModel": "SD 1.5",
  "model": {"name": "DreamShaper", "type": "LORA", "nsfw": false},
  "files": []
}`

func versionClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key")
}

// TestResolveModelVersion_Found: the resolver returns the model TYPE (which
// Graph.Resources entries require and which is not derivable from the id) plus
// a display name, so a confirmation prompt shows a name rather than an integer.
func TestResolveModelVersion_Found(t *testing.T) {
	var path string
	c := versionClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, versionBody)
	}))

	v, err := c.ResolveModelVersion(context.Background(), 250712)
	if err != nil {
		t.Fatalf("ResolveModelVersion: %v", err)
	}
	if path != "/api/v1/model-versions/250712" {
		t.Errorf("path = %q, want the existing public model-versions route", path)
	}
	if v.ModelType != "LORA" {
		t.Errorf("ModelType = %q, want LORA", v.ModelType)
	}
	if v.ModelName != "DreamShaper" || v.VersionName != "v8" || v.BaseModel != "SD 1.5" {
		t.Errorf("resolved = %+v", v)
	}
	if got := v.DisplayName(); got != "DreamShaper — v8" {
		t.Errorf("DisplayName = %q", got)
	}

	// The resource it builds must carry model:{type} — a bare id, and {id}
	// alone, are both rejected by the server with a 400.
	r := v.Resource(Ptr(0.8))
	if r.Model == nil || r.Model.Type != "LORA" {
		t.Fatalf("Resource() dropped the model type: %+v", r)
	}
	m := marshalToMap(t, r)
	inner, ok := m["model"].(map[string]any)
	if !ok || inner["type"] != "LORA" {
		t.Fatalf("marshalled resource has no model.type: %v", m)
	}
	if m["id"] != float64(250712) {
		t.Errorf("resource id = %v, want 250712", m["id"])
	}
	if m["strength"] != 0.8 {
		t.Errorf("resource strength = %v, want 0.8", m["strength"])
	}

	// An unset strength must be ABSENT, not 0 — the same zero-value rule the
	// graph fields follow.
	if _, ok := marshalToMap(t, v.Resource(nil))["strength"]; ok {
		t.Error("unset strength is present on the wire")
	}
}

// TestResolveModelVersion_NotFound is the whole point of resolving at all: it
// converts a nonexistent id from a silent substituted charge (HTTP 200, the
// ecosystem default billed instead) into a hard local 404.
func TestResolveModelVersion_NotFound(t *testing.T) {
	c := versionClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"Model version not found"}`)
	}))
	v, err := c.ResolveModelVersion(context.Background(), 999999999)
	if err == nil {
		t.Fatalf("a nonexistent version resolved cleanly: %+v", v)
	}
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
	if v != nil {
		t.Errorf("returned a version alongside the error: %+v", v)
	}
}

// TestResolveModelVersion_Malformed: a body with no model type cannot build a
// resource, and guessing one would be SILENTLY ACCEPTED by the server (a LoRA
// sent as {type:"Checkpoint"} returns 200), so it must fail loudly.
func TestResolveModelVersion_Malformed(t *testing.T) {
	cases := map[string]string{
		"no model object": `{"id":1,"name":"v1"}`,
		"empty type":      `{"id":1,"name":"v1","model":{"name":"x","type":""}}`,
		"model is null":   `{"id":1,"name":"v1","model":null}`,
		"not json":        `<html>oops</html>`,
		"wrong type":      `{"id":1,"model":{"type":123}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c := versionClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			v, err := c.ResolveModelVersion(context.Background(), 1)
			if err == nil {
				t.Fatalf("malformed body %q resolved to %+v", body, v)
			}
			if v != nil {
				t.Errorf("returned a version alongside the error: %+v", v)
			}
		})
	}
}

// TestResolveModelVersion_RejectsNonPositiveID refuses locally rather than
// building a /model-versions/0 request.
func TestResolveModelVersion_RejectsNonPositiveID(t *testing.T) {
	c := versionClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a request was sent for a non-positive id: %s", r.URL.Path)
	}))
	for _, id := range []int{0, -1} {
		_, err := c.ResolveModelVersion(context.Background(), id)
		if !errors.Is(err, civitai.ErrBadRequest) {
			t.Errorf("id %d: err = %v, want ErrBadRequest", id, err)
		}
	}
}

// TestResolveModelVersion_UsesTheInjectedGetter pins the seam: a caller (or a
// test) can substitute the version lookup without a live server.
func TestResolveModelVersion_UsesTheInjectedGetter(t *testing.T) {
	c := &Client{BaseURL: "http://never.invalid", Tokens: civitai.StaticToken("k"), Versions: fakeGetter{}}
	v, err := c.ResolveModelVersion(context.Background(), 7)
	if err != nil {
		t.Fatalf("ResolveModelVersion: %v", err)
	}
	if v.ModelType != "Checkpoint" || v.DisplayName() != "Fake — vX" {
		t.Errorf("resolved = %+v", v)
	}
}

type fakeGetter struct{}

func (fakeGetter) GetModelVersion(_ context.Context, id string) (*civitai.ModelVersionDetail, []byte, error) {
	if id != "7" {
		return nil, nil, errors.New("unexpected id " + id)
	}
	return &civitai.ModelVersionDetail{
		ID: 7, Name: "vX",
		Model: &civitai.ModelVersionModel{Name: "Fake", Type: "Checkpoint"},
	}, nil, nil
}

// TestDisplayName_Fallbacks: never render an empty label.
func TestDisplayName_Fallbacks(t *testing.T) {
	cases := []struct {
		v    ResolvedVersion
		want string
	}{
		{ResolvedVersion{VersionID: 5}, "5"},
		{ResolvedVersion{VersionID: 5, ModelName: "M"}, "M"},
		{ResolvedVersion{VersionID: 5, VersionName: "v1"}, "v1"},
		{ResolvedVersion{VersionID: 5, ModelName: "M", VersionName: "v1"}, "M — v1"},
	}
	for _, tc := range cases {
		if got := tc.v.DisplayName(); got != tc.want {
			t.Errorf("DisplayName(%+v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

// TestServerMessage covers the three body shapes the CLI sees from this
// surface: a tRPC error envelope, a plain REST error, and raw bytes.
func TestServerMessage(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`{"error":{"json":{"message":"Insufficient funds"}}}`, "Insufficient funds"},
		{`{"message":"nope"}`, "nope"},
		{`{"error":"Model version not found"}`, "Model version not found"},
		{`  plain text  `, "plain text"},
	}
	for _, tc := range cases {
		if got := serverMessage([]byte(tc.raw)); got != tc.want {
			t.Errorf("serverMessage(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestGenerateError_KeepsTheServerMessage: the message text is what tells a
// user WHICH 403 they hit, so it must survive classification unaltered.
func TestGenerateError_KeepsTheServerMessage(t *testing.T) {
	err := generateError("generateFromGraph", http.StatusForbidden,
		[]byte(`{"error":{"json":{"message":"Insufficient funds"}}}`))
	if !strings.Contains(err.Error(), "Insufficient funds") {
		t.Errorf("error dropped the server message: %v", err)
	}
	if !errors.Is(err, civitai.ErrUnauthorized) {
		t.Errorf("403 lost its classification: %v", err)
	}
}
