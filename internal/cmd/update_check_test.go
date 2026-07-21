package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMergeBuildInfo(t *testing.T) {
	tests := []struct {
		name                                string
		version, commit, date               string
		mainVersion, rev, vcsTime, modified string
		wantVersion, wantCommit, wantDate   string
	}{
		{
			name:    "ldflags authoritative — build info ignored",
			version: "1.2.3", commit: "abc123", date: "2026-01-01",
			mainVersion: "v9.9.9", rev: "frombuildinfo", vcsTime: "2000-01-01T00:00:00Z", modified: "true",
			wantVersion: "1.2.3", wantCommit: "abc123", wantDate: "2026-01-01",
		},
		{
			name:    "all defaults — filled from build info, rev shortened",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: "v0.1.1", rev: "0123456789abcdef0123", vcsTime: "2026-06-23T00:00:00Z", modified: "false",
			wantVersion: "v0.1.1", wantCommit: "0123456789ab", wantDate: "2026-06-23T00:00:00Z",
		},
		{
			name:    "dirty working tree appends +dirty",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: "v0.1.1", rev: "0123456789abcdef0123", vcsTime: "2026-06-23T00:00:00Z", modified: "true",
			wantVersion: "v0.1.1", wantCommit: "0123456789ab+dirty", wantDate: "2026-06-23T00:00:00Z",
		},
		{
			name:    "(devel) main version ignored, keeps dev",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: "(devel)", rev: "short", vcsTime: "", modified: "",
			wantVersion: "dev", wantCommit: "short", wantDate: "unknown",
		},
		{
			name:    "partial — version stamped, commit/date filled",
			version: "1.0.0", commit: "none", date: "unknown",
			mainVersion: "v0.1.1", rev: "rev123", vcsTime: "2026-06-23T00:00:00Z", modified: "false",
			wantVersion: "1.0.0", wantCommit: "rev123", wantDate: "2026-06-23T00:00:00Z",
		},
		{
			name:    "empty build info leaves defaults",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: "", rev: "", vcsTime: "", modified: "",
			wantVersion: "dev", wantCommit: "none", wantDate: "unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotV, gotC, gotD := mergeBuildInfo(tc.version, tc.commit, tc.date, tc.mainVersion, tc.rev, tc.vcsTime, tc.modified)
			if gotV != tc.wantVersion {
				t.Errorf("version: got %q want %q", gotV, tc.wantVersion)
			}
			if gotC != tc.wantCommit {
				t.Errorf("commit: got %q want %q", gotC, tc.wantCommit)
			}
			if gotD != tc.wantDate {
				t.Errorf("date: got %q want %q", gotD, tc.wantDate)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		in              string
		wantOK          bool
		maj, min, patch int
	}{
		{"v0.1.10", true, 0, 1, 10},
		{"0.1.10", true, 0, 1, 10},
		{"v1.2.3-rc1", true, 1, 2, 3},
		{"v1.2.3+build.5", true, 1, 2, 3},
		{"v1.2.3-rc1+build", true, 1, 2, 3},
		{"v1", true, 1, 0, 0},
		{"v1.2", true, 1, 2, 0},
		{"dev", false, 0, 0, 0},
		{"", false, 0, 0, 0},
		{"v0.1.1-3-gabc123", true, 0, 1, 1}, // git describe: suffix dropped
		{"v0.0.0-20260101000000-abcdef", true, 0, 0, 0},
		{"vX.Y.Z", false, 0, 0, 0},
		{"1.2.3.4", false, 0, 0, 0},
	}
	for _, tc := range tests {
		sv, ok := parseSemver(tc.in)
		if ok != tc.wantOK {
			t.Errorf("parseSemver(%q) ok=%v want %v", tc.in, ok, tc.wantOK)
			continue
		}
		if ok && (sv.major != tc.maj || sv.minor != tc.min || sv.patch != tc.patch) {
			t.Errorf("parseSemver(%q) = %d.%d.%d want %d.%d.%d", tc.in, sv.major, sv.minor, sv.patch, tc.maj, tc.min, tc.patch)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current, latest string
		want            int
	}{
		{"v0.1.9", "v0.1.10", -1},
		{"v0.1.10", "v0.1.10", 0},
		{"v0.2.0", "v0.1.10", 1},
		{"v1.0.0", "v0.9.9", 1},
		{"dev", "v0.1.10", -1},                // unparseable current => available
		{"v0.0.0-20260101-abc", "v0.1.0", -1}, // pseudo-version parses to 0.0.0
		{"v0.1.9", "garbage", 0},              // unparseable latest => nothing
		{"v0.1.9-rc1", "v0.1.9", 0},           // suffix ignored => equal
		{"v0.1.9+dirty", "v0.1.10", -1},       // build meta ignored
	}
	for _, tc := range tests {
		if got := compareVersions(tc.current, tc.latest); got != tc.want {
			t.Errorf("compareVersions(%q,%q) = %d want %d", tc.current, tc.latest, got, tc.want)
		}
	}
}

// newReleaseServer returns an httptest server that serves the given tag_name and
// records how many times it was hit.
func newReleaseServer(t *testing.T, tag string, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// The call must be token-free.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("update check sent an Authorization header — must be unauthenticated")
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// pointAtServer swaps latestReleaseURL for the test and restores it after.
func pointAtServer(t *testing.T, url string) {
	t.Helper()
	orig := latestReleaseURL
	latestReleaseURL = url
	t.Cleanup(func() { latestReleaseURL = orig })
}

func TestPrintUpdateNotice_NewerAvailable(t *testing.T) {
	srv, hits := newReleaseServer(t, "v0.1.10", http.StatusOK)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")
	out := buf.String()

	if *hits != 1 {
		t.Errorf("expected exactly 1 GitHub hit, got %d", *hits)
	}
	if !strings.Contains(out, "A newer version is available: v0.1.10") {
		t.Errorf("missing newer-version notice: %s", out)
	}
	if !strings.Contains(out, "you have v0.1.9") {
		t.Errorf("notice should show current version: %s", out)
	}
	if !strings.Contains(out, "civitai upgrade") {
		t.Errorf("notice should include the civitai upgrade hint: %s", out)
	}
}

func TestPrintUpdateNotice_UpToDate(t *testing.T) {
	srv, _ := newReleaseServer(t, "v0.1.9", http.StatusOK)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")
	out := buf.String()

	if !strings.Contains(out, "latest version") {
		t.Errorf("expected up-to-date message: %s", out)
	}
	if strings.Contains(out, "newer version is available") {
		t.Errorf("should not claim a newer version when equal: %s", out)
	}
}

func TestPrintUpdateNotice_CurrentAhead(t *testing.T) {
	srv, _ := newReleaseServer(t, "v0.1.9", http.StatusOK)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.2.0")
	out := buf.String()

	if strings.Contains(out, "newer version is available") {
		t.Errorf("should not prompt upgrade when current is ahead: %s", out)
	}
	// When ahead (a dev/pre-release build newer than the latest release, OR the
	// draft-window artifact where /releases/latest momentarily returns the prior
	// tag), we must NOT claim "You're on the latest version." AND must NOT print
	// a misleading "Latest release: <older>" line that reads like the user is
	// behind. current >= latest ⇒ NO notice at all.
	if strings.Contains(out, "You're on the latest version") {
		t.Errorf("should not claim 'on the latest version' when current is ahead: %s", out)
	}
	if strings.Contains(out, "Latest release") {
		t.Errorf("ahead build must not surface an older 'Latest release' line (reads as behind): %s", out)
	}
	if buf.Len() != 0 {
		t.Errorf("current >= latest should produce NO update notice, got: %s", out)
	}
}

func TestPrintUpdateNotice_DevCurrent(t *testing.T) {
	srv, _ := newReleaseServer(t, "v0.1.10", http.StatusOK)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "dev")
	out := buf.String()

	if !strings.Contains(out, "Latest release: v0.1.10") {
		t.Errorf("dev current should surface the latest release: %s", out)
	}
}

func TestPrintUpdateNotice_Non200FailsSilent(t *testing.T) {
	srv, hits := newReleaseServer(t, "", http.StatusInternalServerError)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")

	if *hits != 1 {
		t.Errorf("expected 1 hit, got %d", *hits)
	}
	if buf.Len() != 0 {
		t.Errorf("non-200 should produce no output, got: %s", buf.String())
	}
}

func TestPrintUpdateNotice_GarbageBodyFailsSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	t.Cleanup(srv.Close)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")
	if buf.Len() != 0 {
		t.Errorf("garbage body should produce no output, got: %s", buf.String())
	}
}

func TestPrintUpdateNotice_UnreachableFailsSilent(t *testing.T) {
	// Closed server => connection refused => fail-silent, no panic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	pointAtServer(t, url)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")
	if buf.Len() != 0 {
		t.Errorf("unreachable endpoint should produce no output, got: %s", buf.String())
	}
}

// TestPrintUpdateNotice_UnparseableLatestFailsSilent proves the 🟡 fix: when the
// GitHub response carries a garbage or empty tag_name, compareVersions returns 0
// by fallback — but we must NOT print "You're on the latest version." (that would
// be an unfounded reassurance for a check that effectively failed). Stay silent.
func TestPrintUpdateNotice_UnparseableLatestFailsSilent(t *testing.T) {
	for _, tag := range []string{"", "garbage", "not-a-version", "vX.Y.Z"} {
		t.Run("tag="+tag, func(t *testing.T) {
			srv, hits := newReleaseServer(t, tag, http.StatusOK)
			pointAtServer(t, srv.URL)

			var buf bytes.Buffer
			printUpdateNotice(&buf, "v0.1.9")

			if *hits != 1 {
				t.Errorf("expected exactly 1 GitHub hit, got %d", *hits)
			}
			if buf.Len() != 0 {
				t.Errorf("unparseable latest %q must produce NO output (no 'up to date' claim), got: %s", tag, buf.String())
			}
		})
	}
}

// TestPrintUpdateNotice_VerifiedEqual asserts the up-to-date line prints ONLY on a
// verified equal (both current and latest parse and match).
func TestPrintUpdateNotice_VerifiedEqual(t *testing.T) {
	srv, _ := newReleaseServer(t, "v0.1.9", http.StatusOK)
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	printUpdateNotice(&buf, "v0.1.9")
	out := buf.String()

	if !strings.Contains(out, "You're on the latest version.") {
		t.Errorf("verified-equal should print the up-to-date line: %s", out)
	}
	if strings.Contains(out, "newer version is available") {
		t.Errorf("verified-equal must not prompt an upgrade: %s", out)
	}
}

// TestPrintUpdateNotice_SlowBodyHonorsDeadline proves `civitai version` can never
// hang on a slow server: the endpoint writes a 200 + headers, then STALLS past the
// update-check deadline before writing the body. printUpdateNotice must return
// within roughly the timeout, print nothing, and never error/panic.
func TestPrintUpdateNotice_SlowBodyHonorsDeadline(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // commit 200 + headers, then stall on the body.
		}
		// Stall well past updateCheckTimeout; bail early if the client gives up.
		select {
		case <-r.Context().Done():
		case <-time.After(updateCheckTimeout + 5*time.Second):
		case <-released:
		}
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	t.Cleanup(func() { close(released); srv.Close() })
	pointAtServer(t, srv.URL)

	var buf bytes.Buffer
	done := make(chan struct{})
	start := time.Now()
	go func() {
		printUpdateNotice(&buf, "v0.1.9")
		close(done)
	}()

	// Generous ceiling above the 2.5s deadline but far below a hang.
	select {
	case <-done:
	case <-time.After(updateCheckTimeout + 2*time.Second):
		t.Fatalf("printUpdateNotice hung on a slow server (no return after %s)", updateCheckTimeout+2*time.Second)
	}

	if elapsed := time.Since(start); elapsed > updateCheckTimeout+2*time.Second {
		t.Errorf("printUpdateNotice took %s, expected to bail near the %s deadline", elapsed, updateCheckTimeout)
	}
	if buf.Len() != 0 {
		t.Errorf("slow-body deadline hit should produce no output, got: %s", buf.String())
	}
}

func TestFetchLatestRelease_TimeoutFailsSilent(t *testing.T) {
	// Server that never responds within the context deadline.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // wait until the client cancels.
	}))
	t.Cleanup(func() { close(block); srv.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-cancelled context => immediate failure.
	if _, err := fetchLatestRelease(ctx, srv.URL); err == nil {
		t.Error("expected error on cancelled context")
	}
}

func TestUpdateCheckDisabled(t *testing.T) {
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")
	if updateCheckDisabled(false) {
		t.Error("should be enabled with no flag and empty env")
	}
	if !updateCheckDisabled(true) {
		t.Error("--no-update-check flag should disable")
	}
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	if !updateCheckDisabled(false) {
		t.Error("CIVITAI_NO_UPDATE_CHECK=1 should disable")
	}
}

// TestVersionCommandEnvOptOutMakesNoCall asserts the env opt-out path skips the
// HTTP call entirely (the server must get zero hits).
func TestVersionCommandEnvOptOut(t *testing.T) {
	srv, hits := newReleaseServer(t, "v9.9.9", http.StatusOK)
	pointAtServer(t, srv.URL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if *hits != 0 {
		t.Errorf("opt-out env should prevent any GitHub call, got %d hits", *hits)
	}
	if strings.Contains(out, "newer version") {
		t.Errorf("opt-out should suppress the update notice: %s", out)
	}
}

// TestVersionCommandNoUpdateFlag asserts --no-update-check skips the HTTP call.
func TestVersionCommandNoUpdateFlag(t *testing.T) {
	srv, hits := newReleaseServer(t, "v9.9.9", http.StatusOK)
	pointAtServer(t, srv.URL)

	if _, _, err := run(t, "version", "--no-update-check"); err != nil {
		t.Fatalf("version --no-update-check: %v", err)
	}
	if *hits != 0 {
		t.Errorf("--no-update-check should prevent any GitHub call, got %d hits", *hits)
	}
}

// TestVersionCommandShowsUpdate exercises the full command path with a stub
// server and asserts both the build-info lines and the update notice render.
func TestVersionCommandShowsUpdate(t *testing.T) {
	srv, _ := newReleaseServer(t, "v999.0.0", http.StatusOK)
	pointAtServer(t, srv.URL)
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	for _, want := range []string{"civitai", "commit:", "built:", "go:", "v999.0.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q: %s", want, out)
		}
	}
}
