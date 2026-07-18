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

	"github.com/civitai/cli/internal/api"
)

// TestTargetPathSanitizesServerName proves the API-supplied file name is reduced
// to a bare basename in the two server-named modes (default + --out-dir), so a
// hostile name can never escape the intended directory. --out is used verbatim.
func TestTargetPathSanitizesServerName(t *testing.T) {
	cases := []struct {
		name    string // server-supplied f.Name
		want    string // expected basename
		wantErr bool
	}{
		{"../../etc/passwd", "passwd", false},
		{"/abs/path/x", "x", false},
		{"a/b/c.safetensors", "c.safetensors", false},
		{"model.safetensors", "model.safetensors", false},
		{"..", "", true},
		{"", "", true},
		{"/", "", true},
		{"///", "", true},
	}
	const outDir = "/tmp/models"
	for _, tc := range cases {
		f := api.ModelVersionFile{Name: tc.name}

		// Default mode: resolves to a bare basename in cwd.
		got, _, err := targetPath(f, &downloadOpts{})
		if tc.wantErr {
			if err == nil {
				t.Errorf("default %q: expected an error, got path %q", tc.name, got)
			}
		} else {
			if err != nil {
				t.Errorf("default %q: unexpected error: %v", tc.name, err)
			} else if got != tc.want {
				t.Errorf("default %q -> %q, want basename %q", tc.name, got, tc.want)
			}
			// A resolved default path must be a single path element (no separators).
			if err == nil && strings.ContainsRune(got, filepath.Separator) {
				t.Errorf("default %q escaped cwd: %q contains a separator", tc.name, got)
			}
		}

		// --out-dir mode: resolves strictly inside outDir.
		gotDir, _, errDir := targetPath(f, &downloadOpts{outDir: outDir})
		if tc.wantErr {
			if errDir == nil {
				t.Errorf("out-dir %q: expected an error, got path %q", tc.name, gotDir)
			}
			continue
		}
		if errDir != nil {
			t.Errorf("out-dir %q: unexpected error: %v", tc.name, errDir)
			continue
		}
		if want := filepath.Join(outDir, tc.want); gotDir != want {
			t.Errorf("out-dir %q -> %q, want %q", tc.name, gotDir, want)
		}
		// The resolved path must stay within outDir (no traversal escape).
		clean := filepath.Clean(gotDir)
		if clean != filepath.Clean(outDir) && !strings.HasPrefix(clean, filepath.Clean(outDir)+string(filepath.Separator)) {
			t.Errorf("out-dir %q escaped: resolved %q is outside %q", tc.name, clean, outDir)
		}
	}
}

// TestTargetPathOutIsVerbatim proves --out (the user's OWN explicit target) is
// NOT sanitized — a relative or absolute path is honored as given.
func TestTargetPathOutIsVerbatim(t *testing.T) {
	f := api.ModelVersionFile{Name: "server-name.safetensors"}
	for _, out := range []string{
		"../../my/explicit/path.safetensors",
		"/abs/target.safetensors",
		"rel/name.safetensors",
	} {
		got, _, err := targetPath(f, &downloadOpts{out: out})
		if err != nil {
			t.Errorf("--out %q: unexpected error: %v", out, err)
		}
		if got != out {
			t.Errorf("--out %q -> %q, want it verbatim", out, got)
		}
	}
}

// TestDownloadSanitizesHostileServerName is the end-to-end guard: a version whose
// files[].name is "../../evil.txt" must land as evil.txt INSIDE the --out-dir,
// never escape it.
func TestDownloadSanitizesHostileServerName(t *testing.T) {
	const body = "pwned-bytes"
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SD 1.5","files":[{"id":1,"name":"../../evil.txt","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/blob","hashes":{"SHA256":"%s"}}]}`, base, sha256hex(body))
	})
	// Serve the bytes at a CLEAN path (independent of the hostile file name).
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	dir := t.TempDir()
	chdir(t, t.TempDir())

	if _, _, err := run(t, "download", "128713", "--out-dir", dir); err != nil {
		t.Fatalf("download: %v", err)
	}

	// Lands as dir/evil.txt.
	saved := filepath.Join(dir, "evil.txt")
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("sanitized file should be %s: %v", saved, err)
	}
	if string(got) != body {
		t.Errorf("content = %q, want %q", got, body)
	}
	// Must NOT have escaped two levels up.
	escaped := filepath.Clean(filepath.Join(dir, "..", "..", "evil.txt"))
	if _, err := os.Stat(escaped); err == nil {
		t.Errorf("hostile name escaped the target dir: %s exists", escaped)
	}
}
