package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRouteDirMapsEachTypePerLayout asserts every known resource type routes to
// the documented subfolder under --root for both layouts. The routing key is the
// file type for an ancillary (VAE) and the parent MODEL type for the weights
// file ("Model").
func TestRouteDirMapsEachTypePerLayout(t *testing.T) {
	const root = "/base"
	cases := []struct {
		fileType  string
		modelType string
		a1111     string
		comfyui   string
	}{
		{"Model", "Checkpoint", "models/Stable-diffusion", "models/checkpoints"},
		{"Model", "LORA", "models/Lora", "models/loras"},
		{"Model", "LoCon", "models/Lora", "models/loras"},
		{"Model", "DoRA", "models/Lora", "models/loras"},
		{"Model", "TextualInversion", "embeddings", "models/embeddings"},
		{"Model", "Hypernetwork", "models/hypernetworks", "models/hypernetworks"},
		{"Model", "Controlnet", "models/ControlNet", "models/controlnet"},
		{"Model", "Upscaler", "models/ESRGAN", "models/upscale_models"},
		{"Model", "VAE", "models/VAE", "models/vae"},      // standalone VAE model
		{"VAE", "Checkpoint", "models/VAE", "models/vae"}, // bundled VAE inside a checkpoint
	}
	for _, tc := range cases {
		for _, lay := range []struct {
			name, want string
		}{{"a1111", tc.a1111}, {"comfyui", tc.comfyui}} {
			dir, note := routeDir(lay.name, root, tc.fileType, tc.modelType, "f.safetensors")
			want := filepath.Join(root, filepath.FromSlash(lay.want))
			if dir != want {
				t.Errorf("routeDir(%s, file=%s model=%s) = %q, want %q", lay.name, tc.fileType, tc.modelType, dir, want)
			}
			if note != "" {
				t.Errorf("known type %s/%s should not produce a fallback note, got %q", tc.fileType, tc.modelType, note)
			}
		}
	}
}

// TestRouteDirUnknownFallsBackToRootWithNote proves an unmapped type routes to
// --root itself and returns an explanatory note rather than silently misplacing.
func TestRouteDirUnknownFallsBackToRootWithNote(t *testing.T) {
	for _, ft := range []string{"Archive", "Training Data", "Config", "Poses"} {
		dir, note := routeDir("comfyui", "/base", ft, "Other", "thing.zip")
		if dir != "/base" {
			t.Errorf("unknown type %q should route to --root, got %q", ft, dir)
		}
		if note == "" || !strings.Contains(note, "no comfyui folder") {
			t.Errorf("unknown type %q should carry a fallback note, got %q", ft, note)
		}
	}
}

// TestRouteDirDefaultRoot proves an empty --root defaults to ".".
func TestRouteDirDefaultRoot(t *testing.T) {
	dir, _ := routeDir("a1111", "", "Model", "Checkpoint", "m.safetensors")
	if dir != filepath.Join(".", "models", "Stable-diffusion") {
		t.Errorf("default root = %q, want ./models/Stable-diffusion", dir)
	}
}

// TestMixedTypeWarningFiresOnDifferingTypes proves the --all (no --layout)
// warning fires for a checkpoint + bundled VAE and names the off-type file.
func TestMixedTypeWarningFiresOnDifferingTypes(t *testing.T) {
	files := []fileTypeInfo{
		{name: "model.safetensors", fileType: "Model", modelType: "Checkpoint"},
		{name: "bundled.vae.pt", fileType: "VAE", modelType: "Checkpoint"},
	}
	w := mixedTypeWarning(files)
	if w == "" {
		t.Fatal("expected a mixed-type warning for checkpoint + VAE")
	}
	if !strings.Contains(w, "bundled.vae.pt") {
		t.Errorf("warning should name the off-type file: %q", w)
	}
	if !strings.Contains(w, "--layout") {
		t.Errorf("warning should suggest --layout: %q", w)
	}
}

