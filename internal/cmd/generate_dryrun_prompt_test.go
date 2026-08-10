package cmd

import (
	"strings"
	"testing"
)

// Issue #281 — `--dry-run` structurally CANNOT detect a moderation refusal, and
// it used to echo the prompt back as though it had checked it.
//
// The mechanism: `genapi.whatIfGraph` deletes `prompt` / `negativePrompt`
// before the cost estimate goes out (pinned wire-side by
// `TestWhatIf_StripsPrompts` and `TestWhatIf_StripsPromptsFromRawGraphOnly` in
// internal/genapi). That strip is deliberate and correct — the fields do not
// affect cost and the server substitutes its own defaults — so the repair had
// to happen at the RENDERER, not at the strip.
//
// A sentinel prompt is used rather than a realistic one on purpose: "a cat"
// appears in the shared `baseOpts()` fixture and in a dozen other files, so an
// absence assertion over it could pass or fail for reasons that have nothing to
// do with this renderer.
const dryRunPromptSentinel = "zz-sentinel-prompt-8f3a2b"

const dryRunNegativeSentinel = "zz-sentinel-negative-4d1c90"

// fieldValue returns the value column of the first tabwriter row whose label is
// `label`, or "" if no such row was rendered.
//
// It reads the rendered STATE of that row rather than searching the whole blob
// for a substring: `strings.Contains(out, "<not checked by --dry-run>")` stays
// green when the placeholder AND the real prompt are both printed, which is
// exactly the half-applied fix this test has to be able to see.
func fieldValue(rendered, label string) string {
	for _, line := range strings.Split(rendered, "\n") {
		if !strings.HasPrefix(line, label+":") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, label+":"))
	}
	return ""
}

// 🔴 The defect. `--dry-run` must not present the prompt as something it
// evaluated, on EITHER stream.
func TestGenerateDryRun_DoesNotEchoTheUncheckedPrompt(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	o := baseOpts()
	o.dryRun = true
	o.prompt = dryRunPromptSentinel
	o.negativePrompt = dryRunNegativeSentinel

	c, out, errb := genCmd("y\n")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run: %v", err)
	}
	if s.submitCalls != 0 {
		t.Fatalf("--dry-run: submit called %d times, want 0", s.submitCalls)
	}

	// (1) The sentinel must not reach the user on either stream. The zero here is
	// MEASURED, not assumed: TestGenerateConfirm_StillEchoesThePromptBeforeSpend
	// below drives the same sentinel through the same renderer stack and finds it.
	if strings.Contains(out.String(), dryRunPromptSentinel) {
		t.Errorf("--dry-run echoed the prompt on stdout; the estimate never carried it:\n%s", out.String())
	}
	if strings.Contains(errb.String(), dryRunPromptSentinel) {
		t.Errorf("--dry-run echoed the prompt on stderr; the estimate never carried it:\n%s", errb.String())
	}
	if strings.Contains(out.String(), dryRunNegativeSentinel) || strings.Contains(errb.String(), dryRunNegativeSentinel) {
		t.Errorf("--dry-run echoed the negative prompt; `whatIfGraph` strips it too:\nstdout:\n%s\nstderr:\n%s", out.String(), errb.String())
	}

	// (2) The rows are still rendered, and their VALUE is the label. A row that
	// vanished would be its own defect: the user could not tell whether
	// --negative-prompt registered at all.
	for _, label := range []string{"Prompt", "Negative prompt"} {
		got := fieldValue(out.String(), label)
		if got == "" {
			t.Errorf("--dry-run dropped the %q row entirely:\n%s", label, out.String())
			continue
		}
		if got != promptNotChecked {
			t.Errorf("%q row = %q, want %q", label, got, promptNotChecked)
		}
	}

	// (3) The label alone is cryptic; the reason must be on stderr, and it must
	// name the failure the user actually hits (a refusal at submit).
	for _, want := range []string{"not sent with the estimate", "moderation", "refused"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("--dry-run stderr caveat missing %q:\n%s", want, errb.String())
		}
	}

	// (4) The CLI must keep SENDING the prompt — the strip is genapi's job, and
	// this change is presentational only. If the graph handed to the estimate
	// seam had lost the prompt, the submit that follows a real run would lose it
	// too, which would be a far worse bug than the one being fixed.
	if s.lastGraph.Prompt != dryRunPromptSentinel {
		t.Errorf("graph handed to the whatIf seam lost the prompt: %q", s.lastGraph.Prompt)
	}
	if s.lastGraph.NegativePrompt != dryRunNegativeSentinel {
		t.Errorf("graph handed to the whatIf seam lost the negative prompt: %q", s.lastGraph.NegativePrompt)
	}
}

