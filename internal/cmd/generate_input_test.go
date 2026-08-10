package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// --- harness -----------------------------------------------------------------

// writeGraphFile writes a --input document into a temp dir and returns its path.
func writeGraphFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write graph fixture: %v", err)
	}
	return p
}

// inputOpts is a valid minimal --input invocation: no prompt, --no-wait so the
// case stops at the submit seam, --yes so the money prompt is not the subject.
func inputOpts(path string) generateOpts {
	return generateOpts{
		inputPath: path,
		baseURL:   "https://civitai.com",
		noWait:    true,
		assumeYes: true,
		outDir:    ".",
	}
}

const cleanGraph = `{"workflow":"txt2img","prompt":"a cat","quantity":2}`

// --- happy paths -------------------------------------------------------------

// The graph in the file is what reaches the whatIf seam, verbatim — including a
// key this CLI does not model. Round-tripping through the typed Graph would
// delete it, which would make --input strictly worse than useless: the user's
// parameter would vanish before the request was even built.
func TestGenerateInput_FileIsSentVerbatim(t *testing.T) {
	path := writeGraphFile(t, `{
	  "workflow": "txt2img",
	  "prompt":   "a cat",
	  "someFutureNode": {"shift": 3}
	}`)
	s := &genSeams{}
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), inputOpts(path)); err != nil {
		t.Fatalf("--input: %v", err)
	}
	if s.whatIfCalls != 1 || s.submitCalls != 1 {
		t.Fatalf("whatIf=%d submit=%d, want 1/1", s.whatIfCalls, s.submitCalls)
	}
	raw, err := json.Marshal(s.lastGraph)
	if err != nil {
		t.Fatalf("marshal captured graph: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("captured graph is not an object: %v", err)
	}
	for _, k := range []string{"workflow", "prompt", "someFutureNode"} {
		if _, ok := m[k]; !ok {
			t.Errorf("key %q from the input file never reached the wire: %v", k, m)
		}
	}
	// No model-version resolution happens on this path: there is nothing typed
	// to resolve.
	if s.resolveCalls != 0 {
		t.Errorf("--input resolved %d model versions; it does not interpret the graph", s.resolveCalls)
	}
}

func TestGenerateInput_StdinIsAccepted(t *testing.T) {
	s := &genSeams{}
	c, _, _ := genCmd(cleanGraph)
	if err := runGenerate(c, s.deps(t), inputOpts("-")); err != nil {
		t.Fatalf("--input -: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", s.submitCalls)
	}
	if s.lastGraph.Raw == nil || !strings.Contains(string(s.lastGraph.Raw), `"a cat"`) {
		t.Errorf("the stdin graph did not reach the seam: %s", s.lastGraph.Raw)
	}
}

// --- refusals ----------------------------------------------------------------

func TestGenerateInput_MalformedJSONIsAUsageError(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":  `{"workflow":"txt2img",`,
		"not-object": `["txt2img"]`,
		"scalar":     `42`,
		"empty":      "   \n",
	} {
		t.Run(name, func(t *testing.T) {
			s := &genSeams{}
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t, body)))
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("want ErrUsage (exit 2), got %v", err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("a malformed graph reached the network: whatIf=%d submit=%d", s.whatIfCalls, s.submitCalls)
			}
		})
	}
}

func TestGenerateInput_MissingFileIsAUsageError(t *testing.T) {
	s := &genSeams{}
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), inputOpts(filepath.Join(t.TempDir(), "nope.json")))
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("want ErrUsage, got %v", err)
	}
}

