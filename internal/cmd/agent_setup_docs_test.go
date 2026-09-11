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
// 🔴 WHERE THE URL LIVES: AN ENUMERATION THAT A TEST KEEPS HONEST, NOT AN
// ABSOLUTE. This comment used to say "`internal/cmd/templates/agents-app.md`
// holds the literal that ships, and `wantDocsSection` below holds the only other
// spelling in this repo — correcting the URL is those two edits and nothing
// else, verified by grep". That was FALSE WHEN IT WAS WRITTEN: the same PR (#549)
// also wrote the URL into `claudedocs/decisions/36-agents-block-per-project.md`.
// Re-measured on 2026-09-11 over the whole tree, ignored files included:
// `developer.civitai.com/apps/examples` appears on 9 lines in 4 files.
//
// So the enumeration is not restated in prose here. It is
// `docsURLSpellingLedger` below, and `TestEveryDocsURLSpellingIsLedgered` walks
// the repository and fails BOTH when a file outside the ledger spells one of
// these URLs and when a ledgered file stops spelling any — which is what makes
// "these are the files you edit" a checked claim rather than a remembered one.
// Dated records under `claudedocs/handoff-` are exempt on purpose: they record
// what was true on the day they were written and are not corrected afterwards.

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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

	// 🔴 THE CONTROL COUNTS KINDS, NOT SHAPES. It used to read
	// `if len(shapes) < 3`, which three shapes of the SAME kind satisfy —
	// `allProjectShapesForTest` returns four shapes covering three kinds, so the
	// count and the coverage are different numbers and only the second one is
	// what "for every project kind" in this test's name claims. The wanted set is
	// read out of `agent_setup_project.go`'s own `projectKind` constants
	// (`declaredProjectKinds`), so a fourth kind added there fails HERE until it
	// is rendered and pinned too.
	shapes := allProjectShapesForTest()
	want := declaredProjectKinds(t)
	seen := map[projectKind]bool{}
	for _, shape := range shapes {
		seen[shape.Kind] = true
	}
	var missing, extra []string
	for _, k := range want {
		if !seen[k] {
			missing = append(missing, string(k))
		}
	}
	wanted := map[projectKind]bool{}
	for _, k := range want {
		wanted[k] = true
	}
	for k := range seen {
		if !wanted[k] {
			extra = append(extra, string(k))
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Fatalf("CONTROL failure, not a finding: allProjectShapesForTest returned %d shape(s) covering kind(s) %s, "+
			"but %s is declared in agent_setup_project.go and never rendered. The `### Docs` section is pinned "+
			"per-shape because each kind is a different `{{ if }}` branch of the template; an unrendered kind is a "+
			"branch this guard says nothing about. Add a shape for it to allProjectShapesForTest.",
			len(shapes), strings.Join(sortedKindNames(seen), ", "), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Fatalf("CONTROL failure, not a finding: allProjectShapesForTest renders kind(s) %s that "+
			"agent_setup_project.go does not declare as a projectKind constant. Either the constant was renamed "+
			"(update the shapes) or declaredProjectKinds has stopped reading the file (the wanted set was %s).",
			strings.Join(extra, ", "), strings.Join(sortedKindNames(wanted), ", "))
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
				"internal/cmd/templates/agents-app.md, then run TestEveryDocsURLSpellingIsLedgered — it names "+
				"every OTHER file in this repo that spells a docs URL.",
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

// sortedKindNames renders a kind set for a failure message, deterministically.
func sortedKindNames(set map[projectKind]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

// docsURLSpellingLedger is every file in this repository that spells one of the
// block's docs URLs AND must be reconciled when one of them changes.
//
// 🔴 IT IS A LEDGER BECAUSE THE PROSE VERSION WAS FALSE. See the file header:
// the claim "those two edits and nothing else" shipped in #549 alongside a THIRD
// file that spells the URL twice. A sentence cannot notice a fourth; the walk
// below can.
var docsURLSpellingLedger = map[string]string{
	"internal/cmd/templates/agents-app.md": "the literal that SHIPS — this is the one that is written into somebody " +
		"else's project and cannot be recalled",
	"internal/cmd/agent_setup_docs_test.go": "wantDocsSection, the golden the template is compared against, plus " +
		"this file's own prose about the URL",
	"claudedocs/decisions/36-agents-block-per-project.md": "AGENTS.md item 36's evidence file: it quotes the added " +
		"line verbatim and tabulates each URL's measured status",
}

// docsURLLedgerExemptPrefix is the ONE exemption, and it is not a convenience.
// A handoff doc is a DATED RECORD of what was true when it was written — a URL
// in one is evidence, not a link anybody follows from a shipped artefact, and
// "correcting" it would falsify the record. Anything else that names a docs URL
// is a place the URL can rot, so it belongs in the ledger.
const docsURLLedgerExemptPrefix = "claudedocs/handoff-"

// docsURLLedgerSkipDirs are directories the walk must not descend into. `.claude`
// is load-bearing on a machine that uses git worktrees: this repo's own
// `.gitignore` puts `/.claude/worktrees` there, and each worktree is a FULL COPY
// of the tree, so a walk that entered it would report every ledgered file a
// second time under a different path.
var docsURLLedgerSkipDirs = map[string]bool{
	".git": true, ".claude": true, "bin": true, "dist": true, "node_modules": true, ".venv": true,
}

// TestEveryDocsURLSpellingIsLedgered is the bidirectional guard behind the file
// header's enumeration.
//
// It fails when a file OUTSIDE the ledger spells one of the block's docs URLs
// (the enumeration grew and nobody said so) and when a ledgered file no longer
// spells any (the enumeration shrank and the ledger is now pointing at a file
// with nothing in it). Both directions matter: the first is how the original
// claim went stale, the second is how a ledger becomes decoration.
func TestEveryDocsURLSpellingIsLedgered(t *testing.T) {
	urls := docsLinkRe.FindAllString(wantDocsSection, -1)
	if len(urls) < 4 {
		t.Fatalf("CONTROL failure, not a finding: the golden section parses to %d URL(s), want >= 4 — "+
			"the walk below would be searching for nothing", len(urls))
	}
	// Match on host+path rather than the full URL: the header comment above names
	// the examples page without a scheme, and that mention rots exactly like a
	// link does.
	needles := make([][]byte, 0, len(urls))
	for _, u := range urls {
		needles = append(needles, []byte(strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")))
	}

	root := repoRootDir(t)
	scanned := 0
	found := map[string]bool{}
	exempt := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && docsURLLedgerSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Size() > 2<<20 {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		for _, needle := range needles {
			if bytes.Contains(body, needle) {
				if strings.HasPrefix(rel, docsURLLedgerExemptPrefix) {
					exempt[rel] = true
				} else {
					found[rel] = true
				}
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	// Positive control: the walk must have READ a real tree. A misrooted walk
	// reads nothing, finds nothing, and reports the ledger fully satisfied in the
	// one direction anybody checks.
	if scanned < 200 {
		t.Fatalf("CONTROL failure, not a finding: the walk read %d file(s) under %s, want >= 200. "+
			"This repository had 696 tracked files on 2026-09-11, so a number this low means the walk is "+
			"rooted somewhere else or is skipping everything — not that the ledger is clean", scanned, root)
	}

	for rel := range found {
		if _, ok := docsURLSpellingLedger[rel]; !ok {
			t.Errorf("%s spells one of the block's docs URLs and is NOT in docsURLSpellingLedger.\n"+
				"  A docs URL in this repo is a place it can rot: when the URL changes, this file goes stale "+
				"alone and silently.\n"+
				"  fix: add it to docsURLSpellingLedger in internal/cmd/agent_setup_docs_test.go with what it "+
				"is FOR, or take the URL out of it.", rel)
		}
	}
	for rel, why := range docsURLSpellingLedger {
		if found[rel] {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("docsURLSpellingLedger names %s (%s), which cannot be read: %v. "+
				"The ledger is pointing at a file that moved or was deleted.", rel, why, err)
			continue
		}
		t.Errorf("docsURLSpellingLedger names %s (%s), but it no longer spells ANY of the block's docs URLs.\n"+
			"  Either the URL was removed from it — drop the row — or the golden changed and this file was "+
			"left on the old spelling, which is the exact rot the ledger exists to catch.", rel, why)
	}
	t.Logf("scanned %d file(s); %d ledgered spelling(s), %d exempt dated record(s): %s",
		scanned, len(found), len(exempt), strings.Join(sortedKeys(exempt), ", "))
}

// docsLinkProbeEnv gates the ONE network-touching test in this package.
//
// 🔴 OPT-IN, NEVER DEFAULT. `make ci` must stay green with no network — a test
// that fails on a train is a test somebody deletes. This follows the existing
// pattern in internal/scaffold (CIVITAI_CHECK_PUBLISHED_PINS); no CI job sets
// this one today, so it is a MANUAL pre-merge check, and saying that out loud is
// the honest version of "the links are verified".
const docsLinkProbeEnv = "CIVITAI_CHECK_DOCS_LINKS"

// mustNotResolve is the probe's NEGATIVE control: a path on the same host that
// cannot exist. If THIS comes back < 400 the prober is reading something other
// than the status of the URL it was given, and every green below is a fact about
// the prober rather than about the docs site.
//
// 🔴 IT IS ONLY HALF A CONTROL, AND THE OTHER HALF IS `sawLive` BELOW. An edge
// that blocks this prober answers `403`/`429`/`503` to EVERY request, which
// satisfies "the impossible path is >= 400" perfectly while making every real
// link >= 400 too. This host has form: `developer.civitai.com` hard-blocks
// `Python-urllib/3.11` with Cloudflare `error code: 1010`, and the probe sends a
// custom non-browser User-Agent. So a negative control that passes proves the
// prober is not answering OK to everything; it does NOT prove the prober can see
// a live page. That needs a positive control, and its absence is why this test
// must be able to say "cannot tell" as well as "gone".
const mustNotResolve = "https://developer.civitai.com/__civitai-cli-link-probe-must-404__"

// docsProbeResult is one URL's answer, kept so the loop can finish before
// anything is diagnosed. The earlier version diagnosed inside the loop and
// abandoned the whole run on the first transport error, so one DNS hiccup on the
// FIRST link reported a pass without ever fetching the others.
type docsProbeResult struct {
	url      string
	code     int
	location string
	err      error
}

// TestBlockDocsLinksResolve fetches every URL the block ships and reports what it
// found, in four separable verdicts. The split is the point: "gone", "moved",
// "cannot tell" and "could not reach" lead to four different actions, and the
// version that printed one message for all of them told a reader to edit the
// template when the truth was that the prober had been blocked.
//
//   - < 300               — live at the spelling that ships.
//   - 3xx                 — MOVED. The page answers, but not at this spelling.
//   - 404 / 410           — GONE. This is the dead-link finding.
//   - other >= 400        — CANNOT TELL: 401/403/429/5xx are what a blocked or
//     failing edge returns, and are indistinguishable from a
//     genuinely broken page without a positive control.
//   - transport error     — COULD NOT REACH. Collected, never fatal, and never a
//     reason to stop probing the remaining links.
func TestBlockDocsLinksResolve(t *testing.T) {
	if os.Getenv(docsLinkProbeEnv) != "1" {
		t.Skipf("network guard; set %s=1 to run (no CI job does — this is a manual pre-merge check)", docsLinkProbeEnv)
	}

	// 🔴 REDIRECTS ARE NOT FOLLOWED, BECAUSE THE SPELLING IS THE CLAIM. Go's
	// default CheckRedirect follows up to 10 hops, so a probe that reports 200 is
	// reporting on wherever it LANDED, not on the URL written into the block.
	// Measured 2026-09-11: `/apps/guide` answers 301 to
	// `http://developer.civitai.com/apps/guide/` — a different spelling, and a
	// downgrade to cleartext http — which a following probe scores as a clean 200.
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	probe := func(url string) docsProbeResult {
		out := docsProbeResult{url: url}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			out.err = err
			return out
		}
		req.Header.Set("User-Agent", "civitai-cli-docs-link-probe")
		resp, err := client.Do(req)
		if err != nil {
			out.err = err
			return out
		}
		defer func() { _ = resp.Body.Close() }()
		out.code = resp.StatusCode
		out.location = resp.Header.Get("Location")
		return out
	}

	control := probe(mustNotResolve)
	switch {
	case control.err != nil:
		t.Skipf("%s unreachable (%v) — skipping, not failing; the control could not be run either",
			mustNotResolve, control.err)
	case control.code < 400:
		t.Fatalf("CONTROL failure, not a finding: %s answered %d. A URL that cannot exist is reporting OK, "+
			"so this prober cannot distinguish a live page from a dead one and nothing below it means anything",
			mustNotResolve, control.code)
	case control.code != 404 && control.code != 410:
		// Still a pass, but worth printing: an impossible path answering 403 or
		// 503 is what a blocking edge looks like, not what a 404 handler looks
		// like. It changes how the statuses below should be read.
		t.Logf("note: the negative control %s answered %d rather than 404/410 — if the links below are also "+
			">= 400, read that as 'this prober is being blocked' before reading it as 'the pages are gone'",
			mustNotResolve, control.code)
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

	// Probe EVERY link first. Nothing in this loop can end the run.
	results := make([]docsProbeResult, 0, len(links))
	for _, url := range links {
		results = append(results, probe(url))
	}

	var (
		unreachable []docsProbeResult
		moved       []docsProbeResult
		gone        []docsProbeResult
		cannotTell  []docsProbeResult
		sawLive     bool
	)
	for _, r := range results {
		switch {
		case r.err != nil:
			unreachable = append(unreachable, r)
		case r.code < 300:
			sawLive = true
			t.Logf("%s -> %d", r.url, r.code)
		case r.code < 400:
			moved = append(moved, r)
		case r.code == 404 || r.code == 410:
			gone = append(gone, r)
		default:
			cannotTell = append(cannotTell, r)
		}
	}
	checked := len(links) - len(unreachable)
	for _, r := range unreachable {
		t.Logf("%s -> transport error: %v (continuing; this run does NOT claim to have checked it)", r.url, r.err)
	}
	t.Logf("checked %d of %d link(s); %d unreachable, %d live, %d moved, %d gone, %d undiagnosable",
		checked, len(links), len(unreachable), checkedLive(results), len(moved), len(gone), len(cannotTell))

	if checked == 0 {
		t.Skipf("NONE of the %d link(s) could be reached — every one failed at the transport layer. "+
			"That is a finding about this machine's network, not about the docs site, so it skips rather "+
			"than fails. Nothing was verified.", len(links))
	}

	for _, r := range gone {
		t.Errorf("DEAD LINK IN THE MANAGED BLOCK — this URL is written into every project "+
			"`civitai agent-setup` runs in, and nothing can recall it.\n  url:    %s\n  status: %d\n"+
			"  fix:    correct it in internal/cmd/templates/agents-app.md AND wantDocsSection in this file "+
			"(then run TestEveryDocsURLSpellingIsLedgered for the rest), or publish the page before merging.",
			r.url, r.code)
	}
	for _, r := range moved {
		t.Errorf("MOVED LINK IN THE MANAGED BLOCK — the page answers, but NOT at the spelling that ships. "+
			"This is not a dead link and it is not a live one: a browser follows the hop, and the block is "+
			"left naming a URL the site no longer serves directly.\n  url:      %s\n  status:   %d\n"+
			"  location: %s\n"+
			"  fix:      if the target is the intended canonical spelling, write THAT into "+
			"internal/cmd/templates/agents-app.md and wantDocsSection. Check the scheme before you copy it: "+
			"this host has been measured redirecting https to cleartext http.",
			r.url, r.code, orNone(r.location))
	}
	if len(cannotTell) > 0 {
		var lines []string
		for _, r := range cannotTell {
			lines = append(lines, fmt.Sprintf("    %s -> %d", r.url, r.code))
		}
		msg := fmt.Sprintf("CANNOT TELL whether these links are dead — they answered a status that a BLOCKED "+
			"or FAILING EDGE produces just as readily as a broken page (401/403/407/429/5xx):\n%s\n"+
			"  The negative control %s answered %d.\n"+
			"  🔴 This is NOT a dead-link finding. Do not edit internal/cmd/templates/agents-app.md on the "+
			"strength of it. Re-run; if it persists, fetch the URL in a browser and check whether the host is "+
			"refusing this probe's User-Agent (`civitai-cli-docs-link-probe`) — developer.civitai.com is known "+
			"to hard-block non-browser agents with Cloudflare `error code: 1010`.",
			strings.Join(lines, "\n"), mustNotResolve, control.code)
		if sawLive {
			// A live link in the same run IS the positive control: the prober can
			// see a live page, so a >= 400 elsewhere is a real anomaly worth
			// failing on — it is just not diagnosable as "gone".
			t.Errorf("%s\n  (At least one other link in this run answered < 300, so this prober is not being "+
				"blocked wholesale — which is what makes this worth failing on rather than skipping.)", msg)
		} else if !t.Failed() {
			// No link answered < 300 anywhere. There is no positive control, so
			// this run cannot separate "the site is down/blocking" from "the
			// pages are gone", and a failure here would be a guess.
			t.Skipf("%s\n  NO link in this run answered < 300, so there is no positive control at all: "+
				"this run cannot distinguish a blocked prober from a dead docs site. Skipping rather than "+
				"guessing.", msg)
		} else {
			t.Errorf("%s\n  (No link in this run answered < 300, so the dead-link finding(s) above are "+
				"reported WITHOUT a positive control — a site-wide outage or block is not excluded.)", msg)
		}
	}
	if len(gone) > 0 && !sawLive {
		t.Errorf("POSITIVE CONTROL ABSENT: %d link(s) are reported gone above and NOT ONE link in this run "+
			"answered < 300. A prober that never saw a live page cannot distinguish 'these pages were removed' "+
			"from 'this whole host is answering 404 to me'. Confirm one of these URLs in a browser before "+
			"editing the template.", len(gone))
	}
	if !t.Failed() && len(unreachable) > 0 {
		t.Skipf("PARTIAL RUN: %d of %d link(s) checked, %d unreachable at the transport layer. The links that "+
			"WERE checked are fine — but this run did not verify the whole set, so it reports SKIP rather than "+
			"a pass that reads as 'all links verified'.", checked, len(links), len(unreachable))
	}
}

// checkedLive counts the results that answered < 300, for the summary line.
func checkedLive(results []docsProbeResult) int {
	n := 0
	for _, r := range results {
		if r.err == nil && r.code < 300 {
			n++
		}
	}
	return n
}

// orNone renders an empty header value visibly, so a 3xx with no Location does
// not print as a blank the reader mistakes for a missing line.
func orNone(s string) string {
	if s == "" {
		return "(none — the response carried no Location header)"
	}
	return s
}

// 🔴 A TRAILING SLASH IS LOAD-BEARING ON THIS HOST, PER PATH, AND THAT IS
// MEASURED RATHER THAN ASSUMED. Re-measured 2026-09-11 from this host, over real
// HTTP, with the probe's own User-Agent and WITHOUT following redirects:
//
//	/apps/guide       301 -> http://developer.civitai.com/apps/guide/
//	/apps/guide/      200
//	/apps/reference   301 -> http://developer.civitai.com/apps/reference/
//	/apps/reference/  200
//	/apps/examples    200
//	/apps/examples/   404
//	/apps/showcase    200
//	/apps/showcase/   404
//	/llms.txt         200
//
// All four URLs the block ships answer 200 at exactly the spelling they ship,
// with zero hops.
//
// 🔴 A PREVIOUS VERSION OF THIS COMMENT WAS WRONG AND IS WITHDRAWN, NOT REPLACED
// WITH A NARROWER CLAIM. It said `/apps/showcase` is 200 while `/apps/showcase/`
// 404s — that part re-measures true — and then that "`/apps/guide/` is the other
// way round", which reads as `/apps/guide` 404ing. It does not: it 301s, and the
// hop lands on cleartext http. There is no site-wide rule here to state in its
// place, and the two redirecting paths are the reason a probe that FOLLOWS hops
// cannot tell you whether the spelling you ship is the one the site serves. Each
// URL's spelling is a measurement. Do not tidy one by eye; run the probe above.
