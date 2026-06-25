package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParsePseudoVersion(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantOK   bool
		wantSHA  string
		wantDate string
	}{
		{
			name:     "base-tag form vX.Y.Z-0.ts-sha",
			in:       "v0.1.12-0.20260625194726-4dbfb55abc12",
			wantOK:   true,
			wantSHA:  "4dbfb55abc12",
			wantDate: "2026-06-25T19:47:26Z",
		},
		{
			name:     "no-base form vX.Y.Z-ts-sha",
			in:       "v0.0.0-20260101000000-abcdef012345",
			wantOK:   true,
			wantSHA:  "abcdef012345",
			wantDate: "2026-01-01T00:00:00Z",
		},
		{
			name:     "pre-release base form vX.Y.Z-pre.0.ts-sha",
			in:       "v1.2.3-pre.0.20251231235959-0123456789ab",
			wantOK:   true,
			wantSHA:  "0123456789ab",
			wantDate: "2025-12-31T23:59:59Z",
		},
		{
			name:     "dotted pre-release base form",
			in:       "v1.2.3-beta.1.0.20240615120000-fedcba987654",
			wantOK:   true,
			wantSHA:  "fedcba987654",
			wantDate: "2024-06-15T12:00:00Z",
		},
		// Non-pseudo / malformed inputs must not match and must not panic.
		{name: "clean release tag", in: "v0.1.12", wantOK: false},
		{name: "dev", in: "dev", wantOK: false},
		{name: "empty", in: "", wantOK: false},
		{name: "(devel)", in: "(devel)", wantOK: false},
		{name: "git describe", in: "v0.1.1-3-gabc123", wantOK: false},
		{name: "short sha (not 12 hex)", in: "v0.0.0-20260101000000-abcdef", wantOK: false},
		{name: "bad timestamp (13 digits)", in: "v0.0.0-2026010100000-abcdef012345", wantOK: false},
		{name: "non-hex sha", in: "v0.0.0-20260101000000-zzzzzzzzzzzz", wantOK: false},
		{name: "impossible date", in: "v0.0.0-20261301000000-abcdef012345", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sha, date, ok := parsePseudoVersion(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parsePseudoVersion(%q) ok=%v want %v (sha=%q date=%q)", tc.in, ok, tc.wantOK, sha, date)
			}
			if !ok {
				if sha != "" || date != "" {
					t.Errorf("non-match should return empty sha/date, got sha=%q date=%q", sha, date)
				}
				return
			}
			if sha != tc.wantSHA {
				t.Errorf("sha: got %q want %q", sha, tc.wantSHA)
			}
			if date != tc.wantDate {
				t.Errorf("date: got %q want %q", date, tc.wantDate)
			}
		})
	}
}

// TestMergeBuildInfo_PseudoVersionFallback proves the FIX-1 precedence: a Go
// pseudo-version recovers commit/date offline when vcs.* is absent, but ldflags
// and vcs.* still win when present, and only defaulted fields get filled.
func TestMergeBuildInfo_PseudoVersionFallback(t *testing.T) {
	const pseudo = "v0.1.12-0.20260625194726-4dbfb55abc12"

	tests := []struct {
		name                                string
		version, commit, date               string
		mainVersion, rev, vcsTime, modified string
		wantVersion, wantCommit, wantDate   string
	}{
		{
			name:    "pseudo fills commit+date when no vcs.* present",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: pseudo,
			wantVersion: pseudo, wantCommit: "4dbfb55abc12", wantDate: "2026-06-25T19:47:26Z",
		},
		{
			name:    "vcs.* wins over pseudo-version",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: pseudo, rev: "vcsrevision00", vcsTime: "2020-01-01T00:00:00Z", modified: "false",
			wantVersion: pseudo, wantCommit: "vcsrevision0", wantDate: "2020-01-01T00:00:00Z",
		},
		{
			name:    "ldflags win over pseudo-version",
			version: "1.2.3", commit: "abc123", date: "2026-01-01",
			mainVersion: pseudo,
			wantVersion: "1.2.3", wantCommit: "abc123", wantDate: "2026-01-01",
		},
		{
			name:    "pseudo fills only the defaulted field (commit stamped, date default)",
			version: "dev", commit: "ldflagsha", date: "unknown",
			mainVersion: pseudo,
			wantVersion: pseudo, wantCommit: "ldflagsha", wantDate: "2026-06-25T19:47:26Z",
		},
		{
			name:    "clean tag mainVersion does not fill commit/date",
			version: "dev", commit: "none", date: "unknown",
			mainVersion: "v0.1.12",
			wantVersion: "v0.1.12", wantCommit: "none", wantDate: "unknown",
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

// newCommitServer returns an httptest server that serves a GitHub commit object
// for the requested ref and records hit count + asserts no Authorization header.
func newCommitServer(t *testing.T, sha, date string, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("enrichment sent an Authorization header — must be unauthenticated")
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"sha":"` + sha + `","commit":{"committer":{"date":"` + date + `"}}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func pointCommitsAt(t *testing.T, base string) {
	t.Helper()
	orig := commitsBaseURL
	commitsBaseURL = base
	t.Cleanup(func() { commitsBaseURL = orig })
}

func TestEnrichBuildInfoFromGitHub_FillsCommitAndDate(t *testing.T) {
	srv, hits := newCommitServer(t, "deadbeefcafef00dbabe", "2026-06-20T10:11:12Z", http.StatusOK)
	pointCommitsAt(t, srv.URL)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", "v0.1.12", false)
	if *hits != 1 {
		t.Errorf("expected exactly 1 GitHub hit, got %d", *hits)
	}
	if gotCommit != "deadbeefcafe" {
		t.Errorf("commit: got %q want deadbeefcafe (12-char)", gotCommit)
	}
	if gotDate != "2026-06-20T10:11:12Z" {
		t.Errorf("date: got %q want 2026-06-20T10:11:12Z", gotDate)
	}
}

func TestEnrichBuildInfoFromGitHub_AuthorDateFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// committer.date absent => fall back to author.date.
		_, _ = w.Write([]byte(`{"sha":"abc123def4560000","commit":{"author":{"date":"2025-05-05T05:05:05Z"}}}`))
	}))
	t.Cleanup(srv.Close)
	pointCommitsAt(t, srv.URL)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", "v0.1.12", false)
	if gotCommit != "abc123def456" {
		t.Errorf("commit: got %q want abc123def456", gotCommit)
	}
	if gotDate != "2025-05-05T05:05:05Z" {
		t.Errorf("date: got %q want author.date fallback", gotDate)
	}
}