// 🔴 The content-audit gate. A graph whose workflow is anything but txt2img is
// refused because the server's prompt audit is keyed on a top-level `prompt`
// string node and rebuilds its `data` from DECLARED nodes only — a graph
// carrying its prompt elsewhere is the shape that could slip it. Until that is
// resolved upstream this CLI must not be the vector.
func TestGenerateInput_NonTxt2ImgWorkflowIsRefused(t *testing.T) {
	// 🔴 Each case asserts THIS branch's own message, not merely "some error".
	// Measured while mutation-testing the gate: with the presence check and the
	// type check individually defeated, every case still came back ErrUsage —
	// the equality check catches them all, because an absent or non-string
	// `workflow` leaves the decoded value "" and "" != "txt2img". A test that
	// asserted only the kind was therefore green for the wrong reason on two of
	// the three branches, and would have passed with either deleted.
	cases := []struct {
		name string
		body string
		want string
	}{
		{"img2img", `{"workflow":"img2img","prompt":"a cat"}`, `declares workflow "img2img"`},
		{"comfy", `{"workflow":"comfy","nodes":{"3":{"inputs":{"text":"a cat"}}}}`, `declares workflow "comfy"`},
		{"txt2vid", `{"workflow":"txt2vid","prompt":"a cat"}`, `declares workflow "txt2vid"`},
		{"absent", `{"prompt":"a cat"}`, `has no "workflow" key`},
		{"non-string", `{"workflow":123,"prompt":"a cat"}`, `non-string "workflow" value`},
		{"case-variant", `{"workflow":"TXT2IMG","prompt":"a cat"}`, `declares workflow "TXT2IMG"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &genSeams{}
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t, tc.body)))
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("want ErrUsage (exit 2), got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal came from a different guard than this case exercises\n  want message containing: %s\n  got: %v", tc.want, err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("a non-txt2img graph reached the network: whatIf=%d submit=%d", s.whatIfCalls, s.submitCalls)
			}
		})
	}
	// Positive control: the SAME path accepts txt2img and reaches the seams, so
	// the zeros above are not a harness that never runs anything.
	s := &genSeams{}
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t, cleanGraph))); err != nil {
		t.Fatalf("control txt2img run: %v", err)
	}
	if s.whatIfCalls != 1 || s.submitCalls != 1 {
		t.Fatalf("control: whatIf=%d submit=%d, want 1/1", s.whatIfCalls, s.submitCalls)
	}
}

// --- the envelope-key money guard --------------------------------------------

// envelopeMoneyKeys reports which envelope-only keys are present at the TOP
// level of a tRPC generate body's `json` object — i.e. as SIBLINGS of the graph,
// where the server destructures them and where a tip would actually be charged.
//
// It deliberately does not look inside `json.input`: a key sitting inside the
// graph is inert (the server drops undeclared graph nodes), and conflating the
// two would make the guard fire on a harmless payload while still missing the
// dangerous one.
func envelopeMoneyKeys(t *testing.T, body []byte) []string {
	t.Helper()
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("submit body is not JSON: %v\n%s", err, body)
	}
	inner, ok := outer["json"].(map[string]any)
	if !ok {
		t.Fatalf("submit body has no \"json\" wrapper: %v", outer)
	}
	// 🔴 This list is written out HERE rather than read from envelopeOnlyKeys.
	// Deriving it from the production constant would make the detector shrink in
	// lockstep with the very list it is meant to police: dropping a key from
	// production would silently stop the detector looking for it, and this test
	// would go green on the exact regression it exists to catch. (Measured: with
	// the detector reading envelopeOnlyKeys, deleting "civitaiTip" from
	// production left this assertion unable to see a civitaiTip.)
	//
	// `input` and `externalId` are deliberately absent: `input` IS the graph and
	// `externalId` is the idempotency key the CLI mints itself, so both are
	// CORRECT on this envelope. A file-supplied value for either is stopped
	// earlier, by the refusal test's guard.
	forbidden := []string{"civitaiTip", "creatorTip", "buzzType", "tags",
		"sourceMetadata", "sourceMetadataMap", "remixOfId"}
	var found []string
	for _, k := range forbidden {
		if _, ok := inner[k]; ok {
			found = append(found, k)
		}
	}
	return found
}

// 🔴 A ledger of the envelope siblings the server destructures, asserted as an
// exact SET so it fails when it grows OR shrinks.
//
// Growth matters because a new sibling added server-side is a new key that must
// be refused in an input file; shrinkage matters because deleting one from the
// production list silently reopens the hole. The source of truth is
// `generateFromGraph`'s destructure in
// civitai/civitai -> src/server/routers/orchestrator.router.ts.
func TestEnvelopeOnlyKeys_Ledger(t *testing.T) {
	want := map[string]bool{
		"input": true, "civitaiTip": true, "creatorTip": true, "tags": true,
		"sourceMetadata": true, "sourceMetadataMap": true, "remixOfId": true,
		"buzzType": true, "externalId": true,
	}
	got := map[string]bool{}
	for _, k := range envelopeOnlyKeys {
		got[k] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("envelopeOnlyKeys no longer refuses %q — a graph file could set it again", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("envelopeOnlyKeys gained %q; confirm it against the server's generateFromGraph destructure and update this ledger", k)
		}
	}
}

// 🔴 The headline money guard, asserted on the ACTUAL bytes of the outgoing
// request, with a positive control that proves the assertion can see a tip.
//
// A committed graph.json carrying `civitaiTip: 5000` must not charge a tip. And
// note WHY --dry-run cannot backstop this: whatIf prices a strictly smaller body
// than submit and is never sent tips at all, so the estimate would be the
// untipped one and --max-cost would compare against it.
func TestGenerateInput_EnvelopeKeysNeverReachTheWire(t *testing.T) {
	// --- positive control: the detector CAN observe a tip -------------------
	tipped := []byte(`{"json":{"input":{"workflow":"txt2img"},"externalId":"x","civitaiTip":5000,"creatorTip":10}}`)
	got := envelopeMoneyKeys(t, tipped)
	if len(got) != 2 {
		t.Fatalf("positive control: the detector found %v in a body that carries civitaiTip AND creatorTip — it cannot observe a tip, so the assertion below would be vacuous", got)
	}

	// --- the real request ---------------------------------------------------
	var submitBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == genapi.WhatIfPath:
			_, _ = w.Write([]byte(`{"result":{"data":{"json":{"ready":true,"cost":{"base":8,"total":12}}}}}`))
		case r.Method == http.MethodPost && r.URL.Path == genapi.GeneratePath:
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read submit body: %v", err)
			}
			submitBody = b
			_, _ = w.Write([]byte(`{"result":{"data":{"json":{"id":"wf_1","status":"queued"}}}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	gen := genapi.New(srv.URL, "test-key")
	deps := generateDeps{
		whatIf:      gen.WhatIfFromGraph,
		submit:      gen.GenerateFromGraph,
		buzzBalance: func(ctx context.Context) (int64, error) { return 1_000_000, nil },
		pendingDir:  t.TempDir(),
	}
	// A graph that ALSO happens to name a graph key — the request must still
	// carry no envelope tip.
	c, _, _ := genCmd("")
	if err := runGenerate(c, deps, inputOpts(writeGraphFile(t, cleanGraph))); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(submitBody) == 0 {
		t.Fatal("the submit body was never captured — the assertion below would be vacuous")
	}
	if found := envelopeMoneyKeys(t, submitBody); len(found) != 0 {
		t.Fatalf("envelope-only keys reached the outgoing request: %v\n%s", found, submitBody)
	}
}

// A file carrying an envelope-only key is REFUSED, not stripped. Stripping would
// silently override an explicit instruction in the user's own file, and the
// warning would go to a stream an unattended run discards.
func TestGenerateInput_EnvelopeKeyInTheFileIsRefused(t *testing.T) {
	for _, k := range []string{"civitaiTip", "creatorTip", "buzzType", "tags", "externalId",
		"sourceMetadata", "sourceMetadataMap", "remixOfId", "input"} {
		t.Run(k, func(t *testing.T) {
			body := fmt.Sprintf(`{"workflow":"txt2img","prompt":"a cat",%q:5000}`, k)
			s := &genSeams{}
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t, body)))
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("%q must be refused with ErrUsage, got %v", k, err)
			}
			if !strings.Contains(err.Error(), k) {
				t.Errorf("the error does not name the offending key %q: %v", k, err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("a graph carrying %q reached the network: whatIf=%d submit=%d", k, s.whatIfCalls, s.submitCalls)
			}
		})
	}
}

