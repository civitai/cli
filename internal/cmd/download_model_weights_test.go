package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// mvVer is one version of a model for the multi-version resolution tests: its id,
// name, and the `type` of its single primary file (drives the weights/non-weights
// decision in resolveVersionID).
type mvVer struct {
	id    int
	name  string
	ftype string // primary file type: "Model" = downloadable weights
}

// mvServer serves GET /api/v1/models/{id} with a caller-controlled version list
// (each version's primary file type set), plus GET /api/v1/model-versions/{id}
// which always returns a downloadable weights file so whichever version resolves
// can complete its download. It records the version id the download actually
// fetched.
type mvServer struct {
	srv           *httptest.Server
	fetchedVerURL string // last /api/v1/model-versions/<id> path requested
}

func newMVServer(t *testing.T, modelID int, versions []mvVer) *mvServer {
	t.Helper()
	m := &mvServer{}
	var base string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		var vs []string
		for _, v := range versions {
			vs = append(vs, fmt.Sprintf(
				`{"id":%d,"name":%q,"baseModel":"SD 1.5","files":[{"id":1,"name":"primary","type":%q,"sizeKB":1,"primary":true,"downloadUrl":"%s/dl/primary"}]}`,
				v.id, v.name, v.ftype, base))
		}
		fmt.Fprintf(w, `{"id":%d,"name":"M","type":"Checkpoint","modelVersions":[%s]}`, modelID, strings.Join(vs, ","))
	})
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		m.fetchedVerURL = r.URL.Path
		fmt.Fprintf(w, `{"id":1,"modelId":%d,"name":"v","baseModel":"SD 1.5","files":[{"id":1,"name":"weights.safetensors","type":"Model","sizeKB":1,"primary":true,"downloadUrl":"%s/dl/weights.safetensors"}]}`, modelID, base)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("WEIGHTS-BYTES"))
	})

	m.srv = httptest.NewServer(mux)
	base = m.srv.URL
	t.Cleanup(m.srv.Close)
	return m
}

func setupMVEnv(t *testing.T, m *mvServer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", m.srv.URL)
	chdir(t, t.TempDir())
}

func TestModelResolvesSkipsNonWeightsDefault(t *testing.T) {
	// Default (versions[0]) is training-data; a later version has weights.
	m := newMVServer(t, 4384, []mvVer{
		{id: 900, name: "training-registration", ftype: "Training Data"},
		{id: 800, name: "v1.0-weights", ftype: "Model"},
	})
	setupMVEnv(t, m)

	_, errOut, err := run(t, "download", "--model", "4384")
	if err != nil {
		t.Fatalf("download --model should resolve to the weights version: %v", err)
	}
	// It must have fetched version 800 (the weights one), not 900.
	if !strings.HasSuffix(m.fetchedVerURL, "/800") {
		t.Errorf("expected version 800 to be downloaded, fetched %q", m.fetchedVerURL)
	}
	if !strings.Contains(errOut, "800") || !strings.Contains(errOut, "downloadable weights") {
		t.Errorf("stderr should note the skip + chosen version: %s", errOut)
	}
	if !strings.Contains(errOut, "training data") {
		t.Errorf("stderr note should name the default's type: %s", errOut)
	}
}

func TestModelDefaultAlreadyWeightsNoNote(t *testing.T) {
	m := newMVServer(t, 4384, []mvVer{
		{id: 800, name: "v1.0-weights", ftype: "Model"},
		{id: 700, name: "older", ftype: "Model"},
	})
	setupMVEnv(t, m)

	_, errOut, err := run(t, "download", "--model", "4384")
	if err != nil {
		t.Fatalf("download --model: %v", err)
	}
	if !strings.HasSuffix(m.fetchedVerURL, "/800") {
		t.Errorf("should pick the default version 800, fetched %q", m.fetchedVerURL)
	}
	if strings.Contains(errOut, "downloadable weights") {
		t.Errorf("no skip note should be printed when the default already has weights: %s", errOut)
	}
}

func TestModelAllNonWeightsErrors(t *testing.T) {
	m := newMVServer(t, 4384, []mvVer{
		{id: 900, name: "training", ftype: "Training Data"},
		{id: 901, name: "onsite-gen", ftype: "Archive"},
	})
	setupMVEnv(t, m)

	_, _, err := run(t, "download", "--model", "4384")
	if err == nil || !strings.Contains(err.Error(), "no version with downloadable weights") {
		t.Fatalf("expected a clear no-weights error, got %v", err)
	}
}

// The explicit positional non-weights path is unchanged: it still refuses via the
// --allow-nonmodel guard (resolveVersionID does not touch it).
func TestExplicitPositionalNonWeightsStillRefuses(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "training.zip", typ: "Training Data", primary: true, body: "trainingdata"},
	})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713")
	if err == nil || !strings.Contains(err.Error(), "not model weights") {
		t.Fatalf("explicit positional non-weights should still be refused, got %v", err)
	}
	if _, e := os.Stat("training.zip"); e == nil {
		t.Error("refused positional download must not write the file")
	}
}
