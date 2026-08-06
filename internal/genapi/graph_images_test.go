package genapi

import (
	"encoding/json"
	"testing"
)

// 🔴 Key absence, on a DECODED map — never a strings.Contains search (item 14).
//
// This is the images-carrying sibling of TestGraph_UnsetFieldsAreAbsent: adding
// a value-typed field to Graph must not start emitting keys the caller never
// set, and `images` in particular must be ABSENT rather than `[]` when no
// reference image was passed. An empty array is NOT equivalent: for a workflow
// the server has promoted to img2img:edit, `images: []` fails the node's
// `.min(1)` with "An image is required", while an absent key leaves the job a
// plain txt2img.
func TestGraph_ImagesAbsentWhenUnset(t *testing.T) {
	g := Graph{Workflow: "txt2img", Ecosystem: "Qwen", Prompt: "a cat"}
	m := marshalToMap(t, g)

	if _, ok := m["images"]; ok {
		t.Errorf("images = %v, want the key ABSENT when Graph.Images is nil", m["images"])
	}
	for _, k := range []string{"steps", "cfgScale", "sampler", "seed", "quantity", "model", "resources"} {
		if _, ok := m[k]; ok {
			t.Errorf("key %q present but never set", k)
		}
	}
	if len(m) != 3 {
		t.Errorf("graph marshalled %d keys (%v), want exactly workflow/ecosystem/prompt", len(m), m)
	}
}

// The wire shape of one entry: exactly url/width/height, with the dimensions as
// JSON NUMBERS. The server's images node validates them against
// `z.object({url: z.string(), width: z.number(), height: z.number()})`, so a
// string-encoded dimension is a 400 —
// "Invalid input: expected number, received undefined" is what a MISSING one
// produces, measured against civitai.com.
func TestGraph_ImageEntryShape(t *testing.T) {
	g := Graph{
		Workflow:  "txt2img",
		Ecosystem: "Qwen",
		Images: []GraphImage{
			{URL: "https://orchestration.civitai.com/v2/consumer/blobs/A.png", Width: 1024, Height: 768},
			{URL: "https://example.com/b.jpg", Width: 640, Height: 360},
		},
	}
	m := marshalToMap(t, g)

	// 🔴 The workflow stays txt2img even with images: the server promotes it.
	if m["workflow"] != "txt2img" {
		t.Errorf("workflow = %v, want txt2img", m["workflow"])
	}

	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 2 {
		t.Fatalf("images = %#v, want two entries", m["images"])
	}
	wantW := []float64{1024, 640}
	wantH := []float64{768, 360}
	wantURL := []string{
		"https://orchestration.civitai.com/v2/consumer/blobs/A.png",
		"https://example.com/b.jpg",
	}
	for i, raw := range imgs {
		e, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("images[%d] = %#v, want an object", i, raw)
		}
		if len(e) != 3 {
			t.Errorf("images[%d] has %d keys (%v), want exactly url/width/height", i, len(e), e)
		}
		if e["url"] != wantURL[i] {
			t.Errorf("images[%d].url = %v, want %v", i, e["url"], wantURL[i])
		}
		// Distinct, non-square, pairwise-different values so a transposition or
		// a cross-entry copy cannot pass by coincidence.
		if e["width"] != wantW[i] {
			t.Errorf("images[%d].width = %v, want %v", i, e["width"], wantW[i])
		}
		if e["height"] != wantH[i] {
			t.Errorf("images[%d].height = %v, want %v", i, e["height"], wantH[i])
		}
	}
}

// 🔴 Zero dimensions must still be EMITTED, not dropped.
//
// If GraphImage's width/height ever grow `omitempty`, a 0 disappears and the
// server reports the misleading "expected number, received undefined" instead of
// rejecting the actual value. This pins the absence of that tag.
func TestGraph_ImageZeroDimensionsAreStillEmitted(t *testing.T) {
	g := Graph{Images: []GraphImage{{URL: "https://x/y.png", Width: 0, Height: 0}}}
	m := marshalToMap(t, g)

	e := m["images"].([]any)[0].(map[string]any)
	if _, ok := e["width"]; !ok {
		t.Error("width was dropped for a zero value — GraphImage.Width must not carry omitempty")
	}
	if _, ok := e["height"]; !ok {
		t.Error("height was dropped for a zero value — GraphImage.Height must not carry omitempty")
	}
}

// A raw --input graph carrying its own images survives byte-for-byte through the
// whatIf strip, which must only remove the two prompt keys.
func TestWhatIfGraph_RawImagesSurvive(t *testing.T) {
	raw := json.RawMessage(`{"workflow":"txt2img","ecosystem":"Qwen","prompt":"p","negativePrompt":"n","images":[{"url":"https://x/a.png","width":10,"height":20}]}`)
	out := whatIfGraph(Graph{Raw: raw})
	m := marshalToMap(t, out)

	if _, ok := m["prompt"]; ok {
		t.Error("prompt should be stripped from a whatIf")
	}
	if _, ok := m["negativePrompt"]; ok {
		t.Error("negativePrompt should be stripped from a whatIf")
	}
	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 1 {
		t.Fatalf("images = %#v — the strip must not touch any other key", m["images"])
	}
	e := imgs[0].(map[string]any)
	if e["url"] != "https://x/a.png" || e["width"] != float64(10) || e["height"] != float64(20) {
		t.Errorf("images[0] = %v, want the raw entry unchanged", e)
	}
}

// KnownGraphKeys is derived by reflection, so adding Images must have added its
// key automatically. A hand-maintained list would have drifted here.
func TestKnownGraphKeys_IncludesImages(t *testing.T) {
	keys := KnownGraphKeys()
	for _, want := range []string{"images", "ecosystem", "workflow"} {
		if !keys[want] {
			t.Errorf("KnownGraphKeys() lacks %q — a --input graph carrying it would warn spuriously", want)
		}
	}
	if keys["nonexistentKey"] {
		t.Error("positive control FAILED: KnownGraphKeys() reports a key that does not exist, so the assertions above prove nothing")
	}
}
