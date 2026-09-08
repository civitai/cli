package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The round-4 audit guards for `civitai agent-setup`.
//
// 🔴 EACH TEST SAYS WHICH KIND IT IS, for the reason the round-3 file gives: a
// blanket "every test below was watched red" is how a file comes to claim more
// regression coverage than it has. Count them from the labels:
//
//   - REGRESSION (watched red on c341be4, green after):
//     TestTheHeadlineReadsTheRowsRatherThanTheDryRunFlag's two blocked arms.
//     The two clean arms in the same table are OVER-WIDENING CONTROLS and are
//     labelled at the fixture.
//   - DOCUMENTATION GUARD, i.e. an invariant guard: TestADryRunCannotSeeAFailure
//     OnlyTheWriteCanProduce. It PASSES on c341be4. It pins the divergence the
//     README and `Long` now state positively, so that a later change which
//     silently removes it also removes the sentence.
//
// The round-4 case added to TestJSONNeverExitsSilently lives in the round-3 file
// beside the rest of that shape class rather than here, because the class is the
// guard and splitting it across files is how one half stops being read.
//
// 🔴 BLACK-BOX ON PURPOSE, for the same reason as rounds 2 and 3: a guard that
// names a symbol the fix introduced cannot be watched red — it fails to compile,
// which is not the same observation.

// ---------------------------------------------------------------------------
// F3 — the first line the user reads must describe the rows
// ---------------------------------------------------------------------------

