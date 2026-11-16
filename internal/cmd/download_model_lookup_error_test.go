package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// Issue #878: a bare positional id is looked up as BOTH a version and a model so
// that an id which is both can STOP for disambiguation instead of downloading an
// unrelated model's version. The model lookup used to collapse EVERY failure to
// "not a model", so a throttled (429), failing (5xx) or unreachable model lookup
// read as "version-only" and the download proceeded with no ambiguity check at
// all. Only a genuine 404 means "not a model"; anything else must stop the
// command.

// lookupServer answers /api/v1/model-versions/{id} with versionStatus and
// /api/v1/models/{id} with modelStatus. Both ids are 4384 so a 200 from each is
// the ambiguous shape. dlHits counts requests to the file route.
type lookupServer struct {
	srv    *httptest.Server
	dlHits atomic.Int32
}

func newLookupServer(t *testing.T, versionStatus, modelStatus int) *lookupServer {
	t.Helper()
	ls := &lookupServer{}
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		if versionStatus != http.StatusOK {
			w.WriteHeader(versionStatus)
			fmt.Fprint(w, `{"error":"lookup refused"}`)
			return
		}
		fmt.Fprintf(w, `{"id":4384,"modelId":777,"name":"v","baseModel":"SD 1.5","model":{"name":"Other Model"},"files":[{"id":1,"name":"w.safetensors","type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/w.safetensors"}]}`, base)
	})
	mux.HandleFunc("/api/v1/models/", func(w http.ResponseWriter, r *http.Request) {
		if modelStatus != http.StatusOK {
			// No Retry-After: a header-less 429 is terminal for reads, so the
			// test does not sit in the retry backoff.
			w.WriteHeader(modelStatus)
			fmt.Fprint(w, `{"error":"lookup refused"}`)
			return
		}
		fmt.Fprint(w, `{"id":4384,"name":"Pasted Model","type":"Checkpoint","modelVersions":[{"id":9999,"name":"v1","baseModel":"SD 1.5","files":[]}]}`)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		ls.dlHits.Add(1)
		_, _ = w.Write([]byte("weights"))
	})
	ls.srv = httptest.NewServer(mux)
	base = ls.srv.URL
	t.Cleanup(ls.srv.Close)

	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", ls.srv.URL)
	chdir(t, t.TempDir())
	return ls
}

const dryRunPlanMarker = "Dry run — planning"

// TestDownloadModelLookupFailureStopsBeforePlan is the #878 regression: the
// version lookup succeeds, the model lookup fails with something other than a
// 404, and the command must exit non-zero without printing a plan.
func TestDownloadModelLookupFailureStopsBeforePlan(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantKind error // nil: the status carries no classification sentinel
	}{
		{"429 rate limited", http.StatusTooManyRequests, civitai.ErrRateLimited},
		{"500 server error", http.StatusInternalServerError, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newLookupServer(t, http.StatusOK, tc.status)
			out, _, err := run(t, "download", "4384", "--dry-run")
			if err == nil {
				t.Fatalf("model lookup answered %d, yet the command succeeded (read as version-only):\n%s", tc.status, out)
			}
			if strings.Contains(out, dryRunPlanMarker) {
				t.Errorf("a plan was printed although the ambiguity check never ran:\n%s", out)
			}
			if tc.wantKind != nil && !errors.Is(err, tc.wantKind) {
				t.Errorf("error lost its classification: errors.Is(%v) = false for %v", tc.wantKind, err)
			}
			if !strings.Contains(err.Error(), "4384") || !strings.Contains(err.Error(), "model id") {
				t.Errorf("error should say the model-id check for 4384 failed: %v", err)
			}
		})
	}
}

// TestDownloadModelLookupFailureDownloadsNothing drives the real (non dry-run)
// path: a 429 on the model lookup must not reach the file route.
func TestDownloadModelLookupFailureDownloadsNothing(t *testing.T) {
	ls := newLookupServer(t, http.StatusOK, http.StatusTooManyRequests)
	if _, _, err := run(t, "download", "4384"); err == nil {
		t.Fatal("model lookup answered 429, yet the download succeeded")
	}
	if n := ls.dlHits.Load(); n != 0 {
		t.Errorf("file route was requested %d time(s); want 0", n)
	}
}

// TestDownloadModelLookupFailureAfterVersion404 covers the other caller: the
// version lookup 404s and the model lookup is throttled. The throttle is the
// real problem and must surface as such, not as the version's not-found.
func TestDownloadModelLookupFailureAfterVersion404(t *testing.T) {
	newLookupServer(t, http.StatusNotFound, http.StatusTooManyRequests)
	_, _, err := run(t, "download", "4384", "--dry-run")
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, civitai.ErrRateLimited) {
		t.Errorf("want the model lookup's 429 (ErrRateLimited), got %v", err)
	}
	if errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("a throttled model lookup was reported as not-found: %v", err)
	}
}

// TestDownloadModelLookupFailureYesProceeds: with --yes, a model check that
// fails for a reason other than 404 does not stop the download (operator
// decision on #878). The typed VERSION is downloaded, and a note says the
// model check failed and why, so the user knows the ambiguity check did not
// run.
func TestDownloadModelLookupFailureYesProceeds(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reason string // a fragment of the failure the note must carry
	}{
		{"429 rate limited", http.StatusTooManyRequests, "rate limited"},
		{"500 server error", http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ls := newLookupServer(t, http.StatusOK, tc.status)
			_, errOut, err := run(t, "download", "4384", "--yes")
			if err != nil {
				t.Fatalf("--yes with a failed model check (%d) should download the typed version, got: %v", tc.status, err)
			}
			got, rerr := os.ReadFile("w.safetensors")
			if rerr != nil || string(got) != "weights" {
				t.Fatalf("version file not downloaded: %v (content %q)", rerr, got)
			}
			if n := ls.dlHits.Load(); n != 1 {
				t.Errorf("file route requested %d time(s); want 1", n)
			}
			for _, want := range []string{"model check failed: ", tc.reason, "downloading version 4384 as requested"} {
				if !strings.Contains(errOut, want) {
					t.Errorf("stderr note should contain %q:\n%s", want, errOut)
				}
			}
		})
	}
}
