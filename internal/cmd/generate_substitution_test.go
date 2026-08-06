package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
)

// ---------------------------------------------------------------------------
// Silent model substitutions must be surfaced on EVERY surface that precedes or
// reports a spend (civitai#3665, PRs #3692 / #3673).
//
// 🔴 This mirrors a bug this codebase already shipped once: the img2img caveat
// printed only on the interactive path, so `--yes` (mandatory in CI) and
// `--dry-run` (the documented price-check-first workflow) never showed it. The
// surfaces enumerated below are exactly the ones that regression missed, plus
// the two post-spend reads.
// ---------------------------------------------------------------------------

// substWarn is the lead sentence. It is deliberately matched on the verb, not
// on a whole line, so a wording tweak does not fail the suite while a DELETED
// warning still does.
const substWarn = "SUBSTITUTED a different checkpoint"

const substArrayJSON = `[{"requested":128713,"applied":999001,"reason":"unrecognized"}]`

// quoteWithSubs is a whatIf reply carrying one substitution, on the top-level
// carrier — the only one that path has.
func quoteWithSubs(total float64) (*genapi.WhatIfResult, json.RawMessage) {
	q := okQuote(total)
	q.RawModelSubstitutions = json.RawMessage(substArrayJSON)
	raw := json.RawMessage(`{"ready":true,"cost":{"base":8,"total":12},"modelSubstitutions":` + substArrayJSON + `}`)
	return q, raw
}

// subsSeams builds seams whose ESTIMATE reports a substitution.
func subsSeams() *genSeams {
	return &genSeams{whatIf: func(ctx context.Context, g genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q, raw := quoteWithSubs(12)
		return q, raw, nil
	}}
}

// assertSubstitutionWarning pins the three facts the user needs: that a swap
// happened, which id was asked for, and which id ran.
//
// 🔴 IT PINS THE ID TO ITS LABEL, NOT JUST TO THE OUTPUT. Asserting that both
// integers appear somewhere is satisfied by a renderer that prints them the
// wrong way round — and "you asked for X, you got Y" is the entire content of
// the warning, so a transposition inverts its meaning while every substring
// check stays green. The fixture ids are deliberately distinct and neither is a
// substring of the other.
func assertSubstitutionWarning(t *testing.T, surface, stderr string) {
	t.Helper()
	if !strings.Contains(stderr, substWarn) {
		t.Errorf("%s: the substitution warning is missing.\nstderr:\n%s", surface, stderr)
	}
	if !strings.Contains(stderr, "unrecognized") {
		t.Errorf("%s: the server's reason token must be shown.\nstderr:\n%s", surface, stderr)
	}
	assertLabelledValue(t, surface, stderr, "Requested:", "128713")
	assertLabelledValue(t, surface, stderr, "Ran:", "999001")
}

// assertLabelledValue requires want to appear on the SAME LINE as label.
func assertLabelledValue(t *testing.T, surface, stderr, label, want string) {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, label) {
			if strings.Contains(line, want) {
				return
			}
			t.Errorf("%s: %q line does not carry %q — the ids may be transposed.\nline: %s\nstderr:\n%s",
				surface, label, want, line, stderr)
			return
		}
	}
	t.Errorf("%s: no %q line at all.\nstderr:\n%s", surface, label, stderr)
}

// --- pre-spend surfaces ------------------------------------------------------

func TestSubstitution_WarnsOnDryRun(t *testing.T) {
	withStdinTTY(t, false)
	s := subsSeams()
	o := baseOpts()
	o.dryRun = true
	_, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.whatIfCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: whatIf reached %d times, want 1", s.whatIfCalls)
	}
	assertSubstitutionWarning(t, "--dry-run", errb.String())
}

// --dry-run --json: the machine payload owns stdout and must still carry the
// field, while the human warning goes to stderr. Neither may be dropped — a
// scripted price check is precisely where nobody reads the terminal.
func TestSubstitution_WarnsOnDryRunJSONAndKeepsThePayloadField(t *testing.T) {
	withStdinTTY(t, false)
	s := subsSeams()
	o := baseOpts()
	o.dryRun = true
	o.jsonOut = true
	out, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	assertSubstitutionWarning(t, "--dry-run --json", errb.String())

	var m map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout is not a JSON object (the warning must not have leaked into it): %v\n%s", err, out.String())
	}
	if _, ok := m["modelSubstitutions"]; !ok {
		t.Errorf("--json passthrough lost `modelSubstitutions`:\n%s", out.String())
	}
	if strings.Contains(out.String(), substWarn) {
		t.Errorf("the warning must go to stderr, never into the --json payload:\n%s", out.String())
	}
}

