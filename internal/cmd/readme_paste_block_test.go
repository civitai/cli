package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE MIRROR THIS FILE EXISTS FOR.
//
// `## Using an AI coding agent? Paste this` carries a one-line string that is
// GENERATED in another repository. Its authority is
// `civitai-developer-docs/.vitepress/agent-setup.mjs` → `SETUP_PROMPT`, which
// that repo derives from its own `SHORT_PROMPT_URL` constant and grades on every
// PR (`npm run check:agent-setup`, check 2: the landing page must carry the
// string verbatim). The copy in THIS README is hand-kept, and until this file
// NOTHING compared the two — the string was typed here, byte for byte, with no
// guard in either repo able to see the other.
//
// 🔴 BE PRECISE ABOUT WHICH DIRECTION THIS CLOSES, because the docstring is the
// part that goes stale into a false claim of coverage:
//
//	closed   an edit HERE — a reworded, half-updated or mistyped paste block,
//	         including one that quietly repoints the URL. Mechanical, offline,
//	         and it runs in the ordinary `go test ./...` that gates a merge.
//	OPEN     the docs repo changing `SETUP_PROMPT` and this README not
//	         following. No test in either repository can see that without a
//	         cross-repo read, and this one does not attempt one.
//
// What DOES cover the open half, partially and for free, is
// `readme_external_links_test.go`: the alias is now an absolute URL in this
// README, so the wired weekly `readme-links.yml` run dereferences it and reports
// a 404/410. That catches the alias DYING. It does not catch the alias being
// repointed somewhere that still answers 200, which is the residual to remember.
//
// 🔴 THE URL IS DELIBERATELY THE SHORT ALIAS AND NOT THE CANONICAL ADDRESS. The
// canonical prompt lives at developer.civitai.com/agent-setup/prompt.md and
// every MACHINE surface still names it: the docs site's nginx exact-match route,
// its `.well-known/ai-catalog.json` entry, and its built-artifact byte-identity
// assertions. The alias exists for the one job this block does — being short
// enough to say out loud and to survive retyping.
//
// 🔴 AND IT IS A 302, WHICH IS WHY THIS BLOCK MUST STAY PROSE. Measured
// 2026-09-28: fetched WITHOUT redirect following the alias returns 143 bytes of
// Cloudflare `<html>302 Found</html>` and the fetcher exits 0; with redirects it
// returns the real 7,433 bytes of text/markdown (same sha256 as fetching the
// canonical URL directly). The block above is natural language an agent READS —
// it picks its own fetcher, which follows redirects — so it needs no flag. A
// SHELL COMMAND naming the alias does: `curl -fsSL`, never `curl -s`. The docs
// repo enforces that on its own two agent-setup surfaces (check 6 of
// `scripts/check-agent-setup.mjs`); nothing enforces it here, because this README
// names no fetcher.

// agentSetupPasteString is the published copy-paste string, pinned.
//
// 🔴 A PIN, NOT A SOURCE OF TRUTH. If this and the docs repo ever disagree, the
// docs repo is right — it is what agents are actually served. Verify against the
// published channel in one command (the landing page's text is emitted into
// `llms-full.txt`, which the docs site publishes and catalogues):
//
//	curl -fsSL https://developer.civitai.com/llms-full.txt \
//	  | grep -F 'to set me up for Civitai from'
const agentSetupPasteString = "Fetch and execute the appropriate instructions to set me up for Civitai from " +
	"https://civitai.com/agent-onboarding"

