// Command caskcheck asserts the ONE invariant the Homebrew channel has to hold:
//
//	the cask in civitai/homebrew-tap must never name a version whose archives
//	are not publicly downloadable.
//
// It exists because the mechanism that upholds that invariant can regress, and a
// mechanism regressing is silent. On 2026-08-09 it did: `.goreleaser.yaml` sets
// `release.draft: true` on purpose, but the SAME goreleaser run that created the
// draft also pushed the cask bump to the tap, so from the v0.1.91 tag push
// (01:09Z) until a human clicked "Publish release" hours later, the cask said
//
//	version "0.1.91"
//	GET .../download/v0.1.91/civitai_0.1.91_linux_amd64.tar.gz -> 404
//	GET .../download/v0.1.90/civitai_0.1.90_linux_amd64.tar.gz -> 200
//
// and `brew install civitai/tap/civitai` failed for every user in that window.
// The fix moves the tap push to `release: published` (release-homebrew.yml).
// This is the assertion that the fix — and whatever replaces it — still holds.
//
// # Why a real HTTP request, and why it must be UNAUTHENTICATED
//
// The release API is NOT evidence here, and reading the invariant off it is the
// mistake this whole command exists to avoid. A draft release is a real release
// object: `GET /repos/civitai/cli/releases/tags/v0.1.91` answers 200 for anyone
// holding a repo token — and CI always holds one. So an API-shaped check would
// have reported the pipeline healthy for the entire two-hour outage.
//
// What a user's `brew install` does is fetch the archive URL the cask names, as
// nobody in particular. So that is exactly what this asks, with no Authorization
// header (TestArchiveProbesCarryNoCredential pins it), and the status code it
// gets back is the verdict. A token is used for the releases API call and ONLY
// there, where it buys rate limit and buys nothing else.
//
// The two status codes this turns on were re-verified live against github.com on
// 2026-08-09, after the draft had been published: a version that does not exist
// (v9.9.9) answers 404 and v0.1.91 answers 200. The 0.1.91-while-draft 404 in
// the window above is the maintainer's measurement, not one taken here — it
// cannot be re-taken without un-publishing a release.
//
// # What it deliberately does NOT fail on
//
// A cask that LAGS the latest published release (cask 0.1.90, latest 0.1.91) is
// green. That state is not a violation: 0.1.90 is downloadable, so no user is
// broken. It is also the correct state for the entire window between a tag push
// and the maintainer publishing the draft, which under the new ordering is the
// normal path — failing on it would make the daily run red for doing the right
// thing, and a permanently-red gate is worse than no gate. The latest published
// tag is read for the failure MESSAGE, so a red run names both versions the way
// the incident above is written, and not to decide anything.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	// defaultCaskURL is the file `brew` itself reads, on the tap's default
	// branch. `HEAD` rather than `main` so a tap that renames its default branch
	// does not turn this into a 404 that reads like a real finding.
	defaultCaskURL = "https://raw.githubusercontent.com/civitai/homebrew-tap/HEAD/Casks/civitai.rb"

	// defaultLatestURL resolves the latest NON-draft, non-prerelease release.
	// Diagnostic only — see the package comment.
	defaultLatestURL = "https://api.github.com/repos/civitai/cli/releases/latest"
)

// caskVersionRe and caskURLRe read the two things a cask has to agree about.
// Both are anchored to the start of a line so a version or url mentioned inside
// a comment or a string cannot be picked up as the stanza.
var (
	caskVersionRe = regexp.MustCompile(`(?m)^\s*version\s+"([^"]+)"`)
	caskURLRe     = regexp.MustCompile(`(?m)^\s*url\s+"([^"]+)"`)
	latestTagRe   = regexp.MustCompile(`"tag_name"\s*:\s*"([^"]+)"`)
)

// checker carries the seams. Every field is injectable so the tests drive the
// real code against httptest servers rather than a reimplementation of it.
type checker struct {
	caskURL   string
	latestURL string
	// apiToken is attached to the releases API request and to NOTHING else.
	apiToken string
	client   *http.Client
}

// probe is one archive URL and what asking for it, as a stranger, returned.
type probe struct {
	url    string
	status int
	err    error
}

func (p probe) ok() bool {
	return p.err == nil && (p.status == http.StatusOK || p.status == http.StatusPartialContent)
}

func (p probe) String() string {
	if p.err != nil {
		return fmt.Sprintf("%s -> %v", p.url, p.err)
	}
	return fmt.Sprintf("%s -> HTTP %d", p.url, p.status)
}

// report is what a run learned, whether or not it is a finding.
type report struct {
	caskVersion     string
	latestPublished string // "" when the diagnostic lookup failed
	probes          []probe
}

