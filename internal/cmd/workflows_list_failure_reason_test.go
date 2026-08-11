package cmd

import (
	"strings"
	"testing"
)

// civitai/cli#382 — `workflows list` rendered `failed … 0/1` with no reason
// while the server had handed one back on the SAME response, at
// `steps[].errors`. These tests cover the rendering; the wire shape is pinned in
// internal/genapi/listed_failure_reason_test.go against a real capture.
//
// 🔴 THE HAZARD THE CHOSEN LAYOUT CREATES. The reason is external-provider free
// text printed directly under a table of rows, and safeTerm keeps `\n` on
// purpose, so an unindented reason could put attacker-chosen text at column
// zero — a forged workflow row, with a fake id, status and cost, which is more
// convincing than the forged banner and the forged refund claim already measured
// on the #367 surfaces. The guards below assert the STATE that makes forgery
// impossible (nothing but a real row starts at column zero), never the absence
// of one spelling of an attack.

// listPage builds a one-page payload from row specs. `errors` is written
// verbatim into the JSON, so a case can pass `null`, `[]`, or omit the key.
func listPage(rows ...string) string {
	return `{"items":[` + strings.Join(rows, ",") + `],"nextCursor":null}`
}

// listRow is one workflow with a single step whose `errors` is the raw JSON in
// errorsJSON. An empty errorsJSON omits the key entirely.
func listRow(id, status, errorsJSON string) string {
	step := `{"$type":"imageGen","name":"$0","status":"` + status + `",` +
		`"output":[{"id":"` + id + `_out","available":true}]`
	if errorsJSON != "" {
		step += `,"errors":` + errorsJSON
	}
	step += `}`
	return `{"id":"` + id + `","status":"` + status + `","createdAt":"2026-08-05T12:00:00Z",` +
		`"cost":{"total":0},"steps":[` + step + `]}`
}

// renderList runs the human renderer over a payload and returns stdout, stderr.
func renderList(t *testing.T, payload string) (string, string) {
	t.Helper()
	c, out, errb := genCmd("")
	if err := runWorkflowsList(c, wfListDeps(payload, nil, nil, nil), workflowsListOpts{}); err != nil {
		t.Fatalf("workflows list: %v", err)
	}
	return out.String(), errb.String()
}

// columnZeroLines returns every rendered line that begins at column zero — the
// set a forged row would have to join.
func columnZeroLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if l != "" && !strings.HasPrefix(l, " ") {
			out = append(out, l)
		}
	}
	return out
}

// --- the reason reaches the screen ------------------------------------------

func TestWorkflowsList_PrintsTheServerReasonUnderItsRow(t *testing.T) {
	const reason = "Google Gemini: FIXTURE could not generate images with the given prompts."
	stdout, stderr := renderList(t, listPage(
		listRow("wf_ok", "succeeded", `null`),
		listRow("wf_bad", "failed", `["`+reason+`"]`),
	))

	if !strings.Contains(unwrapFinding(stdout), reason) {
		t.Fatalf("the reason the server sent is not on screen:\n%s", stdout)
	}
	// It belongs to ITS row, so it must follow it.
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	rowAt, reasonAt := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "wf_bad") {
			rowAt = i
		}
		if strings.HasPrefix(l, listReasonIndent) && strings.Contains(l, "FIXTURE") {
			reasonAt = i
		}
	}
	if rowAt < 0 || reasonAt < 0 || reasonAt != rowAt+1 {
		t.Errorf("the reason is at line %d and its row at line %d; it must be the line after its own row:\n%s",
			reasonAt, rowAt, stdout)
	}
	// The succeeded row carries nothing, and nothing empty is printed for it.
	if strings.Contains(stdout, "wf_ok\n"+listReasonIndent) {
		t.Errorf("a workflow with no reason got a reason line:\n%s", stdout)
	}
	if !strings.Contains(stderr, listReasonLegend) {
		t.Errorf("the legend explaining the indented lines is missing:\n%s", stderr)
	}
}

