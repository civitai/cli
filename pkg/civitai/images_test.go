package civitai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseResourcesShapes covers the real meta.resources shapes: a full recipe
// (base model + weighted LoRAs), an empty array, absent, null, and an odd shape
// (must degrade to nil, never panic/error).
func TestParseResourcesShapes(t *testing.T) {
	full := ImageMeta{Resources: []byte(`[
	  {"hash":"78bbf8f416","name":"krea2_turbo_bf16","type":"model"},
	  {"hash":"990d0df641","name":"Purple_Graphics","type":"lora","weight":1.2},
	  {"hash":"ce0672fe6e","name":"Grainy","type":"lora","weight":0.5}
	]`)}
	rs := full.ParseResources()
	if len(rs) != 3 {
		t.Fatalf("want 3 resources, got %d: %+v", len(rs), rs)
	}
	if rs[0].Type != "model" || rs[0].Name != "krea2_turbo_bf16" || rs[0].Hash != "78bbf8f416" {
		t.Errorf("base model resource mis-parsed: %+v", rs[0])
	}
	if rs[0].WeightString() != "" {
		t.Errorf("base model should have no weight, got %q", rs[0].WeightString())
	}
	if rs[1].WeightString() != "1.2" {
		t.Errorf("lora weight = %q, want 1.2", rs[1].WeightString())
	}

	// Absent / null / odd shapes → nil (never error).
	for name, m := range map[string]ImageMeta{
		"absent": {},
		"null":   {Resources: []byte(`null`)},
		"object": {Resources: []byte(`{"not":"an array"}`)},
		"scalar": {Resources: []byte(`"nope"`)},
	} {
		if got := m.ParseResources(); got != nil {
			t.Errorf("%s resources should parse to nil, got %+v", name, got)
		}
	}
	// An empty array is a valid "no resources" — len 0 is what the renderer keys
	// off to omit the section.
	if got := (ImageMeta{Resources: []byte(`[]`)}).ParseResources(); len(got) != 0 {
		t.Errorf("empty resources array should have len 0, got %+v", got)
	}
}

// TestResolveHash covers inline-hash preference and the meta.hashes fallback
// (both bare-name and "<type>:<name>" keys), plus a nil hashes map.
func TestResolveHash(t *testing.T) {
	m := ImageMeta{Hashes: []byte(`{"model":"aaa","lora:MyLora":"bbb","BareName":"ccc"}`)}

	// Inline hash always wins.
	if got := m.ResolveHash(ImageResource{Name: "MyLora", Type: "lora", Hash: "inline"}); got != "inline" {
		t.Errorf("inline hash should win, got %q", got)
	}
	// Fallback via "<type>:<name>" key.
	if got := m.ResolveHash(ImageResource{Name: "MyLora", Type: "lora"}); got != "bbb" {
		t.Errorf("type:name fallback = %q, want bbb", got)
	}
	// Fallback via bare-name key.
	if got := m.ResolveHash(ImageResource{Name: "BareName", Type: "embed"}); got != "ccc" {
		t.Errorf("bare-name fallback = %q, want ccc", got)
	}
	// Nothing resolvable.
	if got := m.ResolveHash(ImageResource{Name: "Unknown", Type: "lora"}); got != "" {
		t.Errorf("unresolvable hash should be empty, got %q", got)
	}
	// Nil hashes map + no inline hash → empty.
	empty := ImageMeta{}
	if got := empty.ResolveHash(ImageResource{Name: "x", Type: "lora"}); got != "" {
		t.Errorf("no hashes → empty, got %q", got)
	}
}

