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
// ⚠ EACH ENTRY RECORDS HOW IT GOT HERE, because an earlier version of this
// comment claimed "EVERY ENTRY HERE WAS PUT IN BY A MEASURED RED" and that was
// FALSE of two of the three — a comment certifying a discipline the table did
// not follow, which is exactly the sentence that stops the next reader
// questioning an entry.
var notAPage = map[string]string{
	// MEASURED 405 to a plain GET. It is a JSON-RPC transport, not a page, so a
	// GET is not the protocol and the status says nothing about the docs. This
	// is load-bearing: 405 falls through to the >= 400 arm and would fail.
	"https://mcp.civitai.com/mcp": "MCP transport endpoint; measured 405 to a GET",

	// The SIBLING of the row above, and it was simply missed: `### The two MCP
	// servers` documents TWO transports and only one was excluded, so this guard
	// has been red on every live run since that section landed — a red it reports
	// as "the page did not serve", which reads like drift rather than like a hole
	// in this table. MEASURED 2026-09-25: 405 to a plain GET, byte-identical
	// treatment to mcp.civitai.com/mcp above. README.md's own row for it says it
	// "returns 401 until an Authorization header is present", which is the same
	// statement that a GET is not the protocol.
	"https://orchestration.civitai.com/mcp": "MCP transport endpoint; measured 405 to a GET",

	// MEASURED RED on the guard's first live run.
	"https://github.com/me/my-app": "placeholder repo URL in a command example",
}

// placeholderHosts are hosts the README uses ON PURPOSE to mean "your address
// here". `example.com` is reserved by RFC 2606 precisely so it can be written
// without resolving, and README.md itself documents that `--image` validates
// against it as a reserved host — so a 404 from it is the specification working,
// not drift.
//
// ⚠ ONLY HOSTS THE README ACTUALLY USES. Three anticipatory entries
// (`www.example.com`, `example.net`, `example.org`) were removed after measuring
// zero matches each: an exclusion that excludes nothing is indistinguishable
// from one that is load-bearing, and the list is where this guard narrows.
// README.md names `.net`/`.org` in PROSE as reserved hosts; it links neither.
var placeholderHosts = map[string]bool{
	"example.com":    true,
	"img.shields.io": true, // badge image service, not documentation
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

// readmeMinExternalURLs is an anti-vacuity floor: it answers "is the extractor
// still reading URLs at all?", and NOTHING else. The live count at the time of
// writing was 38 (cross-checked against an independent shell extraction applying
// the same exclusions), so 20 is slack by design.
//
// 🔴 IT WAS 37 — THE EXACT COUNT — AND THAT WAS THE WRONG SHAPE. Set on the
// count, this fires on any honest link removal with a message telling you to
// lower the constant, which is a change-detector that ratchets, not an
// invariant. The #652 lesson it cited ("set the floor at the ACTUAL count") was
// about a floor too SLACK to see a mutant de-converting two arms; applying it
// here overshot, because the property at risk is different.
//
// 🔴 WHAT ACTUALLY DEFENDS THIS GUARD IS THE POSITIVE CONTROL BELOW, not the
// number. A broken regex returns ~0 and trips 20 just as surely as 37; a regex
// that silently matched only github.com would pass ANY count, and only the
// `mustFind` assertion catches it. Raise this only if it stops being able to
// distinguish "the extractor broke" from "a link was removed".
const readmeMinExternalURLs = 20

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
	// Without this, a regex that matched 30 GitHub URLs and missed every
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
			// 🔴 NOT A DEAD LINK, AND MEASURED: www.npmjs.com answered 403 to
			// this checker and to curl under a browser user-agent, while the
			// page is live in a browser. ⚠ It is INTERMITTENT — two runs an
			// hour apart 403'd a different subset of the same two npm URLs —
			// which is the argument FOR this arm, not against it: a host that
			// refuses some requests and not others is precisely what must never
			// decide a gate's colour. An anti-bot or rate-limit refusal is the
			// host declining to ANSWER, which is the "we could not ask" case;
			// failing on it would make this permanently and randomly red, the
			// one outcome worse than no gate.
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
