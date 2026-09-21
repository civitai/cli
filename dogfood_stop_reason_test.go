package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The TERMINAL-STATE path of the blind dogfood harness (scripts/dogfood).
//
// 🔴 THE DEFECT THESE EXIST FOR, MEASURED. Trial `ab-genpost-glm-01`
// (z-ai/glm-5.3-flash) ended on an assistant message with `content: null`, no
// tool calls, and `completion_tokens: 8000` — exactly the `max_tokens` the
// harness sent — of which 7,992 were reasoning. The model had exhausted its
// output budget inside its reasoning channel and returned nothing. runner.py
// branched on `if not calls: stop = "finished"`, and `finish_reason` appeared
// nowhere in runner.py, grade.sh or oracle.sh. So a cell that hit a HARNESS
// LIMIT was recorded byte-identically to a cell that completed with an empty
// report — a harness limit reported as a task outcome, which is the exact
// confound this arc exists not to introduce.
//
// Everything here runs offline through testdata/fake_trial.py: no Docker, no
// OpenRouter, no money, no account.

// The recorded response body that started this. Replayed verbatim rather than
// rebuilt, so the test cannot quietly disagree with the artifact.
func truncatedFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join(dogfoodDir, "testdata", "truncated_final_turn.json")
}

// The last JSON object the runner printed: its one-line run summary.
func runSummary(t *testing.T, stdout string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var m map[string]any
		if json.Unmarshal([]byte(lines[i]), &m) == nil && m["trial"] != nil {
			return m
		}
	}
	t.Fatalf("no run summary on stdout:\n%s", stdout)
	return nil
}

// The `end` record of a transcript.
func endRecord(t *testing.T, transcript string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(transcript), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		if r["kind"] == "end" {
			return r
		}
	}
	t.Fatalf("no end record in:\n%s", transcript)
	return nil
}

// Every `assistant` record, in order.
func assistantRecords(t *testing.T, transcript string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(transcript), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("transcript line is not JSON: %q", line)
		}
		if r["kind"] == "assistant" {
			out = append(out, r)
		}
	}
	return out
}

// ── the fixture, and the control on it ───────────────────────────────────────

