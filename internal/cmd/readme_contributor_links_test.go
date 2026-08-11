package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 🔴 README.md IS SHIPPED; AGENTS.md IS NOT.
//
// The npm package and the Homebrew cask carry the README and not the
// contributor docs, so a **relative** link to `AGENTS.md` resolves for exactly
// one audience — someone reading the file on github.com — and 404s for the
// user who installed the CLI. Seven such links existed, including the two a
// `generate` user is sent to for the per-ecosystem image-editing rationale and
// an `app listing` user for the media bounds.
//
// The rule is therefore: a README link into a contributor-only file must be an
// ABSOLUTE URL. The link text may stay `AGENTS.md`.
//
// Scope, stated so a green is not over-read: this checks the LINK TARGET, not
// whether routing a user through a contributor doc was the right call in the
// first place. Where the fact itself belongs in the README, inline it.
var contributorOnlyDocs = []string{"AGENTS.md", "CLAUDE.md"}

// relativeMDLinkRe matches a markdown link whose target is a bare repo-relative
// path (no scheme, no anchor-only target).
var relativeMDLinkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)

func TestREADMEDoesNotLinkContributorDocsRelatively(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	body := string(raw)

	links := relativeMDLinkRe.FindAllStringSubmatch(body, -1)
	// POSITIVE CONTROL: a regex that stopped matching would validate nothing
	// and report a serene pass over a README full of broken links.
	if len(links) < 20 {
		t.Fatalf("CONTROL failure: found only %d markdown links in README.md, want >= 20 — "+
			"the pattern %s has stopped matching, so every check below is vacuous", len(links), relativeMDLinkRe)
	}

	checkedTargets := 0
	for _, m := range links {
		target := m[1]
		if strings.Contains(target, "://") {
			continue
		}
		checkedTargets++
		for _, doc := range contributorOnlyDocs {
			if target == doc || strings.HasPrefix(target, doc+"#") {
				t.Errorf("README.md links %s relatively (`](%s)`). That file is not in the npm package "+
					"or the Homebrew cask, so the link 404s for everyone who did not install from a git "+
					"checkout — use the absolute https://github.com/civitai/cli/blob/main/%s URL, or inline "+
					"the fact the reader was being sent for.", doc, target, doc)
			}
		}
	}
	if checkedTargets == 0 {
		t.Fatal("CONTROL failure: no relative link targets were examined at all")
	}
	t.Logf("checked %d relative link target(s) against %d contributor-only doc(s)", checkedTargets, len(contributorOnlyDocs))
}

// TestREADMEIconAspectDoesNotForbidASquareIcon — the aspect table said
// "square-ish, **not exactly square**" while the paragraph two lines below
// recommends starting from 512 × 512, which is exactly square. The bound is
// 0.9 ≤ aspect ≤ 1.1, so 1:1 is the CENTRE of the accepted range; the phrasing
// meant "it need not be exactly square" and read as a prohibition.
//
// This is a contradiction check, not a style one: the two claims cannot both be
// followed, and a reader who believes the table will avoid the size the README
// tells them to start from.
func TestREADMEIconAspectDoesNotForbidASquareIcon(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	body := string(raw)

	// CONTROL: the recommendation this would contradict must actually be there.
	if !strings.Contains(body, "**512 × 512**") {
		t.Fatal("CONTROL failure: the README no longer recommends a 512 × 512 icon, so there is no " +
			"contradiction to guard against — re-check whether this test still means anything")
	}
	if !strings.Contains(body, "0.9 – 1.1") {
		t.Fatal("CONTROL failure: the icon aspect bound (0.9 – 1.1) is not in the README, so the row " +
			"this test is about has moved or gone")
	}
	if strings.Contains(body, "not exactly square") {
		t.Error("the icon aspect row says \"not exactly square\", which reads as a prohibition on 1:1 — " +
			"but 1:1 is the centre of the 0.9–1.1 range, and the README itself recommends a 512 × 512 icon")
	}
}
