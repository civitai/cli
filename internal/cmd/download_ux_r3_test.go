package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDownloadYesEchoesVersionInterpretation proves fix 2: when --yes bypasses the
// ambiguity stop, the CLI ECHOES exactly which interpretation it chose — the
// VERSION (named by its own parent model) plus how to get the OTHER model whose id
// was pasted — so a reflexive --yes can't silently grab the wrong thing. The echo
// goes to stderr (stdout stays the plan/JSON channel).
func TestDownloadYesEchoesVersionInterpretation(t *testing.T) {
	// 277680: version's parent is "Kai Ipusiron"; the SAME id is also model
	// "Pixel Art Diffusion XL" — the unrelated model a reflexive --yes would miss.
	srv := newAmbiguityServer(t, "Pixel Art Diffusion XL", "Kai Ipusiron", true)
	setupAmbiguityEnv(t, srv)

	out, errOut, err := run(t, "download", "277680", "--yes", "--dry-run")
	if err != nil {
		t.Fatalf("--yes should proceed past the ambiguity stop, got: %v", err)
	}
	// The echo names the VERSION being downloaded (by its parent model) and points
	// at --model for the OTHER interpretation.
	if !strings.Contains(errOut, "downloading VERSION 277680") {
		t.Errorf("--yes echo should name the version being downloaded, got stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "Kai Ipusiron") {
		t.Errorf("--yes echo should name the version's parent model \"Kai Ipusiron\", got stderr:\n%s", errOut)
	}
	if !strings.Contains(errOut, "--model 277680") || !strings.Contains(errOut, "Pixel Art Diffusion XL") {
		t.Errorf("--yes echo should point at --model for the other (pasted) model, got stderr:\n%s", errOut)
	}
	// The plan itself still proceeds on stdout.
	if !strings.Contains(out, "would download") {
		t.Errorf("--yes should plan the version download:\n%s", out)
	}
}

// TestDownloadModelIDNoVersionsErrors proves the auto-resolve path fails cleanly
// when the pasted model id is a valid model that has NO published versions: there
// is nothing to resolve, so it errors rather than silently doing nothing.
func TestDownloadModelIDNoVersionsErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"Model not found"}`)
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		// A valid model (echoes the id) but with an empty modelVersions list.
		fmt.Fprintf(w, `{"id":%s,"name":"Empty","type":"Checkpoint","modelVersions":[]}`, lastPathSeg(r.URL.Path))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	setupAmbiguityEnv(t, srv)

	_, _, err := run(t, "download", "4384", "--dry-run")
	if err == nil {
		t.Fatal("a model id with no published versions should error, not silently succeed")
	}
	if !strings.Contains(err.Error(), "no published versions") {
		t.Errorf("error should explain there are no published versions to download, got: %v", err)
	}
}
