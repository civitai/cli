package cmd

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// A meta payload with 7 resources (a checkpoint + 6 LoRAs) plus a name→hash map
// and an inline-hashless resource to exercise the meta.hashes fallback.
const sevenResourceBody = `{"items":[
  {"id":136456589,"url":"https://img/1","width":512,"height":768,"nsfwLevel":"None","username":"alice",
   "meta":{"prompt":"a cat","Model":"krea2","seed":557589798350441,
     "resources":[
       {"hash":"78bbf8f416","name":"krea2_turbo_bf16","type":"model"},
       {"name":"V0idv1_k2","type":"lora","weight":0.8},
       {"hash":"605e3c6825","name":"krmj_epoch_4","type":"lora","weight":1.0},
       {"hash":"5ec8728156","name":"Detailer-KREA2","type":"lora","weight":0.5},
       {"hash":"3e8ed14ad2","name":"Comic Book V2T2","type":"lora","weight":0.6},
       {"hash":"9fec63fcf3","name":"Digital_Painting","type":"lora","weight":0.7},
       {"hash":"43c97c9ebf","name":"masterpieces_v51","type":"lora","weight":0.9}
     ],
     "hashes":{"model":"78bbf8f416","LORA:V0idv1_k2":"d2b95713c9"}}}
],"metadata":{}}`

// TestImagesSearchMetaShowsResources asserts the --meta detail block now carries
// a resources section listing every resource (all 7), with type/name/weight, and
// resolves a hashless resource's hash from meta.hashes ("lora:V0idv1_k2").
func TestImagesSearchMetaShowsResources(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sevenResourceBody))
	})
	out, _, err := run(t, "images", "search", "--meta")
	if err != nil {
		t.Fatalf("images search --meta: %v", err)
	}
	if !strings.Contains(out, "resources:") {
		t.Fatalf("--meta output should have a resources section:\n%s", out)
	}
	// All 7 resource names must appear.
	for _, name := range []string{
		"krea2_turbo_bf16", "V0idv1_k2", "krmj_epoch_4", "Detailer-KREA2",
		"Comic Book V2T2", "Digital_Painting", "masterpieces_v51",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("resources section missing %q:\n%s", name, out)
		}
	}
	// Type + weight are rendered.
	if !strings.Contains(out, "[model]") || !strings.Contains(out, "[lora]") {
		t.Errorf("resource type not rendered:\n%s", out)
	}
	if !strings.Contains(out, "weight 0.8") {
		t.Errorf("resource weight not rendered:\n%s", out)
	}
	// Inline hash of the checkpoint is shown.
	if !strings.Contains(out, "hash 78bbf8f416") {
		t.Errorf("inline resource hash not rendered:\n%s", out)
	}
	// The hashless V0idv1_k2 LoRA resolves its hash from meta.hashes (LORA:V0idv1_k2).
	if !strings.Contains(out, "hash d2b95713c9") {
		t.Errorf("hashless resource should resolve hash from meta.hashes:\n%s", out)
	}
}

// TestImagesSearchMetaResourcesOmittedWhenEmpty asserts the resources section is
// omitted entirely for an image whose meta has no resources (empty array / null),
// so images without a recipe stay compact.
func TestImagesSearchMetaResourcesOmittedWhenEmpty(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
		  {"id":1,"url":"https://img/1","width":1,"height":1,"nsfwLevel":"None","username":"a",
		   "meta":{"prompt":"hi","resources":[],"hashes":null}},
		  {"id":2,"url":"https://img/2","width":1,"height":1,"nsfwLevel":"None","username":"b",
		   "meta":{"prompt":"yo"}}
		],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search", "--meta")
	if err != nil {
		t.Fatalf("images search --meta: %v", err)
	}
	if strings.Contains(out, "resources:") {
		t.Errorf("empty/absent resources should omit the section:\n%s", out)
	}
	// The rest of the block still renders.
	if !strings.Contains(out, "prompt: hi") || !strings.Contains(out, "prompt: yo") {
		t.Errorf("prompts should still render:\n%s", out)
	}
}

// TestImagesSearchMetaResourceSanitized asserts a resource name carrying a
// terminal escape is stripped by safeTerm before printing.
func TestImagesSearchMetaResourceSanitized(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
		  {"id":1,"url":"https://img/1","width":1,"height":1,"nsfwLevel":"None","username":"a",
		   "meta":{"prompt":"p","resources":[{"name":"evil\u001b[31mlora","type":"lora","hash":"h"}]}}
		],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "search", "--meta")
	if err != nil {
		t.Fatalf("images search --meta: %v", err)
	}
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("resource name ESC byte must be stripped by safeTerm: %q", out)
	}
	if !strings.Contains(out, "evil") || !strings.Contains(out, "[31mlora") {
		t.Errorf("printable resource name should survive: %q", out)
	}
}

