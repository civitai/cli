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

// updateCheckTimeout bounds the GitHub round-trip. Kept short so `civitai
// version` never hangs on a slow/offline network.
const updateCheckTimeout = 2500 * time.Millisecond

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

	switch compareVersions(current, latest) {
	case -1:
		// current < latest (or current is unparseable) => newer is available.
		if isParseableVersion(current) {
			fmt.Fprintf(w, "\nA newer version is available: %s (you have %s) — https://github.com/civitai/cli/releases/latest\n", latest, current)
		} else {
			fmt.Fprintf(w, "\nLatest release: %s — https://github.com/civitai/cli/releases/latest\n", latest)
		}
		fmt.Fprintln(w, "Upgrade with: brew upgrade civitai")
	default:
		// current == latest or current > latest: nothing actionable.
		fmt.Fprintln(w, "\nYou're on the latest version.")
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
