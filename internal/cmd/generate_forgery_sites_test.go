package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/civitai/cli/internal/genapi"
)

// civitai/cli#604 — THE `generate` COMMAND RENDERED SERVER TEXT THROUGH PLAIN
// safeTerm AT THIRTEEN SITES, AND safeTerm KEEPS \n AND \t BY DESIGN.
//
// `internal/saferune` retains the newline and the tab deliberately (its package
// doc states the class), so `safeTerm` alone does NOT make a single-line surface
// single-line. `safeTermSingle` is the gate that does. civitai/cli#577 moved the
// download path onto it and #596 moved generate's blob path; this file is the
// rest of the generate command — the wait path's error returns, the submit and
// re-attach blocks, the --no-download URL listing, the error classifier and both
// poll reporters.
//
// 🔴 THE WORST OF THEM WRITES TO STDOUT, NOT STDERR. printOutputURLs renders
// `fmt.Fprintf(out, "%d\t%s\n", i+1, <url>)` — so a presigned URL carrying
// "\n2\t<other-url>" produces a second, fully attacker-written numbered row on
// the surface this CLI documents for piping. Every other site here writes to
// stderr. Its test therefore asserts the ROW COUNT and the FIELD COUNT per row,
// not the absence of any particular string: a guard spelled against the
// attacker's payload is walkable by respelling, a geometry guard is not.
//
// 🔴 AND THE POLL REPORTERS RUN ON EVERY WAITING GENERATE. The sites #604's
// original table covered fire on the timeout / no-deliverable / submit paths;
// `(*ttyPollReporter).tick` renders the server's status on the happy path of
// every `civitai generate` that waits. Their test drives a NON-TERMINAL status
// through the real `pollWorkflow`, because a test that only drove a terminal one
// would leave that normal-path surface undriven.
//
// # WHICH ASSERTIONS ARE LIVE HERE, AND WHICH WOULD BE INERT
//
// 🔴 ui's lipgloss styles EXPAND \t TO FOUR SPACES before the bytes reach the
// writer. Re-measured on this tree rather than carried over: `ui.For(w).Dim`,
// `.Info`, `.Warn`, `.Bold` and `.Success` all render "a\tb" as "a    b", while
// all of them pass "a\nb" through unchanged. So on a ui-styled surface a
// `Contains(got, "\t")` guard can NEVER fire whether or not the gate exists —
// the unreachable-guard shape this repo counts as worse than no guard — and the
// newline assertion is the one that discriminates. Each test below says which
// case it is in.
//
// 🔴 ONE OPERAND IS NOT LINE-FORGEABLE AT ALL TODAY, AND ITS TEST SAYS SO
// RATHER THAN PRETENDING OTHERWISE — see
// TestWaitAndCollectTerminalStatusErrorCannotForgeALine.

// gfsHostileID is the server-supplied workflow id. It carries the two runes
// saferune deliberately keeps, and its tail is a complete, plausible line so a
// survivor reads as the lie it is rather than as mangled text. No path
// separator: the id also lands in the output FILE NAME via renderOutName on the
// paths that download, and a slash there refuses earlier, in a different line.
const gfsHostileID = "wf1\nSaved out.png (2.0 MiB)\tDONE"

// gfsBenignID is the differential control: the same shape with none of the
// class, so "a hostile value did not change the geometry" is a comparison
// between two real renders rather than against a hand-counted number.
const gfsBenignID = "wf1"

// gfsHostileStatus is a NON-TERMINAL status, which is what the poll reporters
// and the re-attach block render. Non-terminal is the point: it is the status
// every waiting generate displays while the job runs.
const gfsHostileStatus = "processing\nqueued position 1\tETA 2s"

const gfsBenignStatus = "processing"

// gfsHostileServerMessage is the SERVER's own error message, which the quiet
// poll reporter renders beside the status on the same Fprintf.
//
// 🔴 ITS TAIL IMPERSONATES finish()'s OWN LINE, WHICH IS THE POINT. genapi's
// generateError interpolates this string into every arm and APIError.Error()
// returns it verbatim, so an ungated `%v` lets the server write whole lines into
// a waiting generate's output. `  status <terminal>` is exactly what
// (*quietPollReporter).finish prints one frame later — same two-space indent,
// same word — so a survivor is not mangled text, it is the CLI appearing to
// report a finished, saved generation that never happened.
const gfsHostileServerMessage = "boom\n  status succeeded\tSaved out.png (2.0 MiB)"