// --- the unknown-key warning -------------------------------------------------

// It WARNS and does not block: the CLI does not vendor the server's node
// registry, so it has no authority to call a key invalid — only to say it does
// not model it.
func TestGenerateInput_UnknownKeyWarnsAndProceeds(t *testing.T) {
	s := &genSeams{}
	c, _, errb := genCmd("")
	err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t,
		`{"workflow":"txt2img","prompt":"a cat","shift":3,"foobar":123}`)))
	if err != nil {
		t.Fatalf("an unknown key must not block: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1 — the warning must not stop the run", s.submitCalls)
	}
	stderr := errb.String()
	for _, want := range []string{"shift", "foobar", unknownKeyWarningMarker} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	// 🔴 Framing: a statement about the CLI's own knowledge, never a verdict on
	// the key — in EITHER direction. Claiming the key is invalid, and claiming
	// the server ignores it, are both mirrors of a registry this CLI does not
	// vendor; #343 shipped the second one and it was false.
	assertNoServerVerdictOnUnknownKeys(t, stderr)
}

// Negative control: every key the CLI models must produce NO warning, or the
// advisory becomes noise everyone learns to ignore.
func TestGenerateInput_KnownKeysDoNotWarn(t *testing.T) {
	s := &genSeams{}
	c, _, errb := genCmd("")
	body := `{"workflow":"txt2img","ecosystem":"SDXL","prompt":"a cat","negativePrompt":"blurry",
	  "quantity":2,"aspectRatio":"1:1","model":{"id":1},"resources":[],"steps":25,"cfgScale":7,
	  "sampler":"Euler","seed":42}`
	if err := runGenerate(c, s.deps(t), inputOpts(writeGraphFile(t, body))); err != nil {
		t.Fatalf("--input: %v", err)
	}
	if strings.Contains(errb.String(), unknownKeyWarningMarker) {
		t.Errorf("a graph of only modelled keys produced an unknown-key warning:\n%s", errb.String())
	}
}

