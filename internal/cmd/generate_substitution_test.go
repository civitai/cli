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
	// 🔴 THE PHASE THE CALL SITE PASSES, asserted structurally. See assertPhase:
	// the previous `Contains(got, "charged")` here was a SPELLED guard satisfied
	// by the quote renderer's own "nothing was submitted and nothing was charged"
	// line, so mutating this call site to substitutionAfterSubmit left the suite
	// green while --dry-run announced "HAS BEEN CHARGED".
	assertPhase(t, got, substitutionAtEstimate)

	if s.submitCalls != 0 {
		t.Errorf("a dry run must never submit; got %d", s.submitCalls)
	}
}

// assertPhase pins WHICH phase a call site passed, by deriving the expected lead
// from the phase constant itself and requiring the other phases' leads to be
// ABSENT.
//
// 🔴 THIS IS THE STRUCTURAL FORM OF A GUARD THAT USED TO BE SPELLED. Matching a
// word ("charged") could not distinguish the phases, and could be satisfied by an
// unrelated line from a different renderer entirely. Deriving the text from the
// constant means a wrong argument at a call site moves the expectation and fails.
//
// It refuses an EMPTY expected lead rather than passing vacuously — Contains(x,
// "") is always true, which is exactly how an emptied lead survived before.
func assertPhase(t *testing.T, got string, want substitutionPhase) {
	t.Helper()

	wantLead := substitutionLead(want)
	if wantLead == "" {
		t.Fatalf("phase %d has an EMPTY lead — every Contains() check against it would pass vacuously", want)
	}
	if !strings.Contains(got, wantLead) {
		t.Errorf("output must carry the lead for phase %d:\n  want: %s\n  got:\n%s", want, wantLead, got)
	}
	for _, other := range []substitutionPhase{substitutionAtEstimate, substitutionAfterSubmit, substitutionOnRead} {
		if other == want {
			continue
		}
		if lead := substitutionLead(other); lead != "" && strings.Contains(got, lead) {
			t.Errorf("output carries the lead for the WRONG phase %d (wanted %d):\n%s", other, want, got)
		}
	}
}

// 🔴 assertPhase is only meaningful if the three leads are non-empty and
// PAIRWISE DISTINCT. If two collapse to the same text, assertPhase cannot tell
// those phases apart and every call-site assertion using it goes quiet — the
// harness-validation half of the guard.
//
// The audit killed three mutants here that nothing had covered: an EMPTY
// substitutionOnRead lead, one identical to the post-submit lead, and — worst —
// one carrying the PRE-SPEND framing, which made `civitai workflows get` tell a
// user whose workflow was already billed that "Nothing has been submitted or
// charged yet".
func TestSubstitutionLead_AllPhasesNonEmptyAndPairwiseDistinct(t *testing.T) {
	phases := map[string]substitutionPhase{
		"atEstimate":  substitutionAtEstimate,
		"afterSubmit": substitutionAfterSubmit,
		"onRead":      substitutionOnRead,
	}
	seen := map[string]string{}
	for name, p := range phases {
		lead := substitutionLead(p)
		if strings.TrimSpace(lead) == "" {
			t.Fatalf("phase %s has an empty lead — assertions against it pass vacuously", name)
		}
		if prev, dup := seen[lead]; dup {
			t.Errorf("phases %s and %s share an identical lead, so they are indistinguishable:\n%s", prev, name, lead)
		}
		seen[lead] = name
	}

	// ...and each must make the RIGHT claim about whether money already moved.
	if !strings.Contains(substitutionLead(substitutionAtEstimate), "Nothing has been submitted or charged yet") {
		t.Errorf("the estimate lead must state that nothing has been charged; got:\n%s", substitutionLead(substitutionAtEstimate))
	}
	for _, p := range []substitutionPhase{substitutionAfterSubmit, substitutionOnRead} {
		lead := substitutionLead(p)
		if !strings.Contains(lead, "CHARGED") {
			t.Errorf("phase %d describes an already-billed generation and must say so; got:\n%s", p, lead)
		}
		// 🔴 The specific inversion the audit found: post-spend phases must NOT
		// carry the pre-spend reassurance.
		if strings.Contains(lead, "Nothing has been submitted or charged yet") {
			t.Errorf("phase %d is post-spend but claims nothing was charged; got:\n%s", p, lead)
		}
	}
}

