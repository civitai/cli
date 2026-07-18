package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dlFile describes a file the fake API version exposes.
type dlFile struct {
	id      int
	name    string
	typ     string
	primary bool
	body    string // bytes served at the download URL
	withSHA bool   // include a correct hashes.SHA256 in the JSON
	badSHA  bool   // include a WRONG hashes.SHA256 (mismatch test)
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// dlServer wires a fake Civitai API + download storage for the download command.
// modelVersions/{id} and models/{id} both surface the same file set; each file's
// downloadUrl points back at this server (optionally via a one-hop redirect).
type dlServer struct {
	srv      *httptest.Server
	lastAuth string
	dlHits   int
}

func newDLServer(t *testing.T, redirect bool, files []dlFile) *dlServer {
	t.Helper()
	d := &dlServer{}
	var base string

	filesJSON := func() string {
		var parts []string
		for _, f := range files {
			hashes := ""
			switch {
			case f.badSHA:
				hashes = `,"hashes":{"SHA256":"` + strings.Repeat("0", 64) + `"}`
			case f.withSHA:
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
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SD 1.5","files":%s}`, filesJSON())
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":4384,"name":"M","type":"Checkpoint","modelVersions":[{"id":128713,"name":"v","baseModel":"SD 1.5","files":%s}]}`, filesJSON())
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
		d.lastAuth = r.Header.Get("Authorization")
		if redirect {
			http.Redirect(w, r, base+"/store"+r.URL.Path[len("/dl"):], http.StatusFound)
			return
		}
		serve(w, r)
	})
	mux.HandleFunc("/store/", serve)

	d.srv = httptest.NewServer(mux)
	base = d.srv.URL
	t.Cleanup(d.srv.Close)
	return d
}

// setupDownloadEnv points the CLI at d and gives it a fresh temp config + cwd.
// token != "" configures CIVITAI_TOKEN so downloads authenticate.
func setupDownloadEnv(t *testing.T, d *dlServer, token string) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("CIVITAI_TOKEN", token)
	t.Setenv("CIVITAI_BASE_URL", d.srv.URL)
	chdir(t, t.TempDir())
}

func TestDownloadVersionIDPrimaryDefault(t *testing.T) {
	const body = "PRIMARY-MODEL-WEIGHTS"
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: body, withSHA: true},
		{id: 2, name: "extra.vae.pt", typ: "VAE", body: "vae"},
	})
	setupDownloadEnv(t, d, "")
	out, _, err := run(t, "download", "128713")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	got, rerr := os.ReadFile("model.safetensors")
	if rerr != nil {
		t.Fatalf("primary file not written: %v", rerr)
	}
	if string(got) != body {
		t.Errorf("content = %q, want %q", got, body)
	}
	if _, err := os.Stat("extra.vae.pt"); err == nil {
		t.Error("only the primary file should be downloaded by default")
	}
	if !strings.Contains(out, "Saved model.safetensors") || !strings.Contains(out, "SHA256 verified") {
		t.Errorf("output should confirm save + verify: %s", out)
	}
	if _, err := os.Stat("model.safetensors.part"); err == nil {
		t.Error(".part file should be renamed away on success")
	}
}

func TestDownloadModelResolvesDefaultVersion(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	if _, _, err := run(t, "download", "--model", "4384"); err != nil {
		t.Fatalf("download --model: %v", err)
	}
	if _, err := os.Stat("m.safetensors"); err != nil {
		t.Errorf("--model should resolve + download the default version's file: %v", err)
	}
}

func TestDownloadRejectsBothIdAndModel(t *testing.T) {
	d := newDLServer(t, false, []dlFile{{id: 1, name: "m", typ: "Model", primary: true, body: "x"}})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713", "--model", "4384")
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

func TestDownloadRequiresAnIdentifier(t *testing.T) {
	d := newDLServer(t, false, nil)
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download")
	if err == nil || !strings.Contains(err.Error(), "version id") {
		t.Fatalf("expected missing-identifier error, got %v", err)
	}
}

func TestDownloadFileSelectExactAndSubstring(t *testing.T) {
	files := []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "w"},
		{id: 2, name: "config.yaml", typ: "Config", body: "cfg"},
	}
	// Exact (case-insensitive) name.
	d := newDLServer(t, false, files)
	setupDownloadEnv(t, d, "")
	if _, _, err := run(t, "download", "128713", "--file", "MODEL.SAFETENSORS"); err != nil {
		t.Fatalf("exact --file: %v", err)
	}
	if _, err := os.Stat("model.safetensors"); err != nil {
		t.Errorf("exact match should download model.safetensors: %v", err)
	}

	// Unique substring — selects the non-"Model" config file, which now downloads
	// with no flag.
	d2 := newDLServer(t, false, files)
	setupDownloadEnv(t, d2, "")
	if _, _, err := run(t, "download", "128713", "--file", "config"); err != nil {
		t.Fatalf("substring --file: %v", err)
	}
	if _, err := os.Stat("config.yaml"); err != nil {
		t.Errorf("substring match should download config.yaml: %v", err)
	}
}