// 🔴 A POSITIVE CONTROL ON THE FIXTURE ITSELF, BEFORE ANY VERDICT IS READ OFF
// IT. The test below asserts that this exact body classifies as `truncated`.
// If the fixture ever stopped BEING a truncation — someone trims the usage
// block, or edits `completion_tokens` — that assertion would still pass while
// measuring nothing about budget exhaustion. So pin the two facts that make it
// one: the reply is empty, and the completion tokens are the whole budget, all
// but 8 of them reasoning.
func TestTruncatedFixtureIsARealBudgetExhaustion(t *testing.T) {
	raw, err := os.ReadFile(truncatedFixture(t))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	var body struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   *string `json:"content"`
				ToolCalls []any   `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
			Details          struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("the fixture is not a response body: %v", err)
	}
	if len(body.Choices) != 1 {
		t.Fatalf("the fixture has %d choices, want 1", len(body.Choices))
	}
	c := body.Choices[0]
	if c.Message.Content != nil || len(c.Message.ToolCalls) != 0 {
		t.Fatalf("the fixture is not the empty terminal turn it claims to be: content=%v tool_calls=%v",
			c.Message.Content, c.Message.ToolCalls)
	}
	if c.FinishReason != "length" {
		t.Fatalf("the fixture's finish_reason is %q, want \"length\" — it is supposed to be "+
			"the budget-exhausted turn", c.FinishReason)
	}
	// 8000 is the max_tokens the harness sent on the run that produced it. The
	// reply got the 8 tokens that were left.
	if body.Usage.CompletionTokens != 8000 || body.Usage.Details.ReasoningTokens != 7992 {
		t.Fatalf("the fixture's usage is completion=%d reasoning=%d, want 8000/7992 — the "+
			"verbatim numbers from ab-genpost-glm-01. Without them this is no longer "+
			"evidence of a budget exhaustion and every assertion keyed on it is vacuous.",
			body.Usage.CompletionTokens, body.Usage.Details.ReasoningTokens)
	}
}

// 🔴 THE REGRESSION TEST. Red on origin/main, where runner.py records
// `stop: "finished"` for this body; green here. Driven from the recorded bytes
// through the real runner, not from a hand-built dict through classify_stop.
func TestDogfoodTruncatedTurnIsNotRecordedAsFinished(t *testing.T) {
	tr := runFakeTrialEnv(t,
		[]string{"FAKE_FINAL_RESPONSE=" + truncatedFixture(t)},
		nil, "")

	end := endRecord(t, readFile(t, tr.transcript))
	if got := end["stop"]; got != "truncated" {
		t.Fatalf("a turn that exhausted its output budget was recorded stop=%q, want \"truncated\".\n"+
			"This is the ab-genpost-glm-01 defect: the harness cannot tell a cell that ran "+
			"out of tokens from one that finished with an empty report.\n%s",
			got, readFile(t, tr.transcript))
	}
	if got := end["finish_reason"]; got != "length" {
		t.Fatalf("the end record's finish_reason is %v, want \"length\" — the provider sent it "+
			"and the harness must keep it", got)
	}
	// The same field on the turn that produced it, so a transcript reader can
	// attribute the stop to a turn rather than inferring it from the last one.
	recs := assistantRecords(t, readFile(t, tr.transcript))
	if len(recs) != 1 {
		t.Fatalf("want 1 assistant record, got %d", len(recs))
	}
	if got := recs[0]["finish_reason"]; got != "length" {
		t.Fatalf("the assistant record's finish_reason is %v, want \"length\"", got)
	}
	// And the summary an operator actually reads.
	s := runSummary(t, tr.stdout)
	if s["stop"] != "truncated" || s["finish_reason"] != "length" {
		t.Fatalf("the run summary hides the truncation: stop=%v finish_reason=%v\n%s",
			s["stop"], s["finish_reason"], tr.stdout)
	}
}

// ── the vocabulary ───────────────────────────────────────────────────────────

// 🔴 THE THREE CASES MUST BE DISTINGUISHABLE, AND THE UNKNOWN ONE MUST NOT BE
// `finished`. `finished` is a POSITIVE claim that the provider said the model
// chose to stop; the harness may only make it when the provider actually did.
// An absent or unrecognised `finish_reason` is exactly the shape of the defect
// that produced this test, one provider vocabulary later, so it gets its own
// loud value carrying the raw string.
func TestDogfoodTerminalStatesAreDistinguishable(t *testing.T) {
	for _, tc := range []struct {
		name         string
		finishReason string
		content      string
		wantStop     string
	}{
		{"budget exhausted", "length", "__null__", "truncated"},
		// Truncation outranks content: a `length` finish with prose is still a
		// report cut off mid-sentence, and grading it `finished` is the same
		// confound in a quieter form.
		{"budget exhausted mid-sentence", "length", "I was in the middle of", "truncated"},
		{"a provider-native budget spelling", "MAX_TOKENS", "__null__", "truncated"},
		{"a real report", "stop", "here is what I did", "finished"},
		{"stopped with an empty reply", "stop", "", "empty-reply"},
		{"stopped with a null reply", "stop", "__null__", "empty-reply"},
		{"whitespace is not a report", "stop", "   \n  ", "empty-reply"},
		{"an alternative natural spelling", "end_turn", "done", "finished"},
		{"no finish_reason at all", "__absent__", "here is what I did", "stopped-unknown:none"},
		{"a vocabulary we have not met", "content_filter", "", "stopped-unknown:content_filter"},
		{"tool_calls with no tool calls", "tool_calls", "", "stopped-unknown:tool_calls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := runFakeTrialEnv(t, []string{
				"FAKE_FINISH_REASON=" + tc.finishReason,
				"FAKE_FINAL_CONTENT=" + tc.content,
			}, nil, "")
			if got := endRecord(t, readFile(t, tr.transcript))["stop"]; got != tc.wantStop {
				t.Fatalf("finish_reason=%q content=%q recorded stop=%q, want %q\n%s",
					tc.finishReason, tc.content, got, tc.wantStop, readFile(t, tr.transcript))
			}
		})
	}
}

// The same claim stated as the thing that must never happen, so a future
// widening of the natural-stop set cannot quietly re-absorb the unknown case:
// no unrecognised or absent finish_reason may produce `finished`, whatever
// else it produces.
func TestDogfoodUnknownFinishReasonIsNeverFinished(t *testing.T) {
	for _, fr := range []string{"__absent__", "content_filter", "error", "ERROR", "unexpected-new-value"} {
		t.Run(fr, func(t *testing.T) {
			tr := runFakeTrialEnv(t, []string{
				"FAKE_FINISH_REASON=" + fr,
				"FAKE_FINAL_CONTENT=a plausible looking report",
			}, nil, "")
			got, _ := endRecord(t, readFile(t, tr.transcript))["stop"].(string)
			if got == "finished" {
				t.Fatalf("finish_reason=%q collapsed into \"finished\" — the harness asserted the "+
					"model chose to stop on evidence it does not have", fr)
			}
			if !strings.HasPrefix(got, "stopped-unknown:") {
				t.Fatalf("finish_reason=%q recorded stop=%q, want a stopped-unknown:* value that "+
					"names the raw string", fr, got)
			}
		})
	}
}

// ── carrying reasoning back ──────────────────────────────────────────────────

// The assistant messages the runner SENT, turn by turn, read out of the
// captured request payloads rather than out of runner.py.
func sentAssistantMessages(t *testing.T, capture string) []map[string]any {
	t.Helper()
	var c struct {
		Payloads []struct {
			Messages []map[string]any `json:"messages"`
		} `json:"payloads"`
	}
	if err := json.Unmarshal([]byte(readFile(t, capture)), &c); err != nil {
		t.Fatalf("reading the capture: %v", err)
	}
	if len(c.Payloads) < 2 {
		t.Fatalf("only %d payload(s) captured — the trial never took a second turn, so there "+
			"is no history to inspect", len(c.Payloads))
	}
	var out []map[string]any
	for _, m := range c.Payloads[len(c.Payloads)-1].Messages {
		if m["role"] == "assistant" {
			out = append(out, m)
		}
	}
	return out
}

// 🔴 87.8% OF THE MODEL'S OUTPUT WAS BEING THROWN AWAY EVERY TURN. runner.py
// appended back only `content` + `tool_calls`; for a model whose `content` is
// `null` on 66 of 67 turns, its entire contribution to its own history was the
// text of the shell commands it ran. Each turn then re-derived what the
// previous one had already worked out, inside the budget that truncated it.
func TestDogfoodCarriesReasoningBackToTheModel(t *testing.T) {
	details := `[{"type":"reasoning.text","text":"THINKING-CANARY","signature":"sig-abc"}]`
	tr := runFakeTrialEnv(t, []string{
		"FAKE_REASONING=THINKING-CANARY",
		"FAKE_REASONING_DETAILS=" + details,
	}, []string{"echo hello"}, "")

	msgs := sentAssistantMessages(t, tr.capture)
	if len(msgs) == 0 {
		t.Fatalf("no assistant message in the history at all")
	}
	first := msgs[0]
	// 🔴 `reasoning_details` VERBATIM, signature included. The blocks carry
	// provider signatures and a provider that validates them rejects anything
	// rebuilt from the flat text — so an implementation that re-serialised the
	// string would satisfy a test keyed only on the canary appearing somewhere.
	blob, _ := json.Marshal(first["reasoning_details"])
	if !strings.Contains(string(blob), "sig-abc") || !strings.Contains(string(blob), "THINKING-CANARY") {
		t.Fatalf("the assistant history dropped reasoning_details (got %s). The model's own "+
			"reasoning is not reaching its next turn.", blob)
	}
	if first["reasoning"] != "THINKING-CANARY" {
		t.Fatalf("the flat `reasoning` fallback was dropped: %v", first["reasoning"])
	}
	// It must still be an assistant turn with its tool call, not a reasoning
	// blob that lost the call it was attached to.
	if first["tool_calls"] == nil {
		t.Fatalf("carrying reasoning back dropped the turn's tool_calls: %v", first)
	}
}

// The escape hatch, because the history change is a real cost: prompt growth is
// already O(n^2) in steps, and a provider could reject an echoed block.
func TestDogfoodNoCarryReasoningOmitsIt(t *testing.T) {
	details := `[{"type":"reasoning.text","text":"THINKING-CANARY","signature":"sig-abc"}]`
	tr := runFakeTrialEnv(t, []string{
		"FAKE_REASONING=THINKING-CANARY",
		"FAKE_REASONING_DETAILS=" + details,
	}, []string{"echo hello"}, "", "--no-carry-reasoning")

	if strings.Contains(readFile(t, tr.capture), "THINKING-CANARY") {
		t.Fatalf("--no-carry-reasoning still sent the reasoning back:\n%s", readFile(t, tr.capture))
	}
}

// 🔴 "OTHERWISE UNCHANGED", PROVED RATHER THAN ASSERTED. A provider that
// returns no reasoning fields must produce exactly the assistant message the
// runner has always sent — `role` + `content`, plus `tool_calls` on a turn that
// made one, and nothing else. A carry-back implemented as "always add the keys,
// empty when absent" passes every test above and fails here, and it would
// re-base every already-measured cell's request shape.
func TestDogfoodAssistantHistoryIsUnchangedWithoutReasoning(t *testing.T) {
	tr := runFakeTrial(t, []string{"echo hello"}, "")
	for i, m := range sentAssistantMessages(t, tr.capture) {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		for _, k := range keys {
			if k != "role" && k != "content" && k != "tool_calls" {
				t.Fatalf("assistant message %d gained key %q when the provider returned no "+
					"reasoning: %v", i, k, keys)
			}
		}
	}
}

// ── the output ceiling ───────────────────────────────────────────────────────

// The `max_tokens` the runner actually sent, read off the captured payload.
func sentMaxTokens(t *testing.T, capture string) float64 {
	t.Helper()
	var c struct {
		Payload struct {
			MaxTokens float64 `json:"max_tokens"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(readFile(t, capture)), &c); err != nil {
		t.Fatalf("reading the capture: %v", err)
	}
	return c.Payload.MaxTokens
}