func TestSubstitution_WarnsOnTTYConfirm(t *testing.T) {
	withStdinTTY(t, true)
	s := subsSeams()
	cmd, _, errb := genCmd("y\n")
	if err := runGenerate(cmd, s.deps(t), baseOpts()); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1", s.submitCalls)
	}
	assertSubstitutionWarning(t, "TTY confirm", errb.String())
	// 🔴 Ordering: the user must read it BEFORE approving, not after.
	stderr := errb.String()
	if i, j := strings.Index(stderr, substWarn), strings.Index(stderr, "Generate? [y/N]"); i < 0 || j < 0 || i > j {
		t.Errorf("the warning must precede the confirmation prompt (warn at %d, prompt at %d).\nstderr:\n%s", i, j, stderr)
	}
}

func TestSubstitution_WarnsOnYes(t *testing.T) {
	withStdinTTY(t, false)
	s := subsSeams()
	o := baseOpts()
	o.assumeYes = true
	_, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1", s.submitCalls)
	}
	assertSubstitutionWarning(t, "--yes", errb.String())
}

// --- post-spend surfaces -----------------------------------------------------

// The SUBMIT reply is a second, independent validation: a version can be gated
// or retired between the estimate and the submit, so this reply is the only
// thing that describes the generation actually charged.
func TestSubstitution_WarnsAfterSubmit(t *testing.T) {
	withStdinTTY(t, false)
	s := &genSeams{
		submitReply: &genapi.SubmitResult{
			ID: "wf_123", Status: "queued",
			RawModelSubstitutions: json.RawMessage(substArrayJSON),
		},
	}
	o := baseOpts()
	o.assumeYes = true
	_, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1", s.submitCalls)
	}
	assertSubstitutionWarning(t, "post-submit", errb.String())
	// The post-spend tense: there is no refund, so it must not read like advice
	// for a decision the user still gets to make.
	if !strings.Contains(errb.String(), "you were charged") {
		t.Errorf("the post-submit warning must say the charge already happened.\nstderr:\n%s", errb.String())
	}
}

// The submit reply's OTHER carrier. The server writes both; a reader that only
// looked at the top level would miss nothing here today, but the metadata copy
// is the one that survives — pinning it stops a refactor from dropping it.
func TestSubstitution_WarnsAfterSubmit_MetadataCarrier(t *testing.T) {
	withStdinTTY(t, false)
	s := &genSeams{
		submitReply: &genapi.SubmitResult{
			ID: "wf_123", Status: "queued",
			RawMetadata: json.RawMessage(`{"params":{},"modelSubstitutions":` + substArrayJSON + `}`),
		},
	}
	o := baseOpts()
	o.assumeYes = true
	_, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	assertSubstitutionWarning(t, "post-submit (metadata carrier)", errb.String())
	// Reported ONCE even when both carriers hold it — see the both-carriers case
	// in genapi's substitution_test.go for the merge itself.
	if n := strings.Count(errb.String(), substWarn); n != 1 {
		t.Errorf("the warning must appear exactly once per surface, got %d.\nstderr:\n%s", n, errb.String())
	}
}

// `workflows get` is the ONLY surface for a job this process did not submit — a
// --no-wait run collected later, a re-attach after Ctrl-C or a --timeout.
func TestSubstitution_WarnsOnWorkflowsGet(t *testing.T) {
	payload := `{"id":"wf_123","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":` + substArrayJSON + `},"steps":[]}`
	for _, jsonOut := range []bool{false, true} {
		name := "human"
		if jsonOut {
			name = "--json"
		}
		t.Run(name, func(t *testing.T) {
			c, out, errb := genCmd("")
			deps := wfGetDeps(payload, nil)
			if err := runWorkflowsGet(c, deps, workflowsGetOpts{jsonOut: jsonOut, baseURL: "https://civitai.com"}, "wf_123"); err != nil {
				t.Fatalf("workflows get: %v", err)
			}
			assertSubstitutionWarning(t, "workflows get "+name, errb.String())
			if jsonOut && strings.Contains(out.String(), substWarn) {
				t.Errorf("the warning must not leak into the --json payload:\n%s", out.String())
			}
		})
	}
}

