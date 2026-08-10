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
// # A cask that LAGS is green for a day, and a finding after that
//
// A cask naming an older-but-downloadable version is NOT the outage above: 0.1.90
// installs fine, nobody is broken, and it is the correct state for the whole
// window between a tag push and the maintainer publishing the draft. Failing on
// it would make the daily run red for doing the right thing.
//
// It stops being normal once the release has been PUBLISHED and the cask still
// has not followed, because the only thing that moves the cask is the
// `release: published` trigger in release-homebrew.yml. A cask still lagging a
// day later therefore means that trigger did not reach its job — a dropped
// webhook (an Actions incident did exactly that to this repo's push/PR triggers
// on 2026-08-06), a failed push nobody read, an expired tap PAT. Nothing else in
// this pipeline notices that, because a lagging cask installs cleanly.
//
// The threshold is measured, not guessed. Over the 30 most recent releases of
// this repo the gap from `published_at` to the tap commit is ~1 minute, and the
// gap from tag to publish — a strictly larger window that this check does not
// even measure — has a median of ~2m30s, a second-worst of 22m46s (v0.1.80) and
// a worst of 1h55m (v0.1.91, the incident itself). 24h is ~1400x the normal
// push latency and >12x the worst publish latency ever recorded here. It is
// also at least the check's own period, so "the release published after
// yesterday's run" can never be reported as a lag.
//
// # Classification, and why it is written to a FILE
//
// A caller has to be able to tell "brew is broken for users" from "we could not
// ask" from "the pipeline did not move the cask", because those three want
// different words in front of a human. That classification does NOT ride the
// exit code, for a measured reason: on go1.25.12 `go run` collapses EVERY
// non-zero exit to 1 (`os.Exit(3)` and `os.Exit(11)` both produce rc=1, printing
// `exit status N` to stderr; the same program built with `go build` propagates 3
// and 11). A workflow invoking this with `go run` would therefore read every
// verdict as the same one. So the kind goes to `-kind-file`, and the exit code
// stays the plain 0/1 it has always been.
//
// A caller that finds NO kind file after a non-zero exit has learned something
// real — this command did not reach its own exit path — and must report that as
// its own state rather than as any verdict here.
//
// # The cask is read at a PINNED COMMIT, because a mutable ref served STALE
//
// raw.githubusercontent.com sits behind Fastly and answers
// `vary: Authorization,Accept-Encoding`, so `.../homebrew-tap/HEAD/Casks/civitai.rb`
// is not one cached object but one PER ENCODING PER EDGE NODE. Measured on
// 2026-08-10, minutes after v0.1.92 was published and the cask bumped, with
// `git ls-remote` giving the tap's true HEAD as df4801b:
//
//	curl                        -> version "0.1.92"  etag  "a2ee…"  x-cache HIT
//	curl --compressed           -> version "0.1.91"  etag W/"a4d5…"  x-cache HIT
//	net/http (adds gzip itself)  -> version "0.1.91"
//	caskcheck                    -> OK: cask version 0.1.91 … all downloadable
//
// Go's transport adds `Accept-Encoding: gzip` unless the caller set the header,
// so this command always read the gzip variant, and the gzip variant was a
// release behind. That is green straight through the outage this command was
// built to catch: if a cask push introduces a BROKEN version, a run reading the
// previous variant validates the previous, working version's URLs and exits 0.
// It did exactly that above — reporting "cask version 0.1.91, all publicly
// downloadable" while the cask said 0.1.92.
//
// Three things were measured and rejected as the fix, each of which looks like
// it should work:
//
//   - `Accept-Encoding: identity`. Reads the OTHER variant, and there is no
//     evidence that variant is systematically fresher. Staleness here is per
//     EDGE NODE: in the same minute, `cache-yyz4539` served the gzip variant a
//     release behind on a HIT while `cache-yyz4528` MISSed and fetched 0.1.92
//     from the origin. Picking a variant picks a different lottery ticket.
//   - A cache-busting query parameter. raw.githubusercontent.com does not put
//     the query string in its cache key: `?cb=…` returned the same stale body
//     and the same weak ETag as the bare URL, under both encodings.
//   - `Cache-Control: no-cache` (with and without `Pragma`) on the request.
//     Fastly does not honour it here; same stale body, same ETag.
//
// So the fix does not try to defeat the cache — it removes the mutable object.
// `-cask-url` carries a `{commit}` placeholder, and the run first resolves the
// tap's HEAD over git's smart-HTTP ref advertisement, which is served by
// GitHub's git backend rather than Fastly and is explicitly uncacheable
// (`cache-control: no-cache, max-age=0, must-revalidate`, `expires` in 1980, no
// `via: varnish` and no `x-cache` on the response). The cask is then read at
// `.../homebrew-tap/<40-hex sha>/Casks/civitai.rb`, whose content is IMMUTABLE
// by construction: there is no earlier body for that cache key to be stale
// from, so a HIT is correct by definition and every encoding variant agrees.
// Verified live — both variants of the pinned URL returned 0.1.92.
//
// This is still exactly what a reader of the tap gets, and still as a stranger:
// same file, same host, no Authorization on either request. It is one extra
// unauthenticated GET, and it buys a property the mutable URL cannot have. The
// resolved commit is printed, so a run says which snapshot it judged.
//
// A `-cask-url` with NO placeholder is used verbatim and skips the lookup. That
// is what the fire drill in release-homebrew.yml passes, and those fixtures are
// already pinned to a commit sha themselves; a run says which mode it took
// rather than degrading silently.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// caskCommitPlaceholder is what a -cask-url puts where a git ref goes. It is
	// replaced by the tap's HEAD commit before anything is fetched — see the
	// package comment for the stale-variant measurement that requires it. A URL
	// without it is used verbatim.
	caskCommitPlaceholder = "{commit}"

	// defaultCaskURL is the file `brew` itself reads, read at the commit the
	// tap's default branch points at right now rather than at the mutable
	// `HEAD` ref, which measured a release behind. Resolving the ref rather
	// than naming `main` also keeps a tap that renames its default branch from
	// turning this into a 404 that reads like a real finding.
	defaultCaskURL = "https://raw.githubusercontent.com/civitai/homebrew-tap/" + caskCommitPlaceholder + "/Casks/civitai.rb"

	// defaultTapRefsURL is git's smart-HTTP ref advertisement for the tap — what
	// `git ls-remote` reads. Unauthenticated (the tap is public) and served by
	// GitHub's git backend, which sets `cache-control: no-cache, max-age=0,
	// must-revalidate` and sits behind no CDN, so unlike the raw host it cannot
	// answer with a previous revision.
	defaultTapRefsURL = "https://github.com/civitai/homebrew-tap.git/info/refs?service=git-upload-pack"

	// defaultLatestURL resolves the latest NON-draft, non-prerelease release.
	// It feeds the failure MESSAGE and the lag finding — never the invariant.
	defaultLatestURL = "https://api.github.com/repos/civitai/cli/releases/latest"

	// defaultLagThreshold — see the package comment for the measurement behind
	// this number. Do not lower it below the interval this command is run at.
	defaultLagThreshold = 24 * time.Hour
)

