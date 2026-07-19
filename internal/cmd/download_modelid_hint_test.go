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

// TestDownloadModelIDPositionalAutoResolves proves the model-id-vs-version-id
// trap now RESOLVES instead of hint-walling: `civitai models search` lists MODEL
// ids, so a user handing a model id (4384) into the version-id positional — an id
// that is a valid MODEL but NOT a valid version — has its default version
// resolved + downloaded automatically, with a note saying it did. This turns the
// common search→download flow into a success instead of an error.
func TestDownloadModelIDPositionalAutoResolves(t *testing.T) {
	// model-versions/4384 404s (4384 is NOT a version) but model-versions/128713
	// (the model's default version) succeeds; models/4384 echoes the model with
	// its default version 128713. --dry-run so no bytes ever move.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		if lastPathSeg(r.URL.Path) == "128713" {
			fmt.Fprint(w, `{"id":128713,"modelId":4384,"name":"v8","baseModel":"SD 1.5","model":{"name":"DreamShaper","type":"Checkpoint"},"files":[{"id":1,"name":"m.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"http://127.0.0.1:1/dl","hashes":{"SHA256":"`+strings.Repeat("a", 64)+`"}}]}`)
			return
		}
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

	out, errOut, err := run(t, "download", "4384", "--dry-run")
	if err != nil {
		t.Fatalf("a model id positional should auto-resolve its default version, got: %v", err)
	}
	// The plan is for the model's default version, and a note explains the resolve.
	if !strings.Contains(out, "would download") {
		t.Errorf("model-id positional should plan its default version:\n%s", out)
	}
	if !strings.Contains(errOut, "4384 is a model id") || !strings.Contains(errOut, "default version 128713") {
		t.Errorf("a note should say 4384 is a model id and name the resolved version 128713, got stderr:\n%s", errOut)
	}
	// It must NOT hint-wall with the old "--model" error.
	if strings.Contains(out, "did you mean") || strings.Contains(errOut, "did you mean") {
		t.Errorf("the old hint-wall must be gone; it should just resolve:\nstdout: %s\nstderr: %s", out, errOut)
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
