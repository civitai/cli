package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// Surfacing silent checkpoint substitutions (civitai/civitai#3665).
//
// The defect being fixed is SILENCE: the server swapped the model, billed for
// the substitute, and the reply was indistinguishable from success. So the
// assertions here are mostly about what reaches the user's screen, and every
// asserted absence has a positive control proving the same assertion could have
// seen the thing it is claiming is missing.

// subQuote is an estimate carrying one substitution with the given reason.
func subQuote(reason string) *genapi.WhatIfResult {
	q := okQuote(12)
	q.ModelSubstitutions = []genapi.ModelSubstitution{
		{Requested: 999999999, Applied: 2436219, Reason: reason},
	}
	return q
}

// withQuote points the whatIf seam at a fixed reply.
func withQuote(s *genSeams, q *genapi.WhatIfResult, raw json.RawMessage) {
	s.whatIf = func(ctx context.Context, g genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		return q, raw, nil
	}
}

// --- the estimate path: warn BEFORE the money -------------------------------

// 🔴 THE HEADLINE PROPERTY. A substitution reported on the estimate must be
// visible, with BOTH ids, before anything is submitted.
func TestGenerate_EstimateWarnsAboutSubstitutionBeforeSpending(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	o := baseOpts()
	o.dryRun = true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("dry run must succeed: %v", err)
	}

	got := errb.String()
	// The requested id and the applied id must BOTH appear: "we used something
	// else" is useless without saying what.
	if !strings.Contains(got, "999999999") {
		t.Errorf("the REQUESTED version id must be shown; got:\n%s", got)
	}
	if !strings.Contains(got, "2436219") {
		t.Errorf("the APPLIED version id must be shown; got:\n%s", got)
	}
	// The server's own token, raw (AGENTS.md item 8).
	if !strings.Contains(got, "unrecognized") {
		t.Errorf("the server's raw reason token must be printed; got:\n%s", got)
	}
	// It must be framed as pre-spend, which is the whole point of warning here.
	if !strings.Contains(got, "charged") {
		t.Errorf("the estimate warning must say nothing has been charged yet; got:\n%s", got)
	}
	if s.submitCalls != 0 {
		t.Errorf("a dry run must never submit; got %d", s.submitCalls)
	}
}

// 🔴 POSITIVE CONTROL for every "no warning" assertion below: the same buffer,
// the same code path, WITH a substitution, must contain the marker. Without this
// a renderer wired to nothing would pass the absence tests.
func TestGenerate_SubstitutionWarningMarkerIsReachable(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	o := baseOpts()
	o.dryRun = true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(errb.String(), "requested version") {
		t.Fatalf("POSITIVE CONTROL FAILED: the marker the absence tests grep for is unreachable; got:\n%s", errb.String())
	}
}

// 🔴 THE BACKWARD-COMPATIBILITY PROPERTY. Against a server that omits the field
// the CLI must behave EXACTLY as before — and in particular must not print a
// reassuring "no substitutions", which would be a false positive claim on every
// server older than the field.
func TestGenerate_NoSubstitutionSaysNothingEitherWay(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams // default okQuote(12) carries no substitutions, like an old server
	o := baseOpts()
	o.dryRun = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("dry run must succeed against an older server: %v", err)
	}

	both := out.String() + errb.String()
	if strings.Contains(both, "requested version") {
		t.Errorf("no substitution must print no substitution block; got:\n%s", both)
	}
	// 🔴 And it must not claim the negative either.
	for _, phrase := range []string{"no substitution", "No substitution", "no model substitution"} {
		if strings.Contains(both, phrase) {
			t.Errorf("absence is ambiguous (no swap OR an old server) and must never be reported as assurance; found %q in:\n%s", phrase, both)
		}
	}
	// The ordinary estimate is unaffected.
	if !strings.Contains(out.String(), "Estimated cost") {
		t.Errorf("the normal estimate output regressed; got:\n%s", out.String())
	}
}