// TestParseHashesNullAndOdd asserts ParseHashes degrades gracefully.
func TestParseHashesNullAndOdd(t *testing.T) {
	for name, m := range map[string]ImageMeta{
		"null":   {Hashes: []byte(`null`)},
		"absent": {},
		"array":  {Hashes: []byte(`[1,2,3]`)},
	} {
		if got := m.ParseHashes(); got != nil {
			t.Errorf("%s hashes should be nil, got %+v", name, got)
		}
	}
	m := ImageMeta{Hashes: []byte(`{"model":"aaa"}`)}
	if got := m.ParseHashes(); len(got) != 1 || got["model"] != "aaa" {
		t.Errorf("valid hashes mis-parsed: %+v", got)
	}
}

// TestParseMetaKeepsResourcesAndHashes asserts that adding resources/hashes to
// ImageMeta doesn't disturb the existing prompt/settings decode, and that an
// odd resources shape still leaves meta MetaOK (raw-JSON-backed tolerance).
func TestParseMetaKeepsResourcesAndHashes(t *testing.T) {
	im := ImageItem{Meta: []byte(`{"prompt":"a cat","Model":"M","seed":123,
	  "resources":"this is not an array","hashes":{"model":"aaa"}}`)}
	m, state := im.ParseMeta()
	if state != MetaOK {
		t.Fatalf("odd resources shape must not fail meta decode, state=%v", state)
	}
	if m.Prompt != "a cat" || m.Model != "M" || m.SeedString() != "123" {
		t.Errorf("prompt/settings decode disturbed: %+v", m)
	}
	// The odd resources value degrades to nil at parse time; hashes still parse.
	if m.ParseResources() != nil {
		t.Error("odd resources should parse to nil")
	}
	if h := m.ParseHashes(); h["model"] != "aaa" {
		t.Errorf("hashes should still parse: %+v", h)
	}
}

// TestGetImageSendsImageIdAndParses asserts GetImage hits /api/v1/images with the
// imageId + meta params, returns the single item, and surfaces the raw body.
func TestGetImageSendsImageIdAndParses(t *testing.T) {
	const body = `{"items":[
	  {"id":136456589,"url":"https://img/1","width":512,"height":768,"nsfwLevel":"None",
	   "username":"alice","baseModel":"Krea 2",
	   "meta":{"prompt":"a cat","seed":557589798350441,
	           "resources":[{"hash":"h1","name":"ckpt","type":"model"}]}}
	],"metadata":{}}`
	srv, gotPath, gotQuery, _ := newTestServer(t, body)

	c := New(srv.URL, "", "")
	im, raw, err := c.GetImage(context.Background(), "136456589")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if *gotPath != "/api/v1/images" {
		t.Errorf("path = %q, want /api/v1/images", *gotPath)
	}
	if gotQuery.Get("imageId") != "136456589" {
		t.Errorf("imageId not sent: %v", *gotQuery)
	}
	if gotQuery.Get("withMeta") != "true" || gotQuery.Get("flatMeta") != "true" {
		t.Errorf("get should request flat meta: %v", *gotQuery)
	}
	if im.ID != 136456589 || im.Username != "alice" {
		t.Errorf("parsed item wrong: %+v", im)
	}
	m, state := im.ParseMeta()
	if state != MetaOK || len(m.ParseResources()) != 1 {
		t.Errorf("meta/resources not parsed: state=%v %+v", state, m)
	}
	if len(raw) == 0 {
		t.Error("raw body should be returned for --json")
	}
}

// TestGetImageNotFound asserts an empty items array yields a not-found error but
// still returns the raw body.
func TestGetImageNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "", "")
	im, raw, err := c.GetImage(context.Background(), "999999999")
	if err == nil {
		t.Fatal("expected not-found error for empty items")
	}
	if !strings.Contains(err.Error(), "no image found with id 999999999") {
		t.Errorf("error should name the id: %v", err)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("not-found error must be tagged ErrNotFound (→ exit 4), got %v", err)
	}
	if im != nil {
		t.Errorf("item should be nil on not-found, got %+v", im)
	}
	if len(raw) == 0 {
		t.Error("raw body should still be returned for --json passthrough")
	}
}
