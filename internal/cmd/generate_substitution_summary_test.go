package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// The approval summary must not present the SUPERSEDED checkpoint as the one
// that will run.
//
// 🔴 THE DEFECT. The summary echoes the checkpoint as a NAME rather than an
// integer, deliberately, so the user approves something recognisable. With a
// substitution reported on the estimate that made the LAST model line before
// `Generate? [y/N]` the name the server had already said it would discard —
// observed live:
//
//	⚠ The server will NOT use the checkpoint you asked for …
//	  …
//	Checkpoint:   DreamShaper — 8 (Checkpoint, id 128713)
//
// The warning scrolls; the summary is what is on screen at the moment of
// approval. It is now annotated, never swapped: the user asked for the requested
// version and must still see that their own input was overridden.

// checkpointLine returns the summary's `Checkpoint:` line from a captured
// stream, or "".
//
// 🔴 THE ASSERTIONS BELOW ARE SCOPED TO THIS LINE ON PURPOSE. The substitution
// warning block a few lines above carries the same applied id and the same tense
// verb, so a whole-buffer Contains() would be satisfied by the warning and could
// not see whether the summary line was annotated at all — which is the entire
// bug.
func checkpointLine(t *testing.T, got string) string {
	t.Helper()
	for _, ln := range strings.Split(got, "\n") {
		if strings.Contains(ln, "Checkpoint:") {
			return ln
		}
	}
	return ""
}

// subCheckpointOpts is an invocation with --checkpoint set to the id the fixture
// substitution reports as `requested`.
func subCheckpointOpts() generateOpts {
	o := baseOpts()
	o.checkpoint = 999999999
	o.checkpointSet = true
	return o
}

// 🔴 THE HEADLINE ASSERTION, on the interactive confirmation — the surface where
// the user is actually approving the spend.
func TestGenerate_ConfirmSummaryMarksTheSupersededCheckpoint(t *testing.T) {
	withStdinTTY(t, true)

	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	c, _, errb := genCmd("n\n") // decline: the summary is printed before the prompt
	if err := runGenerate(c, s.deps(t), subCheckpointOpts()); err == nil {
		t.Fatal("declining must return an error")
	}
	if s.submitCalls != 0 {
		t.Fatalf("declining must never submit; got %d", s.submitCalls)
	}

	line := checkpointLine(t, errb.String())
	if line == "" {
		t.Fatalf("POSITIVE CONTROL FAILED: the confirmation printed no Checkpoint line, so nothing below is being checked:\n%s", errb.String())
	}
	// The user's own input is still there — the annotation supplements it.
	if !strings.Contains(line, "DreamShaper") {
		t.Errorf("the REQUESTED checkpoint must still be named — silently swapping in the applied one hides that the user was overridden; got %q", line)
	}
	if !strings.Contains(line, "SUPERSEDED") {
		t.Errorf("the checkpoint the server will NOT use must be marked at the moment of approval; got %q", line)
	}
	// ...and it names what supersedes it, or the mark is not actionable.
	if !strings.Contains(line, "2436219") {
		t.Errorf("the annotation must name the version that will run instead; got %q", line)
	}
}

// CONTRAST CONTROL: with no substitution the same line is UNCHANGED. Without
// this, an annotation printed unconditionally would pass every assertion above.
func TestGenerate_ConfirmSummaryUnchangedWithoutASubstitution(t *testing.T) {
	withStdinTTY(t, true)

	var s genSeams
	withQuote(&s, okQuote(12), okQuoteRaw(12)) // no ModelSubstitutions
	c, _, errb := genCmd("n\n")
	if err := runGenerate(c, s.deps(t), subCheckpointOpts()); err == nil {
		t.Fatal("declining must return an error")
	}

	line := checkpointLine(t, errb.String())
	if line == "" {
		t.Fatalf("POSITIVE CONTROL FAILED: no Checkpoint line at all:\n%s", errb.String())
	}
	if !strings.Contains(line, "DreamShaper") {
		t.Fatalf("POSITIVE CONTROL FAILED: the Checkpoint line carries no model name, so the check below is vacuous; got %q", line)
	}
	if strings.Contains(line, "SUPERSEDED") {
		t.Errorf("a run with no reported substitution must print the checkpoint unannotated; got %q", line)
	}
}

// The --dry-run quote is the OTHER surface that echoes the checkpoint, and it is
// the documented price-check-first workflow. A fix applied to only one of the two
// renderers is the mistake generate_charge_seam_test.go exists to remember.
func TestGenerate_DryRunQuoteMarksTheSupersededCheckpoint(t *testing.T) {
	withStdinTTY(t, false)

	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionGated), okQuoteRaw(12))
	o := subCheckpointOpts()
	o.dryRun = true

	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("dry run must succeed: %v", err)
	}

	line := checkpointLine(t, out.String())
	if line == "" {
		t.Fatalf("POSITIVE CONTROL FAILED: the quote printed no Checkpoint line:\n%s", out.String())
	}
	if !strings.Contains(line, "DreamShaper") {
		t.Errorf("the requested checkpoint must still be named in the quote; got %q", line)
	}
	if !strings.Contains(line, "SUPERSEDED") {
		t.Errorf("the --dry-run quote must mark a superseded checkpoint too; got %q", line)
	}
	if s.submitCalls != 0 {
		t.Fatalf("a dry run must never submit; got %d", s.submitCalls)
	}
}

