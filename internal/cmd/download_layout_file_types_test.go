package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression coverage for civitai/cli#967 and #968: --layout routed a file by
// its parent model type only when the FILE type was "Model" (or empty), so the
// Civitai file types "Pruned Model", "Negative", "Text Encoder", "Diffusion
// Model" and a checkpoint's "Config" all fell back to --root.
//
// Expected folders are literals from the issues' acceptance criteria and the
// apps' own folder lists — never derived from layoutFolders.

// TestRouteDirRoutesCivitaiFileTypes pins each newly routed (file type, model
// type) pair, under both layouts, with no fallback note.
func TestRouteDirRoutesCivitaiFileTypes(t *testing.T) {
	const root = "/base"
	cases := []struct {
		fileType, modelType string
		a1111, comfyui      string
	}{
		// #967: a pruned checkpoint is the same checkpoint's weights.
		{"Pruned Model", "Checkpoint", "models/Stable-diffusion", "models/checkpoints"},
		// #967: a negative embedding is a textual-inversion embedding.
		{"Negative", "TextualInversion", "embeddings", "models/embeddings"},
		// #968: split-model releases and a checkpoint's YAML.
		{"Text Encoder", "Checkpoint", "models/text_encoder", "models/text_encoders"},
		{"Config", "Checkpoint", "models/Stable-diffusion", "models/configs"},
	}
	for _, tc := range cases {
		for _, lay := range []struct{ name, want string }{{"a1111", tc.a1111}, {"comfyui", tc.comfyui}} {
			dir, note := routeDir(lay.name, root, tc.fileType, tc.modelType, "f.bin")
			if want := filepath.Join(root, filepath.FromSlash(lay.want)); dir != want {
				t.Errorf("routeDir(%s, file=%q model=%q) = %q, want %q", lay.name, tc.fileType, tc.modelType, dir, want)
			}
			if note != "" {
				t.Errorf("routeDir(%s, file=%q model=%q) gave a fallback note %q, want none", lay.name, tc.fileType, tc.modelType, note)
			}
		}
	}

	// ComfyUI loads a standalone diffusion model from models/diffusion_models —
	// NOT models/checkpoints, which is where its parent Checkpoint type points.
	if dir, note := routeDir("comfyui", root, "Diffusion Model", "Checkpoint", "f.bin"); dir != filepath.Join(root, "models", "diffusion_models") || note != "" {
		t.Errorf("comfyui Diffusion Model = (%q, %q), want (%q, \"\")", dir, note, filepath.Join(root, "models", "diffusion_models"))
	}
}

// TestRouteDirKeepsTheFallbackForWhatIsStillUnmapped is an INVARIANT guard (it
// is green before and after the fix): the types #968 leaves unmapped keep the
// --root fallback and its note.
func TestRouteDirKeepsTheFallbackForWhatIsStillUnmapped(t *testing.T) {
	const root = "/base"
	for _, tc := range []struct{ layout, fileType, modelType string }{
		{"a1111", "Diffusion Model", "Checkpoint"}, // non-goal: no A1111 folder
		{"comfyui", "Config", "Controlnet"},        // non-goal: only a checkpoint's config routes
		{"a1111", "Config", "Controlnet"},
		{"comfyui", "Training Data", "Checkpoint"},
		{"comfyui", "Archive", "Checkpoint"},
	} {
		dir, note := routeDir(tc.layout, root, tc.fileType, tc.modelType, "f.bin")
		if dir != root || !strings.Contains(note, "no "+tc.layout+" folder") {
			t.Errorf("routeDir(%s, file=%q model=%q) = (%q, %q), want --root with a fallback note", tc.layout, tc.fileType, tc.modelType, dir, note)
		}
	}
}

// TestDownloadAllLayoutRoutesPrunedCheckpointBesideItsModel drives a real
// transfer shaped like version 4007 from #967: two "Model" and two "Pruned
// Model" files, one pruned file primary. Every file must land in the checkpoint
// folder and nothing in --root.
func TestDownloadAllLayoutRoutesPrunedCheckpointBesideItsModel(t *testing.T) {
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 11, name: "proto.safetensors", typ: "Model", body: "full-st", withSHA: true},
		{id: 12, name: "proto-pruned.safetensors", typ: "Pruned Model", primary: true, body: "pruned-st", withSHA: true},
		{id: 13, name: "proto.ckpt", typ: "Model", body: "full-ck", withSHA: true},
		{id: 14, name: "proto-pruned.ckpt", typ: "Pruned Model", body: "pruned-ck", withSHA: true},
	})
	setupDownloadEnv(t, d, "tok")
	root := t.TempDir()

	_, errOut, err := run(t, "download", "128713", "--all", "--layout", "comfyui", "--root", root)
	if err != nil {
		t.Fatalf("download --all --layout comfyui: %v", err)
	}
	for _, name := range []string{"proto.safetensors", "proto-pruned.safetensors", "proto.ckpt", "proto-pruned.ckpt"} {
		if _, serr := os.Stat(filepath.Join(root, "models", "checkpoints", name)); serr != nil {
			t.Errorf("%s not routed to models/checkpoints: %v", name, serr)
		}
		if _, serr := os.Stat(filepath.Join(root, name)); serr == nil {
			t.Errorf("%s was written to --root", name)
		}
	}
	if strings.Contains(errOut, "has no comfyui folder") {
		t.Errorf("no file should fall back to --root; stderr:\n%s", errOut)
	}
}