// The kinds a run can end in. They are a CLOSED set written verbatim to the
// kind file, and a reader that does not recognise a token must treat it as
// unknown rather than guessing — a new kind added here reaches an old reader as
// "I could not classify this", which is true, instead of as a wrong verdict.
const (
	kindGreen        = "green"
	kindBroken       = "broken"
	kindLagging      = "lagging"
	kindUnmeasurable = "unmeasurable"
	// kindUnclassified is what an error carrying none of the sentinels below
	// becomes. It exists so that adding a new failure path cannot silently
	// inherit another kind's words: every branch of classify is a sentinel, and
	// the default arm is a visible state rather than a fallback.
	kindUnclassified = "unclassified"
)

// caskVersionRe and caskURLRe read the two things a cask has to agree about.
// Both are anchored to the start of a line so a version or url mentioned inside
// a comment or a string cannot be picked up as the stanza.
var (
	caskVersionRe = regexp.MustCompile(`(?m)^\s*version\s+"([^"]+)"`)
	caskURLRe     = regexp.MustCompile(`(?m)^\s*url\s+"([^"]+)"`)
)

// checker carries the seams. Every field is injectable so the tests drive the
// real code against httptest servers rather than a reimplementation of it.
type checker struct {
	// caskURL may carry caskCommitPlaceholder, in which case tapRefsURL is
	// consulted first and the placeholder replaced by the resolved commit.
	caskURL string
	// tapRefsURL is read with NO credential, exactly like the cask itself.
	tapRefsURL string
	latestURL  string
	// apiToken is attached to the releases API request and to NOTHING else.
	apiToken     string
	client       *http.Client
	lagThreshold time.Duration
	// now is a seam so the lag threshold can be exercised without sleeping.
	now func() time.Time
}