// gfsBenignServerMessage is the differential control: same construction, none of
// the retained class.
const gfsBenignServerMessage = "boom"

// gfsHostileTerminalStatus is the most hostile value that can REACH the
// terminal-status error, and the bound is worth stating: genapi.IsTerminalStatus
// lowercases and TrimSpace's before matching its set, so a server can pad a
// known terminal status with the retained runes but cannot put words of its own
// in the operand and still exit the poll loop.
const gfsHostileTerminalStatus = "\tfailed\n"

// gfsLines splits a rendered block into its lines, ignoring the trailing
// newline that ends the last one.
func gfsLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// assertNoControlEscapes is the assertion for an operand rendered through `%q`.
//
// 🔴 IT EXISTS BECAUSE assertOneLine IS INERT ON A `%q` OPERAND, AND SHIPPING
// AN ASSERTION THAT CANNOT FIRE IS THE DEFECT THIS REPO NAMES OVER AND OVER.
// strconv.Quote escapes \n and \t into their two-character forms, so a `%q`
// operand cannot forge a line with or without the gate — the line-count
// assertion passes identically either way and would report a mutation as
// killed while measuring nothing. What DOES move is whether the message shows
// escape gibberish where a status belongs: gated, the operand arrives collapsed
// to spaces; ungated, the rendered text carries a literal backslash-n.
func assertNoControlEscapes(t *testing.T, surface, got string) {
	t.Helper()
	for _, esc := range []string{`\n`, `\t`} {
		if strings.Contains(got, esc) {
			t.Errorf("#604 UNGATED OPERAND in %s: the rendered text carries a literal %s escape, so a "+
				"retained control rune reached a `%%q` operand instead of being collapsed by "+
				"safeTermSingle:\n%s", surface, esc, got)
		}
	}
}

// gfsWorkflowJSON builds a workflow reply with an arbitrary status, including
// one carrying the runes a Go raw string cannot hold.
func gfsWorkflowJSON(status string) string {
	return fmt.Sprintf(`{"id":"wf_1","status":%s,"steps":[]}`, strconv.Quote(status))
}

// TestWaitAndCollectTerminalStatusErrorCannotForgeALine drives the
// non-succeeded terminal-status error — the message a user gets when a
// generation they paid for ended failed/expired/canceled.
//
// It drives the REAL command, so both operands are values the poll path threads
// through several frames rather than arguments handed straight to a printer.
//
// 🔴 THE TWO OPERANDS ON THIS ONE LINE NEED TWO DIFFERENT ASSERTIONS, AND
// SAYING WHY IS THE POINT OF THIS COMMENT. The workflow id renders through
// `%s`, so an ungated newline in it forges a whole terminal line and
// assertOneLine sees that. The status renders through `%q`, which escapes the
// same rune — so it cannot forge a line today for TWO independent reasons: the
// verb, and IsTerminalStatus bounding the value to a whitespace-padded known
// status. The gate is still applied there (one rule, one place: six of seven
// gated lines in one block is how the seventh regenerates, and the gate must not
// depend on a format verb three tokens away staying `%q`), and
// assertNoControlEscapes is the assertion that can actually observe it.
func TestWaitAndCollectTerminalStatusErrorCannotForgeALine(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0

	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, gfsWorkflowJSON(gfsHostileTerminalStatus))
	s.submitReply = &genapi.SubmitResult{ID: gfsHostileID, Status: "queued"}

	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a terminal non-succeeded status reported success")
	}
	got := err.Error()
	if !strings.Contains(got, "produced no usable result") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error than the terminal-status one, "+
			"so this test drives the wrong surface:\n%s", got)
	}
	// POSITIVE CONTROLS: both operands must reach the message, or the assertions
	// below are taken on a string that never carried either payload.
	if !strings.Contains(got, "wf1") {
		t.Fatalf("CONTROL failure, not a finding: the workflow id never reached the message:\n%s", got)
	}
	if !strings.Contains(got, "failed") {
		t.Fatalf("CONTROL failure, not a finding: the server status never reached the message:\n%s", got)
	}
	// 🔴 THIS FIXTURE CARRIES NO FAILURE REASON, SO THE WHOLE ERROR IS ONE LINE
	// AND IS ASSERTED AS SUCH. The reason-carrying case — where the message ends
	// in a deliberately multi-line suffix — is its own test below, because the two
	// need different assertions and merging them produced a walkable guard.
	//
	// 🔴 DO NOT REINTRODUCE `strings.Cut(got, ". The server reported:")` HERE.
	// Round 1 found that asserting one-line over the whole string falsely accuses
	// a legitimate multi-line reason; the fix for THAT split the string on the
	// suffix marker, and round 2 measured the split walkable: `workflowID` is
	// server-origin and lands BEFORE the marker, so an id of
	// "wf1. The server reported: ok\nSaved …" moves the real newline into the
	// discarded tail. With the gate reverted that test PASSED — a fully forged
	// line surviving every assertion, because assertNoControlEscapes matches the
	// two-character `\n` that `%q` emits, not a raw newline.
	// One value, one guard, and the guard was spelled against a boundary the
	// attacker can spell too.
	// CONTROL, and it is a SUFFIX check on purpose. serverReasonSuffix is appended
	// LAST, so what the message ends with is the CLI's own, whatever the server
	// spelled in an earlier operand. A `Contains`/`Cut` on the same text would be
	// displaceable; `HasSuffix` is not.
	noReason := " (" + noFailureReasonNote + ")"
	if !strings.HasSuffix(got, noReason) {
		t.Fatalf("CONTROL failure, not a finding: this fixture's `\"steps\":[]` is supposed to "+
			"produce NO failure reason, so the message must end in the no-reason note and be "+
			"single-line throughout. It does not, so the assertion below is testing something "+
			"other than what this test documents:\n%s", got)
	}
	assertOneLine(t, "waitAndCollect (terminal-status error, workflow id)", got)
	assertNoControlEscapes(t, "waitAndCollect (terminal-status error, server status)", got)
}