func TestDownloadFileAmbiguousErrors(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model-a.safetensors", typ: "Model", primary: true, body: "a"},
		{id: 2, name: "model-b.safetensors", typ: "Model", body: "b"},
	})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713", "--file", "safetensors")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if !strings.Contains(err.Error(), "model-a.safetensors") {
		t.Errorf("ambiguity error should list candidates: %v", err)
	}
}

func TestDownloadFileNoMatchErrors(t *testing.T) {
	d := newDLServer(t, false, []dlFile{{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "a"}})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713", "--file", "nope")
	if err == nil || !strings.Contains(err.Error(), "no file matches") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestDownloadAll(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
		{id: 2, name: "vae.pt", typ: "VAE", body: "vae", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	dir := t.TempDir()
	if _, _, err := run(t, "download", "128713", "--all", "--out-dir", dir); err != nil {
		t.Fatalf("download --all: %v", err)
	}
	for _, name := range []string{"model.safetensors", "vae.pt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("--all should write %s: %v", name, err)
		}
	}
}

// --all now downloads EVERY file regardless of type — the weights AND the
// non-"Model" file — with no skip and no override flag.
func TestDownloadAllDownloadsEveryFileType(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "model.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
		{id: 2, name: "training.zip", typ: "Training Data", body: "trainingdata"},
	})
	setupDownloadEnv(t, d, "")
	dir := t.TempDir()
	_, errOut, err := run(t, "download", "128713", "--all", "--out-dir", dir)
	if err != nil {
		t.Fatalf("download --all: %v", err)
	}
	if _, e := os.Stat(filepath.Join(dir, "model.safetensors")); e != nil {
		t.Errorf("--all should fetch the weights: %v", e)
	}
	if _, e := os.Stat(filepath.Join(dir, "training.zip")); e != nil {
		t.Errorf("--all should now fetch the non-model training file too: %v", e)
	}
	if strings.Contains(errOut, "not model weights") {
		t.Errorf("stderr must NOT note a non-weights skip anymore: %s", errOut)
	}
}

func TestDownloadSHA256Verifies(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "good-weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	out, _, err := run(t, "download", "128713")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if !strings.Contains(out, "SHA256 verified") {
		t.Errorf("should report verification: %s", out)
	}
}

func TestDownloadSHA256MismatchDeletesPartAndErrors(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "some-bytes", badSHA: true},
	})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713")
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("expected SHA256 mismatch error, got %v", err)
	}
	if _, e := os.Stat("m.safetensors.part"); e == nil {
		t.Error("mismatch must delete the .part file")
	}
	if _, e := os.Stat("m.safetensors"); e == nil {
		t.Error("mismatch must not leave a final file")
	}
}

func TestDownloadMissingHashWarnsAndSkipsVerify(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "bytes"}, // no withSHA
	})
	setupDownloadEnv(t, d, "")
	out, errOut, err := run(t, "download", "128713")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if _, e := os.Stat("m.safetensors"); e != nil {
		t.Errorf("file should still download without a hash: %v", e)
	}
	if !strings.Contains(errOut, "no SHA256 published") {
		t.Errorf("should warn about the missing hash: %s", errOut)
	}
	if strings.Contains(out, "SHA256 verified") {
		t.Errorf("should NOT claim verification when no hash exists: %s", out)
	}
}

// Repro (a): a version whose PRIMARY file is a non-"Model" Archive — as a
// "Workflows" model registers — downloads with NO flag and NO refusal.
func TestDownloadArchivePrimaryDownloads(t *testing.T) {
	const body = "WORKFLOW-ARCHIVE-BYTES"
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "workflow.zip", typ: "Archive", primary: true, body: body, withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	out, _, err := run(t, "download", "3083777")
	if err != nil {
		t.Fatalf("an Archive primary file should download with no flag, got %v", err)
	}
	got, rerr := os.ReadFile("workflow.zip")
	if rerr != nil {
		t.Fatalf("Archive file not written: %v", rerr)
	}
	if string(got) != body {
		t.Errorf("content = %q, want %q", got, body)
	}
	if !strings.Contains(out, "Saved workflow.zip") {
		t.Errorf("output should confirm the save: %s", out)
	}
}