func (c *checker) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *checker) lagAfter() time.Duration {
	if c.lagThreshold > 0 {
		return c.lagThreshold
	}
	return defaultLagThreshold
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

// release is the diagnostic lookup's answer. `known` is separate from the
// fields because "the API could not be read" and "the API said nothing was
// published" must not collapse into the same zero value — the lag finding is
// only allowed to fire on a positively known publish time.
type release struct {
	tag         string
	publishedAt time.Time
	known       bool
}

// report is what a run learned, whether or not it is a finding.
type report struct {
	caskVersion string
	// caskURL is the URL actually requested, and caskCommit the tap commit it
	// was pinned to — empty when -cask-url named no placeholder. Both are
	// printed: a verdict about a snapshot has to say which snapshot.
	caskURL         string
	caskCommit      string
	latestPublished string // "" when the diagnostic lookup failed
	probes          []probe
	// lagEvaluated records whether the lag question could be ASKED at all. A
	// green run that could not ask says so, rather than printing a clean line
	// that reads as "checked, and fine".
	lagEvaluated bool
	lagWhy       string
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

// errUnreadableCask is the same shape one field up: a cask whose version stanza
// cannot be read has not been measured either.
var errUnreadableCask = errors.New("could not read the cask's version stanza")

// errArchivesNotDownloadable is THE invariant violation — the 2026-08-09 state.
// It is a sentinel rather than the classifier's default arm on purpose: a
// default arm would hand every future error shape the words "brew install is
// failing for every user right now", which is the one claim in here that must
// only ever be made on evidence.
var errArchivesNotDownloadable = errors.New("the cask names archives that are not publicly downloadable")

// errCaskLags is the pipeline-did-not-move-the-cask finding. Deliberately NOT
// the same kind as errArchivesNotDownloadable: no user is broken by it, so the
// two must not print the same sentence.
var errCaskLags = errors.New("the cask has not followed the latest published release")

// classify maps a verdict onto the closed kind set above. Every branch is an
// `errors.Is` on a sentinel — never a match on message text, which would be a
// guard a reworded message walks straight past.
func classify(err error) string {
	switch {
	case err == nil:
		return kindGreen
	case errors.Is(err, errCaskLags):
		return kindLagging
	case errors.Is(err, errUnreachableTap), errors.Is(err, errNothingChecked), errors.Is(err, errUnreadableCask):
		return kindUnmeasurable
	case errors.Is(err, errArchivesNotDownloadable):
		return kindBroken
	default:
		return kindUnclassified
	}
}

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

// advHeadCommitRe reads the commit HEAD points at out of a smart-HTTP ref
// advertisement. A ref line is `<4-hex pkt length><40-hex sha> <name>` with an
// optional NUL-separated capability list, and HEAD is the first one. The match
// is anchored on the literal " HEAD" terminator rather than on an offset,
// because the pkt-line length prefix is itself hex and runs straight into the
// sha (`00000159df4801…`) — the 40 characters ending immediately before " HEAD"
// are the sha and nothing else can be. A ref merely NAMED HEAD
// (`<sha> refs/heads/HEAD`) does not match: the sha is not adjacent to " HEAD".
var advHeadCommitRe = regexp.MustCompile(`([0-9a-f]{40}) HEAD(\x00|\n|$)`)

// tapCommit resolves the tap's default branch to a commit, over the endpoint
// `git ls-remote` uses. Its failure is errUnreachableTap — not a new kind —
// because "we could not resolve the tap" and "we could not read the cask" are
// the same state to a caller: nothing was measured. There is deliberately no
// fallback to the mutable ref, which would be a silent return to the defect.
func (c *checker) tapCommit(ctx context.Context) (string, error) {
	resp, err := c.get(ctx, c.tapRefsURL, false)
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, c.tapRefsURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w (%s): HTTP %d", errUnreachableTap, c.tapRefsURL, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, c.tapRefsURL, err)
	}
	adv := string(b)
	// Assert the INSTRUMENT before reading its answer: an HTML error page, a
	// login redirect or a protocol-v2 reply would all simply fail to match
	// below, and "no HEAD found" is a much worse diagnosis than "that was not a
	// ref advertisement".
	if !strings.Contains(adv, "# service=git-upload-pack") {
		return "", fmt.Errorf("%w (%s): the response is not a git ref advertisement (no `# service=git-upload-pack` banner) — this run resolved no tap commit and has measured nothing", errUnreachableTap, c.tapRefsURL)
	}
	m := advHeadCommitRe.FindStringSubmatch(adv)
	if m == nil {
		return "", fmt.Errorf("%w (%s): the ref advertisement names no HEAD commit", errUnreachableTap, c.tapRefsURL)
	}
	return m[1], nil
}

// resolveCaskURL turns -cask-url into the URL this run will actually request,
// pinning it to a commit when it carries the placeholder. The returned commit is
// "" for a verbatim URL, which the report prints rather than hides.
func (c *checker) resolveCaskURL(ctx context.Context) (url, commit string, err error) {
	if !strings.Contains(c.caskURL, caskCommitPlaceholder) {
		return c.caskURL, "", nil
	}
	commit, err = c.tapCommit(ctx)
	if err != nil {
		return "", "", err
	}
	return strings.ReplaceAll(c.caskURL, caskCommitPlaceholder, commit), commit, nil
}

// fetchCask reads the cask exactly as a reader of the tap would — same file,
// same host, no credential — at the immutable URL resolveCaskURL produced.
func (c *checker) fetchCask(ctx context.Context, url string) (string, error) {
	resp, err := c.get(ctx, url, false)
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w (%s): HTTP %d", errUnreachableTap, url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w (%s): %w", errUnreachableTap, url, err)
	}
	return string(b), nil
}