// --- the common case: no substitution, no noise ------------------------------

// 🔴 CONTRAST CONTROL WITH ITS POSITIVE CONTROL IN THE SAME TEST. A renderer
// that printed unconditionally would pass every assertion above, and the warning
// would become noise users learn to click past. The populated half proves the
// same code path CAN warn, so the empty half's silence is a measurement rather
// than a wiring failure.
func TestSubstitution_SilentWhenNoneReported(t *testing.T) {
	withStdinTTY(t, false)

	// (A) negative: the default seams report no substitutions.
	quiet := &genSeams{}
	o := baseOpts()
	o.assumeYes = true
	_, quietErr, err := runImg(t, quiet, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if quiet.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit reached %d times, want 1", quiet.submitCalls)
	}
	if strings.Contains(quietErr.String(), substWarn) {
		t.Errorf("a run with no substitutions must print no substitution warning.\nstderr:\n%s", quietErr.String())
	}
	// It must not manufacture reassurance either — absence on the wire also
	// means "a server predating civitai#3665".
	if strings.Contains(strings.ToLower(quietErr.String()), "no substitution") {
		t.Errorf("silence is the contract; a 'no substitutions' line would be an unfounded guarantee.\nstderr:\n%s", quietErr.String())
	}

	// (B) positive control, same path, same helper: it DOES warn when there is
	// something to report.
	loud := subsSeams()
	_, loudErr, err := runImg(t, loud, o)
	if err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if !strings.Contains(loudErr.String(), substWarn) {
		t.Fatalf("POSITIVE CONTROL FAILED: the same path printed nothing for a populated reply.\nstderr:\n%s", loudErr.String())
	}
}

// The same pair for `workflows get`.
func TestSubstitution_WorkflowsGetSilentWhenNoneReported(t *testing.T) {
	quietPayload := `{"id":"wf_123","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"params":{"seed":1}},"steps":[]}`
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(quietPayload, nil), workflowsGetOpts{}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if strings.Contains(errb.String(), substWarn) {
		t.Errorf("no substitutions must mean no warning.\nstderr:\n%s", errb.String())
	}
	// Positive control on the identical call path.
	loudPayload := `{"id":"wf_123","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":` + substArrayJSON + `},"steps":[]}`
	c2, _, errb2 := genCmd("")
	if err := runWorkflowsGet(c2, wfGetDeps(loudPayload, nil), workflowsGetOpts{}, "wf_123"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(errb2.String(), substWarn) {
		t.Fatalf("POSITIVE CONTROL FAILED: the same path printed nothing for a populated payload.\nstderr:\n%s", errb2.String())
	}
}

// --- name resolution is a convenience, never a gate --------------------------

// 🔴 A FAILING ResolveModelVersion MUST NOT SUPPRESS THE WARNING. The name is
// decoration; the substitution is the fact the user is being billed for. A
// renderer that returned early on a lookup error would be silent in exactly the
// situation a substitution is most likely — an id the server does not recognise.
func TestSubstitution_WarnsEvenWhenNameResolutionFails(t *testing.T) {
	withStdinTTY(t, false)
	s := subsSeams()
	s.resolve = func(ctx context.Context, id int) (*genapi.ResolvedVersion, error) {
		return nil, civitai.Tag(civitai.ErrNotFound, errors.New("no such model version"))
	}
	o := baseOpts()
	o.dryRun = true
	_, errb, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("a failed name lookup must not fail the run: %v", err)
	}
	assertSubstitutionWarning(t, "resolution failure", errb.String())
	if !strings.Contains(errb.String(), "could not look up the name") {
		t.Errorf("the failed lookup must be stated, not swallowed.\nstderr:\n%s", errb.String())
	}
	// POSITIVE CONTROL: the resolver was actually consulted, so the assertion
	// above is about a failure and not about an unwired seam.
	if s.resolveCalls == 0 {
		t.Fatalf("POSITIVE CONTROL FAILED: the resolver was never called, so nothing failed")
	}
}

// A surface with NO resolver wired still warns, with ids.
func TestSubstitution_NilResolverStillWarns(t *testing.T) {
	payload := `{"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":` + substArrayJSON + `},"steps":[]}`
	deps := wfGetDeps(payload, nil)
	deps.resolveVersion = nil
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, deps, workflowsGetOpts{}, "wf_1"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	assertSubstitutionWarning(t, "nil resolver", errb.String())
}

