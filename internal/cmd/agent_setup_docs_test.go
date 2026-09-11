package cmd

// The managed block's `### Docs` section: what it links, and the one rule about
// how it links it.
//
// 🔴 THE SECTION NAMES ONE URL PER RESOURCE, AND NEVER A LIST OF REPOSITORIES.
// The example App Blocks live in seven separate GitHub repos, and the obvious
// "helpful" edit is to spell all seven into the block. That edit is refused, and
// the reason is not taste: this block is WRITTEN TO DISK IN SOMEBODY ELSE'S
// PROJECT. A repo URL embedded here is copied into every project `agent-setup`
// has ever run in, and nothing can recall it — only a re-run of `agent-setup`
// rewrites the block, and most authors never re-run it. A docs-site page can be
// corrected once for every reader. See AGENTS.md item 36 — evidence:
// claudedocs/decisions/36-agents-block-per-project.md §"Third: the Docs section
// links ONE URL" for the measurements (the GitHub topic enumeration is already
// wrong, and `civitai.com/apps/...` 404s anonymously).
//
// 🔴 WHERE THE URL LIVES: `internal/cmd/templates/agents-app.md` holds the
// literal that ships, and `wantDocsSection` below holds the only other spelling
// in this repo. Correcting the URL is those two edits and nothing else — verified
// by grep at the time this was written, and the failure message below names both
// so the next person does not have to.

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// wantDocsSection is the WHOLE `### Docs` section, byte for byte.
//
// 🔴 IT IS PINNED AS A WHOLE STRING, NOT AS A SET OF WORDS, BECAUSE THE ARTIFACT
// UNDER TEST IS PROSE. A guard that checks "the block mentions
// developer.civitai.com/apps/examples" is walkable by rewording the line around
// it into something that no longer tells the reader what they get — and the
// wording is the point: this line is read by an agent deciding whether to follow
// the link. Pinning the whole section costs a test edit for a cosmetic reword,
// which is the price of a machine-readable claim about what the section says.
const wantDocsSection = `### Docs

- Guide: https://developer.civitai.com/apps/guide/
- Reference (manifest, scopes, hooks, message bridge, CLI):
  https://developer.civitai.com/apps/reference/
- Example apps you can read end-to-end:
  https://developer.civitai.com/apps/examples
- Full doc index for agents: https://developer.civitai.com/llms.txt`

// docsLinkRe pulls the link SET out of a section. The expected set is derived
// from the golden above rather than written out again, so the two cannot
// disagree. It exists for the failure MESSAGE: a whole-string mismatch reports a
// diff, while the set reports "a link was added" or "a link was removed", which
// is the edit somebody actually made.
var docsLinkRe = regexp.MustCompile(`https?://[^\s)]+`)

// docsSectionOf returns the `### Docs` section of a rendered block, from its
// heading to the end of the block, with surrounding whitespace trimmed. The
// second return is false when there is no such section — a distinct answer from
// "the section is empty", because a guard that cannot tell those apart passes
// vacuously on a block that lost the section entirely.
func docsSectionOf(block string) (string, bool) {
	const heading = "### Docs"
	i := strings.Index(block, heading)
	if i < 0 {
		return "", false
	}
	rest := block[i:]
	if j := strings.Index(rest, agentsEndMarker); j >= 0 {
		rest = rest[:j]
	}
	// A following `### ` heading would end the section too. Today Docs is last;
	// this is here so the extractor does not silently swallow a section added
	// after it.
	if j := strings.Index(rest[len(heading):], "\n### "); j >= 0 {
		rest = rest[:len(heading)+j]
	}
	return strings.TrimSpace(rest), true
}

// TestTheBlocksDocsSectionIsPinnedForEveryProjectKind is the guard.
//
// It is per-SHAPE on purpose. The template branches three ways on the project it
// is being written into (item 36), and each branch is a `{{ if }}`/`{{ else }}`
// boundary a mis-nested edit can move — so "the Docs section survived" is a claim
// about each branch separately, not about the file. An author scaffolding the
// `static` template is exactly the reader with the most to gain from an example
// app and the least local code to read.
func TestTheBlocksDocsSectionIsPinnedForEveryProjectKind(t *testing.T) {
	wantLinks := docsLinkRe.FindAllString(wantDocsSection, -1)
	if len(wantLinks) < 4 {
		t.Fatalf("CONTROL failure, not a finding: the golden section parses to %d link(s), want >= 4. "+
			"docsLinkRe (%s) has stopped matching, so every comparison below is against an empty set",
			len(wantLinks), docsLinkRe)
	}
	sort.Strings(wantLinks)

	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure, not a finding: allProjectShapesForTest returned %d shape(s), want >= 3 — "+
			"the three-branch claim in this test's name is not being exercised", len(shapes))
	}

	for _, shape := range shapes {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		got, ok := docsSectionOf(block)
		if !ok {
			t.Errorf("the rendered block for kind %q (%d recognised script(s)) has NO `### Docs` section at all. "+
				"Every branch of the template must keep it: the reader of a `static` project has the least local "+
				"code to learn from and the most to gain from the examples page", shape.Kind, len(shape.Scripts))
			continue
		}

		gotLinks := docsLinkRe.FindAllString(got, -1)
		sort.Strings(gotLinks)
		if strings.Join(gotLinks, "\n") != strings.Join(wantLinks, "\n") {
			t.Errorf("the `### Docs` link SET changed for kind %q.\n  want: %s\n  got:  %s\n"+
				"The set is asserted so that adding or removing a link is a deliberate edit. If the change is "+
				"intended, update wantDocsSection in internal/cmd/agent_setup_docs_test.go to match "+
				"internal/cmd/templates/agents-app.md — those are the only two places a docs URL is spelled.",
				shape.Kind, strings.Join(wantLinks, ", "), strings.Join(gotLinks, ", "))
		}

		if got != wantDocsSection {
			t.Errorf("the `### Docs` section for kind %q is not what is pinned.\n--- want ---\n%s\n--- got ---\n%s\n"+
				"The WHOLE section is pinned, not individual words, because a reworded line is still a changed "+
				"claim about what a reader gets from following it. Edit internal/cmd/templates/agents-app.md and "+
				"wantDocsSection together.", shape.Kind, wantDocsSection, got)
		}
	}
}