// readmePasteBlock returns the body of the fenced block under the paste heading.
//
// Each failure below is a PREMISE, reported as such: a guard that cannot find
// what it grades must say so, rather than compare an empty string and pass.
func readmePasteBlock(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	const heading = "## Using an AI coding agent? Paste this"
	i := strings.Index(string(raw), heading)
	if i < 0 {
		t.Fatalf("PREMISE BROKEN: README.md has no %q heading — this file is guarding a "+
			"section that no longer exists. If the paste block moved, move this guard with it; "+
			"if it was deleted, delete this file and say so, because the front door then no "+
			"longer carries the one-liner at all.", heading)
	}
	rest := string(raw)[i+len(heading):]

	// Bounded to the paste section: the next `## ` heading ends it. Without the
	// bound a deleted fence would be answered by the next fence anywhere below,
	// and the comparison would report a confident wrong string.
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	open := strings.Index(rest, "```")
	if open < 0 {
		t.Fatalf("PREMISE BROKEN: no fenced block follows %q before the next `## ` heading. "+
			"The paste string is what a user copies; a section that describes it without "+
			"carrying it is worse than no section.", heading)
	}
	body := rest[open+3:]
	// Drop the info string (```text) — everything up to the first newline.
	if nl := strings.Index(body, "\n"); nl >= 0 {
		body = body[nl+1:]
	}
	close := strings.Index(body, "```")
	if close < 0 {
		t.Fatalf("PREMISE BROKEN: the paste fence under %q is never closed, so the whole "+
			"rest of the section reads as its body", heading)
	}
	return strings.TrimSpace(body[:close])
}

// TestREADMEPasteBlockIsTheAdvertisedString compares the README's paste block
// against the pin, WHOLE.
//
// 🔴 WHOLE-STRING, NOT A SUBSTRING OR A URL SCAN, and RULES.md names why: when
// the artifact under test IS prose, a guard on words is walkable by rewording.
// A scan for `civitai.com/agent-onboarding` would pass a block whose sentence had
// been rewritten around it, and a scan for the sentence would pass one whose URL
// had been swapped. The cost is that a deliberate reword fails this test — pay
// it: the string is published and hand-mirrored, so a machine-readable claim is
// the point.
func TestREADMEPasteBlockIsTheAdvertisedString(t *testing.T) {
	// POSITIVE CONTROL on the pin itself. An empty or truncated constant would
	// make the comparison below a statement about nothing, and "" == "" is the
	// reassuring-zero shape in a string guard.
	if len(agentSetupPasteString) < 80 || !strings.Contains(agentSetupPasteString, "https://") {
		t.Fatalf("agentSetupPasteString is %d bytes and %q a URL — it is not the published "+
			"one-liner, so every assertion here would be vacuous",
			len(agentSetupPasteString), map[bool]string{true: "contains", false: "does not contain"}[strings.Contains(agentSetupPasteString, "https://")])
	}

	got := readmePasteBlock(t)
	if got != agentSetupPasteString {
		t.Errorf("README.md's paste block is not the advertised copy-paste string.\n"+
			"  got:  %q\n  want: %q\n\n"+
			"This block is a HAND-KEPT MIRROR of `SETUP_PROMPT` in\n"+
			"civitai-developer-docs/.vitepress/agent-setup.mjs, which is what agents are\n"+
			"actually served. If the docs repo changed the string, update the pin here to\n"+
			"match it — the docs repo wins. If you reworded this block, revert: a user pastes\n"+
			"these bytes, and the two surfaces disagreeing is how somebody is handed a\n"+
			"one-liner that no longer resolves.\n\n"+
			"Verify against the published channel:\n"+
			"  curl -fsSL https://developer.civitai.com/llms-full.txt | grep -F 'to set me up for Civitai from'",
			got, agentSetupPasteString)
	}
}

// TestREADMEPasteBlockIsOneLine pins the shape a user copies.
//
// It is a separate test because it fails for a DIFFERENT reason and needs a
// different fix: a wrapped paste block still contains the right words and would
// satisfy any substring check, while what a user actually copies is two lines
// with a newline in the middle of a URL. Markdown reflow tooling and editors that
// hard-wrap at 80 columns both produce exactly that.
func TestREADMEPasteBlockIsOneLine(t *testing.T) {
	got := readmePasteBlock(t)
	if strings.Contains(got, "\n") {
		t.Errorf("README.md's paste block spans %d lines. It must be ONE line: a user selects "+
			"and copies it, and a wrapped URL is pasted broken.\n  got: %q",
			len(strings.Split(got, "\n")), got)
	}
}
