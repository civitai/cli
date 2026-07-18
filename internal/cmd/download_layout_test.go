package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newLayoutServer wires a fake Civitai API whose version detail carries a parent
// model TYPE (unlike newDLServer) so --layout routing has a model type to route
// the weights file by. Each file's downloadUrl points back at this server.
func newLayoutServer(t *testing.T, modelType string, files []dlFile) *dlServer {
	t.Helper()
	d := &dlServer{}
	var base string

	filesJSON := func() string {
		var parts []string
		for _, f := range files {
			hashes := ""
			if f.withSHA {
				hashes = `,"hashes":{"SHA256":"` + sha256hex(f.body) + `"}`
			}
			parts = append(parts, fmt.Sprintf(
				`{"id":%d,"name":%q,"type":%q,"sizeKB":%f,"primary":%t,"downloadUrl":"%s/dl/%s"%s}`,
				f.id, f.name, f.typ, float64(len(f.body))/1024, f.primary, base, f.name, hashes))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SDXL 1.0","model":{"name":"M","type":%q},"files":%s}`, modelType, filesJSON())
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":4384,"name":"M","type":%q,"modelVersions":[{"id":128713,"name":"v","baseModel":"SDXL 1.0","files":%s}]}`, modelType, filesJSON())
	})
	serve := func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
		for _, f := range files {
			if f.name == name {
				_, _ = w.Write([]byte(f.body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		d.dlHits++
		serve(w, r)
	})

	d.srv = httptest.NewServer(mux)
	base = d.srv.URL
	t.Cleanup(d.srv.Close)
	return d
}

// TestDownloadLayoutRoutesToTypeFolder proves a single checkpoint weights file
// lands under <root>/models/checkpoints (ComfyUI) via a real httptest transfer.
func TestDownloadLayoutRoutesToTypeFolder(t *testing.T) {
	const body = "CKPT-WEIGHTS"
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 1, name: "dream.safetensors", typ: "Model", primary: true, body: body, withSHA: true},
	})
	setupDownloadEnv(t, d, "tok")
	root := t.TempDir()

	out, _, err := run(t, "download", "128713", "--layout", "comfyui", "--root", root)
	if err != nil {
		t.Fatalf("download --layout: %v", err)
	}
	want := filepath.Join(root, "models", "checkpoints", "dream.safetensors")
	got, rerr := os.ReadFile(want)
	if rerr != nil {
		t.Fatalf("file not routed to %s: %v", want, rerr)
	}
	if string(got) != body {
		t.Errorf("content = %q, want %q", got, body)
	}
	if !strings.Contains(out, "base model: SDXL 1.0") {
		t.Errorf("output should show the base model: %s", out)
	}
}

// TestDownloadAllLayoutFansTypesToDifferentFolders is the core footgun fix: a
// checkpoint + a bundled VAE land in DIFFERENT folders under --root.
func TestDownloadAllLayoutFansTypesToDifferentFolders(t *testing.T) {
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 1, name: "dream.safetensors", typ: "Model", primary: true, body: "ckpt", withSHA: true},
		{id: 2, name: "dream.vae.pt", typ: "VAE", body: "vae"},
	})
	setupDownloadEnv(t, d, "tok")
	root := t.TempDir()

	if _, _, err := run(t, "download", "128713", "--all", "--layout", "a1111", "--root", root); err != nil {
		t.Fatalf("download --all --layout: %v", err)
	}
	ckpt := filepath.Join(root, "models", "Stable-diffusion", "dream.safetensors")
	vae := filepath.Join(root, "models", "VAE", "dream.vae.pt")
	if _, err := os.Stat(ckpt); err != nil {
		t.Errorf("checkpoint not in Stable-diffusion: %v", err)
	}
	if _, err := os.Stat(vae); err != nil {
		t.Errorf("bundled VAE not routed to models/VAE: %v", err)
	}
	// The VAE must NOT have landed in the checkpoint folder (the bug we fix).
	if _, err := os.Stat(filepath.Join(root, "models", "Stable-diffusion", "dream.vae.pt")); err == nil {
		t.Error("bundled VAE was mis-filed into the checkpoint folder")
	}
}