// latestRelease is diagnostic. Its failure is recorded in the report rather
// than returned, because the invariant does not depend on it and a rate-limited
// API must not be able to turn a healthy pipeline red. The lag finding below
// therefore fires only when this returns `known` — never on its absence.
func (c *checker) latestRelease(ctx context.Context) release {
	resp, err := c.get(ctx, c.latestURL, true)
	if err != nil {
		return release{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return release{}
	}
	var payload struct {
		TagName string `json:"tag_name"`
		// A POINTER, so a payload with no `published_at` key is distinguishable
		// from one carrying the zero time. A value type here would let a
		// missing key read as "published at the epoch", i.e. maximally stale,
		// and manufacture a lag finding out of a field the server did not send.
		PublishedAt *time.Time `json:"published_at"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return release{}
	}
	if payload.TagName == "" {
		return release{}
	}
	r := release{tag: payload.TagName, known: true}
	if payload.PublishedAt != nil {
		r.publishedAt = *payload.PublishedAt
	}
	return r
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

// lagFinding answers "has the cask followed the latest published release?" and
// is allowed to say NOTHING in three separate cases, each of which is recorded
// on the report rather than silently treated as a pass.
func (c *checker) lagFinding(rep *report, rel release) error {
	switch {
	case !rel.known:
		rep.lagWhy = "not evaluated: the releases API could not be read, so there is no published version to compare against"
		return nil
	case rel.publishedAt.IsZero():
		rep.lagWhy = "not evaluated: the releases API returned no publish time for " + rel.tag
		return nil
	}
	rep.lagEvaluated = true

	want := strings.TrimPrefix(rel.tag, "v")
	if rep.caskVersion == want {
		rep.lagWhy = "cask matches the latest published release " + rel.tag
		return nil
	}
	age := c.clock().Sub(rel.publishedAt)
	if age <= c.lagAfter() {
		rep.lagWhy = fmt.Sprintf("cask %s != latest published %s, published %s ago — inside the %s grace window, which is the normal state right after a publish",
			rep.caskVersion, rel.tag, age.Round(time.Minute), c.lagAfter())
		return nil
	}
	rep.lagWhy = "LAGGING"

	return fmt.Errorf("%w.\n"+
		"cask version:                 %s\n"+
		"latest PUBLISHED release:     %s (published %s ago, at %s)\n"+
		"\nNobody's `brew install` is broken by this — %s is downloadable — so it is not the 2026-08-09 outage.\n"+
		"What it means is that the only thing allowed to move the cask, the `release: published` job in\n"+
		".github/workflows/release-homebrew.yml, has not moved it in over %s. Check that workflow's runs for %s:\n"+
		"a dropped `release` webhook, a failed push nobody read, or an expired HOMEBREW_TAP_GITHUB_TOKEN all look\n"+
		"exactly like this. `gh workflow run release-homebrew.yml -f tag=%s` re-runs the push for that release.\n"+
		"(If %s was deliberately never shipped to Homebrew, this check has no way to know that — say so here and\n"+
		"raise the threshold, currently %s, rather than leaving a permanently red run nobody reads.)",
		errCaskLags, rep.caskVersion, rel.tag, age.Round(time.Minute), rel.publishedAt.UTC().Format(time.RFC3339),
		rep.caskVersion, c.lagAfter(), rel.tag, rel.tag, rel.tag, c.lagAfter())
}

// check runs the whole assertion. A non-nil error is a finding OR a control
// failure; the two are told apart by the sentinels above, via classify.
func (c *checker) check(ctx context.Context) (*report, error) {
	caskURL, commit, err := c.resolveCaskURL(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.fetchCask(ctx, caskURL)
	if err != nil {
		return nil, err
	}

	m := caskVersionRe.FindStringSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("%w: the cask at %s has no `version \"...\"` stanza — this check cannot read it, which is not the same as it being fine", errUnreadableCask, caskURL)
	}
	rep := &report{caskVersion: m[1], caskURL: caskURL, caskCommit: commit}

	var urls []string
	for _, u := range caskURLRe.FindAllStringSubmatch(body, -1) {
		// Homebrew interpolates `#{version}` at install time; do the same, so
		// what is requested here is byte-for-byte what brew would request.
		urls = append(urls, strings.ReplaceAll(u[1], "#{version}", rep.caskVersion))
	}
	if len(urls) == 0 {
		return rep, fmt.Errorf("%w (%s) — a run that asks for nothing cannot fail, so this is a broken check, not a clean pipeline", errNothingChecked, caskURL)
	}

	// Read up front rather than lazily: the lag finding needs it on the GREEN
	// path too, and its failure is still never fatal.
	rel := c.latestRelease(ctx)
	rep.latestPublished = rel.tag

	for _, u := range urls {
		rep.probes = append(rep.probes, c.probeArchive(ctx, u))
	}

	var bad []probe
	for _, p := range rep.probes {
		if !p.ok() {
			bad = append(bad, p)
		}
	}
	if len(bad) > 0 {
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
		return rep, fmt.Errorf("%w: %s", errArchivesNotDownloadable, b.String())
	}

	return rep, c.lagFinding(rep, rel)
}

