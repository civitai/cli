package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// latestReleaseURL is the GitHub public API endpoint for the newest release of
// this CLI. It's a package var so tests can point it at an httptest server.
//
// SECURITY: this is an UNAUTHENTICATED public API call — we never attach the
// user's Civitai API token (or any Authorization header) to it.
var latestReleaseURL = "https://api.github.com/repos/civitai/cli/releases/latest"

// commitsBaseURL is the GitHub public API base for resolving a tag/ref to its
// commit (`<base>/<ref>` => the GitHub commit object). A package var so tests
// can point it at an httptest server.
//
// SECURITY: like latestReleaseURL, this is UNAUTHENTICATED — never tokened.
var commitsBaseURL = "https://api.github.com/repos/civitai/cli/commits"

// updateCheckTimeout bounds the GitHub round-trip. Kept short so `civitai
// version` never hangs on a slow/offline network.
const updateCheckTimeout = 2500 * time.Millisecond

// contextWithUpdateTimeout returns a context bounded by updateCheckTimeout for
// the unauthenticated GitHub round-trip. Shared by the synchronous `version`
// check and the detached background refresh.
func contextWithUpdateTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), updateCheckTimeout)
}

// updateCheckDisabled reports whether the GitHub update check should be skipped:
// the --no-update-check flag (noFlag) or a non-empty CIVITAI_NO_UPDATE_CHECK env
// var both opt out.
func updateCheckDisabled(noFlag bool) bool {
	return noFlag || os.Getenv("CIVITAI_NO_UPDATE_CHECK") != ""
}

// printUpdateNotice queries GitHub for the latest release and, if it's newer
// than current, prints a one-line notice to w. It is deliberately best-effort:
// any network error, non-200, or parse failure is swallowed silently (nothing
// printed, never an error) so it can never break or slow down `civitai version`.
//
// current is the resolved running version (may be "dev"/a pseudo-version).
func printUpdateNotice(w io.Writer, current string) {
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()

	latest, err := fetchLatestRelease(ctx, latestReleaseURL)
	if err != nil || latest == "" {
		return // fail-silent: offline, timeout, non-200, or parse error.
	}

	// If we can't parse latest, the check effectively failed — stay silent
	// rather than make an unfounded "up to date" claim (fail-silent contract).
	if !isParseableVersion(latest) {
		return
	}

	switch compareVersions(current, latest) {
	case -1:
		// current < latest (or current is unparseable) => newer is available.
		if isParseableVersion(current) {
			fmt.Fprintf(w, "\nA newer version is available: %s (you have %s) — https://github.com/civitai/cli/releases/latest\n", latest, current)
		} else {
			fmt.Fprintf(w, "\nLatest release: %s — https://github.com/civitai/cli/releases/latest\n", latest)
		}
		fmt.Fprintln(w, "Upgrade with: brew upgrade civitai")
	case 0:
		// Verified equal: both current and latest parse and match. This is the
		// ONLY path that may honestly claim the user is up to date.
		fmt.Fprintln(w, "\nYou're on the latest version.")
	default:
		// current > latest (dev/pre-release build ahead of the newest release):
		// don't claim "on the latest version" — say nothing actionable, just
		// surface the latest release informationally and honestly.
		fmt.Fprintf(w, "\nLatest release: %s — https://github.com/civitai/cli/releases/latest\n", latest)
	}
}

// fetchLatestRelease does the unauthenticated GitHub call and returns the
// release tag_name (e.g. "v0.1.10"). Returns an error on any non-200 or
// transport/parse failure; callers treat that as "skip the notice".
func fetchLatestRelease(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// NOTE: intentionally NO Authorization header — public, token-free call.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases: status %d", resp.StatusCode)
	}
	// Cap the body so a misbehaving endpoint can't make us read forever.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