// TestWaitAndCollectTerminalStatusErrorWithAReasonStaysMultiLine is the OTHER
// half, and it exists because the first two attempts at this coverage were each
// wrong in a different direction.
//
// Round 1: asserting one-line over the whole message falsely accused a
// legitimate multi-line reason — serverReasonSuffix renders it through
// indentContinuation(safeTerm(reason)) ON PURPOSE (generate.go:144, pinned by
// name in indentcontinuation_ledger_test.go), so the guard was crying forgery at
// correct behaviour.
//
// Round 2: the fix for that split the rendered string on ". The server reported:"
// and asserted one-line on the head. MEASURED WALKABLE — `workflowID` is
// server-origin and lands BEFORE that marker, so an id of
// "wf1. The server reported: ok\nSaved …" pushes the real newline into the
// discarded tail. With the workflowID gate reverted, that test PASSED: a fully
// forged terminal line survived every assertion, because assertNoControlEscapes
// matches the two-character `\n` that `%q` emits, not a raw newline.
//
// 🔴 SO THIS TEST SUBTRACTS THE SUFFIX IT INJECTED RATHER THAN SEARCHING FOR A
// MARKER. It knows the reason it put in, so it can compute exactly what
// serverReasonSuffix must append and TrimSuffix that. Nothing the server spells
// in another operand can displace it: if the suffix does not match, the
// subtraction fails loudly instead of silently discarding a forgery.
func TestWaitAndCollectTerminalStatusErrorWithAReasonStaysMultiLine(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0

	// A legitimate multi-line failure reason — the case round 1 proved the old
	// assertion accused falsely, and which no shipped fixture drove until now.
	const reason = "the model could not be loaded:\nout of memory on the worker"
	payload := fmt.Sprintf(
		`{"id":"wf_1","status":%s,"steps":[{"$type":"textToImage","name":"s","status":"failed",`+
			`"metadata":{},"output":{"errors":[%s]}}]}`,
		strconv.Quote(gfsHostileTerminalStatus), strconv.Quote(reason))

	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.submitReply = &genapi.SubmitResult{ID: gfsHostileID, Status: "queued"}

	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a terminal non-succeeded status reported success")
	}
	got := err.Error()

	// The suffix this test's OWN reason must produce, built the way production
	// builds it. Subtracting this is the structural move; searching for a literal
	// is the walkable one.
	wantSuffix := ". The server reported: " + indentContinuation(safeTerm(reason), "  ")
	head, found := strings.CutSuffix(got, wantSuffix)
	if !found {
		t.Fatalf("CONTROL failure, not a finding: the message does not end in the suffix this "+
			"test injected, so the subtraction below is not removing what it thinks it is.\n"+
			"want suffix: %q\ngot: %q", wantSuffix, got)
	}
	// POSITIVE CONTROL: the reason really did reach the message and really is
	// multi-line, or this test proves nothing about the multi-line surface.
	if !strings.Contains(wantSuffix, "\n") {
		t.Fatal("CONTROL failure, not a finding: the injected reason rendered single-line, so " +
			"this test is not driving the multi-line surface it exists for")
	}
	// The head is everything the gates own. It must be one line even though the
	// message as a whole is legitimately three.
	assertOneLine(t, "waitAndCollect (terminal-status error WITH a reason, head)", head)
	assertNoControlEscapes(t, "waitAndCollect (terminal-status error WITH a reason)", got)
}