// unknownKeyWarningMarker is the phrase both the presence and the absence
// assertions key off, so they cannot drift apart and start proving different
// things — one of them would then pass vacuously.
const unknownKeyWarningMarker = "this CLI does not model"

// retractedUnknownKeyClaims are the assertions about the SERVER that #343
// removed from unknownKeyWarning, plus the nearby shapes the same mistake takes.
//
// 🔴 THIS IS A RETRACTION CHECK, NOT THE GUARD — exactly the split
// `retractedCancelClaims` records for the cancel copy. AGENTS.md item 28 is the
// evidence that a phrase list loses: two rounds of banned substrings were beaten
// by paraphrase, and "does this sentence assert what the server does with the
// key" is not computable from text. The GUARD is the golden file
// (`generate_input_unknown_key_warning`), which fails on ANY change including an
// addition. This list exists only so the SPECIFIC sentences #343 measured to be
// false fail loudly by name if they are ever reinstated.
var retractedUnknownKeyClaims = []string{
	"silently ignores",
	"prices the same",
	"has no effect",
	"returns http 200",
	"silently drops",
	"invalid key",
	"unsupported key",
	"is not a valid",
}

// assertNoServerVerdictOnUnknownKeys is deliberately applied to WHOLE STDERR at
// the run-level call site, not just to unknownKeyWarning's return value: the
// claim is wrong wherever it lands on that screen, and #343's damage came from
// the user reading the screen, not the function.
//
// 🔴 The cost of that breadth, named so it is diagnosed rather than debugged: a
// fixture that grows an --image would drag in validateImageOpts' unrelated
// "silently ignores the images" (generate.go), which is a DIFFERENT and
// separately-measured claim about a different key. It would trip this check as a
// false positive. That is a loud red with this message, not a silent pass — but
// if you hit it, narrow the call site to the warning string rather than deleting
// the banned phrase.
func assertNoServerVerdictOnUnknownKeys(t *testing.T, got string) {
	t.Helper()
	low := strings.ToLower(got)
	for _, banned := range retractedUnknownKeyClaims {
		if strings.Contains(low, banned) {
			t.Errorf("the unknown-key copy asserts a verdict on the key (%q). It may report only that this CLI does not model it "+
				"and that it is sent as written — #343 measured the retracted version wrong: `priority` was honoured and "+
				"more than tripled the price on the same screen.\n%s", banned, got)
		}
	}
}

// The warning must NAME --dry-run, because that is its only actionable half now
// that the false "no effect" reassurance is gone. Without it the advisory tells
// the user something is unchecked and gives them nowhere to check it.
//
// It is a separate assertion from the golden deliberately: the golden proves the
// text has not changed, this proves the text still does its job, and a `-update`
// run silently re-approves the former but not the latter.
func TestUnknownKeyWarning_PointsAtTheOneSurfaceThatCanShowAPriceEffect(t *testing.T) {
	got := unknownKeyWarning([]string{"priority"})
	if !strings.Contains(got, "--dry-run") {
		t.Errorf("the warning does not name --dry-run, so it reports an unchecked key with no way to check it:\n%s", got)
	}
	assertNoServerVerdictOnUnknownKeys(t, got)
}

// --- --fail-on-substitution's --input COVERAGE NOTE (#342) -------------------