// enrichBuildInfoFromGitHub fills the commit and/or date from GitHub when a
// go-install build left them unresolved AND the resolved version is a clean,
// parseable release tag (a tagged `go install module@vX.Y.Z` build: the module
// proxy has no VCS checkout, so the binary carries no commit/date, and the
// pseudo-version offline path can't help because the version is a plain tag).
//
// It is deliberately best-effort and reuses the same contract as the update
// check: a single UNAUTHENTICATED GET (no Authorization header), the shared
// short context timeout, a 1 MiB body cap, and fail-silent on ANY error /
// non-200 / timeout / parse failure (the inputs are returned unchanged). It
// only fills fields that are still at their defaults; ldflag/vcs/pseudo values
// already resolved upstream are never overwritten.
//
// It does NOTHING (no HTTP call) when:
//   - both commit and date are already resolved (a fully-stamped build), or
//   - the version isn't a clean parseable tag (dev / pseudo-version / empty), or
//   - the update check is disabled (--no-update-check / CIVITAI_NO_UPDATE_CHECK).
func enrichBuildInfoFromGitHub(commitIn, dateIn, version string, disabled bool) (commitOut, dateOut string) {
	commitOut, dateOut = commitIn, dateIn
	if disabled {
		return
	}
	if commitIn != "none" && dateIn != "unknown" {
		return // fully stamped — nothing to enrich, make no call.
	}
	if !isCleanReleaseTag(version) {
		return // dev / pseudo-version / empty — not a clean tag we can resolve.
	}

	ctx, cancel := contextWithUpdateTimeout()
	defer cancel()

	sha, date, err := fetchCommitForRef(ctx, commitsBaseURL, version)
	if err != nil {
		return // fail-silent: offline, timeout, non-200, or parse error.
	}
	if commitOut == "none" && sha != "" {
		commitOut = shortenRevision(sha)
	}
	if dateOut == "unknown" && date != "" {
		dateOut = date
	}
	return
}

// fetchCommitForRef does the unauthenticated GitHub call to resolve a ref (tag)
// to its commit, returning the full SHA and the committer (or author) date in
// RFC3339. Returns an error on any non-200 or transport/parse failure; callers
// treat that as "skip the enrichment".
func fetchCommitForRef(ctx context.Context, base, ref string) (sha, date string, err error) {
	url := strings.TrimRight(base, "/") + "/" + ref
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// NOTE: intentionally NO Authorization header — public, token-free call.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github commits: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	var c struct {
		SHA    string `json:"sha"`
		Commit struct {
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
			Author struct {
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return "", "", err
	}
	date = c.Commit.Committer.Date
	if date == "" {
		date = c.Commit.Author.Date
	}
	return c.SHA, date, nil
}

// semver is a minimal parsed vMAJOR.MINOR.PATCH triple. Pre-release/build
// suffixes (-rc1, +meta) are ignored for comparison.
type semver struct {
	major, minor, patch int
}

// parseSemver parses a "vMAJOR.MINOR.PATCH" (or "MAJOR.MINOR.PATCH") string,
// tolerating a leading "v" and ignoring any "-prerelease"/"+build" suffix.
// Missing minor/patch default to 0 (so "v1" and "v1.2" parse). Returns ok=false
// for anything that isn't a numeric dotted version (e.g. "dev", a Go
// pseudo-version, or a git describe like "v0.1.1-3-gabc123").
func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return semver{}, false
	}
	// Strip build metadata first, then pre-release.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return semver{}, false
	}
	var nums [3]int
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			continue // missing minor/patch => 0
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return semver{}, false
		}
		nums[i] = n
	}
	return semver{nums[0], nums[1], nums[2]}, true
}

// isParseableVersion reports whether s looks like a real semver we can compare.
func isParseableVersion(s string) bool {
	_, ok := parseSemver(s)
	return ok
}

// isCleanReleaseTag reports whether s is a real release tag we could resolve to
// a GitHub commit — i.e. a parseable semver that is NOT a Go pseudo-version.
// (parseSemver alone says "yes" to a pseudo-version because it strips at the
// first '-', so we must exclude pseudo-versions explicitly; their commit/date
// is recovered offline instead, not via the GitHub commits API.)
func isCleanReleaseTag(s string) bool {
	if _, _, ok := parsePseudoVersion(s); ok {
		return false
	}
	return isParseableVersion(s)
}

// compareVersions compares current vs latest by semver. Returns:
//
//	-1  current is older than latest, OR current is unparseable (treat any real
//	    release as "available" when we can't tell what the user is running)
//	 0  equal
//	 1  current is newer than latest
//
// If latest itself is unparseable, returns 0 (nothing to recommend).
func compareVersions(current, latest string) int {
	lv, lok := parseSemver(latest)
	if !lok {
		return 0
	}
	cv, cok := parseSemver(current)
	if !cok {
		return -1 // unknown current => surface the latest as available.
	}
	switch {
	case cv.major != lv.major:
		return cmpInt(cv.major, lv.major)
	case cv.minor != lv.minor:
		return cmpInt(cv.minor, lv.minor)
	default:
		return cmpInt(cv.patch, lv.patch)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
