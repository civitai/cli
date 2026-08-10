package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/genapi"
)

// The --dry-run readiness line is a LABEL problem, not a logic one (#279).
//
// The server's `ready` flag is computed as "every job's queuePosition.support is
// available", with a job carrying no queuePosition SKIPPED — a resource-capacity
// answer, not a prediction that the job produces anything. Printed as
// `Generatable` it read as a success predicate: 8 submits across 3 checkpoints
// that every one quoted `ready: true` produced 0 outputs. See AGENTS.md item 28.

// TestDryRun_ReadinessLabelIsNotAGeneratabilityClaim is PAIRED on purpose.
//
// 🔴 A bare Contains on the new label passes while the old one is still printed
// — a two-line renderer satisfies it. So the new label must be PRESENT and the
// retracted one ABSENT, in the same rendered output, and the value must still be
// the server's own boolean rather than a re-derived one.
func TestDryRun_ReadinessLabelIsNotAGeneratabilityClaim(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	o := baseOpts()
	o.dryRun = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run: %v", err)
	}
	if s.submitCalls != 0 {
		t.Fatalf("--dry-run submitted %d time(s)", s.submitCalls)
	}
	got := out.String()
	if got == "" {
		t.Fatal("CONTROL failure: --dry-run printed nothing to stdout, so every assertion below is vacuous")
	}

	if !strings.Contains(got, "Resources ready:") {
		t.Errorf("the readiness line is missing its new label:\n%s", got)
	}
	// The retracted label, matched case-insensitively AND without the colon: a
	// guard keyed on "generatable:" is satisfied by prose that spells the same
	// equation without one, which is how the ready:false warning kept saying it.
	if strings.Contains(strings.ToLower(got), "generatable") {
		t.Errorf("the quote still prints a `Generatable` label. `ready` reports resource availability, not that the job "+
			"will produce output — 8 submits across 3 `ready: true` checkpoints produced 0 outputs (#279, AGENTS.md item 28):\n%s", got)
	}
	// The value is still the server's flag, not something the CLI decided.
	if !strings.Contains(got, "Resources ready:") || !strings.Contains(got, "true") {
		t.Errorf("the readiness line no longer carries the server's boolean:\n%s", got)
	}

	_ = errb
}

// notReadyQuoteSeam returns a whatIf seam quoting a priced-but-not-ready job.
func notReadyQuoteSeam() func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
	return func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q := okQuote(12)
		q.Ready = false
		return q, json.RawMessage(`{"ready":false,"cost":{"base":8,"total":12}}`), nil
	}
}

// TestDryRun_NotReadyStillWarns is the other direction of the same contract.
// `ready: false` is decisive and must stay loud; #279 only removed the claim in
// the TRUE direction.
func TestDryRun_NotReadyStillWarns(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	s.whatIf = notReadyQuoteSeam()
	o := baseOpts()
	o.dryRun = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run with ready:false: %v", err)
	}
	// The quote goes through a tabwriter, so the separator is run-length padding
	// and not a literal tab. Matching either spelling was a DEAD BRANCH — the
	// `\t` arm can never fire — and a dead arm in an OR reads as thoroughness
	// while contributing nothing. Collapse whitespace and assert once.
	if !strings.Contains(collapseWS(out.String()), "Resources ready: false") {
		t.Errorf("the readiness line should report the server's false:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "NOT currently available") {
		t.Errorf("a ready:false quote must still warn — that direction is the one the CLI acts on:\n%s", errb.String())
	}
	// 🔴 …and the warning must not re-teach the equation the rename breaks.
	// Printing "not currently generatable" one line under `Resources ready:
	// false` hands the reader back exactly the word the label dropped, and the
	// label ban is on `"generatable:"` WITH a colon, so it would not notice.
	for _, stream := range []struct{ name, text string }{{"stdout", out.String()}, {"stderr", errb.String()}} {
		if strings.Contains(strings.ToLower(stream.text), "generatable") {
			t.Errorf("--dry-run %s still says \"generatable\", which is the equation #279 exists to break:\n%s",
				stream.name, stream.text)
		}
	}
}