// 🔴 POSITIVE CONTROL, and the trap this fix walks past.
//
// There are TWO prompt echoes in generate.go. `printGenerateQuote` (above) is
// the defect; `confirmGenerate` is not — it is the screen the user reads
// immediately before an irreversible spend, on a graph that really does carry
// the prompt. Silencing that one would gut the money confirmation, and every
// other test in this package would stay green.
//
// This case is also what makes the absence assertions above meaningful: same
// sentinel, same renderer stack, same buffers — found here, so a zero there is
// a measurement rather than a harness wired to nothing.
func TestGenerateConfirm_StillEchoesThePromptBeforeSpend(t *testing.T) {
	withStdinTTY(t, true)
	var s genSeams
	o := baseOpts()
	o.prompt = dryRunPromptSentinel
	o.negativePrompt = dryRunNegativeSentinel

	c, _, errb := genCmd("y\n")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("interactive confirm: %v", err)
	}
	if s.submitCalls != 1 {
		t.Fatalf("interactive confirm: submit called %d times, want 1", s.submitCalls)
	}

	if !strings.Contains(errb.String(), "[y/N]") {
		t.Fatalf("this case must exercise the interactive confirmation, not --yes:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), dryRunPromptSentinel) {
		t.Errorf("the spend confirmation must show the prompt being paid for:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), dryRunNegativeSentinel) {
		t.Errorf("the spend confirmation must show the negative prompt being paid for:\n%s", errb.String())
	}
	if strings.Contains(errb.String(), promptNotChecked) {
		t.Errorf("the spend confirmation must never print %q — the submit DOES carry the prompt:\n%s", promptNotChecked, errb.String())
	}
}

// The `--input` branch never echoed a prompt (the CLI has not interpreted the
// graph), so it must keep naming the file — the fix must not collapse the two
// branches into one placeholder and lose which file was priced.
func TestGenerateDryRun_InputBranchStillNamesTheFile(t *testing.T) {
	withStdinTTY(t, true)
	path := writeGraphFile(t, `{"workflow":"txt2img","prompt":"`+dryRunPromptSentinel+`"}`)

	var s genSeams
	o := inputOpts(path)
	o.dryRun = true
	o.assumeYes = false

	c, out, errb := genCmd("y\n")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run --input: %v", err)
	}
	if got := fieldValue(out.String(), "Graph"); !strings.Contains(got, "graph.json") {
		t.Errorf("--input dry-run must name the graph file, got %q:\n%s", got, out.String())
	}
	if v := fieldValue(out.String(), "Prompt"); v != "" {
		t.Errorf("--input dry-run must not render a Prompt row (the CLI did not interpret the graph), got %q", v)
	}
	// The prompt inside the file is stripped for the estimate exactly as a typed
	// one is, so it must not surface here either.
	if strings.Contains(out.String(), dryRunPromptSentinel) || strings.Contains(errb.String(), dryRunPromptSentinel) {
		t.Errorf("--input dry-run leaked the file's prompt:\nstdout:\n%s\nstderr:\n%s", out.String(), errb.String())
	}
}