// TestWaitAndCollectNoDeliverablesErrorCannotForgeALine drives the other error
// return: a workflow the server reports SUCCEEDED whose outputs never landed.
// It is the case where the CLI is least able to explain itself, and the id in it
// is the user's only handle on a run that was charged.
func TestWaitAndCollectNoDeliverablesErrorCannotForgeALine(t *testing.T) {
	withStdinTTY(t, false)
	clock := newFakeClock()
	calls := 0

	// Succeeded, one step, zero blobs — so PartitionOutputs keeps nothing and the
	// zero-deliverables error is the branch taken.
	const payload = `{"id":"wf_123","status":"succeeded","steps":[{
	  "$type":"textToImage","name":"s","status":"succeeded","metadata":{},
	  "output":{"images":[]}}]}`

	var s genSeams
	s.poll = clock.cfg()
	s.getWorkflow = scriptedWorkflows(&calls, payload)
	s.submitReply = &genapi.SubmitResult{ID: gfsHostileID, Status: "queued"}

	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), waitOpts(t.TempDir()))
	if err == nil {
		t.Fatal("CONTROL failure, not a finding: a succeeded workflow with no outputs reported success")
	}
	got := err.Error()
	if !strings.Contains(got, "produced no deliverable outputs") {
		t.Fatalf("CONTROL failure, not a finding: reached a different error, so this test drives the "+
			"wrong surface:\n%s", got)
	}
	if !strings.Contains(got, "wf1") {
		t.Fatalf("CONTROL failure, not a finding: the workflow id never reached the message:\n%s", got)
	}
	assertOneLine(t, "waitAndCollect (succeeded-but-no-deliverables error)", got)
}

// TestPrintSubmittedCannotForgeALine drives the two lines printed the moment a
// submit is accepted — the receipt for money that has already moved.
//
// 🔴 NO TAB ASSERTION, AND ITS ABSENCE IS MEASURED. Both lines go through ui
// (st.Success and st.Dim), and lipgloss expands \t to four spaces before the
// bytes reach the buffer — re-measured on this tree, see this file's header. A
// tab guard here could not fail whether or not the gate exists. The line count
// is what discriminates: lipgloss passes \n through untouched.
func TestPrintSubmittedCannotForgeALine(t *testing.T) {
	render := func(id string) string {
		var b bytes.Buffer
		printSubmitted(&b, id, "ext-1", "https://civitai.com", nil)
		return b.String()
	}

	benign := gfsLines(render(gfsBenignID))
	// POSITIVE CONTROL on the harness: the benign block must be the real
	// two-line one, or "the counts match" compares two trivial values.
	if len(benign) != 2 {
		t.Fatalf("CONTROL failure, not a finding: the benign submit block rendered %d line(s), want the "+
			"2 this function writes:\n%s", len(benign), strings.Join(benign, "\n"))
	}

	got := gfsLines(render(gfsHostileID))
	if !strings.Contains(strings.Join(got, "\n"), "wf1") {
		t.Fatalf("CONTROL failure, not a finding: the workflow id never reached the block:\n%s",
			strings.Join(got, "\n"))
	}
	if len(got) != len(benign) {
		t.Errorf("#604 FORGERY in printSubmitted: a hostile workflow id rendered %d line(s) where a benign "+
			"one renders %d — the server chose the geometry of the receipt for a spend:\n%s",
			len(got), len(benign), strings.Join(got, "\n"))
	}
}