// TestTheHeadlineReadsTheRowsRatherThanTheDryRunFlag.
//
// 🔴 MEASURED RED at c341be4: with a project directory at mode 0500,
// `civitai agent-setup --agent claude --dir proj` printed
//
//	Configured claude for Civitai App development in …/proj
//	  ✗ blocked …/AGENTS.md   ✗ blocked …/CLAUDE.md   ✗ blocked …/.mcp.json
//
// The verb branched on `dryRun` alone and on nothing else, so a run that wrote
// nothing at all opened by saying it had configured the agent. `ok`, the exit
// code and the error already read `blockedRowSummary`; the headline did not.
//
// This is the sibling of round 3's finding 8 ("the instruction files were
// written" on a run that wrote nothing) one surface over, and round 3 WIDENED
// which inputs reach it: a plan-time refusal used to be a bare `return err` with
// no report at all, and is now a `blocked` row under this headline.
//
// The fixtures use broken symlinks rather than a mode change so the blocked rows
// come from the PLAN — which means the dry-run arm is blocked too, and both the
// real run and the dry run can be asserted on one fixture.
func TestTheHeadlineReadsTheRowsRatherThanTheDryRunFlag(t *testing.T) {
	brokenLink := func(t *testing.T, from, toDir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(from), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(toDir, "gone", filepath.Base(from)), from); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}

	for _, tc := range []struct {
		name string
		// kind labels the fixture: a REGRESSION arm was watched red at c341be4,
		// a CONTROL arm passes there and pins the direction the fix could
		// over-shoot in (a headline that never says "Configured" again).
		kind  string
		setUp func(t *testing.T, dir, home string)
		// wantRealLead / wantDryLead are asserted as a PREFIX of the first line,
		// and wantBlocked is how many rows must carry `blocked` in both runs —
		// so a fixture that stops producing the outcome it was built for fails
		// as a broken premise rather than silently asserting nothing.
		wantBlocked int
		wantRealLead,
		wantDryLead string
	}{
		{
			name: "every step blocked", kind: "REGRESSION",
			setUp: func(t *testing.T, dir, home string) {
				brokenLink(t, filepath.Join(dir, agentsFilename), dir)
				brokenLink(t, filepath.Join(dir, claudeFilename), dir)
				brokenLink(t, filepath.Join(home, ".codex", "config.toml"), home)
			},
			wantBlocked:  3,
			wantRealLead: "Did NOT configure",
			wantDryLead:  "Would NOT configure",
		},
		{
			name: "one step blocked", kind: "REGRESSION",
			setUp: func(t *testing.T, dir, _ string) {
				brokenLink(t, filepath.Join(dir, agentsFilename), dir)
			},
			wantBlocked:  1,
			wantRealLead: "PARTIALLY configured",
			wantDryLead:  "Would PARTIALLY configure",
		},
		{
			name: "nothing blocked", kind: "CONTROL",
			setUp:        func(*testing.T, string, string) {},
			wantBlocked:  0,
			wantRealLead: "Configured",
			wantDryLead:  "Would configure",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []struct {
				name     string
				args     []string
				wantLead string
			}{
				{"dry run", []string{"--dry-run"}, tc.wantDryLead},
				{"real run", nil, tc.wantRealLead},
			} {
				t.Run(mode.name, func(t *testing.T) {
					dir, home := agentSetupProject(t)
					tc.setUp(t, dir, home)

					args := append([]string{"agent-setup", "--dir", dir, "--agent", agentCodex}, mode.args...)
					out, _, _ := run(t, args...)

					// PREMISE: the fixture really did produce the number of
					// blocked rows this row is about. Read from the SAME run,
					// through --json, so the two channels cannot disagree.
					jsonArgs := append(append([]string{}, args...), "--json")
					jsonOut, _, _ := run(t, jsonArgs...)
					var payload agentSetupJSON
					if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
						t.Fatalf("bad json: %v\n%q", err, jsonOut)
					}
					blocked := 0
					for _, c := range payload.Changes {
						if c.Action == actionBlocked {
							blocked++
						}
					}
					if blocked != tc.wantBlocked {
						t.Fatalf("PREMISE BROKEN: %d blocked row(s), want %d — this fixture no longer "+
							"produces the outcome the headline assertion is about:\n%s", blocked, tc.wantBlocked, jsonOut)
					}

					first, _, _ := strings.Cut(out, "\n")
					if !strings.HasPrefix(first, mode.wantLead+" ") {
						t.Errorf("first line does not lead with %q (%d of %d rows blocked):\n%s",
							mode.wantLead, blocked, len(payload.Changes), first)
					}
					// A run with blocked rows must not open with the bare
					// success verb — the specific defect, asserted separately
					// from the wording above so a later rewording of the
					// headline cannot quietly re-open it.
					if tc.wantBlocked > 0 && strings.HasPrefix(first, "Configured ") {
						t.Errorf("a run with %d blocked row(s) opens by claiming it configured the agent:\n%s",
							blocked, first)
					}
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// F2 — what a dry run can and cannot see
// ---------------------------------------------------------------------------

// TestADryRunCannotSeeAFailureOnlyTheWriteCanProduce is a DOCUMENTATION GUARD,
// not a regression test: it PASSES on c341be4, and the behaviour it pins is
// correct and wanted. A dry run performs no write, so a failure that only the
// act of writing can produce is invisible to it, by construction.
//
// It exists because the README and `Long` said the opposite — "the same rows,
// the same `ok` and the same exit code as the real run … for ANY of the three
// files" — while `TestAFirstWriteFailureDoesNotStopTheLaterOnes` in this same
// package REQUIRES the two runs to disagree for a case like this one, failing
// `PREMISE BROKEN` if they ever agree. Two surfaces of one PR, contradicting
// each other. The prose moved; this guard is what stops it moving back.
//
// 🔴 IT PINS A DIVERGENCE, NOT AN AGREEMENT, and that is deliberate. If a later
// change teaches the planner to predict this case, this test goes red — which is
// the correct outcome: the sentence it guards would then be wrong in the other
// direction and must be re-measured rather than inherited.
func TestADryRunCannotSeeAFailureOnlyTheWriteCanProduce(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — a 0500 directory is still writable")
	}
	dir, _ := agentSetupProject(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	dryOut, _, dryErr := run(t, "agent-setup", "--dry-run", "--json", "--dir", dir, "--agent", agentClaude)
	realOut, _, realErr := run(t, "agent-setup", "--json", "--dir", dir, "--agent", agentClaude)

	var dry, real agentSetupJSON
	if err := json.Unmarshal([]byte(dryOut), &dry); err != nil {
		t.Fatalf("bad --dry-run json: %v\n%q", err, dryOut)
	}
	if err := json.Unmarshal([]byte(realOut), &real); err != nil {
		t.Fatalf("bad write-run json: %v\n%q", err, realOut)
	}

	// The dry run sees a writable-looking plan: every row actionable, ok true,
	// exit 0. Nothing it can read says the write will fail.
	if dryErr != nil {
		t.Errorf("--dry-run exited non-zero (%v) — this fixture no longer exercises the divergence:\n%s", dryErr, dryOut)
	}
	if !dry.OK {
		t.Errorf("--dry-run reported ok: false, so the divergence this guards is gone:\n%s", dryOut)
	}
	for _, c := range dry.Changes {
		if c.Action == actionBlocked {
			t.Errorf("--dry-run blocked %s — the planner now predicts a write-time failure, so the "+
				"README sentence about what a dry run cannot see must be RE-MEASURED, not inherited: %s",
				c.Path, c.Reason)
		}
	}

	// The real run blocks every one of them.
	if realErr == nil {
		t.Fatalf("the real run exited 0 into an unwritable directory:\n%s", realOut)
	}
	if !errors.Is(realErr, ErrAgentSetupIncomplete) {
		t.Errorf("the real run's failure is not the incomplete-setup class: %v", realErr)
	}
	if real.OK {
		t.Errorf("the real run reported ok: true though nothing was written:\n%s", realOut)
	}
	for _, c := range real.Changes {
		if c.Action != actionBlocked {
			t.Errorf("the real run reported %q for %s in a directory it cannot write", c.Action, c.Path)
		}
	}
	// And nothing was written, by either run.
	for _, name := range []string{agentsFilename, claudeFilename, ".mcp.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s exists, so the fixture is not the unwritable case it claims to be", name)
		}
	}
}