// 🔴 8000 LEFT THE MODEL 8 TOKENS AFTER ITS REASONING. The default is pinned
// here rather than derived from runner.py, because deriving it would make the
// test agree with whatever runner.py says — which is not a contract. It is a
// CEILING, not a spend cap: money is still bounded by --max-cost.
func TestDogfoodMaxTokensDefaultIsRaised(t *testing.T) {
	tr := runFakeTrial(t, nil, "")
	if got := sentMaxTokens(t, tr.capture); got != 32000 {
		t.Fatalf("the runner sent max_tokens=%v, want 32000. 8000 is the budget that produced "+
			"the ab-genpost-glm-01 truncation (7,992 of it reasoning).", got)
	}
}

func TestDogfoodMaxTokensIsOverridable(t *testing.T) {
	tr := runFakeTrial(t, nil, "", "--max-tokens", "1234")
	if got := sentMaxTokens(t, tr.capture); got != 1234 {
		t.Fatalf("--max-tokens 1234 sent max_tokens=%v", got)
	}
}

// The matrix entry point has to be able to set it too, or the flag is
// unreachable from the only script anyone runs a grid with.
func TestDogfoodDriverPassesMaxTokensThrough(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(dogfoodDir, "driver.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DOGFOOD_MAX_TOKENS", "--max-tokens"} {
		if !strings.Contains(string(src), want) {
			t.Fatalf("driver.sh cannot set the output ceiling (%q missing)", want)
		}
	}
}