// TestDownloadLayoutRoutesANegativeEmbedding drives a real transfer shaped like
// version 60938 from #967: a TextualInversion whose only file is "Negative".
func TestDownloadLayoutRoutesANegativeEmbedding(t *testing.T) {
	d := newLayoutServer(t, "TextualInversion", []dlFile{
		{id: 21, name: "hand-neg.pt", typ: "Negative", primary: true, body: "neg-emb", withSHA: true},
	})
	setupDownloadEnv(t, d, "tok")
	root := t.TempDir()

	_, errOut, err := run(t, "download", "128713", "--layout", "a1111", "--root", root)
	if err != nil {
		t.Fatalf("download --layout a1111: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(root, "embeddings", "hand-neg.pt"))
	if rerr != nil {
		t.Fatalf("negative embedding not routed to embeddings/: %v", rerr)
	}
	if string(got) != "neg-emb" {
		t.Errorf("content = %q, want %q", got, "neg-emb")
	}
	if strings.Contains(errOut, "has no a1111 folder") {
		t.Errorf("unexpected fallback note; stderr:\n%s", errOut)
	}
}

// TestDownloadLayoutDryRunPlansASplitModelRelease plans a release shaped like
// version 3049541 and a checkpoint+YAML shaped like 13259 from #968.
func TestDownloadLayoutDryRunPlansASplitModelRelease(t *testing.T) {
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 31, name: "split_vae.safetensors", typ: "VAE", body: "v"},
		{id: 32, name: "split_te.safetensors", typ: "Text Encoder", body: "t"},
		{id: 33, name: "split_dm.safetensors", typ: "Diffusion Model", primary: true, body: "d"},
		{id: 34, name: "split.yaml", typ: "Config", body: "c"},
	})
	setupDownloadEnv(t, d, "tok")

	out, _, err := run(t, "download", "128713", "--all", "--layout", "comfyui", "--root", "Root", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --layout comfyui: %v", err)
	}
	for _, want := range []string{
		"target: " + filepath.Join("Root", "models", "vae", "split_vae.safetensors"),
		"target: " + filepath.Join("Root", "models", "text_encoders", "split_te.safetensors"),
		"target: " + filepath.Join("Root", "models", "diffusion_models", "split_dm.safetensors"),
		"target: " + filepath.Join("Root", "models", "configs", "split.yaml"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "has no comfyui folder") {
		t.Errorf("no file should fall back to --root:\n%s", out)
	}

	out, _, err = run(t, "download", "128713", "--all", "--layout", "a1111", "--root", "Root", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --layout a1111: %v", err)
	}
	for _, want := range []string{
		"target: " + filepath.Join("Root", "models", "text_encoder", "split_te.safetensors"),
		"target: " + filepath.Join("Root", "models", "Stable-diffusion", "split.yaml"),
		// Non-goal of #968: no A1111 diffusion-model folder, so it keeps --root.
		"target: " + filepath.Join("Root", "split_dm.safetensors"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan missing %q:\n%s", want, out)
		}
	}
}

// TestMixedTypeWarningIgnoresACheckpointsOwnFiles: with --all and no --layout,
// a checkpoint's pruned copy and its YAML belong in the SAME directory as its
// weights (A1111 reads <checkpoint>.yaml from beside it), so neither may be
// named as mis-filed. A bundled VAE still is — the control.
func TestMixedTypeWarningIgnoresACheckpointsOwnFiles(t *testing.T) {
	files := []fileTypeInfo{
		{name: "m.safetensors", fileType: "Model", modelType: "Checkpoint", primary: true},
		{name: "m-pruned.safetensors", fileType: "Pruned Model", modelType: "Checkpoint"},
		{name: "m.yaml", fileType: "Config", modelType: "Checkpoint"},
	}
	if w := mixedTypeWarning(files); w != "" {
		t.Errorf("a checkpoint with its pruned copy and config should not warn, got %q", w)
	}
	files = append(files, fileTypeInfo{name: "m.vae.pt", fileType: "VAE", modelType: "Checkpoint"})
	w := mixedTypeWarning(files)
	if !strings.Contains(w, "m.vae.pt") {
		t.Errorf("control: the bundled VAE should still be named, got %q", w)
	}
	for _, own := range []string{"m-pruned.safetensors", "m.yaml"} {
		if strings.Contains(w, own) {
			t.Errorf("%s is the checkpoint's own file and must not be named as mis-filed: %q", own, w)
		}
	}
}
