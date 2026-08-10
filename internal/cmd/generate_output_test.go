package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// blobServer serves image bytes for any path and records the Authorization
// header of every request it sees.
func blobServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, "image-bytes-"+filepath.Base(r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// presignedFetcher wires the real credential-free downloader at a loopback
// httptest server (plain http, so private hosts must be allowed).
func presignedFetcher(t *testing.T, baseURL, token string) blobFetcher {
	t.Helper()
	c := civitai.New(baseURL, token)
	c.AllowPrivateDownloadHosts = true
	return c.DownloadPresigned
}

// succeededWorkflow builds a succeeded workflow whose step yields n available
// image outputs served by srv.
func succeededWorkflow(srvURL string, n int) *genapi.Workflow {
	imgs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		imgs = append(imgs, `{"id":"blob`+string(rune('a'+i))+`","type":"image","available":true,"url":"`+srvURL+`/o/`+string(rune('a'+i))+`.jpeg"}`)
	}
	payload := `{"id":"wf_123","status":"succeeded","steps":[{"$type":"textToImage","name":"s","status":"succeeded","metadata":{},"output":{"images":[` +
		strings.Join(imgs, ",") + `]}}]}`
	var wf genapi.Workflow
	if err := json.Unmarshal([]byte(payload), &wf); err != nil {
		panic(err)
	}
	return &wf
}

// --- 🔴 the credential guard, at the COMMAND layer ---------------------------

// The command's blob fetcher must reach the blob host with NO Authorization
// header — paired, in this same test, with a positive control proving the
// recorder can see one.
//
// The hazardous configuration is reproduced deliberately: the token is
// non-empty and the base URL EQUALS the blob host, so the trusted-host
// predicate would attach the key if the wrong seam were wired.
func TestDownloadOutputs_SendsNoAuthorizationHeader(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	wf := succeededWorkflow(srv.URL, 2)
	kept, _ := genapi.PartitionOutputs(wf)
	if len(kept) != 2 {
		t.Fatalf("fixture: %d deliverable outputs, want 2", len(kept))
	}

	paths, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, "secret-api-key"),
		io.Discard, io.Discard, "wf_123", kept, dir, "", false)
	if err != nil {
		t.Fatalf("downloadOutputs: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("wrote %d files, want 2", len(paths))
	}
	if len(seen) != 2 {
		t.Fatalf("the server saw %d requests, want 2", len(seen))
	}
	for i, h := range seen {
		if h != "" {
			t.Errorf("blob request %d carried a credential: Authorization = %q", i, h)
		}
	}

	// POSITIVE CONTROL: the SAME recorder, the SAME server, the SAME token —
	// via the token-attaching path. Without this, "0 auth headers" is
	// indistinguishable from a recorder wired to nothing.
	c := civitai.New(srv.URL, "secret-api-key")
	c.AllowPrivateDownloadHosts = true
	resp, err := c.DownloadFile(context.Background(), srv.URL+"/o/a.jpeg")
	if err != nil {
		t.Fatalf("positive control fetch: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if len(seen) != 3 {
		t.Fatalf("the server saw %d requests after the control, want 3", len(seen))
	}
	if !strings.Contains(seen[2], "Bearer secret-api-key") {
		t.Fatalf("POSITIVE CONTROL FAILED: the authenticated fetch sent Authorization = %q — "+
			"the recorder cannot observe the header, so the empty values above prove nothing", seen[2])
	}
}

// --- download behaviour ------------------------------------------------------

func TestDownloadOutputs_HappyPathNamesFilesByWorkflowAndIndex(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 3))

	var out bytes.Buffer
	paths, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""), &out, io.Discard, "wf_123", kept, dir, "", false)
	if err != nil {
		t.Fatalf("downloadOutputs: %v", err)
	}
	want := []string{"wf_123-1.jpeg", "wf_123-2.jpeg", "wf_123-3.jpeg"}
	for i, p := range paths {
		if filepath.Base(p) != want[i] {
			t.Errorf("file %d = %s, want %s", i, filepath.Base(p), want[i])
		}
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s not on disk: %v", p, err)
		}
		if _, err := os.Stat(p + ".part"); !os.IsNotExist(err) {
			t.Errorf("a .part survived next to %s", p)
		}
	}
	if !strings.Contains(out.String(), "Saved") {
		t.Errorf("no save line printed: %q", out.String())
	}
}