// TestMixedTypeWarningNamesOffTypeWhenAncillaryFirst is the Fix 3 assertion: when
// an ancillary file (a bundled VAE) is listed BEFORE the primary weights file,
// the warning must still name the VAE (the off-type minority), never the
// checkpoint — files[0] is not the reference; the primary/weights file is.
func TestMixedTypeWarningNamesOffTypeWhenAncillaryFirst(t *testing.T) {
	files := []fileTypeInfo{
		{name: "bundled.vae.pt", fileType: "VAE", modelType: "Checkpoint", primary: false},
		{name: "dream.safetensors", fileType: "Model", modelType: "Checkpoint", primary: true},
	}
	w := mixedTypeWarning(files)
	if w == "" {
		t.Fatal("expected a mixed-type warning for VAE-first + checkpoint")
	}
	if !strings.Contains(w, "bundled.vae.pt") {
		t.Errorf("warning must name the off-type VAE: %q", w)
	}
	if strings.Contains(w, "dream.safetensors") {
		t.Errorf("warning must NOT name the primary weights file: %q", w)
	}
}

// TestMixedTypeWarningRefFallsBackToMostCommon proves that with no primary flag
// set, the reference category is the MOST COMMON known category (weights usually
// dominate) — so two checkpoints + one leading VAE still name the VAE.
func TestMixedTypeWarningRefFallsBackToMostCommon(t *testing.T) {
	files := []fileTypeInfo{
		{name: "extra.vae.pt", fileType: "VAE", modelType: "Checkpoint"},
		{name: "a.safetensors", fileType: "Model", modelType: "Checkpoint"},
		{name: "b.safetensors", fileType: "Model", modelType: "Checkpoint"},
	}
	w := mixedTypeWarning(files)
	if !strings.Contains(w, "extra.vae.pt") {
		t.Errorf("most-common reference should name the minority VAE: %q", w)
	}
	if strings.Contains(w, "a.safetensors") || strings.Contains(w, "b.safetensors") {
		t.Errorf("majority checkpoints must not be named: %q", w)
	}
}

// TestMixedTypeWarningQuietForSingleType proves a homogeneous set (or one with
// only unknown extras) stays quiet.
func TestMixedTypeWarningQuietForSingleType(t *testing.T) {
	// All weights of the same model type.
	same := []fileTypeInfo{
		{name: "a.safetensors", fileType: "Model", modelType: "Checkpoint"},
		{name: "b.safetensors", fileType: "Model", modelType: "Checkpoint"},
	}
	if w := mixedTypeWarning(same); w != "" {
		t.Errorf("single-type set should not warn, got %q", w)
	}
	// Weights + an unknown-type extra (Training Data) → only one KNOWN category.
	withUnknown := []fileTypeInfo{
		{name: "a.safetensors", fileType: "Model", modelType: "Checkpoint"},
		{name: "train.zip", fileType: "Training Data", modelType: "Checkpoint"},
	}
	if w := mixedTypeWarning(withUnknown); w != "" {
		t.Errorf("weights + unknown extra should not warn (one known category), got %q", w)
	}
	// Unknown model type + VAE → only VAE is a known category → quiet (mirrors
	// the existing --all tests where the version detail carries no model.type).
	unknownModel := []fileTypeInfo{
		{name: "a.safetensors", fileType: "Model", modelType: ""},
		{name: "v.pt", fileType: "VAE", modelType: ""},
	}
	if w := mixedTypeWarning(unknownModel); w != "" {
		t.Errorf("unknown model type should keep the set single-known, got %q", w)
	}
}

// TestFileCategoryRouting documents the category resolution: VAE file type wins
// regardless of model, weights route by model type, other ancillary → unknown.
func TestFileCategoryRouting(t *testing.T) {
	cases := []struct {
		fileType, modelType string
		want                resourceCategory
	}{
		{"VAE", "Checkpoint", catVAE},
		{"Model", "Checkpoint", catCheckpoint},
		{"Model", "LORA", catLora},
		{"", "TextualInversion", catEmbedding},
		{"Archive", "Checkpoint", catUnknown},
		{"Model", "Workflows", catUnknown},
	}
	for _, tc := range cases {
		if got := fileCategory(tc.fileType, tc.modelType); got != tc.want {
			t.Errorf("fileCategory(%q,%q) = %d, want %d", tc.fileType, tc.modelType, got, tc.want)
		}
	}
}