// --- per-reason advice -------------------------------------------------------

// Each of the three reasons must produce DISTINCT, actionable advice. A single
// generic sentence for all three would pass a "warning appeared" test while
// telling a gated user to go fix their command.
func TestGenerate_EachReasonGivesItsOwnAdvice(t *testing.T) {
	cases := []struct {
		reason string
		// want is a phrase that must appear for THIS reason...
		want string
		// ...and notWant is a phrase that must NOT, proving the branches differ.
		notWant string
	}{
		{genapi.SubstitutionWrongWorkflow, "DIFFERENT workflow", "entitlement"},
		{genapi.SubstitutionUnrecognized, "retired", "entitlement"},
		{genapi.SubstitutionGated, "entitlement", "retired"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			var buf bytes.Buffer
			reportModelSubstitutions(&buf, []genapi.ModelSubstitution{
				{Requested: 1, Applied: 2, Reason: tc.reason},
			}, substitutionAtEstimate)

			got := buf.String()
			if !strings.Contains(got, tc.reason) {
				t.Errorf("the raw server token %q must be printed; got:\n%s", tc.reason, got)
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("advice for %q must mention %q; got:\n%s", tc.reason, tc.want, got)
			}
			if strings.Contains(got, tc.notWant) {
				t.Errorf("advice for %q leaked another reason's advice (%q); got:\n%s", tc.reason, tc.notWant, got)
			}
		})
	}
}

// 🔴 A reason this CLI does not know must still WARN, with the ids intact. A
// newer server adding a fourth reason must not make the CLI go quiet — that is
// the original defect, reintroduced through a switch statement.
func TestGenerate_UnknownReasonStillWarnsWithTheIds(t *testing.T) {
	var buf bytes.Buffer
	reportModelSubstitutions(&buf, []genapi.ModelSubstitution{
		{Requested: 42, Applied: 43, Reason: "some-future-reason"},
	}, substitutionAtEstimate)

	got := buf.String()
	if !strings.Contains(got, "42") || !strings.Contains(got, "43") {
		t.Errorf("an unknown reason must not suppress the ids; got:\n%s", got)
	}
	if !strings.Contains(got, "some-future-reason") {
		t.Errorf("the unrecognised token must still be echoed verbatim; got:\n%s", got)
	}
	if !strings.Contains(got, "does not recognise") {
		t.Errorf("an unknown reason should say the explanation may be incomplete; got:\n%s", got)
	}
}

// An empty record prints NOTHING — not a heading, not an empty block.
func TestReportModelSubstitutions_EmptyPrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	reportModelSubstitutions(&buf, nil, substitutionAtEstimate)
	reportModelSubstitutions(&buf, []genapi.ModelSubstitution{}, substitutionAfterSubmit)
	if buf.Len() != 0 {
		t.Errorf("empty record must print nothing at all; got:\n%s", buf.String())
	}
}

// The three phases must differ in whether they say the money already moved.
func TestReportModelSubstitutions_PhaseChangesTheSpendFraming(t *testing.T) {
	sub := []genapi.ModelSubstitution{{Requested: 1, Applied: 2, Reason: genapi.SubstitutionGated}}

	var est, sub2 bytes.Buffer
	reportModelSubstitutions(&est, sub, substitutionAtEstimate)
	reportModelSubstitutions(&sub2, sub, substitutionAfterSubmit)

	if !strings.Contains(est.String(), "Nothing has been submitted or charged yet") {
		t.Errorf("the estimate phase must say nothing was charged; got:\n%s", est.String())
	}
	if !strings.Contains(sub2.String(), "HAS BEEN CHARGED") {
		t.Errorf("the post-submit phase must say the charge happened; got:\n%s", sub2.String())
	}
	// 🔴 They must not be the same string — a phase parameter that is ignored
	// would tell a user who already paid that nothing was charged.
	if est.String() == sub2.String() {
		t.Errorf("phase is being ignored; both rendered:\n%s", est.String())
	}
}