// writeKind records the classification for a caller that has to put different
// words in front of a human for each. It is written BEFORE the process exits and
// its own failure is reported rather than swallowed: a caller that reads no kind
// file treats the run as unclassifiable, so a silently unwritten file would
// downgrade a real verdict.
func writeKind(path, kind string) error {
	if path == "" {
		return nil
	}
	return os.WriteFile(path, []byte(kind+"\n"), 0o644)
}

func main() {
	caskURL := flag.String("cask-url", defaultCaskURL, "URL of the cask file as published by the tap; `"+caskCommitPlaceholder+"` is replaced by the tap's current HEAD commit, and a URL without it is used verbatim")
	tapRefsURL := flag.String("tap-refs-url", defaultTapRefsURL, "git smart-HTTP ref advertisement used to resolve the tap's HEAD commit (only read when -cask-url carries the placeholder)")
	latestURL := flag.String("latest-url", defaultLatestURL, "GitHub API URL for the latest published release (the failure message, and the lag finding)")
	lagThreshold := flag.Duration("lag-threshold", defaultLagThreshold, "how long after a release is PUBLISHED a cask that has not followed it becomes a finding")
	kindFile := flag.String("kind-file", "", "write the verdict's kind (green|broken|lagging|unmeasurable|unclassified) to this file")
	// The overall budget has to cover the ref lookup, the cask fetch and one
	// probe per archive, each of which can take the per-request 30s. Four
	// archives at 30s is 120s, plus 60s for the two tap reads, so the worst case
	// is now exactly 180s and a 3m budget would make a slow-but-healthy
	// github.com produce a red run — the false-red that trains people to ignore
	// a gate. 4m keeps headroom over the worst case and is still bounded.
	timeout := flag.Duration("timeout", 4*time.Minute, "overall timeout")
	flag.Parse()

	c := &checker{
		caskURL:      *caskURL,
		tapRefsURL:   *tapRefsURL,
		latestURL:    *latestURL,
		apiToken:     os.Getenv("GITHUB_TOKEN"),
		client:       &http.Client{Timeout: 30 * time.Second},
		lagThreshold: *lagThreshold,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	rep, err := c.check(ctx)
	kind := classify(err)
	if werr := writeKind(*kindFile, kind); werr != nil {
		fmt.Fprintf(os.Stderr, "could not write the kind file %s: %v\n", *kindFile, werr)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL (%s): %v\n", kind, err)
		os.Exit(1)
	}
	fmt.Printf("OK: cask version %s; %d archive URL(s) checked, all publicly downloadable\n", rep.caskVersion, len(rep.probes))
	fmt.Printf("read: %s\n", readProvenance(rep))
	for _, p := range sortedProbes(rep.probes) {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("lag: %s\n", rep.lagWhy)
}

// readProvenance says WHICH bytes the verdict above is about. A run that reads
// a mutable ref can be a release behind (see the package comment), so "pinned to
// commit X" versus "read verbatim" is part of the verdict, not decoration.
func readProvenance(rep *report) string {
	if rep.caskCommit == "" {
		return rep.caskURL + " (verbatim — -cask-url named no " + caskCommitPlaceholder + " placeholder, so no tap commit was resolved)"
	}
	return rep.caskURL + " (pinned to tap commit " + rep.caskCommit + ")"
}

// sortedProbes keeps the success output stable run to run. Map iteration is not
// involved, but the cask's own url order is, and a stable listing is what makes
// two runs' logs diffable.
func sortedProbes(ps []probe) []probe {
	out := append([]probe(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return out[i].url < out[j].url })
	return out
}