// TestDocsSectionExtractorCanFail is the negative control for docsSectionOf.
//
// Without it, an extractor that has stopped finding the heading returns ("",
// false) for every shape, the test above reports "no Docs section" for all of
// them, and a reader "fixes" the template. Worse in the other direction: an
// extractor returning ("", true) on a block with no section would compare an
// empty string against an empty expectation if the golden were ever blanked.
func TestDocsSectionExtractorCanFail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		want  bool
	}{
		{"no docs section", "## Building a Civitai App\n\n### Gotchas\n\n- a\n" + agentsEndMarker, false},
		{"empty block", "", false},
		{"heading only", "### Docs\n" + agentsEndMarker, true},
	} {
		if _, ok := docsSectionOf(tc.block); ok != tc.want {
			t.Errorf("%s: docsSectionOf reported present=%t, want %t — the extractor cannot tell "+
				"an absent section from a present one, so the guard above proves nothing", tc.name, ok, tc.want)
		}
	}

	// The positive half: it finds the real one, and stops at the END marker.
	block, err := agentsManagedBlock(allProjectShapesForTest()[0])
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	got, ok := docsSectionOf(block)
	if !ok {
		t.Fatal("docsSectionOf found no section in a real rendered block — the extractor is broken, " +
			"not the template")
	}
	if strings.Contains(got, agentsEndMarker) {
		t.Errorf("the extracted section still carries the END marker, so it is not the section but the "+
			"rest of the file:\n%s", got)
	}
}

// docsLinkProbeEnv gates the ONE network-touching test in this package.
//
// 🔴 OPT-IN, NEVER DEFAULT. `make ci` must stay green with no network — a test
// that fails on a train is a test somebody deletes. This follows the existing
// pattern in internal/scaffold (CIVITAI_CHECK_PUBLISHED_PINS); no CI job sets
// this one today, so it is a MANUAL pre-merge check, and saying that out loud is
// the honest version of "the links are verified".
const docsLinkProbeEnv = "CIVITAI_CHECK_DOCS_LINKS"

// mustNotResolve is the probe's own negative control: a path on the same host
// that cannot exist. If THIS comes back < 400 the prober is reading something
// other than the status of the URL it was given, and every green below is a fact
// about the prober rather than about the docs site.
const mustNotResolve = "https://developer.civitai.com/__civitai-cli-link-probe-must-404__"

// TestBlockDocsLinksResolve fetches every URL the block ships and fails on a
// 4xx/5xx. A transport failure SKIPS rather than fails: "the network is down"
// and "the page is gone" are different findings and only one of them is this
// test's.
func TestBlockDocsLinksResolve(t *testing.T) {
	if os.Getenv(docsLinkProbeEnv) != "1" {
		t.Skipf("network guard; set %s=1 to run (no CI job does — this is a manual pre-merge check)", docsLinkProbeEnv)
	}

	client := &http.Client{Timeout: 20 * time.Second}
	status := func(url string) (int, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("User-Agent", "civitai-cli-docs-link-probe")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode, nil
	}

	if code, err := status(mustNotResolve); err != nil {
		t.Skipf("%s unreachable (%v) — skipping, not failing; the control could not be run either", mustNotResolve, err)
	} else if code < 400 {
		t.Fatalf("CONTROL failure, not a finding: %s answered %d. A URL that cannot exist is reporting OK, "+
			"so this prober cannot distinguish a live page from a dead one and nothing below it means anything",
			mustNotResolve, code)
	}

	block, err := agentsManagedBlock(allProjectShapesForTest()[0])
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	section, ok := docsSectionOf(block)
	if !ok {
		t.Fatal("no `### Docs` section to probe")
	}
	links := docsLinkRe.FindAllString(section, -1)
	if len(links) < 4 {
		t.Fatalf("CONTROL failure, not a finding: %d link(s) extracted, want >= 4", len(links))
	}

	for _, url := range links {
		code, err := status(url)
		switch {
		case err != nil:
			t.Skipf("%s unreachable (%v) — skipping, not failing", url, err)
		case code >= 400:
			t.Errorf("DEAD LINK IN THE MANAGED BLOCK — this URL is written into every project "+
				"`civitai agent-setup` runs in, and nothing can recall it.\n  url:    %s\n  status: %d\n"+
				"  fix:    correct it in internal/cmd/templates/agents-app.md AND wantDocsSection in this file, "+
				"or publish the page before merging.", url, code)
		default:
			t.Logf("%s -> %d", url, code)
		}
	}
}

// 🔴 A TRAILING SLASH IS LOAD-BEARING ON THIS HOST, AND THAT IS MEASURED RATHER
// THAN ASSUMED. developer.civitai.com serves `/apps/showcase` (200) and 404s
// `/apps/showcase/`, while `/apps/guide/` is the other way round. So a URL here
// cannot be "tidied" one way or the other by eye; run the probe above.
