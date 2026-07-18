package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// TestDownloadModelIDPassedAsVersionHints proves the model-id-vs-version-id
// trap gives an ACTIONABLE hint: `civitai models search` shows MODEL ids, but
// `civitai download <id>`'s positional is a model-VERSION id — so a user handing
// a model id (4384) gets a 404 on the version lookup. When that id IS a valid
// model id, the error must point at `civitai download --model <id>` instead of a
// bare "not found", and be classified as a usage error.
func TestDownloadModelIDPassedAsVersionHints(t *testing.T) {
	// The version lookup 404s (the id is NOT a version), but the model lookup
	// succeeds (the id IS a valid model). --dry-run so no bytes ever move.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"Model not found"}`)
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":4384,"name":"DreamShaper","type":"Checkpoint","modelVersions":[{"id":128713,"name":"v8","baseModel":"SD 1.5","files":[{"id":1,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"x"}]}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "4384", "--dry-run")
	if err == nil {
		t.Fatal("a model id passed as a version id should error")
	}
	msg := err.Error()
	// Actionable: names the mistake and the fix.
	if !strings.Contains(msg, "is a model id") {
		t.Errorf("error should say the id is a model id, got: %v", err)
	}
	if !strings.Contains(msg, "--model 4384") {
		t.Errorf("error should suggest `--model 4384`, got: %v", err)
	}
	// Must NOT be the bare, misleading not-found message.
	if strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("hint should replace the bare not-found message, got: %v", err)
	}
	// Classified as a usage error (exit code 2), not a not-found (4).
	if !errors.Is(err, ErrUsage) {
		t.Errorf("the hint should be tagged as a usage error, got: %v", err)
	}
}

// TestDownloadUnknownVersionIDKeepsNotFound proves the hint does NOT fire when
// the id is neither a valid version NOR a valid model: the original 404
// (classified as not-found) is preserved so scripts still see exit 4.
func TestDownloadUnknownVersionIDKeepsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "99999999", "--dry-run")
	if err == nil {
		t.Fatal("an unknown id should still error")
	}
	if strings.Contains(err.Error(), "is a model id") {
		t.Errorf("no model-id hint when the id isn't a model either: %v", err)
	}
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("an unknown id must stay classified as not-found (exit 4), got: %v", err)
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("an unknown id is not a usage error, got: %v", err)
	}
}

// TestDownloadValidVersionIDUnaffected proves the happy path is byte-for-byte
// unchanged: a real version id still resolves + downloads with no model lookup
// and no hint.
func TestDownloadValidVersionIDUnaffected(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")

	out, _, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("a valid version id should plan cleanly, got: %v", err)
	}
	if !strings.Contains(out, "would download") {
		t.Errorf("valid version id should still plan the download:\n%s", out)
	}
	if strings.Contains(out, "is a model id") {
		t.Errorf("no hint should appear for a valid version id:\n%s", out)
	}
}

// TestDownloadModelFlagUnknownVersionNoHint proves the hint is scoped to the
// POSITIONAL path: a --model resolution that yields an unknown version id must
// NOT be reinterpreted as a model-id mistake (positional is empty).
func TestDownloadModelFlagUnknownVersionNoHint(t *testing.T) {
	// models/{id} resolves to a version id whose model-versions lookup 404s.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":4384,"name":"M","type":"Checkpoint","modelVersions":[{"id":777,"name":"v","baseModel":"SD 1.5","files":[]}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "--model", "4384", "--dry-run")
	if err == nil {
		t.Fatal("an unknown resolved version should error")
	}
	if strings.Contains(err.Error(), "is a model id") {
		t.Errorf("--model path must not emit the positional model-id hint: %v", err)
	}
}