// TestDownloadLayoutSanitizesHostileServerName is the Fix 4 assertion: under
// --layout, a hostile server filename ("../../evil.txt") is still basename-
// sanitized and lands INSIDE the routed type folder under --root, never outside
// it. The traversal guard is shared with the default/--out-dir cases, but the
// --layout branch (routeDir + filepath.Join) deserves its own end-to-end proof.
func TestDownloadLayoutSanitizesHostileServerName(t *testing.T) {
	const body = "pwned-weights"
	// A hostile file name in the JSON, but the downloadUrl points at a CLEAN path
	// (/dl/blob) so the transfer succeeds regardless of the name — the guard under
	// test is that f.Name is basename-sanitized before it hits the filesystem.
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SDXL 1.0","model":{"name":"M","type":"Checkpoint"},"files":[{"id":1,"name":"../../evil.txt","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/blob","hashes":{"SHA256":"%s"}}]}`, base, sha256hex(body))
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	allowPrivateDownloadHostsInTest(t)
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())
	root := t.TempDir()

	if _, _, err := run(t, "download", "128713", "--layout", "comfyui", "--root", root); err != nil {
		t.Fatalf("download --layout hostile name: %v", err)
	}

	// Lands as <root>/models/checkpoints/evil.txt — basename only, inside the folder.
	saved := filepath.Join(root, "models", "checkpoints", "evil.txt")
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("sanitized file should be %s: %v", saved, err)
	}
	if string(got) != body {
		t.Errorf("content = %q, want %q", got, body)
	}
	// Must NOT have escaped the routed folder (../../ up from models/checkpoints,
	// nor up from --root itself).
	for _, escaped := range []string{
		filepath.Clean(filepath.Join(root, "models", "checkpoints", "..", "..", "evil.txt")),
		filepath.Clean(filepath.Join(root, "..", "..", "evil.txt")),
	} {
		if escaped == saved {
			continue
		}
		if _, err := os.Stat(escaped); err == nil {
			t.Errorf("hostile name escaped the routed folder: %s exists", escaped)
		}
	}
	// The resolved path must stay within --root.
	clean := filepath.Clean(saved)
	if !strings.HasPrefix(clean, filepath.Clean(root)+string(filepath.Separator)) {
		t.Errorf("routed path %q escaped --root %q", clean, root)
	}
}

// TestRouteDirWithHostileBasenameStaysInFolder is the unit-level guard for the
// --layout branch: routeDir + join keeps a basename-sanitized name inside the
// type folder (the caller passes the already-Base'd name, as targetPath does).
func TestRouteDirWithHostileBasenameStaysInFolder(t *testing.T) {
	root := t.TempDir()
	// targetPath calls filepath.Base BEFORE routeDir; emulate that here.
	base := filepath.Base("../../evil.txt")
	dir, _ := routeDir("a1111", root, "Model", "Checkpoint", base)
	full := filepath.Join(dir, base)
	want := filepath.Join(root, "models", "Stable-diffusion", "evil.txt")
	if full != want {
		t.Errorf("routed path = %q, want %q", full, want)
	}
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(root)+string(filepath.Separator)) {
		t.Errorf("routed path %q escaped --root %q", full, root)
	}
}

// TestDownloadLayoutUnknownTypeFallsBackToRootWithNote proves an unmapped type
// (Poses) writes to --root and prints a stderr note.
func TestDownloadLayoutUnknownTypeFallsBackToRootWithNote(t *testing.T) {
	d := newLayoutServer(t, "Poses", []dlFile{
		{id: 1, name: "poses.zip", typ: "Archive", primary: true, body: "poses"},
	})
	setupDownloadEnv(t, d, "tok")
	root := t.TempDir()

	_, errOut, err := run(t, "download", "128713", "--layout", "comfyui", "--root", root)
	if err != nil {
		t.Fatalf("download unknown type: %v", err)
	}
	if _, e := os.Stat(filepath.Join(root, "poses.zip")); e != nil {
		t.Errorf("unknown type should land at --root: %v", e)
	}
	if !strings.Contains(errOut, "no comfyui folder") {
		t.Errorf("expected a fallback note on stderr, got: %s", errOut)
	}
}

