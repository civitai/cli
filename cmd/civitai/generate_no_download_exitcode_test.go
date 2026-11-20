package main

// THE PUBLISHED EXIT CODE FOR `generate --no-download` WHEN NO OUTPUT HAS A URL
// (civitai/cli#945).
//
// internal/cmd's TestGenerate_NoDownloadNoURLsIsAnError pins the error's
// CLASSIFICATION and text; this pins the PROCESS exit status a script sees, by
// building the real binary and driving it against a loopback tRPC fixture. No
// request leaves the loopback server, and nothing real is submitted or charged.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGenerateNoDownloadNoURLsExitsOne(t *testing.T) {
	bin := buildCLI(t)

	workflow := `{"id":"wf_e2e","status":"succeeded","steps":[{"$type":"textToImage","name":"s","status":"succeeded",` +
		`"output":{"images":[{"id":"a","type":"image","available":true,"url":null}]}}]}`
	routes := map[string]string{
		"GET /api/trpc/orchestrator.whatIfFromGraph":    `{"ready":true,"cost":{"total":8}}`,
		"GET /api/trpc/buzz.getBuzzAccount":             `{"blue":100}`,
		"POST /api/trpc/orchestrator.generateFromGraph": `{"id":"wf_e2e","status":"processing","cost":{"total":8}}`,
		"GET /api/trpc/orchestrator.getWorkflow":        workflow,
	}
	var mu sync.Mutex
	var media int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			mu.Lock()
			media++
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"data":{"json":`+body+`}}}`)
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"text", []string{"generate", "a cube", "--yes", "--no-download", "--no-update-check"}},
		{"json", []string{"generate", "a cube", "--yes", "--no-download", "--json", "--no-update-check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			env := []string{
				"HOME=" + home,
				"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
				"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
				"CIVITAI_BASE_URL=" + srv.URL,
				"CIVITAI_TOKEN=fixture-only",
				"NO_COLOR=1",
			}
			rc, stdout, stderr := runCLIAnyStatus(t, bin, env, tc.args...)
			if rc != 1 {
				t.Fatalf("exit = %d, want 1\nstdout: %s\nstderr: %s", rc, stdout, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout must be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, "it was charged") || !strings.Contains(stderr, "civitai workflows get wf_e2e") {
				t.Errorf("stderr must say the run was charged and name the workflow:\n%s", stderr)
			}
			if strings.Contains(stderr, "presigned") {
				t.Errorf("no URL was printed, so no expiry hint:\n%s", stderr)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if media != 0 {
		t.Errorf("%d request(s) hit an unscripted route (a media fetch or an unexpected call)", media)
	}
	// Fixture premise: the workflow payload is well-formed JSON.
	if !json.Valid([]byte(workflow)) {
		t.Fatal("CONTROL failure: fixture workflow is not valid JSON")
	}
}