// ── the run summary ──────────────────────────────────────────────────────────

// 🔴 THE ESCALATION WAS VISIBLE IN THE TRANSCRIPT AND INVISIBLE IN THE `.out`.
// `ab-genpost-glm-01`'s reasoning burn climbed 2,141 -> 4,238 -> 2,021 -> 7,992
// into the cap, and the summary an operator reads reported only cumulative
// totals — so the one signal that would have predicted the truncation was
// available and unread. A total cannot show an escalation; a per-turn row can.
func TestDogfoodSummaryCarriesPerTurnReasoningAndFinishReason(t *testing.T) {
	tr := runFakeTrialEnv(t, []string{"FAKE_REASONING_TOKENS=4238"},
		[]string{"echo one", "echo two"}, "")

	s := runSummary(t, tr.stdout)
	turns, _ := s["turns"].([]any)
	// Two tool-calling turns plus the terminal one.
	if len(turns) != 3 {
		t.Fatalf("the summary reports %d turn(s), want 3 — one row per assistant turn:\n%s",
			len(turns), tr.stdout)
	}
	for i, raw := range turns {
		row, _ := raw.(map[string]any)
		if row["reasoning_tokens"] != float64(4238) {
			t.Fatalf("turn %d carries reasoning_tokens=%v, want 4238", i, row["reasoning_tokens"])
		}
		if row["finish_reason"] == nil || row["finish_reason"] == "" {
			t.Fatalf("turn %d carries no finish_reason: %v", i, row)
		}
	}
	if s["reasoning_tokens"] != float64(3*4238) {
		t.Fatalf("the summary's reasoning total is %v, want %d", s["reasoning_tokens"], 3*4238)
	}
	// The terminal turn's reason is the one the run is graded on, so it is on
	// the summary in its own right and not only inside the rows.
	if s["finish_reason"] != "stop" {
		t.Fatalf("the summary's finish_reason is %v, want \"stop\"", s["finish_reason"])
	}
}

