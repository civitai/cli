package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// collisionServer is a fake API whose download URLs are keyed by FILE ID (not by
// name), so a version can expose two files that share a name yet serve DISTINCT
// bytes — the exact shape of Flux Dev v691639 (fp16 + fp8 both named
// flux_dev.safetensors). The shared newDLServer serves by name and can't model
// that, so the collision + id-disambiguation tests use this.
type collisionServer struct {
	srv    *httptest.Server
	dlHits int
}

func newCollisionServer(t *testing.T, versionID int, files []dlFile) *collisionServer {
	t.Helper()
	c := &collisionServer{}
	var base string
	byID := make(map[int]dlFile, len(files))
	for _, f := range files {
		byID[f.id] = f
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		var parts []string
		for _, f := range files {
			hashes := ""
			if f.withSHA {
				hashes = `,"hashes":{"SHA256":"` + sha256hex(f.body) + `"}`
			}
			parts = append(parts, fmt.Sprintf(
				`{"id":%d,"name":%q,"type":%q,"sizeKB":%f,"primary":%t,"downloadUrl":"%s/dl/%d"%s}`,
				f.id, f.name, f.typ, float64(len(f.body))/1024, f.primary, base, f.id, hashes))
		}
		fmt.Fprintf(w, `{"id":%d,"modelId":618692,"name":"v","baseModel":"Flux.1 D","model":{"name":"Flux","type":"Checkpoint"},"files":[%s]}`,
			versionID, strings.Join(parts, ","))
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		c.dlHits++
		idStr := r.URL.Path[strings.LastIndexByte(r.URL.Path, '/')+1:]
		id, _ := strconv.Atoi(idStr)
		if f, ok := byID[id]; ok {
			_, _ = w.Write([]byte(f.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	c.srv = httptest.NewServer(mux)
	base = c.srv.URL
	t.Cleanup(c.srv.Close)
	return c
}

func setupCollisionEnv(t *testing.T, c *collisionServer, token string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", token)
	t.Setenv("CIVITAI_BASE_URL", c.srv.URL)
	chdir(t, t.TempDir())
}

// sameName is the two-same-named-files fixture (fp16 + fp8, distinct ids/bytes).
func sameNameFiles() []dlFile {
	return []dlFile{
		{id: 691640, name: "flux_dev.safetensors", typ: "Model", primary: true, body: "FP16-TWENTYTWO-GB-BODY", withSHA: true},
		{id: 691641, name: "flux_dev.safetensors", typ: "Model", body: "FP8-SIXTEEN-GB-BODY", withSHA: true},
	}
}

// TestDownloadAllSameNameCollisionErrorsNoClobber is the core data-loss repro:
// --all on a version with two same-named files must FAIL before any transfer,
// name both colliding files with their ids, and write NOTHING (no clobber).
func TestDownloadAllSameNameCollisionErrorsNoClobber(t *testing.T) {
	c := newCollisionServer(t, 691639, sameNameFiles())
	setupCollisionEnv(t, c, "tok")
	dir := t.TempDir()

	_, _, err := run(t, "download", "691639", "--all", "--out-dir", dir)
	if err == nil {
		t.Fatal("expected a collision error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"same path", "691640", "691641", "--file"} {
		if !strings.Contains(msg, want) {
			t.Errorf("collision error should contain %q:\n%s", want, msg)
		}
	}
	// Nothing was written and nothing was fetched — no silent overwrite happened.
	if !dirIsEmpty(t, dir) {
		t.Error("collision must write NOTHING to the out-dir (no clobber)")
	}
	if c.dlHits != 0 {
		t.Errorf("collision must be detected before any transfer (dlHits=%d)", c.dlHits)
	}
}

// TestDownloadDryRunSameNameCollisionErrors proves the guard fires in --dry-run
// too (fail-safe before the plan), so a preview surfaces the unsafe plan.
func TestDownloadDryRunSameNameCollisionErrors(t *testing.T) {
	c := newCollisionServer(t, 691639, sameNameFiles())
	setupCollisionEnv(t, c, "tok")
	dir := t.TempDir()

	_, _, err := run(t, "download", "691639", "--all", "--out-dir", dir, "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "same path") {
		t.Fatalf("dry-run should also refuse a colliding plan, got %v", err)
	}
	if c.dlHits != 0 {
		t.Errorf("dry-run collision must fetch nothing (dlHits=%d)", c.dlHits)
	}
}

// TestDownloadLayoutSameNameCollisionErrors proves the guard also covers
// --layout routing (two Model files route to the same checkpoints folder under
// the same name → same target path).
func TestDownloadLayoutSameNameCollisionErrors(t *testing.T) {
	c := newCollisionServer(t, 691639, sameNameFiles())
	setupCollisionEnv(t, c, "tok")
	root := t.TempDir()

	_, _, err := run(t, "download", "691639", "--all", "--layout", "comfyui", "--root", root)
	if err == nil || !strings.Contains(err.Error(), "same path") {
		t.Fatalf("--layout routing two same-named files into one folder should collide, got %v", err)
	}
	if c.dlHits != 0 {
		t.Errorf("collision must be caught before transfer (dlHits=%d)", c.dlHits)
	}
}

// TestDownloadFileByIDSelectsOneOfTwoSameNamed proves --file <id> downloads
// EXACTLY the requested one of two same-named files, with the correct bytes.
func TestDownloadFileByIDSelectsOneOfTwoSameNamed(t *testing.T) {
	allowPrivateDownloadHostsInTest(t)
	files := sameNameFiles()
	for _, tc := range []struct {
		id   int
		body string
	}{
		{files[0].id, files[0].body},
		{files[1].id, files[1].body},
	} {
		t.Run(strconv.Itoa(tc.id), func(t *testing.T) {
			c := newCollisionServer(t, 691639, files)
			setupCollisionEnv(t, c, "tok")
			if _, _, err := run(t, "download", "691639", "--file", strconv.Itoa(tc.id)); err != nil {
				t.Fatalf("--file %d: %v", tc.id, err)
			}
			got, rerr := os.ReadFile("flux_dev.safetensors")
			if rerr != nil {
				t.Fatalf("selected file not written: %v", rerr)
			}
			if string(got) != tc.body {
				t.Errorf("--file %d downloaded wrong bytes: got %q, want %q", tc.id, got, tc.body)
			}
		})
	}
}

// TestDownloadFileByNameAmbiguousWhenSameNamed proves that selecting the shared
// NAME (rather than an id) no longer silently picks the first file — it errors
// and points at --file <id>, listing both ids.
func TestDownloadFileByNameAmbiguousWhenSameNamed(t *testing.T) {
	c := newCollisionServer(t, 691639, sameNameFiles())
	setupCollisionEnv(t, c, "tok")

	_, _, err := run(t, "download", "691639", "--file", "flux_dev.safetensors")
	if err == nil {
		t.Fatal("selecting two same-named files by name should be ambiguous, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"share this name", "--file <id>", "691640", "691641"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ambiguous-name error should contain %q:\n%s", want, msg)
		}
	}
	if c.dlHits != 0 {
		t.Errorf("ambiguous selection must not fetch (dlHits=%d)", c.dlHits)
	}
	if _, e := os.Stat("flux_dev.safetensors"); e == nil {
		t.Error("ambiguous selection must not write a file (no silent first-match download)")
	}
}

// TestDownloadDistinctNameAllStillWorks proves the guard doesn't regress the
// normal case: a version with DISTINCT-named files still downloads all of them.
func TestDownloadDistinctNameAllStillWorks(t *testing.T) {
	allowPrivateDownloadHostsInTest(t)
	c := newCollisionServer(t, 128713, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
		{id: 2, name: "vae.pt", typ: "VAE", body: "vae", withSHA: true},
	})
	setupCollisionEnv(t, c, "tok")
	dir := t.TempDir()

	if _, _, err := run(t, "download", "128713", "--all", "--out-dir", dir); err != nil {
		t.Fatalf("distinct-name --all should still work: %v", err)
	}
	for name, want := range map[string]string{"model.safetensors": "weights", "vae.pt": "vae"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("--all should write %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// TestDownloadDistinctNameDryRunShowsBothTargets proves --dry-run --all on
// distinct-named files plans BOTH targets and transfers nothing (the guard is a
// no-op here).
func TestDownloadDistinctNameDryRunShowsBothTargets(t *testing.T) {
	c := newCollisionServer(t, 128713, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
		{id: 2, name: "vae.pt", typ: "VAE", body: "v", withSHA: true},
	})
	setupCollisionEnv(t, c, "tok")
	dir := t.TempDir()

	out, _, err := run(t, "download", "128713", "--all", "--out-dir", dir, "--dry-run")
	if err != nil {
		t.Fatalf("distinct-name dry-run --all: %v", err)
	}
	for _, name := range []string{"model.safetensors", "vae.pt"} {
		if !strings.Contains(out, filepath.Join(dir, name)) {
			t.Errorf("plan should show target for %s:\n%s", name, out)
		}
	}
	if c.dlHits != 0 {
		t.Errorf("dry-run must transfer nothing (dlHits=%d)", c.dlHits)
	}
}
