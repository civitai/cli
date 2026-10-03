package cmd

import (
	"strings"
	"testing"
)

// TestEveryBlockTellsYouWhatToDoWhenTheCLIIsNotOnPATH is cli#665's remaining
// payload, and it is deliberately UNCONDITIONAL.
//
// 🔴 THE MEASURED DEFECT WAS AN ABSENCE IN THIS FILE, NOT A MISSING DETECTION.
// On a machine where npm's global prefix is not writable, the documented remedy
// puts the CLI on PATH for the installing shell only — and this block then names
// `civitai …` in every row. 8 of 8 trials ended that way (cli#663), and **0 of 8
// agents connected it to the AGENTS.md they had just written.** The information
// was missing from the file the later session reads.
//
// 🔴 IT IS NOT CONDITIONAL, AND THAT IS THE DECISION. Two drafts detected the
// condition by running a login shell and wrote the binary's absolute path when
// it looked unreachable. Each was measured wrong in a way the previous fix
// introduced — a distro sentinel left set, then `exec.ErrWaitDelay` scored as
// "not reachable" while the answer sat unread on stdout. Every inversion put a
// developer's home directory and a bold-red false directive into a file that is
// normally COMMITTED. A per-project sentence is wrong for nobody; a per-machine
// claim is wrong for everyone who did not generate it, and nothing detects the
// staleness. See claudedocs/decisions/36-agents-block-per-project.md.
func TestEveryBlockTellsYouWhatToDoWhenTheCLIIsNotOnPATH(t *testing.T) {
	shapes := allProjectShapesForTest()
	if len(shapes) < 2 {
		t.Fatalf("PREMISE BROKEN: only %d project shape(s) — this guard exists to prove the "+
			"note is rendered for EVERY shape, so one shape tests nothing", len(shapes))
	}
	for _, shape := range shapes {
		block, err := agentsManagedBlock(shape)
		if err != nil {
			t.Fatalf("rendering for kind %q: %v", shape.Kind, err)
		}
		// The symptom a reader actually sees, and the action that fixes it.
		//
		// 🔴 `--fix-path` IS IN THAT LIST, AND ITS ABSENCE WAS ROUND 1'S R7. This
		// block is the DURABLE artefact cli#665 exists to repair: the file a future
		// agent session reads when `civitai` does not resolve. It described the hand
		// edit and never named the flag that performs it — in a file written by the
		// same binary that owns the flag, so decision 39's sequencing argument (a
		// hosted prompt can skew against a published CLI) does not reach here. A
		// flag NAME is project-independent, so it does not re-open decision 36's
		// per-machine render axis.
		for _, want := range []string{
			"command not found", "command -v civitai", "shell profile",
			"civitai agent-setup --fix-path",
		} {
			if !strings.Contains(block, want) {
				t.Errorf("the %q block does not carry %q. Every row in it starts with `civitai`, "+
					"so a session whose shell cannot resolve that name has no way, from this file, "+
					"to find out why — which is the measured defect (0 of 8).", shape.Kind, want)
			}
		}
		// 🔴 NO ABSOLUTE PATH, EVER. This file is normally committed; a path from
		// the machine that generated it is wrong for everyone else who pulls.
		for _, forbidden := range []string{"/home/", "/Users/", ".npm-global/bin", "not on this machine's PATH"} {
			if strings.Contains(block, forbidden) {
				t.Errorf("the %q block carries %q — a machine-specific value in a file that gets "+
					"committed. The per-machine render axis was abandoned deliberately; see "+
					"decision 36.", shape.Kind, forbidden)
			}
		}
	}
}
