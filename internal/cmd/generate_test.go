package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

// --- harness -----------------------------------------------------------------

// genSeams is a recording stand-in for generateDeps. The counters are the point:
// the safety claims this command makes are all of the form "the submit seam was
// NEVER reached", and a bare zero from a counter nothing increments would look
// identical. Every test that reads submits==0 is paired with a case in this same
// file that drives the SAME counter to 1 (see
// TestGenerate_NonTTYWithoutYesRefusesAndNeverSubmits).
type genSeams struct {
	whatIfCalls  int
	submitCalls  int
	resolveCalls int
	balanceCalls int

	// lastGraph is the graph handed to whatIf — captured so a test can assert on
	// the payload that would go over the wire.
	lastGraph genapi.Graph
	// lastExternalID is the idempotency key the submit seam was handed. It must
	// be non-empty: the CLI mints it BEFORE the POST so it can be recorded.
	lastExternalID string

	whatIf      func(ctx context.Context, g genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error)
	resolve     func(ctx context.Context, id int) (*genapi.ResolvedVersion, error)
	balance     func(ctx context.Context) (int64, error)
	submitErr   error
	submitReply *genapi.SubmitResult
	submitRaw   json.RawMessage

	// getWorkflow is the poll seam; nil leaves it unwired (fine for every test
	// that stops at --no-wait or never reaches a submit).
	getWorkflow getWorkflowFn
	// downloadBlob is the credential-free blob fetcher.
	downloadBlob blobFetcher
	// poll overrides the poll cadence (a test must never really sleep).
	poll pollConfig
	// pendingDirOverride pins the crash-recovery record's directory. Left empty
	// deps() points it at a t.TempDir(): a unit test must NEVER write into the
	// developer's real ~/.config/civitai.
	pendingDirOverride string
	// submitObserver runs INSIDE the submit seam, which is what makes the
	// "state file written BEFORE the POST" claim an ordering assertion rather
	// than an existence one.
	submitObserver func()
}

// deps wires the seams into the struct runGenerate consumes.
func (s *genSeams) deps(t *testing.T) generateDeps {
	t.Helper()
	dir := s.pendingDirOverride
	if dir == "" {
		dir = t.TempDir()
	}
	return generateDeps{
		getWorkflow:  s.getWorkflow,
		downloadBlob: s.downloadBlob,
		pendingDir:   dir,
		poll:         s.poll,
		whatIf: func(ctx context.Context, g genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
			s.whatIfCalls++
			s.lastGraph = g
			if s.whatIf != nil {
				return s.whatIf(ctx, g)
			}
			return okQuote(12), okQuoteRaw(12), nil
		},
		submit: func(ctx context.Context, g genapi.Graph, opts genapi.SubmitOptions) (*genapi.SubmitResult, string, json.RawMessage, error) {
			s.submitCalls++
			s.lastExternalID = opts.ExternalID
			if s.submitObserver != nil {
				s.submitObserver()
			}
			if s.submitErr != nil {
				return nil, opts.ExternalID, nil, s.submitErr
			}
			r := s.submitReply
			if r == nil {
				r = &genapi.SubmitResult{ID: "wf_123", Status: "queued"}
			}
			raw := s.submitRaw
			if raw == nil {
				raw = json.RawMessage(`{"id":"wf_123","status":"queued"}`)
			}
			return r, opts.ExternalID, raw, nil
		},
		resolveVersion: func(ctx context.Context, id int) (*genapi.ResolvedVersion, error) {
			s.resolveCalls++
			if s.resolve != nil {
				return s.resolve(ctx, id)
			}
			return &genapi.ResolvedVersion{VersionID: id, ModelName: "DreamShaper", VersionName: "v8", ModelType: "Checkpoint"}, nil
		},
		buzzBalance: func(ctx context.Context) (int64, error) {
			s.balanceCalls++
			if s.balance != nil {
				return s.balance(ctx)
			}
			return 1_000_000, nil
		},
	}
}