// A non-"Model" Training Data primary file also downloads with no flag.
func TestDownloadTrainingDataPrimaryDownloads(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "training.zip", typ: "Training Data", primary: true, body: "trainingdata"},
	})
	setupDownloadEnv(t, d, "")
	if _, _, err := run(t, "download", "128713"); err != nil {
		t.Fatalf("a Training Data file should download with no flag, got %v", err)
	}
	if _, e := os.Stat("training.zip"); e != nil {
		t.Errorf("the training file should be written: %v", e)
	}
}

func TestDownloadOutPath(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "weights", withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	target := filepath.Join(t.TempDir(), "nested", "custom-name.safetensors")
	if _, _, err := run(t, "download", "128713", "--out", target); err != nil {
		t.Fatalf("download --out: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("--out should create the target (and parent dirs): %v", err)
	}
}

func TestDownloadOutRejectedWithAll(t *testing.T) {
	d := newDLServer(t, false, []dlFile{{id: 1, name: "m", typ: "Model", primary: true, body: "x"}})
	setupDownloadEnv(t, d, "")
	_, _, err := run(t, "download", "128713", "--all", "--out", "x")
	if err == nil || !strings.Contains(err.Error(), "--out") {
		t.Fatalf("expected --out/--all conflict, got %v", err)
	}
}

func TestDownloadFollowsRedirect(t *testing.T) {
	const body = "STORAGE-BYTES"
	d := newDLServer(t, true, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: body, withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	if _, _, err := run(t, "download", "128713"); err != nil {
		t.Fatalf("download with redirect: %v", err)
	}
	got, err := os.ReadFile("m.safetensors")
	if err != nil || string(got) != body {
		t.Errorf("redirected download content = %q (err %v), want %q", got, err, body)
	}
}

func TestDownloadSendsAuthWhenTokenConfigured(t *testing.T) {
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: "w", withSHA: true},
	})
	setupDownloadEnv(t, d, "my-key")
	if _, _, err := run(t, "download", "128713"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if d.lastAuth != "Bearer my-key" {
		t.Errorf("download Authorization = %q, want Bearer my-key", d.lastAuth)
	}
}

func TestDownloadSkipsWhenPresentUnlessForce(t *testing.T) {
	const body = "weights-body"
	d := newDLServer(t, false, []dlFile{
		{id: 1, name: "m.safetensors", typ: "Model", primary: true, body: body, withSHA: true},
	})
	setupDownloadEnv(t, d, "")
	// First download.
	if _, _, err := run(t, "download", "128713"); err != nil {
		t.Fatalf("first download: %v", err)
	}
	hits := d.dlHits
	// Second run: present + SHA matches => skip (no new download hit).
	out, _, err := run(t, "download", "128713")
	if err != nil {
		t.Fatalf("second download: %v", err)
	}
	if !strings.Contains(out, "already present") {
		t.Errorf("second run should skip: %s", out)
	}
	if d.dlHits != hits {
		t.Errorf("skip should not re-fetch: hits %d -> %d", hits, d.dlHits)
	}
	// --force re-downloads.
	if _, _, err := run(t, "download", "128713", "--force"); err != nil {
		t.Fatalf("force download: %v", err)
	}
	if d.dlHits != hits+1 {
		t.Errorf("--force should re-fetch: hits=%d want %d", d.dlHits, hits+1)
	}
}

func TestDownload401ActionableError(t *testing.T) {
	// A server that 401s the download route.
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SD 1.5","files":[{"id":1,"name":"gated.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/gated"}]}`, base)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "128713")
	if err == nil || !strings.Contains(err.Error(), "requires authentication") {
		t.Fatalf("expected actionable 401 error, got %v", err)
	}
	if !strings.Contains(err.Error(), "civitai login") {
		t.Errorf("401 error should point at `civitai login`: %v", err)
	}
}

// A 403 while authenticated means the file is gated (early-access / subscriber /
// not shared), NOT a login problem — the message must say so and must NOT tell
// the user to log in again.
func TestDownload403GatedError(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":"SD 1.5","files":[{"id":1,"name":"gated.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/gated"}]}`, base)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	srv := httptest.NewServer(mux)
	base = srv.URL
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "a-valid-token") // authed, but the file is gated
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	_, _, err := run(t, "download", "128713")
	if err == nil || !strings.Contains(err.Error(), "gated") {
		t.Fatalf("expected a gated/forbidden 403 error, got %v", err)
	}
	if strings.Contains(err.Error(), "civitai login") {
		t.Errorf("a 403 (authed but forbidden) must NOT tell the user to run login: %v", err)
	}
}