func TestEnrichBuildInfoFromGitHub_FailSilent(t *testing.T) {
	cases := []struct {
		name   string
		handle http.HandlerFunc
	}{
		{"non-200", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }},
		{"garbage body", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{not json`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handle)
			t.Cleanup(srv.Close)
			pointCommitsAt(t, srv.URL)

			gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", "v0.1.12", false)
			if gotCommit != "none" || gotDate != "unknown" {
				t.Errorf("fail-silent should leave defaults, got commit=%q date=%q", gotCommit, gotDate)
			}
		})
	}
}

func TestEnrichBuildInfoFromGitHub_UnreachableFailSilent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // connection refused
	pointCommitsAt(t, url)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", "v0.1.12", false)
	if gotCommit != "none" || gotDate != "unknown" {
		t.Errorf("unreachable should leave defaults, got commit=%q date=%q", gotCommit, gotDate)
	}
}

// TestEnrichBuildInfoFromGitHub_NoCallWhenStamped: a fully-stamped build (commit
// AND date already resolved) must make ZERO HTTP calls.
func TestEnrichBuildInfoFromGitHub_NoCallWhenStamped(t *testing.T) {
	srv, hits := newCommitServer(t, "x", "y", http.StatusOK)
	pointCommitsAt(t, srv.URL)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("abc123", "2026-01-01", "v0.1.12", false)
	if *hits != 0 {
		t.Errorf("fully-stamped build must make NO GitHub call, got %d hits", *hits)
	}
	if gotCommit != "abc123" || gotDate != "2026-01-01" {
		t.Errorf("stamped values must be untouched, got commit=%q date=%q", gotCommit, gotDate)
	}
}

// TestEnrichBuildInfoFromGitHub_NoCallForNonTag: dev / pseudo-version / empty are
// not clean tags we can resolve, so no HTTP call fires.
func TestEnrichBuildInfoFromGitHub_NoCallForNonTag(t *testing.T) {
	for _, version := range []string{"dev", "", "v0.1.12-0.20260625194726-4dbfb55abc12", "(devel)"} {
		t.Run("version="+version, func(t *testing.T) {
			srv, hits := newCommitServer(t, "x", "y", http.StatusOK)
			pointCommitsAt(t, srv.URL)

			gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", version, false)
			if *hits != 0 {
				t.Errorf("non-tag version %q must make NO GitHub call, got %d hits", version, *hits)
			}
			if gotCommit != "none" || gotDate != "unknown" {
				t.Errorf("non-tag should leave defaults, got commit=%q date=%q", gotCommit, gotDate)
			}
		})
	}
}

// TestEnrichBuildInfoFromGitHub_NoCallWhenDisabled: opt-out makes ZERO HTTP hits.
func TestEnrichBuildInfoFromGitHub_NoCallWhenDisabled(t *testing.T) {
	srv, hits := newCommitServer(t, "x", "y", http.StatusOK)
	pointCommitsAt(t, srv.URL)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("none", "unknown", "v0.1.12", true)
	if *hits != 0 {
		t.Errorf("disabled enrichment must make NO GitHub call, got %d hits", *hits)
	}
	if gotCommit != "none" || gotDate != "unknown" {
		t.Errorf("disabled should leave defaults, got commit=%q date=%q", gotCommit, gotDate)
	}
}

// TestEnrichBuildInfoFromGitHub_PartialFill: only date unresolved => fills date,
// keeps the already-resolved commit; still makes the call.
func TestEnrichBuildInfoFromGitHub_PartialFill(t *testing.T) {
	srv, hits := newCommitServer(t, "shouldnotwin000", "2026-06-20T10:11:12Z", http.StatusOK)
	pointCommitsAt(t, srv.URL)

	gotCommit, gotDate := enrichBuildInfoFromGitHub("realcommit", "unknown", "v0.1.12", false)
	if *hits != 1 {
		t.Errorf("expected 1 hit when one field unresolved, got %d", *hits)
	}
	if gotCommit != "realcommit" {
		t.Errorf("already-resolved commit must be preserved, got %q", gotCommit)
	}
	if gotDate != "2026-06-20T10:11:12Z" {
		t.Errorf("date should be filled, got %q", gotDate)
	}
}

// TestVersionCommand_HonestOutputWhenUnresolved: offline tagged go-install (no
// enrichment) must print the honest marker, never raw "none"/"unknown".
func TestVersionCommand_HonestOutputWhenUnresolved(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "v0.1.12", "none", "unknown"
	// Opt out so neither enrichment nor the update notice touches the network.
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.Contains(out, "commit: none") {
		t.Errorf("must not print raw 'commit: none': %s", out)
	}
	if strings.Contains(out, "built:  unknown") {
		t.Errorf("must not print raw 'built:  unknown': %s", out)
	}
	if !strings.Contains(out, "unavailable (go install)") {
		t.Errorf("should print the honest go-install marker: %s", out)
	}
	if !strings.Contains(out, "build metadata is unavailable") {
		t.Errorf("should print the honest hint note: %s", out)
	}
}

// TestVersionCommand_EnrichmentFillsCommitDate: online tagged go-install resolves
// commit+date from GitHub and prints them (no honest marker, no note).
func TestVersionCommand_EnrichmentFillsCommitDate(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "v0.1.12", "none", "unknown"
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

	srv, hits := newCommitServer(t, "feedface1234abcd", "2026-06-24T08:09:10Z", http.StatusOK)
	pointCommitsAt(t, srv.URL)
	// Suppress the separate update-notice network call (point it at a dead URL
	// so it fails silently) to keep the assertion focused on enrichment.
	pointAtServer(t, "http://127.0.0.1:0")

	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if *hits != 1 {
		t.Errorf("expected exactly 1 enrichment hit, got %d", *hits)
	}
	if !strings.Contains(out, "commit: feedface1234") {
		t.Errorf("should print enriched commit: %s", out)
	}
	if !strings.Contains(out, "built:  2026-06-24T08:09:10Z") {
		t.Errorf("should print enriched date: %s", out)
	}
	if strings.Contains(out, "unavailable (go install)") {
		t.Errorf("must NOT print the honest marker once enriched: %s", out)
	}
	if strings.Contains(out, "build metadata is unavailable") {
		t.Errorf("must NOT print the hint note once enriched: %s", out)
	}
}

// TestVersionCommand_NoEnrichmentWhenStamped: a fully-stamped build makes NO
// enrichment call.
func TestVersionCommand_NoEnrichmentWhenStamped(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "v0.1.12", "abc123def456", "2026-01-01T00:00:00Z"
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "")

	srv, hits := newCommitServer(t, "x", "y", http.StatusOK)
	pointCommitsAt(t, srv.URL)
	pointAtServer(t, "http://127.0.0.1:0") // suppress update notice

	out, _, err := run(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if *hits != 0 {
		t.Errorf("stamped build must make NO enrichment call, got %d hits", *hits)
	}
	if !strings.Contains(out, "commit: abc123def456") {
		t.Errorf("should print the stamped commit: %s", out)
	}
}

// TestVersionCommand_EnrichmentSkippedWhenDisabled: opt-out env => zero
// enrichment hits.
func TestVersionCommand_EnrichmentSkippedWhenDisabled(t *testing.T) {
	resetBuildVars(t)
	version, commit, date = "v0.1.12", "none", "unknown"
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")

	srv, hits := newCommitServer(t, "x", "y", http.StatusOK)
	pointCommitsAt(t, srv.URL)

	if _, _, err := run(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}
	if *hits != 0 {
		t.Errorf("opt-out must prevent any enrichment call, got %d hits", *hits)
	}
}