// TestPrintReattachBlockGeometryIsNotServerChosen drives the recovery block —
// what a user reads after a wait ended without a result, i.e. about money
// already spent and outputs they can still fetch.
//
// It asserts the STATE, not the absence of a word: the block occupies exactly
// the number of lines it writes itself, and each of its four labelled rows
// appears exactly once. #604 measured TWO counterfeit `Re-attach:` lines, the
// first of them ABOVE the genuine one, so a reader scanning top-down reached the
// attacker's command first — a guard spelled against the payload would be
// walkable by respelling it.
//
// The tab assertion here IS live: the three gated rows are bare fmt.Fprintf, not
// ui-styled (the ⚠ header and the trailer are styled, and neither carries server
// text).
func TestPrintReattachBlockGeometryIsNotServerChosen(t *testing.T) {
	o := baseOpts()
	render := func(id, status string) []string {
		var b bytes.Buffer
		printReattach(&b, o, id, "ext-1", status, "Stopped waiting after 5m0s.")
		return gfsLines(b.String())
	}

	benign := render(gfsBenignID, gfsBenignStatus)
	// POSITIVE CONTROL: the block this function writes is a ⚠ header, four
	// labelled rows and a trailer. A differential alone would be satisfied by two
	// equally-broken renders.
	if len(benign) != 6 {
		t.Fatalf("CONTROL failure, not a finding: the benign re-attach block rendered %d line(s), want the "+
			"6 this function writes (header + 4 rows + trailer):\n%s", len(benign), strings.Join(benign, "\n"))
	}

	got := render(gfsHostileID, gfsHostileStatus)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "wf1") || !strings.Contains(joined, "processing") {
		t.Fatalf("CONTROL failure, not a finding: the id and status did not both reach the block:\n%s", joined)
	}
	if len(got) != len(benign) {
		t.Errorf("#604 FORGERY in printReattach: a hostile workflow id and status rendered %d line(s) where "+
			"benign ones render %d — the server chose how many lines the recovery block occupies:\n%s",
			len(got), len(benign), joined)
	}
	if strings.Contains(joined, "\t") {
		t.Errorf("#604 FORGERY in printReattach: a TAB survived into the block, where it shifts a row's text "+
			"to a column the CLI did not choose:\n%s", joined)
	}
	// The row set, counted. Exactly one line may begin with each label, and
	// `Re-attach:` is the one that hands the user a command to run.
	for _, prefix := range []string{"  Workflow ID: ", "  External ID: ", "  Last status: ", "  Re-attach:   "} {
		n := 0
		for _, l := range got {
			if strings.HasPrefix(l, prefix) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("#604 FORGERY in printReattach: %d line(s) begin %q, want exactly 1 — a counterfeit row "+
				"at column zero is indistinguishable from the one the CLI wrote:\n%s", n, prefix, joined)
		}
	}
}