// 🔴 THE FLAG STAYS LIVE HERE, AND A REFUSAL WAS WITHDRAWN. #342 measured it
// armed, silent and charged: a credentialed run passed --fail-on-substitution
// with `resources:[{modelVersionId:128713,…}]`, was billed 28 Buzz, and the job
// ran model version 2442439 with no report and no refusal. The first fix drafted
// was a usage error on the flag COMBINATION. It was withdrawn because
// substitutionRefusal keys off the ESTIMATE's reply, not off o.checkpoint — so
// on a raw graph the flag is a working PRE-SPEND guard, and refusing the
// combination would have deleted a guard that works. The defect is that its
// coverage is partial and undocumented; the note is the documentation.
//
// This test pins the half that matters for money: the run PROCEEDS.
func TestGenerateInput_FailOnSubstitutionIsNotRefused(t *testing.T) {
	for _, name := range []string{"submit", "dry-run"} {
		t.Run(name, func(t *testing.T) {
			o := inputOpts(writeGraphFile(t, cleanGraph))
			o.failOnSubstitution = true
			o.dryRun = name == "dry-run"
			s := &genSeams{}
			c, _, errb := genCmd("")
			if err := runGenerate(c, s.deps(t), o); err != nil {
				t.Fatalf("--input with --fail-on-substitution must NOT be refused, got %v", err)
			}
			// 🔴 The estimate must still be fetched — that reply is the ONLY thing
			// --fail-on-substitution can fire on. A version of this that skipped the
			// quote would leave the flag genuinely inert while this test stayed green.
			if s.whatIfCalls != 1 {
				t.Errorf("whatIf calls = %d, want 1 — the flag can only fire on the estimate's reply", s.whatIfCalls)
			}
			if !strings.Contains(errb.String(), inputCoverageNoteMarker) {
				t.Errorf("the coverage note did not print:\n%s", errb.String())
			}
		})
	}
}

// 🔴 THE NOTE IS UNCONDITIONAL ON THE FLAGS AND BLIND TO THE FILE. Deciding
// whether to warn by looking at the graph would be the per-key registry item 13
// forbids — and it is what lets the note make a claim about the FLAG rather than
// about the FILE. Three graphs whose model references differ in exactly the way
// a file-inspecting version would branch on must all produce the SAME note.
func TestInputSubstitutionCoverageNote_DoesNotDependOnTheGraph(t *testing.T) {
	graphs := map[string]string{
		"top-level model":    `{"workflow":"txt2img","prompt":"a cat","model":{"id":128713}}`,
		"nested resources":   `{"workflow":"txt2img","prompt":"a cat","resources":[{"id":128713}]}`,
		"no model reference": cleanGraph,
	}
	seen := map[string]bool{}
	for name, body := range graphs {
		o := inputOpts(writeGraphFile(t, body))
		o.failOnSubstitution = true
		s := &genSeams{}
		c, _, errb := genCmd("")
		if err := runGenerate(c, s.deps(t), o); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(errb.String(), inputCoverageNoteMarker) {
			t.Fatalf("%s: no coverage note:\n%s", name, errb.String())
		}
		seen[inputSubstitutionCoverageNote(o)] = true
	}
	if len(seen) != 1 {
		t.Errorf("the coverage note varies with the graph's contents (%d distinct texts) — it must read two flags and never the file, "+
			"or it is making a claim about which keys the server honours (item 13)", len(seen))
	}
}

// Both controls on the renderer, so the assertions above cannot pass because it
// is inert or because it fires unconditionally.
func TestInputSubstitutionCoverageNote_Controls(t *testing.T) {
	both := generateOpts{inputPath: "g.json", failOnSubstitution: true}
	if inputSubstitutionCoverageNote(both) == "" {
		t.Fatal("positive control: both flags set rendered no note")
	}
	for name, o := range map[string]generateOpts{
		"--input alone":                {inputPath: "g.json"},
		"--fail-on-substitution alone": {failOnSubstitution: true},
		"neither":                      {},
	} {
		if got := inputSubstitutionCoverageNote(o); got != "" {
			t.Errorf("%s must render no note, got %q", name, got)
		}
	}
}

// inputCoverageNoteMarker is the phrase the presence assertions key off. It is
// the note's own claim — that the flag is live but its reach is not knowable
// here — so a rewording that drops the claim fails these tests rather than
// silently passing on a phrase that survived by accident.
const inputCoverageNoteMarker = "LIVE with --input"

