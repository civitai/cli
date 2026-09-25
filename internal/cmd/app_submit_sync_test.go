package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/appapi"
)

// ---------------------------------------------------------------------------
// THE REMOTE-COMMIT SYNC
//
// The scenario under test is the one that made this guard necessary: the
// WEBSITE commits to the app's canonical repo (the manifest web form does), the
// author's clone predates that commit, and `civitai app submit` packages the
// stale tree — silently reverting the platform's change, because the bundle IS
// the submission.
//
// 🔴 THE FIXTURE USES A REAL BARE REPO OVER file://, NOT A STUBBED RUNNER. A
// stub would prove the Go branches and nothing about whether `fetch` +
// `merge --ff-only` actually move the tree, which is the entire claim. file://
// keeps it offline and hermetic.
// ---------------------------------------------------------------------------

const syncSlug = "custom-generators"

// syncFixture builds a canonical "remote" bare repo plus a local clone, and
// returns both. The local clone starts EXACTLY level with the remote; the
// individual tests move one side or the other.
type syncFix struct {
	t      *testing.T
	local  string
	remote string
}

func newSyncFixture(t *testing.T) *syncFix {
	t.Helper()
	requireGit(t)
	gitFixtureEnv(t)

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	remote := filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	local := filepath.Join(base, "local")

	runGitT(t, base, "init", "-q", "--bare", "-b", "fixture-main", remote)

	// Seed the canonical repo with v0.6.1.
	mkdirAllT(t, seed)
	runGitT(t, seed, "init", "-q", "-b", "fixture-main")
	writeManifestVersion(t, seed, syncSlug, "0.6.1")
	runGitT(t, seed, "add", "--", "block.manifest.json", "index.html")
	runGitT(t, seed, "commit", "-q", "-m", "seed")
	runGitT(t, seed, "push", "-q", remote, "fixture-main")

	// The author's clone.
	runGitT(t, base, "clone", "-q", "--", remote, local)
	return &syncFix{t: t, local: local, remote: remote}
}

// advanceRemote adds a commit to the canonical repo — this is the website
// editing the manifest.
func (f *syncFix) advanceRemote(version string) {
	f.t.Helper()
	work := filepath.Join(filepath.Dir(f.local), "work-"+version)
	runGitT(f.t, filepath.Dir(f.local), "clone", "-q", "--", f.remote, work)
	writeManifestVersion(f.t, work, syncSlug, version)
	runGitT(f.t, work, "add", "--", "block.manifest.json", "index.html")
	runGitT(f.t, work, "commit", "-q", "-m", "website edit "+version)
	runGitT(f.t, work, "push", "-q", "origin", "HEAD:fixture-main")
}

// commitLocal adds a commit only the author has.
func (f *syncFix) commitLocal(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.local, name), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
	runGitT(f.t, f.local, "add", "--", name)
	runGitT(f.t, f.local, "commit", "-q", "-m", "local "+name)
}

// localVersion reads the manifest version currently on disk in the clone.
func (f *syncFix) localVersion() string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.local, "block.manifest.json"))
	if err != nil {
		f.t.Fatal(err)
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		f.t.Fatal(err)
	}
	return m.Version
}

func runGitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errb
	if err := c.Run(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, errb.String())
	}
	return out.String()
}

// syncServer serves clone info pointing at the fixture's bare repo, the
// submissions list, and the submit route — recording the version actually
// submitted, which is the fact every test here turns on.
// submitRecord is what the fixture server saw on the submit route.
type submitRecord struct {
	version string
	// sourceCommit is the provenance stamp (#411). It must name the commit the
	// BUNDLE was built from — which, after a fast-forward, is the post-sync
	// HEAD and not the one the dirty guard first read.
	sourceCommit string
}