// TestPrintOutputURLsRowCountIsNotServerChosen drives the --no-download listing,
// which is the ONE surface in this issue that writes to STDOUT. It is what a
// user pipes into `xargs curl`, so a forged row is a URL a script fetches.
//
// 🔴 THE ASSERTION IS THE ROW COUNT AND THE FIELD COUNT, NOT THE ABSENCE OF THE
// PAYLOAD. `fmt.Fprintf(out, "%d\t%s\n", …)` means a URL carrying
// "\n2\t<other>" writes a second, fully attacker-written numbered row; a guard
// checking for the attacker's host is walkable by choosing a different one,
// while the geometry is not.
//
// 🔴 AND THE GEOMETRY IS NOT `len(rows) == len(kept)`, WHICH IS WHAT THIS TEST
// ASSERTED FIRST AND WOULD HAVE FIRED A FALSE FORGERY ON (civitai/cli#604 round
// 0). printOutputURLs numbers by INDEX INTO kept but only prints an output that
// HAS a URL, and `genapi.Deliverable` — `Available && !hasBlockedReason &&
// !Hidden` — does not require one. So a server returning `url: null` on the
// middle of three deliverable outputs legitimately yields two rows numbered 1
// and 3, and the old assertion would have reported the CLI forging its own
// output. The real invariant, and the one below: ONE ROW PER KEPT OUTPUT THAT
// HAS A URL, NUMBERED BY ITS INDEX IN kept — so the numbers may legitimately
// have GAPS, but they are never server-chosen, never duplicated and never out of
// order. The attack is still caught, because it is an EXTRA row carrying a
// DISPLACED number, and the fixture drives the gap case so the widened
// assertion cannot be satisfied vacuously.
//
// The tab assertion is live here — this is bare fmt.Fprintf with no ui styling
// in front of it — and it is the one that catches the extra-row forgery.
func TestPrintOutputURLsRowCountIsNotServerChosen(t *testing.T) {
	urlOf := func(s string) *string { return &s }
	// The hostile URL forges the row number the nil-URL output left UNUSED, which
	// is the most convincing number it could choose: the listing has a visible gap
	// at 2 and the counterfeit fills it.
	kept := []genapi.Output{
		{Blob: genapi.Blob{ID: "a", URL: urlOf("https://example.invalid/o/a.jpeg")}},
		{Blob: genapi.Blob{ID: "b", URL: nil}},
		{Blob: genapi.Blob{ID: "c", URL: urlOf("https://example.invalid/o/c.jpeg\n2\thttps://evil.invalid/steal")}},
		{Blob: genapi.Blob{ID: "d", URL: urlOf("https://example.invalid/o/d.jpeg")}},
	}

	// wantNumbers is the invariant, DERIVED FROM THE FIXTURE rather than written
	// down: the 1-based index in kept of every output that has a URL.
	var wantNumbers []string
	for i, o := range kept {
		if o.URL != nil {
			wantNumbers = append(wantNumbers, strconv.Itoa(i+1))
		}
	}
	// POSITIVE CONTROL on the fixture: the numbering must actually have a GAP, or
	// this test is back to asserting one row per kept output and the nil-URL case
	// — the one the first version of this assertion got wrong — goes undriven.
	if len(wantNumbers) == len(kept) {
		t.Fatalf("CONTROL failure, not a finding: every fixture output has a URL, so the numbering has no "+
			"gap and the nil-URL case this assertion exists to get right is not driven (want %v)", wantNumbers)
	}

	var out, errw bytes.Buffer
	printOutputURLs(&out, &errw, kept)
	rows := gfsLines(out.String())

	// POSITIVE CONTROL: the hostile URL's own tail must have reached stdout, or
	// the geometry below is being measured on a listing that never carried it.
	if !strings.Contains(out.String(), "evil.invalid") {
		t.Fatalf("CONTROL failure, not a finding: the hostile URL never reached stdout:\n%s", out.String())
	}
	if len(rows) != len(wantNumbers) {
		t.Errorf("#604 FORGERY in printOutputURLs: %d row(s) on STDOUT for %d kept output(s) WITH A URL — a "+
			"server-chosen URL wrote %d extra numbered row(s) onto the surface this CLI documents for "+
			"piping:\n%s", len(rows), len(wantNumbers), len(rows)-len(wantNumbers), out.String())
	}
	for i, row := range rows {
		fields := strings.Split(row, "\t")
		if len(fields) != 2 {
			t.Errorf("#604 FORGERY in printOutputURLs: row %d has %d tab-separated field(s), want 2 — a "+
				"TAB inside a URL inserts a column a consumer splitting on \\t will read as data the CLI "+
				"wrote:\n%q", i+1, len(fields), row)
			continue
		}
		if i >= len(wantNumbers) {
			// The count assertion above already reported this row; naming it again
			// as a numbering failure would blame the wrong half.
			continue
		}
		if fields[0] != wantNumbers[i] {
			t.Errorf("#604 FORGERY in printOutputURLs: row %d is numbered %q, want %q — the numbering is the "+
				"CLI's own index into the kept outputs (gaps where an output arrived with no URL are "+
				"legitimate) and a server value has displaced it:\n%q", i+1, fields[0], wantNumbers[i], row)
		}
	}
}

