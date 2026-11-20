package cmd

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// generate_no_download_stdout_test.go pins what `generate --no-download` (and
// the graph-only `--print-input`) put on stdout, and what they exit with, for
// three community reports:
//
//   - civitai/cli#935: `--no-download --json` wrote the numbered URL row to
//     stdout BEFORE the workflow document, so the stream did not decode as one
//     JSON document.
//   - civitai/cli#943: `--print-input --json` was refused (exit 2) with "--json
//     without --dry-run submits and spends Buzz", although --print-input never
//     estimates or submits.
//   - civitai/cli#945: `--no-download` on a run whose every kept output came back
//     with no URL printed zero rows, told the caller to fetch "These URLs", and
//     exited 0.

// noDownloadWorkflow is a succeeded workflow whose single step carries the given
// image blobs (raw JSON objects).
func noDownloadWorkflow(images ...string) string {
	return `{"id":"wf_nodl","status":"succeeded","steps":[{"$type":"textToImage","name":"s","status":"succeeded","metadata":{},` +
		`"output":{"images":[` + strings.Join(images, ",") + `]}}]}`
}

// runNoDownload drives the full wait path with --no-download against a scripted
// terminal workflow, returning stdout, stderr and the error.
func runNoDownload(t *testing.T, jsonOut bool, payload string) (string, string, error) {
	t.Helper()
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.downloadBlob = presignedFetcher(t, srv.URL, "")

	o := waitOpts(dir)
	o.noDownload = true
	o.jsonOut = jsonOut

	c, out, errb := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if len(seen) != 0 {
		t.Errorf("--no-download fetched %d blob(s), want 0", len(seen))
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "*")); len(entries) != 0 {
		t.Errorf("--no-download wrote %v", entries)
	}
	return out.String(), errb.String(), err
}

// TestGenerate_NoDownloadJSONStdoutIsOneDocument is the #935 regression.
func TestGenerate_NoDownloadJSONStdoutIsOneDocument(t *testing.T) {
	const url = "https://example.invalid/o/only.jpeg"
	stdout, stderr, err := runNoDownload(t, true,
		noDownloadWorkflow(`{"id":"only","type":"image","available":true,"url":"`+url+`"}`))
	if err != nil {
		t.Fatalf("generate --no-download --json: %v", err)
	}

	// The WHOLE stream must decode as one document — json.Unmarshal rejects
	// trailing or leading non-JSON, which is exactly the defect.
	var doc struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("--no-download --json stdout is not ONE JSON document (%v):\n%q", err, stdout)
	}
	if doc.ID != "wf_nodl" || doc.Status != "succeeded" {
		t.Errorf("stdout document = {id:%q status:%q}, want {id:\"wf_nodl\" status:\"succeeded\"}", doc.ID, doc.Status)
	}
	if !strings.Contains(stdout, url) {
		t.Errorf("the workflow document on stdout must still carry the output URL %q:\n%s", url, stdout)
	}
	// The row is not dropped, it moves: stderr carries it, literally.
	if !strings.Contains(stderr, "1\t"+url+"\n") {
		t.Errorf("with --json the URL row belongs on STDERR, want %q in:\n%q", "1\t"+url+"\n", stderr)
	}
}

// TestGenerate_NoDownloadTextRowsStayOnStdout is an INVARIANT guard (green
// before and after #935): text mode keeps its row on stdout.
func TestGenerate_NoDownloadTextRowsStayOnStdout(t *testing.T) {
	const url = "https://example.invalid/o/only.jpeg"
	stdout, _, err := runNoDownload(t, false,
		noDownloadWorkflow(`{"id":"only","type":"image","available":true,"url":"`+url+`"}`))
	if err != nil {
		t.Fatalf("generate --no-download: %v", err)
	}
	if stdout != "1\t"+url+"\n" {
		t.Errorf("text-mode stdout = %q, want %q", stdout, "1\t"+url+"\n")
	}
}

