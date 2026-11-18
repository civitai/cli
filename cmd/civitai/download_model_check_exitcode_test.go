package main

// THE EXIT CODE WHEN `civitai download <id>`'s MODEL-ID CHECK FAILS ON THE
// NETWORK (#878).
//
// A bare positional id is looked up as a version AND as a model, so an id that
// is both stops for disambiguation. When the model lookup fails for any reason
// other than a 404, the check cannot run: without --yes the command stops, and
// with --yes it proceeds with the typed version and says the check failed.
// internal/cmd pins that for 429 and 500. This file pins the network half by
// running the real binary: a 503 (ErrNetwork) and a dropped connection (a raw
// transport error) must stop with exit 5 without --yes, and proceed under --yes.
//
// --yes rows use --dry-run: the binary's download guard refuses this loopback
// http test server's file URL, and the plan is enough to show the command
// proceeded past the model check.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadModelCheckNetworkFailureExitCodes(t *testing.T) {
	bin := buildCLI(t)

	rows := []struct {
		name   string
		status int // transportFailure: drop the connection instead of answering
		reason string
	}{
		{"503 service unavailable", http.StatusServiceUnavailable, "503"},
		{"dropped connection", transportFailure, "EOF"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var base string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"id":4384,"modelId":777,"name":"v","baseModel":"SD 1.5","model":{"name":"Other"},"files":[{"id":1,"name":"w.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/w.safetensors"}]}`, base)
			})
			mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
				if row.status == transportFailure {
					breakConnection(t, w)
					return
				}
				w.WriteHeader(row.status)
				fmt.Fprint(w, `{"error":"unavailable"}`)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()
			base = srv.URL
			env := func() []string {
				return []string{
					"XDG_CONFIG_HOME=" + filepath.Join(t.TempDir(), "config"),
					"CIVITAI_TOKEN=",
					"CIVITAI_BASE_URL=" + srv.URL,
					"CIVITAI_NO_UPDATE_CHECK=1",
				}
			}

			rc, stdout, stderr := runCLIAnyStatus(t, bin, env(), "download", "4384", "--dry-run")
			if rc != exitNetwork {
				t.Errorf("without --yes: exit %d, want %d (network)\nstderr: %s", rc, exitNetwork, stderr)
			}
			if strings.Contains(stdout, "Dry run") {
				t.Errorf("without --yes a plan was printed although the model check failed:\n%s", stdout)
			}
			if !strings.Contains(stderr, "checking whether it is also a model id failed") {
				t.Errorf("without --yes the error should say the model check failed:\n%s", stderr)
			}

			rc, stdout, stderr = runCLIAnyStatus(t, bin, env(), "download", "4384", "--dry-run", "--yes")
			if rc != exitOK {
				t.Errorf("with --yes: exit %d, want 0\nstderr: %s", rc, stderr)
			}
			if !strings.Contains(stdout, "Dry run") || !strings.Contains(stdout, "w.safetensors") {
				t.Errorf("with --yes the typed version should be planned:\n%s", stdout)
			}
			for _, want := range []string{"model check failed: ", row.reason, "downloading version 4384 as requested"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("with --yes the stderr note should contain %q:\n%s", want, stderr)
				}
			}
		})
	}
}