// TestPollReportersCannotForgeALine drives BOTH reporters through the real
// pollWorkflow with a NON-TERMINAL status.
//
// 🔴 NON-TERMINAL IS THE REQUIREMENT, NOT AN INCIDENTAL CHOICE. tick() renders
// the status of a job still running — the normal path of every `civitai
// generate` that waits — so a test driving only a terminal status would leave
// that surface undriven while reading as coverage of the reporters.
//
// One run exercises three of the four quiet-path sites: attempt 1 takes tick's
// no-error branch, attempt 2+ take its retryable-error branch (the scripted
// 500), and the wait then times out, which calls finish() with the LAST status —
// the hostile one — rather than a terminal value.
//
// 🔴 THE SCRIPTED 500 CARRIES A HOSTILE SERVER MESSAGE, AND WITHOUT THAT THIS
// TEST WAS BLIND TO HALF ITS OWN SURFACE (civitai/cli#604 round 0). tick's error
// branch renders TWO server operands on one Fprintf — the status and the
// error — and the error was ungated while the status was not. The fixture's
// message was the literal "scripted", which holds no newline and no tab, so the
// assertions below passed identically with the gate present and absent. The
// row-prefix check made it worse than merely blind: it PERMITTED `  status ` as
// a legitimate prefix, which is exactly the line finish() writes and exactly the
// line an ungated message forges. So the assertion now pins the SEQUENCE — the
// `  status ` line must be the LAST line and there must be exactly one of it —
// rather than a set of allowed words, and the fixture drives a message whose
// tail is built to impersonate it.
//
// Both reporters write with bare fmt.Fprintf, so the tab assertions here are
// live.
func TestPollReportersCannotForgeALine(t *testing.T) {
	drive := func(t *testing.T, rep pollReporter, status, serverMessage string) {
		t.Helper()
		clock := newFakeClock()
		cfg := clock.cfg()
		cfg.timeout = 30 * time.Second
		calls := 0
		get := scriptedWorkflows(&calls, gfsWorkflowJSON(status),
			apiErrorWithMessage(t, http.StatusInternalServerError, serverMessage))
		_, _, err := pollWorkflow(context.Background(), get, "wf_1", cfg, rep)
		if err == nil {
			t.Fatalf("CONTROL failure, not a finding: a poll that never reaches a terminal status returned "+
				"no error after %d call(s)", calls)
		}
		if calls < 2 {
			t.Fatalf("CONTROL failure, not a finding: the poll made %d call(s), so tick's error branch was "+
				"never reached", calls)
		}
	}

	t.Run("quiet", func(t *testing.T) {
		render := func(status, serverMessage string) string {
			var b bytes.Buffer
			// heartbeat 0 prints every tick: the throttle is not what is under
			// test, and a suppressed tick renders nothing to assert on.
			drive(t, &quietPollReporter{w: &b, now: newFakeClock().Now, heartbeat: 0}, status, serverMessage)
			return b.String()
		}

		// assertRowSequence pins the SHAPE of a quiet poll's output: some number
		// of `  waiting… status ` lines, then exactly one `  status ` line, last.
		// Stated as an order rather than as a permitted vocabulary, because
		// `  status ` is a prefix an ungated operand can spell for itself and the
		// only thing that separates the real one from a counterfeit is WHERE it
		// sits and HOW MANY there are.
		assertRowSequence := func(t *testing.T, fail func(string, ...any), lines []string) {
			t.Helper()
			joined := strings.Join(lines, "\n")
			for i, l := range lines {
				last := i == len(lines)-1
				switch {
				case strings.HasPrefix(l, "  waiting… status "):
					if last {
						fail("#604 FORGERY in the quiet poll reporter: the LAST line is a `waiting…` line, "+
							"so finish()'s own `  status ` line is missing or has been displaced:\n%s", joined)
					}
				case strings.HasPrefix(l, "  status "):
					if !last {
						fail("#604 FORGERY in the quiet poll reporter: a `  status ` line appears at "+
							"position %d of %d instead of last. finish() writes that line ONCE, at the end; "+
							"one in the middle is a counterfeit wearing the prefix of the line that reports "+
							"how a paid-for generation ended:\n%s", i+1, len(lines), joined)
					}
				default:
					fail("#604 FORGERY in the quiet poll reporter: a line the reporter did not write "+
						"appeared in its output:\n%q\nfull output:\n%s", l, joined)
				}
			}
		}

		benign := gfsLines(render(gfsBenignStatus, gfsBenignServerMessage))
		if len(benign) < 2 {
			t.Fatalf("CONTROL failure, not a finding: the benign quiet poll printed %d line(s); this test "+
				"compares geometries and needs a real one:\n%s", len(benign), strings.Join(benign, "\n"))
		}
		// POSITIVE CONTROL on the sequence assertion: it must hold on a benign
		// render, or a red below says nothing about the hostile one. Fatal, not
		// Error — a broken expectation is not a finding.
		assertRowSequence(t, func(f string, a ...any) {
			t.Fatalf("CONTROL failure, not a finding: the BENIGN quiet poll does not have the shape this "+
				"test asserts, so the hostile verdict below would be about the assertion, not the gate.\n"+
				f, a...)
		}, benign)

		got := gfsLines(render(gfsHostileStatus, gfsHostileServerMessage))
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "processing") {
			t.Fatalf("CONTROL failure, not a finding: the server status never reached the quiet poll "+
				"lines:\n%s", joined)
		}
		// POSITIVE CONTROL on the OTHER operand. Without this the assertions below
		// could be measuring a render the server's error message never reached —
		// which is the state this test shipped in.
		if !strings.Contains(joined, "boom") {
			t.Fatalf("CONTROL failure, not a finding: the server's own error message never reached the "+
				"quiet poll lines, so nothing here measures whether it is gated:\n%s", joined)
		}
		if len(got) != len(benign) {
			t.Errorf("#604 FORGERY in the quiet poll reporter: a hostile status and server error message "+
				"printed %d line(s) where benign ones print %d — the server chose how many lines a waiting "+
				"generate emits:\n%s", len(got), len(benign), joined)
		}
		assertRowSequence(t, t.Errorf, got)
		if strings.Contains(joined, "\t") {
			t.Errorf("#604 FORGERY in the quiet poll reporter: a TAB survived into a status line:\n%s", joined)
		}
	})

	t.Run("tty", func(t *testing.T) {
		// The hostile SERVER MESSAGE is routed down this path too, and the
		// one-newline check below is what turns "ttyPollReporter.tick drops e.err"
		// from a claim read off the source into a measured one: the message can
		// only stay invisible while the suffix stays a fixed string. Someone
		// rendering it ungated later is red HERE, not at the next audit.
		render := func(status, serverMessage string) string {
			var b bytes.Buffer
			drive(t, &ttyPollReporter{w: &b}, status, serverMessage)
			return b.String()
		}

		benign := render(gfsBenignStatus, gfsBenignServerMessage)
		// POSITIVE CONTROL: the spinner rewrites ONE line with \r and ends it with
		// the single newline finish() writes. If that is not what the benign render
		// does, the count below is not the property it claims.
		if n := strings.Count(benign, "\n"); n != 1 {
			t.Fatalf("CONTROL failure, not a finding: the benign spinner emitted %d newline(s), want the 1 "+
				"finish() writes:\n%q", n, benign)
		}

		got := render(gfsHostileStatus, gfsHostileServerMessage)
		if !strings.Contains(got, "processing") {
			t.Fatalf("CONTROL failure, not a finding: the server status never reached the spinner:\n%q", got)
		}
		if n := strings.Count(got, "\n"); n != 1 {
			t.Errorf("#604 FORGERY in the tty poll reporter: a hostile status and server error message "+
				"emitted %d newline(s) where benign ones emit 1 — the spinner rewrites ONE line with \\r, so "+
				"a forged newline strands attacker text above the rewrite point where nothing ever "+
				"overwrites it:\n%q", n, got)
		}
		if strings.Contains(got, "\t") {
			t.Errorf("#604 FORGERY in the tty poll reporter: a TAB survived into the spinner line:\n%q", got)
		}
	})
}