// TestDownloadLayoutDryRunPlansRoutedTargets proves --dry-run prints the routed
// target paths and transfers nothing.
func TestDownloadLayoutDryRunPlansRoutedTargets(t *testing.T) {
	d := newLayoutServer(t, "LORA", []dlFile{
		{id: 1, name: "style.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
	})
	setupDownloadEnv(t, d, "tok")

	out, _, err := run(t, "download", "128713", "--layout", "comfyui", "--root", "/base", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run --layout: %v", err)
	}
	want := filepath.Join("/base", "models", "loras", "style.safetensors")
	if !strings.Contains(out, want) {
		t.Errorf("plan should show routed target %q:\n%s", want, out)
	}
	if d.dlHits != 0 {
		t.Errorf("dry-run must not transfer (dlHits=%d)", d.dlHits)
	}
}

// TestDownloadLayoutConflicts proves --layout rejects --out / --out-dir and
// --root without --layout, and an invalid layout value.
func TestDownloadLayoutConflicts(t *testing.T) {
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
	})
	setupDownloadEnv(t, d, "tok")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"layout+out", []string{"download", "1", "--layout", "a1111", "--out", "x.safetensors"}, "can't be combined with --out"},
		{"layout+outdir", []string{"download", "1", "--layout", "a1111", "--out-dir", "d"}, "can't be combined with --out-dir"},
		{"root without layout", []string{"download", "1", "--root", "d"}, "--root only applies with --layout"},
		{"invalid layout", []string{"download", "1", "--layout", "invokeai"}, "--layout must be one of"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := run(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("args %v: err = %v, want containing %q", tc.args, err, tc.want)
			}
		})
	}
}

// TestDownloadForBaseWarnsOnMismatch proves --for-base warns on a confident
// cross-family mismatch, and stays silent on same-family / when absent.
func TestDownloadForBaseWarnsOnMismatch(t *testing.T) {
	// The server's version reports baseModel "SDXL 1.0".
	newServer := func() *dlServer {
		return newLayoutServer(t, "TextualInversion", []dlFile{
			{id: 1, name: "easyneg.pt", typ: "Model", primary: true, body: "emb", withSHA: true},
		})
	}

	// SD 1.5 target vs SDXL 1.0 version → confident mismatch → warn.
	d := newServer()
	setupDownloadEnv(t, d, "tok")
	_, errOut, err := run(t, "download", "128713", "--for-base", "SD 1.5", "--dry-run")
	if err != nil {
		t.Fatalf("for-base mismatch: %v", err)
	}
	if !strings.Contains(errOut, "may be incompatible") {
		t.Errorf("expected an incompatibility warning, got stderr: %s", errOut)
	}

	// Same family (SDXL) → no warn.
	d2 := newServer()
	setupDownloadEnv(t, d2, "tok")
	_, errOut2, err := run(t, "download", "128713", "--for-base", "Pony", "--dry-run")
	if err != nil {
		t.Fatalf("for-base same-family: %v", err)
	}
	if strings.Contains(errOut2, "may be incompatible") {
		t.Errorf("same-family target should not warn, got stderr: %s", errOut2)
	}

	// No --for-base → no warn.
	d3 := newServer()
	setupDownloadEnv(t, d3, "tok")
	_, errOut3, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("no for-base: %v", err)
	}
	if strings.Contains(errOut3, "may be incompatible") {
		t.Errorf("absent --for-base should never warn, got stderr: %s", errOut3)
	}
}

// TestDownloadAllMixedTypeWarnsWithoutLayout proves the --all (no --layout)
// mis-file warning fires for a checkpoint + bundled VAE and is silent for a
// single-type set.
func TestDownloadAllMixedTypeWarnsWithoutLayout(t *testing.T) {
	// Mixed: checkpoint + VAE.
	d := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "ckpt", withSHA: true},
		{id: 2, name: "m.vae.pt", typ: "VAE", body: "vae"},
	})
	setupDownloadEnv(t, d, "tok")
	dir := t.TempDir()
	_, errOut, err := run(t, "download", "128713", "--all", "--out-dir", dir)
	if err != nil {
		t.Fatalf("download --all mixed: %v", err)
	}
	if !strings.Contains(errOut, "mixes file types") {
		t.Errorf("expected a mixed-type warning on stderr, got: %s", errOut)
	}

	// Single-type: two checkpoint files → no warning.
	d2 := newLayoutServer(t, "Checkpoint", []dlFile{
		{id: 1, name: "a.safetensors", typ: "Model", primary: true, body: "a", withSHA: true},
		{id: 2, name: "b.safetensors", typ: "Model", body: "b", withSHA: true},
	})
	setupDownloadEnv(t, d2, "tok")
	dir2 := t.TempDir()
	_, errOut2, err := run(t, "download", "128713", "--all", "--out-dir", dir2)
	if err != nil {
		t.Fatalf("download --all single-type: %v", err)
	}
	if strings.Contains(errOut2, "mixes file types") {
		t.Errorf("single-type set should not warn, got: %s", errOut2)
	}
}