// TestImagesSearchMetaJSONUnchangedByResources asserts --json stays a raw
// passthrough — the resources rendering must not touch machine output.
func TestImagesSearchMetaJSONUnchangedByResources(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sevenResourceBody))
	})
	out, _, err := run(t, "images", "search", "--meta", "--json")
	if err != nil {
		t.Fatalf("images search --meta --json: %v", err)
	}
	if strings.Contains(out, "resources:") {
		t.Errorf("--json must not carry the human resources section:\n%s", out)
	}
	if !strings.Contains(out, `"resources"`) {
		t.Errorf("--json should pass the raw resources through:\n%s", out)
	}
}

// TestImagesGetRegistered asserts the get subcommand shows up in help.
func TestImagesGetRegistered(t *testing.T) {
	out, _, err := run(t, "images", "--help")
	if err != nil {
		t.Fatalf("images --help: %v", err)
	}
	if !strings.Contains(out, "get") {
		t.Errorf("images help should list the get subcommand:\n%s", out)
	}
}

// TestImagesGetRejectsNonNumericId mirrors the other get subcommands' guard.
func TestImagesGetRejectsNonNumericId(t *testing.T) {
	if _, _, err := run(t, "images", "get", "abc"); err == nil {
		t.Fatal("expected error for non-numeric image id")
	}
}

// TestImagesGetSendsImageIdAndRenders asserts get hits imageId + flat meta and
// renders the shared detail block including the resources section.
func TestImagesGetSendsImageIdAndRenders(t *testing.T) {
	var got url.Values
	var path string
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		got = r.URL.Query()
		_, _ = w.Write([]byte(sevenResourceBody))
	})
	out, _, err := run(t, "images", "get", "136456589")
	if err != nil {
		t.Fatalf("images get: %v", err)
	}
	if path != "/api/v1/images" {
		t.Errorf("path = %q, want /api/v1/images", path)
	}
	if got.Get("imageId") != "136456589" {
		t.Errorf("imageId not sent: %v", got)
	}
	if got.Get("withMeta") != "true" || got.Get("flatMeta") != "true" {
		t.Errorf("get should implicitly request flat meta: %v", got)
	}
	if !strings.Contains(out, "136456589") || !strings.Contains(out, "prompt: a cat") {
		t.Errorf("detail block not rendered:\n%s", out)
	}
	if !strings.Contains(out, "resources:") || !strings.Contains(out, "krea2_turbo_bf16") {
		t.Errorf("get should render the resources section:\n%s", out)
	}
	// Large seed survives the tolerant numeric parse.
	if !strings.Contains(out, "seed: 557589798350441") {
		t.Errorf("large seed should render:\n%s", out)
	}
}

// TestImagesGetJSONPassthrough asserts get --json emits the raw envelope, not the
// human block.
func TestImagesGetJSONPassthrough(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("imageId") != "42" {
			t.Errorf("imageId not sent on --json: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"items":[{"id":42,"url":"u","meta":{"prompt":"hi","resources":[{"name":"x","type":"lora"}]}}],"metadata":{}}`))
	})
	out, _, err := run(t, "images", "get", "42", "--json")
	if err != nil {
		t.Fatalf("images get --json: %v", err)
	}
	if strings.Contains(out, "prompt:") || strings.Contains(out, "resources:") {
		t.Errorf("--json must not carry the human detail block:\n%s", out)
	}
	if !strings.Contains(out, `"items"`) || !strings.Contains(out, `"resources"`) {
		t.Errorf("--json should pass the raw body through:\n%s", out)
	}
}

// TestImagesGetSurfacesNotFound asserts an empty items array surfaces a
// not-found error rather than printing a bogus block.
func TestImagesGetSurfacesNotFound(t *testing.T) {
	setupReadServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[],"metadata":{}}`))
	})
	_, _, err := run(t, "images", "get", "999999999")
	if err == nil {
		t.Fatal("expected not-found error for empty items")
	}
	if !strings.Contains(err.Error(), "no image found with id 999999999") {
		t.Errorf("error should name the id: %v", err)
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("images get not-found must be tagged ErrNotFound (→ exit 4), got %v", err)
	}
}

// TestImagesHelpExampleValid guards Fix 4: the images help must not advertise the
// --model-id + --sort combo the tool itself warns is ignored.
func TestImagesHelpExampleValid(t *testing.T) {
	out, _, err := run(t, "images", "--help")
	if err != nil {
		t.Fatalf("images --help: %v", err)
	}
	if strings.Contains(out, `--model-id 4384 --sort`) {
		t.Errorf("help must not demo the model-id + sort combo the tool warns against:\n%s", out)
	}
}