func okQuote(total float64) *genapi.WhatIfResult {
	return &genapi.WhatIfResult{Ready: true, Cost: &genapi.WorkflowCost{
		Base: 8, Total: total, Factors: map[string]float64{"quantity": 1.5}}}
}

func okQuoteRaw(total float64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"ready":true,"cost":{"base":8,"total":%g,"factors":{"quantity":1.5}},"serverOnlyField":"kept"}`, total))
}

// genCmd builds a bare command with captured streams and a scripted stdin.
func genCmd(stdin string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	c := &cobra.Command{}
	var out, errb bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errb)
	c.SetIn(strings.NewReader(stdin))
	return c, &out, &errb
}

// baseOpts is a valid, minimal invocation.
//
// It sets --no-wait because these cases are about the SPEND GATE — what reaches
// the submit seam and what does not. The wait/poll/download half has its own
// tests (generate_wait_test.go, generate_output_test.go) which wire the poll
// seam explicitly. Leaving noWait off here would make every spend-gate case
// depend on an unrelated poll seam.
func baseOpts() generateOpts {
	return generateOpts{
		prompt:  "a cat wearing sunglasses",
		baseURL: "https://civitai.com",
		noWait:  true,
		outDir:  ".",
	}
}

// --- the spend gate ----------------------------------------------------------

// 🔴 The headline safety property, WITH its own positive control.
//
// Case A asserts the submit seam is never reached in a non-interactive shell
// without --yes. On its own that assertion is worthless — a counter wired to
// nothing reads zero too. Case B drives the SAME counter, through the SAME deps
// constructor, to 1. Report the pair, never the zero alone.
func TestGenerate_NonTTYWithoutYesRefusesAndNeverSubmits(t *testing.T) {
	withStdinTTY(t, false)

	// (A) negative: no --yes on a non-TTY.
	var refuse genSeams
	c, _, errb := genCmd("")
	err := runGenerate(c, refuse.deps(t), baseOpts())
	if err == nil {
		t.Fatal("non-TTY without --yes: want a refusal, got nil")
	}
	if refuse.submitCalls != 0 {
		t.Errorf("non-TTY without --yes: submit called %d times, want 0", refuse.submitCalls)
	}
	// The refusal must name the three escapes so the message is actionable.
	for _, want := range []string{"--yes", "--max-cost", "--dry-run"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %s: %v", want, err)
		}
	}
	if strings.Contains(errb.String(), "[y/N]") {
		t.Error("non-TTY must not print a prompt")
	}

	// (B) POSITIVE CONTROL for the counter: identical wiring, --yes set.
	var proceed genSeams
	o := baseOpts()
	o.assumeYes = true
	c2, _, _ := genCmd("")
	if err := runGenerate(c2, proceed.deps(t), o); err != nil {
		t.Fatalf("positive control (--yes): %v", err)
	}
	if proceed.submitCalls != 1 {
		t.Fatalf("POSITIVE CONTROL FAILED: submit called %d times, want 1 — a 0 in case (A) proves nothing", proceed.submitCalls)
	}
}

// An interactive "n" cancels without submitting; the prompt is on STDERR so a
// piped stdout stays clean.
func TestGenerate_TTYDeclineCancels(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	c, out, errb := genCmd("n\n")
	err := runGenerate(c, s.deps(t), baseOpts())
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("declined prompt: err = %v, want a cancellation", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("declined prompt: submit called %d times, want 0", s.submitCalls)
	}
	if !strings.Contains(errb.String(), "[y/N]") {
		t.Errorf("confirmation prompt missing from stderr: %q", errb.String())
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("confirmation leaked to stdout: %q", out.String())
	}
}

// An interactive "y" proceeds and prints the workflow id on stdout.
func TestGenerate_TTYAcceptSubmits(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	c, out, _ := genCmd("y\n")
	if err := runGenerate(c, s.deps(t), baseOpts()); err != nil {
		t.Fatalf("accepted prompt: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("accepted prompt: submit called %d times, want 1", s.submitCalls)
	}
	if !strings.Contains(out.String(), "wf_123") {
		t.Errorf("workflow id missing from stdout: %q", out.String())
	}
}

// --max-cost below the estimate refuses locally, before any submit, and the exit
// code is pinned by errors.Is (never by the message).
func TestGenerate_MaxCostRefusesBelowEstimate(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	o := baseOpts()
	o.maxCost, o.maxCostSet = 5, true
	c, _, _ := genCmd("y\n")
	err := runGenerate(c, s.deps(t), o)
	if err == nil {
		t.Fatal("--max-cost below the estimate: want a refusal, got nil")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("--max-cost refusal: want ErrUsage (exit 2), got %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("--max-cost refusal: submit called %d times, want 0", s.submitCalls)
	}
}

// --max-cost at or above the estimate does not block.
func TestGenerate_MaxCostAllowsAtOrAboveEstimate(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	o := baseOpts()
	o.maxCost, o.maxCostSet, o.assumeYes = 12, true, true
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--max-cost == estimate: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("--max-cost == estimate: submit called %d times, want 1", s.submitCalls)
	}
}

// A cost above the balance stops before submitting, and is NOT an auth failure —
// a script must not be sent round the `civitai login` loop for being broke.
func TestGenerate_InsufficientBalanceStopsEarly(t *testing.T) {
	var s genSeams
	s.balance = func(context.Context) (int64, error) { return 3, nil }
	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, ErrInsufficientBuzz) {
		t.Fatalf("cost > balance: want ErrInsufficientBuzz, got %v", err)
	}
	if errors.Is(err, civitai.ErrUnauthorized) {
		t.Error("cost > balance must NOT classify as an auth failure (exit 3)")
	}
	if s.submitCalls != 0 {
		t.Errorf("cost > balance: submit called %d times, want 0", s.submitCalls)
	}
}

// An unreadable balance is advisory: warn, keep going, and never print a number
// (an unknown balance shown as 0 reads as "you are broke").
func TestGenerate_UnreadableBalanceWarnsAndContinues(t *testing.T) {
	var s genSeams
	s.balance = func(context.Context) (int64, error) { return 0, errors.New("boom") }
	o := baseOpts()
	o.assumeYes = true
	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("unreadable balance: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("unreadable balance: submit called %d times, want 1", s.submitCalls)
	}
	if !strings.Contains(errb.String(), "could not read your Buzz balance") {
		t.Errorf("missing balance warning on stderr: %q", errb.String())
	}
}

// --- dry run -----------------------------------------------------------------

func TestGenerate_DryRunNeverSubmits(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	o := baseOpts()
	o.dryRun = true
	c, out, _ := genCmd("y\n")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run: %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("--dry-run: submit called %d times, want 0", s.submitCalls)
	}
	if s.balanceCalls != 0 {
		t.Errorf("--dry-run must not need the balance, called %d times", s.balanceCalls)
	}
	for _, want := range []string{"Estimated cost", "total", "quantity"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--dry-run output missing %q:\n%s", want, out.String())
		}
	}
}

// --dry-run --json passes the SERVER's payload through on stdout, verbatim —
// including fields this CLI does not model — and prints nothing else there.
func TestGenerate_DryRunJSONEmitsRawPayloadOnStdout(t *testing.T) {
	var s genSeams
	o := baseOpts()
	o.dryRun, o.jsonOut = true, true
	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run --json: %v", err)
	}
	if s.submitCalls != 0 {
		t.Fatalf("--dry-run --json: submit called %d times, want 0", s.submitCalls)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout is not pure JSON (%v):\n%s", err, out.String())
	}
	if m["serverOnlyField"] != "kept" {
		t.Errorf("raw passthrough dropped an unmodelled server field: %v", m)
	}
}

// --- payload shape -----------------------------------------------------------

// 🔴 An unset flag must be ABSENT from the JSON, not a Go zero value: the server
// accepts quantity 0 and silently clamps it rather than erroring. Asserted on a
// decoded map, never with strings.Contains (which cross-matches prefixes).
func TestGenerate_UnsetFlagsAreAbsentFromThePayload(t *testing.T) {
	var s genSeams
	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	raw, err := json.Marshal(s.lastGraph)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"quantity", "negativePrompt", "aspectRatio", "model", "resources", "steps", "cfgScale", "sampler", "seed"} {
		if _, ok := m[key]; ok {
			t.Errorf("unset flag produced key %q in the payload: %s", key, raw)
		}
	}
	// Positive control: a key that IS set must be present, or the absence
	// assertions above could pass against an empty object.
	if m["prompt"] != "a cat wearing sunglasses" {
		t.Fatalf("POSITIVE CONTROL FAILED: prompt missing from payload: %s", raw)
	}
	if m["workflow"] != generateWorkflow {
		t.Fatalf("POSITIVE CONTROL FAILED: workflow missing from payload: %s", raw)
	}
}

// Set flags land in the payload with the resolved resource shape.
func TestGenerate_SetFlagsArePresent(t *testing.T) {
	var s genSeams
	s.resolve = func(_ context.Context, id int) (*genapi.ResolvedVersion, error) {
		typ := "Checkpoint"
		if id == 250712 {
			typ = "LORA"
		}
		return &genapi.ResolvedVersion{VersionID: id, ModelName: "M", VersionName: "v1", ModelType: typ}, nil
	}
	o := baseOpts()
	o.assumeYes = true
	o.quantity, o.quantitySet = 4, true
	o.negativePrompt = "blurry"
	o.aspectRatio = "1:1"
	o.checkpoint, o.checkpointSet = 128713, true
	o.loras = []string{"250712:0.8"}
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	raw, _ := json.Marshal(s.lastGraph)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["quantity"] != float64(4) || m["negativePrompt"] != "blurry" || m["aspectRatio"] != "1:1" {
		t.Errorf("set flags missing/wrong: %s", raw)
	}
	res, ok := m["resources"].([]any)
	if !ok || len(res) != 1 {
		t.Fatalf("resources missing: %s", raw)
	}
	entry := res[0].(map[string]any)
	if entry["id"] != float64(250712) || entry["strength"] != 0.8 {
		t.Errorf("lora entry wrong: %s", raw)
	}
	// 🔴 model:{type} is REQUIRED on a resources[] entry — a bare {id} is a 400.
	model, ok := entry["model"].(map[string]any)
	if !ok || model["type"] != "LORA" {
		t.Errorf("resource entry lacks the required model.type: %s", raw)
	}
}

// --- --lora parsing ----------------------------------------------------------

func TestParseLoraFlag(t *testing.T) {
	strengthOf := func(t *testing.T, spec loraSpec) float64 {
		t.Helper()
		if spec.strength == nil {
			t.Fatal("expected a strength, got nil")
		}
		return *spec.strength
	}

	spec, err := parseLoraFlag("250712")
	if err != nil {
		t.Fatalf("bare id: %v", err)
	}
	if spec.versionID != 250712 {
		t.Errorf("bare id: got %d", spec.versionID)
	}
	if spec.strength != nil {
		// Unset strength must stay ABSENT so the server applies its own default.
		t.Errorf("bare id must not invent a strength, got %v", *spec.strength)
	}

	spec, err = parseLoraFlag("123:0.8")
	if err != nil {
		t.Fatalf("id:strength: %v", err)
	}
	if spec.versionID != 123 || strengthOf(t, spec) != 0.8 {
		t.Errorf("id:strength parsed wrong: %+v", spec)
	}

	if spec, err := parseLoraFlag(" 123 : -0.5 "); err != nil || strengthOf(t, spec) != -0.5 {
		// Negative strengths are meaningful to the generator; whitespace is not.
		t.Errorf("whitespace/negative: spec=%+v err=%v", spec, err)
	}

	for _, bad := range []string{"", "  ", "abc", "123:", ":0.8", "123:abc", "123:0.8:0.9", "0", "-1", "1e3"} {
		if _, err := parseLoraFlag(bad); err == nil {
			t.Errorf("--lora %q: want a usage error, got nil", bad)
		} else if !errors.Is(err, ErrUsage) {
			t.Errorf("--lora %q: want ErrUsage, got %v", bad, err)
		}
	}
}

// A nonexistent version id must fail LOCALLY, before the estimate and before any
// submit — that is the whole point of resolving it (the generator would have
// accepted it, substituted a default model, and billed for it).
func TestGenerate_NonexistentVersionIDFailsBeforeAnySubmit(t *testing.T) {
	var s genSeams
	s.resolve = func(context.Context, int) (*genapi.ResolvedVersion, error) {
		return nil, civitai.Tag(civitai.ErrNotFound, errors.New("model version 999999 not found"))
	}
	o := baseOpts()
	o.assumeYes = true
	o.checkpoint, o.checkpointSet = 999999, true
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, civitai.ErrNotFound) {
		t.Fatalf("nonexistent checkpoint: want ErrNotFound (exit 4), got %v", err)
	}
	if s.submitCalls != 0 || s.whatIfCalls != 0 {
		t.Errorf("nonexistent checkpoint: submit=%d whatIf=%d, want 0/0", s.submitCalls, s.whatIfCalls)
	}
}

// --- flag validation ---------------------------------------------------------

func TestGenerate_UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*generateOpts)
	}{
		{"no prompt", func(o *generateOpts) { o.prompt = "" }},
		{"blank prompt", func(o *generateOpts) { o.prompt = "   " }},
		{"quantity 0", func(o *generateOpts) { o.quantity, o.quantitySet = 0, true }},
		{"negative quantity", func(o *generateOpts) { o.quantity, o.quantitySet = -3, true }},
		{"checkpoint 0", func(o *generateOpts) { o.checkpoint, o.checkpointSet = 0, true }},
		{"negative max-cost", func(o *generateOpts) { o.maxCost, o.maxCostSet = -1, true }},
		{"bad lora", func(o *generateOpts) { o.loras = []string{"nope"} }},
		{"json submit without yes", func(o *generateOpts) { o.jsonOut = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s genSeams
			o := baseOpts()
			tc.mut(&o)
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), o)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("want ErrUsage (exit 2), got %v", err)
			}
			if s.whatIfCalls != 0 || s.submitCalls != 0 || s.resolveCalls != 0 {
				t.Errorf("a usage error must not touch the network: whatIf=%d submit=%d resolve=%d",
					s.whatIfCalls, s.submitCalls, s.resolveCalls)
			}
		})
	}
}

// A quantity above the server's clamp WARNS (money silently disappears
// otherwise) but never blocks.
func TestGenerate_OverClampQuantityWarnsButProceeds(t *testing.T) {
	var s genSeams
	o := baseOpts()
	o.assumeYes = true
	o.quantity, o.quantitySet = 40, true
	c, _, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("over-clamp quantity: %v", err)
	}
	if s.submitCalls != 1 {
		t.Errorf("over-clamp quantity must not block: submit=%d", s.submitCalls)
	}
	if !strings.Contains(errb.String(), "CLAMPS silently") {
		t.Errorf("missing clamp warning: %q", errb.String())
	}
}

// A `ready:false` estimate is refused rather than submitted.
func TestGenerate_NotReadyRefuses(t *testing.T) {
	var s genSeams
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q := okQuote(12)
		q.Ready = false
		return q, okQuoteRaw(12), nil
	}
	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	err := runGenerate(c, s.deps(t), o)
	if !errors.Is(err, civitai.ErrBadRequest) {
		t.Fatalf("ready:false: want ErrBadRequest, got %v", err)
	}
	if s.submitCalls != 0 {
		t.Errorf("ready:false: submit called %d times, want 0", s.submitCalls)
	}
}

// A missing cost must never render as free.
func TestGenerate_MissingCostRefuses(t *testing.T) {
	var s genSeams
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		return &genapi.WhatIfResult{Ready: true}, json.RawMessage(`{"ready":true}`), nil
	}
	o := baseOpts()
	o.assumeYes = true
	c, _, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err == nil {
		t.Fatal("cost-less estimate: want a refusal, got nil")
	}
	if s.submitCalls != 0 {
		t.Errorf("cost-less estimate: submit called %d times, want 0", s.submitCalls)
	}
}

// --- exit-code rows (design §7), driven through a real tRPC error envelope ----

// trpcErrServer answers every request with the tRPC error envelope shape the
// platform really emits, at the given HTTP status.
func trpcErrServer(t *testing.T, status int, message, code string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body, _ := json.Marshal(map[string]any{
			"error": map[string]any{"json": map[string]any{"message": message, "code": code}},
		})
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 🔴 Every row asserts errors.Is, never message text — the exit code is the
// contract, and a test on the message says nothing about it (a classification
// can be stripped while every message stays byte-identical).
//
// The statuses here are the ones the CLI actually observes: tRPC derives the
// HTTP status from the TRPCError CODE, so an upstream orchestrator 403 for
// insufficient funds reaches us as a 400, not a 403.
func TestGenerate_ErrorRowsClassification(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		code    string
		message string
		want    error
		notWant []error
	}{
		{
			name: "missing scope stays an auth failure", status: http.StatusForbidden, code: "FORBIDDEN",
			message: "Your API key does not have the required scope for this action",
			want:    civitai.ErrUnauthorized,
		},
		{
			name: "unauthenticated", status: http.StatusUnauthorized, code: "UNAUTHORIZED",
			message: "UNAUTHORIZED", want: civitai.ErrUnauthorized,
		},
		{
			name: "muted account is NOT an auth failure", status: http.StatusForbidden, code: "FORBIDDEN",
			message: "You cannot perform this action because your account has been restricted",
			want:    ErrAccountRestricted, notWant: []error{civitai.ErrUnauthorized},
		},
		{
			name: "onboarding incomplete is NOT an auth failure", status: http.StatusForbidden, code: "FORBIDDEN",
			message: "You must complete the onboarding process before performing this action",
			want:    ErrAccountRestricted, notWant: []error{civitai.ErrUnauthorized},
		},
		{
			name: "insufficient funds (default text)", status: http.StatusBadRequest, code: "BAD_REQUEST",
			message: "Hey buddy, seems like you don't have enough funds to perform this action.",
			want:    ErrInsufficientBuzz, notWant: []error{civitai.ErrUnauthorized, civitai.ErrBadRequest, ErrUsage},
		},
		{
			name: "insufficient funds arriving as a 403", status: http.StatusForbidden, code: "FORBIDDEN",
			message: "Insufficient funds",
			want:    ErrInsufficientBuzz, notWant: []error{civitai.ErrUnauthorized},
		},
		{
			name: "generation disabled", status: http.StatusBadRequest, code: "BAD_REQUEST",
			message: "Generation is currently disabled",
			want:    ErrGenerationDisabled, notWant: []error{civitai.ErrUnauthorized, ErrUsage},
		},
		{
			name: "prompt blocked", status: http.StatusBadRequest, code: "BAD_REQUEST",
			message: "Your prompt was flagged: minor",
			want:    ErrPromptBlocked, notWant: []error{civitai.ErrUnauthorized, ErrUsage},
		},
		{
			// DECISION: a resource that resolved fine but is not generatable stays
			// ErrBadRequest -> exit 2, NOT ErrNotFound -> exit 4. Exit 4 on this
			// command already means "no such version id", produced locally by the
			// --checkpoint/--lora resolution; conflating the two would make exit 4
			// unactionable.
			name: "resource not generatable", status: http.StatusBadRequest, code: "BAD_REQUEST",
			message: "Some Model is not enabled for generation. Please contact support",
			want:    civitai.ErrBadRequest, notWant: []error{civitai.ErrNotFound, civitai.ErrUnauthorized},
		},
		{
			name: "unknown ecosystem is a usage mistake", status: http.StatusInternalServerError, code: "INTERNAL_SERVER_ERROR",
			message: "Unknown ecosystem: SDXLL",
			want:    ErrUsage,
		},
		{
			name: "rate limited", status: http.StatusTooManyRequests, code: "TOO_MANY_REQUESTS",
			message: "Slow down!", want: civitai.ErrRateLimited,
		},
		{
			name: "service unavailable", status: http.StatusServiceUnavailable, code: "INTERNAL_SERVER_ERROR",
			message: "The orchestrator is unavailable", want: civitai.ErrNetwork,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := trpcErrServer(t, tc.status, tc.message, tc.code)
			client := genapi.New(srv.URL, "test-token")

			var s genSeams
			s.whatIf = client.WhatIfFromGraph
			o := baseOpts()
			o.assumeYes = true
			o.baseURL = srv.URL
			c, _, _ := genCmd("")
			err := runGenerate(c, s.deps(t), o)
			if err == nil {
				t.Fatalf("%s: want an error, got nil", tc.name)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("%s: errors.Is(err, %v) = false; err = %v", tc.name, tc.want, err)
			}
			for _, nw := range tc.notWant {
				if errors.Is(err, nw) {
					t.Errorf("%s: err must NOT classify as %v; err = %v", tc.name, nw, err)
				}
			}
			if s.submitCalls != 0 {
				t.Errorf("%s: submit called %d times after an estimate failure, want 0", tc.name, s.submitCalls)
			}
		})
	}
}

// The same re-classification must apply on the SUBMIT seam, not just the
// estimate — that is the path where the money already moved.
func TestGenerate_SubmitErrorIsClassifiedToo(t *testing.T) {
	srv := trpcErrServer(t, http.StatusForbidden, "You cannot perform this action because your account has been restricted", "FORBIDDEN")
	client := genapi.New(srv.URL, "test-token")

	var s genSeams
	s.submitErr = nil
	deps := s.deps(t)
	// Point ONLY the submit seam at the failing server; the estimate succeeds.
	deps.submit = client.GenerateFromGraph

	o := baseOpts()
	o.assumeYes = true
	o.baseURL = srv.URL
	c, _, _ := genCmd("")
	err := runGenerate(c, deps, o)
	if !errors.Is(err, ErrAccountRestricted) {
		t.Fatalf("submit-path muted account: want ErrAccountRestricted, got %v", err)
	}
	if errors.Is(err, civitai.ErrUnauthorized) {
		t.Error("submit-path muted account must NOT classify as an auth failure (exit 3)")
	}
}

// An unrecognised server message must keep its status-derived classification —
// the classifier fails soft, so a server-side rewording degrades to today's
// behaviour rather than to a wrong kind.
func TestClassifyGenerateError_UnknownMessageKeepsStatusKind(t *testing.T) {
	srv := trpcErrServer(t, http.StatusForbidden, "something nobody has ever seen", "FORBIDDEN")
	client := genapi.New(srv.URL, "test-token")
	_, _, err := client.WhatIfFromGraph(context.Background(), genapi.Graph{})
	got := classifyGenerateError(err)
	if !errors.Is(got, civitai.ErrUnauthorized) {
		t.Fatalf("unrecognised 403: want the status-derived ErrUnauthorized, got %v", got)
	}
}

// A non-APIError passes through untouched.
func TestClassifyGenerateError_PassesThroughForeignErrors(t *testing.T) {
	in := civitai.Tag(civitai.ErrNotFound, errors.New("model version 1 not found"))
	if got := classifyGenerateError(in); got != in {
		t.Fatalf("foreign error was rewritten: %v", got)
	}
}

// 🔴 The 403 message this CLI composes itself lists every cause a 403 can have.
// A classifier matching against err.Error() instead of the SERVER's message
// would therefore label every 403 as "insufficient Buzz". Pin that it doesn't.
func TestClassifyGenerateError_DoesNotMatchOurOwnAdvisoryText(t *testing.T) {
	srv := trpcErrServer(t, http.StatusForbidden, "Your API key does not have the required scope for this action", "FORBIDDEN")
	client := genapi.New(srv.URL, "test-token")
	_, _, err := client.WhatIfFromGraph(context.Background(), genapi.Graph{})
	if !strings.Contains(err.Error(), "insufficient Buzz") {
		t.Skip("the transport's 403 advisory no longer mentions insufficient Buzz — this trap is gone")
	}
	got := classifyGenerateError(err)
	if errors.Is(got, ErrInsufficientBuzz) {
		t.Fatal("classified a scope error as insufficient Buzz — the classifier is reading its own advisory text, not the server's message")
	}
}

// --- command wiring ----------------------------------------------------------

// 🔴 `generate` must stay a LEAF. root.go only installs the unknown-subcommand
// guard on non-runnable parents, so a subcommand here would let `civitai
// generate lst` fall through to the runnable command with "lst" as the PROMPT —
// and charge for it.
func TestGenerateCommandHasNoSubcommands(t *testing.T) {
	c := newGenerateCmd()
	if c.HasSubCommands() {
		t.Fatalf("generate must have no subcommands, got %d", len(c.Commands()))
	}
	if !c.Runnable() {
		t.Fatal("generate must be runnable")
	}
}