// 🔴 A `stop` VALUE IS READ BY PEOPLE, NOT BY grade.sh/oracle.sh, SO WIDENING
// THE VOCABULARY CANNOT CHANGE HOW A CELL GRADES. That is a claim about those
// two files, so it is pinned here rather than asserted in a commit message.
//
// ⚠ THIS GUARD USED TO BE "neither grader reads transcript.jsonl at all", AND
// THAT IS NO LONGER TRUE — deliberately. oracle.sh reads the `start` record to
// derive WHICH BRIEF a trial was run with (see its "WHICH BRIEF" section): the
// old `${3:-celsius}` default graded a `genpost` trial against the `celsius`
// assertion and printed the same `RENDER=no` a model that built nothing earns.
// It fired here first, which is what this guard is for; the claim was then
// narrowed to the one its name makes rather than deleted.
//
// Three halves, and each covers a route the others do not:
//   - no grader mentions a `stop` VALUE, so none can compare against a literal;
//   - no grader selects the `end` record, where every `stop` value lives — that
//     covers a grader that read one into a variable and compared indirectly;
//   - oracle.sh's one transcript read IS the `start` record, which is what makes
//     the second bullet a structural property and not a spelling.
func TestGradersDoNotReadTheStopVocabulary(t *testing.T) {
	// Deliberately NOT the bare word "finished": grade.sh's own header says
	// "Grade one finished trial", which is English rather than a `stop` value.
	vocab := []string{"truncated", "empty-reply", "stopped-unknown", "max-steps", "finish_reason"}
	src := map[string]string{}
	for _, name := range []string{"grade.sh", "oracle.sh"} {
		raw, err := os.ReadFile(filepath.Join(dogfoodDir, name))
		if err != nil {
			t.Fatal(err)
		}
		src[name] = string(raw)
		for _, v := range vocab {
			if strings.Contains(src[name], v) {
				t.Errorf("%s mentions the `stop` value %q. The vocabulary widened "+
					"(finished / truncated / empty-reply / stopped-unknown:*) — check what a "+
					"grader branching on it does to a cell before keeping this.", name, v)
			}
		}
		for _, sel := range []string{`kind=="end"`, `"kind": "end"`, `.stop`} {
			if strings.Contains(src[name], sel) {
				t.Errorf("%s selects the transcript's `end` record (%q), which is where every "+
					"`stop` value lives. A grader must score the CONTAINER, not what the model "+
					"said about itself.", name, sel)
			}
		}
	}
	if strings.Contains(src["grade.sh"], "transcript.jsonl") {
		t.Errorf("grade.sh now reads the transcript. Only oracle.sh does, and only the `start` " +
			"record — see the note above this test.")
	}
	// The positive half. Without it the `end`-selector ban above is a claim
	// about spelling: a grader that streamed the whole file would pass it.
	if !strings.Contains(src["oracle.sh"], `select(.kind=="start")`) {
		t.Errorf("oracle.sh no longer reads the transcript by selecting the `start` record. " +
			"Whatever it reads instead may reach the `stop` vocabulary, which this guard can " +
			"no longer rule out.")
	}
}
