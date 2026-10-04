package cmd

import (
	"strings"
	"testing"
)

// pathNoteWindowsClaim is the WHOLE `--fix-path` claim the managed block makes,
// normalised to single spaces, and it is pinned as one string rather than as a
// set of keywords.
//
// 🔴 THE PAYLOAD WAS THE FIFTH SITE OF A CLAIM THE WINDOWS POSTURE FIX CORRECTED
// AT FOUR, AND THE ONLY ONE THAT SHIPS INSIDE THE BINARY. cli#777 round 3 gated
// `--fix-path` on `GOOS=windows` and swept the prose: the file header, the const
// doc, the no-gate block, the CRLF comments, audit1/audit2, decision 39, the
// README and `Long` help. This template was missed. A Windows run exits 1 saying
// the flag is not supported AND STILL WRITES THIS FILE — the project files are
// deliberately preserved and asserted byte-identical — so the coding agent then
// read, in the file that run had just created, that the refused flag "does that
// for you", directly under the `command not found` advice it was there to act on.
//
// 🔴 PINNED AS A NORMALISED SENTENCE BECAUSE THE ARTEFACT IS PROSE. A keyword
// guard on this is walkable by rewording, which is how this class kept
// recurring across five sites; the whole string is a machine-readable claim. The
// cost is deliberate: a cosmetic reword fails this test and the author must
// restate the claim here, which is the point rather than the friction.
//
// 🔴 IT NAMES PLATFORMS, NOT MACHINES, SO DECISION 36 IS UNTOUCHED. That rule is
// that the block may depend on the PROJECT and never on the MACHINE this run
// happened on. "macOS and Linux" and "Windows" are properties of the published
// binary's build matrix, identical in every copy of this file, and nothing here
// is read off the host — unlike the absolute paths the forbidden list below
// bans. See claudedocs/decisions/36-agents-block-per-project.md.
const pathNoteWindowsClaim = "`civitai agent-setup --fix-path` does that for you on macOS and " +
	"Linux: it writes a marker-guarded block into your shell startup files so a NEW shell resolves " +
	"`civitai` (add `--dry-run` to see the exact block first). On Windows that flag is refused and " +
	"writes no startup file, so add the directory through the Windows environment-variable settings, " +
	"or work inside WSL, where this CLI is a Linux build."

// normaliseProseForTest collapses every whitespace run to one space, so a claim
// can be pinned as a sentence while the template stays hard-wrapped.
func normaliseProseForTest(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

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
		// 🔴 AND THE CLAIM ABOUT THAT FLAG IS PINNED WHOLE, NOT BY KEYWORD. See
		// pathNoteWindowsClaim: a Windows run REFUSES `--fix-path` and still
		// writes this file, so an unqualified "does that for you" recommends,
		// in the file that run just created, the flag that run just declined.
		got := normaliseProseForTest(block)
		if !strings.Contains(got, pathNoteWindowsClaim) {
			t.Errorf("the %q block no longer carries the `--fix-path` claim verbatim.\n"+
				"want (normalised): %s\n"+
				"A Windows run exits 1 refusing this flag and STILL writes this file, so the claim "+
				"has to carry the platform qualifier or it recommends, in the file that run just "+
				"created, the flag that run just declined. This is pinned as a whole normalised "+
				"sentence rather than as keywords because the artefact is prose and a keyword guard "+
				"is walkable by rewording — if you reworded it on purpose, restate it in "+
				"pathNoteWindowsClaim.", shape.Kind, pathNoteWindowsClaim)
		}
		// 🔴 AND IT MUST BE UNCONTRADICTED, NOT MERELY PRESENT — cli#777 ROUND 5'S
		// A. `strings.Contains` proves the sentence is THERE; it is structurally
		// unable to see a second sentence beside it saying the opposite. Measured:
		// appending "In practice `--fix-path` works fine on Windows too, so just
		// run it and ignore the exit code." immediately after the pinned claim left
		// this test AND the whole `internal/cmd` package green, and that text was
		// then read back out of a real `AGENTS.md` this binary wrote — the exact
		// harm the pin exists to stop, shipped straight past the pin.
		//
		// A denylist of contradicting phrases cannot close it: the hazard has
		// unboundedly many spellings, and a guard on WORDS is walkable by
		// REWORDING. So this pins a RELATIONSHIP instead — the block may discuss
		// Windows ONLY inside the claim. Strip the claim and no mention may remain,
		// whatever the wording. Adding Windows prose on purpose therefore means
		// restating pathNoteWindowsClaim, which is the discipline the verbatim pin
		// above already asks for.
		//
		// ⚠ RESIDUAL, STATED BECAUSE A RELATIONSHIP GUARD INVITES BEING READ AS
		// TOTAL: a contradiction that never names Windows ("works on every
		// platform") is NOT caught. This closes the measured shape and every
		// rewording of it, not the whole class.
		if rest := strings.Replace(got, pathNoteWindowsClaim, "", 1); strings.Contains(rest, "Windows") {
			t.Errorf("the %q block mentions Windows OUTSIDE the pinned `--fix-path` claim. The "+
				"claim being present does not make it true of the file: a sentence beside it can "+
				"contradict it, and a measured one did exactly that with this package green and "+
				"the text landing in a real AGENTS.md. Windows is discussed inside "+
				"pathNoteWindowsClaim or not at all — fold the wording into that constant.\n"+
				"block with the claim removed: %s", shape.Kind, rest)
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