// --- the submit path ---------------------------------------------------------

// A substitution reported only on the SUBMIT (a user who skipped the estimate
// warning, or a divergence) must still be reported — from the reply itself.
func TestGenerate_SubmitReplySubstitutionIsReported(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	s.submitReply = &genapi.SubmitResult{
		ID: "wf_123", Status: "queued",
		ModelSubstitutions: []genapi.ModelSubstitution{
			{Requested: 555, Applied: 666, Reason: genapi.SubstitutionWrongWorkflow},
		},
	}
	o := baseOpts()
	o.assumeYes = true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("expected exactly one submit, got %d", s.submitCalls)
	}

	got := errb.String()
	if !strings.Contains(got, "555") || !strings.Contains(got, "666") {
		t.Errorf("the submit reply's substitution must be reported; got:\n%s", got)
	}
	if !strings.Contains(got, "HAS BEEN CHARGED") {
		t.Errorf("post-submit the user must be told the charge already happened; got:\n%s", got)
	}
}

// The same record arriving ONLY under `metadata` — the shape a later read has —
// must be reported too.
func TestGenerate_SubmitMetadataOnlySubstitutionIsReported(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	s.submitReply = &genapi.SubmitResult{ID: "wf_123", Status: "queued"}
	// Build the metadata-only shape through the real decoder, so this test pins
	// the wire contract rather than a hand-built struct.
	var r genapi.SubmitResult
	if err := json.Unmarshal([]byte(
		`{"id":"wf_123","status":"queued","metadata":{"modelSubstitutions":[{"requested":77,"applied":88,"reason":"gated"}]}}`), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s.submitReply = &r
	o := baseOpts()
	o.assumeYes = true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !strings.Contains(errb.String(), "77") || !strings.Contains(errb.String(), "88") {
		t.Errorf("a metadata-only record must be reported; got:\n%s", errb.String())
	}
}

// --- machine-readable output -------------------------------------------------

// 🔴 The record must reach a SCRIPT, and the JSON on stdout must stay parseable.
// The warning goes to stderr precisely so this stays true.
func TestGenerate_JSONCarriesTheRecordAndStdoutStaysParseable(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	raw := json.RawMessage(`{"ready":true,"cost":{"base":8,"total":12},` +
		`"modelSubstitutions":[{"requested":999999999,"applied":2436219,"reason":"unrecognized"}]}`)
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), raw)
	o := baseOpts()
	o.dryRun = true
	o.jsonOut = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("dry-run --json: %v", err)
	}

	// stdout must be VALID JSON carrying the record — decoded, not substring-matched.
	var got struct {
		ModelSubstitutions []genapi.ModelSubstitution `json:"modelSubstitutions"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout must remain parseable JSON, got %q: %v", out.String(), err)
	}
	if len(got.ModelSubstitutions) != 1 || got.ModelSubstitutions[0].Applied != 2436219 {
		t.Errorf("the record must survive to the JSON surface; got %+v", got.ModelSubstitutions)
	}
	// ...and the human warning must still have been emitted, on stderr.
	if !strings.Contains(errb.String(), "requested version") {
		t.Errorf("--json must still warn on stderr; got:\n%s", errb.String())
	}
}

// --- --fail-on-substitution --------------------------------------------------

// 🔴 THE REFUSAL, AND ITS MEASURED ZERO. It must refuse BEFORE the submit seam
// is touched, so nothing is spent.
func TestGenerate_FailOnSubstitutionRefusesAndNeverSubmits(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	o := baseOpts()
	o.assumeYes = true
	o.failOnSubstitution = true

	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("--fail-on-substitution must refuse when the server reports a substitution")
	}
	// Pinned by errors.Is, never by message text.
	if !errors.Is(err, ErrModelSubstituted) {
		t.Errorf("refusal must be tagged ErrModelSubstituted, got %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("MEASURED ZERO FAILED: nothing may be submitted; got %d submits", s.submitCalls)
	}
	if !strings.Contains(err.Error(), "nothing was charged") {
		t.Errorf("the refusal must say no money moved; got %v", err)
	}
}

// POSITIVE CONTROL for the zero above: the SAME counter, through the SAME deps,
// reaches 1 when the flag is off. Without this, "0 submits" could mean a broken
// harness.
func TestGenerate_FailOnSubstitutionOffStillSubmits(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionUnrecognized), okQuoteRaw(12))
	o := baseOpts()
	o.assumeYes = true
	o.failOnSubstitution = false

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("the DEFAULT must warn and continue, not refuse: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: expected 1 submit with the flag off, got %d", s.submitCalls)
	}
	if !strings.Contains(errb.String(), "requested version") {
		t.Errorf("the default path must still WARN; got:\n%s", errb.String())
	}
}

// The flag must not fire when there is no substitution — otherwise it would
// break every ordinary run, including against a server that omits the field.
func TestGenerate_FailOnSubstitutionIsInertWithoutASubstitution(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams // no substitutions
	o := baseOpts()
	o.assumeYes = true
	o.failOnSubstitution = true

	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("no substitution must not trip the flag: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("expected the run to proceed; got %d submits", s.submitCalls)
	}
}

// With --dry-run the estimate is still PRINTED before the refusal: a pre-flight
// that refuses must not swallow the answer it was asked for.
func TestGenerate_FailOnSubstitutionDryRunStillPrintsTheEstimate(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	withQuote(&s, subQuote(genapi.SubstitutionGated), okQuoteRaw(12))
	o := baseOpts()
	o.dryRun = true
	o.failOnSubstitution = true

	c, out, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, ErrModelSubstituted) {
		t.Fatalf("dry run must still refuse, got %v", err)
	}
	if !strings.Contains(out.String(), "Estimated cost") {
		t.Errorf("the estimate must be printed before the refusal; got:\n%s", out.String())
	}
}

// --- the later-read path -----------------------------------------------------

// `civitai generate --no-wait` sends the user to `civitai workflows get`, so
// that command is the last surface where a substitution can be discovered.
func TestPrintWorkflow_ReportsPersistedSubstitution(t *testing.T) {
	var out, errb bytes.Buffer
	var wf genapi.Workflow
	if err := json.Unmarshal([]byte(
		`{"id":"wf_1","status":"succeeded","metadata":{"modelSubstitutions":[{"requested":11,"applied":22,"reason":"unrecognized"}]}}`), &wf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	printWorkflow(&out, &errb, &wf)
	got := errb.String()
	if !strings.Contains(got, "11") || !strings.Contains(got, "22") {
		t.Errorf("a persisted substitution must be reported on read-back; got:\n%s", got)
	}
}

// ...and a workflow without one prints no block and makes no claim.
func TestPrintWorkflow_NoSubstitutionIsSilent(t *testing.T) {
	var out, errb bytes.Buffer
	printWorkflow(&out, &errb, &genapi.Workflow{ID: "wf_1", Status: "succeeded"})
	if strings.Contains(errb.String(), "requested version") {
		t.Errorf("no substitution must print nothing; got:\n%s", errb.String())
	}
}

// --- the flag hint ------------------------------------------------------------

// The hint must not tell a caller to pass a flag they already passed. (The first
// version did exactly that; running the binary is what showed it.)
func TestSubstitutionFlagHint_NotShownWhenAlreadyArmed(t *testing.T) {
	subs := []genapi.ModelSubstitution{{Requested: 1, Applied: 2, Reason: genapi.SubstitutionGated}}

	var armed bytes.Buffer
	o := baseOpts()
	o.failOnSubstitution = true
	substitutionFlagHint(&armed, o, subs)
	if strings.Contains(armed.String(), "--fail-on-substitution") {
		t.Errorf("must not suggest a flag the caller already passed; got:\n%s", armed.String())
	}

	// POSITIVE CONTROL: the same call, flag off, MUST emit the hint — otherwise
	// the assertion above would pass against a function that never prints.
	var off bytes.Buffer
	o.failOnSubstitution = false
	substitutionFlagHint(&off, o, subs)
	if !strings.Contains(off.String(), "--fail-on-substitution") {
		t.Fatalf("POSITIVE CONTROL FAILED: the hint is never emitted; got:\n%s", off.String())
	}

	// ...and never when there is nothing to warn about.
	var none bytes.Buffer
	substitutionFlagHint(&none, o, nil)
	if none.Len() != 0 {
		t.Errorf("no substitution must print no hint; got:\n%s", none.String())
	}
}

// --- the advice must name a command that exists ------------------------------

// 🔴 DOC-ROT GUARD. The "unrecognized" advice tells the user to go inspect the
// version id with a specific command. If that command does not exist, a user who
// has just been mis-billed is sent to a dead end — and the FIRST DRAFT of this
// feature did exactly that (`civitai models versions <id>`, which is not a
// command; the real one is under a different parent). A green unit suite did not
// notice, because nothing walked the command tree — building and RUNNING the
// binary is what caught it.
func TestSubstitutionAdvice_NamesARealCommand(t *testing.T) {
	fields := strings.Fields(substitutionInspectCmd)
	if len(fields) < 2 || fields[0] != "civitai" {
		t.Fatalf("advice command %q must start with `civitai` and name a subcommand", substitutionInspectCmd)
	}

	// 🔴 `err` IS NOT THE SIGNAL, AND A GUARD KEYED ON IT WOULD NEVER FIRE.
	// MEASURED against this command tree: cobra's Find returns err == nil for
	// every unknown path and instead falls back to the nearest resolvable
	// ancestor, handing the unconsumed words back in `rest` —
	//   {"nosuchtop"}         -> cmd "civitai",        rest ["nosuchtop"],  err nil
	//   {"models","versions"} -> cmd "civitai models", rest ["versions"],   err nil
	// So `rest` is what proves the path fully resolved. An earlier draft of this
	// test checked only `err != nil`; it was dead code, and the mutation that
	// should have killed it was caught by a different assertion (which is exactly
	// how a guard passes review while guarding nothing).
	root := NewRootCmd()
	found, rest, err := root.Find(fields[1:])
	if err != nil {
		t.Fatalf("advice names %q and Find errored: %v", substitutionInspectCmd, err)
	}
	if len(rest) != 0 {
		t.Errorf("advice names %q but %q is as far as it resolves — %v is not a command",
			substitutionInspectCmd, found.CommandPath(), rest)
	}
	// Belt and braces: the command we landed on must be the leaf we named.
	if leaf := fields[len(fields)-1]; found.Name() != leaf {
		t.Errorf("advice names %q but the tree resolved to %q — the leaf %q does not exist",
			substitutionInspectCmd, found.CommandPath(), leaf)
	}

	// ...and the advice the user actually sees must contain it.
	if !strings.Contains(substitutionAdvice(genapi.SubstitutionUnrecognized), substitutionInspectCmd) {
		t.Errorf("the unrecognized advice no longer names the inspect command")
	}
}

// --- terminal safety ---------------------------------------------------------

// The reason is a server-supplied STRING and must go through safeTerm like every
// other one, or a hostile payload could rewrite the user's terminal on the line
// that reports a mis-billed generation.
func TestReportModelSubstitutions_SanitizesTheServerReason(t *testing.T) {
	var buf bytes.Buffer
	reportModelSubstitutions(&buf, []genapi.ModelSubstitution{
		{Requested: 1, Applied: 2, Reason: "eviltext\x1b[2Jmore"},
	}, substitutionAtEstimate)

	if strings.Contains(buf.String(), "\x1b[2J") {
		t.Errorf("the server-supplied reason must be sanitized; got %q", buf.String())
	}
}