// errUnreachableTap is returned when the tap could not be read at all.
//
// 🔴 It is a distinct sentinel because "we could not ask" and "we asked and the
// answer was fine" are the two states a network check most easily confuses, and
// only one of them is allowed to look like success. There is no degraded mode
// here: a run that cannot read the cask has measured nothing and says so.
var errUnreachableTap = errors.New("could not read the cask from the tap")

// errNothingChecked is the positive control. A cask that parses to zero archive
// URLs would sail through the loop below having asked no questions, and a run
// that asks nothing cannot fail — the reassuring zero. It is a hard error.
var errNothingChecked = errors.New("parsed 0 archive URLs out of the cask")

func (c *checker) get(ctx context.Context, url string, auth bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if auth && c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	return c.client.Do(req)
}

// fetchCask reads the cask exactly as a reader of the tap would.
func (c *checker) fetchCask(ctx context.Context) (string, error) {
	resp, err := c.get(ctx, c.caskURL, false)
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, c.caskURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w (%s): HTTP %d", errUnreachableTap, c.caskURL, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, c.caskURL, err)
	}
	return string(b), nil
}

// latestPublishedTag is diagnostic. Its failure is recorded in the report rather
// than returned, because the invariant does not depend on it and a rate-limited
// API must not be able to turn a healthy pipeline red.
func (c *checker) latestPublishedTag(ctx context.Context) string {
	resp, err := c.get(ctx, c.latestURL, true)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ""
	}
	if m := latestTagRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// probeArchive asks for one archive as a stranger. The body is closed
// immediately: the status line is the whole answer and these are multi-megabyte
// files, so nothing is read.
func (c *checker) probeArchive(ctx context.Context, url string) probe {
	resp, err := c.get(ctx, url, false)
	if err != nil {
		return probe{url: url, err: err}
	}
	defer resp.Body.Close()
	return probe{url: url, status: resp.StatusCode}
}

// check runs the whole assertion. A non-nil error is a finding OR a control
// failure; the two are told apart by the sentinels above.
func (c *checker) check(ctx context.Context) (*report, error) {
	body, err := c.fetchCask(ctx)
	if err != nil {
		return nil, err
	}

	m := caskVersionRe.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("the cask at %s has no `version \"...\"` stanza — this check cannot read it, which is not the same as it being fine", c.caskURL)
	}
	rep := &report{caskVersion: m[1]}

	var urls []string
	for _, u := range caskURLRe.FindAllStringSubmatch(body, -1) {
		// Homebrew interpolates `#{version}` at install time; do the same, so
		// what is requested here is byte-for-byte what brew would request.
		urls = append(urls, strings.ReplaceAll(u[1], "#{version}", rep.caskVersion))
	}
	if len(urls) == 0 {
		return rep, fmt.Errorf("%w (%s) — a run that asks for nothing cannot fail, so this is a broken check, not a clean pipeline", errNothingChecked, c.caskURL)
	}

	for _, u := range urls {
		rep.probes = append(rep.probes, c.probeArchive(ctx, u))
	}

	var bad []probe
	for _, p := range rep.probes {
		if !p.ok() {
			bad = append(bad, p)
		}
	}
	if len(bad) == 0 {
		return rep, nil
	}

	rep.latestPublished = c.latestPublishedTag(ctx)
	latest := rep.latestPublished
	if latest == "" {
		latest = "unknown (the releases API could not be read; the failure below stands on its own)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "the Homebrew cask names version %q, and %d of its %d archive URL(s) are NOT publicly downloadable.\n",
		rep.caskVersion, len(bad), len(rep.probes))
	fmt.Fprintf(&b, "latest PUBLISHED (non-draft) release: %s\n", latest)
	for _, p := range bad {
		fmt.Fprintf(&b, "  %s\n", p)
	}
	b.WriteString("\n`brew install civitai/tap/civitai` is failing for every user right now.\n" +
		"The cask is only allowed to move when the GitHub Release is PUBLISHED — see .github/workflows/release-homebrew.yml.\n" +
		"If a draft release is sitting unpublished, publishing it resolves this; if the cask was pushed by something other than\n" +
		"that workflow, that is the regression.")
	return rep, errors.New(b.String())
}

func main() {
	caskURL := flag.String("cask-url", defaultCaskURL, "URL of the cask file as published by the tap")
	latestURL := flag.String("latest-url", defaultLatestURL, "GitHub API URL for the latest published release (diagnostic only)")
	timeout := flag.Duration("timeout", 60*time.Second, "overall timeout")
	flag.Parse()

	c := &checker{
		caskURL:   *caskURL,
		latestURL: *latestURL,
		apiToken:  os.Getenv("GITHUB_TOKEN"),
		client:    &http.Client{Timeout: 30 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	rep, err := c.check(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: cask version %s; %d archive URL(s) checked, all publicly downloadable\n", rep.caskVersion, len(rep.probes))
	for _, p := range rep.probes {
		fmt.Printf("  %s\n", p)
	}
}
