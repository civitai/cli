package cmd

import (
	"strings"
	"testing"
)

// THE `### Before you submit` SECTION OF THE MANAGED BLOCK (civitai-app-starters#573).
//
// Human review of agent-built apps sent the same feedback back on app after app,
// and the docs site now carries it as a checklist (`/apps/guide/first-review`,
// civitai-developer-docs#179). This section is the block's pointer to it plus the
// handful of defaults a reviewer can check mechanically.
//
// 🔴 IT PUTS CONTENTS IN THE BLOCK, OVER AGENTS.md ITEM 36's "the CLI ships the
// address, the docs repo ships the contents" — the second named exception after
// the picker-ecosystem gotcha, recorded with its measured byte cost in
// `claudedocs/decisions/36-agents-block-per-project.md`. The bullets are the
// subset that recurred in review AND that an agent can check without taste; the
// rest of the checklist stays on the page.

// wantBeforeSubmitSection is the WHOLE section, whitespace-normalised.
//
// 🔴 PINNED AS A WHOLE STRING, NOT BY KEYWORDS. The artefact is prose, so a guard
// on a word ("first-review", "360px") is walkable by rewording the sentence around
// it into one that no longer tells the reader what to do. A cosmetic reword fails
// here; that is the price of a machine-readable claim.
const wantBeforeSubmitSection = "### Before you submit " +
	"Run the first-review checklist before `civitai app submit` — it is the review feedback that came " +
	"back on app after app: https://developer.civitai.com/apps/guide/first-review.md " +
	"- Check the cold-open and empty states against the live backend, and a ≈360px width — harness " +
	"demo data proves only the populated path. " +
	"- One filled primary action per screen, reserved for send/spend. " +
	"- Errors are your own copy: branch on the refusal `code` / `reason`, never print the raw server " +
	"string. A disabled action names what is missing. " +
	"- One pending submission per app, so batch every fix into it. Withdrawing a first-version " +
	"submission deletes its store listing (icon, cover, screenshots)."

// blockSectionOf returns the `### <heading>` section of a rendered block, from
// its heading up to the next `### ` heading or the END marker, trimmed. The bool
// is false when the heading is absent — a distinct answer from an empty section,
// so a block that LOST the section cannot compare equal to anything.
func blockSectionOf(block, heading string) (string, bool) {
	i := strings.Index(block, "\n### "+heading+"\n")
	if i < 0 {
		return "", false
	}
	rest := block[i+1:]
	if j := strings.Index(rest, agentsEndMarker); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.Index(rest[len("### "):], "\n### "); j >= 0 {
		rest = rest[:len("### ")+j]
	}
	return strings.TrimSpace(rest), true
}

// TestTheBlockCarriesTheBeforeYouSubmitSectionInEveryShape is the guard.
//
// 🔴 EVERY SHAPE × BOTH SDK BRANCHES. The template branches on project kind and
// on `ElementsSDK`; this section sits outside every `{{ if }}`, and a mis-nested
// edit that moved it inside one is exactly what a single-shape check passes.
// allProjectShapesForTest renders no ElementsSDK shape, so both values are set
// here explicitly.
//
// 🔴 WATCHED FAIL at origin/main (fc7daaa), with only this file added: every one
// of the ten renderings reports the section absent.
func TestTheBlockCarriesTheBeforeYouSubmitSectionInEveryShape(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 3 {
		t.Fatalf("CONTROL failure, not a finding: %d shape(s) to render", len(shapes))
	}
	rendered := 0
	for _, sdk := range []bool{false, true} {
		for _, shape := range shapes {
			shape.ElementsSDK = sdk
			block, err := agentsManagedBlock(shape)
			if err != nil {
				t.Fatalf("rendering kind %q (sdk=%t): %v", shape.Kind, sdk, err)
			}
			rendered++
			got, ok := blockSectionOf(block, "Before you submit")
			if !ok {
				t.Errorf("kind %q (%d script(s), wizard=%t, sdk=%t): the managed block has no "+
					"`### Before you submit` section — the agent is never pointed at the first-review "+
					"checklist before `civitai app submit`",
					shape.Kind, len(shape.Scripts), shape.HasSetupWizard, sdk)
				continue
			}
			if n := normaliseMessage(got); n != wantBeforeSubmitSection {
				t.Errorf("kind %q (sdk=%t): the `### Before you submit` section is not what is pinned.\n"+
					"--- want ---\n%s\n--- got ---\n%s\n"+
					"Edit internal/cmd/templates/agents-app.md and wantBeforeSubmitSection together.",
					shape.Kind, sdk, wantBeforeSubmitSection, n)
			}
		}
	}
	if rendered != 2*len(shapes) {
		t.Fatalf("CONTROL failure, not a finding: rendered %d block(s), want %d", rendered, 2*len(shapes))
	}
}

// TestBlockSectionOfCanFail is the extractor's negative and boundary control:
// absent is reported absent, a heading that merely STARTS with the name does not
// match, and the section stops at the next heading and at the END marker.
func TestBlockSectionOfCanFail(t *testing.T) {
	if _, ok := blockSectionOf("## Top\n\n### Gotchas\n\n- a\n"+agentsEndMarker, "Before you submit"); ok {
		t.Error("blockSectionOf reported a section that is not in the document")
	}
	if _, ok := blockSectionOf("x\n### Before you submit later\n", "Before you submit"); ok {
		t.Error("blockSectionOf matched a heading that only starts with the wanted name")
	}
	got, ok := blockSectionOf("x\n### Before you submit\n\nbody\n\n### Next\n\nother\n", "Before you submit")
	if !ok || got != "### Before you submit\n\nbody" {
		t.Errorf("did not stop at the next heading: ok=%t %q", ok, got)
	}
	got, ok = blockSectionOf("x\n### Before you submit\n\nbody\n"+agentsEndMarker+"\ntrailer", "Before you submit")
	if !ok || got != "### Before you submit\n\nbody" {
		t.Errorf("did not stop at the END marker: ok=%t %q", ok, got)
	}
}