// The three ways the server says "none". `null` is the one it actually sends —
// `formatGenerationResponse2` emits `errors: … : undefined` — and a `failed`
// workflow carrying none is a MEASURED branch (1 of 6 failures in the sample),
// so the renderer must print nothing at all rather than an empty label.
func TestWorkflowsList_NoReasonPrintsNothingAndNoLegend(t *testing.T) {
	for _, c := range []struct{ name, errs string }{
		{"null", `null`},
		{"empty array", `[]`},
		{"key absent", ``},
		{"blank entries only", `["   ","\t"]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr := renderList(t, listPage(listRow("wf_bad", "failed", c.errs)))
			// POSITIVE CONTROL: the row itself rendered, so the absence below is
			// about the reason and not about nothing having been printed.
			if !strings.Contains(stdout, "wf_bad") {
				t.Fatalf("CONTROL failure, not a finding: the row did not render:\n%s", stdout)
			}
			for _, l := range strings.Split(stdout, "\n") {
				if strings.HasPrefix(l, listReasonIndent) {
					t.Errorf("an indented line was printed for a workflow with no reason: %q", l)
				}
			}
			if strings.Contains(stderr, listReasonLegend) {
				t.Errorf("the legend explains indented lines that were never printed:\n%s", stderr)
			}
		})
	}
}

// --- the forgery guard ------------------------------------------------------

// 🔴 A ≥3-LINE REASON, BECAUSE A 2-LINE ONE CANNOT SEE THE DEFECT. An
// indent-only-the-second-line bug — a `break` after the first continuation, an
// off-by-one in the loop bound — is invisible at two lines and obvious at four.
// #381 found exactly that gap.
//
// The assertion is the STATE: the lines that begin at column zero must be
// EXACTLY the header, the real rows, and the CLI's own trailer. A forged row is
// any line the attacker's text puts in that set. This cannot be satisfied by an
// attack spelled differently, which is the failure mode #367 hit four rounds
// running.
func TestWorkflowsList_AMultiLineReasonCannotForgeARow(t *testing.T) {
	forged := "FIXTURE the run failed\n" +
		"wf_forged                  succeeded  2026-08-05T12:00:00Z  0     4/4\n" +
		"wf_forged2                 succeeded  2026-08-05T12:00:00Z  0     4/4\n" +
		"Next cursor: attacker-controlled"
	stdout, _ := renderList(t, listPage(
		listRow("wf_real1", "succeeded", `null`),
		listRow("wf_real2", "failed", `["`+strings.ReplaceAll(forged, "\n", `\n`)+`"]`),
	))

	// POSITIVE CONTROL: the reason really rendered. Asserting only that no
	// forged row exists is a reassuring zero — indistinguishable from a renderer
	// that dropped the reason entirely.
	if !strings.Contains(stdout, "FIXTURE the run failed") {
		t.Fatalf("CONTROL failure, not a finding: the reason never rendered, so the absence below proves "+
			"nothing:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wf_forged") {
		t.Fatalf("CONTROL failure, not a finding: the attacker text was dropped rather than moved, so this "+
			"test is not exercising the hazard:\n%s", stdout)
	}

	got := columnZeroLines(stdout)
	want := map[string]bool{}
	for _, l := range got {
		if strings.HasPrefix(l, "WORKFLOW ID") || strings.HasPrefix(l, "wf_real1") || strings.HasPrefix(l, "wf_real2") {
			want[l] = true
			continue
		}
		t.Errorf("a line the CLI did not write starts at column zero, where every real row starts, so it "+
			"reads as a workflow of the user's own: %q\n\nfull output:\n%s", l, stdout)
	}
	if len(want) != 3 {
		t.Errorf("expected the header and 2 rows at column zero, got %d:\n%s", len(want), stdout)
	}
	// And the invariant that produces that state, asserted directly: EVERY line
	// carrying reason text is indented. Stated as a property of all of them, not
	// as "the second one is".
	reasonLines := 0
	for _, l := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		if !strings.Contains(l, "FIXTURE") && !strings.Contains(l, "wf_forged") && !strings.Contains(l, "attacker") {
			continue
		}
		reasonLines++
		if !strings.HasPrefix(l, listReasonIndent) {
			t.Errorf("a line of the server's reason is not indented: %q", l)
		}
	}
	if reasonLines < 4 {
		t.Errorf("CONTROL failure, not a finding: only %d reason lines rendered from a 4-line reason, so a "+
			"first-line-only defect could pass:\n%s", reasonLines, stdout)
	}
}

// A tab in a reason is its own alignment hazard next to a tabwriter. It is safe
// only because the reason is written straight to stdout and never through the
// tabwriter — so the table above must be byte-identical to the same table
// rendered without any reason at all.
//
// 🔴 THIS IS THE "TABLE STAYS ALIGNED" ASSERTION, AND IT IS A COMPARISON
// AGAINST A BASELINE RATHER THAN A COLUMN COUNT. A guard that checked "the
// columns line up with each other" would pass a renderer that re-aligned every
// group of rows independently, which is precisely what interleaving the reason
// lines through the tabwriter would do.
func TestWorkflowsList_ReasonsDoNotDisturbTheTable(t *testing.T) {
	rows := []string{
		listRow("wf_short", "succeeded", `null`),
		listRow("wf_a_much_longer_id_here", "failed", `["FIXTURE A\twith a tab"]`),
		listRow("wf_mid", "succeeded", `null`),
	}
	withReasons, _ := renderList(t, listPage(rows...))

	bare := []string{
		listRow("wf_short", "succeeded", `null`),
		listRow("wf_a_much_longer_id_here", "failed", `null`),
		listRow("wf_mid", "succeeded", `null`),
	}
	without, _ := renderList(t, listPage(bare...))

	// CONTROL: the two renderings must actually differ, or the equality below is
	// vacuous.
	if withReasons == without {
		t.Fatalf("CONTROL failure, not a finding: adding a reason changed nothing at all:\n%s", withReasons)
	}

	// (1) EVERY ROW AGREES ON WHERE THE COLUMNS START. This is the half a
	// baseline comparison structurally cannot see: a renderer that flushed the
	// tabwriter per row — or per group of rows either side of a reason — would
	// degrade the baseline in exactly the same way, so the two would still match
	// while every column jumped on screen.
	rowsOf := columnZeroLines(withReasons)
	if len(rowsOf) < 4 {
		t.Fatalf("CONTROL failure, not a finding: %d column-zero lines, want a header and 3 rows:\n%s",
			len(rowsOf), withReasons)
	}
	at := strings.Index(rowsOf[0], "STATUS")
	if at < 0 {
		t.Fatalf("CONTROL failure, not a finding: no STATUS header to measure against:\n%s", withReasons)
	}
	for _, r := range rowsOf[1:] {
		if got := len(strings.TrimRight(strings.SplitN(r, " ", 2)[0], " ")); got >= at {
			continue // an id wider than the header legitimately pushes the column
		}
		if !strings.HasPrefix(r[at:], "succeeded") && !strings.HasPrefix(r[at:], "failed") {
			t.Errorf("row %q does not put its STATUS at column %d, where the header does — the rows are not "+
				"in one alignment block:\n%s", r, at, withReasons)
		}
	}

	// (2) AND THE ROWS ARE BYTE-IDENTICAL TO THE SAME TABLE WITH NO REASON IN
	// IT, which is the half a self-consistency check cannot see: a renderer
	// whose row text depended on whether a reason existed would still be
	// internally consistent.
	if got, want := columnZeroLines(withReasons), columnZeroLines(without); len(got) != len(want) {
		t.Fatalf("row count changed: %d vs %d", len(got), len(want))
	} else {
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("row %d re-aligned when a reason was added:\n  with:    %q\n  without: %q\n\n"+
					"the reason lines must not pass through the tabwriter — a line with no tab ENDS a "+
					"column block, so every group of rows would be aligned independently", i, got[i], want[i])
			}
		}
	}
}

// 🔴 THE SPLICE IS PAIRED BY LINE COUNT, NOT BY INDEX, AND THIS IS THE CASE
// THAT DISTINGUISHES THEM. safeTerm keeps `\n` in every server-origin cell, ids
// included, so one row can occupy two output lines. Pairing reason blocks with
// rows by position in the flushed buffer would then hand this workflow's reason
// to the next workflow's row — a wrong cause attached to the wrong id, silently.
// The ragged column an embedded newline produces is pre-existing and is not what
// this asserts.
func TestWorkflowsList_AReasonStaysWithItsRowWhenAnIDSpansTwoLines(t *testing.T) {
	stdout, _ := renderList(t, listPage(
		listRow(`wf_split\nsecond_half`, "succeeded", `null`),
		listRow("wf_bad", "failed", `["FIXTURE A"]`),
	))
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	// CONTROL: the id really did span two lines, or this test is the ordinary
	// case wearing a different name.
	if !strings.Contains(stdout, "second_half") {
		t.Fatalf("CONTROL failure, not a finding: the embedded newline did not render:\n%s", stdout)
	}
	rowAt, reasonAt := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "wf_bad") {
			rowAt = i
		}
		if strings.Contains(l, "FIXTURE A") {
			reasonAt = i
		}
	}
	if rowAt < 0 || reasonAt != rowAt+1 {
		t.Errorf("the reason landed at line %d and wf_bad's row at line %d. A reason attached to another "+
			"workflow's row tells the user the wrong cause for the wrong run:\n%s", reasonAt, rowAt, stdout)
	}
}

// --- attribution ------------------------------------------------------------

// 🔴 THE #381 SHAPE, ASKED AT THIS SURFACE. There, two excluded outputs whose
// reason sets stood in a SUBSET relation lost one of the two causes to a gate
// keyed on the wrong thing. Nothing on this path has such a gate — each row
// renders its own workflow's reasons and no row's set is compared against
// another's — and this test is what makes that a checked property rather than a
// claim in a commit message. If a de-duplication gate is ever added ACROSS rows,
// this reddens.
func TestWorkflowsList_EachRowKeepsItsOwnReasonsUnderSubsetOverlap(t *testing.T) {
	stdout, _ := renderList(t, listPage(
		listRow("wf_1", "failed", `["FIXTURE A","FIXTURE B"]`),
		listRow("wf_2", "failed", `["FIXTURE A"]`),
		listRow("wf_3", "failed", `["FIXTURE A","FIXTURE B"]`),
	))
	blocks := reasonBlocksByRow(stdout)
	for id, want := range map[string][]string{
		"wf_1": {"FIXTURE A", "FIXTURE B"},
		"wf_2": {"FIXTURE A"},
		"wf_3": {"FIXTURE A", "FIXTURE B"},
	} {
		got := blocks[id]
		if len(got) != len(want) {
			t.Errorf("%s rendered %d reason line(s), want %d: %q\n\nA row that lost its own cause because "+
				"another row had already said it is the #381 regression at a new surface.", id, len(got), len(want), got)
			continue
		}
		for i := range want {
			if !strings.Contains(got[i], want[i]) {
				t.Errorf("%s reason %d = %q, want it to contain %q", id, i, got[i], want[i])
			}
		}
	}
}

// A multi-step workflow that died the same way in every step says it ONCE. The
// server dedupes within a step and not across them, so this is the shape it
// really hands over.
func TestWorkflowsList_RepeatedCauseAcrossStepsIsSaidOnce(t *testing.T) {
	page := `{"items":[{"id":"wf_1","status":"failed","createdAt":"2026-08-05T12:00:00Z","cost":{"total":0},"steps":[` +
		`{"status":"failed","output":[],"errors":["FIXTURE A"]},` +
		`{"status":"failed","output":[],"errors":["FIXTURE A"]},` +
		`{"status":"failed","output":[],"errors":["FIXTURE B"]}]}],"nextCursor":null}`
	stdout, _ := renderList(t, page)
	if n := strings.Count(stdout, "FIXTURE A"); n != 1 {
		t.Errorf("one cause repeated across steps printed %d times:\n%s", n, stdout)
	}
	if n := strings.Count(stdout, "FIXTURE B"); n != 1 {
		t.Errorf("the cause that DIFFERS printed %d times — de-duplication must not swallow it:\n%s", n, stdout)
	}
}

// reasonBlocksByRow maps a workflow id to the indented lines rendered under it.
func reasonBlocksByRow(stdout string) map[string][]string {
	out := map[string][]string{}
	cur := ""
	for _, l := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "wf_"):
			cur = strings.Fields(l)[0]
		case cur != "" && strings.HasPrefix(l, listReasonIndent):
			out[cur] = append(out[cur], strings.TrimSpace(l))
		default:
			cur = ""
		}
	}
	return out
}

// --- the text is the server's ----------------------------------------------

// 🔴 THE `Google Gemini: ` PREFIX IS SERVER TEXT, NOT A FIELD TO PARSE. It is
// composed by `sanitizeProviderError` from a 16-entry provider table the CLI
// must never vendor (AGENTS.md item 13). This pins that the CLI neither strips
// it, rewrites it, nor truncates the sentence it introduces.
func TestWorkflowsList_ReasonIsPassedThroughVerbatim(t *testing.T) {
	const reason = "Google Gemini: Could not generate images with the given prompts and images. " +
		"Please try again with different inputs."
	stdout, _ := renderList(t, listPage(listRow("wf_bad", "failed", `["`+reason+`"]`)))
	// unwrapFinding collapses the hard wrap back to one logical line, so this
	// asserts the CONTENT without pinning where the layout chose to break.
	if !strings.Contains(unwrapFinding(stdout), reason) {
		t.Errorf("the reason was altered on its way to the screen. It must arrive whole — no truncation, no "+
			"prefix stripping, no re-wording:\n%s", stdout)
	}
}

// Long reasons are WRAPPED, not truncated — and the wrap is what keeps the
// indent meaningful, because a line longer than the terminal is soft-wrapped by
// the TERMINAL back to column zero, where indentContinuation cannot reach it.
func TestWorkflowsList_ALongReasonIsWrappedAndNotTruncated(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("FIXTURE reason word ", 30))
	stdout, _ := renderList(t, listPage(listRow("wf_bad", "failed", `["`+long+`"]`)))
	if !strings.Contains(unwrapFinding(stdout), long) {
		t.Errorf("a long reason lost text — it must be wrapped, never truncated:\n%s", stdout)
	}
	wrapped := 0
	for _, l := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		if !strings.Contains(l, "FIXTURE") {
			continue
		}
		wrapped++
		if !strings.HasPrefix(l, listReasonIndent) {
			t.Errorf("a wrapped line lost the indent, which is what stops it reading as a row: %q", l)
		}
		if n := len([]rune(l)); n > findingWrapWidth {
			t.Errorf("line of %d runes exceeds the %d-column budget, so the terminal will soft-wrap it back "+
				"to column zero: %q", n, findingWrapWidth, l)
		}
	}
	if wrapped < 3 {
		t.Errorf("CONTROL failure, not a finding: a %d-rune reason produced only %d line(s), so this test "+
			"is not exercising the wrap", len([]rune(long)), wrapped)
	}
}