// A collision must be refused BEFORE any bytes move — a presigned URL is
// expiring while the command dithers, and a half-done overwrite is worse than a
// clean refusal.
func TestDownloadOutputs_CollisionWithoutForceRefusesBeforeAnyTransfer(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 2))

	// Pre-create the SECOND target, so a per-file loop would already have
	// fetched the first before noticing.
	existing := filepath.Join(dir, "wf_123-2.jpeg")
	if err := os.WriteFile(existing, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""), io.Discard, io.Discard, "wf_123", kept, dir, "", false)
	if err == nil {
		t.Fatal("collision without --force: want a refusal, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal must name --force: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("the collision check ran AFTER %d transfer(s) — it must run first", len(seen))
	}
	if b, _ := os.ReadFile(existing); string(b) != "keep me" {
		t.Errorf("the existing file was modified: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "wf_123-1.jpeg")); !os.IsNotExist(err) {
		t.Error("no file should have been written")
	}
}

func TestDownloadOutputs_ForceOverwrites(t *testing.T) {
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()
	kept, _ := genapi.PartitionOutputs(succeededWorkflow(srv.URL, 1))

	target := filepath.Join(dir, "wf_123-1.jpeg")
	if err := os.WriteFile(target, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := downloadOutputs(context.Background(), presignedFetcher(t, srv.URL, ""), io.Discard, io.Discard, "wf_123", kept, dir, "", true); err != nil {
		t.Fatalf("--force: %v", err)
	}
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "stale" {
		t.Error("--force did not overwrite the existing file")
	}
	if !strings.HasPrefix(string(b), "image-bytes") {
		t.Errorf("unexpected content %q", b)
	}
}

// Cancelling mid-transfer (what Ctrl-C does through signal.NotifyContext) must
// leave neither a truncated final file nor a stray .part.
func TestDownloadBlobTo_CancelCleansThePartFile(t *testing.T) {
	streaming := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "partial")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(streaming)
		<-r.Context().Done()
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "wf_123-1.jpeg")
	fetch := presignedFetcher(t, srv.URL, "")

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- downloadBlobTo(ctx, fetch, io.Discard, io.Discard, srv.URL+"/o/a.jpeg", target, false)
	}()

	<-streaming
	cancel()

	if err := <-errc; err == nil {
		t.Fatal("expected a cancellation error from the interrupted transfer")
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Errorf(".part should be removed on cancel; stat err = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("no final file should exist on cancel; stat err = %v", err)
	}
}

// A non-2xx blob fetch must point at the EXPIRY remedy, not at `civitai login` —
// a presigned URL is not an auth problem.
func TestBlobStatusError_PointsAtTheExpiryRemedy(t *testing.T) {
	err := blobStatusError(http.StatusForbidden, "wf_123-1.jpeg")
	if err == nil {
		t.Fatal("403 must be an error")
	}
	if !strings.Contains(err.Error(), "expire") || !strings.Contains(err.Error(), "civitai workflows get") {
		t.Errorf("403 remedy = %v", err)
	}
	if strings.Contains(err.Error(), "civitai login") {
		t.Errorf("a presigned-URL 403 must not send the user to login: %v", err)
	}
	if blobStatusError(http.StatusOK, "x") != nil {
		t.Error("200 must not be an error")
	}
}

// The workflow id is server-supplied, so it must not be able to steer the write.
func TestRenderOutName_RejectsAHostileWorkflowId(t *testing.T) {
	if _, err := renderOutName("", "/", 1, "https://x/a.jpeg"); err == nil {
		t.Error("a degenerate workflow id must be rejected")
	}
	got, err := renderOutName("", "../../etc/passwd", 1, "https://x/a.jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.ContainsRune(got, filepath.Separator) {
		t.Errorf("a traversal in the workflow id survived into the filename: %q", got)
	}
	if got != "passwd-1.jpeg" {
		t.Errorf("filename = %q, want passwd-1.jpeg", got)
	}
}