// TestGenerate_NoDownloadNoURLsIsAnError is the #945 regression: every kept
// output arrived without a URL (one null, one omitted).
func TestGenerate_NoDownloadNoURLsIsAnError(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		stdout, stderr, err := runNoDownload(t, jsonOut, noDownloadWorkflow(
			`{"id":"nullurl","type":"image","available":true,"url":null}`,
			`{"id":"nourl","type":"image","available":true}`))
		if err == nil {
			t.Fatalf("json=%v: zero printable URLs exited 0 — a script checking only the exit status proceeds with nothing", jsonOut)
		}
		// Exit 1, the same as the download path's "carries no URL": the shared
		// item-7 sentinel-absence pin. The PROCESS exit status itself is pinned
		// end to end by cmd/civitai's TestGenerateNoDownloadNoURLsExitsOne.
		assertGenericExitCode(t, err)
		// wf_123 is the id the SUBMIT returned (genSeams' default reply) — the one
		// the CLI tracks — deliberately distinct from the polled document's
		// wf_nodl so the assertion shows which id is named.
		const want = "the generation succeeded but none of its 2 deliverable output(s) came back with a URL, so there is nothing to print — it was charged; inspect it with `civitai workflows get wf_123` (add --json for the machine-readable record) (" + noFailureReasonNote + ")"
		if err.Error() != want {
			t.Errorf("json=%v: error = %q\nwant      %q", jsonOut, err.Error(), want)
		}
		if !strings.Contains(err.Error(), "charged") {
			t.Errorf("json=%v: the error must say the run was charged, or a retry-on-exit-1 script pays again: %q", jsonOut, err.Error())
		}
		// Both modes: stdout empty. Under --json that is the failure contract —
		// the error on stderr, nothing on stdout — not a missing document.
		if stdout != "" {
			t.Errorf("json=%v: stdout must be empty when nothing is printable, got %q", jsonOut, stdout)
		}
		if strings.Contains(stderr, "presigned") {
			t.Errorf("json=%v: the expiry hint names URLs that were never printed:\n%s", jsonOut, stderr)
		}
	}
}

// TestGenerate_NoDownloadMixedURLsStillSucceeds is an INVARIANT guard for the
// #945 fix's boundary: one URL-less output beside one with a URL still prints
// the existing row (numbered 2) and the hint, and exits 0.
func TestGenerate_NoDownloadMixedURLsStillSucceeds(t *testing.T) {
	const url = "https://example.invalid/o/second.jpeg"
	stdout, stderr, err := runNoDownload(t, false, noDownloadWorkflow(
		`{"id":"first","type":"image","available":true,"url":null}`,
		`{"id":"second","type":"image","available":true,"url":"`+url+`"}`))
	if err != nil {
		t.Fatalf("a run with one printable URL must exit 0, got %v", err)
	}
	if stdout != "2\t"+url+"\n" {
		t.Errorf("stdout = %q, want %q", stdout, "2\t"+url+"\n")
	}
	if !strings.Contains(stderr, "These URLs are presigned and expire") {
		t.Errorf("a printed URL keeps its expiry hint:\n%s", stderr)
	}
}

// TestGeneratePrintInput_AcceptsJSON is the #943 regression: --print-input
// already emits JSON, never estimates and never submits, so --json alongside it
// is accepted without --yes and prints the same single graph, offline.
func TestGeneratePrintInput_AcceptsJSON(t *testing.T) {
	r := runGenerateCLI(t, "", "a cube", "--print-input", "--json")
	if r.err != nil {
		t.Fatalf("--print-input --json must succeed like --print-input alone, got %v (usage=%v)",
			r.err, errors.Is(r.err, ErrUsage))
	}
	assertOffline(t, r)
	var m map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, r.stdout)
	}
	if m["workflow"] != "txt2img" || m["prompt"] != "a cube" {
		t.Errorf("graph = {workflow:%v prompt:%v}, want {workflow:txt2img prompt:\"a cube\"}", m["workflow"], m["prompt"])
	}

	// CONTROL: the spend refusal itself still fires for a real submit with
	// --json and no --yes, so the narrowing is to --print-input only.
	r = runGenerateCLI(t, "fixture-token", "a cube", "--json")
	if !errors.Is(r.err, ErrUsage) || !strings.Contains(r.err.Error(), "--json without --dry-run submits and spends Buzz") {
		t.Fatalf("--json without --yes on a submitting run must still be refused as usage, got %v", r.err)
	}
	assertOffline(t, r)
}