// 🔴 THE PHASE THE CALL SITE PASSES, ASSERTED STRUCTURALLY (AGENTS.md item
// 21(f)). Both summary call sites are PRE-spend, so the annotation must be in
// the future tense: telling someone deciding whether to spend that the
// substitute already "ran" is the same class of defect as a lead announcing "HAS
// BEEN CHARGED" on --dry-run.
//
// The expected and forbidden texts are DERIVED from substitutionVerb, so a wrong
// phase argument at the call site moves the expectation instead of leaving a
// spelled word that some other line also happens to print.
func TestGenerate_CheckpointAnnotationUsesTheEstimateTense(t *testing.T) {
	withStdinTTY(t, true)

	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	c, _, errb := genCmd("n\n")
	_ = runGenerate(c, s.deps(t), subCheckpointOpts())

	line := checkpointLine(t, errb.String())
	if line == "" {
		t.Fatalf("POSITIVE CONTROL FAILED: no Checkpoint line:\n%s", errb.String())
	}

	want := fmt.Sprintf("the server %s version 2436219", substitutionVerb(substitutionAtEstimate))
	if !strings.Contains(line, want) {
		t.Errorf("the annotation must carry the ESTIMATE tense derived from the phase constant.\n  want: %q\n  line: %q", want, line)
	}
	// The other phases share one verb ("ran"); its rendering must be absent.
	for _, other := range []substitutionPhase{substitutionAfterSubmit, substitutionOnRead} {
		bad := fmt.Sprintf("the server %s version 2436219", substitutionVerb(other))
		if bad == want {
			t.Fatalf("phase %d renders identically to the estimate phase, so this assertion cannot discriminate", other)
		}
		if strings.Contains(line, bad) {
			t.Errorf("the annotation carries the WRONG phase's tense (%d): %q", other, line)
		}
	}
}

// --- the note function itself -------------------------------------------------

// The note is matched on the REQUESTED id. A substitution of something the
// summary line does not name must not annotate it — the line is accurate then,
// and a false mark on a correct line teaches the user to ignore the real one.
func TestSubstitutionCheckpointNote_MatchesOnTheRequestedID(t *testing.T) {
	o := baseOpts()
	o.checkpoint = 128713
	o.checkpointSet = true

	match := []genapi.ModelSubstitution{{Requested: 128713, Applied: 2154472, Reason: genapi.SubstitutionGated}}
	got := substitutionCheckpointNote("DreamShaper — 8", o, match, substitutionAtEstimate)
	if got == "" {
		t.Fatal("POSITIVE CONTROL FAILED: a record naming this very id produced no note")
	}
	if !strings.Contains(got, "2154472") {
		t.Errorf("the note must name the APPLIED version; got %q", got)
	}
	if strings.Contains(got, "128713") {
		t.Errorf("the note must not repeat the requested id — the label it hangs on already carries it; got %q", got)
	}

	// A record about a DIFFERENT id must leave the line alone.
	other := []genapi.ModelSubstitution{{Requested: 999, Applied: 2154472, Reason: genapi.SubstitutionGated}}
	if n := substitutionCheckpointNote("DreamShaper — 8", o, other, substitutionAtEstimate); n != "" {
		t.Errorf("a substitution of another id must not annotate this checkpoint; got %q", n)
	}
}

// Guard rails on the inputs that make an annotation meaningless or lost.
func TestSubstitutionCheckpointNote_SilentWhenThereIsNoLineToAnnotate(t *testing.T) {
	subs := []genapi.ModelSubstitution{{Requested: 128713, Applied: 2154472, Reason: genapi.SubstitutionGated}}

	set := baseOpts()
	set.checkpoint = 128713
	set.checkpointSet = true

	// POSITIVE CONTROL first: the fully-populated call DOES produce a note, so
	// every empty expectation below is a real discrimination.
	if substitutionCheckpointNote("DreamShaper — 8", set, subs, substitutionAtEstimate) == "" {
		t.Fatal("POSITIVE CONTROL FAILED: the note is never produced, so the cases below prove nothing")
	}

	// No label: the --input path prints no Checkpoint line at all, so an
	// annotation would be dropped on the floor.
	if n := substitutionCheckpointNote("", set, subs, substitutionAtEstimate); n != "" {
		t.Errorf("no checkpoint line means no annotation; got %q", n)
	}
	// --checkpoint not passed: the id in the record is not this caller's input.
	unset := baseOpts()
	unset.checkpoint = 128713
	if n := substitutionCheckpointNote("DreamShaper — 8", unset, subs, substitutionAtEstimate); n != "" {
		t.Errorf("no --checkpoint means no annotation; got %q", n)
	}
	// An empty record is AMBIGUOUS (AGENTS.md item 21(b)) — it may mean the
	// server predates the field. Nothing may be claimed from it in either
	// direction, so the line is printed exactly as it would be without the
	// feature.
	if n := substitutionCheckpointNote("DreamShaper — 8", set, nil, substitutionAtEstimate); n != "" {
		t.Errorf("an empty record must annotate nothing; got %q", n)
	}
}