// 🔴 THE NOTE MUST NOT CLAIM WHICH GRAPH KEYS ARE COVERED. Item 21(a)'s
// "TOP-LEVEL" describes where in the REPLY the record is carried — the reply's
// own top level versus nested under `metadata` — NOT which model reference in
// the REQUEST graph the server substitutes and reports on. Reading the first as
// the second turns one paid observation into a per-key rule about the server's
// registry, which is #343's mistake in a new costume. Retraction check, not the
// guard: the golden is the guard.
func TestInputSubstitutionCoverageNote_MakesNoPerKeyCoverageClaim(t *testing.T) {
	got := strings.ToLower(inputSubstitutionCoverageNote(generateOpts{inputPath: "g.json", failOnSubstitution: true}))
	for _, banned := range []string{
		"top-level model substitutions only",
		"covers top-level",
		"are not reported",
		"cannot see them",
		"nested resources are not",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("the coverage note asserts WHICH graph keys the server reports on (%q). "+
				"Item 21(a) is about the REPLY's carrier shape, not the request graph's nesting — that inference is #343's error. "+
				"It may say only that the flag is live, that it fires on the estimate's record, and that this CLI cannot relate that record to an uninterpreted file.", banned)
		}
	}
	// Positive control: the clauses it MUST carry.
	for _, want := range []string{"estimate", "not interpreted", "nothing was reported"} {
		if !strings.Contains(got, want) {
			t.Errorf("the coverage note lost the clause %q, so the ban above could pass over an empty claim:\n%s", want, got)
		}
	}
}

// 🔴 THE LOAD-BEARING TEST FOR THE WHOLE #342 DECISION. The coverage note tells
// the user the flag "is LIVE with --input … a real pre-spend guard". That is a
// behavioural claim, and prose is not evidence for it: this is. With the ESTIMATE
// reporting a substitution on a RAW-GRAPH run, --fail-on-substitution must refuse
// with ErrModelSubstituted and the submit seam must never be reached.
//
// Without this, the withdrawn refusal could be reinstated — or the flag could go
// inert on this path by accident — and every other test here would stay green
// while the note became a false promise on a money surface. It is also the
// counterexample to "the flag structurally cannot fire on a raw graph", the
// premise the refusal was originally drafted on.
func TestGenerateInput_FailOnSubstitutionStillFiresOnARawGraph(t *testing.T) {
	s := &genSeams{}
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q := okQuote(12)
		q.ModelSubstitutions = []genapi.ModelSubstitution{
			{Requested: 128713, Applied: 2442439, Reason: genapi.SubstitutionUnrecognized},
		}
		return q, okQuoteRaw(12), nil
	}
	o := inputOpts(writeGraphFile(t, cleanGraph))
	o.failOnSubstitution = true
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, ErrModelSubstituted) {
		t.Fatalf("--fail-on-substitution must still refuse a REPORTED substitution on a raw graph, got %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("it refused but submitted anyway, so it is not a pre-spend guard: submit=%d", s.submitCalls)
	}
	if s.whatIfCalls != 1 {
		t.Errorf("whatIf calls = %d, want 1", s.whatIfCalls)
	}
}

// The flag stays live on the FLAG path too. A change that disarmed
// --fail-on-substitution everywhere would pass the --input tests above while
// removing the feature.
func TestFailOnSubstitution_StillRefusesOnTheFlagPath(t *testing.T) {
	var s genSeams
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q := okQuote(12)
		q.ModelSubstitutions = []genapi.ModelSubstitution{
			{Requested: 111, Applied: 222, Reason: genapi.SubstitutionUnrecognized},
		}
		return q, okQuoteRaw(12), nil
	}
	o := baseOpts()
	o.assumeYes = true
	o.failOnSubstitution = true
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, ErrModelSubstituted) {
		t.Fatalf("--fail-on-substitution without --input must still refuse with ErrModelSubstituted, got %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("the substitution refusal submitted anyway: submit=%d", s.submitCalls)
	}
}

// The warning message itself, exercised directly with both controls, so the
// "no warning" assertion above cannot pass because the renderer is inert.
func TestUnknownKeyWarning_Controls(t *testing.T) {
	if got := unknownKeyWarning(nil); got != "" {
		t.Errorf("no unknown keys must render no warning, got %q", got)
	}
	got := unknownKeyWarning([]string{"shift"})
	if got == "" {
		t.Fatal("positive control: one unknown key rendered no warning — the empty case above proves nothing")
	}
	if !strings.Contains(got, `"shift"`) {
		t.Errorf("the warning does not name the key: %s", got)
	}
}

// --- mutual exclusion --------------------------------------------------------