func TestBlobExtension(t *testing.T) {
	cases := map[string]string{
		"https://x/a.jpeg?sig=1":     ".jpeg",
		"https://x/a.PNG":            ".png",
		"https://x/a":                ".bin",
		"https://x/a.":               ".bin",
		"https://x/a.verylongextens": ".bin",
		"://nonsense":                ".bin",
	}
	for in, want := range cases {
		if got := blobExtension(in); got != want {
			t.Errorf("blobExtension(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- filtering reported at the command layer ---------------------------------

// A succeeded workflow with blocked/unavailable/hidden outputs must download
// only the deliverable ones AND say so, with the count mismatch called out.
func TestGenerate_FilteredOutputsAreReportedAndOnlyDeliverablesDownload(t *testing.T) {
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	payload := `{"id":"wf_123","status":"succeeded","steps":[{
	  "$type":"textToImage","name":"s","status":"succeeded",
	  "metadata":{"output":{"c":{"hidden":true}}},
	  "output":{"images":[
	    {"id":"a","type":"image","available":true,"url":"` + srv.URL + `/o/a.jpeg"},
	    {"id":"b","type":"image","available":true,"url":"` + srv.URL + `/o/b.jpeg","blockedReason":"minor"},
	    {"id":"c","type":"image","available":true,"url":"` + srv.URL + `/o/c.jpeg"},
	    {"id":"d","type":"image","available":false,"url":null}
	  ]}}]}`

	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.downloadBlob = presignedFetcher(t, srv.URL, "tok")

	o := waitOpts(dir)
	o.quantity, o.quantitySet = 4, true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Exactly ONE file, from the one deliverable output.
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Base(entries[0]) != "wf_123-1.jpeg" {
		t.Fatalf("files written = %v, want exactly [wf_123-1.jpeg]", entries)
	}
	if len(seen) != 1 {
		t.Errorf("the blob host saw %d requests, want 1 — a blocked/hidden/unavailable output must never be fetched", len(seen))
	}

	stderr := errb.String()
	// Each exclusion must be reported WITH its own reason.
	for _, want := range []string{"3 output(s) will NOT be saved", "moderation", "minor", "hidden", "not available"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// And the count gap must be stated explicitly.
	if !strings.Contains(stderr, "you asked for 4 output(s) and 1 are deliverable") {
		t.Errorf("the count mismatch was not reported:\n%s", stderr)
	}
	if !strings.Contains(out.String(), "wf_123-1.jpeg") {
		t.Errorf("stdout should name the saved file: %q", out.String())
	}
}

// POSITIVE CONTROL for the filter: with the SAME wiring and no excluded
// outputs, all four download and neither warning is printed. A test that only
// ever sees the filtered case cannot tell "the filter works" from "downloading
// is broken".
func TestGenerate_UnfilteredWorkflowDownloadsEverythingAndWarnsNothing(t *testing.T) {
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	wf := succeededWorkflow(srv.URL, 4)
	raw, _ := json.Marshal(wf)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, string(raw))
	s.downloadBlob = presignedFetcher(t, srv.URL, "tok")

	o := waitOpts(dir)
	o.quantity, o.quantitySet = 4, true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate: %v", err)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(entries) != 4 {
		t.Fatalf("POSITIVE CONTROL FAILED: %d files written, want 4 — if the happy path cannot write 4, the filtered case proves nothing", len(entries))
	}
	if len(seen) != 4 {
		t.Errorf("the blob host saw %d requests, want 4", len(seen))
	}
	if strings.Contains(errb.String(), "will NOT be saved") {
		t.Errorf("nothing was excluded, so no exclusion warning may print:\n%s", errb.String())
	}
	if strings.Contains(errb.String(), "are deliverable") {
		t.Errorf("the counts match, so no mismatch warning may print:\n%s", errb.String())
	}
}

// --no-download waits and prints the URLs without touching the disk.
func TestGenerate_NoDownloadPrintsURLsAndWritesNothing(t *testing.T) {
	withStdinTTY(t, false)
	var seen []string
	srv := blobServer(t, &seen)
	dir := t.TempDir()

	wf := succeededWorkflow(srv.URL, 2)
	raw, _ := json.Marshal(wf)
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, string(raw))

	o := waitOpts(dir)
	o.noDownload = true

	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("--no-download fetched %d blob(s), want 0", len(seen))
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(entries) != 0 {
		t.Errorf("--no-download wrote %v", entries)
	}
	if !strings.Contains(out.String(), srv.URL+"/o/a.jpeg") {
		t.Errorf("the output URLs should be on stdout: %q", out.String())
	}
}

// A succeeded workflow whose every output was filtered out must FAIL — reporting
// success with no files is the exact silent degradation the filter exists to
// surface.
func TestGenerate_AllOutputsFilteredIsAnError(t *testing.T) {
	withStdinTTY(t, false)
	payload := `{"id":"wf_123","status":"succeeded","steps":[{
	  "$type":"textToImage","name":"s","status":"succeeded","metadata":{},
	  "output":{"images":[{"id":"a","type":"image","available":true,"url":"https://x/a.jpeg","blockedReason":"minor"}]}}]}`
	clock := newFakeClock()
	calls := 0
	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	// Wire a fetcher that must never be called, rather than leaving it nil: a
	// nil seam turns "the filter regressed" into a panic instead of a readable
	// assertion failure.
	fetched := 0
	s.downloadBlob = func(ctx context.Context, u string) (*http.Response, error) {
		fetched++
		return nil, errors.New("no deliverable output should ever be fetched here")
	}

	c, _, errb := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("a workflow with no deliverable outputs must not report success")
	}
	if fetched != 0 {
		t.Errorf("a blocked output was fetched %d time(s) — the filter did not exclude it", fetched)
	}
	// This one KEEPS its charge claim. The workflow SUCCEEDED, so it ran and was
	// billed; nothing in #278 touches that half, and softening it here would
	// over-fix a true statement into a false negative on the one path where the
	// user demonstrably paid for something.
	if !strings.Contains(err.Error(), "charged") {
		t.Errorf("a succeeded workflow with no deliverable outputs was still charged; the error must say so: %v", err)
	}
	if !strings.Contains(errb.String(), "moderation") {
		t.Errorf("the reason must still be reported:\n%s", errb.String())
	}
}

// The per-output exclusion note may say the workflow was CHARGED, and may not
// say the excluded outputs are NOT REFUNDED.
//
// 🔴 The retracted half was "a blocked or missing output is not refunded". It is
// true of a hard block and false of the commonest soft one: a mature-content
// output is `canUpgrade`, where the server's own wording is "your Buzz is held,
// not lost" and the non-yellow debits are credited back on unlock
// (`civitai/civitai → src/server/services/orchestrator/poll-iteration.ts`,
// `src/components/ImageGeneration/QueueItem.tsx`). The payload does not let the
// CLI tell the two kinds apart, so it must settle neither — see #278 and
// AGENTS.md item 28.
//
// The assertion is PAIRED on purpose. The charge half must SURVIVE (it is not in
// doubt and dropping it would understate what happened), the refund half must be
// GONE, and both are checked against the same rendered line — a Contains on the
// new wording alone would pass with the retracted sentence printed beside it.
// It calls reportExcludedOutputs directly rather than asserting a literal from
// afar, so the guard reads the string the user actually sees.
func TestReportExcludedOutputs_DoesNotSettleTheRefund(t *testing.T) {
	blocked := "minor"
	id := "out_1"
	var b bytes.Buffer
	reportExcludedOutputs(&b, []genapi.Output{{Blob: genapi.Blob{ID: id, BlockedReason: &blocked}}})
	got := b.String()

	if got == "" {
		t.Fatal("CONTROL failure: reportExcludedOutputs printed nothing, so every assertion below is vacuous")
	}
	if !strings.Contains(got, "charged") {
		t.Errorf("the note must still say the workflow was charged — that half is not in doubt:\n%s", got)
	}
	if !strings.Contains(got, buzzLedgerUnknownNote) {
		t.Errorf("the exclusion note does not render the shared buzzLedgerUnknownNote verbatim. A mature-content block is "+
			"`canUpgrade` — the server says the Buzz is HELD, not lost — and the CLI cannot tell that apart from a hard block "+
			"here, so it must make the one reviewed non-directional claim rather than its own.\n%s", got)
	}
	lower := strings.ToLower(got)
	for _, banned := range refundDirectionalClaims {
		if strings.Contains(lower, banned) {
			t.Errorf("the exclusion note decides the refund question with %q.\n%s", banned, got)
		}
	}
}
