package cmd

// Every docs PAGE address an agent receives from `agent-setup` is the `.md`
// rendering, never the HTML route.
//
// 🔴 WHY THIS IS A RULE AND NOT A STYLE. The managed block tells its reader
// that "the `.md` suffix serves the plain-text source an agent can read
// directly", and then — until this guard — linked four pages by their HTML
// route (`/apps/reference/`, `/apps/guide/`, `/apps/examples`). Measured
// 2026-10-14: each of those answers `200 text/html` with a full site shell
// (`<!DOCTYPE html>`, 77–118 KB), while its `.md` twin answers
// `200 text/markdown` at a fraction of the size. An agent following the HTML
// link spends its fetch budget on navigation chrome, and the block contradicts
// its own sentence about the suffix. The `--track api` refusal is held to the
// same rule: it is read by the same agent, at the moment it is deciding where
// to go next.
//
// The rule is about the SHAPE of every address, not a list of addresses, so a
// link added later is covered without anyone editing this file. Liveness of the
// `.md` spellings is TestBlockDocsLinksResolve's job (opt-in, networked); this
// test is offline and runs in `make ci`.

import (
	"net/url"
	"strings"
	"testing"
)

// docsHost is the host the rule governs. Other hosts (the MCP endpoints, GitHub)
// are not docs pages and have no `.md` twin.
const docsHost = "developer.civitai.com"

// htmlRouteDocsURLs returns how many developer.civitai.com addresses `text`
// carries and which of them are HTML routes rather than markdown/plain-text
// renderings. `/llms.txt` is the one non-`.md` address that is already plain
// text; a bare host (no path) names the site, not a page.
func htmlRouteDocsURLs(text string) (checked int, bad []string) {
	for _, raw := range docsLinkRe.FindAllString(text, -1) {
		u, err := url.Parse(strings.TrimRight(raw, ".,;:"))
		if err != nil || u.Host != docsHost {
			continue
		}
		checked++
		p := u.Path
		if p == "" || p == "/" || strings.HasSuffix(p, ".md") || p == "/llms.txt" {
			continue
		}
		bad = append(bad, raw)
	}
	return checked, bad
}

// TestHTMLRouteDetectorCanFail is the control for htmlRouteDocsURLs: it must
// flag each HTML-route shape the block actually shipped before this guard
// (trailing slash, no suffix), pass the `.md` and `llms.txt` spellings, and
// ignore other hosts. Without it a detector that matched nothing would report
// every block clean.
func TestHTMLRouteDetectorCanFail(t *testing.T) {
	const in = "a https://" + docsHost + "/apps/guide/ b https://" + docsHost + "/apps/examples " +
		"c https://" + docsHost + "/apps/guide/earning.md d https://" + docsHost + "/llms.txt " +
		"e https://mcp.civitai.com/mcp f `https://" + docsHost + "/apps/reference/x.md`"
	checked, bad := htmlRouteDocsURLs(in)
	if checked != 5 {
		t.Errorf("checked %d docs-host address(es), want 5 — the extractor is not seeing what the block spells", checked)
	}
	want := []string{"https://" + docsHost + "/apps/guide/", "https://" + docsHost + "/apps/examples"}
	if strings.Join(bad, " ") != strings.Join(want, " ") {
		t.Errorf("flagged %q, want exactly %q", bad, want)
	}
}

// TestEveryDocsPageURLAnAgentReceivesIsMarkdown renders the managed block for
// every project shape under BOTH `{{ if .ElementsSDK }}` arms — the whole block,
// not only `### Docs`, so `### Before you submit` is covered too — plus the
// `--track api` refusal, and requires every docs-host address to be `.md` (or
// `/llms.txt`).
func TestEveryDocsPageURLAnAgentReceivesIsMarkdown(t *testing.T) {
	type surface struct{ name, text string }
	var surfaces []surface
	for _, sdk := range []bool{false, true} {
		for _, shape := range allProjectShapesForTest() {
			shape.ElementsSDK = sdk
			block, err := agentsManagedBlock(shape)
			if err != nil {
				t.Fatalf("rendering kind %q (ElementsSDK=%t): %v", shape.Kind, sdk, err)
			}
			surfaces = append(surfaces, surface{
				name: "managed block, kind " + string(shape.Kind) + map[bool]string{false: " (blocks-react)", true: " (@civitai/sdk)"}[sdk],
				text: block,
			})
		}
	}
	surfaces = append(surfaces, surface{"`agent-setup --track api` refusal", agentSetupTrackAPIRefusal().Error()})

	for _, s := range surfaces {
		checked, bad := htmlRouteDocsURLs(s.text)
		// Positive control per surface: a surface that yields no docs address
		// at all is a surface this guard says nothing about.
		if checked == 0 {
			t.Errorf("CONTROL failure, not a finding: %s carries no %s address — either it lost its docs "+
				"links or the extractor stopped matching", s.name, docsHost)
			continue
		}
		for _, u := range bad {
			t.Errorf("%s links the HTML route %s. An agent reads this; link the `.md` rendering instead "+
				"(curl it first: 200, `text/markdown`, body not starting `<!DOCTYPE`) and update the pins "+
				"(wantDocsSection, TestAgentSetupTrackAPIRefuses).", s.name, u)
		}
	}
}