func TestGenerateInput_RejectsContentFlags(t *testing.T) {
	path := writeGraphFile(t, cleanGraph)
	cases := map[string]func(*generateOpts){
		"prompt argument":   func(o *generateOpts) { o.prompt = "a cat" },
		"--negative-prompt": func(o *generateOpts) { o.negativePrompt = "blurry" },
		"--quantity":        func(o *generateOpts) { o.quantity, o.quantitySet = 4, true },
		"--aspect-ratio":    func(o *generateOpts) { o.aspectRatio = "1:1" },
		"--checkpoint":      func(o *generateOpts) { o.checkpoint, o.checkpointSet = 128713, true },
		"--lora":            func(o *generateOpts) { o.loras = []string{"250712:0.8"} },
	}
	for name, apply := range cases {
		t.Run(name, func(t *testing.T) {
			o := inputOpts(path)
			apply(&o)
			s := &genSeams{}
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), o)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("--input with %s must be ErrUsage, got %v", name, err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 {
				t.Errorf("the rejected combination still reached the network: whatIf=%d submit=%d", s.whatIfCalls, s.submitCalls)
			}
		})
	}
}

// The execution flags stay valid with --input: they govern HOW the request is
// made, never what is in the graph. If this ever regressed, --input would become
// unusable in exactly the scripted contexts it exists for.
func TestGenerateInput_ExecutionFlagsAreAllowed(t *testing.T) {
	path := writeGraphFile(t, cleanGraph)
	cases := map[string]func(*generateOpts){
		"--dry-run":     func(o *generateOpts) { o.dryRun = true },
		"--max-cost":    func(o *generateOpts) { o.maxCost, o.maxCostSet = 1000, true },
		"--json+--yes":  func(o *generateOpts) { o.jsonOut = true },
		"--force":       func(o *generateOpts) { o.force = true },
		"--out-dir":     func(o *generateOpts) { o.outDir = "./out" },
		"--external-id": func(o *generateOpts) { o.externalID = "reuse-me" },
		"--timeout":     func(o *generateOpts) { o.timeout = 0 },
	}
	for name, apply := range cases {
		t.Run(name, func(t *testing.T) {
			o := inputOpts(path)
			apply(&o)
			s := &genSeams{}
			c, _, _ := genCmd("")
			if err := runGenerate(c, s.deps(t), o); err != nil {
				t.Fatalf("--input with %s must be accepted, got %v", name, err)
			}
			if s.whatIfCalls != 1 {
				t.Errorf("whatIf calls = %d, want 1", s.whatIfCalls)
			}
		})
	}
}

// --- --print-input -----------------------------------------------------------

// 🔴 --print-input must reach NO money seam: not the submit, and not even the
// estimator. The zeros are paired with a positive control in the same test that
// drives both counters to 1.
func TestPrintInput_MakesNoNetworkCall(t *testing.T) {
	s := &genSeams{}
	c, out, _ := genCmd("")
	o := baseOpts()
	o.printInput = true
	o.quantity, o.quantitySet = 2, true
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--print-input: %v", err)
	}
	if s.whatIfCalls != 0 || s.submitCalls != 0 || s.resolveCalls != 0 || s.balanceCalls != 0 {
		t.Fatalf("--print-input made network calls: whatIf=%d submit=%d resolve=%d balance=%d",
			s.whatIfCalls, s.submitCalls, s.resolveCalls, s.balanceCalls)
	}

	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("--print-input stdout is not JSON: %v\n%s", err, out.String())
	}
	if m["workflow"] != generateWorkflow || m["prompt"] != o.prompt || m["quantity"] != float64(2) {
		t.Errorf("printed graph = %v", m)
	}
	// The envelope siblings are CLI-owned and must NOT be printed: --input
	// refuses them, so emitting them would produce a document that cannot be fed
	// back in.
	for _, k := range []string{"externalId", "civitaiTip", "creatorTip", "tags", "input"} {
		if _, ok := m[k]; ok {
			t.Errorf("--print-input emitted the envelope key %q, which --input then refuses: %v", k, m)
		}
	}

	// --- positive control: the SAME counters move on a normal run -----------
	s2 := &genSeams{}
	c2, _, _ := genCmd("")
	control := baseOpts()
	control.assumeYes = true
	if err := runGenerate(c2, s2.deps(t), control); err != nil {
		t.Fatalf("control run: %v", err)
	}
	if s2.whatIfCalls != 1 || s2.submitCalls != 1 {
		t.Fatalf("control: whatIf=%d submit=%d, want 1/1 — the zeros above would otherwise prove nothing",
			s2.whatIfCalls, s2.submitCalls)
	}
}