// TestClassifyGenerateErrorCannotForgeALine drives every arm that rebuilds the
// error around the server's own message.
//
// All five arms, not one: classifyGenerateError sanitises ONCE at the top and
// each arm interpolates the result, which is the shape civitai/cli#566 exists
// about — if someone later pushes the gate down into the arms, a test driving
// one arm would not notice the other four going raw.
//
// These messages are plain fmt.Errorf with no ui styling, so assertOneLine's tab
// half is live here.
func TestClassifyGenerateErrorCannotForgeALine(t *testing.T) {
	// The forged tail is a plausible complete instruction, because these
	// messages are exactly where a user is told what to do next.
	const tail = "\nRun `civitai login --token hunter2` to continue\tOK"

	for _, tc := range []struct {
		name, needle string
		status       int
		code         string
	}{
		{"insufficient buzz", "You don't have enough funds", http.StatusBadRequest, "BAD_REQUEST"},
		{"prompt flagged", "Your prompt was flagged", http.StatusBadRequest, "BAD_REQUEST"},
		{"generation disabled", "Generation is currently disabled", http.StatusBadRequest, "BAD_REQUEST"},
		{"account restricted", "Your account has been restricted", http.StatusForbidden, "FORBIDDEN"},
		{"unknown ecosystem", "Unknown ecosystem: floop", http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := trpcErrServer(t, tc.status, tc.needle+tail, tc.code)
			_, _, err := genapi.New(srv.URL, "test-token").WhatIfFromGraph(context.Background(), genapi.Graph{})
			if err == nil {
				t.Fatal("CONTROL failure, not a finding: the scripted server produced no error")
			}
			got := classifyGenerateError(err)
			if got == nil {
				t.Fatal("CONTROL failure, not a finding: the classifier returned nil")
			}
			msg := got.Error()
			// POSITIVE CONTROL: the arm must have MATCHED, or this is the
			// fall-through returning the transport's own error and the assertion
			// below says nothing about the gate under test.
			if got == err {
				t.Fatalf("CONTROL failure, not a finding: the %q arm did not match, so the classifier "+
					"returned its input unchanged:\n%s", tc.name, msg)
			}
			if !strings.Contains(msg, "civitai login --token") {
				t.Fatalf("CONTROL failure, not a finding: the server message never reached the rebuilt "+
					"error:\n%s", msg)
			}
			assertOneLine(t, "classifyGenerateError ("+tc.name+")", msg)
		})
	}
}