// collapseWS reduces every whitespace run to a single space, so an assertion
// about a tabwriter-formatted line does not depend on column padding.
func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// 🔴 THE WORD-BAN ABOVE WAS UNREACHABLE ON THE ONE PATH THAT COULD REVIVE IT.
//
// `substitutionAdvice` said "a community checkpoint that was never generatable",
// and `reportModelSubstitutions` prints on the ESTIMATE path — so that sentence
// lands in `--dry-run` stderr a few lines from `Resources ready`. The ban in
// TestDryRun_NotReadyStillWarns never saw it, because that fixture reports no
// substitutions: an audit mutant re-adding the word there SURVIVED the whole
// suite. A guard that cannot be reached by the case it forbids is not a guard.
//
// This drives the substitution path explicitly. It is a REACHABILITY test, and
// simultaneously the false-positive control: a legitimate substitution notice
// must pass the ban, so over-broadening it fails here on real rendered output
// rather than being discovered by a user.
func TestDryRun_SubstitutionNoticeAvoidsTheGeneratableEquation(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	s.whatIf = func(context.Context, genapi.Graph) (*genapi.WhatIfResult, json.RawMessage, error) {
		q := okQuote(12)
		q.ModelSubstitutions = []genapi.ModelSubstitution{
			{Requested: 111, Applied: 222, Reason: genapi.SubstitutionUnrecognized},
		}
		return q, okQuoteRaw(12), nil
	}
	o := baseOpts()
	o.dryRun = true

	c, out, errb := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run with a substitution: %v", err)
	}
	// POSITIVE CONTROL first: without this, a fixture that silently stopped
	// reporting substitutions would make the ban below vacuous all over again —
	// the exact failure this test was written to end.
	if !strings.Contains(errb.String(), "222") {
		t.Fatalf("CONTROL failure: no substitution notice was rendered, so the word-ban below never executed:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), "never offered for generation") {
		t.Fatalf("CONTROL failure: the UNRECOGNISED advice line is not the one being checked:\n%s", errb.String())
	}
	for _, stream := range []struct{ name, text string }{{"stdout", out.String()}, {"stderr", errb.String()}} {
		if strings.Contains(strings.ToLower(stream.text), "generatable") {
			t.Errorf("--dry-run %s says \"generatable\" on the substitution path. Three senses of that word once shared "+
				"this screen; #279 removed it from the readiness label and the ready:false warning, and it must not come "+
				"back through the substitution advice (AGENTS.md item 28).\n%s", stream.name, stream.text)
		}
	}
}

// 🔴 WIRE SAFETY. The rename is a HUMAN-renderer change; `--dry-run --json`
// emits the server's payload untouched, so the machine key must still be the raw
// `ready`. This is asserted on a DECODED map, never with strings.Contains: the
// point is the KEY, and a text search cannot tell `ready` from a label that
// happens to contain it.
func TestDryRunJSON_RawReadyKeySurvivesTheRename(t *testing.T) {
	withStdinTTY(t, false)
	var s genSeams
	o := baseOpts()
	o.dryRun, o.jsonOut = true, true

	c, out, _ := genCmd("")
	if err := runGenerate(c, s.deps(t), o); err != nil {
		t.Fatalf("--dry-run --json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout is not pure JSON (%v):\n%s", err, out.String())
	}
	v, ok := m["ready"]
	if !ok {
		t.Fatalf("the raw `ready` key is gone from --json. The rename was supposed to touch the human label only:\n%v", m)
	}
	if v != true {
		t.Errorf("`ready` decoded as %#v, want the server's true — the value must pass through, not be re-derived", v)
	}
	// And the human label must NOT have leaked onto the machine surface.
	for k := range m {
		if strings.EqualFold(k, "resources ready") || strings.EqualFold(k, "resourcesReady") {
			t.Errorf("the human label leaked into --json as key %q", k)
		}
	}
}

// The --help text has to carry the caveat, because the label alone cannot say
// what `ready` does and does not mean.
func TestGenerateHelp_SaysReadinessIsNotAPromiseOfOutput(t *testing.T) {
	long := newGenerateCmd().Long
	if len(long) < 500 {
		t.Fatalf("CONTROL failure: the generate Long text is %d bytes, far too short to be the real help — "+
			"the assertions below would pass on an empty string", len(long))
	}
	lower := strings.ToLower(long)
	for _, want := range []string{
		"resources ready",
		"not a promise of output",
	} {
		if !strings.Contains(lower, want) {
			t.Errorf("`civitai generate --help` no longer says %q. Without it the label is terse but unexplained, "+
				"and a reader fills the gap with the meaning the old label implied (AGENTS.md item 28).\n---\n%s", want, long)
		}
	}
}