// The one network call --print-input CAN make, stated rather than hidden: with
// --checkpoint/--lora it still performs the public model-version READ, because
// graph `resources[]` REQUIRE the model type and a bare {id} is rejected 400.
// Skipping it would print a document --input could not submit. No MONEY seam is
// touched either way — that is the property that matters.
func TestPrintInput_ResolvesVersionIdsButTouchesNoMoneySeam(t *testing.T) {
	s := &genSeams{}
	c, out, _ := genCmd("")
	o := baseOpts()
	o.printInput = true
	o.checkpoint, o.checkpointSet = 128713, true
	o.loras = []string{"250712:0.8"}
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--print-input: %v", err)
	}
	if s.whatIfCalls != 0 || s.submitCalls != 0 || s.balanceCalls != 0 {
		t.Fatalf("--print-input reached a money seam: whatIf=%d submit=%d balance=%d",
			s.whatIfCalls, s.submitCalls, s.balanceCalls)
	}
	if s.resolveCalls != 2 {
		t.Fatalf("version resolutions = %d, want 2 (checkpoint + lora) — the printed graph would otherwise lack model.type", s.resolveCalls)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("--print-input stdout is not JSON: %v\n%s", err, out.String())
	}
	res, _ := m["resources"].([]any)
	if len(res) != 1 {
		t.Fatalf("printed graph has no resources: %v", m)
	}
	first, _ := res[0].(map[string]any)
	if first["model"] == nil {
		t.Errorf("the printed LoRA carries no model.type; --input would submit it and get a 400: %v", first)
	}
}

// The round-trip the flag exists for: --print-input's output must be accepted by
// --input unchanged. A guard on the two halves agreeing, not on either alone.
func TestPrintInput_OutputRoundTripsThroughInput(t *testing.T) {
	s := &genSeams{}
	c, out, _ := genCmd("")
	o := baseOpts()
	o.printInput = true
	o.negativePrompt = "blurry"
	o.aspectRatio = "1:1"
	o.quantity, o.quantitySet = 3, true
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--print-input: %v", err)
	}
	printed := out.String()

	s2 := &genSeams{}
	c2, _, errb2 := genCmd(printed)
	if err := runGenerate(c2, s2.deps(t), inputOpts("-")); err != nil {
		t.Fatalf("--print-input output was rejected by --input: %v\n%s", err, printed)
	}
	if strings.Contains(errb2.String(), "does not recognise") {
		t.Errorf("--print-input emitted a key --input does not recognise:\n%s\n%s", printed, errb2.String())
	}
	if s2.submitCalls != 1 {
		t.Fatalf("round-trip submit calls = %d, want 1", s2.submitCalls)
	}
}

// --print-input with --input echoes the file after validating it, still without
// touching the network — which is how a user checks a hand-edited graph.
func TestPrintInput_WithInputValidatesAndEchoes(t *testing.T) {
	s := &genSeams{}
	c, out, _ := genCmd("")
	o := inputOpts(writeGraphFile(t, `{"workflow":"txt2img","prompt":"a cat","someFutureNode":{"shift":3}}`))
	o.printInput = true
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--input --print-input: %v", err)
	}
	if s.whatIfCalls != 0 || s.submitCalls != 0 {
		t.Fatalf("--input --print-input made network calls: whatIf=%d submit=%d", s.whatIfCalls, s.submitCalls)
	}
	if !strings.Contains(out.String(), "someFutureNode") {
		t.Errorf("the echoed graph lost an unmodelled key:\n%s", out.String())
	}

	// It still REFUSES a bad graph rather than printing it: --print-input is a
	// preview of what would be sent, and nothing is sent for a refused file.
	c2, out2, _ := genCmd("")
	o2 := inputOpts(writeGraphFile(t, `{"workflow":"img2img","prompt":"a cat"}`))
	o2.printInput = true
	if err := runGenerate(c2, (&genSeams{}).deps(t), o2); !errors.Is(err, ErrUsage) {
		t.Fatalf("--print-input must not launder a refused graph, got %v", err)
	}
	if out2.Len() != 0 {
		t.Errorf("a refused graph was printed anyway: %s", out2.String())
	}
}