// The five content flags, and only those five.
func TestGenerateCommandFlagSurface(t *testing.T) {
	c := newGenerateCmd()
	for _, name := range []string{"negative-prompt", "quantity", "aspect-ratio", "checkpoint", "lora", "dry-run", "json", "yes", "max-cost"} {
		if c.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	// These are deliberately absent: silently-degrading parameters, and --model,
	// which means a MODEL id on `civitai download` while this takes a VERSION id.
	for _, name := range []string{"model", "steps", "cfg-scale", "sampler", "priority", "clip-skip"} {
		if c.Flags().Lookup(name) != nil {
			t.Errorf("flag --%s must not exist on generate", name)
		}
	}
	if c.Flags().ShorthandLookup("n") != nil {
		t.Error("-n must not be a shorthand next to a money prompt")
	}
}

// --max-cost must describe itself as an estimate check, never as a cap.
func TestGenerateMaxCostHelpIsHonest(t *testing.T) {
	c := newGenerateCmd()
	usage := c.Flags().Lookup("max-cost").Usage
	if !strings.Contains(usage, "NOT a spending cap") {
		t.Errorf("--max-cost help must say it is not a cap: %q", usage)
	}
	for _, want := range []string{"SPENDS REAL BUZZ", "personal API key", "not a quote", "estimate check, NOT a spending cap"} {
		if !strings.Contains(c.Long, want) && !strings.Contains(strings.ToUpper(c.Long), strings.ToUpper(want)) {
			t.Errorf("Long is missing the safety statement %q", want)
		}
	}
}

// The command is registered on the root.
func TestGenerateRegisteredOnRoot(t *testing.T) {
	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "generate" {
			return
		}
	}
	t.Fatal("generate is not registered on the root command")
}

// Server-origin strings reach the terminal sanitised.
func TestGenerate_SanitisesServerStrings(t *testing.T) {
	var s genSeams
	s.resolve = func(_ context.Context, id int) (*genapi.ResolvedVersion, error) {
		return &genapi.ResolvedVersion{VersionID: id, ModelName: "evil\x1b[2Kname", ModelType: "Checkpoint"}, nil
	}
	o := baseOpts()
	o.dryRun = true
	o.checkpoint, o.checkpointSet = 1, true
	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run: %v", err)
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Errorf("unsanitised escape reached stdout: %q", out.String())
	}
	// safeTerm removes the ESC introducer (which is what makes the sequence
	// executable), not the remaining printable bytes — so the name survives as
	// inert text rather than disappearing.
	if !strings.Contains(out.String(), "evil[2Kname") {
		t.Errorf("sanitised model name missing: %q", out.String())
	}
}