// Names are shown when the lookup works — that is the whole point of resolving.
func TestSubstitution_ShowsResolvedNames(t *testing.T) {
	payload := `{"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":` + substArrayJSON + `},"steps":[]}`
	deps := wfGetDeps(payload, nil)
	deps.resolveVersion = func(ctx context.Context, id int) (*genapi.ResolvedVersion, error) {
		if id == 128713 {
			return &genapi.ResolvedVersion{VersionID: id, ModelName: "AskedFor", VersionName: "v1", ModelType: "Checkpoint"}, nil
		}
		return &genapi.ResolvedVersion{VersionID: id, ModelName: "ActuallyRan", VersionName: "v2", ModelType: "Checkpoint"}, nil
	}
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, deps, workflowsGetOpts{}, "wf_1"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	for _, want := range []string{"AskedFor", "ActuallyRan"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("the resolved name %q must be shown so the user reads names, not integers.\nstderr:\n%s", want, errb.String())
		}
	}
}

// --- warn, never refuse ------------------------------------------------------

// 🔴 THE DECISION, PINNED. Substitution on a modelLocked ecosystem is legitimate
// graceful degradation and the platform has deliberately NOT decided to reject
// it (its own module note defers that to a later phase). Refusing here would
// block flows that work today and would vendor a policy the server has not made.
// Fail-soft, exactly like serverQuantityClamp.
func TestSubstitution_WarnsButNeverRefuses(t *testing.T) {
	withStdinTTY(t, false)
	s := subsSeams()
	o := baseOpts()
	o.assumeYes = true
	_, _, err := runImg(t, s, o)
	if err != nil {
		t.Fatalf("a substitution must WARN, not refuse: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("the submit must still be reached: submitCalls = %d, want 1", s.submitCalls)
	}
	// And it must not smuggle in a new error kind either.
	if errors.Is(err, ErrUsage) || errors.Is(err, civitai.ErrBadRequest) {
		t.Errorf("a substitution must not classify the run as a failure: %v", err)
	}
}

// Exit codes stay pinned by errors.Is, not by message text: a substitution in
// the payload must not perturb an unrelated failure's classification.
func TestSubstitution_DoesNotDisturbExitCodes(t *testing.T) {
	apiErr := apiErrorWithStatus(t, http.StatusNotFound)
	c, _, _ := genCmd("")
	err := runWorkflowsGet(c, wfGetDeps("", apiErr), workflowsGetOpts{}, "wf_missing")
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Errorf("want civitai.ErrNotFound (exit 4), got %v", err)
	}
}

// --- the reason gloss is advisory ------------------------------------------

// An unknown reason renders VERBATIM and still warns. A gloss table that gated
// the warning would drop every reason added server-side after this build —
// which is the silence the whole feature exists to end.
func TestSubstitution_UnknownReasonStillWarnsVerbatim(t *testing.T) {
	const novel = "a-reason-from-the-future"
	payload := `{"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":[{"requested":1,"applied":2,"reason":"` + novel + `"}]},"steps":[]}`
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{}, "wf_1"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	if !strings.Contains(errb.String(), substWarn) || !strings.Contains(errb.String(), novel) {
		t.Errorf("an unknown reason must warn and be rendered verbatim.\nstderr:\n%s", errb.String())
	}
}

// A KNOWN reason gets its gloss. Positive control for the map: without this the
// test above passes with an empty map, and the gloss could rot unnoticed.
func TestSubstitution_KnownReasonIsGlossed(t *testing.T) {
	payload := `{"id":"wf_1","status":"succeeded","createdAt":"2026-08-05T12:00:00Z",
	  "metadata":{"modelSubstitutions":` + substArrayJSON + `},"steps":[]}`
	c, _, errb := genCmd("")
	if err := runWorkflowsGet(c, wfGetDeps(payload, nil), workflowsGetOpts{}, "wf_1"); err != nil {
		t.Fatalf("workflows get: %v", err)
	}
	gloss, ok := substitutionReasonGloss["unrecognized"]
	if !ok {
		t.Fatal("the gloss table lost `unrecognized`")
	}
	if !strings.Contains(errb.String(), gloss) {
		t.Errorf("a known reason must carry its gloss.\nstderr:\n%s", errb.String())
	}
}
