package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newAmbiguityServer wires a fake API where a single numeric id can resolve as
// BOTH a model-VERSION and a MODEL — the exact footgun `civitai download <id>`
// hits when a user pastes a MODEL id (which `models search` lists) into the
// version-id positional and that number ALSO happens to be a valid version id.
//
//   - GET /api/v1/model-versions/{id} always returns a valid version whose parent
//     model is `parentModel` (this is the UNRELATED model whose file would be
//     silently downloaded).
//   - GET /api/v1/models/{id} returns a valid model named `modelName` whose id
//     ECHOES the requested id when modelIsValid; otherwise it 404s (the id is a
//     version but NOT a model → unambiguous).
//
// The real REST API always echoes the queried id back, so the id-match guard in
// stopIfAmbiguousModelID keys off it; a stub that returned a different id would
// (correctly) be treated as not-ambiguous.
func newAmbiguityServer(t *testing.T, modelName, parentModel string, modelIsValid bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		id := lastPathSeg(r.URL.Path)
		fmt.Fprintf(w, `{"id":%s,"modelId":99999,"name":"v1","baseModel":"SD 1.5","model":{"name":%q,"type":"LORA"},"files":[{"id":1,"name":"unrelated.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"http://127.0.0.1:1/dl","hashes":{"SHA256":"%s"}}]}`,
			id, parentModel, strings.Repeat("a", 64))
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		if !modelIsValid {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"Model not found"}`)
			return
		}
		id := lastPathSeg(r.URL.Path)
		// Echo the requested id (the real API does) so the id-match guard fires.
		fmt.Fprintf(w, `{"id":%s,"name":%q,"type":"Checkpoint","modelVersions":[{"id":424242,"name":"default","baseModel":"SDXL","files":[{"id":9,"name":"default.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"http://127.0.0.1:1/dl"}]}]}`,
			id, modelName)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func lastPathSeg(p string) string {
	return p[strings.LastIndexByte(p, '/')+1:]
}

// setupAmbiguityEnv points the CLI at srv with a fresh anonymous config + cwd.
func setupAmbiguityEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())
}

// TestDownloadAmbiguousModelIDStops is the core footgun fix: a bare positional id
// that is BOTH a valid version id AND a valid model id must STOP for
// disambiguation (exit 2) rather than silently downloading the version — which is
// an unrelated model's file — and laundering it with "SHA256 verified".
func TestDownloadAmbiguousModelIDStops(t *testing.T) {
	// 277680: real-world case — model "Pixel Art Diffusion XL" AND version 277680
	// (of an unrelated model "Kai Ipusiron").
	srv := newAmbiguityServer(t, "Pixel Art Diffusion XL", "Kai Ipusiron", true)
	setupAmbiguityEnv(t, srv)

	out, _, err := run(t, "download", "277680", "--dry-run")
	if err == nil {
		t.Fatal("an id that is both a model id and a version id must STOP, not download")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("the ambiguous stop must be a usage error (exit 2), got: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"ambiguous", "Pixel Art Diffusion XL", "Kai Ipusiron", "--model 277680", "--version 277680"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ambiguity error should mention %q, got: %v", want, err)
		}
	}
	// It must NOT have planned/printed a download for the unrelated version.
	if strings.Contains(out, "would download") {
		t.Errorf("no plan should be printed for an ambiguous id:\n%s", out)
	}
}

// TestDownloadAmbiguousBypassedByVersionFlag proves --version <id> names a version
// explicitly and skips the ambiguity stop (the user has disambiguated by intent).
func TestDownloadAmbiguousBypassedByVersionFlag(t *testing.T) {
	srv := newAmbiguityServer(t, "Pixel Art Diffusion XL", "Kai Ipusiron", true)
	setupAmbiguityEnv(t, srv)

	out, _, err := run(t, "download", "--version", "277680", "--dry-run")
	if err != nil {
		t.Fatalf("--version should proceed past the ambiguity stop, got: %v", err)
	}
	if !strings.Contains(out, "would download") {
		t.Errorf("--version should plan the version download:\n%s", out)
	}
	if strings.Contains(out, "ambiguous") {
		t.Errorf("--version must not trigger the ambiguity stop:\n%s", out)
	}
}

// TestDownloadAmbiguousBypassedByYes proves --yes lets the bare positional proceed
// (download the version as typed) after acknowledging the ambiguity.
func TestDownloadAmbiguousBypassedByYes(t *testing.T) {
	srv := newAmbiguityServer(t, "Pixel Art Diffusion XL", "Kai Ipusiron", true)
	setupAmbiguityEnv(t, srv)

	out, _, err := run(t, "download", "277680", "--yes", "--dry-run")
	if err != nil {
		t.Fatalf("--yes should proceed past the ambiguity stop, got: %v", err)
	}
	if !strings.Contains(out, "would download") {
		t.Errorf("--yes should plan the version download:\n%s", out)
	}
}

// TestDownloadNonAmbiguousVersionUnchanged proves the guard does NOT fire when the
// id is a valid version but NOT a model id (models/{id} 404s): the download
// proceeds exactly as before — no extra stop, no behavior change.
func TestDownloadNonAmbiguousVersionUnchanged(t *testing.T) {
	srv := newAmbiguityServer(t, "", "Some Parent", false) // model lookup 404s
	setupAmbiguityEnv(t, srv)

	out, _, err := run(t, "download", "277680", "--dry-run")
	if err != nil {
		t.Fatalf("a valid version that is NOT a model id must plan cleanly, got: %v", err)
	}
	if !strings.Contains(out, "would download") {
		t.Errorf("non-ambiguous version should still plan the download:\n%s", out)
	}
	if strings.Contains(out, "ambiguous") {
		t.Errorf("no ambiguity stop when the id is not a model id:\n%s", out)
	}
}

// TestDownloadModelIDMismatchNotAmbiguous proves the id-match guard: if the model
// lookup succeeds but returns a DIFFERENT id than requested (i.e. the number did
// not actually resolve as that model), it is treated as NOT ambiguous and the
// version download proceeds. Protects real version downloads from a false stop.
func TestDownloadModelIDMismatchNotAmbiguous(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		id := lastPathSeg(r.URL.Path)
		fmt.Fprintf(w, `{"id":%s,"modelId":99999,"name":"v1","baseModel":"SD 1.5","model":{"name":"Parent","type":"LORA"},"files":[{"id":1,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"http://127.0.0.1:1/dl","hashes":{"SHA256":"%s"}}]}`,
			id, strings.Repeat("a", 64))
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		// Always returns id 4384 regardless of the requested id (like the shared
		// newDLServer). A queried id != 4384 must therefore be non-ambiguous.
		fmt.Fprint(w, `{"id":4384,"name":"Other","type":"Checkpoint","modelVersions":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	setupAmbiguityEnv(t, srv)

	out, _, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("a model lookup that echoes a different id must not stop, got: %v", err)
	}
	if strings.Contains(out, "ambiguous") {
		t.Errorf("id-mismatch must not be treated as ambiguous:\n%s", out)
	}
}

// TestDownloadBadPositionalIDIsUsageError proves a non-integer positional id is a
// local usage error (exit 2), not a generic exit-1 failure.
func TestDownloadBadPositionalIDIsUsageError(t *testing.T) {
	srv := newAmbiguityServer(t, "m", "p", true)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "not-a-number", "--dry-run")
	if err == nil {
		t.Fatal("a non-integer id should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a bad positional id should be a usage error (exit 2), got: %v", err)
	}
}

// TestDownloadLayoutBogusIsUsageError proves an invalid --layout value is exit 2.
func TestDownloadLayoutBogusIsUsageError(t *testing.T) {
	srv := newAmbiguityServer(t, "m", "p", true)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "--version", "277680", "--layout", "bogus", "--dry-run")
	if err == nil {
		t.Fatal("--layout bogus should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--layout bogus should be a usage error (exit 2), got: %v", err)
	}
}

// TestDownloadRootWithoutLayoutIsUsageError proves --root without --layout is exit 2.
func TestDownloadRootWithoutLayoutIsUsageError(t *testing.T) {
	srv := newAmbiguityServer(t, "m", "p", true)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "--version", "277680", "--root", "/tmp/x", "--dry-run")
	if err == nil {
		t.Fatal("--root without --layout should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--root without --layout should be a usage error (exit 2), got: %v", err)
	}
}

// TestDownloadBadFileIsUsageError proves a --file value that matches no file is
// exit 2 (a bad flag value), not a generic failure.
func TestDownloadBadFileIsUsageError(t *testing.T) {
	srv := newAmbiguityServer(t, "", "Parent", false) // non-ambiguous version so we reach file selection
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "277680", "--file", "does-not-exist", "--dry-run")
	if err == nil {
		t.Fatal("a --file that matches nothing should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a bad --file value should be a usage error (exit 2), got: %v", err)
	}
}

// TestDownloadVersionFlagBadIntIsUsageError proves a non-integer --version is exit 2.
func TestDownloadVersionFlagBadIntIsUsageError(t *testing.T) {
	srv := newAmbiguityServer(t, "m", "p", true)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "--version", "abc", "--dry-run")
	if err == nil {
		t.Fatal("a non-integer --version should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("a bad --version should be a usage error (exit 2), got: %v", err)
	}
}

// TestDownloadRejectsPositionalPlusVersion proves the three-way exclusivity: a
// positional id AND --version together is a usage error.
func TestDownloadRejectsPositionalPlusVersion(t *testing.T) {
	srv := newAmbiguityServer(t, "m", "p", true)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "277680", "--version", "128713", "--dry-run")
	if err == nil {
		t.Fatal("a positional id plus --version should error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("positional + --version should be a usage error (exit 2), got: %v", err)
	}
}