func syncServer(t *testing.T, remote string) (*httptest.Server, *submitRecord) {
	t.Helper()
	var rec submitRecord
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, appapi.CloneInfoPath):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{"data": map[string]any{
					"json": map[string]any{
						"slug":     syncSlug,
						"cloneUrl": "file://" + remote,
						"httpUrl":  "file://" + remote,
						"token":    "forgejo-secret-token",
					},
				}},
			})
		case strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []appapi.Submission{}})
		case strings.HasPrefix(r.URL.Path, appapi.DefaultSubmitPath):
			// 🔴 READ THE VERSION OUT OF THE UPLOADED BUNDLE, NOT OFF THE
			// REQUEST. The submit body carries only `bundleBase64` — the slug
			// and version live inside the zip's block.manifest.json, which
			// means the bundle IS the submission and the bundle is therefore
			// the only honest place to assert what was submitted. Reading a
			// `version` field off the request would have asserted nothing:
			// there is no such field, so it decoded to "" on every path and
			// every test would have agreed with every other.
			rec.version, rec.sourceCommit = submitBodyFacts(t, r)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"publishRequestId": "pubreq_new", "slug": syncSlug, "version": rec.version, "status": "pending",
			})
		default:
			http.Error(w, "unexpected route: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &rec
}

// submitBodyFacts decodes the uploaded bundle and returns the version its
// block.manifest.json declares, plus the `sourceCommit` provenance stamp that
// travelled beside it.
//
// 🔴 IT READS THE VERSION OUT OF THE ZIP, NOT OFF THE REQUEST. The submit body
// carries only `bundleBase64` + provenance — the version lives inside the
// bundle, which IS the submission, so the bundle is the only honest place to
// assert what was submitted. An earlier revision decoded a `version` field off
// the request; there is no such field, so it returned "" on every path and
// every assertion agreed with every other.
//
// It FAILS the test rather than returning "" on a malformed body: "" is what
// the assertions compare against, so a silent "" would pass by accident against
// a submit that never happened.
func submitBodyFacts(t *testing.T, r *http.Request) (version, sourceCommit string) {
	t.Helper()
	var body struct {
		BundleBase64 string `json:"bundleBase64"`
		SourceCommit string `json:"sourceCommit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("submit body is not JSON: %v", err)
		return "", ""
	}
	raw, err := base64.StdEncoding.DecodeString(body.BundleBase64)
	if err != nil {
		t.Errorf("bundleBase64 is not base64: %v", err)
		return "", body.SourceCommit
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Errorf("the bundle is not a zip: %v", err)
		return "", body.SourceCommit
	}
	for _, e := range zr.File {
		if filepath.Base(e.Name) != "block.manifest.json" {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			t.Errorf("opening %s in the bundle: %v", e.Name, err)
			return "", body.SourceCommit
		}
		defer func() { _ = rc.Close() }()
		var m struct {
			Version string `json:"version"`
		}
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			t.Errorf("the bundled manifest is not JSON: %v", err)
			return "", body.SourceCommit
		}
		return m.Version, body.SourceCommit
	}
	t.Error("the uploaded bundle contains no block.manifest.json")
	return "", body.SourceCommit
}

// --- THE REGRESSION TEST ------------------------------------------------
//
// 🔴 THIS IS THE ONE THAT MUST FAIL ON PRE-CHANGE CODE, and it does: without
// the sync, `submitted` is "0.6.1" (the stale clone) and the local manifest
// stays "0.6.1", so both assertions below fail. Verified by running it against
// origin/main — see the commit message for the matrix.

func TestSubmitFastForwardsOntoTheCanonicalRepoBeforePackaging(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2") // the website edited the manifest

	if got := f.localVersion(); got != "0.6.1" {
		t.Fatalf("fixture precondition: the clone must start STALE, got %q", got)
	}

	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", f.local, "--yes")
	if err != nil {
		t.Fatalf("submit must succeed after a clean fast-forward: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// The tree moved…
	if got := f.localVersion(); got != "0.6.2" {
		t.Errorf("the working tree must be fast-forwarded onto the canonical repo; manifest version = %q, want 0.6.2", got)
	}
	// …and — the fact that actually matters — the BUNDLE carried the new one.
	if submitted.version != "0.6.2" {
		t.Errorf("the submitted bundle must carry the website's manifest; submitted version = %q, want 0.6.2", submitted.version)
	}
}

// TestSubmitStampsTheProvenanceOfTheSYNCEDTree.
//
// 🔴 THE MUTANT THIS EXISTS FOR SURVIVED A FULL GREEN SUITE. The dirty guard
// runs BEFORE the sync (a refused dirty submit must contact nothing), so the
// `sourceCommit` it computes names the PRE-fast-forward HEAD. If the guard is
// not re-run after the tree moves, the submission is stamped with a commit the
// bundle did not come from — which is exactly the untraceability #411 exists to
// prevent, reintroduced by the fix for a different bug. Nothing else in this
// file notices: the version assertions all pass, because the manifest is
// re-loaded either way.
func TestSubmitStampsTheProvenanceOfTheSyncedTree(t *testing.T) {
	f := newSyncFixture(t)
	preSync := strings.TrimSpace(runGitT(t, f.local, "rev-parse", "HEAD"))
	f.advanceRemote("0.6.2")

	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	if _, _, err := run(t, "app", "submit", f.local, "--yes"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	postSync := strings.TrimSpace(runGitT(t, f.local, "rev-parse", "HEAD"))
	if preSync == postSync {
		t.Fatalf("fixture precondition: the fast-forward must move HEAD, still %s", preSync)
	}
	if submitted.sourceCommit == preSync {
		t.Errorf("the submission was stamped with the PRE-sync commit %s — the bundle was built from %s, "+
			"so the provenance names a commit the bundle did not come from", preSync, postSync)
	}
	if submitted.sourceCommit != postSync {
		t.Errorf("sourceCommit = %q, want the post-fast-forward HEAD %q", submitted.sourceCommit, postSync)
	}
}

// TestSubmitSyncNeverPrintsTheCloneToken is the credential half. The clone URL
// embeds a token; nothing about an ordinary submit should put it on screen.
func TestSubmitSyncNeverPrintsTheCloneToken(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2")
	withStdinTTY(t, false)
	srv, _ := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	stdout, stderr, err := run(t, "app", "submit", f.local, "--yes")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if strings.Contains(stdout+stderr, "forgejo-secret-token") {
		t.Errorf("the Forgejo token reached the terminal:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

// --- THE ONE REFUSAL ----------------------------------------------------

func TestSubmitRefusesWhenHistoryHasDiverged(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2") // the website committed…
	f.commitLocal("notes.md", "mine")

	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	stdout, _, err := run(t, "app", "submit", f.local, "--yes")
	if err == nil {
		t.Fatalf("a diverged history must be refused; stdout:\n%s", stdout)
	}
	// The SENTINEL, not a substring — that is what pins exit code 1.
	if !errors.Is(err, ErrDivergedFromRemote) {
		t.Errorf("the error must carry ErrDivergedFromRemote, got %#v", err)
	}
	if submitted.version != "" {
		t.Errorf("nothing may be submitted from a diverged tree, got version %q", submitted.version)
	}
	// The refusal has to name the way out, or it is a dead end.
	if !strings.Contains(err.Error(), "--no-pull") {
		t.Errorf("the refusal must name --no-pull:\n%s", err.Error())
	}

	// 🔴 AND THE WAY OUT HAS TO WORK. An earlier revision recommended
	// `civitai app pull`, whose sync path is `fetch` + `merge --ff-only`
	// (app_pull.go) — and a fast-forward is exactly what a divergence makes
	// impossible. Measured on git 2.x, a 1-ahead/1-behind repo answers
	// `merge --ff-only FETCH_HEAD` with "Diverging branches can't be
	// fast-forwarded" and rc=128. A refusal whose advice cannot work is worse
	// than one that offers none: it teaches the reader to skip to the override.
	if strings.Contains(err.Error(), "civitai app pull") {
		t.Errorf("the refusal recommends `civitai app pull`, which runs `merge --ff-only` and CANNOT resolve a "+
			"divergence — it fails rc=128 on exactly this state:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "FETCH_HEAD") {
		t.Errorf("the refusal must name FETCH_HEAD — the sync has already fetched it, so it is the one ref the "+
			"reader can act on regardless of how their remotes are configured:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "rebase FETCH_HEAD") {
		t.Errorf("the refusal must give a runnable reconcile command:\n%s", err.Error())
	}
}

// --- THE ESCAPE HATCH ---------------------------------------------------

func TestSubmitNoPullSkipsTheSyncEntirely(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2")
	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	if _, _, err := run(t, "app", "submit", f.local, "--yes", "--no-pull"); err != nil {
		t.Fatalf("--no-pull must submit the tree as it is: %v", err)
	}
	if got := f.localVersion(); got != "0.6.1" {
		t.Errorf("--no-pull must not touch the working tree, got %q", got)
	}
	if submitted.version != "0.6.1" {
		t.Errorf("--no-pull must submit the local tree, got %q", submitted.version)
	}
}

// TestSubmitNoPullStillRefusesADivergedTreeNever — the escape hatch must also
// clear the REFUSAL, or it is not an escape hatch.
func TestSubmitNoPullClearsTheDivergenceRefusal(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2")
	f.commitLocal("notes.md", "mine")
	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	if _, _, err := run(t, "app", "submit", f.local, "--yes", "--no-pull"); err != nil {
		t.Fatalf("--no-pull must submit a diverged tree: %v", err)
	}
	if submitted.version != "0.6.1" {
		t.Errorf("want the local version submitted, got %q", submitted.version)
	}
}

// --- DEGRADE, NEVER ENFORCE ---------------------------------------------

// TestSubmitSyncIsSilentOnADirectoryThatIsNotARepo is the scaffold path — the
// one the dirty guard's docblock says must not break.
func TestSubmitSyncIsSilentOnADirectoryThatIsNotARepo(t *testing.T) {
	dir := t.TempDir()
	writeManifestVersion(t, dir, syncSlug, "0.6.1")
	withStdinTTY(t, false)
	srv, submitted := syncServer(t, t.TempDir()) // remote never consulted
	withGuardEnv(t, srv)

	_, stderr, err := run(t, "app", "submit", dir, "--yes")
	if err != nil {
		t.Fatalf("a non-repo directory must submit normally: %v\nstderr:\n%s", err, stderr)
	}
	if submitted.version != "0.6.1" {
		t.Errorf("want 0.6.1 submitted, got %q", submitted.version)
	}
	if strings.Contains(stderr, "canonical repository") {
		t.Errorf("the sync must say nothing at all on the scaffold path:\n%s", stderr)
	}
}

// TestSubmitProceedsWhenTheCloneInfoCallFails — an author offline, or a
// collaborator who cannot read the owner-only endpoint, must still be able to
// submit. WARN, never refuse.
func TestSubmitProceedsWhenTheCloneInfoCallFails(t *testing.T) {
	f := newSyncFixture(t)
	withStdinTTY(t, false)

	var submitted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, appapi.CloneInfoPath):
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, appapi.SubmissionsPath):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"submissions": []appapi.Submission{}})
		default:
			submitted, _ = submitBodyFacts(t, r)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"publishRequestId": "p", "slug": syncSlug, "version": submitted, "status": "pending"})
		}
	}))
	t.Cleanup(srv.Close)
	withGuardEnv(t, srv)

	_, stderr, err := run(t, "app", "submit", f.local, "--yes")
	if err != nil {
		t.Fatalf("an unreachable clone-info endpoint must not block a submit: %v\nstderr:\n%s", err, stderr)
	}
	if submitted != "0.6.1" {
		t.Errorf("want 0.6.1 submitted, got %q", submitted)
	}
	// It must SAY so, though — silently skipping the protection is how the
	// overwrite happens without anyone knowing it could have been prevented.
	if !strings.Contains(stderr, "overwritten by this bundle") {
		t.Errorf("the skipped-sync warning must name the consequence:\n%s", stderr)
	}
}

// TestSubmitSyncDoesNothingWhenAlreadyUpToDate — the common case must be silent
// and must not touch the tree.
func TestSubmitSyncDoesNothingWhenAlreadyUpToDate(t *testing.T) {
	f := newSyncFixture(t)
	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	_, stderr, err := run(t, "app", "submit", f.local, "--yes")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.version != "0.6.1" {
		t.Errorf("want 0.6.1, got %q", submitted.version)
	}
	if strings.Contains(stderr, "pulled ") {
		t.Errorf("an up-to-date tree must not report a pull:\n%s", stderr)
	}
}

// TestSubmitSyncLeavesUnpushedLocalCommitsAlone — ahead-only is legitimate and
// is not this guard's business.
func TestSubmitSyncLeavesUnpushedLocalCommitsAlone(t *testing.T) {
	f := newSyncFixture(t)
	f.commitLocal("notes.md", "mine")
	withStdinTTY(t, false)
	srv, submitted := syncServer(t, f.remote)
	withGuardEnv(t, srv)

	_, _, err := run(t, "app", "submit", f.local, "--yes")
	if err != nil {
		t.Fatalf("unpushed local work is legitimate and must submit: %v", err)
	}
	if submitted.version != "0.6.1" {
		t.Errorf("want 0.6.1, got %q", submitted.version)
	}
}

// --- THE NO-NETWORK CONTRACT --------------------------------------------

// TestSubmitSyncDoesNotFireOnTheNonTTYNoYesInvocation.
//
// 🔴 THIS IS THE ORDERING CONSTRAINT, AND IT IS THE REASON THE SYNC IS GATED.
// The sync runs BEFORE confirmSubmit, so without the (assumeYes || TTY) gate a
// bare non-interactive `civitai app submit` — which refuses for lack of --yes
// and is pinned elsewhere as touching NOTHING — would acquire a network read.
func TestSubmitSyncDoesNotFireOnTheNonTTYNoYesInvocation(t *testing.T) {
	f := newSyncFixture(t)
	f.advanceRemote("0.6.2")
	withStdinTTY(t, false)

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "nothing may be requested", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	withGuardEnv(t, srv)

	if _, _, err := run(t, "app", "submit", f.local); err == nil {
		t.Fatal("a non-TTY submit without --yes must refuse")
	}
	if hits != 0 {
		t.Errorf("the refused invocation made %d network call(s); it must make none", hits)
	}
	if got := f.localVersion(); got != "0.6.1" {
		t.Errorf("it must not touch the working tree either, got %q", got)
	}
}

// --- UNIT: the parse, whose failure direction is the dangerous one ------

func TestParseAheadBehind(t *testing.T) {
	for _, tc := range []struct {
		in            string
		ahead, behind int
		ok            bool
	}{
		{"0\t0\n", 0, 0, true},
		{"0\t3\n", 0, 3, true},
		{"2\t0\n", 2, 0, true},
		{"2\t3\n", 2, 3, true},
		{"  4   5  ", 4, 5, true},
		// 🔴 EVERY MALFORMED INPUT MUST REPORT ok=false, NEVER 0/0. A silent 0/0
		// is indistinguishable from "up to date", which is the reassuring answer
		// — the one direction a parse failure must never fall, because it turns
		// "I could not tell" into "nothing to do" and the overwrite proceeds.
		{"", 0, 0, false},
		{"0", 0, 0, false},
		{"0\t1\t2", 0, 0, false},
		{"x\t1", 0, 0, false},
		{"1\ty", 0, 0, false},
		{"fatal: bad revision", 0, 0, false},
	} {
		a, b, ok := parseAheadBehind(tc.in)
		if a != tc.ahead || b != tc.behind || ok != tc.ok {
			t.Errorf("parseAheadBehind(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.in, a, b, ok, tc.ahead, tc.behind, tc.ok)
		}
	}
}

// TestRedactSecretEmptyTokenIsANoOp pins the bug this function would otherwise
// have: strings.ReplaceAll(s, "", "***") interleaves the replacement between
// every character, turning an empty token into a shredded message.
func TestRedactSecretEmptyTokenIsANoOp(t *testing.T) {
	const msg = "fatal: could not read from remote"
	if got := redactSecret(msg, ""); got != msg {
		t.Errorf("an empty token must leave the text alone, got %q", got)
	}
	if got := redactSecret("url https://u:abc123@h/r", "abc123"); strings.Contains(got, "abc123") {
		t.Errorf("the token survived redaction: %q", got)
	}
}

// --- UNIT: the degrade matrix, without a server --------------------------

// TestSyncRemoteCommitsSkipsSilentlyWithoutARepo covers the nil/scaffold arms
// directly, so the matrix in the docblock has a test per row rather than only
// the rows the end-to-end fixtures happen to reach.
func TestSyncRemoteCommitsSkipsSilentlyWithoutARepo(t *testing.T) {
	dir := t.TempDir()
	var warn bytes.Buffer
	called := false
	fetch := func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error) {
		called = true
		return nil, nil
	}
	out, err := syncRemoteCommits(context.Background(), fetch, gitOutput, gitWriteRunner, &warn, dir, syncSlug)
	if err != nil {
		t.Fatalf("a non-repo directory must not error: %v", err)
	}
	if out.Advanced {
		t.Error("nothing can have advanced")
	}
	if called {
		t.Error("clone info must not be requested for a directory that is not a repo — that is a network call on the scaffold path")
	}
	if warn.Len() != 0 {
		t.Errorf("the scaffold path must be silent, got: %s", warn.String())
	}
}

// TestSyncRemoteCommitsSkipsWhenTheRepoHasNoCanonicalCounterpart — an app whose
// first version has never been approved has no repo yet, and saying so on every
// first submit would be noise on the happy path.
func TestSyncRemoteCommitsSkipsQuietlyWhenNotYetAvailable(t *testing.T) {
	f := newSyncFixture(t)
	var warn bytes.Buffer
	fetch := func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error) {
		return &appapi.ForgejoCloneInfo{NotYetAvailable: true, Slug: syncSlug}, nil
	}
	out, err := syncRemoteCommits(context.Background(), fetch, gitOutput, gitWriteRunner, &warn, f.local, syncSlug)
	if err != nil {
		t.Fatalf("not-yet-available must not error: %v", err)
	}
	if out.Advanced {
		t.Error("nothing can have advanced")
	}
	if warn.Len() != 0 {
		t.Errorf("must be silent, got: %s", warn.String())
	}
}

// TestSyncRemoteCommitsWarnsAndProceedsWhenTheFetchFails pins the offline arm
// AND that the warning names the consequence rather than just the failure.
func TestSyncRemoteCommitsWarnsAndProceedsWhenTheFetchFails(t *testing.T) {
	f := newSyncFixture(t)
	var warn bytes.Buffer
	fetch := func(ctx context.Context, app string) (*appapi.ForgejoCloneInfo, error) {
		return &appapi.ForgejoCloneInfo{
			Slug:     syncSlug,
			CloneURL: "file://" + filepath.Join(t.TempDir(), "does-not-exist.git"),
			Token:    "tok-secret",
		}, nil
	}
	out, err := syncRemoteCommits(context.Background(), fetch, gitOutput, gitWriteRunner, &warn, f.local, syncSlug)
	if err != nil {
		t.Fatalf("a failed fetch must WARN, never refuse: %v", err)
	}
	if out.Advanced {
		t.Error("nothing can have advanced")
	}
	if !strings.Contains(warn.String(), "overwritten by this bundle") {
		t.Errorf("the warning must name the consequence:\n%s", warn.String())
	}
	if strings.Contains(warn.String(), "tok-secret") {
		t.Errorf("the token leaked into the warning:\n%s", warn.String())
	}
}

// TestGitWriteRunnerScrubsTheRelocatingEnvironment.
//
// 🔴 THE MUTATION THIS KILLS: dropping `c.Env = gitScrubbedEnv(...)` from
// gitWriteRunner. GIT_DIR beats `-C`, so an unscrubbed write runner fetches and
// merges into whatever repository the environment names — and a `civitai app
// submit` run from a git hook inherits exactly such a GIT_DIR.
func TestGitWriteRunnerScrubsTheRelocatingEnvironment(t *testing.T) {
	requireGit(t)
	gitFixtureEnv(t)

	target := t.TempDir()
	runGitT(t, target, "init", "-q", "-b", "fixture-main")
	decoy := t.TempDir()
	runGitT(t, decoy, "init", "-q", "-b", "decoy-main")

	// Aim the environment at the decoy, the way a git hook would.
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)

	out, err := gitWriteRunner(target, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatalf("gitWriteRunner: %v (%s)", err, out)
	}
	got := strings.TrimSpace(out)
	wantPrefix, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("gitWriteRunner obeyed the environment instead of -C: git dir = %q, want one under %q.\n"+
			"GIT_DIR beats -C, so an unscrubbed write runner would fetch and merge into the WRONG repository.", got, wantPrefix)
	}
}
