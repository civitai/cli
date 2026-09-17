package cmd

// README external-link liveness guard.
//
// 🔴 WHY THIS SHIPS WITH THE LINKS, NOT AFTER THE FIRST 404.
//
// readme_contributor_links_test.go answers a different question: it checks that
// a link into a contributor-only doc is ABSOLUTE, so the npm/Homebrew reader
// does not get a relative path that 404s. It never dereferences anything. The
// absolute URLs it prescribes — and the developer.civitai.com pointers the
// read-path sections now lean on — are unchecked by every gate in this repo, and
// a published dead link is a defect no CI job would catch.
//
// That mattered less when the README restated the content it links to. It
// matters now: sections were deleted on the grounds that the CLI guide carries
// them, so a 404 on that guide is not a cosmetic blemish, it is content loss for
// the reader.
//
// NETWORK-GATED, following internal/scaffold/pins_guard_test.go: it runs only
// when CIVITAI_CHECK_README_LINKS=1, so the default `go test ./...` stays
// offline and hermetic.
//
// 🔴 THE TWO FAILURE MODES ARE DELIBERATELY NOT THE SAME, and conflating them is
// how a link guard becomes a permanently-red gate everyone clicks through:
//
//	unreachable (DNS, timeout, connection refused, 5xx) — "we could not ask".
//	            Logged and SKIPPED, never failed. A flaky network must not
//	            redden a docs gate.
//	404 / 410   — the server answered, definitively, that the page is gone.
//	            That is real drift, so it FAILS, exactly as the pins guard
//	            fails on ErrPkgNotFound rather than skipping it.

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// absoluteURLRe matches an absolute https URL anywhere in the README, including
// inside a markdown link target, a bare mention, or a code fence.
var absoluteURLRe = regexp.MustCompile(`https://[A-Za-z0-9._~:/?#@!$&*+,;=%-]+`)

// notAPage lists URLs that are not fetchable documentation and must not be
// dereferenced by this guard. Each entry states WHY, because an unexplained
// exclusion is how a guard silently narrows until it covers nothing.
//
// 🔴 EVERY ENTRY HERE WAS PUT IN BY A MEASURED RED, not by anticipation. The
// first live run of this guard failed on exactly these, which is the negative
// control that proves it can go red at all.
var notAPage = map[string]string{
	// The two MCP transport endpoints. They are JSON-RPC servers, not pages:
	// orchestration deliberately answers 401 without a credential (that refusal
	// is itself documented in `## Set up your coding agent`), and a plain GET is
	// not the protocol. Fetching them would assert nothing about the docs.
	"https://mcp.civitai.com/mcp":           "MCP transport endpoint, not a page",
	"https://orchestration.civitai.com/mcp": "MCP transport endpoint; 401 by design",

	// A deliberate placeholder in the `app listing set-source-repo` examples.
	// It is meant to look like a user's repo and must never resolve.
	"https://github.com/me/my-app": "placeholder repo URL in a command example",
}

// placeholderHosts are hosts the README uses ON PURPOSE to mean "your address
// here". `example.com` is reserved by RFC 2606 precisely so it can be written
// without resolving, and README.md itself documents that `--image` validates
// against it as a reserved host — so a 404 from it is the specification working,
// not drift.
var placeholderHosts = map[string]bool{
	"example.com":     true,
	"www.example.com": true,
	"example.net":     true,
	"example.org":     true,
	"img.shields.io":  true, // badge image service, not documentation
}