// The id line's VERB must follow the same split: on the estimate nothing has run
// yet, so "ran version N" under "nothing has been charged yet" contradicts itself
// on the one path where the user is still deciding whether to spend.
func TestSubstitutionVerb_IsTensedByPhase(t *testing.T) {
	var est bytes.Buffer
	reportModelSubstitutions(&est, []genapi.ModelSubstitution{
		{Requested: 1, Applied: 2, Reason: genapi.SubstitutionGated},
	}, substitutionAtEstimate)
	if !strings.Contains(est.String(), "will run version 2") {
		t.Errorf("the estimate must use the future tense; got:\n%s", est.String())
	}
	if strings.Contains(est.String(), "ran version") {
		t.Errorf("the estimate must not claim a model already ran; got:\n%s", est.String())
	}

	var done bytes.Buffer
	reportModelSubstitutions(&done, []genapi.ModelSubstitution{
		{Requested: 1, Applied: 2, Reason: genapi.SubstitutionGated},
	}, substitutionAfterSubmit)
	if !strings.Contains(done.String(), "ran version 2") {
		t.Errorf("the post-submit line must use the past tense; got:\n%s", done.String())
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
	// 🔴 The phase this call site passes, pinned structurally. The estimate in
	// this run reports nothing, so the ONLY lead present must be the post-submit
	// one — which is what fails if the argument is swapped.
	assertPhase(t, got, substitutionAfterSubmit)
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

// 🔴 THE REFUSAL MESSAGE'S OPERANDS, PINNED. All three of these survived an
// audit's mutation battery:
//
//	(a) swapping Applied/Requested — the money-refusal line then stated the
//	    substitution BACKWARDS, naming the model the caller wanted as the one
//	    that would run;
//	(b) subs[0] -> subs[len(subs)-1] — it named a different swap entirely;
//	(c) an off-by-one in "(and %d more)".
//
// The fixtures are pairwise distinct on every field so no two operands can be
// confused for one another.
func TestSubstitutionRefusal_MessageOperandsAreCorrect(t *testing.T) {
	o := baseOpts()
	o.failOnSubstitution = true
	subs := []genapi.ModelSubstitution{
		{Requested: 111, Applied: 222, Reason: genapi.SubstitutionGated},
		{Requested: 333, Applied: 444, Reason: genapi.SubstitutionUnrecognized},
		{Requested: 555, Applied: 666, Reason: genapi.SubstitutionWrongWorkflow},
	}

	err := substitutionRefusal(o, subs)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	msg := err.Error()

	// (a) APPLIED is what would run; REQUESTED is what was asked for. The whole
	// phrase is asserted, so a swap cannot satisfy it by containing both numbers.
	if !strings.Contains(msg, "would run version 222 instead of the version 111 you asked for") {
		t.Errorf("applied/requested are swapped or mangled; got:\n%s", msg)
	}
	// (b) It must name the FIRST record, not the last.
	if strings.Contains(msg, "666") || strings.Contains(msg, "555") {
		t.Errorf("the refusal named a record other than the first; got:\n%s", msg)
	}
	// (c) 3 records -> "(and 2 more)".
	if !strings.Contains(msg, "(and 2 more)") {
		t.Errorf("the overflow count is wrong (3 records means 2 more); got:\n%s", msg)
	}
	// ...and the first record's reason, not another's.
	if !strings.Contains(msg, genapi.SubstitutionGated) {
		t.Errorf("the refusal must carry the first record's reason; got:\n%s", msg)
	}
}

// The overflow clause must be ABSENT for a single record — "(and 0 more)" is the
// other half of the off-by-one.
func TestSubstitutionRefusal_SingleRecordHasNoOverflowClause(t *testing.T) {
	o := baseOpts()
	o.failOnSubstitution = true
	err := substitutionRefusal(o, []genapi.ModelSubstitution{
		{Requested: 111, Applied: 222, Reason: genapi.SubstitutionGated},
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), "more)") {
		t.Errorf("a single record must not print an overflow clause; got:\n%s", err.Error())
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
	// 🔴 runGenerate must actually CALL substitutionFlagHint. Deleting that call
	// site survived the whole suite: the hint function had its own unit test, but
	// nothing asserted anyone invoked it — the seam, not the component.
	if !strings.Contains(errb.String(), "--fail-on-substitution") {
		t.Errorf("runGenerate must emit the flag hint on the default path; got:\n%s", errb.String())
	}
}

// 🔴 THE FLAG'S HELP MUST DISCLOSE THAT ITS GUARANTEE IS SERVER-CONDITIONAL.
// Against a server that does not report substitutions the flag is silently inert
// — exit 0, submitted, charged (verified). Someone adopting it as a spend guard
// against an older deployment gets no protection and no signal, so the usage
// string has to say so rather than promising an unconditional refusal.
func TestFailOnSubstitutionFlag_HelpStatesItIsNotAGuarantee(t *testing.T) {
	f := newGenerateCmd().Flags().Lookup("fail-on-substitution")
	if f == nil {
		t.Fatal("--fail-on-substitution is not registered")
	}
	usage := f.Usage
	for _, want := range []string{"NOT A GUARANTEE", "inert"} {
		if !strings.Contains(usage, want) {
			t.Errorf("the usage string must disclose the server-version condition (missing %q); got:\n%s", want, usage)
		}
	}
	// It must still say what the flag DOES.
	if !strings.Contains(usage, "ESTIMATE") {
		t.Errorf("the usage string must still say the check runs against the estimate; got:\n%s", usage)
	}
}

// 🔴 THE INERTNESS ITSELF, DRIVEN. Against a reply with no record the flag must
// NOT refuse — the run proceeds and is charged. This is the documented
// limitation, pinned so it cannot be mistaken for a bug later, and so the help
// text above is describing real behaviour rather than a guess.
func TestGenerate_FailOnSubstitutionIsInertAgainstAnOldServer(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams // default quote: no modelSubstitutions key at all
	o := baseOpts()
	o.assumeYes = true
	o.failOnSubstitution = true

	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("an older server must not trip the flag: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("the run must proceed and be charged; got %d submits", s.submitCalls)
	}
	if strings.Contains(errb.String(), "requested version") {
		t.Errorf("nothing may be reported when the server sent no record; got:\n%s", errb.String())
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
	// 🔴 `workflows get` reads an ALREADY-BILLED workflow, so it must never carry
	// the pre-spend framing. Mutating this call site to substitutionAtEstimate
	// survived the whole suite and made this command tell a user whose generation
	// was already charged that "Nothing has been submitted or charged yet".
	assertPhase(t, got, substitutionOnRead)
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
