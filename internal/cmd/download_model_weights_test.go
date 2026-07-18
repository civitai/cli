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
	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", m.srv.URL)
	chdir(t, t.TempDir())
}

// --model resolves the model's DEFAULT (first) version regardless of file type —
// even when that default's primary file is a non-"Model" Archive — and downloads
// it with no skip and no error.
func TestModelResolvesDefaultVersionEvenWhenNonWeights(t *testing.T) {
	// Default (versions[0]) is a non-weights Archive; a later version has weights.
	m := newMVServer(t, 4384, []mvVer{
		{id: 900, name: "workflow-registration", ftype: "Archive"},
		{id: 800, name: "v1.0-weights", ftype: "Model"},
	})
	setupMVEnv(t, m)

	_, errOut, err := run(t, "download", "--model", "4384")
	if err != nil {
		t.Fatalf("download --model should resolve the default version, got %v", err)
	}
	// It must have fetched the DEFAULT version 900, not the later weights one.
	if !strings.HasSuffix(m.fetchedVerURL, "/900") {
		t.Errorf("expected the default version 900 to be downloaded, fetched %q", m.fetchedVerURL)
	}
	if strings.Contains(errOut, "downloadable weights") {
		t.Errorf("no weights-skip note should be printed anymore: %s", errOut)
	}
}

// Repro (b): every version's primary file is non-"Model" — --model still resolves
// the default version and downloads it, no error.
func TestModelAllNonWeightsResolvesDefault(t *testing.T) {
	m := newMVServer(t, 4384, []mvVer{
		{id: 900, name: "workflows", ftype: "Archive"},
		{id: 901, name: "training", ftype: "Training Data"},
	})
	setupMVEnv(t, m)

	_, _, err := run(t, "download", "--model", "4384")
	if err != nil {
		t.Fatalf("--model with all-non-weights versions should resolve the default, got %v", err)
	}
	if !strings.HasSuffix(m.fetchedVerURL, "/900") {
		t.Errorf("expected the default version 900 to be downloaded, fetched %q", m.fetchedVerURL)
	}
}

// Repro (c): when the default version already IS weights, --model resolves it with
// no note.
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

// The explicit positional non-weights path now downloads the file (no refusal).
func TestExplicitPositionalNonWeightsDownloads(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "training.zip", typ: "Training Data", primary: true, body: "trainingdata"},
	})
	setupDownloadEnv(t, d, "")
	if _, _, err := run(t, "download", "128713"); err != nil {
		t.Fatalf("explicit positional non-weights should download, got %v", err)
	}
	if _, e := os.Stat("training.zip"); e != nil {
		t.Errorf("positional non-weights download must write the file: %v", e)
	}
}