// readmeExternalURLs extracts the distinct absolute URLs worth dereferencing.
func readmeExternalURLs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	seen := map[string]bool{}
	for _, u := range absoluteURLRe.FindAllString(string(raw), -1) {
		// Markdown routinely ends a sentence right after a URL; trailing
		// punctuation is not part of the address.
		u = strings.TrimRight(u, ".,);:")
		if u == "" || notAPage[u] != "" {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil || placeholderHosts[parsed.Host] {
			continue
		}
		seen[u] = true
	}
	out := make([]string, 0, len(seen))
	for u := range seen {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// readmeMinExternalURLs is an anti-vacuity floor set at the ACTUAL count at the
// time of writing (37 after the read-path link-out), not a slack round number.
// Cross-checked against an independent shell extraction applying the same
// exclusions, which is what makes 37 a measurement rather than this regex's
// opinion of itself.
//
// 🔴 A SLACK FLOOR IS WHAT LETS LINKS VANISH UNNOTICED — the lesson #652 paid
// for with a `total < 5` floor against a real 7, under which a mutant could
// delete two arms in silence. Set it at the real count and treat moving it as a
// decision to justify in the same commit.
const readmeMinExternalURLs = 37

func TestREADMEExternalURLsAreExtractable(t *testing.T) {
	urls := readmeExternalURLs(t)

	if len(urls) < readmeMinExternalURLs {
		t.Fatalf("extracted only %d external URL(s) from README.md, want >= %d — either the "+
			"extractor stopped matching (every liveness check below would then be vacuous) "+
			"or links were removed. If they were removed on purpose, lower "+
			"readmeMinExternalURLs in the same commit and say why.",
			len(urls), readmeMinExternalURLs)
	}

	// POSITIVE CONTROL on the extractor, not just on the count: the guide the
	// read-path sections were deleted in favour of must be among what we found.
	// Without this, a regex that matched 37 GitHub URLs and missed every
	// developer.civitai.com one would pass the floor above.
	const mustFind = "https://developer.civitai.com/site/guide/cli"
	found := false
	for _, u := range urls {
		if u == mustFind {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CONTROL failure: %q is not among the %d extracted URLs. The read-path "+
			"sections point readers there instead of restating the content, so if that "+
			"link is gone the deletion lost the content rather than relocating it.",
			mustFind, len(urls))
	}
}

func TestREADMEExternalURLsResolve(t *testing.T) {
	if os.Getenv("CIVITAI_CHECK_README_LINKS") != "1" {
		t.Skip("network guard; set CIVITAI_CHECK_README_LINKS=1 to run")
	}

	urls := readmeExternalURLs(t)
	if len(urls) < readmeMinExternalURLs {
		t.Fatalf("extracted only %d URL(s) — the extractor is broken and this run would "+
			"report a serene pass over an unchecked README", len(urls))
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var checked, skipped int

	for _, u := range urls {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			cancel()
			t.Errorf("%s — malformed URL in README.md: %v", u, err)
			continue
		}
		// A default Go user-agent is 403'd by some doc hosts; identify honestly.
		req.Header.Set("User-Agent", "civitai-cli-readme-link-check")
		resp, err := client.Do(req)
		cancel()
		if err != nil {
			// "We could not ask" — never a failure. See the header comment.
			skipped++
			t.Logf("SKIP %s — unreachable (%v)", u, err)
			continue
		}
		resp.Body.Close()
		checked++

		switch {
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			t.Errorf("DEAD LINK — README.md points at %s, which answers %d.\n"+
				"The README is the shipped user contract and this address is published in it. "+
				"Fix the link, or restore the content it was pointing at.", u, resp.StatusCode)
		case resp.StatusCode >= 500:
			skipped++
			checked--
			t.Logf("SKIP %s — server error %d, treated as unreachable", u, resp.StatusCode)
		case resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests:
			// 🔴 NOT A DEAD LINK, AND MEASURED: www.npmjs.com answers 403 to
			// this checker AND to curl under a browser user-agent, while the
			// page is perfectly live in a browser. An anti-bot or rate-limit
			// refusal is the host declining to ANSWER, which is the
			// "we could not ask" case — failing on it would make this a
			// permanently-red gate, the one outcome worse than no gate.
			skipped++
			checked--
			t.Logf("SKIP %s — %d (access-controlled or bot-filtered, not a dead link)", u, resp.StatusCode)
		case resp.StatusCode >= 400:
			t.Errorf("README.md points at %s, which answers %d — not a 404, but the page did "+
				"not serve. Check whether the address moved.", u, resp.StatusCode)
		}
	}

	// Report the pair, never the reassuring zero alone: "0 dead links" is
	// indistinguishable from "nothing was fetched" unless the checked count is
	// shown beside it.
	t.Logf("liveness: %d URL(s) fetched, %d skipped as unreachable, out of %d extracted",
		checked, skipped, len(urls))
	if checked == 0 {
		t.Fatalf("fetched 0 of %d URLs — every request failed, so this run proves nothing "+
			"about link health. Treat it as no measurement, not as a pass.", len(urls))
	}
}
